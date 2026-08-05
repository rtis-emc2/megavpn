package http

import (
	"testing"
	"time"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

func TestPublicSessionResponseDoesNotExposeBearerToken(t *testing.T) {
	payload := publicSessionResponse(domain.UserSession{
		ID:        "session-1",
		ExpiresAt: time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC),
	})
	if _, ok := payload["token"]; ok {
		t.Fatalf("public session response must not expose bearer token: %#v", payload)
	}
	if payload["id"] != "session-1" {
		t.Fatalf("session id = %v, want session-1", payload["id"])
	}
}
