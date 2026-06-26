package postgres

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/netip"
	"strings"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

func (s *Store) materializeInstanceDriverSpecDefaults(ctx context.Context, instance domain.Instance, spec map[string]any) (map[string]any, error) {
	spec = cloneMap(spec)
	if hasExplicitInstanceConfig(spec) {
		return spec, nil
	}

	switch normalizeInstanceRuntimeCode(instance.ServiceCode) {
	case "xray-core":
		return s.materializeXrayDefaults(ctx, instance, spec)
	case "openvpn":
		return s.materializeOpenVPNDefaults(ctx, instance, spec)
	case "wireguard":
		return s.materializeWireGuardDefaults(ctx, instance, spec)
	case "xl2tpd":
		return s.materializeXL2TPDDefaults(ctx, instance, spec)
	case "shadowsocks":
		return s.materializeShadowsocksDefaults(ctx, instance, spec)
	default:
		return spec, nil
	}
}

func hasExplicitInstanceConfig(spec map[string]any) bool {
	return firstString(spec["config_content"]) != "" || spec["config_json"] != nil
}

func (s *Store) materializeXrayDefaults(ctx context.Context, instance domain.Instance, spec map[string]any) (map[string]any, error) {
	if !strings.EqualFold(firstString(spec["security"], "reality"), "reality") {
		return spec, nil
	}

	privateKeyRefID := firstString(spec["reality_private_key_secret_ref_id"])
	privateKey := firstString(spec["reality_private_key"])
	publicKey := firstString(spec["reality_public_key"])
	if privateKeyRefID == "" || publicKey == "" {
		if privateKey == "" || publicKey == "" {
			var err error
			privateKey, publicKey, err = generateXrayRealityKeyPair()
			if err != nil {
				return nil, err
			}
		}
		ref, err := s.CreateSecretRef(ctx, "private_key", []byte(privateKey), map[string]any{
			"scope":       "instance",
			"instance_id": instance.ID,
			"material":    "xray_reality_private_key",
		})
		if err != nil {
			return nil, err
		}
		spec["reality_private_key_secret_ref_id"] = ref.ID
		spec["reality_public_key"] = publicKey
		delete(spec, "reality_private_key")
	}
	if firstString(spec["short_id"]) == "" && len(stringList(spec["short_ids"])) == 0 {
		raw, err := randomBytes(4)
		if err != nil {
			return nil, err
		}
		spec["short_id"] = hex.EncodeToString(raw)
	}
	if firstString(spec["server_name"], spec["sni"]) == "" && len(stringList(spec["server_names"])) == 0 && instance.EndpointHost != "" {
		spec["server_name"] = instance.EndpointHost
	}
	return spec, nil
}

func (s *Store) materializeWireGuardDefaults(ctx context.Context, instance domain.Instance, spec map[string]any) (map[string]any, error) {
	privateKeyRefID := firstString(spec["server_private_key_secret_ref_id"])
	privateKey := firstString(spec["server_private_key"])
	publicKey := firstString(spec["server_public_key"])
	if privateKeyRefID == "" || publicKey == "" {
		if privateKey == "" || publicKey == "" {
			var err error
			privateKey, publicKey, err = generateX25519KeyPair()
			if err != nil {
				return nil, err
			}
		}
		ref, err := s.CreateSecretRef(ctx, "private_key", []byte(privateKey), map[string]any{
			"scope":       "instance",
			"instance_id": instance.ID,
			"material":    "wireguard_server_private_key",
		})
		if err != nil {
			return nil, err
		}
		spec["server_private_key_secret_ref_id"] = ref.ID
		spec["server_public_key"] = publicKey
		delete(spec, "server_private_key")
	}
	if shouldAllocateAddressPool(spec, "network_cidr") {
		allocation, ok, err := s.ensureInstanceAddressPoolAllocation(ctx, instance, "wireguard")
		if err != nil {
			return nil, err
		}
		if ok {
			spec["address_pool_allocation_id"] = allocation.ID
			spec["network_cidr"] = allocation.CIDR
			spec["route_export"] = allocation.RouteExport
		}
	}
	if firstString(spec["network_cidr"]) == "" {
		spec["network_cidr"] = "10.66.0.0/24"
	}
	if firstString(spec["server_address"]) == "" {
		address, err := wireGuardDefaultServerAddress(firstString(spec["network_cidr"]), firstIntValue(spec["server_host_index"], 1))
		if err != nil {
			return nil, err
		}
		spec["server_address"] = address
	}
	if firstIntValue(spec["listen_port"], spec["server_port"]) <= 0 {
		spec["listen_port"] = firstIntValue(instance.EndpointPort, 51820)
	}
	if firstString(spec["client_allowed_ips"]) == "" {
		spec["client_allowed_ips"] = "0.0.0.0/0, ::/0"
	}
	if firstIntValue(spec["persistent_keepalive"]) <= 0 {
		spec["persistent_keepalive"] = 25
	}
	return spec, nil
}

