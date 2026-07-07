package http

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	nethttp "net/http"

	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/service/driver"
)

type servicePackComponent = domain.ServicePackComponent
type servicePackDefinition = domain.ServicePackDefinition

var errDefaultPlatformLeafCertificateNotFound = errors.New("default platform leaf certificate not found")

type servicePackCatalogStore interface {
	EnsureDefaultServicePacks(context.Context, []domain.ServicePackDefinition) error
	ListServicePacks(context.Context) ([]domain.ServicePackDefinition, error)
	GetServicePack(context.Context, string) (domain.ServicePackDefinition, error)
}

type servicePackCatalogManageStore interface {
	servicePackCatalogStore
	ListServicePackCatalog(context.Context) ([]domain.ServicePackDefinition, error)
	UpsertServicePack(context.Context, domain.ServicePackDefinition) (domain.ServicePackDefinition, error)
	SetServicePackStatus(context.Context, string, string) (domain.ServicePackDefinition, error)
}

type createServicePackRequest struct {
	NodeID              string                              `json:"node_id"`
	BaseName            string                              `json:"base_name"`
	EndpointHost        string                              `json:"endpoint_host"`
	CertificateID       string                              `json:"certificate_id"`
	OpenVPNPKIProfile   string                              `json:"openvpn_pki_profile"`
	XrayEgressMode      string                              `json:"xray_egress_mode"`
	XrayEgressNodeID    string                              `json:"xray_egress_node_id"`
	CamouflagePath      string                              `json:"camouflage_path"`
	FallbackUpstreamURL string                              `json:"fallback_upstream_url"`
	FallbackHostHeader  string                              `json:"fallback_host_header"`
	FallbackSNI         string                              `json:"fallback_sni"`
	Components          []createServicePackComponentRequest `json:"components"`
	AutoInstallRuntime  *bool                               `json:"auto_install_runtime"`
}

type createServicePackComponentRequest struct {
	Index             *int   `json:"index"`
	EndpointPort      int    `json:"endpoint_port"`
	OpenVPNPKIProfile string `json:"openvpn_pki_profile"`
}

type selectedServicePackComponentHTTP struct {
	Component         domain.ServicePackComponent
	OpenVPNPKIProfile string
}

func defaultVLESSOutboundGroups() []any {
	return []any{
		map[string]any{
			"key":          "default",
			"label":        "Default access",
			"access_mode":  "instance_default",
			"egress_mode":  "default",
			"outbound_tag": "direct",
		},
	}
}

func openVPNFullTunnelServerExtraLines() []string {
	return []string{
		`push "redirect-gateway def1 bypass-dhcp"`,
		`push "dhcp-option DNS 1.1.1.1"`,
		`push "dhcp-option DNS 1.0.0.1"`,
	}
}

