package postgres

import (
	"testing"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

func TestNormalizeServicePackTemplateUsesJSONArrayDefaults(t *testing.T) {
	pack, err := normalizeServicePackTemplate(domain.ServicePackDefinition{
		Key:              "test_pack",
		Label:            "Test Pack",
		BaseNameTemplate: "edge-test",
		Components: []domain.ServicePackComponent{
			{
				Label:       "WireGuard",
				ServiceCode: "wireguard",
				NameSuffix:  "wg",
				SlugSuffix:  "wg",
			},
		},
	}, 0)
	if err != nil {
		t.Fatalf("normalizeServicePackTemplate() error = %v", err)
	}
	if pack.PlatformNotes == nil {
		t.Fatal("PlatformNotes must be a non-nil empty slice")
	}
	if pack.Recommendations == nil {
		t.Fatal("Recommendations must be a non-nil empty slice")
	}
	if got := string(mustJSON(pack.PlatformNotes)); got != "[]" {
		t.Fatalf("PlatformNotes JSON = %s, want []", got)
	}
	if got := string(mustJSON(pack.Recommendations)); got != "[]" {
		t.Fatalf("Recommendations JSON = %s, want []", got)
	}
	if got := string(mustJSON(pack.Components)); got == "null" {
		t.Fatal("Components JSON must not be null")
	}
}
