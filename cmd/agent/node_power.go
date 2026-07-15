package main

import (
	"context"
	"strings"
	"time"
	"unicode"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

const (
	nodeRebootSystemdDelaySeconds  = 5
	nodeRebootShutdownDelaySeconds = 60
	nodeRebootOutputLimit          = 1200
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
	if err := domain.ValidateNodeRebootReason(reason); err != nil {
		return "failed", map[string]any{"error": "reboot reason is missing or invalid"}
	}
	result := map[string]any{
		"message":       "node reboot command scheduled",
		"node_id":       nodeID,
		"node_name":     expectedConfirmation,
		"reason":        reason,
		"agent_version": appVersion,
		"delay_seconds": nodeRebootSystemdDelaySeconds,
	}
	systemdUSRUnit := nodeRebootUnitName(j.ID, "usr")
	systemdSbinUnit := nodeRebootUnitName(j.ID, "sbin")

	attempts := []struct {
		mechanism    string
		name         string
		args         []string
		unit         string
		delaySeconds int
	}{
		{
			mechanism:    "systemd-run",
			name:         "systemd-run",
			args:         []string{"--unit", systemdUSRUnit, "--description", reason, "--on-active=5s", "/usr/sbin/reboot"},
			unit:         systemdUSRUnit,
			delaySeconds: nodeRebootSystemdDelaySeconds,
		},
		{
			mechanism:    "systemd-run",
			name:         "systemd-run",
			args:         []string{"--unit", systemdSbinUnit, "--description", reason, "--on-active=5s", "/sbin/reboot"},
			unit:         systemdSbinUnit,
			delaySeconds: nodeRebootSystemdDelaySeconds,
		},
		{
			mechanism:    "shutdown",
			name:         "shutdown",
			args:         []string{"-r", "+1", reason},
			delaySeconds: nodeRebootShutdownDelaySeconds,
		},
	}

	steps := make([]map[string]any, 0, len(attempts))
	for idx, attempt := range attempts {
		code, out := runInstallCommand(ctx, attempt.name, attempt.args...)
		step := map[string]any{
			"mechanism": attempt.mechanism,
			"command":   append([]string{attempt.name}, attempt.args...),
			"exit_code": code,
			"output":    redactNodeRebootCommandOutput(out),
		}
		if attempt.unit != "" {
			step["unit"] = attempt.unit
		}
		steps = append(steps, step)
		if code == 0 {
			result["scheduled"] = true
			result["command"] = step["command"]
			result["mechanism"] = attempt.mechanism
			result["fallback_used"] = idx > 0
			result["delay_seconds"] = attempt.delaySeconds
			result["steps"] = steps
			if attempt.unit != "" {
				result["unit"] = attempt.unit
			}
			result["duration_ms"] = time.Since(started).Milliseconds()
			return "succeeded", result
		}
	}

	result["scheduled"] = false
	result["steps"] = steps
	result["error"] = "failed to schedule node reboot command"
	result["duration_ms"] = time.Since(started).Milliseconds()
	return "failed", result
}

func redactNodeRebootCommandOutput(output string) string {
	output = truncate(strings.TrimSpace(output), nodeRebootOutputLimit)
	lower := strings.ToLower(output)
	for _, marker := range []string{
		"authorization:",
		"bearer ",
		"agent_token",
		"enrollment_token",
		"token_hash",
		"x-megavpn-agent-signature",
		"x-megavpn-agent-nonce",
		"private_key",
		"secret_ref",
		"password",
	} {
		if strings.Contains(lower, marker) {
			return "[redacted sensitive reboot command output]"
		}
	}
	return output
}

func nodeRebootUnitName(jobID, suffix string) string {
	base := strings.ToLower(strings.TrimSpace(jobID))
	var b strings.Builder
	for _, r := range base {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	safeID := strings.Trim(b.String(), "-_.")
	if safeID == "" {
		safeID = "unknown"
	}
	safeSuffix := strings.Trim(nodeRebootUnitSuffix(suffix), "-_.")
	if safeSuffix == "" {
		safeSuffix = "run"
	}
	const prefix = "megavpn-agent-reboot-"
	const maxUnitNameLength = 80
	maxIDLen := maxUnitNameLength - len(prefix) - len(safeSuffix) - 1
	if maxIDLen < 8 {
		maxIDLen = 8
	}
	if len(safeID) > maxIDLen {
		safeID = strings.Trim(safeID[:maxIDLen], "-_.")
		if safeID == "" {
			safeID = "unknown"
		}
	}
	return prefix + safeID + "-" + safeSuffix
}

func nodeRebootUnitSuffix(suffix string) string {
	suffix = strings.ToLower(strings.TrimSpace(suffix))
	var b strings.Builder
	for _, r := range suffix {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return b.String()
}
