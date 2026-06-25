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
		Secure:   s.sessionCookieSecureForRequest(r),
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
		"session":     publicSessionResponse(authCtx.Session),
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
	shareLinks := []domain.ShareLink{}
	if req.CreateShareLink || len(artifacts) == 0 {
		link, err := s.store.PublishShareLink(r.Context(), clientID, "", ttl)
		if err != nil {
			writeErr(w, 500, "share link create failed")
			return
		}
		shareLinks = append(shareLinks, link)
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
	htmlBody := buildClientAccessHTML(clientRecord, req.Message, artifacts, shareLinks, attachmentNotes, s.publicBaseURL)
	err = s.sendPlatformMail(r.Context(), settings, mail.Message{
		FromEmail:   settings.FromEmail,
		FromName:    settings.FromName,
		ReplyTo:     settings.ReplyToEmail,
		To:          []string{clientRecord.Email},
		Subject:     delivery.Subject,
		TextBody:    body,
		HTMLBody:    htmlBody,
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
	sections := []string{
		buildMailSection("Activation steps", buildMailOrderedList([]string{
			"Open the one-time invitation link.",
			"Set your operator password.",
			"Sign in to MegaVPN Control Plane.",
		})),
		buildMailSection("Invitation details", buildMailMetaTable([]mailMetaRow{
			{"Invited user", invitee},
			{"Username", firstNonEmpty(user.Username, "n/a")},
			{"Email", firstNonEmpty(user.Email, "n/a")},
			{"Roles", firstNonEmpty(strings.Join(user.RoleCodes, ", "), "n/a")},
			{"Invited by", operator},
			{"Expires at", expiresAt.Format(time.RFC3339)},
		})),
		buildMailNotice("If this invitation is unexpected or the link has expired, contact the MegaVPN administrator and request a new invite."),
	}
	return buildMailLayout(mailLayout{
		Title:     "MegaVPN operator invitation",
		Preheader: "You have been invited to MegaVPN Control Plane.",
		Eyebrow:   "Operator onboarding",
		Heading:   "You have been invited to MegaVPN Control Plane",
		Intro:     "An administrator created an operator invitation for you. Use the one-time activation link below to set your password and start working in the control plane.",
		Badges: []mailBadge{
			{Label: "Invited user", Value: invitee, Detail: "Operator profile prepared for activation."},
			{Label: "Invited by", Value: operator, Detail: "Operator who issued this invitation."},
			{Label: "Expires", Value: expiresAt.Format(time.RFC3339), Detail: "Single-use and time-limited."},
		},
		Action: &mailAction{
			Label: "Open invitation",
			URL:   link,
			Hint:  "If the button does not open, copy and paste the link into your browser.",
		},
		Sections: sections,
	})
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
	return buildMailLayout(mailLayout{
		Title:     "MegaVPN mail delivery test",
		Preheader: "SMTP delivery from MegaVPN Control Plane is working.",
		Eyebrow:   "Delivery validation",
		Heading:   "SMTP test delivered successfully",
		Intro:     "MegaVPN mail settings are configured correctly. This message confirms that the control plane can deliver outbound SMTP mail from the current profile.",
		Badges: []mailBadge{
			{Label: "Status", Value: "Delivered", Detail: "Outbound SMTP check completed."},
			{Label: "Recipient", Value: firstNonEmpty(recipient, "n/a"), Detail: "Test message target mailbox."},
			{Label: "Requested by", Value: operator, Detail: "Operator who triggered the delivery test."},
		},
		Sections: []string{
			buildMailSection("Test metadata", buildMailMetaTable([]mailMetaRow{
				{"Requested by", operator},
				{"Operator username", firstNonEmpty(user.Username, "n/a")},
				{"Operator email", firstNonEmpty(user.Email, "n/a")},
				{"Recipient", firstNonEmpty(recipient, "n/a")},
				{"Sent at", testedAt.Format(time.RFC3339)},
				{"SMTP host", firstNonEmpty(settings.SMTPHost, "n/a") + ":" + strconv.Itoa(settings.SMTPPort)},
				{"TLS mode", firstNonEmpty(settings.SMTPTLSMode, "n/a")},
				{"Auth mode", firstNonEmpty(settings.SMTPAuthMode, "n/a")},
				{"From", firstNonEmpty(settings.FromName, "MegaVPN") + " <" + settings.FromEmail + ">"},
			})),
		},
	})
}

type mailLayout struct {
	Title     string
	Preheader string
	Eyebrow   string
	Heading   string
	Intro     string
	Badges    []mailBadge
	Action    *mailAction
	Sections  []string
}

type mailBadge struct {
	Label  string
	Value  string
	Detail string
}

type mailAction struct {
	Label string
	URL   string
	Hint  string
}

type mailMetaRow struct {
	Label string
	Value string
}

func buildMailLayout(layout mailLayout) string {
	title := firstNonEmpty(layout.Title, "MegaVPN notification")
	sections := strings.Join(layout.Sections, "")
	return `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width,initial-scale=1" />
  <meta name="color-scheme" content="light" />
  <title>` + html.EscapeString(title) + `</title>
</head>
<body style="margin:0;padding:0;background:#eef2f7;color:#111827;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Arial,sans-serif;">
  <div style="display:none;max-height:0;overflow:hidden;opacity:0;color:transparent;">` + html.EscapeString(layout.Preheader) + `</div>
  <table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="width:100%;border-collapse:collapse;background:#eef2f7;">
    <tr>
      <td align="center" style="padding:28px 14px;">
        <table role="presentation" width="680" cellspacing="0" cellpadding="0" style="width:100%;max-width:680px;border-collapse:collapse;">
          <tr>
            <td style="padding:0 4px 16px 4px;">
              <table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="border-collapse:collapse;">
                <tr>
                  <td width="58" valign="middle" style="width:58px;">
                    <div style="width:48px;height:48px;border-radius:14px;background:#b91c1c;color:#ffffff;font-weight:900;font-size:14px;line-height:48px;text-align:center;letter-spacing:0.04em;">NL</div>
                  </td>
                  <td valign="middle">
                    <div style="font-size:16px;font-weight:800;color:#111827;line-height:1.25;">NLGate MegaVPN</div>
                    <div style="font-size:11px;line-height:1.4;color:#64748b;text-transform:uppercase;letter-spacing:0.12em;">Control Plane Notification</div>
                  </td>
                </tr>
              </table>
            </td>
          </tr>
          <tr>
            <td style="background:#ffffff;border:1px solid #d9e2ec;border-radius:18px;overflow:hidden;box-shadow:0 18px 50px rgba(15,23,42,0.10);">
              <table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="border-collapse:collapse;">
                <tr>
                  <td style="padding:26px 30px 22px;border-bottom:1px solid #e5e7eb;background:#ffffff;">
                    <div style="font-size:11px;font-weight:800;color:#b91c1c;text-transform:uppercase;letter-spacing:0.14em;">` + html.EscapeString(layout.Eyebrow) + `</div>
                    <h1 style="margin:9px 0 10px;font-size:28px;line-height:1.18;color:#111827;font-weight:850;letter-spacing:0;">` + html.EscapeString(layout.Heading) + `</h1>
                    <p style="margin:0;font-size:15px;line-height:1.65;color:#475569;">` + html.EscapeString(layout.Intro) + `</p>
                  </td>
                </tr>
                <tr>
                  <td style="padding:24px 30px 28px;background:#ffffff;">
                    ` + buildMailBadges(layout.Badges) + `
                    ` + buildMailAction(layout.Action) + `
                    ` + sections + `
                    <div style="margin-top:22px;padding-top:18px;border-top:1px solid #e5e7eb;color:#64748b;font-size:12px;line-height:1.65;">
                      This is an automated message from MegaVPN Control Plane. Do not forward links or attached access materials to unauthorized recipients.
                    </div>
                  </td>
                </tr>
              </table>
            </td>
          </tr>
          <tr>
            <td align="center" style="padding:16px 8px 0;color:#94a3b8;font-size:12px;line-height:1.6;">
              NLGate MegaVPN · secure operations notification
            </td>
          </tr>
        </table>
      </td>
    </tr>
  </table>
</body>
</html>`
}

func buildMailBadges(badges []mailBadge) string {
	if len(badges) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="width:100%;border-collapse:collapse;margin:0 0 18px;">`)
	for i, badge := range badges {
		if i%3 == 0 {
			b.WriteString("<tr>")
		}
		b.WriteString(`<td valign="top" style="width:33.333%;padding:0 6px 12px 0;">`)
		b.WriteString(`<div style="border:1px solid #dbe4ee;background:#f8fafc;border-radius:14px;padding:14px;">`)
		b.WriteString(`<div style="font-size:10px;font-weight:800;color:#64748b;text-transform:uppercase;letter-spacing:0.12em;">` + html.EscapeString(badge.Label) + `</div>`)
		b.WriteString(`<div style="margin-top:7px;font-size:18px;line-height:1.25;font-weight:800;color:#111827;word-break:break-word;">` + html.EscapeString(badge.Value) + `</div>`)
		if strings.TrimSpace(badge.Detail) != "" {
			b.WriteString(`<div style="margin-top:5px;font-size:12px;line-height:1.45;color:#64748b;">` + html.EscapeString(badge.Detail) + `</div>`)
		}
		b.WriteString(`</div></td>`)
		if i%3 == 2 || i == len(badges)-1 {
			b.WriteString("</tr>")
		}
	}
	b.WriteString(`</table>`)
	return b.String()
}

func buildMailAction(action *mailAction) string {
	if action == nil || strings.TrimSpace(action.URL) == "" {
		return ""
	}
	hint := firstNonEmpty(action.Hint, "If the button does not open, copy and paste the link into your browser.")
	return `<table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="width:100%;border-collapse:collapse;margin:0 0 20px;">
  <tr>
    <td align="center" style="padding:18px;border:1px solid #fecdd3;background:#fff1f2;border-radius:16px;">
      <a href="` + html.EscapeString(action.URL) + `" style="display:inline-block;background:#b91c1c;color:#ffffff;text-decoration:none;font-size:15px;font-weight:800;line-height:1;padding:14px 22px;border-radius:12px;">` + html.EscapeString(action.Label) + `</a>
      <div style="margin-top:12px;color:#64748b;font-size:12px;line-height:1.6;">` + html.EscapeString(hint) + `<br><span style="color:#334155;word-break:break-all;">` + html.EscapeString(action.URL) + `</span></div>
    </td>
  </tr>
</table>`
}

func buildMailSection(title, body string) string {
	if strings.TrimSpace(body) == "" {
		return ""
	}
	return `<div style="margin-top:16px;border:1px solid #e2e8f0;border-radius:16px;background:#ffffff;overflow:hidden;">
  <div style="padding:14px 16px;border-bottom:1px solid #e2e8f0;background:#f8fafc;font-size:11px;font-weight:800;color:#64748b;text-transform:uppercase;letter-spacing:0.12em;">` + html.EscapeString(title) + `</div>
  <div style="padding:16px;color:#334155;font-size:14px;line-height:1.65;">` + body + `</div>
</div>`
}

func buildMailNotice(text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	return `<div style="margin-top:16px;padding:14px 16px;border:1px solid #fde68a;background:#fffbeb;border-radius:14px;color:#92400e;font-size:13px;line-height:1.6;">` + html.EscapeString(text) + `</div>`
}

func buildMailMetaTable(rows []mailMetaRow) string {
	if len(rows) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="width:100%;border-collapse:collapse;">`)
	for _, row := range rows {
		b.WriteString(`<tr><td valign="top" style="padding:10px 10px 10px 0;border-bottom:1px solid #e5e7eb;color:#64748b;font-size:12px;font-weight:800;text-transform:uppercase;letter-spacing:0.08em;">`)
		b.WriteString(html.EscapeString(row.Label))
		b.WriteString(`</td><td valign="top" align="right" style="padding:10px 0;border-bottom:1px solid #e5e7eb;color:#111827;font-size:14px;font-weight:700;word-break:break-word;">`)
		b.WriteString(html.EscapeString(row.Value))
		b.WriteString(`</td></tr>`)
	}
	b.WriteString(`</table>`)
	return b.String()
}

func buildMailOrderedList(items []string) string {
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<ol style="margin:0;padding-left:22px;color:#334155;">`)
	for _, item := range items {
		if strings.TrimSpace(item) == "" {
			continue
		}
		b.WriteString(`<li style="margin:0 0 8px;">` + html.EscapeString(item) + `</li>`)
	}
	b.WriteString(`</ol>`)
	return b.String()
}

func buildMailUnorderedList(items []string) string {
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<ul style="margin:0;padding-left:22px;color:#334155;">`)
	for _, item := range items {
		if strings.TrimSpace(item) == "" {
			continue
		}
		b.WriteString(`<li style="margin:0 0 8px;">` + html.EscapeString(item) + `</li>`)
	}
	b.WriteString(`</ul>`)
	return b.String()
}

