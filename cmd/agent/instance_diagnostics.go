package main

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/rtis-emc2/megavpn/internal/service/driver"
)

func (c client) handleInstanceDiagnoseJob(ctx context.Context, j job) (string, map[string]any) {
	payload, err := decodeInstanceJobPayload(j.Payload)
	if err != nil {
		return "failed", map[string]any{"error": err.Error(), "job_type": j.Type}
	}
	return instanceRuntimeExecutor{}.Diagnose(ctx, payload)
}

func (e instanceRuntimeExecutor) Diagnose(ctx context.Context, payload instanceJobPayload) (string, map[string]any) {
	started := time.Now()
	unit := strings.TrimSpace(payload.SystemdUnit)
	result := map[string]any{
		"message":       "instance diagnostics collected",
		"instance_id":   payload.InstanceID,
		"service_code":  payload.ServiceCode,
		"systemd_unit":  unit,
		"endpoint_host": payload.EndpointHost,
		"endpoint_port": payload.EndpointPort,
	}

	result["binaries"] = instanceBinaryDiagnostics(payload)
	if unit == "" {
		result["unit_warning"] = "systemd_unit is empty"
	} else if !isAllowedInstanceUnit(payload, unit) {
		result["unit_warning"] = "systemd_unit is not allowed for this managed instance"
	} else {
		result["active_state"] = currentUnitState(unit)
		result["enabled_state"] = normalizeSystemctlState(strings.TrimSpace(runOutput("systemctl", "is-enabled", unit)))
		result["load_state"] = strings.TrimSpace(runOutput("systemctl", "show", unit, "-p", "LoadState", "--value"))
		result["sub_state"] = strings.TrimSpace(runOutput("systemctl", "show", unit, "-p", "SubState", "--value"))

		code, out := runInstallCommand(ctx, "systemctl", "status", unit, "--no-pager", "-l", "--lines=80")
		result["systemctl_status_exit_code"] = code
		result["systemctl_status_output"] = truncate(strings.TrimSpace(out), 8000)

		code, out = runInstallCommand(ctx, "journalctl", "-u", unit, "--since", "-2h", "--no-pager", "-n", "160", "-o", "short-iso")
		result["journal_tail_exit_code"] = code
		result["journal_tail"] = truncate(strings.TrimSpace(out), 12000)
	}

	files, err := materializeInstanceSpec(payload)
	if err != nil {
		result["render_error"] = err.Error()
	} else {
		result["managed_files"] = managedFileDiagnostics(payload, files)
		if err := validateRenderedConfig(ctx, payload, files); err != nil {
			result["rendered_config_validation_error"] = err.Error()
		} else {
			result["rendered_config_validation"] = "ok"
		}
	}
	result["duration_ms"] = time.Since(started).Milliseconds()
	return "succeeded", result
}

func instanceBinaryDiagnostics(payload instanceJobPayload) []map[string]any {
	type runtimeBinary struct {
		name       string
		candidates []string
	}
	binaries := []runtimeBinary{}
	switch driver.NormalizeCode(payload.ServiceCode) {
	case driver.XrayCore, driver.MTProto:
		binaries = append(binaries, runtimeBinary{name: "xray", candidates: []string{"/usr/local/bin/xray", "/usr/bin/xray", "/opt/xray/xray"}})
	case driver.OpenVPN:
		binaries = append(binaries, runtimeBinary{name: "openvpn", candidates: []string{"/usr/sbin/openvpn", "/usr/bin/openvpn"}})
	case driver.WireGuard:
		binaries = append(binaries,
			runtimeBinary{name: "wg", candidates: []string{"/usr/bin/wg", "/usr/local/bin/wg"}},
			runtimeBinary{name: "wg-quick", candidates: []string{"/usr/bin/wg-quick", "/usr/local/bin/wg-quick"}},
		)
	case driver.Shadowsocks:
		binaries = append(binaries, runtimeBinary{name: "ss-server", candidates: []string{"/usr/bin/ss-server", "/usr/local/bin/ss-server"}})
	case driver.HTTPProxy:
		binaries = append(binaries, runtimeBinary{name: "squid", candidates: []string{"/usr/sbin/squid", "/usr/bin/squid"}})
	case driver.Nginx:
		binaries = append(binaries, runtimeBinary{name: "nginx", candidates: []string{"/usr/sbin/nginx", "/usr/bin/nginx"}})
	}
	out := make([]map[string]any, 0, len(binaries))
	for _, item := range binaries {
		path, ok := resolveExecutable(item.name, item.candidates...)
		row := map[string]any{"name": item.name, "found": ok}
		if ok {
			row["path"] = path
		} else if len(item.candidates) > 0 {
			row["checked_paths"] = item.candidates
		}
		out = append(out, row)
	}
	return out
}

func managedFileDiagnostics(payload instanceJobPayload, files []managedFileSpec) []map[string]any {
	out := make([]map[string]any, 0, len(files))
	seen := map[string]bool{}
	for _, file := range files {
		path := cleanAbsPath(file.Path)
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		row := map[string]any{"path": path, "declared_mode": file.Mode}
		if err := validateManagedDeletePathPolicy(payload, path); err != nil {
			row["policy_warning"] = err.Error()
			out = append(out, row)
			continue
		}
		info, err := os.Lstat(path)
		if err != nil {
			row["exists"] = false
			row["error"] = err.Error()
			out = append(out, row)
			continue
		}
		row["exists"] = true
		row["mode"] = info.Mode().String()
		row["size"] = info.Size()
		row["modified_at"] = info.ModTime().UTC().Format(time.RFC3339)
		row["is_symlink"] = info.Mode()&os.ModeSymlink != 0
		out = append(out, row)
	}
	return out
}
