package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type instanceJobPayload struct {
	InstanceID         string         `json:"instance_id"`
	Action             string         `json:"action"`
	ServiceCode        string         `json:"service_code"`
	RuntimeServiceCode string         `json:"runtime_service_code"`
	Name               string         `json:"name"`
	Slug               string         `json:"slug"`
	SystemdUnit        string         `json:"systemd_unit"`
	EndpointHost       string         `json:"endpoint_host"`
	EndpointPort       int            `json:"endpoint_port"`
	Enabled            bool           `json:"enabled"`
	Spec               map[string]any `json:"spec"`
}

type managedFileSpec struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Mode    string `json:"mode"`
}

func (c client) handleInstanceJob(ctx context.Context, j job) (string, map[string]any) {
	payload, err := decodeInstanceJobPayload(j.Payload)
	if err != nil {
		return "failed", map[string]any{"error": err.Error(), "job_type": j.Type}
	}
	switch payload.Action {
	case "apply":
		return applyInstance(ctx, payload)
	case "restart":
		return systemdInstanceAction(ctx, payload, "restart")
	case "start":
		return systemdInstanceAction(ctx, payload, "start")
	case "stop":
		return systemdInstanceAction(ctx, payload, "stop")
	case "enable":
		return systemdInstanceAction(ctx, payload, "enable")
	case "disable":
		return systemdInstanceAction(ctx, payload, "disable")
	default:
		return "failed", map[string]any{"error": "unsupported instance action", "action": payload.Action}
	}
}

func decodeInstanceJobPayload(raw map[string]any) (instanceJobPayload, error) {
	b, err := json.Marshal(raw)
	if err != nil {
		return instanceJobPayload{}, err
	}
	var payload instanceJobPayload
	if err := json.Unmarshal(b, &payload); err != nil {
		return instanceJobPayload{}, err
	}
	payload.ServiceCode = normalizeCapabilityCode(first(payload.RuntimeServiceCode, payload.ServiceCode))
	if payload.RuntimeServiceCode == "" {
		payload.RuntimeServiceCode = payload.ServiceCode
	}
	payload.Action = strings.TrimSpace(payload.Action)
	if payload.Action == "" {
		payload.Action = "apply"
	}
	if payload.Slug == "" {
		payload.Slug = slugifyLocal(first(payload.Name, payload.InstanceID, "instance"))
	}
	if payload.ServiceCode == "ipsec" && strings.TrimSpace(payload.SystemdUnit) == "strongswan" {
		payload.SystemdUnit = "strongswan-starter"
	}
	if payload.SystemdUnit == "" {
		payload.SystemdUnit = defaultInstanceSystemdUnit(payload)
	}
	if payload.Spec == nil {
		payload.Spec = map[string]any{}
	}
	return payload, nil
}

func applyInstance(ctx context.Context, payload instanceJobPayload) (string, map[string]any) {
	started := time.Now()
	files, err := materializeInstanceSpec(payload)
	if err != nil {
		return "failed", map[string]any{"error": err.Error(), "instance_id": payload.InstanceID, "service_code": payload.ServiceCode, "action": payload.Action}
	}
	changed := make([]string, 0, len(files))
	for _, file := range files {
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
	}
	if payload.SystemdUnit != "" {
		action := "restart"
		if normalizeSystemctlState(strings.TrimSpace(runOutput("systemctl", "is-active", payload.SystemdUnit))) == "unknown" {
			action = "enable"
		}
		if payload.Enabled {
			status, result = systemdInstanceAction(ctx, payload, action)
			result["changed_files"] = changed
			result["config_applied"] = len(changed) > 0
			result["duration_ms"] = time.Since(started).Milliseconds()
			return status, result
		}
		_, _ = runInstallCommand(ctx, "systemctl", "disable", payload.SystemdUnit)
	}
	result["active_state"] = currentUnitState(payload.SystemdUnit)
	return status, result
}

