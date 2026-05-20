package mail

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"strings"
	"time"
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
	if strings.TrimSpace(cfg.Host) == "" || cfg.Port <= 0 {
		return fmt.Errorf("smtp host/port are required")
	}
	if strings.TrimSpace(msg.FromEmail) == "" {
		return fmt.Errorf("from email is required")
	}
	if len(msg.To) == 0 {
		return fmt.Errorf("at least one recipient is required")
	}
	addr := net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", cfg.Port))
	tlsMode := strings.ToLower(strings.TrimSpace(cfg.TLSMode))
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	var conn net.Conn
	var err error
	if tlsMode == "tls" || tlsMode == "ssl" || tlsMode == "smtps" || (tlsMode == "" && cfg.Port == 465) {
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12})
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return err
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		return err
	}
	defer client.Quit()

	if strings.EqualFold(cfg.TLSMode, "starttls") || strings.EqualFold(cfg.TLSMode, "starttls_required") {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12}); err != nil {
				return err
			}
		} else if strings.EqualFold(cfg.TLSMode, "starttls_required") {
			return fmt.Errorf("smtp server does not support STARTTLS")
		}
	}

	if !strings.EqualFold(cfg.AuthMode, "none") && strings.TrimSpace(cfg.Username) != "" {
		auth := smtpAuth(cfg)
		if err := client.Auth(auth); err != nil {
			return err
		}
	}

	if err := client.Mail(msg.FromEmail); err != nil {
		return err
	}
	for _, rcpt := range msg.To {
		if err := client.Rcpt(strings.TrimSpace(rcpt)); err != nil {
			return err
		}
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	defer w.Close()

	raw, err := buildMessage(msg)
	if err != nil {
		return err
	}
	if _, err := w.Write(raw); err != nil {
		return err
	}
	return w.Close()
}

func smtpAuth(cfg SMTPConfig) smtp.Auth {
	switch strings.ToLower(strings.TrimSpace(cfg.AuthMode)) {
	case "", "plain":
		return smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
	case "login":
		return &loginAuth{username: cfg.Username, password: cfg.Password}
	default:
		return smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
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
	var body bytes.Buffer
	from := msg.FromEmail
	if strings.TrimSpace(msg.FromName) != "" {
		from = mime.QEncoding.Encode("utf-8", msg.FromName) + " <" + msg.FromEmail + ">"
	}
	body.WriteString("From: " + from + "\r\n")
	body.WriteString("To: " + strings.Join(msg.To, ", ") + "\r\n")
	body.WriteString("Subject: " + mime.QEncoding.Encode("utf-8", msg.Subject) + "\r\n")
	body.WriteString("MIME-Version: 1.0\r\n")
	if strings.TrimSpace(msg.ReplyTo) != "" {
		body.WriteString("Reply-To: " + msg.ReplyTo + "\r\n")
	}
	if len(msg.Attachments) == 0 && strings.TrimSpace(msg.HTMLBody) == "" {
		body.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
		body.WriteString("\r\n")
		body.WriteString(msg.TextBody)
		return body.Bytes(), nil
	}

	mixedBoundary := fmt.Sprintf("megavpn-mixed-%d", time.Now().UnixNano())
	alternativeBoundary := fmt.Sprintf("megavpn-alt-%d", time.Now().UnixNano())
	if len(msg.Attachments) == 0 {
		body.WriteString("Content-Type: multipart/alternative; boundary=" + alternativeBoundary + "\r\n")
		body.WriteString("\r\n")
		writeAlternativeParts(&body, alternativeBoundary, msg)
		return body.Bytes(), nil
	}

	body.WriteString("Content-Type: multipart/mixed; boundary=" + mixedBoundary + "\r\n")
	body.WriteString("\r\n")
	if strings.TrimSpace(msg.HTMLBody) != "" {
		body.WriteString("--" + mixedBoundary + "\r\n")
		body.WriteString("Content-Type: multipart/alternative; boundary=" + alternativeBoundary + "\r\n\r\n")
		writeAlternativeParts(&body, alternativeBoundary, msg)
	} else {
		body.WriteString("--" + mixedBoundary + "\r\n")
		body.WriteString("Content-Type: text/plain; charset=utf-8\r\n\r\n")
		body.WriteString(msg.TextBody)
		body.WriteString("\r\n")
	}

	for _, attachment := range msg.Attachments {
		contentType := strings.TrimSpace(attachment.ContentType)
		if contentType == "" {
			contentType = "application/octet-stream"
		}
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

func writeAlternativeParts(body *bytes.Buffer, boundary string, msg Message) {
	body.WriteString("--" + boundary + "\r\n")
	body.WriteString("Content-Type: text/plain; charset=utf-8\r\n\r\n")
	body.WriteString(msg.TextBody)
	body.WriteString("\r\n")
	if strings.TrimSpace(msg.HTMLBody) != "" {
		body.WriteString("--" + boundary + "\r\n")
		body.WriteString("Content-Type: text/html; charset=utf-8\r\n\r\n")
		body.WriteString(msg.HTMLBody)
		body.WriteString("\r\n")
	}
	body.WriteString("--" + boundary + "--\r\n")
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
