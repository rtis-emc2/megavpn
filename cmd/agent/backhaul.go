package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const defaultBackhaulRoot = "/etc/megavpn/backhaul/"

func (c client) applyBackhaul(ctx context.Context, j job, st agentState) (string, map[string]any) {
	nodeID := stringify(j.Payload["node_id"])
	if nodeID == "" {
		nodeID = st.NodeID
	}
	if st.NodeID != "" && nodeID != "" && nodeID != st.NodeID {
		return "failed", map[string]any{"error": "backhaul node_id does not match enrolled agent", "payload_node_id": nodeID, "agent_node_id": st.NodeID}
	}
	linkID := stringify(j.Payload["link_id"])
	transportID := stringify(j.Payload["transport_id"])
	role := strings.ToLower(stringify(j.Payload["role"]))
	driver := stringify(j.Payload["driver"])
	if linkID == "" || transportID == "" || role == "" || driver == "" {
		return "failed", map[string]any{"error": "link_id, transport_id, role and driver are required"}
	}
	files, err := decodeManagedFiles(j.Payload["files"])
	if err != nil {
		return "failed", map[string]any{"error": err.Error(), "stage": "decode_files", "link_id": linkID, "transport_id": transportID, "role": role}
	}
	activate := stringify(j.Payload["activate"]) == "true"
	capability := ensureBackhaulRuntimeCapability(ctx, driver, role, activate)
	if capability["ok"] != true {
		return "failed", map[string]any{
			"error":         firstNonEmptyAgentString(stringify(capability["message"]), "backhaul runtime capability is not available"),
			"stage":         "ensure_capability",
			"node_id":       nodeID,
			"link_id":       linkID,
			"transport_id":  transportID,
			"role":          role,
			"driver":        driver,
			"capability":    capability,
			"active_state":  "capability_missing",
			"health":        map[string]any{"status": "unhealthy", "reason": "runtime capability is not available"},
			"activation_ok": false,
		}
	}
	changed := make([]string, 0, len(files)+1)
	for _, file := range files {
		if !isSafeBackhaulManagedPath(file.Path) {
			return "failed", map[string]any{"error": "backhaul managed file path is not allowed", "path": file.Path, "link_id": linkID, "transport_id": transportID, "role": role}
		}
		if err := writeManagedFile(file); err != nil {
			return "failed", map[string]any{"error": err.Error(), "stage": "write_file", "path": file.Path, "link_id": linkID, "transport_id": transportID, "role": role}
		}
		changed = append(changed, file.Path)
	}
	outputPath := stringify(j.Payload["output_path"])
	if outputPath == "" {
		outputPath = defaultBackhaulRoot + linkID + "-" + role + ".json"
	}
	if !isSafeBackhaulManagedPath(outputPath) {
		return "failed", map[string]any{"error": "backhaul manifest path is not allowed", "output_path": outputPath, "link_id": linkID, "transport_id": transportID, "role": role}
	}
	previousManifest := readBackhaulManifest(outputPath)
	manifest := redactedBackhaulManifest(j.Payload)
	b, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "failed", map[string]any{"error": err.Error(), "stage": "marshal_manifest", "link_id": linkID, "transport_id": transportID, "role": role}
	}
	if err := writeManagedFile(managedFileSpec{Path: outputPath, Content: string(b) + "\n", Mode: "0640"}); err != nil {
		return "failed", map[string]any{"error": err.Error(), "stage": "write_manifest", "output_path": outputPath, "link_id": linkID, "transport_id": transportID, "role": role}
	}
	changed = append(changed, outputPath)

	result := map[string]any{
		"message":          "backhaul transport materialized",
		"node_id":          nodeID,
		"link_id":          linkID,
		"transport_id":     transportID,
		"role":             role,
		"driver":           driver,
		"changed_files":    changed,
		"output_path":      outputPath,
		"activation_mode":  stringify(j.Payload["activation_mode"]),
		"interface_name":   stringify(j.Payload["interface_name"]),
		"routing_table":    stringify(j.Payload["routing_table"]),
		"endpoint_host":    stringify(j.Payload["endpoint_host"]),
		"endpoint_port":    stringify(j.Payload["endpoint_port"]),
		"tunnel_cidr":      stringify(j.Payload["tunnel_cidr"]),
		"active_state":     "materialized",
		"enforced":         false,
		"enforcement_note": "client route kernel enforcement is not enabled by backhaul apply",
		"capability":       capability,
	}
	if activate {
		unit := stringify(j.Payload["systemd_unit"])
		if !isSafeBackhaulUnit(unit) {
			result["error"] = "backhaul systemd_unit is invalid"
			return "failed", result
		}
		iface := stringify(j.Payload["interface_name"])
		if !isSafeBackhaulInterface(iface) {
			result["error"] = "backhaul interface_name is invalid"
			return "failed", result
		}
		previousCleanup, err := cleanupPreviousBackhaulRuntime(ctx, previousManifest, unit, iface, changed)
		if len(previousCleanup) > 0 {
			result["previous_runtime_cleanup"] = previousCleanup
		}
		if err != nil {
			result["error"] = err.Error()
			return "failed", result
		}
		if !backhaulUnitNotFound(unit) {
			stopCode, stopOut := runInstallCommand(ctx, "systemctl", "stop", unit)
			result["pre_start_stop_output"] = truncate(stopOut, 2000)
			if stopCode != 0 {
				result["pre_start_stop_warning"] = "existing backhaul unit stop failed: " + firstLine(stopOut)
			}
			_, _ = runInstallCommand(ctx, "systemctl", "reset-failed", unit)
		}
		ifaceCleanup, err := cleanupBackhaulInterface(ctx, iface)
		if err != nil {
			result["error"] = err.Error()
			result["interface_cleanup"] = ifaceCleanup
			return "failed", result
		}
		result["interface_cleanup"] = ifaceCleanup
		_, _ = runInstallCommand(ctx, "systemctl", "daemon-reload")
		code, out := runInstallCommand(ctx, "systemctl", "enable", "--now", unit)
		result["systemd_unit"] = unit
		result["systemd_output"] = truncate(out, 2000)
		result["active_state"] = currentUnitState(unit)
		if code != 0 {
			health := backhaulReadinessHealth(ctx, stringify(j.Payload["interface_name"]), unit, true)
			result["error"] = backhaulHealthError("backhaul systemd activation failed", health)
			result["health"] = health
			addBackhaulHealthFields(result, health)
			return "failed", result
		}
		health := backhaulReadinessHealth(ctx, stringify(j.Payload["interface_name"]), unit, true)
		result["health"] = health
		addBackhaulHealthFields(result, health)
		if stringify(health["status"]) == "unhealthy" {
			result["error"] = backhaulHealthError("backhaul service readiness check failed", health)
			return "failed", result
		}
		result["message"] = "backhaul transport materialized and activated"
		return "succeeded", result
	}
	health := backhaulReadinessHealth(ctx, stringify(j.Payload["interface_name"]), stringify(j.Payload["systemd_unit"]), stringify(j.Payload["activate"]) == "true")
	result["health"] = health
	addBackhaulHealthFields(result, health)
	return "succeeded", result
}

