package main

import (
	"strings"
	"testing"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

func TestValidateControlPlaneTLSApplySettingsRejectsUnsafeServerName(t *testing.T) {
	t.Parallel()

	settings := domain.ControlPlaneTLSSettings{
		Enabled:       true,
		PublicBaseURL: "https://control.example.com",
		ServerName:    "control.example.com;\nlocation /x { return 200; }",
		ListenPort:    443,
		UpstreamURL:   "http://127.0.0.1:8080",
	}
	err := validateControlPlaneTLSApplySettings(settings)
	if err == nil || !strings.Contains(err.Error(), "server_name") {
		t.Fatalf("error = %v, want unsafe server_name rejection", err)
	}
}

func TestValidateControlPlaneTLSApplySettingsAllowsWildcardDNS(t *testing.T) {
	t.Parallel()

	settings := domain.ControlPlaneTLSSettings{
		Enabled:       true,
		PublicBaseURL: "https://control.example.com",
		ServerName:    "*.control.example.com",
		ListenPort:    443,
		UpstreamURL:   "http://127.0.0.1:8080",
	}
	if err := validateControlPlaneTLSApplySettings(settings); err != nil {
		t.Fatalf("expected wildcard DNS server_name to be accepted: %v", err)
	}
}

func TestValidateControlPlaneTLSApplySettingsRejectsMalformedIPLiteral(t *testing.T) {
	t.Parallel()

	settings := domain.ControlPlaneTLSSettings{
		Enabled:       true,
		PublicBaseURL: "https://control.example.com",
		ServerName:    "[2001:db8::1",
		ListenPort:    443,
		UpstreamURL:   "http://127.0.0.1:8080",
	}
	err := validateControlPlaneTLSApplySettings(settings)
	if err == nil || !strings.Contains(err.Error(), "server_name") {
		t.Fatalf("error = %v, want malformed server_name rejection", err)
	}
}

func TestRenderControlPlaneNginxConfigSupportsWebSocketUpgrade(t *testing.T) {
	t.Parallel()

	conf := renderControlPlaneNginxConfig(domain.ControlPlaneTLSSettings{
		ServerName:  "control.example.com",
		ListenPort:  58765,
		UpstreamURL: "http://127.0.0.1:8080",
	}, "/etc/megavpn/fullchain.pem", "/etc/megavpn/privkey.pem")
	for _, want := range []string{
		"map $http_upgrade $megavpn_control_plane_connection_upgrade",
		"proxy_http_version 1.1;",
		"proxy_set_header Upgrade $http_upgrade;",
		"proxy_set_header Connection $megavpn_control_plane_connection_upgrade;",
		"proxy_buffering off;",
		"proxy_read_timeout 2h;",
		"proxy_send_timeout 2h;",
	} {
		if !strings.Contains(conf, want) {
			t.Fatalf("rendered nginx config does not contain %q:\n%s", want, conf)
		}
	}
}
