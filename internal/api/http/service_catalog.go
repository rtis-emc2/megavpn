package http

import "github.com/rtis-emc2/megavpn/internal/domain"

type serviceCatalogPreset struct {
	Key         string         `json:"key"`
	Label       string         `json:"label"`
	Description string         `json:"description"`
	Recommended bool           `json:"recommended"`
	Draft       map[string]any `json:"draft"`
}

type serviceCatalogProfile struct {
	DisplayName     string                 `json:"display_name"`
	Label           string                 `json:"label"`
	RuntimeCode     string                 `json:"runtime_code"`
	Runtime         string                 `json:"runtime"`
	ServiceKind     string                 `json:"service_kind"`
	CompanionTo     []string               `json:"companion_to,omitempty"`
	CompanionNote   string                 `json:"companion_note,omitempty"`
	Description     string                 `json:"description"`
	UnitPattern     string                 `json:"unit_pattern"`
	PathPattern     string                 `json:"path_pattern"`
	NameTemplate    string                 `json:"name_template"`
	SlugTemplate    string                 `json:"slug_template"`
	EndpointHint    string                 `json:"endpoint_hint"`
	PlatformNotes   []string               `json:"platform_notes,omitempty"`
	Recommendations []string               `json:"recommendations"`
	Presets         []serviceCatalogPreset `json:"presets"`
}

