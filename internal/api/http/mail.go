package http

import (
	"context"
	"errors"
	"html"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	authn "github.com/rtis-emc2/megavpn/internal/auth"
	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/infra/postgres"
	"github.com/rtis-emc2/megavpn/internal/mail"
)

type platformUserInviteRequest struct {
	Username    string   `json:"username"`
	Email       string   `json:"email"`
	DisplayName string   `json:"display_name"`
	RoleCodes   []string `json:"role_codes"`
	TTLHours    int      `json:"ttl_hours"`
}

type inviteAcceptRequest struct {
	Password string `json:"password"`
}

type mailSettingsRequest struct {
	Enabled              bool    `json:"enabled"`
	SMTPHost             string  `json:"smtp_host"`
	SMTPPort             int     `json:"smtp_port"`
	SMTPUsername         string  `json:"smtp_username"`
	SMTPPasswordSecretID *string `json:"smtp_password_secret_ref_id"`
	SMTPPassword         string  `json:"smtp_password"`
	SMTPAuthMode         string  `json:"smtp_auth_mode"`
	SMTPTLSMode          string  `json:"smtp_tls_mode"`
	FromEmail            string  `json:"from_email"`
	FromName             string  `json:"from_name"`
	ReplyToEmail         string  `json:"reply_to_email"`
	InviteURLBase        string  `json:"invite_url_base"`
}

type mailTestRequest struct {
	Email string `json:"email"`
}

type clientEmailRequest struct {
	Subject         string `json:"subject"`
	Message         string `json:"message"`
	TTLHours        int    `json:"ttl_hours"`
	CreateShareLink bool   `json:"create_share_link"`
}

type resendInviteRequest struct {
	TTLHours int `json:"ttl_hours"`
}

func (s *Server) getPlatformMailSettings(w http.ResponseWriter, r *http.Request) {
	x, err := s.store.GetPlatformMailSettings(r.Context())
	if err != nil {
		writeErr(w, 500, "mail settings lookup failed")
		return
	}
	writeJSON(w, 200, x)
}

func (s *Server) updatePlatformMailSettings(w http.ResponseWriter, r *http.Request) {
	authCtx, ok := authFromRequest(r)
	if !ok {
		writeErr(w, 401, "authentication required")
		return
	}
	var req mailSettingsRequest
	if !decode(r, &req) {
		writeErr(w, 400, "invalid mail settings payload")
		return
	}
	settings := domain.PlatformMailSettings{
		Enabled:                 req.Enabled,
		Provider:                "smtp",
		SMTPHost:                strings.TrimSpace(req.SMTPHost),
		SMTPPort:                req.SMTPPort,
		SMTPUsername:            strings.TrimSpace(req.SMTPUsername),
		SMTPPasswordSecretRefID: req.SMTPPasswordSecretID,
		SMTPAuthMode:            strings.TrimSpace(req.SMTPAuthMode),
		SMTPTLSMode:             strings.TrimSpace(req.SMTPTLSMode),
		FromEmail:               strings.TrimSpace(req.FromEmail),
		FromName:                strings.TrimSpace(req.FromName),
		ReplyToEmail:            strings.TrimSpace(req.ReplyToEmail),
		InviteURLBase:           strings.TrimSpace(req.InviteURLBase),
	}
	if settings.SMTPPort <= 0 {
		settings.SMTPPort = 587
	}
	if settings.SMTPAuthMode == "" {
		settings.SMTPAuthMode = "plain"
	}
	if settings.SMTPTLSMode == "" {
		settings.SMTPTLSMode = "starttls"
	}
	if rawPassword := strings.TrimSpace(req.SMTPPassword); rawPassword != "" {
		secretRef, err := s.store.CreateSecretRef(r.Context(), "password", []byte(rawPassword), map[string]any{"usage": "smtp_password"})
		if err != nil {
			if errors.Is(err, postgres.ErrSecretServiceUnavailable) {
				writeErr(w, 409, "smtp password storage requires MEGAVPN_MASTER_KEY_PATH to be configured")
				return
			}
			writeErr(w, 400, err.Error())
			return
		}
		settings.SMTPPasswordSecretRefID = &secretRef.ID
	}
	x, err := s.store.UpsertPlatformMailSettings(r.Context(), settings, &authCtx.User.ID)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, x)
}

