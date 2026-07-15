package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	NodeAgentRevokeMaxConfirmationLength = 512
	NodeAgentRevokeMinReasonLength       = 5
	NodeAgentRevokeMaxReasonLength       = 500
)

var (
	ErrNodeAgentIdentityMissing            = errors.New("node agent identity is missing")
	ErrNodeAgentRevokeConfirmationMismatch = errors.New("node agent revoke confirmation mismatch")
	ErrNodeAgentRevokeConflict             = errors.New("node agent revoke conflict")
	ErrNodeAgentRevokeAuditFailed          = errors.New("node agent revoke audit failed")
)

type NodeAgentIdentityRevokeInput struct {
	Confirmation string
	Reason       string
	ActorUserID  *string
}

type NodeAgentIdentityRevokeResult struct {
	Status                  string    `json:"status"`
	NodeID                  string    `json:"node_id"`
	AgentStatus             string    `json:"agent_status"`
	RevokedAt               time.Time `json:"revoked_at"`
	AlreadyRevoked          bool      `json:"already_revoked"`
	RevokedEnrollmentTokens int64     `json:"revoked_enrollment_tokens"`
}

func (in *NodeAgentIdentityRevokeInput) NormalizeAndValidate() error {
	in.Confirmation = strings.TrimSpace(in.Confirmation)
	in.Reason = strings.TrimSpace(in.Reason)

	if err := validateBoundedRequiredSingleLine("confirmation", in.Confirmation, 1, NodeAgentRevokeMaxConfirmationLength); err != nil {
		return err
	}
	if err := validateBoundedRequiredSingleLine("reason", in.Reason, NodeAgentRevokeMinReasonLength, NodeAgentRevokeMaxReasonLength); err != nil {
		return err
	}
	if reasonLooksLikeSecretOrRequestBody(in.Reason) {
		return ValidationError{Message: "reason must not contain tokens, credentials, headers, or request bodies"}
	}
	if in.ActorUserID == nil || strings.TrimSpace(*in.ActorUserID) == "" {
		return ValidationError{Message: "authenticated actor is required"}
	}
	return nil
}

func validateBoundedRequiredSingleLine(field, value string, minLen, maxLen int) error {
	if value == "" {
		return ValidationError{Message: fmt.Sprintf("%s is required", field)}
	}
	if len(value) < minLen {
		return ValidationError{Message: fmt.Sprintf("%s must be at least %d characters", field, minLen)}
	}
	if len(value) > maxLen {
		return ValidationError{Message: fmt.Sprintf("%s must be at most %d characters", field, maxLen)}
	}
	if err := ValidateSingleLine(field, value); err != nil {
		return ValidationError{Message: err.Error()}
	}
	return nil
}

func reasonLooksLikeSecretOrRequestBody(reason string) bool {
	lower := strings.ToLower(reason)
	if strings.ContainsAny(reason, "{}") {
		return true
	}
	for _, marker := range []string{
		"authorization:",
		"bearer ",
		"agent_token",
		"enrollment_token",
		"token_hash",
		"x-megavpn-agent-signature",
		"x-megavpn-agent-nonce",
		"private_key",
		"secret_ref",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}
