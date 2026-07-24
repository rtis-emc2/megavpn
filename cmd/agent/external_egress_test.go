package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
)

func validExternalEgressPayload(protocol, config string) externalEgressJobPayload {
	return externalEgressJobPayload{
		NodeID: "node-1", ProfileID: "123e4567-e89b-42d3-a456-426614174000",
		DeploymentID: "123e4567-e89b-42d3-a456-426614174001", Protocol: protocol,
		InterfaceName: "mgev0123456789", RoutingTable: 40123, RouteMetric: 100,
		FWMark: 0x4d590123, Secrets: map[string]string{"config": config},
	}
}

func externalEgressProbeJob(payload externalEgressJobPayload) job {
	return job{Payload: map[string]any{
		"node_id": payload.NodeID, "profile_id": payload.ProfileID,
		"deployment_id": payload.DeploymentID, "protocol": payload.Protocol,
		"interface_name": payload.InterfaceName, "routing_table": payload.RoutingTable,
		"route_metric": payload.RouteMetric, "fwmark": payload.FWMark,
		"proxy_port": payload.ProxyPort,
	}}
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

func TestRenderManagedShadowsocksXrayConfigUsesLoopbackOnly(t *testing.T) {
	payload := validExternalEgressPayload("shadowsocks", `{"server":"ss.example.com","server_port":8388,"method":"aes-256-gcm"}`)
	payload.ProxyPort = externalEgressProxyPort(payload.RoutingTable)
	payload.Secrets["password"] = "provider-secret"
	config, err := renderManagedShadowsocksXrayConfig(payload)
	if err != nil {
		t.Fatalf("renderManagedShadowsocksXrayConfig returned error: %v", err)
	}
	if !strings.Contains(config, `"listen": "127.0.0.1"`) || !strings.Contains(config, `"port": 20123`) {
		t.Fatalf("Shadowsocks loopback proxy is missing:\n%s", config)
	}
	if strings.Contains(config, `"listen": "0.0.0.0"`) {
		t.Fatalf("Shadowsocks proxy exposed a public listener:\n%s", config)
	}
}

func TestRenderManagedVLESSXrayConfigPreservesSecureTransport(t *testing.T) {
	payload := validExternalEgressPayload("vless", "vless://123e4567-e89b-42d3-a456-426614174002@edge.example.com:443?security=tls&sni=edge.example.com&type=ws&path=%2Fprovider&host=edge.example.com")
	payload.ProxyPort = externalEgressProxyPort(payload.RoutingTable)
	config, err := renderManagedVLESSXrayConfig(payload)
	if err != nil {
		t.Fatalf("renderManagedVLESSXrayConfig returned error: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(config), &parsed); err != nil {
		t.Fatalf("rendered VLESS config is not JSON: %v", err)
	}
	if !strings.Contains(config, `"serverName": "edge.example.com"`) || !strings.Contains(config, `"allowInsecure": false`) || !strings.Contains(config, `"path": "/provider"`) {
		t.Fatalf("VLESS secure transport was not preserved:\n%s", config)
	}
}

func TestRenderManagedL2TPIPsecConfigIsolatesDefaultRoute(t *testing.T) {
	payload := validExternalEgressPayload("l2tp_ipsec", "server=l2tp.example.com\nremote_id=@l2tp.example.com\n")
	payload.Secrets["username"] = "provider-user"
	payload.Secrets["password"] = "provider-pass"
	payload.Secrets["preshared_key"] = "provider-psk"
	ipsecConfig, ipsecSecrets, xl2tpConfig, pppOptions, err := renderManagedL2TPIPsecConfig(payload)
	if err != nil {
		t.Fatalf("renderManagedL2TPIPsecConfig returned error: %v", err)
	}
	for _, required := range []string{"type=transport", "right=l2tp.example.com", "auto=add"} {
		if !strings.Contains(ipsecConfig, required) {
			t.Fatalf("IPsec config is missing %q:\n%s", required, ipsecConfig)
		}
	}
	if !strings.Contains(ipsecSecrets, `PSK "provider-psk"`) || !strings.Contains(xl2tpConfig, "autodial = yes") {
		t.Fatalf("L2TP/IPsec managed files are incomplete")
	}
	for _, required := range []string{
		"nodefaultroute",
		"ifname mgev0123456789",
		"ipparam mgev0123456789",
		"linkname mgev0123456789",
	} {
		if !strings.Contains(pppOptions, required) {
			t.Fatalf("PPP options do not contain %q:\n%s", required, pppOptions)
		}
	}
}

func TestRenderManagedL2TPIPsecConfigPreservesSeparateSecretWhitespace(t *testing.T) {
	payload := validExternalEgressPayload("l2tp_ipsec", "server=l2tp.example.com\n")
	payload.Secrets["username"] = "provider-user"
	payload.Secrets["password"] = " password with spaces "
	payload.Secrets["preshared_key"] = " psk with spaces "
	_, ipsecSecrets, _, pppOptions, err := renderManagedL2TPIPsecConfig(payload)
	if err != nil {
		t.Fatalf("renderManagedL2TPIPsecConfig returned error: %v", err)
	}
	if !strings.Contains(ipsecSecrets, `PSK " psk with spaces "`) || !strings.Contains(pppOptions, `password " password with spaces "`) {
		t.Fatalf("separate secret whitespace was changed: secrets=%q ppp=%q", ipsecSecrets, pppOptions)
	}
}

func TestRenderManagedL2TPIPsecConfigSupportsCertificateAuthentication(t *testing.T) {
	caPEM, certificatePEM, privateKeyPEM := testL2TPIPsecCertificateMaterial(t)
	payload := validExternalEgressPayload("l2tp_ipsec", `{"server":"l2tp.example.com","remote_id":"gateway.example.com","auth_method":"certificate"}`)
	payload.Secrets["username"] = "provider-user"
	payload.Secrets["password"] = "provider-pass"
	payload.Secrets["ca_certificate"] = caPEM
	payload.Secrets["certificate"] = certificatePEM
	payload.Secrets["private_key"] = privateKeyPEM

	ipsecConfig, ipsecSecrets, _, _, err := renderManagedL2TPIPsecConfig(payload)
	if err != nil {
		t.Fatalf("renderManagedL2TPIPsecConfig returned error: %v", err)
	}
	for _, required := range []string{"leftauth=pubkey", "rightauth=pubkey", "leftcert=/etc/ipsec.d/certs/megavpn-", "rightca="} {
		if !strings.Contains(ipsecConfig, required) {
			t.Fatalf("certificate IPsec config is missing %q:\n%s", required, ipsecConfig)
		}
	}
	if !strings.Contains(ipsecSecrets, ": RSA /etc/ipsec.d/private/megavpn-") || strings.Contains(ipsecSecrets, "PSK") {
		t.Fatalf("certificate IPsec secrets are invalid:\n%s", ipsecSecrets)
	}
	material, err := renderManagedL2TPIPsecCertificateMaterial(payload)
	if err != nil {
		t.Fatalf("render certificate material: %v", err)
	}
	if len(material.Files) != 3 {
		t.Fatalf("certificate material files = %d, want 3", len(material.Files))
	}
}

func TestRenderManagedL2TPIPsecConfigRejectsMismatchedPrivateKey(t *testing.T) {
	caPEM, certificatePEM, _ := testL2TPIPsecCertificateMaterial(t)
	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate mismatched key: %v", err)
	}
	payload := validExternalEgressPayload("l2tp_ipsec", `{"server":"l2tp.example.com","auth_method":"certificate"}`)
	payload.Secrets["username"] = "provider-user"
	payload.Secrets["password"] = "provider-pass"
	payload.Secrets["ca_certificate"] = caPEM
	payload.Secrets["certificate"] = certificatePEM
	payload.Secrets["private_key"] = string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(otherKey)}))
	if _, _, _, _, err := renderManagedL2TPIPsecConfig(payload); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("mismatched private key error = %v", err)
	}
}

