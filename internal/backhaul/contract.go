package backhaul

import (
	"fmt"
	"net/netip"
	"sort"
	"strings"
)

const (
	DriverWireGuard     = "wireguard"
	DriverOpenVPNUDP    = "openvpn_udp"
	DriverOpenVPNTCP443 = "openvpn_tcp_443"
	DriverIPSecL2TP     = "ipsec_l2tp"
	DriverIKEv2         = "ikev2"
	DriverXrayVLESSWS   = "xray_vless_ws_tls"
	DriverXrayVLESSGRPC = "xray_vless_grpc_tls"
	DriverXrayReality   = "xray_vless_reality"
	DriverXrayTUNVLESS  = "xray_tun_vless"
)

type DriverDefinition struct {
	Code                 string   `json:"code"`
	Label                string   `json:"label"`
	Layer                string   `json:"layer"`
	DefaultPort          int      `json:"default_port"`
	DefaultProtocol      string   `json:"default_protocol"`
	ActivationMode       string   `json:"activation_mode"`
	SupportsKernelRoutes bool     `json:"supports_kernel_routes"`
	Capabilities         []string `json:"capabilities"`
	Warnings             []string `json:"warnings,omitempty"`
}

func Catalog() []DriverDefinition {
	return []DriverDefinition{
		{Code: DriverWireGuard, Label: "WireGuard L3", Layer: "l3", DefaultPort: 51830, DefaultProtocol: "udp", ActivationMode: "managed_systemd", SupportsKernelRoutes: true, Capabilities: []string{"wireguard"}},
		{Code: DriverOpenVPNUDP, Label: "OpenVPN UDP P2P", Layer: "l3", DefaultPort: 11950, DefaultProtocol: "udp", ActivationMode: "managed_systemd", SupportsKernelRoutes: true, Capabilities: []string{"openvpn"}, Warnings: []string{"Static-key OpenVPN P2P is a fallback transport; prefer certificate mode before broad production rollout."}},
		{Code: DriverOpenVPNTCP443, Label: "OpenVPN TCP 443 P2P", Layer: "l3", DefaultPort: 443, DefaultProtocol: "tcp", ActivationMode: "managed_systemd", SupportsKernelRoutes: true, Capabilities: []string{"openvpn"}, Warnings: []string{"TCP-over-TCP can degrade throughput; use only as blocked-UDP fallback."}},
		{Code: DriverIPSecL2TP, Label: "L2TP + IPsec", Layer: "l3", DefaultPort: 1701, DefaultProtocol: "udp", ActivationMode: "materialize_only", SupportsKernelRoutes: true, Capabilities: []string{"ipsec", "xl2tpd"}, Warnings: []string{"Requires host-level strongSwan/xl2tpd integration before auto activation."}},
		{Code: DriverIKEv2, Label: "IKEv2 / IPsec", Layer: "l3", DefaultPort: 500, DefaultProtocol: "udp", ActivationMode: "materialize_only", SupportsKernelRoutes: true, Capabilities: []string{"ipsec"}, Warnings: []string{"Requires site-to-site swanctl/ipsec profile before auto activation."}},
		{Code: DriverXrayVLESSWS, Label: "Xray VLESS WebSocket TLS", Layer: "proxy", DefaultPort: 443, DefaultProtocol: "tcp", ActivationMode: "materialize_only", SupportsKernelRoutes: false, Capabilities: []string{"xray-core"}, Warnings: []string{"Proxy/camouflage transport; kernel L3 routing needs an explicit TUN/TProxy stage."}},
		{Code: DriverXrayVLESSGRPC, Label: "Xray VLESS gRPC TLS", Layer: "proxy", DefaultPort: 443, DefaultProtocol: "tcp", ActivationMode: "materialize_only", SupportsKernelRoutes: false, Capabilities: []string{"xray-core"}, Warnings: []string{"Proxy/camouflage transport; kernel L3 routing needs an explicit TUN/TProxy stage."}},
		{Code: DriverXrayReality, Label: "Xray VLESS Reality", Layer: "proxy", DefaultPort: 443, DefaultProtocol: "tcp", ActivationMode: "materialize_only", SupportsKernelRoutes: false, Capabilities: []string{"xray-core"}, Warnings: []string{"Reality is suitable for camouflage, not direct L3 routing by itself."}},
		{Code: DriverXrayTUNVLESS, Label: "Xray TUN over VLESS", Layer: "l3_over_proxy", DefaultPort: 443, DefaultProtocol: "tcp", ActivationMode: "materialize_only", SupportsKernelRoutes: true, Capabilities: []string{"xray-core"}, Warnings: []string{"Requires careful Linux policy routing and loop protection before auto activation."}},
	}
}

func Definition(driver string) (DriverDefinition, bool) {
	driver = NormalizeDriver(driver)
	for _, def := range Catalog() {
		if def.Code == driver {
			return def, true
		}
	}
	return DriverDefinition{}, false
}

func NormalizeDriver(driver string) string {
	return strings.ToLower(strings.TrimSpace(strings.ReplaceAll(driver, "-", "_")))
}

func SupportedDrivers() []string {
	out := make([]string, 0, len(Catalog()))
	for _, def := range Catalog() {
		out = append(out, def.Code)
	}
	sort.Strings(out)
	return out
}

func ValidateDriver(driver string) error {
	if _, ok := Definition(driver); !ok {
		return fmt.Errorf("unsupported backhaul driver %q", strings.TrimSpace(driver))
	}
	return nil
}

func IsL3Capable(driver string) bool {
	def, ok := Definition(driver)
	return ok && def.SupportsKernelRoutes
}

func ValidateTunnelCIDR(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("tunnel_cidr is required")
	}
	prefix, err := netip.ParsePrefix(value)
	if err != nil {
		return fmt.Errorf("tunnel_cidr must be a valid CIDR: %w", err)
	}
	if !prefix.Addr().Is4() || prefix.Bits() > 30 {
		return fmt.Errorf("tunnel_cidr must be an IPv4 point-to-point prefix with at least two usable addresses")
	}
	return nil
}
