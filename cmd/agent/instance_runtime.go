package main

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/rtis-emc2/megavpn/internal/service/driver"
)

func (e instanceRuntimeExecutor) Execute(ctx context.Context, payload instanceJobPayload) (string, map[string]any) {
	op, ok := driver.OperationFor(payload.ServiceCode, payload.Action)
	if !ok || !op.AgentExecutable {
		return "failed", map[string]any{"error": "unsupported instance operation for driver", "action": payload.Action, "service_code": payload.ServiceCode}
	}
	switch op.Code {
	case driver.OperationApply:
		return e.Apply(ctx, payload)
	case driver.OperationRestart, driver.OperationStart, driver.OperationStop, driver.OperationEnable, driver.OperationDisable:
		return e.Systemd(ctx, payload, op.Code)
	case driver.OperationDelete:
		return e.Delete(ctx, payload)
	default:
		return "failed", map[string]any{"error": "unsupported instance operation", "action": payload.Action}
	}
}

func (e instanceRuntimeExecutor) Apply(ctx context.Context, payload instanceJobPayload) (string, map[string]any) {
	started := time.Now()
	runtimePreflight, err := ensureInstanceRuntime(ctx, payload)
	if err != nil {
		result := map[string]any{
			"error":        err.Error(),
			"instance_id":  payload.InstanceID,
			"service_code": payload.ServiceCode,
			"action":       payload.Action,
			"duration_ms":  time.Since(started).Milliseconds(),
		}
		if len(runtimePreflight) > 0 {
			result["runtime_preflight"] = runtimePreflight
		}
		return "failed", result
	}
	files, err := materializeInstanceSpec(payload)
	if err != nil {
		return "failed", map[string]any{"error": err.Error(), "instance_id": payload.InstanceID, "service_code": payload.ServiceCode, "action": payload.Action}
	}
	for _, file := range files {
		if err := validateManagedFilePolicy(payload, file); err != nil {
			return "failed", map[string]any{
				"error":        err.Error(),
				"instance_id":  payload.InstanceID,
				"service_code": payload.ServiceCode,
				"path":         file.Path,
			}
		}
	}
	backups, err := snapshotManagedFiles(files)
	if err != nil {
		return "failed", map[string]any{
			"error":        err.Error(),
			"instance_id":  payload.InstanceID,
			"service_code": payload.ServiceCode,
			"action":       payload.Action,
		}
	}
	rollback := func() map[string]any {
		result := rollbackManagedFiles(backups)
		reloadResult, err := runSystemdDaemonReload(ctx)
		result["systemd_reload"] = reloadResult
		if err != nil {
			result["systemd_reload_warning"] = err.Error()
		}
		return result
	}
	changed := make([]string, 0, len(files))
	for _, file := range files {
		if err := writeManagedFile(file); err != nil {
			return "failed", map[string]any{
				"error":        err.Error(),
				"instance_id":  payload.InstanceID,
				"service_code": payload.ServiceCode,
				"path":         file.Path,
				"rollback":     rollback(),
			}
		}
		changed = append(changed, file.Path)
	}
	reloadResult, err := runSystemdDaemonReload(ctx)
	if err != nil {
		return "failed", map[string]any{
			"error":          err.Error(),
			"stage":          "systemd_daemon_reload",
			"instance_id":    payload.InstanceID,
			"service_code":   payload.ServiceCode,
			"changed_files":  changed,
			"systemd_reload": reloadResult,
			"rollback":       rollback(),
		}
	}
	if err := validateRenderedConfig(ctx, payload, files); err != nil {
		return "failed", map[string]any{
			"error":         err.Error(),
			"instance_id":   payload.InstanceID,
			"service_code":  payload.ServiceCode,
			"changed_files": changed,
			"rollback":      rollback(),
		}
	}
	networkPolicy, err := applyNetworkPolicy(ctx, payload)
	if err != nil {
		return "failed", map[string]any{
			"error":          err.Error(),
			"instance_id":    payload.InstanceID,
			"service_code":   payload.ServiceCode,
			"changed_files":  changed,
			"network_policy": networkPolicy,
			"rollback":       rollback(),
		}
	}

	status := "succeeded"
	result := map[string]any{
		"message":         "instance applied",
		"instance_id":     payload.InstanceID,
		"service_code":    payload.ServiceCode,
		"systemd_unit":    payload.SystemdUnit,
		"changed_files":   changed,
		"duration_ms":     time.Since(started).Milliseconds(),
		"config_applied":  len(changed) > 0,
		"endpoint_host":   payload.EndpointHost,
		"endpoint_port":   payload.EndpointPort,
		"enabled_desired": payload.Enabled,
		"network_policy":  networkPolicy,
		"systemd_reload":  reloadResult,
	}
	if len(runtimePreflight) > 0 {
		result["runtime_preflight"] = runtimePreflight
	}
	if payload.SystemdUnit != "" {
		action := driver.OperationRestart
		if normalizeSystemctlState(strings.TrimSpace(runOutput("systemctl", "is-active", payload.SystemdUnit))) == "unknown" {
			action = driver.OperationEnable
		}
		if payload.Enabled {
			status, result = e.Systemd(ctx, payload, action)
			result["changed_files"] = changed
			result["config_applied"] = len(changed) > 0
			result["duration_ms"] = time.Since(started).Milliseconds()
			result["network_policy"] = networkPolicy
			if status != "succeeded" {
				result["rollback"] = rollback()
			}
			return status, result
		}
		if !isAllowedInstanceUnit(payload, payload.SystemdUnit) {
			return "failed", map[string]any{"error": "systemd_unit is not allowed for disabled instance apply", "instance_id": payload.InstanceID, "service_code": payload.ServiceCode, "systemd_unit": payload.SystemdUnit}
		}
		disableCode, disableOut := runInstallCommand(ctx, "systemctl", "disable", payload.SystemdUnit)
		result["systemd_disable_output"] = truncate(strings.TrimSpace(disableOut), 2000)
		if disableCode != 0 && !isMissingSystemdUnitOutput(disableOut) {
			result["error"] = commandExitError("systemd disable failed for disabled instance apply", disableOut).Error()
			return "failed", result
		}
	}
	result["active_state"] = currentUnitState(payload.SystemdUnit)
	return status, result
}

