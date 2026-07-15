package http

import (
	"errors"
	nethttp "net/http"

	"github.com/jackc/pgx/v5"
	"github.com/rtis-emc2/megavpn/internal/domain"
)

const (
	nodeStaleRotationRequestInvalid        = "node_stale_rotation_request_invalid"
	nodeStaleRotationNodeNotFound          = "node_stale_rotation_node_not_found"
	nodeStaleRotationConfirmationMismatch  = "node_stale_rotation_confirmation_mismatch"
	nodeStaleRotationNotFound              = "node_stale_rotation_not_found"
	nodeStaleRotationPreviewChanged        = "node_stale_rotation_preview_changed"
	nodeStaleRotationEvidenceAmbiguous     = "node_stale_rotation_evidence_ambiguous"
	nodeStaleRotationPendingStateAmbiguous = "node_stale_rotation_pending_state_ambiguous"
	nodeStaleRotationConflict              = "node_stale_rotation_conflict"
	nodeStaleRotationInternalError         = "node_stale_rotation_internal_error"
)

type nodeStaleRotationClearRequest struct {
	Confirmation              string   `json:"confirmation"`
	Reason                    string   `json:"reason"`
	AcknowledgeCancelRotation *bool    `json:"acknowledge_cancel_rotation"`
	ExpectedJobIDs            []string `json:"expected_job_ids"`
}

func (s *Server) retryNodeInventorySync(w nethttp.ResponseWriter, r *nethttp.Request) {
	job, err := s.store.CreateNodeInventoryJob(r.Context(), idParam(r))
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 202, response{
		"status":  "queued",
		"message": "inventory sync queued",
		"job":     redactedJob(job),
	})
}

func (s *Server) retryNodeDiscoverySync(w nethttp.ResponseWriter, r *nethttp.Request) {
	job, err := s.store.CreateNodeServiceDiscoveryJob(r.Context(), idParam(r))
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 202, response{
		"status":  "queued",
		"message": "discovery sync queued",
		"job":     redactedJob(job),
	})
}

func (s *Server) requeueNodeStuckJob(w nethttp.ResponseWriter, r *nethttp.Request) {
	job, err := s.store.RequeueNodeStuckJob(r.Context(), idParam(r))
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 202, response{
		"status":  "queued",
		"message": "stuck node job requeued",
		"job":     redactedJob(job),
	})
}

func (s *Server) probeNodeChannel(w nethttp.ResponseWriter, r *nethttp.Request) {
	job, err := s.store.CreateNodeChannelProbeJob(r.Context(), idParam(r))
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 202, response{
		"status":  "queued",
		"message": "agent channel probe queued",
		"job":     redactedJob(job),
	})
}

func (s *Server) applyNodeRoutePolicy(w nethttp.ResponseWriter, r *nethttp.Request) {
	job, err := s.store.CreateNodeRoutePolicyApplyJob(r.Context(), idParam(r))
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 202, response{
		"status":  "queued",
		"message": "route policy apply queued",
		"job":     redactedJob(job),
	})
}

func (s *Server) cleanupNodeRoutePolicy(w nethttp.ResponseWriter, r *nethttp.Request) {
	job, err := s.store.CreateNodeRoutePolicyCleanupJob(r.Context(), idParam(r))
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 202, response{
		"status":  "queued",
		"message": "route policy cleanup queued",
		"job":     redactedJob(job),
	})
}

func (s *Server) previewNodeRoutePolicy(w nethttp.ResponseWriter, r *nethttp.Request) {
	preview, err := s.store.PreviewNodeRoutePolicy(r.Context(), idParam(r))
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 200, preview)
}

func (s *Server) previewNodeStaleRotation(w nethttp.ResponseWriter, r *nethttp.Request) {
	preview, err := s.store.PreviewNodeStaleRotation(r.Context(), idParam(r))
	if err != nil {
		writeNodeStaleRotationError(w, err)
		return
	}
	writeSensitiveJSON(w, nethttp.StatusOK, preview)
}

