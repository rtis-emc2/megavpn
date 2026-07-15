package domain

import (
	"errors"
	"sort"
	"strings"
	"time"
)

const (
	NodeStaleRotationMaxConfirmationLength = 512
	NodeStaleRotationMinReasonLength       = 5
	NodeStaleRotationMaxReasonLength       = 500
	NodeStaleRotationMaxExpectedJobIDs     = 20

	NodeStaleRotationReasonUnclaimedInactive          = "unclaimed_without_agent_progress"
	NodeStaleRotationReasonClaimedInactive            = "claimed_without_result_and_agent_inactive"
	NodeStaleRotationReasonFresh                      = "fresh_rotation"
	NodeStaleRotationReasonAgentProgressAfterCreation = "agent_progress_after_creation"
	NodeStaleRotationReasonAgentProgressAfterClaim    = "agent_progress_after_claim"
	NodeStaleRotationReasonClaimOrLeaseActive         = "claim_or_lease_still_active"
	NodeStaleRotationReasonResultSubmitted            = "result_already_submitted"
	NodeStaleRotationReasonSuperseded                 = "superseded_by_newer_rotation"
	NodeStaleRotationReasonClaimMissing               = "claim_evidence_missing"
	NodeStaleRotationReasonAgentIdentityInactive      = "agent_identity_not_active"
	NodeStaleRotationReasonEvidenceAmbiguous          = "evidence_ambiguous"
	NodeStaleRotationReasonPendingStateAmbiguous      = "pending_state_ambiguous"
)

var (
	ErrNodeStaleRotationConfirmationMismatch  = errors.New("node stale rotation confirmation mismatch")
	ErrNodeStaleRotationAcknowledgement       = errors.New("node stale rotation acknowledgement is required")
	ErrNodeStaleRotationNotFound              = errors.New("node stale rotation safe candidates not found")
	ErrNodeStaleRotationPreviewChanged        = errors.New("node stale rotation preview changed")
	ErrNodeStaleRotationEvidenceAmbiguous     = errors.New("node stale rotation evidence is ambiguous")
	ErrNodeStaleRotationPendingStateAmbiguous = errors.New("node stale rotation pending state is ambiguous")
	ErrNodeStaleRotationConflict              = errors.New("node stale rotation conflict")
	ErrNodeStaleRotationAuditFailed           = errors.New("node stale rotation audit failed")
	ErrNodeStaleRotationJobLogFailed          = errors.New("node stale rotation job log failed")
)

type NodeStaleRotationClearInput struct {
	Confirmation              string
	Reason                    string
	AcknowledgeCancelRotation bool
	ExpectedJobIDs            []string
	ActorUserID               *string
}

type NodeStaleRotationPreview struct {
	NodeID                string                       `json:"node_id"`
	StaleRotationDetected bool                         `json:"stale_rotation_detected"`
	TokenRotationStatus   string                       `json:"token_rotation_status"`
	EvaluatedAt           time.Time                    `json:"evaluated_at"`
	Candidates            []NodeStaleRotationCandidate `json:"candidates"`
}

type NodeStaleRotationCandidate struct {
	JobID        string     `json:"job_id"`
	Status       string     `json:"status"`
	CreatedAt    time.Time  `json:"created_at"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	LastClaimAt  *time.Time `json:"last_claim_at,omitempty"`
	LastResultAt *time.Time `json:"last_result_at,omitempty"`
	LastSeenAt   *time.Time `json:"last_seen_at,omitempty"`
	LastPollAt   *time.Time `json:"last_poll_at,omitempty"`
	AgeSeconds   int64      `json:"age_seconds"`
	StaleReason  string     `json:"stale_reason"`
	SafeToClear  bool       `json:"safe_to_clear"`
}

type NodeStaleRotationClearResult struct {
	Status                       string                        `json:"status"`
	NodeID                       string                        `json:"node_id"`
	ClearedCount                 int                           `json:"cleared_count"`
	ClearedJobs                  []NodeStaleRotationClearedJob `json:"cleared_jobs"`
	PendingRotationStateCleared  bool                          `json:"pending_rotation_state_cleared"`
	ActiveAgentIdentityPreserved bool                          `json:"active_agent_identity_preserved"`
}

type NodeStaleRotationClearedJob struct {
	JobID          string    `json:"job_id"`
	PreviousStatus string    `json:"previous_status"`
	Status         string    `json:"status"`
	StaleReason    string    `json:"stale_reason"`
	FinishedAt     time.Time `json:"finished_at"`
}

func (in *NodeStaleRotationClearInput) NormalizeAndValidate() error {
	in.Confirmation = strings.TrimSpace(in.Confirmation)
	in.Reason = strings.TrimSpace(in.Reason)

	if err := validateBoundedRequiredSingleLine("confirmation", in.Confirmation, 1, NodeStaleRotationMaxConfirmationLength); err != nil {
		return err
	}
	if err := validateBoundedRequiredSingleLine("reason", in.Reason, NodeStaleRotationMinReasonLength, NodeStaleRotationMaxReasonLength); err != nil {
		return err
	}
	if reasonLooksLikeSecretOrRequestBody(in.Reason) {
		return ValidationError{Message: "reason must not contain tokens, credentials, headers, or request bodies"}
	}
	if !in.AcknowledgeCancelRotation {
		return ErrNodeStaleRotationAcknowledgement
	}
	if in.ActorUserID == nil || strings.TrimSpace(*in.ActorUserID) == "" {
		return ValidationError{Message: "authenticated actor is required"}
	}

	if len(in.ExpectedJobIDs) == 0 {
		return ValidationError{Message: "expected_job_ids is required"}
	}
	if len(in.ExpectedJobIDs) > NodeStaleRotationMaxExpectedJobIDs {
		return ValidationError{Message: "expected_job_ids contains too many items"}
	}
	seen := make(map[string]struct{}, len(in.ExpectedJobIDs))
	normalized := make([]string, 0, len(in.ExpectedJobIDs))
	for _, raw := range in.ExpectedJobIDs {
		jobID := strings.TrimSpace(raw)
		if err := validateBoundedRequiredSingleLine("expected_job_ids", jobID, 1, 128); err != nil {
			return err
		}
		if _, ok := seen[jobID]; ok {
			return ValidationError{Message: "expected_job_ids must be unique"}
		}
		seen[jobID] = struct{}{}
		normalized = append(normalized, jobID)
	}
	sort.Strings(normalized)
	in.ExpectedJobIDs = normalized
	return nil
}
