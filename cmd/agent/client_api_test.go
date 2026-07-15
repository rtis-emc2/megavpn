package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/rtis-emc2/megavpn/internal/agentauth"
)

type httpDoerFunc func(*http.Request) (*http.Response, error)

func (f httpDoerFunc) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestRegisterAcceptsUnsignedHardenedResponse(t *testing.T) {
	t.Parallel()

	c := newClient("https://control.example", "", "/tmp/agent-state.json")
	c.http = httpDoerFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/agent/register" {
			t.Fatalf("path = %s, want /agent/register", req.URL.Path)
		}
		if req.Header.Get(agentauth.HeaderSignature) != "" {
			t.Fatal("initial registration request should not be signed without a local agent token")
		}
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		for _, field := range []string{`"node_id":"11111111-1111-4111-8111-111111111111"`, `"name":"bootstrap-node"`, `"address":"10.0.0.10"`, `"enrollment_token":`, `"agent_version":"`, `"protocol_version":"v1"`} {
			if !strings.Contains(string(body), field) {
				t.Fatalf("registration request missing expected field %s", field)
			}
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{
				"status":"registered",
				"node":{"id":"11111111-1111-4111-8111-111111111111","name":"operator-node","address":"10.0.0.20"},
				"agent_token":"returned-agent-token",
				"token_hint":"returned...token"
			}`)),
		}, nil
	})

	state, err := c.register(context.Background(), bootstrapConfig{
		NodeID:          "11111111-1111-4111-8111-111111111111",
		NodeName:        "bootstrap-node",
		NodeAddress:     "10.0.0.10",
		ControlPlaneURL: "https://control.example",
		EnrollmentToken: "enroll-secret",
	})
	if err != nil {
		t.Fatalf("register error = %v", err)
	}
	if state.AgentToken != "returned-agent-token" || state.NodeName != "operator-node" || state.NodeAddress != "10.0.0.20" {
		t.Fatalf("state = %#v, want token and persisted node projection from response", state)
	}
}

func TestHeartbeatAndInventoryRequestSigningUnchanged(t *testing.T) {
	t.Parallel()

	seenHeartbeat := false
	seenInventory := false
	c := newClient("https://control.example", "agent-token", "")
	c.http = httpDoerFunc(func(req *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if req.Header.Get(agentauth.HeaderSignature) == "" {
			t.Fatalf("%s request should be signed", req.URL.Path)
		}
		if err := agentauth.Verify(
			"agent-token",
			req.Method,
			req.URL.RequestURI(),
			req.Header.Get(agentauth.HeaderTimestamp),
			req.Header.Get(agentauth.HeaderNonce),
			req.Header.Get(agentauth.HeaderBodyHash),
			req.Header.Get(agentauth.HeaderSignature),
			body,
			time.Now().UTC(),
			time.Minute,
		); err != nil {
			t.Fatalf("%s signature verification failed: %v", req.URL.Path, err)
		}
		switch req.URL.Path {
		case "/agent/heartbeat":
			seenHeartbeat = true
		case "/agent/inventory":
			seenInventory = true
		default:
			t.Fatalf("unexpected path %s", req.URL.Path)
		}
		return signedAgentTestResponse(req, "agent-token", http.StatusOK, []byte(`{"status":"ok"}`+"\n")), nil
	})

	if err := c.heartbeat(context.Background(), "node-1", "node-name"); err != nil {
		t.Fatalf("heartbeat error = %v", err)
	}
	if err := c.submitInventory(context.Background(), "node-1", "test", map[string]any{"hostname": "node-1"}); err != nil {
		t.Fatalf("submitInventory error = %v", err)
	}
	if !seenHeartbeat || !seenInventory {
		t.Fatalf("seen heartbeat=%v inventory=%v, want both", seenHeartbeat, seenInventory)
	}
}

func TestNextJobVerifiesSignedResponse(t *testing.T) {
	t.Parallel()

	c := newClient("https://control.example", "agent-token", "")
	c.http = httpDoerFunc(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get(agentauth.HeaderSignature) == "" {
			t.Fatal("agent request should be signed")
		}
		body := []byte(`{"id":"job-1","type":"instance.apply","payload":{}}` + "\n")
		return signedAgentTestResponse(req, "agent-token", http.StatusOK, body), nil
	})

	j, ok, err := c.nextJob(context.Background(), "node-1")
	if err != nil {
		t.Fatalf("nextJob error = %v", err)
	}
	if !ok || j.ID != "job-1" || j.Type != "instance.apply" {
		t.Fatalf("nextJob = %#v/%v, want signed job", j, ok)
	}
}

func TestNextJobRejectsTamperedSignedResponse(t *testing.T) {
	t.Parallel()

	c := newClient("https://control.example", "agent-token", "")
	c.http = httpDoerFunc(func(req *http.Request) (*http.Response, error) {
		signedBody := []byte(`{"id":"job-1","type":"instance.apply","payload":{}}` + "\n")
		resp := signedAgentTestResponse(req, "agent-token", http.StatusOK, signedBody)
		resp.Body = io.NopCloser(bytes.NewReader([]byte(`{"id":"job-1","type":"node.bootstrap","payload":{}}` + "\n")))
		return resp, nil
	})

	_, _, err := c.nextJob(context.Background(), "node-1")
	if err == nil || !strings.Contains(err.Error(), "signature verification failed") {
		t.Fatalf("nextJob tampered response error = %v, want signature verification failure", err)
	}
}

func TestNextJobRejectsUnsignedOKResponse(t *testing.T) {
	t.Parallel()

	c := newClient("https://control.example", "agent-token", "")
	c.http = httpDoerFunc(func(req *http.Request) (*http.Response, error) {
		body := []byte(`{"id":"job-1","type":"instance.apply","payload":{}}` + "\n")
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewReader(body)),
		}, nil
	})

	_, _, err := c.nextJob(context.Background(), "node-1")
	if err == nil || !strings.Contains(err.Error(), "unsigned agent response rejected") {
		t.Fatalf("nextJob unsigned 200 error = %v, want unsigned response rejection", err)
	}
}

func TestNextJobRejectsUnsignedNoContentResponse(t *testing.T) {
	t.Parallel()

	c := newClient("https://control.example", "agent-token", "")
	c.http = httpDoerFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusNoContent,
			Header:     http.Header{},
			Body:       io.NopCloser(bytes.NewReader(nil)),
		}, nil
	})

	_, _, err := c.nextJob(context.Background(), "node-1")
	if err == nil || !strings.Contains(err.Error(), "unsigned agent response rejected") {
		t.Fatalf("nextJob unsigned 204 error = %v, want unsigned response rejection", err)
	}
}

func TestNextJobAcceptsSignedNoContentResponse(t *testing.T) {
	t.Parallel()

	c := newClient("https://control.example", "agent-token", "")
	c.http = httpDoerFunc(func(req *http.Request) (*http.Response, error) {
		return signedAgentTestResponse(req, "agent-token", http.StatusNoContent, nil), nil
	})

	j, ok, err := c.nextJob(context.Background(), "node-1")
	if err != nil {
		t.Fatalf("nextJob signed 204 error = %v", err)
	}
	if ok || j.ID != "" {
		t.Fatalf("nextJob signed 204 = %#v/%v, want no job", j, ok)
	}
}

func TestNextJobRejectsReplayedSignedResponse(t *testing.T) {
	t.Parallel()

	c := newClient("https://control.example", "agent-token", "")
	var replayHeaders http.Header
	c.http = httpDoerFunc(func(req *http.Request) (*http.Response, error) {
		body := []byte(`{"id":"job-1","type":"instance.apply","payload":{}}` + "\n")
		resp := signedAgentTestResponse(req, "agent-token", http.StatusOK, body)
		if replayHeaders == nil {
			replayHeaders = resp.Header.Clone()
		} else {
			resp.Header = replayHeaders.Clone()
		}
		return resp, nil
	})

	if _, ok, err := c.nextJob(context.Background(), "node-1"); err != nil || !ok {
		t.Fatalf("first nextJob error = %v ok=%v, want success", err, ok)
	}
	_, _, err := c.nextJob(context.Background(), "node-1")
	if err == nil || !strings.Contains(err.Error(), "replay") {
		t.Fatalf("second nextJob error = %v, want replay rejection", err)
	}
}

func signedAgentTestResponse(req *http.Request, token string, status int, body []byte) *http.Response {
	timestamp := strconv.FormatInt(time.Now().UTC().Unix(), 10)
	nonce := agentauth.NewNonce()
	signature, bodyHash := agentauth.Sign(token, "RESPONSE", req.URL.RequestURI(), timestamp, nonce, body)
	header := http.Header{}
	header.Set("Content-Type", "application/json; charset=utf-8")
	header.Set(agentauth.HeaderTimestamp, timestamp)
	header.Set(agentauth.HeaderNonce, nonce)
	header.Set(agentauth.HeaderBodyHash, bodyHash)
	header.Set(agentauth.HeaderSignature, signature)
	return &http.Response{
		StatusCode: status,
		Header:     header,
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
}