func (c client) probeBackhaul(ctx context.Context, j job, st agentState) (string, map[string]any) {
	nodeID := stringify(j.Payload["node_id"])
	if nodeID == "" {
		nodeID = st.NodeID
	}
	if st.NodeID != "" && nodeID != "" && nodeID != st.NodeID {
		return "failed", map[string]any{"error": "backhaul node_id does not match enrolled agent", "payload_node_id": nodeID, "agent_node_id": st.NodeID}
	}
	linkID := stringify(j.Payload["link_id"])
	transportID := stringify(j.Payload["transport_id"])
	role := strings.ToLower(stringify(j.Payload["role"]))
	driver := stringify(j.Payload["driver"])
	if linkID == "" || transportID == "" || role == "" || driver == "" {
		return "failed", map[string]any{"error": "link_id, transport_id, role and driver are required"}
	}
	unit := stringify(j.Payload["systemd_unit"])
	if unit != "" && !isSafeBackhaulUnit(unit) {
		return "failed", map[string]any{"error": "backhaul systemd_unit is invalid", "systemd_unit": unit}
	}
	count := intFromPayload(j.Payload["probe_count"], 3)
	health := probeBackhaulHealth(ctx, stringify(j.Payload["interface_name"]), stringify(j.Payload["peer_address"]), unit, true, count)
	status := "succeeded"
	if stringify(health["status"]) != "healthy" {
		status = "failed"
	}
	result := map[string]any{
		"message":        "backhaul probe completed",
		"node_id":        nodeID,
		"link_id":        linkID,
		"transport_id":   transportID,
		"role":           role,
		"driver":         driver,
		"interface_name": stringify(j.Payload["interface_name"]),
		"systemd_unit":   unit,
		"peer_address":   stringify(j.Payload["peer_address"]),
		"health":         health,
	}
	addBackhaulHealthFields(result, health)
	return status, result
}

