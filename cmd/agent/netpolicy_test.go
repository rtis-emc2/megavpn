package main

import (
	"strings"
	"testing"
)

func TestRenderNetworkPolicyScript(t *testing.T) {
	t.Parallel()

	script, counts, hasPolicy, err := renderNetworkPolicyScript(instanceJobPayload{
		InstanceID:  "inst-01",
		Name:        "edge wg",
		Slug:        "edge-wg",
		ServiceCode: "wireguard",
		Spec: map[string]any{
			"sysctl": map[string]any{
				"net.ipv4.ip_forward": "1",
			},
			"firewall_rules": []any{
				map[string]any{
					"direction": "input",
					"action":    "allow",
					"protocol":  "udp",
					"port":      51820,
				},
			},
			"routes": []any{
				map[string]any{
					"destination": "10.66.0.0/24",
					"dev":         "wg0",
					"table":       "main",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("renderNetworkPolicyScript returned error: %v", err)
	}
	if !hasPolicy {
		t.Fatal("expected network policy to be detected")
	}
	if counts["sysctl"] != 1 || counts["firewall"] != 1 || counts["routes"] != 1 {
		t.Fatalf("unexpected counts: %#v", counts)
	}
	checks := []string{
		"sysctl -w 'net.ipv4.ip_forward=1'",
		"nft add table inet megavpn",
		"nft add rule inet megavpn input udp dport 51820 accept comment 'megavpn:edge-wg:input:udp:51820:allow'",
		"ip route replace '10.66.0.0/24' dev 'wg0' table 'main'",
	}
	for _, check := range checks {
		if !strings.Contains(script, check) {
			t.Fatalf("expected script to contain %q, got:\n%s", check, script)
		}
	}
}

func TestRenderNetworkPolicyScriptRejectsInvalidFirewallRule(t *testing.T) {
	t.Parallel()

	_, _, _, err := renderNetworkPolicyScript(instanceJobPayload{
		Spec: map[string]any{
			"firewall_rules": []any{
				map[string]any{
					"protocol": "icmp",
					"port":     1,
				},
			},
		},
	})
	if err == nil {
		t.Fatal("expected invalid firewall protocol error")
	}
}

func TestNetworkPolicyManagedPathAllowlist(t *testing.T) {
	t.Parallel()

	for _, path := range []string{
		"/usr/local/lib/megavpn/netpolicy/edge.sh",
		"/etc/systemd/system/megavpn-netpolicy-edge.service",
	} {
		if !isSafeNetworkPolicyManagedPath(path) {
			t.Fatalf("expected managed path %q to be allowed", path)
		}
	}
	for _, path := range []string{
		"/usr/local/lib/megavpn/netpolicy/../evil.sh",
		"/etc/systemd/system/ssh.service",
		"/tmp/megavpn-netpolicy-edge.service",
	} {
		if isSafeNetworkPolicyManagedPath(path) {
			t.Fatalf("expected managed path %q to be rejected", path)
		}
	}
}

func TestNetworkPolicyUnitAllowlist(t *testing.T) {
	t.Parallel()

	if !isSafeNetworkPolicyUnit("megavpn-netpolicy-edge.service") {
		t.Fatal("expected managed network policy unit to be allowed")
	}
	for _, unit := range []string{"ssh.service", "megavpn-netpolicy-../ssh.service", "/etc/systemd/system/megavpn-netpolicy-edge.service"} {
		if isSafeNetworkPolicyUnit(unit) {
			t.Fatalf("expected unit %q to be rejected", unit)
		}
	}
}
