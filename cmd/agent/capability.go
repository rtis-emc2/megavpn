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
		res := installUbuntuPackageCapability(ctx, "shadowsocks", "shadowsocks-libev", "shadowsocks-libev", ubuntuPackageEnableUnits("shadowsocks"))
		if res["ok"] != true && hasBinaryRepositoryInstallPayload(j, "binary_repository_fallback") {
			ubuntuResult := cloneInstallResult(res)
			fallback := c.installBinaryRepositoryCapabilityFromPayload(ctx, j, serviceCode, "binary_repository_fallback")
			fallback["fallback_from"] = "ubuntu_repo"
			fallback["ubuntu_repo_result"] = ubuntuResult
			fallback["message"] = firstNonEmptyString(stringFromMap(fallback, "message"), "shadowsocks binary repository fallback completed")
			inv := collectInventory()
			_ = c.submitInventory(ctx, st.NodeID, "inventory", inv)
			fallback = normalizeInstallResult(fallback, serviceCode, "binary_repository", started)
			fallback["requested_strategy"] = strategy
			fallback["agent_version"] = appVersion
			if fallback["ok"] == true {
				return "succeeded", fallback
			}
			return "failed", fallback
		}
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

func hasBinaryRepositoryInstallPayload(j job, key string) bool {
	repo, ok := j.Payload[strings.TrimSpace(key)].(map[string]any)
	return ok && repo != nil
}

