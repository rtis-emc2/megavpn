package postgres

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

func TestNormalizeClientAccessGroupInputAllowsReadyServices(t *testing.T) {
	for _, serviceCode := range []string{"openvpn", "wireguard", "l2tp", "http_proxy", "shadowsocks", "mtproto"} {
		t.Run(serviceCode, func(t *testing.T) {
			out, err := normalizeClientAccessGroupInput(domain.ClientAccessGroupInput{
				ServiceCode: serviceCode,
				GroupKey:    serviceCode + "_clients",
				DisplayName: serviceCode + " clients",
				Status:      "active",
				PolicyJSON:  json.RawMessage(`{"ignored":"for generic services"}`),
			}, true)
			if err != nil {
				t.Fatalf("ready service rejected: %v", err)
			}
			if string(out.PolicyJSON) != "{}" {
				t.Fatalf("generic policy = %s, want canonical empty policy", out.PolicyJSON)
			}
		})
	}
}

func TestNormalizeClientAccessGroupInputRejectsPlannedService(t *testing.T) {
	_, err := normalizeClientAccessGroupInput(domain.ClientAccessGroupInput{
		ServiceCode: "socks_proxy",
		GroupKey:    "socks_clients",
		DisplayName: "SOCKS clients",
		Status:      "active",
	}, true)
	if err == nil || !strings.Contains(err.Error(), "not available") {
		t.Fatalf("planned service error = %v, want not available", err)
	}
}

func TestNormalizeClientAccessGroupInputRejectsExternalEgressForNonVLESS(t *testing.T) {
	_, err := normalizeClientAccessGroupInput(domain.ClientAccessGroupInput{
		ServiceCode:             "wireguard",
		GroupKey:                "wireguard_clients",
		DisplayName:             "WireGuard clients",
		Status:                  "active",
		ExternalEgressProfileID: stringPtr("profile-1"),
	}, true)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "vless groups only") {
		t.Fatalf("external egress error = %v, want VLESS-only validation", err)
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
