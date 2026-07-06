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
	}, nil, "rev-1")
	if err != nil {
		t.Fatalf("renderRoutePolicyKernelScript returned error: %v", err)
	}
	if plan.RuleCount != 1 || plan.MarkedCount != 1 {
		t.Fatalf("unexpected plan counts: %#v", plan)
	}
	checks := []string{
		"route_policy_replace_route '203.0.113.0/24' '21001' 70 'mgbh123' '' ''",
		"nft add rule inet megavpn route_policy_prerouting ip saddr 10.66.0.2/32 ip daddr 203.0.113.0/24 meta mark set 0x4d560001",
		`comment '"megavpn:route-policy:route-1"'`,
		"ip rule add fwmark 0x4d560001 table '21001' priority 22000",
	}
	for _, check := range checks {
		if !strings.Contains(plan.Script, check) {
			t.Fatalf("expected script to contain %q, got:\n%s", check, plan.Script)
		}
	}
}

func TestRenderRoutePolicyKernelScriptWithManagedBackhaulCandidates(t *testing.T) {
	t.Parallel()

	plan, err := renderRoutePolicyKernelScript([]any{
		map[string]any{
			"route_id":         "route-candidates",
			"status":           "active",
			"action":           "allow",
			"destination_type": "cidr",
			"destination":      "203.0.113.0/24",
			"protocol":         "any",
			"source_identity":  map[string]any{"type": "wireguard_ip", "value": "10.66.0.2/32"},
			"egress": map[string]any{
				"status":    "candidate",
				"mode":      "remote_egress",
				"interface": "mgbh-primary",
				"table":     "21001",
				"managed_backhauls": []any{
					map[string]any{
						"interface":      "mgbh-primary",
						"egress_address": "10.240.1.2/30",
						"route_metric":   10,
					},
					map[string]any{
						"interface":      "mgbh-backup",
						"egress_address": "10.240.2.2/30",
						"route_metric":   100,
					},
				},
			},
			"enforcement": map[string]any{"mode": "l3_l4_candidate"},
		},
	}, nil, "rev-candidates")
	if err != nil {
		t.Fatalf("renderRoutePolicyKernelScript returned error: %v", err)
	}
	if plan.RuleCount != 1 || plan.MarkedCount != 1 {
		t.Fatalf("unexpected plan counts: %#v", plan)
	}
	checks := []string{
		"route_policy_replace_route '203.0.113.0/24' '21001' 10 'mgbh-primary' '' '10.240.1.2'",
		"route_policy_replace_route '203.0.113.0/24' '21001' 100 'mgbh-backup' '' '10.240.2.2'",
		"nft add rule inet megavpn route_policy_prerouting ip saddr 10.66.0.2/32 ip daddr 203.0.113.0/24 meta mark set 0x4d560001",
		`comment '"megavpn:route-policy:route-candidates"'`,
		"ip rule add fwmark 0x4d560001 table '21001' priority 22000",
	}
	for _, check := range checks {
		if !strings.Contains(plan.Script, check) {
			t.Fatalf("expected script to contain %q, got:\n%s", check, plan.Script)
		}
	}
}

func TestRenderRoutePolicyKernelScriptWithXraySystemRoute(t *testing.T) {
	t.Parallel()

	plan, err := renderRoutePolicyKernelScript(nil, []any{
		map[string]any{
			"system_route_id": "xray-route-1",
			"kind":            "xray_vless_remote_egress",
			"status":          "active",
			"source":          "10.240.35.245/32",
			"destination":     "0.0.0.0/0",
			"interface":       "mgbh1234567890",
			"table":           "21001",
			"managed_backhaul": map[string]any{
				"route_metric":   70,
				"egress_address": "10.240.35.246/30",
			},
		},
	}, "rev-system")
	if err != nil {
		t.Fatalf("renderRoutePolicyKernelScript returned error: %v", err)
	}
	if plan.SystemRuleCount != 1 || plan.RuleCount != 0 || plan.MarkedCount != 1 {
		t.Fatalf("unexpected plan counts: %#v", plan)
	}
	checks := []string{
		"route_policy_replace_route '0.0.0.0/0' '21001' 70 'mgbh1234567890' '' '10.240.35.246'",
		"if route_policy_replace_route '0.0.0.0/0' '21001' 70 'mgbh1234567890' '' '10.240.35.246'; then route_policy_rule_ready=1; fi",
		"nft add rule inet megavpn route_policy_output ip saddr 10.240.35.245/32 ip daddr 0.0.0.0/0 meta mark set 0x4d550001",
		`comment '"megavpn:route-policy:system:xray-route-1"'`,
		"ip rule add fwmark 0x4d550001 table '21001' priority 21900",
		"route policy rule xray-route-1 has no ready managed backhaul candidate",
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
	}, nil, "rev-2")
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

