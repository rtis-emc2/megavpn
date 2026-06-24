package http

import (
	"io"
	nethttp "net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rtis-emc2/megavpn/internal/agentauth"
)

func TestWriteSignedAgentJSON(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(nethttp.MethodGet, "/agent/jobs/next?node_id=node-1", nil)
	rr := httptest.NewRecorder()
	writeSignedAgentJSON(rr, req, "agent-token", nethttp.StatusOK, response{"status": "ok"})

	resp := rr.Result()
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read signed response body: %v", err)
	}
	if resp.Header.Get(agentauth.HeaderSignature) == "" {
		t.Fatal("signed response should include signature header")
	}
	err = agentauth.Verify(
		"agent-token",
		"RESPONSE",
		req.URL.RequestURI(),
		resp.Header.Get(agentauth.HeaderTimestamp),
		resp.Header.Get(agentauth.HeaderNonce),
		resp.Header.Get(agentauth.HeaderBodyHash),
		resp.Header.Get(agentauth.HeaderSignature),
		body,
		time.Now().UTC(),
		time.Minute,
	)
	if err != nil {
		t.Fatalf("signed response verification error = %v, want nil", err)
	}
}

func TestWriteSignedAgentNoContent(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(nethttp.MethodGet, "/agent/jobs/next?node_id=node-1", nil)
	rr := httptest.NewRecorder()
	writeSignedAgentNoContent(rr, req, "agent-token")

	resp := rr.Result()
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read signed no-content body: %v", err)
	}
	if len(body) != 0 {
		t.Fatalf("no-content body length = %d, want 0", len(body))
	}
	if resp.StatusCode != nethttp.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
	err = agentauth.Verify(
		"agent-token",
		"RESPONSE",
		req.URL.RequestURI(),
		resp.Header.Get(agentauth.HeaderTimestamp),
		resp.Header.Get(agentauth.HeaderNonce),
		resp.Header.Get(agentauth.HeaderBodyHash),
		resp.Header.Get(agentauth.HeaderSignature),
		body,
		time.Now().UTC(),
		time.Minute,
	)
	if err != nil {
		t.Fatalf("signed no-content verification error = %v, want nil", err)
	}
}
