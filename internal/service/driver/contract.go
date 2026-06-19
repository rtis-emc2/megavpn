package driver

import "strings"

const (
	XrayCore    = "xray-core"
	OpenVPN     = "openvpn"
	WireGuard   = "wireguard"
	IPSec       = "ipsec"
	XL2TPD      = "xl2tpd"
	HTTPProxy   = "http_proxy"
	MTProto     = "mtproto"
	Shadowsocks = "shadowsocks"
	Nginx       = "nginx"
)

const (
	ArtifactAll             = "all"
	ArtifactZipBundle       = "zip_bundle"
	ArtifactOpenVPNProfile  = "ovpn"
	ArtifactVLESSURL        = "vless_url"
	ArtifactWireGuardConfig = "wg_conf"
	ArtifactMTProtoURL      = "mtproto_url"
	ArtifactHTTPProxyBundle = "http_proxy_bundle"
	ArtifactShadowsocksURL  = "ss_url"
	ArtifactIPSecBundle     = "ipsec_bundle"
)

type Contract struct {
	Code                  string            `json:"code"`
	RuntimeCode           string            `json:"runtime_code"`
	DisplayName           string            `json:"display_name"`
	DefaultUnitPattern    string            `json:"default_unit_pattern"`
	DefaultConfigPath     string            `json:"default_config_path"`
	DefaultConfigMode     string            `json:"default_config_mode"`
	ProvisioningSupported bool              `json:"provisioning_supported"`
	AccountSupported      bool              `json:"account_supported"`
	Artifacts             []string          `json:"artifacts"`
	InstallStrategies     []string          `json:"install_strategies"`
	RequiresIPForwarding  bool              `json:"requires_ip_forwarding"`
	Operations            []OperationSpec   `json:"operations"`
	HealthChecks          []HealthCheckSpec `json:"health_checks"`
}

var contracts = map[string]Contract{
	XrayCore: {
		Code:                  XrayCore,
		RuntimeCode:           XrayCore,
		DisplayName:           "Xray VLESS / Reality",
		DefaultUnitPattern:    "megavpn-xray-<slug>",
		DefaultConfigPath:     "/usr/local/etc/xray/<slug>.json",
		DefaultConfigMode:     "0640",
		ProvisioningSupported: true,
		AccountSupported:      true,
		Artifacts:             []string{ArtifactVLESSURL},
		InstallStrategies:     []string{"xtls_install_release", "manual_present"},
	},
	OpenVPN: {
		Code:                  OpenVPN,
		RuntimeCode:           OpenVPN,
		DisplayName:           "OpenVPN",
		DefaultUnitPattern:    "openvpn-server@<slug>",
		DefaultConfigPath:     "/etc/openvpn/server/<slug>.conf",
		DefaultConfigMode:     "0644",
		ProvisioningSupported: true,
		AccountSupported:      true,
		Artifacts:             []string{ArtifactOpenVPNProfile},
		InstallStrategies:     []string{"ubuntu_repo", "manual_present"},
		RequiresIPForwarding:  true,
	},
	WireGuard: {
		Code:                  WireGuard,
		RuntimeCode:           WireGuard,
		DisplayName:           "WireGuard",
		DefaultUnitPattern:    "wg-quick@<slug>",
		DefaultConfigPath:     "/etc/wireguard/<slug>.conf",
		DefaultConfigMode:     "0600",
		ProvisioningSupported: true,
		AccountSupported:      true,
		Artifacts:             []string{ArtifactWireGuardConfig},
		InstallStrategies:     []string{"ubuntu_repo", "manual_present"},
		RequiresIPForwarding:  true,
	},
	IPSec: {
		Code:                  IPSec,
		RuntimeCode:           IPSec,
		DisplayName:           "IPsec / IKEv2",
		DefaultUnitPattern:    "strongswan-starter",
		DefaultConfigPath:     "/etc/ipsec.conf",
		DefaultConfigMode:     "0644",
		ProvisioningSupported: true,
		AccountSupported:      true,
		Artifacts:             []string{ArtifactIPSecBundle},
		InstallStrategies:     []string{"ubuntu_repo", "manual_present"},
		RequiresIPForwarding:  true,
	},
	XL2TPD: {
		Code:                 XL2TPD,
		RuntimeCode:          XL2TPD,
		DisplayName:          "XL2TPD",
		DefaultUnitPattern:   "xl2tpd",
		DefaultConfigPath:    "/etc/xl2tpd/xl2tpd.conf",
		DefaultConfigMode:    "0644",
		AccountSupported:     false,
		InstallStrategies:    []string{"ubuntu_repo", "manual_present"},
		RequiresIPForwarding: true,
	},
	HTTPProxy: {
		Code:                  HTTPProxy,
		RuntimeCode:           HTTPProxy,
		DisplayName:           "HTTP Proxy",
		DefaultUnitPattern:    "megavpn-http-proxy-<slug>",
		DefaultConfigPath:     "/etc/squid/<slug>.conf",
		DefaultConfigMode:     "0644",
		ProvisioningSupported: true,
		AccountSupported:      true,
		Artifacts:             []string{ArtifactHTTPProxyBundle},
		InstallStrategies:     []string{"ubuntu_repo", "manual_present"},
	},
	MTProto: {
		Code:                  MTProto,
		RuntimeCode:           XrayCore,
		DisplayName:           "MTProto",
		DefaultUnitPattern:    "megavpn-mtproto-<slug>",
		DefaultConfigPath:     "/usr/local/etc/xray/<slug>.json",
		DefaultConfigMode:     "0640",
		ProvisioningSupported: true,
		AccountSupported:      true,
		Artifacts:             []string{ArtifactMTProtoURL},
		InstallStrategies:     []string{"manual_present"},
	},
	Shadowsocks: {
		Code:                  Shadowsocks,
		RuntimeCode:           Shadowsocks,
		DisplayName:           "Shadowsocks",
		DefaultUnitPattern:    "megavpn-shadowsocks-<slug>",
		DefaultConfigPath:     "/etc/shadowsocks-libev/<slug>.json",
		DefaultConfigMode:     "0640",
		ProvisioningSupported: true,
		AccountSupported:      true,
		Artifacts:             []string{ArtifactShadowsocksURL},
		InstallStrategies:     []string{"ubuntu_repo", "manual_present"},
	},
	Nginx: {
		Code:               Nginx,
		RuntimeCode:        Nginx,
		DisplayName:        "Nginx",
		DefaultUnitPattern: "nginx",
		DefaultConfigPath:  "/etc/nginx/conf.d/megavpn-<slug>.conf",
		DefaultConfigMode:  "0644",
		InstallStrategies:  []string{"nginx_org_repo", "ubuntu_repo", "manual_present"},
	},
}