func (s *Server) sendPlatformMailTest(w http.ResponseWriter, r *http.Request) {
	authCtx, ok := authFromRequest(r)
	if !ok {
		writeErr(w, 401, "authentication required")
		return
	}
	var req mailTestRequest
	if !decode(r, &req) || strings.TrimSpace(req.Email) == "" {
		writeErr(w, 400, "test email is required")
		return
	}
	settings, err := s.store.GetPlatformMailSettings(r.Context())
	if err != nil {
		writeErr(w, 500, "mail settings lookup failed")
		return
	}
	testedAt := time.Now().UTC()
	testPayload := response{
		"recipient": strings.TrimSpace(req.Email),
		"tested_at": testedAt,
		"requested_by": response{
			"id":           authCtx.User.ID,
			"username":     authCtx.User.Username,
			"display_name": authCtx.User.DisplayName,
			"email":        authCtx.User.Email,
		},
		"transport": response{
			"provider":                 settings.Provider,
			"smtp_host":                settings.SMTPHost,
			"smtp_port":                settings.SMTPPort,
			"smtp_auth_mode":           settings.SMTPAuthMode,
			"smtp_tls_mode":            settings.SMTPTLSMode,
			"from_email":               settings.FromEmail,
			"from_name":                settings.FromName,
			"reply_to_email":           settings.ReplyToEmail,
			"smtp_password_configured": settings.SMTPPasswordConfigured,
		},
	}
	err = s.sendPlatformMail(r.Context(), settings, mail.Message{
		FromEmail: settings.FromEmail,
		FromName:  settings.FromName,
		ReplyTo:   settings.ReplyToEmail,
		To:        []string{strings.TrimSpace(req.Email)},
		Subject:   "RTIS MegaVPN test email",
		TextBody:  buildPlatformMailTestText(authCtx.User, settings, strings.TrimSpace(req.Email), testedAt),
		HTMLBody:  buildPlatformMailTestHTML(authCtx.User, settings, strings.TrimSpace(req.Email), testedAt),
	})
	_ = s.store.MarkPlatformMailTest(r.Context(), errorString(err))
	if err != nil {
		_, _ = s.store.CreateAuditForUser(r.Context(), &authCtx.User.ID, "platform.mail_settings.test", "platform_mail_settings", nil, "mail settings test failed")
		writeJSON(w, 502, response{"status": "delivery_failed", "error": err.Error(), "test": testPayload})
		return
	}
	_, _ = s.store.CreateAuditForUser(r.Context(), &authCtx.User.ID, "platform.mail_settings.test", "platform_mail_settings", nil, "mail settings test sent")
	writeJSON(w, 200, response{"status": "ok", "message": "test email sent", "test": testPayload})
}

func (s *Server) invitePlatformUser(w http.ResponseWriter, r *http.Request) {
	authCtx, ok := authFromRequest(r)
	if !ok {
		writeErr(w, 401, "authentication required")
		return
	}
	var req platformUserInviteRequest
	if !decode(r, &req) || strings.TrimSpace(req.Username) == "" || strings.TrimSpace(req.Email) == "" {
		writeErr(w, 400, "invalid invite payload")
		return
	}
	settings, err := s.store.GetPlatformMailSettings(r.Context())
	if err != nil {
		writeErr(w, 500, "mail settings lookup failed")
		return
	}
	if !settings.Enabled {
		writeErr(w, 409, "mail settings are disabled")
		return
	}
	ttl := time.Duration(req.TTLHours) * time.Hour
	user, invite, err := s.store.CreatePlatformUserInvite(r.Context(), req.Username, req.Email, req.DisplayName, req.RoleCodes, &authCtx.User.ID, ttl)
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	linkBase := firstNonEmpty(settings.InviteURLBase, s.publicBaseURL)
	if linkBase == "" {
		linkBase = "/"
	}
	link := strings.TrimRight(linkBase, "/") + "/?invite_token=" + invite.Token
	err = s.sendPlatformMail(r.Context(), settings, mail.Message{
		FromEmail: settings.FromEmail,
		FromName:  settings.FromName,
		ReplyTo:   settings.ReplyToEmail,
		To:        []string{invite.Email},
		Subject:   "RTIS MegaVPN operator invitation",
		TextBody:  buildOperatorInviteText(user, authCtx.User, link, invite.ExpiresAt),
		HTMLBody:  buildOperatorInviteHTML(user, authCtx.User, link, invite.ExpiresAt),
	})
	_ = s.store.MarkPlatformUserInviteDelivered(r.Context(), invite.ID, errorString(err))
	if err != nil {
		writeJSON(w, 502, response{"status": "delivery_failed", "user": user, "invite": invite, "error": err.Error(), "invite_url": link})
		return
	}
	writeJSON(w, 201, response{"status": "ok", "user": user, "invite": invite, "invite_url": link})
}

