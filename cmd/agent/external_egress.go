package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/rtis-emc2/megavpn/internal/externalegress"
)

const externalEgressManagedRoot = "/etc/megavpn/external-egress"

var (
	externalEgressUUIDPattern              = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	externalEgressCompactUUIDPattern       = regexp.MustCompile(`^[0-9a-f]{32}$`)
	externalEgressInterfacePattern         = regexp.MustCompile(`^mgev[0-9a-f]{10}$`)
	externalEgressListenerPIDPattern       = regexp.MustCompile(`\bpid=([0-9]+)\b`)
	externalEgressL2TPConfigPathPattern    = regexp.MustCompile(`^/etc/megavpn/external-egress/[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}/xl2tpd\.conf$`)
	externalEgressL2TPRuntimePathPattern   = regexp.MustCompile(`^/run/megavpn/external-egress/[0-9a-f]{32}/xl2tpd\.(pid|control)$`)
	resolveExternalEgressCleanupExecutable = resolveExecutable
	globExternalEgressPaths                = filepath.Glob
	removeExternalEgressManagedPath        = removeManagedPath
	readExternalEgressManagedFile          = os.ReadFile
	readExternalEgressProcessExecutable    = os.Readlink
	readExternalEgressProcessCommandLine   = os.ReadFile
	readExternalEgressProcessCgroup        = os.ReadFile
	externalEgressInterfaceReleaseInterval = 200 * time.Millisecond
	externalEgressProbeInterval            = time.Second
	signalExternalEgressProcess            = func(pid int, signal syscall.Signal) error {
		return syscall.Kill(pid, signal)
	}
)

const (
	externalEgressIPsecConfigInclude  = "include /etc/megavpn/external-egress/*/ipsec.conf"
	externalEgressIPsecSecretsInclude = "include /etc/megavpn/external-egress/*/ipsec.secrets"
)

type externalEgressJobPayload struct {
	NodeID        string
	ProfileID     string
	DeploymentID  string
	Protocol      string
	InterfaceName string
	RoutingTable  int
	RouteMetric   int
	FWMark        int
	ProxyPort     int
	Secrets       map[string]string
}

func (c *client) applyExternalEgress(ctx context.Context, j job, st agentState) (string, map[string]any) {
	payload, err := decodeExternalEgressJob(j, st, true)
	if err != nil {
		return "failed", map[string]any{"error": err.Error(), "stage": "validate"}
	}
	if err := validateExternalEgressRuntimeConfig(payload); err != nil {
		return "failed", map[string]any{"error": err.Error(), "stage": "validate_config", "protocol": payload.Protocol}
	}
	runtime := ensureExternalEgressRuntimeCapability(ctx, payload.Protocol)
	if runtime["ok"] != true {
		return "failed", map[string]any{
			"error": firstNonEmptyAgentString(stringify(runtime["message"]), "external egress runtime capability is unavailable"),
			"stage": "runtime_capability", "runtime": runtime,
		}
	}
	files, unit, err := renderExternalEgressRuntime(payload)
	if err != nil {
		return "failed", map[string]any{"error": err.Error(), "stage": "render", "protocol": payload.Protocol}
	}
	unitName := externalEgressUnitName(payload)
	unitPath := externalEgressUnitPath(payload)
	if err := validateExternalEgressManagedArtifacts(payload, files, unitPath); err != nil {
		return "failed", map[string]any{"error": err.Error(), "stage": "validate_artifacts"}
	}
	if code, output := runInstallCommand(ctx, "systemctl", "disable", "--now", unitName); code != 0 && !isMissingSystemdUnitOutput(output) && !strings.Contains(strings.ToLower(output), "not loaded") {
		return "failed", map[string]any{"error": "existing external egress service could not be stopped: " + firstLine(output), "stage": "stop_existing", "unit": unitName}
	}
	var l2tpTakeover map[string]any
	if payload.Protocol == "l2tp_ipsec" {
		l2tpTakeover, err = prepareExternalEgressL2TPTakeover(ctx, payload)
		if err != nil {
			return "failed", map[string]any{"error": err.Error(), "stage": "prepare_l2tp_takeover", "takeover": l2tpTakeover}
		}
		interfaceRelease, releaseErr := releaseExternalEgressL2TPInterface(ctx, payload)
		l2tpTakeover["interface_release"] = interfaceRelease
		if releaseErr != nil {
			return "failed", map[string]any{
				"error": releaseErr.Error(), "stage": "prepare_l2tp_interface",
				"takeover": l2tpTakeover,
			}
		}
		code, output := runInstallCommand(ctx, "ss", "-H", "-lunp")
		if code != 0 {
			return "failed", map[string]any{"error": "cannot verify UDP/1701 availability: " + firstLine(output), "stage": "port_preflight"}
		}
		if externalEgressUDPPortListening(output, 1701) {
			listenerProcess, listenerErr := terminateManagedExternalEgressL2TPListener(ctx, output, 1701, true)
			l2tpTakeover["orphan_listener"] = listenerProcess
			if listenerErr != nil {
				return "failed", map[string]any{
					"error": "inspect stale managed UDP/1701 listener: " + listenerErr.Error(),
					"stage": "port_preflight", "takeover": l2tpTakeover,
				}
			}
			if listenerProcess["terminated"] == true {
				code, output = runInstallCommand(ctx, "ss", "-H", "-lunp")
				if code != 0 {
					return "failed", map[string]any{
						"error": "cannot verify UDP/1701 after managed listener teardown: " + firstLine(output),
						"stage": "port_preflight", "takeover": l2tpTakeover,
					}
				}
			}
		}
		if externalEgressUDPPortListening(output, 1701) {
			return "failed", map[string]any{
				"error": "UDP/1701 is already in use after managed L2TP teardown; stop or move the remaining listener before applying this provider profile",
				"stage": "port_preflight",
				"owner": externalEgressUDPPortOwner(output, 1701),
			}
		}
		preview, parseErr := externalegress.ParseL2TPIPsec([]byte(payload.Secrets["config"]))
		if parseErr != nil {
			return "failed", map[string]any{"error": parseErr.Error(), "stage": "validate_config"}
		}
		if preview.AuthMethod != "certificate" {
			for _, path := range externalEgressIPsecCertificatePaths(payload) {
				if _, removeErr := removeManagedPath(path, false); removeErr != nil && !os.IsNotExist(removeErr) {
					return "failed", map[string]any{"error": removeErr.Error(), "stage": "remove_stale_certificate", "path": path}
				}
			}
		}
	}
	if externalEgressUsesPolicyRouting(payload.Protocol) {
		if err := installExternalEgressFailClosedGuard(ctx, payload); err != nil {
			return "failed", map[string]any{"error": err.Error(), "stage": "install_fail_closed_guard", "unit": unitName}
		}
	}
	for _, file := range files {
		if err := writeManagedFile(file); err != nil {
			return "failed", map[string]any{"error": err.Error(), "stage": "write", "path": file.Path}
		}
	}
	if err := writeManagedFile(managedFileSpec{Path: unitPath, Content: unit, Mode: "0644"}); err != nil {
		return "failed", map[string]any{"error": err.Error(), "stage": "write_unit"}
	}
	if externalEgressUsesLoopbackProxyRuntime(payload.Protocol) {
		if err := validateExternalEgressXrayConfig(ctx, payload); err != nil {
			return "failed", map[string]any{"error": err.Error(), "stage": "validate_xray_config"}
		}
	}
	if _, err := runSystemdDaemonReload(ctx); err != nil {
		return "failed", map[string]any{"error": err.Error(), "stage": "daemon_reload"}
	}
	code, output := runInstallCommand(ctx, "systemctl", "enable", "--now", unitName)
	if code != 0 {
		return "failed", map[string]any{
			"error": "external egress service failed to start: " + firstLine(output), "stage": "systemd_start",
			"unit": unitName, "output": truncate(output, 4000),
		}
	}
	status, result := c.probeExternalEgressUntilStable(ctx, j, st, payload.Protocol)
	if len(l2tpTakeover) > 0 {
		result["l2tp_takeover"] = l2tpTakeover
	}
	return status, result
}

func releaseExternalEgressL2TPInterface(ctx context.Context, payload externalEgressJobPayload) (map[string]any, error) {
	result := map[string]any{
		"interface": payload.InterfaceName,
		"released":  false,
	}
	for attempt := 0; attempt < 25; attempt++ {
		code, output := runInstallCommand(ctx, "ip", "-o", "link", "show", "dev", payload.InterfaceName)
		if code != 0 {
			if externalEgressInterfaceMissing(output) {
				result["released"] = true
				result["attempts"] = attempt + 1
				return result, nil
			}
			result["inspect_error"] = firstLine(output)
			return result, fmt.Errorf("inspect stale managed PPP interface %s: %s", payload.InterfaceName, firstNonEmptyAgentString(firstLine(output), "ip link failed"))
		}
		if attempt == 0 {
			result["stale_interface"] = truncate(strings.TrimSpace(output), 1000)
			_, _ = runInstallCommand(ctx, "ip", "link", "set", "dev", payload.InterfaceName, "down")
			deleteCode, deleteOutput := runInstallCommand(ctx, "ip", "link", "delete", "dev", payload.InterfaceName)
			result["delete_exit_code"] = deleteCode
			if deleteCode != 0 && !externalEgressInterfaceMissing(deleteOutput) {
				result["delete_output"] = truncate(strings.TrimSpace(deleteOutput), 1000)
			}
		}
		timer := time.NewTimer(externalEgressInterfaceReleaseInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return result, ctx.Err()
		case <-timer.C:
		}
	}
	return result, fmt.Errorf("stale managed PPP interface %s remained after stopping the previous L2TP runtime", payload.InterfaceName)
}

func externalEgressInterfaceMissing(output string) bool {
	normalized := strings.ToLower(strings.TrimSpace(output))
	return strings.Contains(normalized, "cannot find device") ||
		strings.Contains(normalized, "does not exist") ||
		strings.Contains(normalized, "no such device")
}

