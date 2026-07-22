package main

import (
	"strings"
	"testing"
)

func validExternalEgressPayload(protocol, config string) externalEgressJobPayload {
	return externalEgressJobPayload{
		NodeID: "node-1", ProfileID: "123e4567-e89b-42d3-a456-426614174000",
		DeploymentID: "123e4567-e89b-42d3-a456-426614174001", Protocol: protocol,
		InterfaceName: "mgev0123456789", RoutingTable: 40123, RouteMetric: 100,
		FWMark: 0x4d590123, Secrets: map[string]string{"config": config},
	}
}

func TestRenderManagedOpenVPNConfigIsolatesRouting(t *testing.T) {
	raw := "client\nproto tcp-client\nremote vpn.example.com 443\ndev tun9\nredirect-gateway def1\nauth-user-pass\n"
	payload := validExternalEgressPayload("openvpn", raw)
	payload.Secrets["username"] = "client"
	payload.Secrets["password"] = "secret"
	config, files, err := renderManagedOpenVPNConfig(payload, "/etc/megavpn/external-egress/123e4567-e89b-42d3-a456-426614174001")
	if err != nil {
		t.Fatalf("renderManagedOpenVPNConfig returned error: %v", err)
	}
	for _, forbidden := range []string{"dev tun9", "redirect-gateway def1"} {
		if strings.Contains(config, forbidden) {
			t.Fatalf("managed config preserved unsafe routing directive %q", forbidden)
		}
	}
	for _, required := range []string{"dev mgev0123456789", "route-nopull", "route-noexec", "auth-user-pass /etc/megavpn/"} {
		if !strings.Contains(config, required) {
			t.Fatalf("managed config is missing %q:\n%s", required, config)
		}
	}
	if len(files) != 1 || !strings.HasSuffix(files[0].Path, "/auth.txt") || files[0].Mode != "0600" {
		t.Fatalf("unexpected managed auth file: %#v", files)
	}
}

func TestRenderManagedOpenVPNConfigAcceptsInlineCredentials(t *testing.T) {
	raw := "client\nremote vpn.example.com 443\nauth-user-pass [inline]\n<auth-user-pass>\nclient\nsecret\n</auth-user-pass>\n"
	payload := validExternalEgressPayload("openvpn", raw)
	config, files, err := renderManagedOpenVPNConfig(payload, "/etc/megavpn/external-egress/123e4567-e89b-42d3-a456-426614174001")
	if err != nil {
		t.Fatalf("renderManagedOpenVPNConfig returned error: %v", err)
	}
	if !strings.Contains(config, "<auth-user-pass>") || len(files) != 0 {
		t.Fatalf("inline credentials were not preserved: files=%#v config=%s", files, config)
	}
}

func TestRenderManagedOpenVPNConfigReplacesInlineCertificate(t *testing.T) {
	raw := "client\nremote vpn.example.com 443\nca [inline]\n<ca>\nold-ca\n</ca>\n"
	payload := validExternalEgressPayload("openvpn", raw)
	payload.Secrets["ca_certificate"] = "new-ca"
	config, files, err := renderManagedOpenVPNConfig(payload, externalEgressManagedDir(payload))
	if err != nil {
		t.Fatalf("renderManagedOpenVPNConfig returned error: %v", err)
	}
	if strings.Contains(config, "old-ca") || strings.Contains(config, "<ca>") {
		t.Fatalf("superseded inline CA was retained:\n%s", config)
	}
	if !strings.Contains(config, "ca "+externalEgressManagedDir(payload)+"/ca_certificate.pem") {
		t.Fatalf("managed CA path is missing:\n%s", config)
	}
	if len(files) != 1 || files[0].Content != "new-ca" {
		t.Fatalf("unexpected replacement CA file: %#v", files)
	}
}

func TestRenderManagedWireGuardConfigUsesDedicatedTable(t *testing.T) {
	raw := "[Interface]\nPrivateKey = private\nAddress = 10.10.0.2/32\nDNS = 1.1.1.1\nTable = auto\n[Peer]\nPublicKey = public\nEndpoint = vpn.example.com:51820\nAllowedIPs = 0.0.0.0/0\n"
	payload := validExternalEgressPayload("wireguard", raw)
	config, err := renderManagedWireGuardConfig(payload)
	if err != nil {
		t.Fatalf("renderManagedWireGuardConfig returned error: %v", err)
	}
	if strings.Contains(config, "DNS =") || strings.Contains(config, "Table = auto") || !strings.Contains(config, "Table = 40123") {
		t.Fatalf("WireGuard managed routing is invalid:\n%s", config)
	}
}

func TestRenderManagedWireGuardConfigInjectsSeparatePrivateKey(t *testing.T) {
	raw := "[Interface]\nAddress = 10.10.0.2/32\n[Peer]\nPublicKey = public\nEndpoint = vpn.example.com:51820\nAllowedIPs = 0.0.0.0/0\n"
	payload := validExternalEgressPayload("wireguard", raw)
	payload.Secrets["private_key"] = "separate-private-key"
	config, err := renderManagedWireGuardConfig(payload)
	if err != nil {
		t.Fatalf("renderManagedWireGuardConfig returned error: %v", err)
	}
	if !strings.Contains(config, "PrivateKey = separate-private-key") {
		t.Fatalf("separate private key was not injected:\n%s", config)
	}
}

