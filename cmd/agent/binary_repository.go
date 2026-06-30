package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rtis-emc2/megavpn/internal/agentauth"
)

const binaryDownloadTicketHeader = "X-MegaVPN-Binary-Ticket"

type downloadedBinaryArtifact struct {
	Path    string
	Dir     string
	SHA256  string
	Version string
	Kind    string
	Mode    string
}

func (c client) installBinaryRepositoryCapability(ctx context.Context, j job, serviceCode string) map[string]any {
	startedSteps := []map[string]any{}
	if os.Geteuid() != 0 {
		return map[string]any{"ok": false, "message": serviceCode + " binary repository install requires root", "steps": startedSteps}
	}
	repo, ok := j.Payload["binary_repository"].(map[string]any)
	if !ok || repo == nil {
		return map[string]any{"ok": false, "message": "binary repository payload is missing", "steps": startedSteps}
	}
	artifact, err := c.downloadBinaryRepositoryArtifact(ctx, j, repo)
	if err != nil {
		return map[string]any{"ok": false, "message": "binary repository artifact download failed", "error": err.Error(), "steps": startedSteps}
	}
	defer os.RemoveAll(artifact.Dir)

	steps := append(startedSteps, map[string]any{
		"stage":   "download",
		"path":    artifact.Path,
		"sha256":  artifact.SHA256,
		"kind":    artifact.Kind,
		"version": artifact.Version,
	})
	mode := artifact.Mode
	if mode == "" {
		mode = defaultBinaryInstallMode(serviceCode, artifact.Kind, artifact.Path)
	}
	repositoryResult := binaryRepositoryInstallResult(repo, artifact, mode)
	if mode == "" {
		return map[string]any{"ok": false, "message": "binary repository artifact install_mode is unsupported", "kind": artifact.Kind, "steps": steps, "binary_repository": repositoryResult}
	}
	run := func(name string, args ...string) bool {
		code, out := runInstallCommand(ctx, name, args...)
		steps = append(steps, map[string]any{"command": append([]string{name}, args...), "exit_code": code, "output": truncate(out, 4000)})
		return code == 0
	}
	switch mode {
	case "xray_install_script":
		if serviceCode != "xray-core" {
			return map[string]any{"ok": false, "message": "xray_install_script mode is only valid for xray-core", "steps": steps, "binary_repository": repositoryResult}
		}
		if err := os.Chmod(artifact.Path, 0o700); err != nil {
			return map[string]any{"ok": false, "message": "binary repository script chmod failed", "error": err.Error(), "steps": steps, "binary_repository": repositoryResult}
		}
		if !run("bash", artifact.Path, "install") {
			return map[string]any{"ok": false, "message": "xray-core repository install script failed", "steps": steps, "binary_repository": repositoryResult}
		}
	case "deb_package":
		if !run("dpkg", "-i", artifact.Path) {
			_ = run("env", "DEBIAN_FRONTEND=noninteractive", "apt-get", "-f", "install", "-y")
		}
	default:
		return map[string]any{"ok": false, "message": "unsupported binary repository install_mode", "install_mode": mode, "steps": steps, "binary_repository": repositoryResult}
	}
	verify := verifyInstalledCapability(ctx, serviceCode)
	verify["steps"] = steps
	verify["binary_repository"] = repositoryResult
	return verify
}