func prepareExternalEgressL2TPTakeover(ctx context.Context, payload externalEgressJobPayload) (map[string]any, error) {
	result := map[string]any{
		"system_xl2tpd_stopped": false,
		"current_process":       map[string]any{},
		"stale_deployments":     []string{},
		"removed_paths":         []string{},
	}
	currentProcess, err := terminateManagedExternalEgressL2TPProcess(ctx, payload)
	result["current_process"] = currentProcess
	if err != nil {
		return result, fmt.Errorf("stop current managed L2TP process: %w", err)
	}
	code, output := runInstallCommand(ctx, "systemctl", "disable", "--now", "xl2tpd.service")
	if code != 0 && !isMissingSystemdUnitOutput(output) {
		result["system_xl2tpd_output"] = truncate(output, 2000)
		return result, fmt.Errorf("existing xl2tpd service could not be stopped: %s", firstLine(output))
	}
	result["system_xl2tpd_stopped"] = code == 0

	configs, err := globExternalEgressPaths(filepath.Join(externalEgressManagedRoot, "*", "xl2tpd.conf"))
	if err != nil {
		return result, fmt.Errorf("discover stale managed L2TP runtimes: %w", err)
	}
	staleDeployments := []string{}
	removedPaths := []string{}
	for _, configPath := range configs {
		deploymentID := strings.ToLower(filepath.Base(filepath.Dir(configPath)))
		if deploymentID == payload.DeploymentID || !externalEgressUUIDPattern.MatchString(deploymentID) {
			continue
		}
		stale := payload
		stale.DeploymentID = deploymentID
		unitName := externalEgressUnitName(stale)
		if stopCode, stopOutput := runInstallCommand(ctx, "systemctl", "disable", "--now", unitName); stopCode != 0 && !isMissingSystemdUnitOutput(stopOutput) {
			return result, fmt.Errorf("stop stale managed L2TP runtime %s: %s", deploymentID, firstLine(stopOutput))
		}
		processResult, processErr := terminateManagedExternalEgressL2TPProcess(ctx, stale)
		if processErr != nil {
			return result, fmt.Errorf("stop stale managed L2TP process %s: %w", deploymentID, processErr)
		}
		result["stale_process_"+deploymentID] = processResult
		routeScript := filepath.Join(externalEgressManagedDir(stale), "route-policy.sh")
		if info, statErr := os.Stat(routeScript); statErr == nil && info.Mode()&0o111 != 0 {
			if routeCode, routeOutput := runInstallCommand(ctx, routeScript, "cleanup"); routeCode != 0 && !isMissingIPRouteTableOutput(routeOutput) {
				return result, fmt.Errorf("cleanup stale managed L2TP policy %s: %s", deploymentID, firstLine(routeOutput))
			}
		}
		for _, path := range append(
			[]string{
				externalEgressUnitPath(stale),
				externalEgressManagedDir(stale),
				externalEgressRuntimeDir(stale),
			},
			externalEgressIPsecCertificatePaths(stale)...,
		) {
			recursive := path == externalEgressManagedDir(stale) || path == externalEgressRuntimeDir(stale)
			removed, removeErr := removeExternalEgressManagedPath(path, recursive)
			if removeErr != nil && !os.IsNotExist(removeErr) {
				return result, fmt.Errorf("remove stale managed L2TP path %s: %w", path, removeErr)
			}
			if removed {
				removedPaths = append(removedPaths, path)
			}
		}
		staleDeployments = append(staleDeployments, deploymentID)
	}
	result["stale_deployments"] = staleDeployments
	result["removed_paths"] = removedPaths
	if len(staleDeployments) > 0 {
		if _, reloadErr := runSystemdDaemonReload(ctx); reloadErr != nil {
			return result, reloadErr
		}
	}
	return result, nil
}

func validateExternalEgressRuntimeConfig(payload externalEgressJobPayload) error {
	switch payload.Protocol {
	case "openvpn":
		_, _, err := renderManagedOpenVPNConfig(payload, externalEgressManagedDir(payload))
		return err
	case "wireguard":
		_, err := renderManagedWireGuardConfig(payload)
		return err
	case "shadowsocks":
		_, err := renderManagedShadowsocksXrayConfig(payload)
		return err
	case "vless":
		_, err := renderManagedVLESSXrayConfig(payload)
		return err
	case "l2tp_ipsec":
		_, _, _, _, err := renderManagedL2TPIPsecConfig(payload)
		return err
	default:
		return fmt.Errorf("external egress protocol %q is not runtime-ready", payload.Protocol)
	}
}

func ensureExternalEgressRuntimeCapability(ctx context.Context, protocol string) map[string]any {
	var result map[string]any
	switch protocol {
	case "openvpn":
		result = verifyOpenVPN(ctx)
		if result["ok"] != true {
			result = installUbuntuPackageCapability(ctx, "openvpn", "openvpn", "openvpn", []string{"openvpn"})
		}
	case "wireguard":
		result = verifyWireGuard(ctx)
		if result["ok"] != true {
			result = installUbuntuPackageCapability(ctx, "wireguard", "wireguard-tools", "wireguard-tools", nil)
		}
	case "vless", "shadowsocks":
		result = verifyXrayCore(ctx)
		if result["ok"] != true {
			result = installXrayCore(ctx, "stable")
		}
	case "l2tp_ipsec":
		result = ensureL2TPIPsecExternalEgressCapability(ctx)
	default:
		return map[string]any{"ok": false, "message": "external egress protocol runtime is not supported"}
	}
	return result
}

func ensureL2TPIPsecExternalEgressCapability(ctx context.Context) map[string]any {
	checks := []struct {
		verify func(context.Context) map[string]any
		code   string
		pkg    string
		hint   string
	}{
		{verify: verifyIPsec, code: "ipsec", pkg: "strongswan", hint: "strongswan"},
		{verify: verifyPPP, code: "ppp", pkg: "ppp", hint: "pppd"},
		{verify: verifyXL2TPD, code: "xl2tpd", pkg: "xl2tpd", hint: "xl2tpd"},
	}
	results := []map[string]any{}
	for _, check := range checks {
		result := check.verify(ctx)
		if result["ok"] != true {
			result = installUbuntuPackageCapability(ctx, check.code, check.pkg, check.hint, nil)
		}
		results = append(results, result)
		if result["ok"] != true {
			return map[string]any{
				"ok": false, "message": firstNonEmptyAgentString(stringify(result["message"]), check.code+" capability is unavailable"),
				"components": results,
			}
		}
	}
	if _, ok := resolveExecutable("flock"); !ok {
		return map[string]any{"ok": false, "message": "flock capability is unavailable", "components": results}
	}
	return map[string]any{"ok": true, "message": "L2TP/IPsec external egress capability verified", "components": results}
}

func validateExternalEgressXrayConfig(ctx context.Context, payload externalEgressJobPayload) error {
	binary, ok := resolveExecutable("xray")
	if !ok {
		return fmt.Errorf("xray binary is not installed or not executable")
	}
	configPath := filepath.Join(externalEgressManagedDir(payload), "config.json")
	code, output := runInstallCommand(ctx, binary, "run", "-test", "-config", configPath)
	if code != 0 {
		fallbackCode, fallbackOutput := runInstallCommand(ctx, binary, "-test", "-config", configPath)
		if fallbackCode != 0 {
			return fmt.Errorf("xray rejected external egress config: %s", firstLine(firstNonEmptyAgentString(fallbackOutput, output)))
		}
	}
	return nil
}

func externalEgressLoopbackPortListening(output string, port int) bool {
	if port < 1 || port > 65535 {
		return false
	}
	wanted := strconv.Itoa(port)
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		for _, address := range fields {
			if address == "127.0.0.1:"+wanted || address == "[::1]:"+wanted {
				return true
			}
		}
	}
	return false
}

func externalEgressUDPPortListening(output string, port int) bool {
	if port < 1 || port > 65535 {
		return false
	}
	wanted := strconv.Itoa(port)
	for _, line := range strings.Split(output, "\n") {
		for _, address := range strings.Fields(strings.TrimSpace(line)) {
			host, value, err := net.SplitHostPort(address)
			if err == nil && value == wanted && host != "" {
				return true
			}
		}
	}
	return false
}

func externalEgressUDPPortOwner(output string, port int) string {
	if port < 1 || port > 65535 {
		return ""
	}
	wanted := strconv.Itoa(port)
	for _, line := range strings.Split(output, "\n") {
		for _, address := range strings.Fields(strings.TrimSpace(line)) {
			host, value, err := net.SplitHostPort(address)
			if err == nil && value == wanted && host != "" {
				return strings.TrimSpace(line)
			}
		}
	}
	return ""
}

func terminateManagedExternalEgressL2TPProcess(ctx context.Context, payload externalEgressJobPayload) (map[string]any, error) {
	pidPath := filepath.Join(externalEgressRuntimeDir(payload), "xl2tpd.pid")
	return terminateManagedExternalEgressL2TPProcessAt(ctx, pidPath)
}

func terminateManagedExternalEgressL2TPProcessAt(ctx context.Context, pidPath string) (map[string]any, error) {
	result := map[string]any{
		"pid_file":   pidPath,
		"pid":        0,
		"terminated": false,
	}
	rawPID, err := readExternalEgressManagedFile(pidPath)
	if err != nil {
		if os.IsNotExist(err) {
			result["pid_file_present"] = false
			return result, nil
		}
		return result, fmt.Errorf("read managed xl2tpd pid file: %w", err)
	}
	result["pid_file_present"] = true
	pid, err := strconv.Atoi(strings.TrimSpace(string(rawPID)))
	if err != nil || pid <= 1 {
		return result, fmt.Errorf("managed xl2tpd pid file is invalid")
	}
	result["pid"] = pid

	active, owned, executable, err := externalEgressManagedL2TPProcess(pid)
	if err != nil {
		return result, err
	}
	result["executable"] = executable
	if active && owned {
		managed, commandLine, commandErr := externalEgressManagedL2TPCommandLine(pid)
		if commandErr != nil {
			return result, commandErr
		}
		result["command_line"] = commandLine
		if !managed {
			result["stale_pid"] = true
			result["pid_reused"] = true
			result["ownership_mismatch"] = true
			return result, nil
		}
		result["ownership"] = "managed_arguments"
	}
	return terminateVerifiedExternalEgressL2TPProcess(ctx, pid, result)
}

