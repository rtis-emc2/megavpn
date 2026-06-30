package postgres

import (
	"testing"

	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/service/driver"
)

func TestRuntimeStateDerivationForApplySuccess(t *testing.T) {
	t.Parallel()

	instance := domain.Instance{
		CurrentRevisionID:     stringPtr("rev-1"),
		LastAppliedRevisionID: stringPtr("rev-1"),
	}
	if got := runtimeStatusFromJob("instance.apply", "succeeded", "active"); got != "active" {
		t.Fatalf("runtime status = %q", got)
	}
	if got := healthStatusFromRuntime("instance.apply", "succeeded", "active"); got != "healthy" {
		t.Fatalf("health status = %q", got)
	}
	if got := driftStatusForInstance(instance, "instance.apply", "succeeded"); got != "in_sync" {
		t.Fatalf("drift status = %q", got)
	}
}

func TestRuntimeStateDerivationForFailure(t *testing.T) {
	t.Parallel()

	if got := runtimeStatusFromJob("instance.apply", "failed", "failed"); got != "failed" {
		t.Fatalf("runtime status = %q", got)
	}
	if got := healthStatusFromRuntime("instance.apply", "failed", "failed"); got != "unhealthy" {
		t.Fatalf("health status = %q", got)
	}
	if got := driftStatusForInstance(domain.Instance{CurrentRevisionID: stringPtr("rev-2")}, "instance.apply", "failed"); got != "unknown" {
		t.Fatalf("drift status = %q", got)
	}
}

func TestRuntimeStateDerivationForQueuedApply(t *testing.T) {
	t.Parallel()

	instance := domain.Instance{CurrentRevisionID: stringPtr("rev-2"), LastAppliedRevisionID: stringPtr("rev-1")}
	if got := runtimeStatusFromJob("instance.apply", "queued", ""); got != "provisioning" {
		t.Fatalf("runtime status = %q", got)
	}
	if got := healthStatusFromRuntime("instance.apply", "queued", ""); got != "provisioning" {
		t.Fatalf("health status = %q", got)
	}
	if got := driftStatusForInstance(instance, "instance.apply", "queued"); got != "pending_apply" {
		t.Fatalf("drift status = %q", got)
	}
}

func TestRuntimeProjectionExplainsQueuedJob(t *testing.T) {
	t.Parallel()

	_, healthReasons, driftReasons := buildRuntimeProjection(runtimeProjectionInput{
		ServiceCode:    driver.WireGuard,
		DesiredStatus:  "provisioning",
		RuntimeStatus:  "provisioning",
		HealthStatus:   "provisioning",
		DriftStatus:    "pending_apply",
		LastJobType:    "instance.apply",
		LastJobStatus:  "queued",
		EndpointPort:   51820,
		ListeningPorts: []map[string]any{},
	})
	if len(healthReasons) == 0 || healthReasons[0] != "instance.apply is queued; waiting for agent convergence." {
		t.Fatalf("health reasons = %#v", healthReasons)
	}
	if len(driftReasons) != 1 || driftReasons[0] != "instance.apply is queued; desired state is waiting for agent convergence." {
		t.Fatalf("drift reasons = %#v", driftReasons)
	}
}

func TestRuntimeStateDerivationForStoppedInstance(t *testing.T) {
	t.Parallel()

	if got := runtimeStatusFromJob("instance.stop", "succeeded", "inactive"); got != "stopped" {
		t.Fatalf("runtime status = %q", got)
	}
	if got := healthStatusFromRuntime("instance.stop", "succeeded", "inactive"); got != "stopped" {
		t.Fatalf("health status = %q", got)
	}
}

func TestAgentRuntimeReportDerivationHealthy(t *testing.T) {
	t.Parallel()

	instance := domain.Instance{
		ServiceCode:           driver.WireGuard,
		Status:                "enabled",
		Enabled:               true,
		EndpointPort:          51820,
		CurrentRevisionID:     stringPtr("rev-1"),
		LastAppliedRevisionID: stringPtr("rev-1"),
	}
	report := domain.AgentInstanceRuntimeReport{
		ServiceCode:        driver.WireGuard,
		ConfigHash:         "sha256:test",
		ActiveState:        "active",
		ObservedRevisionID: stringPtr("rev-1"),
		ListeningPorts:     []map[string]any{{"port": 51820}},
	}
	if got := runtimeStatusFromAgentReport(instance, report); got != "active" {
		t.Fatalf("runtime status = %q", got)
	}
	if got := healthStatusFromAgentReport(instance, report); got != "healthy" {
		t.Fatalf("health status = %q", got)
	}
	if got := driftStatusFromAgentReport(instance, report); got != "in_sync" {
		t.Fatalf("drift status = %q", got)
	}
}

