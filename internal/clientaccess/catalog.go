package clientaccess

import "github.com/rtis-emc2/megavpn/internal/domain"

var catalog = []domain.ClientAccessService{
	{
		ServiceCode:             "vless",
		DisplayName:             "VLESS / Xray",
		Description:             "VLESS client membership, runtime access and route policy management.",
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
	readyService("openvpn", "OpenVPN", "vpn", []string{"openvpn"}, "OpenVPN client membership, runtime access and configuration generation."),
	readyService("wireguard", "WireGuard", "vpn", []string{"wireguard"}, "WireGuard client membership, runtime peer access and configuration generation."),
	readyService("l2tp", "L2TP / IPsec", "vpn", []string{"ipsec"}, "L2TP/IPsec client membership, runtime access and credential package generation."),
	readyService("http_proxy", "HTTP Proxy", "proxy", []string{"http_proxy"}, "Authenticated HTTP proxy client membership and credential generation."),
	readyService("shadowsocks", "Shadowsocks", "proxy", []string{"shadowsocks"}, "Shadowsocks client membership, managed accounts and configuration generation."),
	readyService("mtproto", "MTProto", "proxy", []string{"mtproto"}, "MTProto client membership, runtime access and configuration generation."),
	{
		ServiceCode:             "socks_proxy",
		DisplayName:             "SOCKS Proxy",
		Description:             "SOCKS proxy runtime and client access materialization are not available yet.",
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

func readyService(code, name, category string, runtimeCodes []string, description string) domain.ClientAccessService {
	return domain.ClientAccessService{
		ServiceCode:             code,
		DisplayName:             name,
		Description:             description,
		Category:                category,
		Implemented:             true,
		SupportsGroups:          true,
		SupportsPolicy:          false,
		SupportsScope:           true,
		SupportsMembership:      true,
		SupportsMaterialization: true,
		Status:                  "active",
		RuntimeServiceCodes:     runtimeCodes,
	}
}

// Catalog returns a deep-enough copy so API callers cannot mutate the shared
// capability source through slices or nested policy capability maps.
func Catalog() []domain.ClientAccessService {
	out := make([]domain.ClientAccessService, len(catalog))
	for index, service := range catalog {
		out[index] = service
		out[index].RuntimeServiceCodes = append([]string(nil), service.RuntimeServiceCodes...)
		out[index].PolicyCapabilities = cloneMap(service.PolicyCapabilities)
	}
	return out
}

func Find(serviceCode string) (domain.ClientAccessService, bool) {
	for _, service := range catalog {
		if service.ServiceCode == serviceCode {
			return service, true
		}
	}
	return domain.ClientAccessService{}, false
}

func MaterializationReady(serviceCode string) bool {
	service, ok := Find(serviceCode)
	return ok && service.Status == "active" && service.SupportsGroups && service.SupportsMembership && service.SupportsMaterialization
}

func cloneMap(source map[string]any) map[string]any {
	if source == nil {
		return nil
	}
	out := make(map[string]any, len(source))
	for key, value := range source {
		switch typed := value.(type) {
		case map[string]any:
			out[key] = cloneMap(typed)
		case map[string]bool:
			copyValue := make(map[string]bool, len(typed))
			for childKey, childValue := range typed {
				copyValue[childKey] = childValue
			}
			out[key] = copyValue
		case []string:
			out[key] = append([]string(nil), typed...)
		default:
			out[key] = value
		}
	}
	return out
}