func terminateManagedExternalEgressL2TPListener(ctx context.Context, output string, port int, allowSystemService bool) (map[string]any, error) {
	owner := externalEgressUDPPortOwner(output, port)
	result := map[string]any{
		"owner":      owner,
		"pid":        0,
		"managed":    false,
		"terminated": false,
	}
	match := externalEgressListenerPIDPattern.FindStringSubmatch(owner)
	if len(match) != 2 {
		return result, nil
	}
	pid, err := strconv.Atoi(match[1])
	if err != nil || pid <= 1 {
		return result, nil
	}
	result["pid"] = pid

	active, owned, executable, err := externalEgressManagedL2TPProcess(pid)
	if err != nil {
		return result, err
	}
	result["executable"] = executable
	if !active {
		result["stale_pid"] = true
		return result, nil
	}
	if !owned {
		return result, nil
	}
	managed, commandLine, err := externalEgressManagedL2TPCommandLine(pid)
	if err != nil {
		return result, err
	}
	result["command_line"] = commandLine
	if !managed {
		if !allowSystemService {
			return result, nil
		}
		systemService, cgroup, cgroupErr := externalEgressL2TPSystemdServiceProcess(pid)
		if cgroupErr != nil {
			return result, cgroupErr
		}
		result["cgroup"] = cgroup
		if !systemService {
			return result, nil
		}
		result["ownership"] = "xl2tpd.service"
	} else {
		result["ownership"] = "managed_arguments"
	}
	result["managed"] = true
	return terminateVerifiedExternalEgressL2TPProcess(ctx, pid, result)
}

func externalEgressManagedL2TPCommandLine(pid int) (bool, string, error) {
	raw, err := readExternalEgressProcessCommandLine(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
	if err != nil {
		if os.IsNotExist(err) {
			return false, "", nil
		}
		return false, "", fmt.Errorf("inspect managed xl2tpd command line %d: %w", pid, err)
	}
	arguments := strings.Split(strings.TrimRight(string(raw), "\x00"), "\x00")
	for _, argument := range arguments {
		cleaned := filepath.Clean(strings.TrimSpace(argument))
		if externalEgressL2TPConfigPathPattern.MatchString(cleaned) ||
			externalEgressL2TPRuntimePathPattern.MatchString(cleaned) {
			return true, strings.Join(arguments, " "), nil
		}
	}
	return false, strings.Join(arguments, " "), nil
}

func externalEgressL2TPSystemdServiceProcess(pid int) (bool, string, error) {
	raw, err := readExternalEgressProcessCgroup(filepath.Join("/proc", strconv.Itoa(pid), "cgroup"))
	if err != nil {
		if os.IsNotExist(err) {
			return false, "", nil
		}
		return false, "", fmt.Errorf("inspect xl2tpd systemd cgroup %d: %w", pid, err)
	}
	cgroup := strings.TrimSpace(string(raw))
	for _, line := range strings.Split(cgroup, "\n") {
		fields := strings.SplitN(line, ":", 3)
		if len(fields) != 3 {
			continue
		}
		for _, component := range strings.Split(fields[2], "/") {
			if component == "xl2tpd.service" {
				return true, cgroup, nil
			}
		}
	}
	return false, cgroup, nil
}

func terminateVerifiedExternalEgressL2TPProcess(ctx context.Context, pid int, result map[string]any) (map[string]any, error) {
	active, owned, executable, err := externalEgressManagedL2TPProcess(pid)
	if err != nil {
		return result, err
	}
	result["executable"] = executable
	if !active {
		result["stale_pid"] = true
		return result, nil
	}
	if !owned {
		// The PID was reused after xl2tpd exited. Never signal an unrelated process.
		result["stale_pid"] = true
		result["pid_reused"] = true
		return result, nil
	}
	switch stringify(result["ownership"]) {
	case "managed_arguments":
		managed, commandLine, commandErr := externalEgressManagedL2TPCommandLine(pid)
		result["command_line"] = commandLine
		if commandErr != nil {
			return result, commandErr
		}
		if !managed {
			result["ownership_changed"] = true
			return result, nil
		}
	case "xl2tpd.service":
		systemService, cgroup, cgroupErr := externalEgressL2TPSystemdServiceProcess(pid)
		result["cgroup"] = cgroup
		if cgroupErr != nil {
			return result, cgroupErr
		}
		if !systemService {
			result["ownership_changed"] = true
			return result, nil
		}
	default:
		result["ownership_missing"] = true
		return result, nil
	}

	if err := signalExternalEgressProcess(pid, syscall.SIGTERM); err != nil && err != syscall.ESRCH {
		return result, fmt.Errorf("terminate managed xl2tpd process %d: %w", pid, err)
	}
	result["term_sent"] = true
	exited, err := waitForManagedExternalEgressL2TPProcess(ctx, pid, 30, 100*time.Millisecond)
	if err != nil {
		return result, err
	}
	if exited {
		result["terminated"] = true
		return result, nil
	}

	active, owned, executable, err = externalEgressManagedL2TPProcess(pid)
	if err != nil {
		return result, err
	}
	if !active || !owned {
		result["terminated"] = true
		result["executable"] = executable
		return result, nil
	}
	if err := signalExternalEgressProcess(pid, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
		return result, fmt.Errorf("kill managed xl2tpd process %d: %w", pid, err)
	}
	result["kill_sent"] = true
	exited, err = waitForManagedExternalEgressL2TPProcess(ctx, pid, 20, 100*time.Millisecond)
	if err != nil {
		return result, err
	}
	if !exited {
		return result, fmt.Errorf("managed xl2tpd process %d did not exit", pid)
	}
	result["terminated"] = true
	return result, nil
}

func externalEgressManagedL2TPProcess(pid int) (active, owned bool, executable string, err error) {
	executable, err = readExternalEgressProcessExecutable(filepath.Join("/proc", strconv.Itoa(pid), "exe"))
	if err != nil {
		if os.IsNotExist(err) {
			return false, false, "", nil
		}
		return false, false, "", fmt.Errorf("inspect managed xl2tpd process %d: %w", pid, err)
	}
	name := filepath.Base(strings.TrimSuffix(executable, " (deleted)"))
	return true, name == "xl2tpd", executable, nil
}

func waitForManagedExternalEgressL2TPProcess(ctx context.Context, pid, attempts int, interval time.Duration) (bool, error) {
	for attempt := 0; attempt < attempts; attempt++ {
		active, owned, _, err := externalEgressManagedL2TPProcess(pid)
		if err != nil {
			return false, err
		}
		if !active || !owned {
			return true, nil
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return false, ctx.Err()
		case <-timer.C:
		}
	}
	return false, nil
}

func externalEgressRuntimeProtocol(protocol string) bool {
	switch protocol {
	case "openvpn", "wireguard", "l2tp_ipsec", "vless", "shadowsocks":
		return true
	default:
		return false
	}
}

func externalEgressUsesLoopbackProxyRuntime(protocol string) bool {
	return protocol == "vless" || protocol == "shadowsocks"
}

func externalEgressUsesPolicyRouting(protocol string) bool {
	return !externalEgressUsesLoopbackProxyRuntime(protocol)
}

func externalEgressProxyPort(routingTable int) int {
	return 20000 + routingTable - 40000
}

func (c *client) probeExternalEgress(ctx context.Context, j job, st agentState) (string, map[string]any) {
	payload, err := decodeExternalEgressJob(j, st, false)
	if err != nil {
		return "failed", map[string]any{"error": err.Error(), "stage": "validate"}
	}
	unitName := externalEgressUnitName(payload)
	unitCode, unitOutput := runInstallCommand(ctx, "systemctl", "is-active", unitName)
	unitState := normalizeSystemctlState(strings.TrimSpace(unitOutput))
	if externalEgressUsesLoopbackProxyRuntime(payload.Protocol) {
		listenCode, listenOutput := runInstallCommand(ctx, "ss", "-H", "-lnt")
		listening := listenCode == 0 && externalEgressLoopbackPortListening(listenOutput, payload.ProxyPort)
		health := map[string]any{
			"status": "active", "unit": unitName, "unit_state": unitState,
			"proxy_address": "127.0.0.1", "proxy_port": payload.ProxyPort, "proxy_listening": listening,
			"unit_check_exit_code": unitCode, "listener_check_exit_code": listenCode,
		}
		if unitState != "active" || !listening {
			health["status"] = "failed"
			return "failed", map[string]any{"error": "external egress loopback proxy is incomplete", "health": health}
		}
		return "succeeded", map[string]any{"message": "external egress loopback proxy is active", "health": health}
	}
	interfaceCode, interfaceOutput := runInstallCommand(ctx, "ip", "-o", "link", "show", "dev", payload.InterfaceName)
	interfaceState := strings.TrimSpace(interfaceOutput)
	addressCode, addressOutput := runInstallCommand(ctx, "ip", "-4", "-o", "addr", "show", "dev", payload.InterfaceName)
	addressState := strings.TrimSpace(addressOutput)
	addressRequired := payload.Protocol == "l2tp_ipsec"
	rulesCode, rules := runInstallCommand(ctx, "ip", "rule", "show")
	mark := fmt.Sprintf("0x%x", payload.FWMark)
	rulePresent := rulesCode == 0 && strings.Contains(strings.ToLower(rules), strings.ToLower(mark)) && strings.Contains(rules, strconv.Itoa(payload.RoutingTable))
	routesCode, routesOutput := runInstallCommand(ctx, "ip", "route", "show", "table", strconv.Itoa(payload.RoutingTable))
	routes := strings.TrimSpace(routesOutput)
	providerRoutePresent, failClosedRoutePresent := externalEgressRouteHealth(routes, payload.InterfaceName)
	ipsecEstablished := true
	ipsecCode := 0
	ipsecOutput := ""
	if payload.Protocol == "l2tp_ipsec" {
		ipsecCode, ipsecOutput = runInstallCommand(ctx, "ipsec", "status", externalEgressIPsecConnectionName(payload))
		normalized := strings.ToUpper(ipsecOutput)
		ipsecEstablished = ipsecCode == 0 &&
			strings.Contains(normalized, "ESTABLISHED") &&
			strings.Contains(normalized, "INSTALLED")
	}
	health := map[string]any{
		"status": "active", "unit": unitName, "unit_state": unitState,
		"interface": payload.InterfaceName, "interface_present": interfaceCode == 0 && interfaceState != "",
		"interface_address_present": addressCode == 0 && addressState != "", "interface_address_required": addressRequired,
		"routing_table": payload.RoutingTable, "route_present": providerRoutePresent,
		"provider_default_route_present": providerRoutePresent, "fail_closed_route_present": failClosedRoutePresent,
		"fwmark": mark, "rule_present": rulePresent,
		"unit_check_exit_code": unitCode, "interface_check_exit_code": interfaceCode,
		"address_check_exit_code": addressCode, "rule_check_exit_code": rulesCode,
		"route_check_exit_code": routesCode,
	}
	missing := []string{}
	interfaceDescription := "tunnel interface "
	if payload.Protocol == "l2tp_ipsec" {
		interfaceDescription = "PPP interface "
	}
	if unitState != "active" {
		missing = append(missing, "systemd unit "+unitName)
	}
	if interfaceCode != 0 || interfaceState == "" {
		missing = append(missing, interfaceDescription+payload.InterfaceName)
	}
	if addressRequired && (addressCode != 0 || addressState == "") {
		missing = append(missing, "IPv4 address on "+payload.InterfaceName)
	}
	if !providerRoutePresent {
		missing = append(missing, "provider default route in table "+strconv.Itoa(payload.RoutingTable))
	}
	if !rulePresent {
		missing = append(missing, "policy rule for "+mark)
	}
	if payload.Protocol == "l2tp_ipsec" {
		health["ipsec_connection"] = externalEgressIPsecConnectionName(payload)
		health["ipsec_established"] = ipsecEstablished
		health["ipsec_check_exit_code"] = ipsecCode
		health["ipsec_status"] = truncate(strings.TrimSpace(ipsecOutput), 2000)
		if !ipsecEstablished {
			missing = append(missing, "IPsec security association "+externalEgressIPsecConnectionName(payload))
		}
	}
	if len(missing) > 0 {
		health["status"] = "failed"
		health["missing_components"] = missing
		diagnosticCommands := []string{
			"systemctl status " + unitName + " --no-pager -l",
			"journalctl -u " + unitName + " -n 200 --no-pager -l",
			"ip -4 addr show dev " + payload.InterfaceName,
			"ip route show table " + strconv.Itoa(payload.RoutingTable),
			"ip rule show",
		}
		if payload.Protocol == "l2tp_ipsec" {
			diagnosticCommands = append([]string{
				"ipsec status " + externalEgressIPsecConnectionName(payload),
			}, diagnosticCommands...)
		}
		return "failed", map[string]any{
			"error":               "external egress runtime is incomplete: missing " + strings.Join(missing, ", "),
			"message":             externalEgressProbeRequirementMessage(payload.Protocol),
			"diagnostic_commands": diagnosticCommands,
			"health":              health,
		}
	}
	return "succeeded", map[string]any{"message": "external egress runtime is active", "health": health}
}

func (c *client) probeExternalEgressUntilStable(ctx context.Context, j job, st agentState, protocol string) (string, map[string]any) {
	requiredConsecutive := 1
	attempts := 1
	if protocol == "l2tp_ipsec" {
		requiredConsecutive = 2
		attempts = 4
	}
	consecutive := 0
	status := "failed"
	result := map[string]any{"error": "external egress runtime probe did not run"}
	for attempt := 0; attempt < attempts; attempt++ {
		status, result = c.probeExternalEgress(ctx, j, st)
		result["probe_attempt"] = attempt + 1
		if status == "succeeded" {
			consecutive++
			if consecutive >= requiredConsecutive {
				result["stable_observations"] = consecutive
				return status, result
			}
		} else {
			consecutive = 0
		}
		if attempt+1 < attempts {
			timer := time.NewTimer(externalEgressProbeInterval)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				return "failed", map[string]any{"error": ctx.Err().Error(), "stage": "stability_probe"}
			case <-timer.C:
			}
		}
	}
	result["stable_observations"] = consecutive
	if consecutive < requiredConsecutive {
		if status == "succeeded" {
			result["error"] = fmt.Sprintf(
				"external egress runtime did not remain stable: observed %d of %d required consecutive complete probes",
				consecutive,
				requiredConsecutive,
			)
			if health, ok := result["health"].(map[string]any); ok {
				health["status"] = "failed"
				health["stability_check"] = "failed"
			}
		}
		return "failed", result
	}
	return status, result
}