func serviceCatalogProfiles() map[string]serviceCatalogProfile {
	return map[string]serviceCatalogProfile{
		"xray-core": {
			DisplayName:  "Xray VLESS / Reality",
			Label:        "Xray VLESS / Reality",
			RuntimeCode:  "xray-core",
			Runtime:      "xray-core runtime",
			ServiceKind:  "transport",
			Description:  "Основной modern-transport сервис для персональных VPN/anti-censorship профилей. Это продуктовый сервис, работающий поверх runtime xray-core.",
			UnitPattern:  "megavpn-xray-<slug>",
			PathPattern:  "/usr/local/etc/xray/<slug>.json",
			NameTemplate: "edge-xray-reality",
			SlugTemplate: "edge-xray-reality",
			EndpointHint: "vpn.example.com",
			Recommendations: []string{
				"Для первого production-среза держать порт 443 и валидный SNI/Server Name.",
				"Для прямого client-facing профиля использовать Reality c short-id и chrome fingerprint.",
				"Nginx/gRPC и HTTP edge-сценарии делать отдельным backend-profile без Reality на runtime-порту.",
				"Оставлять manual JSON override только для нестандартных transport-экспериментов.",
			},
			Presets: []serviceCatalogPreset{
				{Key: "reality_tcp", Label: "Reality TCP", Description: "Рекомендуемый baseline для большинства egress-нод.", Recommended: true, Draft: map[string]any{"endpoint_port": 443, "xray_network": "tcp", "xray_dest": "www.cloudflare.com:443", "xray_fingerprint": "chrome", "config_mode": "0640"}},
				{Key: "nginx_grpc_backend", Label: "Nginx gRPC Backend", Description: "Backend-профиль для связки Xray + Nginx gRPC edge. Публичный TLS терминируется на Nginx.", Draft: map[string]any{"endpoint_port": 7443, "xray_security": "none", "xray_network": "grpc", "xray_service_name": "vless-grpc", "config_mode": "0640"}},
				{Key: "nginx_ws_backend", Label: "Nginx HTTP/WebSocket Backend", Description: "Backend-профиль для связки Xray + Nginx HTTP/WebSocket edge.", Draft: map[string]any{"endpoint_port": 7080, "xray_security": "none", "xray_network": "ws", "xray_path": "/ws", "config_mode": "0640"}},
			},
		},
		"openvpn": {
			DisplayName:  "OpenVPN",
			Label:        "OpenVPN",
			RuntimeCode:  "openvpn",
			Runtime:      "openvpn server runtime",
			ServiceKind:  "transport",
			Description:  "Классический VPN-сервис для широкой клиентской совместимости и управляемого PKI lifecycle.",
			UnitPattern:  "openvpn-server@<slug>",
			PathPattern:  "/etc/openvpn/server/<slug>.conf",
			NameTemplate: "edge-openvpn",
			SlugTemplate: "edge-openvpn",
			EndpointHint: "ovpn.example.com",
			PlatformNotes: []string{
				"Control Plane выступает CA center для OpenVPN platform PKI и показывает active roots в Settings.",
			},
			Recommendations: []string{
				"Для массовых клиентов безопасный baseline: TCP/11994, platform PKI, AES-GCM.",
				"UDP имеет смысл только там, где сеть стабильна и нет жестких ограничений firewall.",
				"Не смешивать ручной PKI и platform-managed PKI в одном instance.",
			},
			Presets: []serviceCatalogPreset{
				{Key: "tcp_11994", Label: "TCP 11994", Description: "Рекомендуемый baseline для совместимости и отдельного TCP-порта под OpenVPN.", Recommended: true, Draft: map[string]any{"endpoint_port": 11994, "ovpn_proto": "tcp", "ovpn_dev": "tun", "address_pool_mode": "auto", "config_mode": "0644", "ovpn_pki_profile": "default"}},
				{Key: "udp_1194", Label: "UDP 1194", Description: "Более классический профиль там, где throughput важнее camouflage.", Draft: map[string]any{"endpoint_port": 1194, "ovpn_proto": "udp", "ovpn_dev": "tun", "address_pool_mode": "auto", "config_mode": "0644", "ovpn_pki_profile": "default"}},
			},
		},
		"wireguard": {
			DisplayName:  "WireGuard",
			Label:        "WireGuard",
			RuntimeCode:  "wireguard",
			Runtime:      "wg-quick / wireguard-tools",
			ServiceKind:  "transport",
			Description:  "Высокопроизводительный VPN для современных клиентов и минимальной конфигурационной поверхности.",
			UnitPattern:  "wg-quick@<slug>",
			PathPattern:  "/etc/wireguard/<slug>.conf",
			NameTemplate: "edge-wireguard",
			SlugTemplate: "edge-wireguard",
			EndpointHint: "wg.example.com",
			Recommendations: []string{
				"Держать отдельную /24 сеть на каждый instance и не переиспользовать address pool.",
				"Для удаленных клиентов рекомендуемый keepalive 25 секунд.",
				"Endpoint port обычно 51820, если нет требований camouflage.",
			},
			Presets: []serviceCatalogPreset{
				{Key: "roadwarrior", Label: "Road Warrior", Description: "Рекомендуемый full-tunnel профиль для клиентов.", Recommended: true, Draft: map[string]any{"endpoint_port": 51820, "address_pool_mode": "auto", "wg_client_allowed_ips": "0.0.0.0/0, ::/0", "wg_client_dns": "1.1.1.1, 1.0.0.1", "wg_keepalive": 25, "config_mode": "0600"}},
				{Key: "split_tunnel", Label: "Split Tunnel", Description: "Профиль для корпоративных и частичных маршрутов.", Draft: map[string]any{"endpoint_port": 51820, "address_pool_mode": "auto", "wg_client_allowed_ips": "10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16", "wg_client_dns": "1.1.1.1, 1.0.0.1", "wg_keepalive": 25, "config_mode": "0600"}},
			},
		},
		"ipsec": {
			DisplayName:   "IPsec / IKEv2",
			Label:         "IPsec / IKEv2",
			RuntimeCode:   "ipsec",
			Runtime:       "strongSwan / ipsec",
			ServiceKind:   "transport",
			CompanionTo:   []string{"xl2tpd"},
			CompanionNote: "Если нужен L2TP remote-access, сразу после IPsec создавай companion instance XL2TPD с тем же endpoint host.",
			Description:   "Базовый IPsec/IKE слой. Используется как отдельный managed service и как база для L2TP companion flow.",
			UnitPattern:   "strongswan-starter",
			PathPattern:   "/etc/ipsec.conf",
			NameTemplate:  "edge-ipsec",
			SlugTemplate:  "edge-ipsec",
			EndpointHint:  "ipsec.example.com",
			Recommendations: []string{
				"Использовать как transport/security layer, а L2TP держать отдельным companion instance.",
				"PSK-профиль оставить только как bootstrap baseline; дальше переводить в более строгие схемы.",
				"Секреты хранить отдельно и не дублировать руками в нескольких инстансах.",
			},
			Presets: []serviceCatalogPreset{
				{Key: "ikev2_psk", Label: "IKEv2 PSK", Description: "Рекомендуемый baseline, пока в продукте нет сертификатного IKEv2 flow.", Recommended: true, Draft: map[string]any{"endpoint_port": 1701, "ipsec_left": "%defaultroute", "ipsec_right": "%any", "ipsec_ike": "aes256-sha1-modp1024", "ipsec_esp": "aes256-sha1", "config_mode": "0644", "ipsec_secrets_mode": "0600"}},
			},
		},
		"xl2tpd": {
			DisplayName:   "L2TP Access",
			Label:         "L2TP Access",
			RuntimeCode:   "xl2tpd",
			Runtime:       "xl2tpd + pppd",
			ServiceKind:   "companion",
			CompanionTo:   []string{"ipsec"},
			CompanionNote: "Используй вместе с companion instance IPsec / IKEv2 на том же endpoint host.",
			Description:   "Companion-сервис к IPsec для L2TP remote-access профиля. Сам по себе не должен восприниматься как отдельный secure transport.",
			UnitPattern:   "xl2tpd",
			PathPattern:   "/etc/xl2tpd/xl2tpd.conf",
			NameTemplate:  "edge-l2tp-access",
			SlugTemplate:  "edge-l2tp-access",
			EndpointHint:  "l2tp.example.com",
			Recommendations: []string{
				"Деплоить вместе с companion IPsec instance.",
				"Явно задавать pool, DNS и default credentials bootstrap-пути.",
				"Не использовать без сопутствующего IPsec transport.",
			},
			Presets: []serviceCatalogPreset{
				{Key: "remote_access", Label: "Remote Access", Description: "Рекомендуемый baseline для L2TP поверх IPsec.", Recommended: true, Draft: map[string]any{"endpoint_port": 1701, "address_pool_mode": "auto", "xl2tpd_dns_primary": "1.1.1.1", "xl2tpd_dns_secondary": "1.0.0.1", "config_mode": "0644"}},
			},
		},
		"http_proxy": {
			DisplayName:  "HTTP Proxy / Squid",
			Label:        "HTTP Proxy / Squid",
			RuntimeCode:  "http_proxy",
			Runtime:      "squid runtime",
			ServiceKind:  "proxy",
			Description:  "Классический authenticated HTTP proxy. Должен быть отдельным изолированным instance на своем config/unit.",
			UnitPattern:  "megavpn-http-proxy-<slug>",
			PathPattern:  "/etc/squid/<slug>.conf",
			NameTemplate: "edge-http-proxy",
			SlugTemplate: "edge-http-proxy",
			EndpointHint: "proxy.example.com",
			Recommendations: []string{
				"По умолчанию только authenticated profile; open proxy не делать preset-ом.",
				"Для каждого instance держать отдельные config, passwd, pid и log paths.",
				"Visible hostname и auth realm задавать осмысленно, чтобы упростить поддержку.",
			},
			Presets: []serviceCatalogPreset{
				{Key: "authenticated_edge", Label: "Authenticated Edge", Description: "Рекомендуемый production baseline с обязательной аутентификацией.", Recommended: true, Draft: map[string]any{"endpoint_port": 3128, "proxy_auth_realm": "RTIS MegaVPN HTTP Proxy", "proxy_auth_helper_path": "/usr/lib/squid/basic_ncsa_auth", "config_mode": "0644"}},
				{Key: "authenticated_alt_8080", Label: "Authenticated 8080", Description: "Альтернативный профиль для окружений, где 3128 конфликтует.", Draft: map[string]any{"endpoint_port": 8080, "proxy_auth_realm": "RTIS MegaVPN HTTP Proxy", "proxy_auth_helper_path": "/usr/lib/squid/basic_ncsa_auth", "config_mode": "0644"}},
			},
		},
		"mtproto": {
			DisplayName:  "MTProto",
			Label:        "MTProto",
			RuntimeCode:  "xray-core",
			Runtime:      "xray-core runtime",
			ServiceKind:  "proxy",
			Description:  "Telegram-oriented proxy profile. Это отдельный продуктовый сервис, но его runtime движок тоже xray-core.",
			UnitPattern:  "megavpn-mtproto-<slug>",
			PathPattern:  "/usr/local/etc/xray/<slug>.json",
			NameTemplate: "edge-mtproto",
			SlugTemplate: "edge-mtproto",
			EndpointHint: "tg.example.com",
			Recommendations: []string{
				"Держать отдельный unit/config на каждый instance, не смешивать с VLESS instance.",
				"Стандартный production baseline: port 443, отдельный secret per access rotation.",
				"Использовать только как специализированный transport для Telegram-кейса.",
			},
			Presets: []serviceCatalogPreset{
				{Key: "telegram_443", Label: "Telegram 443", Description: "Рекомендуемый baseline для основного MTProto traffic.", Recommended: true, Draft: map[string]any{"endpoint_port": 443, "mtproto_listen": "0.0.0.0", "config_mode": "0640"}},
				{Key: "telegram_8443", Label: "Telegram 8443", Description: "Альтернативный профиль для нод, где 443 уже занят.", Draft: map[string]any{"endpoint_port": 8443, "mtproto_listen": "0.0.0.0", "config_mode": "0640"}},
			},
		},
		"shadowsocks": {
			DisplayName:  "Shadowsocks",
			Label:        "Shadowsocks",
			RuntimeCode:  "shadowsocks",
			Runtime:      "shadowsocks-libev runtime",
			ServiceKind:  "proxy",
			Description:  "Легковесный proxy/VPN-like сервис для клиентских приложений и быстрых персональных доступов.",
			UnitPattern:  "megavpn-shadowsocks-<slug>",
			PathPattern:  "/etc/shadowsocks-libev/<slug>.json",
			NameTemplate: "edge-shadowsocks",
			SlugTemplate: "edge-shadowsocks",
			EndpointHint: "ss.example.com",
			Recommendations: []string{
				"Стартовый baseline: chacha20-ietf-poly1305 и tcp_and_udp.",
				"Держать отдельные server/access secrets и не переиспользовать их между профилями.",
				"Port base планировать так, чтобы access rotation не конфликтовал с соседними сервисами.",
			},
			Presets: []serviceCatalogPreset{
				{Key: "chacha_full", Label: "Chacha Full", Description: "Рекомендуемый universal baseline.", Recommended: true, Draft: map[string]any{"endpoint_port": 8388, "ss_method": "chacha20-ietf-poly1305", "ss_mode": "tcp_and_udp", "ss_timeout": 300, "config_mode": "0640"}},
				{Key: "aes_tcp", Label: "AES TCP Only", Description: "Консервативный профиль для TCP-only клиентов.", Draft: map[string]any{"endpoint_port": 8388, "ss_method": "aes-256-gcm", "ss_mode": "tcp_only", "ss_timeout": 300, "config_mode": "0640"}},
			},
		},
		"nginx": {
			DisplayName:  "Nginx Edge",
			Label:        "Nginx Edge",
			RuntimeCode:  "nginx",
			Runtime:      "nginx runtime",
			ServiceKind:  "edge",
			Description:  "Edge/service front для reverse-proxy и static publishing. Это не VPN transport, а обслуживающий ingress layer.",
			UnitPattern:  "nginx",
			PathPattern:  "/etc/nginx/conf.d/megavpn-<slug>.conf",
			NameTemplate: "edge-nginx",
			SlugTemplate: "edge-nginx",
			EndpointHint: "edge.example.com",
			Recommendations: []string{
				"Использовать как reverse-proxy front для UI/API или как static edge.",
				"Для Xray gRPC/HTTP edge держать отдельный Nginx ingress и отдельный backend-port Xray.",
				"TLS-сертификаты и upstream path держать явными, без магических defaults.",
				"Отдельный ingress слой не должен смешиваться с транспортными сервисами.",
			},
			Presets: []serviceCatalogPreset{
				{Key: "reverse_proxy", Label: "Reverse Proxy", Description: "Рекомендуемый edge profile для API/UI.", Recommended: true, Draft: map[string]any{"endpoint_port": 8080, "nginx_mode": "reverse_proxy", "nginx_index_files": "index.html index.htm", "config_mode": "0644"}},
				{Key: "grpc_edge", Label: "Xray gRPC Edge", Description: "TLS edge для backend Xray gRPC. Требует cert/key и grpc upstream.", Draft: map[string]any{"endpoint_port": 443, "nginx_mode": "grpc_proxy", "nginx_location_path": "/vless-grpc", "nginx_upstream_url": "grpc://127.0.0.1:7443", "nginx_tls_enabled": "true", "nginx_http_to_https_redirect": "true", "config_mode": "0644"}},
				{Key: "ws_edge", Label: "Xray HTTP/WebSocket Edge", Description: "TLS reverse-proxy для backend Xray HTTP/WebSocket transport.", Draft: map[string]any{"endpoint_port": 443, "nginx_mode": "reverse_proxy", "nginx_upstream_url": "http://127.0.0.1:7080", "nginx_tls_enabled": "true", "nginx_http_to_https_redirect": "true", "config_mode": "0644"}},
				{Key: "static_site", Label: "Static Site", Description: "Профиль для статической публикации контента.", Draft: map[string]any{"endpoint_port": 8080, "nginx_mode": "static", "nginx_index_files": "index.html index.htm", "config_mode": "0644"}},
			},
		},
	}
}

