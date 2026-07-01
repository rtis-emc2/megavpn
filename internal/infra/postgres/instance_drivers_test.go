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

func TestAttachDefaultNetworkPolicyWireGuard(t *testing.T) {
	t.Parallel()

	spec := attachDefaultNetworkPolicy(domain.Instance{
		ServiceCode:  "wireguard",
		EndpointPort: 51820,
	}, map[string]any{})

	sysctl, ok := spec["sysctl"].(map[string]any)
	if !ok || stringify(sysctl["net.ipv4.ip_forward"]) != "1" {
		t.Fatalf("expected net.ipv4.ip_forward sysctl, got %#v", spec["sysctl"])
	}
	rules, ok := spec["firewall_rules"].([]any)
	if !ok || len(rules) != 1 {
		t.Fatalf("expected one firewall rule, got %#v", spec["firewall_rules"])
	}
	rule, ok := rules[0].(map[string]any)
	if !ok {
		t.Fatalf("expected firewall rule object, got %#v", rules[0])
	}
	if got := stringify(rule["protocol"]); got != "udp" {
		t.Fatalf("protocol = %q, want udp", got)
	}
	if got := firstIntValue(rule["port"]); got != 51820 {
		t.Fatalf("port = %d, want 51820", got)
	}
}

func TestAttachDefaultNetworkPolicyCanBeDisabled(t *testing.T) {
	t.Parallel()

	spec := attachDefaultNetworkPolicy(domain.Instance{
		ServiceCode:  "openvpn",
		EndpointPort: 1194,
	}, map[string]any{
		"network_policy_enabled": false,
	})

	if spec["sysctl"] != nil {
		t.Fatalf("did not expect sysctl when network policy disabled: %#v", spec["sysctl"])
	}
	if spec["firewall_rules"] != nil {
		t.Fatalf("did not expect firewall rules when network policy disabled: %#v", spec["firewall_rules"])
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

func TestBuildNginxServerConfigReverseProxyWithWebsiteFallback(t *testing.T) {
	cfg, err := buildNginxServerConfig(domain.Instance{
		Name:         "edge-nginx-ws",
		Slug:         "edge-nginx-ws",
		EndpointHost: "enter.example.com",
		EndpointPort: 443,
	}, map[string]any{
		"mode":                  "reverse_proxy",
		"tls_enabled":           true,
		"tls_cert_path":         "/etc/ssl/certs/enter.crt",
		"tls_key_path":          "/etc/ssl/private/enter.key",
		"location_path":         "/assets/rtis-sync",
		"upstream_url":          "http://127.0.0.1:7080",
		"fallback_upstream_url": "https://example.com",
		"fallback_host_header":  "example.com",
		"fallback_sni":          "example.com",
		"location_extra_lines":  "proxy_set_header Upgrade $http_upgrade;\nproxy_set_header Connection \"upgrade\";",
	})
	if err != nil {
		t.Fatalf("buildNginxServerConfig returned error: %v", err)
	}

	checks := []string{
		"server_name enter.example.com;",
		"location /assets/rtis-sync {",
		"proxy_pass http://127.0.0.1:7080;",
		"proxy_set_header Upgrade $http_upgrade;",
		"location / {",
		"proxy_pass https://example.com;",
		"proxy_set_header Host example.com;",
		"proxy_ssl_server_name on;",
		"proxy_ssl_name example.com;",
	}
	for _, check := range checks {
		if !strings.Contains(cfg, check) {
			t.Fatalf("expected config to contain %q, got:\n%s", check, cfg)
		}
	}
}

func TestBuildNginxServerConfigRejectsUnsafeLocationPath(t *testing.T) {
	_, err := buildNginxServerConfig(domain.Instance{
		Name:         "edge-nginx-ws",
		Slug:         "edge-nginx-ws",
		EndpointHost: "enter.example.com",
		EndpointPort: 443,
	}, map[string]any{
		"mode":          "reverse_proxy",
		"tls_enabled":   false,
		"location_path": "/assets/rtis-sync; return 200",
		"upstream_url":  "http://127.0.0.1:7080",
	})
	if err == nil {
		t.Fatal("expected unsafe location_path to be rejected")
	}
}

func TestBuildNginxServerConfigRejectsUnsafeServerName(t *testing.T) {
	_, err := buildNginxServerConfig(domain.Instance{
		Name:         "edge-nginx-ws",
		Slug:         "edge-nginx-ws",
		EndpointHost: "enter.example.com",
		EndpointPort: 443,
	}, map[string]any{
		"mode":          "reverse_proxy",
		"tls_enabled":   false,
		"server_name":   "edge.example.com;\nroot /tmp;",
		"location_path": "/assets/rtis-sync",
		"upstream_url":  "http://127.0.0.1:7080",
	})
	if err == nil {
		t.Fatal("expected unsafe server_name to be rejected")
	}
}

func TestValidateNginxServerNameAllowsDNSWildcardAndIP(t *testing.T) {
	for _, name := range []string{"edge.example.com", "*.edge.example.com", "127.0.0.1", "[2001:db8::1]", "_"} {
		if err := validateNginxServerName(name); err != nil {
			t.Fatalf("validateNginxServerName(%q) = %v", name, err)
		}
	}
}

func TestValidateNginxServerNameRejectsMalformedIPLiteral(t *testing.T) {
	for _, name := range []string{"[2001:db8::1", "2001:db8::zz"} {
		if err := validateNginxServerName(name); err == nil {
			t.Fatalf("expected malformed IP literal %q to be rejected", name)
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

func TestBuildXrayServerConfigAddsVLESSGroupRouting(t *testing.T) {
	cfg, err := buildXrayServerConfig(domain.Instance{
		Name:         "edge-vless",
		Slug:         "edge-vless",
		EndpointHost: "vpn.example.test",
		EndpointPort: 443,
	}, map[string]any{
		"security":            "none",
		"default_vless_group": "default",
		"managed_clients": []any{
			map[string]any{
				"id":          "11111111-1111-4111-8111-111111111111",
				"email":       "direct-user",
				"vless_group": "default",
			},
			map[string]any{
				"id":          "22222222-2222-4222-8222-222222222222",
				"email":       "restricted-user",
				"vless_group": "restricted",
			},
		},
		"vless_groups": []any{
			map[string]any{
				"key":          "default",
				"label":        "Default direct",
				"outbound_tag": "direct",
			},
			map[string]any{
				"key":          "restricted",
				"label":        "Restricted",
				"outbound_tag": "block",
				"rules": []any{
					map[string]any{
						"domain":       []any{"geosite:category-ads-all"},
						"outbound_tag": "block",
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("buildXrayServerConfig returned error: %v", err)
	}

	inbounds := cfg["inbounds"].([]any)
	settings := inbounds[0].(map[string]any)["settings"].(map[string]any)
	clients := settings["clients"].([]any)
	if len(clients) != 2 {
		t.Fatalf("clients = %#v, want 2", clients)
	}
	if _, leaked := clients[0].(map[string]any)["vless_group"]; leaked {
		t.Fatalf("internal vless_group leaked into xray client: %#v", clients[0])
	}

	routing, ok := cfg["routing"].(map[string]any)
	if !ok {
		t.Fatalf("routing missing: %#v", cfg)
	}
	rules, ok := routing["rules"].([]any)
	if !ok || len(rules) != 3 {
		t.Fatalf("routing rules = %#v, want 3", routing["rules"])
	}
	if got := stringify(rules[0].(map[string]any)["outboundTag"]); got != "direct" {
		t.Fatalf("first routing outboundTag = %q, want direct", got)
	}
	if got := stringify(rules[1].(map[string]any)["outboundTag"]); got != "block" {
		t.Fatalf("restricted domain rule outboundTag = %q, want block", got)
	}
	if got := stringify(rules[2].(map[string]any)["outboundTag"]); got != "block" {
		t.Fatalf("restricted fallback outboundTag = %q, want block", got)
	}
}

func TestBuildXrayServerConfigAddsVLESSRemoteEgressOutbound(t *testing.T) {
	cfg, err := buildXrayServerConfig(domain.Instance{
		Name:         "edge-vless",
		Slug:         "edge-vless",
		EndpointHost: "vpn.example.test",
		EndpointPort: 443,
	}, map[string]any{
		"security":            "none",
		"default_vless_group": "default",
		"managed_clients": []any{
			map[string]any{
				"id":          "33333333-3333-4333-8333-333333333333",
				"email":       "remote-user",
				"vless_group": "default",
			},
		},
		"vless_groups": []any{
			map[string]any{
				"key":          "default",
				"label":        "Default egress",
				"outbound_tag": "egress-default",
				"outbound": map[string]any{
					"tag":         "egress-default",
					"protocol":    "freedom",
					"sendThrough": "10.240.35.245",
					"settings":    map[string]any{"domainStrategy": "UseIP"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("buildXrayServerConfig returned error: %v", err)
	}

	outbounds, ok := cfg["outbounds"].([]any)
	if !ok {
		t.Fatalf("outbounds missing: %#v", cfg["outbounds"])
	}
	found := false
	for _, item := range outbounds {
		outbound, _ := item.(map[string]any)
		if stringify(outbound["tag"]) != "egress-default" {
			continue
		}
		found = true
		if got := stringify(outbound["sendThrough"]); got != "10.240.35.245" {
			t.Fatalf("sendThrough = %q, want backhaul ingress address", got)
		}
	}
	if !found {
		t.Fatalf("remote egress outbound missing from %#v", outbounds)
	}
	routing := cfg["routing"].(map[string]any)
	rules := routing["rules"].([]any)
	if got := stringify(rules[len(rules)-1].(map[string]any)["outboundTag"]); got != "egress-default" {
		t.Fatalf("fallback outboundTag = %q, want egress-default", got)
	}
}

func TestBuildXrayUnitFileDoesNotUseShell(t *testing.T) {
	t.Parallel()

	unit := buildXrayUnitFile("megavpn-xray-edge", "/usr/local/etc/xray/edge.json", domain.Instance{Name: "edge", Slug: "edge"})
	if strings.Contains(unit, "/bin/sh") || strings.Contains(unit, " -c ") {
		t.Fatalf("unit must not use shell execution:\n%s", unit)
	}
	if !strings.Contains(unit, "ExecStart=/usr/bin/env xray run -config /usr/local/etc/xray/edge.json") {
		t.Fatalf("unit missing direct xray ExecStart:\n%s", unit)
	}
}

func TestValidateSystemdExecPathArgRejectsShellTokens(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"/tmp/$(id>/root/pwn)", "/tmp/a;reboot", "/tmp/with space.json", "../relative.json"} {
		path := path
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			if err := validateSystemdExecPathArg(path); err == nil {
				t.Fatalf("expected path %q to be rejected", path)
			}
		})
	}
}