func externalEgressProbeRequirementMessage(protocol string) string {
	if protocol == "l2tp_ipsec" {
		return "systemd unit, IPsec association, PPP interface address, provider route and policy rule must all be active"
	}
	return "systemd unit, tunnel interface, provider route and policy rule must all be active"
}

func externalEgressRouteHealth(routes, interfaceName string) (providerDefault, failClosed bool) {
	for _, line := range strings.Split(routes, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 {
			continue
		}
		if fields[0] == "unreachable" && fields[1] == "default" {
			failClosed = true
			continue
		}
		if fields[0] != "default" {
			continue
		}
		for index := 1; index+1 < len(fields); index++ {
			if fields[index] == "dev" && fields[index+1] == interfaceName {
				providerDefault = true
				break
			}
		}
	}
	return providerDefault, failClosed
}

func (c *client) cleanupExternalEgress(ctx context.Context, j job, st agentState) (string, map[string]any) {
	payload, err := decodeExternalEgressJob(j, st, false)
	if err != nil {
		return "failed", map[string]any{"error": err.Error(), "stage": "validate"}
	}
	unitName := externalEgressUnitName(payload)
	warnings := []string{}
	l2tpProcess := map[string]any{}
	l2tpListenerProcess := map[string]any{}
	l2tpInterfaceRelease := map[string]any{}
	l2tpTeardownSafe := true
	if code, output := runInstallCommand(ctx, "systemctl", "disable", "--now", unitName); code != 0 && !isMissingSystemdUnitOutput(output) {
		warnings = append(warnings, "systemd stop: "+firstLine(output))
	}
	if externalEgressUsesPolicyRouting(payload.Protocol) {
		if err := cleanupExternalEgressPolicy(ctx, payload); err != nil {
			warnings = append(warnings, err.Error())
		}
	}
	if payload.Protocol == "l2tp_ipsec" {
		if code, output := runInstallCommand(ctx, "systemctl", "disable", "--now", "xl2tpd.service"); code != 0 && !isMissingSystemdUnitOutput(output) {
			warnings = append(warnings, "system xl2tpd stop: "+firstLine(output))
		}
		processResult, processErr := terminateManagedExternalEgressL2TPProcess(ctx, payload)
		l2tpProcess = processResult
		if processErr != nil {
			l2tpTeardownSafe = false
			warnings = append(warnings, processErr.Error())
		}
		if code, output := runInstallCommand(ctx, "ss", "-H", "-lunp"); code != 0 {
			l2tpTeardownSafe = false
			warnings = append(warnings, "verify UDP/1701 cleanup: "+firstLine(output))
		} else {
			if externalEgressUDPPortListening(output, 1701) {
				listenerResult, listenerErr := terminateManagedExternalEgressL2TPListener(ctx, output, 1701, true)
				l2tpListenerProcess = listenerResult
				if listenerErr != nil {
					l2tpTeardownSafe = false
					warnings = append(warnings, "inspect stale managed UDP/1701 listener: "+listenerErr.Error())
				}
				if listenerResult["terminated"] == true {
					code, output = runInstallCommand(ctx, "ss", "-H", "-lunp")
					if code != 0 {
						l2tpTeardownSafe = false
						warnings = append(warnings, "verify UDP/1701 after managed listener teardown: "+firstLine(output))
					}
				}
			}
			if code == 0 && externalEgressUDPPortListening(output, 1701) {
				l2tpTeardownSafe = false
				owner := externalEgressUDPPortOwner(output, 1701)
				warnings = append(warnings, "UDP/1701 remains in use after managed cleanup: "+firstNonEmptyAgentString(owner, "owner unavailable"))
			}
		}
		if l2tpTeardownSafe {
			var releaseErr error
			l2tpInterfaceRelease, releaseErr = releaseExternalEgressL2TPInterface(ctx, payload)
			if releaseErr != nil {
				l2tpTeardownSafe = false
				warnings = append(warnings, releaseErr.Error())
			}
		}
	}
	paths := []string{externalEgressUnitPath(payload), externalEgressManagedDir(payload), externalEgressRuntimeDir(payload)}
	if !l2tpTeardownSafe {
		paths = nil
		warnings = append(warnings, "managed L2TP artifacts were preserved for a safe cleanup retry")
	}
	for index, path := range paths {
		if _, err := removeExternalEgressManagedPath(path, index > 0); err != nil && !os.IsNotExist(err) {
			warnings = append(warnings, path+": "+err.Error())
		}
	}
	if payload.Protocol == "l2tp_ipsec" {
		for _, path := range externalEgressIPsecCertificatePaths(payload) {
			if _, err := removeExternalEgressManagedPath(path, false); err != nil && !os.IsNotExist(err) {
				warnings = append(warnings, path+": "+err.Error())
			}
		}
		if includeWarnings := cleanupExternalEgressIPsecIncludesIfUnused(); len(includeWarnings) > 0 {
			warnings = append(warnings, includeWarnings...)
		}
		if warning := reloadExternalEgressIPsecAfterCleanup(ctx); warning != "" {
			warnings = append(warnings, warning)
		}
	}
	if _, err := runSystemdDaemonReload(ctx); err != nil {
		warnings = append(warnings, err.Error())
	}
	result := map[string]any{
		"message": "external egress runtime removed", "unit": unitName, "warnings": warnings,
		"health": map[string]any{"status": "inactive"},
	}
	if payload.Protocol == "l2tp_ipsec" {
		result["l2tp_process"] = l2tpProcess
		result["l2tp_listener_process"] = l2tpListenerProcess
		result["l2tp_interface_release"] = l2tpInterfaceRelease
	}
	if len(warnings) > 0 {
		result["error"] = strings.Join(warnings, "; ")
		return "failed", result
	}
	return "succeeded", result
}

