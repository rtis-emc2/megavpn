package http

import (
	"crypto/tls"
	"net/http"
	"testing"
	"time"
)

func TestTerminalSessionStoreConsumesTicketOnce(t *testing.T) {
	t.Parallel()

	store := newTerminalSessionStore()
	now := time.Now().UTC()
	ticket, err := store.create("node-1", "user-1", "session-1", now)
	if err != nil {
		t.Fatalf("create ticket failed: %v", err)
	}
	got, ok := store.consume(ticket.ID, now.Add(time.Second))
	if !ok {
		t.Fatal("expected ticket to be consumable")
	}
	if got.NodeID != "node-1" || got.UserID != "user-1" || got.SessionID != "session-1" {
		t.Fatalf("unexpected ticket payload: %#v", got)
	}
	if _, ok := store.consume(ticket.ID, now.Add(2*time.Second)); ok {
		t.Fatal("ticket must be one-time use")
	}
}

func TestTerminalSessionStoreRejectsExpiredTicket(t *testing.T) {
	t.Parallel()

	store := newTerminalSessionStore()
	now := time.Now().UTC()
	ticket, err := store.create("node-1", "user-1", "session-1", now)
	if err != nil {
		t.Fatalf("create ticket failed: %v", err)
	}
	if _, ok := store.consume(ticket.ID, now.Add(terminalSessionTTL+time.Second)); ok {
		t.Fatal("expired ticket must be rejected")
	}
}

func TestWebSocketAcceptKey(t *testing.T) {
	t.Parallel()

	got := websocketAcceptKey("dGhlIHNhbXBsZSBub25jZQ==")
	const want = "s3pPLMBiTxaQ9kYGzzhZRbK+xOo="
	if got != want {
		t.Fatalf("accept key = %q, want %q", got, want)
	}
}

func TestTerminalWebSocketOriginAllowed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		origin        string
		host          string
		publicBaseURL string
		tls           bool
		want          bool
	}{
		{name: "public base match", origin: "https://control.example.com", host: "127.0.0.1:8080", publicBaseURL: "https://control.example.com", want: true},
		{name: "public base default port match", origin: "https://control.example.com", host: "control.example.com", publicBaseURL: "https://control.example.com:443", want: true},
		{name: "public base port mismatch", origin: "https://control.example.com:8443", host: "control.example.com", publicBaseURL: "https://control.example.com", want: false},
		{name: "public base scheme mismatch", origin: "http://control.example.com", host: "control.example.com", publicBaseURL: "https://control.example.com", want: false},
		{name: "foreign origin", origin: "https://evil.example", host: "control.example.com", publicBaseURL: "https://control.example.com", want: false},
		{name: "missing origin", host: "control.example.com", publicBaseURL: "https://control.example.com", want: false},
		{name: "invalid origin scheme", origin: "file://control.example.com", host: "control.example.com", publicBaseURL: "https://control.example.com", want: false},
		{name: "origin path rejected", origin: "https://control.example.com/terminal", host: "control.example.com", publicBaseURL: "https://control.example.com", want: false},
		{name: "development http host match", origin: "http://localhost:8080", host: "localhost:8080", want: true},
		{name: "development https host match", origin: "https://localhost:8443", host: "localhost:8443", tls: true, want: true},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := &http.Request{Host: tt.host, Header: http.Header{}}
			req.Header.Set("Origin", tt.origin)
			if tt.tls {
				req.TLS = &tls.ConnectionState{}
			}
			if got := terminalWebSocketOriginAllowed(req, tt.publicBaseURL); got != tt.want {
				t.Fatalf("terminalWebSocketOriginAllowed() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTerminalKnownHostFingerprintMatches(t *testing.T) {
	t.Parallel()

	out := "256 SHA256:abcdefghijklmnopqrstuvwxyzABCDEFGH1234567890+/= node.example (ED25519)\n"
	if !terminalKnownHostFingerprintMatches(out, "SHA256:abcdefghijklmnopqrstuvwxyzABCDEFGH1234567890+/=") {
		t.Fatal("expected fingerprint match")
	}
	if terminalKnownHostFingerprintMatches(out, "SHA256:0000000000000000000000000000000000000000000=") {
		t.Fatal("unexpected fingerprint match")
	}
}
