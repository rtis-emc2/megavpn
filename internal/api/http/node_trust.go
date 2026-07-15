package http

import (
	"errors"
	nethttp "net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rtis-emc2/megavpn/internal/domain"
)

const (
	nodeAgentRevokeRequestInvalid       = "node_agent_revoke_request_invalid"
	nodeAgentRevokeNodeNotFound         = "node_agent_revoke_node_not_found"
	nodeAgentIdentityMissing            = "node_agent_identity_missing"
	nodeAgentRevokeConfirmationMismatch = "node_agent_revoke_confirmation_mismatch"
	nodeAgentRevokeConflict             = "node_agent_revoke_conflict"
	nodeAgentRevokeInternalError        = "node_agent_revoke_internal_error"
)

type nodeAgentIdentityRevokeRequest struct {
	Confirmation string `json:"confirmation"`
	Reason       string `json:"reason"`
}

func (s *Server) rotateNodeAgentToken(w nethttp.ResponseWriter, r *nethttp.Request) {
	jobRecord, err := s.store.CreateNodeAgentTokenRotateJob(r.Context(), idParam(r))
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 202, redactedJob(jobRecord))
}

func (s *Server) rotateNodeEnrollmentToken(w nethttp.ResponseWriter, r *nethttp.Request) {
	ttlHours, _ := strconv.Atoi(r.URL.Query().Get("ttl_hours"))
	ttl := time.Duration(ttlHours) * time.Hour
	token, err := s.store.RotateNodeEnrollmentToken(r.Context(), idParam(r), ttl)
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 200, token)
}

func (s *Server) revokeNodeEnrollmentToken(w nethttp.ResponseWriter, r *nethttp.Request) {
	token, err := s.store.RevokeNodeEnrollmentToken(r.Context(), idParam(r), r.PathValue("token_id"))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeErr(w, 404, "enrollment token not found")
			return
		}
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 200, token)
}

func (s *Server) revokeNodeAgentIdentity(w nethttp.ResponseWriter, r *nethttp.Request) {
	var req nodeAgentIdentityRevokeRequest
	if !decode(r, &req) {
		writeSensitiveCodeErr(w, nethttp.StatusBadRequest, nodeAgentRevokeRequestInvalid, "invalid agent identity revoke payload")
		return
	}
	authCtx, ok := authFromRequest(r)
	if !ok {
		writeSensitiveCodeErr(w, nethttp.StatusUnauthorized, nodeAgentRevokeRequestInvalid, "authentication required")
		return
	}
	input := domain.NodeAgentIdentityRevokeInput{
		Confirmation: req.Confirmation,
		Reason:       req.Reason,
		ActorUserID:  &authCtx.User.ID,
	}
	if err := input.NormalizeAndValidate(); err != nil {
		writeSensitiveCodeErr(w, nethttp.StatusBadRequest, nodeAgentRevokeRequestInvalid, err.Error())
		return
	}
	result, err := s.store.RevokeNodeAgentIdentity(r.Context(), idParam(r), input)
	if err != nil {
		writeNodeAgentRevokeError(w, err)
		return
	}
	writeSensitiveJSON(w, nethttp.StatusOK, result)
}

func writeNodeAgentRevokeError(w nethttp.ResponseWriter, err error) {
	var validationErr domain.ValidationError
	switch {
	case errors.As(err, &validationErr):
		writeSensitiveCodeErr(w, nethttp.StatusBadRequest, nodeAgentRevokeRequestInvalid, validationErr.Error())
	case errors.Is(err, pgx.ErrNoRows):
		writeSensitiveCodeErr(w, nethttp.StatusNotFound, nodeAgentRevokeNodeNotFound, "node not found")
	case errors.Is(err, domain.ErrNodeAgentIdentityMissing):
		writeSensitiveCodeErr(w, nethttp.StatusConflict, nodeAgentIdentityMissing, "node agent identity is missing")
	case errors.Is(err, domain.ErrNodeAgentRevokeConfirmationMismatch):
		writeSensitiveCodeErr(w, nethttp.StatusConflict, nodeAgentRevokeConfirmationMismatch, "node confirmation does not match")
	case errors.Is(err, domain.ErrNodeAgentRevokeConflict):
		writeSensitiveCodeErr(w, nethttp.StatusConflict, nodeAgentRevokeConflict, "node agent identity cannot be revoked from its current state")
	default:
		writeSensitiveCodeErr(w, nethttp.StatusInternalServerError, nodeAgentRevokeInternalError, "agent identity revoke failed")
	}
}
