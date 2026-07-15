package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEmergencyCleanupManagedUnitAllowlist(t *testing.T) {
	fullNodeAllowed := []string{
		"megavpn-route-policy.service",
		"megavpn-backhaul-link.service",
		"megavpn-netpolicy-edge.service",
		"megavpn-xray-edge.service",
		"megavpn-shadowsocks-edge.service",
		"megavpn-http-proxy-edge.service",
		"megavpn-mtproto-edge.service",
	}
	for _, unit := range fullNodeAllowed {
		if !isEmergencyManagedUnit(unit) {
			t.Fatalf("expected %s to be allowed", unit)
		}
	}
	blocked := []string{
		"ssh.service",
		"nginx.service",
		"openvpn-server@server.service",
		"wg-quick@wg0.service",
		"megavpn-../ssh.service",
		"/etc/systemd/system/megavpn-xray-edge.service",
	}
	for _, unit := range blocked {
		if isEmergencyManagedUnit(unit) {
			t.Fatalf("expected %s to be blocked", unit)
		}
	}
}

func TestEmergencyCleanupVersionedPayloadValidation(t *testing.T) {
	payload := validEmergencyCleanupPayloadForTest()
	scope, err := validateEmergencyCleanupJobPayload(payload, true)
	if err != nil {
		t.Fatalf("validate payload: %v", err)
	}
	if scope != "full_node" {
		t.Fatalf("scope = %q, want full_node", scope)
	}
}

func TestEmergencyCleanupVersionedPayloadRejectsInvalidContract(t *testing.T) {
	cases := []struct {
		name string
		edit func(map[string]any)
		want string
	}{
		{name: "missing reason", edit: func(p map[string]any) { delete(p, "reason") }, want: "reason"},
		{name: "future version", edit: func(p map[string]any) { p["cleanup_plan_version"] = 2 }, want: "unsupported"},
		{name: "legacy scope alias", edit: func(p map[string]any) { p["cleanup_scope"] = "wipe" }, want: "cleanup_scope"},
		{name: "agent removal services only", edit: func(p map[string]any) { p["cleanup_scope"] = "services_only" }, want: "include_agent"},
		{name: "missing instances", edit: func(p map[string]any) { delete(p, "instances") }, want: "instances"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			payload := validEmergencyCleanupPayloadForTest()
			payload["instances"] = []any{}
			tc.edit(payload)
			_, err := validateEmergencyCleanupJobPayload(payload, true)
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want marker %q", err.Error(), tc.want)
			}
		})
	}
}

func TestEmergencyCleanupManagedServiceConfigDetection(t *testing.T) {
	openVPN := emergencyManagedRuntimeConfigHeader + "\nport 1194\n"
	if !isEmergencyManagedOpenVPNConfigContent(openVPN) {
		t.Fatal("expected OpenVPN config with managed header to be detected")
	}
	legacyOpenVPN := "ca /etc/openvpn/server/megavpn-edge/ca.crt\ncert /etc/openvpn/server/megavpn-edge/server.crt\n"
	if !isEmergencyManagedOpenVPNConfigContent(legacyOpenVPN) {
		t.Fatal("expected legacy OpenVPN config with megavpn runtime dir to be detected")
	}
	if isEmergencyManagedOpenVPNConfigContent("ca /etc/openvpn/server/server/ca.crt\n") {
		t.Fatal("did not expect unrelated OpenVPN config to be detected")
	}

	wireGuard := emergencyManagedRuntimeConfigHeader + "\n[Interface]\n"
	if !isEmergencyManagedWireGuardConfigContent(wireGuard) {
		t.Fatal("expected WireGuard config with managed header to be detected")
	}
	legacyWireGuard := "[Interface]\n\n# megavpn-service-access-id = access-1\n[Peer]\n"
	if !isEmergencyManagedWireGuardConfigContent(legacyWireGuard) {
		t.Fatal("expected legacy WireGuard config with managed peer marker to be detected")
	}
	if isEmergencyManagedWireGuardConfigContent("[Interface]\nAddress = 10.0.0.1/24\n") {
		t.Fatal("did not expect unrelated WireGuard config to be detected")
	}
}

