package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/rtis-emc2/megavpn/internal/service/driver"
)

func (c client) installCapability(ctx context.Context, j job, st agentState) (string, map[string]any) {
	serviceCode := normalizeCapabilityCode(stringFromMap(j.Payload, "service_code"))
	strategy := stringFromMap(j.Payload, "strategy")
	channel := stringFromMap(j.Payload, "channel")
	if channel == "" {
		channel = "latest"
	}
	switch serviceCode {
	case "nginx":
		if strategy == "" {
			strategy = "nginx_org_repo"
		}
		if strategy == "manual_present" {
			res := verifyNginx(ctx)
			inv := collectInventory()
			_ = c.submitInventory(ctx, st.NodeID, "inventory", inv)
			res["service_code"] = serviceCode
			res["strategy"] = strategy
			res["agent_version"] = appVersion
			if res["ok"] == true {
				return "succeeded", normalizeInstallResult(res, serviceCode, strategy, time.Now())
			}
			return "failed", normalizeInstallResult(res, serviceCode, strategy, time.Now())
		}
		if strategy != "nginx_org_repo" && strategy != "ubuntu_repo" {
			return "failed", normalizeInstallResult(map[string]any{"ok": false, "message": "unsupported nginx install strategy", "strategy": strategy}, serviceCode, strategy, time.Now())
		}
		started := time.Now()
		res := installNginx(ctx, strategy, channel)
		inv := collectInventory()
		_ = c.submitInventory(ctx, st.NodeID, "inventory", inv)
		res = normalizeInstallResult(res, serviceCode, stringFromMap(res, "effective_strategy"), started)
		if res["strategy"] == "" {
			res["strategy"] = strategy
		}
		res["requested_strategy"] = strategy
		res["agent_version"] = appVersion
		if res["ok"] == true {
			return "succeeded", res
		}
		return "failed", res
	case "xray-core":
		if strategy == "" {
			strategy = "xtls_install_release"
		}
		if strategy == "binary_repository" {
			started := time.Now()
			res := c.installBinaryRepositoryCapability(ctx, j, serviceCode)
			inv := collectInventory()
			_ = c.submitInventory(ctx, st.NodeID, "inventory", inv)
			res = normalizeInstallResult(res, serviceCode, strategy, started)
			res["agent_version"] = appVersion
			if res["ok"] == true {
				return "succeeded", res
			}
			return "failed", res
		}
		if strategy == "manual_present" {
			started := time.Now()
			res := verifyXrayCore(ctx)
			inv := collectInventory()
			_ = c.submitInventory(ctx, st.NodeID, "inventory", inv)
			res = normalizeInstallResult(res, serviceCode, strategy, started)
			res["agent_version"] = appVersion
			if res["ok"] == true {
				return "succeeded", res
			}
			return "failed", res
		}
		if strategy != "xtls_install_release" {
			return "failed", normalizeInstallResult(map[string]any{"ok": false, "message": "unsupported xray-core install strategy", "strategy": strategy}, serviceCode, strategy, time.Now())
		}
		started := time.Now()
		res := installXrayCore(ctx, channel)
		inv := collectInventory()
		_ = c.submitInventory(ctx, st.NodeID, "inventory", inv)
		res = normalizeInstallResult(res, serviceCode, strategy, started)
		res["agent_version"] = appVersion
		if res["ok"] == true {
			return "succeeded", res
		}
		return "failed", res
	case "openvpn":
		if strategy == "" {
			strategy = "ubuntu_repo"
		}
		if strategy == "manual_present" {
			started := time.Now()
			res := verifyOpenVPN(ctx)
			inv := collectInventory()
			_ = c.submitInventory(ctx, st.NodeID, "inventory", inv)
			res = normalizeInstallResult(res, serviceCode, strategy, started)
			res["agent_version"] = appVersion
			if res["ok"] == true {
				return "succeeded", res
			}
			return "failed", res
		}
		if strategy != "ubuntu_repo" {
			return "failed", normalizeInstallResult(map[string]any{"ok": false, "message": "unsupported openvpn install strategy", "strategy": strategy}, serviceCode, strategy, time.Now())
		}
		started := time.Now()
		res := installUbuntuPackageCapability(ctx, "openvpn", "openvpn", "openvpn", []string{"openvpn"})
		inv := collectInventory()
		_ = c.submitInventory(ctx, st.NodeID, "inventory", inv)
		res = normalizeInstallResult(res, serviceCode, strategy, started)
		res["agent_version"] = appVersion
		if res["ok"] == true {
			return "succeeded", res
		}
		return "failed", res
	case "wireguard":
		if strategy == "" {
			strategy = "ubuntu_repo"
		}
		if strategy == "manual_present" {
			started := time.Now()
			res := verifyWireGuard(ctx)
			inv := collectInventory()
			_ = c.submitInventory(ctx, st.NodeID, "inventory", inv)
			res = normalizeInstallResult(res, serviceCode, strategy, started)
			res["agent_version"] = appVersion
			if res["ok"] == true {
				return "succeeded", res
			}
			return "failed", res
		}
		if strategy != "ubuntu_repo" {
			return "failed", normalizeInstallResult(map[string]any{"ok": false, "message": "unsupported wireguard install strategy", "strategy": strategy}, serviceCode, strategy, time.Now())
		}
		started := time.Now()
		res := installUbuntuPackageCapability(ctx, "wireguard", "wireguard-tools", "wireguard-tools", nil)
		inv := collectInventory()
		_ = c.submitInventory(ctx, st.NodeID, "inventory", inv)
		res = normalizeInstallResult(res, serviceCode, strategy, started)
		res["agent_version"] = appVersion
		if res["ok"] == true {
			return "succeeded", res
		}
		return "failed", res
	case "ipsec":
		if strategy == "" {
			strategy = "ubuntu_repo"
		}
		if strategy == "manual_present" {
			started := time.Now()
			res := verifyIPsec(ctx)
			inv := collectInventory()
			_ = c.submitInventory(ctx, st.NodeID, "inventory", inv)
			res = normalizeInstallResult(res, serviceCode, strategy, started)
			res["agent_version"] = appVersion
			if res["ok"] == true {
				return "succeeded", res
			}
			return "failed", res
		}
		if strategy != "ubuntu_repo" {
			return "failed", normalizeInstallResult(map[string]any{"ok": false, "message": "unsupported ipsec install strategy", "strategy": strategy}, serviceCode, strategy, time.Now())
		}
		started := time.Now()
		res := installUbuntuPackageCapability(ctx, "ipsec", "strongswan", "strongswan", []string{"strongswan-starter", "strongswan"})
		inv := collectInventory()
		_ = c.submitInventory(ctx, st.NodeID, "inventory", inv)
		res = normalizeInstallResult(res, serviceCode, strategy, started)
		res["agent_version"] = appVersion
		if res["ok"] == true {
			return "succeeded", res
		}
		return "failed", res
	case "http_proxy":
		if strategy == "" {
			strategy = "ubuntu_repo"
		}
		if strategy == "manual_present" {
			started := time.Now()
			res := verifyHTTPProxy(ctx)
			inv := collectInventory()
			_ = c.submitInventory(ctx, st.NodeID, "inventory", inv)
			res = normalizeInstallResult(res, serviceCode, strategy, started)
			res["agent_version"] = appVersion
			if res["ok"] == true {
				return "succeeded", res
			}
			return "failed", res
		}
		if strategy != "ubuntu_repo" {
			return "failed", normalizeInstallResult(map[string]any{"ok": false, "message": "unsupported http_proxy install strategy", "strategy": strategy}, serviceCode, strategy, time.Now())
		}
		started := time.Now()
		res := installUbuntuPackageCapability(ctx, "http_proxy", "squid", "squid", []string{"squid"})
		inv := collectInventory()
		_ = c.submitInventory(ctx, st.NodeID, "inventory", inv)
		res = normalizeInstallResult(res, serviceCode, strategy, started)
		res["agent_version"] = appVersion
		if res["ok"] == true {
			return "succeeded", res
		}
		return "failed", res
	case "xl2tpd":
		if strategy == "" {
			strategy = "ubuntu_repo"
		}
		if strategy == "manual_present" {
			started := time.Now()
			res := verifyXL2TPD(ctx)
			inv := collectInventory()
			_ = c.submitInventory(ctx, st.NodeID, "inventory", inv)
			res = normalizeInstallResult(res, serviceCode, strategy, started)
			res["agent_version"] = appVersion
			if res["ok"] == true {
				return "succeeded", res
			}
			return "failed", res
		}
		if strategy != "ubuntu_repo" {
			return "failed", normalizeInstallResult(map[string]any{"ok": false, "message": "unsupported xl2tpd install strategy", "strategy": strategy}, serviceCode, strategy, time.Now())
		}
		started := time.Now()
		res := installUbuntuPackageCapability(ctx, "xl2tpd", "xl2tpd", "xl2tpd", []string{"xl2tpd"})
		inv := collectInventory()
		_ = c.submitInventory(ctx, st.NodeID, "inventory", inv)
		res = normalizeInstallResult(res, serviceCode, strategy, started)
		res["agent_version"] = appVersion
		if res["ok"] == true {
			return "succeeded", res
		}
		return "failed", res
	case "shadowsocks":
		if strategy == "" {
			strategy = "ubuntu_repo"
		}
		if strategy == "binary_repository" {
			started := time.Now()
			res := c.installBinaryRepositoryCapability(ctx, j, serviceCode)
			inv := collectInventory()
			_ = c.submitInventory(ctx, st.NodeID, "inventory", inv)
			res = normalizeInstallResult(res, serviceCode, strategy, started)
			res["agent_version"] = appVersion
			if res["ok"] == true {
				return "succeeded", res
			}
			return "failed", res
		}
		if strategy == "manual_present" {
			started := time.Now()
			res := verifyShadowsocks(ctx)
			inv := collectInventory()
			_ = c.submitInventory(ctx, st.NodeID, "inventory", inv)
			res = normalizeInstallResult(res, serviceCode, strategy, started)
			res["agent_version"] = appVersion
			if res["ok"] == true {
				return "succeeded", res
			}
			return "failed", res
		}
		if strategy != "ubuntu_repo" {
			return "failed", normalizeInstallResult(map[string]any{"ok": false, "message": "unsupported shadowsocks install strategy", "strategy": strategy}, serviceCode, strategy, time.Now())
		}
		started := time.Now()
		res := installUbuntuPackageCapability(ctx, "shadowsocks", "shadowsocks-libev", "shadowsocks-libev", []string{"shadowsocks-libev"})
		inv := collectInventory()
		_ = c.submitInventory(ctx, st.NodeID, "inventory", inv)
		res = normalizeInstallResult(res, serviceCode, strategy, started)
		res["agent_version"] = appVersion
		if res["ok"] == true {
			return "succeeded", res
		}
		return "failed", res
	default:
		return "failed", map[string]any{"error": "capability is not whitelisted for install", "service_code": serviceCode}
	}
}

