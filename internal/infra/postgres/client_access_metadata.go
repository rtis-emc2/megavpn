package postgres

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/rtis-emc2/megavpn/internal/domain"
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

func (s *Store) clientProvisioningServiceMetadata(ctx context.Context, instanceID string, options map[string]any) (map[string]any, error) {
	metadata, err := s.clientInboundServiceMetadata(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	inbound, _ := metadata["inbound_service"].(map[string]any)
	if normalizeCapabilityCode(firstNonEmptyRouteValue(stringify(inbound["service_code"]))) != "xray-core" {
		return metadata, nil
	}
	spec, err := s.latestInstanceSpec(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	spec, err = s.ensureXrayProvisioningGroups(ctx, instanceID, spec)
	if err != nil {
		return nil, err
	}
	groups := xrayVLESSGroups(spec)
	defaultGroup := xrayDefaultVLESSGroupKey(spec, groups)
	group := normalizeXrayVLESSGroupKey(firstNonEmptyRouteValue(
		stringify(options["vless_group"]),
		stringify(options["xray_group"]),
		stringify(options["outbound_group"]),
	))
	if group == "" {
		group = defaultGroup
	}
	allowed := xrayVLESSGroupKeySet(spec)
	if _, ok := allowed[group]; !ok {
		return nil, fmt.Errorf("vless outbound group %q is not defined for service instance %s", group, strings.TrimSpace(instanceID))
	}
	metadata["vless_group"] = group
	metadata["xray_group"] = group
	metadata["outbound_group"] = group
	if inbound == nil {
		inbound = map[string]any{}
	}
	inbound["vless_group"] = group
	inbound["xray_group"] = group
	inbound["outbound_group"] = group
	metadata["inbound_service"] = inbound
	return metadata, nil
}

func (s *Store) ensureXrayProvisioningGroups(ctx context.Context, instanceID string, spec map[string]any) (map[string]any, error) {
	enriched, changed, err := s.specWithVLESSGroupCatalog(ctx, spec)
	if err != nil {
		return nil, err
	}
	if !changed {
		return enriched, nil
	}
	revision, err := s.ReplaceInstanceSpec(ctx, instanceID, "system:vless-groups", enriched)
	if err != nil {
		return nil, fmt.Errorf("sync vless group catalog into service instance: %w", err)
	}
	if !in(strings.TrimSpace(revision.Status), "validated", "applied") {
		return nil, fmt.Errorf("vless group catalog revision is not apply-ready; status=%s", strings.TrimSpace(revision.Status))
	}
	return enriched, nil
}

func (s *Store) specWithVLESSGroupCatalog(ctx context.Context, spec map[string]any) (map[string]any, bool, error) {
	templates, err := s.ListVLESSGroupTemplates(ctx)
	if err != nil {
		return nil, false, err
	}
	if len(templates) == 0 {
		return spec, false, nil
	}
	enriched := cloneMap(spec)
	merged := make([]any, 0, len(templates)+len(xraySpecGroupItems(spec)))
	seen := map[string]struct{}{}
	add := func(raw any) {
		group, _ := cloneAny(raw).(map[string]any)
		if group == nil {
			return
		}
		key := normalizeXrayVLESSGroupKey(firstString(group["key"], group["name"], group["id"]))
		if key == "" {
			return
		}
		if _, exists := seen[key]; exists {
			return
		}
		group["key"] = key
		merged = append(merged, group)
		seen[key] = struct{}{}
	}
	for _, template := range templates {
		add(vlessTemplateSpecGroup(template))
	}
	for _, raw := range xraySpecGroupItems(spec) {
		add(raw)
	}
	if len(merged) == 0 {
		return spec, false, nil
	}
	defaultGroup := normalizeXrayVLESSGroupKey(firstString(spec["default_vless_group"], spec["default_xray_group"], spec["default_outbound_group"]))
	if _, ok := seen[defaultGroup]; defaultGroup == "" || !ok {
		if first, _ := merged[0].(map[string]any); first != nil {
			defaultGroup = normalizeXrayVLESSGroupKey(firstString(first["key"]))
		}
	}
	previousGroups := firstNonNil(spec["vless_groups"], spec["xray_groups"], spec["outbound_groups"])
	changed := !reflect.DeepEqual(previousGroups, merged)
	if normalizeXrayVLESSGroupKey(firstString(spec["default_vless_group"])) != defaultGroup {
		changed = true
	}
	enriched["vless_groups"] = merged
	enriched["default_vless_group"] = defaultGroup
	return enriched, changed, nil
}

func xraySpecGroupItems(spec map[string]any) []any {
	if spec == nil {
		return nil
	}
	if items := sliceRuleItemsFromAny(spec["vless_groups"]); len(items) > 0 {
		return items
	}
	if items := sliceRuleItemsFromAny(spec["xray_groups"]); len(items) > 0 {
		return items
	}
	return sliceRuleItemsFromAny(spec["outbound_groups"])
}

func vlessTemplateSpecGroup(template domain.VLESSGroupTemplate) map[string]any {
	group := map[string]any{
		"key":          normalizeXrayVLESSGroupKey(template.Key),
		"label":        strings.TrimSpace(template.Label),
		"access_mode":  strings.TrimSpace(template.AccessMode),
		"egress_mode":  strings.TrimSpace(template.EgressMode),
		"outbound_tag": strings.TrimSpace(template.OutboundTag),
	}
	if group["label"] == "" {
		group["label"] = group["key"]
	}
	if group["outbound_tag"] == "" {
		group["outbound_tag"] = "direct"
	}
	if description := strings.TrimSpace(template.Description); description != "" {
		group["description"] = description
	}
	if egressNodeID := strings.TrimSpace(template.EgressNodeID); egressNodeID != "" {
		group["egress_node_id"] = egressNodeID
	}
	if targetInstanceID := strings.TrimSpace(template.TargetInstanceID); targetInstanceID != "" {
		group["target_instance_id"] = targetInstanceID
	}
	if template.AdBlock {
		group["ad_block"] = true
	}
	if len(template.Rules) > 0 {
		group["rules"] = cloneVLESSGroupTemplateRules(template.Rules)
	}
	if len(template.ExtraRules) > 0 {
		group["extra_rules"] = cloneVLESSGroupTemplateRules(template.ExtraRules)
	}
	return group
}

func cloneVLESSGroupTemplateRules(rules []map[string]any) []any {
	out := make([]any, 0, len(rules))
	for _, rule := range rules {
		out = append(out, cloneMap(rule))
	}
	return out
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
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
