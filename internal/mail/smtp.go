package mail

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	stdmail "net/mail"
	"net/smtp"
	"strings"
	"time"
)

const (
	maxRecipients           = 100
	maxAttachments          = 25
	maxAttachmentBytes      = 25 << 20
	maxTotalAttachmentBytes = 50 << 20
	defaultSendTimeout      = 30 * time.Second
)

type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	AuthMode string
	TLSMode  string
}

type Attachment struct {
	Filename    string
	ContentType string
	Data        []byte
}

type Message struct {
	FromEmail   string
	FromName    string
	ReplyTo     string
	To          []string
	Subject     string
	TextBody    string
	HTMLBody    string
	Attachments []Attachment
}

func SendSMTP(ctx context.Context, cfg SMTPConfig, msg Message) error {
	if ctx == nil {
		return errors.New("smtp context is required")
	}
	if strings.TrimSpace(cfg.Host) == "" || cfg.Port <= 0 {
		return fmt.Errorf("smtp host/port are required")
	}
	if err := validateMessage(msg); err != nil {
		return err
	}
	addr := net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", cfg.Port))
	tlsMode := strings.ToLower(strings.TrimSpace(cfg.TLSMode))
	if tlsMode == "" {
		if cfg.Port == 465 {
			tlsMode = "tls"
		} else {
			tlsMode = "starttls"
		}
	}
	if tlsMode != "tls" && tlsMode != "ssl" && tlsMode != "smtps" && tlsMode != "starttls" && tlsMode != "starttls_required" && tlsMode != "none" {
		return fmt.Errorf("unsupported smtp tls mode %q", cfg.TLSMode)
	}
	authMode := strings.ToLower(strings.TrimSpace(cfg.AuthMode))
	if authMode == "" {
		authMode = "plain"
	}
	if authMode != "none" && authMode != "plain" && authMode != "login" {
		return fmt.Errorf("unsupported smtp auth mode %q", cfg.AuthMode)
	}
	if tlsMode == "none" && authMode != "none" && strings.TrimSpace(cfg.Username) != "" {
		return errors.New("smtp authentication requires TLS")
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	var conn net.Conn
	var err error
	if tlsMode == "tls" || tlsMode == "ssl" || tlsMode == "smtps" {
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12})
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return err
	}
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()
	defer conn.Close()
	deadline := time.Now().Add(defaultSendTimeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return fmt.Errorf("set smtp connection deadline: %w", err)
	}

	client, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		return err
	}
	defer client.Close()

	if tlsMode == "starttls" || tlsMode == "starttls_required" {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12}); err != nil {
				return err
			}
		} else {
			return fmt.Errorf("smtp server does not support STARTTLS")
		}
	}

	if authMode != "none" && strings.TrimSpace(cfg.Username) != "" {
		auth, err := smtpAuth(cfg)
		if err != nil {
			return err
		}
		if err := client.Auth(auth); err != nil {
			return err
		}
	}

	fromAddress, _ := parseMailbox(msg.FromEmail)
	if err := client.Mail(fromAddress); err != nil {
		return err
	}
	for _, rcpt := range msg.To {
		recipient, _ := parseMailbox(rcpt)
		if err := client.Rcpt(recipient); err != nil {
			return err
		}
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	raw, err := buildMessage(msg)
	if err != nil {
		_ = w.Close()
		return err
	}
	if _, err := w.Write(raw); err != nil {
		_ = w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func smtpAuth(cfg SMTPConfig) (smtp.Auth, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.AuthMode)) {
	case "", "plain":
		return smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host), nil
	case "login":
		return &loginAuth{username: cfg.Username, password: cfg.Password}, nil
	case "none":
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported smtp auth mode %q", cfg.AuthMode)
	}
}

type loginAuth struct {
	username string
	password string
}

func (a *loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	return "LOGIN", nil, nil
}

