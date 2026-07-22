package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/rtis-emc2/megavpn/internal/externalegress"
)

const externalEgressManagedRoot = "/etc/megavpn/external-egress"

var (
	externalEgressUUIDPattern      = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	externalEgressInterfacePattern = regexp.MustCompile(`^mgev[0-9a-f]{10}$`)
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
	if err := installExternalEgressFailClosedGuard(ctx, payload); err != nil {
		return "failed", map[string]any{"error": err.Error(), "stage": "install_fail_closed_guard", "unit": unitName}
	}
	for _, file := range files {
		if err := writeManagedFile(file); err != nil {
			return "failed", map[string]any{"error": err.Error(), "stage": "write", "path": file.Path}
		}
	}
	if err := writeManagedFile(managedFileSpec{Path: unitPath, Content: unit, Mode: "0644"}); err != nil {
		return "failed", map[string]any{"error": err.Error(), "stage": "write_unit"}
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
	return c.probeExternalEgress(ctx, j, st)
}

func validateExternalEgressRuntimeConfig(payload externalEgressJobPayload) error {
	switch payload.Protocol {
	case "openvpn":
		_, _, err := renderManagedOpenVPNConfig(payload, externalEgressManagedDir(payload))
		return err
	case "wireguard":
		_, err := renderManagedWireGuardConfig(payload)
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
	default:
		return map[string]any{"ok": false, "message": "external egress protocol runtime is not supported"}
	}
	return result
}

func (c *client) probeExternalEgress(ctx context.Context, j job, st agentState) (string, map[string]any) {
	payload, err := decodeExternalEgressJob(j, st, false)
	if err != nil {
		return "failed", map[string]any{"error": err.Error(), "stage": "validate"}
	}
	unitName := externalEgressUnitName(payload)
	unitState := normalizeSystemctlState(strings.TrimSpace(runOutput("systemctl", "is-active", unitName)))
	interfaceState := strings.TrimSpace(runOutput("ip", "-o", "link", "show", "dev", payload.InterfaceName))
	rules := runOutput("ip", "rule", "show")
	mark := fmt.Sprintf("0x%x", payload.FWMark)
	rulePresent := strings.Contains(strings.ToLower(rules), strings.ToLower(mark)) && strings.Contains(rules, strconv.Itoa(payload.RoutingTable))
	routes := strings.TrimSpace(runOutput("ip", "route", "show", "table", strconv.Itoa(payload.RoutingTable)))
	providerRoutePresent, failClosedRoutePresent := externalEgressRouteHealth(routes, payload.InterfaceName)
	health := map[string]any{
		"status": "active", "unit": unitName, "unit_state": unitState,
		"interface": payload.InterfaceName, "interface_present": interfaceState != "",
		"routing_table": payload.RoutingTable, "route_present": providerRoutePresent,
		"provider_default_route_present": providerRoutePresent, "fail_closed_route_present": failClosedRoutePresent,
		"fwmark": mark, "rule_present": rulePresent,
	}
	if unitState != "active" || interfaceState == "" || !providerRoutePresent || !rulePresent {
		health["status"] = "failed"
		return "failed", map[string]any{
			"error": "external egress runtime is incomplete", "message": "unit, interface, route and policy rule must all be active",
			"health": health,
		}
	}
	return "succeeded", map[string]any{"message": "external egress runtime is active", "health": health}
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
	if code, output := runInstallCommand(ctx, "systemctl", "disable", "--now", unitName); code != 0 && !isMissingSystemdUnitOutput(output) {
		warnings = append(warnings, "systemd stop: "+firstLine(output))
	}
	if err := cleanupExternalEgressPolicy(ctx, payload); err != nil {
		warnings = append(warnings, err.Error())
	}
	paths := []string{externalEgressUnitPath(payload), externalEgressManagedDir(payload)}
	for index, path := range paths {
		if _, err := removeManagedPath(path, index == 1); err != nil && !os.IsNotExist(err) {
			warnings = append(warnings, path+": "+err.Error())
		}
	}
	if _, err := runSystemdDaemonReload(ctx); err != nil {
		warnings = append(warnings, err.Error())
	}
	result := map[string]any{"message": "external egress runtime removed", "unit": unitName, "warnings": warnings, "health": map[string]any{"status": "inactive"}}
	if len(warnings) > 0 {
		result["error"] = strings.Join(warnings, "; ")
		return "failed", result
	}
	return "succeeded", result
}

func decodeExternalEgressJob(j job, st agentState, requireSecrets bool) (externalEgressJobPayload, error) {
	payload := externalEgressJobPayload{
		NodeID: stringify(j.Payload["node_id"]), ProfileID: strings.ToLower(stringify(j.Payload["profile_id"])),
		DeploymentID: strings.ToLower(stringify(j.Payload["deployment_id"])), Protocol: strings.ToLower(stringify(j.Payload["protocol"])),
		InterfaceName: stringify(j.Payload["interface_name"]), RoutingTable: intFromPayload(j.Payload["routing_table"], 0),
		RouteMetric: intFromPayload(j.Payload["route_metric"], 100), FWMark: intFromPayload(j.Payload["fwmark"], 0), Secrets: map[string]string{},
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
	if payload.Protocol != "openvpn" && payload.Protocol != "wireguard" {
		return payload, fmt.Errorf("external egress protocol %q is not runtime-ready", payload.Protocol)
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
	files := []managedFileSpec{{Path: routeScript, Content: renderExternalEgressRouteScript(payload), Mode: "0700"}}
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
	allowed := map[string]string{
		filepath.Join(dir, "route-policy.sh"): "0700",
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
ip route replace default dev %s table %d metric %d
	`, priority, mark, payload.RoutingTable, payload.RoutingTable,
		payload.RoutingTable, payload.RoutingTable, priority, mark, payload.RoutingTable, priority, mark, payload.RoutingTable,
		payload.InterfaceName, payload.InterfaceName, payload.InterfaceName, payload.RoutingTable, payload.RouteMetric)
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
	return fmt.Sprintf(`[Unit]
Description=MegaVPN external egress %s
After=network-online.target
Wants=network-online.target

[Service]
Type=%s
%sExecStart=%s
ExecStartPost=%s apply
%sRestart=%s
RestartSec=3s

[Install]
WantedBy=multi-user.target
`, payload.DeploymentID, typeName, remain, execStart, routeScript, stops.String(), map[bool]string{true: "no", false: "on-failure"}[payload.Protocol == "wireguard"])
}

func cleanupExternalEgressPolicy(ctx context.Context, payload externalEgressJobPayload) error {
	script := filepath.Join(externalEgressManagedDir(payload), "route-policy.sh")
	if info, err := os.Stat(script); err == nil && info.Mode()&0o111 != 0 {
		if code, output := runInstallCommand(ctx, script, "cleanup"); code != 0 {
			return fmt.Errorf("external egress policy cleanup failed: %s", firstLine(output))
		}
	} else {
		priority := externalEgressRulePriority(payload.RoutingTable)
		_, _ = runInstallCommand(ctx, "ip", "rule", "del", "pref", strconv.Itoa(priority), "fwmark", fmt.Sprintf("0x%x", payload.FWMark), "lookup", strconv.Itoa(payload.RoutingTable))
		if code, output := runInstallCommand(ctx, "ip", "route", "flush", "table", strconv.Itoa(payload.RoutingTable)); code != 0 {
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
	if code != 0 {
		return fmt.Errorf("verify external egress route cleanup: %s", firstLine(output))
	}
	if strings.TrimSpace(output) != "" {
		return fmt.Errorf("external egress routing table is not empty after cleanup")
	}
	return nil
}

func installExternalEgressFailClosedGuard(ctx context.Context, payload externalEgressJobPayload) error {
	table := strconv.Itoa(payload.RoutingTable)
	priority := strconv.Itoa(externalEgressRulePriority(payload.RoutingTable))
	mark := fmt.Sprintf("0x%x", payload.FWMark)
	if code, output := runInstallCommand(ctx, "ip", "route", "flush", "table", table); code != 0 {
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

func externalEgressUnitName(payload externalEgressJobPayload) string {
	return "megavpn-external-egress-" + strings.ReplaceAll(payload.DeploymentID, "-", "") + ".service"
}

func externalEgressUnitPath(payload externalEgressJobPayload) string {
	return filepath.Join("/etc/systemd/system", externalEgressUnitName(payload))
}