func TestRenderManagedL2TPIPsecConfigRejectsControlCharacters(t *testing.T) {
	payload := validExternalEgressPayload("l2tp_ipsec", "server=l2tp.example.com\n")
	payload.Secrets["username"] = "provider-user"
	payload.Secrets["password"] = "provider\tpass"
	payload.Secrets["preshared_key"] = "provider-psk"
	if _, _, _, _, err := renderManagedL2TPIPsecConfig(payload); err == nil {
		t.Fatal("expected a control character in an L2TP secret to be rejected")
	}
}

func TestCleanupExternalEgressPolicyTreatsMissingRoutingTableAsClean(t *testing.T) {
	oldRun := runInstallCommand
	t.Cleanup(func() { runInstallCommand = oldRun })
	runInstallCommand = func(_ context.Context, name string, args ...string) (int, string) {
		command := name + " " + strings.Join(args, " ")
		switch command {
		case "ip rule del pref 50123 fwmark 0x4d590123 lookup 40123":
			return 2, "RTNETLINK answers: No such file or directory"
		case "ip route flush table 40123", "ip route show table 40123":
			return 2, "Error: ipv4: FIB table does not exist."
		case "ip rule show":
			return 0, ""
		default:
			t.Fatalf("unexpected cleanup command: %s", command)
			return 1, "unexpected command"
		}
	}

	if err := cleanupExternalEgressPolicy(context.Background(), validExternalEgressPayload("l2tp_ipsec", "server=l2tp.example.com\n")); err != nil {
		t.Fatalf("cleanupExternalEgressPolicy returned error for an absent routing table: %v", err)
	}
}

func TestMissingIPRouteTableOutput(t *testing.T) {
	for _, output := range []string{
		"Error: ipv4: FIB table does not exist.",
		"Error: ipv6: FIB table does not exist.",
		"routing table does not exist",
	} {
		if !isMissingIPRouteTableOutput(output) {
			t.Fatalf("missing route table output was not recognized: %q", output)
		}
	}
	if isMissingIPRouteTableOutput("RTNETLINK answers: Operation not permitted") {
		t.Fatal("permission failure must not be treated as an absent routing table")
	}
}

func TestInstallExternalEgressFailClosedGuardCreatesMissingRoutingTable(t *testing.T) {
	oldRun := runInstallCommand
	t.Cleanup(func() { runInstallCommand = oldRun })
	commands := []string{}
	runInstallCommand = func(_ context.Context, name string, args ...string) (int, string) {
		command := name + " " + strings.Join(args, " ")
		commands = append(commands, command)
		switch command {
		case "ip route flush table 40123":
			return 2, "Error: ipv4: FIB table does not exist."
		case "ip route replace unreachable default metric 32767 table 40123":
			return 0, ""
		case "ip rule del pref 50123 fwmark 0x4d590123 lookup 40123":
			return 2, "RTNETLINK answers: No such file or directory"
		case "ip rule add pref 50123 fwmark 0x4d590123 lookup 40123":
			return 0, ""
		default:
			t.Fatalf("unexpected guard command: %s", command)
			return 1, "unexpected command"
		}
	}

	if err := installExternalEgressFailClosedGuard(context.Background(), validExternalEgressPayload("l2tp_ipsec", "server=l2tp.example.com\n")); err != nil {
		t.Fatalf("installExternalEgressFailClosedGuard returned error for an absent routing table: %v", err)
	}
	want := []string{
		"ip route flush table 40123",
		"ip route replace unreachable default metric 32767 table 40123",
		"ip rule del pref 50123 fwmark 0x4d590123 lookup 40123",
		"ip rule add pref 50123 fwmark 0x4d590123 lookup 40123",
	}
	if !slices.Equal(commands, want) {
		t.Fatalf("guard commands = %#v, want %#v", commands, want)
	}
}