func (s *Server) resendPlatformUserInvite(w http.ResponseWriter, r *http.Request) {
	authCtx, ok := authFromRequest(r)
	if !ok {
		writeErr(w, 401, "authentication required")
		return
	}
	var req resendInviteRequest
	_ = decode(r, &req)
	settings, err := s.store.GetPlatformMailSettings(r.Context())
	if err != nil {
		writeErr(w, 500, "mail settings lookup failed")
		return
	}
	if !settings.Enabled {
		writeErr(w, 409, "mail settings are disabled")
		return
	}
	userID := strings.TrimSpace(idParam(r))
	user, err := s.store.GetPlatformUserRecord(r.Context(), userID)
	if err != nil {
		writeErr(w, 404, "platform user not found")
		return
	}
	ttl := time.Duration(req.TTLHours) * time.Hour
	invite, err := s.store.CreatePlatformUserInviteForUser(r.Context(), user.ID, user.Username, user.Email, user.DisplayName, &authCtx.User.ID, ttl)
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	linkBase := firstNonEmpty(settings.InviteURLBase, s.publicBaseURL)
	if linkBase == "" {
		linkBase = "/"
	}
	link := strings.TrimRight(linkBase, "/") + "/?invite_token=" + invite.Token
	err = s.sendPlatformMail(r.Context(), settings, mail.Message{
		FromEmail: settings.FromEmail,
		FromName:  settings.FromName,
		ReplyTo:   settings.ReplyToEmail,
		To:        []string{invite.Email},
		Subject:   "RTIS MegaVPN operator invitation",
		TextBody:  buildOperatorInviteText(user, authCtx.User, link, invite.ExpiresAt),
		HTMLBody:  buildOperatorInviteHTML(user, authCtx.User, link, invite.ExpiresAt),
	})
	_ = s.store.MarkPlatformUserInviteDelivered(r.Context(), invite.ID, errorString(err))
	if err != nil {
		writeJSON(w, 502, response{"status": "delivery_failed", "user": user, "invite": invite, "error": err.Error(), "invite_url": link})
		return
	}
	writeJSON(w, 200, response{"status": "ok", "user": user, "invite": invite, "invite_url": link})
}

func (s *Server) listPlatformUserInvites(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	x, err := s.store.ListPlatformUserInvites(r.Context(), limit)
	if err != nil {
		writeErr(w, 500, "list platform user invites failed")
		return
	}
	if x == nil {
		x = []domain.PlatformUserInvite{}
	}
	writeJSON(w, 200, x)
}

func (s *Server) getPlatformInvitePublic(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.PathValue("token"))
	if token == "" {
		writeErr(w, 400, "invite token is required")
		return
	}
	invite, err := s.store.GetPlatformUserInviteByToken(r.Context(), token)
	if err != nil {
		writeErr(w, 404, "invite not found")
		return
	}
	writeJSON(w, 200, response{
		"id":           invite.ID,
		"username":     invite.Username,
		"display_name": invite.DisplayName,
		"email":        invite.Email,
		"status":       invite.Status,
		"expires_at":   invite.ExpiresAt,
	})
}

func (s *Server) acceptPlatformInvite(w http.ResponseWriter, r *http.Request) {
	var req inviteAcceptRequest
	if !decode(r, &req) || strings.TrimSpace(req.Password) == "" {
		writeErr(w, 400, "password is required")
		return
	}
	passwordHash, err := authn.HashPassword(req.Password)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	token := strings.TrimSpace(r.PathValue("token"))
	invite, user, err := s.store.AcceptPlatformUserInvite(r.Context(), token, passwordHash)
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	sessionToken, sessionTokenHash, err := authn.NewSessionToken()
	if err != nil {
		writeErr(w, 500, "session token generation failed")
		return
	}
	expiresAt := time.Now().UTC().Add(s.sessionTTL)
	sess, err := s.store.CreateUserSession(r.Context(), user.ID, sessionTokenHash, s.clientIP(r), r.UserAgent(), expiresAt)
	if err != nil {
		writeErr(w, 500, "session creation failed")
		return
	}
	_ = s.store.TouchPlatformUserLogin(r.Context(), user.ID)
	http.SetCookie(w, &http.Cookie{
		Name:     s.sessionCookieName,
		Value:    sessionToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.sessionCookieSecure,
		SameSite: http.SameSiteLaxMode,
		Expires:  sess.ExpiresAt,
	})
	authCtx, err := s.store.ResolveAuthContext(r.Context(), sessionTokenHash)
	if err != nil {
		writeErr(w, 500, "session resolution failed")
		return
	}
	writeJSON(w, 200, response{
		"status":      "ok",
		"invite":      invite,
		"user":        authCtx.User,
		"roles":       authCtx.RoleCodes,
		"permissions": authCtx.PermissionCodes,
		"session": response{
			"id":         authCtx.Session.ID,
			"expires_at": authCtx.Session.ExpiresAt,
			"token":      sessionToken,
		},
	})
}