func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	prompt := strings.ToLower(strings.TrimSpace(string(fromServer)))
	switch {
	case strings.Contains(prompt, "username"):
		return []byte(a.username), nil
	case strings.Contains(prompt, "password"):
		return []byte(a.password), nil
	default:
		return nil, fmt.Errorf("unexpected LOGIN auth prompt: %s", prompt)
	}
}

func buildMessage(msg Message) ([]byte, error) {
	if err := validateMessage(msg); err != nil {
		return nil, err
	}
	var body bytes.Buffer
	fromAddress, _ := parseMailbox(msg.FromEmail)
	from := (&stdmail.Address{Name: strings.TrimSpace(msg.FromName), Address: fromAddress}).String()
	recipients := make([]string, 0, len(msg.To))
	for _, recipient := range msg.To {
		address, _ := parseMailbox(recipient)
		recipients = append(recipients, (&stdmail.Address{Address: address}).String())
	}
	body.WriteString("From: " + from + "\r\n")
	body.WriteString("To: " + strings.Join(recipients, ", ") + "\r\n")
	body.WriteString("Subject: " + mime.QEncoding.Encode("utf-8", msg.Subject) + "\r\n")
	body.WriteString("Date: " + time.Now().UTC().Format(time.RFC1123Z) + "\r\n")
	body.WriteString("MIME-Version: 1.0\r\n")
	if strings.TrimSpace(msg.ReplyTo) != "" {
		replyTo, _ := parseMailbox(msg.ReplyTo)
		body.WriteString("Reply-To: " + (&stdmail.Address{Address: replyTo}).String() + "\r\n")
	}
	if len(msg.Attachments) == 0 && strings.TrimSpace(msg.HTMLBody) == "" {
		body.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
		body.WriteString("Content-Transfer-Encoding: quoted-printable\r\n")
		body.WriteString("\r\n")
		if err := writeQuotedPrintable(&body, msg.TextBody); err != nil {
			return nil, err
		}
		return body.Bytes(), nil
	}

	mixedBoundary := fmt.Sprintf("megavpn-mixed-%d", time.Now().UnixNano())
	alternativeBoundary := fmt.Sprintf("megavpn-alt-%d", time.Now().UnixNano())
	if len(msg.Attachments) == 0 {
		body.WriteString("Content-Type: multipart/alternative; boundary=" + alternativeBoundary + "\r\n")
		body.WriteString("\r\n")
		if err := writeAlternativeParts(&body, alternativeBoundary, msg); err != nil {
			return nil, err
		}
		return body.Bytes(), nil
	}

	body.WriteString("Content-Type: multipart/mixed; boundary=" + mixedBoundary + "\r\n")
	body.WriteString("\r\n")
	if strings.TrimSpace(msg.HTMLBody) != "" {
		body.WriteString("--" + mixedBoundary + "\r\n")
		body.WriteString("Content-Type: multipart/alternative; boundary=" + alternativeBoundary + "\r\n\r\n")
		if err := writeAlternativeParts(&body, alternativeBoundary, msg); err != nil {
			return nil, err
		}
	} else {
		body.WriteString("--" + mixedBoundary + "\r\n")
		body.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
		body.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
		if err := writeQuotedPrintable(&body, msg.TextBody); err != nil {
			return nil, err
		}
		body.WriteString("\r\n")
	}

	for _, attachment := range msg.Attachments {
		contentType := normalizedContentType(attachment.ContentType)
		body.WriteString("--" + mixedBoundary + "\r\n")
		body.WriteString("Content-Type: " + contentType + "\r\n")
		body.WriteString("Content-Transfer-Encoding: base64\r\n")
		body.WriteString("Content-Disposition: attachment; filename=\"" + sanitizeHeaderFilename(attachment.Filename) + "\"\r\n\r\n")
		encoded := make([]byte, base64.StdEncoding.EncodedLen(len(attachment.Data)))
		base64.StdEncoding.Encode(encoded, attachment.Data)
		for len(encoded) > 76 {
			body.Write(encoded[:76])
			body.WriteString("\r\n")
			encoded = encoded[76:]
		}
		body.Write(encoded)
		body.WriteString("\r\n")
	}
	body.WriteString("--" + mixedBoundary + "--\r\n")
	return body.Bytes(), nil
}

