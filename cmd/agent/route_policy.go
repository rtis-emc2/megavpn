package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

const defaultRoutePolicyPath = "/etc/megavpn/client-access-routes.json"

func (c client) applyRoutePolicy(ctx context.Context, j job, st agentState) (string, map[string]any) {
	nodeID := stringify(j.Payload["node_id"])
	if nodeID == "" {
		nodeID = st.NodeID
	}
	if st.NodeID != "" && nodeID != "" && nodeID != st.NodeID {
		return "failed", map[string]any{"error": "route policy node_id does not match enrolled agent", "payload_node_id": nodeID, "agent_node_id": st.NodeID}
	}
	outputPath := stringify(j.Payload["output_path"])
	if outputPath == "" {
		outputPath = defaultRoutePolicyPath
	}
	if !isSafeRoutePolicyPath(outputPath) {
		return "failed", map[string]any{"error": "route policy output_path is outside /etc/megavpn", "output_path": outputPath}
	}
	routes, ok := j.Payload["routes"].([]any)
	if !ok {
		return "failed", map[string]any{"error": "route policy routes payload must be an array"}
	}
	systemRoutes, ok := j.Payload["system_routes"].([]any)
	if !ok {
		systemRoutes = []any{}
	}
	previousSnapshot, previousSnapshotResult := readRoutePolicySnapshot(outputPath)
	snapshot := cloneRoutePolicyPayload(j.Payload)
	snapshot["node_id"] = nodeID
	snapshot["output_path"] = outputPath
	b, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return "failed", map[string]any{"error": err.Error(), "stage": "marshal_route_policy"}
	}
	if err := writeManagedFile(managedFileSpec{Path: outputPath, Content: string(b) + "\n", Mode: "0640"}); err != nil {
		return "failed", map[string]any{"error": err.Error(), "stage": "write_route_policy", "output_path": outputPath}
	}
	kernelPlan, err := renderRoutePolicyKernelScript(routes, systemRoutes, stringify(j.Payload["revision"]), previousSnapshot)
	if err != nil {
		return "failed", map[string]any{"error": err.Error(), "stage": "render_route_policy_enforcement", "output_path": outputPath}
	}
	kernelResult := map[string]any{
		"enforced":          false,
		"rule_count":        kernelPlan.RuleCount,
		"system_rule_count": kernelPlan.SystemRuleCount,
		"marked_count":      kernelPlan.MarkedCount,
		"skipped_count":     kernelPlan.SkippedCount,
		"skipped_reasons":   kernelPlan.Reasons,
		"previous_snapshot": previousSnapshotResult,
	}
	telemetryTables := routePolicyTelemetryTables(routes, systemRoutes, previousSnapshot)
	if strings.TrimSpace(kernelPlan.Script) != "" {
		if !isSafeRoutePolicyManagedPath(routePolicyScriptPath) || !isSafeRoutePolicyManagedPath(routePolicyUnitPath) || !isSafeRoutePolicyManagedPath(routePolicyTimerPath) {
			return "failed", map[string]any{"error": "route policy managed path is not allowed", "stage": "route_policy_enforcement_paths"}
		}
		if err := writeManagedFile(managedFileSpec{Path: routePolicyScriptPath, Content: kernelPlan.Script, Mode: "0700"}); err != nil {
			return "failed", map[string]any{"error": err.Error(), "stage": "write_route_policy_script", "path": routePolicyScriptPath}
		}
		if err := writeManagedFile(managedFileSpec{Path: routePolicyUnitPath, Content: renderRoutePolicyUnit(), Mode: "0644"}); err != nil {
			return "failed", map[string]any{"error": err.Error(), "stage": "write_route_policy_unit", "path": routePolicyUnitPath}
		}
		if err := writeManagedFile(managedFileSpec{Path: routePolicyTimerPath, Content: renderRoutePolicyTimer(), Mode: "0644"}); err != nil {
			return "failed", map[string]any{"error": err.Error(), "stage": "write_route_policy_timer", "path": routePolicyTimerPath}
		}
		_, _ = runInstallCommand(ctx, "systemctl", "daemon-reload")
		timerCode, timerOut := runInstallCommand(ctx, "systemctl", "enable", "--now", routePolicyTimerName)
		code, out := runInstallCommand(ctx, "systemctl", "restart", routePolicyUnitName)
		kernelResult["unit"] = routePolicyUnitName
		kernelResult["timer"] = routePolicyTimerName
		kernelResult["script_path"] = routePolicyScriptPath
		kernelResult["output"] = truncate(out, 2000)
		kernelResult["timer_output"] = truncate(timerOut, 2000)
		kernelResult["enforced"] = code == 0 && timerCode == 0
		if timerCode != 0 {
			errText := routePolicyExecutionError("route policy refresh timer failed", timerOut)
			kernelResult["error"] = errText
			kernelResult["telemetry"] = collectRoutePolicyKernelTelemetry(ctx, telemetryTables...)
			return "failed", map[string]any{"error": errText, "kernel": kernelResult}
		}
		if code != 0 {
			errText := routePolicyExecutionError("route policy kernel enforcement failed", out)
			kernelResult["error"] = errText
			kernelResult["telemetry"] = collectRoutePolicyKernelTelemetry(ctx, telemetryTables...)
			return "failed", map[string]any{"error": errText, "kernel": kernelResult}
		}
		kernelResult["telemetry"] = collectRoutePolicyKernelTelemetry(ctx, telemetryTables...)
	}
	activeCount := 0
	enforceableCount := 0
	observeOnlyCount := 0
	egressCandidateCount := 0
	egressBlockedCount := 0
	for _, item := range routes {
		route, _ := item.(map[string]any)
		if strings.EqualFold(stringify(route["status"]), "active") {
			activeCount++
		}
		egress, _ := route["egress"].(map[string]any)
		switch stringify(egress["status"]) {
		case "candidate":
			egressCandidateCount++
		default:
			egressBlockedCount++
		}
		enforcement, _ := route["enforcement"].(map[string]any)
		switch stringify(enforcement["mode"]) {
		case "l3_l4_candidate":
			enforceableCount++
		default:
			observeOnlyCount++
		}
	}
	message := "route policy snapshot applied"
	if kernelPlan.RuleCount+kernelPlan.SystemRuleCount > 0 {
		message = "route policy snapshot and kernel egress enforcement applied"
	} else if kernelResult["enforced"] == true {
		message = "route policy snapshot and kernel egress cleanup applied"
	}
	activeSystemRouteCount := 0
	for _, item := range systemRoutes {
		route, _ := item.(map[string]any)
		if strings.EqualFold(stringify(route["status"]), "active") {
			activeSystemRouteCount++
		}
	}
	return "succeeded", map[string]any{
		"message":                   message,
		"node_id":                   nodeID,
		"output_path":               outputPath,
		"revision":                  stringify(j.Payload["revision"]),
		"enforcement_mode":          first(stringify(j.Payload["enforcement_mode"]), "snapshot"),
		"enforced":                  kernelResult["enforced"],
		"kernel":                    kernelResult,
		"enforceable_routes":        enforceableCount,
		"observe_only_routes":       observeOnlyCount,
		"egress_candidate_routes":   egressCandidateCount,
		"egress_blocked_routes":     egressBlockedCount,
		"route_count":               len(routes),
		"active_route_count":        activeCount,
		"system_route_count":        len(systemRoutes),
		"active_system_route_count": activeSystemRouteCount,
	}
}