func TestInstallExternalEgressFailClosedGuardRejectsRouteFlushFailure(t *testing.T) {
	oldRun := runInstallCommand
	t.Cleanup(func() { runInstallCommand = oldRun })
	runInstallCommand = func(_ context.Context, name string, args ...string) (int, string) {
		command := name + " " + strings.Join(args, " ")
		if command != "ip route flush table 40123" {
			t.Fatalf("unexpected guard command after route flush failure: %s", command)
		}
		return 2, "RTNETLINK answers: Operation not permitted"
	}

	err := installExternalEgressFailClosedGuard(context.Background(), validExternalEgressPayload("l2tp_ipsec", "server=l2tp.example.com\n"))
	if err == nil || !strings.Contains(err.Error(), "Operation not permitted") {
		t.Fatalf("route flush permission error = %v", err)
	}
}

func TestReloadExternalEgressIPsecAfterCleanupSkipsMissingRuntime(t *testing.T) {
	oldResolve := resolveExternalEgressCleanupExecutable
	oldRun := runInstallCommand
	t.Cleanup(func() {
		resolveExternalEgressCleanupExecutable = oldResolve
		runInstallCommand = oldRun
	})
	resolveExternalEgressCleanupExecutable = func(name string, _ ...string) (string, bool) {
		if name != "ipsec" {
			t.Fatalf("unexpected executable lookup: %s", name)
		}
		return name, false
	}
	runInstallCommand = func(_ context.Context, name string, _ ...string) (int, string) {
		t.Fatalf("cleanup attempted to execute missing binary: %s", name)
		return 1, "unexpected command"
	}

	if warning := reloadExternalEgressIPsecAfterCleanup(context.Background()); warning != "" {
		t.Fatalf("missing runtime produced cleanup warning: %q", warning)
	}
}

func TestRemoveExactManagedConfigLineContent(t *testing.T) {
	t.Parallel()

	input := strings.Join([]string{
		"config setup",
		"  " + externalEgressIPsecConfigInclude + "  ",
		"include /etc/ipsec.d/*.conf",
		"",
	}, "\n")
	got, removed := removeExactManagedConfigLineContent(input, externalEgressIPsecConfigInclude)
	if !removed {
		t.Fatal("expected managed include to be removed")
	}
	if strings.Contains(got, externalEgressIPsecConfigInclude) {
		t.Fatalf("managed include remains in %q", got)
	}
	if !strings.Contains(got, "include /etc/ipsec.d/*.conf") {
		t.Fatalf("unrelated include was removed from %q", got)
	}
	unchanged, removed := removeExactManagedConfigLineContent(got, externalEgressIPsecConfigInclude)
	if removed || unchanged != got {
		t.Fatalf("idempotent cleanup changed content: removed=%v content=%q", removed, unchanged)
	}
}

func TestPrepareExternalEgressL2TPTakeoverStopsSystemAndStaleManagedRuntime(t *testing.T) {
	oldRun := runInstallCommand
	oldGlob := globExternalEgressPaths
	oldRemove := removeExternalEgressManagedPath
	t.Cleanup(func() {
		runInstallCommand = oldRun
		globExternalEgressPaths = oldGlob
		removeExternalEgressManagedPath = oldRemove
	})
	current := validExternalEgressPayload("l2tp_ipsec", "server=l2tp.example.com\n")
	staleID := "223e4567-e89b-42d3-a456-426614174002"
	globExternalEgressPaths = func(pattern string) ([]string, error) {
		if pattern != filepath.Join(externalEgressManagedRoot, "*", "xl2tpd.conf") {
			t.Fatalf("unexpected glob pattern: %s", pattern)
		}
		return []string{
			filepath.Join(externalEgressManagedRoot, current.DeploymentID, "xl2tpd.conf"),
			filepath.Join(externalEgressManagedRoot, staleID, "xl2tpd.conf"),
			filepath.Join(externalEgressManagedRoot, "unsafe", "xl2tpd.conf"),
		}, nil
	}
	removeExternalEgressManagedPath = func(string, bool) (bool, error) {
		return true, nil
	}
	commands := []string{}
	runInstallCommand = func(_ context.Context, name string, args ...string) (int, string) {
		command := name + " " + strings.Join(args, " ")
		commands = append(commands, command)
		return 0, ""
	}

	result, err := prepareExternalEgressL2TPTakeover(context.Background(), current)
	if err != nil {
		t.Fatalf("prepareExternalEgressL2TPTakeover: %v", err)
	}
	got := strings.Join(commands, "\n")
	for _, want := range []string{
		"systemctl disable --now xl2tpd.service",
		"systemctl disable --now megavpn-external-egress-223e4567e89b42d3a456426614174002.service",
		"systemctl daemon-reload",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("commands %q do not contain %q", got, want)
		}
	}
	if strings.Contains(got, strings.ReplaceAll(current.DeploymentID, "-", "")+".service") {
		t.Fatalf("current deployment was treated as stale: %q", got)
	}
	stale, _ := result["stale_deployments"].([]string)
	if len(stale) != 1 || stale[0] != staleID {
		t.Fatalf("stale deployments = %#v", result["stale_deployments"])
	}
}

