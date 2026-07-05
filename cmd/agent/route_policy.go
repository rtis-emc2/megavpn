package main

import (
	"context"
	"encoding/json"
	"fmt"
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
	kernelPlan, err := renderRoutePolicyKernelScript(routes, systemRoutes, stringify(j.Payload["revision"]))
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
	}
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
			kernelResult["error"] = "route policy refresh timer failed"
			kernelResult["telemetry"] = collectRoutePolicyKernelTelemetry(ctx)
			return "failed", map[string]any{"error": "route policy refresh timer failed", "kernel": kernelResult}
		}
		if code != 0 {
			kernelResult["error"] = "route policy kernel enforcement failed"
			kernelResult["telemetry"] = collectRoutePolicyKernelTelemetry(ctx)
			return "failed", map[string]any{"error": "route policy kernel enforcement failed", "kernel": kernelResult}
		}
		kernelResult["telemetry"] = collectRoutePolicyKernelTelemetry(ctx)
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

func collectRoutePolicyKernelTelemetry(ctx context.Context) map[string]any {
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
	return map[string]any{
		"collected_at": time.Now().UTC().Format(time.RFC3339),
		"steps":        steps,
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
