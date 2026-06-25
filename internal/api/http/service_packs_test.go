package http

import (
	"strings"
	"testing"
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
