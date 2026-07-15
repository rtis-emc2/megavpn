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

type nodeAgentRevokeHTTPTestStore struct {
	Store

	permissions []string
	revokeErr   error
	called      bool
	nodeID      string
	input       domain.NodeAgentIdentityRevokeInput
}

func (s *nodeAgentRevokeHTTPTestStore) ResolveAuthContext(context.Context, string) (domain.AuthContext, error) {
	return domain.AuthContext{
		User:            domain.PlatformUser{ID: "user-1", Username: "operator"},
		Session:         domain.UserSession{ID: "session-1", UserID: "user-1", ExpiresAt: time.Now().Add(time.Hour)},
		PermissionCodes: append([]string(nil), s.permissions...),
	}, nil
}

func (s *nodeAgentRevokeHTTPTestStore) RevokeNodeAgentIdentity(_ context.Context, nodeID string, input domain.NodeAgentIdentityRevokeInput) (domain.NodeAgentIdentityRevokeResult, error) {
	s.called = true
	s.nodeID = nodeID
	s.input = input
	if s.revokeErr != nil {
		return domain.NodeAgentIdentityRevokeResult{}, s.revokeErr
	}
	return domain.NodeAgentIdentityRevokeResult{
		Status:                  "revoked",
		NodeID:                  nodeID,
		AgentStatus:             "revoked",
		RevokedAt:               time.Unix(100, 0).UTC(),
		AlreadyRevoked:          false,
		RevokedEnrollmentTokens: 1,
	}, nil
}

func TestRevokeNodeAgentIdentityRouteRequiresNodeBootstrap(t *testing.T) {
	t.Parallel()

	store := &nodeAgentRevokeHTTPTestStore{permissions: []string{"node.read"}}
	rec := sendNodeAgentRevokeTestRequest(store, nethttp.MethodPost, "/api/v1/nodes/node-1/agent-identity/revoke", "Bearer test-session", false, validNodeAgentRevokePayload())
	if rec.Code != nethttp.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
	if store.called {
		t.Fatal("store must not be called without node.bootstrap permission")
	}
}

func TestRevokeNodeAgentIdentityCookieAuthRequiresCSRF(t *testing.T) {
	t.Parallel()

	store := &nodeAgentRevokeHTTPTestStore{permissions: []string{"node.bootstrap"}}
	rec := sendNodeAgentRevokeTestRequest(store, nethttp.MethodPost, "/api/v1/nodes/node-1/agent-identity/revoke", "Cookie test-session", false, validNodeAgentRevokePayload())
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

func TestRevokeNodeAgentIdentityRejectsInvalidContract(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body string
	}{
		{name: "unknown field", body: `{"confirmation":"node-prod-1","reason":"incident response","node_id":"caller-supplied"}`},
		{name: "missing confirmation", body: `{"reason":"incident response"}`},
		{name: "missing reason", body: `{"confirmation":"node-prod-1"}`},
		{name: "short reason", body: `{"confirmation":"node-prod-1","reason":"bad"}`},
		{name: "control character", body: "{\"confirmation\":\"node-prod-1\",\"reason\":\"incident\\nresponse\"}"},
		{name: "request body marker", body: `{"confirmation":"node-prod-1","reason":"contains {full request body}"}`},
		{name: "token marker", body: `{"confirmation":"node-prod-1","reason":"Authorization: Bearer redacted"}`},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store := &nodeAgentRevokeHTTPTestStore{permissions: []string{"node.bootstrap"}}
			rec := sendNodeAgentRevokeTestRequest(store, nethttp.MethodPost, "/api/v1/nodes/node-1/agent-identity/revoke", "Bearer test-session", false, tc.body)
			if rec.Code != nethttp.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
			}
			assertNodeAgentRevokeErrorCode(t, rec, nodeAgentRevokeRequestInvalid)
			assertNodeAgentRevokeNoStore(t, rec)
			if store.called {
				t.Fatal("store must not be called for invalid request contracts")
			}
		})
	}
}

