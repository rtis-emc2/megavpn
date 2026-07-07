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

func TestRoutePolicyTelemetryTablesIncludeCurrentAndPreviousManagedTables(t *testing.T) {
	t.Parallel()

	tables := routePolicyTelemetryTables(
		[]any{
			map[string]any{
				"egress": map[string]any{"table": "21001"},
			},
			map[string]any{
				"egress": map[string]any{"table": "main"},
			},
		},
		[]any{
			map[string]any{"table": "59714"},
			map[string]any{"table": "bad;table"},
		},
		routePolicyCleanupSnapshot{
			Routes: []any{
				map[string]any{
					"status":           "active",
					"action":           "allow",
					"destination_type": "cidr",
					"destination":      "203.0.113.0/24",
					"egress":           map[string]any{"status": "candidate", "table": "21002"},
					"enforcement":      map[string]any{"mode": "l3_l4_candidate"},
				},
			},
		},
	)
	want := []string{"21001", "21002", "59714"}
	if len(tables) != len(want) {
		t.Fatalf("tables = %#v, want %#v", tables, want)
	}
	for idx := range want {
		if tables[idx] != want[idx] {
			t.Fatalf("tables = %#v, want %#v", tables, want)
		}
	}
}
