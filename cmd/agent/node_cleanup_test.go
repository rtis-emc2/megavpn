package main

import (
	"strings"
	"testing"
)

func TestEmergencyCleanupManagedUnitAllowlist(t *testing.T) {
	fullNodeAllowed := []string{
		"megavpn-route-policy.service",
		"megavpn-backhaul-link.service",
		"megavpn-netpolicy-edge.service",
		"megavpn-xray-edge.service",
		"megavpn-shadowsocks-edge.service",
		"megavpn-http-proxy-edge.service",
		"megavpn-mtproto-edge.service",
	}
	for _, unit := range fullNodeAllowed {
		if !isEmergencyManagedUnit(unit) {
			t.Fatalf("expected %s to be allowed", unit)
		}
	}
	blocked := []string{
		"ssh.service",
		"nginx.service",
		"openvpn-server@server.service",
		"wg-quick@wg0.service",
		"megavpn-../ssh.service",
		"/etc/systemd/system/megavpn-xray-edge.service",
	}
	for _, unit := range blocked {
		if isEmergencyManagedUnit(unit) {
			t.Fatalf("expected %s to be blocked", unit)
		}
	}
}

func TestEmergencyCleanupVersionedPayloadValidation(t *testing.T) {
	payload := map[string]any{
		"node_id":              "node-1",
		"node_name":            "edge-01",
		"confirmation":         "edge-01",
		"reason":               "maintenance window",
		"cleanup_scope":        "full_node",
		"include_agent":        true,
		"cleanup_plan_version": 1,
		"instances": []any{map[string]any{
			"instance_id":  "inst-1",
			"action":       "delete",
			"service_code": "xray-core",
		}},
	}
	scope, err := validateEmergencyCleanupJobPayload(payload, true)
	if err != nil {
		t.Fatalf("validate payload: %v", err)
	}
	if scope != "full_node" {
		t.Fatalf("scope = %q, want full_node", scope)
	}
}

func TestEmergencyCleanupVersionedPayloadRejectsInvalidContract(t *testing.T) {
	cases := []struct {
		name string
		edit func(map[string]any)
		want string
	}{
		{name: "missing reason", edit: func(p map[string]any) { delete(p, "reason") }, want: "reason"},
		{name: "future version", edit: func(p map[string]any) { p["cleanup_plan_version"] = 2 }, want: "unsupported"},
		{name: "legacy scope alias", edit: func(p map[string]any) { p["cleanup_scope"] = "wipe" }, want: "cleanup_scope"},
		{name: "agent removal services only", edit: func(p map[string]any) { p["cleanup_scope"] = "services_only" }, want: "include_agent"},
		{name: "missing instances", edit: func(p map[string]any) { delete(p, "instances") }, want: "instances"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			payload := map[string]any{
				"node_id":              "node-1",
				"node_name":            "edge-01",
				"confirmation":         "edge-01",
				"reason":               "maintenance window",
				"cleanup_scope":        "full_node",
				"include_agent":        true,
				"cleanup_plan_version": 1,
				"instances":            []any{},
			}
			tc.edit(payload)
			_, err := validateEmergencyCleanupJobPayload(payload, true)
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want marker %q", err.Error(), tc.want)
			}
		})
	}
}

func TestEmergencyCleanupManagedServiceConfigDetection(t *testing.T) {
	openVPN := emergencyManagedRuntimeConfigHeader + "\nport 1194\n"
	if !isEmergencyManagedOpenVPNConfigContent(openVPN) {
		t.Fatal("expected OpenVPN config with managed header to be detected")
	}
	legacyOpenVPN := "ca /etc/openvpn/server/megavpn-edge/ca.crt\ncert /etc/openvpn/server/megavpn-edge/server.crt\n"
	if !isEmergencyManagedOpenVPNConfigContent(legacyOpenVPN) {
		t.Fatal("expected legacy OpenVPN config with megavpn runtime dir to be detected")
	}
	if isEmergencyManagedOpenVPNConfigContent("ca /etc/openvpn/server/server/ca.crt\n") {
		t.Fatal("did not expect unrelated OpenVPN config to be detected")
	}

	wireGuard := emergencyManagedRuntimeConfigHeader + "\n[Interface]\n"
	if !isEmergencyManagedWireGuardConfigContent(wireGuard) {
		t.Fatal("expected WireGuard config with managed header to be detected")
	}
	legacyWireGuard := "[Interface]\n\n# megavpn-service-access-id = access-1\n[Peer]\n"
	if !isEmergencyManagedWireGuardConfigContent(legacyWireGuard) {
		t.Fatal("expected legacy WireGuard config with managed peer marker to be detected")
	}
	if isEmergencyManagedWireGuardConfigContent("[Interface]\nAddress = 10.0.0.1/24\n") {
		t.Fatal("did not expect unrelated WireGuard config to be detected")
	}
}

