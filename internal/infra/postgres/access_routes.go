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
	destination := strings.TrimSpace(instance.EndpointHost)
	if destination == "" {
		destination = strings.TrimSpace(instance.Name)
	}
	if destination == "" {
		destination = strings.TrimSpace(instance.ID)
	}
	destinationType := "endpoint"
	portText := "*"
	if instance.EndpointPort > 0 {
		portText = strconv.Itoa(instance.EndpointPort)
	}
	nodeID := strings.TrimSpace(instance.NodeID)
	now := time.Now().UTC()
	accessMeta, err := s.clientInboundServiceMetadata(ctx, instanceID)
	if err != nil {
		return err
	}
	inbound, _ := accessMeta["inbound_service"].(map[string]any)
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
	revision := routePolicyRevision(routes)
	enforcementSummary := routePolicyEnforcementSummary(routes)
	return map[string]any{
		"node_id":            strings.TrimSpace(nodeID),
		"generated_at":       time.Now().UTC().Format(time.RFC3339),
		"revision":           revision,
		"enforcement_mode":   "kernel_policy_apply",
		"output_path":        "/etc/megavpn/client-access-routes.json",
		"route_count":        len(routes),
		"active_route_count": activeCount,
		"enforcement":        enforcementSummary,
		"routes":             routes,
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

func routePolicyRevision(routes []map[string]any) string {
	b, err := json.Marshal(routes)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
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

func routePolicyEnforcementSummary(routes []map[string]any) map[string]any {
	out := map[string]any{
		"mode":                "kernel_policy_apply",
		"enforced":            false,
		"enforceable_routes":  0,
		"observe_only_routes": 0,
		"notes": []string{
			"agent materializes a signed route-policy snapshot",
			"agent applies kernel policy routing only for L3/L4 allow candidates with explicit egress output",
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
	return out
}

func routeEgressProjection(routeNode routePolicyNode, policy map[string]any, egressNodes map[string]routePolicyNode, managedBackhauls map[string]routePolicyBackhaul) map[string]any {
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
			if backhaul, ok := managedBackhauls[selected.ID]; ok {
				base["status"] = "candidate"
				base["interface"] = backhaul.InterfaceName
				base["table"] = firstNonEmptyRouteValue(table, backhaul.RoutingTable)
				base["next_hop"] = ""
				base["managed_backhaul"] = map[string]any{
					"link_id":         backhaul.LinkID,
					"transport_id":    backhaul.TransportID,
					"driver":          backhaul.Driver,
					"interface":       backhaul.InterfaceName,
					"ingress_address": backhaul.IngressAddress,
					"egress_address":  backhaul.EgressAddress,
					"route_metric":    backhaul.RouteMetric,
					"status":          backhaul.Status,
				}
				base["reasons"] = []string{"remote egress uses an active managed backhaul transport"}
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
