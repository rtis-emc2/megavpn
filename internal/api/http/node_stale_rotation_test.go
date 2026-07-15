package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rtis-emc2/megavpn/internal/domain"
)

type nodeStaleRotationHTTPTestStore struct {
	Store

	permissions   []string
	previewErr    error
	clearErr      error
	previewCalled bool
	clearCalled   bool
	nodeID        string
	input         domain.NodeStaleRotationClearInput
}

func (s *nodeStaleRotationHTTPTestStore) ResolveAuthContext(context.Context, string) (domain.AuthContext, error) {
	return domain.AuthContext{
		User:            domain.PlatformUser{ID: "user-1", Username: "operator"},
		Session:         domain.UserSession{ID: "session-1", UserID: "user-1", ExpiresAt: time.Now().Add(time.Hour)},
		PermissionCodes: append([]string(nil), s.permissions...),
	}, nil
}

func (s *nodeStaleRotationHTTPTestStore) PreviewNodeStaleRotation(_ context.Context, nodeID string) (domain.NodeStaleRotationPreview, error) {
	s.previewCalled = true
	s.nodeID = nodeID
	if s.previewErr != nil {
		return domain.NodeStaleRotationPreview{}, s.previewErr
	}
	now := time.Unix(1000, 0).UTC()
	startedAt := now.Add(-10 * time.Minute)
	return domain.NodeStaleRotationPreview{
		NodeID:                nodeID,
		StaleRotationDetected: true,
		TokenRotationStatus:   "rotating",
		EvaluatedAt:           now,
		Candidates: []domain.NodeStaleRotationCandidate{{
			JobID:       "job-1",
			Status:      "running",
			CreatedAt:   now.Add(-15 * time.Minute),
			StartedAt:   &startedAt,
			AgeSeconds:  900,
			StaleReason: domain.NodeStaleRotationReasonClaimedInactive,
			SafeToClear: true,
		}},
	}, nil
}

func (s *nodeStaleRotationHTTPTestStore) ClearNodeStalePendingRotation(_ context.Context, nodeID string, input domain.NodeStaleRotationClearInput) (domain.NodeStaleRotationClearResult, error) {
	s.clearCalled = true
	s.nodeID = nodeID
	s.input = input
	if s.clearErr != nil {
		return domain.NodeStaleRotationClearResult{}, s.clearErr
	}
	return domain.NodeStaleRotationClearResult{
		Status:       "cleared",
		NodeID:       nodeID,
		ClearedCount: 1,
		ClearedJobs: []domain.NodeStaleRotationClearedJob{{
			JobID:          "job-1",
			PreviousStatus: "running",
			Status:         "cancelled",
			StaleReason:    domain.NodeStaleRotationReasonClaimedInactive,
			FinishedAt:     time.Unix(1100, 0).UTC(),
		}},
		PendingRotationStateCleared:  true,
		ActiveAgentIdentityPreserved: true,
	}, nil
}

func TestPreviewNodeStaleRotationRouteRequiresNodeRead(t *testing.T) {
	t.Parallel()

	store := &nodeStaleRotationHTTPTestStore{permissions: []string{"node.bootstrap"}}
	rec := sendNodeStaleRotationTestRequest(store, nethttp.MethodGet, "/api/v1/nodes/node-1/diagnostics/stale-rotation", "Bearer test-session", false, "")
	if rec.Code != nethttp.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
	if store.previewCalled {
		t.Fatal("preview store must not be called without node.read permission")
	}
}