func (s *Server) deliverClientEmail(w http.ResponseWriter, r *http.Request) {
	authCtx, ok := authFromRequest(r)
	if !ok {
		writeErr(w, 401, "authentication required")
		return
	}
	var req clientEmailRequest
	if !decodeOptional(r, &req) {
		writeErr(w, 400, "invalid client email payload")
		return
	}
	clientID := idParam(r)
	clientRecord, err := s.store.GetClient(r.Context(), clientID)
	if err != nil {
		writeErr(w, 404, "client not found")
		return
	}
	if strings.TrimSpace(clientRecord.Email) == "" {
		writeErr(w, 409, "client email is not configured")
		return
	}
	settings, err := s.store.GetPlatformMailSettings(r.Context())
	if err != nil {
		writeErr(w, 500, "mail settings lookup failed")
		return
	}
	artifacts, err := s.store.ListArtifacts(r.Context(), clientID)
	if err != nil {
		writeErr(w, 500, "list client artifacts failed")
		return
	}
	ttl := time.Duration(req.TTLHours) * time.Hour
	if req.CreateShareLink || len(artifacts) == 0 {
		if _, err := s.store.PublishShareLink(r.Context(), clientID, "", ttl); err != nil {
			writeErr(w, 500, "share link create failed")
			return
		}
	}
	shareLinks, err := s.store.ListShareLinks(r.Context(), clientID)
	if err != nil {
		writeErr(w, 500, "list client share links failed")
		return
	}
	delivery := domain.ClientEmailDelivery{
		ClientAccountID: clientID,
		Email:           strings.TrimSpace(clientRecord.Email),
		Subject:         firstNonEmpty(strings.TrimSpace(req.Subject), "RTIS MegaVPN access package"),
		Status:          "queued",
		CreatedBy:       &authCtx.User.ID,
		Payload:         map[string]any{"client_username": clientRecord.Username},
	}
	for _, artifact := range artifacts {
		delivery.ArtifactIDs = append(delivery.ArtifactIDs, artifact.ID)
	}
	for _, link := range shareLinks {
		delivery.ShareLinkIDs = append(delivery.ShareLinkIDs, link.ID)
	}
	delivery, err = s.store.CreateClientEmailDelivery(r.Context(), delivery)
	if err != nil {
		writeErr(w, 500, "client email delivery create failed")
		return
	}
	attachments, attachmentNotes := collectArtifactAttachments(artifacts)
	body := buildClientAccessBody(clientRecord, req.Message, artifacts, shareLinks, attachmentNotes, s.publicBaseURL)
	err = s.sendPlatformMail(r.Context(), settings, mail.Message{
		FromEmail:   settings.FromEmail,
		FromName:    settings.FromName,
		ReplyTo:     settings.ReplyToEmail,
		To:          []string{clientRecord.Email},
		Subject:     delivery.Subject,
		TextBody:    body,
		Attachments: attachments,
	})
	status := "sent"
	errText := ""
	if err != nil {
		status = "failed"
		errText = err.Error()
	}
	_ = s.store.UpdateClientEmailDeliveryStatus(r.Context(), delivery.ID, status, errText, map[string]any{
		"attachment_count": len(attachments),
		"share_link_count": len(shareLinks),
	})
	if err != nil {
		writeJSON(w, 502, response{"status": status, "delivery": delivery, "error": err.Error()})
		return
	}
	writeJSON(w, 200, response{"status": "ok", "delivery": delivery})
}

func (s *Server) sendPlatformMail(ctx context.Context, settings domain.PlatformMailSettings, msg mail.Message) error {
	if !settings.Enabled {
		return errors.New("mail settings are disabled")
	}
	password := ""
	if settings.SMTPPasswordSecretRefID != nil && strings.TrimSpace(*settings.SMTPPasswordSecretRefID) != "" {
		_, secretValue, err := s.store.ResolveSecretValue(ctx, *settings.SMTPPasswordSecretRefID)
		if err != nil {
			if errors.Is(err, postgres.ErrSecretServiceUnavailable) {
				return errors.New("smtp password is configured but secret storage is unavailable; set MEGAVPN_MASTER_KEY_PATH")
			}
			return err
		}
		password = strings.TrimSpace(string(secretValue))
	}
	return mail.SendSMTP(ctx, mail.SMTPConfig{
		Host:     settings.SMTPHost,
		Port:     settings.SMTPPort,
		Username: settings.SMTPUsername,
		Password: password,
		AuthMode: settings.SMTPAuthMode,
		TLSMode:  settings.SMTPTLSMode,
	}, msg)
}