func validateMessage(msg Message) error {
	if _, err := parseMailbox(msg.FromEmail); err != nil {
		return fmt.Errorf("invalid from email: %w", err)
	}
	if hasHeaderControl(msg.FromName) {
		return errors.New("from name contains prohibited header characters")
	}
	if hasHeaderControl(msg.Subject) {
		return errors.New("subject contains prohibited header characters")
	}
	if len(msg.To) == 0 {
		return errors.New("at least one recipient is required")
	}
	if len(msg.To) > maxRecipients {
		return fmt.Errorf("too many recipients: %d exceeds %d", len(msg.To), maxRecipients)
	}
	for _, recipient := range msg.To {
		if _, err := parseMailbox(recipient); err != nil {
			return fmt.Errorf("invalid recipient: %w", err)
		}
	}
	if strings.TrimSpace(msg.ReplyTo) != "" {
		if _, err := parseMailbox(msg.ReplyTo); err != nil {
			return fmt.Errorf("invalid reply-to email: %w", err)
		}
	}
	if len(msg.Attachments) > maxAttachments {
		return fmt.Errorf("too many attachments: %d exceeds %d", len(msg.Attachments), maxAttachments)
	}
	total := 0
	for _, attachment := range msg.Attachments {
		if len(attachment.Data) > maxAttachmentBytes {
			return fmt.Errorf("attachment %q exceeds %d bytes", sanitizeHeaderFilename(attachment.Filename), maxAttachmentBytes)
		}
		total += len(attachment.Data)
		if total > maxTotalAttachmentBytes {
			return fmt.Errorf("total attachment size exceeds %d bytes", maxTotalAttachmentBytes)
		}
		if _, _, err := mime.ParseMediaType(normalizedContentType(attachment.ContentType)); err != nil {
			return fmt.Errorf("invalid attachment content type: %w", err)
		}
	}
	return nil
}

func parseMailbox(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("email address is required")
	}
	if hasHeaderControl(value) {
		return "", errors.New("email address contains prohibited header characters")
	}
	parsed, err := stdmail.ParseAddress(value)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(parsed.Address) == "" {
		return "", errors.New("email address is empty")
	}
	return parsed.Address, nil
}

func hasHeaderControl(value string) bool {
	return strings.ContainsAny(value, "\r\n\x00")
}

func normalizedContentType(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "application/octet-stream"
	}
	mediaType, params, err := mime.ParseMediaType(value)
	if err != nil {
		return value
	}
	return mime.FormatMediaType(mediaType, params)
}

func writeAlternativeParts(body *bytes.Buffer, boundary string, msg Message) error {
	body.WriteString("--" + boundary + "\r\n")
	body.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	body.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
	if err := writeQuotedPrintable(body, msg.TextBody); err != nil {
		return err
	}
	body.WriteString("\r\n")
	if strings.TrimSpace(msg.HTMLBody) != "" {
		body.WriteString("--" + boundary + "\r\n")
		body.WriteString("Content-Type: text/html; charset=utf-8\r\n")
		body.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
		if err := writeQuotedPrintable(body, msg.HTMLBody); err != nil {
			return err
		}
		body.WriteString("\r\n")
	}
	body.WriteString("--" + boundary + "--\r\n")
	return nil
}

func writeQuotedPrintable(body *bytes.Buffer, value string) error {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	value = strings.ReplaceAll(value, "\n", "\r\n")
	w := quotedprintable.NewWriter(body)
	if _, err := w.Write([]byte(value)); err != nil {
		_ = w.Close()
		return fmt.Errorf("encode MIME body: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("encode MIME body: %w", err)
	}
	return nil
}

func sanitizeHeaderFilename(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "attachment.bin"
	}
	name = strings.ReplaceAll(name, "\"", "_")
	name = strings.ReplaceAll(name, "\n", "_")
	name = strings.ReplaceAll(name, "\r", "_")
	return name
}