func routePolicyExecutionError(prefix, output string) string {
	detail := firstLine(strings.TrimSpace(output))
	if detail == "" {
		return prefix
	}
	return prefix + ": " + detail
}

func (c client) cleanupRoutePolicy(ctx context.Context, j job, st agentState) (string, map[string]any) {
	nodeID := stringify(j.Payload["node_id"])
	if nodeID == "" {
		nodeID = st.NodeID
	}
	if st.NodeID != "" && nodeID != "" && nodeID != st.NodeID {
		return "failed", map[string]any{"error": "route policy cleanup node_id does not match enrolled agent", "payload_node_id": nodeID, "agent_node_id": st.NodeID}
	}
	outputPath := stringify(j.Payload["output_path"])
	if outputPath == "" {
		outputPath = defaultRoutePolicyPath
	}
	if !isSafeRoutePolicyPath(outputPath) {
		return "failed", map[string]any{"error": "route policy output_path is outside /etc/megavpn", "output_path": outputPath}
	}
	payloadSnapshot := routePolicySnapshotFromPayload(j.Payload)
	fileSnapshot, fileSnapshotResult := readRoutePolicySnapshot(outputPath)
	combined := routePolicyCleanupSnapshot{
		Routes:       append(append([]any{}, payloadSnapshot.Routes...), fileSnapshot.Routes...),
		SystemRoutes: append(append([]any{}, payloadSnapshot.SystemRoutes...), fileSnapshot.SystemRoutes...),
	}
	plan := renderRoutePolicyCleanupScript(combined, stringify(j.Payload["revision"]))
	telemetryTables := routePolicyTelemetryTables(combined.Routes, combined.SystemRoutes)
	result := map[string]any{
		"message":           "route policy cleanup completed",
		"node_id":           nodeID,
		"output_path":       outputPath,
		"unit":              routePolicyUnitName,
		"timer":             routePolicyTimerName,
		"script_path":       routePolicyCleanupScriptPath,
		"cleanup_targets":   plan.MarkedCount,
		"skipped_count":     plan.SkippedCount,
		"skipped_reasons":   plan.Reasons,
		"previous_snapshot": fileSnapshotResult,
		"telemetry_before":  collectRoutePolicyKernelTelemetry(ctx, telemetryTables...),
	}
	stopCode, stopOut := runInstallCommand(ctx, "systemctl", "disable", "--now", routePolicyTimerName, routePolicyUnitName)
	result["stop_output"] = truncate(strings.TrimSpace(stopOut), 2000)
	if stopCode != 0 && !isMissingSystemdUnitOutput(stopOut) {
		result["stop_warning"] = "route policy unit/timer stop returned non-zero"
	}
	if !isSafeRoutePolicyManagedPath(routePolicyCleanupScriptPath) {
		return "failed", map[string]any{"error": "route policy cleanup script path is not allowed", "stage": "route_policy_cleanup_path"}
	}
	if err := writeManagedFile(managedFileSpec{Path: routePolicyCleanupScriptPath, Content: plan.Script, Mode: "0700"}); err != nil {
		return "failed", map[string]any{"error": err.Error(), "stage": "write_route_policy_cleanup_script", "path": routePolicyCleanupScriptPath}
	}
	code, out := runInstallCommand(ctx, routePolicyCleanupScriptPath)
	result["cleanup_output"] = truncate(strings.TrimSpace(out), 4000)
	result["cleanup_exit_code"] = code
	removed := []string{}
	skipped := []string{}
	warnings := []string{}
	for _, path := range []string{outputPath, routePolicyScriptPath, routePolicyCleanupScriptPath, routePolicyUnitPath, routePolicyTimerPath} {
		wasRemoved, err := removeManagedPath(path, false)
		if err != nil {
			warnings = append(warnings, path+": "+err.Error())
			continue
		}
		if wasRemoved {
			removed = append(removed, path)
		} else {
			skipped = append(skipped, path+": not found")
		}
	}
	_, _ = runInstallCommand(ctx, "systemctl", "daemon-reload")
	_, _ = runInstallCommand(ctx, "systemctl", "reset-failed", routePolicyUnitName, routePolicyTimerName)
	result["removed_paths"] = removed
	result["skipped_paths"] = skipped
	if len(warnings) > 0 {
		result["warnings"] = warnings
	}
	result["telemetry_after"] = collectRoutePolicyKernelTelemetry(ctx, telemetryTables...)
	if code != 0 {
		result["error"] = "route policy cleanup script failed"
		return "failed", result
	}
	return "succeeded", result
}

