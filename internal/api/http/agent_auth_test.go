package http

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rtis-emc2/megavpn/internal/agentauth"
)

type agentAuthTestStore struct {
	Store
}

type agentAuthFailureRecordingStore struct {
	agentAuthTestStore
	reasons []string
}

type agentHeartbeatPersistenceStore struct {
	agentAuthTestStore
	err error
}

func (agentAuthTestStore) ValidateAgentToken(_ context.Context, nodeID, token string) bool {
	return nodeID == "node-1" && token == "node-token"
}

func (agentAuthTestStore) ValidateAgentTokenForJob(_ context.Context, jobID, token string) bool {
	return jobID == "job-1" && token == "node-token"
}

func (agentAuthTestStore) RecordAgentAuthFailure(_ context.Context, _, _ string) error {
	return nil
}

func (s *agentAuthFailureRecordingStore) RecordAgentAuthFailure(_ context.Context, _, reason string) error {
	s.reasons = append(s.reasons, reason)
	return nil
}

func (s agentHeartbeatPersistenceStore) HeartbeatByNodeIDWithVersion(_ context.Context, _, _, _ string) error {
	return s.err
}

func TestAuthorizeAgentBootstrapRequiresConfiguredToken(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/agent/register", nil)
	req.Header.Set("Authorization", "Bearer shared")

	s := &Server{}
	if s.authorizeAgentBootstrap(req, "shared") {
		t.Fatal("bootstrap auth should fail when no server token is configured")
	}
}

func TestAuthorizeAgentBootstrapAcceptsBearerOrPayloadToken(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/agent/register", nil)

	s := &Server{agentToken: "shared"}
	if !s.authorizeAgentBootstrap(req, "shared") {
		t.Fatal("bootstrap auth should accept matching payload token")
	}
	req.Header.Set("Authorization", "Bearer shared")
	if !s.authorizeAgentBootstrap(req, "") {
		t.Fatal("bootstrap auth should accept matching bearer token")
	}
}

func TestGlobalAgentTokenIsNotAcceptedForNodeOrJobAuthorization(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/agent/heartbeat", nil)
	req.Header.Set("Authorization", "Bearer shared")

	s := &Server{agentToken: "shared"}
	if s.authorizeAgentNode(req, "node-1") {
		t.Fatal("global agent token must not authorize node calls")
	}
	if s.authorizeAgentJob(req, "job-1") {
		t.Fatal("global agent token must not authorize job calls")
	}
}