func cloneInstallResult(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	out := make(map[string]any, len(src))
	for key, value := range src {
		out[key] = value
	}
	return out
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
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
		command := append([]string{name}, args...)
		maxAttempts := aptInstallCommandAttempts(name, args...)
		for attempt := 1; attempt <= maxAttempts; attempt++ {
			code, out := runInstallCommand(ctx, name, args...)
			step := map[string]any{"command": command, "exit_code": code, "output": truncate(out, 4000), "attempt": attempt}
			if code == 0 {
				steps = append(steps, step)
				return true
			}
			if !shouldRetryAptInstallCommand(name, args, code, out) || attempt == maxAttempts {
				steps = append(steps, step)
				return false
			}
			delay := aptInstallRetryDelay(attempt)
			step["retry_after_seconds"] = int(delay.Seconds())
			steps = append(steps, step)
			if err := sleepContext(ctx, delay); err != nil {
				return false
			}
		}
		return false
	}
	if os.Geteuid() != 0 {
		return map[string]any{"ok": false, "message": capabilityCode + " install requires root", "steps": steps}
	}
	if !run("apt-get", "-o", "Acquire::Retries=3", "-o", "Dpkg::Lock::Timeout=120", "update") {
		if !aptPackageCandidateAvailable(ctx, packageName, &steps) {
			return aptInstallFailureResult(capabilityCode, packageName, "apt update failed", steps)
		}
		steps = append(steps, map[string]any{
			"command": []string{"apt-cache", "policy", packageName},
			"note":    "apt update failed, but local package metadata has an install candidate; continuing with package install",
		})
	} else if !aptPackageCandidateAvailable(ctx, packageName, &steps) {
		_ = tryPrepareUbuntuUniverseRepository(ctx, packageName, &steps)
	}
	cleanupServiceAutostart := func() {}
	if shouldPreventPackageServiceAutostart(capabilityCode) {
		cleanup, err := preventPackageServiceAutostart(&steps)
		if err != nil {
			return map[string]any{"ok": false, "message": capabilityCode + " install failed before package install", "error": err.Error(), "steps": steps, "package": packageName}
		}
		cleanupServiceAutostart = cleanup
	}
	defer cleanupServiceAutostart()
	if !run("env", "DEBIAN_FRONTEND=noninteractive", "NEEDRESTART_MODE=a", "UCF_FORCE_CONFFOLD=1", "apt-get", "-o", "Dpkg::Lock::Timeout=120", "-o", "Dpkg::Options::=--force-confdef", "-o", "Dpkg::Options::=--force-confold", "install", "-y", packageName) {
		detail := lastFailedInstallCommand(steps)
		if aptFailureSuggestsRepair(detail.output) {
			steps = append(steps, map[string]any{"stage": "apt_repair", "reason": detail.outputLine})
			_ = run("env", "DEBIAN_FRONTEND=noninteractive", "NEEDRESTART_MODE=a", "dpkg", "--configure", "-a")
			_ = run("env", "DEBIAN_FRONTEND=noninteractive", "NEEDRESTART_MODE=a", "apt-get", "-o", "Dpkg::Lock::Timeout=120", "-f", "install", "-y")
			if !run("env", "DEBIAN_FRONTEND=noninteractive", "NEEDRESTART_MODE=a", "UCF_FORCE_CONFFOLD=1", "apt-get", "-o", "Dpkg::Lock::Timeout=120", "-o", "Dpkg::Options::=--force-confdef", "-o", "Dpkg::Options::=--force-confold", "install", "-y", packageName) {
				return aptInstallFailureResult(capabilityCode, packageName, "package install failed after apt repair", steps)
			}
		} else {
			return aptInstallFailureResult(capabilityCode, packageName, "package install failed", steps)
		}
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

func ubuntuPackageEnableUnits(capabilityCode string, units ...string) []string {
	switch normalizeCapabilityCode(capabilityCode) {
	case driver.Shadowsocks:
		return nil
	default:
		return append([]string(nil), units...)
	}
}

func shouldPreventPackageServiceAutostart(capabilityCode string) bool {
	switch normalizeCapabilityCode(capabilityCode) {
	case driver.Shadowsocks:
		return true
	default:
		return false
	}
}

func preventPackageServiceAutostart(steps *[]map[string]any) (func(), error) {
	const path = "/usr/sbin/policy-rc.d"
	appendStep := func(step map[string]any) {
		if steps != nil {
			*steps = append(*steps, step)
		}
	}
	if _, err := os.Stat(path); err == nil {
		appendStep(map[string]any{"stage": "package_service_autostart", "path": path, "action": "existing_policy_left_unchanged"})
		return func() {}, nil
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 101\n"), 0o755); err != nil {
		return nil, err
	}
	appendStep(map[string]any{"stage": "package_service_autostart", "path": path, "action": "temporary_policy_created"})
	return func() {
		if err := os.Remove(path); err == nil {
			appendStep(map[string]any{"stage": "package_service_autostart", "path": path, "action": "temporary_policy_removed"})
		}
	}, nil
}

func aptPackageCandidateAvailable(ctx context.Context, packageName string, steps *[]map[string]any) bool {
	packageName = strings.TrimSpace(packageName)
	if packageName == "" {
		return false
	}
	code, out := runInstallCommand(ctx, "apt-cache", "policy", packageName)
	if steps != nil {
		*steps = append(*steps, map[string]any{"command": []string{"apt-cache", "policy", packageName}, "exit_code": code, "output": truncate(out, 4000)})
	}
	return code == 0 && aptPolicyHasCandidate(out)
}

func tryPrepareUbuntuUniverseRepository(ctx context.Context, packageName string, steps *[]map[string]any) bool {
	packageName = strings.TrimSpace(packageName)
	if !shouldPrepareUbuntuUniverseRepository(hostOSReleaseID(), packageName) {
		return false
	}
	appendStep := func(step map[string]any) {
		if steps != nil {
			*steps = append(*steps, step)
		}
	}
	appendStep(map[string]any{"stage": "repository_prepare", "repository": "ubuntu_universe", "package": packageName})
	if _, ok := resolveExecutable("add-apt-repository", "/usr/bin/add-apt-repository"); !ok {
		code, out := runInstallCommand(ctx, "env", "DEBIAN_FRONTEND=noninteractive", "apt-get", "-o", "Dpkg::Lock::Timeout=120", "install", "-y", "software-properties-common")
		appendStep(map[string]any{
			"command":   []string{"env", "DEBIAN_FRONTEND=noninteractive", "apt-get", "-o", "Dpkg::Lock::Timeout=120", "install", "-y", "software-properties-common"},
			"exit_code": code,
			"output":    truncate(out, 4000),
		})
		if code != 0 {
			return false
		}
	}
	code, out := runInstallCommand(ctx, "add-apt-repository", "-y", "universe")
	appendStep(map[string]any{
		"command":   []string{"add-apt-repository", "-y", "universe"},
		"exit_code": code,
		"output":    truncate(out, 4000),
	})
	if code != 0 {
		return false
	}
	code, out = runInstallCommand(ctx, "apt-get", "-o", "Acquire::Retries=3", "-o", "Dpkg::Lock::Timeout=120", "update")
	appendStep(map[string]any{
		"command":   []string{"apt-get", "-o", "Acquire::Retries=3", "-o", "Dpkg::Lock::Timeout=120", "update"},
		"exit_code": code,
		"output":    truncate(out, 4000),
	})
	if code != 0 {
		return false
	}
	return aptPackageCandidateAvailable(ctx, packageName, steps)
}

func shouldPrepareUbuntuUniverseRepository(osReleaseID, packageName string) bool {
	return strings.EqualFold(strings.TrimSpace(osReleaseID), "ubuntu") && packageRequiresUbuntuUniverse(packageName)
}

func packageRequiresUbuntuUniverse(packageName string) bool {
	switch strings.TrimSpace(packageName) {
	case "shadowsocks-libev":
		return true
	default:
		return false
	}
}

func hostOSReleaseID() string {
	b, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return ""
	}
	return osReleaseIDFromContent(string(b))
}

func osReleaseIDFromContent(content string) string {
	for _, line := range strings.Split(content, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok || key != "ID" {
			continue
		}
		return strings.Trim(strings.TrimSpace(value), `"'`)
	}
	return ""
}

func aptInstallCommandAttempts(name string, args ...string) int {
	if aptInstallCommandVerb(name, args...) == "" {
		return 1
	}
	return 3
}