func (c client) verifyCapability(ctx context.Context, j job, st agentState) (string, map[string]any) {
	serviceCode := normalizeCapabilityCode(stringFromMap(j.Payload, "service_code"))
	var res map[string]any
	switch serviceCode {
	case "nginx":
		res = verifyNginx(ctx)
	case "xray-core":
		res = verifyXrayCore(ctx)
	case "openvpn":
		res = verifyOpenVPN(ctx)
	case "wireguard":
		res = verifyWireGuard(ctx)
	case "ipsec":
		res = verifyIPsec(ctx)
	case "http_proxy":
		res = verifyHTTPProxy(ctx)
	case "xl2tpd":
		res = verifyXL2TPD(ctx)
	case "shadowsocks":
		res = verifyShadowsocks(ctx)
	default:
		return "failed", map[string]any{"error": "capability is not whitelisted for verify", "service_code": serviceCode}
	}
	inv := collectInventory()
	_ = c.submitInventory(ctx, st.NodeID, "inventory", inv)
	res["service_code"] = serviceCode
	res["agent_version"] = appVersion
	if res["ok"] == true {
		return "succeeded", res
	}
	return "failed", res
}

func normalizeInstallResult(res map[string]any, serviceCode, strategy string, started time.Time) map[string]any {
	if res == nil {
		res = map[string]any{}
	}
	res["service_code"] = serviceCode
	if _, ok := res["strategy"]; !ok {
		res["strategy"] = strategy
	}
	if _, ok := res["effective_strategy"]; !ok {
		res["effective_strategy"] = strategy
	}
	if _, ok := res["duration_ms"]; !ok {
		res["duration_ms"] = time.Since(started).Milliseconds()
	}
	if _, ok := res["status"]; !ok {
		if res["ok"] == true {
			res["status"] = "succeeded"
		} else {
			res["status"] = "failed"
		}
	}
	if _, ok := res["message"]; !ok {
		if res["ok"] == true {
			res["message"] = "capability install succeeded"
		} else {
			res["message"] = "capability install failed"
		}
	}
	return res
}