func (s *Server) clearNodeStaleRotation(w nethttp.ResponseWriter, r *nethttp.Request) {
	var req nodeStaleRotationClearRequest
	if !decode(r, &req) {
		writeSensitiveCodeErr(w, nethttp.StatusBadRequest, nodeStaleRotationRequestInvalid, "invalid stale rotation cleanup payload")
		return
	}
	authCtx, ok := authFromRequest(r)
	if !ok {
		writeSensitiveCodeErr(w, nethttp.StatusUnauthorized, nodeStaleRotationRequestInvalid, "authentication required")
		return
	}
	ack := false
	if req.AcknowledgeCancelRotation != nil {
		ack = *req.AcknowledgeCancelRotation
	}
	input := domain.NodeStaleRotationClearInput{
		Confirmation:              req.Confirmation,
		Reason:                    req.Reason,
		AcknowledgeCancelRotation: ack,
		ExpectedJobIDs:            req.ExpectedJobIDs,
		ActorUserID:               &authCtx.User.ID,
	}
	if err := input.NormalizeAndValidate(); err != nil {
		writeNodeStaleRotationError(w, err)
		return
	}
	result, err := s.store.ClearNodeStalePendingRotation(r.Context(), idParam(r), input)
	if err != nil {
		writeNodeStaleRotationError(w, err)
		return
	}
	writeSensitiveJSON(w, nethttp.StatusOK, result)
}

func writeNodeStaleRotationError(w nethttp.ResponseWriter, err error) {
	var validationErr domain.ValidationError
	switch {
	case errors.As(err, &validationErr):
		writeSensitiveCodeErr(w, nethttp.StatusBadRequest, nodeStaleRotationRequestInvalid, validationErr.Error())
	case errors.Is(err, domain.ErrNodeStaleRotationAcknowledgement):
		writeSensitiveCodeErr(w, nethttp.StatusBadRequest, nodeStaleRotationRequestInvalid, "stale rotation cancellation acknowledgement is required")
	case errors.Is(err, pgx.ErrNoRows):
		writeSensitiveCodeErr(w, nethttp.StatusNotFound, nodeStaleRotationNodeNotFound, "node not found")
	case errors.Is(err, domain.ErrNodeStaleRotationConfirmationMismatch):
		writeSensitiveCodeErr(w, nethttp.StatusConflict, nodeStaleRotationConfirmationMismatch, "node confirmation does not match")
	case errors.Is(err, domain.ErrNodeStaleRotationNotFound):
		writeSensitiveCodeErr(w, nethttp.StatusConflict, nodeStaleRotationNotFound, "no safe stale token rotation candidates found")
	case errors.Is(err, domain.ErrNodeStaleRotationPreviewChanged):
		writeSensitiveCodeErr(w, nethttp.StatusConflict, nodeStaleRotationPreviewChanged, "stale rotation preview changed; refresh before applying")
	case errors.Is(err, domain.ErrNodeStaleRotationEvidenceAmbiguous):
		writeSensitiveCodeErr(w, nethttp.StatusConflict, nodeStaleRotationEvidenceAmbiguous, "stale rotation evidence is ambiguous")
	case errors.Is(err, domain.ErrNodeStaleRotationPendingStateAmbiguous):
		writeSensitiveCodeErr(w, nethttp.StatusConflict, nodeStaleRotationPendingStateAmbiguous, "stale rotation pending state is ambiguous")
	case errors.Is(err, domain.ErrNodeStaleRotationConflict):
		writeSensitiveCodeErr(w, nethttp.StatusConflict, nodeStaleRotationConflict, "stale rotation cleanup conflicts with current node state")
	default:
		writeSensitiveCodeErr(w, nethttp.StatusInternalServerError, nodeStaleRotationInternalError, "stale rotation cleanup failed")
	}
}
