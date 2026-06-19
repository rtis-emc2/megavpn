package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

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

func defaultInstanceSystemdUnit(payload instanceJobPayload) string {
	return driver.DefaultSystemdUnit(payload.ServiceCode, payload.Slug)
}

func defaultConfigPath(payload instanceJobPayload) string {
	return driver.DefaultConfigPath(payload.ServiceCode, payload.Slug)
}

func defaultConfigMode(payload instanceJobPayload) string {
	return driver.DefaultConfigMode(payload.ServiceCode)
}
