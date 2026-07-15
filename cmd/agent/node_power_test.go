package main

import (
	"context"
	"strings"
	"testing"
)

type nodePowerCommandCall struct {
	name string
	args []string
}

func TestRebootNodeRejectsUnsafePayloadBeforeCommandExecution(t *testing.T) {
	cases := []struct {
		name    string
		job     job
		state   agentState
		wantErr string
	}{
		{
			name:    "wrong node",
			job:     rebootTestJob("job-wrong-node", "node-other", "edge-01", "edge-01", "maintenance window"),
			state:   agentState{NodeID: "node-1", NodeName: "edge-01"},
			wantErr: "node_id",
		},
		{
			name:    "wrong confirmation",
			job:     rebootTestJob("job-wrong-confirmation", "node-1", "edge-01", "EDGE-01", "maintenance window"),
			state:   agentState{NodeID: "node-1", NodeName: "edge-01"},
			wantErr: "confirmation",
		},
		{
			name:    "missing reason",
			job:     rebootTestJob("job-missing-reason", "node-1", "edge-01", "edge-01", ""),
			state:   agentState{NodeID: "node-1", NodeName: "edge-01"},
			wantErr: "reason",
		},
		{
			name:    "control character reason",
			job:     rebootTestJob("job-control-reason", "node-1", "edge-01", "edge-01", "maintenance\nwindow"),
			state:   agentState{NodeID: "node-1", NodeName: "edge-01"},
			wantErr: "reason",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := installNodePowerCommandRunner(t, nil, nil)
			status, result := client{}.rebootNode(context.Background(), tc.job, tc.state)
			if status != "failed" {
				t.Fatalf("status = %q, want failed; result=%#v", status, result)
			}
			if !strings.Contains(stringify(result["error"]), tc.wantErr) {
				t.Fatalf("error = %#v, want to contain %q", result["error"], tc.wantErr)
			}
			if len(*calls) != 0 {
				t.Fatalf("commands executed for rejected payload: %#v", *calls)
			}
		})
	}
}

func TestRebootNodeSchedulesWithSystemdRunUSRSbin(t *testing.T) {
	calls := installNodePowerCommandRunner(t, []int{0}, []string{"scheduled"})

	status, result := client{}.rebootNode(context.Background(), rebootTestJob("job-usr-1", "node-1", "edge-01", "edge-01", "maintenance window"), agentState{NodeID: "node-1", NodeName: "edge-01"})
	if status != "succeeded" {
		t.Fatalf("status = %q, want succeeded; result=%#v", status, result)
	}
	if result["scheduled"] != true || result["mechanism"] != "systemd-run" || result["fallback_used"] != false || result["delay_seconds"] != nodeRebootSystemdDelaySeconds {
		t.Fatalf("result scheduling metadata mismatch: %#v", result)
	}
	if len(*calls) != 1 {
		t.Fatalf("command calls = %d, want 1", len(*calls))
	}
	assertNodePowerCommand(t, (*calls)[0], "systemd-run", "/usr/sbin/reboot")
	unit := stringify(result["unit"])
	if unit == "" || strings.Contains(unit, "maintenance") || !strings.Contains(unit, "job-usr-1") {
		t.Fatalf("unsafe or missing systemd unit: %q", unit)
	}
	if strings.Contains(strings.ToLower(stringify(result["message"])), "completed") || strings.Contains(strings.ToLower(stringify(result["message"])), "recovered") {
		t.Fatalf("message claims completion/recovery: %#v", result["message"])
	}
}

func TestRebootNodeFallsBackToSecondSystemdPath(t *testing.T) {
	calls := installNodePowerCommandRunner(t, []int{1, 0}, []string{"missing usr", "scheduled sbin"})

	status, result := client{}.rebootNode(context.Background(), rebootTestJob("job-sbin-1", "node-1", "edge-01", "edge-01", "maintenance window"), agentState{NodeID: "node-1", NodeName: "edge-01"})
	if status != "succeeded" {
		t.Fatalf("status = %q, want succeeded; result=%#v", status, result)
	}
	if result["fallback_used"] != true || result["mechanism"] != "systemd-run" || result["delay_seconds"] != nodeRebootSystemdDelaySeconds {
		t.Fatalf("fallback metadata mismatch: %#v", result)
	}
	if len(*calls) != 2 {
		t.Fatalf("command calls = %d, want 2", len(*calls))
	}
	assertNodePowerCommand(t, (*calls)[0], "systemd-run", "/usr/sbin/reboot")
	assertNodePowerCommand(t, (*calls)[1], "systemd-run", "/sbin/reboot")
	firstUnit := nodePowerArgAfter((*calls)[0].args, "--unit")
	secondUnit := nodePowerArgAfter((*calls)[1].args, "--unit")
	if firstUnit == "" || secondUnit == "" || firstUnit == secondUnit {
		t.Fatalf("fallback systemd units must be present and distinct: first=%q second=%q", firstUnit, secondUnit)
	}
}

func TestRebootNodeFallsBackToShutdown(t *testing.T) {
	calls := installNodePowerCommandRunner(t, []int{1, 1, 0}, []string{"missing usr", "missing sbin", "shutdown scheduled"})

	status, result := client{}.rebootNode(context.Background(), rebootTestJob("job-shutdown-1", "node-1", "edge-01", "edge-01", "maintenance window"), agentState{NodeID: "node-1", NodeName: "edge-01"})
	if status != "succeeded" {
		t.Fatalf("status = %q, want succeeded; result=%#v", status, result)
	}
	if result["fallback_used"] != true || result["mechanism"] != "shutdown" || result["delay_seconds"] != nodeRebootShutdownDelaySeconds {
		t.Fatalf("shutdown fallback metadata mismatch: %#v", result)
	}
	if len(*calls) != 3 {
		t.Fatalf("command calls = %d, want 3", len(*calls))
	}
	assertNodePowerCommand(t, (*calls)[2], "shutdown", "-r")
	if got := (*calls)[2].args; len(got) != 3 || got[1] != "+1" || got[2] != "maintenance window" {
		t.Fatalf("shutdown args = %#v", got)
	}
}

