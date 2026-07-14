package postgres

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/rtis-emc2/megavpn/internal/domain"
)

func (s *Store) UpdateClient(ctx context.Context, clientID string, input domain.ClientUpdate) (domain.Client, error) {
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return domain.Client{}, errors.New("client id is required")
	}
	if _, err := s.GetClient(ctx, clientID); err != nil {
		return domain.Client{}, err
	}
	if input.DisplayName == nil && input.Email == nil && input.Notes == nil && input.ExpiresAt == nil && !input.ClearExpires {
		return domain.Client{}, errors.New("at least one editable client field is required")
	}
	displayName, err := normalizeEditableClientString(input.DisplayName, 200, "display_name")
	if err != nil {
		return domain.Client{}, err
	}
	email := normalizeEditableClientEmail(input.Email)
	notes, err := normalizeEditableClientString(input.Notes, 4000, "notes")
	if err != nil {
		return domain.Client{}, err
	}
	if input.Email != nil && *email != "" && !strings.Contains(*email, "@") {
		return domain.Client{}, errors.New("email must be a valid address")
	}
	if input.ExpiresAt != nil && input.ExpiresAt.Before(time.Now().UTC().Add(-time.Minute)) {
		return domain.Client{}, errors.New("expires_at must be in the future")
	}

	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.Client{}, err
	}
	defer tx.Rollback(ctx)

	var current domain.Client
	err = tx.QueryRow(ctx, `select id,username,coalesce(display_name,''),coalesce(email,''),status,coalesce(notes,''),expires_at,created_at,updated_at
		from client_accounts
		where id=$1 and status <> 'deleted'
		for update`, clientID).
		Scan(&current.ID, &current.Username, &current.DisplayName, &current.Email, &current.Status, &current.Notes, &current.ExpiresAt, &current.CreatedAt, &current.UpdatedAt)
	if err != nil {
		return domain.Client{}, err
	}
	next := current
	if displayName != nil {
		next.DisplayName = *displayName
	}
	if email != nil {
		next.Email = *email
	}
	if notes != nil {
		next.Notes = *notes
	}
	if input.ClearExpires {
		next.ExpiresAt = nil
	} else if input.ExpiresAt != nil {
		expiresAt := input.ExpiresAt.UTC()
		next.ExpiresAt = &expiresAt
	}
	if _, err := tx.Exec(ctx, `update client_accounts
		set display_name=nullif($2,''),
		    email=nullif($3,''),
		    notes=nullif($4,''),
		    expires_at=$5,
		    updated_at=now()
		where id=$1`, clientID, next.DisplayName, next.Email, next.Notes, next.ExpiresAt); err != nil {
		return domain.Client{}, err
	}
	if _, err := tx.Exec(ctx, `insert into audit_events(id,actor_type,action,resource_type,resource_id,summary,payload_json,created_at)
		values(gen_random_uuid(),'system','client.update','client',$1,'client metadata updated',$2,now())`,
		clientID,
		mustJSON(map[string]any{
			"changed_fields": clientChangedFields(current, next),
		}),
	); err != nil {
		return domain.Client{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Client{}, err
	}
	return s.GetClient(ctx, clientID)
}

func normalizeEditableClientString(value *string, maxRunes int, field string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	normalized := strings.TrimSpace(*value)
	if maxRunes > 0 && utf8.RuneCountInString(normalized) > maxRunes {
		return nil, errors.New(field + " is too long")
	}
	return &normalized, nil
}

func normalizeEditableClientEmail(value *string) *string {
	if value == nil {
		return nil
	}
	normalized := strings.ToLower(strings.TrimSpace(*value))
	return &normalized
}

func clientChangedFields(before, after domain.Client) []string {
	out := make([]string, 0, 4)
	if before.DisplayName != after.DisplayName {
		out = append(out, "display_name")
	}
	if before.Email != after.Email {
		out = append(out, "email")
	}
	if before.Notes != after.Notes {
		out = append(out, "notes")
	}
	beforeExpires := ""
	afterExpires := ""
	if before.ExpiresAt != nil {
		beforeExpires = before.ExpiresAt.UTC().Format(time.RFC3339Nano)
	}
	if after.ExpiresAt != nil {
		afterExpires = after.ExpiresAt.UTC().Format(time.RFC3339Nano)
	}
	if beforeExpires != afterExpires {
		out = append(out, "expires_at")
	}
	return out
}