func TestReleaseExternalEgressL2TPInterfaceRemovesStaleInterface(t *testing.T) {
	oldRun := runInstallCommand
	oldInterval := externalEgressInterfaceReleaseInterval
	t.Cleanup(func() {
		runInstallCommand = oldRun
		externalEgressInterfaceReleaseInterval = oldInterval
	})
	externalEgressInterfaceReleaseInterval = 0

	payload := validExternalEgressPayload("l2tp_ipsec", "server=l2tp.example.com\n")
	commands := []string{}
	inspections := 0
	runInstallCommand = func(_ context.Context, name string, args ...string) (int, string) {
		command := name + " " + strings.Join(args, " ")
		commands = append(commands, command)
		if command == "ip -o link show dev "+payload.InterfaceName {
			inspections++
			if inspections == 1 {
				return 0, "14: " + payload.InterfaceName + ": <POINTOPOINT,UP> mtu 1400"
			}
			return 1, `Device "` + payload.InterfaceName + `" does not exist.`
		}
		return 0, ""
	}

	result, err := releaseExternalEgressL2TPInterface(context.Background(), payload)
	if err != nil {
		t.Fatalf("releaseExternalEgressL2TPInterface returned error: %v", err)
	}
	if result["released"] != true {
		t.Fatalf("interface release result = %#v", result)
	}
	got := strings.Join(commands, "\n")
	for _, required := range []string{
		"ip link set dev " + payload.InterfaceName + " down",
		"ip link delete dev " + payload.InterfaceName,
	} {
		if !strings.Contains(got, required) {
			t.Fatalf("commands %q do not contain %q", got, required)
		}
	}
}

func TestProbeExternalEgressL2TPRequiresStableCompleteRuntime(t *testing.T) {
	oldRun := runInstallCommand
	oldInterval := externalEgressProbeInterval
	t.Cleanup(func() {
		runInstallCommand = oldRun
		externalEgressProbeInterval = oldInterval
	})
	externalEgressProbeInterval = 0

	payload := validExternalEgressPayload("l2tp_ipsec", "server=l2tp.example.com\n")
	probes := 0
	runInstallCommand = func(_ context.Context, name string, args ...string) (int, string) {
		command := name + " " + strings.Join(args, " ")
		switch command {
		case "systemctl is-active " + externalEgressUnitName(payload):
			probes++
			return 0, "active\n"
		case "ip -o link show dev " + payload.InterfaceName:
			return 0, "14: " + payload.InterfaceName + ": <POINTOPOINT,UP> mtu 1400\n"
		case "ip -4 -o addr show dev " + payload.InterfaceName:
			return 0, "14: " + payload.InterfaceName + " inet 10.20.30.40 peer 10.20.30.1/32 scope global\n"
		case "ip rule show":
			return 0, "50123: from all fwmark 0x4d590123 lookup 40123\n"
		case "ip route show table 40123":
			return 0, "default dev " + payload.InterfaceName + " metric 100\nunreachable default metric 32767\n"
		case "ipsec status " + externalEgressIPsecConnectionName(payload):
			return 0, externalEgressIPsecConnectionName(payload) + "[1]: ESTABLISHED; INSTALLED, TRANSPORT\n"
		default:
			t.Fatalf("unexpected probe command: %s", command)
			return 1, ""
		}
	}

	status, result := (&client{}).probeExternalEgressUntilStable(
		context.Background(),
		externalEgressProbeJob(payload),
		agentState{NodeID: payload.NodeID},
		payload.Protocol,
	)
	if status != "succeeded" {
		t.Fatalf("probe status = %s, result = %#v", status, result)
	}
	if probes != 2 || result["stable_observations"] != 2 {
		t.Fatalf("stable probe count = %d, result = %#v", probes, result)
	}
}

func TestProbeExternalEgressL2TPReportsMissingLayers(t *testing.T) {
	oldRun := runInstallCommand
	t.Cleanup(func() { runInstallCommand = oldRun })

	payload := validExternalEgressPayload("l2tp_ipsec", "server=l2tp.example.com\n")
	runInstallCommand = func(_ context.Context, name string, args ...string) (int, string) {
		command := name + " " + strings.Join(args, " ")
		switch command {
		case "systemctl is-active " + externalEgressUnitName(payload):
			return 0, "active\n"
		case "ip -o link show dev " + payload.InterfaceName:
			return 1, `Device "` + payload.InterfaceName + `" does not exist.`
		case "ip -4 -o addr show dev " + payload.InterfaceName:
			return 1, `Device "` + payload.InterfaceName + `" does not exist.`
		case "ip rule show":
			return 0, ""
		case "ip route show table 40123":
			return 0, "unreachable default metric 32767\n"
		case "ipsec status " + externalEgressIPsecConnectionName(payload):
			return 0, "Security Associations (0 up, 0 connecting):\n"
		default:
			t.Fatalf("unexpected probe command: %s", command)
			return 1, ""
		}
	}

	status, result := (&client{}).probeExternalEgress(
		context.Background(),
		externalEgressProbeJob(payload),
		agentState{NodeID: payload.NodeID},
	)
	if status != "failed" {
		t.Fatalf("probe status = %s, result = %#v", status, result)
	}
	message := stringify(result["error"])
	for _, required := range []string{
		"PPP interface " + payload.InterfaceName,
		"IPv4 address on " + payload.InterfaceName,
		"provider default route in table 40123",
		"policy rule for 0x4d590123",
		"IPsec security association " + externalEgressIPsecConnectionName(payload),
	} {
		if !strings.Contains(message, required) {
			t.Fatalf("probe error %q does not contain %q", message, required)
		}
	}
}