func cleanupExternalEgressIPsecIncludesIfUnused() []string {
	configs, err := globExternalEgressPaths(filepath.Join(externalEgressManagedRoot, "*", "ipsec.conf"))
	if err != nil {
		return []string{"discover managed IPsec configs: " + err.Error()}
	}
	if len(configs) > 0 {
		return nil
	}
	warnings := []string{}
	for path, line := range map[string]string{
		"/etc/ipsec.conf":    externalEgressIPsecConfigInclude,
		"/etc/ipsec.secrets": externalEgressIPsecSecretsInclude,
	} {
		if _, err := removeExactManagedConfigLine(path, line); err != nil && !os.IsNotExist(err) {
			warnings = append(warnings, path+": "+err.Error())
		}
	}
	return warnings
}

func removeExactManagedConfigLine(path, target string) (bool, error) {
	path = cleanAbsPath(path)
	target = strings.TrimSpace(target)
	if path == "" || target == "" {
		return false, fmt.Errorf("managed config line cleanup requires an absolute path and exact line")
	}
	if err := rejectSymlinkPath(path); err != nil {
		return false, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	next, removed := removeExactManagedConfigLineContent(string(body), target)
	if !removed {
		return false, nil
	}
	if err := writeManagedFile(managedFileSpec{
		Path: path, Content: next, Mode: fmt.Sprintf("%04o", info.Mode().Perm()),
	}); err != nil {
		return false, err
	}
	return true, nil
}

func removeExactManagedConfigLineContent(content, target string) (string, bool) {
	target = strings.TrimSpace(target)
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))
	removed := false
	for _, line := range lines {
		if strings.TrimSpace(line) == target {
			removed = true
			continue
		}
		out = append(out, line)
	}
	if !removed {
		return content, false
	}
	next := strings.Join(out, "\n")
	next = strings.TrimRight(next, "\n") + "\n"
	return next, true
}

func reloadExternalEgressIPsecAfterCleanup(ctx context.Context) string {
	ipsecBinary, ok := resolveExternalEgressCleanupExecutable("ipsec")
	if !ok {
		return ""
	}
	_, _ = runInstallCommand(ctx, ipsecBinary, "rereadsecrets")
	if code, output := runInstallCommand(ctx, ipsecBinary, "reload"); code != 0 {
		return "strongSwan reload: " + firstLine(output)
	}
	return ""
}

func decodeExternalEgressJob(j job, st agentState, requireSecrets bool) (externalEgressJobPayload, error) {
	payload := externalEgressJobPayload{
		NodeID: stringify(j.Payload["node_id"]), ProfileID: strings.ToLower(stringify(j.Payload["profile_id"])),
		DeploymentID: strings.ToLower(stringify(j.Payload["deployment_id"])), Protocol: strings.ToLower(stringify(j.Payload["protocol"])),
		InterfaceName: stringify(j.Payload["interface_name"]), RoutingTable: intFromPayload(j.Payload["routing_table"], 0),
		RouteMetric: intFromPayload(j.Payload["route_metric"], 100), FWMark: intFromPayload(j.Payload["fwmark"], 0),
		ProxyPort: intFromPayload(j.Payload["proxy_port"], 0), Secrets: map[string]string{},
	}
	if payload.NodeID == "" || payload.NodeID != strings.TrimSpace(st.NodeID) {
		return payload, fmt.Errorf("external egress job node mismatch")
	}
	if !externalEgressUUIDPattern.MatchString(payload.ProfileID) || !externalEgressUUIDPattern.MatchString(payload.DeploymentID) {
		return payload, fmt.Errorf("external egress job identity is invalid")
	}
	if !externalEgressInterfacePattern.MatchString(payload.InterfaceName) {
		return payload, fmt.Errorf("external egress interface name is invalid")
	}
	if payload.RoutingTable < 40000 || payload.RoutingTable > 48999 {
		return payload, fmt.Errorf("external egress routing table is outside the managed range")
	}
	if payload.RouteMetric < 1 || payload.RouteMetric > 32767 {
		return payload, fmt.Errorf("external egress route metric is invalid")
	}
	if payload.FWMark < 0x4d590000 || payload.FWMark > 0x4d59ffff {
		return payload, fmt.Errorf("external egress fwmark is outside the managed range")
	}
	if !externalEgressRuntimeProtocol(payload.Protocol) {
		return payload, fmt.Errorf("external egress protocol %q is not runtime-ready", payload.Protocol)
	}
	if externalEgressUsesLoopbackProxyRuntime(payload.Protocol) {
		expected := externalEgressProxyPort(payload.RoutingTable)
		if payload.ProxyPort == 0 {
			payload.ProxyPort = expected
		}
		if payload.ProxyPort != expected {
			return payload, fmt.Errorf("external egress loopback proxy port does not match the managed routing resource")
		}
	}
	if rawSecrets, ok := j.Payload["secrets"].(map[string]any); ok {
		for purpose, raw := range rawSecrets {
			payload.Secrets[strings.ToLower(strings.TrimSpace(purpose))] = stringify(raw)
		}
	}
	if requireSecrets && payload.Secrets["config"] == "" {
		return payload, fmt.Errorf("external egress imported config is missing")
	}
	return payload, nil
}

func renderExternalEgressRuntime(payload externalEgressJobPayload) ([]managedFileSpec, string, error) {
	dir := externalEgressManagedDir(payload)
	routeScript := filepath.Join(dir, "route-policy.sh")
	files := []managedFileSpec{}
	if externalEgressUsesPolicyRouting(payload.Protocol) {
		files = append(files, managedFileSpec{Path: routeScript, Content: renderExternalEgressRouteScript(payload), Mode: "0700"})
	} else {
		routeScript = ""
	}
	var execStart string
	stopCommands := []string{}
	switch payload.Protocol {
	case "openvpn":
		binary, ok := resolveExecutable("openvpn")
		if !ok {
			return nil, "", fmt.Errorf("capability missing: openvpn binary is not installed or not executable")
		}
		config, extraFiles, err := renderManagedOpenVPNConfig(payload, dir)
		if err != nil {
			return nil, "", err
		}
		configPath := filepath.Join(dir, "client.ovpn")
		files = append(files, managedFileSpec{Path: configPath, Content: config, Mode: "0600"})
		files = append(files, extraFiles...)
		execStart = strings.Join([]string{binary, "--config", configPath}, " ")
		stopCommands = append(stopCommands, routeScript+" guard")
	case "wireguard":
		binary, ok := resolveExecutable("wg-quick")
		if !ok {
			return nil, "", fmt.Errorf("capability missing: wg-quick is not installed or not executable")
		}
		config, err := renderManagedWireGuardConfig(payload)
		if err != nil {
			return nil, "", err
		}
		configPath := filepath.Join(dir, payload.InterfaceName+".conf")
		files = append(files, managedFileSpec{Path: configPath, Content: config, Mode: "0600"})
		execStart = strings.Join([]string{binary, "up", configPath}, " ")
		stopCommands = append(stopCommands, routeScript+" guard", strings.Join([]string{binary, "down", configPath}, " "))
	case "shadowsocks":
		binary, ok := resolveExecutable("xray")
		if !ok {
			return nil, "", fmt.Errorf("capability missing: xray binary is not installed or not executable")
		}
		config, err := renderManagedShadowsocksXrayConfig(payload)
		if err != nil {
			return nil, "", err
		}
		configPath := filepath.Join(dir, "config.json")
		files = append(files, managedFileSpec{Path: configPath, Content: config, Mode: "0600"})
		execStart = strings.Join([]string{binary, "run", "-config", configPath}, " ")
	case "vless":
		binary, ok := resolveExecutable("xray")
		if !ok {
			return nil, "", fmt.Errorf("capability missing: xray binary is not installed or not executable")
		}
		config, err := renderManagedVLESSXrayConfig(payload)
		if err != nil {
			return nil, "", err
		}
		configPath := filepath.Join(dir, "config.json")
		files = append(files, managedFileSpec{Path: configPath, Content: config, Mode: "0600"})
		execStart = strings.Join([]string{binary, "run", "-config", configPath}, " ")
	case "l2tp_ipsec":
		ipsecBinary, ok := resolveExecutable("ipsec")
		if !ok {
			return nil, "", fmt.Errorf("capability missing: ipsec binary is not installed or not executable")
		}
		xl2tpdBinary, ok := resolveExecutable("xl2tpd")
		if !ok {
			return nil, "", fmt.Errorf("capability missing: xl2tpd binary is not installed or not executable")
		}
		flockBinary, ok := resolveExecutable("flock")
		if !ok {
			return nil, "", fmt.Errorf("capability missing: flock binary is not installed or not executable")
		}
		ipsecConfig, ipsecSecrets, xl2tpConfig, pppOptions, err := renderManagedL2TPIPsecConfig(payload)
		if err != nil {
			return nil, "", err
		}
		files = append(files,
			managedFileSpec{Path: filepath.Join(dir, "ipsec.conf"), Content: ipsecConfig, Mode: "0600"},
			managedFileSpec{Path: filepath.Join(dir, "ipsec.secrets"), Content: ipsecSecrets, Mode: "0600"},
			managedFileSpec{Path: filepath.Join(dir, "xl2tpd.conf"), Content: xl2tpConfig, Mode: "0600"},
			managedFileSpec{Path: filepath.Join(dir, "ppp-options"), Content: pppOptions, Mode: "0600"},
		)
		preview, parseErr := externalegress.ParseL2TPIPsec([]byte(payload.Secrets["config"]))
		if parseErr != nil {
			return nil, "", parseErr
		}
		if preview.AuthMethod == "certificate" {
			material, materialErr := renderManagedL2TPIPsecCertificateMaterial(payload)
			if materialErr != nil {
				return nil, "", materialErr
			}
			files = append(files, material.Files...)
		}
		startScript := renderManagedL2TPIPsecStartScript(payload, ipsecBinary, xl2tpdBinary, flockBinary)
		startPath := filepath.Join(dir, "start.sh")
		files = append(files, managedFileSpec{Path: startPath, Content: startScript, Mode: "0700"})
		execStart = startPath
		stopCommands = append(stopCommands, routeScript+" guard", ipsecBinary+" down "+externalEgressIPsecConnectionName(payload))
	default:
		return nil, "", fmt.Errorf("external egress protocol %q is not runtime-ready", payload.Protocol)
	}
	unit := renderExternalEgressUnit(payload, routeScript, execStart, stopCommands)
	return files, unit, nil
}

