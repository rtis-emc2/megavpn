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
		"delete table inet megavpn_firewall",
		"add table inet megavpn_firewall",
		"add rule inet megavpn_firewall firewall_input",
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
	if strings.Contains(plan.Script, "delete table inet megavpn\n") {
		t.Fatalf("firewall plan must not delete shared inet megavpn table, got:\n%s", plan.Script)
	}
}

func TestRenderNodeFirewallPlanRendersIPv6ICMP(t *testing.T) {
	t.Parallel()

	plan, err := renderNodeFirewallPlan(map[string]any{
		"node_id": "node-1",
		"rules": []any{
			map[string]any{
				"id":       "allow-icmpv6",
				"priority": 105,
				"chain":    "input",
				"action":   "accept",
				"protocol": "icmpv6",
				"enabled":  true,
				"status":   "active",
			},
		},
	})
	if err != nil {
		t.Fatalf("renderNodeFirewallPlan returned error: %v", err)
	}
	if plan.Count != 1 {
		t.Fatalf("rule count = %d, want 1", plan.Count)
	}
	if !strings.Contains(plan.Script, `ip6 nexthdr icmpv6 accept comment "megavpn:firewall:allow-icmpv6"`) {
		t.Fatalf("expected ICMPv6 rule, got:\n%s", plan.Script)
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
		"add set inet megavpn_firewall fwlist_trusted_ops_v4 { type ipv4_addr; flags interval; }",
		"add element inet megavpn_firewall fwlist_trusted_ops_v4 { 10.10.10.10, 10.20.0.0/16, 10.30.0.10-10.30.0.20 }",
		"ip saddr @fwlist_trusted_ops_v4",
		"tcp dport { 22, 443 }",
	} {
		if !strings.Contains(plan.Script, check) {
			t.Fatalf("expected script to contain %q, got:\n%s", check, plan.Script)
		}
	}
}

func TestRenderNodeFirewallPlanReportsIgnoredDNSAddressEntries(t *testing.T) {
	t.Parallel()

	plan, err := renderNodeFirewallPlan(map[string]any{
		"node_id": "node-1",
		"address_lists": []any{
			map[string]any{
				"id":     "list-1",
				"key":    "mixed_sources",
				"status": "active",
				"entries": []any{
					map[string]any{"value": "10.20.0.0/16", "value_type": "cidr", "status": "active"},
					map[string]any{"value": "operator.example.test", "value_type": "dns", "status": "active"},
				},
			},
		},
		"rules": []any{
			map[string]any{
				"id":           "allow-mixed",
				"priority":     100,
				"chain":        "input",
				"action":       "accept",
				"protocol":     "tcp",
				"src_list_key": "mixed_sources",
				"dst_ports":    "443",
				"enabled":      true,
				"status":       "active",
			},
		},
	})
	if err != nil {
		t.Fatalf("renderNodeFirewallPlan returned error: %v", err)
	}
	if len(plan.Warnings) == 0 {
		t.Fatal("expected DNS ignored warning")
	}
	if len(plan.AddressListCounts) != 1 {
		t.Fatalf("address list counts = %#v, want one item", plan.AddressListCounts)
	}
	stats := plan.AddressListCounts[0]
	if stats["rendered_entries"] != 1 || stats["ignored_dns_entries"] != 1 {
		t.Fatalf("address list stats = %#v, want 1 rendered and 1 ignored DNS", stats)
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
		"add set inet megavpn_firewall fwlist_dual_stack_v4 { type ipv4_addr; flags interval; }",
		"add set inet megavpn_firewall fwlist_dual_stack_v6 { type ipv6_addr; flags interval; }",
		"ip daddr @fwlist_dual_stack_v4",
		"ip6 daddr @fwlist_dual_stack_v6",
	} {
		if !strings.Contains(plan.Script, check) {
			t.Fatalf("expected script to contain %q, got:\n%s", check, plan.Script)
		}
	}
}

