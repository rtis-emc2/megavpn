package main

import (
	"strings"
	"testing"
)

func TestRenderRoutePolicyKernelScriptWithManagedBackhaul(t *testing.T) {
	t.Parallel()

	plan, err := renderRoutePolicyKernelScript([]any{
		map[string]any{
			"route_id":         "route-1",
			"status":           "active",
			"action":           "allow",
			"destination_type": "cidr",
			"destination":      "203.0.113.0/24",
			"protocol":         "any",
			"source_identity": map[string]any{
				"type":  "wireguard_ip",
				"value": "10.66.0.2/32",
			},
			"egress": map[string]any{
				"status":    "candidate",
				"mode":      "remote_egress",
				"interface": "mgbh123",
				"table":     "21001",
				"managed_backhaul": map[string]any{
					"route_metric": 70,
				},
			},
			"enforcement": map[string]any{"mode": "l3_l4_candidate"},
		},
	}, "rev-1")
	if err != nil {
		t.Fatalf("renderRoutePolicyKernelScript returned error: %v", err)
	}
	if plan.RuleCount != 1 || plan.MarkedCount != 0 {
		t.Fatalf("unexpected plan counts: %#v", plan)
	}
	checks := []string{
		"ip route replace '203.0.113.0/24' dev 'mgbh123' table '21001' metric 70",
		"ip rule add from '10.66.0.2/32' to '203.0.113.0/24' table '21001' priority 22000",
	}
	for _, check := range checks {
		if !strings.Contains(plan.Script, check) {
			t.Fatalf("expected script to contain %q, got:\n%s", check, plan.Script)
		}
	}
}

func TestRenderRoutePolicyKernelScriptWithPortMark(t *testing.T) {
	t.Parallel()

	plan, err := renderRoutePolicyKernelScript([]any{
		map[string]any{
			"route_id":         "route-2",
			"status":           "active",
			"action":           "allow",
			"destination_type": "endpoint",
			"destination":      "198.51.100.10:443",
			"protocol":         "tcp",
			"ports":            "443,8443",
			"source_identity":  map[string]any{"type": "wireguard_ip", "value": "10.66.0.2"},
			"egress":           map[string]any{"status": "candidate", "mode": "remote_egress", "interface": "mgbh123", "table": "21001"},
			"enforcement":      map[string]any{"mode": "l3_l4_candidate"},
		},
	}, "rev-2")
	if err != nil {
		t.Fatalf("renderRoutePolicyKernelScript returned error: %v", err)
	}
	if plan.RuleCount != 1 || plan.MarkedCount != 1 {
		t.Fatalf("unexpected plan counts: %#v", plan)
	}
	checks := []string{
		"nft add rule inet megavpn route_policy_prerouting ip saddr 10.66.0.2/32 ip daddr 198.51.100.10/32 tcp dport { 443, 8443 } meta mark set 0x4d560001",
		"ip rule add fwmark 0x4d560001 table '21001' priority 22000",
	}
	for _, check := range checks {
		if !strings.Contains(plan.Script, check) {
			t.Fatalf("expected script to contain %q, got:\n%s", check, plan.Script)
		}
	}
}

func TestRenderRoutePolicyKernelScriptSkipsMainTable(t *testing.T) {
	t.Parallel()

	plan, err := renderRoutePolicyKernelScript([]any{
		map[string]any{
			"route_id":         "route-3",
			"status":           "active",
			"action":           "allow",
			"destination_type": "cidr",
			"destination":      "203.0.113.0/24",
			"protocol":         "any",
			"source_identity":  map[string]any{"type": "wireguard_ip", "value": "10.66.0.2/32"},
			"egress":           map[string]any{"status": "candidate", "mode": "remote_egress", "interface": "mgbh123", "table": "main"},
			"enforcement":      map[string]any{"mode": "l3_l4_candidate"},
		},
	}, "rev-3")
	if err != nil {
		t.Fatalf("renderRoutePolicyKernelScript returned error: %v", err)
	}
	if plan.RuleCount != 0 || plan.SkippedCount != 1 {
		t.Fatalf("unexpected plan counts: %#v", plan)
	}
}