func (c client) cleanupBackhaul(ctx context.Context, j job, st agentState) (string, map[string]any) {
	nodeID := stringify(j.Payload["node_id"])
	if nodeID == "" {
		nodeID = st.NodeID
	}
	if st.NodeID != "" && nodeID != "" && nodeID != st.NodeID {
		return "failed", map[string]any{"error": "backhaul node_id does not match enrolled agent", "payload_node_id": nodeID, "agent_node_id": st.NodeID}
	}
	linkID := stringify(j.Payload["link_id"])
	transportID := stringify(j.Payload["transport_id"])
	role := strings.ToLower(stringify(j.Payload["role"]))
	driver := stringify(j.Payload["driver"])
	if linkID == "" || transportID == "" || role == "" || driver == "" {
		return "failed", map[string]any{"error": "link_id, transport_id, role and driver are required"}
	}
	units := stringSliceFromPayload(j.Payload["systemd_units"])
	paths := stringSliceFromPayload(j.Payload["paths"])
	dirs := stringSliceFromPayload(j.Payload["directories"])
	stoppedUnits := []string{}
	removedPaths := []string{}
	skippedItems := []string{}
	warnings := []string{}
	for _, unit := range units {
		if !isSafeBackhaulUnit(unit) {
			return "failed", map[string]any{"error": "backhaul systemd_unit is invalid", "systemd_unit": unit, "link_id": linkID, "transport_id": transportID, "role": role}
		}
		if backhaulUnitNotFound(unit) {
			skippedItems = append(skippedItems, unit+": not found - skip")
			warnings = append(warnings, "systemd unit not found - skip: "+unit)
			continue
		}
		stopCode, stopOut := runInstallCommand(ctx, "systemctl", "stop", unit)
		if stopCode != 0 {
			warnings = append(warnings, "systemd stop warning for "+unit+": "+firstLine(stopOut))
		}
		disableCode, disableOut := runInstallCommand(ctx, "systemctl", "disable", unit)
		if disableCode != 0 {
			if isMissingSystemdUnitOutput(disableOut) {
				skippedItems = append(skippedItems, unit+": not found - skip")
				warnings = append(warnings, "systemd unit not found - skip: "+unit)
			} else {
				warnings = append(warnings, "systemd disable warning for "+unit+": "+firstLine(disableOut))
			}
		}
		_, _ = runInstallCommand(ctx, "systemctl", "reset-failed", unit)
		stoppedUnits = append(stoppedUnits, unit)
	}
	ifaceCleanup, err := cleanupBackhaulInterface(ctx, stringify(j.Payload["interface_name"]))
	if err != nil {
		return "failed", map[string]any{"error": err.Error(), "interface_cleanup": ifaceCleanup, "link_id": linkID, "transport_id": transportID, "role": role}
	}
	if stringify(ifaceCleanup["status"]) == "removed" {
		removedPaths = append(removedPaths, stringify(ifaceCleanup["interface"])+": interface removed")
	} else if skipped := stringify(ifaceCleanup["status"]); skipped != "" && stringify(ifaceCleanup["interface"]) != "" {
		skippedItems = append(skippedItems, stringify(ifaceCleanup["interface"])+": "+skipped)
	}
	for _, path := range paths {
		if !isSafeBackhaulManagedPath(path) {
			return "failed", map[string]any{"error": "backhaul managed file path is not allowed", "path": path, "link_id": linkID, "transport_id": transportID, "role": role}
		}
		removed, err := removeManagedPath(path, false)
		if err != nil {
			return "failed", map[string]any{"error": err.Error(), "path": path, "link_id": linkID, "transport_id": transportID, "role": role}
		}
		if removed {
			removedPaths = append(removedPaths, path)
		} else {
			skippedItems = append(skippedItems, path+": not found - skip")
		}
	}
	for _, dir := range dirs {
		if !isSafeBackhaulManagedDir(dir) {
			return "failed", map[string]any{"error": "backhaul managed directory is not allowed", "path": dir, "link_id": linkID, "transport_id": transportID, "role": role}
		}
		removed, err := removeManagedPath(dir, true)
		if err != nil {
			return "failed", map[string]any{"error": err.Error(), "path": dir, "link_id": linkID, "transport_id": transportID, "role": role}
		}
		if removed {
			removedPaths = append(removedPaths, dir)
		} else {
			skippedItems = append(skippedItems, dir+": not found - skip")
		}
	}
	_, _ = runInstallCommand(ctx, "systemctl", "daemon-reload")
	return "succeeded", map[string]any{
		"message":           "backhaul transport cleaned from node",
		"node_id":           nodeID,
		"link_id":           linkID,
		"transport_id":      transportID,
		"role":              role,
		"driver":            driver,
		"delete_batch_id":   stringify(j.Payload["delete_batch_id"]),
		"stopped_units":     stoppedUnits,
		"removed_paths":     removedPaths,
		"skipped_items":     skippedItems,
		"warnings":          warnings,
		"interface_cleanup": ifaceCleanup,
	}
}

