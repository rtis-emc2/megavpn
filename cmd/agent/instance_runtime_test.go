package main

import (
	"context"
	"strings"
	"testing"

	"github.com/rtis-emc2/megavpn/internal/service/driver"
)

func TestSystemdArgsForOperation(t *testing.T) {
	t.Parallel()

	cases := map[string][]string{
		driver.OperationRestart: {"restart", "megavpn-test"},
		driver.OperationStart:   {"start", "megavpn-test"},
		driver.OperationStop:    {"stop", "megavpn-test"},
		driver.OperationEnable:  {"enable", "--now", "megavpn-test"},
		driver.OperationDisable: {"disable", "--now", "megavpn-test"},
	}
	for operation, want := range cases {
		operation, want := operation, want
		t.Run(operation, func(t *testing.T) {
			t.Parallel()
			got, err := systemdArgsForOperation(operation, "megavpn-test")
			if err != nil {
				t.Fatalf("systemdArgsForOperation returned error: %v", err)
			}
			if len(got) != len(want) {
				t.Fatalf("args len = %d, want %d: %#v", len(got), len(want), got)
			}
			for i := range got {
				if got[i] != want[i] {
					t.Fatalf("args[%d] = %q, want %q; all args=%#v", i, got[i], want[i], got)
				}
			}
		})
	}
}

func TestSystemdArgsForUnsupportedOperation(t *testing.T) {
	t.Parallel()

	if _, err := systemdArgsForOperation("destroy", "megavpn-test"); err == nil {
		t.Fatal("expected unsupported operation error")
	}
}

func TestEnsureInstanceRuntimeRecoversMissingNginxBinary(t *testing.T) {
	oldInstall := installNginxForRuntime
	oldResolve := resolveExecutableForRuntime
	defer func() {
		installNginxForRuntime = oldInstall
		resolveExecutableForRuntime = oldResolve
	}()

	installed := false
	installNginxForRuntime = func(context.Context, string, string) map[string]any {
		installed = true
		return map[string]any{"ok": false, "message": "nginx -t failed before managed config rewrite"}
	}
	resolveExecutableForRuntime = func(name string, candidates ...string) (string, bool) {
		if name == "nginx" && installed {
			return "/usr/sbin/nginx", true
		}
		return name, false
	}

	result, err := ensureInstanceRuntime(context.Background(), instanceJobPayload{ServiceCode: driver.Nginx})
	if err != nil {
		t.Fatalf("ensureInstanceRuntime() error = %v, result=%#v", err, result)
	}
	if result["binary_available_after_install"] != true {
		t.Fatalf("binary_available_after_install = %#v, result=%#v", result["binary_available_after_install"], result)
	}
	if !strings.Contains(stringify(result["warning"]), "continuing to rendered config validation") {
		t.Fatalf("expected rendered-config continuation warning, result=%#v", result)
	}
}

func TestEnsureInstanceRuntimeDoesNotAutoInstallNonNginxBinary(t *testing.T) {
	oldInstall := installNginxForRuntime
	oldResolve := resolveExecutableForRuntime
	defer func() {
		installNginxForRuntime = oldInstall
		resolveExecutableForRuntime = oldResolve
	}()

	installCalled := false
	installNginxForRuntime = func(context.Context, string, string) map[string]any {
		installCalled = true
		return map[string]any{"ok": true}
	}
	resolveExecutableForRuntime = func(name string, candidates ...string) (string, bool) {
		return name, false
	}

	result, err := ensureInstanceRuntime(context.Background(), instanceJobPayload{ServiceCode: driver.OpenVPN})
	if err == nil {
		t.Fatalf("expected missing OpenVPN binary error, result=%#v", result)
	}
	if installCalled {
		t.Fatal("nginx installer must not be called for non-nginx runtime preflight")
	}
}

func TestSystemdArgsRejectUnsafeUnit(t *testing.T) {
	t.Parallel()

	for _, unit := range []string{"../../evil.service", "evil unit.service", "/etc/systemd/system/evil.service"} {
		unit := unit
		t.Run(unit, func(t *testing.T) {
			t.Parallel()
			if _, err := systemdArgsForOperation(driver.OperationRestart, unit); err == nil {
				t.Fatalf("expected unsafe unit %q to be rejected", unit)
			}
		})
	}
}

