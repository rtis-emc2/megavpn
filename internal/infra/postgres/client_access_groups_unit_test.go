package postgres

import (
	"strings"
	"testing"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

func TestNormalizeClientAccessGroupInputRejectsUnsupportedServices(t *testing.T) {
	_, err := normalizeClientAccessGroupInput(domain.ClientAccessGroupInput{
		ServiceCode: "openvpn",
		GroupKey:    "openvpn_clients",
		DisplayName: "OpenVPN clients",
		Status:      "active",
	}, true)
	if err == nil {
		t.Fatal("openvpn client access group create must be rejected until materialization is implemented")
	}
	if !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNormalizeClientAccessGroupInputAllowsVLESS(t *testing.T) {
	out, err := normalizeClientAccessGroupInput(domain.ClientAccessGroupInput{
		ServiceCode: "xray-core",
		GroupKey:    "out_usa_sf",
		DisplayName: "Outgoing USA San Francisco",
		Status:      "active",
	}, true)
	if err != nil {
		t.Fatalf("vless input rejected: %v", err)
	}
	if out.ServiceCode != "vless" {
		t.Fatalf("service = %q, want vless", out.ServiceCode)
	}
}