func isSafeBackhaulManagedPath(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" || strings.Contains(path, "\x00") || strings.Contains(path, "..") {
		return false
	}
	if strings.HasPrefix(path, defaultBackhaulRoot) {
		return true
	}
	if strings.HasPrefix(path, "/etc/systemd/system/megavpn-backhaul-") && strings.HasSuffix(path, ".service") {
		return true
	}
	return false
}

func isSafeBackhaulManagedDir(path string) bool {
	path = strings.TrimRight(strings.TrimSpace(path), "/")
	if path == "" || strings.Contains(path, "\x00") || strings.Contains(path, "..") {
		return false
	}
	if !strings.HasPrefix(path, strings.TrimRight(defaultBackhaulRoot, "/")+"/") {
		return false
	}
	rest := strings.TrimPrefix(path, strings.TrimRight(defaultBackhaulRoot, "/")+"/")
	return rest != "" && !strings.Contains(rest, "/")
}

func isSafeBackhaulUnit(unit string) bool {
	unit = strings.TrimSpace(unit)
	return strings.HasPrefix(unit, "megavpn-backhaul-") && strings.HasSuffix(unit, ".service") && !strings.Contains(unit, "/") && !strings.Contains(unit, "..")
}

func readBackhaulManifest(path string) map[string]any {
	path = strings.TrimSpace(path)
	if path == "" || !isSafeBackhaulManagedPath(path) {
		return nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var manifest map[string]any
	if err := json.Unmarshal(b, &manifest); err != nil {
		return nil
	}
	return manifest
}

func isSafeBackhaulInterface(iface string) bool {
	iface = strings.TrimSpace(iface)
	if iface == "" || len(iface) > 15 || !strings.HasPrefix(iface, "mgbh") {
		return false
	}
	for _, r := range iface {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}

func cleanupPreviousBackhaulRuntime(ctx context.Context, previous map[string]any, currentUnit, currentIface string, currentPaths []string) (map[string]any, error) {
	if len(previous) == 0 {
		return nil, nil
	}
	result := map[string]any{}
	currentPathSet := map[string]bool{}
	for _, path := range currentPaths {
		currentPathSet[strings.TrimSpace(path)] = true
	}
	previousUnit := stringify(previous["systemd_unit"])
	if previousUnit != "" && previousUnit != currentUnit {
		if !isSafeBackhaulUnit(previousUnit) {
			result["previous_systemd_unit"] = previousUnit
			result["previous_unit_status"] = "skipped"
			result["previous_unit_reason"] = "previous systemd unit is invalid"
			return result, fmt.Errorf("previous backhaul systemd_unit is invalid")
		}
		result["previous_systemd_unit"] = previousUnit
		if backhaulUnitNotFound(previousUnit) {
			result["previous_unit_status"] = "skipped"
			result["previous_unit_reason"] = "unit not found - skip"
		} else {
			stopCode, stopOut := runInstallCommand(ctx, "systemctl", "stop", previousUnit)
			result["previous_unit_stop_output"] = truncate(stopOut, 2000)
			if stopCode != 0 {
				result["previous_unit_status"] = "failed"
				result["previous_unit_reason"] = "stop failed"
				return result, fmt.Errorf("previous backhaul unit stop failed for %s: %s", previousUnit, firstLine(stopOut))
			}
			_, _ = runInstallCommand(ctx, "systemctl", "reset-failed", previousUnit)
			result["previous_unit_status"] = "stopped"
		}
		disableCode, disableOut := runInstallCommand(ctx, "systemctl", "disable", previousUnit)
		result["previous_unit_disable_output"] = truncate(disableOut, 2000)
		if disableCode != 0 && !isMissingSystemdUnitOutput(disableOut) {
			result["previous_unit_status"] = "failed"
			result["previous_unit_reason"] = "disable failed"
			return result, fmt.Errorf("previous backhaul unit disable failed for %s: %s", previousUnit, firstLine(disableOut))
		}
	}
	previousIface := stringify(previous["interface_name"])
	if previousIface != "" && previousIface != currentIface {
		ifaceCleanup, err := cleanupBackhaulInterface(ctx, previousIface)
		result["previous_interface_cleanup"] = ifaceCleanup
		if err != nil {
			return result, err
		}
	}
	removedPaths := []string{}
	skippedPaths := []string{}
	for _, path := range backhaulManifestFilePaths(previous) {
		if currentPathSet[path] {
			continue
		}
		if !isSafeBackhaulManagedPath(path) {
			return result, fmt.Errorf("previous backhaul managed file path is not allowed: %s", path)
		}
		removed, err := removeManagedPath(path, false)
		if err != nil {
			return result, err
		}
		if removed {
			removedPaths = append(removedPaths, path)
		} else {
			skippedPaths = append(skippedPaths, path+": not found - skip")
		}
	}
	if len(removedPaths) > 0 {
		result["previous_removed_paths"] = removedPaths
	}
	if len(skippedPaths) > 0 {
		result["previous_skipped_paths"] = skippedPaths
	}
	return result, nil
}

func backhaulManifestFilePaths(manifest map[string]any) []string {
	items, ok := manifest["files"].([]any)
	if !ok {
		return nil
	}
	paths := []string{}
	seen := map[string]bool{}
	for _, item := range items {
		fileMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		path := strings.TrimSpace(stringify(fileMap["path"]))
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		paths = append(paths, path)
	}
	return paths
}

func cleanupBackhaulInterface(ctx context.Context, iface string) (map[string]any, error) {
	iface = strings.TrimSpace(iface)
	result := map[string]any{"interface": iface}
	if iface == "" {
		result["status"] = "skipped"
		result["reason"] = "interface name is empty"
		return result, nil
	}
	if !isSafeBackhaulInterface(iface) {
		result["status"] = "failed"
		result["reason"] = "interface name is not a managed backhaul interface"
		return result, fmt.Errorf("backhaul interface_name is invalid")
	}
	showCode, showOut := runInstallCommand(ctx, "ip", "link", "show", "dev", iface)
	result["link_output"] = truncate(showOut, 1000)
	if showCode != 0 {
		result["status"] = "skipped"
		result["reason"] = "interface not found - skip"
		return result, nil
	}
	deleteCode, deleteOut := runInstallCommand(ctx, "ip", "link", "delete", "dev", iface)
	result["delete_output"] = truncate(deleteOut, 1000)
	if deleteCode != 0 {
		result["status"] = "failed"
		result["reason"] = "interface delete failed"
		return result, fmt.Errorf("backhaul interface cleanup failed for %s: %s", iface, firstLine(deleteOut))
	}
	result["status"] = "removed"
	return result, nil
}

func redactedBackhaulManifest(payload map[string]any) map[string]any {
	out := make(map[string]any, len(payload))
	for key, value := range payload {
		if key == "files" {
			out[key] = redactedBackhaulFileList(value)
			continue
		}
		out[key] = value
	}
	return out
}

func redactedBackhaulFileList(raw any) []map[string]any {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		fileMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, map[string]any{
			"path":          stringify(fileMap["path"]),
			"mode":          stringify(fileMap["mode"]),
			"content_bytes": len(fmt.Sprint(fileMap["content"])),
			"redacted":      true,
		})
	}
	return out
}