func TestEmergencyManagedServiceUnitForConfigPath(t *testing.T) {
	cases := map[string]string{
		"/etc/openvpn/server/edge.conf": "openvpn-server@edge.service",
		"/etc/wireguard/wg-edge.conf":   "wg-quick@wg-edge.service",
	}
	for path, want := range cases {
		if got := emergencyManagedServiceUnitForConfigPath(path); got != want {
			t.Fatalf("unit for %s = %q, want %q", path, got, want)
		}
	}
	for _, path := range []string{
		"/etc/openvpn/client/edge.conf",
		"/tmp/wg-edge.conf",
		"/etc/wireguard/../ssh.conf",
	} {
		if got := emergencyManagedServiceUnitForConfigPath(path); got != "" {
			t.Fatalf("expected %s to be rejected, got unit %q", path, got)
		}
	}
}

func TestEmergencyCleanupServicesOnlyPreservesBackhaulAndRoutePolicy(t *testing.T) {
	allowed := []string{
		"megavpn-netpolicy-edge.service",
		"megavpn-xray-edge.service",
		"megavpn-shadowsocks-edge.service",
		"megavpn-http-proxy-edge.service",
		"megavpn-mtproto-edge.service",
	}
	for _, unit := range allowed {
		if !isEmergencyManagedUnitForScope(unit, "services_only") {
			t.Fatalf("expected %s to be allowed for services_only", unit)
		}
	}
	blocked := []string{
		"megavpn-route-policy.service",
		"megavpn-backhaul-link.service",
	}
	for _, unit := range blocked {
		if isEmergencyManagedUnitForScope(unit, "services_only") {
			t.Fatalf("expected %s to be preserved for services_only", unit)
		}
	}
}

func TestEmergencyCleanupManagedFileAllowlist(t *testing.T) {
	allowed := []string{
		"/etc/nginx/conf.d/megavpn-edge.conf",
		"/etc/systemd/system/megavpn-xray-edge.service",
	}
	for _, path := range allowed {
		if !isEmergencyManagedFilePath(path) {
			t.Fatalf("expected %s to be allowed", path)
		}
	}
	blocked := []string{
		"/etc/nginx/conf.d/default.conf",
		"/etc/wireguard/wg0.conf",
		"/etc/systemd/system/ssh.service",
		"/etc/systemd/system/megavpn-../ssh.service",
	}
	for _, path := range blocked {
		if isEmergencyManagedFilePath(path) {
			t.Fatalf("expected %s to be blocked", path)
		}
	}
}

func TestEmergencyCleanupServicesOnlyFileAllowlistPreservesBackhaulAndRoutePolicy(t *testing.T) {
	allowed := []string{
		"/etc/nginx/conf.d/megavpn-edge.conf",
		"/etc/systemd/system/megavpn-xray-edge.service",
	}
	for _, path := range allowed {
		if !isEmergencyManagedFilePathForScope(path, "services_only") {
			t.Fatalf("expected %s to be allowed for services_only", path)
		}
	}
	blocked := []string{
		"/etc/systemd/system/megavpn-route-policy.service",
		"/etc/systemd/system/megavpn-backhaul-link.service",
	}
	for _, path := range blocked {
		if isEmergencyManagedFilePathForScope(path, "services_only") {
			t.Fatalf("expected %s to be preserved for services_only", path)
		}
	}
}

func TestEmergencyCleanupManagedUnitPathUsesScope(t *testing.T) {
	if got := emergencyManagedUnitFilePathForScope("megavpn-xray-edge.service", "services_only"); got != "/etc/systemd/system/megavpn-xray-edge.service" {
		t.Fatalf("services_only service unit path = %q", got)
	}
	for _, unit := range []string{"megavpn-route-policy.service", "megavpn-backhaul-link.service"} {
		if got := emergencyManagedUnitFilePathForScope(unit, "services_only"); got != "" {
			t.Fatalf("services_only must preserve %s, got path %q", unit, got)
		}
	}
	if got := emergencyManagedUnitFilePathForScope("megavpn-backhaul-link.service", "full_node"); got != "/etc/systemd/system/megavpn-backhaul-link.service" {
		t.Fatalf("full_node backhaul unit path = %q", got)
	}
}

