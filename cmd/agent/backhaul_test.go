package main

import "testing"

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
