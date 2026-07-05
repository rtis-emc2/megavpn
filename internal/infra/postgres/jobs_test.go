package postgres

import (
	"strings"
	"testing"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

func TestNormalizeJobForInsertRequiresQueuedStatus(t *testing.T) {
	t.Parallel()

	_, _, err := normalizeJobForInsert(domain.Job{
		Type:   "artifact.build",
		Status: "succeeded",
		Payload: map[string]any{
			"client_id": "client-1",
		},
	})
	if err == nil {
		t.Fatal("expected non-queued status to be rejected")
	}
	if !strings.Contains(err.Error(), "queued") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNormalizeJobForInsertDefaultsQueuedStatus(t *testing.T) {
	t.Parallel()

	job, _, err := normalizeJobForInsert(domain.Job{
		Type: "artifact.build",
		Payload: map[string]any{
			"client_id": "client-1",
		},
	})
	if err != nil {
		t.Fatalf("normalize job: %v", err)
	}
	if job.Status != "queued" {
		t.Fatalf("status = %q, want queued", job.Status)
	}
}