func TestRenderNodeFirewallPlanEnforcesDefaultPolicies(t *testing.T) {
	t.Parallel()

	plan, err := renderNodeFirewallPlan(map[string]any{
		"node_id":                 "node-1",
		"enforce_default_policy":  true,
		"default_input_policy":    "drop",
		"default_forward_policy":  "reject",
		"default_output_policy":   "drop",
		"agent_control_plane_url": "https://198.51.100.10:8443",
		"rules":                   []any{},
		"address_lists":           []any{},
		"firewall_policy_version": 1,
	})
	if err != nil {
		t.Fatalf("renderNodeFirewallPlan returned error: %v", err)
	}
	if plan.DefaultPolicyEnforcement != "enforced" {
		t.Fatalf("default policy enforcement = %q, want enforced", plan.DefaultPolicyEnforcement)
	}
	for _, check := range []string{
		"add chain inet megavpn_firewall firewall_input { type filter hook input priority filter; policy drop; }",
		"add chain inet megavpn_firewall firewall_forward { type filter hook forward priority filter; policy drop; }",
		"add chain inet megavpn_firewall firewall_output { type filter hook output priority filter; policy drop; }",
		`ct state { established, related } accept comment "megavpn:firewall:system-established-input"`,
		`iifname "lo" accept comment "megavpn:firewall:system-loopback-input"`,
		`ip daddr 198.51.100.10/32 tcp dport 8443 accept comment "megavpn:firewall:system-control-plane-output"`,
		`reject comment "megavpn:firewall:default-reject-forward"`,
	} {
		if !strings.Contains(plan.Script, check) {
			t.Fatalf("expected script to contain %q, got:\n%s", check, plan.Script)
		}
	}
	if plan.SystemRuleCount != 7 {
		t.Fatalf("system rule count = %d, want 7", plan.SystemRuleCount)
	}
}

func TestRenderNodeFirewallPlanRendersManagedSSHBootstrapAllow(t *testing.T) {
	t.Parallel()

	plan, err := renderNodeFirewallPlan(map[string]any{
		"node_id":                "node-1",
		"enforce_default_policy": true,
		"default_input_policy":   "drop",
		"ssh_bootstrap_ports":    []any{22, "2222"},
		"address_lists": []any{
			map[string]any{
				"id":     "trusted-control-plane",
				"key":    "trusted_control_plane",
				"status": "active",
				"entries": []any{
					map[string]any{"value": "203.0.113.10", "value_type": "address", "status": "active"},
				},
			},
		},
		"rules": []any{},
	})
	if err != nil {
		t.Fatalf("renderNodeFirewallPlan returned error: %v", err)
	}
	if !plan.SSHBootstrapPreserved {
		t.Fatal("expected ssh_bootstrap_preserved")
	}
	for _, check := range []string{
		"add set inet megavpn_firewall fwlist_trusted_control_plane_v4",
		`ip saddr @fwlist_trusted_control_plane_v4 tcp dport { 22, 2222 } accept comment "megavpn:firewall:system-ssh-bootstrap-trusted_control_plane-ip"`,
	} {
		if !strings.Contains(plan.Script, check) {
			t.Fatalf("expected script to contain %q, got:\n%s", check, plan.Script)
		}
	}
}

func TestRenderNodeFirewallPlanSkipsBroadManagedSSHBootstrapSource(t *testing.T) {
	t.Parallel()

	plan, err := renderNodeFirewallPlan(map[string]any{
		"node_id":                "node-1",
		"enforce_default_policy": true,
		"default_input_policy":   "drop",
		"ssh_bootstrap_ports":    []any{22},
		"address_lists": []any{
			map[string]any{
				"id":     "trusted-control-plane",
				"key":    "trusted_control_plane",
				"status": "active",
				"entries": []any{
					map[string]any{"value": "0.0.0.0/0", "value_type": "cidr", "status": "active"},
				},
			},
		},
		"rules": []any{},
	})
	if err != nil {
		t.Fatalf("renderNodeFirewallPlan returned error: %v", err)
	}
	if plan.SSHBootstrapPreserved {
		t.Fatal("expected broad management source not to preserve SSH bootstrap")
	}
	if strings.Contains(plan.Script, "system-ssh-bootstrap") {
		t.Fatalf("expected no generated SSH bootstrap rule for broad source, got:\n%s", plan.Script)
	}
}

func TestRenderNodeFirewallPlanRejectsStrictOutputWithoutPinnedControlPlane(t *testing.T) {
	t.Parallel()

	_, err := renderNodeFirewallPlan(map[string]any{
		"node_id":                 "node-1",
		"enforce_default_policy":  true,
		"default_output_policy":   "drop",
		"agent_control_plane_url": "https://control.example.test",
		"rules":                   []any{},
		"address_lists":           []any{},
	})
	if err == nil {
		t.Fatal("expected strict output policy to require an IP-pinned control-plane URL")
	}
}