func collectRoutePolicyKernelTelemetry(ctx context.Context, tables ...string) map[string]any {
	steps := make([]any, 0, 5)
	add := func(label, name string, args ...string) {
		code, out := runInstallCommand(ctx, name, args...)
		steps = append(steps, map[string]any{
			"label":     label,
			"command":   append([]string{name}, args...),
			"exit_code": code,
			"output":    truncate(strings.TrimSpace(out), 4000),
		})
	}
	add("route_policy_unit_active", "systemctl", "is-active", routePolicyUnitName)
	add("route_policy_timer_active", "systemctl", "is-active", routePolicyTimerName)
	add("ip_rule_show", "ip", "rule", "show")
	add("nft_route_policy_output", "nft", "list", "chain", "inet", "megavpn", "route_policy_output")
	add("nft_route_policy_prerouting", "nft", "list", "chain", "inet", "megavpn", "route_policy_prerouting")
	for _, table := range routePolicySafeSortedTables(tables...) {
		add("ip_route_show_table_"+table, "ip", "route", "show", "table", table)
	}
	return map[string]any{
		"collected_at": time.Now().UTC().Format(time.RFC3339),
		"steps":        steps,
	}
}

func routePolicyTelemetryTables(routes []any, systemRoutes []any, snapshots ...routePolicyCleanupSnapshot) []string {
	tables := []string{}
	add := func(table string) {
		table = strings.TrimSpace(table)
		if table == "" || strings.EqualFold(table, "main") || !isSafeNetworkToken(table) {
			return
		}
		tables = append(tables, table)
	}
	for _, item := range systemRoutes {
		route, _ := item.(map[string]any)
		if route == nil {
			continue
		}
		add(stringify(route["table"]))
	}
	for _, item := range routes {
		route, _ := item.(map[string]any)
		if route == nil {
			continue
		}
		add(stringify(nestedMap(route, "egress")["table"]))
	}
	for _, snapshot := range snapshots {
		targets, _ := routePolicyCleanupTargetsFromSnapshot(snapshot)
		for _, target := range targets {
			add(target.Table)
		}
	}
	return routePolicySafeSortedTables(tables...)
}