func servicePackDefinitions() []servicePackDefinition {
	return []servicePackDefinition{
		{
			Key:                  "default_access_suite",
			Label:                "Default Remote Access Suite",
			Description:          "Создает базовый multi-protocol remote-access набор: VLESS + OpenVPN TCP pair, отдельный VLESS, OpenVPN UDP, Shadowsocks и WireGuard.",
			BaseNameTemplate:     "edge-access",
			EndpointHint:         "access.example.com",
			RequiresEndpointHost: true,
			PlatformNotes: []string{
				"Template не хранит runtime secrets: Reality keys, WireGuard private key и Shadowsocks password генерируются при создании revision/apply и сохраняются как secret refs.",
				"Компоненты используют разные listen ports, чтобы pack можно было создать на одной ноде и одном endpoint host.",
				"OpenVPN components используют выбранный Service PKI profile; один profile можно использовать на нескольких нодах для общего trust-root.",
			},
			Recommendations: []string{
				"Перед production rollout проверь DNS, firewall/NAT и conflict-free ports на выбранной ноде.",
				"WireGuard/OpenVPN/L2TP address pools выделяются автоматически из Address Pools catalog.",
				"Для публичного VLESS на 443 используй корректный endpoint host/SNI; второй VLESS вынесен на 8443, чтобы не конфликтовать с основным transport.",
			},
			Components: []servicePackComponent{
				{Label: "VLESS TCP Edge", Description: "VLESS/Reality component для пары VLESS + OpenVPN TCP.", ServiceCode: "xray-core", PresetKey: "reality_tcp", NameSuffix: "vless-tcp-edge", SlugSuffix: "vless-tcp-edge", EndpointPort: 443, RequiresEndpointHost: true, Spec: map[string]any{"service_profile": "reality_tcp", "security": "reality", "network": "tcp", "dest": "www.cloudflare.com:443", "fingerprint": "chrome", "auto_generate_reality_keys": true, "egress_mode": "auto", "default_vless_group": "default", "vless_groups": defaultVLESSOutboundGroups(), "traffic_accounting_enabled": true, "config_mode": "0640"}},
				{Label: "OpenVPN TCP Companion", Description: "OpenVPN TCP component для пары VLESS + OpenVPN TCP.", ServiceCode: "openvpn", PresetKey: "tcp_11994", NameSuffix: "openvpn-tcp", SlugSuffix: "openvpn-tcp", EndpointPort: 11994, RequiresEndpointHost: true, Spec: map[string]any{"service_profile": "tcp_11994", "pki_scope": "platform", "pki_profile": "default", "proto": "tcp", "dev": "tun", "address_pool_mode": "auto", "server_extra_lines": openVPNFullTunnelServerExtraLines(), "traffic_accounting_enabled": true, "config_mode": "0644"}},
				{Label: "VLESS Standalone", Description: "Отдельный VLESS/Reality instance на альтернативном TCP-порту.", ServiceCode: "xray-core", PresetKey: "reality_tcp", NameSuffix: "vless", SlugSuffix: "vless", EndpointPort: 8443, RequiresEndpointHost: true, Spec: map[string]any{"service_profile": "reality_tcp", "security": "reality", "network": "tcp", "dest": "www.cloudflare.com:443", "fingerprint": "chrome", "auto_generate_reality_keys": true, "egress_mode": "auto", "default_vless_group": "default", "vless_groups": defaultVLESSOutboundGroups(), "traffic_accounting_enabled": true, "config_mode": "0640"}},
				{Label: "OpenVPN UDP", Description: "Классический OpenVPN UDP baseline.", ServiceCode: "openvpn", PresetKey: "udp_1194", NameSuffix: "openvpn-udp", SlugSuffix: "openvpn-udp", EndpointPort: 1194, RequiresEndpointHost: true, Spec: map[string]any{"service_profile": "udp_1194", "pki_scope": "platform", "pki_profile": "default", "proto": "udp", "dev": "tun", "address_pool_mode": "auto", "server_extra_lines": openVPNFullTunnelServerExtraLines(), "traffic_accounting_enabled": true, "config_mode": "0644"}},
				{Label: "Shadowsocks", Description: "Standalone Shadowsocks chacha20-ietf-poly1305 baseline.", ServiceCode: "shadowsocks", PresetKey: "chacha_full", NameSuffix: "shadowsocks", SlugSuffix: "shadowsocks", EndpointPort: 8388, RequiresEndpointHost: true, Spec: map[string]any{"service_profile": "chacha_full", "method": "chacha20-ietf-poly1305", "mode": "tcp_and_udp", "timeout": 300, "auto_generate_server_password": true, "config_mode": "0640"}},
				{Label: "WireGuard", Description: "Standalone WireGuard road-warrior baseline.", ServiceCode: "wireguard", PresetKey: "roadwarrior", NameSuffix: "wireguard", SlugSuffix: "wireguard", EndpointPort: 51820, RequiresEndpointHost: true, Spec: map[string]any{"service_profile": "roadwarrior", "address_pool_mode": "auto", "client_allowed_ips": "0.0.0.0/0, ::/0", "client_dns": "1.1.1.1, 1.0.0.1", "persistent_keepalive": 25, "auto_generate_server_key": true, "traffic_accounting_enabled": true, "config_mode": "0600"}},
			},
		},
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
				{Label: "L2TP Access", Description: "Companion access service на том же endpoint host.", ServiceCode: "xl2tpd", PresetKey: "remote_access", NameSuffix: "l2tp-access", SlugSuffix: "l2tp-access", EndpointPort: 1701, RequiresEndpointHost: true, Spec: map[string]any{"service_profile": "remote_access", "address_pool_mode": "auto", "ppp_dns_primary": "1.1.1.1", "ppp_dns_secondary": "1.0.0.1", "config_mode": "0644"}},
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
				{Label: "Xray VLESS / Reality", Description: "Прямой публичный transport без edge companion.", ServiceCode: "xray-core", PresetKey: "reality_tcp", NameSuffix: "xray-reality", SlugSuffix: "xray-reality", EndpointPort: 443, RequiresEndpointHost: true, Spec: map[string]any{"service_profile": "reality_tcp", "security": "reality", "network": "tcp", "dest": "www.cloudflare.com:443", "fingerprint": "chrome", "egress_mode": "auto", "default_vless_group": "default", "vless_groups": defaultVLESSOutboundGroups(), "traffic_accounting_enabled": true, "config_mode": "0640"}},
			},
		},
		{
			Key:                  "xray_nginx_grpc_edge",
			Label:                "Xray + Nginx gRPC Edge",
			Description:          "Создает backend Xray gRPC instance и отдельный Nginx TLS edge с website fallback для публичного endpoint.",
			BaseNameTemplate:     "edge-xray-grpc",
			EndpointHint:         "edge.example.com",
			RequiresEndpointHost: true,
			PlatformNotes: []string{
				"Nginx отвечает за публичный TLS edge и маршрутизацию, Xray слушает отдельный backend-port без Reality.",
				"Прямой заход на корень endpoint проксируется на fallback website, VPN-транспорт живет только на gRPC location.",
				"Для боевого контура задай реальный cert/key path или переведи edge на внешний certificate management.",
			},
			Recommendations: []string{
				"Используй, когда нужен app-like gRPC transport под публичным TLS edge и нормальное поведение домена при прямом заходе.",
				"Не смешивай этот пакет с direct-Reality на том же 443 без явного ingress-плана.",
			},
			Components: []servicePackComponent{
				{Label: "Xray gRPC Backend", Description: "Runtime backend для Nginx gRPC edge.", ServiceCode: "xray-core", PresetKey: "nginx_grpc_backend", NameSuffix: "xray-grpc", SlugSuffix: "xray-grpc", EndpointPort: 7443, RequiresEndpointHost: true, Spec: map[string]any{"service_profile": "nginx_grpc_backend", "listen": "127.0.0.1", "security": "none", "network": "grpc", "service_name": "vless-grpc", "public_security": "tls", "public_network": "grpc", "public_service_name": "vless-grpc", "public_port": 443, "egress_mode": "auto", "default_vless_group": "default", "vless_groups": defaultVLESSOutboundGroups(), "traffic_accounting_enabled": true, "config_mode": "0640"}},
				{Label: "Nginx gRPC Edge", Description: "Публичный TLS ingress для backend Xray gRPC с fallback на реальный сайт.", ServiceCode: "nginx", PresetKey: "grpc_edge", NameSuffix: "nginx-grpc-edge", SlugSuffix: "nginx-grpc-edge", EndpointPort: 443, RequiresEndpointHost: true, Spec: map[string]any{"service_profile": "grpc_edge", "mode": "grpc_proxy", "location_path": "/vless-grpc", "server_name": "{{endpoint_host}}", "upstream_url": "grpc://127.0.0.1:7443", "fallback_upstream_url": "{{fallback_upstream_url}}", "fallback_host_header": "{{fallback_host_header}}", "fallback_sni": "{{fallback_sni}}", "tls_enabled": true, "http_to_https_redirect": true, "tls_cert_path": "/etc/letsencrypt/live/{{endpoint_host}}/fullchain.pem", "tls_key_path": "/etc/letsencrypt/live/{{endpoint_host}}/privkey.pem", "config_mode": "0644"}},
			},
		},
		{
			Key:                  "xray_nginx_http_edge",
			Label:                "Xray VLESS WebSocket Camouflage",
			Description:          "Создает backend Xray VLESS/WebSocket instance и Nginx TLS edge с fallback на реальный сайт.",
			BaseNameTemplate:     "edge-xray-http",
			EndpointHint:         "enter.example.com",
			RequiresEndpointHost: true,
			PlatformNotes: []string{
				"Это рекомендуемый public ingress-профиль для домена с DNS round-robin.",
				"Прямой заход на корень endpoint проксируется на fallback website, VPN-транспорт живет только на скрытом WebSocket path.",
			},
			Recommendations: []string{
				"Используй нейтральный host вроде `enter.example.com`; все ingress-ноды должны иметь одинаковый certificate/SNI.",
				"При необходимости path можно потом поменять в revision/spec и на Xray, и на Nginx edge. Не используй очевидный `/ws` в production.",
			},
			Components: []servicePackComponent{
				{Label: "Xray VLESS/WebSocket Backend", Description: "Runtime backend для Nginx WebSocket edge.", ServiceCode: "xray-core", PresetKey: "nginx_ws_backend", NameSuffix: "xray-ws", SlugSuffix: "xray-ws", EndpointPort: 7080, RequiresEndpointHost: true, Spec: map[string]any{"service_profile": "nginx_ws_backend", "listen": "127.0.0.1", "security": "none", "network": "ws", "path": "{{camouflage_path}}", "public_security": "tls", "public_network": "ws", "public_path": "{{camouflage_path}}", "public_host_header": "{{endpoint_host}}", "public_port": 443, "egress_mode": "auto", "default_vless_group": "default", "vless_groups": defaultVLESSOutboundGroups(), "traffic_accounting_enabled": true, "config_mode": "0640"}},
				{Label: "Nginx WebSocket Camouflage Edge", Description: "Публичный TLS reverse-proxy для Xray ws path и fallback website для обычного web-трафика.", ServiceCode: "nginx", PresetKey: "ws_camouflage_edge", NameSuffix: "nginx-camouflage-edge", SlugSuffix: "nginx-camouflage-edge", EndpointPort: 443, RequiresEndpointHost: true, Spec: map[string]any{"service_profile": "ws_camouflage_edge", "mode": "reverse_proxy", "location_path": "{{camouflage_path}}", "server_name": "{{endpoint_host}}", "upstream_url": "http://127.0.0.1:7080", "fallback_upstream_url": "{{fallback_upstream_url}}", "fallback_host_header": "{{fallback_host_header}}", "fallback_sni": "{{fallback_sni}}", "tls_enabled": true, "http_to_https_redirect": true, "tls_cert_path": "/etc/letsencrypt/live/{{endpoint_host}}/fullchain.pem", "tls_key_path": "/etc/letsencrypt/live/{{endpoint_host}}/privkey.pem", "websocket_upgrade": true, "config_mode": "0644"}},
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
				"OpenVPN использует Service PKI root по выбранному pki_profile; default profile создается автоматически при apply, если root еще не создан.",
			},
			Recommendations: []string{
				"Рекомендуемый baseline, если нужен более совместимый TCP transport.",
			},
			Components: []servicePackComponent{
				{Label: "OpenVPN TCP", Description: "OpenVPN instance с platform-managed PKI.", ServiceCode: "openvpn", PresetKey: "tcp_11994", NameSuffix: "openvpn-tcp", SlugSuffix: "openvpn-tcp", EndpointPort: 11994, RequiresEndpointHost: true, Spec: map[string]any{"service_profile": "tcp_11994", "pki_scope": "platform", "pki_profile": "default", "proto": "tcp", "dev": "tun", "address_pool_mode": "auto", "server_extra_lines": openVPNFullTunnelServerExtraLines(), "traffic_accounting_enabled": true, "config_mode": "0644"}},
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
				"OpenVPN использует Service PKI root по выбранному pki_profile; default profile создается автоматически при apply, если root еще не создан.",
			},
			Recommendations: []string{
				"Подходит там, где throughput важнее camouflage и firewall-path стабильнее.",
			},
			Components: []servicePackComponent{
				{Label: "OpenVPN UDP", Description: "OpenVPN instance с platform-managed PKI.", ServiceCode: "openvpn", PresetKey: "udp_1194", NameSuffix: "openvpn-udp", SlugSuffix: "openvpn-udp", EndpointPort: 1194, RequiresEndpointHost: true, Spec: map[string]any{"service_profile": "udp_1194", "pki_scope": "platform", "pki_profile": "default", "proto": "udp", "dev": "tun", "address_pool_mode": "auto", "server_extra_lines": openVPNFullTunnelServerExtraLines(), "traffic_accounting_enabled": true, "config_mode": "0644"}},
			},
		},
		{
			Key:                  "wireguard_roadwarrior",
			Label:                "WireGuard Road Warrior",
			Description:          "Создает standalone WireGuard instance для road-warrior клиентов с full-tunnel baseline.",
			BaseNameTemplate:     "edge-wireguard",
			EndpointHint:         "wg.example.com",
			RequiresEndpointHost: true,
			Recommendations: []string{
				"Это канонический стартовый пакет для modern full-tunnel VPN-клиентов.",
				"Network CIDR выделяется автоматически из Address Pools catalog.",
			},
			Components: []servicePackComponent{
				{Label: "WireGuard Road Warrior", Description: "Standalone WireGuard instance с managed peers.", ServiceCode: "wireguard", PresetKey: "roadwarrior", NameSuffix: "wireguard", SlugSuffix: "wireguard", EndpointPort: 51820, RequiresEndpointHost: true, Spec: map[string]any{"service_profile": "roadwarrior", "address_pool_mode": "auto", "client_allowed_ips": "0.0.0.0/0, ::/0", "client_dns": "1.1.1.1, 1.0.0.1", "persistent_keepalive": 25, "traffic_accounting_enabled": true, "config_mode": "0600"}},
			},
		},
		{
			Key:                  "http_proxy_authenticated",
			Label:                "HTTP Proxy Authenticated Edge",
			Description:          "Создает standalone authenticated HTTP proxy / Squid instance.",
			BaseNameTemplate:     "edge-http-proxy",
			EndpointHint:         "proxy.example.com",
			RequiresEndpointHost: true,
			Recommendations: []string{
				"Рекомендуемый baseline для managed authenticated HTTP proxy.",
				"Используй отдельный endpoint и не смешивай его runtime paths с соседними proxy/VPN сервисами.",
			},
			Components: []servicePackComponent{
				{Label: "HTTP Proxy Edge", Description: "Standalone authenticated HTTP proxy instance.", ServiceCode: "http_proxy", PresetKey: "authenticated_edge", NameSuffix: "http-proxy", SlugSuffix: "http-proxy", EndpointPort: 3128, RequiresEndpointHost: true, Spec: map[string]any{"service_profile": "authenticated_edge", "auth_realm": "RTIS MegaVPN HTTP Proxy", "auth_helper_path": "/usr/lib/squid/basic_ncsa_auth", "config_mode": "0644"}},
			},
		},
		{
			Key:                  "mtproto_telegram_443",
			Label:                "MTProto Telegram 443",
			Description:          "Создает standalone MTProto instance для Telegram-oriented proxy traffic.",
			BaseNameTemplate:     "edge-mtproto",
			EndpointHint:         "tg.example.com",
			RequiresEndpointHost: true,
			Recommendations: []string{
				"Рекомендуемый baseline для отдельного MTProto endpoint на 443.",
				"Не смешивай этот runtime instance с VLESS/Reality instance на том же slug/config path.",
			},
			Components: []servicePackComponent{
				{Label: "MTProto 443", Description: "Standalone MTProto instance на xray-core runtime.", ServiceCode: "mtproto", PresetKey: "telegram_443", NameSuffix: "mtproto", SlugSuffix: "mtproto", EndpointPort: 443, RequiresEndpointHost: true, Spec: map[string]any{"service_profile": "telegram_443", "mtproto_listen": "0.0.0.0", "config_mode": "0640"}},
			},
		},
		{
			Key:                  "shadowsocks_chacha",
			Label:                "Shadowsocks Chacha Full",
			Description:          "Создает standalone Shadowsocks instance с рекомендуемым chacha20 full baseline.",
			BaseNameTemplate:     "edge-shadowsocks",
			EndpointHint:         "ss.example.com",
			RequiresEndpointHost: true,
			Recommendations: []string{
				"Рекомендуемый universal baseline для персонального Shadowsocks-доступа.",
				"При необходимости later можешь перевести порт или cipher через revision/spec.",
			},
			Components: []servicePackComponent{
				{Label: "Shadowsocks Chacha", Description: "Standalone Shadowsocks instance с managed access secrets.", ServiceCode: "shadowsocks", PresetKey: "chacha_full", NameSuffix: "shadowsocks", SlugSuffix: "shadowsocks", EndpointPort: 8388, RequiresEndpointHost: true, Spec: map[string]any{"service_profile": "chacha_full", "method": "chacha20-ietf-poly1305", "mode": "tcp_and_udp", "timeout": 300, "config_mode": "0640"}},
			},
		},
	}
}