func installNginx(ctx context.Context, strategy, channel string) map[string]any {
	if strategy == "ubuntu_repo" {
		res := installNginxUbuntuRepo(ctx, "explicit ubuntu_repo strategy")
		res["requested_strategy"] = strategy
		res["effective_strategy"] = "ubuntu_repo"
		return res
	}
	if reason := nginxOrgPreflightBlockReason(); reason != "" {
		res := installNginxUbuntuRepo(ctx, reason)
		res["requested_strategy"] = "nginx_org_repo"
		res["effective_strategy"] = "ubuntu_repo"
		res["fallback_reason"] = reason
		return res
	}
	res := installNginxOrg(ctx, channel)
	res["requested_strategy"] = "nginx_org_repo"
	res["effective_strategy"] = "nginx_org_repo"
	if res["ok"] == false && strings.Contains(strings.ToLower(stringify(res["verify_output"])+" "+stringify(res["message"])+" "+stringify(res["version"])), "isa level") {
		fallback := installNginxUbuntuRepo(ctx, "nginx.org binary failed with CPU ISA compatibility error")
		fallback["requested_strategy"] = "nginx_org_repo"
		fallback["effective_strategy"] = "ubuntu_repo"
		fallback["fallback_reason"] = "nginx.org binary failed with CPU ISA compatibility error"
		fallback["primary_result"] = res
		return fallback
	}
	return res
}

