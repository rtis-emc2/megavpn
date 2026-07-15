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

type nodeRebootHTTPTestStore struct {
	Store

	permissions []string
	createErr   error
	called      bool
	nodeID      string
	input       domain.NodeRebootJobCreateInput
}

func (s *nodeRebootHTTPTestStore) ResolveAuthContext(context.Context, string) (domain.AuthContext, error) {
	return domain.AuthContext{
		User:            domain.PlatformUser{ID: "user-1", Username: "operator"},
		Session:         domain.UserSession{ID: "session-1", UserID: "user-1", ExpiresAt: time.Now().Add(time.Hour)},
		PermissionCodes: append([]string(nil), s.permissions...),
	}, nil
}

func (s *nodeRebootHTTPTestStore) CreateNodeRebootJob(_ context.Context, nodeID string, input domain.NodeRebootJobCreateInput) (domain.Job, error) {
	s.called = true
	s.nodeID = nodeID
	s.input = input
	if s.createErr != nil {
		return domain.Job{}, s.createErr
	}
	return domain.Job{
		ID:        "job-1",
		Type:      "node.reboot",
		ScopeType: "node",
		ScopeID:   &nodeID,
		NodeID:    &nodeID,
		Status:    "queued",
		Priority:  1,
		Payload: map[string]any{
			"node_id":          nodeID,
			"node_name":        "node-prod-1",
			"confirmation":     "node-prod-1",
			"reason":           input.Reason,
			"agent_token":      "must-redact",
			"agent_token_hash": "must-redact",
		},
		CreatedAt: time.Unix(100, 0).UTC(),
	}, nil
}

func TestCreateNodeRebootRouteRequiresNodeBootstrap(t *testing.T) {
	t.Parallel()

	store := &nodeRebootHTTPTestStore{permissions: []string{"node.read"}}
	rec := sendNodeRebootTestRequest(store, nethttp.MethodPost, "/api/v1/nodes/node-1/reboot", "Bearer test-session", false, validNodeRebootPayload())
	if rec.Code != nethttp.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
	if store.called {
		t.Fatal("store must not be called without node.bootstrap permission")
	}
}

func TestCreateNodeRebootCookieAuthRequiresCSRF(t *testing.T) {
	t.Parallel()

	store := &nodeRebootHTTPTestStore{permissions: []string{"node.bootstrap"}}
	rec := sendNodeRebootTestRequest(store, nethttp.MethodPost, "/api/v1/nodes/node-1/reboot", "Cookie test-session", false, validNodeRebootPayload())
	if rec.Code != nethttp.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "csrf protection required") {
		t.Fatalf("csrf error body = %s", rec.Body.String())
	}
	if store.called {
		t.Fatal("store must not be called when CSRF protection rejects request")
	}
}

func TestCreateNodeRebootRejectsInvalidContract(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body string
	}{
		{name: "unknown field", body: `{"confirmation":"node-prod-1","reason":"maintenance window","node_id":"caller-supplied"}`},
		{name: "missing confirmation", body: `{"reason":"maintenance window"}`},
		{name: "missing reason", body: `{"confirmation":"node-prod-1"}`},
		{name: "short reason", body: `{"confirmation":"node-prod-1","reason":"bad"}`},
		{name: "oversized confirmation", body: `{"confirmation":"` + strings.Repeat("n", domain.NodeRebootMaxConfirmationLength+1) + `","reason":"maintenance window"}`},
		{name: "oversized reason", body: `{"confirmation":"node-prod-1","reason":"` + strings.Repeat("r", domain.NodeRebootMaxReasonLength+1) + `"}`},
		{name: "control character", body: "{\"confirmation\":\"node-prod-1\",\"reason\":\"maintenance\\nwindow\"}"},
		{name: "request body marker", body: `{"confirmation":"node-prod-1","reason":"contains {full request body}"}`},
		{name: "token marker", body: `{"confirmation":"node-prod-1","reason":"Authorization: Bearer redacted"}`},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store := &nodeRebootHTTPTestStore{permissions: []string{"node.bootstrap"}}
			rec := sendNodeRebootTestRequest(store, nethttp.MethodPost, "/api/v1/nodes/node-1/reboot", "Bearer test-session", false, tc.body)
			if rec.Code != nethttp.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
			}
			assertNodeRebootErrorCode(t, rec, nodeRebootRequestInvalid)
			assertNodeRebootNoStore(t, rec)
			if store.called {
				t.Fatal("store must not be called for invalid request contracts")
			}
		})
	}
}

