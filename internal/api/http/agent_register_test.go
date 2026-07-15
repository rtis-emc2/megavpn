package http

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

type agentRegisterHTTPTestStore struct {
	Store
	enrollmentNode       domain.Node
	enrollmentAgentToken string
	enrollmentErr        error
	enrollmentCalls      int
	enrollmentNodeID     string
	enrollmentToken      string
	enrollmentName       string
	enrollmentAddress    string
	legacyNode           domain.Node
	legacyErr            error
	legacyCalls          int
	legacyToken          string
}

func (s *agentRegisterHTTPTestStore) RegisterAgentWithEnrollmentVersion(_ context.Context, nodeID, token, name, address, _ string, _ string) (domain.Node, string, error) {
	s.enrollmentCalls++
	s.enrollmentNodeID = nodeID
	s.enrollmentToken = token
	s.enrollmentName = name
	s.enrollmentAddress = address
	if s.enrollmentErr != nil {
		return domain.Node{}, "", s.enrollmentErr
	}
	return s.enrollmentNode, s.enrollmentAgentToken, nil
}

func (s *agentRegisterHTTPTestStore) UpsertAgentNodeWithVersion(_ context.Context, name, address, token, _ string, _ string) (domain.Node, error) {
	s.legacyCalls++
	s.legacyToken = token
	if s.legacyErr != nil {
		return domain.Node{}, s.legacyErr
	}
	n := s.legacyNode
	if n.ID == "" {
		n = domain.Node{ID: "11111111-1111-4111-8111-111111111111", Name: name, Address: address}
	}
	return n, nil
}

