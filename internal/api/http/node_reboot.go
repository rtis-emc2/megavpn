package http

import (
	"errors"
	nethttp "net/http"

	"github.com/jackc/pgx/v5"
	"github.com/rtis-emc2/megavpn/internal/domain"
)

const (
	nodeRebootRequestInvalid       = "node_reboot_request_invalid"
	nodeRebootNodeNotFound         = "node_reboot_node_not_found"
	nodeRebootConfirmationMismatch = "node_reboot_confirmation_mismatch"
	nodeRebootAgentMissing         = "node_reboot_agent_missing"
	nodeRebootAgentUnavailable     = "node_reboot_agent_unavailable"
	nodeRebootConflict             = "node_reboot_conflict"
	nodeRebootInternalError        = "node_reboot_internal_error"
)

func writeNodeRebootError(w nethttp.ResponseWriter, err error) {
	var validationErr domain.ValidationError
	switch {
	case errors.As(err, &validationErr):
		writeSensitiveCodeErr(w, nethttp.StatusBadRequest, nodeRebootRequestInvalid, validationErr.Error())
	case errors.Is(err, pgx.ErrNoRows):
		writeSensitiveCodeErr(w, nethttp.StatusNotFound, nodeRebootNodeNotFound, "node not found")
	case errors.Is(err, domain.ErrNodeRebootConfirmationMismatch):
		writeSensitiveCodeErr(w, nethttp.StatusConflict, nodeRebootConfirmationMismatch, "node confirmation does not match")
	case errors.Is(err, domain.ErrNodeRebootAgentMissing):
		writeSensitiveCodeErr(w, nethttp.StatusConflict, nodeRebootAgentMissing, "active agent identity is missing")
	case errors.Is(err, domain.ErrNodeRebootAgentUnavailable):
		writeSensitiveCodeErr(w, nethttp.StatusConflict, nodeRebootAgentUnavailable, "agent channel is unavailable for reboot")
	case errors.Is(err, domain.ErrNodeRebootConflict):
		writeSensitiveCodeErr(w, nethttp.StatusConflict, nodeRebootConflict, "node reboot conflicts with an active node job")
	default:
		writeSensitiveCodeErr(w, nethttp.StatusInternalServerError, nodeRebootInternalError, "node reboot queue failed")
	}
}