func ensureInstanceRuntime(ctx context.Context, payload instanceJobPayload) (map[string]any, error) {
	err := preflightInstanceRuntime(payload)
	if err == nil {
		return nil, nil
	}
	if driver.NormalizeCode(payload.ServiceCode) != driver.Nginx {
		return nil, err
	}
	recovery := installNginxForRuntime(ctx, "nginx_org_repo", "stable")
	if recovery == nil {
		recovery = map[string]any{}
	}
	recovery["trigger"] = "instance_apply_preflight"
	recovery["recovered_capability"] = "nginx"
	if errAfterInstall := preflightInstanceRuntime(payload); errAfterInstall == nil {
		recovery["binary_available_after_install"] = true
		if recovery["ok"] != true {
			recovery["warning"] = "nginx installer verification did not fully pass, but the nginx binary is available; continuing to rendered config validation"
		}
		return recovery, nil
	}
	if recoveryMessage := stringify(recovery["message"]); recoveryMessage != "" {
		return recovery, fmt.Errorf("%w; managed nginx recovery failed: %s", err, recoveryMessage)
	}
	return recovery, err
}

var resolveExecutableForRuntime = resolveExecutable

func preflightInstanceRuntime(payload instanceJobPayload) error {
	type runtimeBinary struct {
		name       string
		candidates []string
	}
	var required []runtimeBinary
	switch driver.NormalizeCode(payload.ServiceCode) {
	case driver.XrayCore, driver.MTProto:
		required = []runtimeBinary{{name: "xray", candidates: []string{"/usr/local/bin/xray", "/usr/bin/xray", "/opt/xray/xray"}}}
	case driver.WireGuard:
		required = []runtimeBinary{{name: "wg-quick", candidates: []string{"/usr/bin/wg-quick", "/usr/sbin/wg-quick"}}, {name: "wg", candidates: []string{"/usr/bin/wg", "/usr/sbin/wg"}}}
	case driver.OpenVPN:
		required = []runtimeBinary{{name: "openvpn", candidates: []string{"/usr/sbin/openvpn", "/usr/bin/openvpn"}}}
	case driver.Shadowsocks:
		required = []runtimeBinary{{name: "ss-server", candidates: []string{"/usr/bin/ss-server", "/usr/local/bin/ss-server"}}}
	case driver.HTTPProxy:
		required = []runtimeBinary{{name: "squid", candidates: []string{"/usr/sbin/squid", "/usr/bin/squid"}}}
	case driver.Nginx:
		required = []runtimeBinary{{name: "nginx", candidates: []string{"/usr/sbin/nginx", "/usr/bin/nginx"}}}
	}
	for _, item := range required {
		if _, ok := resolveExecutableForRuntime(item.name, item.candidates...); !ok {
			return fmt.Errorf("capability missing: %s binary is not installed or not executable", item.name)
		}
	}
	return nil
}

