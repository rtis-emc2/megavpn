package http

import (
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
