package main

import "testing"

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
