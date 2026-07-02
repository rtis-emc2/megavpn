package postgres

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/platform/id"
)

const (
	defaultClientSubscriptionTTL = 30 * 24 * time.Hour
	maxClientSubscriptionTTL     = 365 * 24 * time.Hour
)

func (s *Store) ListClientSubscriptions(ctx context.Context, clientID string) ([]domain.ClientSubscription, error) {
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return nil, errors.New("client id is required")
	}
	rows, err := s.db.Query(ctx, `select id,client_account_id,coalesce(token_hint,''),status,expires_at,download_count,last_used_at,created_at,updated_at
		from client_subscriptions
		where client_account_id=$1
		order by created_at desc`, clientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.ClientSubscription, 0)
	for rows.Next() {
		var sub domain.ClientSubscription
		if err := rows.Scan(&sub.ID, &sub.ClientAccountID, &sub.TokenHint, &sub.Status, &sub.ExpiresAt, &sub.DownloadCount, &sub.LastUsedAt, &sub.CreatedAt, &sub.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, sub)
	}
	return out, rows.Err()
}

func (s *Store) RotateClientSubscription(ctx context.Context, clientID string, ttl time.Duration) (domain.ClientSubscription, error) {
	client, err := s.GetClient(ctx, clientID)
	if err != nil {
		return domain.ClientSubscription{}, err
	}
	if client.Status != "active" {
		return domain.ClientSubscription{}, fmt.Errorf("client subscription requires active client status, got %s", client.Status)
	}
	if client.ExpiresAt != nil && client.ExpiresAt.Before(time.Now().UTC()) {
		return domain.ClientSubscription{}, errors.New("client is expired")
	}
	if ttl <= 0 {
		ttl = defaultClientSubscriptionTTL
	}
	if ttl > maxClientSubscriptionTTL {
		return domain.ClientSubscription{}, fmt.Errorf("client subscription ttl exceeds maximum %s", maxClientSubscriptionTTL)
	}
	token := randomToken(32)
	now := time.Now().UTC()
	sub := domain.ClientSubscription{
		ID:              id.New(),
		ClientAccountID: client.ID,
		Token:           token,
		TokenHint:       tokenHint(token),
		Status:          "active",
		ExpiresAt:       now.Add(ttl),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return domain.ClientSubscription{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `update client_subscriptions
		set status='revoked',updated_at=now()
		where client_account_id=$1 and status='active'`, client.ID); err != nil {
		return domain.ClientSubscription{}, err
	}
	if _, err := tx.Exec(ctx, `insert into client_subscriptions(id,client_account_id,token_hash,token_hint,status,expires_at,created_at,updated_at)
		values($1,$2,$3,$4,$5,$6,$7,$8)`,
		sub.ID, sub.ClientAccountID, hashToken(token), sub.TokenHint, sub.Status, sub.ExpiresAt, sub.CreatedAt, sub.UpdatedAt); err != nil {
		return domain.ClientSubscription{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.ClientSubscription{}, err
	}
	_, _ = s.CreateAudit(ctx, "system", "client_subscription.rotate", "client", &client.ID, "client VLESS subscription token rotated")
	return sub, nil
}

func (s *Store) RevokeClientSubscription(ctx context.Context, clientID, subscriptionID string) (domain.ClientSubscription, error) {
	clientID = strings.TrimSpace(clientID)
	subscriptionID = strings.TrimSpace(subscriptionID)
	if clientID == "" || subscriptionID == "" {
		return domain.ClientSubscription{}, errors.New("client id and subscription id are required")
	}
	var sub domain.ClientSubscription
	err := s.db.QueryRow(ctx, `update client_subscriptions
		set status='revoked',updated_at=now()
		where id=$1 and client_account_id=$2 and status in ('active','expired')
		returning id,client_account_id,coalesce(token_hint,''),status,expires_at,download_count,last_used_at,created_at,updated_at`,
		subscriptionID, clientID).
		Scan(&sub.ID, &sub.ClientAccountID, &sub.TokenHint, &sub.Status, &sub.ExpiresAt, &sub.DownloadCount, &sub.LastUsedAt, &sub.CreatedAt, &sub.UpdatedAt)
	if err != nil {
		return domain.ClientSubscription{}, err
	}
	_, _ = s.CreateAudit(ctx, "system", "client_subscription.revoke", "client", &clientID, "client VLESS subscription token revoked")
	return sub, nil
}

func (s *Store) ResolveClientVLESSSubscription(ctx context.Context, token string) (domain.ClientSubscriptionDocument, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return domain.ClientSubscriptionDocument{}, errors.New("subscription token is required")
	}
	var sub domain.ClientSubscription
	var client domain.Client
	err := s.db.QueryRow(ctx, `select
			cs.id,cs.client_account_id,coalesce(cs.token_hint,''),cs.status,cs.expires_at,cs.download_count,cs.last_used_at,cs.created_at,cs.updated_at,
			ca.id,ca.username,coalesce(ca.display_name,''),coalesce(ca.email,''),ca.status,coalesce(ca.notes,''),ca.expires_at,ca.created_at,ca.updated_at
		from client_subscriptions cs
		join client_accounts ca on ca.id=cs.client_account_id
		where cs.token_hash=$1`, hashToken(token)).
		Scan(
			&sub.ID,
			&sub.ClientAccountID,
			&sub.TokenHint,
			&sub.Status,
			&sub.ExpiresAt,
			&sub.DownloadCount,
			&sub.LastUsedAt,
			&sub.CreatedAt,
			&sub.UpdatedAt,
			&client.ID,
			&client.Username,
			&client.DisplayName,
			&client.Email,
			&client.Status,
			&client.Notes,
			&client.ExpiresAt,
			&client.CreatedAt,
			&client.UpdatedAt,
		)
	if err != nil {
		return domain.ClientSubscriptionDocument{}, err
	}
	now := time.Now().UTC()
	if sub.Status != "active" {
		return domain.ClientSubscriptionDocument{}, errors.New("subscription token is not active")
	}
	if sub.ExpiresAt.Before(now) {
		_, _ = s.db.Exec(ctx, `update client_subscriptions set status='expired',updated_at=now() where id=$1 and status='active'`, sub.ID)
		return domain.ClientSubscriptionDocument{}, errors.New("subscription token has expired")
	}
	if client.Status != "active" {
		return domain.ClientSubscriptionDocument{}, errors.New("client is not active")
	}
	if client.ExpiresAt != nil && client.ExpiresAt.Before(now) {
		return domain.ClientSubscriptionDocument{}, errors.New("client is expired")
	}
	records, err := s.ListProvisioningAccesses(ctx, client.ID)
	if err != nil {
		return domain.ClientSubscriptionDocument{}, err
	}
	profiles := make([]domain.ClientSubscriptionProfile, 0)
	for _, record := range records {
		profile, ok := clientVLESSSubscriptionProfile(record)
		if ok {
			profiles = append(profiles, profile)
		}
	}
	_, _ = s.db.Exec(ctx, `update client_subscriptions set download_count=download_count+1,last_used_at=now(),updated_at=now() where id=$1`, sub.ID)
	sub.DownloadCount++
	sub.LastUsedAt = &now
	sub.UpdatedAt = now
	return domain.ClientSubscriptionDocument{
		Subscription: sub,
		Client:       client,
		Profiles:     profiles,
		GeneratedAt:  now,
	}, nil
}

func clientVLESSSubscriptionProfile(record domain.ProvisioningAccess) (domain.ClientSubscriptionProfile, bool) {
	if strings.TrimSpace(record.Access.Status) != "active" {
		return domain.ClientSubscriptionProfile{}, false
	}
	if strings.TrimSpace(record.Instance.ServiceCode) != "xray-core" {
		return domain.ClientSubscriptionProfile{}, false
	}
	if !record.Instance.Enabled || strings.TrimSpace(record.Instance.Status) != "active" {
		return domain.ClientSubscriptionProfile{}, false
	}
	uri, outboundGroup, ok := buildClientVLESSSubscriptionURI(record)
	if !ok {
		return domain.ClientSubscriptionProfile{}, false
	}
	return domain.ClientSubscriptionProfile{
		ServiceAccessID: record.Access.ID,
		InstanceID:      record.Instance.ID,
		InstanceName:    record.Instance.Name,
		InstanceSlug:    record.Instance.Slug,
		NodeID:          record.Instance.NodeID,
		ServiceCode:     record.Instance.ServiceCode,
		EndpointHost:    record.Instance.EndpointHost,
		EndpointPort:    record.Instance.EndpointPort,
		OutboundGroup:   outboundGroup,
		URI:             uri,
	}, true
}

func buildClientVLESSSubscriptionURI(record domain.ProvisioningAccess) (string, string, bool) {
	spec := record.Instance.Spec
	if spec == nil {
		spec = map[string]any{}
	}
	meta := record.Access.Metadata
	if meta == nil {
		meta = map[string]any{}
	}
	uuid := firstString(meta["xray_uuid"], meta["uuid"])
	if uuid == "" {
		return "", "", false
	}
	host := firstString(meta["public_host"], meta["server_host"], spec["public_host"], spec["server_host"], record.Instance.EndpointHost)
	if host == "" {
		return "", "", false
	}
	port := firstIntValue(meta["public_port"], meta["server_port"], spec["public_port"], spec["server_port"], spec["port"], record.Instance.EndpointPort)
	if port <= 0 {
		port = 443
	}
	security := firstString(meta["public_security"], meta["security"], spec["public_security"], spec["security"])
	if security == "" {
		security = "reality"
	}
	query := url.Values{}
	query.Set("encryption", "none")
	query.Set("security", security)
	if flow := firstString(meta["public_flow"], meta["flow"], spec["public_flow"], spec["flow"]); flow != "" {
		query.Set("flow", flow)
	}
	if sni := firstString(meta["public_sni"], meta["sni"], spec["public_sni"], spec["sni"], spec["public_server_name"], spec["server_name"], host); sni != "" {
		query.Set("sni", sni)
	}
	if fp := firstString(meta["public_fingerprint"], meta["fingerprint"], spec["public_fingerprint"], spec["fingerprint"]); fp != "" || security == "reality" {
		if fp == "" {
			fp = "chrome"
		}
		query.Set("fp", fp)
	}
	if pbk := firstString(meta["reality_public_key"], spec["reality_public_key"], spec["public_key"], spec["pbk"]); pbk != "" && security == "reality" {
		query.Set("pbk", pbk)
	}
	if sid := firstString(meta["short_id"], spec["short_id"], spec["sid"]); sid != "" && security == "reality" {
		query.Set("sid", sid)
	}
	network := firstString(meta["public_network"], meta["type"], spec["public_network"], spec["type"], spec["transport"], spec["network"])
	if network == "" {
		network = "tcp"
	}
	query.Set("type", network)
	if path := firstString(meta["public_path"], meta["path"], spec["public_path"], spec["path"]); path != "" {
		query.Set("path", path)
	}
	if serviceName := firstString(meta["public_service_name"], meta["service_name"], spec["public_service_name"], spec["service_name"]); serviceName != "" {
		query.Set("serviceName", serviceName)
	}
	if hostHeader := firstString(meta["public_host_header"], spec["public_host_header"]); hostHeader != "" {
		query.Set("host", hostHeader)
	}
	if alpn := firstString(meta["public_alpn"], meta["alpn"], spec["public_alpn"], spec["alpn"]); alpn != "" {
		query.Set("alpn", alpn)
	}
	inbound, _ := meta["inbound_service"].(map[string]any)
	outboundGroup := firstString(
		meta["vless_group"],
		meta["xray_group"],
		meta["outbound_group"],
		inbound["vless_group"],
		inbound["xray_group"],
		inbound["outbound_group"],
		spec["default_vless_group"],
	)
	if outboundGroup == "" {
		outboundGroup = "default"
	}
	labelBase := firstString(spec["client_label_prefix"], record.Instance.Name, record.Instance.Slug, "xray")
	clientLabel := firstString(record.Client.Username, record.Client.DisplayName, record.Access.ID)
	label := url.QueryEscape(labelBase + "-" + clientLabel)
	return fmt.Sprintf("vless://%s@%s:%d?%s#%s", uuid, host, port, query.Encode(), label), outboundGroup, true
}
