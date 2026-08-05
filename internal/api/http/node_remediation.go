package http

import (
	"errors"
	nethttp "net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

const (
	nodeStaleRotationMaxExpectedJobIDs     = 20
	nodeStaleRotationMaxConfirmationLength = 512
	nodeStaleRotationMinReasonLength       = 5
	nodeStaleRotationMaxReasonLength       = 500
)

var nodeStaleRotationJobIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

type nodeStaleRotationClearRequest struct {
	Confirmation              string   `json:"confirmation"`
	Reason                    string   `json:"reason"`
	AcknowledgeCancelRotation bool     `json:"acknowledge_cancel_rotation"`
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
	writeJSON(w, nethttp.StatusOK, preview)
}

func (s *Server) clearNodeStaleRotation(w nethttp.ResponseWriter, r *nethttp.Request) {
	var req nodeStaleRotationClearRequest
	if !decode(r, &req) {
		writeNodeStaleRotationValidationError(w, "node_stale_rotation_request_invalid", "invalid stale rotation clear request")
		return
	}
	command, err := validateNodeStaleRotationClearRequest(req)
	if err != nil {
		writeNodeStaleRotationValidationError(w, "node_stale_rotation_request_invalid", err.Error())
		return
	}
	result, err := s.store.ClearNodeStalePendingRotation(r.Context(), idParam(r), command)
	if err != nil {
		writeNodeStaleRotationError(w, err)
		return
	}
	writeJSON(w, nethttp.StatusOK, result)
}

func validateNodeStaleRotationClearRequest(req nodeStaleRotationClearRequest) (domain.NodeStaleRotationClearCommand, error) {
	confirmation := strings.TrimSpace(req.Confirmation)
	reason := strings.TrimSpace(req.Reason)
	if confirmation == "" {
		return domain.NodeStaleRotationClearCommand{}, errors.New("confirmation is required")
	}
	if !utf8.ValidString(confirmation) || utf8.RuneCountInString(confirmation) > nodeStaleRotationMaxConfirmationLength || containsControlCharacter(confirmation) {
		return domain.NodeStaleRotationClearCommand{}, errors.New("confirmation is invalid")
	}
	reasonLength := utf8.RuneCountInString(reason)
	if !utf8.ValidString(reason) || reasonLength < nodeStaleRotationMinReasonLength || reasonLength > nodeStaleRotationMaxReasonLength || unsafeNodeStaleRotationReason(reason) {
		return domain.NodeStaleRotationClearCommand{}, errors.New("reason is invalid")
	}
	if !req.AcknowledgeCancelRotation {
		return domain.NodeStaleRotationClearCommand{}, errors.New("explicit cancellation acknowledgement is required")
	}
	if len(req.ExpectedJobIDs) == 0 || len(req.ExpectedJobIDs) > nodeStaleRotationMaxExpectedJobIDs {
		return domain.NodeStaleRotationClearCommand{}, errors.New("expected job set is invalid")
	}
	expectedJobIDs := make([]string, 0, len(req.ExpectedJobIDs))
	seen := make(map[string]struct{}, len(req.ExpectedJobIDs))
	for _, rawJobID := range req.ExpectedJobIDs {
		jobID := strings.TrimSpace(rawJobID)
		if !nodeStaleRotationJobIDPattern.MatchString(jobID) {
			return domain.NodeStaleRotationClearCommand{}, errors.New("expected job set is invalid")
		}
		if _, duplicate := seen[jobID]; duplicate {
			return domain.NodeStaleRotationClearCommand{}, errors.New("expected job set contains duplicates")
		}
		seen[jobID] = struct{}{}
		expectedJobIDs = append(expectedJobIDs, jobID)
	}
	return domain.NodeStaleRotationClearCommand{
		Confirmation:   confirmation,
		Reason:         reason,
		ExpectedJobIDs: expectedJobIDs,
	}, nil
}

func unsafeNodeStaleRotationReason(reason string) bool {
	if containsControlCharacter(reason) || strings.ContainsAny(reason, "{}") {
		return true
	}
	lower := strings.ToLower(reason)
	for _, marker := range []string{
		"authorization:", "bearer ", "agent_token", "enrollment_token", "token_hash",
		"private_key", "secret_ref", "credential", "x-megavpn-agent-signature", "x-megavpn-agent-nonce",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func containsControlCharacter(value string) bool {
	for _, r := range value {
		if r < 32 || r == 127 {
			return true
		}
	}
	return false
}

func writeNodeStaleRotationValidationError(w nethttp.ResponseWriter, code, message string) {
	writeJSON(w, nethttp.StatusBadRequest, response{"status": "error", "code": code, "error": message})
}

func writeNodeStaleRotationError(w nethttp.ResponseWriter, err error) {
	status := nethttp.StatusInternalServerError
	code := "node_stale_rotation_internal_error"
	message := "stale rotation operation failed"
	switch {
	case errors.Is(err, domain.ErrNodeStaleRotationNodeNotFound):
		status, code, message = nethttp.StatusNotFound, "node_stale_rotation_node_not_found", "node not found"
	case errors.Is(err, domain.ErrNodeStaleRotationConfirmationMismatch):
		status, code, message = nethttp.StatusConflict, "node_stale_rotation_confirmation_mismatch", "confirmation does not match the selected node"
	case errors.Is(err, domain.ErrNodeStaleRotationNotFound):
		status, code, message = nethttp.StatusConflict, "node_stale_rotation_not_found", "no safe stale rotation candidates remain"
	case errors.Is(err, domain.ErrNodeStaleRotationPreviewChanged):
		status, code, message = nethttp.StatusConflict, "node_stale_rotation_preview_changed", "stale rotation preview changed"
	case errors.Is(err, domain.ErrNodeStaleRotationEvidenceAmbiguous):
		status, code, message = nethttp.StatusConflict, "node_stale_rotation_evidence_ambiguous", "stale rotation evidence is ambiguous"
	}
	if status >= nethttp.StatusInternalServerError {
		if recorder, ok := w.(interface{ recordPrivateError(string) }); ok {
			recorder.recordPrivateError(privateErrorMessage(err.Error()))
		}
	}
	writeJSON(w, status, response{"status": "error", "code": code, "error": message})
}