func (s *Store) UpdateClientAccessRoute(ctx context.Context, clientID, routeID string, route domain.ClientAccessRoute) (domain.ClientAccessRoute, error) {
	clientID = strings.TrimSpace(clientID)
	routeID = strings.TrimSpace(routeID)
	if clientID == "" || routeID == "" {
		return domain.ClientAccessRoute{}, errors.New("client_id and route_id are required")
	}
	if route.ID != "" && route.ID != routeID {
		return domain.ClientAccessRoute{}, errors.New("route id is immutable")
	}
	if route.ClientAccountID != "" && route.ClientAccountID != clientID {
		return domain.ClientAccessRoute{}, errors.New("client_account_id is immutable")
	}
	current, err := s.GetClientAccessRoute(ctx, clientID, routeID)
	if err != nil {
		return domain.ClientAccessRoute{}, err
	}
	if isBaselineRoute(current) {
		return domain.ClientAccessRoute{}, errors.New("baseline service route cannot be edited; create an explicit route instead")
	}
	route.ID = routeID
	route.ClientAccountID = clientID
	route.CreatedAt = current.CreatedAt
	if route.Status == "" {
		route.Status = current.Status
	}
	if route.Policy == nil {
		route.Policy = current.Policy
	}
	if route.Metadata == nil {
		route.Metadata = current.Metadata
	}
	if err := s.hydrateClientAccessRouteRefs(ctx, &route); err != nil {
		return domain.ClientAccessRoute{}, err
	}
	if route.NodeID == nil || strings.TrimSpace(*route.NodeID) == "" {
		return domain.ClientAccessRoute{}, errors.New("route must target a service access, instance or node")
	}
	if err := normalizeClientAccessRoute(&route); err != nil {
		return domain.ClientAccessRoute{}, err
	}
	if err := s.ensureClientAccessRouteNotDuplicate(ctx, routeID, route); err != nil {
		return domain.ClientAccessRoute{}, err
	}
	if _, err := s.db.Exec(ctx, `update client_access_routes
		set service_access_id=$3,
		    instance_id=$4,
		    node_id=$5,
		    name=$6,
		    status=$7,
		    action=$8,
		    destination_type=$9,
		    destination=$10,
		    protocol=$11,
		    ports=$12,
		    description=nullif($13,''),
		    policy_json=$14,
		    metadata_json=$15,
		    updated_at=now()
		where client_account_id=$1 and id=$2`,
		clientID,
		routeID,
		route.ServiceAccessID,
		route.InstanceID,
		route.NodeID,
		route.Name,
		route.Status,
		route.Action,
		route.DestinationType,
		route.Destination,
		route.Protocol,
		route.Ports,
		route.Description,
		mustJSON(route.Policy),
		mustJSON(route.Metadata),
	); err != nil {
		return domain.ClientAccessRoute{}, err
	}
	_, _ = s.CreateAudit(ctx, "system", "client.route.update", "client", &clientID, "client access route updated")
	updated, err := s.GetClientAccessRoute(ctx, clientID, routeID)
	if err == nil {
		err = s.queueRoutePolicyForRoute(ctx, updated)
	}
	return updated, err
}

func (s *Store) ensureClientAccessRouteNotDuplicate(ctx context.Context, routeID string, route domain.ClientAccessRoute) error {
	var count int
	err := s.db.QueryRow(ctx, `select count(*)
		from client_access_routes
		where client_account_id=$1
		  and id <> $2
		  and status <> 'revoked'
		  and coalesce(service_access_id::text,'') = coalesce($3::text,'')
		  and coalesce(instance_id::text,'') = coalesce($4::text,'')
		  and coalesce(node_id::text,'') = coalesce($5::text,'')
		  and action=$6
		  and destination_type=$7
		  and destination=$8
		  and protocol=$9
		  and ports=$10`,
		route.ClientAccountID,
		routeID,
		derefString(route.ServiceAccessID),
		derefString(route.InstanceID),
		derefString(route.NodeID),
		route.Action,
		route.DestinationType,
		route.Destination,
		route.Protocol,
		route.Ports,
	).Scan(&count)
	if err != nil {
		return err
	}
	if count > 0 {
		return errors.New("duplicate client route")
	}
	return nil
}