func (e instanceRuntimeExecutor) Systemd(ctx context.Context, payload instanceJobPayload, operation string) (string, map[string]any) {
	started := time.Now()
	if !isAllowedInstanceUnit(payload, payload.SystemdUnit) {
		return "failed", map[string]any{"error": "systemd_unit is not allowed for instance", "action": operation, "systemd_unit": payload.SystemdUnit, "service_code": payload.ServiceCode}
	}
	args, err := systemdArgsForOperation(operation, payload.SystemdUnit)
	if err != nil {
		return "failed", map[string]any{"error": err.Error(), "action": operation}
	}
	if payload.SystemdUnit == "" {
		return "failed", map[string]any{"error": "systemd_unit is required for action", "action": operation}
	}
	code, out := runInstallCommand(ctx, "systemctl", args...)
	result := map[string]any{
		"instance_id":  payload.InstanceID,
		"service_code": payload.ServiceCode,
		"systemd_unit": payload.SystemdUnit,
		"action":       operation,
		"duration_ms":  time.Since(started).Milliseconds(),
		"output":       truncate(out, 4000),
		"active_state": currentUnitState(payload.SystemdUnit),
	}
	if code != 0 {
		result["error"] = "systemd action failed"
		return "failed", result
	}
	result["message"] = "systemd action applied"
	return "succeeded", result
}