func installNginxUbuntuRepo(ctx context.Context, reason string) map[string]any {
	steps := []map[string]any{}
	run := func(name string, args ...string) bool {
		code, out := runInstallCommand(ctx, name, args...)
		steps = append(steps, map[string]any{"command": append([]string{name}, args...), "exit_code": code, "output": truncate(out, 4000)})
		return code == 0
	}
	if os.Geteuid() != 0 {
		return map[string]any{"ok": false, "message": "nginx install requires root", "steps": steps, "effective_strategy": "ubuntu_repo", "fallback_reason": reason}
	}
	_ = os.Remove("/etc/apt/sources.list.d/nginx.list")
	_ = os.Remove("/etc/apt/preferences.d/99nginx")
	_ = os.Remove("/usr/share/keyrings/nginx-archive-keyring.gpg")
	_ = run("env", "DEBIAN_FRONTEND=noninteractive", "apt-get", "purge", "-y", "nginx")
	_ = run("systemctl", "daemon-reload")
	if !run("apt-get", "update") {
		return map[string]any{"ok": false, "message": "apt update failed before ubuntu nginx install", "steps": steps, "effective_strategy": "ubuntu_repo", "fallback_reason": reason}
	}
	policyCode, policyOut := runInstallCommand(ctx, "apt-cache", "policy", "nginx")
	steps = append(steps, map[string]any{"command": []string{"apt-cache", "policy", "nginx"}, "exit_code": policyCode, "output": truncate(policyOut, 4000)})
	if !aptPolicyHasCandidate(policyOut) {
		return map[string]any{"ok": false, "message": "ubuntu nginx package is not available after removing nginx.org repository", "steps": steps, "effective_strategy": "ubuntu_repo", "fallback_reason": reason, "apt_policy": truncate(policyOut, 4000)}
	}
	if aptPolicyMentionsNginxOrg(policyOut) {
		return map[string]any{"ok": false, "message": "ubuntu_repo fallback still resolves to nginx.org package; refusing unsafe reinstall", "steps": steps, "effective_strategy": "ubuntu_repo", "fallback_reason": reason, "apt_policy": truncate(policyOut, 4000)}
	}
	if !run("env", "DEBIAN_FRONTEND=noninteractive", "apt-get", "install", "-y", "nginx") {
		return map[string]any{"ok": false, "message": "ubuntu nginx package install failed", "steps": steps, "effective_strategy": "ubuntu_repo", "fallback_reason": reason}
	}
	_ = run("systemctl", "enable", "--now", "nginx")
	verify := verifyNginx(ctx)
	verify["steps"] = steps
	verify["effective_strategy"] = "ubuntu_repo"
	if reason != "" {
		verify["fallback_reason"] = reason
	}
	return verify
}

