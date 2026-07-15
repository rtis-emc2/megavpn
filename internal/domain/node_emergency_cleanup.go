package domain

import (
	"errors"
	"strings"
)

const (
	NodeEmergencyCleanupScopeServicesOnly = "services_only"
	NodeEmergencyCleanupScopeFullNode     = "full_node"

	NodeEmergencyCleanupMaxConfirmationLength = 512
	NodeEmergencyCleanupMinReasonLength       = 5
	NodeEmergencyCleanupMaxReasonLength       = 500
	NodeEmergencyCleanupPlanVersion           = 1
)

var (
	ErrNodeEmergencyCleanupConfirmationMismatch = errors.New("node emergency cleanup confirmation mismatch")
	ErrNodeEmergencyCleanupMaintenanceRequired  = errors.New("node emergency cleanup requires maintenance mode")
	ErrNodeEmergencyCleanupAgentMissing         = errors.New("node emergency cleanup agent identity is missing")
	ErrNodeEmergencyCleanupAgentUnavailable     = errors.New("node emergency cleanup agent channel is unavailable")
	ErrNodeEmergencyCleanupScopeInvalid         = errors.New("node emergency cleanup scope is invalid")
	ErrNodeEmergencyCleanupAcknowledgement      = errors.New("node emergency cleanup acknowledgement is required")
	ErrNodeEmergencyCleanupPlanInvalid          = errors.New("node emergency cleanup plan is invalid")
	ErrNodeEmergencyCleanupConflict             = errors.New("node emergency cleanup conflicts with an active node job")
	ErrNodeEmergencyCleanupAuditFailed          = errors.New("node emergency cleanup audit failed")
)

type NodeEmergencyCleanupJobCreateInput struct {
	CleanupScope                  string
	IncludeAgent                  bool
	Confirmation                  string
	Reason                        string
	AcknowledgeDestructiveCleanup bool
	AcknowledgeAgentRemoval       bool
	ActorUserID                   *string
}

type NodeEmergencyCleanupPlanSummary struct {
	CleanupScope          string         `json:"cleanup_scope"`
	IncludeAgent          bool           `json:"include_agent"`
	InstanceTargetCount   int            `json:"instance_target_count"`
	ServiceCounts         map[string]int `json:"service_counts"`
	NodeRuntimeCleanup    bool           `json:"node_runtime_cleanup"`
	AgentRemovalRequested bool           `json:"agent_removal_requested"`
}

type NodeEmergencyCleanupJobCreateResult struct {
	Job         Job
	PlanSummary NodeEmergencyCleanupPlanSummary
}

func (in *NodeEmergencyCleanupJobCreateInput) NormalizeAndValidate() error {
	in.CleanupScope = strings.TrimSpace(in.CleanupScope)
	in.Confirmation = strings.TrimSpace(in.Confirmation)
	in.Reason = strings.TrimSpace(in.Reason)

	if !IsNodeEmergencyCleanupScope(in.CleanupScope) {
		return ErrNodeEmergencyCleanupScopeInvalid
	}
	if in.IncludeAgent && in.CleanupScope != NodeEmergencyCleanupScopeFullNode {
		return ErrNodeEmergencyCleanupScopeInvalid
	}
	if !in.IncludeAgent && in.AcknowledgeAgentRemoval {
		return ErrNodeEmergencyCleanupAcknowledgement
	}
	if !in.AcknowledgeDestructiveCleanup {
		return ErrNodeEmergencyCleanupAcknowledgement
	}
	if in.IncludeAgent && !in.AcknowledgeAgentRemoval {
		return ErrNodeEmergencyCleanupAcknowledgement
	}
	if err := validateBoundedRequiredSingleLine("confirmation", in.Confirmation, 1, NodeEmergencyCleanupMaxConfirmationLength); err != nil {
		return err
	}
	if err := ValidateNodeEmergencyCleanupReason(in.Reason); err != nil {
		return err
	}
	if in.ActorUserID == nil || strings.TrimSpace(*in.ActorUserID) == "" {
		return ValidationError{Message: "authenticated actor is required"}
	}
	return nil
}

func IsNodeEmergencyCleanupScope(scope string) bool {
	switch scope {
	case NodeEmergencyCleanupScopeServicesOnly, NodeEmergencyCleanupScopeFullNode:
		return true
	default:
		return false
	}
}

func ValidateNodeEmergencyCleanupReason(reason string) error {
	reason = strings.TrimSpace(reason)
	if err := validateBoundedRequiredSingleLine("reason", reason, NodeEmergencyCleanupMinReasonLength, NodeEmergencyCleanupMaxReasonLength); err != nil {
		return err
	}
	if reasonLooksLikeSecretOrRequestBody(reason) {
		return ValidationError{Message: "reason must not contain tokens, credentials, headers, or request bodies"}
	}
	return nil
}