func validateExternalEgressManagedArtifacts(payload externalEgressJobPayload, files []managedFileSpec, unitPath string) error {
	dir := cleanAbsPath(externalEgressManagedDir(payload))
	if dir == "" || dir != externalEgressManagedDir(payload) {
		return fmt.Errorf("external egress managed directory is invalid")
	}
	if cleanAbsPath(unitPath) != externalEgressUnitPath(payload) {
		return fmt.Errorf("external egress systemd unit path is invalid")
	}
	allowed := map[string]string{}
	if externalEgressUsesPolicyRouting(payload.Protocol) {
		allowed[filepath.Join(dir, "route-policy.sh")] = "0700"
	}
	switch payload.Protocol {
	case "openvpn":
		for _, name := range []string{
			"client.ovpn", "auth.txt", "ca_certificate.pem", "certificate.pem", "private_key.pem",
			"client.p12", "tls_auth_key.pem", "tls_crypt_key.pem", "tls_crypt_v2_key.pem", "static_key.pem",
		} {
			allowed[filepath.Join(dir, name)] = "0600"
		}
	case "wireguard":
		allowed[filepath.Join(dir, payload.InterfaceName+".conf")] = "0600"
	case "vless", "shadowsocks":
		allowed[filepath.Join(dir, "config.json")] = "0600"
	case "l2tp_ipsec":
		allowed[filepath.Join(dir, "ipsec.conf")] = "0600"
		allowed[filepath.Join(dir, "ipsec.secrets")] = "0600"
		allowed[filepath.Join(dir, "xl2tpd.conf")] = "0600"
		allowed[filepath.Join(dir, "ppp-options")] = "0600"
		allowed[filepath.Join(dir, "start.sh")] = "0700"
		for _, path := range externalEgressIPsecCertificatePaths(payload) {
			allowed[path] = "0600"
		}
	default:
		return fmt.Errorf("external egress protocol %q has no managed artifact allowlist", payload.Protocol)
	}
	seen := make(map[string]struct{}, len(files))
	for _, file := range files {
		path := cleanAbsPath(file.Path)
		expectedMode, ok := allowed[path]
		if !ok || path != file.Path {
			return fmt.Errorf("external egress artifact path is outside the deployment allowlist: %s", file.Path)
		}
		if file.Mode != expectedMode {
			return fmt.Errorf("external egress artifact %s has unsafe mode %s", file.Path, file.Mode)
		}
		if _, duplicate := seen[path]; duplicate {
			return fmt.Errorf("external egress artifact path is duplicated: %s", file.Path)
		}
		seen[path] = struct{}{}
	}
	return nil
}

func renderManagedOpenVPNConfig(payload externalEgressJobPayload, dir string) (string, []managedFileSpec, error) {
	raw := payload.Secrets["config"]
	preview, err := externalegress.ParseOpenVPN([]byte(raw))
	if err != nil {
		return "", nil, err
	}
	extra := []managedFileSpec{}
	replacements := map[string]string{}
	for directive, purpose := range map[string]string{
		"ca": "ca_certificate", "cert": "certificate", "key": "private_key", "pkcs12": "pkcs12",
		"tls-auth": "tls_auth_key", "tls-crypt": "tls_crypt_key", "tls-crypt-v2": "tls_crypt_v2_key", "secret": "static_key",
	} {
		if value := payload.Secrets[purpose]; value != "" {
			path := filepath.Join(dir, purpose+".pem")
			if purpose == "pkcs12" {
				path = filepath.Join(dir, "client.p12")
			}
			replacements[directive] = path
			extra = append(extra, managedFileSpec{Path: path, Content: value, Mode: "0600"})
		}
	}
	inlineAuth := false
	for _, block := range preview.InlineBlocks {
		if block == "auth-user-pass" {
			inlineAuth = true
			break
		}
	}
	if preview.AuthUserPass && !inlineAuth {
		username, password := payload.Secrets["username"], payload.Secrets["password"]
		if username == "" || password == "" || strings.ContainsAny(username+password, "\r\n\x00") {
			return "", nil, fmt.Errorf("OpenVPN profile requires separate username and password secrets")
		}
		authPath := filepath.Join(dir, "auth.txt")
		replacements["auth-user-pass"] = authPath
		extra = append(extra, managedFileSpec{Path: authPath, Content: username + "\n" + password + "\n", Mode: "0600"})
	}

	var out strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(raw))
	inline := ""
	skipInline := false
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "<") && !strings.HasPrefix(trimmed, "</") {
			inline = strings.ToLower(strings.Trim(strings.Fields(trimmed)[0], "<>/"))
			skipInline = replacements[inline] != ""
		}
		if inline != "" {
			if !skipInline {
				out.WriteString(line + "\n")
			}
			if strings.HasPrefix(strings.ToLower(trimmed), "</"+inline+">") {
				inline = ""
				skipInline = false
			}
			continue
		}
		fields, splitErr := splitExternalOpenVPNLine(trimmed)
		if splitErr != nil {
			return "", nil, splitErr
		}
		if len(fields) == 0 {
			out.WriteString(line + "\n")
			continue
		}
		directive := strings.ToLower(strings.TrimLeft(fields[0], "-"))
		switch directive {
		case "dev", "dev-type", "route", "route-ipv6", "redirect-gateway", "redirect-private", "route-nopull", "route-noexec", "auth-nocache":
			continue
		}
		if path, ok := replacements[directive]; ok {
			extraArgs := ""
			if len(fields) > 2 {
				if len(fields) != 3 || (fields[2] != "0" && fields[2] != "1") {
					return "", nil, fmt.Errorf("OpenVPN directive %q has unsupported managed key arguments", directive)
				}
				extraArgs = " " + fields[2]
			}
			out.WriteString(directive + " " + path + extraArgs + "\n")
			continue
		}
		if _, managedPath := map[string]struct{}{"ca": {}, "cert": {}, "key": {}, "pkcs12": {}, "tls-auth": {}, "tls-crypt": {}, "tls-crypt-v2": {}, "secret": {}, "auth-user-pass": {}}[directive]; managedPath && len(fields) > 1 {
			if fields[1] == "[inline]" {
				out.WriteString(line + "\n")
				continue
			}
			return "", nil, fmt.Errorf("OpenVPN directive %q requires a separately uploaded managed secret", directive)
		}
		out.WriteString(line + "\n")
	}
	if err := scanner.Err(); err != nil {
		return "", nil, err
	}
	out.WriteString("\ndev " + payload.InterfaceName + "\ndev-type tun\nroute-nopull\nroute-noexec\nauth-nocache\n")
	if authPath := replacements["auth-user-pass"]; authPath != "" {
		out.WriteString("auth-user-pass " + authPath + "\n")
	}
	return out.String(), extra, nil
}

func splitExternalOpenVPNLine(line string) ([]string, error) {
	if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
		return nil, nil
	}
	return strings.Fields(line), nil
}

func renderManagedWireGuardConfig(payload externalEgressJobPayload) (string, error) {
	raw := payload.Secrets["config"]
	preview, err := externalegress.ParseWireGuard([]byte(raw))
	if err != nil {
		return "", err
	}
	defaultRoute := false
	for _, cidr := range preview.AllowedIPs {
		if cidr == "0.0.0.0/0" {
			defaultRoute = true
		}
	}
	if !defaultRoute {
		return "", fmt.Errorf("WireGuard external egress requires AllowedIPs to include 0.0.0.0/0")
	}
	var out strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(raw))
	inInterface := false
	tableWritten := false
	privateKeyWritten := false
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if strings.EqualFold(trimmed, "[Interface]") {
			inInterface = true
			out.WriteString(line + "\n")
			continue
		}
		if strings.HasPrefix(trimmed, "[") {
			if inInterface && !tableWritten {
				out.WriteString("Table = " + strconv.Itoa(payload.RoutingTable) + "\n")
				tableWritten = true
			}
			if inInterface && !privateKeyWritten {
				privateKey := strings.TrimSpace(payload.Secrets["private_key"])
				if privateKey == "" || strings.ContainsAny(privateKey, "\r\n\x00") {
					return "", fmt.Errorf("WireGuard profile requires a valid private key secret")
				}
				out.WriteString("PrivateKey = " + privateKey + "\n")
				privateKeyWritten = true
			}
			inInterface = false
		}
		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) == 2 {
			key := strings.ToLower(strings.TrimSpace(parts[0]))
			if key == "table" || key == "dns" {
				continue
			}
			if inInterface && key == "privatekey" {
				privateKey := strings.TrimSpace(payload.Secrets["private_key"])
				if privateKey == "" {
					privateKey = strings.TrimSpace(parts[1])
				}
				if privateKey == "" || strings.ContainsAny(privateKey, "\r\n\x00") {
					return "", fmt.Errorf("WireGuard profile requires a valid private key")
				}
				out.WriteString("PrivateKey = " + privateKey + "\n")
				privateKeyWritten = true
				continue
			}
			if !inInterface && key == "presharedkey" && strings.TrimSpace(payload.Secrets["preshared_key"]) != "" {
				presharedKey := strings.TrimSpace(payload.Secrets["preshared_key"])
				if strings.ContainsAny(presharedKey, "\r\n\x00") {
					return "", fmt.Errorf("WireGuard pre-shared key secret is invalid")
				}
				out.WriteString("PresharedKey = " + presharedKey + "\n")
				continue
			}
		}
		out.WriteString(line + "\n")
	}
	if inInterface && !tableWritten {
		out.WriteString("Table = " + strconv.Itoa(payload.RoutingTable) + "\n")
	}
	if inInterface && !privateKeyWritten {
		privateKey := strings.TrimSpace(payload.Secrets["private_key"])
		if privateKey == "" || strings.ContainsAny(privateKey, "\r\n\x00") {
			return "", fmt.Errorf("WireGuard profile requires a valid private key secret")
		}
		out.WriteString("PrivateKey = " + privateKey + "\n")
	}
	return out.String(), scanner.Err()
}

