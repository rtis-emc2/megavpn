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

func TestNormalizeServicePackTemplateGeneratesKeyFromLabel(t *testing.T) {
	pack, err := normalizeServicePackTemplate(domain.ServicePackDefinition{
		Label:            "Premium Access Suite",
		BaseNameTemplate: "premium-access",
		Components: []domain.ServicePackComponent{
			{Label: "WireGuard", ServiceCode: "wireguard"},
		},
	}, 0)
	if err != nil {
		t.Fatalf("normalizeServicePackTemplate() error = %v", err)
	}
	if pack.Key != "premium-access-suite" {
		t.Fatalf("Key = %q, want premium-access-suite", pack.Key)
	}
}

func TestDedupeServicePackDefinitionsCollapsesSemanticDuplicates(t *testing.T) {
	packs := []domain.ServicePackDefinition{
		testServicePack("default_access_suite", "Default Remote Access Suite", "default", 1, 10, "xray-core", "reality_tcp"),
		testServicePack("default_access_suite_legacy", "Default Remote Access Suite", "default", 2, 20, "xray-core", "reality_tcp"),
		testServicePack("xray_vless_reality", "Xray VLESS / Reality", "default", 1, 30, "xray-core", "reality_tcp"),
	}

	got := dedupeServicePackDefinitions(packs)
	if len(got) != 2 {
		t.Fatalf("dedupeServicePackDefinitions() length = %d, want 2: %#v", len(got), got)
	}
	if got[0].Key != "default_access_suite_legacy" {
		t.Fatalf("semantic duplicate winner = %q, want newer default_access_suite_legacy", got[0].Key)
	}
	if got[1].Key != "xray_vless_reality" {
		t.Fatalf("second pack = %q, want xray_vless_reality", got[1].Key)
	}
}

func TestDedupeServicePackDefinitionsPrefersCustomClone(t *testing.T) {
	packs := []domain.ServicePackDefinition{
		testServicePack("default_access_suite", "Default Remote Access Suite", "default", 3, 10, "xray-core", "reality_tcp"),
		testServicePack("operator_access_suite", "Default Remote Access Suite", "custom", 1, 20, "xray-core", "reality_tcp"),
	}

	got := dedupeServicePackDefinitions(packs)
	if len(got) != 1 {
		t.Fatalf("dedupeServicePackDefinitions() length = %d, want 1: %#v", len(got), got)
	}
	if got[0].Key != "operator_access_suite" {
		t.Fatalf("semantic duplicate winner = %q, want custom operator_access_suite", got[0].Key)
	}
}

func testServicePack(key, label, source string, version, displayOrder int, serviceCode, presetKey string) domain.ServicePackDefinition {
	return domain.ServicePackDefinition{
		Key:                  key,
		Label:                label,
		BaseNameTemplate:     "edge-access",
		EndpointHint:         "access.example.com",
		RequiresEndpointHost: true,
		Status:               "active",
		Source:               source,
		Version:              version,
		DisplayOrder:         displayOrder,
		Components: []domain.ServicePackComponent{
			{
				Label:                serviceCode,
				ServiceCode:          serviceCode,
				PresetKey:            presetKey,
				NameSuffix:           "runtime",
				SlugSuffix:           "runtime",
				EndpointPort:         443,
				RequiresEndpointHost: true,
				Spec:                 map[string]any{"service_profile": presetKey},
			},
		},
	}
}