func TestPreviewNodeStaleRotationReturnsRedactedCandidates(t *testing.T) {
	t.Parallel()

	store := &nodeStaleRotationHTTPTestStore{permissions: []string{"node.read"}}
	rec := sendNodeStaleRotationTestRequest(store, nethttp.MethodGet, "/api/v1/nodes/node-1/diagnostics/stale-rotation", "Bearer test-session", false, "")
	if rec.Code != nethttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	assertNodeStaleRotationNoStore(t, rec)
	if !store.previewCalled || store.nodeID != "node-1" {
		t.Fatalf("preview call = %t node_id=%q", store.previewCalled, store.nodeID)
	}
	body := rec.Body.String()
	for _, forbidden := range []string{"\"payload\"", "\"result\"", "token_hash", "new_agent_token", "secret_ref", "Authorization"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("preview response leaked %q: %s", forbidden, body)
		}
	}
	var payload domain.NodeStaleRotationPreview
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	if payload.NodeID != "node-1" || !payload.StaleRotationDetected || len(payload.Candidates) != 1 || !payload.Candidates[0].SafeToClear {
		t.Fatalf("preview payload mismatch: %#v", payload)
	}
}

func TestClearNodeStaleRotationRouteRequiresNodeBootstrap(t *testing.T) {
	t.Parallel()

	store := &nodeStaleRotationHTTPTestStore{permissions: []string{"node.read"}}
	rec := sendNodeStaleRotationTestRequest(store, nethttp.MethodPost, "/api/v1/nodes/node-1/diagnostics/clear-stale-rotation", "Bearer test-session", false, validNodeStaleRotationPayload())
	if rec.Code != nethttp.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
	if store.clearCalled {
		t.Fatal("clear store must not be called without node.bootstrap permission")
	}
}

func TestClearNodeStaleRotationCookieAuthRequiresCSRF(t *testing.T) {
	t.Parallel()

	store := &nodeStaleRotationHTTPTestStore{permissions: []string{"node.bootstrap"}}
	rec := sendNodeStaleRotationTestRequest(store, nethttp.MethodPost, "/api/v1/nodes/node-1/diagnostics/clear-stale-rotation", "Cookie test-session", false, validNodeStaleRotationPayload())
	if rec.Code != nethttp.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "csrf protection required") {
		t.Fatalf("csrf error body = %s", rec.Body.String())
	}
	if store.clearCalled {
		t.Fatal("clear store must not be called when CSRF protection rejects request")
	}
}

func TestClearNodeStaleRotationRejectsInvalidContract(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body string
	}{
		{name: "empty body", body: ""},
		{name: "unknown field", body: `{"confirmation":"node-prod-1","reason":"maintenance window","acknowledge_cancel_rotation":true,"expected_job_ids":["job-1"],"node_id":"caller-supplied"}`},
		{name: "missing confirmation", body: `{"reason":"maintenance window","acknowledge_cancel_rotation":true,"expected_job_ids":["job-1"]}`},
		{name: "missing reason", body: `{"confirmation":"node-prod-1","acknowledge_cancel_rotation":true,"expected_job_ids":["job-1"]}`},
		{name: "short reason", body: `{"confirmation":"node-prod-1","reason":"bad","acknowledge_cancel_rotation":true,"expected_job_ids":["job-1"]}`},
		{name: "missing ack", body: `{"confirmation":"node-prod-1","reason":"maintenance window","expected_job_ids":["job-1"]}`},
		{name: "false ack", body: `{"confirmation":"node-prod-1","reason":"maintenance window","acknowledge_cancel_rotation":false,"expected_job_ids":["job-1"]}`},
		{name: "missing expected ids", body: `{"confirmation":"node-prod-1","reason":"maintenance window","acknowledge_cancel_rotation":true}`},
		{name: "duplicate expected ids", body: `{"confirmation":"node-prod-1","reason":"maintenance window","acknowledge_cancel_rotation":true,"expected_job_ids":["job-1"," job-1 "]}`},
		{name: "empty expected id", body: `{"confirmation":"node-prod-1","reason":"maintenance window","acknowledge_cancel_rotation":true,"expected_job_ids":[""]}`},
		{name: "control character", body: "{\"confirmation\":\"node-prod-1\",\"reason\":\"maintenance\\nwindow\",\"acknowledge_cancel_rotation\":true,\"expected_job_ids\":[\"job-1\"]}"},
		{name: "token marker", body: `{"confirmation":"node-prod-1","reason":"Authorization: Bearer redacted","acknowledge_cancel_rotation":true,"expected_job_ids":["job-1"]}`},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store := &nodeStaleRotationHTTPTestStore{permissions: []string{"node.bootstrap"}}
			rec := sendNodeStaleRotationTestRequest(store, nethttp.MethodPost, "/api/v1/nodes/node-1/diagnostics/clear-stale-rotation", "Bearer test-session", false, tc.body)
			if rec.Code != nethttp.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
			}
			assertNodeStaleRotationErrorCode(t, rec, nodeStaleRotationRequestInvalid)
			assertNodeStaleRotationNoStore(t, rec)
			if store.clearCalled {
				t.Fatal("clear store must not be called for invalid request contracts")
			}
		})
	}
}

