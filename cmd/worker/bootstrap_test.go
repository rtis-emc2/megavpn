package main

import (
	"strings"
	"testing"
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
