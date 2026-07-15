package postgres

import (
	"testing"
	"time"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

func TestClassifyNodeStaleRotationCandidates(t *testing.T) {
	t.Parallel()

	now := time.Unix(10_000, 0).UTC()
	nodeID := "node-1"
	baseJob := func(id, status string, createdAt time.Time) domain.Job {
		return domain.Job{
			ID:        id,
			Type:      nodeAgentRotateTokenJobType,
			ScopeType: "node",
			ScopeID:   &nodeID,
			NodeID:    &nodeID,
			Status:    status,
			Payload: map[string]any{
				"new_agent_token_secret_ref_id": "secret-1",
				"new_agent_token_hash":          "hash-1",
			},
			CreatedAt: createdAt,
		}
	}
	activeAgent := func() nodeStaleRotationAgentEvidence {
		lastSeen := now.Add(-30 * time.Minute)
		return nodeStaleRotationAgentEvidence{
			Exists:     true,
			Status:     "active",
			LastSeenAt: &lastSeen,
		}
	}

	cases := []struct {
		name       string
		job        domain.Job
		jobs       []domain.Job
		agent      nodeStaleRotationAgentEvidence
		wantSafe   bool
		wantReason string
	}{
		{
			name:       "fresh queued is not stale",
			job:        baseJob("job-fresh", "queued", now.Add(-time.Minute)),
			agent:      activeAgent(),
			wantReason: domain.NodeStaleRotationReasonFresh,
		},
		{
			name: "queued with recent poll is not stale",
			job:  baseJob("job-poll", "queued", now.Add(-10*time.Minute)),
			agent: func() nodeStaleRotationAgentEvidence {
				agent := activeAgent()
				poll := now.Add(-5 * time.Minute)
				agent.LastJobPollAt = &poll
				return agent
			}(),
			wantReason: domain.NodeStaleRotationReasonAgentProgressAfterCreation,
		},
		{
			name:       "stale queued without progress is safe",
			job:        baseJob("job-queued", "queued", now.Add(-10*time.Minute)),
			agent:      activeAgent(),
			wantSafe:   true,
			wantReason: domain.NodeStaleRotationReasonUnclaimedInactive,
		},
		{
			name: "claimed with recent result is not stale",
			job: func() domain.Job {
				job := baseJob("job-result", "running", now.Add(-10*time.Minute))
				started := now.Add(-9 * time.Minute)
				expired := now.Add(-5 * time.Minute)
				owner := "agent:node-1"
				job.StartedAt = &started
				job.LockedBy = &owner
				job.LockedUntil = &expired
				return job
			}(),
			agent: func() nodeStaleRotationAgentEvidence {
				agent := activeAgent()
				claim := now.Add(-9 * time.Minute)
				result := now.Add(-8 * time.Minute)
				jobID := "job-result"
				agent.LastJobClaimAt = &claim
				agent.LastJobClaimJobID = &jobID
				agent.LastJobClaimType = nodeAgentRotateTokenJobType
				agent.LastJobResultAt = &result
				agent.LastJobResultJobID = &jobID
				agent.LastJobResultType = nodeAgentRotateTokenJobType
				agent.LastJobResultStatus = "succeeded"
				return agent
			}(),
			wantReason: domain.NodeStaleRotationReasonResultSubmitted,
		},
		{
			name: "claimed without result and inactive is safe",
			job: func() domain.Job {
				job := baseJob("job-claimed", "running", now.Add(-10*time.Minute))
				started := now.Add(-9 * time.Minute)
				expired := now.Add(-5 * time.Minute)
				owner := "agent:node-1"
				job.StartedAt = &started
				job.LockedBy = &owner
				job.LockedUntil = &expired
				return job
			}(),
			agent: func() nodeStaleRotationAgentEvidence {
				agent := activeAgent()
				claim := now.Add(-9 * time.Minute)
				jobID := "job-claimed"
				agent.LastJobClaimAt = &claim
				agent.LastJobClaimJobID = &jobID
				agent.LastJobClaimType = nodeAgentRotateTokenJobType
				return agent
			}(),
			wantSafe:   true,
			wantReason: domain.NodeStaleRotationReasonClaimedInactive,
		},
		{
			name: "newer successful rotation supersedes older active job",
			job:  baseJob("job-old", "queued", now.Add(-10*time.Minute)),
			jobs: []domain.Job{
				baseJob("job-old", "queued", now.Add(-10*time.Minute)),
				baseJob("job-new", "succeeded", now.Add(-5*time.Minute)),
			},
			agent:      activeAgent(),
			wantReason: domain.NodeStaleRotationReasonSuperseded,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			jobs := tc.jobs
			if len(jobs) == 0 {
				jobs = []domain.Job{tc.job}
			}
			got := classifyNodeStaleRotationCandidate(tc.job, tc.agent, jobs, now)
			if got.SafeToClear != tc.wantSafe || got.StaleReason != tc.wantReason {
				t.Fatalf("classification = safe:%t reason:%q, want safe:%t reason:%q", got.SafeToClear, got.StaleReason, tc.wantSafe, tc.wantReason)
			}
		})
	}
}
