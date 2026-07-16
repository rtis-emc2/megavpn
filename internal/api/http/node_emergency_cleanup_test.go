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

type nodeEmergencyCleanupHTTPTestStore struct {
	Store

	permissions []string
	createErr   error
	called      bool
	nodeID      string
	input       domain.NodeEmergencyCleanupJobCreateInput
}

func (s *nodeEmergencyCleanupHTTPTestStore) ResolveAuthContext(context.Context, string) (domain.AuthContext, error) {
	return domain.AuthContext{
		User:            domain.PlatformUser{ID: "user-1", Username: "operator"},
		Session:         domain.UserSession{ID: "session-1", UserID: "user-1", ExpiresAt: time.Now().Add(time.Hour)},
		PermissionCodes: append([]string(nil), s.permissions...),
	}, nil
}

func (s *nodeEmergencyCleanupHTTPTestStore) CreateNodeEmergencyCleanupJob(_ context.Context, nodeID string, input domain.NodeEmergencyCleanupJobCreateInput) (domain.NodeEmergencyCleanupJobCreateResult, error) {
	s.called = true
	s.nodeID = nodeID
	s.input = input
	if s.createErr != nil {
		return domain.NodeEmergencyCleanupJobCreateResult{}, s.createErr
	}
	return domain.NodeEmergencyCleanupJobCreateResult{
		Job: domain.Job{
			ID:        "job-1",
			Type:      "node.emergency_cleanup",
			ScopeType: "node",
			ScopeID:   &nodeID,
			NodeID:    &nodeID,
			Status:    "queued",
			Priority:  1,
			Payload: map[string]any{
				"node_id":          nodeID,
				"node_name":        "node-prod-1",
				"cleanup_scope":    input.CleanupScope,
				"include_agent":    input.IncludeAgent,
				"reason":           input.Reason,
				"agent_token":      "must-redact",
				"agent_token_hash": "must-redact",
				"instances": []any{map[string]any{
					"instance_id":  "inst-1",
					"action":       "delete",
					"service_code": "xray-core",
				}},
			},
			CreatedAt: time.Unix(100, 0).UTC(),
		},
		PlanSummary: domain.NodeEmergencyCleanupPlanSummary{
			CleanupScope:          input.CleanupScope,
			IncludeAgent:          input.IncludeAgent,
			InstanceTargetCount:   1,
			ServiceCounts:         map[string]int{"xray-core": 1},
			NodeRuntimeCleanup:    input.CleanupScope == domain.NodeEmergencyCleanupScopeFullNode,
			AgentRemovalRequested: input.IncludeAgent,
		},
	}, nil
}

func TestCreateNodeEmergencyCleanupRouteRequiresNodeBootstrap(t *testing.T) {
	t.Parallel()

	store := &nodeEmergencyCleanupHTTPTestStore{permissions: []string{"node.read"}}
	rec := sendNodeEmergencyCleanupTestRequest(store, nethttp.MethodPost, "/api/v1/nodes/node-1/emergency-cleanup", "Bearer test-session", false, validNodeEmergencyCleanupPayload())
	if rec.Code != nethttp.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
	if store.called {
		t.Fatal("store must not be called without node.bootstrap permission")
	}
}