func TestEmergencyCleanupPayloadRejectsStrictContractViolations(t *testing.T) {
	cases := []struct {
		name string
		edit func(map[string]any)
		want string
	}{
		{name: "missing schema", edit: func(p map[string]any) { delete(p, "schema") }, want: "schema"},
		{name: "unsupported schema", edit: func(p map[string]any) { p["schema"] = "node.emergency_cleanup.v2" }, want: "schema"},
		{name: "missing plan version", edit: func(p map[string]any) { delete(p, "cleanup_plan_version") }, want: "version"},
		{name: "unsupported plan version", edit: func(p map[string]any) { p["cleanup_plan_version"] = 2 }, want: "unsupported"},
		{name: "legacy scope alias", edit: func(p map[string]any) { p["cleanup_scope"] = "wipe" }, want: "scope"},
		{name: "wrong node", edit: func(p map[string]any) { p["node_id"] = "other-node" }, want: "node"},
		{name: "wrong confirmation", edit: func(p map[string]any) { p["confirmation"] = "wrong" }, want: "confirmation"},
		{name: "duplicate instance", edit: func(p map[string]any) {
			p["instances"] = []any{validEmergencyCleanupTargetForTest(), validEmergencyCleanupTargetForTest()}
		}, want: "duplicate"},
		{name: "unsupported action", edit: func(p map[string]any) {
			t := validEmergencyCleanupTargetForTest()
			t["action"] = "remove"
			p["instances"] = []any{t}
		}, want: "action"},
		{name: "unsupported service", edit: func(p map[string]any) {
			t := validEmergencyCleanupTargetForTest()
			t["service_code"] = "unsupported"
			p["instances"] = []any{t}
		}, want: "service"},
		{name: "unknown target field", edit: func(p map[string]any) {
			t := validEmergencyCleanupTargetForTest()
			t["secret_ref_id"] = "secret-1"
			p["instances"] = []any{t}
		}, want: "unknown"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			payload := validEmergencyCleanupPayloadForTest()
			tc.edit(payload)
			_, err := decodeEmergencyCleanupJobPayload(payload, agentState{NodeID: "node-1", NodeName: "edge-01"})
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want marker %q", err.Error(), tc.want)
			}
		})
	}
}

func TestEmergencyCleanupFilesystemPolicyRejectsTraversalAndSymlinks(t *testing.T) {
	root := t.TempDir()
	exec := testEmergencyCleanupExecutor(root, nil)
	if code, _ := exec.removeLogicalManagedPath("/etc/nginx/conf.d/../default.conf", "full_node", false, false, &emergencyCleanupSummary{}); code != "cleanup_path_unsafe" {
		t.Fatalf("traversal code = %q", code)
	}
	if code, _ := exec.removeLogicalManagedPath("/etc/nginx/conf.d-other/megavpn-edge.conf", "full_node", false, false, &emergencyCleanupSummary{}); code != "cleanup_path_forbidden" {
		t.Fatalf("root prefix confusion code = %q", code)
	}

	mustWriteFile(t, root, "/outside.txt", "sentinel")
	if err := os.MkdirAll(filepath.Join(root, "etc/nginx"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "outside.txt"), filepath.Join(root, "etc/nginx/conf.d")); err != nil {
		t.Fatal(err)
	}
	code, _ := exec.removeLogicalManagedPath("/etc/nginx/conf.d/megavpn-edge.conf", "full_node", false, false, &emergencyCleanupSummary{})
	if code != "cleanup_path_unsafe" {
		t.Fatalf("symlink parent code = %q", code)
	}
	body, err := os.ReadFile(filepath.Join(root, "outside.txt"))
	if err != nil || string(body) != "sentinel" {
		t.Fatalf("outside sentinel changed: body=%q err=%v", body, err)
	}
}

