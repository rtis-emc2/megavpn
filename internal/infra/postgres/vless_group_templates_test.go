package postgres

import (
	"testing"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

func TestNormalizeVLESSGroupTemplateGeneratesKeyFromLabel(t *testing.T) {
	template, err := normalizeVLESSGroupTemplate(domain.VLESSGroupTemplate{
		Label:      "Remote Egress Premium",
		AccessMode: "instance_default",
	}, 0)
	if err != nil {
		t.Fatalf("normalizeVLESSGroupTemplate() error = %v", err)
	}
	if template.Key != "remote_egress_premium" {
		t.Fatalf("Key = %q, want remote_egress_premium", template.Key)
	}
}

func TestShouldQueueVLESSGroupCatalogApply(t *testing.T) {
	tests := []struct {
		name    string
		status  string
		enabled bool
		want    bool
	}{
		{name: "active enabled", status: "active", enabled: true, want: true},
		{name: "failed enabled", status: "failed", enabled: true, want: true},
		{name: "degraded enabled", status: "degraded", enabled: true, want: true},
		{name: "draft enabled", status: "draft", enabled: true, want: false},
		{name: "disabled status", status: "disabled", enabled: true, want: false},
		{name: "disabled flag", status: "active", enabled: false, want: false},
		{name: "deleting", status: "deleting", enabled: true, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := shouldQueueVLESSGroupCatalogApply(domain.Instance{Status: tt.status, Enabled: tt.enabled})
			if got != tt.want {
				t.Fatalf("shouldQueueVLESSGroupCatalogApply() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClearResolvedXrayVLESSGroupEgress(t *testing.T) {
	group := map[string]any{
		"key":                "remote",
		"outbound":           map[string]any{"tag": "egress-remote"},
		"egress":             map[string]any{"mode": "remote_egress"},
		"egress_resolved_by": "control-plane:vless-group-catalog-sync",
	}
	if !clearResolvedXrayVLESSGroupEgress(group) {
		t.Fatal("clearResolvedXrayVLESSGroupEgress() = false, want true")
	}
	for _, key := range []string{"outbound", "egress", "egress_resolved_by"} {
		if _, ok := group[key]; ok {
			t.Fatalf("%s was not cleared: %#v", key, group)
		}
	}
}
