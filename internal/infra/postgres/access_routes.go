package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/platform/id"
)

var routeDNSNamePattern = regexp.MustCompile(`(?i)^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)*\.?$`)

type routePolicyNode struct {
	ID      string
	Name    string
	Role    string
	Address string
}

type XrayVLESSEgressResolution struct {
	Mode           string
	CurrentNodeID  string
	CurrentName    string
	CurrentRole    string
	EgressNodeID   string
	EgressNodeName string
	EgressAddress  string
	LinkID         string
	TransportID    string
	Driver         string
	InterfaceName  string
	IngressAddress string
	SendThrough    string
	RoutingTable   string
	RouteMetric    int
	ResolutionNote string
}

func (r XrayVLESSEgressResolution) Map() map[string]any {
	out := map[string]any{
		"mode":              r.Mode,
		"current_node_id":   r.CurrentNodeID,
		"current_node_name": r.CurrentName,
		"current_node_role": r.CurrentRole,
	}
	if r.EgressNodeID != "" {
		out["egress_node_id"] = r.EgressNodeID
	}
	if r.EgressNodeName != "" {
		out["egress_node_name"] = r.EgressNodeName
	}
	if r.EgressAddress != "" {
		out["egress_address"] = r.EgressAddress
	}
	if r.LinkID != "" {
		out["link_id"] = r.LinkID
	}
	if r.TransportID != "" {
		out["transport_id"] = r.TransportID
	}
	if r.Driver != "" {
		out["driver"] = r.Driver
	}
	if r.InterfaceName != "" {
		out["interface"] = r.InterfaceName
	}
	if r.IngressAddress != "" {
		out["ingress_address"] = r.IngressAddress
	}
	if r.SendThrough != "" {
		out["send_through"] = r.SendThrough
	}
	if r.RoutingTable != "" {
		out["routing_table"] = r.RoutingTable
	}
	if r.RouteMetric > 0 {
		out["route_metric"] = r.RouteMetric
	}
	if r.ResolutionNote != "" {
		out["resolution_note"] = r.ResolutionNote
	}
	return out
}