func TestRenderRoutePolicyKernelScriptWithProtocolOnlyMark(t *testing.T) {
	t.Parallel()

	plan, err := renderRoutePolicyKernelScript([]any{
		map[string]any{
			"route_id":         "route-tcp",
			"status":           "active",
			"action":           "allow",
			"destination_type": "cidr",
			"destination":      "198.51.100.0/24",
			"protocol":         "tcp",
			"source_identity":  map[string]any{"type": "openvpn_ip", "value": "10.8.0.10/32"},
			"egress":           map[string]any{"status": "candidate", "mode": "remote_egress", "interface": "mgbh123", "table": "21001"},
			"enforcement":      map[string]any{"mode": "l3_l4_candidate"},
		},
		map[string]any{
			"route_id":         "route-icmp",
			"status":           "active",
			"action":           "allow",
			"destination_type": "cidr",
			"destination":      "203.0.113.0/24",
			"protocol":         "icmp",
			"source_identity":  map[string]any{"type": "openvpn_ip", "value": "10.8.0.10/32"},
			"egress":           map[string]any{"status": "candidate", "mode": "remote_egress", "interface": "mgbh123", "table": "21001"},
			"enforcement":      map[string]any{"mode": "l3_l4_candidate"},
		},
	}, nil, "rev-proto")
	if err != nil {
		t.Fatalf("renderRoutePolicyKernelScript returned error: %v", err)
	}
	if plan.RuleCount != 2 || plan.MarkedCount != 2 {
		t.Fatalf("unexpected plan counts: %#v", plan)
	}
	checks := []string{
		"nft add rule inet megavpn route_policy_prerouting ip saddr 10.8.0.10/32 ip daddr 198.51.100.0/24 ip protocol tcp meta mark set 0x4d560001",
		"nft add rule inet megavpn route_policy_prerouting ip saddr 10.8.0.10/32 ip daddr 203.0.113.0/24 ip protocol icmp meta mark set 0x4d560002",
		"ip rule add fwmark 0x4d560001 table '21001' priority 22000",
		"ip rule add fwmark 0x4d560002 table '21001' priority 22001",
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
	}, nil, "rev-3")
	if err != nil {
		t.Fatalf("renderRoutePolicyKernelScript returned error: %v", err)
	}
	if plan.RuleCount != 0 || plan.SkippedCount != 1 {
		t.Fatalf("unexpected plan counts: %#v", plan)
	}
}

func TestRenderRoutePolicyKernelScriptWithoutRulesStillCleansKernelState(t *testing.T) {
	t.Parallel()

	plan, err := renderRoutePolicyKernelScript(nil, nil, "rev-empty")
	if err != nil {
		t.Fatalf("renderRoutePolicyKernelScript returned error: %v", err)
	}
	if plan.RuleCount != 0 || plan.SystemRuleCount != 0 || plan.MarkedCount != 0 {
		t.Fatalf("unexpected plan counts: %#v", plan)
	}
	checks := []string{
		"while ip rule delete priority \"$p\" >/dev/null 2>&1; do :; done",
		"nft list chain inet megavpn route_policy_prerouting >/dev/null 2>&1 && nft flush chain inet megavpn route_policy_prerouting || true",
		"nft list chain inet megavpn route_policy_output >/dev/null 2>&1 && nft flush chain inet megavpn route_policy_output || true",
	}
	for _, check := range checks {
		if !strings.Contains(plan.Script, check) {
			t.Fatalf("expected cleanup script to contain %q, got:\n%s", check, plan.Script)
		}
	}
	if strings.Contains(plan.Script, "sysctl -w net.ipv4.ip_forward=1") {
		t.Fatalf("empty cleanup should not enable forwarding, got:\n%s", plan.Script)
	}
}

func TestRenderRoutePolicyKernelScriptCleansPreviousSnapshotDestinations(t *testing.T) {
	t.Parallel()

	previous := routePolicyCleanupSnapshot{
		Routes: []any{
			map[string]any{
				"route_id":         "old-route",
				"status":           "active",
				"action":           "allow",
				"destination_type": "cidr",
				"destination":      "203.0.113.0/24",
				"source_identity":  map[string]any{"type": "wireguard_ip", "value": "10.66.0.2/32"},
				"egress":           map[string]any{"status": "candidate", "table": "21001"},
				"enforcement":      map[string]any{"mode": "l3_l4_candidate"},
			},
		},
		SystemRoutes: []any{
			map[string]any{
				"system_route_id": "old-system-route",
				"status":          "active",
				"destination":     "0.0.0.0/0",
				"table":           "21002",
			},
		},
	}
	plan, err := renderRoutePolicyKernelScript(nil, nil, "rev-clean-old", previous)
	if err != nil {
		t.Fatalf("renderRoutePolicyKernelScript returned error: %v", err)
	}
	checks := []string{
		"route_policy_flush_destination '203.0.113.0/24' '21001'",
		"route_policy_flush_destination '0.0.0.0/0' '21002'",
	}
	for _, check := range checks {
		if !strings.Contains(plan.Script, check) {
			t.Fatalf("expected stale cleanup %q, got:\n%s", check, plan.Script)
		}
	}
}

func TestRenderRoutePolicyCleanupScript(t *testing.T) {
	t.Parallel()

	plan := renderRoutePolicyCleanupScript(routePolicyCleanupSnapshot{
		Routes: []any{
			map[string]any{
				"status":           "active",
				"action":           "allow",
				"destination_type": "cidr",
				"destination":      "198.51.100.0/24",
				"egress":           map[string]any{"status": "candidate", "table": "22001"},
				"enforcement":      map[string]any{"mode": "l3_l4_candidate"},
			},
		},
	}, "cleanup-rev")
	checks := []string{
		"while ip rule delete priority \"$p\" >/dev/null 2>&1; do :; done",
		"while ip route delete '198.51.100.0/24' table '22001' >/dev/null 2>&1; do :; done",
		"nft list chain inet megavpn route_policy_prerouting >/dev/null 2>&1 && nft flush chain inet megavpn route_policy_prerouting || true",
		"nft list chain inet megavpn route_policy_output >/dev/null 2>&1 && nft flush chain inet megavpn route_policy_output || true",
	}
	for _, check := range checks {
		if !strings.Contains(plan.Script, check) {
			t.Fatalf("expected cleanup script to contain %q, got:\n%s", check, plan.Script)
		}
	}
	if plan.MarkedCount != 1 {
		t.Fatalf("cleanup target count = %d, want 1", plan.MarkedCount)
	}
}