func NormalizeCode(code string) string {
	code = strings.ToLower(strings.TrimSpace(code))
	switch code {
	case "xray", "xray-core", "xray_core":
		return XrayCore
	case "openvpn":
		return OpenVPN
	case "wireguard", "wg", "wg-quick":
		return WireGuard
	case "mtproto", "telegram-mtproto":
		return MTProto
	case "nginx":
		return Nginx
	case "ipsec", "strongswan":
		return IPSec
	case "http_proxy", "http-proxy", "squid":
		return HTTPProxy
	case "l2tp", "l2tpd", "xl2tpd":
		return XL2TPD
	case "shadowsocks", "shadowsocks-libev", "ss-server":
		return Shadowsocks
	default:
		return code
	}
}

func ContractFor(code string) (Contract, bool) {
	contract, ok := contracts[NormalizeCode(code)]
	if !ok {
		return Contract{}, false
	}
	return enrichContract(contract), true
}

func Contracts() []Contract {
	order := []string{XrayCore, OpenVPN, WireGuard, IPSec, XL2TPD, HTTPProxy, MTProto, Shadowsocks, Nginx}
	out := make([]Contract, 0, len(order))
	for _, code := range order {
		out = append(out, enrichContract(contracts[code]))
	}
	return out
}

func enrichContract(contract Contract) Contract {
	contract.Operations = OperationsFor(contract.Code)
	contract.HealthChecks = HealthChecksFor(contract.Code)
	return contract
}

func IsProvisioningSupported(code string) bool {
	contract, ok := ContractFor(code)
	return ok && contract.ProvisioningSupported
}

func IsValidInstallStrategy(code, strategy string) bool {
	contract, ok := ContractFor(code)
	if !ok {
		return false
	}
	for _, item := range contract.InstallStrategies {
		if item == strategy {
			return true
		}
	}
	return false
}

func NormalizeArtifactType(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	if IsSupportedArtifactType(value) {
		return value
	}
	return value
}

func IsSupportedArtifactType(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case ArtifactAll, ArtifactZipBundle, ArtifactOpenVPNProfile, ArtifactVLESSURL, ArtifactWireGuardConfig, ArtifactMTProtoURL, ArtifactHTTPProxyBundle, ArtifactShadowsocksURL, ArtifactIPSecBundle:
		return true
	default:
		return false
	}
}

func DefaultSystemdUnit(code, slug string) string {
	contract, ok := ContractFor(code)
	if !ok {
		return ""
	}
	return materializePattern(contract.DefaultUnitPattern, slug, fallbackSlug(contract.Code))
}

func DefaultConfigPath(code, slug string) string {
	contract, ok := ContractFor(code)
	if !ok {
		return ""
	}
	return materializePattern(contract.DefaultConfigPath, slug, fallbackSlug(contract.Code))
}

func DefaultConfigMode(code string) string {
	contract, ok := ContractFor(code)
	if !ok || strings.TrimSpace(contract.DefaultConfigMode) == "" {
		return "0644"
	}
	return contract.DefaultConfigMode
}

func RequiresIPForwarding(code string) bool {
	contract, ok := ContractFor(code)
	return ok && contract.RequiresIPForwarding
}

func materializePattern(pattern, slug, fallback string) string {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return ""
	}
	slug = strings.TrimSpace(slug)
	if slug == "" {
		slug = fallback
	}
	return strings.ReplaceAll(pattern, "<slug>", slug)
}

func fallbackSlug(code string) string {
	switch NormalizeCode(code) {
	case OpenVPN:
		return "server"
	case WireGuard:
		return "wg0"
	case HTTPProxy:
		return "proxy"
	case Shadowsocks:
		return "shadowsocks"
	default:
		return NormalizeCode(code)
	}
}