func TestProbeExternalEgressL2TPRejectsSingleFinalSuccess(t *testing.T) {
	oldRun := runInstallCommand
	oldInterval := externalEgressProbeInterval
	t.Cleanup(func() {
		runInstallCommand = oldRun
		externalEgressProbeInterval = oldInterval
	})
	externalEgressProbeInterval = 0

	payload := validExternalEgressPayload("l2tp_ipsec", "server=l2tp.example.com\n")
	probeNumber := 0
	runInstallCommand = func(_ context.Context, name string, args ...string) (int, string) {
		command := name + " " + strings.Join(args, " ")
		switch command {
		case "systemctl is-active " + externalEgressUnitName(payload):
			probeNumber++
			if probeNumber%2 == 0 {
				return 0, "active\n"
			}
			return 3, "inactive\n"
		case "ip -o link show dev " + payload.InterfaceName:
			return 0, "14: " + payload.InterfaceName + ": <POINTOPOINT,UP> mtu 1400\n"
		case "ip -4 -o addr show dev " + payload.InterfaceName:
			return 0, "14: " + payload.InterfaceName + " inet 10.20.30.40 peer 10.20.30.1/32 scope global\n"
		case "ip rule show":
			return 0, "50123: from all fwmark 0x4d590123 lookup 40123\n"
		case "ip route show table 40123":
			return 0, "default dev " + payload.InterfaceName + " metric 100\nunreachable default metric 32767\n"
		case "ipsec status " + externalEgressIPsecConnectionName(payload):
			return 0, externalEgressIPsecConnectionName(payload) + "[1]: ESTABLISHED; INSTALLED, TRANSPORT\n"
		default:
			t.Fatalf("unexpected probe command: %s", command)
			return 1, ""
		}
	}

	status, result := (&client{}).probeExternalEgressUntilStable(
		context.Background(),
		externalEgressProbeJob(payload),
		agentState{NodeID: payload.NodeID},
		payload.Protocol,
	)
	if status != "failed" || !strings.Contains(stringify(result["error"]), "did not remain stable") {
		t.Fatalf("unstable probe status = %s, result = %#v", status, result)
	}
	if result["stable_observations"] != 1 {
		t.Fatalf("unstable probe result = %#v", result)
	}
}

func testL2TPIPsecCertificateMaterial(t *testing.T) (string, string, string) {
	t.Helper()
	now := time.Now().UTC()
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Test L2TP CA"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create CA certificate: %v", err)
	}
	clientKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate client key: %v", err)
	}
	clientTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "client.example.com"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	clientDER, err := x509.CreateCertificate(rand.Reader, clientTemplate, caTemplate, &clientKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create client certificate: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})),
		string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: clientDER})),
		string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(clientKey)}))
}

func TestRenderExternalEgressRuntimeOmitsPolicyRuleForLoopbackProxy(t *testing.T) {
	payload := validExternalEgressPayload("shadowsocks", `{"server":"ss.example.com","server_port":8388,"method":"aes-256-gcm","password":"secret"}`)
	payload.ProxyPort = externalEgressProxyPort(payload.RoutingTable)
	config, err := renderManagedShadowsocksXrayConfig(payload)
	if err != nil {
		t.Fatalf("renderManagedShadowsocksXrayConfig returned error: %v", err)
	}
	files := []managedFileSpec{{Path: externalEgressManagedDir(payload) + "/config.json", Content: config, Mode: "0600"}}
	unit := renderExternalEgressUnit(payload, "", "/usr/bin/xray run -config "+files[0].Path, nil)
	if strings.Contains(unit, "ExecStartPost=") {
		t.Fatalf("loopback proxy unit unexpectedly applies a routing table:\n%s", unit)
	}
	if err := validateExternalEgressManagedArtifacts(payload, files, externalEgressUnitPath(payload)); err != nil {
		t.Fatalf("loopback proxy artifacts were rejected: %v", err)
	}
}

func TestExternalEgressLoopbackPortListeningRejectsWildcard(t *testing.T) {
	if !externalEgressLoopbackPortListening("LISTEN 0 4096 127.0.0.1:20123 0.0.0.0:*", 20123) {
		t.Fatal("loopback listener was not detected")
	}
	if externalEgressLoopbackPortListening("LISTEN 0 4096 0.0.0.0:20123 0.0.0.0:*", 20123) {
		t.Fatal("wildcard listener was accepted as a loopback proxy")
	}
}

func TestExternalEgressUDPPortListeningDetectsL2TPConflicts(t *testing.T) {
	for _, output := range []string{
		"UNCONN 0 0 0.0.0.0:1701 0.0.0.0:*",
		"UNCONN 0 0 [::]:1701 [::]:*",
		"UNCONN 0 0 192.0.2.10:1701 0.0.0.0:*",
	} {
		if !externalEgressUDPPortListening(output, 1701) {
			t.Fatalf("L2TP listener was not detected in %q", output)
		}
	}
	if externalEgressUDPPortListening("UNCONN 0 0 0.0.0.0:1194 0.0.0.0:*", 1701) {
		t.Fatal("unrelated UDP listener was reported as an L2TP conflict")
	}
	owner := `UNCONN 0 0 0.0.0.0:1701 0.0.0.0:* users:(("xl2tpd",pid=4242,fd=3))`
	if got := externalEgressUDPPortOwner(owner, 1701); got != owner {
		t.Fatalf("UDP/1701 owner = %q, want %q", got, owner)
	}
}