func TestAgentRegisterEnrollmentResponseCompatibilityAndNoStore(t *testing.T) {
	t.Parallel()

	store := &agentRegisterHTTPTestStore{
		enrollmentNode: domain.Node{
			ID:      "11111111-1111-4111-8111-111111111111",
			Name:    "operator-node",
			Address: "10.0.0.10",
		},
		enrollmentAgentToken: "agent-secret-token",
	}
	handler := New(slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil)), store, Options{})
	body := `{"node_id":" 11111111-1111-4111-8111-111111111111 ","name":"agent-supplied","address":"192.0.2.99","enrollment_token":"enroll-secret","agent_version":"test-agent","protocol_version":"v1"}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(nethttp.MethodPost, "/agent/register", strings.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != nethttp.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if store.enrollmentCalls != 1 {
		t.Fatalf("enrollment calls = %d, want 1", store.enrollmentCalls)
	}
	if store.enrollmentNodeID != "11111111-1111-4111-8111-111111111111" {
		t.Fatalf("node_id = %q, want trimmed uuid", store.enrollmentNodeID)
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["status"] != "registered" || payload["agent_token"] != "agent-secret-token" || payload["token_hint"] == "" {
		t.Fatalf("response compatibility fields missing: %#v", payload)
	}
	node, ok := payload["node"].(map[string]any)
	if !ok {
		t.Fatalf("node response type = %T", payload["node"])
	}
	if node["id"] != "11111111-1111-4111-8111-111111111111" || node["name"] != "operator-node" || node["address"] != "10.0.0.10" {
		t.Fatalf("node response = %#v, want persisted operator projection", node)
	}
	if _, ok := payload["agent_token_hash"]; ok {
		t.Fatalf("agent_token_hash must not be returned: %#v", payload)
	}
	assertAgentRegisterNoStoreHeaders(t, rec)
}

func TestAgentRegisterRejectsPartialMixedAndUnknownCredentials(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body string
	}{
		{name: "node without enrollment", body: `{"node_id":"11111111-1111-4111-8111-111111111111"}`},
		{name: "enrollment without node", body: `{"enrollment_token":"enroll-secret"}`},
		{name: "enrollment plus legacy payload token", body: `{"node_id":"11111111-1111-4111-8111-111111111111","enrollment_token":"enroll-secret","token":"legacy-secret"}`},
		{name: "unknown field", body: `{"node_id":"11111111-1111-4111-8111-111111111111","enrollment_token":"enroll-secret","unexpected":true}`},
		{name: "control character metadata", body: "{\"node_id\":\"11111111-1111-4111-8111-111111111111\",\"enrollment_token\":\"enroll-secret\",\"agent_version\":\"bad\\nversion\"}"},
		{name: "oversized metadata", body: `{"node_id":"11111111-1111-4111-8111-111111111111","enrollment_token":"enroll-secret","protocol_version":"` + strings.Repeat("v", maxAgentRegisterProtocolLength+1) + `"}`},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store := &agentRegisterHTTPTestStore{}
			handler := New(slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil)), store, Options{})
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(nethttp.MethodPost, "/agent/register", strings.NewReader(tc.body))
			handler.ServeHTTP(rec, req)
			if rec.Code != nethttp.StatusBadRequest {
				t.Fatalf("status = %d body=%s, want 400", rec.Code, rec.Body.String())
			}
			assertAgentRegisterErrorCode(t, rec, agentRegistrationRequestInvalid)
			if store.enrollmentCalls != 0 || store.legacyCalls != 0 {
				t.Fatalf("store was called for invalid credentials: enrollment=%d legacy=%d", store.enrollmentCalls, store.legacyCalls)
			}
			assertAgentRegisterNoStoreHeaders(t, rec)
		})
	}
}

func TestAgentRegisterRejectsEnrollmentPlusLegacyBearer(t *testing.T) {
	t.Parallel()

	store := &agentRegisterHTTPTestStore{}
	handler := New(slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil)), store, Options{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(nethttp.MethodPost, "/agent/register", strings.NewReader(`{"node_id":"11111111-1111-4111-8111-111111111111","enrollment_token":"enroll-secret"}`))
	req.Header.Set("Authorization", "Bearer legacy-secret")
	handler.ServeHTTP(rec, req)
	if rec.Code != nethttp.StatusBadRequest {
		t.Fatalf("status = %d body=%s, want 400", rec.Code, rec.Body.String())
	}
	assertAgentRegisterErrorCode(t, rec, agentRegistrationRequestInvalid)
	if store.enrollmentCalls != 0 {
		t.Fatalf("enrollment store called for mixed bearer credential")
	}
}

func TestAgentRegisterLegacyModeRemainsExplicitlyGated(t *testing.T) {
	t.Parallel()

	store := &agentRegisterHTTPTestStore{}
	handler := New(slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil)), store, Options{AgentToken: "legacy-secret"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(nethttp.MethodPost, "/agent/register", strings.NewReader(`{"name":"legacy-node","address":"127.0.0.1","token":"legacy-secret"}`))
	handler.ServeHTTP(rec, req)
	if rec.Code != nethttp.StatusUnauthorized {
		t.Fatalf("status = %d body=%s, want 401 when allowAutoRegister is disabled", rec.Code, rec.Body.String())
	}
	assertAgentRegisterErrorCode(t, rec, agentEnrollmentReissueRequired)
	if store.legacyCalls != 0 {
		t.Fatalf("legacy store called while auto-register disabled")
	}

	enabled := New(slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil)), store, Options{AgentToken: "legacy-secret", AllowAutoRegister: true})
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(nethttp.MethodPost, "/agent/register", strings.NewReader(`{"name":"legacy-node","address":"127.0.0.1","token":"legacy-secret"}`))
	enabled.ServeHTTP(rec, req)
	if rec.Code != nethttp.StatusOK {
		t.Fatalf("status = %d body=%s, want 200 when explicit legacy gate is enabled", rec.Code, rec.Body.String())
	}
	if store.legacyCalls != 1 || store.legacyToken != "legacy-secret" {
		t.Fatalf("legacy call/token = %d/%q, want one safe legacy call", store.legacyCalls, store.legacyToken)
	}
}

func TestAgentRegisterSafeErrorMappingAndNoTokenLeak(t *testing.T) {
	t.Parallel()

	store := &agentRegisterHTTPTestStore{enrollmentErr: domain.ErrAgentRegistrationInvalidCredential}
	logs := bytes.NewBuffer(nil)
	handler := New(slog.New(slog.NewTextHandler(logs, nil)), store, Options{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(nethttp.MethodPost, "/agent/register", strings.NewReader(`{"node_id":"11111111-1111-4111-8111-111111111111","enrollment_token":"super-secret-enrollment-token"}`))
	handler.ServeHTTP(rec, req)
	if rec.Code != nethttp.StatusUnauthorized {
		t.Fatalf("status = %d body=%s, want 401", rec.Code, rec.Body.String())
	}
	assertAgentRegisterErrorCode(t, rec, agentEnrollmentReissueRequired)
	for _, state := range []string{"invalid", "expired", "revoked", "used", "consumed"} {
		if strings.Contains(strings.ToLower(rec.Body.String()), state) {
			t.Fatalf("credential state %q leaked in generic reissue-required response", state)
		}
	}
	if strings.Contains(rec.Body.String(), "super-secret-enrollment-token") {
		t.Fatal("error response leaked enrollment token")
	}
	if strings.Contains(logs.String(), "super-secret-enrollment-token") {
		t.Fatal("request logs leaked enrollment token")
	}
	assertAgentRegisterNoStoreHeaders(t, rec)
}

func TestAgentRegisterCredentialStatesShareGenericReissueCode(t *testing.T) {
	t.Parallel()

	for _, state := range []string{"invalid", "expired", "revoked", "used"} {
		state := state
		t.Run(state, func(t *testing.T) {
			t.Parallel()
			store := &agentRegisterHTTPTestStore{enrollmentErr: domain.ErrAgentRegistrationInvalidCredential}
			handler := New(slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil)), store, Options{})
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(nethttp.MethodPost, "/agent/register", strings.NewReader(`{"node_id":"11111111-1111-4111-8111-111111111111","enrollment_token":"test-enrollment-token"}`))
			handler.ServeHTTP(rec, req)
			if rec.Code != nethttp.StatusUnauthorized {
				t.Fatalf("status = %d body=%s, want 401", rec.Code, rec.Body.String())
			}
			assertAgentRegisterErrorCode(t, rec, agentEnrollmentReissueRequired)
			if strings.Contains(strings.ToLower(rec.Body.String()), state) {
				t.Fatalf("credential state %q leaked in response", state)
			}
		})
	}
}

func TestAgentRegisterRateLimitUsesSensitiveErrorHeaders(t *testing.T) {
	t.Parallel()

	s := &Server{rateLimiter: newRateLimiter()}
	handler := s.withRateLimit("agent_register", 1, time.Minute, func(w nethttp.ResponseWriter, r *nethttp.Request) {
		writeSensitiveJSON(w, nethttp.StatusOK, response{"status": "ok"})
	})
	rec := httptest.NewRecorder()
	handler(rec, httptest.NewRequest(nethttp.MethodPost, "/agent/register", nil))
	if rec.Code != nethttp.StatusOK {
		t.Fatalf("first status = %d, want 200", rec.Code)
	}
	rec = httptest.NewRecorder()
	handler(rec, httptest.NewRequest(nethttp.MethodPost, "/agent/register", nil))
	if rec.Code != nethttp.StatusTooManyRequests {
		t.Fatalf("second status = %d, want 429", rec.Code)
	}
	assertAgentRegisterErrorCode(t, rec, agentRegistrationRateLimited)
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("Retry-After header should be preserved for agent registration rate limit")
	}
	assertAgentRegisterNoStoreHeaders(t, rec)
}

func TestAgentRegisterTerminalAndInternalErrorCodes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		err      error
		wantHTTP int
		wantCode string
	}{
		{name: "retired", err: domain.ErrAgentRegistrationNodeRetired, wantHTTP: nethttp.StatusConflict, wantCode: agentEnrollmentNodeRetired},
		{name: "audit", err: domain.ErrAgentRegistrationAuditFailed, wantHTTP: nethttp.StatusInternalServerError, wantCode: agentRegistrationInternalError},
		{name: "unexpected", err: errAgentRegisterTestStoreFailure{}, wantHTTP: nethttp.StatusServiceUnavailable, wantCode: agentRegistrationTemporarilyUnavailable},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store := &agentRegisterHTTPTestStore{enrollmentErr: tc.err}
			handler := New(slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil)), store, Options{})
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(nethttp.MethodPost, "/agent/register", strings.NewReader(`{"node_id":"11111111-1111-4111-8111-111111111111","enrollment_token":"test-enrollment-token"}`))
			handler.ServeHTTP(rec, req)
			if rec.Code != tc.wantHTTP {
				t.Fatalf("status = %d body=%s, want %d", rec.Code, rec.Body.String(), tc.wantHTTP)
			}
			assertAgentRegisterErrorCode(t, rec, tc.wantCode)
			if strings.Contains(rec.Body.String(), "database is down") {
				t.Fatalf("raw store error leaked: %s", rec.Body.String())
			}
			assertAgentRegisterNoStoreHeaders(t, rec)
		})
	}
}

type errAgentRegisterTestStoreFailure struct{}

func (errAgentRegisterTestStoreFailure) Error() string {
	return "database is down with internal detail"
}

func assertAgentRegisterNoStoreHeaders(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if got := rec.Header().Get("Cache-Control"); got != "no-store, private, max-age=0" {
		t.Fatalf("Cache-Control = %q", got)
	}
	if got := rec.Header().Get("Pragma"); got != "no-cache" {
		t.Fatalf("Pragma = %q", got)
	}
	if got := rec.Header().Get("Expires"); got != "0" {
		t.Fatalf("Expires = %q", got)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q", got)
	}
	if got := rec.Header().Get("Referrer-Policy"); got != "no-referrer" {
		t.Fatalf("Referrer-Policy = %q", got)
	}
	if got := rec.Header().Get("ETag"); got != "" {
		t.Fatalf("ETag = %q, want absent", got)
	}
	if got := rec.Header().Get("Last-Modified"); got != "" {
		t.Fatalf("Last-Modified = %q, want absent", got)
	}
}

func assertAgentRegisterErrorCode(t *testing.T, rec *httptest.ResponseRecorder, want string) {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if payload["status"] != "error" {
		t.Fatalf("error status field = %#v, want error", payload["status"])
	}
	if payload["code"] != want {
		t.Fatalf("error code = %#v, want %q; payload=%#v", payload["code"], want, payload)
	}
	if _, ok := payload["agent_token"]; ok {
		t.Fatalf("error response must not include agent_token: %#v", payload)
	}
}
