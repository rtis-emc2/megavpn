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
