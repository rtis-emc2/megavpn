package postgres

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

func TestNormalizeClientAccessGroupInputRejectsUnsupportedServices(t *testing.T) {
	_, err := normalizeClientAccessGroupInput(domain.ClientAccessGroupInput{
		ServiceCode: "openvpn",
		GroupKey:    "openvpn_clients",
		DisplayName: "OpenVPN clients",
		Status:      "active",
	}, true)
	if err == nil {
		t.Fatal("openvpn client access group create must be rejected until materialization is implemented")
	}
	if !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNormalizeClientAccessGroupInputRejectsUnsafeVLESSPolicy(t *testing.T) {
	tests := []struct {
		name   string
		policy string
		want   string
	}{
		{name: "unknown top-level field", policy: `{"access_mode":"instance_default","exec":"id"}`, want: "not supported"},
		{name: "unknown rule field", policy: `{"rules":[{"type":"field","user":["other-client"]}]}`, want: "not supported"},
		{name: "invalid outbound tag", policy: `{"outbound_tag":"direct; include /tmp/payload"}`, want: "outbound_tag"},
		{name: "egress without node", policy: `{"access_mode":"egress_node","egress_mode":"egress_node"}`, want: "egress_node_id"},
		{name: "instance without target", policy: `{"access_mode":"instance_only","egress_mode":"instance_only"}`, want: "target_instance_id"},
		{name: "invalid ad block type", policy: `{"ad_block":"true"}`, want: "boolean"},
		{name: "nested rule object", policy: `{"rules":[{"domain":{"value":"example.com"}}]}`, want: "string or an array"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := normalizeClientAccessGroupInput(domain.ClientAccessGroupInput{
				ServiceCode: "vless",
				GroupKey:    "secure_group",
				DisplayName: "Secure group",
				Status:      "active",
				PolicyJSON:  json.RawMessage(tt.policy),
			}, true)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestNormalizeClientAccessGroupInputCanonicalizesVLESSPolicy(t *testing.T) {
	out, err := normalizeClientAccessGroupInput(domain.ClientAccessGroupInput{
		ServiceCode: "vless",
		GroupKey:    "local_users",
		DisplayName: "Local users",
		PolicyJSON:  json.RawMessage(`{"access_mode":"local_breakout","egress_mode":"local_breakout","outbound_tag":"direct","rules":[{"domain":["geosite:private"],"outboundTag":"direct"}]}`),
	}, true)
	if err != nil {
		t.Fatalf("normalize policy: %v", err)
	}
	var policy map[string]any
	if err := json.Unmarshal(out.PolicyJSON, &policy); err != nil {
		t.Fatalf("decode normalized policy: %v", err)
	}
	rules, ok := policy["rules"].([]any)
	if !ok || len(rules) != 1 {
		t.Fatalf("rules = %#v, want one rule", policy["rules"])
	}
	rule, _ := rules[0].(map[string]any)
	if rule["outbound_tag"] != "direct" || rule["type"] != "field" {
		t.Fatalf("normalized rule = %#v", rule)
	}
}

func TestNormalizeClientAccessGroupInputAllowsVLESS(t *testing.T) {
	out, err := normalizeClientAccessGroupInput(domain.ClientAccessGroupInput{
		ServiceCode: "xray-core",
		GroupKey:    "out_usa_sf",
		DisplayName: "Outgoing USA San Francisco",
		Status:      "active",
	}, true)
	if err != nil {
		t.Fatalf("vless input rejected: %v", err)
	}
	if out.ServiceCode != "vless" {
		t.Fatalf("service = %q, want vless", out.ServiceCode)
	}
}