func buildOperatorInviteText(user domain.PlatformUserRecord, invitedBy domain.PlatformUser, link string, expiresAt time.Time) string {
	invitee := firstNonEmpty(user.DisplayName, user.Username)
	operator := firstNonEmpty(invitedBy.DisplayName, invitedBy.Username, invitedBy.Email, "RTIS MegaVPN operator")
	var b strings.Builder
	b.WriteString("Hello,\n\n")
	b.WriteString("You have been invited to RTIS MegaVPN Control Plane.\n\n")
	b.WriteString("Invitation details:\n")
	b.WriteString("- Invited user: " + invitee + "\n")
	b.WriteString("- Username: " + user.Username + "\n")
	b.WriteString("- Email: " + firstNonEmpty(user.Email, "n/a") + "\n")
	b.WriteString("- Roles: " + firstNonEmpty(strings.Join(user.RoleCodes, ", "), "n/a") + "\n")
	b.WriteString("- Invited by: " + operator + "\n")
	b.WriteString("- Expires at: " + expiresAt.Format(time.RFC3339) + "\n\n")
	b.WriteString("Activation link:\n")
	b.WriteString(link + "\n\n")
	b.WriteString("How to activate:\n")
	b.WriteString("1. Open the one-time invitation link.\n")
	b.WriteString("2. Set your password.\n")
	b.WriteString("3. Sign in to RTIS MegaVPN Control Plane.\n\n")
	b.WriteString("If the link expires, ask the administrator to send a new invitation.")
	return b.String()
}

