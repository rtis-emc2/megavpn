package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	authn "github.com/rtis-emc2/megavpn/internal/auth"
	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/platform/id"
)

func (s *Store) GetPlatformMailSettings(ctx context.Context) (domain.PlatformMailSettings, error) {
	var x domain.PlatformMailSettings
	err := s.db.QueryRow(ctx, `select enabled,provider,smtp_host,smtp_port,smtp_username,smtp_password_secret_ref_id,smtp_auth_mode,smtp_tls_mode,from_email,from_name,reply_to_email,invite_url_base,last_test_at,last_error,created_at,updated_at from platform_mail_settings where id=true`).
		Scan(&x.Enabled, &x.Provider, &x.SMTPHost, &x.SMTPPort, &x.SMTPUsername, &x.SMTPPasswordSecretRefID, &x.SMTPAuthMode, &x.SMTPTLSMode, &x.FromEmail, &x.FromName, &x.ReplyToEmail, &x.InviteURLBase, &x.LastTestAt, &x.LastError, &x.CreatedAt, &x.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return defaultPlatformMailSettings(), nil
		}
		return x, err
	}
	x.SMTPPasswordConfigured = x.SMTPPasswordSecretRefID != nil && strings.TrimSpace(*x.SMTPPasswordSecretRefID) != ""
	return x, nil
}

func defaultPlatformMailSettings() domain.PlatformMailSettings {
	return domain.PlatformMailSettings{
		Enabled:      false,
		Provider:     "smtp",
		SMTPPort:     587,
		SMTPAuthMode: "plain",
		SMTPTLSMode:  "starttls",
	}
}

func (s *Store) UpsertPlatformMailSettings(ctx context.Context, x domain.PlatformMailSettings, updatedBy *string) (domain.PlatformMailSettings, error) {
	if x.Enabled && strings.TrimSpace(x.FromEmail) == "" {
		return domain.PlatformMailSettings{}, fmt.Errorf("from_email is required when mail delivery is enabled")
	}
	if x.Enabled && strings.TrimSpace(x.SMTPHost) == "" {
		return domain.PlatformMailSettings{}, fmt.Errorf("smtp_host is required when mail delivery is enabled")
	}
	if x.SMTPPort <= 0 {
		x.SMTPPort = 587
	}
	if x.Enabled && !strings.EqualFold(strings.TrimSpace(x.SMTPAuthMode), "none") && strings.TrimSpace(x.SMTPUsername) != "" && (x.SMTPPasswordSecretRefID == nil || strings.TrimSpace(*x.SMTPPasswordSecretRefID) == "") {
		return domain.PlatformMailSettings{}, fmt.Errorf("smtp password is required for authenticated smtp delivery")
	}
	x.Provider = strings.TrimSpace(x.Provider)
	if x.Provider == "" {
		x.Provider = "smtp"
	}
	x.SMTPAuthMode = strings.TrimSpace(x.SMTPAuthMode)
	if x.SMTPAuthMode == "" {
		x.SMTPAuthMode = "plain"
	}
	x.SMTPTLSMode = strings.TrimSpace(x.SMTPTLSMode)
	if x.SMTPTLSMode == "" {
		x.SMTPTLSMode = "starttls"
	}
	if _, err := s.db.Exec(ctx, `insert into platform_mail_settings(id,enabled,provider,smtp_host,smtp_port,smtp_username,smtp_password_secret_ref_id,smtp_auth_mode,smtp_tls_mode,from_email,from_name,reply_to_email,invite_url_base,last_error,created_at,updated_at)
		values(true,$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,'',now(),now())
		on conflict (id) do update set
		  enabled=excluded.enabled,
		  provider=excluded.provider,
		  smtp_host=excluded.smtp_host,
		  smtp_port=excluded.smtp_port,
		  smtp_username=excluded.smtp_username,
		  smtp_password_secret_ref_id=excluded.smtp_password_secret_ref_id,
		  smtp_auth_mode=excluded.smtp_auth_mode,
		  smtp_tls_mode=excluded.smtp_tls_mode,
		  from_email=excluded.from_email,
		  from_name=excluded.from_name,
		  reply_to_email=excluded.reply_to_email,
		  invite_url_base=excluded.invite_url_base,
		  updated_at=now()`,
		x.Enabled, x.Provider, strings.TrimSpace(x.SMTPHost), x.SMTPPort, strings.TrimSpace(x.SMTPUsername), x.SMTPPasswordSecretRefID, x.SMTPAuthMode, x.SMTPTLSMode, strings.TrimSpace(x.FromEmail), strings.TrimSpace(x.FromName), strings.TrimSpace(x.ReplyToEmail), strings.TrimSpace(x.InviteURLBase)); err != nil {
		return domain.PlatformMailSettings{}, err
	}
	_, _ = s.CreateAuditForUser(ctx, updatedBy, "platform.mail_settings.update", "platform_mail_settings", nil, "mail settings updated")
	return s.GetPlatformMailSettings(ctx)
}

func (s *Store) MarkPlatformMailTest(ctx context.Context, errText string) error {
	_, err := s.db.Exec(ctx, `update platform_mail_settings set last_test_at=now(),last_error=$1,updated_at=now() where id=true`, strings.TrimSpace(errText))
	return err
}

