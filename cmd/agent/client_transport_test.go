package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rtis-emc2/megavpn/internal/platform/config"
)

func TestNewAgentHTTPClientRejectsRemotePlainHTTP(t *testing.T) {
	_, err := newAgentHTTPClient("http://control.example.com:8080", config.AgentConfig{})
	if err == nil || !strings.Contains(err.Error(), "must use HTTPS") {
		t.Fatalf("expected secure transport error, got %v", err)
	}
}

func TestNewAgentHTTPClientAllowsLoopbackHTTP(t *testing.T) {
	if _, err := newAgentHTTPClient("http://127.0.0.1:8080", config.AgentConfig{}); err != nil {
		t.Fatalf("loopback HTTP should remain available for local development: %v", err)
	}
}

func TestNewAgentHTTPClientRequiresCertificatePair(t *testing.T) {
	_, err := newAgentHTTPClient("https://control.example.com", config.AgentConfig{TLSCertFile: "/tmp/client.crt"})
	if err == nil || !strings.Contains(err.Error(), "must be configured together") {
		t.Fatalf("expected certificate pair validation error, got %v", err)
	}
}

func TestNewAgentHTTPClientRejectsURLQueryAndFragment(t *testing.T) {
	for _, rawURL := range []string{
		"https://control.example.com/api?token=secret",
		"https://control.example.com/api#fragment",
	} {
		if _, err := newAgentHTTPClient(rawURL, config.AgentConfig{}); err == nil || !strings.Contains(err.Error(), "query parameters or a fragment") {
			t.Fatalf("expected ambiguous URL rejection for %q, got %v", rawURL, err)
		}
	}
}

func TestNewAgentHTTPClientRejectsTLSFilesForPlainHTTP(t *testing.T) {
	_, err := newAgentHTTPClient("http://127.0.0.1:8080", config.AgentConfig{TLSCAFile: "/tmp/ca.pem"})
	if err == nil || !strings.Contains(err.Error(), "require an HTTPS") {
		t.Fatalf("expected unused TLS configuration rejection, got %v", err)
	}
}

func TestNewAgentHTTPClientRejectsInvalidCA(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(path, []byte("not a certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := newAgentHTTPClient("https://control.example.com", config.AgentConfig{TLSCAFile: path})
	if err == nil || !strings.Contains(err.Error(), "no valid certificates") {
		t.Fatalf("expected invalid CA error, got %v", err)
	}
}

func TestNewAgentHTTPClientRejectsURLCredentials(t *testing.T) {
	_, err := newAgentHTTPClient("https://user:password@control.example.com", config.AgentConfig{})
	if err == nil || !strings.Contains(err.Error(), "must not contain credentials") {
		t.Fatalf("expected URL credential rejection, got %v", err)
	}
}
