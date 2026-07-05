package http

import (
	"strings"
	"testing"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

func TestValidateNodeProfileRejectsControlCharacters(t *testing.T) {
	t.Parallel()

	err := validateNodeProfile(domain.Node{
		Name:    "edge-1\nMEGAVPN_AGENT_TOKEN=attacker",
		Address: "203.0.113.10",
		Kind:    "remote",
		Role:    "egress",
	})
	if err == "" {
		t.Fatal("expected node profile with newline in name to be rejected")
	}
	if !strings.Contains(err, "control characters") {
		t.Fatalf("unexpected validation error: %q", err)
	}
}