func DefaultServicePackDefinitions() []domain.ServicePackDefinition {
	defaults := servicePackDefinitions()
	out := make([]domain.ServicePackDefinition, len(defaults))
	for i, pack := range defaults {
		out[i] = pack
		out[i].PlatformNotes = append([]string(nil), pack.PlatformNotes...)
		out[i].Recommendations = append([]string(nil), pack.Recommendations...)
		out[i].Components = make([]domain.ServicePackComponent, len(pack.Components))
		for j, component := range pack.Components {
			out[i].Components[j] = component
			out[i].Components[j].Spec = cloneMapHTTP(component.Spec)
		}
	}
	return out
}

func findServicePack(key string) (servicePackDefinition, bool) {
	for _, pack := range DefaultServicePackDefinitions() {
		if pack.Key == key {
			return pack, true
		}
	}
	return servicePackDefinition{}, false
}

func (s *Server) listServicePacks(w nethttp.ResponseWriter, r *nethttp.Request) {
	if truthyQuery(r, "include_inactive") {
		authCtx, ok := authFromRequest(r)
		if !ok || !authContextHasPermission(authCtx, "settings.manage") {
			writeErr(w, 403, "settings.manage permission is required")
			return
		}
		catalog, ok := s.store.(servicePackCatalogManageStore)
		if !ok {
			writeErr(w, 501, "service pack catalog management is not supported")
			return
		}
		if err := catalog.EnsureDefaultServicePacks(r.Context(), DefaultServicePackDefinitions()); err != nil {
			if isServicePackCatalogUnavailable(err) {
				writeJSON(w, 200, DefaultServicePackDefinitions())
				return
			}
			writeErr(w, 500, err.Error())
			return
		}
		packs, err := catalog.ListServicePackCatalog(r.Context())
		if err != nil {
			if isServicePackCatalogUnavailable(err) {
				writeJSON(w, 200, DefaultServicePackDefinitions())
				return
			}
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, packs)
		return
	}
	packs, err := s.availableServicePacks(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, packs)
}

