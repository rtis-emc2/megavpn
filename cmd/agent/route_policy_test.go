package main

import "testing"

func TestRoutePolicyPathSafety(t *testing.T) {
	t.Parallel()

	allowed := []string{
		defaultRoutePolicyPath,
		"/etc/megavpn/policies/client-access-routes.json",
	}
	for _, path := range allowed {
		if !isSafeRoutePolicyPath(path) {
			t.Fatalf("expected %q to be allowed", path)
		}
	}
	blocked := []string{
		"",
		"/tmp/client-access-routes.json",
		"/etc/megavpn/../shadow",
		"/etc/megavpn/policy\x00.json",
	}
	for _, path := range blocked {
		if isSafeRoutePolicyPath(path) {
			t.Fatalf("expected %q to be blocked", path)
		}
	}
}

func TestValidateRoutePolicyPayload(t *testing.T) {
	t.Parallel()

	if err := validateRoutePolicyPayload(map[string]any{"routes": []any{}}); err != nil {
		t.Fatalf("expected valid route policy payload, got %v", err)
	}
	if err := validateRoutePolicyPayload(map[string]any{"routes": "bad"}); err == nil {
		t.Fatal("expected invalid routes payload")
	}
}
