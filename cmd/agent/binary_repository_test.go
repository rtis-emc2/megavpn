package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestDownloadBinaryRepositoryArtifactRequiresSignedResponse(t *testing.T) {
	t.Parallel()

	body := []byte("runtime artifact")
	expectedSHA := sha256Hex(body)
	c := newClient("https://control.example", "agent-token", "")
	c.http = httpDoerFunc(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get(binaryDownloadTicketHeader) != "ticket-secret" {
			t.Fatalf("download ticket header = %q, want ticket-secret", req.Header.Get(binaryDownloadTicketHeader))
		}
		return signedAgentTestResponse(req, "agent-token", http.StatusOK, body), nil
	})

	artifact, err := c.downloadBinaryRepositoryArtifact(context.Background(), binaryRepositoryTestJob(), binaryRepositoryTestPayload(expectedSHA))
	if err != nil {
		t.Fatalf("download signed artifact: %v", err)
	}
	defer artifactCleanup(artifact)
	if artifact.SHA256 != expectedSHA {
		t.Fatalf("artifact sha = %q, want %q", artifact.SHA256, expectedSHA)
	}
}

func TestDownloadBinaryRepositoryArtifactRejectsUnsignedOK(t *testing.T) {
	t.Parallel()

	body := []byte("runtime artifact")
	c := newClient("https://control.example", "agent-token", "")
	c.http = httpDoerFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/octet-stream"}},
			Body:       io.NopCloser(bytes.NewReader(body)),
		}, nil
	})

	_, err := c.downloadBinaryRepositoryArtifact(context.Background(), binaryRepositoryTestJob(), binaryRepositoryTestPayload(sha256Hex(body)))
	if err == nil || !strings.Contains(err.Error(), "unsigned binary repository response rejected") {
		t.Fatalf("download unsigned artifact error = %v, want unsigned rejection", err)
	}
}

func TestDownloadBinaryRepositoryArtifactRejectsTamperedSignedBody(t *testing.T) {
	t.Parallel()

	signedBody := []byte("runtime artifact")
	tamperedBody := []byte("tampered artifact")
	c := newClient("https://control.example", "agent-token", "")
	c.http = httpDoerFunc(func(req *http.Request) (*http.Response, error) {
		resp := signedAgentTestResponse(req, "agent-token", http.StatusOK, signedBody)
		resp.Body = io.NopCloser(bytes.NewReader(tamperedBody))
		return resp, nil
	})

	_, err := c.downloadBinaryRepositoryArtifact(context.Background(), binaryRepositoryTestJob(), binaryRepositoryTestPayload(sha256Hex(signedBody)))
	if err == nil || !strings.Contains(err.Error(), "signature verification failed") {
		t.Fatalf("download tampered artifact error = %v, want signature failure", err)
	}
}

func binaryRepositoryTestJob() job {
	return job{
		ID: "job-1",
		Payload: map[string]any{
			"node_id": "node-1",
		},
	}
}

func binaryRepositoryTestPayload(sha string) map[string]any {
	return map[string]any{
		"artifact_id":     "artifact-1",
		"download_path":   "/agent/binary-artifacts/artifact-1/download",
		"download_token":  "ticket-secret",
		"sha256":          sha,
		"kind":            "runtime",
		"version":         "1.0.0",
		"metadata":        map[string]any{},
		"ticket_hint":     "tick...cret",
		"service_code":    "xray-core",
		"binary_artifact": "test",
	}
}

func sha256Hex(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func artifactCleanup(artifact downloadedBinaryArtifact) {
	if artifact.Dir != "" {
		_ = os.RemoveAll(artifact.Dir)
	}
}