func TestInstanceUnitPolicyAllowsDriverDefaultOnly(t *testing.T) {
	t.Parallel()

	payload := instanceJobPayload{ServiceCode: driver.WireGuard, Slug: "corp", SystemdUnit: "wg-quick@corp"}
	if !isAllowedInstanceUnit(payload, payload.SystemdUnit) {
		t.Fatalf("expected default unit to be allowed")
	}
	payload.SystemdUnit = "evil.service"
	if isAllowedInstanceUnit(payload, payload.SystemdUnit) {
		t.Fatalf("expected non-default unit to be rejected")
	}
}

func TestInstanceUnitPolicyAllowsWireGuardRuntimeInterfaceName(t *testing.T) {
	t.Parallel()

	payload := instanceJobPayload{
		ServiceCode:  driver.WireGuard,
		Slug:         "portal-example-com-wireguard",
		SystemdUnit:  "wg-quick@wg-edge",
		Spec:         map[string]any{"interface_name": "wg-edge"},
		InstanceID:   "instance-1",
		EndpointPort: 51820,
	}
	if !isAllowedInstanceUnit(payload, payload.SystemdUnit) {
		t.Fatalf("expected explicit wireguard runtime interface unit to be allowed")
	}
	payload.SystemdUnit = "wg-quick@portal-example-com-wireguard"
	if isAllowedInstanceUnit(payload, payload.SystemdUnit) {
		t.Fatalf("expected long legacy wireguard unit to be rejected for systemctl execution")
	}
}

func TestInstanceOwnedRuntimeUnitPolicy(t *testing.T) {
	t.Parallel()

	owned := []instanceJobPayload{
		{ServiceCode: driver.WireGuard, Slug: "corp", SystemdUnit: "wg-quick@corp"},
		{ServiceCode: driver.OpenVPN, Slug: "corp", SystemdUnit: "openvpn-server@corp"},
		{ServiceCode: driver.XrayCore, Slug: "edge", SystemdUnit: "megavpn-xray-edge"},
		{ServiceCode: driver.Shadowsocks, Slug: "edge", SystemdUnit: "megavpn-shadowsocks-edge"},
	}
	for _, payload := range owned {
		payload := payload
		t.Run(payload.ServiceCode, func(t *testing.T) {
			t.Parallel()
			if !isInstanceOwnedRuntimeUnit(payload, payload.SystemdUnit) {
				t.Fatalf("expected %s unit %s to be instance-owned", payload.ServiceCode, payload.SystemdUnit)
			}
		})
	}

	shared := []instanceJobPayload{
		{ServiceCode: driver.Nginx, Slug: "edge", SystemdUnit: "nginx"},
		{ServiceCode: driver.IPSec, Slug: "edge", SystemdUnit: "strongswan-starter"},
		{ServiceCode: driver.XL2TPD, Slug: "edge", SystemdUnit: "xl2tpd"},
	}
	for _, payload := range shared {
		payload := payload
		t.Run(payload.ServiceCode, func(t *testing.T) {
			t.Parallel()
			if isInstanceOwnedRuntimeUnit(payload, payload.SystemdUnit) {
				t.Fatalf("expected %s unit %s to be treated as shared", payload.ServiceCode, payload.SystemdUnit)
			}
		})
	}
}

func TestManagedFilePolicyRejectsArbitraryServiceUnit(t *testing.T) {
	t.Parallel()

	payload := instanceJobPayload{ServiceCode: driver.WireGuard, Slug: "corp", SystemdUnit: "wg-quick@corp"}
	file := managedFileSpec{Path: "/etc/systemd/system/evil.service", Content: "[Service]\nExecStart=/bin/true\n", Mode: "0644"}
	if err := validateManagedFilePolicy(payload, file); err == nil {
		t.Fatal("expected arbitrary service unit path to be rejected")
	}
}

func TestManagedFilePolicyRejectsArbitraryContentForDefaultServiceUnit(t *testing.T) {
	t.Parallel()

	payload := instanceJobPayload{ServiceCode: driver.XrayCore, Slug: "edge", SystemdUnit: "megavpn-xray-edge"}
	file := managedFileSpec{Path: "/etc/systemd/system/megavpn-xray-edge.service", Content: "[Service]\nExecStart=/bin/sh -c 'id'\n", Mode: "0644"}
	if err := validateManagedFilePolicy(payload, file); err == nil {
		t.Fatal("expected arbitrary service unit content to be rejected")
	}
}