func TestAgentSignatureEnforcement(t *testing.T) {
	body := []byte(`{"node_id":"node-1"}`)
	req := httptest.NewRequest(http.MethodPost, "/agent/heartbeat", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer node-token")
	signTestAgentRequest(req, "node-token", body)

	s := &Server{
		store:                 agentAuthTestStore{},
		agentSignatureEnforce: true,
		agentSignatureWindow:  time.Minute,
		agentSignatureReplay:  newAgentSignatureReplayCache(time.Minute),
	}
	if !s.authorizeAgentNode(req, "node-1") {
		t.Fatal("signed agent request should be authorized when enforcement is enabled")
	}
	var payload map[string]any
	if !decode(req, &payload) || payload["node_id"] != "node-1" {
		t.Fatalf("signed request body should remain readable after auth: %#v", payload)
	}
}

func TestAgentSignatureRejectsUnsignedWhenEnforced(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/agent/jobs/next?node_id=node-1", nil)
	req.Header.Set("Authorization", "Bearer node-token")

	s := &Server{store: agentAuthTestStore{}, agentSignatureEnforce: true}
	if s.authorizeAgentNode(req, "node-1") {
		t.Fatal("unsigned agent request should be rejected when enforcement is enabled")
	}
}

func TestAgentHeartbeatUnauthorizedResponseIsSigned(t *testing.T) {
	body := []byte(`{"node_id":"node-1","name":"node-1"}`)
	req := httptest.NewRequest(http.MethodPost, "/agent/heartbeat", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer stale-node-token")
	signTestAgentRequest(req, "stale-node-token", body)
	rr := httptest.NewRecorder()

	s := &Server{
		store:                 agentAuthTestStore{},
		agentSignatureEnforce: true,
		agentSignatureWindow:  time.Minute,
		agentSignatureReplay:  newAgentSignatureReplayCache(time.Minute),
	}
	s.agentHeartbeat(rr, req)

	resp := rr.Result()
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read unauthorized heartbeat response: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	if bytes.Contains(responseBody, []byte("stale-node-token")) {
		t.Fatal("unauthorized response must not echo the presented agent token")
	}
	err = agentauth.Verify(
		"stale-node-token",
		"RESPONSE",
		req.URL.RequestURI(),
		resp.Header.Get(agentauth.HeaderTimestamp),
		resp.Header.Get(agentauth.HeaderNonce),
		resp.Header.Get(agentauth.HeaderBodyHash),
		resp.Header.Get(agentauth.HeaderSignature),
		responseBody,
		time.Now().UTC(),
		time.Minute,
	)
	if err != nil {
		t.Fatalf("unauthorized heartbeat response signature error = %v, want nil", err)
	}
}

func TestAgentHeartbeatRecordsActionableClockSkewForValidIdentity(t *testing.T) {
	body := []byte(`{"node_id":"node-1","name":"node-1"}`)
	req := httptest.NewRequest(http.MethodPost, "/agent/heartbeat", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer node-token")
	timestamp := strconv.FormatInt(time.Now().UTC().Add(-11*time.Minute).Unix(), 10)
	nonce := agentauth.NewNonce()
	signature, bodyHash := agentauth.Sign("node-token", req.Method, req.URL.RequestURI(), timestamp, nonce, body)
	req.Header.Set(agentauth.HeaderTimestamp, timestamp)
	req.Header.Set(agentauth.HeaderNonce, nonce)
	req.Header.Set(agentauth.HeaderBodyHash, bodyHash)
	req.Header.Set(agentauth.HeaderSignature, signature)
	rr := httptest.NewRecorder()
	store := &agentAuthFailureRecordingStore{}

	s := &Server{
		store:                 store,
		agentSignatureEnforce: true,
		agentSignatureWindow:  time.Minute,
		agentSignatureReplay:  newAgentSignatureReplayCache(time.Minute),
	}
	s.agentHeartbeat(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
	if len(store.reasons) != 1 {
		t.Fatalf("recorded auth failures = %#v, want one", store.reasons)
	}
	for _, want := range []string{"timestamp is outside allowed window", "agent clock is behind", "synchronize NTP"} {
		if !bytes.Contains([]byte(store.reasons[0]), []byte(want)) {
			t.Fatalf("recorded auth failure = %q, want %q", store.reasons[0], want)
		}
	}
}

func TestAgentHeartbeatDoesNotLetInvalidTokenChangeNodeDiagnostics(t *testing.T) {
	body := []byte(`{"node_id":"node-1","name":"node-1"}`)
	req := httptest.NewRequest(http.MethodPost, "/agent/heartbeat", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer attacker-token")
	signTestAgentRequest(req, "attacker-token", body)
	rr := httptest.NewRecorder()
	store := &agentAuthFailureRecordingStore{}

	s := &Server{
		store:                 store,
		agentSignatureEnforce: true,
		agentSignatureWindow:  time.Minute,
		agentSignatureReplay:  newAgentSignatureReplayCache(time.Minute),
	}
	s.agentHeartbeat(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
	if len(store.reasons) != 0 {
		t.Fatalf("invalid token changed node diagnostics: %#v", store.reasons)
	}
}

func TestAgentHeartbeatDistinguishesMissingNodeFromPersistenceFailure(t *testing.T) {
	tests := []struct {
		name       string
		storeErr   error
		wantStatus int
		wantBody   string
	}{
		{name: "missing node", storeErr: pgx.ErrNoRows, wantStatus: http.StatusNotFound, wantBody: "node not registered"},
		{name: "database failure", storeErr: errors.New("database write failed"), wantStatus: http.StatusInternalServerError, wantBody: "heartbeat persistence failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(`{"node_id":"node-1","name":"node-1"}`)
			req := httptest.NewRequest(http.MethodPost, "/agent/heartbeat", bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer node-token")
			signTestAgentRequest(req, "node-token", body)
			rr := httptest.NewRecorder()

			s := &Server{
				store:                 agentHeartbeatPersistenceStore{err: tt.storeErr},
				agentSignatureEnforce: true,
				agentSignatureWindow:  time.Minute,
				agentSignatureReplay:  newAgentSignatureReplayCache(time.Minute),
			}
			s.agentHeartbeat(rr, req)

			if rr.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rr.Code, tt.wantStatus)
			}
			if !strings.Contains(rr.Body.String(), tt.wantBody) {
				t.Fatalf("body = %q, want %q", rr.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestAgentSignatureCompatibilityAllowsUnsignedButRejectsInvalidSigned(t *testing.T) {
	unsigned := httptest.NewRequest(http.MethodGet, "/agent/jobs/next?node_id=node-1", nil)
	unsigned.Header.Set("Authorization", "Bearer node-token")
	s := &Server{store: agentAuthTestStore{}}
	if !s.authorizeAgentNode(unsigned, "node-1") {
		t.Fatal("unsigned agent request should be allowed before enforcement is enabled")
	}

	body := []byte(`{"node_id":"node-1"}`)
	invalid := httptest.NewRequest(http.MethodPost, "/agent/heartbeat", bytes.NewReader([]byte(`{"node_id":"tampered"}`)))
	invalid.Header.Set("Authorization", "Bearer node-token")
	signTestAgentRequest(invalid, "node-token", body)
	if s.authorizeAgentNode(invalid, "node-1") {
		t.Fatal("invalid signed agent request should be rejected even before enforcement")
	}
}

func TestAgentSignatureRejectsReplay(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/agent/jobs/next?node_id=node-1", nil)
	req.Header.Set("Authorization", "Bearer node-token")
	signTestAgentRequest(req, "node-token", nil)

	s := &Server{
		store:                 agentAuthTestStore{},
		agentSignatureEnforce: true,
		agentSignatureWindow:  time.Minute,
		agentSignatureReplay:  newAgentSignatureReplayCache(time.Minute),
	}
	if !s.authorizeAgentNode(req, "node-1") {
		t.Fatal("first signed request should be authorized")
	}
	if s.authorizeAgentNode(req, "node-1") {
		t.Fatal("replayed signed request should be rejected")
	}
}

func TestAgentSignatureReplayCacheFailsClosedAtCapacity(t *testing.T) {
	cache := newAgentSignatureReplayCache(time.Minute)
	now := time.Now().UTC()
	for index := 0; index < maxAgentSignatureReplayEntries; index++ {
		if !cache.accept(fmt.Sprintf("node:%d", index), now) {
			t.Fatalf("replay cache rejected entry %d before capacity", index)
		}
	}
	if cache.accept("node:overflow", now) {
		t.Fatal("replay cache must fail closed at capacity")
	}
	if cache.accept("node:0", now) {
		t.Fatal("replay cache must continue rejecting a replay at capacity")
	}
}

func signTestAgentRequest(req *http.Request, token string, body []byte) {
	timestamp := strconv.FormatInt(time.Now().UTC().Unix(), 10)
	nonce := agentauth.NewNonce()
	signature, bodyHash := agentauth.Sign(token, req.Method, req.URL.RequestURI(), timestamp, nonce, body)
	req.Header.Set(agentauth.HeaderTimestamp, timestamp)
	req.Header.Set(agentauth.HeaderNonce, nonce)
	req.Header.Set(agentauth.HeaderBodyHash, bodyHash)
	req.Header.Set(agentauth.HeaderSignature, signature)
}