func TestTerminateManagedExternalEgressL2TPProcessStopsOwnedOrphan(t *testing.T) {
	oldReadFile := readExternalEgressManagedFile
	oldReadlink := readExternalEgressProcessExecutable
	oldReadCommandLine := readExternalEgressProcessCommandLine
	oldSignal := signalExternalEgressProcess
	t.Cleanup(func() {
		readExternalEgressManagedFile = oldReadFile
		readExternalEgressProcessExecutable = oldReadlink
		readExternalEgressProcessCommandLine = oldReadCommandLine
		signalExternalEgressProcess = oldSignal
	})

	readExternalEgressManagedFile = func(string) ([]byte, error) {
		return []byte("4242\n"), nil
	}
	processExited := false
	readExternalEgressProcessExecutable = func(path string) (string, error) {
		if path != "/proc/4242/exe" {
			t.Fatalf("unexpected process path: %s", path)
		}
		if processExited {
			return "", os.ErrNotExist
		}
		return "/usr/sbin/xl2tpd", nil
	}
	readExternalEgressProcessCommandLine = func(string) ([]byte, error) {
		return []byte("/usr/sbin/xl2tpd\x00-D\x00-p\x00/run/megavpn/external-egress/38fdafed754e4e5e8bdef546e05674f3/xl2tpd.pid\x00"), nil
	}
	signals := []syscall.Signal{}
	signalExternalEgressProcess = func(pid int, signal syscall.Signal) error {
		if pid != 4242 {
			t.Fatalf("unexpected process id: %d", pid)
		}
		signals = append(signals, signal)
		processExited = true
		return nil
	}

	result, err := terminateManagedExternalEgressL2TPProcess(
		context.Background(),
		validExternalEgressPayload("l2tp_ipsec", "server=l2tp.example.com\n"),
	)
	if err != nil {
		t.Fatalf("terminateManagedExternalEgressL2TPProcess: %v", err)
	}
	if result["terminated"] != true {
		t.Fatalf("managed process result = %#v", result)
	}
	if len(signals) != 1 || signals[0] != syscall.SIGTERM {
		t.Fatalf("managed process signals = %#v", signals)
	}
}

func TestTerminateManagedExternalEgressL2TPProcessNeverSignalsReusedPID(t *testing.T) {
	oldReadFile := readExternalEgressManagedFile
	oldReadlink := readExternalEgressProcessExecutable
	oldReadCommandLine := readExternalEgressProcessCommandLine
	oldSignal := signalExternalEgressProcess
	t.Cleanup(func() {
		readExternalEgressManagedFile = oldReadFile
		readExternalEgressProcessExecutable = oldReadlink
		readExternalEgressProcessCommandLine = oldReadCommandLine
		signalExternalEgressProcess = oldSignal
	})

	readExternalEgressManagedFile = func(string) ([]byte, error) {
		return []byte("4242\n"), nil
	}
	readExternalEgressProcessExecutable = func(string) (string, error) {
		return "/usr/sbin/nginx", nil
	}
	signalExternalEgressProcess = func(int, syscall.Signal) error {
		t.Fatal("reused PID was signaled")
		return nil
	}

	result, err := terminateManagedExternalEgressL2TPProcess(
		context.Background(),
		validExternalEgressPayload("l2tp_ipsec", "server=l2tp.example.com\n"),
	)
	if err != nil {
		t.Fatalf("terminateManagedExternalEgressL2TPProcess: %v", err)
	}
	if result["pid_reused"] != true || result["terminated"] != false {
		t.Fatalf("reused PID result = %#v", result)
	}
}

func TestTerminateManagedExternalEgressL2TPProcessRejectsUnmanagedCommandLine(t *testing.T) {
	oldReadFile := readExternalEgressManagedFile
	oldReadlink := readExternalEgressProcessExecutable
	oldReadCommandLine := readExternalEgressProcessCommandLine
	oldSignal := signalExternalEgressProcess
	t.Cleanup(func() {
		readExternalEgressManagedFile = oldReadFile
		readExternalEgressProcessExecutable = oldReadlink
		readExternalEgressProcessCommandLine = oldReadCommandLine
		signalExternalEgressProcess = oldSignal
	})

	readExternalEgressManagedFile = func(string) ([]byte, error) {
		return []byte("4242\n"), nil
	}
	readExternalEgressProcessExecutable = func(string) (string, error) {
		return "/usr/sbin/xl2tpd", nil
	}
	readExternalEgressProcessCommandLine = func(string) ([]byte, error) {
		return []byte("/usr/sbin/xl2tpd\x00-D\x00-c\x00/etc/xl2tpd/xl2tpd.conf\x00"), nil
	}
	signalExternalEgressProcess = func(int, syscall.Signal) error {
		t.Fatal("XL2TPD process without managed arguments was signaled")
		return nil
	}

	result, err := terminateManagedExternalEgressL2TPProcess(
		context.Background(),
		validExternalEgressPayload("l2tp_ipsec", "server=l2tp.example.com\n"),
	)
	if err != nil {
		t.Fatalf("terminateManagedExternalEgressL2TPProcess: %v", err)
	}
	if result["ownership_mismatch"] != true || result["terminated"] != false {
		t.Fatalf("unmanaged command line result = %#v", result)
	}
}

func TestTerminateManagedExternalEgressL2TPListenerRecoversLostPIDFile(t *testing.T) {
	oldReadlink := readExternalEgressProcessExecutable
	oldReadCommandLine := readExternalEgressProcessCommandLine
	oldReadCgroup := readExternalEgressProcessCgroup
	oldSignal := signalExternalEgressProcess
	t.Cleanup(func() {
		readExternalEgressProcessExecutable = oldReadlink
		readExternalEgressProcessCommandLine = oldReadCommandLine
		readExternalEgressProcessCgroup = oldReadCgroup
		signalExternalEgressProcess = oldSignal
	})

	processExited := false
	readExternalEgressProcessExecutable = func(path string) (string, error) {
		if path != "/proc/4242/exe" {
			t.Fatalf("unexpected process path: %s", path)
		}
		if processExited {
			return "", os.ErrNotExist
		}
		return "/usr/sbin/xl2tpd", nil
	}
	readExternalEgressProcessCommandLine = func(path string) ([]byte, error) {
		if path != "/proc/4242/cmdline" {
			t.Fatalf("unexpected command line path: %s", path)
		}
		return []byte("/usr/sbin/xl2tpd\x00-D\x00-c\x00/etc/megavpn/external-egress/38fdafed-754e-4e5e-8bde-f546e05674f3/xl2tpd.conf\x00"), nil
	}
	signalExternalEgressProcess = func(pid int, signal syscall.Signal) error {
		if pid != 4242 || signal != syscall.SIGTERM {
			t.Fatalf("unexpected managed listener signal: pid=%d signal=%v", pid, signal)
		}
		processExited = true
		return nil
	}

	output := `UNCONN 0 0 0.0.0.0:1701 0.0.0.0:* users:(("xl2tpd",pid=4242,fd=3))`
	result, err := terminateManagedExternalEgressL2TPListener(context.Background(), output, 1701, false)
	if err != nil {
		t.Fatalf("terminateManagedExternalEgressL2TPListener: %v", err)
	}
	if result["managed"] != true || result["terminated"] != true {
		t.Fatalf("managed listener result = %#v", result)
	}
}

