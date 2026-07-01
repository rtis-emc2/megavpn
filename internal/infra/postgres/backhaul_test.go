package postgres

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/rtis-emc2/megavpn/internal/backhaul"
	"github.com/rtis-emc2/megavpn/internal/domain"
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

func TestBackhaulEndpointPortIsStableAndDriverScoped(t *testing.T) {
	t.Parallel()

	first := backhaulEndpointPort("link-1", backhaul.DriverWireGuard, 51830, 0)
	second := backhaulEndpointPort("link-2", backhaul.DriverWireGuard, 51830, 0)
	if first < 51830 || first >= 52830 {
		t.Fatalf("wireguard endpoint port = %d, want 51830..52829", first)
	}
	if first != backhaulEndpointPort("link-1", backhaul.DriverWireGuard, 51830, 0) {
		t.Fatal("wireguard endpoint port must be stable")
	}
	if first == second {
		t.Fatalf("wireguard endpoint ports should vary by link, both got %d", first)
	}
	if got := backhaulEndpointPort("link-1", backhaul.DriverOpenVPNUDP, 1194, 0); got != 1194 {
		t.Fatalf("openvpn endpoint port = %d, want default 1194", got)
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

func TestNormalizeBackhaulApplyHealthPreservesFailedRoleReason(t *testing.T) {
	t.Parallel()

	checkedAt := time.Date(2026, 6, 19, 12, 30, 0, 0, time.UTC)
	health, lastError := normalizeBackhaulApplyHealth("failed", map[string]any{
		"error": "backhaul systemd activation failed",
		"stage": "systemd_enable",
		"health": map[string]any{
			"status":       "unhealthy",
			"reason":       "systemd unit is not active",
			"active_state": "failed",
		},
	}, checkedAt)

	if got := stringify(health["status"]); got != "unhealthy" {
		t.Fatalf("status = %q, want unhealthy", got)
	}
	if got := stringify(health["reason"]); got != "systemd unit is not active" {
		t.Fatalf("reason = %q, want systemd unit is not active", got)
	}
	if got := stringify(health["error"]); got != "systemd unit is not active" {
		t.Fatalf("error = %q, want systemd unit is not active", got)
	}
	if got := stringify(health["stage"]); got != "systemd_enable" {
		t.Fatalf("stage = %q, want systemd_enable", got)
	}
	if got := stringify(health["job_status"]); got != "failed" {
		t.Fatalf("job_status = %q, want failed", got)
	}
	if got := stringify(health["checked_at"]); got != checkedAt.Format(time.RFC3339) {
		t.Fatalf("checked_at = %q, want %q", got, checkedAt.Format(time.RFC3339))
	}
	if lastError != "systemd unit is not active" {
		t.Fatalf("lastError = %q, want systemd unit is not active", lastError)
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

func TestRenderWireGuardBackhaulConfigUsesPointToPointPrefix(t *testing.T) {
	t.Parallel()

	transport := domain.BackhaulTransport{
		Driver:         backhaul.DriverWireGuard,
		EndpointHost:   "203.0.113.10",
		EndpointPort:   51830,
		TunnelCIDR:     "10.240.86.112/30",
		IngressAddress: "10.240.86.113",
		EgressAddress:  "10.240.86.114",
	}

	ingressConfig := renderWireGuardBackhaulConfig(transport, "ingress", "ingress-private", "egress-public")
	for _, want := range []string{
		"Address = 10.240.86.113/30",
		"Endpoint = 203.0.113.10:51830",
		"# Peer tunnel address: 10.240.86.114",
		"AllowedIPs = 0.0.0.0/0, ::/0",
		"Table = off",
	} {
		if !strings.Contains(ingressConfig, want) {
			t.Fatalf("ingress config missing %q:\n%s", want, ingressConfig)
		}
	}

	egressConfig := renderWireGuardBackhaulConfig(transport, "egress", "egress-private", "ingress-public")
	for _, want := range []string{
		"Address = 10.240.86.114/30",
		"ListenPort = 51830",
		"# Peer tunnel address: 10.240.86.113",
	} {
		if !strings.Contains(egressConfig, want) {
			t.Fatalf("egress config missing %q:\n%s", want, egressConfig)
		}
	}
	if strings.Contains(egressConfig, "Endpoint =") {
		t.Fatalf("egress config must not contain Endpoint:\n%s", egressConfig)
	}
}

func TestBackhaulSystemdUnitIsRoleSpecificAndBounded(t *testing.T) {
	t.Parallel()

	link := domain.BackhaulLink{
		Name: "backhaul-" + strings.Repeat("very-long-name-", 20),
	}
	transport := domain.BackhaulTransport{
		ID:     "transport-1234567890",
		Driver: backhaul.DriverWireGuard,
	}

	ingressUnit := backhaulSystemdUnit(link, transport, "ingress")
	egressUnit := backhaulSystemdUnit(link, transport, "egress")
	if ingressUnit == egressUnit {
		t.Fatalf("ingress and egress units must differ: %q", ingressUnit)
	}
	for _, unit := range []string{ingressUnit, egressUnit} {
		if !strings.HasPrefix(unit, "megavpn-backhaul-") || !strings.HasSuffix(unit, ".service") {
			t.Fatalf("unexpected unit name %q", unit)
		}
		if len(unit) > 128 {
			t.Fatalf("unit %q length = %d, want <= 128", unit, len(unit))
		}
	}
}

func TestBackhaulCleanupUnitsIncludeLegacyCommonUnit(t *testing.T) {
	t.Parallel()

	link := domain.BackhaulLink{Name: "edge-backhaul"}
	transport := domain.BackhaulTransport{ID: "transport-abcdef12", Driver: backhaul.DriverOpenVPNUDP}
	units := backhaulSystemdUnits(link, transport, "ingress")
	if len(units) != 2 {
		t.Fatalf("units = %#v, want current and legacy", units)
	}
	if !strings.Contains(units[0], "-ingress.service") {
		t.Fatalf("first unit should be current ingress unit: %#v", units)
	}
	if strings.Contains(units[1], "-ingress.service") || strings.Contains(units[1], "-egress.service") {
		t.Fatalf("second unit should be legacy common unit: %#v", units)
	}
	paths := backhaulCleanupPaths(link, transport, "ingress")
	if len(paths) != 2 {
		t.Fatalf("cleanup paths = %#v, want current and legacy unit paths", paths)
	}
}

func TestRenderBackhaulIngressStartScriptAddsOutboundSNAT(t *testing.T) {
	t.Parallel()

	script := renderBackhaulIngressStartScript("link-wireguard-abcd", "mgbh1234567890", "/usr/bin/wg-quick up '/etc/megavpn/backhaul/link/wg.conf'", "10.240.35.245", "21001", 70)
	for _, want := range []string{
		"sysctl -w net.ipv4.ip_forward=1",
		"megavpn:backhaul:link-wireguard-abcd:ingress-snat",
		"nft add rule ip megavpn_backhaul postrouting oifname 'mgbh1234567890' masquerade comment '\"megavpn:backhaul:link-wireguard-abcd:ingress-snat\"'",
		"ip route replace default dev 'mgbh1234567890' table '21001' metric 70",
		"ip rule add from '10.240.35.245/32' table '21001' priority 21950",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("ingress start script missing %q:\n%s", want, script)
		}
	}
}

func TestRenderBackhaulEgressStartScriptKeepsInboundMasquerade(t *testing.T) {
	t.Parallel()

	script := renderBackhaulEgressStartScript("link-wireguard-abcd", "mgbh1234567890", "/usr/bin/wg-quick up '/etc/megavpn/backhaul/link/wg.conf'")
	for _, want := range []string{
		"megavpn:backhaul:link-wireguard-abcd:egress-masquerade",
		"nft add rule ip megavpn_backhaul postrouting iifname 'mgbh1234567890' masquerade comment '\"megavpn:backhaul:link-wireguard-abcd:egress-masquerade\"'",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("egress start script missing %q:\n%s", want, script)
		}
	}
}

func TestRenderBackhaulStopScriptRemovesSelectedNATComments(t *testing.T) {
	t.Parallel()

	script := renderBackhaulIngressStopScript("link-wireguard-abcd", "mgbh1234567890", "/usr/bin/wg-quick down '/etc/megavpn/backhaul/link/wg.conf'", "10.240.35.245", "21001")
	if !strings.Contains(script, "megavpn:backhaul:link-wireguard-abcd:ingress-snat") {
		t.Fatalf("stop script missing ingress-snat cleanup:\n%s", script)
	}
	if !strings.Contains(script, "ip rule delete from '10.240.35.245/32' table '21001'") {
		t.Fatalf("stop script missing source policy cleanup:\n%s", script)
	}
	if strings.Contains(script, "egress-masquerade") {
		t.Fatalf("ingress stop script should not remove egress NAT comment:\n%s", script)
	}
}
