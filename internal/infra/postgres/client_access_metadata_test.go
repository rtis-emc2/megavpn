package postgres

import "testing"

func TestApplyXrayPublicClientEndpointMetadata(t *testing.T) {
	t.Parallel()

	metadata := map[string]any{}
	inbound := map[string]any{
		"service_code":   "xray-core",
		"endpoint_host":  "portal.nlgate.ru",
		"endpoint_port":  7080,
		"endpoint":       "portal.nlgate.ru:7080",
		"instance_name":  "portal-edge-xray-http-xray-ws",
		"service_label":  "Xray-core",
		"route_source":   "service",
		"route_endpoint": "portal.nlgate.ru:7080",
	}
	spec := map[string]any{
		"security":           "none",
		"network":            "ws",
		"path":               "/assets/rtis-sync",
		"public_security":    "tls",
		"public_network":     "ws",
		"public_path":        "/assets/rtis-sync",
		"public_host_header": "portal.nlgate.ru",
		"public_port":        443,
	}

	applyXrayPublicClientEndpointMetadata(metadata, inbound, spec)

	if got := firstString(inbound["endpoint"]); got != "portal.nlgate.ru:443" {
		t.Fatalf("inbound endpoint = %q, want public endpoint portal.nlgate.ru:443", got)
	}
	if got := firstString(inbound["backend_endpoint"]); got != "portal.nlgate.ru:7080" {
		t.Fatalf("backend endpoint = %q, want portal.nlgate.ru:7080", got)
	}
	if got := firstString(inbound["endpoint_kind"]); got != "public" {
		t.Fatalf("endpoint kind = %q, want public", got)
	}
	if got := firstString(metadata["public_security"]); got != "tls" {
		t.Fatalf("public security = %q, want tls", got)
	}
	if got := firstIntValue(metadata["public_port"]); got != 443 {
		t.Fatalf("public port = %d, want 443", got)
	}
}

func TestApplyXrayPublicClientEndpointMetadataIgnoresPlainBackend(t *testing.T) {
	t.Parallel()

	metadata := map[string]any{}
	inbound := map[string]any{
		"endpoint_host": "vpn.example.com",
		"endpoint_port": 8443,
		"endpoint":      "vpn.example.com:8443",
	}
	spec := map[string]any{
		"security": "reality",
		"network":  "tcp",
	}

	applyXrayPublicClientEndpointMetadata(metadata, inbound, spec)

	if got := firstString(inbound["endpoint"]); got != "vpn.example.com:8443" {
		t.Fatalf("plain endpoint changed to %q", got)
	}
	if got := firstString(inbound["endpoint_kind"]); got != "" {
		t.Fatalf("endpoint kind = %q, want empty for plain backend", got)
	}
}
