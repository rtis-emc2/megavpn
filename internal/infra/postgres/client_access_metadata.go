package postgres

import (
	"context"
	"fmt"
	"reflect"
	"sort"
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
		"service_label":     firstString(serviceName, serviceCode),
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

func (s *Store) clientProvisioningServiceMetadata(ctx context.Context, clientID, instanceID string, options map[string]any) (map[string]any, error) {
	metadata, err := s.clientInboundServiceMetadata(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	inbound, _ := metadata["inbound_service"].(map[string]any)
	if normalizeCapabilityCode(firstString(inbound["service_code"])) != "xray-core" {
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
	applyXrayPublicClientEndpointMetadata(metadata, inbound, spec)
	profileKey := xrayClientIdentityProfileKey(metadata)
	metadata["xray_identity_key"] = profileKey
	forceNewUUID, err := s.serviceAccessForcesNewXrayUUID(ctx, clientID, instanceID)
	if err != nil {
		return nil, err
	}
	if !forceNewUUID {
		reusableUUID, err := lookupXrayClientIdentityUUIDTx(ctx, s.db, clientID, profileKey)
		if err != nil {
			return nil, err
		}
		if reusableUUID == "" {
			reusableUUID, err = s.lookupReusableXrayClientUUID(ctx, clientID, "")
			if err != nil {
				return nil, err
			}
		}
		if reusableUUID != "" {
			metadata["xray_uuid"] = reusableUUID
		}
	}
	groups := xrayVLESSGroups(spec)
	defaultGroup := xrayDefaultVLESSGroupKey(spec, groups)
	allowed := xrayVLESSGroupKeySet(spec)
	requestedGroup := xrayVLESSGroupFromOptions(options)
	existingGroup, err := s.existingClientXrayProvisioningGroup(ctx, clientID, instanceID)
	if err != nil {
		return nil, err
	}
	group, err := selectXrayProvisioningGroup(requestedGroup, existingGroup, defaultGroup, groups, allowed, requestedGroup != "", instanceID)
	if err != nil {
		return nil, err
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

func (s *Store) existingClientXrayProvisioningGroup(ctx context.Context, clientID, instanceID string) (string, error) {
	clientID = strings.TrimSpace(clientID)
	instanceID = strings.TrimSpace(instanceID)
	if clientID == "" || instanceID == "" {
		return "", nil
	}
	rows, err := s.db.Query(ctx, `select metadata_json,policy_json
		from service_accesses
		where client_account_id=$1 and instance_id=$2
		order by updated_at desc
		limit 1`, clientID, instanceID)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	if !rows.Next() {
		return "", rows.Err()
	}
	var metadataRaw, policyRaw []byte
	if err := rows.Scan(&metadataRaw, &policyRaw); err != nil {
		return "", err
	}
	metadata := map[string]any{}
	policy := map[string]any{}
	if err := decodeJSONField(metadataRaw, &metadata, "service_accesses.metadata_json"); err != nil {
		return "", err
	}
	if err := decodeJSONField(policyRaw, &policy, "service_accesses.policy_json"); err != nil {
		return "", err
	}
	return firstString(
		xrayVLESSGroupFromMetadata(metadata),
		xrayVLESSGroupFromMetadata(policy),
	), rows.Err()
}

func xrayVLESSGroupFromOptions(options map[string]any) string {
	if options == nil {
		return ""
	}
	return normalizeXrayVLESSGroupKey(firstString(
		options["vless_group"],
		options["xray_group"],
		options["outbound_group"],
		options["group"],
	))
}

func xrayVLESSGroupFromMetadata(metadata map[string]any) string {
	if metadata == nil {
		return ""
	}
	inbound := mapFromAny(metadata["inbound_service"])
	return normalizeXrayVLESSGroupKey(firstString(
		metadata["vless_group"],
		metadata["xray_group"],
		metadata["outbound_group"],
		metadata["group"],
		inbound["vless_group"],
		inbound["xray_group"],
		inbound["outbound_group"],
		inbound["group"],
	))
}

func selectXrayProvisioningGroup(requestedGroup, existingGroup, defaultGroup string, groups []xrayVLESSGroup, allowed map[string]struct{}, strictRequested bool, instanceID string) (string, error) {
	requestedGroup = normalizeXrayVLESSGroupKey(requestedGroup)
	existingGroup = normalizeXrayVLESSGroupKey(existingGroup)
	defaultGroup = normalizeXrayVLESSGroupKey(defaultGroup)
	group := firstString(requestedGroup, existingGroup, defaultGroup)
	if _, ok := allowed[group]; ok && group != "" {
		return group, nil
	}
	if strictRequested && requestedGroup != "" {
		return "", fmt.Errorf("vless outbound group %q is not available for service instance %s after catalog sync; available groups: %s", requestedGroup, strings.TrimSpace(instanceID), strings.Join(sortedXrayVLESSGroupKeys(allowed), ", "))
	}
	if fallback := fallbackXrayVLESSGroupKey(defaultGroup, groups, allowed); fallback != "" {
		return fallback, nil
	}
	return "", fmt.Errorf("no vless outbound groups are available for service instance %s after catalog sync", strings.TrimSpace(instanceID))
}

func fallbackXrayVLESSGroupKey(defaultGroup string, groups []xrayVLESSGroup, allowed map[string]struct{}) string {
	for _, key := range []string{normalizeXrayVLESSGroupKey(defaultGroup), "default"} {
		if key == "" {
			continue
		}
		if _, ok := allowed[key]; ok {
			return key
		}
	}
	for _, group := range groups {
		if _, ok := allowed[group.Key]; ok && group.Key != "" {
			return group.Key
		}
	}
	keys := sortedXrayVLESSGroupKeys(allowed)
	if len(keys) > 0 {
		return keys[0]
	}
	return ""
}

func sortedXrayVLESSGroupKeys(set map[string]struct{}) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		if strings.TrimSpace(key) != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

func applyXrayPublicClientEndpointMetadata(metadata, inbound, spec map[string]any) {
	if metadata == nil || inbound == nil || spec == nil {
		return
	}
	hasPublicProfile := firstString(
		spec["public_host"],
		spec["public_security"],
		spec["public_network"],
		spec["public_path"],
		spec["public_host_header"],
		spec["public_sni"],
		spec["public_server_name"],
	) != "" || firstIntValue(spec["public_port"]) > 0
	if !hasPublicProfile {
		return
	}

	host := firstString(
		spec["public_host"],
		spec["server_host"],
		inbound["endpoint_host"],
		spec["public_host_header"],
	)
	port := firstIntValue(spec["public_port"], spec["server_port"], inbound["endpoint_port"])
	if host == "" || port <= 0 {
		return
	}

	endpoint := clientEndpointString(host, port)
	backendEndpoint := firstString(inbound["backend_endpoint"], inbound["endpoint"])
	inbound["client_endpoint_host"] = host
	inbound["client_endpoint_port"] = port
	inbound["client_endpoint"] = endpoint
	inbound["endpoint_kind"] = "public"
	if backendEndpoint != "" && backendEndpoint != endpoint {
		inbound["backend_endpoint"] = backendEndpoint
	}
	inbound["endpoint"] = endpoint

	metadata["public_host"] = host
	metadata["public_port"] = port
	for _, key := range []string{
		"public_security",
		"public_network",
		"public_path",
		"public_host_header",
		"public_sni",
		"public_server_name",
		"public_service_name",
		"public_fingerprint",
		"public_alpn",
		"public_flow",
	} {
		if value := firstString(spec[key]); value != "" {
			metadata[key] = value
		}
	}
}

func clientEndpointString(host string, port int) string {
	host = strings.TrimSpace(host)
	if port > 0 {
		if host != "" {
			return fmt.Sprintf("%s:%d", host, port)
		}
		return fmt.Sprintf(":%d", port)
	}
	return host
}

func (s *Store) ensureXrayProvisioningGroups(ctx context.Context, instanceID string, spec map[string]any) (map[string]any, error) {
	instance, err := s.GetInstance(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	catalog, err := s.loadVLESSGroupCatalogSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	enriched, changed, err := s.syncXrayVLESSGroupCatalogSpec(ctx, instance, spec, catalog)
	if err != nil {
		return nil, fmt.Errorf("prepare vless group catalog for service instance: %w", err)
	}
	if !changed {
		return enriched, nil
	}
	materialized, err := s.materializeInstanceDriverSpecDefaults(ctx, instance, enriched)
	if err != nil {
		return nil, fmt.Errorf("materialize vless group catalog for service instance: %w", err)
	}
	status, _, validationErrors := s.validateInstanceRevisionSpec(ctx, instance, materialized)
	if !in(status, "validated", "applied") {
		return nil, fmt.Errorf("vless group catalog revision is not apply-ready; status=%s errors=%v", status, validationErrors)
	}
	revision, err := s.ReplaceInstanceSpec(ctx, instanceID, "system:vless-groups", materialized)
	if err != nil {
		return nil, fmt.Errorf("sync vless group catalog into service instance: %w", err)
	}
	if !in(strings.TrimSpace(revision.Status), "validated", "applied") {
		return nil, fmt.Errorf("vless group catalog revision is not apply-ready; status=%s errors=%v", strings.TrimSpace(revision.Status), revision.ValidationErrors)
	}
	return revision.Spec, nil
}

func (s *Store) SyncVLESSGroupCatalog(ctx context.Context) (domain.VLESSGroupCatalogSyncResult, error) {
	catalog, err := s.loadVLESSGroupCatalogSnapshot(ctx)
	if err != nil {
		return domain.VLESSGroupCatalogSyncResult{}, err
	}
	instances, err := s.listXrayVLESSGroupSyncInstances(ctx)
	if err != nil {
		return domain.VLESSGroupCatalogSyncResult{}, err
	}
	result := domain.VLESSGroupCatalogSyncResult{}
	for _, instance := range instances {
		result.ScannedInstances++
		record := domain.VLESSGroupCatalogSyncInstance{
			InstanceID: instance.ID,
			NodeID:     instance.NodeID,
			Status:     instance.Status,
		}
		next, changed, err := s.syncXrayVLESSGroupCatalogSpec(ctx, instance, instance.Spec, catalog)
		if err != nil {
			result.FailedInstances = append(result.FailedInstances, domain.VLESSGroupCatalogSyncFailure{
				InstanceID: instance.ID,
				NodeID:     instance.NodeID,
				Stage:      "sync_spec",
				Error:      err.Error(),
			})
			continue
		}
		if !changed {
			result.UnchangedInstances++
			continue
		}
		materialized, err := s.materializeInstanceDriverSpecDefaults(ctx, instance, next)
		if err != nil {
			result.FailedInstances = append(result.FailedInstances, domain.VLESSGroupCatalogSyncFailure{
				InstanceID: instance.ID,
				NodeID:     instance.NodeID,
				Stage:      "materialize_defaults",
				Error:      err.Error(),
			})
			continue
		}
		status, _, validationErrors := s.validateInstanceRevisionSpec(ctx, instance, materialized)
		if !in(status, "validated", "applied") {
			result.FailedInstances = append(result.FailedInstances, domain.VLESSGroupCatalogSyncFailure{
				InstanceID: instance.ID,
				NodeID:     instance.NodeID,
				Stage:      "validate_revision",
				Error:      fmt.Sprintf("vless group catalog revision is not apply-ready; status=%s errors=%v", status, validationErrors),
			})
			continue
		}
		revision, err := s.ReplaceInstanceSpec(ctx, instance.ID, "system:vless-group-catalog-sync", materialized)
		if err != nil {
			result.FailedInstances = append(result.FailedInstances, domain.VLESSGroupCatalogSyncFailure{
				InstanceID: instance.ID,
				NodeID:     instance.NodeID,
				Stage:      "create_revision",
				Error:      err.Error(),
			})
			continue
		}
		if !in(strings.TrimSpace(revision.Status), "validated", "applied") {
			result.FailedInstances = append(result.FailedInstances, domain.VLESSGroupCatalogSyncFailure{
				InstanceID: instance.ID,
				NodeID:     instance.NodeID,
				Stage:      "create_revision",
				Error:      fmt.Sprintf("vless group catalog revision is not apply-ready; status=%s errors=%v", strings.TrimSpace(revision.Status), revision.ValidationErrors),
			})
			continue
		}
		result.ChangedInstances++
		record.Changed = true
		record.RevisionID = revision.ID
		if ok, reason := shouldQueueVLESSGroupCatalogApply(instance); !ok {
			record.Skipped = true
			record.Reason = reason
			result.SkippedApplyJobs++
			result.Instances = append(result.Instances, record)
			continue
		}
		job, err := s.UpdateInstanceStatus(ctx, instance.ID, "apply")
		if err != nil {
			result.FailedInstances = append(result.FailedInstances, domain.VLESSGroupCatalogSyncFailure{
				InstanceID: instance.ID,
				NodeID:     instance.NodeID,
				Stage:      "queue_apply",
				Error:      err.Error(),
			})
			record.Skipped = true
			record.Reason = "apply queue failed: " + err.Error()
			result.Instances = append(result.Instances, record)
			continue
		}
		record.ApplyQueued = true
		record.ApplyJobID = job.ID
		record.ApplyJobType = job.Type
		result.QueuedApplyJobs++
		result.Instances = append(result.Instances, record)
	}
	if result.ScannedInstances > 0 || len(result.FailedInstances) > 0 {
		_, _ = s.CreateAudit(ctx, "system", "vless_group.catalog_sync", "vless_group", nil, fmt.Sprintf("vless group catalog sync: changed=%d queued=%d failed=%d", result.ChangedInstances, result.QueuedApplyJobs, len(result.FailedInstances)))
	}
	return result, nil
}

func (s *Store) listXrayVLESSGroupSyncInstances(ctx context.Context) ([]domain.Instance, error) {
	rows, err := s.db.Query(ctx, `select
		i.id,
		i.node_id,
		sd.code,
		i.name,
		i.slug,
		coalesce(i.systemd_unit,''),
		i.status,
		i.enabled,
		coalesce(i.endpoint_host,''),
		coalesce(i.endpoint_port,0),
		i.current_revision_id,
		i.last_applied_revision_id,
		i.created_at,
		i.updated_at,
		coalesce(r.spec_json,'{}'::jsonb)
	from instances i
	join service_definitions sd on sd.id=i.service_definition_id
	left join lateral (
		select spec_json
		from instance_revisions
		where instance_id=i.id
		order by revision_no desc
		limit 1
	) r on true
	where i.status <> 'deleted'
	  and sd.code='xray-core'
	order by i.created_at desc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.Instance, 0)
	for rows.Next() {
		var instance domain.Instance
		var specRaw []byte
		if err := rows.Scan(
			&instance.ID,
			&instance.NodeID,
			&instance.ServiceCode,
			&instance.Name,
			&instance.Slug,
			&instance.SystemdUnit,
			&instance.Status,
			&instance.Enabled,
			&instance.EndpointHost,
			&instance.EndpointPort,
			&instance.CurrentRevisionID,
			&instance.LastAppliedRevisionID,
			&instance.CreatedAt,
			&instance.UpdatedAt,
			&specRaw,
		); err != nil {
			return nil, err
		}
		if err := decodeJSONField(specRaw, &instance.Spec, "instance_revisions.spec_json"); err != nil {
			return nil, err
		}
		if instance.Spec == nil {
			instance.Spec = map[string]any{}
		}
		out = append(out, instance)
	}
	return out, rows.Err()
}

func (s *Store) syncXrayVLESSGroupCatalogSpec(ctx context.Context, instance domain.Instance, spec map[string]any, catalog vlessGroupCatalogSnapshot) (map[string]any, bool, error) {
	original := cloneMap(spec)
	enriched, _ := specWithVLESSGroupCatalogSnapshot(spec, catalog)
	if _, err := s.resolveXrayDefaultEgress(ctx, instance, enriched, "control-plane:vless-group-catalog-sync"); err != nil {
		return nil, false, err
	}
	if _, err := s.resolveXrayVLESSGroupEgress(ctx, instance, enriched); err != nil {
		return nil, false, err
	}
	return enriched, !reflect.DeepEqual(original, enriched), nil
}

func shouldQueueVLESSGroupCatalogApply(instance domain.Instance) (bool, string) {
	if !instance.Enabled {
		return false, "instance is disabled"
	}
	switch strings.ToLower(strings.TrimSpace(instance.Status)) {
	case "", "active", "degraded", "failed", "provisioning":
		return true, ""
	case "draft":
		return false, "instance is still a draft"
	case "disabled":
		return false, "instance status is disabled"
	case "deleting", "deleted":
		return false, "instance is deleting or deleted"
	default:
		return true, ""
	}
}

type xrayVLESSEgressResolver func(context.Context, string, string) (XrayVLESSEgressResolution, error)

func (s *Store) resolveXrayVLESSGroupEgress(ctx context.Context, instance domain.Instance, spec map[string]any) (bool, error) {
	return resolveXrayVLESSGroupEgressWithResolver(ctx, instance, spec, s.ResolveXrayVLESSEgress, "control-plane:vless-group-catalog-sync")
}

func (s *Store) resolveXrayDefaultEgress(ctx context.Context, instance domain.Instance, spec map[string]any, resolvedBy string) (bool, error) {
	return resolveXrayDefaultEgressWithResolver(ctx, instance, spec, s.ResolveXrayVLESSEgress, resolvedBy)
}

func resolveXrayDefaultEgressWithResolver(ctx context.Context, instance domain.Instance, spec map[string]any, resolver xrayVLESSEgressResolver, resolvedBy string) (bool, error) {
	if spec == nil {
		return false, nil
	}
	if resolver == nil {
		return false, fmt.Errorf("xray default egress resolver is required")
	}
	egress, _ := cloneAny(spec["xray_egress"]).(map[string]any)
	if egress == nil {
		egress = map[string]any{}
	}
	mode := strings.ToLower(firstString(
		spec["egress_mode"],
		spec["xray_egress_mode"],
		spec["vless_egress_mode"],
		egress["mode"],
		egress["egress_mode"],
	))
	egressNodeID := firstString(
		spec["egress_node_id"],
		spec["xray_egress_node_id"],
		spec["vless_egress_node_id"],
		egress["egress_node_id"],
		egress["node_id"],
	)
	hasResolvedEgress := firstString(
		egress["send_through"],
		egress["sendThrough"],
		egress["link_id"],
		egress["transport_id"],
	) != ""
	if mode == "" && egressNodeID == "" && !hasResolvedEgress {
		return false, nil
	}
	if mode == "" || mode == "auto" || mode == "node" || mode == "remote_node" || mode == "remote_egress" {
		mode = "egress_node"
	}
	changed := false
	switch mode {
	case "local", "direct", "local_breakout", "current_node":
		next := map[string]any{
			"mode":            "local_breakout",
			"current_node_id": instance.NodeID,
		}
		if setVLESSGroupMapValue(spec, "xray_egress", next) {
			changed = true
		}
		if _, ok := spec["xray_default_outbound"]; ok {
			delete(spec, "xray_default_outbound")
			changed = true
		}
		if _, ok := spec["default_xray_outbound"]; ok {
			delete(spec, "default_xray_outbound")
			changed = true
		}
		return changed, nil
	case "egress_node":
	default:
		return changed, fmt.Errorf("unsupported xray egress_mode %q", mode)
	}
	resolved, err := resolver(ctx, instance.ID, egressNodeID)
	if err != nil {
		return changed, fmt.Errorf("xray default egress resolution failed: %w", err)
	}
	if setVLESSGroupMapValue(spec, "xray_egress", resolved.Map()) {
		changed = true
	}
	if resolved.Mode == "local_breakout" {
		if _, ok := spec["xray_default_outbound"]; ok {
			delete(spec, "xray_default_outbound")
			changed = true
		}
		if _, ok := spec["default_xray_outbound"]; ok {
			delete(spec, "default_xray_outbound")
			changed = true
		}
		return changed, nil
	}
	if resolvedBy = strings.TrimSpace(resolvedBy); resolvedBy != "" {
		next, _ := cloneAny(spec["xray_egress"]).(map[string]any)
		if next == nil {
			next = resolved.Map()
		}
		next["egress_resolved_by"] = resolvedBy
		if setVLESSGroupMapValue(spec, "xray_egress", next) {
			changed = true
		}
	}
	outbound := map[string]any{
		"tag":         "egress-default",
		"protocol":    "freedom",
		"sendThrough": resolved.SendThrough,
		"settings": map[string]any{
			"domainStrategy": "UseIP",
		},
	}
	if setVLESSGroupMapValue(spec, "xray_default_outbound", outbound) {
		changed = true
	}
	if _, ok := spec["default_xray_outbound"]; ok {
		delete(spec, "default_xray_outbound")
		changed = true
	}
	return changed, nil
}

func resolveXrayVLESSGroupEgressWithResolver(ctx context.Context, instance domain.Instance, spec map[string]any, resolver xrayVLESSEgressResolver, resolvedBy string) (bool, error) {
	groupsKey, groups := xrayMutableSpecGroupItems(spec)
	if len(groups) == 0 {
		return false, nil
	}
	if resolver == nil {
		return false, fmt.Errorf("xray vless egress resolver is required")
	}
	resolvedBy = strings.TrimSpace(resolvedBy)
	if resolvedBy == "" {
		resolvedBy = "control-plane:vless-group-catalog-sync"
	}
	changed := false
	resolvedByEgressNode := map[string]XrayVLESSEgressResolution{}
	updated := make([]any, 0, len(groups))
	for idx, item := range groups {
		group, _ := cloneAny(item).(map[string]any)
		if group == nil {
			updated = append(updated, item)
			continue
		}
		groupKey := normalizeXrayVLESSGroupKey(firstString(group["key"], group["name"], group["id"]))
		if groupKey == "" {
			groupKey = fmt.Sprintf("group-%d", idx+1)
			if setVLESSGroupMapValue(group, "key", groupKey) {
				changed = true
			}
		}
		mode := strings.ToLower(firstString(
			group["egress_mode"],
			group["access_mode"],
			group["mode"],
			"default",
		))
		switch mode {
		case "", "auto", "default", "instance_default", "inherit":
			if clearResolvedXrayVLESSGroupEgress(group) {
				changed = true
			}
		case "local", "direct", "local_breakout", "current_node":
			if setVLESSGroupMapValue(group, "egress_mode", "local_breakout") {
				changed = true
			}
			if setVLESSGroupMapValue(group, "outbound_tag", "direct") {
				changed = true
			}
			if clearResolvedXrayVLESSGroupEgress(group) {
				changed = true
			}
		case "block", "blocked", "deny":
			if setVLESSGroupMapValue(group, "outbound_tag", "block") {
				changed = true
			}
			if clearResolvedXrayVLESSGroupEgress(group) {
				changed = true
			}
		case "instance_only", "allow_instance", "target_instance":
			if clearResolvedXrayVLESSGroupEgress(group) {
				changed = true
			}
		case "egress_node", "remote_node", "remote_egress", "node":
			egressNodeID := firstString(
				group["egress_node_id"],
				group["node_id"],
				nestedVLESSGroupMap(group, "egress")["egress_node_id"],
				nestedVLESSGroupMap(group, "egress")["node_id"],
			)
			if egressNodeID == "" {
				return changed, fmt.Errorf("vless group %q requires egress_node_id", groupKey)
			}
			resolved, ok := resolvedByEgressNode[egressNodeID]
			if !ok {
				var err error
				resolved, err = resolver(ctx, instance.ID, egressNodeID)
				if err != nil {
					return changed, fmt.Errorf("vless group %q egress resolution failed: %w", groupKey, err)
				}
				resolvedByEgressNode[egressNodeID] = resolved
			}
			if setVLESSGroupMapValue(group, "egress_mode", "egress_node") {
				changed = true
			}
			if setVLESSGroupMapValue(group, "egress_node_id", egressNodeID) {
				changed = true
			}
			if setVLESSGroupMapValue(group, "egress", resolved.Map()) {
				changed = true
			}
			if setVLESSGroupMapValue(group, "egress_resolved_by", resolvedBy) {
				changed = true
			}
			if resolved.Mode == "local_breakout" {
				if _, ok := group["outbound"]; ok {
					delete(group, "outbound")
					changed = true
				}
				if setVLESSGroupMapValue(group, "outbound_tag", "direct") {
					changed = true
				}
				updated = append(updated, group)
				continue
			}
			tag := normalizeXrayOutboundTag("egress-" + groupKey)
			if tag == "" {
				tag = fmt.Sprintf("egress-group-%d", idx+1)
			}
			outbound := map[string]any{
				"tag":         tag,
				"protocol":    "freedom",
				"sendThrough": resolved.SendThrough,
				"settings": map[string]any{
					"domainStrategy": "UseIP",
				},
			}
			if setVLESSGroupMapValue(group, "outbound_tag", tag) {
				changed = true
			}
			if setVLESSGroupMapValue(group, "outbound", outbound) {
				changed = true
			}
		default:
			return changed, fmt.Errorf("unsupported vless group %q egress_mode %q", groupKey, mode)
		}
		updated = append(updated, group)
	}
	if !reflect.DeepEqual(spec[groupsKey], updated) {
		spec[groupsKey] = updated
		changed = true
	}
	return changed, nil
}

func xrayMutableSpecGroupItems(spec map[string]any) (string, []any) {
	if spec == nil {
		return "vless_groups", nil
	}
	for _, key := range []string{"vless_groups", "xray_groups", "outbound_groups"} {
		if items := sliceRuleItemsFromAny(spec[key]); len(items) > 0 {
			return key, items
		}
	}
	return "vless_groups", nil
}

func clearResolvedXrayVLESSGroupEgress(group map[string]any) bool {
	resolvedBy := strings.ToLower(firstString(group["egress_resolved_by"]))
	if resolvedBy != "worker:xray-group-egress" &&
		resolvedBy != "worker:xray-egress" &&
		!strings.HasPrefix(resolvedBy, "control-plane:") &&
		!strings.HasPrefix(resolvedBy, "system:backhaul-") {
		return false
	}
	changed := false
	for _, key := range []string{"outbound", "egress", "egress_resolved_by"} {
		if _, ok := group[key]; ok {
			delete(group, key)
			changed = true
		}
	}
	return changed
}

func setVLESSGroupMapValue(target map[string]any, key string, value any) bool {
	if reflect.DeepEqual(target[key], value) {
		return false
	}
	target[key] = value
	return true
}

func nestedVLESSGroupMap(source map[string]any, key string) map[string]any {
	value, _ := source[key].(map[string]any)
	if value == nil {
		return map[string]any{}
	}
	return value
}

type vlessGroupCatalogSnapshot struct {
	active  []domain.VLESSGroupTemplate
	catalog []domain.VLESSGroupTemplate
	managed map[string]struct{}
}

func (s *Store) loadVLESSGroupCatalogSnapshot(ctx context.Context) (vlessGroupCatalogSnapshot, error) {
	active, err := s.ListVLESSGroupTemplates(ctx)
	if err != nil {
		return vlessGroupCatalogSnapshot{}, err
	}
	catalog, err := s.ListVLESSGroupTemplateCatalog(ctx)
	if err != nil {
		return vlessGroupCatalogSnapshot{}, err
	}
	snapshot := vlessGroupCatalogSnapshot{
		active:  active,
		catalog: catalog,
		managed: make(map[string]struct{}, len(catalog)+len(active)),
	}
	for _, template := range catalog {
		if key := normalizeXrayVLESSGroupKey(template.Key); key != "" {
			snapshot.managed[key] = struct{}{}
		}
	}
	for _, template := range active {
		if key := normalizeXrayVLESSGroupKey(template.Key); key != "" {
			snapshot.managed[key] = struct{}{}
		}
	}
	return snapshot, nil
}

func specWithVLESSGroupCatalogSnapshot(spec map[string]any, catalog vlessGroupCatalogSnapshot) (map[string]any, bool) {
	if len(catalog.active) == 0 && len(catalog.catalog) == 0 {
		return spec, false
	}
	enriched := cloneMap(spec)
	merged := make([]any, 0, len(catalog.active)+len(xraySpecGroupItems(spec)))
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
	for _, template := range catalog.active {
		add(vlessTemplateSpecGroup(template))
	}
	for _, raw := range xraySpecGroupItems(spec) {
		group, _ := cloneAny(raw).(map[string]any)
		key := normalizeXrayVLESSGroupKey(firstString(group["key"], group["name"], group["id"]))
		if _, ok := catalog.managed[key]; key != "" && ok {
			continue
		}
		add(raw)
	}
	if len(merged) == 0 {
		return spec, false
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
	return enriched, changed
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