func TestRebootNodeReportsBoundedFailure(t *testing.T) {
	longOutput := strings.Repeat("x", nodeRebootOutputLimit+200)
	calls := installNodePowerCommandRunner(t, []int{1, 1, 1}, []string{longOutput, longOutput, longOutput})

	status, result := client{}.rebootNode(context.Background(), rebootTestJob("job-fail-1", "node-1", "edge-01", "edge-01", "maintenance window"), agentState{NodeID: "node-1", NodeName: "edge-01"})
	if status != "failed" {
		t.Fatalf("status = %q, want failed; result=%#v", status, result)
	}
	if result["scheduled"] != false {
		t.Fatalf("scheduled = %#v, want false", result["scheduled"])
	}
	if len(*calls) != 3 {
		t.Fatalf("command calls = %d, want 3", len(*calls))
	}
	steps, ok := result["steps"].([]map[string]any)
	if !ok || len(steps) != 3 {
		t.Fatalf("steps = %#v, want three maps", result["steps"])
	}
	for _, step := range steps {
		output := stringify(step["output"])
		if len(output) > nodeRebootOutputLimit+len("...<truncated>") {
			t.Fatalf("output was not bounded: len=%d", len(output))
		}
	}
}

func TestRebootNodeRedactsSensitiveCommandOutput(t *testing.T) {
	calls := installNodePowerCommandRunner(t, []int{1, 1, 1}, []string{
		"Authorization: Bearer secret",
		"agent_token=secret",
		"plain failure",
	})

	status, result := client{}.rebootNode(context.Background(), rebootTestJob("job-redact-1", "node-1", "edge-01", "edge-01", "maintenance window"), agentState{NodeID: "node-1", NodeName: "edge-01"})
	if status != "failed" {
		t.Fatalf("status = %q, want failed; result=%#v", status, result)
	}
	if len(*calls) != 3 {
		t.Fatalf("command calls = %d, want 3", len(*calls))
	}
	steps, ok := result["steps"].([]map[string]any)
	if !ok || len(steps) != 3 {
		t.Fatalf("steps = %#v, want three maps", result["steps"])
	}
	for _, step := range steps[:2] {
		if output := stringify(step["output"]); output != "[redacted sensitive reboot command output]" {
			t.Fatalf("sensitive output was not redacted: %q", output)
		}
	}
	if output := stringify(steps[2]["output"]); output != "plain failure" {
		t.Fatalf("non-sensitive output = %q, want plain failure", output)
	}
}

func TestNodeRebootUnitNameIsRepeatSafeAndReasonFree(t *testing.T) {
	reason := "maintenance window"
	first := nodeRebootUnitName("JOB/One with spaces", "usr")
	second := nodeRebootUnitName("job-two", "usr")
	if first == second {
		t.Fatalf("unit names must differ: %q", first)
	}
	for _, unit := range []string{first, second} {
		if strings.Contains(unit, reason) || len(unit) > 80 {
			t.Fatalf("unsafe unit name %q", unit)
		}
		for _, r := range unit {
			if !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '-' && r != '_' && r != '.' {
				t.Fatalf("unit contains unsafe character %q in %q", r, unit)
			}
		}
	}
}

func rebootTestJob(jobID, nodeID, nodeName, confirmation, reason string) job {
	payload := map[string]any{
		"node_id":      nodeID,
		"node_name":    nodeName,
		"confirmation": confirmation,
	}
	if reason != "" {
		payload["reason"] = reason
	}
	return job{ID: jobID, Type: "node.reboot", Payload: payload}
}

func installNodePowerCommandRunner(t *testing.T, codes []int, outputs []string) *[]nodePowerCommandCall {
	t.Helper()

	oldRun := runInstallCommand
	calls := []nodePowerCommandCall{}
	runInstallCommand = func(_ context.Context, name string, args ...string) (int, string) {
		calls = append(calls, nodePowerCommandCall{name: name, args: append([]string(nil), args...)})
		idx := len(calls) - 1
		code := 1
		if idx < len(codes) {
			code = codes[idx]
		}
		output := ""
		if idx < len(outputs) {
			output = outputs[idx]
		}
		return code, output
	}
	t.Cleanup(func() { runInstallCommand = oldRun })
	return &calls
}

func assertNodePowerCommand(t *testing.T, call nodePowerCommandCall, wantName, wantExecutable string) {
	t.Helper()

	if call.name != wantName {
		t.Fatalf("command name = %q, want %q", call.name, wantName)
	}
	if len(call.args) == 1 && strings.Contains(call.args[0], " ") {
		t.Fatalf("command used shell-like single arg: %#v", call.args)
	}
	if wantName == "systemd-run" {
		if got := call.args[len(call.args)-1]; got != wantExecutable {
			t.Fatalf("systemd-run executable = %q, want %q; args=%#v", got, wantExecutable, call.args)
		}
		return
	}
	if len(call.args) == 0 || call.args[0] != wantExecutable {
		t.Fatalf("command args = %#v, want first arg %q", call.args, wantExecutable)
	}
}

func nodePowerArgAfter(args []string, flag string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag {
			return args[i+1]
		}
	}
	return ""
}
