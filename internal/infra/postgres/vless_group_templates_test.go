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