func TestRenderNodeFirewallPlanReportsForwardPreservation(t *testing.T) {
	t.Parallel()

	blocked, err := renderNodeFirewallPlan(map[string]any{
		"node_id":                            "node-1",
		"enforce_default_policy":             true,
		"default_forward_policy":             "drop",
		"node_requires_forward_preservation": true,
		"rules":                              []any{},
		"address_lists":                      []any{},
	})
	if err != nil {
		t.Fatalf("renderNodeFirewallPlan blocked case returned error: %v", err)
	}
	if blocked.ForwardEgressPreserved {
		t.Fatal("expected forward egress not preserved without source-scoped allow")
	}

	preserved, err := renderNodeFirewallPlan(map[string]any{
		"node_id":                            "node-1",
		"enforce_default_policy":             true,
		"default_forward_policy":             "drop",
		"node_requires_forward_preservation": true,
		"address_lists": []any{
			map[string]any{
				"id":     "vpn-client-sources",
				"key":    "vpn_client_sources",
				"status": "active",
				"entries": []any{
					map[string]any{"value": "10.0.0.0/8", "value_type": "cidr", "status": "active"},
				},
			},
		},
		"rules": []any{
			map[string]any{
				"id":           "allow-vpn-forward",
				"chain":        "forward",
				"action":       "accept",
				"protocol":     "any",
				"src_list_key": "vpn_client_sources",
				"state_match":  []any{"new", "established"},
				"enabled":      true,
				"status":       "active",
			},
		},
	})
	if err != nil {
		t.Fatalf("renderNodeFirewallPlan preserved case returned error: %v", err)
	}
	if !preserved.ForwardEgressPreserved {
		t.Fatal("expected forward egress preserved")
	}
}

func TestRenderNodeFirewallPlanAllowsStrictOutputWithExplicitControlPlaneRule(t *testing.T) {
	t.Parallel()

	plan, err := renderNodeFirewallPlan(map[string]any{
		"node_id":                 "node-1",
		"enforce_default_policy":  true,
		"default_output_policy":   "drop",
		"agent_control_plane_url": "https://control.example.test:8443",
		"rules": []any{
			map[string]any{
				"id":          "allow-control-plane-egress",
				"priority":    100,
				"chain":       "output",
				"action":      "accept",
				"protocol":    "tcp",
				"dst_cidr":    "198.51.100.10/32",
				"dst_ports":   "8443",
				"state_match": []any{"new", "established"},
				"enabled":     true,
				"status":      "active",
			},
		},
		"address_lists": []any{},
	})
	if err != nil {
		t.Fatalf("renderNodeFirewallPlan returned error: %v", err)
	}
	if !strings.Contains(plan.Script, `accept comment "megavpn:firewall:allow-control-plane-egress"`) {
		t.Fatalf("expected explicit output allow rule, got:\n%s", plan.Script)
	}
	if strings.Contains(plan.Script, "system-control-plane-output") {
		t.Fatalf("expected no generated control-plane output rule for DNS host with explicit allow, got:\n%s", plan.Script)
	}
	if len(plan.Warnings) == 0 {
		t.Fatal("expected DNS control-plane warning")
	}
}

func TestRenderNodeFirewallPlanObserveModeKeepsBasePoliciesAccept(t *testing.T) {
	t.Parallel()

	plan, err := renderNodeFirewallPlan(map[string]any{
		"node_id":                "node-1",
		"enforce_default_policy": false,
		"default_input_policy":   "drop",
		"default_forward_policy": "reject",
		"default_output_policy":  "drop",
		"rules":                  []any{},
		"address_lists":          []any{},
	})
	if err != nil {
		t.Fatalf("renderNodeFirewallPlan returned error: %v", err)
	}
	if plan.DefaultPolicyEnforcement != "observe_only" {
		t.Fatalf("default policy enforcement = %q, want observe_only", plan.DefaultPolicyEnforcement)
	}
	for _, check := range []string{
		"add chain inet megavpn_firewall firewall_input { type filter hook input priority filter; policy accept; }",
		"add chain inet megavpn_firewall firewall_forward { type filter hook forward priority filter; policy accept; }",
		"add chain inet megavpn_firewall firewall_output { type filter hook output priority filter; policy accept; }",
	} {
		if !strings.Contains(plan.Script, check) {
			t.Fatalf("expected script to contain %q, got:\n%s", check, plan.Script)
		}
	}
	if len(plan.Warnings) == 0 {
		t.Fatal("expected observe-mode default policy warning")
	}
}
