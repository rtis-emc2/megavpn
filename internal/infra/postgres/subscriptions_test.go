package postgres

import (
	"strings"
	"testing"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

func TestClientVLESSSubscriptionProfileBuildsURI(t *testing.T) {
	t.Parallel()

	record := domain.ProvisioningAccess{
		Access: domain.ServiceAccess{
			ID:     "access-1",
			Status: "active",
			Metadata: map[string]any{
				"xray_uuid":          "11111111-1111-4111-8111-111111111111",
				"reality_public_key": "public-key",
				"short_id":           "abcd",
				"outbound_group":     "egress-eu",
			},
		},
		Client: domain.Client{DisplayName: "Alice Smith"},
		Instance: domain.Instance{
			ID:           "instance-1",
			Name:         "edge-vless",
			Slug:         "edge-vless",
			ServiceCode:  "xray-core",
			Status:       "active",
			Enabled:      true,
			EndpointHost: "vpn.example.com",
			EndpointPort: 8443,
			Spec: map[string]any{
				"security":    "reality",
				"network":     "tcp",
				"fingerprint": "chrome",
			},
		},
	}

	profile, ok := clientVLESSSubscriptionProfile(record)
	if !ok {
		t.Fatal("expected active VLESS access to produce a subscription profile")
	}
	if profile.OutboundGroup != "egress-eu" {
		t.Fatalf("outbound group = %q, want egress-eu", profile.OutboundGroup)
	}
	for _, fragment := range []string{
		"vless://11111111-1111-4111-8111-111111111111@vpn.example.com:8443?",
		"security=reality",
		"type=tcp",
		"fp=chrome",
		"pbk=public-key",
		"sid=abcd",
		"#edge-vless-Alice%20Smith",
	} {
		if !strings.Contains(profile.URI, fragment) {
			t.Fatalf("subscription URI %q does not contain %q", profile.URI, fragment)
		}
	}
	if strings.Contains(profile.URI, "#edge-vless-Alice+Smith") {
		t.Fatalf("subscription URI label must use fragment escaping, got %q", profile.URI)
	}
}

func TestClientVLESSSubscriptionProfileUsesCamouflagePublicEndpoint(t *testing.T) {
	t.Parallel()

	record := domain.ProvisioningAccess{
		Access: domain.ServiceAccess{
			ID:     "access-1",
			Status: "active",
			Metadata: map[string]any{
				"xray_uuid": "22222222-2222-4222-8222-222222222222",
			},
		},
		Client: domain.Client{Username: "bob"},
		Instance: domain.Instance{
			ID:           "instance-1",
			Name:         "edge-xray-http-xray-ws",
			Slug:         "edge-xray-http-xray-ws",
			ServiceCode:  "xray-core",
			Status:       "active",
			Enabled:      true,
			EndpointHost: "enter.example.com",
			EndpointPort: 7080,
			Spec: map[string]any{
				"security":           "none",
				"network":            "ws",
				"path":               "/assets/rtis-sync",
				"public_security":    "tls",
				"public_network":     "ws",
				"public_path":        "/assets/rtis-sync",
				"public_host_header": "enter.example.com",
				"public_port":        443,
			},
		},
	}

	profile, ok := clientVLESSSubscriptionProfile(record)
	if !ok {
		t.Fatal("expected active camouflage VLESS access to produce a subscription profile")
	}
	for _, fragment := range []string{
		"vless://22222222-2222-4222-8222-222222222222@enter.example.com:443?",
		"security=tls",
		"type=ws",
		"path=%2Fassets%2Frtis-sync",
		"host=enter.example.com",
	} {
		if !strings.Contains(profile.URI, fragment) {
			t.Fatalf("subscription URI %q does not contain %q", profile.URI, fragment)
		}
	}
	if strings.Contains(profile.URI, "@enter.example.com:7080?") {
		t.Fatalf("subscription URI leaked backend endpoint: %q", profile.URI)
	}
}

func TestClientVLESSSubscriptionProfileRejectsUnprovisionedAccess(t *testing.T) {
	t.Parallel()

	_, ok := clientVLESSSubscriptionProfile(domain.ProvisioningAccess{
		Access: domain.ServiceAccess{Status: "active"},
		Instance: domain.Instance{
			ServiceCode:  "xray-core",
			Status:       "active",
			Enabled:      true,
			EndpointHost: "vpn.example.com",
			EndpointPort: 443,
		},
	})
	if ok {
		t.Fatal("unprovisioned VLESS access without UUID must not produce a subscription profile")
	}
}
