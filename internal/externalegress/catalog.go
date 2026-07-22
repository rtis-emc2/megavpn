package externalegress

import "strings"

const (
	RuntimeReady   = "ready"
	RuntimePreview = "preview"
	RuntimePlanned = "planned"
)

type ProtocolDefinition struct {
	Code            string   `json:"code"`
	Label           string   `json:"label"`
	Category        string   `json:"category"`
	RuntimeSupport  string   `json:"runtime_support"`
	ImportFormats   []string `json:"import_formats"`
	CredentialModes []string `json:"credential_modes"`
	Transports      []string `json:"transports,omitempty"`
	Notes           string   `json:"notes"`
}

var protocolCatalog = []ProtocolDefinition{
	{Code: "openvpn", Label: "OpenVPN", Category: "tunnel", RuntimeSupport: RuntimeReady, ImportFormats: []string{"ovpn", "inline"}, CredentialModes: []string{"config_file", "username_password", "certificate_private_key"}, Transports: []string{"udp", "tcp"}, Notes: "Parsed client profile with managed route isolation."},
	{Code: "wireguard", Label: "WireGuard", Category: "tunnel", RuntimeSupport: RuntimeReady, ImportFormats: []string{"conf", "inline"}, CredentialModes: []string{"config_file", "private_key", "preshared_key"}, Transports: []string{"udp"}, Notes: "Managed wg-quick client profile with a dedicated routing table."},
	{Code: "socks5", Label: "SOCKS5 proxy", Category: "proxy", RuntimeSupport: RuntimePreview, ImportFormats: []string{"structured"}, CredentialModes: []string{"username_password", "none"}, Transports: []string{"tcp"}, Notes: "Can be used by proxy-aware runtimes; transparent L3 routing requires a managed TUN adapter."},
	{Code: "http_connect", Label: "HTTP CONNECT proxy", Category: "proxy", RuntimeSupport: RuntimePreview, ImportFormats: []string{"structured", "url"}, CredentialModes: []string{"username_password", "none"}, Transports: []string{"tcp", "tls"}, Notes: "Can be used by proxy-aware runtimes; transparent L3 routing requires a managed TUN adapter."},
	{Code: "shadowsocks", Label: "Shadowsocks", Category: "proxy_tunnel", RuntimeSupport: RuntimeReady, ImportFormats: []string{"url", "json"}, CredentialModes: []string{"password"}, Transports: []string{"tcp", "udp"}, Notes: "Runs as an isolated loopback Xray proxy and is selected only by assigned client access groups."},
	{Code: "vless", Label: "VLESS", Category: "proxy_tunnel", RuntimeSupport: RuntimeReady, ImportFormats: []string{"url", "json"}, CredentialModes: []string{"uuid"}, Transports: []string{"tcp", "ws", "grpc", "httpupgrade", "xhttp"}, Notes: "Runs as an isolated loopback Xray proxy with mandatory TLS or REALITY security."},
	{Code: "l2tp", Label: "L2TP", Category: "tunnel", RuntimeSupport: RuntimePlanned, ImportFormats: []string{"structured"}, CredentialModes: []string{"username_password"}, Transports: []string{"udp"}, Notes: "Unencrypted L2TP is catalogued but cannot be activated because it is not safe for production."},
	{Code: "l2tp_ipsec", Label: "L2TP over IPsec", Category: "tunnel", RuntimeSupport: RuntimeReady, ImportFormats: []string{"key_value", "json"}, CredentialModes: []string{"username_password", "psk"}, Transports: []string{"udp", "ipsec"}, Notes: "Managed strongSwan + xl2tpd client with an isolated PPP interface and fail-closed policy routing."},
	{Code: "ikev2", Label: "IKEv2 / IPsec", Category: "tunnel", RuntimeSupport: RuntimePlanned, ImportFormats: []string{"structured", "mobileconfig"}, CredentialModes: []string{"username_password", "certificate_private_key", "psk"}, Transports: []string{"udp", "ipsec"}, Notes: "Preferred IPsec family option; client-mode runtime driver is planned."},
	{Code: "trojan", Label: "Trojan", Category: "proxy_tunnel", RuntimeSupport: RuntimePlanned, ImportFormats: []string{"url", "json", "structured"}, CredentialModes: []string{"password", "certificate"}, Transports: []string{"tls"}, Notes: "Catalogued for a future managed TUN runtime."},
	{Code: "hysteria2", Label: "Hysteria 2", Category: "proxy_tunnel", RuntimeSupport: RuntimePlanned, ImportFormats: []string{"url", "yaml", "structured"}, CredentialModes: []string{"password", "certificate"}, Transports: []string{"quic"}, Notes: "Catalogued for a future managed TUN runtime."},
}

func Catalog() []ProtocolDefinition {
	out := make([]ProtocolDefinition, len(protocolCatalog))
	copy(out, protocolCatalog)
	return out
}

func Definition(code string) (ProtocolDefinition, bool) {
	code = NormalizeProtocol(code)
	for _, item := range protocolCatalog {
		if item.Code == code {
			return item, true
		}
	}
	return ProtocolDefinition{}, false
}

func NormalizeProtocol(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "ovpn", "openvpn_udp", "openvpn_tcp", "openvpn":
		return "openvpn"
	case "wg", "wireguard":
		return "wireguard"
	case "http", "https", "http_proxy", "connect", "http_connect":
		return "http_connect"
	case "socks", "socks5":
		return "socks5"
	case "ss", "shadowsocks":
		return "shadowsocks"
	case "xray", "xray-core", "vless":
		return "vless"
	case "l2tp+ipsec", "l2tp_ipsec", "xl2tpd_ipsec":
		return "l2tp_ipsec"
	case "ipsec", "ikev2":
		return "ikev2"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}