func TestEmergencyCleanupSafeRecursiveRemovalRejectsSymlinkChild(t *testing.T) {
	root := t.TempDir()
	exec := testEmergencyCleanupExecutor(root, nil)
	mustWriteFile(t, root, "/outside/sentinel.txt", "keep")
	mustWriteFile(t, root, "/etc/megavpn/certs/owned.pem", "cert")
	if err := os.Symlink(filepath.Join(root, "outside"), filepath.Join(root, "etc/megavpn/certs/link")); err != nil {
		t.Fatal(err)
	}
	code, _ := exec.removeLogicalManagedPath("/etc/megavpn/certs", "services_only", true, false, &emergencyCleanupSummary{})
	if code != "cleanup_remove_failed" {
		t.Fatalf("recursive symlink child code = %q", code)
	}
	assertExists(t, root, "/outside/sentinel.txt")
}

func TestEmergencyCleanupSystemdUnitValidationRejectsUnsafeTokens(t *testing.T) {
	blocked := []string{
		"-megavpn-xray-edge.service",
		"megavpn-xray/edge.service",
		`megavpn-xray\edge.service`,
		"megavpn-xray edge.service",
		"megavpn-xray-edge.service;reboot",
		"megavpn-xray-edge.timer",
		"ssh.service",
	}
	for _, unit := range blocked {
		if isEmergencyCleanupRunnableUnitForScope(unit, "full_node") {
			t.Fatalf("unsafe unit accepted: %q", unit)
		}
	}
	if !isEmergencyCleanupRunnableUnitForScope("megavpn-xray-edge.service", "services_only") {
		t.Fatal("expected managed service unit to be runnable")
	}
}

func TestEmergencyCleanupPreservesUnrelatedConfigsAndFullNodeOnlyState(t *testing.T) {
	root := t.TempDir()
	exec := testEmergencyCleanupExecutor(root, nil)
	mustWriteFile(t, root, "/etc/openvpn/server/unrelated.conf", "port 1194\n")
	mustWriteFile(t, root, "/etc/openvpn/server/managed.conf", emergencyManagedRuntimeConfigHeader+"\n")
	mustWriteFile(t, root, "/etc/wireguard/wg0.conf", "[Interface]\nAddress=10.0.0.1/24\n")
	mustWriteFile(t, root, "/etc/wireguard/wg-managed.conf", emergencyManagedRuntimeConfigHeader+"\n")
	mustWriteFile(t, root, "/etc/nginx/conf.d/default.conf", "server {}\n")
	mustWriteFile(t, root, "/etc/systemd/system/ssh.service", "[Service]\n")
	mustWriteFile(t, root, "/etc/megavpn/backhaul/link.json", "{}")
	mustWriteFile(t, root, "/etc/systemd/system/megavpn-route-policy.service", "[Service]\n")

	summary := &emergencyCleanupSummary{}
	changed, failures, _ := exec.cleanupManagedLeftovers(context.Background(), "services_only", map[string]struct{}{}, map[string]struct{}{}, summary)
	if failures != 0 {
		t.Fatalf("cleanup failures = %d", failures)
	}
	if changed {
		t.Fatal("services_only must not remove route-policy unit")
	}
	assertExists(t, root, "/etc/openvpn/server/unrelated.conf")
	assertAbsent(t, root, "/etc/openvpn/server/managed.conf")
	assertExists(t, root, "/etc/wireguard/wg0.conf")
	assertAbsent(t, root, "/etc/wireguard/wg-managed.conf")
	assertExists(t, root, "/etc/nginx/conf.d/default.conf")
	assertExists(t, root, "/etc/systemd/system/ssh.service")
	assertExists(t, root, "/etc/megavpn/backhaul/link.json")
	assertExists(t, root, "/etc/systemd/system/megavpn-route-policy.service")
}

