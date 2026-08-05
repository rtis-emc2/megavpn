package main

import (
	"context"
	"time"
)

func (c client) rebootNode(ctx context.Context, j job, st agentState) (string, map[string]any) {
	started := time.Now()
	nodeID := stringify(j.Payload["node_id"])
	if nodeID == "" {
		nodeID = st.NodeID
	}
	if st.NodeID != "" && nodeID != "" && nodeID != st.NodeID {
		return "failed", map[string]any{"error": "reboot node_id does not match enrolled agent", "payload_node_id": nodeID, "agent_node_id": st.NodeID}
	}
	expectedConfirmation := stringify(j.Payload["node_name"])
	if expectedConfirmation == "" {
		expectedConfirmation = st.NodeName
	}
	if expectedConfirmation == "" || stringify(j.Payload["confirmation"]) != expectedConfirmation {
		return "failed", map[string]any{"error": "reboot confirmation is missing or invalid", "expected_node_name": expectedConfirmation}
	}

	reason := stringify(j.Payload["reason"])
	if reason == "" {
		reason = "MegaVPN agent requested reboot"
	}
	result := map[string]any{
		"message":       "node reboot scheduled",
		"node_id":       nodeID,
		"node_name":     expectedConfirmation,
		"reason":        reason,
		"agent_version": appVersion,
		"delay_seconds": 5,
	}

	attempts := []struct {
		name string
		args []string
	}{
		{name: "systemd-run", args: []string{"--unit", "megavpn-agent-requested-reboot", "--description", reason, "--on-active=5s", "/usr/sbin/reboot"}},
		{name: "systemd-run", args: []string{"--unit", "megavpn-agent-requested-reboot", "--description", reason, "--on-active=5s", "/sbin/reboot"}},
		{name: "shutdown", args: []string{"-r", "+1", reason}},
	}

	steps := make([]map[string]any, 0, len(attempts))
	for _, attempt := range attempts {
		code, out := runInstallCommand(ctx, attempt.name, attempt.args...)
		step := map[string]any{
			"command":   append([]string{attempt.name}, attempt.args...),
			"exit_code": code,
			"output":    truncate(out, 2000),
		}
		steps = append(steps, step)
		if code == 0 {
			result["scheduled"] = true
			result["command"] = step["command"]
			result["steps"] = steps
			if attempt.name == "shutdown" {
				result["delay_seconds"] = 60
				result["message"] = "node reboot scheduled with shutdown fallback"
			}
			result["duration_ms"] = time.Since(started).Milliseconds()
			return "succeeded", result
		}
	}

	result["scheduled"] = false
	result["steps"] = steps
	result["error"] = "failed to schedule node reboot"
	result["duration_ms"] = time.Since(started).Milliseconds()
	return "failed", result
}
