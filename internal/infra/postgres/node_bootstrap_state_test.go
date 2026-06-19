package postgres

import "testing"

func TestShouldHandoffNodeBootstrapToAgentForSSHBootstrap(t *testing.T) {
	if !shouldHandoffNodeBootstrapToAgent(
		map[string]any{"bootstrap_mode": "ssh_bootstrap"},
		map[string]any{"bootstrap_mode": "ssh_bootstrap"},
	) {
		t.Fatal("expected ssh_bootstrap to hand off lifecycle to agent-managed channel")
	}
}

func TestShouldHandoffNodeBootstrapToAgentUsesResultMode(t *testing.T) {
	if !shouldHandoffNodeBootstrapToAgent(
		map[string]any{"bootstrap_mode": "manual_bundle"},
		map[string]any{"bootstrap_mode": "ssh_bootstrap"},
	) {
		t.Fatal("expected result bootstrap mode to define handoff behavior")
	}
}

func TestShouldHandoffNodeBootstrapToAgentSkipsManualBundle(t *testing.T) {
	if shouldHandoffNodeBootstrapToAgent(
		map[string]any{"bootstrap_mode": "manual_bundle"},
		map[string]any{"bootstrap_mode": "manual_bundle"},
	) {
		t.Fatal("manual_bundle only prepares operator materials and must not switch channel")
	}
}