func renderManagedShadowsocksXrayConfig(payload externalEgressJobPayload) (string, error) {
	preview, err := externalegress.ParseShadowsocks([]byte(payload.Secrets["config"]))
	if err != nil {
		return "", err
	}
	password := firstNonEmptyAgentSecret(payload.Secrets["password"], preview.Password)
	if password == "" || strings.ContainsAny(password, "\r\n\x00") {
		return "", fmt.Errorf("Shadowsocks profile requires a valid password secret")
	}
	config := map[string]any{
		"log":      map[string]any{"loglevel": "warning"},
		"inbounds": []any{externalEgressSOCKSInbound(payload.ProxyPort)},
		"outbounds": []any{
			map[string]any{
				"tag": "provider-out", "protocol": "shadowsocks",
				"settings": map[string]any{"servers": []any{map[string]any{
					"address": preview.EndpointHost, "port": preview.EndpointPort,
					"method": preview.Method, "password": password,
				}}},
			},
			map[string]any{"tag": "block", "protocol": "blackhole"},
		},
		"routing": map[string]any{"domainStrategy": "AsIs", "rules": []any{}},
	}
	return marshalExternalEgressXrayConfig(config)
}

func renderManagedVLESSXrayConfig(payload externalEgressJobPayload) (string, error) {
	preview, err := externalegress.ParseVLESS([]byte(payload.Secrets["config"]))
	if err != nil {
		return "", err
	}
	uuid := firstNonEmptyAgentString(strings.TrimSpace(payload.Secrets["uuid"]), preview.UUID)
	if !externalEgressUUIDPattern.MatchString(strings.ToLower(uuid)) {
		return "", fmt.Errorf("VLESS profile requires a valid UUID secret")
	}
	user := map[string]any{"id": uuid, "encryption": firstNonEmptyAgentString(preview.Encryption, "none"), "level": 0}
	if preview.Flow != "" {
		user["flow"] = preview.Flow
	}
	stream := map[string]any{"network": preview.Transport, "security": preview.Security}
	switch preview.Security {
	case "tls":
		tls := map[string]any{"serverName": preview.ServerName, "allowInsecure": false}
		if preview.Fingerprint != "" {
			tls["fingerprint"] = preview.Fingerprint
		}
		if len(preview.ALPN) > 0 {
			tls["alpn"] = preview.ALPN
		}
		stream["tlsSettings"] = tls
	case "reality":
		reality := map[string]any{
			"serverName": preview.ServerName, "fingerprint": firstNonEmptyAgentString(preview.Fingerprint, "chrome"),
			"password": preview.PublicKey, "shortId": preview.ShortID,
		}
		if preview.SpiderX != "" {
			reality["spiderX"] = preview.SpiderX
		}
		stream["realitySettings"] = reality
	default:
		return "", fmt.Errorf("managed VLESS profile requires TLS or REALITY")
	}
	switch preview.Transport {
	case "ws":
		settings := map[string]any{"path": firstNonEmptyAgentString(preview.Path, "/")}
		if preview.Host != "" {
			settings["headers"] = map[string]any{"Host": preview.Host}
		}
		stream["wsSettings"] = settings
	case "grpc":
		stream["grpcSettings"] = map[string]any{"serviceName": preview.ServiceName}
	case "httpupgrade":
		settings := map[string]any{"path": firstNonEmptyAgentString(preview.Path, "/")}
		if preview.Host != "" {
			settings["host"] = preview.Host
		}
		stream["httpupgradeSettings"] = settings
	case "xhttp":
		settings := map[string]any{"path": firstNonEmptyAgentString(preview.Path, "/")}
		if preview.Host != "" {
			settings["host"] = preview.Host
		}
		stream["xhttpSettings"] = settings
	case "tcp":
	default:
		return "", fmt.Errorf("unsupported managed VLESS transport %q", preview.Transport)
	}
	config := map[string]any{
		"log":      map[string]any{"loglevel": "warning"},
		"inbounds": []any{externalEgressSOCKSInbound(payload.ProxyPort)},
		"outbounds": []any{
			map[string]any{
				"tag": "provider-out", "protocol": "vless",
				"settings": map[string]any{"vnext": []any{map[string]any{
					"address": preview.EndpointHost, "port": preview.EndpointPort, "users": []any{user},
				}}},
				"streamSettings": stream,
			},
			map[string]any{"tag": "block", "protocol": "blackhole"},
		},
		"routing": map[string]any{"domainStrategy": "AsIs", "rules": []any{}},
	}
	return marshalExternalEgressXrayConfig(config)
}

func externalEgressSOCKSInbound(port int) map[string]any {
	return map[string]any{
		"tag": "provider-in", "listen": "127.0.0.1", "port": port, "protocol": "socks",
		"settings": map[string]any{"auth": "noauth", "udp": true, "ip": "127.0.0.1"},
		"sniffing": map[string]any{"enabled": true, "destOverride": []string{"http", "tls", "quic"}},
	}
}

func marshalExternalEgressXrayConfig(config map[string]any) (string, error) {
	raw, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", fmt.Errorf("render external egress Xray config: %w", err)
	}
	return string(raw) + "\n", nil
}

func renderManagedL2TPIPsecConfig(payload externalEgressJobPayload) (string, string, string, string, error) {
	preview, err := externalegress.ParseL2TPIPsec([]byte(payload.Secrets["config"]))
	if err != nil {
		return "", "", "", "", err
	}
	username := firstNonEmptyAgentSecret(payload.Secrets["username"], preview.Username)
	password := firstNonEmptyAgentSecret(payload.Secrets["password"], preview.Password)
	if err := validateExternalEgressCredential("L2TP username", username); err != nil {
		return "", "", "", "", err
	}
	if err := validateExternalEgressCredential("L2TP password", password); err != nil {
		return "", "", "", "", err
	}
	dir := externalEgressManagedDir(payload)
	connection := externalEgressIPsecConnectionName(payload)
	remoteID := firstNonEmptyAgentString(preview.RemoteID, preview.EndpointHost)
	authConfig := ""
	ipsecSecrets := ""
	switch preview.AuthMethod {
	case "psk":
		psk := firstNonEmptyAgentSecret(payload.Secrets["preshared_key"], preview.PSK)
		if err := validateExternalEgressCredential("IPsec pre-shared key", psk); err != nil {
			return "", "", "", "", err
		}
		authConfig = "    authby=psk\n"
		ipsecSecrets = fmt.Sprintf("%%any %s : PSK %s\n", remoteID, quoteExternalEgressSecret(psk))
	case "certificate":
		material, err := renderManagedL2TPIPsecCertificateMaterial(payload)
		if err != nil {
			return "", "", "", "", err
		}
		authConfig = fmt.Sprintf("    leftauth=pubkey\n    leftcert=%s\n    rightauth=pubkey\n    rightca=%s\n",
			material.CertificatePath, quoteExternalEgressSecret(material.CASubject))
		ipsecSecrets = fmt.Sprintf(": %s %s\n", material.PrivateKeyType, material.PrivateKeyPath)
	default:
		return "", "", "", "", fmt.Errorf("unsupported L2TP/IPsec authentication method")
	}
	ipsecConfig := fmt.Sprintf(`conn %s
    type=transport
    keyexchange=ikev1
%s
    left=%%defaultroute
    leftprotoport=17/1701
    right=%s
    rightid=%s
    rightprotoport=17/1701
    ike=%s!
    esp=%s!
    dpdaction=clear
    dpddelay=30s
    keyingtries=3
    auto=add
`, connection, strings.TrimSuffix(authConfig, "\n"), preview.EndpointHost, remoteID, preview.IKEProposal, preview.ESPProposal)
	xl2tpConfig := fmt.Sprintf(`[global]
port = 1701

[lac %s]
lns = %s
pppoptfile = %s
autodial = yes
redial = yes
redial timeout = 5
max redials = 0
length bit = yes
`, connection, preview.EndpointHost, filepath.Join(dir, "ppp-options"))
	pppOptions := fmt.Sprintf(`ifname %s
ipparam %s
linkname %s
name %s
password %s
noauth
nodefaultroute
refuse-eap
noccp
mtu 1400
mru 1400
persist
maxfail 0
holdoff 5
`, payload.InterfaceName, payload.InterfaceName, payload.InterfaceName,
		quoteExternalEgressSecret(username), quoteExternalEgressSecret(password))
	return ipsecConfig, ipsecSecrets, xl2tpConfig, pppOptions, nil
}

type managedL2TPIPsecCertificateMaterial struct {
	Files           []managedFileSpec
	CertificatePath string
	PrivateKeyPath  string
	PrivateKeyType  string
	CASubject       string
}

func renderManagedL2TPIPsecCertificateMaterial(payload externalEgressJobPayload) (managedL2TPIPsecCertificateMaterial, error) {
	var material managedL2TPIPsecCertificateMaterial
	caPEM := payload.Secrets["ca_certificate"]
	certificatePEM := payload.Secrets["certificate"]
	privateKeyPEM := payload.Secrets["private_key"]
	if caPEM == "" || certificatePEM == "" || privateKeyPEM == "" {
		return material, fmt.Errorf("certificate authentication requires CA certificate, client certificate and private key")
	}
	validated, err := externalegress.ValidateCertificateMaterial(caPEM, certificatePEM, privateKeyPEM)
	if err != nil {
		return material, err
	}
	paths := externalEgressIPsecCertificatePaths(payload)
	material.CertificatePath = paths[1]
	material.PrivateKeyPath = paths[2]
	material.PrivateKeyType = validated.PrivateKeyType
	material.CASubject = validated.CASubject
	material.Files = []managedFileSpec{
		{Path: paths[0], Content: caPEM, Mode: "0600"},
		{Path: paths[1], Content: certificatePEM, Mode: "0600"},
		{Path: paths[2], Content: privateKeyPEM, Mode: "0600"},
	}
	return material, nil
}