func (e instanceRuntimeExecutor) Delete(ctx context.Context, payload instanceJobPayload) (string, map[string]any) {
	started := time.Now()
	result := map[string]any{
		"instance_id":  payload.InstanceID,
		"service_code": payload.ServiceCode,
		"systemd_unit": payload.SystemdUnit,
		"action":       driver.OperationDelete,
	}
	removedPaths := []string{}
	skippedItems := []string{}
	warnings := []string{}
	stoppedUnits := []string{}

	unit := strings.TrimSpace(payload.SystemdUnit)
	if unit != "" {
		if !isAllowedInstanceUnit(payload, unit) {
			skippedItems = append(skippedItems, unit+": unit is not allowed for managed stop - skip")
		} else if isInstanceOwnedRuntimeUnit(payload, unit) {
			code, out := runInstallCommand(ctx, "systemctl", "disable", "--now", unit)
			result["systemd_disable_output"] = truncate(out, 2000)
			if code != 0 {
				if isMissingSystemdUnitOutput(out) || instanceUnitNotFound(payload, unit) {
					skippedItems = append(skippedItems, unit+": unit not found - skip")
				} else {
					result["error"] = "systemd disable failed"
					result["active_state"] = currentUnitState(unit)
					result["duration_ms"] = time.Since(started).Milliseconds()
					return "failed", result
				}
			} else {
				stoppedUnits = append(stoppedUnits, unit)
			}
		} else {
			skippedItems = append(skippedItems, unit+": shared runtime unit not stopped")
		}
	}

	files, err := materializeInstanceSpec(payload)
	if err != nil {
		result["error"] = err.Error()
		result["duration_ms"] = time.Since(started).Milliseconds()
		return "failed", result
	}
	seen := map[string]bool{}
	for _, file := range files {
		if err := validateManagedDeletePathPolicy(payload, file.Path); err != nil {
			result["error"] = err.Error()
			result["path"] = file.Path
			result["duration_ms"] = time.Since(started).Milliseconds()
			return "failed", result
		}
		path := cleanAbsPath(file.Path)
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		removed, err := removeManagedPath(path, false)
		if err != nil {
			result["error"] = err.Error()
			result["path"] = path
			result["duration_ms"] = time.Since(started).Milliseconds()
			return "failed", result
		}
		if removed {
			removedPaths = append(removedPaths, path)
		} else {
			skippedItems = append(skippedItems, path+": not found - skip")
		}
	}
	for _, path := range legacyManagedDeletePaths(payload) {
		if path == "" || seen[path] {
			continue
		}
		if err := validateManagedDeletePathPolicy(payload, path); err != nil {
			warnings = append(warnings, "legacy cleanup path skipped: "+err.Error())
			continue
		}
		seen[path] = true
		removed, err := removeManagedPath(path, false)
		if err != nil {
			warnings = append(warnings, path+": "+err.Error())
			continue
		}
		if removed {
			removedPaths = append(removedPaths, path)
		} else {
			skippedItems = append(skippedItems, path+": not found - skip")
		}
	}
	if unitFile := managedInstanceUnitFilePath(payload); unitFile != "" && !seen[unitFile] {
		removed, err := removeManagedPath(unitFile, false)
		if err != nil {
			result["error"] = err.Error()
			result["path"] = unitFile
			result["duration_ms"] = time.Since(started).Milliseconds()
			return "failed", result
		}
		if removed {
			removedPaths = append(removedPaths, unitFile)
		} else {
			skippedItems = append(skippedItems, unitFile+": not found - skip")
		}
	}

	networkPolicy := cleanupNetworkPolicy(ctx, payload)
	if errText := stringify(networkPolicy["error"]); errText != "" {
		warnings = append(warnings, "network policy cleanup warning: "+errText)
	}
	if driver.NormalizeCode(payload.ServiceCode) == driver.Nginx {
		nginxRuntime := finalizeSharedNginxRuntime(ctx)
		result["nginx_runtime"] = nginxRuntime
		if errText := stringify(nginxRuntime["error"]); errText != "" {
			warnings = append(warnings, "nginx cleanup warning: "+errText)
		}
	}
	reloadResult, err := runSystemdDaemonReload(ctx)
	result["systemd_reload"] = reloadResult
	if err != nil {
		result["error"] = err.Error()
		result["duration_ms"] = time.Since(started).Milliseconds()
		return "failed", result
	}

	result["message"] = "instance runtime cleanup completed"
	result["removed_paths"] = removedPaths
	result["skipped_items"] = skippedItems
	result["stopped_units"] = stoppedUnits
	result["warnings"] = warnings
	result["network_policy"] = networkPolicy
	result["active_state"] = currentUnitState(unit)
	result["duration_ms"] = time.Since(started).Milliseconds()
	return "succeeded", result
}

func legacyManagedDeletePaths(payload instanceJobPayload) []string {
	switch driver.NormalizeCode(payload.ServiceCode) {
	case driver.WireGuard:
		slug := strings.TrimSpace(first(payload.Slug, payload.Name, payload.InstanceID))
		if slug == "" {
			return nil
		}
		legacyPath := cleanAbsPath("/etc/wireguard/" + slug + ".conf")
		currentPath := cleanAbsPath(driver.DefaultConfigPath(driver.WireGuard, slug))
		if legacyPath == "" || legacyPath == currentPath {
			return nil
		}
		return []string{legacyPath}
	default:
		return nil
	}
}