func aptInstallCommandVerb(name string, args ...string) string {
	if name == "apt-get" {
		for _, arg := range args {
			switch arg {
			case "update", "install":
				return arg
			}
		}
		return ""
	}
	if name == "env" {
		for idx, arg := range args {
			if arg != "apt-get" {
				continue
			}
			return aptInstallCommandVerb(arg, args[idx+1:]...)
		}
	}
	return ""
}

func shouldRetryAptInstallCommand(name string, args []string, exitCode int, output string) bool {
	if exitCode == 0 || aptInstallCommandVerb(name, args...) == "" {
		return false
	}
	lower := strings.ToLower(output)
	if strings.Contains(lower, "unable to locate package") || strings.Contains(lower, "has no installation candidate") {
		return false
	}
	return aptFailureLooksTransient(output)
}

func aptFailureLooksTransient(output string) bool {
	lower := strings.ToLower(output)
	for _, marker := range []string{
		"could not get lock",
		"unable to acquire the dpkg frontend lock",
		"is another process using it",
		"temporary failure resolving",
		"temporary failure",
		"failed to fetch",
		"connection timed out",
		"could not connect",
		"connection failed",
		"hash sum mismatch",
		"mirror sync in progress",
		"resource temporarily unavailable",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func aptInstallRetryDelay(attempt int) time.Duration {
	switch attempt {
	case 1:
		return 5 * time.Second
	case 2:
		return 15 * time.Second
	default:
		return 30 * time.Second
	}
}

func aptFailureSuggestsRepair(output string) bool {
	lower := strings.ToLower(output)
	for _, marker := range []string{
		"dpkg was interrupted",
		"dpkg --configure -a",
		"apt --fix-broken install",
		"apt-get -f install",
		"fix-broken install",
		"dependency problems prevent configuration",
		"dependencies - leaving unconfigured",
		"post-installation script subprocess returned error",
		"sub-process /usr/bin/dpkg returned an error code",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func aptInstallFailureMessage(capabilityCode, packageName, reason string) string {
	base := strings.TrimSpace(reason)
	if base == "" {
		base = "apt install failed"
	}
	if packageName != "" {
		base += " for package " + packageName
	}
	switch normalizeCapabilityCode(capabilityCode) {
	case driver.Shadowsocks:
		return base + "; fix node apt repositories/network or install a pinned ss-server artifact from the runtime repository"
	case driver.OpenVPN:
		return base + "; fix node apt repositories/network and retry the OpenVPN runtime install"
	default:
		return base + "; fix node apt repositories/network and retry runtime install"
	}
}

func aptInstallFailureResult(capabilityCode, packageName, reason string, steps []map[string]any) map[string]any {
	detail := lastFailedInstallCommand(steps)
	message := aptInstallFailureMessage(capabilityCode, packageName, reason)
	if detail.command != "" {
		message += "; failed command: " + detail.command
	}
	if detail.outputLine != "" {
		message += "; output: " + detail.outputLine
	}
	result := map[string]any{"ok": false, "message": message, "steps": steps, "package": packageName}
	if detail.command != "" {
		result["last_failed_command"] = detail.command
	}
	if detail.exitCode != nil {
		result["last_failed_exit_code"] = *detail.exitCode
	}
	if detail.output != "" {
		result["last_failed_output"] = detail.output
	}
	return result
}

type failedInstallCommand struct {
	command    string
	exitCode   *int
	output     string
	outputLine string
}

func lastFailedInstallCommand(steps []map[string]any) failedInstallCommand {
	for idx := len(steps) - 1; idx >= 0; idx-- {
		step := steps[idx]
		code, ok := intFromInstallStep(step["exit_code"])
		if !ok || code == 0 {
			continue
		}
		command := commandLineFromStep(step["command"])
		if command == "" {
			continue
		}
		output := strings.TrimSpace(stringify(step["output"]))
		codeCopy := code
		return failedInstallCommand{
			command:    command,
			exitCode:   &codeCopy,
			output:     truncate(output, 4000),
			outputLine: firstLine(output),
		}
	}
	return failedInstallCommand{}
}

func commandLineFromStep(raw any) string {
	switch value := raw.(type) {
	case []string:
		return strings.Join(value, " ")
	case []any:
		parts := make([]string, 0, len(value))
		for _, item := range value {
			part := stringify(item)
			if part != "" {
				parts = append(parts, part)
			}
		}
		return strings.Join(parts, " ")
	case string:
		return strings.TrimSpace(value)
	default:
		return ""
	}
}

func intFromInstallStep(raw any) (int, bool) {
	switch value := raw.(type) {
	case int:
		return value, true
	case int8:
		return int(value), true
	case int16:
		return int(value), true
	case int32:
		return int(value), true
	case int64:
		return int(value), true
	case uint:
		return int(value), true
	case uint8:
		return int(value), true
	case uint16:
		return int(value), true
	case uint32:
		return int(value), true
	case uint64:
		if value > uint64(^uint(0)>>1) {
			return 0, false
		}
		return int(value), true
	case float64:
		return int(value), value == float64(int(value))
	default:
		return 0, false
	}
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