func (s *Server) upsertServicePack(w nethttp.ResponseWriter, r *nethttp.Request) {
	catalog, ok := s.store.(servicePackCatalogManageStore)
	if !ok {
		writeErr(w, 501, "service pack catalog management is not supported")
		return
	}
	key := strings.TrimSpace(r.PathValue("key"))
	var pack domain.ServicePackDefinition
	if !decode(r, &pack) {
		writeErr(w, 400, "invalid service pack payload")
		return
	}
	if key != "" {
		pack.Key = key
	}
	if strings.TrimSpace(pack.Source) == "" {
		pack.Source = "operator"
	}
	if strings.TrimSpace(pack.Status) == "" {
		pack.Status = "active"
	}
	updated, err := catalog.UpsertServicePack(r.Context(), pack)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	authCtx, ok := authFromRequest(r)
	if ok {
		_, _ = s.store.CreateAuditForUser(r.Context(), &authCtx.User.ID, "service_pack.upsert", "service_pack", nil, "service pack template upserted: "+updated.Key)
	}
	writeJSON(w, 200, updated)
}

func (s *Server) setServicePackStatus(status string) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		catalog, ok := s.store.(servicePackCatalogManageStore)
		if !ok {
			writeErr(w, 501, "service pack catalog management is not supported")
			return
		}
		key := strings.TrimSpace(r.PathValue("key"))
		updated, err := catalog.SetServicePackStatus(r.Context(), key, status)
		if errors.Is(err, domain.ErrServicePackNotFound) {
			writeErr(w, 404, "service pack not found")
			return
		}
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		authCtx, ok := authFromRequest(r)
		if ok {
			_, _ = s.store.CreateAuditForUser(r.Context(), &authCtx.User.ID, "service_pack."+status, "service_pack", nil, "service pack template "+status+": "+updated.Key)
		}
		writeJSON(w, 200, updated)
	}
}