func buildOperatorInviteHTML(user domain.PlatformUserRecord, invitedBy domain.PlatformUser, link string, expiresAt time.Time) string {
	invitee := firstNonEmpty(user.DisplayName, user.Username)
	operator := firstNonEmpty(invitedBy.DisplayName, invitedBy.Username, invitedBy.Email, "RTIS MegaVPN operator")
	rows := []string{
		buildMailMetaRow("Invited user", invitee),
		buildMailMetaRow("Username", firstNonEmpty(user.Username, "n/a")),
		buildMailMetaRow("Email", firstNonEmpty(user.Email, "n/a")),
		buildMailMetaRow("Roles", firstNonEmpty(strings.Join(user.RoleCodes, ", "), "n/a")),
		buildMailMetaRow("Invited by", operator),
		buildMailMetaRow("Expires at", expiresAt.Format(time.RFC3339)),
	}
	return `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>RTIS MegaVPN operator invitation</title>
</head>
<body style="margin:0;padding:0;background:#07111a;font-family:'Segoe UI',Inter,Arial,sans-serif;color:#e7edf5;">
  <div style="padding:32px 18px;background:
    radial-gradient(circle at 14% 0%, rgba(238,26,45,0.18), transparent 26%),
    radial-gradient(circle at 86% 18%, rgba(255,107,120,0.11), transparent 24%),
    linear-gradient(145deg, #04090f, #07111a 46%, #08131c);">
    <div style="max-width:760px;margin:0 auto;">
      <div style="display:flex;align-items:center;gap:14px;margin-bottom:20px;">
        <div style="min-width:56px;height:46px;padding:0 12px;border-radius:14px;display:grid;place-items:center;background:linear-gradient(135deg,#ee1a2d,#b20f22);color:#fff6f7;font-weight:900;letter-spacing:0.04em;">RTIS</div>
        <div>
          <div style="font-size:18px;font-weight:800;color:#e7edf5;">RTIS</div>
          <div style="font-size:12px;letter-spacing:0.14em;text-transform:uppercase;color:#74869a;">MegaVPN Operator Invitation</div>
        </div>
      </div>
      <div style="border:1px solid rgba(148,163,184,0.16);border-radius:20px;overflow:hidden;background:linear-gradient(180deg, rgba(17,31,45,0.92), rgba(13,23,34,0.9));box-shadow:0 24px 80px rgba(0,0,0,0.42);">
        <div style="padding:22px 24px;border-bottom:1px solid rgba(148,163,184,0.16);">
          <div style="font-size:11px;letter-spacing:0.14em;text-transform:uppercase;color:#74869a;">Access Onboarding</div>
          <h1 style="margin:8px 0 10px;font-size:28px;line-height:1.1;letter-spacing:-0.04em;color:#e7edf5;">You have been invited to RTIS MegaVPN Control Plane</h1>
          <p style="margin:0;color:#9aa9b9;line-height:1.6;">An administrator created an operator invitation for you. Use the one-time activation link below to set your password and start working in the control plane.</p>
        </div>
        <div style="padding:24px;">
          <div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(220px,1fr));gap:14px;margin-bottom:18px;">
            <div style="border:1px solid rgba(238,26,45,0.28);background:rgba(238,26,45,0.08);border-radius:16px;padding:16px;">
              <div style="font-size:11px;letter-spacing:0.14em;text-transform:uppercase;color:#74869a;">Invited User</div>
              <div style="margin-top:8px;font-size:24px;font-weight:800;color:#f8fafc;">` + html.EscapeString(invitee) + `</div>
              <div style="margin-top:4px;color:#9aa9b9;font-size:13px;">Operator profile prepared for activation.</div>
            </div>
            <div style="border:1px solid rgba(148,163,184,0.16);background:rgba(7,16,24,0.28);border-radius:16px;padding:16px;">
              <div style="font-size:11px;letter-spacing:0.14em;text-transform:uppercase;color:#74869a;">Invited By</div>
              <div style="margin-top:8px;font-size:18px;font-weight:700;color:#e7edf5;">` + html.EscapeString(operator) + `</div>
              <div style="margin-top:4px;color:#9aa9b9;font-size:13px;">Operator who issued this invitation.</div>
            </div>
            <div style="border:1px solid rgba(52,211,153,0.32);background:rgba(52,211,153,0.08);border-radius:16px;padding:16px;">
              <div style="font-size:11px;letter-spacing:0.14em;text-transform:uppercase;color:#74869a;">Expires</div>
              <div style="margin-top:8px;font-size:18px;font-weight:700;color:#bbf7d0;">` + html.EscapeString(expiresAt.Format(time.RFC3339)) + `</div>
              <div style="margin-top:4px;color:#9aa9b9;font-size:13px;">The activation link is single-use and time-limited.</div>
            </div>
          </div>
          <div style="margin-bottom:18px;border:1px solid rgba(148,163,184,0.16);background:rgba(7,16,24,0.28);border-radius:18px;padding:18px;text-align:center;">
            <a href="` + html.EscapeString(link) + `" style="display:inline-block;padding:14px 22px;border-radius:14px;background:linear-gradient(135deg,#ee1a2d,#b20f22);color:#fff6f7;text-decoration:none;font-weight:800;letter-spacing:0.01em;">Open Invitation Link</a>
            <div style="margin-top:12px;color:#9aa9b9;font-size:13px;line-height:1.6;">If the button does not open, copy and paste this link into your browser:<br /><span style="color:#d8e3ef;word-break:break-all;">` + html.EscapeString(link) + `</span></div>
          </div>
          <div style="display:grid;grid-template-columns:minmax(0,1.15fr) minmax(0,0.85fr);gap:16px;">
            <div style="border:1px solid rgba(148,163,184,0.16);background:rgba(7,16,24,0.28);border-radius:18px;padding:18px;">
              <div style="font-size:11px;letter-spacing:0.14em;text-transform:uppercase;color:#74869a;margin-bottom:10px;">Activation Steps</div>
              <ol style="margin:0;padding-left:18px;color:#d8e3ef;line-height:1.8;">
                <li>Open the one-time invitation link.</li>
                <li>Set your operator password.</li>
                <li>Sign in to RTIS MegaVPN Control Plane.</li>
              </ol>
            </div>
            <div style="border:1px solid rgba(148,163,184,0.16);background:rgba(7,16,24,0.28);border-radius:18px;padding:18px;">
              <div style="font-size:11px;letter-spacing:0.14em;text-transform:uppercase;color:#74869a;margin-bottom:10px;">Invitation Details</div>
              <table style="width:100%;border-collapse:collapse;">` + strings.Join(rows, "") + `</table>
            </div>
          </div>
          <div style="margin-top:18px;color:#9aa9b9;font-size:13px;line-height:1.7;">If this invitation is unexpected or the link has expired, contact the RTIS MegaVPN administrator and request a new invite.</div>
        </div>
      </div>
    </div>
  </div>
</body>
</html>`
}