func managedInstanceUnitFilePath(payload instanceJobPayload) string {
	unit := strings.TrimSpace(payload.SystemdUnit)
	if unit == "" || !isAllowedInstanceUnit(payload, unit) {
		return ""
	}
	switch driver.NormalizeCode(payload.ServiceCode) {
	case driver.XrayCore, driver.MTProto, driver.HTTPProxy, driver.Shadowsocks:
		return "/etc/systemd/system/" + strings.TrimSuffix(unit, ".service") + ".service"
	default:
		return ""
	}
}

func isInstanceOwnedRuntimeUnit(payload instanceJobPayload, unit string) bool {
	if !isAllowedInstanceUnit(payload, unit) {
		return false
	}
	switch driver.NormalizeCode(payload.ServiceCode) {
	case driver.XrayCore, driver.MTProto, driver.HTTPProxy, driver.Shadowsocks, driver.OpenVPN, driver.WireGuard:
		return true
	default:
		return false
	}
}

func instanceUnitNotFound(payload instanceJobPayload, unit string) bool {
	unit = strings.TrimSpace(unit)
	if !isAllowedInstanceUnit(payload, unit) {
		return false
	}
	state := strings.ToLower(strings.TrimSpace(runOutput("systemctl", "show", unit, "-p", "LoadState", "--value")))
	return state == "" || state == "not-found"
}

func systemdArgsForOperation(operation, unit string) ([]string, error) {
	unit = strings.TrimSpace(unit)
	if !isSafeSystemdUnitToken(unit) {
		return nil, fmt.Errorf("invalid systemd unit %q", unit)
	}
	switch operation {
	case driver.OperationRestart:
		return []string{"restart", unit}, nil
	case driver.OperationStart:
		return []string{"start", unit}, nil
	case driver.OperationStop:
		return []string{"stop", unit}, nil
	case driver.OperationEnable:
		return []string{"enable", "--now", unit}, nil
	case driver.OperationDisable:
		return []string{"disable", "--now", unit}, nil
	default:
		return nil, fmt.Errorf("unsupported systemd action %q", operation)
	}
}

func isAllowedInstanceUnit(payload instanceJobPayload, unit string) bool {
	unit = strings.TrimSuffix(strings.TrimSpace(unit), ".service")
	if unit == "" || !isSafeSystemdUnitToken(unit) {
		return false
	}
	serviceCode := driver.NormalizeCode(payload.ServiceCode)
	if serviceCode == "" {
		serviceCode = driver.NormalizeCode(payload.RuntimeServiceCode)
	}
	if serviceCode == driver.WireGuard {
		expectedIface := driver.WireGuardInterfaceName(first(payload.Slug, payload.Name, payload.InstanceID, "wg0"))
		if strings.HasPrefix(strings.TrimSpace(payload.SystemdUnit), "wg-quick@") {
			expectedIface = driver.WireGuardInterfaceName(strings.TrimPrefix(strings.TrimSpace(payload.SystemdUnit), "wg-quick@"))
		}
		if iface := stringify(payload.Spec["interface_name"]); iface != "" {
			expectedIface = driver.WireGuardInterfaceName(iface)
		}
		return unit == "wg-quick@"+expectedIface
	}
	expected := strings.TrimSuffix(driver.DefaultSystemdUnit(serviceCode, payload.Slug), ".service")
	return expected != "" && unit == expected
}

func isSafeSystemdUnitToken(unit string) bool {
	unit = strings.TrimSpace(unit)
	if unit == "" || strings.Contains(unit, "/") || strings.Contains(unit, "..") {
		return false
	}
	for _, r := range unit {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return false
		}
	}
	return true
}
