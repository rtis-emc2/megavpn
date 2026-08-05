package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

type nodeStaleRotationTestStore struct {
	Store
	preview        domain.NodeStaleRotationPreview
	previewErr     error
	clearResult    domain.NodeStaleRotationClearResult
	clearErr       error
	clearNodeID    string
	clearCommand   domain.NodeStaleRotationClearCommand
	clearCallCount int
}

func (s *nodeStaleRotationTestStore) PreviewNodeStaleRotation(context.Context, string) (domain.NodeStaleRotationPreview, error) {
	return s.preview, s.previewErr
}

func (s *nodeStaleRotationTestStore) ClearNodeStalePendingRotation(_ context.Context, nodeID string, command domain.NodeStaleRotationClearCommand) (domain.NodeStaleRotationClearResult, error) {
	s.clearCallCount++
	s.clearNodeID = nodeID
	s.clearCommand = command
	return s.clearResult, s.clearErr
}

func TestPreviewNodeStaleRotationReturnsReadOnlyContract(t *testing.T) {
	t.Parallel()
	evaluatedAt := time.Date(2026, 8, 6, 10, 0, 0, 0, time.UTC)
	store := &nodeStaleRotationTestStore{preview: domain.NodeStaleRotationPreview{
		NodeID:                "node-1",
		StaleRotationDetected: false,
		TokenRotationStatus:   "active",
		EvaluatedAt:           evaluatedAt,
		Candidates:            []domain.NodeStaleRotationCandidate{},
	}}
	server := &Server{store: store}
	req := httptest.NewRequest("GET", "/api/v1/nodes/node-1/diagnostics/stale-rotation", nil)
	req.SetPathValue("id", "node-1")
	rr := httptest.NewRecorder()

	server.previewNodeStaleRotation(rr, req)

	if rr.Code != 200 {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var got domain.NodeStaleRotationPreview
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.NodeID != "node-1" || got.StaleRotationDetected || got.EvaluatedAt != evaluatedAt {
		t.Fatalf("unexpected preview: %#v", got)
	}
	if bytes.Contains(rr.Body.Bytes(), []byte("secret")) || bytes.Contains(rr.Body.Bytes(), []byte("payload")) {
		t.Fatalf("preview exposed forbidden fields: %s", rr.Body.String())
	}
}

func TestClearNodeStaleRotationValidatesAndPassesExactCommand(t *testing.T) {
	t.Parallel()
	finishedAt := time.Date(2026, 8, 6, 10, 1, 0, 0, time.UTC)
	store := &nodeStaleRotationTestStore{clearResult: domain.NodeStaleRotationClearResult{
		Status:                       "cleared",
		NodeID:                       "node-1",
		ClearedCount:                 1,
		ClearedJobs:                  []domain.NodeStaleRotationClearedJob{{JobID: "job-1", PreviousStatus: "queued", Status: "cancelled", StaleReason: "unclaimed_without_agent_progress", FinishedAt: finishedAt}},
		PendingRotationStateCleared:  true,
		ActiveAgentIdentityPreserved: true,
	}}
	server := &Server{store: store}
	body := []byte(`{"confirmation":"Edge One","reason":"operator reviewed stale task","acknowledge_cancel_rotation":true,"expected_job_ids":["job-1"]}`)
	req := httptest.NewRequest("POST", "/api/v1/nodes/node-1/diagnostics/clear-stale-rotation", bytes.NewReader(body))
	req.SetPathValue("id", "node-1")
	rr := httptest.NewRecorder()

	server.clearNodeStaleRotation(rr, req)

	if rr.Code != 200 {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	wantCommand := domain.NodeStaleRotationClearCommand{Confirmation: "Edge One", Reason: "operator reviewed stale task", ExpectedJobIDs: []string{"job-1"}}
	if store.clearCallCount != 1 || store.clearNodeID != "node-1" || !reflect.DeepEqual(store.clearCommand, wantCommand) {
		t.Fatalf("unexpected clear call: count=%d node=%q command=%#v", store.clearCallCount, store.clearNodeID, store.clearCommand)
	}
	var got domain.NodeStaleRotationClearResult
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Status != "cleared" || got.ClearedCount != 1 || !got.ActiveAgentIdentityPreserved {
		t.Fatalf("unexpected result: %#v", got)
	}
}

func TestClearNodeStaleRotationRejectsIncompleteRequestBeforeStore(t *testing.T) {
	t.Parallel()
	store := &nodeStaleRotationTestStore{}
	server := &Server{store: store}
	req := httptest.NewRequest("POST", "/api/v1/nodes/node-1/diagnostics/clear-stale-rotation", bytes.NewReader([]byte(`{"confirmation":"Edge One","reason":"ok","expected_job_ids":["job-1"]}`)))
	req.SetPathValue("id", "node-1")
	rr := httptest.NewRecorder()

	server.clearNodeStaleRotation(rr, req)

	if rr.Code != 400 || store.clearCallCount != 0 {
		t.Fatalf("status=%d clear_calls=%d body=%s", rr.Code, store.clearCallCount, rr.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["code"] != "node_stale_rotation_request_invalid" {
		t.Fatalf("unexpected error payload: %#v", payload)
	}
}

func TestNodeStaleRotationErrorsUseStablePublicCodes(t *testing.T) {
	t.Parallel()
	store := &nodeStaleRotationTestStore{previewErr: domain.ErrNodeStaleRotationNodeNotFound}
	server := &Server{store: store}
	req := httptest.NewRequest("GET", "/api/v1/nodes/missing/diagnostics/stale-rotation", nil)
	req.SetPathValue("id", "missing")
	rr := httptest.NewRecorder()

	server.previewNodeStaleRotation(rr, req)

	if rr.Code != 404 {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["code"] != "node_stale_rotation_node_not_found" {
		t.Fatalf("unexpected error payload: %#v", payload)
	}
}
