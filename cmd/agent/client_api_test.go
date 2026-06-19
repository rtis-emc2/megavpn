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