func backhaulReadinessHealth(ctx context.Context, iface, unit string, activated bool) map[string]any {
	health := map[string]any{
		"status":     "skipped",
		"activated":  activated,
		"probe_mode": "service_readiness",
	}
	unit = strings.TrimSpace(unit)
	if unit != "" {
		if !isSafeBackhaulUnit(unit) {
			health["status"] = "unhealthy"
			health["reason"] = "systemd unit is invalid"
			health["systemd_unit"] = unit
			return health
		}
		health["systemd_unit"] = unit
	}
	if !activated {
		health["reason"] = "transport materialized without automatic activation"
		return health
	}
	if unit != "" {
		state := waitBackhaulUnitActive(ctx, unit, 12)
		health["active_state"] = state
	}
	if unit != "" && stringify(health["active_state"]) != "active" {
		health["status"] = "unhealthy"
		health["reason"] = "systemd unit is not active"
		_, statusOut := runInstallCommand(ctx, "systemctl", "status", unit, "--no-pager", "-l")
		health["unit_status_output"] = truncate(statusOut, 2000)
		return health
	}
	iface = strings.TrimSpace(iface)
	if iface == "" {
		health["status"] = "unhealthy"
		health["reason"] = "interface name is empty"
		return health
	}
	code, out, attempts := waitBackhaulInterface(ctx, iface, 20)
	health["interface"] = iface
	health["link_output"] = truncate(out, 1000)
	health["interface_attempts"] = attempts
	if code != 0 {
		health["status"] = "unhealthy"
		health["reason"] = "interface is not present"
		return health
	}
	health["status"] = "healthy"
	health["reason"] = "service active and interface present"
	return health
}