func TestCreateNodeEmergencyCleanupCookieAuthRequiresCSRF(t *testing.T) {
	t.Parallel()

	store := &nodeEmergencyCleanupHTTPTestStore{permissions: []string{"node.bootstrap"}}
	rec := sendNodeEmergencyCleanupTestRequest(store, nethttp.MethodPost, "/api/v1/nodes/node-1/emergency-cleanup", "Cookie test-session", false, validNodeEmergencyCleanupPayload())
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

func TestCreateNodeEmergencyCleanupRejectsInvalidContract(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		body    string
		wantErr string
	}{
		{name: "unknown field", body: `{"cleanup_scope":"services_only","include_agent":false,"confirmation":"node-prod-1","reason":"maintenance window","acknowledge_destructive_cleanup":true,"node_id":"caller-supplied"}`, wantErr: nodeEmergencyCleanupRequestInvalid},
		{name: "missing cleanup scope", body: `{"confirmation":"node-prod-1","reason":"maintenance window","acknowledge_destructive_cleanup":true}`, wantErr: nodeEmergencyCleanupScopeInvalid},
		{name: "unsupported cleanup scope", body: `{"cleanup_scope":"unsupported","confirmation":"node-prod-1","reason":"maintenance window","acknowledge_destructive_cleanup":true}`, wantErr: nodeEmergencyCleanupScopeInvalid},
		{name: "legacy alias", body: `{"cleanup_scope":"wipe","confirmation":"node-prod-1","reason":"maintenance window","acknowledge_destructive_cleanup":true}`, wantErr: nodeEmergencyCleanupScopeInvalid},
		{name: "agent with services only", body: `{"cleanup_scope":"services_only","include_agent":true,"confirmation":"node-prod-1","reason":"maintenance window","acknowledge_destructive_cleanup":true,"acknowledge_agent_removal":true}`, wantErr: nodeEmergencyCleanupScopeInvalid},
		{name: "missing confirmation", body: `{"cleanup_scope":"services_only","reason":"maintenance window","acknowledge_destructive_cleanup":true}`, wantErr: nodeEmergencyCleanupRequestInvalid},
		{name: "missing reason", body: `{"cleanup_scope":"services_only","confirmation":"node-prod-1","acknowledge_destructive_cleanup":true}`, wantErr: nodeEmergencyCleanupRequestInvalid},
		{name: "short reason", body: `{"cleanup_scope":"services_only","confirmation":"node-prod-1","reason":"bad","acknowledge_destructive_cleanup":true}`, wantErr: nodeEmergencyCleanupRequestInvalid},
		{name: "oversized reason", body: `{"cleanup_scope":"services_only","confirmation":"node-prod-1","reason":"` + strings.Repeat("r", domain.NodeEmergencyCleanupMaxReasonLength+1) + `","acknowledge_destructive_cleanup":true}`, wantErr: nodeEmergencyCleanupRequestInvalid},
		{name: "control character", body: "{\"cleanup_scope\":\"services_only\",\"confirmation\":\"node-prod-1\",\"reason\":\"maintenance\\nwindow\",\"acknowledge_destructive_cleanup\":true}", wantErr: nodeEmergencyCleanupRequestInvalid},
		{name: "token marker", body: `{"cleanup_scope":"services_only","confirmation":"node-prod-1","reason":"Authorization: Bearer redacted","acknowledge_destructive_cleanup":true}`, wantErr: nodeEmergencyCleanupRequestInvalid},
		{name: "missing destructive acknowledgement", body: `{"cleanup_scope":"services_only","confirmation":"node-prod-1","reason":"maintenance window"}`, wantErr: nodeEmergencyCleanupAckRequired},
		{name: "missing agent acknowledgement", body: `{"cleanup_scope":"full_node","include_agent":true,"confirmation":"node-prod-1","reason":"maintenance window","acknowledge_destructive_cleanup":true}`, wantErr: nodeEmergencyCleanupAckRequired},
		{name: "unexpected agent acknowledgement", body: `{"cleanup_scope":"services_only","include_agent":false,"confirmation":"node-prod-1","reason":"maintenance window","acknowledge_destructive_cleanup":true,"acknowledge_agent_removal":true}`, wantErr: nodeEmergencyCleanupAckRequired},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store := &nodeEmergencyCleanupHTTPTestStore{permissions: []string{"node.bootstrap"}}
			rec := sendNodeEmergencyCleanupTestRequest(store, nethttp.MethodPost, "/api/v1/nodes/node-1/emergency-cleanup", "Bearer test-session", false, tc.body)
			if rec.Code != nethttp.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
			}
			assertNodeEmergencyCleanupErrorCode(t, rec, tc.wantErr)
			assertNodeEmergencyCleanupNoStore(t, rec)
			if store.called {
				t.Fatal("store must not be called for invalid request contracts")
			}
		})
	}
}

func TestCreateNodeEmergencyCleanupMapsSafeStoreErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		err      error
		wantCode int
		wantErr  string
	}{
		{name: "not found", err: pgx.ErrNoRows, wantCode: nethttp.StatusNotFound, wantErr: nodeEmergencyCleanupNodeNotFound},
		{name: "confirmation mismatch", err: domain.ErrNodeEmergencyCleanupConfirmationMismatch, wantCode: nethttp.StatusConflict, wantErr: nodeEmergencyCleanupConfirmationMismatch},
		{name: "maintenance required", err: domain.ErrNodeEmergencyCleanupMaintenanceRequired, wantCode: nethttp.StatusConflict, wantErr: nodeEmergencyCleanupMaintenanceRequired},
		{name: "missing agent", err: domain.ErrNodeEmergencyCleanupAgentMissing, wantCode: nethttp.StatusConflict, wantErr: nodeEmergencyCleanupAgentMissing},
		{name: "unavailable agent", err: domain.ErrNodeEmergencyCleanupAgentUnavailable, wantCode: nethttp.StatusConflict, wantErr: nodeEmergencyCleanupAgentUnavailable},
		{name: "plan invalid", err: domain.ErrNodeEmergencyCleanupPlanInvalid, wantCode: nethttp.StatusUnprocessableEntity, wantErr: nodeEmergencyCleanupPlanInvalid},
		{name: "conflict", err: domain.ErrNodeEmergencyCleanupConflict, wantCode: nethttp.StatusConflict, wantErr: nodeEmergencyCleanupConflict},
		{name: "audit failure", err: domain.ErrNodeEmergencyCleanupAuditFailed, wantCode: nethttp.StatusInternalServerError, wantErr: nodeEmergencyCleanupInternalError},
		{name: "raw internal", err: errors.New("pq: agent_token_hash leaked"), wantCode: nethttp.StatusInternalServerError, wantErr: nodeEmergencyCleanupInternalError},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store := &nodeEmergencyCleanupHTTPTestStore{permissions: []string{"node.bootstrap"}, createErr: tc.err}
			rec := sendNodeEmergencyCleanupTestRequest(store, nethttp.MethodPost, "/api/v1/nodes/node-1/emergency-cleanup", "Bearer test-session", false, validNodeEmergencyCleanupPayload())
			if rec.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tc.wantCode, rec.Body.String())
			}
			assertNodeEmergencyCleanupErrorCode(t, rec, tc.wantErr)
			assertNodeEmergencyCleanupNoStore(t, rec)
			if strings.Contains(rec.Body.String(), "agent_token_hash") || strings.Contains(rec.Body.String(), "pq:") {
				t.Fatalf("response leaked raw store error: %s", rec.Body.String())
			}
		})
	}
}