func (s *Store) RevokeClientServiceAccess(ctx context.Context, clientID, accessID string) (domain.ClientServiceAccessRevokeResult, error) {
	clientID = strings.TrimSpace(clientID)
	accessID = strings.TrimSpace(accessID)
	out := domain.ClientServiceAccessRevokeResult{ClientID: clientID, ServiceAccessID: accessID}
	if clientID == "" || accessID == "" {
		return out, errors.New("client id and service access id are required")
	}
	if _, err := s.GetClient(ctx, clientID); err != nil {
		return out, err
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)

	var status string
	err = tx.QueryRow(ctx, `select instance_id::text,status from service_accesses where client_account_id=$1 and id=$2 for update`, clientID, accessID).Scan(&out.InstanceID, &status)
	if err != nil {
		return out, err
	}
	instanceIDs, err := activeServiceAccessInstanceIDsTx(ctx, tx, accessID)
	if err != nil {
		return out, err
	}
	nodeIDs, err := serviceAccessRouteNodeIDsTx(ctx, tx, clientID, accessID)
	if err != nil {
		return out, err
	}
	if strings.EqualFold(status, "revoked") {
		out.AlreadyRevoked = true
		out.Revoked = true
		if err := tx.Commit(ctx); err != nil {
			return out, err
		}
		return out, nil
	}
	if _, err := tx.Exec(ctx, `update service_accesses set status='revoked',updated_at=now() where client_account_id=$1 and id=$2`, clientID, accessID); err != nil {
		return out, err
	}
	tag, err := tx.Exec(ctx, `update client_access_routes set status='revoked',updated_at=now() where client_account_id=$1 and service_access_id=$2 and status <> 'revoked'`, clientID, accessID)
	if err != nil {
		return out, err
	}
	out.AccessRoutesRevoked = tag.RowsAffected()
	tag, err = tx.Exec(ctx, `update share_links set status='revoked' where client_account_id=$1 and status='active' and target_type='artifact' and target_id in (
		select id from artifacts where client_account_id=$1 and service_access_id=$2
	)`, clientID, accessID)
	if err != nil {
		return out, err
	}
	out.ShareLinksRevoked = tag.RowsAffected()
	if _, err := tx.Exec(ctx, `insert into audit_events(id,actor_type,action,resource_type,resource_id,summary,payload_json,created_at)
		values(gen_random_uuid(),'system','service_access.revoke','service_access',$1,'service access revoked',$2,now())`,
		accessID,
		mustJSON(map[string]any{"client_id": clientID, "instance_id": out.InstanceID}),
	); err != nil {
		return out, err
	}
	if err := tx.Commit(ctx); err != nil {
		return out, err
	}
	out.Revoked = true
	out.InstanceApplyJobsQueued, out.RoutePolicyJobsQueued, out.QueueErrors = s.queueClientCleanupConvergence(ctx, instanceIDs, nodeIDs)
	return out, nil
}

func (s *Store) ListClientEmailDeliveries(ctx context.Context, clientID string, limit, offset int) ([]domain.ClientDeliveryHistoryItem, error) {
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return nil, errors.New("client id is required")
	}
	if _, err := s.GetClient(ctx, clientID); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.db.Query(ctx, `select
		id::text,
		client_account_id::text,
		email,
		status,
		artifact_ids,
		share_link_ids,
		error_text,
		created_by::text,
		sent_at,
		created_at
	from client_email_deliveries
	where client_account_id=$1
	order by created_at desc, id desc
	limit $2 offset $3`, clientID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.ClientDeliveryHistoryItem{}
	for rows.Next() {
		var item domain.ClientDeliveryHistoryItem
		var email, status, errorText string
		var artifactRaw, shareRaw []byte
		if err := rows.Scan(&item.ID, &item.ClientAccountID, &email, &status, &artifactRaw, &shareRaw, &errorText, &item.CreatedBy, &item.SentAt, &item.CreatedAt); err != nil {
			return nil, err
		}
		artifactIDs := []string{}
		shareLinkIDs := []string{}
		if err := decodeJSONField(artifactRaw, &artifactIDs, "client_email_deliveries.artifact_ids"); err != nil {
			return nil, err
		}
		if err := decodeJSONField(shareRaw, &shareLinkIDs, "client_email_deliveries.share_link_ids"); err != nil {
			return nil, err
		}
		item.DeliveryType = "client_access_email"
		item.Channel = "email"
		item.DestinationHint = maskDeliveryEmail(email)
		item.Status = status
		item.ArtifactCount = len(artifactIDs)
		item.ShareLinkCount = len(shareLinkIDs)
		item.RelatedArtifactIDs = artifactIDs
		item.RelatedShareLinkIDs = shareLinkIDs
		item.SafeErrorSummary = safeDeliveryErrorSummary(errorText)
		if item.Status == "sent" {
			item.CompletedAt = item.SentAt
		}
		if item.Status == "failed" {
			failedAt := item.CreatedAt
			if item.SentAt != nil {
				failedAt = *item.SentAt
			}
			item.FailedAt = &failedAt
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func maskDeliveryEmail(email string) string {
	email = strings.TrimSpace(email)
	if email == "" {
		return "internal delivery"
	}
	local, domainPart, ok := strings.Cut(email, "@")
	if !ok || strings.TrimSpace(domainPart) == "" {
		return "masked recipient"
	}
	localRunes := []rune(local)
	if len(localRunes) == 0 {
		return "***@" + domainPart
	}
	return string(localRunes[0]) + "***@" + domainPart
}

func safeDeliveryErrorSummary(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	if utf8.RuneCountInString(value) > 240 {
		value = string([]rune(value)[:240])
	}
	for _, token := range []string{"token=", "password=", "secret=", "private_key", "BEGIN PRIVATE KEY"} {
		if strings.Contains(strings.ToLower(value), strings.ToLower(token)) {
			return "delivery failed; sensitive error details are redacted"
		}
	}
	return value
}