func (c client) downloadBinaryRepositoryArtifact(ctx context.Context, j job, repo map[string]any) (downloadedBinaryArtifact, error) {
	artifactID := stringify(repo["artifact_id"])
	downloadPath := stringify(repo["download_path"])
	downloadToken := stringify(repo["download_token"])
	expectedSHA := strings.ToLower(strings.TrimSpace(stringify(repo["sha256"])))
	if artifactID == "" || downloadPath == "" || downloadToken == "" || expectedSHA == "" {
		return downloadedBinaryArtifact{}, errors.New("artifact_id, download_path, download_token and sha256 are required")
	}
	if !strings.HasPrefix(downloadPath, "/agent/binary-artifacts/") {
		return downloadedBinaryArtifact{}, errors.New("download_path must target the agent binary artifact endpoint")
	}
	u, err := url.Parse(c.baseURL + downloadPath)
	if err != nil {
		return downloadedBinaryArtifact{}, err
	}
	q := u.Query()
	q.Set("node_id", stringify(j.Payload["node_id"]))
	q.Set("job_id", j.ID)
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return downloadedBinaryArtifact{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set(binaryDownloadTicketHeader, downloadToken)
	c.signRequest(req, nil)
	resp, err := c.http.Do(req)
	if err != nil {
		return downloadedBinaryArtifact{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, readErr := c.readSignedResponseBody(req, resp, true)
		if readErr != nil {
			return downloadedBinaryArtifact{}, readErr
		}
		if int64(len(b)) > 4096 {
			b = b[:4096]
		}
		return downloadedBinaryArtifact{}, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(b))
	}
	dir, err := os.MkdirTemp("", "megavpn-runtime-*")
	if err != nil {
		return downloadedBinaryArtifact{}, err
	}
	path := filepath.Join(dir, "artifact")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		os.RemoveAll(dir)
		return downloadedBinaryArtifact{}, err
	}
	hash := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(f, hash), resp.Body)
	closeErr := f.Close()
	if copyErr != nil || closeErr != nil {
		os.RemoveAll(dir)
		if copyErr != nil {
			return downloadedBinaryArtifact{}, copyErr
		}
		return downloadedBinaryArtifact{}, closeErr
	}
	gotSHA := hex.EncodeToString(hash.Sum(nil))
	if err := c.verifyBinaryRepositoryDownloadSignature(req, resp, gotSHA); err != nil {
		os.RemoveAll(dir)
		return downloadedBinaryArtifact{}, err
	}
	if gotSHA != expectedSHA {
		os.RemoveAll(dir)
		return downloadedBinaryArtifact{}, fmt.Errorf("sha256 mismatch: got %s", gotSHA)
	}
	return downloadedBinaryArtifact{
		Path:    path,
		Dir:     dir,
		SHA256:  gotSHA,
		Version: stringify(repo["version"]),
		Kind:    stringify(repo["kind"]),
		Mode:    binaryInstallMode(repo),
	}, nil
}

func (c client) verifyBinaryRepositoryDownloadSignature(req *http.Request, resp *http.Response, bodyHash string) error {
	if !responseHasAgentSignature(resp) {
		return errors.New("unsigned binary repository response rejected")
	}
	if strings.TrimSpace(c.token) == "" {
		return errors.New("signed binary repository response received without local agent token")
	}
	err := agentauth.VerifyBodyHash(
		c.token,
		"RESPONSE",
		req.URL.RequestURI(),
		resp.Header.Get(agentauth.HeaderTimestamp),
		resp.Header.Get(agentauth.HeaderNonce),
		resp.Header.Get(agentauth.HeaderBodyHash),
		resp.Header.Get(agentauth.HeaderSignature),
		bodyHash,
		time.Now().UTC(),
		5*time.Minute,
	)
	if err != nil {
		return fmt.Errorf("binary repository response signature verification failed: %w", err)
	}
	if c.responseReplay == nil {
		c.responseReplay = newResponseReplayCache(5 * time.Minute)
	}
	replayKey := req.URL.RequestURI() + ":" + strings.TrimSpace(resp.Header.Get(agentauth.HeaderNonce))
	if !c.responseReplay.accept(replayKey, time.Now().UTC()) {
		return errors.New("binary repository response signature replay rejected")
	}
	return nil
}

func binaryInstallMode(repo map[string]any) string {
	metadata, _ := repo["metadata"].(map[string]any)
	return stringify(metadata["install_mode"])
}

func binaryRepositoryInstallResult(repo map[string]any, artifact downloadedBinaryArtifact, mode string) map[string]any {
	result := map[string]any{
		"kind":                 artifact.Kind,
		"version":              artifact.Version,
		"sha256":               artifact.SHA256,
		"install_mode":         mode,
		"download_verified":    true,
		"download_ticket_id":   stringify(repo["ticket_id"]),
		"download_ticket_hint": stringify(repo["ticket_hint"]),
	}
	return result
}

func defaultBinaryInstallMode(serviceCode, kind, path string) string {
	serviceCode = strings.TrimSpace(serviceCode)
	kind = strings.TrimSpace(kind)
	if serviceCode == "xray-core" && kind == "script" {
		return "xray_install_script"
	}
	if kind == "package" || strings.HasSuffix(strings.ToLower(path), ".deb") {
		return "deb_package"
	}
	return ""
}

func verifyInstalledCapability(ctx context.Context, serviceCode string) map[string]any {
	switch serviceCode {
	case "xray-core":
		return verifyXrayCore(ctx)
	case "shadowsocks":
		return verifyShadowsocks(ctx)
	default:
		return map[string]any{"ok": false, "message": "no binary repository verify handler for " + serviceCode}
	}
}