func TestClearNodeStaleRotationMapsSafeStoreErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		err      error
		wantCode int
		wantErr  string
	}{
		{name: "not found", err: pgx.ErrNoRows, wantCode: nethttp.StatusNotFound, wantErr: nodeStaleRotationNodeNotFound},
		{name: "confirmation mismatch", err: domain.ErrNodeStaleRotationConfirmationMismatch, wantCode: nethttp.StatusConflict, wantErr: nodeStaleRotationConfirmationMismatch},
		{name: "no safe candidates", err: domain.ErrNodeStaleRotationNotFound, wantCode: nethttp.StatusConflict, wantErr: nodeStaleRotationNotFound},
		{name: "preview changed", err: domain.ErrNodeStaleRotationPreviewChanged, wantCode: nethttp.StatusConflict, wantErr: nodeStaleRotationPreviewChanged},
		{name: "evidence ambiguous", err: domain.ErrNodeStaleRotationEvidenceAmbiguous, wantCode: nethttp.StatusConflict, wantErr: nodeStaleRotationEvidenceAmbiguous},
		{name: "pending ambiguous", err: domain.ErrNodeStaleRotationPendingStateAmbiguous, wantCode: nethttp.StatusConflict, wantErr: nodeStaleRotationPendingStateAmbiguous},
		{name: "conflict", err: domain.ErrNodeStaleRotationConflict, wantCode: nethttp.StatusConflict, wantErr: nodeStaleRotationConflict},
		{name: "audit failure", err: domain.ErrNodeStaleRotationAuditFailed, wantCode: nethttp.StatusInternalServerError, wantErr: nodeStaleRotationInternalError},
		{name: "job log failure", err: domain.ErrNodeStaleRotationJobLogFailed, wantCode: nethttp.StatusInternalServerError, wantErr: nodeStaleRotationInternalError},
		{name: "raw internal", err: errors.New("pq: new_agent_token_hash leaked"), wantCode: nethttp.StatusInternalServerError, wantErr: nodeStaleRotationInternalError},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store := &nodeStaleRotationHTTPTestStore{permissions: []string{"node.bootstrap"}, clearErr: tc.err}
			rec := sendNodeStaleRotationTestRequest(store, nethttp.MethodPost, "/api/v1/nodes/node-1/diagnostics/clear-stale-rotation", "Bearer test-session", false, validNodeStaleRotationPayload())
			if rec.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tc.wantCode, rec.Body.String())
			}
			assertNodeStaleRotationErrorCode(t, rec, tc.wantErr)
			assertNodeStaleRotationNoStore(t, rec)
			if strings.Contains(rec.Body.String(), "new_agent_token") || strings.Contains(rec.Body.String(), "pq:") {
				t.Fatalf("response leaked raw store error: %s", rec.Body.String())
			}
		})
	}
}