func (s *Server) createServicePackInstances(w nethttp.ResponseWriter, r *nethttp.Request) {
	packKey := strings.TrimSpace(r.PathValue("key"))
	pack, err := s.getServicePack(r.Context(), packKey)
	if errors.Is(err, domain.ErrServicePackNotFound) {
		writeErr(w, 404, "service pack not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	var req createServicePackRequest
	if !decode(r, &req) || strings.TrimSpace(req.NodeID) == "" {
		writeErr(w, 400, "invalid service pack payload: node_id is required")
		return
	}
	req.BaseName = strings.TrimSpace(req.BaseName)
	req.EndpointHost = strings.TrimSpace(req.EndpointHost)
	req.CertificateID = strings.TrimSpace(req.CertificateID)
	req.OpenVPNPKIProfile = strings.TrimSpace(req.OpenVPNPKIProfile)
	req.XrayEgressNodeID = strings.TrimSpace(req.XrayEgressNodeID)
	req.CamouflagePath = strings.TrimSpace(req.CamouflagePath)
	req.FallbackUpstreamURL = strings.TrimSpace(req.FallbackUpstreamURL)
	req.FallbackHostHeader = strings.TrimSpace(req.FallbackHostHeader)
	req.FallbackSNI = strings.TrimSpace(req.FallbackSNI)
	req.XrayEgressMode, err = normalizeXrayEgressModeHTTP(req.XrayEgressMode, req.XrayEgressNodeID)
	if err != nil {
		writeErr(w, 400, "invalid service pack payload: "+err.Error())
		return
	}
	selectedComponents, err := selectServicePackComponentsHTTP(pack, req)
	if err != nil {
		writeErr(w, 400, "invalid service pack payload: "+err.Error())
		return
	}
	componentList := servicePackSelectedComponentListHTTP(selectedComponents)
	selectedPack := pack
	selectedPack.Components = componentList
	if servicePackUsesTrafficCamouflage(selectedPack) {
		if err := normalizeServicePackCamouflageRequestHTTP(&req, selectedPack); err != nil {
			writeErr(w, 400, "invalid service pack payload: "+err.Error())
			return
		}
	}
	if req.BaseName == "" {
		req.BaseName = pack.BaseNameTemplate
	}
	if servicePackComponentsRequireEndpointHost(componentList) && req.EndpointHost == "" {
		writeErr(w, 400, "invalid service pack payload: endpoint_host is required")
		return
	}
	autoInstallRuntime := true
	if req.AutoInstallRuntime != nil {
		autoInstallRuntime = *req.AutoInstallRuntime
	}
	missingRuntimes := map[string]bool{}
	if autoInstallRuntime {
		capabilities, err := s.store.ListNodeCapabilities(r.Context(), req.NodeID)
		if err != nil {
			writeErr(w, 409, "node runtime capability preflight failed: "+err.Error())
			return
		}
		for _, serviceCode := range missingServicePackRuntimeCapabilities(componentList, capabilities) {
			missingRuntimes[serviceCode] = true
		}
	}
	if req.CertificateID == "" && servicePackComponentsUseTLSEdgeCertificate(componentList) {
		certificateID, err := s.defaultPlatformLeafCertificateID(r.Context())
		if err != nil && !errors.Is(err, errDefaultPlatformLeafCertificateNotFound) {
			writeErr(w, 409, "default TLS edge certificate preflight failed: "+err.Error())
			return
		}
		req.CertificateID = certificateID
	}
	created := make([]domain.Instance, 0, len(componentList))
	existing := make([]domain.Instance, 0)
	applyNowInstanceIDs := []string{}
	dependentInstanceIDsByRuntime := map[string][]string{}
	existingInstances, err := s.store.ListInstances(r.Context())
	if err != nil {
		writeErr(w, 409, "instance identity preflight failed: "+err.Error())
		return
	}
	identitySet := newServicePackInstanceIdentitySet(existingInstances)
	for _, selected := range selectedComponents {
		component := selected.Component
		baseName, baseSlug := servicePackComponentBaseIdentity(req.BaseName, component)
		if existingInstance, ok := findExistingServicePackComponentInstance(existingInstances, req.NodeID, component, baseName, baseSlug, req.EndpointHost); ok {
			existing = append(existing, existingInstance)
			continue
		}
		var createdInstance domain.Instance
		var createErr error
		for attempt := 0; attempt < 16; attempt++ {
			name, slug := identitySet.reserve(req.NodeID, baseName, baseSlug)
			spec := interpolatePackSpec(cloneMapHTTP(component.Spec), map[string]string{
				"endpoint_host":         req.EndpointHost,
				"base_name":             req.BaseName,
				"component_slug":        slug,
				"camouflage_path":       req.CamouflagePath,
				"fallback_upstream_url": req.FallbackUpstreamURL,
				"fallback_host_header":  req.FallbackHostHeader,
				"fallback_sni":          req.FallbackSNI,
			})
			if req.CertificateID != "" && servicePackComponentUsesTLSEdgeCertificate(component) {
				spec["certificate_id"] = req.CertificateID
			}
			openVPNPKIProfile := req.OpenVPNPKIProfile
			if selected.OpenVPNPKIProfile != "" {
				openVPNPKIProfile = selected.OpenVPNPKIProfile
			}
			if driver.NormalizeCode(component.ServiceCode) == driver.OpenVPN && openVPNPKIProfile != "" {
				spec["pki_scope"] = "platform"
				spec["pki_profile"] = openVPNPKIProfile
			}
			applyXrayEgressOverrideHTTP(component, spec, req.XrayEgressMode, req.XrayEgressNodeID)
			componentService := strings.TrimSpace(component.ServiceCode)
			if strings.EqualFold(componentService, driver.XrayCore) || strings.EqualFold(componentService, "xray") {
				vlessGroups, err := s.availableVLESSGroupTemplates(r.Context())
				if err != nil {
					writeErr(w, 409, "vless group catalog preflight failed: "+err.Error())
					return
				}
				ensureVLESSDefaultGroup(spec, vlessGroups)
			}
			instance := domain.Instance{
				NodeID:       req.NodeID,
				ServiceCode:  component.ServiceCode,
				Name:         name,
				Slug:         slug,
				EndpointHost: req.EndpointHost,
				EndpointPort: component.EndpointPort,
				Spec:         spec,
			}
			createdInstance, createErr = s.store.CreateInstanceValidatedDraft(r.Context(), instance)
			if createErr == nil {
				break
			}
			if !isInstanceIdentityUniqueViolation(createErr) {
				break
			}
		}
		if createErr != nil {
			discarded := discardCreatedServicePackInstances(r.Context(), s.store, created)
			writeJSON(w, 409, response{
				"status":             "partial_failure",
				"error":              fmt.Sprintf("service pack component %q is not apply-ready: %v", servicePackComponentLabel(component), createErr),
				"component":          servicePackComponentLabel(component),
				"service_pack_key":   pack.Key,
				"created_instances":  created,
				"existing_instances": existing,
				"discarded_count":    discarded,
			})
			return
		}
		created = append(created, createdInstance)
		runtimeCode := runtimeCapabilityCodeHTTP(component.ServiceCode)
		if autoInstallRuntime && missingRuntimes[runtimeCode] {
			dependentInstanceIDsByRuntime[runtimeCode] = append(dependentInstanceIDsByRuntime[runtimeCode], createdInstance.ID)
		} else {
			applyNowInstanceIDs = append(applyNowInstanceIDs, createdInstance.ID)
		}
	}
	runtimeInstallJobs := []domain.Job{}
	runtimeCodes := make([]string, 0, len(dependentInstanceIDsByRuntime))
	for runtimeCode := range dependentInstanceIDsByRuntime {
		runtimeCodes = append(runtimeCodes, runtimeCode)
	}
	sort.Strings(runtimeCodes)
	for _, runtimeCode := range runtimeCodes {
		dependentIDs := dependentInstanceIDsByRuntime[runtimeCode]
		job, err := s.store.CreateNodeCapabilityInstallJobWithDependents(r.Context(), req.NodeID, runtimeCode, "", "", dependentIDs)
		if err != nil {
			writeJSON(w, 409, response{
				"status":               "runtime_install_failed",
				"error":                err.Error(),
				"service_pack_key":     pack.Key,
				"runtime_service_code": runtimeCode,
				"created_instances":    created,
				"existing_instances":   existing,
				"runtime_install_jobs": redactedJobs(runtimeInstallJobs),
			})
			return
		}
		runtimeInstallJobs = append(runtimeInstallJobs, job)
	}
	applyJobs := []domain.Job{}
	for _, instanceID := range applyNowInstanceIDs {
		job, err := s.store.UpdateInstanceStatus(r.Context(), instanceID, driver.OperationApply)
		if err != nil {
			writeJSON(w, 409, response{
				"status":               "apply_queue_failed",
				"error":                err.Error(),
				"service_pack_key":     pack.Key,
				"created_instances":    created,
				"existing_instances":   existing,
				"runtime_install_jobs": redactedJobs(runtimeInstallJobs),
				"apply_jobs":           redactedJobs(applyJobs),
			})
			return
		}
		applyJobs = append(applyJobs, job)
	}
	authCtx, ok := authFromRequest(r)
	if ok {
		if len(created) == 0 && len(existing) > 0 {
			_, _ = s.store.CreateAuditForUser(r.Context(), &authCtx.User.ID, "service_pack.reuse", "service_pack", nil, "service pack already existed: "+pack.Key)
		} else {
			_, _ = s.store.CreateAuditForUser(r.Context(), &authCtx.User.ID, "service_pack.create", "service_pack", nil, "service pack created: "+pack.Key)
		}
	}
	statusCode := 201
	statusText := "ok"
	if len(created) == 0 && len(existing) > 0 {
		statusCode = 200
		statusText = "already_exists"
	}
	writeJSON(w, statusCode, response{
		"status":               statusText,
		"service_pack_key":     pack.Key,
		"created_instances":    created,
		"existing_instances":   existing,
		"runtime_install_jobs": redactedJobs(runtimeInstallJobs),
		"apply_jobs":           redactedJobs(applyJobs),
	})
}

func discardCreatedServicePackInstances(ctx context.Context, store interface {
	DiscardInstanceDraft(context.Context, string) error
}, instances []domain.Instance) int {
	discarded := 0
	for i := len(instances) - 1; i >= 0; i-- {
		if strings.TrimSpace(instances[i].ID) == "" {
			continue
		}
		if err := store.DiscardInstanceDraft(ctx, instances[i].ID); err == nil {
			discarded++
		}
	}
	return discarded
}

func servicePackComponentLabel(component domain.ServicePackComponent) string {
	for _, value := range []string{component.Label, component.NameSuffix, component.SlugSuffix, component.ServiceCode} {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return "component"
}

func selectServicePackComponentsHTTP(pack domain.ServicePackDefinition, req createServicePackRequest) ([]selectedServicePackComponentHTTP, error) {
	if len(pack.Components) == 0 {
		return nil, fmt.Errorf("service pack has no components")
	}
	if req.Components == nil {
		out := make([]selectedServicePackComponentHTTP, 0, len(pack.Components))
		for _, component := range pack.Components {
			out = append(out, selectedServicePackComponentHTTP{Component: cloneServicePackComponentHTTP(component)})
		}
		return out, nil
	}
	if len(req.Components) == 0 {
		return nil, fmt.Errorf("select at least one service pack component")
	}
	seen := map[int]bool{}
	out := make([]selectedServicePackComponentHTTP, 0, len(req.Components))
	for _, selection := range req.Components {
		if selection.Index == nil {
			return nil, fmt.Errorf("component index is required")
		}
		index := *selection.Index
		if index < 0 || index >= len(pack.Components) {
			return nil, fmt.Errorf("component index %d is out of range", index)
		}
		if seen[index] {
			return nil, fmt.Errorf("component index %d is duplicated", index)
		}
		seen[index] = true
		component := cloneServicePackComponentHTTP(pack.Components[index])
		if selection.EndpointPort != 0 {
			if selection.EndpointPort < 1 || selection.EndpointPort > 65535 {
				return nil, fmt.Errorf("component %d endpoint_port must be between 1 and 65535", index)
			}
			component.EndpointPort = selection.EndpointPort
		}
		openVPNPKIProfile := strings.TrimSpace(selection.OpenVPNPKIProfile)
		if openVPNPKIProfile != "" && driver.NormalizeCode(component.ServiceCode) != driver.OpenVPN {
			return nil, fmt.Errorf("component %d openvpn_pki_profile can only be used with OpenVPN", index)
		}
		out = append(out, selectedServicePackComponentHTTP{
			Component:         component,
			OpenVPNPKIProfile: openVPNPKIProfile,
		})
	}
	return out, nil
}

func cloneServicePackComponentHTTP(component domain.ServicePackComponent) domain.ServicePackComponent {
	component.Spec = cloneMapHTTP(component.Spec)
	return component
}

func servicePackSelectedComponentListHTTP(selected []selectedServicePackComponentHTTP) []domain.ServicePackComponent {
	out := make([]domain.ServicePackComponent, 0, len(selected))
	for _, item := range selected {
		out = append(out, item.Component)
	}
	return out
}

func servicePackComponentsRequireEndpointHost(components []domain.ServicePackComponent) bool {
	for _, component := range components {
		if component.RequiresEndpointHost {
			return true
		}
	}
	return false
}

func normalizeXrayEgressModeHTTP(mode string, egressNodeID string) (string, error) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	egressNodeID = strings.TrimSpace(egressNodeID)
	if mode == "" {
		if egressNodeID != "" {
			return "egress_node", nil
		}
		return "", nil
	}
	switch mode {
	case "auto":
		return "auto", nil
	case "egress_node", "remote_egress", "remote_node", "node":
		if egressNodeID == "" {
			return "", errors.New("xray_egress_node_id is required when xray_egress_mode is egress_node")
		}
		return "egress_node", nil
	case "local_breakout", "local", "direct":
		return "local_breakout", nil
	default:
		return "", fmt.Errorf("unsupported xray_egress_mode %q", mode)
	}
}

func normalizeServicePackCamouflageRequestHTTP(req *createServicePackRequest, pack domain.ServicePackDefinition) error {
	if req == nil {
		return fmt.Errorf("camouflage request is required")
	}
	if servicePackUsesConfigurableCamouflagePathHTTP(pack) {
		if req.CamouflagePath == "" {
			req.CamouflagePath = defaultServicePackCamouflagePathHTTP(pack)
		}
		if err := validateServicePackCamouflagePathHTTP(req.CamouflagePath); err != nil {
			return err
		}
	} else {
		req.CamouflagePath = ""
	}
	if req.FallbackUpstreamURL == "" {
		return fmt.Errorf("fallback_upstream_url is required for traffic camouflage packs")
	}
	parsed, err := validateServicePackFallbackURLHTTP(req.FallbackUpstreamURL)
	if err != nil {
		return err
	}
	if req.FallbackHostHeader == "" {
		req.FallbackHostHeader = parsed.Host
	}
	if req.FallbackSNI == "" && strings.EqualFold(parsed.Scheme, "https") {
		req.FallbackSNI = parsed.Hostname()
	}
	if err := validateServicePackFallbackLoopGuardHTTP(req, parsed); err != nil {
		return err
	}
	for _, item := range []struct {
		name  string
		value string
	}{
		{name: "fallback_host_header", value: req.FallbackHostHeader},
		{name: "fallback_sni", value: req.FallbackSNI},
	} {
		if err := validateServicePackNginxDirectiveValueHTTP(item.name, item.value); err != nil {
			return err
		}
	}
	return nil
}

func validateServicePackFallbackLoopGuardHTTP(req *createServicePackRequest, parsed *url.URL) error {
	endpointHost := normalizeServicePackHostHTTP(req.EndpointHost)
	if endpointHost == "" || parsed == nil {
		return nil
	}
	for _, item := range []struct {
		name  string
		value string
	}{
		{name: "fallback_upstream_url", value: parsed.Hostname()},
		{name: "fallback_host_header", value: req.FallbackHostHeader},
		{name: "fallback_sni", value: req.FallbackSNI},
	} {
		if normalizeServicePackHostHTTP(item.value) == endpointHost {
			return fmt.Errorf("%s must not target the public ingress endpoint; choose a separate fallback website", item.name)
		}
	}
	return nil
}

func normalizeServicePackHostHTTP(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.Contains(value, "://") {
		if parsed, err := url.Parse(value); err == nil && parsed != nil {
			value = parsed.Hostname()
		}
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	}
	value = strings.TrimPrefix(strings.TrimSuffix(value, "]"), "[")
	value = strings.TrimSuffix(value, ".")
	return strings.ToLower(value)
}

func validateServicePackCamouflagePathHTTP(path string) error {
	path = strings.TrimSpace(path)
	if path == "" || !strings.HasPrefix(path, "/") {
		return fmt.Errorf("camouflage_path must start with /")
	}
	if path == "/" {
		return fmt.Errorf("camouflage_path must not be /; fallback website needs the root location")
	}
	if strings.ContainsAny(path, "{};\"'`\\#?\r\n\t ") {
		return fmt.Errorf("camouflage_path contains unsafe characters")
	}
	return nil
}

func validateServicePackFallbackURLHTTP(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("fallback_upstream_url is required")
	}
	if err := validateServicePackNginxDirectiveValueHTTP("fallback_upstream_url", raw); err != nil {
		return nil, err
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed == nil {
		return nil, fmt.Errorf("fallback_upstream_url is invalid")
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("fallback_upstream_url must not contain credentials")
	}
	if parsed.Host == "" || parsed.Scheme == "" {
		return nil, fmt.Errorf("fallback_upstream_url must be an absolute http or https URL")
	}
	if !strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https") {
		return nil, fmt.Errorf("fallback_upstream_url scheme must be http or https")
	}
	return parsed, nil
}

func validateServicePackNginxDirectiveValueHTTP(name, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if strings.ContainsAny(value, "{};\"'`\\#\r\n\t ") {
		return fmt.Errorf("%s contains unsafe characters", name)
	}
	return nil
}

func servicePackUsesTrafficCamouflage(pack domain.ServicePackDefinition) bool {
	for _, component := range pack.Components {
		if servicePackComponentUsesTrafficCamouflage(component) {
			return true
		}
	}
	return false
}

func servicePackComponentUsesTrafficCamouflage(component domain.ServicePackComponent) bool {
	profile := strings.ToLower(strings.TrimSpace(stringifyHTTP(component.Spec["service_profile"])))
	switch profile {
	case "ws_camouflage_edge", "nginx_ws_backend", "grpc_edge", "nginx_grpc_backend":
		return true
	}
	return specValueContainsTemplateHTTP(component.Spec, "camouflage_path") ||
		specValueContainsTemplateHTTP(component.Spec, "fallback_upstream_url")
}

func servicePackUsesConfigurableCamouflagePathHTTP(pack domain.ServicePackDefinition) bool {
	for _, component := range pack.Components {
		if specValueContainsTemplateHTTP(component.Spec, "camouflage_path") {
			return true
		}
	}
	return false
}

func defaultServicePackCamouflagePathHTTP(pack domain.ServicePackDefinition) string {
	for _, component := range pack.Components {
		for _, key := range []string{"location_path", "public_path", "path"} {
			value := strings.TrimSpace(stringifyHTTP(component.Spec[key]))
			if value == "" || strings.Contains(value, "{{") {
				continue
			}
			if strings.HasPrefix(value, "/") && value != "/" {
				return value
			}
		}
	}
	if strings.Contains(strings.ToLower(pack.Key), "grpc") {
		return "/vless-grpc"
	}
	return "/assets/rtis-sync"
}

func specValueContainsTemplateHTTP(value any, name string) bool {
	token := "{{" + name + "}}"
	switch typed := value.(type) {
	case string:
		return strings.Contains(typed, token)
	case map[string]any:
		for _, item := range typed {
			if specValueContainsTemplateHTTP(item, name) {
				return true
			}
		}
	case []any:
		for _, item := range typed {
			if specValueContainsTemplateHTTP(item, name) {
				return true
			}
		}
	}
	return false
}

func applyXrayEgressOverrideHTTP(component domain.ServicePackComponent, spec map[string]any, mode string, egressNodeID string) {
	if driver.NormalizeCode(component.ServiceCode) != driver.XrayCore {
		return
	}
	mode = strings.TrimSpace(mode)
	egressNodeID = strings.TrimSpace(egressNodeID)
	if mode == "" && egressNodeID == "" {
		return
	}
	if mode == "" {
		mode = "egress_node"
	}
	spec["egress_mode"] = mode
	egress := map[string]any{"mode": mode}
	if egressNodeID != "" {
		spec["egress_node_id"] = egressNodeID
		egress["egress_node_id"] = egressNodeID
	}
	spec["xray_egress"] = egress
}

func servicePackUsesTLSEdgeCertificate(pack domain.ServicePackDefinition) bool {
	return servicePackComponentsUseTLSEdgeCertificate(pack.Components)
}

func servicePackComponentsUseTLSEdgeCertificate(components []domain.ServicePackComponent) bool {
	for _, component := range components {
		if servicePackComponentUsesTLSEdgeCertificate(component) {
			return true
		}
	}
	return false
}

func servicePackComponentUsesTLSEdgeCertificate(component domain.ServicePackComponent) bool {
	switch driver.NormalizeCode(component.ServiceCode) {
	case driver.Nginx:
		return true
	case driver.XrayCore:
		return strings.EqualFold(firstStringHTTP(component.Spec["security"], component.Spec["tls_security"]), "tls")
	default:
		return false
	}
}

func (s *Server) defaultPlatformLeafCertificateID(ctx context.Context) (string, error) {
	items, err := s.store.ListPlatformCertificates(ctx)
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	for _, item := range items {
		if !item.IsDefault || !strings.EqualFold(strings.TrimSpace(item.Kind), "leaf") || !strings.EqualFold(strings.TrimSpace(item.Status), "active") {
			continue
		}
		if item.KeySecretRefID == nil || strings.TrimSpace(*item.KeySecretRefID) == "" {
			continue
		}
		if item.NotAfter != nil && !item.NotAfter.After(now) {
			continue
		}
		return item.ID, nil
	}
	return "", errDefaultPlatformLeafCertificateNotFound
}

func missingServicePackRuntimeCapabilities(components []domain.ServicePackComponent, capabilities []domain.NodeCapability) []string {
	ready := map[string]bool{}
	for _, capability := range capabilities {
		code := runtimeCapabilityCodeHTTP(capability.CapabilityCode)
		if code == "" {
			continue
		}
		if isRuntimeCapabilityReadyHTTP(capability.Status) {
			ready[code] = true
		}
	}
	seen := map[string]bool{}
	out := []string{}
	for _, component := range components {
		code := runtimeCapabilityCodeHTTP(component.ServiceCode)
		if code == "" || ready[code] || seen[code] || !hasAutomaticInstallStrategyHTTP(code) {
			continue
		}
		seen[code] = true
		out = append(out, code)
	}
	return out
}

func runtimeCapabilityCodeHTTP(serviceCode string) string {
	contract, ok := driver.ContractFor(serviceCode)
	if ok && strings.TrimSpace(contract.RuntimeCode) != "" {
		return driver.NormalizeCode(contract.RuntimeCode)
	}
	return driver.NormalizeCode(serviceCode)
}

func isRuntimeCapabilityReadyHTTP(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "available", "installed", "detected":
		return true
	default:
		return false
	}
}