func TestCreateNodeRebootMapsSafeStoreErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		err      error
		wantCode int
		wantErr  string
	}{
		{name: "not found", err: pgx.ErrNoRows, wantCode: nethttp.StatusNotFound, wantErr: nodeRebootNodeNotFound},
		{name: "confirmation mismatch", err: domain.ErrNodeRebootConfirmationMismatch, wantCode: nethttp.StatusConflict, wantErr: nodeRebootConfirmationMismatch},
		{name: "missing agent", err: domain.ErrNodeRebootAgentMissing, wantCode: nethttp.StatusConflict, wantErr: nodeRebootAgentMissing},
		{name: "unavailable agent", err: domain.ErrNodeRebootAgentUnavailable, wantCode: nethttp.StatusConflict, wantErr: nodeRebootAgentUnavailable},
		{name: "conflict", err: domain.ErrNodeRebootConflict, wantCode: nethttp.StatusConflict, wantErr: nodeRebootConflict},
		{name: "audit failure", err: domain.ErrNodeRebootAuditFailed, wantCode: nethttp.StatusInternalServerError, wantErr: nodeRebootInternalError},
		{name: "raw internal", err: errors.New("pq: agent_token_hash leaked"), wantCode: nethttp.StatusInternalServerError, wantErr: nodeRebootInternalError},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store := &nodeRebootHTTPTestStore{permissions: []string{"node.bootstrap"}, createErr: tc.err}
			rec := sendNodeRebootTestRequest(store, nethttp.MethodPost, "/api/v1/nodes/node-1/reboot", "Bearer test-session", false, validNodeRebootPayload())
			if rec.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tc.wantCode, rec.Body.String())
			}
			assertNodeRebootErrorCode(t, rec, tc.wantErr)
			assertNodeRebootNoStore(t, rec)
			if strings.Contains(rec.Body.String(), "agent_token_hash") || strings.Contains(rec.Body.String(), "pq:") {
				t.Fatalf("response leaked raw store error: %s", rec.Body.String())
			}
		})
	}
}

func TestCreateNodeRebootReturnsRedactedQueuedJob(t *testing.T) {
	t.Parallel()

	store := &nodeRebootHTTPTestStore{permissions: []string{"node.bootstrap"}}
	rec := sendNodeRebootTestRequest(store, nethttp.MethodPost, "/api/v1/nodes/node-1/reboot", "Bearer test-session", false, `{"confirmation":"  node-prod-1  ","reason":"  maintenance window  "}`)
	if rec.Code != nethttp.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", rec.Code, rec.Body.String())
	}
	assertNodeRebootNoStore(t, rec)
	if !store.called || store.nodeID != "node-1" {
		t.Fatalf("store call = %t node_id=%q", store.called, store.nodeID)
	}
	if store.input.Confirmation != "node-prod-1" || store.input.Reason != "maintenance window" {
		t.Fatalf("normalized input = %#v", store.input)
	}
	if store.input.ActorUserID == nil || *store.input.ActorUserID != "user-1" {
		t.Fatalf("actor user id = %#v", store.input.ActorUserID)
	}
	body := rec.Body.String()
	for _, forbidden := range []string{"must-redact", "agent_token\":\"must"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("response leaked %q: %s", forbidden, body)
		}
	}
	var payload domain.Job
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.ID != "job-1" || payload.Type != "node.reboot" || payload.Status != "queued" || payload.NodeID == nil || *payload.NodeID != "node-1" {
		t.Fatalf("unexpected job response: %#v", payload)
	}
	if payload.Payload["agent_token"] != redactedValue || payload.Payload["agent_token_hash"] != redactedValue {
		t.Fatalf("sensitive payload fields not redacted: %#v", payload.Payload)
	}
}

func sendNodeRebootTestRequest(store *nodeRebootHTTPTestStore, method, path, auth string, csrf bool, body string) *httptest.ResponseRecorder {
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
	newNodeRebootTestHandler(store).ServeHTTP(rec, req)
	return rec
}

func newNodeRebootTestHandler(store *nodeRebootHTTPTestStore) nethttp.Handler {
	return New(slog.New(slog.NewTextHandler(io.Discard, nil)), store, Options{
		Version:           "test",
		SessionCookieName: "megavpn_session",
	})
}

func validNodeRebootPayload() string {
	return `{"confirmation":"node-prod-1","reason":"maintenance window"}`
}

func assertNodeRebootErrorCode(t *testing.T, rec *httptest.ResponseRecorder, want string) {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if payload["code"] != want {
		t.Fatalf("error code = %#v, want %q; payload=%#v", payload["code"], want, payload)
	}
}

func assertNodeRebootNoStore(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()

	if !strings.Contains(strings.ToLower(rec.Header().Get("Cache-Control")), "no-store") {
		t.Fatalf("Cache-Control = %q, want no-store", rec.Header().Get("Cache-Control"))
	}
	if strings.ToLower(rec.Header().Get("Pragma")) != "no-cache" {
		t.Fatalf("Pragma = %q, want no-cache", rec.Header().Get("Pragma"))
	}
}