func backhaulHealthError(prefix string, health map[string]any) string {
	parts := []string{strings.TrimSpace(prefix)}
	if reason := stringify(health["reason"]); reason != "" {
		parts = append(parts, reason)
	}
	details := []string{}
	if state := stringify(health["active_state"]); state != "" && state != "unknown" {
		details = append(details, "active_state="+state)
	}
	if iface := stringify(health["interface"]); iface != "" {
		details = append(details, "interface="+iface)
	}
	if attempts := stringify(health["interface_attempts"]); attempts != "" {
		details = append(details, "interface_attempts="+attempts)
	}
	out := strings.Join(nonEmptyStrings(parts), ": ")
	if len(details) > 0 {
		out += " (" + strings.Join(details, ", ") + ")"
	}
	return out
}

func nonEmptyStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, strings.TrimSpace(value))
		}
	}
	return out
}

func probeBackhaulHealth(ctx context.Context, iface, peerAddress, unit string, activated bool, count int) map[string]any {
	health := backhaulReadinessHealth(ctx, iface, unit, activated)
	health["probe_mode"] = "connectivity"
	if stringify(health["status"]) != "healthy" {
		return health
	}
	peer := hostOnlyAddress(peerAddress)
	if peer == "" {
		health["status"] = "degraded"
		health["reason"] = "peer address is empty"
		return health
	}
	if count < 1 || count > 5 {
		count = 3
	}
	health["peer"] = peer
	routeCode, routeOut, routeAttempts := waitBackhaulPeerRoute(ctx, peer, iface, 20)
	health["peer_route_output"] = truncate(routeOut, 1000)
	health["route_attempts"] = routeAttempts
	if routeCode != 0 {
		health["status"] = "degraded"
		health["reason"] = "peer route lookup failed"
		return health
	}
	if !routeUsesInterface(routeOut, iface) {
		health["status"] = "degraded"
		health["reason"] = "peer route does not use backhaul interface"
		health["route_warning"] = "expected dev " + iface
		return health
	}
	code, out, pingAttempts := waitBackhaulPeerPing(ctx, peer, count, 8)
	health["peer_probe_output"] = truncate(out, 1000)
	health["ping_attempts"] = pingAttempts
	for key, value := range parsePingStats(out) {
		health[key] = value
	}
	if code != 0 {
		health["status"] = "degraded"
		health["reason"] = "peer ping failed"
		return health
	}
	health["status"] = "healthy"
	health["reason"] = "peer connectivity verified"
	return health
}

func waitBackhaulUnitActive(ctx context.Context, unit string, attempts int) string {
	state := ""
	if attempts < 1 {
		attempts = 1
	}
	for attempt := 0; attempt < attempts; attempt++ {
		state = currentUnitState(unit)
		if state == "active" {
			return state
		}
		if !sleepBackhaulProbe(ctx, time.Second) {
			return state
		}
	}
	return state
}

