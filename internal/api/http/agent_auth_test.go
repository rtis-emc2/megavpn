package http

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/rtis-emc2/megavpn/internal/agentauth"
)

type agentAuthTestStore struct {
	Store
}

func (agentAuthTestStore) ValidateAgentToken(_ context.Context, nodeID, token string) bool {
	return nodeID == "node-1" && token == "node-token"
}

func (agentAuthTestStore) ValidateAgentTokenForJob(_ context.Context, jobID, token string) bool {
	return jobID == "job-1" && token == "node-token"
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

func signTestAgentRequest(req *http.Request, token string, body []byte) {
	timestamp := strconv.FormatInt(time.Now().UTC().Unix(), 10)
	nonce := agentauth.NewNonce()
	signature, bodyHash := agentauth.Sign(token, req.Method, req.URL.RequestURI(), timestamp, nonce, body)
	req.Header.Set(agentauth.HeaderTimestamp, timestamp)
	req.Header.Set(agentauth.HeaderNonce, nonce)
	req.Header.Set(agentauth.HeaderBodyHash, bodyHash)
	req.Header.Set(agentauth.HeaderSignature, signature)
}
