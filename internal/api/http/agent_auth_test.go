package http

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

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

func TestGlobalAgentTokenFallbackRequiresAutoRegisterMode(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/agent/heartbeat", nil)
	req.Header.Set("Authorization", "Bearer shared")

	s := &Server{agentToken: "shared"}
	if s.authorizeAgentNode(req, "node-1") {
		t.Fatal("global agent token fallback should be disabled without auto-register mode")
	}
	if s.authorizeAgentJob(req, "job-1") {
		t.Fatal("global job token fallback should be disabled without auto-register mode")
	}

	s.allowAutoRegister = true
	if !s.authorizeAgentNode(req, "node-1") {
		t.Fatal("global agent token fallback should work in auto-register mode")
	}
	if !s.authorizeAgentJob(req, "job-1") {
		t.Fatal("global job token fallback should work in auto-register mode")
	}
}
