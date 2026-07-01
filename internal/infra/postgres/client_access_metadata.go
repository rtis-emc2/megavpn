package postgres

import (
	"context"
	"fmt"
	"strings"
)

func (s *Store) clientInboundServiceMetadata(ctx context.Context, instanceID string) (map[string]any, error) {
	instanceID = strings.TrimSpace(instanceID)
	if instanceID == "" {
		return nil, fmt.Errorf("instance id is required")
	}

	var instanceName, instanceSlug, instanceStatus, endpointHost string
	var serviceCode, serviceName, serviceCategory string
	var nodeID, nodeName, nodeRole, nodeAddress string
	var endpointPort int
	var enabled bool
	err := s.db.QueryRow(ctx, `select
			i.id::text,
			i.name,
			i.slug,
			i.status,
			i.enabled,
			coalesce(i.endpoint_host,''),
			coalesce(i.endpoint_port,0),
			sd.code,
			sd.name,
			coalesce(sd.category,''),
			n.id::text,
			n.name,
			n.role,
			n.address
		from instances i
		join service_definitions sd on sd.id=i.service_definition_id
		join nodes n on n.id=i.node_id
		where i.id=$1`, instanceID).Scan(
		&instanceID,
		&instanceName,
		&instanceSlug,
		&instanceStatus,
		&enabled,
		&endpointHost,
		&endpointPort,
		&serviceCode,
		&serviceName,
		&serviceCategory,
		&nodeID,
		&nodeName,
		&nodeRole,
		&nodeAddress,
	)
	if err != nil {
		return nil, err
	}

	serviceCode = normalizeCapabilityCode(serviceCode)
	endpoint := strings.TrimSpace(endpointHost)
	if endpointPort > 0 {
		if endpoint != "" {
			endpoint = fmt.Sprintf("%s:%d", endpoint, endpointPort)
		} else {
			endpoint = fmt.Sprintf(":%d", endpointPort)
		}
	}
	availability := "available"
	if !enabled || strings.EqualFold(instanceStatus, "deleted") || strings.EqualFold(instanceStatus, "disabled") {
		availability = "disabled"
	} else if strings.EqualFold(instanceStatus, "failed") || strings.EqualFold(instanceStatus, "degraded") {
		availability = "attention_required"
	}

	inbound := map[string]any{
		"available":         availability == "available",
		"availability":      availability,
		"service_code":      serviceCode,
		"service_label":     firstNonEmptyRouteValue(serviceName, serviceCode),
		"service_category":  strings.TrimSpace(serviceCategory),
		"instance_id":       instanceID,
		"instance_name":     strings.TrimSpace(instanceName),
		"instance_slug":     strings.TrimSpace(instanceSlug),
		"instance_status":   strings.TrimSpace(instanceStatus),
		"instance_enabled":  enabled,
		"node_id":           strings.TrimSpace(nodeID),
		"node_name":         strings.TrimSpace(nodeName),
		"node_role":         strings.TrimSpace(nodeRole),
		"node_address":      strings.TrimSpace(nodeAddress),
		"endpoint_host":     strings.TrimSpace(endpointHost),
		"endpoint_port":     endpointPort,
		"endpoint":          endpoint,
		"route_source_type": routeSourceTypeForService(serviceCode),
	}

	return map[string]any{
		"available_inbound": availability == "available",
		"inbound_service":   inbound,
	}, nil
}

func routeSourceTypeForService(serviceCode string) string {
	switch normalizeCapabilityCode(serviceCode) {
	case "wireguard":
		return "wireguard_ip"
	case "openvpn":
		return "openvpn_common_name"
	case "xray-core":
		return "xray_uuid"
	case "http_proxy":
		return "http_proxy_username"
	case "ipsec", "xl2tpd":
		return "ppp_username"
	case "shadowsocks":
		return "shadowsocks_access"
	case "mtproto":
		return "mtproto_secret"
	default:
		return "unknown"
	}
}
