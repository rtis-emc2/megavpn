package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func (c client) emergencyCleanupNode(ctx context.Context, j job, st agentState) (string, map[string]any) {
	started := time.Now()
	nodeID := stringify(j.Payload["node_id"])
	if nodeID == "" {
		nodeID = st.NodeID
	}
	if st.NodeID != "" && nodeID != "" && nodeID != st.NodeID {
		return "failed", map[string]any{"error": "cleanup node_id does not match enrolled agent", "payload_node_id": nodeID, "agent_node_id": st.NodeID}
	}
	if stringify(j.Payload["confirmation"]) != "ERASE_NODE_MANAGED_STATE" {
		return "failed", map[string]any{"error": "cleanup confirmation is missing or invalid"}
	}

	includeAgent := boolFromAny(j.Payload["include_agent"])
	result := map[string]any{
		"message":       "node emergency cleanup completed",
		"node_id":       nodeID,
		"include_agent": includeAgent,
	}
	instanceResults, instanceFailed := emergencyCleanupInstances(ctx, j.Payload["instances"])
	leftovers := emergencyCleanupManagedLeftovers(ctx)
	result["instances"] = instanceResults
	result["leftovers"] = leftovers

	warnings := stringSliceFromAny(leftovers["warnings"])
	if instanceFailed {
		warnings = append(warnings, "one or more instance cleanup operations failed")
	}
	if includeAgent {
		agentCleanup, err := scheduleAgentSelfRemoval(ctx)
		result["agent_self_removal"] = agentCleanup
		if err != nil {
			result["error"] = err.Error()
			result["duration_ms"] = time.Since(started).Milliseconds()
			return "failed", result
		}
	}
	if len(warnings) > 0 {
		result["warnings"] = warnings
	}
	result["duration_ms"] = time.Since(started).Milliseconds()
	if instanceFailed {
		result["error"] = "node cleanup finished with failed instance cleanup operations"
		return "failed", result
	}
	return "succeeded", result
}

func emergencyCleanupInstances(ctx context.Context, raw any) ([]map[string]any, bool) {
	items, _ := raw.([]any)
	results := make([]map[string]any, 0, len(items))
	failed := false
	exec := instanceRuntimeExecutor{}
	for _, item := range items {
		payloadMap, ok := item.(map[string]any)
		if !ok {
			failed = true
			results = append(results, map[string]any{"status": "failed", "error": "instance cleanup payload is not an object"})
			continue
		}
		payloadMap["action"] = "delete"
		payload, err := decodeInstanceJobPayload(payloadMap)
		if err != nil {
			failed = true
			results = append(results, map[string]any{"status": "failed", "error": err.Error()})
			continue
		}
		status, result := exec.Delete(ctx, payload)
		if result == nil {
			result = map[string]any{}
		}
		result["status"] = status
		result["instance_id"] = payload.InstanceID
		result["service_code"] = payload.ServiceCode
		if status != "succeeded" {
			failed = true
		}
		results = append(results, result)
	}
	return results, failed
}

func emergencyCleanupManagedLeftovers(ctx context.Context) map[string]any {
	stoppedUnits := []string{}
	removedPaths := []string{}
	skippedItems := []string{}
	warnings := []string{}

	for _, unit := range emergencyCleanupUnitNames() {
		code, out := runInstallCommand(ctx, "systemctl", "disable", "--now", unit)
		if code != 0 && !isMissingSystemdUnitOutput(out) {
			warnings = append(warnings, "systemd disable warning for "+unit+": "+firstLine(out))
		} else if code == 0 {
			stoppedUnits = append(stoppedUnits, unit)
		}
		_, _ = runInstallCommand(ctx, "systemctl", "reset-failed", unit)
		if path := emergencyManagedUnitFilePath(unit); path != "" {
			removed, err := removeManagedPath(path, false)
			if err != nil {
				warnings = append(warnings, path+": "+err.Error())
			} else if removed {
				removedPaths = append(removedPaths, path)
			} else {
				skippedItems = append(skippedItems, path+": not found - skip")
			}
		}
	}

	for _, path := range []string{
		"/etc/megavpn/client-access-routes.json",
		"/etc/systemd/system/megavpn-route-policy.service",
	} {
		removed, err := removeManagedPath(path, false)
		if err != nil {
			warnings = append(warnings, path+": "+err.Error())
		} else if removed {
			removedPaths = append(removedPaths, path)
		} else {
			skippedItems = append(skippedItems, path+": not found - skip")
		}
	}

	for _, path := range []string{
		"/etc/megavpn/backhaul",
		"/etc/megavpn/certs",
		"/var/lib/megavpn/openvpn",
	} {
		removed, err := removeManagedPath(path, true)
		if err != nil {
			warnings = append(warnings, path+": "+err.Error())
		} else if removed {
			removedPaths = append(removedPaths, path)
		} else {
			skippedItems = append(skippedItems, path+": not found - skip")
		}
	}

	for _, pattern := range []string{
		"/etc/nginx/conf.d/megavpn-*.conf",
		"/etc/systemd/system/megavpn-*.service",
	} {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			warnings = append(warnings, pattern+": "+err.Error())
			continue
		}
		for _, path := range matches {
			if !isEmergencyManagedFilePath(path) {
				continue
			}
			removed, err := removeManagedPath(path, false)
			if err != nil {
				warnings = append(warnings, path+": "+err.Error())
			} else if removed {
				removedPaths = append(removedPaths, path)
			}
		}
	}

	_, _ = runInstallCommand(ctx, "systemctl", "daemon-reload")
	return map[string]any{
		"stopped_units": stoppedUnits,
		"removed_paths": removedPaths,
		"skipped_items": skippedItems,
		"warnings":      warnings,
	}
}

