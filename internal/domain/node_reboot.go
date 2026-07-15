package domain

import (
	"errors"
	"strings"
)

const (
	NodeRebootMaxConfirmationLength = 512
	NodeRebootMinReasonLength       = 5
	NodeRebootMaxReasonLength       = 500
)

var (
	ErrNodeRebootConfirmationMismatch = errors.New("node reboot confirmation mismatch")
	ErrNodeRebootAgentMissing         = errors.New("node reboot agent identity is missing")
	ErrNodeRebootAgentUnavailable     = errors.New("node reboot agent channel is unavailable")
	ErrNodeRebootConflict             = errors.New("node reboot conflicts with an active node job")
	ErrNodeRebootAuditFailed          = errors.New("node reboot audit failed")
)

type NodeRebootJobCreateInput struct {
	Confirmation string
	Reason       string
	ActorUserID  *string
}

func (in *NodeRebootJobCreateInput) NormalizeAndValidate() error {
	in.Confirmation = strings.TrimSpace(in.Confirmation)
	in.Reason = strings.TrimSpace(in.Reason)

	if err := ValidateNodeRebootConfirmation(in.Confirmation); err != nil {
		return err
	}
	if err := ValidateNodeRebootReason(in.Reason); err != nil {
		return err
	}
	if in.ActorUserID == nil || strings.TrimSpace(*in.ActorUserID) == "" {
		return ValidationError{Message: "authenticated actor is required"}
	}
	return nil
}

func ValidateNodeRebootConfirmation(confirmation string) error {
	return validateBoundedRequiredSingleLine("confirmation", strings.TrimSpace(confirmation), 1, NodeRebootMaxConfirmationLength)
}

func ValidateNodeRebootReason(reason string) error {
	reason = strings.TrimSpace(reason)
	if err := validateBoundedRequiredSingleLine("reason", reason, NodeRebootMinReasonLength, NodeRebootMaxReasonLength); err != nil {
		return err
	}
	if reasonLooksLikeSecretOrRequestBody(reason) {
		return ValidationError{Message: "reason must not contain tokens, credentials, headers, or request bodies"}
	}
	return nil
}
