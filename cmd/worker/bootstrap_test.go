package main

import (
	"strings"
	"testing"
	"time"

	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/platform/config"
)

func TestNormalizeSSHPrivateKeyAcceptsEscapedNewlines(t *testing.T) {
	input := `-----BEGIN OPENSSH PRIVATE KEY-----\nabc\n-----END OPENSSH PRIVATE KEY-----`
	got, err := normalizeSSHPrivateKey(input)
	if err != nil {
		t.Fatalf("normalize ssh private key: %v", err)
	}
	if !strings.Contains(got, "\nabc\n") {
		t.Fatalf("expected escaped newlines to be normalized, got %q", got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Fatalf("expected trailing newline")
	}
}

func TestNormalizeSSHPrivateKeyRejectsPublicKey(t *testing.T) {
	_, err := normalizeSSHPrivateKey("ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAItest operator@example")
	if err == nil {
		t.Fatal("expected public key to be rejected")
	}
	if !strings.Contains(err.Error(), "public key") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateSSHBootstrapTargetRejectsOptionInjection(t *testing.T) {
	t.Parallel()

	method := domain.NodeAccessMethod{
		SSHHost:          "bootstrap.example.com",
		SSHUser:          "-oProxyCommand=sh",
		SSHHostKeySHA256: "SHA256:abcdefghijklmnopqrstuvwxyzABCDEFGH1234567890+/=",
	}
	if err := validateSSHBootstrapTarget(method); err == nil {
		t.Fatal("expected unsafe ssh_user to be rejected")
	}
}

func TestRenderBootstrapEnvFilesRejectsNewlineInjection(t *testing.T) {
	t.Parallel()

	cfg := config.Config{}
	cfg.Agent.PollInterval = 10 * time.Second
	node := domain.Node{
		ID:      "node-1",
		Name:    "edge-1\nMEGAVPN_AGENT_TOKEN=attacker",
		Address: "203.0.113.10",
	}
	_, _, err := renderBootstrapEnvFiles(cfg, "https://control.example.com", node, "enrollment-token")
	if err == nil {
		t.Fatal("expected newline injection to be rejected")
	}
	if !strings.Contains(err.Error(), "control characters") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRenderBootstrapEnvFilesProducesSimpleEnv(t *testing.T) {
	t.Parallel()

	cfg := config.Config{}
	cfg.Agent.PollInterval = 10 * time.Second
	node := domain.Node{ID: "node-1", Name: "edge-1", Address: "203.0.113.10"}
	agentEnv, bootstrapEnv, err := renderBootstrapEnvFiles(cfg, "https://control.example.com", node, "enrollment-token")
	if err != nil {
		t.Fatalf("render bootstrap env files: %v", err)
	}
	if !strings.Contains(agentEnv, "MEGAVPN_AGENT_POLL_INTERVAL=10s\n") {
		t.Fatalf("agent env missing poll interval: %q", agentEnv)
	}
	if !strings.Contains(bootstrapEnv, "MEGAVPN_AGENT_NODE_NAME=edge-1\n") {
		t.Fatalf("bootstrap env missing node name: %q", bootstrapEnv)
	}
	if !strings.HasSuffix(bootstrapEnv, "\n") {
		t.Fatalf("bootstrap env must end with newline: %q", bootstrapEnv)
	}
}

func TestValidateSSHBootstrapTargetRequiresHostKeyPin(t *testing.T) {
	t.Parallel()

	method := domain.NodeAccessMethod{SSHHost: "bootstrap.example.com", SSHUser: "root"}
	if err := validateSSHBootstrapTarget(method); err == nil {
		t.Fatal("expected missing host-key pin to be rejected")
	}
}

func TestValidateSSHBootstrapTargetStrictHostValidation(t *testing.T) {
	t.Parallel()

	pin := "SHA256:abcdefghijklmnopqrstuvwxyzABCDEFGH1234567890+/="
	for _, host := range []string{"bad..host", "[2001:db8::1", "2001:db8::zz"} {
		method := domain.NodeAccessMethod{SSHHost: host, SSHUser: "root", SSHHostKeySHA256: pin}
		if err := validateSSHBootstrapTarget(method); err == nil {
			t.Fatalf("expected ssh_host %q to be rejected", host)
		}
	}
	method := domain.NodeAccessMethod{SSHHost: "[2001:db8::1]", SSHUser: "root", SSHHostKeySHA256: pin}
	if err := validateSSHBootstrapTarget(method); err != nil {
		t.Fatalf("expected bracketed IPv6 ssh_host to be accepted: %v", err)
	}
}

func TestSSHCommandArgsUseStrictHostKeyCheckingAndSeparator(t *testing.T) {
	t.Parallel()

	s := &sshSession{method: domain.NodeAccessMethod{SSHHost: "bootstrap.example.com", SSHUser: "root", SSHPort: 22, AuthType: "ssh_key"}, keyPath: "/tmp/key"}
	_, args := s.commandArgs("ssh", "true")
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "StrictHostKeyChecking=accept-new") {
		t.Fatalf("ssh args must not use accept-new: %#v", args)
	}
	if !strings.Contains(joined, "StrictHostKeyChecking=yes") {
		t.Fatalf("ssh args must use strict host key checking: %#v", args)
	}
	foundSeparator := false
	for _, arg := range args {
		if arg == "--" {
			foundSeparator = true
			break
		}
	}
	if !foundSeparator {
		t.Fatalf("ssh args must include -- before target: %#v", args)
	}
}

func TestKnownHostFingerprintMatches(t *testing.T) {
	t.Parallel()

	out := "256 SHA256:abcdefghijklmnopqrstuvwxyzABCDEFGH1234567890+/= bootstrap.example.com (ED25519)\n"
	if !knownHostFingerprintMatches(out, "SHA256:abcdefghijklmnopqrstuvwxyzABCDEFGH1234567890+/=") {
		t.Fatal("expected fingerprint match")
	}
	if knownHostFingerprintMatches(out, "SHA256:0000000000000000000000000000000000000000000=") {
		t.Fatal("unexpected fingerprint match")
	}
}

func TestKnownHostFingerprintRejectsChangedHostKey(t *testing.T) {
	t.Parallel()

	knownHostAfterRotation := "256 SHA256:rotatedHostKeyFingerprint1234567890ABCDEFGH= bootstrap.example.com (ED25519)\n"
	pinnedFingerprint := "SHA256:originalHostKeyFingerprint1234567890ABCDEFG="
	if knownHostFingerprintMatches(knownHostAfterRotation, pinnedFingerprint) {
		t.Fatal("changed SSH host key must not match the pinned fingerprint")
	}
}