func TestTerminateManagedExternalEgressL2TPListenerLeavesUnmanagedProcess(t *testing.T) {
	oldReadlink := readExternalEgressProcessExecutable
	oldReadCommandLine := readExternalEgressProcessCommandLine
	oldReadCgroup := readExternalEgressProcessCgroup
	oldSignal := signalExternalEgressProcess
	t.Cleanup(func() {
		readExternalEgressProcessExecutable = oldReadlink
		readExternalEgressProcessCommandLine = oldReadCommandLine
		readExternalEgressProcessCgroup = oldReadCgroup
		signalExternalEgressProcess = oldSignal
	})

	readExternalEgressProcessExecutable = func(string) (string, error) {
		return "/usr/sbin/xl2tpd", nil
	}
	readExternalEgressProcessCommandLine = func(string) ([]byte, error) {
		return []byte("/usr/sbin/xl2tpd\x00-D\x00-c\x00/etc/xl2tpd/xl2tpd.conf\x00"), nil
	}
	readExternalEgressProcessCgroup = func(string) ([]byte, error) {
		return []byte("0::/system.slice/xl2tpd.service.attacker\n"), nil
	}
	signalExternalEgressProcess = func(int, syscall.Signal) error {
		t.Fatal("unmanaged XL2TPD listener was signaled")
		return nil
	}

	output := `UNCONN 0 0 [::]:1701 [::]:* users:(("xl2tpd",pid=4242,fd=3))`
	result, err := terminateManagedExternalEgressL2TPListener(context.Background(), output, 1701, true)
	if err != nil {
		t.Fatalf("terminateManagedExternalEgressL2TPListener: %v", err)
	}
	if result["managed"] != false || result["terminated"] != false {
		t.Fatalf("unmanaged listener result = %#v", result)
	}
}

func TestTerminateManagedExternalEgressL2TPListenerStopsSystemdServiceProcess(t *testing.T) {
	oldReadlink := readExternalEgressProcessExecutable
	oldReadCommandLine := readExternalEgressProcessCommandLine
	oldReadCgroup := readExternalEgressProcessCgroup
	oldSignal := signalExternalEgressProcess
	t.Cleanup(func() {
		readExternalEgressProcessExecutable = oldReadlink
		readExternalEgressProcessCommandLine = oldReadCommandLine
		readExternalEgressProcessCgroup = oldReadCgroup
		signalExternalEgressProcess = oldSignal
	})

	processExited := false
	readExternalEgressProcessExecutable = func(string) (string, error) {
		if processExited {
			return "", os.ErrNotExist
		}
		return "/usr/sbin/xl2tpd", nil
	}
	readExternalEgressProcessCommandLine = func(string) ([]byte, error) {
		return []byte("/usr/sbin/xl2tpd\x00-D\x00-c\x00/etc/xl2tpd/xl2tpd.conf\x00"), nil
	}
	readExternalEgressProcessCgroup = func(path string) ([]byte, error) {
		if path != "/proc/994/cgroup" {
			t.Fatalf("unexpected cgroup path: %s", path)
		}
		return []byte("0::/system.slice/xl2tpd.service\n"), nil
	}
	signalExternalEgressProcess = func(pid int, signal syscall.Signal) error {
		if pid != 994 || signal != syscall.SIGTERM {
			t.Fatalf("unexpected system service signal: pid=%d signal=%v", pid, signal)
		}
		processExited = true
		return nil
	}

	output := `UNCONN 0 0 0.0.0.0:1701 0.0.0.0:* users:(("xl2tpd",pid=994,fd=3))`
	result, err := terminateManagedExternalEgressL2TPListener(context.Background(), output, 1701, true)
	if err != nil {
		t.Fatalf("terminateManagedExternalEgressL2TPListener: %v", err)
	}
	if result["managed"] != true || result["terminated"] != true || result["ownership"] != "xl2tpd.service" {
		t.Fatalf("system service listener result = %#v", result)
	}
}

func TestTerminateManagedExternalEgressL2TPListenerRevalidatesSystemdServiceProcess(t *testing.T) {
	oldReadlink := readExternalEgressProcessExecutable
	oldReadCommandLine := readExternalEgressProcessCommandLine
	oldReadCgroup := readExternalEgressProcessCgroup
	oldSignal := signalExternalEgressProcess
	t.Cleanup(func() {
		readExternalEgressProcessExecutable = oldReadlink
		readExternalEgressProcessCommandLine = oldReadCommandLine
		readExternalEgressProcessCgroup = oldReadCgroup
		signalExternalEgressProcess = oldSignal
	})

	readExternalEgressProcessExecutable = func(string) (string, error) {
		return "/usr/sbin/xl2tpd", nil
	}
	readExternalEgressProcessCommandLine = func(string) ([]byte, error) {
		return []byte("/usr/sbin/xl2tpd\x00-D\x00-c\x00/etc/xl2tpd/xl2tpd.conf\x00"), nil
	}
	cgroupReads := 0
	readExternalEgressProcessCgroup = func(string) ([]byte, error) {
		cgroupReads++
		if cgroupReads == 1 {
			return []byte("0::/system.slice/xl2tpd.service\n"), nil
		}
		return []byte("0::/system.slice/operator-l2tp.service\n"), nil
	}
	signalExternalEgressProcess = func(int, syscall.Signal) error {
		t.Fatal("process with changed cgroup ownership was signaled")
		return nil
	}

	output := `UNCONN 0 0 0.0.0.0:1701 0.0.0.0:* users:(("xl2tpd",pid=994,fd=3))`
	result, err := terminateManagedExternalEgressL2TPListener(context.Background(), output, 1701, true)
	if err != nil {
		t.Fatalf("terminateManagedExternalEgressL2TPListener: %v", err)
	}
	if result["ownership_changed"] != true || result["terminated"] == true {
		t.Fatalf("changed system service listener result = %#v", result)
	}
}