func TestEmergencyManagedServiceUnitForConfigPath(t *testing.T) {
	cases := map[string]string{
		"/etc/openvpn/server/edge.conf": "openvpn-server@edge.service",
		"/etc/wireguard/wg-edge.conf":   "wg-quick@wg-edge.service",
	}
	for path, want := range cases {
		if got := emergencyManagedServiceUnitForConfigPath(path); got != want {
			t.Fatalf("unit for %s = %q, want %q", path, got, want)
		}
	}
	for _, path := range []string{
		"/etc/openvpn/client/edge.conf",
		"/tmp/wg-edge.conf",
		"/etc/wireguard/../ssh.conf",
	} {
		if got := emergencyManagedServiceUnitForConfigPath(path); got != "" {
			t.Fatalf("expected %s to be rejected, got unit %q", path, got)
		}
	}
}

func TestEmergencyCleanupServicesOnlyPreservesBackhaulAndRoutePolicy(t *testing.T) {
	allowed := []string{
		"megavpn-netpolicy-edge.service",
		"megavpn-xray-edge.service",
		"megavpn-shadowsocks-edge.service",
		"megavpn-http-proxy-edge.service",
		"megavpn-mtproto-edge.service",
	}
	for _, unit := range allowed {
		if !isEmergencyManagedUnitForScope(unit, "services_only") {
			t.Fatalf("expected %s to be allowed for services_only", unit)
		}
	}
	blocked := []string{
		"megavpn-route-policy.service",
		"megavpn-backhaul-link.service",
	}
	for _, unit := range blocked {
		if isEmergencyManagedUnitForScope(unit, "services_only") {
			t.Fatalf("expected %s to be preserved for services_only", unit)
		}
	}
}

func TestEmergencyCleanupManagedFileAllowlist(t *testing.T) {
	allowed := []string{
		"/etc/nginx/conf.d/megavpn-edge.conf",
		"/etc/systemd/system/megavpn-xray-edge.service",
	}
	for _, path := range allowed {
		if !isEmergencyManagedFilePath(path) {
			t.Fatalf("expected %s to be allowed", path)
		}
	}
	blocked := []string{
		"/etc/nginx/conf.d/default.conf",
		"/etc/wireguard/wg0.conf",
		"/etc/systemd/system/ssh.service",
		"/etc/systemd/system/megavpn-../ssh.service",
	}
	for _, path := range blocked {
		if isEmergencyManagedFilePath(path) {
			t.Fatalf("expected %s to be blocked", path)
		}
	}
}

func TestEmergencyCleanupServicesOnlyFileAllowlistPreservesBackhaulAndRoutePolicy(t *testing.T) {
	allowed := []string{
		"/etc/nginx/conf.d/megavpn-edge.conf",
		"/etc/systemd/system/megavpn-xray-edge.service",
	}
	for _, path := range allowed {
		if !isEmergencyManagedFilePathForScope(path, "services_only") {
			t.Fatalf("expected %s to be allowed for services_only", path)
		}
	}
	blocked := []string{
		"/etc/systemd/system/megavpn-route-policy.service",
		"/etc/systemd/system/megavpn-backhaul-link.service",
	}
	for _, path := range blocked {
		if isEmergencyManagedFilePathForScope(path, "services_only") {
			t.Fatalf("expected %s to be preserved for services_only", path)
		}
	}
}

func TestEmergencyCleanupManagedUnitPathUsesScope(t *testing.T) {
	if got := emergencyManagedUnitFilePathForScope("megavpn-xray-edge.service", "services_only"); got != "/etc/systemd/system/megavpn-xray-edge.service" {
		t.Fatalf("services_only service unit path = %q", got)
	}
	for _, unit := range []string{"megavpn-route-policy.service", "megavpn-backhaul-link.service"} {
		if got := emergencyManagedUnitFilePathForScope(unit, "services_only"); got != "" {
			t.Fatalf("services_only must preserve %s, got path %q", unit, got)
		}
	}
	if got := emergencyManagedUnitFilePathForScope("megavpn-backhaul-link.service", "full_node"); got != "/etc/systemd/system/megavpn-backhaul-link.service" {
		t.Fatalf("full_node backhaul unit path = %q", got)
	}
}