func TestRevokeNodeAgentIdentityMapsSafeStoreErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		err      error
		wantCode int
		wantErr  string
	}{
		{name: "not found", err: pgx.ErrNoRows, wantCode: nethttp.StatusNotFound, wantErr: nodeAgentRevokeNodeNotFound},
		{name: "missing identity", err: domain.ErrNodeAgentIdentityMissing, wantCode: nethttp.StatusConflict, wantErr: nodeAgentIdentityMissing},
		{name: "confirmation mismatch", err: domain.ErrNodeAgentRevokeConfirmationMismatch, wantCode: nethttp.StatusConflict, wantErr: nodeAgentRevokeConfirmationMismatch},
		{name: "conflict", err: domain.ErrNodeAgentRevokeConflict, wantCode: nethttp.StatusConflict, wantErr: nodeAgentRevokeConflict},
		{name: "raw internal", err: errors.New("pq: token_hash leaked"), wantCode: nethttp.StatusInternalServerError, wantErr: nodeAgentRevokeInternalError},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store := &nodeAgentRevokeHTTPTestStore{permissions: []string{"node.bootstrap"}, revokeErr: tc.err}
			rec := sendNodeAgentRevokeTestRequest(store, nethttp.MethodPost, "/api/v1/nodes/node-1/agent-identity/revoke", "Bearer test-session", false, validNodeAgentRevokePayload())
			if rec.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tc.wantCode, rec.Body.String())
			}
			assertNodeAgentRevokeErrorCode(t, rec, tc.wantErr)
			assertNodeAgentRevokeNoStore(t, rec)
			if strings.Contains(rec.Body.String(), "token_hash") || strings.Contains(rec.Body.String(), "pq:") {
				t.Fatalf("response leaked raw store error: %s", rec.Body.String())
			}
		})
	}
}

func TestRevokeNodeAgentIdentityReturnsRedactedResponse(t *testing.T) {
	t.Parallel()

	store := &nodeAgentRevokeHTTPTestStore{permissions: []string{"node.bootstrap"}}
	rec := sendNodeAgentRevokeTestRequest(store, nethttp.MethodPost, "/api/v1/nodes/node-1/agent-identity/revoke", "Bearer test-session", false, `{"confirmation":"  node-prod-1  ","reason":"  incident response rotation  "}`)
	if rec.Code != nethttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	assertNodeAgentRevokeNoStore(t, rec)
	if !store.called || store.nodeID != "node-1" {
		t.Fatalf("store call = %t node_id=%q", store.called, store.nodeID)
	}
	if store.input.Confirmation != "node-prod-1" || store.input.Reason != "incident response rotation" {
		t.Fatalf("normalized input = %#v", store.input)
	}
	if store.input.ActorUserID == nil || *store.input.ActorUserID != "user-1" {
		t.Fatalf("actor user id = %#v", store.input.ActorUserID)
	}
	body := rec.Body.String()
	for _, forbidden := range []string{"agent_token", "token_hash", "token_hint", "fingerprint", "enrollment_token_id", "enrollment_token_hash", "secret_ref"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("response leaked %q: %s", forbidden, body)
		}
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["status"] != "revoked" || payload["node_id"] != "node-1" || payload["agent_status"] != "revoked" || payload["already_revoked"] != false {
		t.Fatalf("unexpected redacted response: %#v", payload)
	}
	if payload["revoked_enrollment_tokens"] != float64(1) {
		t.Fatalf("revoked_enrollment_tokens = %#v, want 1", payload["revoked_enrollment_tokens"])
	}
	if _, ok := payload["revoked_at"].(string); !ok {
		t.Fatalf("revoked_at missing or non-string: %#v", payload)
	}
}

func sendNodeAgentRevokeTestRequest(store *nodeAgentRevokeHTTPTestStore, method, path, auth string, csrf bool, body string) *httptest.ResponseRecorder {
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
	newNodeAgentRevokeTestHandler(store).ServeHTTP(rec, req)
	return rec
}

func newNodeAgentRevokeTestHandler(store *nodeAgentRevokeHTTPTestStore) nethttp.Handler {
	return New(slog.New(slog.NewTextHandler(io.Discard, nil)), store, Options{
		Version:           "test",
		SessionCookieName: "megavpn_session",
	})
}

func validNodeAgentRevokePayload() string {
	return `{"confirmation":"node-prod-1","reason":"incident response rotation"}`
}

func assertNodeAgentRevokeErrorCode(t *testing.T, rec *httptest.ResponseRecorder, want string) {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if payload["code"] != want {
		t.Fatalf("error code = %#v, want %q; payload=%#v", payload["code"], want, payload)
	}
}

func assertNodeAgentRevokeNoStore(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()

	if !strings.Contains(strings.ToLower(rec.Header().Get("Cache-Control")), "no-store") {
		t.Fatalf("Cache-Control = %q, want no-store", rec.Header().Get("Cache-Control"))
	}
	if strings.ToLower(rec.Header().Get("Pragma")) != "no-cache" {
		t.Fatalf("Pragma = %q, want no-cache", rec.Header().Get("Pragma"))
	}
}
