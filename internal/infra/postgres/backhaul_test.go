package postgres

import (
	"reflect"
	"testing"
	"time"

	"github.com/rtis-emc2/megavpn/internal/backhaul"
)

func TestBackhaulDriversFromMetadataExplicitListDoesNotExpandFallbacks(t *testing.T) {
	t.Parallel()

	drivers := backhaulDriversFromMetadata(map[string]any{
		"drivers": []any{backhaul.DriverWireGuard},
	}, backhaul.DriverWireGuard)

	want := []string{backhaul.DriverWireGuard}
	if !reflect.DeepEqual(drivers, want) {
		t.Fatalf("drivers = %#v, want %#v", drivers, want)
	}
}

func TestBackhaulDriversFromMetadataAddsDesiredDriver(t *testing.T) {
	t.Parallel()

	drivers := backhaulDriversFromMetadata(map[string]any{
		"drivers": []any{backhaul.DriverOpenVPNUDP},
	}, backhaul.DriverWireGuard)

	want := []string{backhaul.DriverWireGuard, backhaul.DriverOpenVPNUDP}
	if !reflect.DeepEqual(drivers, want) {
		t.Fatalf("drivers = %#v, want %#v", drivers, want)
	}
}

func TestBackhaulApplyCompleteStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		activationMode string
		driver         string
		want           string
	}{
		{name: "managed mode", activationMode: "managed_systemd", driver: backhaul.DriverWireGuard, want: "active"},
		{name: "fallback to catalog", activationMode: "", driver: backhaul.DriverOpenVPNUDP, want: "active"},
		{name: "materialize only", activationMode: "materialize_only", driver: backhaul.DriverXrayVLESSWS, want: "materialized"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := backhaulApplyCompleteStatus(tt.activationMode, tt.driver); got != tt.want {
				t.Fatalf("status = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUniqueBackhaulTunnelCIDRDoesNotReuseSelectedCIDR(t *testing.T) {
	t.Parallel()

	used := map[string]bool{"10.240.10.0/30": true}
	got := uniqueBackhaulTunnelCIDR("link-1", "10.240.10.0/30", backhaul.DriverOpenVPNUDP, 1, used)
	if got == "10.240.10.0/30" {
		t.Fatalf("unique CIDR reused existing CIDR")
	}
	if err := backhaul.ValidateTunnelCIDR(got); err != nil {
		t.Fatalf("generated CIDR %q is invalid: %v", got, err)
	}
}

func TestNormalizeBackhaulProbeHealthPreservesDegradedReason(t *testing.T) {
	t.Parallel()

	checkedAt := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	health, lastError := normalizeBackhaulProbeHealth("failed", map[string]any{
		"health": map[string]any{
			"status":              "degraded",
			"reason":              "peer ping failed",
			"packet_loss_percent": 100.0,
			"peer":                "10.240.86.113",
		},
		"health_reason": "peer ping failed",
	}, checkedAt)

	if got := stringify(health["status"]); got != "degraded" {
		t.Fatalf("status = %q, want degraded", got)
	}
	if got := stringify(health["reason"]); got != "peer ping failed" {
		t.Fatalf("reason = %q, want peer ping failed", got)
	}
	if got := stringify(health["error"]); got != "peer ping failed" {
		t.Fatalf("error = %q, want peer ping failed", got)
	}
	if got := stringify(health["job_status"]); got != "failed" {
		t.Fatalf("job_status = %q, want failed", got)
	}
	if got := stringify(health["checked_at"]); got != checkedAt.Format(time.RFC3339) {
		t.Fatalf("checked_at = %q, want %q", got, checkedAt.Format(time.RFC3339))
	}
	if lastError != "peer ping failed" {
		t.Fatalf("lastError = %q, want peer ping failed", lastError)
	}
}

func TestNormalizeBackhaulProbeHealthClearsLastErrorOnHealthySuccess(t *testing.T) {
	t.Parallel()

	health, lastError := normalizeBackhaulProbeHealth("succeeded", map[string]any{
		"health": map[string]any{
			"status":         "healthy",
			"latency_avg_ms": 0.42,
		},
	}, time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC))

	if got := stringify(health["status"]); got != "healthy" {
		t.Fatalf("status = %q, want healthy", got)
	}
	if lastError != "" {
		t.Fatalf("lastError = %q, want empty", lastError)
	}
}
