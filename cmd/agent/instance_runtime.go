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
	default:
		return "failed", map[string]any{"error": "unsupported instance operation", "action": payload.Action}
	}
}

func (e instanceRuntimeExecutor) Apply(ctx context.Context, payload instanceJobPayload) (string, map[string]any) {
	started := time.Now()
	files, err := materializeInstanceSpec(payload)
	if err != nil {
		return "failed", map[string]any{"error": err.Error(), "instance_id": payload.InstanceID, "service_code": payload.ServiceCode, "action": payload.Action}
	}
	changed := make([]string, 0, len(files))
	for _, file := range files {
		if err := validateManagedFilePolicy(payload, file); err != nil {
			return "failed", map[string]any{
				"error":        err.Error(),
				"instance_id":  payload.InstanceID,
				"service_code": payload.ServiceCode,
				"path":         file.Path,
			}
		}
		if err := writeManagedFile(file); err != nil {
			return "failed", map[string]any{
				"error":        err.Error(),
				"instance_id":  payload.InstanceID,
				"service_code": payload.ServiceCode,
				"path":         file.Path,
			}
		}
		changed = append(changed, file.Path)
	}
	_, _ = runInstallCommand(ctx, "systemctl", "daemon-reload")
	if err := validateRenderedConfig(ctx, payload, files); err != nil {
		return "failed", map[string]any{
			"error":         err.Error(),
			"instance_id":   payload.InstanceID,
			"service_code":  payload.ServiceCode,
			"changed_files": changed,
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
			return status, result
		}
		_, _ = runInstallCommand(ctx, "systemctl", "disable", payload.SystemdUnit)
	}
	result["active_state"] = currentUnitState(payload.SystemdUnit)
	return status, result
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
