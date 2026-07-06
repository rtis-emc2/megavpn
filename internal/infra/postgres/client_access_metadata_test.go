package postgres

import (
	"context"
	"strings"
	"testing"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

func TestApplyXrayPublicClientEndpointMetadata(t *testing.T) {
	t.Parallel()

	metadata := map[string]any{}
	inbound := map[string]any{
		"service_code":   "xray-core",
		"endpoint_host":  "portal.nlgate.ru",
		"endpoint_port":  7080,
		"endpoint":       "portal.nlgate.ru:7080",
		"instance_name":  "portal-edge-xray-http-xray-ws",
		"service_label":  "Xray-core",
		"route_source":   "service",
		"route_endpoint": "portal.nlgate.ru:7080",
	}
	spec := map[string]any{
		"security":           "none",
		"network":            "ws",
		"path":               "/assets/rtis-sync",
		"public_security":    "tls",
		"public_network":     "ws",
		"public_path":        "/assets/rtis-sync",
		"public_host_header": "portal.nlgate.ru",
		"public_port":        443,
	}

	applyXrayPublicClientEndpointMetadata(metadata, inbound, spec)

	if got := firstString(inbound["endpoint"]); got != "portal.nlgate.ru:443" {
		t.Fatalf("inbound endpoint = %q, want public endpoint portal.nlgate.ru:443", got)
	}
	if got := firstString(inbound["backend_endpoint"]); got != "portal.nlgate.ru:7080" {
		t.Fatalf("backend endpoint = %q, want portal.nlgate.ru:7080", got)
	}
	if got := firstString(inbound["endpoint_kind"]); got != "public" {
		t.Fatalf("endpoint kind = %q, want public", got)
	}
	if got := firstString(metadata["public_security"]); got != "tls" {
		t.Fatalf("public security = %q, want tls", got)
	}
	if got := firstIntValue(metadata["public_port"]); got != 443 {
		t.Fatalf("public port = %d, want 443", got)
	}
}

func TestApplyXrayPublicClientEndpointMetadataIgnoresPlainBackend(t *testing.T) {
	t.Parallel()

	metadata := map[string]any{}
	inbound := map[string]any{
		"endpoint_host": "vpn.example.com",
		"endpoint_port": 8443,
		"endpoint":      "vpn.example.com:8443",
	}
	spec := map[string]any{
		"security": "reality",
		"network":  "tcp",
	}

	applyXrayPublicClientEndpointMetadata(metadata, inbound, spec)

	if got := firstString(inbound["endpoint"]); got != "vpn.example.com:8443" {
		t.Fatalf("plain endpoint changed to %q", got)
	}
	if got := firstString(inbound["endpoint_kind"]); got != "" {
		t.Fatalf("endpoint kind = %q, want empty for plain backend", got)
	}
}

func TestSelectXrayProvisioningGroupPreservesExistingGroup(t *testing.T) {
	t.Parallel()

	groups := []xrayVLESSGroup{{Key: "default"}, {Key: "out_usa_sf"}}
	allowed := map[string]struct{}{"default": {}, "out_usa_sf": {}}

	got, err := selectXrayProvisioningGroup("", "out_usa_sf", "default", groups, allowed, false, "instance-1")
	if err != nil {
		t.Fatalf("selectXrayProvisioningGroup() error = %v", err)
	}
	if got != "out_usa_sf" {
		t.Fatalf("selected group = %q, want existing out_usa_sf", got)
	}
}

func TestSelectXrayProvisioningGroupRejectsInvalidExplicitGroup(t *testing.T) {
	t.Parallel()

	groups := []xrayVLESSGroup{{Key: "default"}, {Key: "out_usa_sf"}}
	allowed := map[string]struct{}{"default": {}, "out_usa_sf": {}}

	_, err := selectXrayProvisioningGroup("route", "out_usa_sf", "default", groups, allowed, true, "instance-1")
	if err == nil {
		t.Fatal("selectXrayProvisioningGroup() error = nil, want invalid explicit group error")
	}
	if !strings.Contains(err.Error(), `vless outbound group "route"`) {
		t.Fatalf("error = %q, want invalid route group context", err.Error())
	}
}

func TestSelectXrayProvisioningGroupFallsBackFromStaleImplicitGroup(t *testing.T) {
	t.Parallel()

	groups := []xrayVLESSGroup{{Key: "default"}, {Key: "out_usa_sf"}}
	allowed := map[string]struct{}{"default": {}, "out_usa_sf": {}}

	got, err := selectXrayProvisioningGroup("", "route", "route", groups, allowed, false, "instance-1")
	if err != nil {
		t.Fatalf("selectXrayProvisioningGroup() error = %v", err)
	}
	if got != "default" {
		t.Fatalf("selected group = %q, want safe default fallback", got)
	}
}

func TestXrayVLESSGroupFromOptionsDoesNotInventRoute(t *testing.T) {
	t.Parallel()

	if got := xrayVLESSGroupFromOptions(nil); got != "" {
		t.Fatalf("nil options group = %q, want empty", got)
	}
	if got := xrayVLESSGroupFromOptions(map[string]any{}); got != "" {
		t.Fatalf("empty options group = %q, want empty", got)
	}
	if got := xrayVLESSGroupFromMetadata(map[string]any{"inbound_service": map[string]any{"outbound_group": "out_usa_sf"}}); got != "out_usa_sf" {
		t.Fatalf("metadata group = %q, want out_usa_sf", got)
	}
}

func TestResolveXrayVLESSGroupEgressWithResolverMaterializesSelectedEgress(t *testing.T) {
	t.Parallel()

	spec := map[string]any{
		"vless_groups": []any{
			map[string]any{
				"key":            "route",
				"label":          "Outgoing USA San Francisco",
				"egress_mode":    "egress_node",
				"egress_node_id": "node-egress",
			},
		},
	}
	calls := 0
	changed, err := resolveXrayVLESSGroupEgressWithResolver(
		context.Background(),
		domain.Instance{ID: "instance-ingress"},
		spec,
		func(_ context.Context, instanceID, egressNodeID string) (XrayVLESSEgressResolution, error) {
			calls++
			if instanceID != "instance-ingress" {
				t.Fatalf("instanceID = %q, want instance-ingress", instanceID)
			}
			if egressNodeID != "node-egress" {
				t.Fatalf("egressNodeID = %q, want node-egress", egressNodeID)
			}
			return XrayVLESSEgressResolution{
				Mode:           "remote_egress",
				CurrentNodeID:  "node-ingress",
				CurrentName:    "ingress",
				CurrentRole:    "ingress",
				EgressNodeID:   "node-egress",
				EgressNodeName: "egress",
				SendThrough:    "10.240.35.245",
				RoutingTable:   "21001",
				InterfaceName:  "mgbh1234567890",
			}, nil
		},
		"control-plane:vless-group-catalog-sync",
	)
	if err != nil {
		t.Fatalf("resolveXrayVLESSGroupEgressWithResolver() error = %v", err)
	}
	if !changed {
		t.Fatal("resolveXrayVLESSGroupEgressWithResolver() changed = false, want true")
	}
	if calls != 1 {
		t.Fatalf("resolver calls = %d, want 1", calls)
	}
	groups := sliceRuleItemsFromAny(spec["vless_groups"])
	if len(groups) != 1 {
		t.Fatalf("vless_groups = %#v, want one group", spec["vless_groups"])
	}
	group, _ := groups[0].(map[string]any)
	if got := firstString(group["outbound_tag"]); got != "egress-route" {
		t.Fatalf("outbound_tag = %q, want egress-route: %#v", got, group)
	}
	if got := firstString(group["egress_resolved_by"]); got != "control-plane:vless-group-catalog-sync" {
		t.Fatalf("egress_resolved_by = %q, want control-plane:vless-group-catalog-sync", got)
	}
	outbound, _ := group["outbound"].(map[string]any)
	if got := firstString(outbound["sendThrough"]); got != "10.240.35.245" {
		t.Fatalf("outbound sendThrough = %q, want 10.240.35.245: %#v", got, outbound)
	}
	egress, _ := group["egress"].(map[string]any)
	if got := firstString(egress["routing_table"]); got != "21001" {
		t.Fatalf("egress routing_table = %q, want 21001: %#v", got, egress)
	}
}

func TestResolveXrayDefaultEgressWithResolverRefreshesSendThrough(t *testing.T) {
	t.Parallel()

	spec := map[string]any{
		"xray_egress": map[string]any{
			"mode":           "remote_egress",
			"egress_node_id": "node-egress",
			"transport_id":   "transport-old",
			"send_through":   "10.240.254.237",
			"routing_table":  "59714",
			"interface":      "mgbh-old",
		},
		"xray_default_outbound": map[string]any{
			"tag":         "egress-default",
			"protocol":    "freedom",
			"sendThrough": "10.240.254.237",
		},
	}
	changed, err := resolveXrayDefaultEgressWithResolver(
		context.Background(),
		domain.Instance{ID: "instance-ingress", NodeID: "node-ingress"},
		spec,
		func(_ context.Context, instanceID, egressNodeID string) (XrayVLESSEgressResolution, error) {
			if instanceID != "instance-ingress" {
				t.Fatalf("instanceID = %q, want instance-ingress", instanceID)
			}
			if egressNodeID != "node-egress" {
				t.Fatalf("egressNodeID = %q, want node-egress", egressNodeID)
			}
			return XrayVLESSEgressResolution{
				Mode:           "remote_egress",
				CurrentNodeID:  "node-ingress",
				CurrentName:    "ingress",
				CurrentRole:    "ingress",
				EgressNodeID:   "node-egress",
				EgressNodeName: "egress",
				LinkID:         "backhaul-1",
				TransportID:    "transport-new",
				Driver:         "openvpn_udp",
				InterfaceName:  "mgbh8ae211fbfb",
				IngressAddress: "10.240.102.41/30",
				SendThrough:    "10.240.102.41",
				RoutingTable:   "59714",
				RouteMetric:    50,
			}, nil
		},
		"system:backhaul-transport-promote",
	)
	if err != nil {
		t.Fatalf("resolveXrayDefaultEgressWithResolver() error = %v", err)
	}
	if !changed {
		t.Fatal("resolveXrayDefaultEgressWithResolver() changed = false, want true")
	}
	egress := mapFromAny(spec["xray_egress"])
	if got := firstString(egress["transport_id"]); got != "transport-new" {
		t.Fatalf("xray_egress transport_id = %q, want transport-new: %#v", got, egress)
	}
	if got := firstString(egress["send_through"]); got != "10.240.102.41" {
		t.Fatalf("xray_egress send_through = %q, want 10.240.102.41: %#v", got, egress)
	}
	if got := firstString(egress["egress_resolved_by"]); got != "system:backhaul-transport-promote" {
		t.Fatalf("xray_egress egress_resolved_by = %q, want system:backhaul-transport-promote: %#v", got, egress)
	}
	outbound := mapFromAny(spec["xray_default_outbound"])
	if got := firstString(outbound["sendThrough"]); got != "10.240.102.41" {
		t.Fatalf("xray_default_outbound sendThrough = %q, want 10.240.102.41: %#v", got, outbound)
	}
}