func TestEmergencyCleanupNginxRollbackAndNeverStopsSharedNginx(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, root, "/etc/nginx/conf.d/megavpn-edge.conf", "managed")
	commands := []string{}
	nginxTests := 0
	run := func(_ context.Context, name string, args ...string) (int, string) {
		commands = append(commands, name+" "+strings.Join(args, " "))
		if name == "nginx" {
			nginxTests++
			if nginxTests > 1 {
				return 0, ""
			}
			return 1, "raw nginx output should not surface"
		}
		return 0, ""
	}
	exec := testEmergencyCleanupExecutor(root, run)
	nginx := exec.cleanupNginxManagedConfigs(context.Background(), map[string]struct{}{"/etc/nginx/conf.d/megavpn-edge.conf": {}}, &emergencyCleanupSummary{})
	if nginx.ErrorCode != "nginx_validation_failed" {
		t.Fatalf("nginx error code = %q", nginx.ErrorCode)
	}
	assertExists(t, root, "/etc/nginx/conf.d/megavpn-edge.conf")
	for _, cmd := range commands {
		if strings.Contains(cmd, "disable") || strings.Contains(cmd, "stop") {
			t.Fatalf("nginx cleanup must not stop/disable shared nginx, command=%q", cmd)
		}
	}
}

func TestEmergencyCleanupNginxReloadsOnlyWhenActive(t *testing.T) {
	for _, tc := range []struct {
		name       string
		state      string
		wantReload bool
	}{
		{name: "active reloads", state: "active", wantReload: true},
		{name: "inactive unchanged", state: "inactive", wantReload: false},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			mustWriteFile(t, root, "/etc/nginx/conf.d/megavpn-edge.conf", "managed")
			reloaded := false
			exec := testEmergencyCleanupExecutor(root, func(_ context.Context, name string, args ...string) (int, string) {
				if name == "systemctl" && len(args) > 0 && args[0] == "reload" {
					reloaded = true
				}
				return 0, ""
			})
			exec.unitState = func(string) string { return tc.state }
			nginx := exec.cleanupNginxManagedConfigs(context.Background(), map[string]struct{}{"/etc/nginx/conf.d/megavpn-edge.conf": {}}, &emergencyCleanupSummary{})
			if nginx.ErrorCode != "" {
				t.Fatalf("nginx error = %q", nginx.ErrorCode)
			}
			if reloaded != tc.wantReload {
				t.Fatalf("reloaded=%v want=%v", reloaded, tc.wantReload)
			}
		})
	}
}

func TestEmergencyCleanupResultRedactedAndPartialFailureBlocksSelfRemoval(t *testing.T) {
	payload := mustDecodeEmergencyPayloadForTest(t)
	payload.IncludeAgent = true
	exec := testEmergencyCleanupExecutor(t.TempDir(), func(_ context.Context, name string, args ...string) (int, string) {
		if name == "systemctl" && len(args) > 0 && args[0] == "disable" {
			return 1, "secret raw output"
		}
		return 0, ""
	})
	outcome := exec.Execute(context.Background(), "job-1", payload)
	if outcome.Status != "failed" {
		t.Fatalf("status = %q, want failed", outcome.Status)
	}
	if outcome.AfterSubmit != nil {
		t.Fatal("partial failure must not produce post-ack action")
	}
	encoded := stringifyJSONForTest(outcome.Result)
	for _, forbidden := range []string{"secret raw output", "private_key", "token", "signature", "/etc/"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("result leaked %q: %s", forbidden, encoded)
		}
	}
}

func TestAgentSelfRemovalPostAckMarkerAndScheduling(t *testing.T) {
	root := t.TempDir()
	pendingDir := filepath.Join(root, "pending")
	runDir := filepath.Join(root, "run")
	calls := []string{}
	manager := agentSelfRemovalManager{
		pendingDir: pendingDir,
		runDir:     runDir,
		run: func(_ context.Context, name string, args ...string) (int, string) {
			calls = append(calls, name+" "+strings.Join(args, " "))
			return 0, ""
		},
		clock: func() time.Time { return time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC) },
	}
	err := manager.activateAfterAck(context.Background(), "job/operator text")
	if !strings.Contains(err.Error(), "self-removal scheduled") {
		t.Fatalf("activate err = %v", err)
	}
	if len(calls) != 1 || !strings.Contains(calls[0], "systemd-run") {
		t.Fatalf("calls = %#v", calls)
	}
	if strings.Contains(calls[0], "operator text") || strings.Contains(calls[0], "megavpn-agent-self-remove --on-active") {
		t.Fatalf("unsafe unit/call = %q", calls[0])
	}
	if _, err := os.Stat(manager.markerPath("job/operator text")); !os.IsNotExist(err) {
		t.Fatalf("pending marker should be removed after successful scheduling, err=%v", err)
	}
}

