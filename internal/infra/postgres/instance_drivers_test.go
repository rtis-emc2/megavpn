package postgres

import (
	"strings"
	"testing"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

func TestBuildXrayServerConfigGRPCBackend(t *testing.T) {
	cfg, err := buildXrayServerConfig(domain.Instance{
		Name:         "edge-xray-grpc",
		Slug:         "edge-xray-grpc",
		EndpointHost: "edge.example.com",
		EndpointPort: 7443,
	}, map[string]any{
		"security":     "none",
		"network":      "grpc",
		"service_name": "vless-grpc",
	})
	if err != nil {
		t.Fatalf("buildXrayServerConfig returned error: %v", err)
	}

	inbounds, ok := cfg["inbounds"].([]any)
	if !ok || len(inbounds) != 1 {
		t.Fatalf("expected one inbound, got %#v", cfg["inbounds"])
	}
	inbound, ok := inbounds[0].(map[string]any)
	if !ok {
		t.Fatalf("expected inbound object, got %#v", inbounds[0])
	}
	stream, ok := inbound["streamSettings"].(map[string]any)
	if !ok {
		t.Fatalf("expected streamSettings object, got %#v", inbound["streamSettings"])
	}
	if got := stringify(stream["security"]); got != "none" {
		t.Fatalf("expected security=none, got %q", got)
	}
	if got := stringify(stream["network"]); got != "grpc" {
		t.Fatalf("expected network=grpc, got %q", got)
	}
	if _, exists := stream["realitySettings"]; exists {
		t.Fatalf("did not expect realitySettings for backend profile")
	}
	grpcSettings, ok := stream["grpcSettings"].(map[string]any)
	if !ok {
		t.Fatalf("expected grpcSettings object, got %#v", stream["grpcSettings"])
	}
	if got := stringify(grpcSettings["serviceName"]); got != "vless-grpc" {
		t.Fatalf("expected serviceName=vless-grpc, got %q", got)
	}
}

func TestBuildNginxServerConfigGRPCProxy(t *testing.T) {
	cfg, err := buildNginxServerConfig(domain.Instance{
		Name:         "edge-nginx-grpc",
		Slug:         "edge-nginx-grpc",
		EndpointHost: "edge.example.com",
		EndpointPort: 443,
	}, map[string]any{
		"mode":          "grpc_proxy",
		"tls_enabled":   true,
		"tls_cert_path": "/etc/ssl/certs/test.crt",
		"tls_key_path":  "/etc/ssl/private/test.key",
		"location_path": "/vless-grpc",
		"upstream_url":  "grpc://127.0.0.1:7443",
	})
	if err != nil {
		t.Fatalf("buildNginxServerConfig returned error: %v", err)
	}

	checks := []string{
		"listen 443 ssl http2;",
		"location /vless-grpc {",
		"grpc_pass grpc://127.0.0.1:7443;",
		"grpc_set_header Host $host;",
	}
	for _, check := range checks {
		if !strings.Contains(cfg, check) {
			t.Fatalf("expected config to contain %q, got:\n%s", check, cfg)
		}
	}
}

func TestBuildXrayServerConfigTLS(t *testing.T) {
	cfg, err := buildXrayServerConfig(domain.Instance{
		Name:         "edge-xray-tls",
		Slug:         "edge-xray-tls",
		EndpointHost: "edge.example.com",
		EndpointPort: 443,
	}, map[string]any{
		"security":      "tls",
		"network":       "ws",
		"path":          "/ws",
		"tls_cert_path": "/etc/megavpn/certs/edge/fullchain.pem",
		"tls_key_path":  "/etc/megavpn/certs/edge/privkey.pem",
	})
	if err != nil {
		t.Fatalf("buildXrayServerConfig returned error: %v", err)
	}

	inbounds, ok := cfg["inbounds"].([]any)
	if !ok || len(inbounds) != 1 {
		t.Fatalf("expected one inbound, got %#v", cfg["inbounds"])
	}
	inbound, ok := inbounds[0].(map[string]any)
	if !ok {
		t.Fatalf("expected inbound object, got %#v", inbounds[0])
	}
	stream, ok := inbound["streamSettings"].(map[string]any)
	if !ok {
		t.Fatalf("expected streamSettings object, got %#v", inbound["streamSettings"])
	}
	if got := stringify(stream["security"]); got != "tls" {
		t.Fatalf("expected security=tls, got %q", got)
	}
	tlsSettings, ok := stream["tlsSettings"].(map[string]any)
	if !ok {
		t.Fatalf("expected tlsSettings object, got %#v", stream["tlsSettings"])
	}
	certs, ok := tlsSettings["certificates"].([]any)
	if !ok || len(certs) != 1 {
		t.Fatalf("expected one tls certificate entry, got %#v", tlsSettings["certificates"])
	}
}
