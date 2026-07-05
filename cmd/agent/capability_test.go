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

func TestPGPFingerprintOutputContains(t *testing.T) {
	t.Parallel()

	output := "pub:-:4096:1:ABF5BD827BD9BF62:1563811491:::-:::scESC::::::23::0:\n" +
		"fpr:::::::::573BFD6B3D8FBC641079A6ABABF5BD827BD9BF62:\n"
	if !pgpFingerprintOutputContains(output, "573B FD6B 3D8F BC64 1079 A6AB ABF5 BD82 7BD9 BF62") {
		t.Fatal("expected nginx.org signing key fingerprint to be accepted")
	}
	if pgpFingerprintOutputContains(output, "0000000000000000000000000000000000000000") {
		t.Fatal("unexpected fingerprint match")
	}
}

func TestSafeAptCodename(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"jammy", "noble", "bookworm", "ubuntu-24.04"} {
		if !safeAptCodename(value) {
			t.Fatalf("expected codename %q to be accepted", value)
		}
	}
	for _, value := range []string{"", "noble main", "noble\nmain", "$(id)", "Noble"} {
		if safeAptCodename(value) {
			t.Fatalf("expected codename %q to be rejected", value)
		}
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

func TestAptInstallFailureResultIncludesLastFailedCommand(t *testing.T) {
	t.Parallel()

	result := aptInstallFailureResult("shadowsocks", "shadowsocks-libev", "package install failed", []map[string]any{
		{
			"command":   []string{"apt-get", "update"},
			"exit_code": 0,
			"output":    "ok",
		},
		{
			"command":   []string{"env", "DEBIAN_FRONTEND=noninteractive", "apt-get", "install", "-y", "shadowsocks-libev"},
			"exit_code": 100,
			"output":    "E: Sub-process /usr/bin/dpkg returned an error code (1)\nsecond line",
		},
	})
	message := stringify(result["message"])
	if !strings.Contains(message, "failed command: env DEBIAN_FRONTEND=noninteractive apt-get install -y shadowsocks-libev") {
		t.Fatalf("message missing command: %q", message)
	}
	if !strings.Contains(message, "output: E: Sub-process /usr/bin/dpkg returned an error code (1)") {
		t.Fatalf("message missing first output line: %q", message)
	}
	if got := stringify(result["last_failed_command"]); got == "" {
		t.Fatal("last_failed_command is empty")
	}
	if got := result["last_failed_exit_code"]; got != 100 {
		t.Fatalf("last_failed_exit_code = %#v, want 100", got)
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

func TestAptFailureSuggestsRepair(t *testing.T) {
	t.Parallel()

	if !aptFailureSuggestsRepair("E: dpkg was interrupted, you must manually run 'sudo dpkg --configure -a' to correct the problem.") {
		t.Fatal("expected interrupted dpkg output to request repair")
	}
	if !aptFailureSuggestsRepair("dpkg: error processing package shadowsocks-libev (--configure): installed shadowsocks-libev package post-installation script subprocess returned error exit status 1") {
		t.Fatal("expected package post-install failure to request repair")
	}
	if aptFailureSuggestsRepair("Temporary failure resolving archive.ubuntu.com") {
		t.Fatal("network resolution failure must not request dpkg repair")
	}
}

func TestAptSandboxSetuidFallbackCommand(t *testing.T) {
	t.Parallel()

	output := "E: seteuid 42 failed - seteuid (1: Operation not permitted)\nE: Method gave invalid 400 URI Failure message: Failed to set new user ids - setresuid (1: Operation not permitted)"
	if !aptFailureLooksSandboxSetuid(output) {
		t.Fatal("expected apt sandbox setuid failure to be detected")
	}
	name, args, ok := aptSandboxRootFallbackCommand("env", "DEBIAN_FRONTEND=noninteractive", "apt-get", "-o", "Dpkg::Lock::Timeout=120", "install", "-y", "shadowsocks-libev")
	if !ok {
		t.Fatal("expected apt sandbox fallback command")
	}
	if name != "env" {
		t.Fatalf("fallback name = %q, want env", name)
	}
	got := strings.Join(args, " ")
	if !strings.Contains(got, "apt-get -o APT::Sandbox::User=root -o Dpkg::Lock::Timeout=120 install -y shadowsocks-libev") {
		t.Fatalf("fallback args = %q", got)
	}
	if aptFailureLooksSandboxSetuid("Temporary failure resolving archive.ubuntu.com") {
		t.Fatal("network failure must not trigger apt sandbox fallback")
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

func TestUbuntuUniverseRepositoryPreparationSelection(t *testing.T) {
	t.Parallel()

	if !shouldPrepareUbuntuUniverseRepository("ubuntu", "shadowsocks-libev") {
		t.Fatal("expected shadowsocks-libev on Ubuntu to request universe repository preparation")
	}
	if shouldPrepareUbuntuUniverseRepository("debian", "shadowsocks-libev") {
		t.Fatal("did not expect Debian to request Ubuntu universe repository preparation")
	}
	if shouldPrepareUbuntuUniverseRepository("ubuntu", "openvpn") {
		t.Fatal("did not expect mainline OpenVPN package to request universe repository preparation")
	}
}

func TestOSReleaseIDFromContent(t *testing.T) {
	t.Parallel()

	got := osReleaseIDFromContent("NAME=\"Ubuntu\"\nID=ubuntu\nVERSION_ID=\"24.04\"\n")
	if got != "ubuntu" {
		t.Fatalf("os release id = %q, want ubuntu", got)
	}
	quoted := osReleaseIDFromContent("ID=\"ubuntu\"\n")
	if quoted != "ubuntu" {
		t.Fatalf("quoted os release id = %q, want ubuntu", quoted)
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

func TestPackageServiceAutostartPreventionSelection(t *testing.T) {
	t.Parallel()

	if !shouldPreventPackageServiceAutostart("shadowsocks") {
		t.Fatal("expected Shadowsocks package install to block distro service autostart")
	}
	if shouldPreventPackageServiceAutostart("openvpn") {
		t.Fatal("did not expect OpenVPN package install to block distro service autostart")
	}
}

func TestHasBinaryRepositoryInstallPayload(t *testing.T) {
	t.Parallel()

	j := job{Payload: map[string]any{
		"binary_repository_fallback": map[string]any{"artifact_id": "artifact-1"},
	}}
	if !hasBinaryRepositoryInstallPayload(j, "binary_repository_fallback") {
		t.Fatal("expected fallback binary repository payload to be detected")
	}
	if hasBinaryRepositoryInstallPayload(j, "binary_repository") {
		t.Fatal("unexpected primary binary repository payload")
	}
}