func routePolicySafeSortedTables(tables ...string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, table := range tables {
		table = strings.TrimSpace(table)
		if table == "" || strings.EqualFold(table, "main") || !isSafeNetworkToken(table) || seen[table] {
			continue
		}
		seen[table] = true
		out = append(out, table)
	}
	sort.Strings(out)
	if len(out) > 20 {
		return out[:20]
	}
	return out
}

func readRoutePolicySnapshot(path string) (routePolicyCleanupSnapshot, map[string]any) {
	result := map[string]any{"path": path, "found": false, "loaded": false}
	if !isSafeRoutePolicyPath(path) {
		result["error"] = "unsafe route policy snapshot path"
		return routePolicyCleanupSnapshot{}, result
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return routePolicyCleanupSnapshot{}, result
		}
		result["error"] = err.Error()
		return routePolicyCleanupSnapshot{}, result
	}
	result["found"] = true
	var payload map[string]any
	if err := json.Unmarshal(b, &payload); err != nil {
		result["error"] = err.Error()
		return routePolicyCleanupSnapshot{}, result
	}
	snapshot := routePolicySnapshotFromPayload(payload)
	result["loaded"] = true
	result["route_count"] = len(snapshot.Routes)
	result["system_route_count"] = len(snapshot.SystemRoutes)
	return snapshot, result
}

func routePolicySnapshotFromPayload(payload map[string]any) routePolicyCleanupSnapshot {
	if payload == nil {
		return routePolicyCleanupSnapshot{}
	}
	routes, _ := payload["routes"].([]any)
	systemRoutes, _ := payload["system_routes"].([]any)
	return routePolicyCleanupSnapshot{
		Routes:       append([]any{}, routes...),
		SystemRoutes: append([]any{}, systemRoutes...),
	}
}

func cloneRoutePolicyPayload(payload map[string]any) map[string]any {
	out := make(map[string]any, len(payload))
	for key, value := range payload {
		out[key] = value
	}
	return out
}

func isSafeRoutePolicyPath(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	if strings.Contains(path, "\x00") || strings.Contains(path, "..") {
		return false
	}
	return path == defaultRoutePolicyPath || strings.HasPrefix(path, "/etc/megavpn/")
}

func isSafeRoutePolicyManagedPath(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" || strings.Contains(path, "\x00") || strings.Contains(path, "..") {
		return false
	}
	if strings.HasPrefix(path, "/usr/local/lib/megavpn/route-policy/") && strings.HasSuffix(path, ".sh") {
		return true
	}
	return path == routePolicyUnitPath || path == routePolicyTimerPath
}

func validateRoutePolicyPayload(payload map[string]any) error {
	if payload == nil {
		return fmt.Errorf("route policy payload is empty")
	}
	if _, ok := payload["routes"].([]any); !ok {
		return fmt.Errorf("route policy routes must be an array")
	}
	if raw, ok := payload["system_routes"]; ok {
		if _, ok := raw.([]any); !ok {
			return fmt.Errorf("route policy system_routes must be an array")
		}
	}
	return nil
}