func TestManagedFilePolicyAllowsShadowsocksConfigFlag(t *testing.T) {
	t.Parallel()

	payload := instanceJobPayload{ServiceCode: driver.Shadowsocks, Slug: "edge", SystemdUnit: "megavpn-shadowsocks-edge"}
	file := managedFileSpec{Path: "/etc/systemd/system/megavpn-shadowsocks-edge.service", Content: "[Service]\nExecStart=/usr/bin/env ss-server -c /etc/shadowsocks-libev/edge.json\n", Mode: "0644"}
	if err := validateManagedFilePolicy(payload, file); err != nil {
		t.Fatalf("expected shadowsocks -c unit to be allowed: %v", err)
	}
}

func TestManagedFilePolicyRejectsExtraExecDirective(t *testing.T) {
	t.Parallel()

	payload := instanceJobPayload{ServiceCode: driver.XrayCore, Slug: "edge", SystemdUnit: "megavpn-xray-edge"}
	file := managedFileSpec{
		Path:    "/etc/systemd/system/megavpn-xray-edge.service",
		Content: "[Service]\nExecStart=/usr/bin/env xray run -config /usr/local/etc/xray/edge.json\nExecStartPre=/usr/bin/id\n",
		Mode:    "0644",
	}
	if err := validateManagedFilePolicy(payload, file); err == nil {
		t.Fatal("expected additional ExecStartPre directive to be rejected")
	}
}

func TestManagedFilePolicyRejectsHTTPProxyUnexpectedExec(t *testing.T) {
	t.Parallel()

	payload := instanceJobPayload{ServiceCode: driver.HTTPProxy, Slug: "edge", SystemdUnit: "megavpn-http-proxy-edge"}
	file := managedFileSpec{
		Path:    "/etc/systemd/system/megavpn-http-proxy-edge.service",
		Content: "[Service]\nExecStart=/usr/bin/env squid -f /etc/squid/edge.conf -N\nExecReload=/usr/bin/env squid -k reconfigure -f /etc/squid/edge.conf\nExecStop=/usr/bin/env squid -k shutdown -f /etc/squid/edge.conf\nExecStartPost=/usr/bin/id\n",
		Mode:    "0644",
	}
	if err := validateManagedFilePolicy(payload, file); err == nil {
		t.Fatal("expected additional ExecStartPost directive to be rejected")
	}
}

func TestManagedDeletePathPolicyAllowsMalformedGeneratedUnitPath(t *testing.T) {
	t.Parallel()

	payload := instanceJobPayload{ServiceCode: driver.XrayCore, Slug: "edge", SystemdUnit: "megavpn-xray-edge"}
	if err := validateManagedDeletePathPolicy(payload, "/etc/systemd/system/megavpn-xray-edge.service"); err != nil {
		t.Fatalf("expected delete path policy to allow cleanup of generated unit path: %v", err)
	}
	if err := validateManagedDeletePathPolicy(payload, "/etc/systemd/system/evil.service"); err == nil {
		t.Fatal("expected arbitrary service unit path to remain rejected for delete")
	}
}

func TestLegacyManagedDeletePathsIncludesOldWireGuardSlugConfig(t *testing.T) {
	t.Parallel()

	payload := instanceJobPayload{ServiceCode: driver.WireGuard, Slug: "portal-example-com-wireguard", SystemdUnit: "wg-quick@portal-example-com-wireguard"}
	paths := legacyManagedDeletePaths(payload)
	if len(paths) != 1 || paths[0] != "/etc/wireguard/portal-example-com-wireguard.conf" {
		t.Fatalf("legacy paths = %#v", paths)
	}
}

func TestManagedFilePolicyRejectsPathOutsideDriverRoots(t *testing.T) {
	t.Parallel()

	payload := instanceJobPayload{ServiceCode: driver.WireGuard, Slug: "corp", SystemdUnit: "wg-quick@corp"}
	file := managedFileSpec{Path: "/tmp/wg.conf", Content: "[Interface]\n", Mode: "0600"}
	if err := validateManagedFilePolicy(payload, file); err == nil {
		t.Fatal("expected path outside wireguard roots to be rejected")
	}
}

func TestManagedFilePolicyAllowsDriverConfigPath(t *testing.T) {
	t.Parallel()

	payload := instanceJobPayload{ServiceCode: driver.WireGuard, Slug: "corp", SystemdUnit: "wg-quick@corp"}
	file := managedFileSpec{Path: "/etc/wireguard/corp.conf", Content: "[Interface]\n", Mode: "0600"}
	if err := validateManagedFilePolicy(payload, file); err != nil {
		t.Fatalf("expected driver config path to be allowed: %v", err)
	}
}