func hasAutomaticInstallStrategyHTTP(serviceCode string) bool {
	switch driver.NormalizeCode(serviceCode) {
	case driver.Nginx, driver.XrayCore, driver.OpenVPN, driver.WireGuard, driver.IPSec, driver.XL2TPD, driver.HTTPProxy, driver.Shadowsocks:
		return true
	default:
		return false
	}
}

func (s *Server) availableServicePacks(ctx context.Context) ([]domain.ServicePackDefinition, error) {
	catalog, ok := s.store.(servicePackCatalogStore)
	if !ok {
		return DefaultServicePackDefinitions(), nil
	}
	defaults := DefaultServicePackDefinitions()
	if err := catalog.EnsureDefaultServicePacks(ctx, defaults); err != nil {
		if isServicePackCatalogUnavailable(err) {
			return defaults, nil
		}
		return nil, err
	}
	packs, err := catalog.ListServicePacks(ctx)
	if isServicePackCatalogUnavailable(err) {
		return defaults, nil
	}
	return packs, err
}

func (s *Server) getServicePack(ctx context.Context, key string) (domain.ServicePackDefinition, error) {
	key = strings.TrimSpace(key)
	catalog, ok := s.store.(servicePackCatalogStore)
	if !ok {
		pack, found := findServicePack(key)
		if !found {
			return domain.ServicePackDefinition{}, domain.ErrServicePackNotFound
		}
		return pack, nil
	}
	defaults := DefaultServicePackDefinitions()
	if err := catalog.EnsureDefaultServicePacks(ctx, defaults); err != nil {
		if isServicePackCatalogUnavailable(err) {
			pack, found := findServicePack(key)
			if !found {
				return domain.ServicePackDefinition{}, domain.ErrServicePackNotFound
			}
			return pack, nil
		}
		return domain.ServicePackDefinition{}, err
	}
	pack, err := catalog.GetServicePack(ctx, key)
	if isServicePackCatalogUnavailable(err) {
		fallback, found := findServicePack(key)
		if !found {
			return domain.ServicePackDefinition{}, domain.ErrServicePackNotFound
		}
		return fallback, nil
	}
	return pack, err
}

