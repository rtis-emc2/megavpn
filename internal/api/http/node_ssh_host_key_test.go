package http

import "testing"

func TestParseSSHHostKeyFingerprints(t *testing.T) {
	knownHosts := sshKnownHostKeyLines(`
# 203.0.113.10:22 SSH-2.0-OpenSSH_9.6
203.0.113.10 ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITestEd25519
203.0.113.10 rsa-sha2-256 AAAAB3NzaC1yc2EAAAADAQABAAABAQCTestRSA
`)
	if len(knownHosts) != 2 {
		t.Fatalf("known host lines len = %d, want 2", len(knownHosts))
	}
	got := parseSSHHostKeyFingerprints(`
256 SHA256:qwertyuiopasdfghjklzxcvbnm1234567890abcd 203.0.113.10 (ED25519)
3072 SHA256:abcdefghijklmnoqrstuvwxyz1234567890ABCDE 203.0.113.10 (RSA)
`, knownHosts)
	if len(got) != 2 {
		t.Fatalf("fingerprints len = %d, want 2", len(got))
	}
	if got[0].Fingerprint != "SHA256:qwertyuiopasdfghjklzxcvbnm1234567890abcd" || got[0].Algorithm != "ssh-ed25519" || got[0].Bits != 256 {
		t.Fatalf("first fingerprint = %#v", got[0])
	}
	if got[1].Algorithm != "rsa-sha2-256" || got[1].Bits != 3072 {
		t.Fatalf("second fingerprint = %#v", got[1])
	}
}

func TestIsSafeSSHHostKeyScanHost(t *testing.T) {
	valid := []string{
		"node1.example.com",
		"203.0.113.10",
		"2001:db8::10",
		"[2001:db8::10]",
	}
	for _, host := range valid {
		if !isSafeSSHHostKeyScanHost(host) {
			t.Fatalf("expected host %q to be accepted", host)
		}
	}

	invalid := []string{
		"",
		"-oProxyCommand=sh",
		"node1.example.com;whoami",
		"node1.example.com\n127.0.0.1",
		"node1.example.com/foo",
		"[2001:db8::10",
	}
	for _, host := range invalid {
		if isSafeSSHHostKeyScanHost(host) {
			t.Fatalf("expected host %q to be rejected", host)
		}
	}
}