func TestAgentSelfRemovalSchedulingFailureCreatesNonSecretPendingMarker(t *testing.T) {
	root := t.TempDir()
	manager := agentSelfRemovalManager{
		pendingDir: filepath.Join(root, "pending"),
		runDir:     filepath.Join(root, "run"),
		run: func(context.Context, string, ...string) (int, string) {
			return 1, "raw systemd output"
		},
		clock: func() time.Time { return time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC) },
	}
	err := manager.activateAfterAck(context.Background(), "job-1")
	if err == nil || !strings.Contains(err.Error(), "schedule_failed") {
		t.Fatalf("activate err = %v", err)
	}
	body, err := os.ReadFile(manager.markerPath("job-1"))
	if err != nil {
		t.Fatalf("read marker: %v", err)
	}
	if agentSelfRemovalMarkerContainsSecret(body) {
		t.Fatalf("marker contains forbidden secret terms: %s", body)
	}
	if strings.Contains(string(body), "raw systemd output") {
		t.Fatalf("marker leaked command output: %s", body)
	}
}

func TestEmergencyCleanupControlLoopActivatesSelfRemovalOnlyAfterSignedSubmitAck(t *testing.T) {
	order := []string{}
	c, restore := setupEmergencyCleanupControlLoopTest(t, func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/agent/heartbeat":
			return signedAgentTestResponse(req, "agent-token", http.StatusOK, []byte(`{"status":"ok"}`)), nil
		case "/agent/runtime/instances":
			return signedAgentTestResponse(req, "agent-token", http.StatusOK, []byte(`{"targets":[]}`)), nil
		case "/agent/jobs/next":
			body, _ := json.Marshal(job{ID: "job-1", Type: "node.emergency_cleanup", Payload: validEmergencyCleanupPayloadWithoutTargetsForTest(true)})
			return signedAgentTestResponse(req, "agent-token", http.StatusOK, body), nil
		case "/agent/jobs/job-1/result":
			order = append(order, "submit")
			return signedAgentTestResponse(req, "agent-token", http.StatusOK, []byte(`{"status":"accepted"}`)), nil
		default:
			t.Fatalf("unexpected path %s", req.URL.Path)
			return nil, nil
		}
	}, func(context.Context, string, ...string) (int, string) {
		order = append(order, "schedule")
		return 0, ""
	})
	defer restore()

	err := syncWithCore(context.Background(), testAgentLogger{}, c, &agentState{NodeID: "node-1", NodeName: "edge-01"})
	if !errors.Is(err, ErrAgentSelfRemovalScheduled) {
		t.Fatalf("sync err = %v, want self-removal sentinel", err)
	}
	if strings.Join(order, ",") != "submit,schedule" {
		t.Fatalf("order = %#v, want submit before schedule", order)
	}
}