func systemdInstanceAction(ctx context.Context, payload instanceJobPayload, action string) (string, map[string]any) {
	started := time.Now()
	var args []string
	switch action {
	case "restart":
		args = []string{"restart", payload.SystemdUnit}
	case "start":
		args = []string{"start", payload.SystemdUnit}
	case "stop":
		args = []string{"stop", payload.SystemdUnit}
	case "enable":
		args = []string{"enable", "--now", payload.SystemdUnit}
	case "disable":
		args = []string{"disable", "--now", payload.SystemdUnit}
	default:
		return "failed", map[string]any{"error": "unsupported systemd action", "action": action}
	}
	if payload.SystemdUnit == "" {
		return "failed", map[string]any{"error": "systemd_unit is required for action", "action": action}
	}
	code, out := runInstallCommand(ctx, "systemctl", args...)
	result := map[string]any{
		"instance_id":  payload.InstanceID,
		"service_code": payload.ServiceCode,
		"systemd_unit": payload.SystemdUnit,
		"action":       action,
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

func materializeInstanceSpec(payload instanceJobPayload) ([]managedFileSpec, error) {
	spec := payload.Spec
	if spec == nil {
		spec = map[string]any{}
	}
	files, err := decodeManagedFiles(spec["files"])
	if err != nil {
		return nil, err
	}
	if len(files) > 0 {
		return files, nil
	}

	path := strings.TrimSpace(stringify(spec["config_path"]))
	if path == "" {
		path = defaultConfigPath(payload)
	}
	content, hasContent, err := singleConfigContent(spec)
	if err != nil {
		return nil, err
	}
	if !hasContent {
		return nil, nil
	}
	mode := stringify(spec["config_mode"])
	if mode == "" {
		mode = defaultConfigMode(payload)
	}
	return []managedFileSpec{{Path: path, Content: content, Mode: mode}}, nil
}

func decodeManagedFiles(raw any) ([]managedFileSpec, error) {
	if raw == nil {
		return nil, nil
	}
	list, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("spec.files must be an array")
	}
	out := make([]managedFileSpec, 0, len(list))
	for _, item := range list {
		fileMap, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("spec.files items must be objects")
		}
		path := strings.TrimSpace(stringify(fileMap["path"]))
		if path == "" {
			return nil, fmt.Errorf("spec.files item path is required")
		}
		mode := stringify(fileMap["mode"])
		content := stringify(fileMap["content"])
		if content == "" {
			if rawJSON, ok := fileMap["json"]; ok {
				b, err := json.MarshalIndent(rawJSON, "", "  ")
				if err != nil {
					return nil, fmt.Errorf("spec.files json render failed for %s: %w", path, err)
				}
				content = string(b) + "\n"
			}
		}
		if content == "" {
			return nil, fmt.Errorf("spec.files item content/json is required for %s", path)
		}
		out = append(out, managedFileSpec{Path: path, Content: content, Mode: mode})
	}
	return out, nil
}

func singleConfigContent(spec map[string]any) (string, bool, error) {
	if content := stringify(spec["config_content"]); content != "" {
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		return content, true, nil
	}
	if rawJSON, ok := spec["config_json"]; ok {
		b, err := json.MarshalIndent(rawJSON, "", "  ")
		if err != nil {
			return "", false, err
		}
		return string(b) + "\n", true, nil
	}
	if rawConfig, ok := spec["config"]; ok {
		switch cfg := rawConfig.(type) {
		case string:
			if cfg == "" {
				return "", false, nil
			}
			if !strings.HasSuffix(cfg, "\n") {
				cfg += "\n"
			}
			return cfg, true, nil
		default:
			b, err := json.MarshalIndent(cfg, "", "  ")
			if err != nil {
				return "", false, err
			}
			return string(b) + "\n", true, nil
		}
	}
	return "", false, nil
}

func writeManagedFile(file managedFileSpec) error {
	if strings.TrimSpace(file.Path) == "" {
		return fmt.Errorf("managed file path is empty")
	}
	mode, err := parseFileMode(file.Mode)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(file.Path), 0o755); err != nil {
		return err
	}
	tmp := file.Path + ".megavpn.tmp"
	if err := os.WriteFile(tmp, []byte(file.Content), mode); err != nil {
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil {
		return err
	}
	return os.Rename(tmp, file.Path)
}

func parseFileMode(raw string) (os.FileMode, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0o640, nil
	}
	v, err := strconv.ParseUint(raw, 8, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid file mode %q", raw)
	}
	return os.FileMode(v), nil
}