func externalEgressIPsecCertificatePaths(payload externalEgressJobPayload) []string {
	compact := strings.ReplaceAll(strings.ToLower(payload.DeploymentID), "-", "")
	if len(compact) < 12 {
		sum := sha256.Sum256([]byte(payload.DeploymentID))
		compact = hex.EncodeToString(sum[:])
	}
	prefix := "megavpn-" + compact[:12]
	return []string{
		filepath.Join("/etc/ipsec.d/cacerts", prefix+"-ca.pem"),
		filepath.Join("/etc/ipsec.d/certs", prefix+"-client.pem"),
		filepath.Join("/etc/ipsec.d/private", prefix+"-client.key"),
	}
}

func renderManagedL2TPIPsecStartScript(payload externalEgressJobPayload, ipsecBinary, xl2tpdBinary, flockBinary string) string {
	dir := externalEgressManagedDir(payload)
	connection := externalEgressIPsecConnectionName(payload)
	runtimeDir := externalEgressRuntimeDir(payload)
	rereadCAs := ""
	if preview, err := externalegress.ParseL2TPIPsec([]byte(payload.Secrets["config"])); err == nil && preview.AuthMethod == "certificate" {
		rereadCAs = ipsecBinary + " rereadcacerts"
	}
	return fmt.Sprintf(`#!/bin/sh
set -eu
umask 077
mkdir -p %s
exec 9>/run/lock/megavpn-external-ipsec.lock
%s -x 9
touch /etc/ipsec.conf /etc/ipsec.secrets
grep -Fqx 'include /etc/megavpn/external-egress/*/ipsec.conf' /etc/ipsec.conf || printf '\ninclude /etc/megavpn/external-egress/*/ipsec.conf\n' >>/etc/ipsec.conf
grep -Fqx 'include /etc/megavpn/external-egress/*/ipsec.secrets' /etc/ipsec.secrets || printf '\ninclude /etc/megavpn/external-egress/*/ipsec.secrets\n' >>/etc/ipsec.secrets
%s -u 9
exec 9>&-
systemctl start strongswan-starter 2>/dev/null || systemctl start strongswan 2>/dev/null || true
%s
%s rereadsecrets
%s reload
%s up %s
exec %s -D -c %s -p %s/xl2tpd.pid -C %s/xl2tpd.control
`, runtimeDir, flockBinary, flockBinary, rereadCAs, ipsecBinary, ipsecBinary, ipsecBinary, connection,
		xl2tpdBinary, filepath.Join(dir, "xl2tpd.conf"), runtimeDir, runtimeDir)
}

func externalEgressIPsecConnectionName(payload externalEgressJobPayload) string {
	compact := strings.ReplaceAll(strings.ToLower(payload.DeploymentID), "-", "")
	if len(compact) < 12 {
		sum := sha256.Sum256([]byte(payload.DeploymentID))
		compact = hex.EncodeToString(sum[:])
	}
	return "mgev-" + compact[:12]
}

func validateExternalEgressCredential(label, value string) error {
	if value == "" || len(value) > 4096 || strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return fmt.Errorf("%s secret is missing or invalid", label)
	}
	return nil
}

func firstNonEmptyAgentSecret(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func quoteExternalEgressSecret(value string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value) + `"`
}

func renderExternalEgressRouteScript(payload externalEgressJobPayload) string {
	mark := fmt.Sprintf("0x%x", payload.FWMark)
	priority := externalEgressRulePriority(payload.RoutingTable)
	return fmt.Sprintf(`#!/bin/sh
set -eu
action="${1:-apply}"
if [ "$action" = cleanup ]; then
  ip rule del pref %d fwmark %s lookup %d 2>/dev/null || true
  ip route flush table %d 2>/dev/null || true
  exit 0
fi
ip route flush table %d 2>/dev/null || true
ip route replace unreachable default metric 32767 table %d
ip rule del pref %d fwmark %s lookup %d 2>/dev/null || true
ip rule add pref %d fwmark %s lookup %d
if [ "$action" = guard ]; then
  exit 0
fi
i=0
while ! ip link show dev %s >/dev/null 2>&1; do
  i=$((i + 1))
  [ "$i" -lt 60 ] || { echo "interface %s did not appear" >&2; exit 1; }
  sleep 1
done
%s
ip route replace default dev %s table %d metric %d
`, priority, mark, payload.RoutingTable, payload.RoutingTable,
		payload.RoutingTable, payload.RoutingTable, priority, mark, payload.RoutingTable, priority, mark, payload.RoutingTable,
		payload.InterfaceName, payload.InterfaceName, externalEgressInterfaceAddressWait(payload),
		payload.InterfaceName, payload.RoutingTable, payload.RouteMetric)
}

func externalEgressInterfaceAddressWait(payload externalEgressJobPayload) string {
	if payload.Protocol != "l2tp_ipsec" {
		return ""
	}
	return fmt.Sprintf(`i=0
while ! ip -4 addr show dev %s | grep -q 'inet '; do
  i=$((i + 1))
  [ "$i" -lt 60 ] || { echo "interface %s did not receive an IPv4 address" >&2; exit 1; }
  sleep 1
done`, payload.InterfaceName, payload.InterfaceName)
}

func externalEgressRulePriority(routingTable int) int {
	return routingTable + 10000
}

func renderExternalEgressUnit(payload externalEgressJobPayload, routeScript, execStart string, stopCommands []string) string {
	typeName := "simple"
	remain := ""
	if payload.Protocol == "wireguard" {
		typeName, remain = "oneshot", "RemainAfterExit=yes\n"
	}
	var stops strings.Builder
	for _, command := range stopCommands {
		stops.WriteString("ExecStop=" + command + "\n")
	}
	postStart := ""
	if routeScript != "" {
		postStart = "ExecStartPost=" + routeScript + " apply\n"
	}
	return fmt.Sprintf(`[Unit]
Description=MegaVPN external egress %s
After=network-online.target
Wants=network-online.target

[Service]
Type=%s
%sExecStart=%s
%s%sRestart=%s
RestartSec=3s
KillMode=control-group
TimeoutStartSec=120s
TimeoutStopSec=30s

[Install]
WantedBy=multi-user.target
`, payload.DeploymentID, typeName, remain, execStart, postStart, stops.String(), map[bool]string{true: "no", false: "on-failure"}[payload.Protocol == "wireguard"])
}

func cleanupExternalEgressPolicy(ctx context.Context, payload externalEgressJobPayload) error {
	script := filepath.Join(externalEgressManagedDir(payload), "route-policy.sh")
	if info, err := os.Stat(script); err == nil && info.Mode()&0o111 != 0 {
		if code, output := runInstallCommand(ctx, script, "cleanup"); code != 0 {
			if isMissingIPRouteTableOutput(output) {
				return nil
			}
			return fmt.Errorf("external egress policy cleanup failed: %s", firstLine(output))
		}
	} else {
		priority := externalEgressRulePriority(payload.RoutingTable)
		_, _ = runInstallCommand(ctx, "ip", "rule", "del", "pref", strconv.Itoa(priority), "fwmark", fmt.Sprintf("0x%x", payload.FWMark), "lookup", strconv.Itoa(payload.RoutingTable))
		if code, output := runInstallCommand(ctx, "ip", "route", "flush", "table", strconv.Itoa(payload.RoutingTable)); code != 0 && !isMissingIPRouteTableOutput(output) {
			return fmt.Errorf("external egress route cleanup failed: %s", firstLine(output))
		}
	}
	code, output := runInstallCommand(ctx, "ip", "rule", "show")
	if code != 0 {
		return fmt.Errorf("verify external egress policy cleanup: %s", firstLine(output))
	}
	rules := strings.ToLower(output)
	mark := strings.ToLower(fmt.Sprintf("0x%x", payload.FWMark))
	if strings.Contains(rules, mark) && strings.Contains(rules, strconv.Itoa(payload.RoutingTable)) {
		return fmt.Errorf("external egress policy rule is still present after cleanup")
	}
	code, output = runInstallCommand(ctx, "ip", "route", "show", "table", strconv.Itoa(payload.RoutingTable))
	if code != 0 && !isMissingIPRouteTableOutput(output) {
		return fmt.Errorf("verify external egress route cleanup: %s", firstLine(output))
	}
	if code == 0 && strings.TrimSpace(output) != "" {
		return fmt.Errorf("external egress routing table is not empty after cleanup")
	}
	return nil
}

func isMissingIPRouteTableOutput(output string) bool {
	normalized := strings.ToLower(strings.TrimSpace(output))
	return strings.Contains(normalized, "fib table does not exist") ||
		strings.Contains(normalized, "routing table does not exist")
}

func installExternalEgressFailClosedGuard(ctx context.Context, payload externalEgressJobPayload) error {
	table := strconv.Itoa(payload.RoutingTable)
	priority := strconv.Itoa(externalEgressRulePriority(payload.RoutingTable))
	mark := fmt.Sprintf("0x%x", payload.FWMark)
	if code, output := runInstallCommand(ctx, "ip", "route", "flush", "table", table); code != 0 && !isMissingIPRouteTableOutput(output) {
		return fmt.Errorf("flush external egress routing table: %s", firstLine(output))
	}
	if code, output := runInstallCommand(ctx, "ip", "route", "replace", "unreachable", "default", "metric", "32767", "table", table); code != 0 {
		return fmt.Errorf("install unreachable provider fallback: %s", firstLine(output))
	}
	_, _ = runInstallCommand(ctx, "ip", "rule", "del", "pref", priority, "fwmark", mark, "lookup", table)
	if code, output := runInstallCommand(ctx, "ip", "rule", "add", "pref", priority, "fwmark", mark, "lookup", table); code != 0 {
		return fmt.Errorf("install external egress policy rule: %s", firstLine(output))
	}
	return nil
}

func externalEgressManagedDir(payload externalEgressJobPayload) string {
	return filepath.Join(externalEgressManagedRoot, payload.DeploymentID)
}

func externalEgressRuntimeDir(payload externalEgressJobPayload) string {
	return filepath.Join("/run/megavpn/external-egress", strings.ReplaceAll(payload.DeploymentID, "-", ""))
}

func externalEgressUnitName(payload externalEgressJobPayload) string {
	return "megavpn-external-egress-" + strings.ReplaceAll(payload.DeploymentID, "-", "") + ".service"
}

func externalEgressUnitPath(payload externalEgressJobPayload) string {
	return filepath.Join("/etc/systemd/system", externalEgressUnitName(payload))
}