func isServicePackCatalogUnavailable(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "42P01", "42703":
			return true
		}
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "service_pack_templates") && (strings.Contains(text, "does not exist") || strings.Contains(text, "undefined"))
}

type servicePackInstanceIdentitySet struct {
	slugs     map[string]struct{}
	nodeNames map[string]struct{}
}

func servicePackComponentBaseIdentity(baseName string, component domain.ServicePackComponent) (string, string) {
	baseName = strings.TrimSpace(baseName)
	if baseName == "" {
		baseName = "instance"
	}
	name := baseName
	if suffix := strings.TrimSpace(component.NameSuffix); suffix != "" {
		name += "-" + suffix
	}
	slug := slugifyHTTP(baseName)
	if suffix := strings.TrimSpace(component.SlugSuffix); suffix != "" {
		slug += "-" + slugifyHTTP(suffix)
	}
	return name, slug
}

func findExistingServicePackComponentInstance(instances []domain.Instance, nodeID string, component domain.ServicePackComponent, baseName, baseSlug, endpointHost string) (domain.Instance, bool) {
	nodeID = strings.TrimSpace(nodeID)
	serviceCode := driver.NormalizeCode(component.ServiceCode)
	endpointHost = strings.ToLower(strings.TrimSpace(endpointHost))
	endpointPort := component.EndpointPort
	baseName = strings.ToLower(strings.TrimSpace(baseName))
	baseSlug = strings.ToLower(slugifyHTTP(baseSlug))
	bestScore := 0
	var best domain.Instance
	for _, instance := range instances {
		if strings.TrimSpace(instance.ID) == "" || strings.EqualFold(strings.TrimSpace(instance.Status), "deleted") {
			continue
		}
		if strings.TrimSpace(instance.NodeID) != nodeID {
			continue
		}
		if driver.NormalizeCode(instance.ServiceCode) != serviceCode {
			continue
		}
		if endpointPort != 0 && instance.EndpointPort != endpointPort {
			continue
		}
		if endpointHost != "" && strings.ToLower(strings.TrimSpace(instance.EndpointHost)) != endpointHost {
			continue
		}
		score := servicePackExistingInstanceMatchScore(instance, baseName, baseSlug)
		if best.ID == "" || score < bestScore || (score == bestScore && instance.CreatedAt.Before(best.CreatedAt)) {
			best = instance
			bestScore = score
		}
	}
	return best, best.ID != ""
}