func validateRenderedConfig(ctx context.Context, payload instanceJobPayload, files []managedFileSpec) error {
	switch payload.ServiceCode {
	case "nginx":
		code, out := runInstallCommand(ctx, "nginx", "-t")
		if code != 0 {
			return fmt.Errorf("nginx config validation failed: %s", truncate(out, 3000))
		}
	case "http_proxy":
		configPath := firstConfigPath(files, defaultConfigPath(payload))
		if configPath != "" {
			code, out := runInstallCommand(ctx, "squid", "-k", "parse", "-f", configPath)
			if code != 0 {
				return fmt.Errorf("http proxy config validation failed: %s", truncate(out, 3000))
			}
		}
	case "wireguard":
		configPath := firstConfigPath(files, defaultConfigPath(payload))
		if configPath != "" {
			code, out := runInstallCommand(ctx, "wg-quick", "strip", configPath)
			if code != 0 {
				return fmt.Errorf("wireguard config validation failed: %s", truncate(out, 3000))
			}
		}
	case "xray-core":
		configPath := firstConfigPath(files, defaultConfigPath(payload))
		if configPath != "" {
			code, out := runInstallCommand(ctx, "xray", "run", "-test", "-config", configPath)
			if code != 0 {
				code, out = runInstallCommand(ctx, "xray", "-test", "-config", configPath)
			}
			if code != 0 {
				return fmt.Errorf("xray config validation failed: %s", truncate(out, 3000))
			}
		}
	case "mtproto":
		configPath := firstConfigPath(files, defaultConfigPath(payload))
		if configPath != "" {
			code, out := runInstallCommand(ctx, "xray", "run", "-test", "-config", configPath)
			if code != 0 {
				code, out = runInstallCommand(ctx, "xray", "-test", "-config", configPath)
			}
			if code != 0 {
				return fmt.Errorf("xray config validation failed: %s", truncate(out, 3000))
			}
		}
	}
	return nil
}

func firstConfigPath(files []managedFileSpec, fallback string) string {
	for _, file := range files {
		if file.Path != "" {
			return file.Path
		}
	}
	return fallback
}

func currentUnitState(unit string) string {
	if strings.TrimSpace(unit) == "" {
		return "unknown"
	}
	return normalizeSystemctlState(strings.TrimSpace(runOutput("systemctl", "is-active", unit)))
}

func defaultInstanceSystemdUnit(payload instanceJobPayload) string {
	switch payload.ServiceCode {
	case "xray-core":
		return "xray"
	case "nginx":
		return "nginx"
	case "mtproto":
		return "xray"
	case "http_proxy":
		return "squid"
	case "openvpn":
		if payload.Slug != "" {
			return "openvpn-server@" + payload.Slug
		}
		return "openvpn-server@server"
	case "wireguard":
		if payload.Slug != "" {
			return "wg-quick@" + payload.Slug
		}
		return "wg-quick@wg0"
	case "ipsec":
		return "strongswan-starter"
	case "xl2tpd":
		return "xl2tpd"
	case "shadowsocks":
		return "shadowsocks-libev"
	default:
		return ""
	}
}

func defaultConfigPath(payload instanceJobPayload) string {
	switch payload.ServiceCode {
	case "xray-core":
		return "/usr/local/etc/xray/config.json"
	case "mtproto":
		return "/usr/local/etc/xray/config.json"
	case "nginx":
		return "/etc/nginx/nginx.conf"
	case "http_proxy":
		return "/etc/squid/squid.conf"
	case "openvpn":
		slug := first(payload.Slug, "server")
		return "/etc/openvpn/server/" + slug + ".conf"
	case "wireguard":
		slug := first(payload.Slug, "wg0")
		return "/etc/wireguard/" + slug + ".conf"
	case "ipsec":
		return "/etc/ipsec.conf"
	case "xl2tpd":
		return "/etc/xl2tpd/xl2tpd.conf"
	case "shadowsocks":
		return "/etc/shadowsocks-libev/config.json"
	default:
		return ""
	}
}

func defaultConfigMode(payload instanceJobPayload) string {
	switch payload.ServiceCode {
	case "xray-core", "mtproto", "shadowsocks":
		return "0640"
	case "wireguard":
		return "0600"
	default:
		return "0644"
	}
}

func slugifyLocal(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "instance"
	}
	return out
}
