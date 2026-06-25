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

func TestBuildClientAccessHTMLDoesNotInventShareURLWithoutPlaintextToken(t *testing.T) {
	t.Parallel()

	body := buildClientAccessHTML(domain.Client{Username: "client-a", Email: "client@example.invalid"}, "<b>operator note</b>", nil, []domain.ShareLink{
		{TokenHint: "deadbeef...cafe01", ExpiresAt: time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)},
	}, []string{"wireguard attached as wg.conf"}, "https://control.example")

	if strings.Contains(body, "/share/") {
		t.Fatalf("html body must not include public share URL when plaintext token is unavailable:\n%s", body)
	}
	if !strings.Contains(body, "deadbeef...cafe01") {
		t.Fatalf("html body should include token hint for operator traceability:\n%s", body)
	}
	if strings.Contains(body, "<b>operator note</b>") || !strings.Contains(body, "&lt;b&gt;operator note&lt;/b&gt;") {
		t.Fatalf("operator message must be HTML-escaped:\n%s", body)
	}
}

func TestBuildClientAccessHTMLIncludesNewlyCreatedShareURL(t *testing.T) {
	t.Parallel()

	body := buildClientAccessHTML(domain.Client{Username: "client-a"}, "", nil, []domain.ShareLink{
		{Token: "plain-once-token", TokenHint: "plain-on...-token", ExpiresAt: time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)},
	}, nil, "https://control.example")

	if !strings.Contains(body, "https://control.example/share/plain-once-token") {
		t.Fatalf("html body should include URL for a newly created one-time visible token:\n%s", body)
	}
	if !strings.Contains(body, `table role="presentation"`) {
		t.Fatalf("html body should use email-safe table layout:\n%s", body)
	}
}

func TestSystemMailHTMLUsesModernSharedLayout(t *testing.T) {
	t.Parallel()

	invite := buildOperatorInviteHTML(
		domain.PlatformUserRecord{
			PlatformUser: domain.PlatformUser{Username: "operator-a", Email: "operator@example.invalid"},
			RoleCodes:    []string{"admin"},
		},
		domain.PlatformUser{Username: "admin"},
		"https://control.example/?invite_token=secret",
		time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC),
	)
	testMail := buildPlatformMailTestHTML(
		domain.PlatformUser{Username: "admin", Email: "admin@example.invalid"},
		domain.PlatformMailSettings{SMTPHost: "smtp.example.invalid", SMTPPort: 587, SMTPTLSMode: "starttls", SMTPAuthMode: "plain", FromEmail: "noreply@example.invalid"},
		"ops@example.invalid",
		time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC),
	)
	for name, body := range map[string]string{"invite": invite, "test": testMail} {
		if !strings.Contains(body, "NLGate MegaVPN") {
			t.Fatalf("%s email should include updated product branding:\n%s", name, body)
		}
		if !strings.Contains(body, `table role="presentation"`) {
			t.Fatalf("%s email should use email-safe table layout:\n%s", name, body)
		}
		if strings.Contains(body, "radial-gradient") || strings.Contains(body, "display:grid") {
			t.Fatalf("%s email should not use old decorative/grid layout:\n%s", name, body)
		}
	}
}
