package mail

import (
	"context"
	"strings"
	"testing"
)

func TestBuildMessageRejectsHeaderInjection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		msg  Message
	}{
		{
			name: "from address",
			msg:  validMessage(Message{FromEmail: "sender@example.invalid\r\nBcc: attacker@example.invalid"}),
		},
		{
			name: "from name",
			msg:  validMessage(Message{FromName: "MegaVPN\nBcc: attacker@example.invalid"}),
		},
		{
			name: "recipient",
			msg:  validMessage(Message{To: []string{"recipient@example.invalid\r\nBcc: attacker@example.invalid"}}),
		},
		{
			name: "reply to",
			msg:  validMessage(Message{ReplyTo: "reply@example.invalid\nX-Injected: yes"}),
		},
		{
			name: "subject",
			msg:  validMessage(Message{Subject: "Status\r\nBcc: attacker@example.invalid"}),
		},
		{
			name: "attachment content type",
			msg: validMessage(Message{Attachments: []Attachment{{
				Filename:    "client.conf",
				ContentType: "text/plain\r\nX-Injected: yes",
				Data:        []byte("profile"),
			}}}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := buildMessage(tt.msg); err == nil {
				t.Fatal("expected unsafe message to be rejected")
			}
		})
	}
}

func TestBuildMessageUsesCanonicalHeadersAndMultipartBody(t *testing.T) {
	t.Parallel()

	raw, err := buildMessage(validMessage(Message{
		FromName: "NLGate Operations",
		ReplyTo:  "support@example.invalid",
		HTMLBody: "<strong>Ready</strong>",
		Attachments: []Attachment{{
			Filename:    "client.conf",
			ContentType: "text/plain; charset=utf-8",
			Data:        []byte("profile"),
		}},
	}))
	if err != nil {
		t.Fatalf("build message: %v", err)
	}
	body := string(raw)
	for _, want := range []string{
		"From: \"NLGate Operations\" <sender@example.invalid>\r\n",
		"To: <recipient@example.invalid>\r\n",
		"Reply-To: <support@example.invalid>\r\n",
		"Date: ",
		"Content-Type: multipart/mixed;",
		"Content-Type: multipart/alternative;",
		"Content-Transfer-Encoding: quoted-printable",
		"Content-Disposition: attachment; filename=\"client.conf\"",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("message does not contain %q:\n%s", want, body)
		}
	}
}

func TestBuildMessageEncodesUTF8BodySafely(t *testing.T) {
	t.Parallel()

	raw, err := buildMessage(validMessage(Message{TextBody: "Доступ готов"}))
	if err != nil {
		t.Fatalf("build message: %v", err)
	}
	body := string(raw)
	if !strings.Contains(body, "Content-Transfer-Encoding: quoted-printable") {
		t.Fatalf("message does not declare quoted-printable transfer encoding:\n%s", body)
	}
	if strings.Contains(body, "Доступ готов") || !strings.Contains(body, "=D0") {
		t.Fatalf("UTF-8 body was not quoted-printable encoded:\n%s", body)
	}
}

func TestValidateMessageEnforcesRecipientAndAttachmentBounds(t *testing.T) {
	t.Parallel()

	manyRecipients := make([]string, maxRecipients+1)
	for i := range manyRecipients {
		manyRecipients[i] = "recipient@example.invalid"
	}
	if err := validateMessage(validMessage(Message{To: manyRecipients})); err == nil {
		t.Fatal("expected recipient bound validation error")
	}

	large := make([]byte, maxAttachmentBytes+1)
	if err := validateMessage(validMessage(Message{Attachments: []Attachment{{Filename: "large.bin", Data: large}}})); err == nil {
		t.Fatal("expected attachment size validation error")
	}
}

func TestSendSMTPRejectsInsecureAuthenticationBeforeDial(t *testing.T) {
	t.Parallel()

	err := SendSMTP(context.Background(), SMTPConfig{
		Host:     "smtp.example.invalid",
		Port:     25,
		Username: "operator",
		Password: "secret",
		AuthMode: "plain",
		TLSMode:  "none",
	}, validMessage(Message{}))
	if err == nil || !strings.Contains(err.Error(), "requires TLS") {
		t.Fatalf("expected insecure authentication rejection, got %v", err)
	}
}

func validMessage(overrides Message) Message {
	msg := Message{
		FromEmail: "sender@example.invalid",
		To:        []string{"recipient@example.invalid"},
		Subject:   "MegaVPN notification",
		TextBody:  "Ready",
	}
	if overrides.FromEmail != "" {
		msg.FromEmail = overrides.FromEmail
	}
	if overrides.FromName != "" {
		msg.FromName = overrides.FromName
	}
	if overrides.ReplyTo != "" {
		msg.ReplyTo = overrides.ReplyTo
	}
	if overrides.To != nil {
		msg.To = overrides.To
	}
	if overrides.Subject != "" {
		msg.Subject = overrides.Subject
	}
	if overrides.TextBody != "" {
		msg.TextBody = overrides.TextBody
	}
	if overrides.HTMLBody != "" {
		msg.HTMLBody = overrides.HTMLBody
	}
	if overrides.Attachments != nil {
		msg.Attachments = overrides.Attachments
	}
	return msg
}