func waitBackhaulInterface(ctx context.Context, iface string, attempts int) (int, string, int) {
	var code int
	var out string
	if attempts < 1 {
		attempts = 1
	}
	for attempt := 1; attempt <= attempts; attempt++ {
		code, out = runInstallCommand(ctx, "ip", "link", "show", "dev", iface)
		if code == 0 {
			return code, out, attempt
		}
		if !sleepBackhaulProbe(ctx, time.Second) {
			return code, out, attempt
		}
	}
	return code, out, attempts
}

func waitBackhaulPeerRoute(ctx context.Context, peer, iface string, attempts int) (int, string, int) {
	var code int
	var out string
	if attempts < 1 {
		attempts = 1
	}
	for attempt := 1; attempt <= attempts; attempt++ {
		code, out = runInstallCommand(ctx, "ip", "route", "get", peer)
		if code == 0 && routeUsesInterface(out, iface) {
			return code, out, attempt
		}
		if !sleepBackhaulProbe(ctx, time.Second) {
			return code, out, attempt
		}
	}
	return code, out, attempts
}

func waitBackhaulPeerPing(ctx context.Context, peer string, count, attempts int) (int, string, int) {
	var code int
	var out string
	if attempts < 1 {
		attempts = 1
	}
	if count < 1 || count > 5 {
		count = 3
	}
	for attempt := 1; attempt <= attempts; attempt++ {
		code, out = runInstallCommand(ctx, "ping", "-c", strconv.Itoa(count), "-W", "2", peer)
		if code == 0 {
			return code, out, attempt
		}
		if !sleepBackhaulProbe(ctx, time.Second) {
			return code, out, attempt
		}
	}
	return code, out, attempts
}

func sleepBackhaulProbe(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func routeUsesInterface(routeOut, iface string) bool {
	iface = strings.TrimSpace(iface)
	if iface == "" {
		return false
	}
	fields := strings.Fields(routeOut)
	for idx := 0; idx < len(fields)-1; idx++ {
		if fields[idx] == "dev" && fields[idx+1] == iface {
			return true
		}
	}
	return false
}

func parsePingStats(out string) map[string]any {
	stats := map[string]any{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "packet loss") {
			for _, part := range strings.Split(line, ",") {
				part = strings.TrimSpace(part)
				if !strings.Contains(part, "packet loss") {
					continue
				}
				raw := strings.TrimSpace(strings.TrimSuffix(part, "packet loss"))
				raw = strings.TrimSuffix(raw, "%")
				if v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64); err == nil {
					stats["packet_loss_percent"] = v
				}
			}
		}
		if strings.Contains(line, "min/avg/max") && strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			values := strings.Fields(strings.TrimSpace(parts[1]))
			if len(values) == 0 {
				continue
			}
			nums := strings.Split(values[0], "/")
			keys := []string{"latency_min_ms", "latency_avg_ms", "latency_max_ms", "latency_stddev_ms"}
			for idx, raw := range nums {
				if idx >= len(keys) {
					break
				}
				if v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64); err == nil {
					stats[keys[idx]] = v
				}
			}
		}
	}
	return stats
}

func addBackhaulHealthFields(result, health map[string]any) {
	if result == nil || health == nil {
		return
	}
	for _, key := range []string{
		"status",
		"reason",
		"error",
		"probe_mode",
		"interface_attempts",
		"packet_loss_percent",
		"latency_min_ms",
		"latency_avg_ms",
		"latency_max_ms",
		"latency_stddev_ms",
		"peer",
		"peer_probe_output",
		"peer_route_output",
		"route_attempts",
		"ping_attempts",
		"route_warning",
		"active_state",
		"unit_status_output",
	} {
		if value, ok := health[key]; ok {
			result["health_"+key] = value
		}
	}
}

func removeManagedPath(path string, recursive bool) (bool, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if recursive {
		return true, os.RemoveAll(path)
	}
	return true, os.Remove(path)
}

func isMissingSystemdUnitOutput(out string) bool {
	out = strings.ToLower(out)
	return strings.Contains(out, "does not exist") ||
		strings.Contains(out, "not loaded") ||
		strings.Contains(out, "not-found") ||
		strings.Contains(out, "not found") ||
		strings.Contains(out, "could not be found") ||
		strings.Contains(out, "no such file")
}

func backhaulUnitNotFound(unit string) bool {
	state := strings.ToLower(strings.TrimSpace(runOutput("systemctl", "show", unit, "-p", "LoadState", "--value")))
	return state == "" || state == "not-found"
}

