package postgres

import (
	"reflect"
	"testing"

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