func TestDecodeExternalEgressJobRejectsNodeAndMarkMismatch(t *testing.T) {
	payload := map[string]any{
		"node_id": "node-2", "profile_id": "123e4567-e89b-42d3-a456-426614174000",
		"deployment_id": "123e4567-e89b-42d3-a456-426614174001", "protocol": "openvpn",
		"interface_name": "mgev0123456789", "routing_table": 40123, "route_metric": 100, "fwmark": 0x4d590123,
	}
	if _, err := decodeExternalEgressJob(job{Payload: payload}, agentState{NodeID: "node-1"}, false); err == nil {
		t.Fatal("expected node mismatch to be rejected")
	}
	payload["node_id"] = "node-1"
	payload["fwmark"] = 1
	if _, err := decodeExternalEgressJob(job{Payload: payload}, agentState{NodeID: "node-1"}, false); err == nil {
		t.Fatal("expected unmanaged fwmark to be rejected")
	}
}

func TestRenderExternalEgressRouteScriptUsesDeploymentSpecificRule(t *testing.T) {
	payload := validExternalEgressPayload("openvpn", "client\nremote vpn.example.com 1194\n")
	script := renderExternalEgressRouteScript(payload)
	for _, required := range []string{
		"pref 50123 fwmark 0x4d590123 lookup 40123",
		"ip route flush table 40123",
		"ip route replace unreachable default metric 32767 table 40123",
		`if [ "$action" = guard ]`,
	} {
		if !strings.Contains(script, required) {
			t.Fatalf("route script is missing %q:\n%s", required, script)
		}
	}
	if strings.Contains(script, "ip rule del pref 50123 2>") {
		t.Fatalf("route script deletes rules by priority only:\n%s", script)
	}
}

func TestRenderExternalEgressUnitKeepsFailClosedGuardOnStop(t *testing.T) {
	payload := validExternalEgressPayload("openvpn", "client\nremote vpn.example.com 1194\n")
	dir := externalEgressManagedDir(payload)
	routeScript := dir + "/route-policy.sh"
	files := []managedFileSpec{
		{Path: routeScript, Content: "#!/bin/sh\n", Mode: "0700"},
		{Path: dir + "/client.ovpn", Content: "client\n", Mode: "0600"},
	}
	unit := renderExternalEgressUnit(payload, routeScript, "openvpn --config "+dir+"/client.ovpn", []string{routeScript + " guard"})
	if !strings.Contains(unit, "route-policy.sh guard") || strings.Contains(unit, "route-policy.sh cleanup") {
		t.Fatalf("unit does not preserve fail-closed routing on stop:\n%s", unit)
	}
	if err := validateExternalEgressManagedArtifacts(payload, files, externalEgressUnitPath(payload)); err != nil {
		t.Fatalf("generated artifacts were rejected: %v", err)
	}
}

func TestValidateExternalEgressManagedArtifactsRejectsPathEscapeAndMode(t *testing.T) {
	payload := validExternalEgressPayload("openvpn", "client\nremote vpn.example.com 1194\n")
	for name, files := range map[string][]managedFileSpec{
		"path escape": {{Path: "/tmp/client.ovpn", Content: "client\n", Mode: "0600"}},
		"unsafe mode": {{Path: externalEgressManagedDir(payload) + "/client.ovpn", Content: "client\n", Mode: "0644"}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateExternalEgressManagedArtifacts(payload, files, externalEgressUnitPath(payload)); err == nil {
				t.Fatal("expected managed artifact validation to fail")
			}
		})
	}
}

func TestExternalEgressRouteHealthRequiresProviderDefault(t *testing.T) {
	provider, guard := externalEgressRouteHealth("unreachable default metric 32767", "mgev0123456789")
	if provider || !guard {
		t.Fatalf("fail-closed route health = provider %t guard %t, want false/true", provider, guard)
	}

	routes := "default via 10.10.0.1 dev mgev0123456789 metric 100\nunreachable default metric 32767"
	provider, guard = externalEgressRouteHealth(routes, "mgev0123456789")
	if !provider || !guard {
		t.Fatalf("active route health = provider %t guard %t, want true/true", provider, guard)
	}

	provider, _ = externalEgressRouteHealth(routes, "mgev9999999999")
	if provider {
		t.Fatal("provider route on a different interface was accepted")
	}
}

func TestParseEmergencyExternalEgressRulesOnlyAcceptsManagedTriples(t *testing.T) {
	output := `0: from all lookup local
50123: from all fwmark 0x4d590123 lookup 40123
50124: from all fwmark 0x4d590124/0xffffffff lookup 40124
50125: from all fwmark 0x4d590125 lookup 40126
32766: from all lookup main
`
	rules := parseEmergencyExternalEgressRules(output)
	if len(rules) != 2 {
		t.Fatalf("managed external egress rules = %#v, want 2", rules)
	}
	if rules[0].Priority != 50123 || rules[0].Table != 40123 || rules[0].Mark != 0x4d590123 {
		t.Fatalf("first managed external egress rule = %#v", rules[0])
	}
}
