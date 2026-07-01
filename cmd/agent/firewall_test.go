package main

import (
	"strings"
	"testing"
)

func TestRenderNodeFirewallPlan(t *testing.T) {
	t.Parallel()

	plan, err := renderNodeFirewallPlan(map[string]any{
		"node_id": "node-1",
		"rules": []any{
			map[string]any{
				"id":          "rule-1",
				"priority":    100,
				"chain":       "input",
				"action":      "accept",
				"protocol":    "tcp",
				"src_cidr":    "10.0.0.0/8",
				"dst_ports":   "443,8443",
				"state_match": []any{"established", "related"},
				"comment":     "control channel",
				"enabled":     true,
				"status":      "active",
			},
		},
	})
	if err != nil {
		t.Fatalf("renderNodeFirewallPlan returned error: %v", err)
	}
	if plan.Count != 1 {
		t.Fatalf("rule count = %d, want 1", plan.Count)
	}
	checks := []string{
		"flush chain inet megavpn firewall_input",
		"add rule inet megavpn firewall_input",
		"ct state { established, related }",
		"ip saddr 10.0.0.0/8",
		"tcp dport { 443, 8443 }",
		`accept comment "megavpn:firewall:rule-1"`,
	}
	for _, check := range checks {
		if !strings.Contains(plan.Script, check) {
			t.Fatalf("expected script to contain %q, got:\n%s", check, plan.Script)
		}
	}
}

func TestRenderNodeFirewallPlanRejectsInvalidPort(t *testing.T) {
	t.Parallel()

	_, err := renderNodeFirewallPlan(map[string]any{
		"node_id": "node-1",
		"rules": []any{
			map[string]any{
				"id":        "rule-1",
				"chain":     "input",
				"action":    "accept",
				"protocol":  "tcp",
				"dst_ports": "70000",
				"enabled":   true,
				"status":    "active",
			},
		},
	})
	if err == nil {
		t.Fatal("expected invalid port error")
	}
}

func TestRenderNodeFirewallPlanRendersAddressListSets(t *testing.T) {
	t.Parallel()

	plan, err := renderNodeFirewallPlan(map[string]any{
		"node_id": "node-1",
		"address_lists": []any{
			map[string]any{
				"id":     "list-1",
				"key":    "trusted_ops",
				"status": "active",
				"entries": []any{
					map[string]any{"value": "10.10.10.10", "value_type": "address", "status": "active"},
					map[string]any{"value": "10.20.0.0/16", "value_type": "cidr", "status": "active"},
					map[string]any{"value": "10.30.0.10-10.30.0.20", "value_type": "range", "status": "active"},
				},
			},
		},
		"rules": []any{
			map[string]any{
				"id":           "allow-ops",
				"priority":     100,
				"chain":        "input",
				"action":       "accept",
				"protocol":     "tcp",
				"src_list_key": "trusted_ops",
				"dst_ports":    "22,443",
				"enabled":      true,
				"status":       "active",
			},
		},
	})
	if err != nil {
		t.Fatalf("renderNodeFirewallPlan returned error: %v", err)
	}
	if plan.Count != 1 {
		t.Fatalf("rule count = %d, want 1", plan.Count)
	}
	for _, check := range []string{
		"add set inet megavpn fwlist_trusted_ops_v4 { type ipv4_addr; flags interval; }",
		"add element inet megavpn fwlist_trusted_ops_v4 { 10.10.10.10, 10.20.0.0/16, 10.30.0.10-10.30.0.20 }",
		"ip saddr @fwlist_trusted_ops_v4",
		"tcp dport { 22, 443 }",
	} {
		if !strings.Contains(plan.Script, check) {
			t.Fatalf("expected script to contain %q, got:\n%s", check, plan.Script)
		}
	}
}

func TestRenderNodeFirewallPlanRejectsEmptyAddressList(t *testing.T) {
	t.Parallel()

	_, err := renderNodeFirewallPlan(map[string]any{
		"node_id": "node-1",
		"address_lists": []any{
			map[string]any{
				"id":      "list-1",
				"key":     "dns_only",
				"status":  "active",
				"entries": []any{map[string]any{"value": "example.test", "value_type": "dns", "status": "active"}},
			},
		},
		"rules": []any{
			map[string]any{
				"id":           "allow-dns-list",
				"chain":        "input",
				"action":       "accept",
				"protocol":     "tcp",
				"src_list_key": "dns_only",
				"dst_ports":    "443",
				"enabled":      true,
				"status":       "active",
			},
		},
	})
	if err == nil {
		t.Fatal("expected empty address-list error")
	}
}

func TestRenderNodeFirewallPlanRendersDualStackAddressListRules(t *testing.T) {
	t.Parallel()

	plan, err := renderNodeFirewallPlan(map[string]any{
		"node_id": "node-1",
		"address_lists": []any{
			map[string]any{
				"id":     "list-1",
				"key":    "dual_stack",
				"status": "active",
				"entries": []any{
					map[string]any{"value": "192.0.2.10", "value_type": "address", "status": "active"},
					map[string]any{"value": "2001:db8::/64", "value_type": "cidr", "status": "active"},
				},
			},
		},
		"rules": []any{
			map[string]any{
				"id":           "allow-dual",
				"chain":        "forward",
				"action":       "accept",
				"protocol":     "tcp",
				"dst_list_key": "dual_stack",
				"dst_ports":    "443",
				"enabled":      true,
				"status":       "active",
			},
		},
	})
	if err != nil {
		t.Fatalf("renderNodeFirewallPlan returned error: %v", err)
	}
	if plan.Count != 2 {
		t.Fatalf("rule count = %d, want 2", plan.Count)
	}
	for _, check := range []string{
		"add set inet megavpn fwlist_dual_stack_v4 { type ipv4_addr; flags interval; }",
		"add set inet megavpn fwlist_dual_stack_v6 { type ipv6_addr; flags interval; }",
		"ip daddr @fwlist_dual_stack_v4",
		"ip6 daddr @fwlist_dual_stack_v6",
	} {
		if !strings.Contains(plan.Script, check) {
			t.Fatalf("expected script to contain %q, got:\n%s", check, plan.Script)
		}
	}
}

func TestRenderNodeFirewallPlanWarnsForDefaultPolicyEnforcement(t *testing.T) {
	t.Parallel()

	plan, err := renderNodeFirewallPlan(map[string]any{
		"node_id":                 "node-1",
		"enforce_default_policy":  true,
		"default_input_policy":    "drop",
		"default_forward_policy":  "drop",
		"default_output_policy":   "accept",
		"rules":                   []any{},
		"address_lists":           []any{},
		"firewall_policy_version": 1,
	})
	if err != nil {
		t.Fatalf("renderNodeFirewallPlan returned error: %v", err)
	}
	if len(plan.Warnings) == 0 {
		t.Fatal("expected default policy warning")
	}
}