func enrichServiceDefinitions(defs []domain.ServiceDefinition) []response {
	profiles := serviceCatalogProfiles()
	out := make([]response, 0, len(defs))
	for _, def := range defs {
		item := response{
			"id":                 def.ID,
			"code":               def.Code,
			"name":               def.Name,
			"category":           def.Category,
			"tier":               def.Tier,
			"supports_accounts":  def.SupportsAccounts,
			"supports_artifacts": def.SupportsArtifacts,
			"supports_install":   def.SupportsInstall,
			"supports_instances": def.SupportsInstances,
			"enabled":            def.Enabled,
			"created_at":         def.CreatedAt,
		}
		if profile, ok := profiles[def.Code]; ok {
			item["display_name"] = profile.DisplayName
			item["label"] = profile.Label
			item["runtime_code"] = profile.RuntimeCode
			item["runtime"] = profile.Runtime
			item["service_kind"] = profile.ServiceKind
			item["description"] = profile.Description
			item["unit_pattern"] = profile.UnitPattern
			item["path_pattern"] = profile.PathPattern
			item["name_template"] = profile.NameTemplate
			item["slug_template"] = profile.SlugTemplate
			item["endpoint_hint"] = profile.EndpointHint
			item["platform_notes"] = profile.PlatformNotes
			item["recommendations"] = profile.Recommendations
			item["presets"] = profile.Presets
			if len(profile.CompanionTo) > 0 {
				item["companion_to"] = profile.CompanionTo
			}
			if profile.CompanionNote != "" {
				item["companion_note"] = profile.CompanionNote
			}
		}
		out = append(out, item)
	}
	return out
}
