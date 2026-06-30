package http

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rtis-emc2/megavpn/internal/domain"
)

func TestDefaultServicePacksAreCreateReady(t *testing.T) {
	packs := servicePackDefinitions()
	if len(packs) == 0 {
		t.Fatal("expected default service packs")
	}
	seen := map[string]bool{}
	for _, pack := range packs {
		if strings.TrimSpace(pack.Key) == "" {
			t.Fatal("service pack key is required")
		}
		if seen[pack.Key] {
			t.Fatalf("duplicate service pack key %q", pack.Key)
		}
		seen[pack.Key] = true
		if strings.TrimSpace(pack.Label) == "" {
			t.Fatalf("service pack %q label is required", pack.Key)
		}
		if strings.TrimSpace(pack.BaseNameTemplate) == "" {
			t.Fatalf("service pack %q base_name_template is required", pack.Key)
		}
		if len(pack.Components) == 0 {
			t.Fatalf("service pack %q must define components", pack.Key)
		}
		for idx, component := range pack.Components {
			if strings.TrimSpace(component.ServiceCode) == "" {
				t.Fatalf("service pack %q component %d service_code is required", pack.Key, idx)
			}
			if strings.TrimSpace(component.NameSuffix) == "" {
				t.Fatalf("service pack %q component %d name_suffix is required", pack.Key, idx)
			}
			if strings.TrimSpace(component.SlugSuffix) == "" {
				t.Fatalf("service pack %q component %d slug_suffix is required", pack.Key, idx)
			}
			if component.Spec == nil {
				t.Fatalf("service pack %q component %d spec is required", pack.Key, idx)
			}
		}
	}
}

func TestDefaultAccessSuiteHasExpectedComponentsWithoutInlineSecrets(t *testing.T) {
	pack, found := findServicePack("default_access_suite")
	if !found {
		t.Fatal("default_access_suite service pack is required")
	}
	expected := []struct {
		serviceCode string
		presetKey   string
		port        int
	}{
		{"xray-core", "reality_tcp", 443},
		{"openvpn", "tcp_11994", 11994},
		{"xray-core", "reality_tcp", 8443},
		{"openvpn", "udp_1194", 1194},
		{"shadowsocks", "chacha_full", 8388},
		{"wireguard", "roadwarrior", 51820},
	}
	if len(pack.Components) != len(expected) {
		t.Fatalf("component count = %d, want %d", len(pack.Components), len(expected))
	}
	for idx, want := range expected {
		component := pack.Components[idx]
		if component.ServiceCode != want.serviceCode || component.PresetKey != want.presetKey || component.EndpointPort != want.port {
			t.Fatalf("component %d = %s/%s/%d, want %s/%s/%d", idx, component.ServiceCode, component.PresetKey, component.EndpointPort, want.serviceCode, want.presetKey, want.port)
		}
		for _, secretKey := range []string{
			"password",
			"server_password",
			"server_private_key",
			"reality_private_key",
			"short_id",
			"short_ids",
			"psk",
			"private_key",
		} {
			if _, exists := component.Spec[secretKey]; exists {
				t.Fatalf("component %d must not include inline secret key %q", idx, secretKey)
			}
		}
	}
}

func TestMissingServicePackRuntimeCapabilitiesDeduplicatesRuntime(t *testing.T) {
	components := []domain.ServicePackComponent{
		{ServiceCode: "xray-core"},
		{ServiceCode: "xray-core"},
		{ServiceCode: "openvpn"},
		{ServiceCode: "wireguard"},
	}
	capabilities := []domain.NodeCapability{
		{CapabilityCode: "openvpn", Status: "available"},
	}
	got := missingServicePackRuntimeCapabilities(components, capabilities)
	want := []string{"xray-core", "wireguard"}
	if len(got) != len(want) {
		t.Fatalf("missing runtime count = %d, want %d: %#v", len(got), len(want), got)
	}
	for idx := range want {
		if got[idx] != want[idx] {
			t.Fatalf("missing runtime %d = %q, want %q; all=%#v", idx, got[idx], want[idx], got)
		}
	}
}

