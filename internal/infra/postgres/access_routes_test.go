package postgres

import (
	"testing"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

func TestFirstNonEmptyRouteValueDoesNotInventRoute(t *testing.T) {
	t.Parallel()

	if got := firstNonEmptyRouteValue("", " "); got != "" {
		t.Fatalf("firstNonEmptyRouteValue() = %q, want empty", got)
	}
	if got := firstNonEmptyRouteValue("", "explicit"); got != "explicit" {
		t.Fatalf("firstNonEmptyRouteValue() = %q, want explicit", got)
	}
}

func TestRouteEnforcementProjectionL3L4Candidate(t *testing.T) {
	t.Parallel()

	entry := map[string]any{
		"status":           "active",
		"action":           "allow",
		"destination_type": "cidr",
		"destination":      "10.20.0.0/16",
		"protocol":         "tcp",
		"ports":            "443",
		"source_identity": map[string]any{
			"type":  "wireguard_ip",
			"value": "10.66.0.2/32",
		},
		"egress": map[string]any{
			"mode":   "local_breakout",
			"status": "candidate",
		},
	}
	enforcement := routeEnforcementProjection(entry)
	if got := stringify(enforcement["mode"]); got != "l3_l4_candidate" {
		t.Fatalf("enforcement mode = %q, want l3_l4_candidate: %#v", got, enforcement)
	}
	summary := routePolicyEnforcementSummary([]map[string]any{{"enforcement": enforcement}})
	if got := intFromRouteSummary(summary["enforceable_routes"]); got != 1 {
		t.Fatalf("enforceable routes = %d, want 1: %#v", got, summary)
	}
	if got := intFromRouteSummary(summary["observe_only_routes"]); got != 0 {
		t.Fatalf("observe-only routes = %d, want 0: %#v", got, summary)
	}
}

func TestRouteEnforcementProjectionObserveOnlyForL7Identity(t *testing.T) {
	t.Parallel()

	entry := map[string]any{
		"status":           "active",
		"action":           "allow",
		"destination_type": "cidr",
		"destination":      "10.20.0.0/16",
		"protocol":         "tcp",
		"ports":            "443",
		"source_identity": map[string]any{
			"type":  "xray_uuid",
			"value": "client-uuid",
		},
		"egress": map[string]any{
			"mode":   "local_breakout",
			"status": "candidate",
		},
	}
	enforcement := routeEnforcementProjection(entry)
	if got := stringify(enforcement["mode"]); got != "observe_only" {
		t.Fatalf("enforcement mode = %q, want observe_only: %#v", got, enforcement)
	}
	reasons, _ := enforcement["reasons"].([]string)
	if len(reasons) == 0 {
		t.Fatalf("observe-only enforcement should include reasons: %#v", enforcement)
	}
}

func TestRouteEnforcementProjectionObserveOnlyForDNSDestination(t *testing.T) {
	t.Parallel()

	entry := map[string]any{
		"status":           "active",
		"action":           "allow",
		"destination_type": "dns",
		"destination":      "internal.example.com",
		"protocol":         "tcp",
		"ports":            "443",
		"source_identity": map[string]any{
			"type":  "wireguard_ip",
			"value": "10.66.0.2",
		},
		"egress": map[string]any{
			"mode":   "local_breakout",
			"status": "candidate",
		},
	}
	enforcement := routeEnforcementProjection(entry)
	if got := stringify(enforcement["mode"]); got != "observe_only" {
		t.Fatalf("enforcement mode = %q, want observe_only: %#v", got, enforcement)
	}
}

func TestRouteEnforcementProjectionObserveOnlyWithoutEgress(t *testing.T) {
	t.Parallel()

	entry := map[string]any{
		"status":           "active",
		"action":           "allow",
		"destination_type": "cidr",
		"destination":      "10.20.0.0/16",
		"protocol":         "tcp",
		"ports":            "443",
		"source_identity": map[string]any{
			"type":  "wireguard_ip",
			"value": "10.66.0.2/32",
		},
	}
	enforcement := routeEnforcementProjection(entry)
	if got := stringify(enforcement["mode"]); got != "observe_only" {
		t.Fatalf("enforcement mode = %q, want observe_only: %#v", got, enforcement)
	}
}

func TestRouteEnforcementProjectionObserveOnlyForDeny(t *testing.T) {
	t.Parallel()

	entry := map[string]any{
		"status":           "active",
		"action":           "deny",
		"destination_type": "cidr",
		"destination":      "10.20.0.0/16",
		"protocol":         "tcp",
		"ports":            "443",
		"source_identity":  map[string]any{"type": "wireguard_ip", "value": "10.66.0.2/32"},
		"egress":           map[string]any{"mode": "remote_egress", "status": "candidate", "interface": "mgbh123", "table": "21001"},
	}
	enforcement := routeEnforcementProjection(entry)
	if got := stringify(enforcement["mode"]); got != "observe_only" {
		t.Fatalf("enforcement mode = %q, want observe_only: %#v", got, enforcement)
	}
}

func TestRouteEnforcementProjectionObserveOnlyForIPv6(t *testing.T) {
	t.Parallel()

	entry := map[string]any{
		"status":           "active",
		"action":           "allow",
		"destination_type": "cidr",
		"destination":      "2001:db8::/64",
		"protocol":         "tcp",
		"ports":            "443",
		"source_identity":  map[string]any{"type": "wireguard_ip", "value": "10.66.0.2/32"},
		"egress":           map[string]any{"mode": "remote_egress", "status": "candidate", "interface": "mgbh123", "table": "21001"},
	}
	enforcement := routeEnforcementProjection(entry)
	if got := stringify(enforcement["mode"]); got != "observe_only" {
		t.Fatalf("enforcement mode = %q, want observe_only: %#v", got, enforcement)
	}
}

func TestRouteEgressProjectionIngressAutoRequiresExplicitOutput(t *testing.T) {
	t.Parallel()

	egress := routeEgressProjection(routePolicyNode{ID: "ingress-1", Name: "ingress-1", Role: "ingress"}, nil, nil, nil)
	if got := stringify(egress["status"]); got != "blocked" {
		t.Fatalf("egress status = %q, want blocked: %#v", got, egress)
	}
	if got := stringify(egress["mode"]); got != "explicit_egress_required" {
		t.Fatalf("egress mode = %q, want explicit_egress_required: %#v", got, egress)
	}
}

func TestRouteEgressProjectionIngressLocalBreakout(t *testing.T) {
	t.Parallel()

	egress := routeEgressProjection(
		routePolicyNode{ID: "ingress-1", Name: "ingress-1", Role: "ingress"},
		map[string]any{"egress_mode": "local_breakout"},
		nil,
		nil,
	)
	if got := stringify(egress["status"]); got != "candidate" {
		t.Fatalf("egress status = %q, want candidate: %#v", got, egress)
	}
	if got := stringify(egress["mode"]); got != "local_breakout" {
		t.Fatalf("egress mode = %q, want local_breakout: %#v", got, egress)
	}
}

func TestRouteEgressProjectionRemoteEgressRequiresBackhaul(t *testing.T) {
	t.Parallel()

	egressNodes := map[string]routePolicyNode{
		"egress-1": {ID: "egress-1", Name: "egress-1", Role: "egress", Address: "10.0.0.2"},
	}
	egress := routeEgressProjection(
		routePolicyNode{ID: "ingress-1", Name: "ingress-1", Role: "ingress", Address: "10.0.0.1"},
		map[string]any{"egress_mode": "egress_node", "egress_node_id": "egress-1"},
		egressNodes,
		nil,
	)
	if got := stringify(egress["status"]); got != "blocked" {
		t.Fatalf("egress status = %q, want blocked: %#v", got, egress)
	}
}

func TestRouteEgressProjectionRemoteEgressWithNextHop(t *testing.T) {
	t.Parallel()

	egressNodes := map[string]routePolicyNode{
		"egress-1": {ID: "egress-1", Name: "egress-1", Role: "egress", Address: "10.0.0.2"},
	}
	egress := routeEgressProjection(
		routePolicyNode{ID: "ingress-1", Name: "ingress-1", Role: "ingress", Address: "10.0.0.1"},
		map[string]any{
			"egress": map[string]any{
				"mode":     "egress_node",
				"node_id":  "egress-1",
				"next_hop": "10.0.0.2",
			},
		},
		egressNodes,
		nil,
	)
	if got := stringify(egress["status"]); got != "candidate" {
		t.Fatalf("egress status = %q, want candidate: %#v", got, egress)
	}
	if got := stringify(egress["mode"]); got != "remote_egress" {
		t.Fatalf("egress mode = %q, want remote_egress: %#v", got, egress)
	}
}

func TestRouteEgressProjectionEgressNodeDefaultsToLocalBreakout(t *testing.T) {
	t.Parallel()

	egress := routeEgressProjection(routePolicyNode{ID: "egress-1", Name: "egress-1", Role: "egress"}, nil, nil, nil)
	if got := stringify(egress["status"]); got != "candidate" {
		t.Fatalf("egress status = %q, want candidate: %#v", got, egress)
	}
	if got := stringify(egress["mode"]); got != "local_breakout" {
		t.Fatalf("egress mode = %q, want local_breakout: %#v", got, egress)
	}
}

func TestRouteEgressProjectionUsesManagedBackhaul(t *testing.T) {
	t.Parallel()

	egressNodes := map[string]routePolicyNode{
		"egress-1": {ID: "egress-1", Name: "egress-1", Role: "egress", Address: "10.0.0.2"},
	}
	managed := map[string][]routePolicyBackhaul{
		"egress-1": {{
			LinkID:         "backhaul-1",
			TransportID:    "transport-1",
			Driver:         "wireguard",
			InterfaceName:  "mgbh123",
			IngressAddress: "10.240.1.1",
			EgressAddress:  "10.240.1.2",
			RoutingTable:   "main",
			RouteMetric:    50,
			Status:         "active",
		}},
	}
	egress := routeEgressProjection(
		routePolicyNode{ID: "ingress-1", Name: "ingress-1", Role: "ingress", Address: "10.0.0.1"},
		map[string]any{"egress_mode": "egress_node", "egress_node_id": "egress-1"},
		egressNodes,
		managed,
	)
	if got := stringify(egress["status"]); got != "candidate" {
		t.Fatalf("egress status = %q, want candidate: %#v", got, egress)
	}
	if got := stringify(egress["interface"]); got != "mgbh123" {
		t.Fatalf("egress interface = %q, want managed interface: %#v", got, egress)
	}
	backhaul, ok := egress["managed_backhaul"].(map[string]any)
	if !ok || stringify(backhaul["link_id"]) != "backhaul-1" {
		t.Fatalf("managed_backhaul = %#v, want link projection", egress["managed_backhaul"])
	}
	candidates, ok := egress["managed_backhauls"].([]any)
	if !ok || len(candidates) != 1 {
		t.Fatalf("managed_backhauls = %#v, want one candidate", egress["managed_backhauls"])
	}
}

func TestXrayVLESSSystemRoutesForSpecUsesSendThroughBackhaul(t *testing.T) {
	t.Parallel()

	managed := map[string][]routePolicyBackhaul{
		"egress-1": {{
			LinkID:         "backhaul-1",
			TransportID:    "transport-1",
			Driver:         "wireguard",
			Priority:       10,
			InterfaceName:  "mgbh1234567890",
			IngressAddress: "10.240.35.245/30",
			EgressAddress:  "10.240.35.246/30",
			RoutingTable:   "21001",
			RouteMetric:    70,
			Status:         "active",
		}},
	}
	routes := xrayVLESSSystemRoutesForSpec(routePolicyXrayInstance{
		ID:          "instance-1",
		Name:        "edge-vless",
		SystemdUnit: "megavpn-edge-vless",
		NodeID:      "ingress-1",
	}, map[string]any{
		"xray_egress": map[string]any{
			"mode":           "remote_egress",
			"egress_node_id": "egress-1",
			"send_through":   "10.240.35.245",
			"routing_table":  "21001",
			"interface":      "mgbh1234567890",
		},
		"xray_default_outbound": map[string]any{
			"tag":         "egress-default",
			"protocol":    "freedom",
			"sendThrough": "10.240.35.245",
		},
		"vless_groups": []any{
			map[string]any{
				"key":          "remote",
				"outbound_tag": "egress-remote",
				"egress": map[string]any{
					"mode":           "remote_egress",
					"egress_node_id": "egress-1",
					"send_through":   "10.240.35.245",
					"routing_table":  "21001",
					"interface":      "mgbh1234567890",
				},
				"outbound": map[string]any{
					"tag":         "egress-remote",
					"protocol":    "freedom",
					"sendThrough": "10.240.35.245",
				},
			},
		},
	}, managed)
	routes = dedupeRoutePolicySystemRoutes(routes)
	if len(routes) != 1 {
		t.Fatalf("system routes = %#v, want one deduplicated source route", routes)
	}
	route := routes[0]
	if got := stringify(route["status"]); got != "active" {
		t.Fatalf("status = %q, want active: %#v", got, route)
	}
	if got := stringify(route["source"]); got != "10.240.35.245/32" {
		t.Fatalf("source = %q, want sendThrough /32: %#v", got, route)
	}
	if got := stringify(route["table"]); got != "21001" {
		t.Fatalf("table = %q, want backhaul table: %#v", got, route)
	}
	if got := stringify(route["interface"]); got != "mgbh1234567890" {
		t.Fatalf("interface = %q, want managed backhaul interface: %#v", got, route)
	}
	refs, ok := route["references"].([]any)
	if !ok || len(refs) != 2 {
		t.Fatalf("references = %#v, want default and group refs", route["references"])
	}
	backhaul, ok := route["managed_backhaul"].(map[string]any)
	if !ok || stringify(backhaul["link_id"]) != "backhaul-1" {
		t.Fatalf("managed_backhaul = %#v, want backhaul projection", route["managed_backhaul"])
	}
}

func TestXrayVLESSSystemRoutesForSpecBlocksWithoutActiveManagedBackhaul(t *testing.T) {
	t.Parallel()

	routes := xrayVLESSSystemRoutesForSpec(routePolicyXrayInstance{
		ID:     "instance-1",
		Name:   "edge-vless",
		NodeID: "ingress-1",
	}, map[string]any{
		"xray_egress": map[string]any{
			"mode":           "remote_egress",
			"egress_node_id": "egress-1",
			"send_through":   "10.240.35.245",
			"routing_table":  "21001",
			"interface":      "mgbh1234567890",
		},
	}, nil)
	if len(routes) != 1 {
		t.Fatalf("system routes = %#v, want one blocked route", routes)
	}
	route := routes[0]
	if got := stringify(route["status"]); got != "blocked" {
		t.Fatalf("status = %q, want blocked: %#v", got, route)
	}
	reasons, ok := route["reasons"].([]string)
	if !ok || len(reasons) == 0 {
		t.Fatalf("reasons = %#v, want blocked reason", route["reasons"])
	}
	found := false
	for _, reason := range reasons {
		if reason == "xray remote egress requires an active managed backhaul matching the sendThrough source" {
			found = true
		}
	}
	if !found {
		t.Fatalf("reasons = %#v, want active managed backhaul reason", reasons)
	}
}

func TestRoutePolicyPreviewRedactsL7SourceIdentity(t *testing.T) {
	t.Parallel()

	preview := routePolicyPreviewForNode(testRoutePolicyNode(), map[string]any{
		"generated_at": "2026-07-06T00:00:00Z",
		"revision":     "rev-test",
		"routes": []map[string]any{{
			"route_id":         "route-1",
			"username":         "client-1",
			"service_code":     "xray-core",
			"status":           "active",
			"action":           "allow",
			"destination_type": "cidr",
			"destination":      "0.0.0.0/0",
			"protocol":         "any",
			"source_identity":  map[string]any{"type": "xray_uuid", "value": "8c4f0c2e-0000-4000-9000-000000000001"},
			"egress":           map[string]any{"mode": "local_breakout", "status": "candidate"},
			"enforcement":      map[string]any{"mode": "observe_only", "reasons": []string{"route source identity is not enforceable at L3/L4"}},
		}},
	})
	routes, ok := preview["routes"].([]any)
	if !ok || len(routes) != 1 {
		t.Fatalf("preview routes = %#v, want one route", preview["routes"])
	}
	route, ok := routes[0].(map[string]any)
	if !ok {
		t.Fatalf("route type = %T", routes[0])
	}
	identity, ok := route["source_identity"].(map[string]any)
	if !ok {
		t.Fatalf("source_identity type = %T", route["source_identity"])
	}
	if got := stringify(identity["value"]); got != "[redacted]" {
		t.Fatalf("source identity value = %q, want redacted", got)
	}
	if identity["redacted"] != true {
		t.Fatalf("source identity redacted flag = %#v, want true", identity["redacted"])
	}
}

func TestRoutePolicyPreviewSummarizesSystemWarnings(t *testing.T) {
	t.Parallel()

	preview := routePolicyPreviewForNode(testRoutePolicyNode(), map[string]any{
		"system_routes": []map[string]any{
			{
				"system_route_id": "xray-active",
				"kind":            "xray_vless_remote_egress",
				"status":          "active",
				"source":          "10.240.1.1/32",
				"destination":     "0.0.0.0/0",
				"table":           "21001",
				"interface":       "mgbh123",
			},
			{
				"system_route_id": "xray-blocked",
				"kind":            "xray_vless_remote_egress",
				"status":          "blocked",
				"source":          "10.240.2.1/32",
				"destination":     "0.0.0.0/0",
				"reasons":         []string{"xray remote egress requires a managed backhaul interface"},
			},
		},
	})
	summary, ok := preview["summary"].(map[string]any)
	if !ok {
		t.Fatalf("summary type = %T", preview["summary"])
	}
	if got := intFromRouteSummary(summary["active_system_route_count"]); got != 1 {
		t.Fatalf("active system routes = %d, want 1: %#v", got, summary)
	}
	if got := intFromRouteSummary(summary["blocked_system_route_count"]); got != 1 {
		t.Fatalf("blocked system routes = %d, want 1: %#v", got, summary)
	}
	if got := intFromRouteSummary(summary["warning_count"]); got != 1 {
		t.Fatalf("warning count = %d, want 1: %#v", got, summary)
	}
}

func testRoutePolicyNode() domain.Node {
	return domain.Node{
		ID:      "node-1",
		Name:    "ingress-1",
		Role:    "ingress",
		Status:  "online",
		Address: "203.0.113.10",
	}
}