func buildMailParagraphs(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	parts := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	var b strings.Builder
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		b.WriteString(`<p style="margin:0 0 10px;color:#334155;font-size:14px;line-height:1.65;">` + html.EscapeString(part) + `</p>`)
	}
	return b.String()
}

func shareLinkPublicURL(link domain.ShareLink, publicBaseURL string) string {
	if strings.TrimSpace(link.Token) == "" || strings.TrimSpace(publicBaseURL) == "" {
		return ""
	}
	return strings.TrimRight(publicBaseURL, "/") + "/share/" + strings.TrimSpace(link.Token)
}

func buildClientAccessHTML(client domain.Client, extraMessage string, artifacts []domain.Artifact, shareLinks []domain.ShareLink, attachmentNotes []string, publicBaseURL string) string {
	clientName := firstNonEmpty(client.DisplayName, client.Username, "client")
	sections := []string{}
	if strings.TrimSpace(extraMessage) != "" {
		sections = append(sections, buildMailSection("Operator message", buildMailParagraphs(extraMessage)))
	}
	if len(attachmentNotes) > 0 {
		sections = append(sections, buildMailSection("Attachments", buildMailUnorderedList(attachmentNotes)))
	}
	if len(artifacts) > 0 {
		items := make([]string, 0, len(artifacts))
		for _, artifact := range artifacts {
			items = append(items, firstNonEmpty(artifact.ArtifactType, "artifact")+" ["+firstNonEmpty(artifact.Status, "unknown")+"]")
		}
		sections = append(sections, buildMailSection("Prepared artifacts", buildMailUnorderedList(items)))
	}
	if len(shareLinks) > 0 {
		rows := make([]mailMetaRow, 0, len(shareLinks)*2)
		for i, link := range shareLinks {
			label := "Temporary link"
			if len(shareLinks) > 1 {
				label = label + " " + strconv.Itoa(i+1)
			}
			url := shareLinkPublicURL(link, publicBaseURL)
			rows = append(rows, mailMetaRow{label, firstNonEmpty(url, "one-time token not visible after creation")})
			rows = append(rows, mailMetaRow{"Token hint", firstNonEmpty(link.TokenHint, tokenHint(link.Token), "n/a")})
			rows = append(rows, mailMetaRow{"Expires at", link.ExpiresAt.Format(time.RFC3339)})
		}
		sections = append(sections, buildMailSection("Temporary access links", buildMailMetaTable(rows)))
	}
	sections = append(sections, buildMailNotice("Treat attached profiles, configuration files and temporary links as secrets. Do not publish or forward them outside the intended recipient."))

	var action *mailAction
	for _, link := range shareLinks {
		if url := shareLinkPublicURL(link, publicBaseURL); url != "" {
			action = &mailAction{
				Label: "Open access package",
				URL:   url,
				Hint:  "This temporary link is time-limited. Keep it private.",
			}
			break
		}
	}
	return buildMailLayout(mailLayout{
		Title:     "MegaVPN access package",
		Preheader: "Your MegaVPN access package is ready.",
		Eyebrow:   "Client access package",
		Heading:   "Your VPN access package is ready",
		Intro:     "MegaVPN access materials have been prepared for your account. Use the attached profiles or the temporary access link below according to your administrator's instructions.",
		Badges: []mailBadge{
			{Label: "Client", Value: clientName, Detail: firstNonEmpty(client.Email, "Recipient account")},
			{Label: "Attachments", Value: strconv.Itoa(len(attachmentNotes)), Detail: "Files included with this message."},
			{Label: "Links", Value: strconv.Itoa(len(shareLinks)), Detail: "Temporary access links generated."},
		},
		Action:   action,
		Sections: sections,
	})
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
			b.WriteString("- token hint: " + firstNonEmpty(link.TokenHint, tokenHint(link.Token)) + " expires: " + link.ExpiresAt.Format(time.RFC3339))
			if strings.TrimSpace(link.Token) != "" && strings.TrimSpace(publicBaseURL) != "" {
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