func buildPlatformMailTestText(user domain.PlatformUser, settings domain.PlatformMailSettings, recipient string, testedAt time.Time) string {
	operator := firstNonEmpty(user.DisplayName, user.Username, user.Email, "operator")
	var b strings.Builder
	b.WriteString("RTIS MegaVPN mail settings are configured correctly.\n\n")
	b.WriteString("If you received this email, SMTP delivery is working.\n\n")
	b.WriteString("Test details:\n")
	b.WriteString("- Requested by: " + operator + "\n")
	if strings.TrimSpace(user.Username) != "" {
		b.WriteString("- Operator username: " + user.Username + "\n")
	}
	if strings.TrimSpace(user.Email) != "" {
		b.WriteString("- Operator email: " + user.Email + "\n")
	}
	b.WriteString("- Recipient: " + recipient + "\n")
	b.WriteString("- Sent at: " + testedAt.Format(time.RFC3339) + "\n")
	b.WriteString("- SMTP host: " + settings.SMTPHost + ":" + strconv.Itoa(settings.SMTPPort) + "\n")
	b.WriteString("- TLS mode: " + settings.SMTPTLSMode + "\n")
	b.WriteString("- Auth mode: " + settings.SMTPAuthMode + "\n")
	b.WriteString("- From: " + firstNonEmpty(settings.FromName, "RTIS MegaVPN") + " <" + settings.FromEmail + ">\n")
	return b.String()
}

func buildPlatformMailTestHTML(user domain.PlatformUser, settings domain.PlatformMailSettings, recipient string, testedAt time.Time) string {
	operator := firstNonEmpty(user.DisplayName, user.Username, user.Email, "operator")
	rows := []string{
		buildMailMetaRow("Requested by", operator),
		buildMailMetaRow("Operator username", firstNonEmpty(user.Username, "n/a")),
		buildMailMetaRow("Operator email", firstNonEmpty(user.Email, "n/a")),
		buildMailMetaRow("Recipient", firstNonEmpty(recipient, "n/a")),
		buildMailMetaRow("Sent at", testedAt.Format(time.RFC3339)),
		buildMailMetaRow("SMTP host", firstNonEmpty(settings.SMTPHost, "n/a")+":"+strconv.Itoa(settings.SMTPPort)),
		buildMailMetaRow("TLS mode", firstNonEmpty(settings.SMTPTLSMode, "n/a")),
		buildMailMetaRow("Auth mode", firstNonEmpty(settings.SMTPAuthMode, "n/a")),
		buildMailMetaRow("From", firstNonEmpty(settings.FromName, "RTIS MegaVPN")+" <"+settings.FromEmail+">"),
	}
	return `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>RTIS MegaVPN mail test</title>
</head>
<body style="margin:0;padding:0;background:#07111a;font-family:'Segoe UI',Inter,Arial,sans-serif;color:#e7edf5;">
  <div style="padding:32px 18px;background:
    radial-gradient(circle at 14% 0%, rgba(238,26,45,0.18), transparent 26%),
    radial-gradient(circle at 86% 18%, rgba(255,107,120,0.11), transparent 24%),
    linear-gradient(145deg, #04090f, #07111a 46%, #08131c);">
    <div style="max-width:760px;margin:0 auto;">
      <div style="display:flex;align-items:center;gap:14px;margin-bottom:20px;">
        <div style="min-width:56px;height:46px;padding:0 12px;border-radius:14px;display:grid;place-items:center;background:linear-gradient(135deg,#ee1a2d,#b20f22);color:#fff6f7;font-weight:900;letter-spacing:0.04em;">RTIS</div>
        <div>
          <div style="font-size:18px;font-weight:800;color:#e7edf5;">RTIS</div>
          <div style="font-size:12px;letter-spacing:0.14em;text-transform:uppercase;color:#74869a;">MegaVPN Mail Check</div>
        </div>
      </div>
      <div style="border:1px solid rgba(148,163,184,0.16);border-radius:20px;overflow:hidden;background:linear-gradient(180deg, rgba(17,31,45,0.92), rgba(13,23,34,0.9));box-shadow:0 24px 80px rgba(0,0,0,0.42);">
        <div style="padding:22px 24px;border-bottom:1px solid rgba(148,163,184,0.16);">
          <div style="font-size:11px;letter-spacing:0.14em;text-transform:uppercase;color:#74869a;">Delivery Validation</div>
          <h1 style="margin:8px 0 10px;font-size:28px;line-height:1.1;letter-spacing:-0.04em;color:#e7edf5;">SMTP test delivered successfully</h1>
          <p style="margin:0;color:#9aa9b9;line-height:1.6;">RTIS MegaVPN mail settings are configured correctly. This message confirms that the control plane can deliver outbound SMTP mail from the current profile.</p>
        </div>
        <div style="padding:24px;">
          <div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(220px,1fr));gap:14px;margin-bottom:18px;">
            <div style="border:1px solid rgba(52,211,153,0.32);background:rgba(52,211,153,0.08);border-radius:16px;padding:16px;">
              <div style="font-size:11px;letter-spacing:0.14em;text-transform:uppercase;color:#74869a;">Status</div>
              <div style="margin-top:8px;font-size:24px;font-weight:800;color:#bbf7d0;">Delivered</div>
              <div style="margin-top:4px;color:#9aa9b9;font-size:13px;">Outbound SMTP check completed.</div>
            </div>
            <div style="border:1px solid rgba(148,163,184,0.16);background:rgba(7,16,24,0.28);border-radius:16px;padding:16px;">
              <div style="font-size:11px;letter-spacing:0.14em;text-transform:uppercase;color:#74869a;">Recipient</div>
              <div style="margin-top:8px;font-size:18px;font-weight:700;color:#e7edf5;">` + html.EscapeString(firstNonEmpty(recipient, "n/a")) + `</div>
              <div style="margin-top:4px;color:#9aa9b9;font-size:13px;">Test message target mailbox.</div>
            </div>
            <div style="border:1px solid rgba(148,163,184,0.16);background:rgba(7,16,24,0.28);border-radius:16px;padding:16px;">
              <div style="font-size:11px;letter-spacing:0.14em;text-transform:uppercase;color:#74869a;">Requested By</div>
              <div style="margin-top:8px;font-size:18px;font-weight:700;color:#e7edf5;">` + html.EscapeString(operator) + `</div>
              <div style="margin-top:4px;color:#9aa9b9;font-size:13px;">Operator who triggered the delivery test.</div>
            </div>
          </div>
          <div style="border:1px solid rgba(148,163,184,0.16);background:rgba(7,16,24,0.28);border-radius:18px;padding:18px;">
            <div style="font-size:11px;letter-spacing:0.14em;text-transform:uppercase;color:#74869a;margin-bottom:10px;">Test Metadata</div>
            <table style="width:100%;border-collapse:collapse;">` + strings.Join(rows, "") + `</table>
          </div>
        </div>
      </div>
    </div>
  </div>
</body>
</html>`
}

