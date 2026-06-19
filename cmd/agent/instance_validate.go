package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rtis-emc2/megavpn/internal/service/driver"
)

type instanceConfigValidator func(context.Context, instanceJobPayload, []managedFileSpec) error

var instanceConfigValidators = map[string]instanceConfigValidator{
	driver.Nginx:       validateNginxConfig,
	driver.HTTPProxy:   validateHTTPProxyConfig,
	driver.WireGuard:   validateWireGuardConfig,
	driver.XrayCore:    validateXrayConfig,
	driver.MTProto:     validateXrayConfig,
	driver.OpenVPN:     validateConfigPresence,
	driver.IPSec:       validateConfigPresence,
	driver.Shadowsocks: validateShadowsocksConfig,
}

func validateRenderedConfig(ctx context.Context, payload instanceJobPayload, files []managedFileSpec) error {
	validator := instanceConfigValidators[driver.NormalizeCode(payload.ServiceCode)]
	if validator == nil {
		return nil
	}
	return validator(ctx, payload, files)
}

func validateNginxConfig(ctx context.Context, _ instanceJobPayload, _ []managedFileSpec) error {
	code, out := runInstallCommand(ctx, "nginx", "-t")
	if code != 0 {
		return fmt.Errorf("nginx config validation failed: %s", truncate(out, 3000))
	}
	return nil
}

func validateHTTPProxyConfig(ctx context.Context, payload instanceJobPayload, files []managedFileSpec) error {
	configPath := firstConfigPath(filterConfigFiles(files), defaultConfigPath(payload))
	if configPath == "" {
		return nil
	}
	code, out := runInstallCommand(ctx, "squid", "-k", "parse", "-f", configPath)
	if code != 0 {
		return fmt.Errorf("http proxy config validation failed: %s", truncate(out, 3000))
	}
	return nil
}

func validateWireGuardConfig(ctx context.Context, payload instanceJobPayload, files []managedFileSpec) error {
	configPath := firstConfigPath(filterConfigFiles(files), defaultConfigPath(payload))
	if configPath == "" {
		return nil
	}
	code, out := runInstallCommand(ctx, "wg-quick", "strip", configPath)
	if code != 0 {
		return fmt.Errorf("wireguard config validation failed: %s", truncate(out, 3000))
	}
	return nil
}

func validateXrayConfig(ctx context.Context, payload instanceJobPayload, files []managedFileSpec) error {
	configPath := firstConfigPath(filterConfigFiles(files), defaultConfigPath(payload))
	if configPath == "" {
		return nil
	}
	code, out := runInstallCommand(ctx, "xray", "run", "-test", "-config", configPath)
	if code != 0 {
		code, out = runInstallCommand(ctx, "xray", "-test", "-config", configPath)
	}
	if code != 0 {
		return fmt.Errorf("xray config validation failed: %s", truncate(out, 3000))
	}
	return nil
}

func validateConfigPresence(_ context.Context, payload instanceJobPayload, files []managedFileSpec) error {
	file, ok := firstConfigFile(filterConfigFiles(files), "")
	if !ok || file.Path == "" {
		return nil
	}
	if strings.TrimSpace(file.Content) == "" {
		return fmt.Errorf("%s config validation failed: config content is empty", payload.ServiceCode)
	}
	return nil
}

func validateShadowsocksConfig(_ context.Context, payload instanceJobPayload, files []managedFileSpec) error {
	file, ok := firstConfigFile(filterConfigFiles(files), "")
	if !ok || file.Path == "" || strings.TrimSpace(file.Content) == "" {
		return nil
	}
	if !json.Valid([]byte(file.Content)) {
		return fmt.Errorf("%s config validation failed: config content must be valid JSON", payload.ServiceCode)
	}
	return nil
}

func firstConfigFile(files []managedFileSpec, fallback string) (managedFileSpec, bool) {
	for _, file := range files {
		if strings.TrimSpace(file.Path) != "" {
			return file, true
		}
	}
	if strings.TrimSpace(fallback) == "" {
		return managedFileSpec{}, false
	}
	return managedFileSpec{Path: fallback}, true
}

func firstConfigPath(files []managedFileSpec, fallback string) string {
	file, ok := firstConfigFile(files, fallback)
	if !ok {
		return ""
	}
	return file.Path
}

func filterConfigFiles(files []managedFileSpec) []managedFileSpec {
	out := make([]managedFileSpec, 0, len(files))
	for _, file := range files {
		if strings.HasSuffix(file.Path, ".service") {
			continue
		}
		out = append(out, file)
	}
	return out
}

func currentUnitState(unit string) string {
	if strings.TrimSpace(unit) == "" {
		return "unknown"
	}
	return normalizeSystemctlState(strings.TrimSpace(runOutput("systemctl", "is-active", unit)))
}
