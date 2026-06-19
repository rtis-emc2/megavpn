package postgres

import "testing"

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
	managed := map[string]routePolicyBackhaul{
		"egress-1": {
			LinkID:         "backhaul-1",
			TransportID:    "transport-1",
			Driver:         "wireguard",
			InterfaceName:  "mgbh123",
			IngressAddress: "10.240.1.1",
			EgressAddress:  "10.240.1.2",
			RoutingTable:   "main",
			RouteMetric:    50,
			Status:         "active",
		},
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
}