func (s *Store) materializeOpenVPNDefaults(ctx context.Context, instance domain.Instance, spec map[string]any) (map[string]any, error) {
	if shouldAllocateAddressPool(spec, "server_network") {
		allocation, ok, err := s.ensureInstanceAddressPoolAllocation(ctx, instance, "openvpn")
		if err != nil {
			return nil, err
		}
		if ok {
			network, netmask, err := ipv4NetworkAndNetmask(allocation.CIDR)
			if err != nil {
				return nil, err
			}
			spec["address_pool_allocation_id"] = allocation.ID
			spec["address_pool_cidr"] = allocation.CIDR
			spec["server_network"] = network
			spec["server_netmask"] = netmask
			spec["route_export"] = allocation.RouteExport
		}
	}
	if firstString(spec["server_network"]) == "" {
		spec["server_network"] = "10.8.0.0"
	}
	if firstString(spec["server_netmask"]) == "" {
		spec["server_netmask"] = "255.255.255.0"
	}
	if firstIntValue(spec["server_port"], spec["port"]) <= 0 {
		spec["server_port"] = firstIntValue(instance.EndpointPort, 1194)
	}
	if firstString(spec["proto"]) == "" {
		spec["proto"] = "tcp"
	}
	if firstString(spec["dev"]) == "" {
		spec["dev"] = "tun"
	}
	return spec, nil
}

func (s *Store) materializeXL2TPDDefaults(ctx context.Context, instance domain.Instance, spec map[string]any) (map[string]any, error) {
	if shouldAllocateAddressPool(spec, "local_ip", "ip_range_start", "ip_range_end") {
		allocation, ok, err := s.ensureInstanceAddressPoolAllocation(ctx, instance, "xl2tpd")
		if err != nil {
			return nil, err
		}
		if ok {
			localIP, rangeStart, rangeEnd, err := l2tpPoolFromPrefix(allocation.CIDR)
			if err != nil {
				return nil, err
			}
			spec["address_pool_allocation_id"] = allocation.ID
			spec["address_pool_cidr"] = allocation.CIDR
			spec["local_ip"] = localIP
			spec["ip_range_start"] = rangeStart
			spec["ip_range_end"] = rangeEnd
			spec["route_export"] = allocation.RouteExport
		}
	}
	if firstString(spec["local_ip"]) == "" {
		spec["local_ip"] = "10.20.0.1"
	}
	if firstString(spec["ip_range_start"]) == "" {
		spec["ip_range_start"] = "10.20.0.10"
	}
	if firstString(spec["ip_range_end"]) == "" {
		spec["ip_range_end"] = "10.20.0.200"
	}
	return spec, nil
}

func (s *Store) materializeShadowsocksDefaults(ctx context.Context, instance domain.Instance, spec map[string]any) (map[string]any, error) {
	passwordRefID := firstString(spec["server_password_secret_ref_id"], spec["password_secret_ref_id"])
	password := firstString(spec["server_password"], spec["password"])
	if passwordRefID == "" {
		if password == "" {
			var err error
			password, err = randomBase64(32)
			if err != nil {
				return nil, err
			}
		}
		ref, err := s.CreateSecretRef(ctx, "password", []byte(password), map[string]any{
			"scope":       "instance",
			"instance_id": instance.ID,
			"material":    "shadowsocks_server_password",
		})
		if err != nil {
			return nil, err
		}
		spec["server_password_secret_ref_id"] = ref.ID
		delete(spec, "password")
		delete(spec, "server_password")
	}
	if firstString(spec["listen"], spec["server"]) == "" {
		spec["listen"] = "0.0.0.0"
	}
	if firstString(spec["method"]) == "" {
		spec["method"] = "chacha20-ietf-poly1305"
	}
	if firstString(spec["mode"]) == "" {
		spec["mode"] = "tcp_and_udp"
	}
	if firstIntValue(spec["timeout"]) <= 0 {
		spec["timeout"] = 300
	}
	return spec, nil
}

func shouldAllocateAddressPool(spec map[string]any, fields ...string) bool {
	mode := strings.ToLower(strings.TrimSpace(firstString(spec["address_pool_mode"], spec["pool_mode"])))
	if mode == "manual" || mode == "static" || mode == "disabled" {
		return false
	}
	if mode == "auto" {
		return true
	}
	for _, field := range fields {
		if firstString(spec[field]) == "" {
			return true
		}
	}
	return false
}

func wireGuardDefaultServerAddress(cidr string, hostIndex int) (string, error) {
	prefix, err := netip.ParsePrefix(firstString(cidr, "10.66.0.0/24"))
	if err != nil {
		return "", fmt.Errorf("wireguard network_cidr is invalid: %w", err)
	}
	if !prefix.Addr().Is4() {
		return "", fmt.Errorf("wireguard network_cidr must be IPv4 for the current driver")
	}
	if hostIndex <= 0 {
		hostIndex = 1
	}
	base := prefix.Addr().As4()
	addr := netip.AddrFrom4([4]byte{
		base[0],
		base[1],
		base[2],
		byte(int(base[3]) + hostIndex),
	})
	if !prefix.Contains(addr) {
		return "", fmt.Errorf("wireguard server host index %d is outside of prefix %s", hostIndex, prefix.String())
	}
	return fmt.Sprintf("%s/%d", addr.String(), prefix.Bits()), nil
}