func nginxOrgPreflightBlockReason() string {
	osInfo := readOSRelease()
	codename := strings.ToLower(stringify(osInfo["version_codename"]))
	versionID := strings.ToLower(stringify(osInfo["version_id"]))
	if codename == "resolute" || versionID == "26.04" {
		return "nginx.org packages for Ubuntu 26.04/resolute are blocked on this node until CPU ISA compatibility is verified"
	}
	if !supportsX8664V2() {
		return "nginx.org packages may require a higher x86-64 ISA level than this CPU exposes"
	}
	return ""
}

func supportsX8664V2() bool {
	if runtime.GOARCH != "amd64" {
		return true
	}
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return true
	}
	flags := strings.ToLower(string(data))
	required := []string{"cx16", "lahf_lm", "popcnt", "sse4_1", "sse4_2", "ssse3"}
	for _, f := range required {
		if !strings.Contains(flags, f) {
			return false
		}
	}
	return true
}

func aptPolicyHasCandidate(policy string) bool {
	for _, line := range strings.Split(policy, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Candidate:") {
			candidate := strings.TrimSpace(strings.TrimPrefix(line, "Candidate:"))
			return candidate != "" && candidate != "(none)"
		}
	}
	return false
}

func aptPolicyMentionsNginxOrg(policy string) bool {
	return strings.Contains(strings.ToLower(policy), "nginx.org")
}

func installNginxOrg(ctx context.Context, channel string) map[string]any {
	steps := []map[string]any{}
	run := func(name string, args ...string) bool {
		code, out := runInstallCommand(ctx, name, args...)
		steps = append(steps, map[string]any{"command": append([]string{name}, args...), "exit_code": code, "output": truncate(out, 4000)})
		return code == 0
	}
	if os.Geteuid() != 0 {
		return map[string]any{"ok": false, "message": "nginx install requires root", "steps": steps}
	}
	if !run("apt-get", "update") {
		return map[string]any{"ok": false, "message": "apt update failed before nginx install", "steps": steps}
	}
	if !run("env", "DEBIAN_FRONTEND=noninteractive", "apt-get", "install", "-y", "curl", "gnupg2", "ca-certificates", "lsb-release", "ubuntu-keyring") {
		return map[string]any{"ok": false, "message": "nginx prerequisites install failed", "steps": steps}
	}
	if !run("bash", "-c", "curl -fsSL https://nginx.org/keys/nginx_signing.key | gpg --dearmor --yes --batch -o /usr/share/keyrings/nginx-archive-keyring.gpg") {
		return map[string]any{"ok": false, "message": "nginx signing key import failed", "steps": steps}
	}
	repo := "https://nginx.org/packages/ubuntu"
	if channel == "mainline" {
		repo = "https://nginx.org/packages/mainline/ubuntu"
	}
	cmd := "echo 'deb [signed-by=/usr/share/keyrings/nginx-archive-keyring.gpg] " + repo + " '$(lsb_release -cs)' nginx' > /etc/apt/sources.list.d/nginx.list"
	if !run("bash", "-c", cmd) {
		return map[string]any{"ok": false, "message": "nginx repository setup failed", "steps": steps}
	}
	if !run("bash", "-c", "printf 'Package: *\nPin: origin nginx.org\nPin: release o=nginx\nPin-Priority: 900\n' > /etc/apt/preferences.d/99nginx") {
		return map[string]any{"ok": false, "message": "nginx apt pinning setup failed", "steps": steps}
	}
	if !run("apt-get", "update") {
		return map[string]any{"ok": false, "message": "apt update failed after nginx repo setup", "steps": steps}
	}
	if !run("env", "DEBIAN_FRONTEND=noninteractive", "apt-get", "install", "-y", "nginx") {
		return map[string]any{"ok": false, "message": "nginx package install failed", "steps": steps}
	}
	_ = run("systemctl", "enable", "--now", "nginx")
	verify := verifyNginx(ctx)
	verify["steps"] = steps
	return verify
}

