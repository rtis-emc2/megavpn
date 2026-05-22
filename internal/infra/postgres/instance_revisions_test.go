package postgres

import "testing"

func TestStaticInstanceValidationErrorsRequiresConfigMaterial(t *testing.T) {
	errors := staticInstanceValidationErrors(map[string]any{})
	if len(errors) == 0 {
		t.Fatal("expected validation errors for empty rendered spec")
	}
}

func TestStaticInstanceValidationErrorsAcceptsConfigContent(t *testing.T) {
	errors := staticInstanceValidationErrors(map[string]any{
		"config_content": "listen 443;\n",
	})
	if len(errors) != 0 {
		t.Fatalf("expected no validation errors, got %v", errors)
	}
}

func TestStaticManagedFileErrors(t *testing.T) {
	errors := staticManagedFileErrors([]any{
		map[string]any{"path": "/etc/test.conf"},
	})
	if len(errors) == 0 {
		t.Fatal("expected validation errors for file without content/json")
	}
}

func TestRenderedInstanceSpecHashStable(t *testing.T) {
	spec := map[string]any{
		"config_json": map[string]any{
			"listen": 443,
		},
	}
	left := renderedInstanceSpecHash(spec)
	right := renderedInstanceSpecHash(spec)
	if left == "" {
		t.Fatal("expected non-empty rendered hash")
	}
	if left != right {
		t.Fatalf("expected deterministic rendered hash, got %q and %q", left, right)
	}
}
