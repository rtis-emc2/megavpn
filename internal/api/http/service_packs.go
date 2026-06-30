package http

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	nethttp "net/http"

	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/rbac"
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
	NodeID             string `json:"node_id"`
	BaseName           string `json:"base_name"`
	EndpointHost       string `json:"endpoint_host"`
	CertificateID      string `json:"certificate_id"`
	OpenVPNPKIProfile  string `json:"openvpn_pki_profile"`
	AutoInstallRuntime *bool  `json:"auto_install_runtime"`
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
				{Label: "VLESS TCP Edge", Description: "VLESS/Reality component для пары VLESS + OpenVPN TCP.", ServiceCode: "xray-core", PresetKey: "reality_tcp", NameSuffix: "vless-tcp-edge", SlugSuffix: "vless-tcp-edge", EndpointPort: 443, RequiresEndpointHost: true, Spec: map[string]any{"service_profile": "reality_tcp", "security": "reality", "network": "tcp", "dest": "www.cloudflare.com:443", "fingerprint": "chrome", "auto_generate_reality_keys": true, "config_mode": "0640"}},
				{Label: "OpenVPN TCP Companion", Description: "OpenVPN TCP component для пары VLESS + OpenVPN TCP.", ServiceCode: "openvpn", PresetKey: "tcp_11994", NameSuffix: "openvpn-tcp", SlugSuffix: "openvpn-tcp", EndpointPort: 11994, RequiresEndpointHost: true, Spec: map[string]any{"service_profile": "tcp_11994", "pki_scope": "platform", "pki_profile": "default", "proto": "tcp", "dev": "tun", "address_pool_mode": "auto", "config_mode": "0644"}},
				{Label: "VLESS Standalone", Description: "Отдельный VLESS/Reality instance на альтернативном TCP-порту.", ServiceCode: "xray-core", PresetKey: "reality_tcp", NameSuffix: "vless", SlugSuffix: "vless", EndpointPort: 8443, RequiresEndpointHost: true, Spec: map[string]any{"service_profile": "reality_tcp", "security": "reality", "network": "tcp", "dest": "www.cloudflare.com:443", "fingerprint": "chrome", "auto_generate_reality_keys": true, "config_mode": "0640"}},
				{Label: "OpenVPN UDP", Description: "Классический OpenVPN UDP baseline.", ServiceCode: "openvpn", PresetKey: "udp_1194", NameSuffix: "openvpn-udp", SlugSuffix: "openvpn-udp", EndpointPort: 1194, RequiresEndpointHost: true, Spec: map[string]any{"service_profile": "udp_1194", "pki_scope": "platform", "pki_profile": "default", "proto": "udp", "dev": "tun", "address_pool_mode": "auto", "config_mode": "0644"}},
				{Label: "Shadowsocks", Description: "Standalone Shadowsocks chacha20-ietf-poly1305 baseline.", ServiceCode: "shadowsocks", PresetKey: "chacha_full", NameSuffix: "shadowsocks", SlugSuffix: "shadowsocks", EndpointPort: 8388, RequiresEndpointHost: true, Spec: map[string]any{"service_profile": "chacha_full", "method": "chacha20-ietf-poly1305", "mode": "tcp_and_udp", "timeout": 300, "auto_generate_server_password": true, "config_mode": "0640"}},
				{Label: "WireGuard", Description: "Standalone WireGuard road-warrior baseline.", ServiceCode: "wireguard", PresetKey: "roadwarrior", NameSuffix: "wireguard", SlugSuffix: "wireguard", EndpointPort: 51820, RequiresEndpointHost: true, Spec: map[string]any{"service_profile": "roadwarrior", "address_pool_mode": "auto", "client_allowed_ips": "0.0.0.0/0, ::/0", "client_dns": "1.1.1.1, 1.0.0.1", "persistent_keepalive": 25, "auto_generate_server_key": true, "config_mode": "0600"}},
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
				{Label: "Xray VLESS / Reality", Description: "Прямой публичный transport без edge companion.", ServiceCode: "xray-core", PresetKey: "reality_tcp", NameSuffix: "xray-reality", SlugSuffix: "xray-reality", EndpointPort: 443, RequiresEndpointHost: true, Spec: map[string]any{"service_profile": "reality_tcp", "security": "reality", "network": "tcp", "dest": "www.cloudflare.com:443", "fingerprint": "chrome", "config_mode": "0640"}},
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
				{Label: "Xray gRPC Backend", Description: "Runtime backend для Nginx gRPC edge.", ServiceCode: "xray-core", PresetKey: "nginx_grpc_backend", NameSuffix: "xray-grpc", SlugSuffix: "xray-grpc", EndpointPort: 7443, RequiresEndpointHost: true, Spec: map[string]any{"service_profile": "nginx_grpc_backend", "security": "none", "network": "grpc", "service_name": "vless-grpc", "public_security": "tls", "public_network": "grpc", "public_service_name": "vless-grpc", "public_port": 443, "config_mode": "0640"}},
				{Label: "Nginx gRPC Edge", Description: "Публичный TLS ingress для backend Xray gRPC с fallback на реальный сайт.", ServiceCode: "nginx", PresetKey: "grpc_edge", NameSuffix: "nginx-grpc-edge", SlugSuffix: "nginx-grpc-edge", EndpointPort: 443, RequiresEndpointHost: true, Spec: map[string]any{"service_profile": "grpc_edge", "mode": "grpc_proxy", "location_path": "/vless-grpc", "server_name": "{{endpoint_host}}", "upstream_url": "grpc://127.0.0.1:7443", "fallback_upstream_url": "https://example.com", "fallback_host_header": "example.com", "fallback_sni": "example.com", "tls_enabled": true, "tls_cert_path": "/etc/letsencrypt/live/{{endpoint_host}}/fullchain.pem", "tls_key_path": "/etc/letsencrypt/live/{{endpoint_host}}/privkey.pem", "config_mode": "0644"}},
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
				{Label: "Xray VLESS/WebSocket Backend", Description: "Runtime backend для Nginx WebSocket edge.", ServiceCode: "xray-core", PresetKey: "nginx_ws_backend", NameSuffix: "xray-ws", SlugSuffix: "xray-ws", EndpointPort: 7080, RequiresEndpointHost: true, Spec: map[string]any{"service_profile": "nginx_ws_backend", "security": "none", "network": "ws", "path": "/assets/rtis-sync", "public_security": "tls", "public_network": "ws", "public_path": "/assets/rtis-sync", "public_host_header": "{{endpoint_host}}", "public_port": 443, "config_mode": "0640"}},
				{Label: "Nginx WebSocket Camouflage Edge", Description: "Публичный TLS reverse-proxy для Xray ws path и fallback website для обычного web-трафика.", ServiceCode: "nginx", PresetKey: "ws_camouflage_edge", NameSuffix: "nginx-camouflage-edge", SlugSuffix: "nginx-camouflage-edge", EndpointPort: 443, RequiresEndpointHost: true, Spec: map[string]any{"service_profile": "ws_camouflage_edge", "mode": "reverse_proxy", "location_path": "/assets/rtis-sync", "server_name": "{{endpoint_host}}", "upstream_url": "http://127.0.0.1:7080", "fallback_upstream_url": "https://example.com", "fallback_host_header": "example.com", "fallback_sni": "example.com", "tls_enabled": true, "tls_cert_path": "/etc/letsencrypt/live/{{endpoint_host}}/fullchain.pem", "tls_key_path": "/etc/letsencrypt/live/{{endpoint_host}}/privkey.pem", "location_extra_lines": "proxy_set_header Upgrade $http_upgrade;\nproxy_set_header Connection \"upgrade\";", "config_mode": "0644"}},
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
				{Label: "OpenVPN TCP", Description: "OpenVPN instance с platform-managed PKI.", ServiceCode: "openvpn", PresetKey: "tcp_11994", NameSuffix: "openvpn-tcp", SlugSuffix: "openvpn-tcp", EndpointPort: 11994, RequiresEndpointHost: true, Spec: map[string]any{"service_profile": "tcp_11994", "pki_scope": "platform", "pki_profile": "default", "proto": "tcp", "dev": "tun", "address_pool_mode": "auto", "config_mode": "0644"}},
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
				{Label: "OpenVPN UDP", Description: "OpenVPN instance с platform-managed PKI.", ServiceCode: "openvpn", PresetKey: "udp_1194", NameSuffix: "openvpn-udp", SlugSuffix: "openvpn-udp", EndpointPort: 1194, RequiresEndpointHost: true, Spec: map[string]any{"service_profile": "udp_1194", "pki_scope": "platform", "pki_profile": "default", "proto": "udp", "dev": "tun", "address_pool_mode": "auto", "config_mode": "0644"}},
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
				{Label: "WireGuard Road Warrior", Description: "Standalone WireGuard instance с managed peers.", ServiceCode: "wireguard", PresetKey: "roadwarrior", NameSuffix: "wireguard", SlugSuffix: "wireguard", EndpointPort: 51820, RequiresEndpointHost: true, Spec: map[string]any{"service_profile": "roadwarrior", "address_pool_mode": "auto", "client_allowed_ips": "0.0.0.0/0, ::/0", "client_dns": "1.1.1.1, 1.0.0.1", "persistent_keepalive": 25, "config_mode": "0600"}},
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
		if !ok || !rbac.HasPermission(authCtx.PermissionCodes, "settings.manage") {
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
	if req.BaseName == "" {
		req.BaseName = pack.BaseNameTemplate
	}
	if pack.RequiresEndpointHost && req.EndpointHost == "" {
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
		for _, serviceCode := range missingServicePackRuntimeCapabilities(pack.Components, capabilities) {
			missingRuntimes[serviceCode] = true
		}
	}
	if req.CertificateID == "" && servicePackUsesTLSEdgeCertificate(pack) {
		certificateID, err := s.defaultPlatformLeafCertificateID(r.Context())
		if err != nil && !errors.Is(err, errDefaultPlatformLeafCertificateNotFound) {
			writeErr(w, 409, "default TLS edge certificate preflight failed: "+err.Error())
			return
		}
		req.CertificateID = certificateID
	}
	created := make([]domain.Instance, 0, len(pack.Components))
	applyNowInstanceIDs := []string{}
	dependentInstanceIDsByRuntime := map[string][]string{}
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
		if req.CertificateID != "" && servicePackComponentUsesTLSEdgeCertificate(component) {
			spec["certificate_id"] = req.CertificateID
		}
		if component.ServiceCode == "openvpn" && req.OpenVPNPKIProfile != "" {
			spec["pki_scope"] = "platform"
			spec["pki_profile"] = req.OpenVPNPKIProfile
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
		createdInstance, err := s.store.CreateInstanceValidatedDraft(r.Context(), instance)
		if err != nil {
			discarded := discardCreatedServicePackInstances(r.Context(), s.store, created)
			writeJSON(w, 409, response{
				"status":            "partial_failure",
				"error":             fmt.Sprintf("service pack component %q is not apply-ready: %v", servicePackComponentLabel(component), err),
				"component":         servicePackComponentLabel(component),
				"service_pack_key":  pack.Key,
				"created_instances": created,
				"discarded_count":   discarded,
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
				"runtime_install_jobs": redactedJobs(runtimeInstallJobs),
				"apply_jobs":           redactedJobs(applyJobs),
			})
			return
		}
		applyJobs = append(applyJobs, job)
	}
	authCtx, ok := authFromRequest(r)
	if ok {
		_, _ = s.store.CreateAuditForUser(r.Context(), &authCtx.User.ID, "service_pack.create", "service_pack", nil, "service pack created: "+pack.Key)
	}
	writeJSON(w, 201, response{
		"status":               "ok",
		"service_pack_key":     pack.Key,
		"created_instances":    created,
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

func servicePackUsesTLSEdgeCertificate(pack domain.ServicePackDefinition) bool {
	for _, component := range pack.Components {
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
