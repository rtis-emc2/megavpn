package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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
	kernelPlan, err := renderRoutePolicyKernelScript(routes, stringify(j.Payload["revision"]))
	if err != nil {
		return "failed", map[string]any{"error": err.Error(), "stage": "render_route_policy_enforcement", "output_path": outputPath}
	}
	kernelResult := map[string]any{
		"enforced":        false,
		"rule_count":      kernelPlan.RuleCount,
		"marked_count":    kernelPlan.MarkedCount,
		"skipped_count":   kernelPlan.SkippedCount,
		"skipped_reasons": kernelPlan.Reasons,
	}
	if kernelPlan.RuleCount > 0 {
		if !isSafeRoutePolicyManagedPath(routePolicyScriptPath) || !isSafeRoutePolicyManagedPath(routePolicyUnitPath) {
			return "failed", map[string]any{"error": "route policy managed path is not allowed", "stage": "route_policy_enforcement_paths"}
		}
		if err := writeManagedFile(managedFileSpec{Path: routePolicyScriptPath, Content: kernelPlan.Script, Mode: "0700"}); err != nil {
			return "failed", map[string]any{"error": err.Error(), "stage": "write_route_policy_script", "path": routePolicyScriptPath}
		}
		if err := writeManagedFile(managedFileSpec{Path: routePolicyUnitPath, Content: renderRoutePolicyUnit(), Mode: "0644"}); err != nil {
			return "failed", map[string]any{"error": err.Error(), "stage": "write_route_policy_unit", "path": routePolicyUnitPath}
		}
		_, _ = runInstallCommand(ctx, "systemctl", "daemon-reload")
		code, out := runInstallCommand(ctx, "systemctl", "enable", "--now", routePolicyUnitName)
		kernelResult["unit"] = routePolicyUnitName
		kernelResult["script_path"] = routePolicyScriptPath
		kernelResult["output"] = truncate(out, 2000)
		kernelResult["enforced"] = code == 0
		if code != 0 {
			kernelResult["error"] = "route policy kernel enforcement failed"
			return "failed", map[string]any{"error": "route policy kernel enforcement failed", "kernel": kernelResult}
		}
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
	if kernelPlan.RuleCount > 0 {
		message = "route policy snapshot and kernel egress enforcement applied"
	}
	return "succeeded", map[string]any{
		"message":                 message,
		"node_id":                 nodeID,
		"output_path":             outputPath,
		"revision":                stringify(j.Payload["revision"]),
		"enforcement_mode":        first(stringify(j.Payload["enforcement_mode"]), "snapshot"),
		"enforced":                kernelResult["enforced"],
		"kernel":                  kernelResult,
		"enforceable_routes":      enforceableCount,
		"observe_only_routes":     observeOnlyCount,
		"egress_candidate_routes": egressCandidateCount,
		"egress_blocked_routes":   egressBlockedCount,
		"route_count":             len(routes),
		"active_route_count":      activeCount,
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
	return path == routePolicyUnitPath
}

func validateRoutePolicyPayload(payload map[string]any) error {
	if payload == nil {
		return fmt.Errorf("route policy payload is empty")
	}
	if _, ok := payload["routes"].([]any); !ok {
		return fmt.Errorf("route policy routes must be an array")
	}
	return nil
}
