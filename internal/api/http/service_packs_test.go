package http

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
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
