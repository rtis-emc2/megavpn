package postgres

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"strings"
	"testing"

	"github.com/rtis-emc2/megavpn/internal/domain"
	"golang.org/x/crypto/ssh"
)

func TestNormalizeNodeSSHAccessMethodCreateInput(t *testing.T) {
	t.Parallel()

	validKey := generatedOpenSSHPrivateKey(t)
	input := domain.NodeSSHAccessMethodCreateInput{
		SSHHost:          " bootstrap.example.com ",
		SSHUser:          " support ",
		SSHHostKeySHA256: " SHA256:abcdefghijklmnopqrstuvwxyzABCDEFGH1234567890+/= ",
		PrivateKey:       validKey,
		IsEnabled:        true,
	}
	normalized, keyBytes, err := normalizeNodeSSHAccessMethodCreateInput(input)
	if err != nil {
		t.Fatalf("normalize valid input: %v", err)
	}
	defer zeroBytes(keyBytes)
	if normalized.SSHHost != "bootstrap.example.com" {
		t.Fatalf("ssh_host = %q", normalized.SSHHost)
	}
	if normalized.SSHPort != 22 {
		t.Fatalf("default ssh_port = %d, want 22", normalized.SSHPort)
	}
	if normalized.SSHUser != "support" {
		t.Fatalf("ssh_user = %q", normalized.SSHUser)
	}
	if string(keyBytes) != validKey {
		t.Fatal("private key bytes should preserve request material until encryption")
	}
}

func TestNormalizeNodeSSHAccessMethodCreateInputRejectsUnsafeFields(t *testing.T) {
	t.Parallel()

	validKey := generatedOpenSSHPrivateKey(t)
	base := domain.NodeSSHAccessMethodCreateInput{
		SSHHost:          "bootstrap.example.com",
		SSHPort:          22,
		SSHUser:          "support",
		SSHHostKeySHA256: "SHA256:abcdefghijklmnopqrstuvwxyzABCDEFGH1234567890+/=",
		PrivateKey:       validKey,
		IsEnabled:        true,
	}
	cases := []struct {
		name string
		edit func(*domain.NodeSSHAccessMethodCreateInput)
	}{
		{name: "empty host", edit: func(in *domain.NodeSSHAccessMethodCreateInput) { in.SSHHost = " " }},
		{name: "unsafe host", edit: func(in *domain.NodeSSHAccessMethodCreateInput) { in.SSHHost = "-oProxyCommand=sh" }},
		{name: "invalid port", edit: func(in *domain.NodeSSHAccessMethodCreateInput) { in.SSHPort = 70000 }},
		{name: "empty user", edit: func(in *domain.NodeSSHAccessMethodCreateInput) { in.SSHUser = " " }},
		{name: "unsafe user", edit: func(in *domain.NodeSSHAccessMethodCreateInput) { in.SSHUser = "root;sh" }},
		{name: "missing fingerprint", edit: func(in *domain.NodeSSHAccessMethodCreateInput) { in.SSHHostKeySHA256 = "" }},
		{name: "bad fingerprint", edit: func(in *domain.NodeSSHAccessMethodCreateInput) { in.SSHHostKeySHA256 = "SHA1:bad" }},
		{name: "empty key", edit: func(in *domain.NodeSSHAccessMethodCreateInput) { in.PrivateKey = " " }},
		{name: "public key", edit: func(in *domain.NodeSSHAccessMethodCreateInput) {
			in.PrivateKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITestOnly"
		}},
		{name: "malformed key", edit: func(in *domain.NodeSSHAccessMethodCreateInput) {
			in.PrivateKey = strings.Join([]string{
				"-----BEGIN OPENSSH " + "PRIVATE KEY-----",
				"not-valid",
				"-----END OPENSSH " + "PRIVATE KEY-----",
			}, "\n")
		}},
		{name: "encrypted key", edit: func(in *domain.NodeSSHAccessMethodCreateInput) {
			in.PrivateKey = generatedEncryptedOpenSSHPrivateKey(t)
		}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			input := base
			tc.edit(&input)
			_, keyBytes, err := normalizeNodeSSHAccessMethodCreateInput(input)
			if keyBytes != nil {
				t.Fatal("key bytes should not be returned for invalid input")
			}
			var validationErr domain.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("error = %T %v, want domain.ValidationError", err, err)
			}
			if strings.Contains(err.Error(), "BEGIN OPENSSH PRIVATE KEY") {
				t.Fatalf("error leaked key material: %q", err.Error())
			}
		})
	}
}

func TestPrepareSecretRefRequiresSecretService(t *testing.T) {
	t.Parallel()

	store := New(nil)
	_, err := store.prepareSecretRef("ssh_key", []byte(generatedOpenSSHPrivateKey(t)), map[string]any{"purpose": "test"})
	if !errors.Is(err, ErrSecretServiceUnavailable) {
		t.Fatalf("error = %v, want ErrSecretServiceUnavailable", err)
	}
}

func generatedOpenSSHPrivateKey(t *testing.T) string {
	t.Helper()

	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	block, err := ssh.MarshalPrivateKey(key, "generated-test-key")
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	return string(pem.EncodeToMemory(block))
}

func generatedEncryptedOpenSSHPrivateKey(t *testing.T) string {
	t.Helper()

	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate encrypted key: %v", err)
	}
	block, err := ssh.MarshalPrivateKeyWithPassphrase(key, "generated-test-key", []byte("passphrase"))
	if err != nil {
		t.Fatalf("marshal encrypted key: %v", err)
	}
	return string(pem.EncodeToMemory(block))
}
