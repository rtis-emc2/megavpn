package postgres

import (
	"testing"
	"time"
)

func TestClassifyNodeStaleRotationCandidate(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 6, 10, 0, 0, 0, time.UTC)
	old := now.Add(-3 * time.Minute)
	fresh := now.Add(-time.Minute)
	claim := now.Add(-4 * time.Minute)
	expiredLease := now.Add(-time.Minute)
	activeLease := now.Add(time.Minute)
	jobID := "job-1"

	tests := []struct {
		name       string
		job        nodeStaleRotationJobEvidence
		agent      nodeStaleRotationAgentEvidence
		wantReason string
		wantSafe   bool
	}{
		{
			name:       "fresh queued rotation remains protected",
			job:        nodeStaleRotationJobEvidence{ID: jobID, Status: "queued", CreatedAt: fresh},
			agent:      nodeStaleRotationAgentEvidence{Status: "active"},
			wantReason: "fresh_rotation",
		},
		{
			name:       "stale unclaimed rotation without progress is safe",
			job:        nodeStaleRotationJobEvidence{ID: jobID, Status: "queued", CreatedAt: old},
			agent:      nodeStaleRotationAgentEvidence{Status: "active"},
			wantReason: "unclaimed_without_agent_progress",
			wantSafe:   true,
		},
		{
			name:       "agent progress after creation blocks clearing",
			job:        nodeStaleRotationJobEvidence{ID: jobID, Status: "queued", CreatedAt: old},
			agent:      nodeStaleRotationAgentEvidence{Status: "active", LastPollAt: pointerToTime(now.Add(-time.Minute))},
			wantReason: "agent_progress_after_creation",
		},
		{
			name:       "expired claimed rotation without progress is safe",
			job:        nodeStaleRotationJobEvidence{ID: jobID, Status: "running", CreatedAt: now.Add(-5 * time.Minute), LockedUntil: &expiredLease},
			agent:      nodeStaleRotationAgentEvidence{Status: "active", LastClaimAt: &claim, LastClaimJobID: &jobID},
			wantReason: "claimed_without_result_and_agent_inactive",
			wantSafe:   true,
		},
		{
			name:       "active lease blocks clearing",
			job:        nodeStaleRotationJobEvidence{ID: jobID, Status: "running", CreatedAt: now.Add(-5 * time.Minute), LockedUntil: &activeLease},
			agent:      nodeStaleRotationAgentEvidence{Status: "active", LastClaimAt: &claim, LastClaimJobID: &jobID},
			wantReason: "claim_or_lease_still_active",
		},
		{
			name:       "inactive identity blocks clearing",
			job:        nodeStaleRotationJobEvidence{ID: jobID, Status: "queued", CreatedAt: old},
			agent:      nodeStaleRotationAgentEvidence{Status: "revoked"},
			wantReason: "agent_identity_not_active",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got := classifyNodeStaleRotationCandidate(test.job, test.agent, now)
			if got.StaleReason != test.wantReason || got.SafeToClear != test.wantSafe {
				t.Fatalf("reason=%q safe=%v, want reason=%q safe=%v", got.StaleReason, got.SafeToClear, test.wantReason, test.wantSafe)
			}
		})
	}
}

func pointerToTime(value time.Time) *time.Time { return &value }