func TestEmergencyCleanupControlLoopSubmitFailureDoesNotActivateSelfRemoval(t *testing.T) {
	scheduled := false
	c, restore := setupEmergencyCleanupControlLoopTest(t, func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/agent/heartbeat":
			return signedAgentTestResponse(req, "agent-token", http.StatusOK, []byte(`{"status":"ok"}`)), nil
		case "/agent/runtime/instances":
			return signedAgentTestResponse(req, "agent-token", http.StatusOK, []byte(`{"targets":[]}`)), nil
		case "/agent/jobs/next":
			body, _ := json.Marshal(job{ID: "job-1", Type: "node.emergency_cleanup", Payload: validEmergencyCleanupPayloadWithoutTargetsForTest(true)})
			return signedAgentTestResponse(req, "agent-token", http.StatusOK, body), nil
		case "/agent/jobs/job-1/result":
			return signedAgentTestResponse(req, "agent-token", http.StatusInternalServerError, []byte(`{"code":"failed","error":"submit failed"}`)), nil
		default:
			t.Fatalf("unexpected path %s", req.URL.Path)
			return nil, nil
		}
	}, func(context.Context, string, ...string) (int, string) {
		scheduled = true
		return 0, ""
	})
	defer restore()

	err := syncWithCore(context.Background(), testAgentLogger{}, c, &agentState{NodeID: "node-1", NodeName: "edge-01"})
	if err == nil || !strings.Contains(err.Error(), "submit job result failed") {
		t.Fatalf("sync err = %v, want submit failure", err)
	}
	if scheduled {
		t.Fatal("self-removal scheduled after failed submit")
	}
}