func installXrayCore(ctx context.Context, channel string) map[string]any {
	_ = channel
	steps := []map[string]any{}
	run := func(name string, args ...string) bool {
		code, out := runInstallCommand(ctx, name, args...)
		steps = append(steps, map[string]any{"command": append([]string{name}, args...), "exit_code": code, "output": truncate(out, 4000)})
		return code == 0
	}
	if os.Geteuid() != 0 {
		return map[string]any{"ok": false, "message": "xray-core install requires root", "steps": steps}
	}
	expectedSHA256 := strings.ToLower(strings.TrimSpace(os.Getenv("MEGAVPN_XRAY_INSTALL_SCRIPT_SHA256")))
	if expectedSHA256 == "" {
		return map[string]any{"ok": false, "message": "xray-core install requires MEGAVPN_XRAY_INSTALL_SCRIPT_SHA256 for pinned supply-chain verification", "steps": steps}
	}
	installerPath := "/tmp/megavpn-xray-install-release.sh"
	if !run("curl", "-fsSL", "https://github.com/XTLS/Xray-install/raw/main/install-release.sh", "-o", installerPath) {
		return map[string]any{"ok": false, "message": "xray-core install script download failed", "steps": steps}
	}
	if err := verifyFileSHA256(installerPath, expectedSHA256); err != nil {
		return map[string]any{"ok": false, "message": "xray-core install script checksum verification failed", "error": err.Error(), "steps": steps}
	}
	if !run("bash", installerPath, "install") {
		return map[string]any{"ok": false, "message": "xray-core install script failed", "steps": steps}
	}
	verify := verifyXrayCore(ctx)
	verify["steps"] = steps
	return verify
}

