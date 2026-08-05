package accesspolicy

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

func TestDecodeVLESSRejectsUnsafeStoredPolicy(t *testing.T) {
	tests := []struct {
		name   string
		policy string
		want   string
	}{
		{name: "unknown top-level field", policy: `{"access_mode":"instance_default","exec":"id"}`, want: "not supported"},
		{name: "managed user selector", policy: `{"rules":[{"type":"field","user":["other-client"]}]}`, want: "not supported"},
		{name: "invalid outbound tag", policy: `{"outbound_tag":"direct; include /tmp/payload"}`, want: "outbound_tag"},
		{name: "nested rule object", policy: `{"rules":[{"domain":{"value":"example.com"}}]}`, want: "string or an array"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecodeVLESS(domain.ClientAccessGroup{
				GroupKey: "secure_group", DisplayName: "Secure group", Status: "active",
				PolicyJSON: json.RawMessage(tt.policy),
			})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestNormalizeVLESSPreservesCaseSensitiveOutboundTag(t *testing.T) {
	raw, err := NormalizeVLESS(json.RawMessage(`{
		"access_mode":"instance_default",
		"egress_mode":"default",
		"outbound_tag":"ProviderRoute",
		"rules":[{"outboundTag":"ProviderRoute","domain":["example.com"]}]
	}`), false)
	if err != nil {
		t.Fatalf("NormalizeVLESS: %v", err)
	}
	var policy map[string]any
	if err := json.Unmarshal(raw, &policy); err != nil {
		t.Fatalf("decode normalized policy: %v", err)
	}
	if policy["outbound_tag"] != "ProviderRoute" {
		t.Fatalf("outbound_tag = %#v, want case preserved", policy["outbound_tag"])
	}
	rules, _ := policy["rules"].([]any)
	rule, _ := rules[0].(map[string]any)
	if rule["outbound_tag"] != "ProviderRoute" {
		t.Fatalf("rule outbound_tag = %#v, want case preserved", rule["outbound_tag"])
	}
}
