package http

import (
	"strings"
	"testing"
	"time"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

func TestBuildClientAccessBodyDoesNotInventShareURLWithoutPlaintextToken(t *testing.T) {
	t.Parallel()

	body := buildClientAccessBody(domain.Client{Username: "client-a"}, "", nil, []domain.ShareLink{
		{TokenHint: "deadbeef...cafe01", ExpiresAt: time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)},
	}, nil, "https://control.example")

	if strings.Contains(body, "/share/") {
		t.Fatalf("body must not include public share URL when plaintext token is unavailable:\n%s", body)
	}
	if !strings.Contains(body, "deadbeef...cafe01") {
		t.Fatalf("body should include token hint for operator traceability:\n%s", body)
	}
}

func TestBuildClientAccessBodyIncludesNewlyCreatedShareURL(t *testing.T) {
	t.Parallel()

	body := buildClientAccessBody(domain.Client{Username: "client-a"}, "", nil, []domain.ShareLink{
		{Token: "plain-once-token", TokenHint: "plain-on...-token", ExpiresAt: time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)},
	}, nil, "https://control.example")

	if !strings.Contains(body, "https://control.example/share/plain-once-token") {
		t.Fatalf("body should include URL for a newly created one-time visible token:\n%s", body)
	}
}
