package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/rtis-emc2/megavpn/internal/service/driver"
)

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
	path := cleanAbsPath(file.Path)
	if path == "" {
		return fmt.Errorf("managed file path is empty")
	}
	if hasUnsafePathToken(path) {
		return fmt.Errorf("managed file path contains unsafe characters: %s", file.Path)
	}
	mode, err := parseFileMode(file.Mode)
	if err != nil {
		return err
	}
	if err := rejectSymlinkPath(path); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := rejectSymlinkPath(path); err != nil {
		return err
	}
	tmpFile, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.megavpn.tmp")
	if err != nil {
		return err
	}
	tmp := tmpFile.Name()
	defer os.Remove(tmp)
	if err := tmpFile.Chmod(mode); err != nil {
		tmpFile.Close()
		return err
	}
	if _, err := tmpFile.WriteString(file.Content); err != nil {
		tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func rejectSymlinkPath(path string) error {
	path = filepath.Clean(path)
	if path == string(filepath.Separator) {
		return fmt.Errorf("managed file path must not be root")
	}
	dir := filepath.Dir(path)
	current := string(filepath.Separator)
	for _, part := range strings.Split(strings.TrimPrefix(dir, string(filepath.Separator)), string(filepath.Separator)) {
		if part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			break
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("managed file parent must not be a symlink: %s", current)
		}
	}
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("managed file path must not be a symlink: %s", path)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func validateManagedFilePolicy(payload instanceJobPayload, file managedFileSpec) error {
	path := cleanAbsPath(file.Path)
	if path == "" {
		return fmt.Errorf("managed file path must be absolute")
	}
	if hasUnsafePathToken(path) {
		return fmt.Errorf("managed file path contains unsafe characters: %s", file.Path)
	}
	if !isAllowedInstanceManagedPath(payload, path) {
		return fmt.Errorf("managed file path is outside allowed roots for %s: %s", payload.ServiceCode, path)
	}
	if strings.HasPrefix(path, "/etc/systemd/system/") {
		unit := strings.TrimSuffix(strings.TrimPrefix(path, "/etc/systemd/system/"), ".service")
		if !isAllowedInstanceUnit(payload, unit) {
			return fmt.Errorf("managed systemd unit is not allowed for %s: %s", payload.ServiceCode, unit)
		}
		if !isAllowedManagedServiceFile(payload, file.Content) {
			return fmt.Errorf("managed systemd unit content is not allowed for %s: %s", payload.ServiceCode, unit)
		}
	}
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("managed file path must not be a symlink: %s", path)
	}
	return nil
}

func validateManagedDeletePathPolicy(payload instanceJobPayload, rawPath string) error {
	path := cleanAbsPath(rawPath)
	if path == "" {
		return fmt.Errorf("managed file path must be absolute")
	}
	if hasUnsafePathToken(path) {
		return fmt.Errorf("managed file path contains unsafe characters: %s", rawPath)
	}
	if !isAllowedInstanceManagedPath(payload, path) {
		return fmt.Errorf("managed file path is outside allowed roots for %s: %s", payload.ServiceCode, path)
	}
	if strings.HasPrefix(path, "/etc/systemd/system/") {
		unit := strings.TrimSuffix(strings.TrimPrefix(path, "/etc/systemd/system/"), ".service")
		if !isAllowedInstanceUnit(payload, unit) {
			return fmt.Errorf("managed systemd unit is not allowed for %s: %s", payload.ServiceCode, unit)
		}
	}
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("managed file path must not be a symlink: %s", path)
	}
	return nil
}

func isAllowedManagedServiceFile(payload instanceJobPayload, content string) bool {
	if strings.Contains(content, "/bin/sh") || strings.Contains(content, " -c ") {
		return false
	}
	switch driver.NormalizeCode(payload.ServiceCode) {
	case driver.XrayCore, driver.MTProto:
		return strings.Contains(content, "ExecStart=/usr/bin/env xray run -config ")
	case driver.HTTPProxy:
		return strings.Contains(content, "ExecStart=/usr/bin/env squid -f ") &&
			strings.Contains(content, "ExecReload=/usr/bin/env squid -k reconfigure -f ") &&
			strings.Contains(content, "ExecStop=/usr/bin/env squid -k shutdown -f ")
	case driver.Shadowsocks:
		return strings.Contains(content, "ExecStart=/usr/bin/env ss-server -c ")
	default:
		return false
	}
}

func cleanAbsPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || strings.Contains(path, "\x00") || !filepath.IsAbs(path) {
		return ""
	}
	return filepath.Clean(path)
}

func hasUnsafePathToken(path string) bool {
	if strings.Contains(path, "/../") || strings.HasSuffix(path, "/..") {
		return true
	}
	for _, r := range path {
		if r == 0 || unicode.IsControl(r) || unicode.IsSpace(r) {
			return true
		}
	}
	return false
}

func isAllowedInstanceManagedPath(payload instanceJobPayload, path string) bool {
	path = cleanAbsPath(path)
	if path == "" {
		return false
	}
	if strings.HasPrefix(path, "/etc/systemd/system/") {
		return strings.HasSuffix(path, ".service")
	}
	for _, root := range allowedInstancePathRoots(payload) {
		if pathWithinRoot(path, root) {
			return true
		}
	}
	for _, allowed := range allowedInstanceExactPaths(payload) {
		if path == cleanAbsPath(allowed) {
			return true
		}
	}
	return false
}

func allowedInstancePathRoots(payload instanceJobPayload) []string {
	switch driver.NormalizeCode(payload.ServiceCode) {
	case driver.XrayCore, driver.MTProto:
		return []string{"/usr/local/etc/xray", "/etc/megavpn/certs"}
	case driver.OpenVPN:
		return []string{"/etc/openvpn/server", "/etc/megavpn/certs"}
	case driver.WireGuard:
		return []string{"/etc/wireguard"}
	case driver.IPSec:
		return []string{"/etc/megavpn/certs"}
	case driver.XL2TPD:
		return []string{"/etc/xl2tpd"}
	case driver.HTTPProxy:
		return []string{"/etc/squid"}
	case driver.Shadowsocks:
		return []string{"/etc/shadowsocks-libev"}
	case driver.Nginx:
		return []string{"/etc/nginx/conf.d", "/etc/megavpn/certs"}
	default:
		return nil
	}
}

func allowedInstanceExactPaths(payload instanceJobPayload) []string {
	switch driver.NormalizeCode(payload.ServiceCode) {
	case driver.IPSec:
		return []string{"/etc/ipsec.conf", "/etc/ipsec.secrets"}
	case driver.XL2TPD:
		return []string{"/etc/ppp/options.xl2tpd", "/etc/ppp/chap-secrets"}
	default:
		return nil
	}
}

func pathWithinRoot(path, root string) bool {
	path = cleanAbsPath(path)
	root = cleanAbsPath(root)
	if path == "" || root == "" {
		return false
	}
	if path == root {
		return true
	}
	return strings.HasPrefix(path, strings.TrimRight(root, "/")+"/")
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

func defaultInstanceSystemdUnit(payload instanceJobPayload) string {
	return driver.DefaultSystemdUnit(payload.ServiceCode, payload.Slug)
}

func defaultConfigPath(payload instanceJobPayload) string {
	return driver.DefaultConfigPath(payload.ServiceCode, payload.Slug)
}

func defaultConfigMode(payload instanceJobPayload) string {
	return driver.DefaultConfigMode(payload.ServiceCode)
}
