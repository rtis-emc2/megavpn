package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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
	}
	peerAddress := stringify(j.Payload["egress_address"])
	if role == "egress" {
		peerAddress = stringify(j.Payload["ingress_address"])
	}
	if stringify(j.Payload["activate"]) == "true" {
		unit := stringify(j.Payload["systemd_unit"])
		if !isSafeBackhaulUnit(unit) {
			result["error"] = "backhaul systemd_unit is invalid"
			return "failed", result
		}
		_, _ = runInstallCommand(ctx, "systemctl", "daemon-reload")
		code, out := runInstallCommand(ctx, "systemctl", "enable", "--now", unit)
		result["systemd_unit"] = unit
		result["systemd_output"] = truncate(out, 2000)
		result["active_state"] = currentUnitState(unit)
		if code != 0 {
			result["error"] = "backhaul systemd activation failed"
			result["health"] = probeBackhaulHealth(ctx, stringify(j.Payload["interface_name"]), peerAddress, true)
			return "failed", result
		}
		result["message"] = "backhaul transport materialized and activated"
	}
	result["health"] = probeBackhaulHealth(ctx, stringify(j.Payload["interface_name"]), peerAddress, stringify(j.Payload["activate"]) == "true")
	return "succeeded", result
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

func isSafeBackhaulUnit(unit string) bool {
	unit = strings.TrimSpace(unit)
	return strings.HasPrefix(unit, "megavpn-backhaul-") && strings.HasSuffix(unit, ".service") && !strings.Contains(unit, "/") && !strings.Contains(unit, "..")
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

func probeBackhaulHealth(ctx context.Context, iface, peerAddress string, activated bool) map[string]any {
	health := map[string]any{
		"status":    "skipped",
		"activated": activated,
	}
	if !activated {
		health["reason"] = "transport materialized without automatic activation"
		return health
	}
	iface = strings.TrimSpace(iface)
	if iface == "" {
		health["status"] = "unhealthy"
		health["reason"] = "interface name is empty"
		return health
	}
	code, out := runInstallCommand(ctx, "ip", "link", "show", "dev", iface)
	health["interface"] = iface
	health["link_output"] = truncate(out, 1000)
	if code != 0 {
		health["status"] = "unhealthy"
		health["reason"] = "interface is not present"
		return health
	}
	peer := hostOnlyAddress(peerAddress)
	if peer == "" {
		health["status"] = "degraded"
		health["reason"] = "peer address is empty"
		return health
	}
	code, out = runInstallCommand(ctx, "ping", "-c", "1", "-W", "2", peer)
	health["peer"] = peer
	health["peer_probe_output"] = truncate(out, 1000)
	if code != 0 {
		health["status"] = "degraded"
		health["reason"] = "peer ping failed"
		return health
	}
	health["status"] = "healthy"
	return health
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