func ensureBackhaulRuntimeCapability(ctx context.Context, driver, role string, activate bool) map[string]any {
	result := map[string]any{
		"ok":       true,
		"required": activate,
		"driver":   driver,
		"role":     role,
	}
	if !activate {
		result["status"] = "skipped"
		result["message"] = "runtime capability is not required for materialize-only backhaul profile"
		return result
	}

	var capability map[string]any
	switch strings.ToLower(strings.TrimSpace(driver)) {
	case "wireguard":
		capability = verifyWireGuard(ctx)
		if capability["ok"] != true {
			capability = installUbuntuPackageCapability(ctx, "wireguard", "wireguard-tools", "wireguard-tools", nil)
		}
	case "openvpn_udp", "openvpn_tcp_443":
		capability = verifyOpenVPN(ctx)
		if capability["ok"] != true {
			capability = installUbuntuPackageCapability(ctx, "openvpn", "openvpn", "openvpn", []string{"openvpn"})
		}
	default:
		result["status"] = "skipped"
		result["message"] = "driver does not require automatic runtime capability installation"
		return result
	}
	result["runtime"] = capability
	if capability["ok"] != true {
		result["ok"] = false
		result["status"] = "failed"
		result["message"] = "runtime package install/verify failed: " + firstNonEmptyAgentString(stringify(capability["message"]), driver)
		return result
	}

	checks := []map[string]any{
		ensureBackhaulCommand(ctx, "ip", "iproute2"),
	}
	if role == "egress" {
		checks = append(checks, ensureBackhaulCommand(ctx, "nft", "nftables"))
	}
	result["system_commands"] = checks
	for _, check := range checks {
		if check["ok"] != true {
			result["ok"] = false
			result["status"] = "failed"
			result["message"] = "required system command is not available: " + stringify(check["binary"])
			return result
		}
	}
	result["status"] = "succeeded"
	result["message"] = "runtime capability verified"
	return result
}

func ensureBackhaulCommand(ctx context.Context, binary, packageName string) map[string]any {
	out := map[string]any{
		"binary":  binary,
		"package": packageName,
	}
	if path := strings.TrimSpace(runOutput("which", binary)); path != "" {
		out["ok"] = true
		out["status"] = "present"
		out["path"] = path
		return out
	}
	if os.Geteuid() != 0 {
		out["ok"] = false
		out["status"] = "missing"
		out["message"] = packageName + " install requires root"
		return out
	}
	steps := []map[string]any{}
	run := func(name string, args ...string) bool {
		code, output := runInstallCommand(ctx, name, args...)
		steps = append(steps, map[string]any{"command": append([]string{name}, args...), "exit_code": code, "output": truncate(output, 4000)})
		return code == 0
	}
	if !run("apt-get", "update") {
		out["ok"] = false
		out["status"] = "failed"
		out["message"] = "apt update failed before installing " + packageName
		out["steps"] = steps
		return out
	}
	if !run("env", "DEBIAN_FRONTEND=noninteractive", "apt-get", "install", "-y", packageName) {
		out["ok"] = false
		out["status"] = "failed"
		out["message"] = packageName + " package install failed"
		out["steps"] = steps
		return out
	}
	out["steps"] = steps
	if path := strings.TrimSpace(runOutput("which", binary)); path != "" {
		out["ok"] = true
		out["status"] = "installed"
		out["path"] = path
		return out
	}
	out["ok"] = false
	out["status"] = "failed"
	out["message"] = binary + " binary is still missing after installing " + packageName
	return out
}

func stringSliceFromPayload(raw any) []string {
	out := []string{}
	switch value := raw.(type) {
	case []string:
		for _, item := range value {
			if item = strings.TrimSpace(item); item != "" {
				out = append(out, item)
			}
		}
	case []any:
		for _, item := range value {
			if text := stringify(item); text != "" {
				out = append(out, text)
			}
		}
	}
	return out
}

func intFromPayload(raw any, fallback int) int {
	switch value := raw.(type) {
	case int:
		return value
	case int32:
		return int(value)
	case int64:
		return int(value)
	case float64:
		return int(value)
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
			return parsed
		}
	}
	return fallback
}

func hostOnlyAddress(address string) string {
	address = strings.TrimSpace(address)
	if address == "" {
		return ""
	}
	if idx := strings.Index(address, "/"); idx > 0 {
		return address[:idx]
	}
	return address
}

func firstNonEmptyAgentString(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