func servicePackExistingInstanceMatchScore(instance domain.Instance, baseName, baseSlug string) int {
	name := strings.ToLower(strings.TrimSpace(instance.Name))
	slug := strings.ToLower(strings.TrimSpace(instance.Slug))
	switch {
	case baseSlug != "" && slug == baseSlug:
		return 0
	case baseName != "" && name == baseName:
		return 1
	case baseSlug != "" && servicePackOrdinalMatch(slug, baseSlug, "-"):
		return 2
	case baseName != "" && servicePackOrdinalMatch(name, baseName, " "):
		return 3
	default:
		return 10
	}
}

func servicePackOrdinalMatch(value, base, separator string) bool {
	if value == base {
		return true
	}
	if !strings.HasPrefix(value, base+separator) {
		return false
	}
	suffix := strings.TrimPrefix(value, base+separator)
	if suffix == "" {
		return false
	}
	for _, r := range suffix {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func newServicePackInstanceIdentitySet(instances []domain.Instance) *servicePackInstanceIdentitySet {
	set := &servicePackInstanceIdentitySet{
		slugs:     map[string]struct{}{},
		nodeNames: map[string]struct{}{},
	}
	for _, instance := range instances {
		if strings.EqualFold(strings.TrimSpace(instance.Status), "deleted") {
			continue
		}
		if slug := strings.ToLower(strings.TrimSpace(instance.Slug)); slug != "" {
			set.slugs[slug] = struct{}{}
		}
		if name := strings.ToLower(strings.TrimSpace(instance.Name)); name != "" {
			set.nodeNames[servicePackNodeNameKey(instance.NodeID, name)] = struct{}{}
		}
	}
	return set
}

func (set *servicePackInstanceIdentitySet) reserve(nodeID, baseName, baseSlug string) (string, string) {
	if set == nil {
		set = newServicePackInstanceIdentitySet(nil)
	}
	baseName = strings.TrimSpace(baseName)
	if baseName == "" {
		baseName = "instance"
	}
	baseSlug = slugifyHTTP(baseSlug)
	for ordinal := 1; ordinal < 1000; ordinal++ {
		name := servicePackOrdinalName(baseName, ordinal)
		slug := servicePackOrdinalSlug(baseSlug, ordinal)
		_, slugTaken := set.slugs[strings.ToLower(slug)]
		_, nameTaken := set.nodeNames[servicePackNodeNameKey(nodeID, strings.ToLower(name))]
		if slugTaken || nameTaken {
			continue
		}
		set.slugs[strings.ToLower(slug)] = struct{}{}
		set.nodeNames[servicePackNodeNameKey(nodeID, strings.ToLower(name))] = struct{}{}
		return name, slug
	}
	fallback := fmt.Sprintf("%s-%d", baseSlug, time.Now().UnixNano())
	return servicePackOrdinalName(baseName, 1000), fallback
}

func servicePackNodeNameKey(nodeID, name string) string {
	return strings.TrimSpace(nodeID) + "\x00" + strings.ToLower(strings.TrimSpace(name))
}

func servicePackOrdinalName(baseName string, ordinal int) string {
	if ordinal <= 1 {
		return baseName
	}
	return fmt.Sprintf("%s %d", baseName, ordinal)
}

func servicePackOrdinalSlug(baseSlug string, ordinal int) string {
	if ordinal <= 1 {
		return baseSlug
	}
	return fmt.Sprintf("%s-%d", baseSlug, ordinal)
}

func isInstanceIdentityUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return false
	}
	name := strings.ToLower(strings.TrimSpace(pgErr.ConstraintName))
	return strings.Contains(name, "instances_active_slug") ||
		strings.Contains(name, "instances_active_node_name") ||
		strings.Contains(name, "instances_slug_key") ||
		strings.Contains(name, "instances_node_id_name_key")
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

func firstStringHTTP(values ...any) string {
	for _, value := range values {
		text := strings.TrimSpace(stringifyHTTP(value))
		if text != "" {
			return text
		}
	}
	return ""
}

func stringifyHTTP(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		if value == nil {
			return ""
		}
		return fmt.Sprint(value)
	}
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

func truthyQuery(r *nethttp.Request, key string) bool {
	value := strings.ToLower(strings.TrimSpace(r.URL.Query().Get(key)))
	return value == "1" || value == "true" || value == "yes" || value == "y" || value == "on"
}