func TestClearNodeStaleRotationReturnsRedactedResult(t *testing.T) {
	t.Parallel()

	store := &nodeStaleRotationHTTPTestStore{permissions: []string{"node.bootstrap"}}
	rec := sendNodeStaleRotationTestRequest(store, nethttp.MethodPost, "/api/v1/nodes/node-1/diagnostics/clear-stale-rotation", "Bearer test-session", false, `{"confirmation":"  node-prod-1  ","reason":"  maintenance window  ","acknowledge_cancel_rotation":true,"expected_job_ids":["job-2","job-1"]}`)
	if rec.Code != nethttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	assertNodeStaleRotationNoStore(t, rec)
	if !store.clearCalled || store.nodeID != "node-1" {
		t.Fatalf("clear call = %t node_id=%q", store.clearCalled, store.nodeID)
	}
	if store.input.Confirmation != "node-prod-1" || store.input.Reason != "maintenance window" || !store.input.AcknowledgeCancelRotation {
		t.Fatalf("normalized input = %#v", store.input)
	}
	if strings.Join(store.input.ExpectedJobIDs, ",") != "job-1,job-2" {
		t.Fatalf("expected_job_ids not sorted: %#v", store.input.ExpectedJobIDs)
	}
	if store.input.ActorUserID == nil || *store.input.ActorUserID != "user-1" {
		t.Fatalf("actor user id = %#v", store.input.ActorUserID)
	}
	body := rec.Body.String()
	for _, forbidden := range []string{"\"payload\"", "\"result\"", "token_hash", "new_agent_token", "secret_ref", "Authorization", "\"locked_"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("clear response leaked %q: %s", forbidden, body)
		}
	}
	var payload domain.NodeStaleRotationClearResult
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode clear result: %v", err)
	}
	if payload.Status != "cleared" || payload.NodeID != "node-1" || payload.ClearedCount != 1 || !payload.PendingRotationStateCleared || !payload.ActiveAgentIdentityPreserved {
		t.Fatalf("clear result mismatch: %#v", payload)
	}
}

func sendNodeStaleRotationTestRequest(store *nodeStaleRotationHTTPTestStore, method, path, auth string, csrf bool, body string) *httptest.ResponseRecorder {
	var reader io.Reader = strings.NewReader(body)
	if body == "" {
		reader = nethttp.NoBody
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if strings.HasPrefix(auth, "Bearer ") {
		req.Header.Set("Authorization", auth)
	}
	if strings.HasPrefix(auth, "Cookie ") {
		req.AddCookie(&nethttp.Cookie{Name: "megavpn_session", Value: strings.TrimPrefix(auth, "Cookie ")})
	}
	if csrf {
		req.Header.Set("X-MegaVPN-CSRF", "1")
	}
	rec := httptest.NewRecorder()
	newNodeStaleRotationTestHandler(store).ServeHTTP(rec, req)
	return rec
}

func newNodeStaleRotationTestHandler(store *nodeStaleRotationHTTPTestStore) nethttp.Handler {
	return New(slog.New(slog.NewTextHandler(io.Discard, nil)), store, Options{
		Version:           "test",
		SessionCookieName: "megavpn_session",
	})
}

func validNodeStaleRotationPayload() string {
	return `{"confirmation":"node-prod-1","reason":"maintenance window","acknowledge_cancel_rotation":true,"expected_job_ids":["job-1"]}`
}

func assertNodeStaleRotationErrorCode(t *testing.T, rec *httptest.ResponseRecorder, want string) {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if payload["code"] != want {
		t.Fatalf("error code = %#v, want %q; payload=%#v", payload["code"], want, payload)
	}
}

func assertNodeStaleRotationNoStore(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()

	if !strings.Contains(strings.ToLower(rec.Header().Get("Cache-Control")), "no-store") {
		t.Fatalf("Cache-Control = %q, want no-store", rec.Header().Get("Cache-Control"))
	}
	if strings.ToLower(rec.Header().Get("Pragma")) != "no-cache" {
		t.Fatalf("Pragma = %q, want no-cache", rec.Header().Get("Pragma"))
	}
}
