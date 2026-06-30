package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
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

func TestBinaryRepositoryRuntimeDefaultsToCopyBinary(t *testing.T) {
	t.Parallel()

	if got := defaultBinaryInstallMode("xray-core", "runtime", "/tmp/artifact"); got != "copy_binary" {
		t.Fatalf("xray runtime mode = %q, want copy_binary", got)
	}
	if got := defaultBinaryInstallMode("shadowsocks", "runtime", "/tmp/artifact"); got != "copy_binary" {
		t.Fatalf("shadowsocks runtime mode = %q, want copy_binary", got)
	}
	if got := defaultBinaryInstallMode("xray-core", "bundle", "/tmp/artifact"); got != "zip_binary" {
		t.Fatalf("xray bundle mode = %q, want zip_binary", got)
	}
	if got := defaultBinaryInstallPath("xray-core"); got != "/usr/local/bin/xray" {
		t.Fatalf("xray default install path = %q", got)
	}
	if err := validateBinaryInstallPath("xray-core", "/etc/passwd"); err == nil {
		t.Fatal("validateBinaryInstallPath accepted disallowed path")
	}
	if err := validateBinaryInstallPath("shadowsocks", "/usr/local/bin/ss-server"); err != nil {
		t.Fatalf("validateBinaryInstallPath rejected ss-server path: %v", err)
	}
}

func TestExtractZipBinaryArtifactSelectsXrayBinary(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	archivePath := filepath.Join(dir, "xray.zip")
	writeTestZip(t, archivePath, map[string]string{
		"README.md": "runtime archive",
		"xray":      "#!/bin/sh\nexit 0\n",
	})

	extracted, member, err := extractZipBinaryArtifact(archivePath, dir, "xray-core", "")
	if err != nil {
		t.Fatalf("extract zip binary: %v", err)
	}
	if member != "xray" {
		t.Fatalf("selected member = %q, want xray", member)
	}
	body, err := os.ReadFile(extracted)
	if err != nil {
		t.Fatalf("read extracted binary: %v", err)
	}
	if string(body) != "#!/bin/sh\nexit 0\n" {
		t.Fatalf("extracted body = %q", string(body))
	}
	info, err := os.Stat(extracted)
	if err != nil {
		t.Fatalf("stat extracted binary: %v", err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("extracted mode = %v, want 0700", info.Mode().Perm())
	}
}

func TestExtractZipBinaryArtifactRejectsUnsafePreferredMember(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	archivePath := filepath.Join(dir, "xray.zip")
	writeTestZip(t, archivePath, map[string]string{"xray": "binary"})
	_, _, err := extractZipBinaryArtifact(archivePath, dir, "xray-core", "../xray")
	if err == nil || !strings.Contains(err.Error(), "not safe") {
		t.Fatalf("unsafe preferred member error = %v, want safety error", err)
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

func writeTestZip(t *testing.T, path string, files map[string]string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	writer := zip.NewWriter(file)
	for name, body := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatalf("create zip member %s: %v", name, err)
		}
		if _, err := entry.Write([]byte(body)); err != nil {
			t.Fatalf("write zip member %s: %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close zip file: %v", err)
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