func TestEmergencyCleanupControlLoopUnsignedSubmitAckDoesNotActivateSelfRemoval(t *testing.T) {
	scheduled := false
	c, restore := setupEmergencyCleanupControlLoopTest(t, func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/agent/heartbeat":
			return signedAgentTestResponse(req, "agent-token", http.StatusOK, []byte(`{"status":"ok"}`)), nil
		case "/agent/runtime/instances":
			return signedAgentTestResponse(req, "agent-token", http.StatusOK, []byte(`{"targets":[]}`)), nil
		case "/agent/jobs/next":
			body, _ := json.Marshal(job{ID: "job-1", Type: "node.emergency_cleanup", Payload: validEmergencyCleanupPayloadWithoutTargetsForTest(true)})
			return signedAgentTestResponse(req, "agent-token", http.StatusOK, body), nil
		case "/agent/jobs/job-1/result":
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"status":"accepted"}`))}, nil
		default:
			t.Fatalf("unexpected path %s", req.URL.Path)
			return nil, nil
		}
	}, func(context.Context, string, ...string) (int, string) {
		scheduled = true
		return 0, ""
	})
	defer restore()

	err := syncWithCore(context.Background(), testAgentLogger{}, c, &agentState{NodeID: "node-1", NodeName: "edge-01"})
	if err == nil || !strings.Contains(err.Error(), "unsigned agent response rejected") {
		t.Fatalf("sync err = %v, want unsigned ack rejection", err)
	}
	if scheduled {
		t.Fatal("self-removal scheduled after unsigned submit ack")
	}
}

func TestEmergencyCleanupPendingSelfRemovalProcessedBeforePolling(t *testing.T) {
	root := t.TempDir()
	oldManager := defaultAgentSelfRemovalManager
	scheduled := false
	defaultAgentSelfRemovalManager = agentSelfRemovalManager{
		pendingDir: filepath.Join(root, "pending"),
		runDir:     filepath.Join(root, "run"),
		run: func(context.Context, string, ...string) (int, string) {
			scheduled = true
			return 0, ""
		},
		clock: func() time.Time { return time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC) },
	}
	t.Cleanup(func() { defaultAgentSelfRemovalManager = oldManager })
	if _, err := defaultAgentSelfRemovalManager.writePendingMarker("job-1"); err != nil {
		t.Fatalf("write pending marker: %v", err)
	}
	c := newClient("https://control.example", "agent-token", "")
	c.http = httpDoerFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("HTTP must not be reached while post-ack self-removal is pending: %s", req.URL.Path)
		return nil, nil
	})
	err := syncWithCore(context.Background(), testAgentLogger{}, c, &agentState{NodeID: "node-1", NodeName: "edge-01"})
	if !errors.Is(err, ErrAgentSelfRemovalScheduled) {
		t.Fatalf("sync err = %v, want self-removal sentinel", err)
	}
	if !scheduled {
		t.Fatal("pending self-removal was not scheduled")
	}
}

func validEmergencyCleanupPayloadForTest() map[string]any {
	return map[string]any{
		"schema":               emergencyCleanupSchemaVersion,
		"node_id":              "node-1",
		"node_name":            "edge-01",
		"confirmation":         "edge-01",
		"reason":               "maintenance window",
		"requested_at":         "2026-07-15T12:00:00Z",
		"cleanup_scope":        "full_node",
		"include_agent":        true,
		"cleanup_plan_version": 1,
		"instances":            []any{validEmergencyCleanupTargetForTest()},
	}
}

func validEmergencyCleanupPayloadWithoutTargetsForTest(includeAgent bool) map[string]any {
	payload := validEmergencyCleanupPayloadForTest()
	payload["include_agent"] = includeAgent
	payload["instances"] = []any{}
	return payload
}

func validEmergencyCleanupTargetForTest() map[string]any {
	return map[string]any{
		"instance_id":          "inst-1",
		"action":               "delete",
		"service_code":         "xray-core",
		"runtime_service_code": "xray-core",
		"name":                 "Edge Xray",
		"slug":                 "edge-xray",
		"systemd_unit":         "megavpn-xray-edge-xray.service",
		"endpoint_host":        "127.0.0.1",
		"endpoint_port":        443,
		"enabled":              false,
	}
}

func mustDecodeEmergencyPayloadForTest(t *testing.T) emergencyCleanupJobPayload {
	t.Helper()
	payload, err := decodeEmergencyCleanupJobPayload(validEmergencyCleanupPayloadForTest(), agentState{NodeID: "node-1", NodeName: "edge-01"})
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	return payload
}

func testEmergencyCleanupExecutor(root string, run func(context.Context, string, ...string) (int, string)) emergencyCleanupExecutor {
	if run == nil {
		run = func(context.Context, string, ...string) (int, string) { return 0, "" }
	}
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	return emergencyCleanupExecutor{
		root:       root,
		run:        run,
		unitState:  func(string) string { return "active" },
		unitOutput: func(string, ...string) string { return "not-found" },
		clock:      func() time.Time { return time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC) },
	}
}

func mustWriteFile(t *testing.T, root, logicalPath, body string) {
	t.Helper()
	path := filepath.Join(root, strings.TrimPrefix(logicalPath, "/"))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertExists(t *testing.T, root, logicalPath string) {
	t.Helper()
	if _, err := os.Lstat(filepath.Join(root, strings.TrimPrefix(logicalPath, "/"))); err != nil {
		t.Fatalf("expected %s to exist: %v", logicalPath, err)
	}
}

func assertAbsent(t *testing.T, root, logicalPath string) {
	t.Helper()
	if _, err := os.Lstat(filepath.Join(root, strings.TrimPrefix(logicalPath, "/"))); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be absent, err=%v", logicalPath, err)
	}
}

func stringifyJSONForTest(value any) string {
	b, _ := json.Marshal(value)
	return string(b)
}

type testAgentLogger struct{}

func (testAgentLogger) Info(string, ...any)  {}
func (testAgentLogger) Error(string, ...any) {}

func setupEmergencyCleanupControlLoopTest(t *testing.T, httpFn func(*http.Request) (*http.Response, error), scheduleFn func(context.Context, string, ...string) (int, string)) (*client, func()) {
	t.Helper()
	root := t.TempDir()
	oldFactory := newEmergencyCleanupExecutor
	oldManager := defaultAgentSelfRemovalManager
	newEmergencyCleanupExecutor = func() emergencyCleanupExecutor {
		return testEmergencyCleanupExecutor(root, func(context.Context, string, ...string) (int, string) { return 0, "" })
	}
	defaultAgentSelfRemovalManager = agentSelfRemovalManager{
		pendingDir: filepath.Join(root, "pending"),
		runDir:     filepath.Join(root, "run"),
		run:        scheduleFn,
		clock:      func() time.Time { return time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC) },
	}
	c := newClient("https://control.example", "agent-token", "")
	c.http = httpDoerFunc(httpFn)
	c.trafficReportInterval = time.Nanosecond
	return c, func() {
		newEmergencyCleanupExecutor = oldFactory
		defaultAgentSelfRemovalManager = oldManager
	}
}