func (s *Store) CreatePlatformUserInvite(ctx context.Context, username, email, displayName string, roleCodes []string, createdBy *string, ttl time.Duration) (domain.PlatformUserRecord, domain.PlatformUserInvite, error) {
	if ttl <= 0 {
		ttl = 48 * time.Hour
	}
	placeholderPassword := randomToken(32)
	passwordHash, err := authn.HashPassword(placeholderPassword)
	if err != nil {
		return domain.PlatformUserRecord{}, domain.PlatformUserInvite{}, err
	}
	user, err := s.CreatePlatformUser(ctx, username, email, displayName, passwordHash, roleCodes, createdBy)
	if err != nil {
		return domain.PlatformUserRecord{}, domain.PlatformUserInvite{}, err
	}
	invite, err := s.CreatePlatformUserInviteForUser(ctx, user.ID, user.Username, user.Email, user.DisplayName, createdBy, ttl)
	if err != nil {
		return domain.PlatformUserRecord{}, domain.PlatformUserInvite{}, err
	}
	return user, invite, nil
}

func (s *Store) CreatePlatformUserInviteForUser(ctx context.Context, userID, username, email, displayName string, createdBy *string, ttl time.Duration) (domain.PlatformUserInvite, error) {
	if ttl <= 0 {
		ttl = 48 * time.Hour
	}
	token := randomToken(32)
	hash := hashToken(token)
	now := time.Now().UTC()
	invite := domain.PlatformUserInvite{
		ID:          id.New(),
		UserID:      strings.TrimSpace(userID),
		Username:    strings.TrimSpace(username),
		Email:       normalizeEmail(email),
		DisplayName: strings.TrimSpace(displayName),
		Token:       token,
		TokenHint:   tokenHint(token),
		Status:      "pending",
		ExpiresAt:   now.Add(ttl),
		CreatedBy:   createdBy,
		CreatedAt:   now,
	}
	if invite.Email == "" {
		return domain.PlatformUserInvite{}, errors.New("invite email is required")
	}
	_, err := s.db.Exec(ctx, `insert into platform_user_invites(id,user_id,email,token_hash,token_hint,status,expires_at,created_by,created_at) values($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		invite.ID, invite.UserID, invite.Email, hash, invite.TokenHint, invite.Status, invite.ExpiresAt, invite.CreatedBy, invite.CreatedAt)
	if err != nil {
		return domain.PlatformUserInvite{}, err
	}
	_, _ = s.CreateAuditForUser(ctx, createdBy, "auth.user.invite.create", "platform_user", &invite.UserID, "platform user invite created")
	return invite, nil
}

func (s *Store) ListPlatformUserInvites(ctx context.Context, limit int) ([]domain.PlatformUserInvite, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `select i.id,i.user_id,u.username,u.display_name,i.email,i.token_hint,i.status,i.expires_at,i.sent_at,i.accepted_at,i.delivery_error,i.created_by,i.created_at
		from platform_user_invites i
		join platform_users u on u.id=i.user_id
		order by i.created_at desc
		limit $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.PlatformUserInvite
	for rows.Next() {
		var x domain.PlatformUserInvite
		if err := rows.Scan(&x.ID, &x.UserID, &x.Username, &x.DisplayName, &x.Email, &x.TokenHint, &x.Status, &x.ExpiresAt, &x.SentAt, &x.AcceptedAt, &x.DeliveryError, &x.CreatedBy, &x.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) GetPlatformUserInviteByToken(ctx context.Context, token string) (domain.PlatformUserInvite, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return domain.PlatformUserInvite{}, errors.New("invite token is required")
	}
	hash := hashToken(token)
	var x domain.PlatformUserInvite
	err := s.db.QueryRow(ctx, `select i.id,i.user_id,u.username,u.display_name,i.email,i.token_hint,i.status,i.expires_at,i.sent_at,i.accepted_at,i.delivery_error,i.created_by,i.created_at
		from platform_user_invites i
		join platform_users u on u.id=i.user_id
		where i.token_hash=$1`, hash).
		Scan(&x.ID, &x.UserID, &x.Username, &x.DisplayName, &x.Email, &x.TokenHint, &x.Status, &x.ExpiresAt, &x.SentAt, &x.AcceptedAt, &x.DeliveryError, &x.CreatedBy, &x.CreatedAt)
	if err != nil {
		return domain.PlatformUserInvite{}, err
	}
	if x.Status == "pending" && x.ExpiresAt.Before(time.Now().UTC()) {
		if _, err := s.db.Exec(ctx, `update platform_user_invites set status='expired' where id=$1 and status='pending'`, x.ID); err != nil {
			return domain.PlatformUserInvite{}, err
		}
		x.Status = "expired"
	}
	return x, nil
}

func (s *Store) MarkPlatformUserInviteDelivered(ctx context.Context, inviteID string, deliveryErr string) error {
	status := "sent"
	sentAt := time.Now().UTC()
	if strings.TrimSpace(deliveryErr) != "" {
		status = "delivery_failed"
	}
	_, err := s.db.Exec(ctx, `update platform_user_invites set status=$2,sent_at=$3,delivery_error=$4 where id=$1 and status in ('pending','delivery_failed')`, strings.TrimSpace(inviteID), status, sentAt, strings.TrimSpace(deliveryErr))
	return err
}

func (s *Store) AcceptPlatformUserInvite(ctx context.Context, token, passwordHash string) (domain.PlatformUserInvite, domain.PlatformUserRecord, error) {
	token = strings.TrimSpace(token)
	if token == "" || strings.TrimSpace(passwordHash) == "" {
		return domain.PlatformUserInvite{}, domain.PlatformUserRecord{}, errors.New("invite token and password hash are required")
	}
	hash := hashToken(token)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return domain.PlatformUserInvite{}, domain.PlatformUserRecord{}, err
	}
	defer tx.Rollback(ctx)

	var invite domain.PlatformUserInvite
	err = tx.QueryRow(ctx, `select i.id,i.user_id,u.username,u.display_name,i.email,i.token_hint,i.status,i.expires_at,i.sent_at,i.accepted_at,i.delivery_error,i.created_by,i.created_at
		from platform_user_invites i
		join platform_users u on u.id=i.user_id
		where i.token_hash=$1
		for update`, hash).
		Scan(&invite.ID, &invite.UserID, &invite.Username, &invite.DisplayName, &invite.Email, &invite.TokenHint, &invite.Status, &invite.ExpiresAt, &invite.SentAt, &invite.AcceptedAt, &invite.DeliveryError, &invite.CreatedBy, &invite.CreatedAt)
	if err != nil {
		return domain.PlatformUserInvite{}, domain.PlatformUserRecord{}, err
	}
	now := time.Now().UTC()
	if invite.Status != "pending" && invite.Status != "sent" && invite.Status != "delivery_failed" {
		return domain.PlatformUserInvite{}, domain.PlatformUserRecord{}, errors.New("invite is no longer active")
	}
	if invite.ExpiresAt.Before(now) {
		_, _ = tx.Exec(ctx, `update platform_user_invites set status='expired' where id=$1`, invite.ID)
		return domain.PlatformUserInvite{}, domain.PlatformUserRecord{}, errors.New("invite has expired")
	}
	if _, err := tx.Exec(ctx, `update platform_users set password_hash=$2,status='active',updated_at=now() where id=$1`, invite.UserID, passwordHash); err != nil {
		return domain.PlatformUserInvite{}, domain.PlatformUserRecord{}, err
	}
	if _, err := tx.Exec(ctx, `update platform_user_invites set status='accepted',accepted_at=$2 where id=$1`, invite.ID, now); err != nil {
		return domain.PlatformUserInvite{}, domain.PlatformUserRecord{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.PlatformUserInvite{}, domain.PlatformUserRecord{}, err
	}
	invite.Status = "accepted"
	invite.AcceptedAt = &now
	user, err := s.getPlatformUserRecord(ctx, invite.UserID)
	if err != nil {
		return domain.PlatformUserInvite{}, domain.PlatformUserRecord{}, err
	}
	_, _ = s.CreateAuditForUser(ctx, &invite.UserID, "auth.user.invite.accept", "platform_user", &invite.UserID, "platform user invite accepted")
	return invite, user, nil
}

func (s *Store) CreateClientEmailDelivery(ctx context.Context, delivery domain.ClientEmailDelivery) (domain.ClientEmailDelivery, error) {
	if delivery.ID == "" {
		delivery.ID = id.New()
	}
	if delivery.Payload == nil {
		delivery.Payload = map[string]any{}
	}
	if delivery.ArtifactIDs == nil {
		delivery.ArtifactIDs = []string{}
	}
	if delivery.ShareLinkIDs == nil {
		delivery.ShareLinkIDs = []string{}
	}
	if delivery.Status == "" {
		delivery.Status = "queued"
	}
	delivery.CreatedAt = time.Now().UTC()
	_, err := s.db.Exec(ctx, `insert into client_email_deliveries(id,client_account_id,email,subject,status,artifact_ids,share_link_ids,payload_json,error_text,created_by,sent_at,created_at)
		values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		delivery.ID, delivery.ClientAccountID, delivery.Email, delivery.Subject, delivery.Status, mustJSON(delivery.ArtifactIDs), mustJSON(delivery.ShareLinkIDs), mustJSON(delivery.Payload), delivery.ErrorText, delivery.CreatedBy, delivery.SentAt, delivery.CreatedAt)
	return delivery, err
}

func (s *Store) UpdateClientEmailDeliveryStatus(ctx context.Context, deliveryID, status, errorText string, payload map[string]any) error {
	if payload == nil {
		payload = map[string]any{}
	}
	var sentAt *time.Time
	if status == "sent" {
		now := time.Now().UTC()
		sentAt = &now
	}
	_, err := s.db.Exec(ctx, `update client_email_deliveries set status=$2,error_text=$3,payload_json=$4,sent_at=$5 where id=$1`, deliveryID, status, strings.TrimSpace(errorText), mustJSON(payload), sentAt)
	return err
}
