package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyFileSHA256(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "installer.sh")
	content := []byte("#!/usr/bin/env sh\nexit 0\n")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	if err := verifyFileSHA256(path, hex.EncodeToString(sum[:])); err != nil {
		t.Fatalf("expected checksum to match: %v", err)
	}
	if err := verifyFileSHA256(path, "0000000000000000000000000000000000000000000000000000000000000000"); err == nil {
		t.Fatal("expected checksum mismatch")
	}
	if err := verifyFileSHA256(path, "not-a-sha"); err == nil {
		t.Fatal("expected malformed checksum to be rejected")
	}
}

func TestAptInstallFailureMessageForShadowsocks(t *testing.T) {
	t.Parallel()

	got := aptInstallFailureMessage("shadowsocks", "shadowsocks-libev", "apt update failed")
	for _, want := range []string{"shadowsocks-libev", "ss-server", "apt repositories/network"} {
		if !strings.Contains(got, want) {
			t.Fatalf("message %q does not contain %q", got, want)
		}
	}
}

func TestAptInstallCommandRetryClassification(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		command string
		args    []string
		output  string
		want    bool
	}{
		{
			name:    "apt update network failure retries",
			command: "apt-get",
			args:    []string{"update"},
			output:  "Temporary failure resolving archive.ubuntu.com",
			want:    true,
		},
		{
			name:    "env apt install lock retries",
			command: "env",
			args:    []string{"DEBIAN_FRONTEND=noninteractive", "apt-get", "install", "-y", "shadowsocks-libev"},
			output:  "Could not get lock /var/lib/dpkg/lock-frontend. It is held by process 123",
			want:    true,
		},
		{
			name:    "missing package does not retry",
			command: "env",
			args:    []string{"DEBIAN_FRONTEND=noninteractive", "apt-get", "install", "-y", "missing-package"},
			output:  "E: Unable to locate package missing-package",
			want:    false,
		},
		{
			name:    "non apt command does not retry",
			command: "systemctl",
			args:    []string{"enable", "--now", "shadowsocks-libev"},
			output:  "failed",
			want:    false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldRetryAptInstallCommand(tc.command, tc.args, 100, tc.output)
			if got != tc.want {
				t.Fatalf("shouldRetryAptInstallCommand() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAptPolicyHasCandidate(t *testing.T) {
	t.Parallel()

	if !aptPolicyHasCandidate("Installed: (none)\nCandidate: 3.3.5-1\nVersion table:\n") {
		t.Fatal("expected package candidate to be detected")
	}
	if aptPolicyHasCandidate("Installed: (none)\nCandidate: (none)\nVersion table:\n") {
		t.Fatal("expected missing package candidate to be rejected")
	}
}

func TestShadowsocksUbuntuRuntimeInstallDoesNotEnablePackageUnit(t *testing.T) {
	t.Parallel()

	if got := ubuntuPackageEnableUnits("shadowsocks", "shadowsocks-libev"); len(got) != 0 {
		t.Fatalf("shadowsocks runtime install enable units = %#v, want none", got)
	}
	if got := ubuntuPackageEnableUnits("openvpn", "openvpn"); len(got) != 1 || got[0] != "openvpn" {
		t.Fatalf("openvpn runtime install enable units = %#v, want [openvpn]", got)
	}
}