func verifyFileSHA256(path, expectedHex string) error {
	expectedHex = strings.ToLower(strings.TrimSpace(expectedHex))
	if len(expectedHex) != 64 {
		return fmt.Errorf("expected sha256 must be 64 hex characters")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(b)
	got := hex.EncodeToString(sum[:])
	if got != expectedHex {
		return fmt.Errorf("sha256 mismatch: got %s", got)
	}
	return nil
}

func installUbuntuPackageCapability(ctx context.Context, capabilityCode, packageName, verifyHint string, enableUnits []string) map[string]any {
	steps := []map[string]any{}
	run := func(name string, args ...string) bool {
		code, out := runInstallCommand(ctx, name, args...)
		steps = append(steps, map[string]any{"command": append([]string{name}, args...), "exit_code": code, "output": truncate(out, 4000)})
		return code == 0
	}
	if os.Geteuid() != 0 {
		return map[string]any{"ok": false, "message": capabilityCode + " install requires root", "steps": steps}
	}
	if !run("apt-get", "update") {
		return map[string]any{"ok": false, "message": "apt update failed before " + capabilityCode + " install", "steps": steps}
	}
	if !run("env", "DEBIAN_FRONTEND=noninteractive", "apt-get", "install", "-y", packageName) {
		return map[string]any{"ok": false, "message": capabilityCode + " package install failed", "steps": steps, "package": packageName}
	}
	for _, unit := range enableUnits {
		_ = run("systemctl", "enable", "--now", unit)
	}
	var verify map[string]any
	switch capabilityCode {
	case "openvpn":
		verify = verifyOpenVPN(ctx)
	case "wireguard":
		verify = verifyWireGuard(ctx)
	case "ipsec":
		verify = verifyIPsec(ctx)
	case "http_proxy":
		verify = verifyHTTPProxy(ctx)
	case "xl2tpd":
		verify = verifyXL2TPD(ctx)
	case "shadowsocks":
		verify = verifyShadowsocks(ctx)
	default:
		verify = map[string]any{"ok": false, "message": "no verify handler for " + verifyHint}
	}
	verify["steps"] = steps
	verify["package"] = packageName
	return verify
}

func verifyNginx(ctx context.Context) map[string]any {
	version := strings.TrimSpace(firstLine(runCombinedOutput("nginx", "-v")))
	if version == "" {
		return map[string]any{"ok": false, "message": "nginx binary not found or version unavailable"}
	}
	code, out := runInstallCommand(ctx, "nginx", "-t")
	active := strings.TrimSpace(runOutput("systemctl", "is-active", "nginx"))
	return map[string]any{"ok": code == 0, "message": "nginx capability verified", "version": version, "verify_output": truncate(out, 2000), "systemd_unit": "nginx", "active_state": normalizeSystemctlState(active)}
}

func verifyXrayCore(ctx context.Context) map[string]any {
	xrayPath := xrayExecutablePath()
	version := strings.TrimSpace(firstLine(runOutput(xrayPath, "version")))
	if version == "" {
		version = strings.TrimSpace(firstLine(runOutput(xrayPath, "--version")))
	}
	if version == "" {
		return map[string]any{"ok": false, "message": "xray-core binary not found or version unavailable"}
	}
	active := strings.TrimSpace(runOutput("systemctl", "is-active", "xray"))
	return map[string]any{"ok": true, "message": "xray-core capability verified", "version": version, "binary_path": xrayPath, "systemd_unit": "xray", "active_state": normalizeSystemctlState(active)}
}

func verifyOpenVPN(ctx context.Context) map[string]any {
	version := strings.TrimSpace(firstLine(runOutput("openvpn", "--version")))
	if version == "" {
		return map[string]any{"ok": false, "message": "openvpn binary not found or version unavailable"}
	}
	active := firstKnownActiveState("openvpn", "openvpn-server@server", "megavpn-openvpn")
	return map[string]any{"ok": true, "message": "openvpn capability verified", "version": version, "systemd_unit": firstKnownUnit("openvpn", "openvpn-server@server", "megavpn-openvpn"), "active_state": active}
}

func verifyWireGuard(ctx context.Context) map[string]any {
	version := strings.TrimSpace(firstLine(runCombinedOutput("wg", "--version")))
	if version == "" {
		version = strings.TrimSpace(firstLine(runCombinedOutput("wg-quick", "--version")))
	}
	if version == "" {
		return map[string]any{"ok": false, "message": "wireguard binary not found or version unavailable"}
	}
	active := firstKnownActiveState("wg-quick@wg0")
	return map[string]any{"ok": true, "message": "wireguard capability verified", "version": version, "systemd_unit": firstKnownUnit("wg-quick@wg0"), "active_state": active}
}

func verifyIPsec(ctx context.Context) map[string]any {
	version := strings.TrimSpace(firstLine(runCombinedOutput("ipsec", "--version")))
	if version == "" {
		version = strings.TrimSpace(firstLine(runOutput("strongswan", "--version")))
	}
	if version == "" {
		return map[string]any{"ok": false, "message": "ipsec/strongswan binary not found or version unavailable"}
	}
	active := firstKnownActiveState("strongswan", "strongswan-starter", "ipsec")
	return map[string]any{"ok": true, "message": "ipsec capability verified", "version": version, "systemd_unit": firstKnownUnit("strongswan", "strongswan-starter", "ipsec"), "active_state": active}
}

func verifyXL2TPD(ctx context.Context) map[string]any {
	version := strings.TrimSpace(firstLine(runCombinedOutput("xl2tpd", "-v")))
	if version == "" {
		path := strings.TrimSpace(runOutput("which", "xl2tpd"))
		if path == "" {
			return map[string]any{"ok": false, "message": "xl2tpd binary not found or version unavailable"}
		}
		version = path
	}
	active := firstKnownActiveState("xl2tpd")
	return map[string]any{"ok": true, "message": "xl2tpd capability verified", "version": version, "systemd_unit": firstKnownUnit("xl2tpd"), "active_state": active}
}

func verifyHTTPProxy(ctx context.Context) map[string]any {
	version := strings.TrimSpace(firstLine(runCombinedOutput("squid", "-v")))
	if version == "" {
		return map[string]any{"ok": false, "message": "squid binary not found or version unavailable"}
	}
	active := firstKnownActiveState("squid")
	return map[string]any{"ok": true, "message": "http proxy capability verified", "version": version, "systemd_unit": firstKnownUnit("squid"), "active_state": active}
}

func verifyShadowsocks(ctx context.Context) map[string]any {
	version := strings.TrimSpace(firstLine(runCombinedOutput("ss-server", "-h")))
	if version == "" {
		return map[string]any{"ok": false, "message": "shadowsocks binary not found or version unavailable"}
	}
	active := firstKnownActiveState("shadowsocks-libev", "shadowsocks")
	return map[string]any{"ok": true, "message": "shadowsocks capability verified", "version": version, "systemd_unit": firstKnownUnit("shadowsocks-libev", "shadowsocks"), "active_state": active}
}

func normalizeCapabilityCode(code string) string {
	return driver.NormalizeCode(code)
}
