package externalegress

import (
	"strings"
	"testing"
)

func TestParseOpenVPNInlineProfile(t *testing.T) {
	raw := `client
dev tun
proto udp
remote vpn.example.com 1194
auth-user-pass
<ca>
-----BEGIN CERTIFICATE-----
test
-----END CERTIFICATE-----
</ca>
`
	preview, err := ParseOpenVPN([]byte(raw))
	if err != nil {
		t.Fatalf("ParseOpenVPN returned error: %v", err)
	}
	if preview.EndpointHost != "vpn.example.com" || preview.EndpointPort != 1194 || preview.Transport != "udp" {
		t.Fatalf("unexpected preview: %#v", preview)
	}
	if !preview.AuthUserPass || !containsString(preview.RequiredSecrets, "username_password") {
		t.Fatalf("expected separate username/password requirement: %#v", preview)
	}
}

func TestParseOpenVPNRejectsExecutionAndPathDirectives(t *testing.T) {
	for _, directive := range []string{
		"up /tmp/run", "plugin /tmp/plugin.so", "iproute /tmp/ip", "config /tmp/extra.conf", "crl-verify /etc/shadow",
		"tls-verify /tmp/provider-hook", "tls-crypt-v2-verify /tmp/provider-hook",
		"socks-proxy proxy.example.com 1080 /etc/shadow", "mark 0x4d590001", "engine /tmp/provider-engine.so",
	} {
		t.Run(strings.Fields(directive)[0], func(t *testing.T) {
			raw := "client\nremote vpn.example.com 1194\n" + directive + "\n"
			if _, err := ParseOpenVPN([]byte(raw)); err == nil {
				t.Fatalf("expected %q to be rejected", directive)
			}
		})
	}
}

func TestParseOpenVPNRejectsUnclosedOrUnknownInlineBlock(t *testing.T) {
	for _, raw := range []string{
		"client\nremote vpn.example.com 1194\n<ca>\nvalue\n",
		"client\nremote vpn.example.com 1194\n<unknown>\nvalue\n</unknown>\n",
	} {
		if _, err := ParseOpenVPN([]byte(raw)); err == nil {
			t.Fatal("expected unsafe inline block to be rejected")
		}
	}
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
