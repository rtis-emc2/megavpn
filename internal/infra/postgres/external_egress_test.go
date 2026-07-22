package postgres

import (
	"strings"
	"testing"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

func TestValidateExternalEgressRuntimeProfileRequirements(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		protocol  string
		config    string
		purposes  map[string]bool
		wantError string
	}{
		{
			name: "OpenVPN separate credentials are present", protocol: "openvpn",
			config:   "client\nremote vpn.example.com 1194\nauth-user-pass\n",
			purposes: map[string]bool{"config": true, "username": true, "password": true},
		},
		{
			name: "OpenVPN password is missing", protocol: "openvpn",
			config:    "client\nremote vpn.example.com 1194\nauth-user-pass\n",
			purposes:  map[string]bool{"config": true, "username": true},
			wantError: "username and password",
		},
		{
			name: "WireGuard separate private key is present", protocol: "wireguard",
			config:   "[Interface]\nAddress = 10.0.0.2/32\n[Peer]\nPublicKey = public\nEndpoint = vpn.example.com:51820\nAllowedIPs = 0.0.0.0/0\n",
			purposes: map[string]bool{"config": true, "private_key": true},
		},
		{
			name: "WireGuard default route is missing", protocol: "wireguard",
			config:    "[Interface]\nPrivateKey = private\nAddress = 10.0.0.2/32\n[Peer]\nPublicKey = public\nEndpoint = vpn.example.com:51820\nAllowedIPs = 10.20.0.0/16\n",
			purposes:  map[string]bool{"config": true},
			wantError: "0.0.0.0/0",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateExternalEgressRuntimeProfile(test.protocol, []byte(test.config), test.purposes)
			if test.wantError == "" {
				if err != nil {
					t.Fatalf("validateExternalEgressRuntimeProfile() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("validateExternalEgressRuntimeProfile() error = %v, want substring %q", err, test.wantError)
			}
		})
	}
}

func TestExternalEgressResourceCandidatesAreManagedAndDistinct(t *testing.T) {
	t.Parallel()

	profileID := "123e4567-e89b-42d3-a456-426614174000"
	nodeID := "123e4567-e89b-42d3-a456-426614174001"
	interfaces := map[string]bool{}
	tables := map[int]bool{}
	marks := map[int]bool{}
	for attempt := 0; attempt < 256; attempt++ {
		interfaceName := externalEgressInterfaceName(profileID, nodeID, attempt)
		table := externalEgressRoutingTable(profileID, nodeID, attempt)
		mark := externalEgressFWMark(profileID, attempt)
		if len(interfaceName) != 14 || !strings.HasPrefix(interfaceName, "mgev") {
			t.Fatalf("interface candidate %q is outside the managed format", interfaceName)
		}
		if table < 40000 || table > 48999 {
			t.Fatalf("routing table %d is outside the managed range", table)
		}
		if mark < 0x4d590000 || mark > 0x4d59ffff {
			t.Fatalf("fwmark %#x is outside the managed range", mark)
		}
		if interfaces[interfaceName] || tables[table] || marks[mark] {
			t.Fatalf("resource candidate collision at attempt %d", attempt)
		}
		interfaces[interfaceName], tables[table], marks[mark] = true, true, true
	}
}

func TestApplyExternalEgressGroupRoutingSpecMarksOnlySelectedGroup(t *testing.T) {
	t.Parallel()

	spec := map[string]any{
		"vless_groups": []any{
			map[string]any{"key": "default", "outbound_tag": "direct"},
			map[string]any{"key": "provider_users", "outbound_tag": "direct"},
		},
	}
	group := domain.ClientAccessGroup{GroupKey: "provider_users"}
	profile := domain.ExternalEgressProfile{ID: "123e4567-e89b-42d3-a456-426614174000"}
	deployment := domain.ExternalEgressDeployment{
		ID: "123e4567-e89b-42d3-a456-426614174001", RoutingTable: "40123",
		InterfaceName: "mgev0123456789", FWMark: 0x4d590123,
	}

	next, changed, err := applyExternalEgressGroupRoutingSpec(spec, group, profile, deployment)
	if err != nil {
		t.Fatalf("applyExternalEgressGroupRoutingSpec() error = %v", err)
	}
	if !changed {
		t.Fatal("applyExternalEgressGroupRoutingSpec() changed = false, want true")
	}
	groups := next["vless_groups"].([]any)
	defaultGroup := groups[0].(map[string]any)
	if defaultGroup["outbound_tag"] != "direct" || defaultGroup["outbound"] != nil || defaultGroup["egress"] != nil {
		t.Fatalf("unselected group was modified: %#v", defaultGroup)
	}
	selectedGroup := groups[1].(map[string]any)
	outbound := selectedGroup["outbound"].(map[string]any)
	stream := outbound["streamSettings"].(map[string]any)
	sockopt := stream["sockopt"].(map[string]any)
	if sockopt["mark"] != deployment.FWMark {
		t.Fatalf("selected group mark = %#v, want %d", sockopt["mark"], deployment.FWMark)
	}
	egress := selectedGroup["egress"].(map[string]any)
	if egress["external_egress_profile_id"] != profile.ID || egress["deployment_id"] != deployment.ID {
		t.Fatalf("selected group external egress metadata = %#v", egress)
	}
}