func TestCreateNodeEmergencyCleanupReturnsQueuedRedactedResponse(t *testing.T) {
	t.Parallel()

	store := &nodeEmergencyCleanupHTTPTestStore{permissions: []string{"node.bootstrap"}}
	rec := sendNodeEmergencyCleanupTestRequest(store, nethttp.MethodPost, "/api/v1/nodes/node-1/emergency-cleanup", "Bearer test-session", false, `{"cleanup_scope":"services_only","include_agent":false,"confirmation":"  node-prod-1  ","reason":"  maintenance window  ","acknowledge_destructive_cleanup":true}`)
	if rec.Code != nethttp.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", rec.Code, rec.Body.String())
	}
	assertNodeEmergencyCleanupNoStore(t, rec)
	if !store.called || store.nodeID != "node-1" {
		t.Fatalf("store call = %t node_id=%q", store.called, store.nodeID)
	}
	if store.input.Confirmation != "node-prod-1" || store.input.Reason != "maintenance window" || store.input.CleanupScope != "services_only" || store.input.IncludeAgent {
		t.Fatalf("normalized input = %#v", store.input)
	}
	if !store.input.AcknowledgeDestructiveCleanup || store.input.AcknowledgeAgentRemoval {
		t.Fatalf("acknowledgements = %#v", store.input)
	}
	if store.input.ActorUserID == nil || *store.input.ActorUserID != "user-1" {
		t.Fatalf("actor user id = %#v", store.input.ActorUserID)
	}
	body := rec.Body.String()
	for _, forbidden := range []string{"must-redact", "completed", "cleaned", "agent removed", "private_key", "secret_ref", "config_content", `"payload"`, `"result"`, `"instances"`, `"systemd_unit"`, `"locked_by"`} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("response leaked or overclaimed %q: %s", forbidden, body)
		}
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["status"] != "queued" || payload["message"] != "emergency cleanup job queued" {
		t.Fatalf("queued response mismatch: %#v", payload)
	}
	summary, ok := payload["plan_summary"].(map[string]any)
	if !ok {
		t.Fatalf("plan_summary missing: %#v", payload)
	}
	if summary["cleanup_scope"] != "services_only" || summary["include_agent"] != false || summary["agent_removal_requested"] != false {
		t.Fatalf("plan_summary mismatch: %#v", summary)
	}
}

func sendNodeEmergencyCleanupTestRequest(store *nodeEmergencyCleanupHTTPTestStore, method, path, auth string, csrf bool, body string) *httptest.ResponseRecorder {
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
	newNodeEmergencyCleanupTestHandler(store).ServeHTTP(rec, req)
	return rec
}

func newNodeEmergencyCleanupTestHandler(store *nodeEmergencyCleanupHTTPTestStore) nethttp.Handler {
	return New(slog.New(slog.NewTextHandler(io.Discard, nil)), store, Options{
		Version:           "test",
		SessionCookieName: "megavpn_session",
	})
}

func validNodeEmergencyCleanupPayload() string {
	return `{"cleanup_scope":"services_only","include_agent":false,"confirmation":"node-prod-1","reason":"maintenance window","acknowledge_destructive_cleanup":true}`
}

func assertNodeEmergencyCleanupErrorCode(t *testing.T, rec *httptest.ResponseRecorder, want string) {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if payload["code"] != want {
		t.Fatalf("error code = %#v, want %q; payload=%#v", payload["code"], want, payload)
	}
}

func assertNodeEmergencyCleanupNoStore(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()

	if !strings.Contains(strings.ToLower(rec.Header().Get("Cache-Control")), "no-store") {
		t.Fatalf("Cache-Control = %q, want no-store", rec.Header().Get("Cache-Control"))
	}
	if strings.ToLower(rec.Header().Get("Pragma")) != "no-cache" {
		t.Fatalf("Pragma = %q, want no-cache", rec.Header().Get("Pragma"))
	}
}