func (s *Store) ResolveXrayVLESSEgress(ctx context.Context, instanceID, requestedEgressNodeID string) (XrayVLESSEgressResolution, error) {
	instance, err := s.GetInstance(ctx, strings.TrimSpace(instanceID))
	if err != nil {
		return XrayVLESSEgressResolution{}, err
	}
	node, err := s.GetNode(ctx, strings.TrimSpace(instance.NodeID))
	if err != nil {
		return XrayVLESSEgressResolution{}, err
	}
	current := XrayVLESSEgressResolution{
		Mode:           "local_breakout",
		CurrentNodeID:  node.ID,
		CurrentName:    node.Name,
		CurrentRole:    node.Role,
		EgressNodeID:   node.ID,
		EgressNodeName: node.Name,
		EgressAddress:  node.Address,
	}
	if strings.EqualFold(node.Role, "egress") {
		current.ResolutionNote = "instance is already on an egress node"
		return current, nil
	}
	if !strings.EqualFold(node.Role, "ingress") {
		return XrayVLESSEgressResolution{}, fmt.Errorf("xray remote egress requires node role ingress or egress; node %s has role=%s", firstNonEmptyRouteValue(node.Name, node.ID), node.Role)
	}
	managed, err := s.listRoutePolicyBackhauls(ctx, node.ID)
	if err != nil {
		return XrayVLESSEgressResolution{}, err
	}
	egressNodes, err := s.listRoutePolicyEgressNodes(ctx)
	if err != nil {
		return XrayVLESSEgressResolution{}, err
	}
	requestedEgressNodeID = strings.TrimSpace(requestedEgressNodeID)
	selectedID := requestedEgressNodeID
	if selectedID == "" {
		switch len(managed) {
		case 0:
			return XrayVLESSEgressResolution{}, fmt.Errorf("xray remote egress requires an active managed backhaul from ingress node %s", firstNonEmptyRouteValue(node.Name, node.ID))
		case 1:
			for id := range managed {
				selectedID = id
			}
		default:
			ids := make([]string, 0, len(managed))
			for id := range managed {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			return XrayVLESSEgressResolution{}, fmt.Errorf("xray remote egress is ambiguous for ingress node %s; set egress_node_id, candidates=%s", firstNonEmptyRouteValue(node.Name, node.ID), strings.Join(ids, ","))
		}
	}
	selected, ok := egressNodes[selectedID]
	if !ok {
		return XrayVLESSEgressResolution{}, fmt.Errorf("selected xray egress node %s is not an active egress node", selectedID)
	}
	candidates := managed[selectedID]
	if len(candidates) == 0 {
		return XrayVLESSEgressResolution{}, fmt.Errorf("selected xray egress node %s has no active managed backhaul from ingress node %s", firstNonEmptyRouteValue(selected.Name, selected.ID), firstNonEmptyRouteValue(node.Name, node.ID))
	}
	backhaul := candidates[0]
	sendThrough := hostAddress(backhaul.IngressAddress)
	if addr, err := netip.ParseAddr(sendThrough); err != nil || !addr.Is4() {
		return XrayVLESSEgressResolution{}, fmt.Errorf("managed backhaul ingress address %q is not a valid IPv4 source address", backhaul.IngressAddress)
	}
	return XrayVLESSEgressResolution{
		Mode:           "remote_egress",
		CurrentNodeID:  node.ID,
		CurrentName:    node.Name,
		CurrentRole:    node.Role,
		EgressNodeID:   selected.ID,
		EgressNodeName: selected.Name,
		EgressAddress:  selected.Address,
		LinkID:         backhaul.LinkID,
		TransportID:    backhaul.TransportID,
		Driver:         backhaul.Driver,
		InterfaceName:  backhaul.InterfaceName,
		IngressAddress: backhaul.IngressAddress,
		SendThrough:    sendThrough,
		RoutingTable:   backhaul.RoutingTable,
		RouteMetric:    backhaul.RouteMetric,
		ResolutionNote: "remote egress resolved through the lowest-metric active managed backhaul",
	}, nil
}

func (s *Store) ListClientAccessRoutes(ctx context.Context, clientID string) ([]domain.ClientAccessRoute, error) {
	rows, err := s.db.Query(ctx, `select
		id,
		client_account_id,
		service_access_id,
		instance_id,
		node_id,
		name,
		status,
		action,
		destination_type,
		destination,
		protocol,
		ports,
		coalesce(description,''),
		policy_json,
		metadata_json,
		created_at,
		updated_at
	from client_access_routes
	where client_account_id=$1 and status <> 'revoked'
	order by created_at desc`, strings.TrimSpace(clientID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.ClientAccessRoute{}
	for rows.Next() {
		route, err := scanClientAccessRoute(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, route)
	}
	return out, rows.Err()
}

func (s *Store) CreateClientAccessRoute(ctx context.Context, clientID string, route domain.ClientAccessRoute) (domain.ClientAccessRoute, error) {
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return domain.ClientAccessRoute{}, errors.New("client_id is required")
	}
	if _, err := s.GetClient(ctx, clientID); err != nil {
		return domain.ClientAccessRoute{}, err
	}
	route.ClientAccountID = clientID
	if err := s.hydrateClientAccessRouteRefs(ctx, &route); err != nil {
		return domain.ClientAccessRoute{}, err
	}
	if route.NodeID == nil || strings.TrimSpace(*route.NodeID) == "" {
		return domain.ClientAccessRoute{}, errors.New("route must target a service access, instance or node")
	}
	if err := normalizeClientAccessRoute(&route); err != nil {
		return domain.ClientAccessRoute{}, err
	}
	if route.ID == "" {
		route.ID = id.New()
	}
	now := time.Now().UTC()
	route.CreatedAt = now
	route.UpdatedAt = now
	policy := mustJSON(route.Policy)
	metadata := mustJSON(route.Metadata)
	if _, err := s.db.Exec(ctx, `insert into client_access_routes(
		id,
		client_account_id,
		service_access_id,
		instance_id,
		node_id,
		name,
		status,
		action,
		destination_type,
		destination,
		protocol,
		ports,
		description,
		policy_json,
		metadata_json,
		created_at,
		updated_at
	) values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,nullif($13,''),$14,$15,$16,$17)`,
		route.ID,
		route.ClientAccountID,
		route.ServiceAccessID,
		route.InstanceID,
		route.NodeID,
		route.Name,
		route.Status,
		route.Action,
		route.DestinationType,
		route.Destination,
		route.Protocol,
		route.Ports,
		route.Description,
		policy,
		metadata,
		route.CreatedAt,
		route.UpdatedAt,
	); err != nil {
		return domain.ClientAccessRoute{}, err
	}
	_, _ = s.CreateAudit(ctx, "system", "client.route.create", "client", &clientID, "client access route created")
	created, err := s.GetClientAccessRoute(ctx, clientID, route.ID)
	if err == nil {
		s.queueRoutePolicyForRoute(ctx, created)
	}
	return created, err
}

func (s *Store) hydrateClientAccessRouteRefs(ctx context.Context, route *domain.ClientAccessRoute) error {
	if route.ServiceAccessID != nil && strings.TrimSpace(*route.ServiceAccessID) != "" {
		accessID := strings.TrimSpace(*route.ServiceAccessID)
		var instanceID, nodeID, accessStatus string
		err := s.db.QueryRow(ctx, `select sa.instance_id::text, i.node_id::text
			, sa.status
			from service_accesses sa
			join instances i on i.id=sa.instance_id
			where sa.id=$1 and sa.client_account_id=$2 and sa.status <> 'revoked'`, accessID, route.ClientAccountID).Scan(&instanceID, &nodeID, &accessStatus)
		if err != nil {
			return errors.New("service_access_id does not belong to client")
		}
		route.ServiceAccessID = &accessID
		route.InstanceID = &instanceID
		route.NodeID = &nodeID
		if strings.TrimSpace(route.Status) == "" && in(accessStatus, "pending", "active", "disabled") {
			route.Status = accessStatus
		}
		return nil
	}
	if route.InstanceID != nil && strings.TrimSpace(*route.InstanceID) != "" {
		instanceID := strings.TrimSpace(*route.InstanceID)
		var nodeID string
		if err := s.db.QueryRow(ctx, `select node_id::text from instances where id=$1 and status <> 'deleted'`, instanceID).Scan(&nodeID); err != nil {
			return errors.New("instance_id does not exist")
		}
		route.InstanceID = &instanceID
		route.NodeID = &nodeID
	}
	if route.NodeID != nil && strings.TrimSpace(*route.NodeID) != "" {
		nodeID := strings.TrimSpace(*route.NodeID)
		node, err := s.GetNode(ctx, nodeID)
		if err != nil {
			return errors.New("node_id does not exist")
		}
		if strings.EqualFold(node.Status, "retired") {
			return errors.New("node is retired")
		}
		route.NodeID = &nodeID
	}
	return nil
}

func (s *Store) DeleteClientAccessRoute(ctx context.Context, clientID, routeID string) (domain.ClientAccessRoute, error) {
	clientID = strings.TrimSpace(clientID)
	routeID = strings.TrimSpace(routeID)
	if clientID == "" || routeID == "" {
		return domain.ClientAccessRoute{}, errors.New("client_id and route_id are required")
	}
	route, err := s.GetClientAccessRoute(ctx, clientID, routeID)
	if err != nil {
		return domain.ClientAccessRoute{}, err
	}
	if isBaselineRoute(route) {
		return domain.ClientAccessRoute{}, errors.New("baseline service route cannot be deleted; revoke the service access or client instead")
	}
	if _, err := s.db.Exec(ctx, `update client_access_routes set status='revoked', updated_at=now() where id=$1 and client_account_id=$2`, routeID, clientID); err != nil {
		return domain.ClientAccessRoute{}, err
	}
	_, _ = s.CreateAudit(ctx, "system", "client.route.delete", "client", &clientID, "client access route revoked")
	route.Status = "revoked"
	route.UpdatedAt = time.Now().UTC()
	s.queueRoutePolicyForRoute(ctx, route)
	return route, nil
}

func (s *Store) GetClientAccessRoute(ctx context.Context, clientID, routeID string) (domain.ClientAccessRoute, error) {
	row := s.db.QueryRow(ctx, `select
		id,
		client_account_id,
		service_access_id,
		instance_id,
		node_id,
		name,
		status,
		action,
		destination_type,
		destination,
		protocol,
		ports,
		coalesce(description,''),
		policy_json,
		metadata_json,
		created_at,
		updated_at
	from client_access_routes
	where client_account_id=$1 and id=$2`, strings.TrimSpace(clientID), strings.TrimSpace(routeID))
	return scanClientAccessRoute(row)
}

func (s *Store) ensureBaselineClientAccessRoute(ctx context.Context, clientID, accessID, instanceID string) error {
	instance, err := s.GetInstance(ctx, instanceID)
	if err != nil {
		return err
	}
	accessMeta, err := s.clientProvisioningServiceMetadata(ctx, instanceID, nil)
	if err != nil {
		return err
	}
	inbound, _ := accessMeta["inbound_service"].(map[string]any)
	destination := strings.TrimSpace(firstString(inbound["client_endpoint_host"], inbound["endpoint_host"], instance.EndpointHost))
	if destination == "" {
		destination = strings.TrimSpace(instance.Name)
	}
	if destination == "" {
		destination = strings.TrimSpace(instance.ID)
	}
	destinationType := "endpoint"
	portText := "*"
	if port := firstIntValue(inbound["client_endpoint_port"], inbound["endpoint_port"], instance.EndpointPort); port > 0 {
		portText = strconv.Itoa(port)
	}
	nodeID := strings.TrimSpace(instance.NodeID)
	now := time.Now().UTC()
	meta := mustJSON(map[string]any{
		"baseline":     true,
		"route_type":   "service_inbound_baseline",
		"service_code": instance.ServiceCode,
		"instance":     instance.Name,
		"slug":         instance.Slug,
		"inbound":      inbound,
	})
	_, err = s.db.Exec(ctx, `insert into client_access_routes(
		id,
		client_account_id,
		service_access_id,
		instance_id,
		node_id,
		name,
		status,
		action,
		destination_type,
		destination,
		protocol,
		ports,
		description,
		policy_json,
		metadata_json,
		created_at,
		updated_at
	) values($1,$2,$3,$4,$5,$6,'pending','allow',$7,$8,'any',$9,$10,'{}'::jsonb,$11,$12,$12)
	on conflict(service_access_id) where service_access_id is not null and (metadata_json->>'baseline') = 'true'
	do update set
		instance_id=excluded.instance_id,
		node_id=excluded.node_id,
		name=excluded.name,
		destination_type=excluded.destination_type,
		destination=excluded.destination,
		ports=excluded.ports,
		description=excluded.description,
		metadata_json=excluded.metadata_json,
		updated_at=excluded.updated_at`,
		id.New(),
		clientID,
		accessID,
		instanceID,
		nullIfEmpty(nodeID),
		"Service: "+firstNonEmptyRouteValue(instance.Name, instance.Slug, instance.ServiceCode),
		destinationType,
		destination,
		portText,
		"Baseline route created from service access provisioning.",
		meta,
		now,
	)
	return err
}

func (s *Store) CreateNodeRoutePolicyApplyJob(ctx context.Context, nodeID string) (domain.Job, error) {
	node, err := s.GetNode(ctx, strings.TrimSpace(nodeID))
	if err != nil {
		return domain.Job{}, err
	}
	if strings.EqualFold(node.Status, "retired") {
		return domain.Job{}, errors.New("node is retired")
	}
	payload, err := s.buildNodeRoutePolicyPayload(ctx, node.ID)
	if err != nil {
		return domain.Job{}, err
	}
	j, err := s.CreateJob(ctx, domain.Job{
		Type:      "node.route_policy.apply",
		ScopeType: "node",
		ScopeID:   &node.ID,
		NodeID:    &node.ID,
		Priority:  80,
		Payload:   payload,
	})
	if err == nil {
		_, _ = s.CreateAudit(ctx, "system", "node.route_policy.apply", "node", &node.ID, "node route policy apply queued")
	}
	return j, err
}

func (s *Store) PreviewNodeRoutePolicy(ctx context.Context, nodeID string) (map[string]any, error) {
	node, err := s.GetNode(ctx, strings.TrimSpace(nodeID))
	if err != nil {
		return nil, err
	}
	if strings.EqualFold(node.Status, "retired") {
		return nil, errors.New("node is retired")
	}
	payload, err := s.buildNodeRoutePolicyPayload(ctx, node.ID)
	if err != nil {
		return nil, err
	}
	return routePolicyPreviewForNode(node, payload), nil
}

func (s *Store) buildNodeRoutePolicyPayload(ctx context.Context, nodeID string) (map[string]any, error) {
	egressNodes, err := s.listRoutePolicyEgressNodes(ctx)
	if err != nil {
		return nil, err
	}
	managedBackhauls, err := s.listRoutePolicyBackhauls(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `select
		car.id,
		car.client_account_id,
		ca.username,
		coalesce(ca.display_name,''),
		car.service_access_id,
		car.instance_id,
		car.node_id,
		coalesce(sd.code,''),
		car.name,
		car.status,
		car.action,
		car.destination_type,
		car.destination,
		car.protocol,
		car.ports,
		coalesce(car.description,''),
		car.policy_json,
		car.metadata_json,
		coalesce(sa.metadata_json, '{}'::jsonb),
		coalesce(rn.name,''),
		coalesce(rn.role,''),
		coalesce(rn.address,''),
		car.created_at,
		car.updated_at
	from client_access_routes car
	join client_accounts ca on ca.id=car.client_account_id
	join nodes rn on rn.id=car.node_id
	left join service_accesses sa on sa.id=car.service_access_id
	left join instances i on i.id=coalesce(car.instance_id, sa.instance_id)
	left join service_definitions sd on sd.id=i.service_definition_id
	where car.node_id=$1 and car.status <> 'revoked'
	order by ca.username asc, car.destination_type asc, car.destination asc, car.created_at asc`, strings.TrimSpace(nodeID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	routes := make([]map[string]any, 0)
	activeCount := 0
	for rows.Next() {
		var routeID, clientID, username, displayName, serviceCode, name, status, action, destinationType, destination, protocol, ports, description string
		var routeNodeName, routeNodeRole, routeNodeAddress string
		var serviceAccessID, instanceID, routeNodeID *string
		var policyRaw, metadataRaw, accessMetadataRaw []byte
		var createdAt, updatedAt time.Time
		if err := rows.Scan(
			&routeID,
			&clientID,
			&username,
			&displayName,
			&serviceAccessID,
			&instanceID,
			&routeNodeID,
			&serviceCode,
			&name,
			&status,
			&action,
			&destinationType,
			&destination,
			&protocol,
			&ports,
			&description,
			&policyRaw,
			&metadataRaw,
			&accessMetadataRaw,
			&routeNodeName,
			&routeNodeRole,
			&routeNodeAddress,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, err
		}
		policy := map[string]any{}
		metadata := map[string]any{}
		accessMetadata := map[string]any{}
		_ = json.Unmarshal(policyRaw, &policy)
		_ = json.Unmarshal(metadataRaw, &metadata)
		_ = json.Unmarshal(accessMetadataRaw, &accessMetadata)
		if status == "active" {
			activeCount++
		}
		entry := map[string]any{
			"route_id":         routeID,
			"client_id":        clientID,
			"username":         username,
			"display_name":     displayName,
			"service_code":     normalizeCapabilityCode(serviceCode),
			"name":             name,
			"status":           status,
			"action":           action,
			"destination_type": destinationType,
			"destination":      destination,
			"protocol":         protocol,
			"ports":            ports,
			"description":      description,
			"source_identity":  routeSourceIdentity(serviceCode, accessMetadata),
			"policy":           policy,
			"metadata":         metadata,
			"created_at":       createdAt.UTC().Format(time.RFC3339),
			"updated_at":       updatedAt.UTC().Format(time.RFC3339),
		}
		if inbound := routeInboundServiceProjection(metadata, accessMetadata); len(inbound) > 0 {
			entry["inbound_service"] = inbound
		}
		if serviceAccessID != nil && strings.TrimSpace(*serviceAccessID) != "" {
			entry["service_access_id"] = strings.TrimSpace(*serviceAccessID)
		}
		if instanceID != nil && strings.TrimSpace(*instanceID) != "" {
			entry["instance_id"] = strings.TrimSpace(*instanceID)
		}
		if routeNodeID != nil && strings.TrimSpace(*routeNodeID) != "" {
			entry["node_id"] = strings.TrimSpace(*routeNodeID)
		}
		routeNode := routePolicyNode{
			ID:      strings.TrimSpace(stringPtrValue(routeNodeID)),
			Name:    strings.TrimSpace(routeNodeName),
			Role:    strings.TrimSpace(routeNodeRole),
			Address: strings.TrimSpace(routeNodeAddress),
		}
		entry["egress"] = routeEgressProjection(routeNode, policy, egressNodes, managedBackhauls)
		entry["enforcement"] = routeEnforcementProjection(entry)
		routes = append(routes, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	systemRoutes, err := s.buildNodeRoutePolicySystemRoutes(ctx, nodeID, managedBackhauls)
	if err != nil {
		return nil, err
	}
	activeSystemRouteCount := 0
	for _, route := range systemRoutes {
		if strings.EqualFold(strings.TrimSpace(stringify(route["status"])), "active") {
			activeSystemRouteCount++
		}
	}
	revision := routePolicyRevision(routes, systemRoutes)
	enforcementSummary := routePolicyEnforcementSummary(routes, systemRoutes)
	return map[string]any{
		"node_id":                   strings.TrimSpace(nodeID),
		"generated_at":              time.Now().UTC().Format(time.RFC3339),
		"revision":                  revision,
		"enforcement_mode":          "kernel_policy_apply",
		"output_path":               "/etc/megavpn/client-access-routes.json",
		"route_count":               len(routes),
		"active_route_count":        activeCount,
		"system_route_count":        len(systemRoutes),
		"active_system_route_count": activeSystemRouteCount,
		"enforcement":               enforcementSummary,
		"routes":                    routes,
		"system_routes":             systemRoutes,
	}, nil
}

func (s *Store) listRoutePolicyEgressNodes(ctx context.Context) (map[string]routePolicyNode, error) {
	rows, err := s.db.Query(ctx, `select id::text, coalesce(name,''), coalesce(role,''), coalesce(address,'') from nodes where role='egress' and status <> 'retired'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]routePolicyNode{}
	for rows.Next() {
		var node routePolicyNode
		if err := rows.Scan(&node.ID, &node.Name, &node.Role, &node.Address); err != nil {
			return nil, err
		}
		out[node.ID] = node
	}
	return out, rows.Err()
}

func routePolicyRevision(routes []map[string]any, systemRoutes ...[]map[string]any) string {
	payload := map[string]any{"routes": routes}
	if len(systemRoutes) > 0 {
		payload["system_routes"] = systemRoutes[0]
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func (s *Store) buildNodeRoutePolicySystemRoutes(ctx context.Context, nodeID string, managedBackhauls map[string][]routePolicyBackhaul) ([]map[string]any, error) {
	rows, err := s.db.Query(ctx, `select
		i.id::text,
		i.name,
		i.slug,
		coalesce(i.systemd_unit,''),
		i.status,
		i.enabled,
		coalesce(ir.spec_json, '{}'::jsonb)
	from instances i
	join service_definitions sd on sd.id=i.service_definition_id
	left join instance_revisions ir on ir.id=coalesce(i.last_applied_revision_id, i.current_revision_id)
	where i.node_id=$1
	  and i.status <> 'deleted'
	  and sd.code in ('xray-core','xray','xray_core')
	order by i.name asc, i.created_at asc`, strings.TrimSpace(nodeID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := make([]map[string]any, 0)
	for rows.Next() {
		var instanceID, name, slug, unit, status string
		var enabled bool
		var raw []byte
		if err := rows.Scan(&instanceID, &name, &slug, &unit, &status, &enabled, &raw); err != nil {
			return nil, err
		}
		if !enabled || !strings.EqualFold(status, "active") {
			continue
		}
		spec := map[string]any{}
		_ = json.Unmarshal(raw, &spec)
		entries = append(entries, xrayVLESSSystemRoutesForSpec(routePolicyXrayInstance{
			ID:          instanceID,
			Name:        name,
			Slug:        slug,
			SystemdUnit: unit,
			NodeID:      strings.TrimSpace(nodeID),
		}, spec, managedBackhauls)...)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return dedupeRoutePolicySystemRoutes(entries), nil
}

type routePolicyXrayInstance struct {
	ID          string
	Name        string
	Slug        string
	SystemdUnit string
	NodeID      string
}

func xrayVLESSSystemRoutesForSpec(instance routePolicyXrayInstance, spec map[string]any, managedBackhauls map[string][]routePolicyBackhaul) []map[string]any {
	if spec == nil {
		return nil
	}
	entries := make([]map[string]any, 0, 2)
	if egress, _ := spec["xray_egress"].(map[string]any); len(egress) > 0 {
		outbound, _ := spec["xray_default_outbound"].(map[string]any)
		entries = append(entries, routePolicySystemRouteFromXrayEgress(instance, "instance_default", "default", firstString(outbound["tag"], "egress-default"), egress, outbound, managedBackhauls))
	}
	for _, item := range rawXrayVLESSGroupList(spec) {
		group, _ := item.(map[string]any)
		if group == nil {
			continue
		}
		egress, _ := group["egress"].(map[string]any)
		outbound, _ := group["outbound"].(map[string]any)
		if len(egress) == 0 && len(outbound) == 0 {
			continue
		}
		groupKey := normalizeXrayVLESSGroupKey(firstString(group["key"], group["name"], group["id"]))
		if groupKey == "" {
			continue
		}
		outboundTag := normalizeXrayOutboundTag(firstString(outbound["tag"], group["outbound_tag"], group["outboundTag"], group["tag"]))
		if outboundTag == "" {
			outboundTag = "direct"
		}
		entries = append(entries, routePolicySystemRouteFromXrayEgress(instance, "vless_group", groupKey, outboundTag, egress, outbound, managedBackhauls))
	}
	out := entries[:0]
	for _, entry := range entries {
		if entry != nil {
			out = append(out, entry)
		}
	}
	return out
}

func rawXrayVLESSGroupList(spec map[string]any) []any {
	if spec == nil {
		return nil
	}
	for _, key := range []string{"vless_groups", "xray_groups", "outbound_groups"} {
		if list, ok := spec[key].([]any); ok {
			return list
		}
	}
	return nil
}

func routePolicySystemRouteFromXrayEgress(instance routePolicyXrayInstance, scope, scopeKey, outboundTag string, egress, outbound map[string]any, managedBackhauls map[string][]routePolicyBackhaul) map[string]any {
	if egress == nil {
		egress = map[string]any{}
	}
	if outbound == nil {
		outbound = map[string]any{}
	}
	mode := strings.ToLower(firstString(egress["mode"], egress["egress_mode"]))
	source := hostAddress(firstString(egress["send_through"], egress["sendThrough"], outbound["sendThrough"], outbound["send_through"]))
	if source == "" {
		return nil
	}
	if mode == "" {
		mode = "remote_egress"
	}
	if mode == "local" || mode == "direct" || mode == "local_breakout" {
		return nil
	}
	entry := map[string]any{
		"system_route_id": routePolicySystemRouteID(instance.ID, scope, scopeKey, source),
		"kind":            "xray_vless_remote_egress",
		"status":          "active",
		"source":          source + "/32",
		"source_address":  source,
		"destination":     "0.0.0.0/0",
		"mode":            "remote_egress",
		"outbound_tag":    outboundTag,
		"references": []any{map[string]any{
			"instance_id":   instance.ID,
			"instance_name": firstNonEmptyRouteValue(instance.Name, instance.Slug, instance.ID),
			"systemd_unit":  instance.SystemdUnit,
			"scope":         scope,
			"scope_key":     scopeKey,
			"outbound_tag":  outboundTag,
		}},
		"reasons": []string{"xray/vless remote egress uses sendThrough source routing over a managed backhaul"},
	}
	if instance.NodeID != "" {
		entry["node_id"] = instance.NodeID
	}
	egressNodeID := firstString(egress["egress_node_id"], egress["node_id"])
	if egressNodeID != "" {
		entry["egress_node_id"] = egressNodeID
	}
	if linkID := firstString(egress["link_id"]); linkID != "" {
		entry["link_id"] = linkID
	}
	if transportID := firstString(egress["transport_id"]); transportID != "" {
		entry["transport_id"] = transportID
	}
	table := firstString(egress["routing_table"], egress["table"])
	iface := firstString(egress["interface"], egress["interface_name"], egress["dev"])
	metric := intFromAnyRoutePolicy(egress["route_metric"], egress["metric"])
	matched := matchingRoutePolicyBackhaulsForSource(managedBackhauls[egressNodeID], source)
	if len(matched) > 0 {
		if table == "" || strings.EqualFold(table, "main") {
			table = matched[0].RoutingTable
		}
		if iface == "" {
			iface = matched[0].InterfaceName
		}
		if metric <= 0 {
			metric = matched[0].RouteMetric
		}
		entry["managed_backhaul"] = routePolicyBackhaulProjection(matched[0], table)
		entry["managed_backhauls"] = routePolicyBackhaulProjections(matched, table)
	}
	if table != "" {
		entry["table"] = table
		entry["routing_table"] = table
	}
	if iface != "" {
		entry["interface"] = iface
	}
	if metric > 0 {
		entry["route_metric"] = metric
	}
	reasons := routePolicySystemRouteBlockReasons(source, table, iface)
	if egressNodeID != "" && len(matched) == 0 {
		reasons = append(reasons, "xray remote egress requires an active managed backhaul matching the sendThrough source")
	}
	if len(reasons) > 0 {
		entry["status"] = "blocked"
		entry["reasons"] = reasons
	}
	return entry
}

func routePolicySystemRouteBlockReasons(source, table, iface string) []string {
	reasons := []string{}
	if addr, err := netip.ParseAddr(source); err != nil || !addr.Is4() {
		reasons = append(reasons, "xray sendThrough source is not a valid IPv4 address")
	}
	if strings.TrimSpace(table) == "" || strings.EqualFold(strings.TrimSpace(table), "main") {
		reasons = append(reasons, "xray remote egress requires a non-main managed backhaul routing table")
	}
	if strings.TrimSpace(iface) == "" {
		reasons = append(reasons, "xray remote egress requires a managed backhaul interface")
	}
	return reasons
}

func matchingRoutePolicyBackhaulsForSource(backhauls []routePolicyBackhaul, source string) []routePolicyBackhaul {
	out := make([]routePolicyBackhaul, 0, len(backhauls))
	for _, backhaul := range backhauls {
		if hostAddress(backhaul.IngressAddress) == source {
			out = append(out, backhaul)
		}
	}
	return out
}

func routePolicySystemRouteID(instanceID, scope, scopeKey, source string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{instanceID, scope, scopeKey, source}, "|")))
	return "xray-" + hex.EncodeToString(sum[:6])
}

func dedupeRoutePolicySystemRoutes(entries []map[string]any) []map[string]any {
	byKey := map[string]map[string]any{}
	order := []string{}
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		key := strings.Join([]string{
			strings.TrimSpace(stringify(entry["source"])),
			strings.TrimSpace(stringify(entry["destination"])),
			strings.TrimSpace(stringify(entry["table"])),
			strings.TrimSpace(stringify(entry["interface"])),
		}, "|")
		if key == "|||" {
			key = strings.TrimSpace(stringify(entry["system_route_id"]))
		}
		if existing, ok := byKey[key]; ok {
			existing["references"] = appendSystemRouteReferences(existing["references"], entry["references"])
			if !strings.EqualFold(stringify(existing["status"]), "active") && strings.EqualFold(stringify(entry["status"]), "active") {
				for k, v := range entry {
					if k != "references" {
						existing[k] = v
					}
				}
			}
			continue
		}
		byKey[key] = entry
		order = append(order, key)
	}
	sort.Slice(order, func(i, j int) bool {
		left, right := byKey[order[i]], byKey[order[j]]
		return strings.Join([]string{stringify(left["source"]), stringify(left["table"]), stringify(left["interface"]), stringify(left["system_route_id"])}, "|") <
			strings.Join([]string{stringify(right["source"]), stringify(right["table"]), stringify(right["interface"]), stringify(right["system_route_id"])}, "|")
	})
	out := make([]map[string]any, 0, len(order))
	for _, key := range order {
		out = append(out, byKey[key])
	}
	return out
}

func appendSystemRouteReferences(existingRaw, nextRaw any) []any {
	out := []any{}
	if existing, ok := existingRaw.([]any); ok {
		out = append(out, existing...)
	}
	if next, ok := nextRaw.([]any); ok {
		out = append(out, next...)
	}
	return out
}

func intFromAnyRoutePolicy(values ...any) int {
	for _, value := range values {
		switch x := value.(type) {
		case int:
			if x != 0 {
				return x
			}
		case int64:
			if x != 0 {
				return int(x)
			}
		case float64:
			if x != 0 {
				return int(x)
			}
		case string:
			if parsed, err := strconv.Atoi(strings.TrimSpace(x)); err == nil && parsed != 0 {
				return parsed
			}
		}
	}
	return 0
}

func routeSourceIdentity(serviceCode string, metadata map[string]any) map[string]any {
	if metadata == nil {
		metadata = map[string]any{}
	}
	switch normalizeCapabilityCode(serviceCode) {
	case "wireguard":
		if value := firstRouteIdentityValue(metadata["wireguard_client_address"]); value != "" {
			return map[string]any{"type": "wireguard_ip", "value": value}
		}
	case "openvpn":
		if value := firstRouteIdentityValue(metadata["openvpn_client_common_name"]); value != "" {
			return map[string]any{"type": "openvpn_common_name", "value": value}
		}
	case "xray-core":
		if value := firstRouteIdentityValue(metadata["xray_uuid"], metadata["uuid"]); value != "" {
			return map[string]any{"type": "xray_uuid", "value": value}
		}
	case "http_proxy":
		if value := firstRouteIdentityValue(metadata["username"], metadata["proxy_username"], metadata["http_proxy_username"]); value != "" {
			return map[string]any{"type": "http_proxy_username", "value": value}
		}
	case "ipsec", "xl2tpd":
		if value := firstRouteIdentityValue(metadata["username"], metadata["l2tp_username"], metadata["ppp_username"]); value != "" {
			return map[string]any{"type": "ppp_username", "value": value}
		}
	}
	return map[string]any{"type": "unknown"}
}

func routeInboundServiceProjection(routeMetadata, accessMetadata map[string]any) map[string]any {
	if routeMetadata != nil {
		if inbound, ok := routeMetadata["inbound"].(map[string]any); ok && len(inbound) > 0 {
			return inbound
		}
		if inbound, ok := routeMetadata["inbound_service"].(map[string]any); ok && len(inbound) > 0 {
			return inbound
		}
	}
	if accessMetadata != nil {
		if inbound, ok := accessMetadata["inbound_service"].(map[string]any); ok && len(inbound) > 0 {
			return inbound
		}
	}
	return nil
}

func routeEnforcementProjection(entry map[string]any) map[string]any {
	status := strings.ToLower(strings.TrimSpace(stringify(entry["status"])))
	action := strings.ToLower(strings.TrimSpace(stringify(entry["action"])))
	destinationType := strings.ToLower(strings.TrimSpace(stringify(entry["destination_type"])))
	destination := strings.TrimSpace(stringify(entry["destination"]))
	protocol := strings.ToLower(strings.TrimSpace(stringify(entry["protocol"])))
	ports := strings.TrimSpace(stringify(entry["ports"]))
	sourceIdentity, _ := entry["source_identity"].(map[string]any)
	sourceType := strings.ToLower(strings.TrimSpace(stringify(sourceIdentity["type"])))
	sourceValue := strings.TrimSpace(stringify(sourceIdentity["value"]))
	egress, _ := entry["egress"].(map[string]any)

	reasons := []string{}
	if status != "active" {
		reasons = append(reasons, "route status is not active")
	}
	if action != "allow" {
		reasons = append(reasons, "route action is not a kernel-routable allow policy")
	}
	if !routeSourceIdentityEnforceable(sourceType, sourceValue) {
		reasons = append(reasons, "route source identity is not enforceable at L3/L4")
	}
	if !routeDestinationEnforceable(destinationType, destination) {
		reasons = append(reasons, "route destination is not an IP prefix or IP endpoint")
	}
	if !routeProtocolPortsEnforceable(protocol, ports) {
		reasons = append(reasons, "route protocol/ports are not enforceable at L3/L4")
	}
	if !routeEgressEnforceable(egress) {
		reasons = append(reasons, "route egress output is not enforceable")
	}
	if len(reasons) > 0 {
		return map[string]any{
			"mode":    "observe_only",
			"layer":   "control_plane_snapshot",
			"reasons": reasons,
		}
	}
	return map[string]any{
		"mode":    "l3_l4_candidate",
		"layer":   "kernel_policy_candidate",
		"reasons": []string{"source identity and destination can be represented as L3/L4 policy"},
	}
}

func routePolicyEnforcementSummary(routes []map[string]any, systemRoutes ...[]map[string]any) map[string]any {
	out := map[string]any{
		"mode":                 "kernel_policy_apply",
		"enforced":             false,
		"enforceable_routes":   0,
		"observe_only_routes":  0,
		"system_routes":        0,
		"active_system_routes": 0,
		"notes": []string{
			"agent materializes a signed route-policy snapshot",
			"agent applies kernel policy routing only for L3/L4 allow candidates with explicit egress output",
			"agent applies managed system egress rules for Xray/VLESS remote egress outbounds",
		},
	}
	for _, route := range routes {
		enforcement, _ := route["enforcement"].(map[string]any)
		if strings.TrimSpace(stringify(enforcement["mode"])) == "l3_l4_candidate" {
			out["enforceable_routes"] = intFromRouteSummary(out["enforceable_routes"]) + 1
			continue
		}
		out["observe_only_routes"] = intFromRouteSummary(out["observe_only_routes"]) + 1
	}
	if len(systemRoutes) > 0 {
		out["system_routes"] = len(systemRoutes[0])
		for _, route := range systemRoutes[0] {
			if strings.EqualFold(strings.TrimSpace(stringify(route["status"])), "active") {
				out["active_system_routes"] = intFromRouteSummary(out["active_system_routes"]) + 1
			}
		}
	}
	return out
}

func routePolicyPreviewForNode(node domain.Node, payload map[string]any) map[string]any {
	if payload == nil {
		payload = map[string]any{}
	}
	routes := routePolicyMapList(payload["routes"])
	systemRoutes := routePolicyMapList(payload["system_routes"])
	previewRoutes := make([]any, 0, len(routes))
	for _, route := range routes {
		previewRoutes = append(previewRoutes, routePolicyPreviewRoute(route))
	}
	previewSystemRoutes := make([]any, 0, len(systemRoutes))
	for _, route := range systemRoutes {
		previewSystemRoutes = append(previewSystemRoutes, routePolicyPreviewSystemRoute(route))
	}
	warnings := routePolicyPreviewWarnings(routes, systemRoutes)
	return map[string]any{
		"status":        "ok",
		"node_id":       node.ID,
		"node_name":     node.Name,
		"node_role":     node.Role,
		"node_address":  node.Address,
		"generated_at":  stringify(payload["generated_at"]),
		"revision":      stringify(payload["revision"]),
		"output_path":   stringify(payload["output_path"]),
		"enforcement":   payload["enforcement"],
		"kernel":        routePolicyPreviewKernel(payload),
		"summary":       routePolicyPreviewSummary(routes, systemRoutes, len(warnings)),
		"warnings":      warnings,
		"routes":        previewRoutes,
		"system_routes": previewSystemRoutes,
	}
}

func routePolicyPreviewKernel(payload map[string]any) map[string]any {
	return map[string]any{
		"managed_table":          "inet megavpn",
		"client_mark_chain":      "route_policy_prerouting",
		"system_mark_chain":      "route_policy_output",
		"client_priority_range":  "22000..22999",
		"system_priority_range":  "21900..21949",
		"refresh_unit":           "megavpn-route-policy.service",
		"refresh_timer":          "megavpn-route-policy.timer",
		"managed_script":         "/usr/local/lib/megavpn/route-policy/client-access-routes.sh",
		"managed_snapshot":       firstNonEmptyRouteValue(stringify(payload["output_path"]), "/etc/megavpn/client-access-routes.json"),
		"enforcement_primitives": []string{"nftables fwmark", "ip rule fwmark lookup table", "ip route replace table"},
	}
}

func routePolicyPreviewSummary(routes, systemRoutes []map[string]any, warningCount int) map[string]any {
	out := map[string]any{
		"route_count":                len(routes),
		"active_route_count":         0,
		"enforceable_routes":         0,
		"observe_only_routes":        0,
		"candidate_egress_outputs":   0,
		"blocked_egress_outputs":     0,
		"system_route_count":         len(systemRoutes),
		"active_system_route_count":  0,
		"blocked_system_route_count": 0,
		"warning_count":              warningCount,
	}
	for _, route := range routes {
		if strings.EqualFold(stringify(route["status"]), "active") {
			out["active_route_count"] = intFromRouteSummary(out["active_route_count"]) + 1
		}
		enforcement := routePolicyNestedMap(route, "enforcement")
		if stringify(enforcement["mode"]) == "l3_l4_candidate" {
			out["enforceable_routes"] = intFromRouteSummary(out["enforceable_routes"]) + 1
		} else {
			out["observe_only_routes"] = intFromRouteSummary(out["observe_only_routes"]) + 1
		}
		egress := routePolicyNestedMap(route, "egress")
		if strings.EqualFold(stringify(egress["status"]), "candidate") {
			out["candidate_egress_outputs"] = intFromRouteSummary(out["candidate_egress_outputs"]) + 1
		}
		if strings.EqualFold(stringify(egress["status"]), "blocked") {
			out["blocked_egress_outputs"] = intFromRouteSummary(out["blocked_egress_outputs"]) + 1
		}
	}
	for _, route := range systemRoutes {
		switch strings.ToLower(strings.TrimSpace(stringify(route["status"]))) {
		case "active":
			out["active_system_route_count"] = intFromRouteSummary(out["active_system_route_count"]) + 1
		case "blocked":
			out["blocked_system_route_count"] = intFromRouteSummary(out["blocked_system_route_count"]) + 1
		}
	}
	return out
}

func routePolicyPreviewRoute(route map[string]any) map[string]any {
	out := map[string]any{
		"route_id":         stringify(route["route_id"]),
		"client_id":        stringify(route["client_id"]),
		"username":         stringify(route["username"]),
		"display_name":     stringify(route["display_name"]),
		"service_code":     stringify(route["service_code"]),
		"name":             stringify(route["name"]),
		"status":           stringify(route["status"]),
		"action":           stringify(route["action"]),
		"destination_type": stringify(route["destination_type"]),
		"destination":      stringify(route["destination"]),
		"protocol":         stringify(route["protocol"]),
		"ports":            stringify(route["ports"]),
		"source_identity":  routePolicyPreviewSourceIdentity(routePolicyNestedMap(route, "source_identity")),
		"egress":           routePolicyPreviewEgress(routePolicyNestedMap(route, "egress")),
		"enforcement":      route["enforcement"],
	}
	if serviceAccessID := strings.TrimSpace(stringify(route["service_access_id"])); serviceAccessID != "" {
		out["service_access_id"] = serviceAccessID
	}
	if instanceID := strings.TrimSpace(stringify(route["instance_id"])); instanceID != "" {
		out["instance_id"] = instanceID
	}
	if nodeID := strings.TrimSpace(stringify(route["node_id"])); nodeID != "" {
		out["node_id"] = nodeID
	}
	if inbound := routePolicyNestedMap(route, "inbound_service"); len(inbound) > 0 {
		out["inbound_service"] = inbound
	}
	return out
}

func routePolicyPreviewSystemRoute(route map[string]any) map[string]any {
	out := map[string]any{
		"system_route_id": stringify(route["system_route_id"]),
		"kind":            stringify(route["kind"]),
		"status":          stringify(route["status"]),
		"source":          stringify(route["source"]),
		"source_address":  stringify(route["source_address"]),
		"destination":     stringify(route["destination"]),
		"mode":            stringify(route["mode"]),
		"outbound_tag":    stringify(route["outbound_tag"]),
		"node_id":         stringify(route["node_id"]),
		"egress_node_id":  stringify(route["egress_node_id"]),
		"link_id":         stringify(route["link_id"]),
		"transport_id":    stringify(route["transport_id"]),
		"table":           stringify(route["table"]),
		"routing_table":   stringify(route["routing_table"]),
		"interface":       stringify(route["interface"]),
		"route_metric":    intFromAnyRoutePolicy(route["route_metric"]),
		"reasons":         routePolicyReasonStrings(route["reasons"]),
	}
	if refs := routePolicyAnyList(route["references"]); len(refs) > 0 {
		out["references"] = refs
	}
	if backhaul := routePolicyNestedMap(route, "managed_backhaul"); len(backhaul) > 0 {
		out["managed_backhaul"] = backhaul
	}
	if candidates := routePolicyAnyList(route["managed_backhauls"]); len(candidates) > 0 {
		out["managed_backhaul_count"] = len(candidates)
		out["managed_backhauls"] = candidates
	}
	return out
}

func routePolicyPreviewSourceIdentity(identity map[string]any) map[string]any {
	sourceType := strings.TrimSpace(stringify(identity["type"]))
	sourceValue := strings.TrimSpace(stringify(identity["value"]))
	out := map[string]any{"type": firstNonEmptyRouteValue(sourceType, "unknown")}
	if sourceValue == "" {
		return out
	}
	if routeSourceIdentityEnforceable(sourceType, sourceValue) {
		out["value"] = sourceValue
		return out
	}
	out["value"] = "[redacted]"
	out["redacted"] = true
	return out
}

func routePolicyPreviewEgress(egress map[string]any) map[string]any {
	out := map[string]any{
		"mode":     stringify(egress["mode"]),
		"status":   stringify(egress["status"]),
		"next_hop": stringify(egress["next_hop"]),
		"interface": stringify(firstNonEmptyRouteValue(
			stringify(egress["interface"]),
			stringify(egress["egress_interface"]),
		)),
		"table":   stringify(egress["table"]),
		"reasons": routePolicyReasonStrings(egress["reasons"]),
	}
	if selected := routePolicyNestedMap(egress, "selected_node"); len(selected) > 0 {
		out["selected_node"] = selected
	}
	if current := routePolicyNestedMap(egress, "current_node"); len(current) > 0 {
		out["current_node"] = current
	}
	if backhaul := routePolicyNestedMap(egress, "managed_backhaul"); len(backhaul) > 0 {
		out["managed_backhaul"] = backhaul
	}
	if candidates := routePolicyAnyList(egress["managed_backhauls"]); len(candidates) > 0 {
		out["managed_backhaul_count"] = len(candidates)
		out["managed_backhauls"] = candidates
	}
	return out
}

func routePolicyPreviewWarnings(routes, systemRoutes []map[string]any) []any {
	warnings := make([]any, 0)
	for _, route := range routes {
		egress := routePolicyNestedMap(route, "egress")
		if strings.EqualFold(stringify(egress["status"]), "blocked") {
			warnings = append(warnings, map[string]any{
				"type":     "route_egress_blocked",
				"route_id": stringify(route["route_id"]),
				"username": stringify(route["username"]),
				"reasons":  routePolicyReasonStrings(egress["reasons"]),
			})
		}
		enforcement := routePolicyNestedMap(route, "enforcement")
		if stringify(enforcement["mode"]) != "l3_l4_candidate" {
			warnings = append(warnings, map[string]any{
				"type":     "route_observe_only",
				"route_id": stringify(route["route_id"]),
				"username": stringify(route["username"]),
				"reasons":  routePolicyReasonStrings(enforcement["reasons"]),
			})
		}
	}
	for _, route := range systemRoutes {
		if strings.EqualFold(stringify(route["status"]), "blocked") {
			warnings = append(warnings, map[string]any{
				"type":            "system_route_blocked",
				"system_route_id": stringify(route["system_route_id"]),
				"kind":            stringify(route["kind"]),
				"source":          stringify(route["source"]),
				"destination":     stringify(route["destination"]),
				"reasons":         routePolicyReasonStrings(route["reasons"]),
			})
		}
	}
	if len(warnings) > 80 {
		return append(warnings[:80], map[string]any{"type": "truncated", "reasons": []string{"route policy preview warnings were truncated to 80 items"}})
	}
	return warnings
}

func routePolicyMapList(raw any) []map[string]any {
	switch items := raw.(type) {
	case []map[string]any:
		out := make([]map[string]any, 0, len(items))
		for _, item := range items {
			if item != nil {
				out = append(out, item)
			}
		}
		return out
	case []any:
		out := make([]map[string]any, 0, len(items))
		for _, item := range items {
			if mapped, ok := item.(map[string]any); ok && mapped != nil {
				out = append(out, mapped)
			}
		}
		return out
	default:
		return nil
	}
}

func routePolicyAnyList(raw any) []any {
	switch items := raw.(type) {
	case []any:
		return items
	case []map[string]any:
		out := make([]any, 0, len(items))
		for _, item := range items {
			out = append(out, item)
		}
		return out
	default:
		return nil
	}
}

func routePolicyNestedMap(src map[string]any, key string) map[string]any {
	if src == nil {
		return nil
	}
	if mapped, ok := src[key].(map[string]any); ok {
		return mapped
	}
	return nil
}

func routePolicyReasonStrings(raw any) []string {
	switch items := raw.(type) {
	case []string:
		out := make([]string, 0, len(items))
		for _, item := range items {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				out = append(out, trimmed)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(items))
		for _, item := range items {
			if trimmed := strings.TrimSpace(stringify(item)); trimmed != "" {
				out = append(out, trimmed)
			}
		}
		return out
	default:
		if trimmed := strings.TrimSpace(stringify(raw)); trimmed != "" {
			return []string{trimmed}
		}
		return nil
	}
}

func routeEgressProjection(routeNode routePolicyNode, policy map[string]any, egressNodes map[string]routePolicyNode, managedBackhauls map[string][]routePolicyBackhaul) map[string]any {
	if policy == nil {
		policy = map[string]any{}
	}
	mode := strings.ToLower(routePolicyString(policy, "egress_mode", "mode"))
	nodeID := routePolicyString(policy, "egress_node_id", "node_id")
	nextHop := routePolicyString(policy, "egress_next_hop", "next_hop", "backhaul_next_hop")
	iface := routePolicyString(policy, "egress_interface", "interface", "backhaul_interface")
	table := routePolicyString(policy, "routing_table", "table")
	current := map[string]any{
		"node_id": routeNode.ID,
		"name":    routeNode.Name,
		"role":    firstNonEmptyRouteValue(routeNode.Role, "unknown"),
		"address": routeNode.Address,
	}
	base := map[string]any{
		"current_node": current,
		"next_hop":     nextHop,
		"interface":    iface,
		"table":        table,
	}
	if mode == "" || mode == "auto" {
		if strings.EqualFold(routeNode.Role, "egress") {
			base["mode"] = "local_breakout"
			base["status"] = "candidate"
			base["selected_node"] = current
			base["reasons"] = []string{"route node is an egress node; local breakout is the default output"}
			return base
		}
		base["mode"] = "explicit_egress_required"
		base["status"] = "blocked"
		base["reasons"] = []string{"ingress node requires explicit route policy egress_mode"}
		return base
	}
	switch mode {
	case "local", "direct", "local_breakout":
		base["mode"] = "local_breakout"
		base["status"] = "candidate"
		base["selected_node"] = current
		if strings.EqualFold(routeNode.Role, "ingress") {
			base["reasons"] = []string{"traffic is explicitly configured to exit through the current ingress node"}
		} else {
			base["reasons"] = []string{"traffic exits through the current egress node"}
		}
		return base
	case "node", "remote_node", "egress_node":
		if nodeID == "" {
			base["mode"] = "egress_node"
			base["status"] = "blocked"
			base["reasons"] = []string{"egress_node_id is required for remote egress output"}
			return base
		}
		selected, ok := egressNodes[nodeID]
		if !ok {
			base["mode"] = "egress_node"
			base["status"] = "blocked"
			base["selected_node"] = map[string]any{"node_id": nodeID}
			base["reasons"] = []string{"selected egress node is not an active egress node"}
			return base
		}
		selectedMap := map[string]any{"node_id": selected.ID, "name": selected.Name, "role": selected.Role, "address": selected.Address}
		base["selected_node"] = selectedMap
		if selected.ID == routeNode.ID {
			base["mode"] = "local_breakout"
			base["status"] = "candidate"
			base["reasons"] = []string{"selected egress node is the current route node"}
			return base
		}
		base["mode"] = "remote_egress"
		if nextHop == "" && iface == "" {
			if candidates := managedBackhauls[selected.ID]; len(candidates) > 0 {
				backhaul := candidates[0]
				effectiveTable := firstNonEmptyRouteValue(table, backhaul.RoutingTable)
				base["status"] = "candidate"
				base["interface"] = backhaul.InterfaceName
				base["table"] = effectiveTable
				base["next_hop"] = ""
				base["managed_backhaul"] = routePolicyBackhaulProjection(backhaul, effectiveTable)
				base["managed_backhauls"] = routePolicyBackhaulProjections(candidates, effectiveTable)
				base["reasons"] = []string{"remote egress uses active managed backhaul transports ordered by route_metric"}
				return base
			}
			base["status"] = "blocked"
			base["reasons"] = []string{"remote egress requires egress_next_hop, egress_interface/backhaul_interface or an active managed backhaul link"}
			return base
		}
		base["status"] = "candidate"
		base["reasons"] = []string{"remote egress output has an explicit backhaul next-hop or interface"}
		return base
	default:
		base["mode"] = mode
		base["status"] = "blocked"
		base["reasons"] = []string{"unsupported egress_mode"}
		return base
	}
}

func routePolicyBackhaulProjection(backhaul routePolicyBackhaul, effectiveTable string) map[string]any {
	table := firstNonEmptyRouteValue(effectiveTable, backhaul.RoutingTable)
	return map[string]any{
		"link_id":         backhaul.LinkID,
		"transport_id":    backhaul.TransportID,
		"driver":          backhaul.Driver,
		"priority":        backhaul.Priority,
		"interface":       backhaul.InterfaceName,
		"ingress_address": backhaul.IngressAddress,
		"egress_address":  backhaul.EgressAddress,
		"routing_table":   backhaul.RoutingTable,
		"table":           table,
		"route_metric":    backhaul.RouteMetric,
		"status":          backhaul.Status,
	}
}

func routePolicyBackhaulProjections(backhauls []routePolicyBackhaul, effectiveTable string) []any {
	out := make([]any, 0, len(backhauls))
	for _, backhaul := range backhauls {
		out = append(out, routePolicyBackhaulProjection(backhaul, effectiveTable))
	}
	return out
}

func routePolicyString(policy map[string]any, keys ...string) string {
	nested, _ := policy["egress"].(map[string]any)
	for _, key := range keys {
		if value := strings.TrimSpace(stringify(policy[key])); value != "" {
			return value
		}
		if nested != nil {
			if value := strings.TrimSpace(stringify(nested[key])); value != "" {
				return value
			}
		}
	}
	return ""
}

func routeEgressEnforceable(egress map[string]any) bool {
	if egress == nil {
		return false
	}
	if strings.ToLower(strings.TrimSpace(stringify(egress["status"]))) != "candidate" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(stringify(egress["mode"]))) {
	case "local_breakout":
		return true
	case "remote_egress":
		iface := strings.TrimSpace(stringify(egress["interface"]))
		table := strings.TrimSpace(stringify(egress["table"]))
		return iface != "" && table != "" && !strings.EqualFold(table, "main")
	default:
		return false
	}
}

func routeSourceIdentityEnforceable(sourceType, sourceValue string) bool {
	switch sourceType {
	case "wireguard_ip":
		return routeIPOrPrefixValid(sourceValue)
	default:
		return false
	}
}

func routeDestinationEnforceable(destinationType, destination string) bool {
	switch destinationType {
	case "cidr":
		prefix, err := netip.ParsePrefix(destination)
		return err == nil && prefix.Addr().Is4()
	case "endpoint":
		host := endpointHostForRoute(destination)
		addr, err := netip.ParseAddr(host)
		return err == nil && addr.Is4()
	default:
		return false
	}
}

func routeProtocolPortsEnforceable(protocol, ports string) bool {
	if protocol == "any" || protocol == "icmp" {
		ports = strings.ToLower(strings.TrimSpace(ports))
		return ports == "" || ports == "any" || ports == "*"
	}
	return protocol == "tcp" || protocol == "udp"
}

func routeIPOrPrefixValid(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	if addr, err := netip.ParseAddr(value); err == nil {
		return addr.Is4()
	}
	prefix, err := netip.ParsePrefix(value)
	return err == nil && prefix.Addr().Is4()
}

func endpointHostForRoute(destination string) string {
	host := strings.TrimSpace(destination)
	if hp, err := netip.ParseAddrPort(host); err == nil {
		return hp.Addr().String()
	}
	if splitHost, splitPort, err := net.SplitHostPort(host); err == nil {
		if port, parseErr := strconv.Atoi(splitPort); parseErr == nil && port >= 1 && port <= 65535 {
			return strings.Trim(splitHost, "[]")
		}
	}
	if strings.Count(host, ":") == 1 {
		splitHost, splitPort, _ := strings.Cut(host, ":")
		if port, err := strconv.Atoi(splitPort); err == nil && port >= 1 && port <= 65535 {
			return strings.TrimSpace(splitHost)
		}
	}
	return host
}

func intFromRouteSummary(value any) int {
	switch x := value.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	default:
		return 0
	}
}

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func firstRouteIdentityValue(values ...any) string {
	for _, raw := range values {
		if value := strings.TrimSpace(stringify(raw)); value != "" {
			return value
		}
	}
	return ""
}

func (s *Store) queueRoutePolicyForRoute(ctx context.Context, route domain.ClientAccessRoute) {
	if route.NodeID == nil || strings.TrimSpace(*route.NodeID) == "" {
		return
	}
	_, _ = s.CreateNodeRoutePolicyApplyJob(ctx, strings.TrimSpace(*route.NodeID))
}

func (s *Store) queueRoutePolicyJobForInstance(ctx context.Context, instanceID string) {
	instance, err := s.GetInstance(ctx, strings.TrimSpace(instanceID))
	if err != nil || strings.TrimSpace(instance.NodeID) == "" {
		return
	}
	_, _ = s.CreateNodeRoutePolicyApplyJob(ctx, instance.NodeID)
}

func (s *Store) queueRoutePolicyJobsForClient(ctx context.Context, clientID string) {
	rows, err := s.db.Query(ctx, `select distinct node_id::text from client_access_routes where client_account_id=$1 and node_id is not null`, strings.TrimSpace(clientID))
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var nodeID string
		if err := rows.Scan(&nodeID); err == nil && strings.TrimSpace(nodeID) != "" {
			_, _ = s.CreateNodeRoutePolicyApplyJob(ctx, nodeID)
		}
	}
}

func normalizeClientAccessRoute(route *domain.ClientAccessRoute) error {
	route.Name = strings.TrimSpace(route.Name)
	route.Status = strings.ToLower(strings.TrimSpace(route.Status))
	route.Action = strings.ToLower(strings.TrimSpace(route.Action))
	route.DestinationType = strings.ToLower(strings.TrimSpace(route.DestinationType))
	route.Destination = strings.TrimSpace(route.Destination)
	route.Protocol = strings.ToLower(strings.TrimSpace(route.Protocol))
	route.Ports = strings.TrimSpace(route.Ports)
	route.Description = strings.TrimSpace(route.Description)
	if route.Name == "" {
		route.Name = route.Destination
	}
	if route.Status == "" {
		route.Status = "active"
	}
	if route.Action == "" {
		route.Action = "allow"
	}
	if route.Protocol == "" {
		route.Protocol = "any"
	}
	if route.Ports == "" {
		route.Ports = "*"
	}
	if route.Policy == nil {
		route.Policy = map[string]any{}
	}
	if route.Metadata == nil {
		route.Metadata = map[string]any{}
	}
	if !in(route.Status, "pending", "active", "disabled", "revoked") {
		return fmt.Errorf("unsupported route status %q", route.Status)
	}
	if !in(route.Action, "allow", "deny") {
		return fmt.Errorf("unsupported route action %q", route.Action)
	}
	if !in(route.DestinationType, "endpoint", "cidr", "dns", "service") {
		return fmt.Errorf("unsupported destination_type %q", route.DestinationType)
	}
	if !in(route.Protocol, "any", "tcp", "udp", "icmp") {
		return fmt.Errorf("unsupported route protocol %q", route.Protocol)
	}
	if route.Destination == "" {
		return errors.New("route destination is required")
	}
	if err := validateRouteDestination(route.DestinationType, route.Destination); err != nil {
		return err
	}
	if err := validateRoutePorts(route.Protocol, route.Ports); err != nil {
		return err
	}
	return nil
}

func validateRouteDestination(destinationType, destination string) error {
	switch destinationType {
	case "cidr":
		if _, err := netip.ParsePrefix(destination); err != nil {
			return fmt.Errorf("destination must be a valid CIDR prefix: %w", err)
		}
	case "dns":
		if !routeDNSNamePattern.MatchString(destination) {
			return errors.New("destination must be a valid DNS name")
		}
	case "endpoint":
		host := destination
		if strings.Contains(host, ":") {
			if hp, err := netip.ParseAddrPort(host); err == nil {
				host = hp.Addr().String()
			} else if splitHost, splitPort, err := net.SplitHostPort(host); err == nil {
				port, parseErr := strconv.Atoi(splitPort)
				if parseErr != nil || port < 1 || port > 65535 {
					return errors.New("endpoint port must be between 1 and 65535")
				}
				host = strings.Trim(splitHost, "[]")
			} else if strings.Count(host, ":") == 1 {
				splitHost, splitPort, _ := strings.Cut(host, ":")
				port, parseErr := strconv.Atoi(splitPort)
				if parseErr != nil || port < 1 || port > 65535 {
					return errors.New("endpoint port must be between 1 and 65535")
				}
				host = splitHost
			}
		}
		if _, err := netip.ParseAddr(host); err == nil {
			return nil
		}
		if !routeDNSNamePattern.MatchString(host) {
			return errors.New("endpoint destination must be an IP address or DNS name")
		}
	case "service":
		if !isSafeRouteIdentifier(destination) {
			return errors.New("service destination must be a safe service identifier")
		}
	}
	return nil
}

func firstNonEmptyRouteValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return "route"
}

func isSafeRouteIdentifier(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.' || r == ':' {
			continue
		}
		return false
	}
	return true
}

func validateRoutePorts(protocol, ports string) error {
	if protocol == "icmp" || protocol == "any" {
		return nil
	}
	if ports == "*" {
		return nil
	}
	for _, part := range strings.Split(ports, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return errors.New("route ports contain an empty item")
		}
		limits := strings.Split(part, "-")
		if len(limits) > 2 {
			return fmt.Errorf("invalid port range %q", part)
		}
		for _, raw := range limits {
			n, err := strconv.Atoi(strings.TrimSpace(raw))
			if err != nil || n < 1 || n > 65535 {
				return fmt.Errorf("invalid route port %q", raw)
			}
		}
		if len(limits) == 2 {
			left, _ := strconv.Atoi(strings.TrimSpace(limits[0]))
			right, _ := strconv.Atoi(strings.TrimSpace(limits[1]))
			if left > right {
				return fmt.Errorf("invalid route port range %q", part)
			}
		}
	}
	return nil
}

func scanClientAccessRoute(row pgx.Row) (domain.ClientAccessRoute, error) {
	var route domain.ClientAccessRoute
	var policyRaw, metadataRaw []byte
	if err := row.Scan(
		&route.ID,
		&route.ClientAccountID,
		&route.ServiceAccessID,
		&route.InstanceID,
		&route.NodeID,
		&route.Name,
		&route.Status,
		&route.Action,
		&route.DestinationType,
		&route.Destination,
		&route.Protocol,
		&route.Ports,
		&route.Description,
		&policyRaw,
		&metadataRaw,
		&route.CreatedAt,
		&route.UpdatedAt,
	); err != nil {
		return domain.ClientAccessRoute{}, err
	}
	_ = json.Unmarshal(policyRaw, &route.Policy)
	_ = json.Unmarshal(metadataRaw, &route.Metadata)
	if route.Policy == nil {
		route.Policy = map[string]any{}
	}
	if route.Metadata == nil {
		route.Metadata = map[string]any{}
	}
	return route, nil
}

func isBaselineRoute(route domain.ClientAccessRoute) bool {
	value, ok := route.Metadata["baseline"]
	if !ok {
		return false
	}
	switch x := value.(type) {
	case bool:
		return x
	case string:
		return strings.EqualFold(x, "true")
	default:
		return false
	}
}