func TestCleanupExternalEgressL2TPStopsOrphanBeforeRemovingRuntime(t *testing.T) {
	oldRun := runInstallCommand
	oldReadFile := readExternalEgressManagedFile
	oldReadlink := readExternalEgressProcessExecutable
	oldReadCommandLine := readExternalEgressProcessCommandLine
	oldSignal := signalExternalEgressProcess
	oldRemove := removeExternalEgressManagedPath
	oldGlob := globExternalEgressPaths
	oldResolve := resolveExternalEgressCleanupExecutable
	t.Cleanup(func() {
		runInstallCommand = oldRun
		readExternalEgressManagedFile = oldReadFile
		readExternalEgressProcessExecutable = oldReadlink
		readExternalEgressProcessCommandLine = oldReadCommandLine
		signalExternalEgressProcess = oldSignal
		removeExternalEgressManagedPath = oldRemove
		globExternalEgressPaths = oldGlob
		resolveExternalEgressCleanupExecutable = oldResolve
	})

	processExited := false
	readExternalEgressManagedFile = func(string) ([]byte, error) {
		return []byte("4242\n"), nil
	}
	readExternalEgressProcessExecutable = func(string) (string, error) {
		if processExited {
			return "", os.ErrNotExist
		}
		return "/usr/sbin/xl2tpd", nil
	}
	readExternalEgressProcessCommandLine = func(string) ([]byte, error) {
		return []byte("/usr/sbin/xl2tpd\x00-D\x00-p\x00/run/megavpn/external-egress/38fdafed754e4e5e8bdef546e05674f3/xl2tpd.pid\x00"), nil
	}
	signalExternalEgressProcess = func(pid int, signal syscall.Signal) error {
		if pid != 4242 || signal != syscall.SIGTERM {
			t.Fatalf("unexpected managed process signal: pid=%d signal=%v", pid, signal)
		}
		processExited = true
		return nil
	}
	removedPaths := []string{}
	removeExternalEgressManagedPath = func(path string, _ bool) (bool, error) {
		removedPaths = append(removedPaths, path)
		return true, nil
	}
	globExternalEgressPaths = func(string) ([]string, error) {
		return []string{filepath.Join(externalEgressManagedRoot, "other", "ipsec.conf")}, nil
	}
	resolveExternalEgressCleanupExecutable = func(string, ...string) (string, bool) {
		return "", false
	}
	runInstallCommand = func(_ context.Context, name string, args ...string) (int, string) {
		command := name + " " + strings.Join(args, " ")
		switch {
		case name == "systemctl":
			return 0, ""
		case command == "ip rule del pref 50123 fwmark 0x4d590123 lookup 40123":
			return 2, "RTNETLINK answers: No such file or directory"
		case command == "ip route flush table 40123", command == "ip route show table 40123":
			return 2, "Error: ipv4: FIB table does not exist."
		case command == "ip rule show":
			return 0, ""
		case command == "ss -H -lunp":
			return 0, ""
		case command == "ip -o link show dev mgev0123456789":
			return 1, `Device "mgev0123456789" does not exist.`
		default:
			t.Fatalf("unexpected cleanup command: %s", command)
			return 1, "unexpected command"
		}
	}

	payload := validExternalEgressPayload("l2tp_ipsec", "server=l2tp.example.com\n")
	status, result := (&client{}).cleanupExternalEgress(context.Background(), job{Payload: map[string]any{
		"node_id": payload.NodeID, "profile_id": payload.ProfileID, "deployment_id": payload.DeploymentID,
		"protocol": payload.Protocol, "interface_name": payload.InterfaceName, "routing_table": payload.RoutingTable,
		"route_metric": payload.RouteMetric, "fwmark": payload.FWMark,
	}}, agentState{NodeID: payload.NodeID})
	if status != "succeeded" {
		t.Fatalf("cleanup status = %s, result = %#v", status, result)
	}
	processResult, _ := result["l2tp_process"].(map[string]any)
	if processResult["terminated"] != true {
		t.Fatalf("cleanup process result = %#v", processResult)
	}
	removed := strings.Join(removedPaths, "\n")
	for _, required := range []string{externalEgressManagedDir(payload), externalEgressRuntimeDir(payload)} {
		if !strings.Contains(removed, required) {
			t.Fatalf("removed paths %q do not contain %q", removed, required)
		}
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

func TestRenderExternalEgressRouteScriptWaitsForL2TPAddress(t *testing.T) {
	payload := validExternalEgressPayload("l2tp_ipsec", "server=l2tp.example.com\n")
	script := renderExternalEgressRouteScript(payload)
	if !strings.Contains(script, "ip -4 addr show dev mgev0123456789") ||
		!strings.Contains(script, "did not receive an IPv4 address") {
		t.Fatalf("L2TP route script does not wait for PPP address:\n%s", script)
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
	for _, required := range []string{"\n[Service]\n", "KillMode=control-group", "TimeoutStartSec=120s", "TimeoutStopSec=30s"} {
		if !strings.Contains(unit, required) {
			t.Fatalf("unit is missing %q:\n%s", required, unit)
		}
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
