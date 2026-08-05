package http

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

// TestRenderMailPreviews is an opt-in design QA helper. Normal test runs skip
// filesystem output; release reviews can set MEGAVPN_MAIL_PREVIEW_DIR.
func TestRenderMailPreviews(t *testing.T) {
	dir := os.Getenv("MEGAVPN_MAIL_PREVIEW_DIR")
	if dir == "" {
		t.Skip("MEGAVPN_MAIL_PREVIEW_DIR is not set")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	expiresAt := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	previews := map[string]string{
		"operator-invite.html": buildOperatorInviteHTML(
			domain.PlatformUserRecord{
				PlatformUser: domain.PlatformUser{Username: "operator.smith", DisplayName: "Alex Smith", Email: "alex@example.com"},
				RoleCodes:    []string{"engineer"},
			},
			domain.PlatformUser{Username: "security.admin", DisplayName: "Security Administrator"},
			"https://control.example.com/?invite_token=preview-token",
			expiresAt,
		),
		"client-access.html": buildClientAccessHTML(
			domain.Client{Username: "client.corporate.iphone", DisplayName: "Corporate iPhone", Email: "client@example.com"},
			"Install one profile only. Keep this message private.",
			[]domain.Artifact{{ArtifactType: "vless-url", Status: "ready"}, {ArtifactType: "wireguard-conf", Status: "ready"}},
			[]domain.ShareLink{{Token: "preview-token", TokenHint: "preview...token", ExpiresAt: expiresAt}},
			[]string{"VLESS profile attached as client.txt", "WireGuard profile attached as client.conf"},
			"https://control.example.com",
		),
	}
	for name, content := range previews {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