func TestAgentRuntimeReportDerivationDrifted(t *testing.T) {
	t.Parallel()

	instance := domain.Instance{
		Status:                "disabled",
		Enabled:               false,
		CurrentRevisionID:     stringPtr("rev-2"),
		LastAppliedRevisionID: stringPtr("rev-2"),
	}
	report := domain.AgentInstanceRuntimeReport{
		ActiveState:        "active",
		ObservedRevisionID: stringPtr("rev-1"),
	}
	if got := healthStatusFromAgentReport(instance, report); got != "degraded" {
		t.Fatalf("health status = %q", got)
	}
	if got := driftStatusFromAgentReport(instance, report); got != "drifted" {
		t.Fatalf("drift status = %q", got)
	}
}

func TestAgentRuntimeReportDerivationPendingApply(t *testing.T) {
	t.Parallel()

	instance := domain.Instance{
		ServiceCode:           driver.Nginx,
		Status:                "enabled",
		Enabled:               true,
		EndpointPort:          443,
		CurrentRevisionID:     stringPtr("rev-2"),
		LastAppliedRevisionID: stringPtr("rev-1"),
	}
	report := domain.AgentInstanceRuntimeReport{
		ServiceCode:    driver.Nginx,
		ConfigHash:     "sha256:test",
		ActiveState:    "active",
		ListeningPorts: []map[string]any{{"port": 8443}},
	}
	if got := healthStatusFromAgentReport(instance, report); got != "degraded" {
		t.Fatalf("health status = %q", got)
	}
	if got := driftStatusFromAgentReport(instance, report); got != "pending_apply" {
		t.Fatalf("drift status = %q", got)
	}
}

func TestAgentRuntimeReportDerivationTransitioning(t *testing.T) {
	t.Parallel()

	instance := domain.Instance{
		ServiceCode:           driver.OpenVPN,
		Status:                "active",
		Enabled:               true,
		EndpointPort:          1194,
		CurrentRevisionID:     stringPtr("rev-1"),
		LastAppliedRevisionID: stringPtr("rev-1"),
	}
	report := domain.AgentInstanceRuntimeReport{
		ServiceCode:        driver.OpenVPN,
		ConfigHash:         "sha256:test",
		ActiveState:        "activating",
		ObservedRevisionID: stringPtr("rev-1"),
	}
	if got := runtimeStatusFromAgentReport(instance, report); got != "transitioning" {
		t.Fatalf("runtime status = %q", got)
	}
	if got := healthStatusFromAgentReport(instance, report); got != "provisioning" {
		t.Fatalf("health status = %q", got)
	}
	if got := driftStatusFromAgentReport(instance, report); got != "pending_apply" {
		t.Fatalf("drift status = %q", got)
	}
}

func TestAgentRuntimeReportDerivationOpenVPNOperationalWhileSystemdActivating(t *testing.T) {
	t.Parallel()

	instance := domain.Instance{
		ServiceCode:           driver.OpenVPN,
		Status:                "active",
		Enabled:               true,
		EndpointPort:          1194,
		CurrentRevisionID:     stringPtr("rev-1"),
		LastAppliedRevisionID: stringPtr("rev-1"),
	}
	report := domain.AgentInstanceRuntimeReport{
		ServiceCode:        driver.OpenVPN,
		ConfigHash:         "sha256:test",
		ActiveState:        "activating",
		ObservedRevisionID: stringPtr("rev-1"),
		ListeningPorts:     []map[string]any{{"port": 1194}},
	}
	if got := runtimeStatusFromAgentReport(instance, report); got != "active" {
		t.Fatalf("runtime status = %q", got)
	}
	if got := healthStatusFromAgentReport(instance, report); got != "healthy" {
		t.Fatalf("health status = %q", got)
	}
	if got := driftStatusFromAgentReport(instance, report); got != "in_sync" {
		t.Fatalf("drift status = %q", got)
	}
}

func TestRuntimeProjectionBuildsDriverHealthChecks(t *testing.T) {
	t.Parallel()

	checks, healthReasons, driftReasons := buildRuntimeProjection(runtimeProjectionInput{
		ServiceCode:    driver.WireGuard,
		DesiredStatus:  "enabled",
		RuntimeStatus:  "active",
		HealthStatus:   "healthy",
		DriftStatus:    "in_sync",
		ActiveState:    "active",
		ConfigHash:     "sha256:test",
		LastJobType:    "agent.runtime.report",
		EndpointPort:   51820,
		ListeningPorts: []map[string]any{{"port": 51820}},
	})
	if len(checks) != 3 {
		t.Fatalf("health checks len = %d, want 3: %#v", len(checks), checks)
	}
	for _, check := range checks {
		if check.Status != "ok" {
			t.Fatalf("health check %s status = %q, want ok: %#v", check.Code, check.Status, check)
		}
	}
	if len(healthReasons) != 1 || healthReasons[0] != "All required runtime checks passed." {
		t.Fatalf("health reasons = %#v, want all checks passed", healthReasons)
	}
	if len(driftReasons) != 1 || driftReasons[0] != "Applied revision and observed runtime state are in sync." {
		t.Fatalf("drift reasons = %#v, want in-sync reason", driftReasons)
	}
}

func stringPtr(value string) *string {
	return &value
}