func TestServicePackTLSEdgeCertificateScope(t *testing.T) {
	cases := []struct {
		name      string
		component domain.ServicePackComponent
		want      bool
	}{
		{
			name:      "nginx always terminates edge TLS",
			component: domain.ServicePackComponent{ServiceCode: "nginx", Spec: map[string]any{}},
			want:      true,
		},
		{
			name:      "xray tls uses edge certificate",
			component: domain.ServicePackComponent{ServiceCode: "xray-core", Spec: map[string]any{"security": "tls"}},
			want:      true,
		},
		{
			name:      "xray reality does not use leaf TLS certificate",
			component: domain.ServicePackComponent{ServiceCode: "xray-core", Spec: map[string]any{"security": "reality"}},
			want:      false,
		},
		{
			name:      "openvpn uses service pki profile instead",
			component: domain.ServicePackComponent{ServiceCode: "openvpn", Spec: map[string]any{}},
			want:      false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := servicePackComponentUsesTLSEdgeCertificate(tc.component); got != tc.want {
				t.Fatalf("servicePackComponentUsesTLSEdgeCertificate() = %v, want %v", got, tc.want)
			}
			pack := domain.ServicePackDefinition{Components: []domain.ServicePackComponent{tc.component}}
			if got := servicePackUsesTLSEdgeCertificate(pack); got != tc.want {
				t.Fatalf("servicePackUsesTLSEdgeCertificate() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDefaultServicePackDefinitionsReturnsCopy(t *testing.T) {
	first := DefaultServicePackDefinitions()
	if len(first) == 0 || len(first[0].Components) == 0 {
		t.Fatal("expected default service pack definitions")
	}
	originalLabel := first[0].Label
	originalSpecValue := first[0].Components[0].Spec["service_profile"]
	first[0].Label = "mutated"
	first[0].Components[0].Spec["service_profile"] = "mutated"

	second := DefaultServicePackDefinitions()
	if second[0].Label != originalLabel {
		t.Fatalf("default pack label was mutated: %q", second[0].Label)
	}
	if second[0].Components[0].Spec["service_profile"] != originalSpecValue {
		t.Fatalf("default pack spec was mutated: %v", second[0].Components[0].Spec["service_profile"])
	}
}

func TestServicePackCatalogUnavailableDetection(t *testing.T) {
	if !isServicePackCatalogUnavailable(&pgconn.PgError{Code: "42P01", Message: "relation service_pack_templates does not exist"}) {
		t.Fatal("expected undefined table to be treated as catalog unavailable")
	}
	if !isServicePackCatalogUnavailable(&pgconn.PgError{Code: "42703", Message: "column display_order does not exist"}) {
		t.Fatal("expected undefined column to be treated as catalog unavailable")
	}
	if isServicePackCatalogUnavailable(&pgconn.PgError{Code: "23505", Message: "duplicate key"}) {
		t.Fatal("duplicate key must not be treated as catalog unavailable")
	}
}

func TestInterpolatePackSpecReplacesNestedPlaceholders(t *testing.T) {
	spec := map[string]any{
		"server_name": "{{endpoint_host}}",
		"nested": map[string]any{
			"url": "https://{{endpoint_host}}/{{component_slug}}",
		},
		"list": []any{"{{base_name}}", "static"},
	}
	out := interpolatePackSpec(spec, map[string]string{
		"endpoint_host":  "edge.example.com",
		"component_slug": "edge-xray",
		"base_name":      "edge",
	})
	if containsInterpolationToken(out) {
		t.Fatalf("interpolation left template tokens in %#v", out)
	}
	if out["server_name"] != "edge.example.com" {
		t.Fatalf("unexpected server_name %v", out["server_name"])
	}
}

func containsInterpolationToken(value any) bool {
	switch typed := value.(type) {
	case string:
		return strings.Contains(typed, "{{") || strings.Contains(typed, "}}")
	case map[string]any:
		for _, item := range typed {
			if containsInterpolationToken(item) {
				return true
			}
		}
	case []any:
		for _, item := range typed {
			if containsInterpolationToken(item) {
				return true
			}
		}
	}
	return false
}
