package http

import (
	nethttp "net/http"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

func (s *Server) listClientAccessServices(w nethttp.ResponseWriter, r *nethttp.Request) {
	writeJSON(w, 200, clientAccessServiceCatalog())
}

func clientAccessServiceCatalog() []domain.ClientAccessService {
	return []domain.ClientAccessService{
		{
			ServiceCode:             "vless",
			DisplayName:             "VLESS / Xray",
			Description:             "Client access groups for VLESS route policy. Runtime Xray instances receive materialized service access automatically.",
			Category:                "vpn",
			Implemented:             true,
			SupportsGroups:          true,
			SupportsPolicy:          true,
			SupportsScope:           true,
			SupportsMembership:      true,
			SupportsMaterialization: true,
			Status:                  "active",
			RuntimeServiceCodes:     []string{"xray-core"},
			PolicyCapabilities: map[string]any{
				"modes": []string{"instance_default", "local_breakout", "egress_node", "instance_only", "block"},
				"routing": map[string]bool{
					"egress_node": true,
					"ad_block":    true,
					"target_only": true,
				},
			},
		},
		{
			ServiceCode:         "openvpn",
			DisplayName:         "OpenVPN",
			Description:         "Runtime and client config generation are implemented. Client access group materialization is planned.",
			Category:            "vpn",
			Implemented:         true,
			SupportsGroups:      false,
			SupportsPolicy:      false,
			SupportsScope:       false,
			SupportsMembership:  false,
			Status:              "coming_soon",
			RuntimeServiceCodes: []string{"openvpn"},
		},
		{
			ServiceCode:         "wireguard",
			DisplayName:         "WireGuard",
			Description:         "Runtime and client config generation are implemented. Client access group materialization is planned.",
			Category:            "vpn",
			Implemented:         true,
			SupportsGroups:      false,
			SupportsPolicy:      false,
			SupportsScope:       false,
			SupportsMembership:  false,
			Status:              "coming_soon",
			RuntimeServiceCodes: []string{"wireguard"},
		},
		{
			ServiceCode:         "l2tp",
			DisplayName:         "L2TP / IPsec",
			Description:         "Combined client access service backed by IPsec and XL2TPD runtime components. Group materialization is planned.",
			Category:            "vpn",
			Implemented:         true,
			SupportsGroups:      false,
			SupportsPolicy:      false,
			SupportsScope:       false,
			SupportsMembership:  false,
			Status:              "coming_soon",
			RuntimeServiceCodes: []string{"ipsec", "xl2tpd"},
		},
		{
			ServiceCode:         "http_proxy",
			DisplayName:         "HTTP Proxy",
			Description:         "Authenticated HTTP proxy runtime is available. Client access group materialization is planned.",
			Category:            "proxy",
			Implemented:         true,
			SupportsGroups:      false,
			SupportsPolicy:      false,
			SupportsScope:       false,
			SupportsMembership:  false,
			Status:              "coming_soon",
			RuntimeServiceCodes: []string{"http_proxy"},
		},
		{
			ServiceCode:         "shadowsocks",
			DisplayName:         "Shadowsocks",
			Description:         "Shadowsocks runtime support exists. Client access group materialization is planned.",
			Category:            "proxy",
			Implemented:         true,
			SupportsGroups:      false,
			SupportsPolicy:      false,
			SupportsScope:       false,
			SupportsMembership:  false,
			Status:              "coming_soon",
			RuntimeServiceCodes: []string{"shadowsocks"},
		},
		{
			ServiceCode:         "mtproto",
			DisplayName:         "MTProto",
			Description:         "MTProto runtime support exists. Client access group materialization is planned.",
			Category:            "proxy",
			Implemented:         true,
			SupportsGroups:      false,
			SupportsPolicy:      false,
			SupportsScope:       false,
			SupportsMembership:  false,
			Status:              "coming_soon",
			RuntimeServiceCodes: []string{"mtproto", "xray-core"},
		},
		{
			ServiceCode:             "socks_proxy",
			DisplayName:             "SOCKS Proxy",
			Description:             "Catalog placeholder for future SOCKS proxy support. No runtime materialization is available yet.",
			Category:                "proxy",
			Implemented:             false,
			SupportsGroups:          false,
			SupportsPolicy:          false,
			SupportsScope:           false,
			SupportsMembership:      false,
			SupportsMaterialization: false,
			Status:                  "planned",
		},
	}
}
