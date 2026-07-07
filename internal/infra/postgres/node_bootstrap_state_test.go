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

func TestFirewallPortListContains(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name  string
		ports string
		port  int
		want  bool
	}{
		{name: "empty means any", ports: "", port: 22, want: true},
		{name: "single match", ports: "22", port: 22, want: true},
		{name: "list match", ports: "80, 443, 2222", port: 2222, want: true},
		{name: "range match", ports: "2200-2299", port: 2222, want: true},
		{name: "miss", ports: "80,443", port: 22, want: false},
		{name: "invalid ignored", ports: "abc,443", port: 22, want: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := firewallPortListContains(tt.ports, tt.port); got != tt.want {
				t.Fatalf("firewallPortListContains(%q,%d) = %v, want %v", tt.ports, tt.port, got, tt.want)
			}
		})
	}
}