func buildMailMetaRow(label, value string) string {
	return `<tr>
  <td style="padding:10px 0;border-bottom:1px solid rgba(148,163,184,0.10);color:#74869a;font-size:12px;text-transform:uppercase;letter-spacing:0.12em;">` + html.EscapeString(label) + `</td>
  <td style="padding:10px 0;border-bottom:1px solid rgba(148,163,184,0.10);color:#e7edf5;font-size:14px;text-align:right;">` + html.EscapeString(value) + `</td>
</tr>`
}

func buildClientAccessBody(client domain.Client, extraMessage string, artifacts []domain.Artifact, shareLinks []domain.ShareLink, attachmentNotes []string, publicBaseURL string) string {
	var b strings.Builder
	b.WriteString("Hello,\n\n")
	b.WriteString("RTIS MegaVPN access package has been prepared for you.\n\n")
	b.WriteString("Client: " + firstNonEmpty(client.DisplayName, client.Username) + "\n")
	if strings.TrimSpace(extraMessage) != "" {
		b.WriteString("\n" + strings.TrimSpace(extraMessage) + "\n")
	}
	if len(attachmentNotes) > 0 {
		b.WriteString("\nAttachments:\n")
		for _, note := range attachmentNotes {
			b.WriteString("- " + note + "\n")
		}
	}
	if len(artifacts) > 0 {
		b.WriteString("\nArtifacts:\n")
		for _, artifact := range artifacts {
			b.WriteString("- " + artifact.ArtifactType + " [" + artifact.Status + "]\n")
		}
	}
	if len(shareLinks) > 0 {
		b.WriteString("\nTemporary links:\n")
		for _, link := range shareLinks {
			b.WriteString("- token: " + link.Token + " expires: " + link.ExpiresAt.Format(time.RFC3339))
			if strings.TrimSpace(publicBaseURL) != "" {
				b.WriteString(" url: " + strings.TrimRight(publicBaseURL, "/") + "/share/" + link.Token)
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}

func collectArtifactAttachments(artifacts []domain.Artifact) ([]mail.Attachment, []string) {
	attachments := []mail.Attachment{}
	notes := []string{}
	for _, artifact := range artifacts {
		path := strings.TrimSpace(artifact.StoragePath)
		if path == "" {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			notes = append(notes, artifact.ArtifactType+" not attached (file unavailable)")
			continue
		}
		filename := filepath.Base(path)
		contentType := mime.TypeByExtension(filepath.Ext(filename))
		attachments = append(attachments, mail.Attachment{
			Filename:    filename,
			ContentType: contentType,
			Data:        data,
		})
		notes = append(notes, artifact.ArtifactType+" attached as "+filename)
	}
	return attachments, notes
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
