package http

import (
	"strings"

	nethttp "net/http"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

type servicePackComponent struct {
	Label                string         `json:"label"`
	Description          string         `json:"description,omitempty"`
	ServiceCode          string         `json:"service_code"`
	PresetKey            string         `json:"preset_key"`
	NameSuffix           string         `json:"name_suffix"`
	SlugSuffix           string         `json:"slug_suffix"`
	EndpointPort         int            `json:"endpoint_port"`
	RequiresEndpointHost bool           `json:"requires_endpoint_host"`
	Spec                 map[string]any `json:"spec"`
}

type servicePackDefinition struct {
	Key                  string                 `json:"key"`
	Label                string                 `json:"label"`
	Description          string                 `json:"description"`
	BaseNameTemplate     string                 `json:"base_name_template"`
	EndpointHint         string                 `json:"endpoint_hint"`
	RequiresEndpointHost bool                   `json:"requires_endpoint_host"`
	PlatformNotes        []string               `json:"platform_notes"`
	Recommendations      []string               `json:"recommendations"`
	Components           []servicePackComponent `json:"components"`
}

type createServicePackRequest struct {
	NodeID       string `json:"node_id"`
	BaseName     string `json:"base_name"`
	EndpointHost string `json:"endpoint_host"`
}

func servicePackDefinitions() []servicePackDefinition {
	return []servicePackDefinition{
		{
			Key:                  "ipsec_xl2tpd_access",
			Label:                "IPsec + XL2TPD Access",
			Description:          "Создает transport instance IPsec / IKEv2 и companion instance XL2TPD для L2TP remote-access на одном endpoint.",
			BaseNameTemplate:     "edge-l2tp",
			EndpointHint:         "l2tp.example.com",
			RequiresEndpointHost: true,
			Recommendations: []string{
				"Используй один endpoint host для обоих инстансов.",
				"После создания проверь PSK и bootstrap credentials перед выдачей доступов.",
			},
			Components: []servicePackComponent{
				{Label: "IPsec / IKEv2", Description: "Transport/security layer для L2TP remote-access.", ServiceCode: "ipsec", PresetKey: "ikev2_psk", NameSuffix: "ipsec", SlugSuffix: "ipsec", EndpointPort: 1701, RequiresEndpointHost: true, Spec: map[string]any{"service_profile": "ikev2_psk", "left": "%defaultroute", "right": "%any", "ike": "aes256-sha1-modp1024", "esp": "aes256-sha1", "secrets_mode": "0600", "config_mode": "0644"}},
				{Label: "L2TP Access", Description: "Companion access service на том же endpoint host.", ServiceCode: "xl2tpd", PresetKey: "remote_access", NameSuffix: "l2tp-access", SlugSuffix: "l2tp-access", EndpointPort: 1701, RequiresEndpointHost: true, Spec: map[string]any{"service_profile": "remote_access", "local_ip": "10.20.0.1", "ip_range_start": "10.20.0.10", "ip_range_end": "10.20.0.200", "ppp_dns_primary": "1.1.1.1", "ppp_dns_secondary": "1.0.0.1", "config_mode": "0644"}},
			},
		},
		{
			Key:                  "xray_vless_reality",
			Label:                "Xray VLESS / Reality",
			Description:          "Создает базовый VLESS/Reality instance для прямого client-facing endpoint.",
			BaseNameTemplate:     "edge-xray",
			EndpointHint:         "vpn.example.com",
			RequiresEndpointHost: true,
			Recommendations: []string{
				"Это канонический стартовый пакет для Xray.",
				"Reality keys и short-id control plane сгенерирует автоматически на первом apply.",
			},
			Components: []servicePackComponent{
				{Label: "Xray VLESS / Reality", Description: "Прямой публичный transport без edge companion.", ServiceCode: "xray-core", PresetKey: "reality_tcp", NameSuffix: "xray-reality", SlugSuffix: "xray-reality", EndpointPort: 443, RequiresEndpointHost: true, Spec: map[string]any{"service_profile": "reality_tcp", "security": "reality", "network": "tcp", "dest": "www.cloudflare.com:443", "fingerprint": "chrome", "config_mode": "0640"}},
			},
		},
		{
			Key:                  "xray_nginx_grpc_edge",
			Label:                "Xray + Nginx gRPC Edge",
			Description:          "Создает backend Xray gRPC instance и отдельный Nginx TLS edge для публичного endpoint.",
			BaseNameTemplate:     "edge-xray-grpc",
			EndpointHint:         "edge.example.com",
			RequiresEndpointHost: true,
			PlatformNotes: []string{
				"Nginx отвечает за публичный TLS edge и маршрутизацию, Xray слушает отдельный backend-port без Reality.",
				"Для боевого контура задай реальный cert/key path или переведи edge на внешний certificate management.",
			},
			Recommendations: []string{
				"Используй, когда нужен app-like gRPC transport под публичным TLS edge.",
				"Не смешивай этот пакет с direct-Reality на том же 443 без явного ingress-плана.",
			},
			Components: []servicePackComponent{
				{Label: "Xray gRPC Backend", Description: "Runtime backend для Nginx gRPC edge.", ServiceCode: "xray-core", PresetKey: "nginx_grpc_backend", NameSuffix: "xray-grpc", SlugSuffix: "xray-grpc", EndpointPort: 7443, RequiresEndpointHost: true, Spec: map[string]any{"service_profile": "nginx_grpc_backend", "security": "none", "network": "grpc", "service_name": "vless-grpc", "public_security": "tls", "public_network": "grpc", "public_service_name": "vless-grpc", "public_port": 443, "config_mode": "0640"}},
				{Label: "Nginx gRPC Edge", Description: "Публичный TLS ingress для backend Xray gRPC.", ServiceCode: "nginx", PresetKey: "grpc_edge", NameSuffix: "nginx-grpc-edge", SlugSuffix: "nginx-grpc-edge", EndpointPort: 443, RequiresEndpointHost: true, Spec: map[string]any{"service_profile": "grpc_edge", "mode": "grpc_proxy", "location_path": "/vless-grpc", "server_name": "{{endpoint_host}}", "upstream_url": "grpc://127.0.0.1:7443", "tls_enabled": true, "tls_cert_path": "/etc/letsencrypt/live/{{endpoint_host}}/fullchain.pem", "tls_key_path": "/etc/letsencrypt/live/{{endpoint_host}}/privkey.pem", "config_mode": "0644"}},
			},
		},
		{
			Key:                  "xray_nginx_http_edge",
			Label:                "Xray HTTP/WebSocket Edge",
			Description:          "Создает backend Xray HTTP/WebSocket instance и Nginx TLS edge для web-like транспорта.",
			BaseNameTemplate:     "edge-xray-http",
			EndpointHint:         "edge.example.com",
			RequiresEndpointHost: true,
			PlatformNotes: []string{
				"Этот пакет нужен для HTTP/WebSocket transport. Это не direct-Reality профиль.",
			},
			Recommendations: []string{
				"Используй, когда нужен HTTP/WebSocket transport вместо прямого Reality.",
				"При необходимости path можно потом поменять в revision/spec и на Xray, и на Nginx edge.",
			},
			Components: []servicePackComponent{
				{Label: "Xray HTTP/WebSocket Backend", Description: "Runtime backend для Nginx HTTP/WebSocket edge.", ServiceCode: "xray-core", PresetKey: "nginx_ws_backend", NameSuffix: "xray-http", SlugSuffix: "xray-http", EndpointPort: 7080, RequiresEndpointHost: true, Spec: map[string]any{"service_profile": "nginx_ws_backend", "security": "none", "network": "ws", "path": "/ws", "public_security": "tls", "public_network": "ws", "public_path": "/ws", "public_host_header": "{{endpoint_host}}", "public_port": 443, "config_mode": "0640"}},
				{Label: "Nginx HTTP/WebSocket Edge", Description: "Публичный TLS reverse-proxy для backend Xray ws.", ServiceCode: "nginx", PresetKey: "ws_edge", NameSuffix: "nginx-http-edge", SlugSuffix: "nginx-http-edge", EndpointPort: 443, RequiresEndpointHost: true, Spec: map[string]any{"service_profile": "ws_edge", "mode": "reverse_proxy", "server_name": "{{endpoint_host}}", "upstream_url": "http://127.0.0.1:7080", "tls_enabled": true, "tls_cert_path": "/etc/letsencrypt/live/{{endpoint_host}}/fullchain.pem", "tls_key_path": "/etc/letsencrypt/live/{{endpoint_host}}/privkey.pem", "location_extra_lines": "proxy_set_header Upgrade $http_upgrade;\nproxy_set_header Connection \"upgrade\";", "config_mode": "0644"}},
			},
		},
		{
			Key:                  "openvpn_tcp_11994",
			Label:                "OpenVPN TCP 11994",
			Description:          "Создает OpenVPN instance с TCP baseline на порту 11994.",
			BaseNameTemplate:     "edge-openvpn-tcp",
			EndpointHint:         "ovpn.example.com",
			RequiresEndpointHost: true,
			PlatformNotes: []string{
				"Control plane автоматически использует platform CA root для OpenVPN, если pki_scope=platform.",
			},
			Recommendations: []string{
				"Рекомендуемый baseline, если нужен более совместимый TCP transport.",
			},
			Components: []servicePackComponent{
				{Label: "OpenVPN TCP", Description: "OpenVPN instance с platform-managed PKI.", ServiceCode: "openvpn", PresetKey: "tcp_11994", NameSuffix: "openvpn-tcp", SlugSuffix: "openvpn-tcp", EndpointPort: 11994, RequiresEndpointHost: true, Spec: map[string]any{"service_profile": "tcp_11994", "pki_scope": "platform", "pki_profile": "default", "proto": "tcp", "dev": "tun", "server_network": "10.8.0.0", "server_netmask": "255.255.255.0", "config_mode": "0644"}},
			},
		},
		{
			Key:                  "openvpn_udp_1194",
			Label:                "OpenVPN UDP 1194",
			Description:          "Создает OpenVPN instance с классическим UDP baseline на порту 1194.",
			BaseNameTemplate:     "edge-openvpn-udp",
			EndpointHint:         "ovpn.example.com",
			RequiresEndpointHost: true,
			PlatformNotes: []string{
				"Control plane автоматически использует platform CA root для OpenVPN, если pki_scope=platform.",
			},
			Recommendations: []string{
				"Подходит там, где throughput важнее camouflage и firewall-path стабильнее.",
			},
			Components: []servicePackComponent{
				{Label: "OpenVPN UDP", Description: "OpenVPN instance с platform-managed PKI.", ServiceCode: "openvpn", PresetKey: "udp_1194", NameSuffix: "openvpn-udp", SlugSuffix: "openvpn-udp", EndpointPort: 1194, RequiresEndpointHost: true, Spec: map[string]any{"service_profile": "udp_1194", "pki_scope": "platform", "pki_profile": "default", "proto": "udp", "dev": "tun", "server_network": "10.8.0.0", "server_netmask": "255.255.255.0", "config_mode": "0644"}},
			},
		},
	}
}

func findServicePack(key string) (servicePackDefinition, bool) {
	for _, pack := range servicePackDefinitions() {
		if pack.Key == key {
			return pack, true
		}
	}
	return servicePackDefinition{}, false
}

func (s *Server) listServicePacks(w nethttp.ResponseWriter, r *nethttp.Request) {
	writeJSON(w, 200, servicePackDefinitions())
}

func (s *Server) createServicePackInstances(w nethttp.ResponseWriter, r *nethttp.Request) {
	packKey := strings.TrimSpace(r.PathValue("key"))
	pack, ok := findServicePack(packKey)
	if !ok {
		writeErr(w, 404, "service pack not found")
		return
	}
	var req createServicePackRequest
	if !decode(r, &req) || strings.TrimSpace(req.NodeID) == "" {
		writeErr(w, 400, "invalid service pack payload: node_id is required")
		return
	}
	req.BaseName = strings.TrimSpace(req.BaseName)
	req.EndpointHost = strings.TrimSpace(req.EndpointHost)
	if req.BaseName == "" {
		req.BaseName = pack.BaseNameTemplate
	}
	if pack.RequiresEndpointHost && req.EndpointHost == "" {
		writeErr(w, 400, "invalid service pack payload: endpoint_host is required")
		return
	}
	created := make([]domain.Instance, 0, len(pack.Components))
	for _, component := range pack.Components {
		name := req.BaseName
		if suffix := strings.TrimSpace(component.NameSuffix); suffix != "" {
			name += "-" + suffix
		}
		slug := slugifyHTTP(req.BaseName)
		if suffix := strings.TrimSpace(component.SlugSuffix); suffix != "" {
			slug += "-" + slugifyHTTP(suffix)
		}
		spec := interpolatePackSpec(cloneMapHTTP(component.Spec), map[string]string{
			"endpoint_host":  req.EndpointHost,
			"base_name":      req.BaseName,
			"component_slug": slug,
		})
		instance := domain.Instance{
			NodeID:       req.NodeID,
			ServiceCode:  component.ServiceCode,
			Name:         name,
			Slug:         slug,
			EndpointHost: req.EndpointHost,
			EndpointPort: component.EndpointPort,
			Spec:         spec,
		}
		createdInstance, err := s.store.CreateInstance(r.Context(), instance)
		if err != nil {
			writeJSON(w, 409, response{
				"status":            "partial_failure",
				"error":             err.Error(),
				"service_pack_key":  pack.Key,
				"created_instances": created,
			})
			return
		}
		created = append(created, createdInstance)
	}
	authCtx, ok := authFromRequest(r)
	if ok {
		_, _ = s.store.CreateAuditForUser(r.Context(), &authCtx.User.ID, "service_pack.create", "service_pack", nil, "service pack created: "+pack.Key)
	}
	writeJSON(w, 201, response{
		"status":            "ok",
		"service_pack_key":  pack.Key,
		"created_instances": created,
	})
}

func slugifyHTTP(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var out strings.Builder
	lastDash := false
	for _, r := range value {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			out.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			out.WriteByte('-')
			lastDash = true
		}
	}
	value = strings.Trim(out.String(), "-")
	if value == "" {
		return "instance"
	}
	return value
}

func cloneMapHTTP(src map[string]any) map[string]any {
	if src == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(src))
	for key, value := range src {
		out[key] = value
	}
	return out
}

func interpolatePackSpec(value any, vars map[string]string) map[string]any {
	out, _ := interpolatePackValue(value, vars).(map[string]any)
	if out == nil {
		return map[string]any{}
	}
	return out
}

func interpolatePackValue(value any, vars map[string]string) any {
	switch typed := value.(type) {
	case string:
		out := typed
		for key, val := range vars {
			out = strings.ReplaceAll(out, "{{"+key+"}}", val)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = interpolatePackValue(item, vars)
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, interpolatePackValue(item, vars))
		}
		return out
	default:
		return value
	}
}
