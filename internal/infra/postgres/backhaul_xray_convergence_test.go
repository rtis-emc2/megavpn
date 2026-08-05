package postgres

import (
	"testing"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

func TestSeedXrayBackhaulEgressNodeIDUsesKnownLink(t *testing.T) {
	t.Parallel()

	spec := map[string]any{
		"xray_egress": map[string]any{
			"mode":         "remote_egress",
			"link_id":      "backhaul-1",
			"send_through": "10.240.254.237",
		},
		"vless_groups": []any{
			map[string]any{
				"key":         "route",
				"egress_mode": "egress_node",
				"egress": map[string]any{
					"transport_id": "transport-1",
					"send_through": "10.240.254.237",
				},
			},
		},
	}
	changed := seedXrayBackhaulEgressNodeID(spec, domain.BackhaulLink{
		ID:           "backhaul-1",
		EgressNodeID: "egress-1",
		Transports: []domain.BackhaulTransport{
			{ID: "transport-1"},
		},
	})
	if !changed {
		t.Fatal("seedXrayBackhaulEgressNodeID() changed = false, want true")
	}
	egress := mapFromAny(spec["xray_egress"])
	if got := firstString(egress["egress_node_id"]); got != "egress-1" {
		t.Fatalf("xray_egress egress_node_id = %q, want egress-1: %#v", got, egress)
	}
	groups := sliceRuleItemsFromAny(spec["vless_groups"])
	group := mapFromAny(groups[0])
	groupEgress := mapFromAny(group["egress"])
	if got := firstString(groupEgress["egress_node_id"]); got != "egress-1" {
		t.Fatalf("group egress_node_id = %q, want egress-1: %#v", got, groupEgress)
	}
}
