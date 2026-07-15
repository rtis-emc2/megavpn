package http

import (
	"errors"
	nethttp "net/http"

	"github.com/jackc/pgx/v5"
	"github.com/rtis-emc2/megavpn/internal/domain"
)

const (
	nodeEmergencyCleanupRequestInvalid       = "node_emergency_cleanup_request_invalid"
	nodeEmergencyCleanupNodeNotFound         = "node_emergency_cleanup_node_not_found"
	nodeEmergencyCleanupConfirmationMismatch = "node_emergency_cleanup_confirmation_mismatch"
	nodeEmergencyCleanupMaintenanceRequired  = "node_emergency_cleanup_maintenance_required"
	nodeEmergencyCleanupAgentMissing         = "node_emergency_cleanup_agent_missing"
	nodeEmergencyCleanupAgentUnavailable     = "node_emergency_cleanup_agent_unavailable"
	nodeEmergencyCleanupScopeInvalid         = "node_emergency_cleanup_scope_invalid"
	nodeEmergencyCleanupAckRequired          = "node_emergency_cleanup_acknowledgement_required"
	nodeEmergencyCleanupPlanInvalid          = "node_emergency_cleanup_plan_invalid"
	nodeEmergencyCleanupConflict             = "node_emergency_cleanup_conflict"
	nodeEmergencyCleanupInternalError        = "node_emergency_cleanup_internal_error"
)

func writeNodeEmergencyCleanupError(w nethttp.ResponseWriter, err error) {
	var validationErr domain.ValidationError
	switch {
	case errors.As(err, &validationErr):
		writeSensitiveCodeErr(w, nethttp.StatusBadRequest, nodeEmergencyCleanupRequestInvalid, validationErr.Error())
	case errors.Is(err, domain.ErrNodeEmergencyCleanupScopeInvalid):
		writeSensitiveCodeErr(w, nethttp.StatusBadRequest, nodeEmergencyCleanupScopeInvalid, "emergency cleanup scope is invalid")
	case errors.Is(err, domain.ErrNodeEmergencyCleanupAcknowledgement):
		writeSensitiveCodeErr(w, nethttp.StatusBadRequest, nodeEmergencyCleanupAckRequired, "destructive cleanup acknowledgement is required")
	case errors.Is(err, pgx.ErrNoRows):
		writeSensitiveCodeErr(w, nethttp.StatusNotFound, nodeEmergencyCleanupNodeNotFound, "node not found")
	case errors.Is(err, domain.ErrNodeEmergencyCleanupConfirmationMismatch):
		writeSensitiveCodeErr(w, nethttp.StatusConflict, nodeEmergencyCleanupConfirmationMismatch, "node confirmation does not match")
	case errors.Is(err, domain.ErrNodeEmergencyCleanupMaintenanceRequired):
		writeSensitiveCodeErr(w, nethttp.StatusConflict, nodeEmergencyCleanupMaintenanceRequired, "node maintenance mode is required")
	case errors.Is(err, domain.ErrNodeEmergencyCleanupAgentMissing):
		writeSensitiveCodeErr(w, nethttp.StatusConflict, nodeEmergencyCleanupAgentMissing, "active agent identity is missing")
	case errors.Is(err, domain.ErrNodeEmergencyCleanupAgentUnavailable):
		writeSensitiveCodeErr(w, nethttp.StatusConflict, nodeEmergencyCleanupAgentUnavailable, "agent channel is unavailable for emergency cleanup")
	case errors.Is(err, domain.ErrNodeEmergencyCleanupPlanInvalid):
		writeSensitiveCodeErr(w, nethttp.StatusUnprocessableEntity, nodeEmergencyCleanupPlanInvalid, "emergency cleanup plan cannot be generated safely")
	case errors.Is(err, domain.ErrNodeEmergencyCleanupConflict):
		writeSensitiveCodeErr(w, nethttp.StatusConflict, nodeEmergencyCleanupConflict, "emergency cleanup conflicts with an active node job")
	default:
		writeSensitiveCodeErr(w, nethttp.StatusInternalServerError, nodeEmergencyCleanupInternalError, "emergency cleanup queue failed")
	}
}