func emergencyCleanupUnitNames() []string {
	seen := map[string]bool{}
	add := func(unit string) {
		unit = strings.TrimSpace(unit)
		unit = strings.TrimSuffix(unit, ".service") + ".service"
		if isEmergencyManagedUnit(unit) {
			seen[unit] = true
		}
	}
	for _, raw := range []string{
		runCombinedOutput("systemctl", "list-unit-files", "--type=service", "--no-legend", "--no-pager"),
		runCombinedOutput("systemctl", "list-units", "--type=service", "--all", "--no-legend", "--no-pager"),
	} {
		for _, line := range strings.Split(raw, "\n") {
			fields := strings.Fields(line)
			if len(fields) > 0 {
				add(fields[0])
			}
		}
	}
	matches, _ := filepath.Glob("/etc/systemd/system/megavpn-*.service")
	for _, path := range matches {
		add(filepath.Base(path))
	}
	out := make([]string, 0, len(seen))
	for unit := range seen {
		out = append(out, unit)
	}
	sort.Strings(out)
	return out
}

func isEmergencyManagedUnit(unit string) bool {
	unit = strings.TrimSpace(unit)
	if unit == "" || !isSafeSystemdUnitToken(unit) {
		return false
	}
	unit = strings.TrimSuffix(unit, ".service")
	if unit == "megavpn-route-policy" {
		return true
	}
	for _, prefix := range []string{
		"megavpn-backhaul-",
		"megavpn-netpolicy-",
		"megavpn-xray-",
		"megavpn-mtproto-",
		"megavpn-shadowsocks-",
		"megavpn-http-proxy-",
	} {
		if strings.HasPrefix(unit, prefix) {
			return true
		}
	}
	return false
}

func emergencyManagedUnitFilePath(unit string) string {
	unit = strings.TrimSpace(unit)
	if !isEmergencyManagedUnit(unit) {
		return ""
	}
	unit = strings.TrimSuffix(unit, ".service") + ".service"
	return "/etc/systemd/system/" + unit
}

func isEmergencyManagedFilePath(path string) bool {
	path = cleanAbsPath(path)
	if path == "" || strings.Contains(path, "..") || hasUnsafePathToken(path) {
		return false
	}
	if strings.HasPrefix(path, "/etc/nginx/conf.d/megavpn-") && strings.HasSuffix(path, ".conf") {
		return true
	}
	if strings.HasPrefix(path, "/etc/systemd/system/megavpn-") && strings.HasSuffix(path, ".service") {
		return true
	}
	return false
}

func scheduleAgentSelfRemoval(ctx context.Context) (map[string]any, error) {
	scriptPath := "/run/megavpn-agent-self-remove.sh"
	script := `#!/bin/sh
set -eu
systemctl disable --now megavpn-agent.service >/dev/null 2>&1 || true
rm -f /etc/systemd/system/megavpn-agent.service
rm -f /etc/megavpn/agent.env /etc/megavpn/agent-bootstrap.env
rm -rf /var/lib/megavpn/agent
rm -f /opt/megavpn/bin/megavpn-agent
rmdir /opt/megavpn/bin >/dev/null 2>&1 || true
rmdir /opt/megavpn >/dev/null 2>&1 || true
systemctl daemon-reload >/dev/null 2>&1 || true
rm -f "$0"
`
	if err := os.WriteFile(scriptPath, []byte(script), 0700); err != nil {
		return map[string]any{"scheduled": false, "error": err.Error()}, err
	}
	code, out := runInstallCommand(ctx, "systemd-run", "--unit", "megavpn-agent-self-remove", "--on-active=5s", "/bin/sh", scriptPath)
	result := map[string]any{
		"scheduled": code == 0,
		"script":    scriptPath,
		"output":    truncate(out, 2000),
	}
	if code != 0 {
		return result, fmt.Errorf("agent self-removal schedule failed: %s", firstLine(out))
	}
	return result, nil
}

func boolFromAny(value any) bool {
	switch x := value.(type) {
	case bool:
		return x
	case string:
		switch strings.ToLower(strings.TrimSpace(x)) {
		case "true", "1", "yes":
			return true
		}
	}
	return false
}

func stringSliceFromAny(value any) []string {
	items, ok := value.([]string)
	if ok {
		return append([]string{}, items...)
	}
	raw, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s := stringify(item); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func mapFromAny(value any) map[string]any {
	item, ok := value.(map[string]any)
	if ok {
		return item
	}
	b, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil
	}
	return out
}
