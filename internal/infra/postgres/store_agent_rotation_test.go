package postgres

import (
	"sort"
	"strings"
	"testing"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

func TestSetAgentRotationExecutionMaterialDropsInternalTrustMetadata(t *testing.T) {
	t.Parallel()

	job := domain.Job{Payload: map[string]any{
		"node_id":                       "node-1",
		"new_agent_token_secret_ref_id": "secret-ref-internal",
		"new_agent_token_hash":          "token-hash-internal",
		"new_token_hint":                "abcd...1234",
	}}
	setAgentRotationExecutionMaterial(&job, "pending-agent-token")

	keys := make([]string, 0, len(job.Payload))
	for key := range job.Payload {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if got, want := strings.Join(keys, ","), "new_agent_token,new_token_hint,node_id"; got != want {
		t.Fatalf("agent rotation execution payload keys = %q, want %q", got, want)
	}
	if job.Payload["new_agent_token"] != "pending-agent-token" || job.Payload["new_token_hint"] != "abcd...1234" {
		t.Fatal("agent rotation execution material was not preserved")
	}
}
