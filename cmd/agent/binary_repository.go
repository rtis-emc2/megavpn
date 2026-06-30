package main

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/rtis-emc2/megavpn/internal/agentauth"
)

const binaryDownloadTicketHeader = "X-MegaVPN-Binary-Ticket"
const maxExtractedBinaryBytes int64 = 256 * 1024 * 1024

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
	case "copy_binary":
		targetPath := binaryInstallPath(serviceCode, repo)
		if err := validateBinaryInstallPath(serviceCode, targetPath); err != nil {
			return map[string]any{"ok": false, "message": "binary repository install_path is not allowed", "error": err.Error(), "steps": steps, "binary_repository": repositoryResult}
		}
		if err := installBinaryExecutable(artifact.Path, targetPath); err != nil {
			return map[string]any{"ok": false, "message": "binary repository executable install failed", "error": err.Error(), "steps": steps, "binary_repository": repositoryResult}
		}
		repositoryResult["install_path"] = targetPath
		steps = append(steps, map[string]any{"stage": "install", "mode": mode, "install_path": targetPath})
	case "zip_binary":
		targetPath := binaryInstallPath(serviceCode, repo)
		if err := validateBinaryInstallPath(serviceCode, targetPath); err != nil {
			return map[string]any{"ok": false, "message": "binary repository install_path is not allowed", "error": err.Error(), "steps": steps, "binary_repository": repositoryResult}
		}
		binaryPath, member, err := extractZipBinaryArtifact(artifact.Path, artifact.Dir, serviceCode, binaryArchiveBinaryPath(repo))
		if err != nil {
			return map[string]any{"ok": false, "message": "binary repository zip extraction failed", "error": err.Error(), "steps": steps, "binary_repository": repositoryResult}
		}
		steps = append(steps, map[string]any{"stage": "extract", "mode": mode, "archive_binary_path": member})
		if err := installBinaryExecutable(binaryPath, targetPath); err != nil {
			return map[string]any{"ok": false, "message": "binary repository executable install failed", "error": err.Error(), "steps": steps, "binary_repository": repositoryResult}
		}
		repositoryResult["install_path"] = targetPath
		repositoryResult["archive_binary_path"] = member
		steps = append(steps, map[string]any{"stage": "install", "mode": mode, "install_path": targetPath})
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

func binaryInstallPath(serviceCode string, repo map[string]any) string {
	metadata, _ := repo["metadata"].(map[string]any)
	if path := strings.TrimSpace(stringify(metadata["install_path"])); path != "" {
		return path
	}
	return defaultBinaryInstallPath(serviceCode)
}

func binaryArchiveBinaryPath(repo map[string]any) string {
	metadata, _ := repo["metadata"].(map[string]any)
	return strings.TrimSpace(stringify(metadata["archive_binary_path"]))
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
	if kind == "bundle" {
		switch serviceCode {
		case "xray-core":
			return "zip_binary"
		}
	}
	if kind == "runtime" {
		switch serviceCode {
		case "xray-core", "shadowsocks":
			return "copy_binary"
		}
	}
	return ""
}

func defaultBinaryInstallPath(serviceCode string) string {
	switch strings.TrimSpace(serviceCode) {
	case "xray-core":
		return "/usr/local/bin/xray"
	case "shadowsocks":
		return "/usr/local/bin/ss-server"
	default:
		return ""
	}
}

func validateBinaryInstallPath(serviceCode, target string) error {
	target = filepath.Clean(strings.TrimSpace(target))
	if target == "." || !filepath.IsAbs(target) {
		return errors.New("install_path must be absolute")
	}
	for _, allowed := range allowedBinaryInstallPaths(serviceCode) {
		if target == allowed {
			return nil
		}
	}
	return fmt.Errorf("%s is not allowed for %s", target, serviceCode)
}

func allowedBinaryInstallPaths(serviceCode string) []string {
	switch strings.TrimSpace(serviceCode) {
	case "xray-core":
		return []string{"/usr/local/bin/xray", "/usr/bin/xray", "/opt/xray/xray"}
	case "shadowsocks":
		return []string{"/usr/local/bin/ss-server", "/usr/bin/ss-server"}
	default:
		return nil
	}
}

func installBinaryExecutable(source, target string) error {
	target = filepath.Clean(target)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create binary directory: %w", err)
	}
	src, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open downloaded artifact: %w", err)
	}
	defer src.Close()
	tmp, err := os.CreateTemp(filepath.Dir(target), "."+filepath.Base(target)+".*")
	if err != nil {
		return fmt.Errorf("create binary temp file: %w", err)
	}
	tmpPath := tmp.Name()
	removeTmp := true
	defer func() {
		if removeTmp {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := io.Copy(tmp, src); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("copy binary: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close binary temp file: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o755); err != nil {
		return fmt.Errorf("chmod binary: %w", err)
	}
	if err := os.Chown(tmpPath, 0, 0); err != nil && os.Geteuid() == 0 {
		return fmt.Errorf("chown binary: %w", err)
	}
	if err := os.Rename(tmpPath, target); err != nil {
		return fmt.Errorf("install binary: %w", err)
	}
	removeTmp = false
	return nil
}

func extractZipBinaryArtifact(archivePath, workDir, serviceCode, preferredMember string) (string, string, error) {
	workDir = strings.TrimSpace(workDir)
	if workDir == "" {
		return "", "", errors.New("work directory is required")
	}
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", "", fmt.Errorf("open zip artifact: %w", err)
	}
	defer reader.Close()

	member, err := selectZipBinaryMember(reader.File, serviceCode, preferredMember)
	if err != nil {
		return "", "", err
	}
	if member.UncompressedSize64 == 0 {
		return "", "", fmt.Errorf("zip member %s is empty", member.Name)
	}
	if member.UncompressedSize64 > uint64(maxExtractedBinaryBytes) {
		return "", "", fmt.Errorf("zip member %s exceeds maximum extracted size", member.Name)
	}

	in, err := member.Open()
	if err != nil {
		return "", "", fmt.Errorf("open zip member %s: %w", member.Name, err)
	}
	defer in.Close()

	outPath := filepath.Join(workDir, "extracted-"+safeExtractedBinaryName(path.Base(normalizeArchiveMemberPath(member.Name))))
	out, err := os.OpenFile(outPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o700)
	if err != nil {
		return "", "", fmt.Errorf("create extracted binary: %w", err)
	}
	limited := &io.LimitedReader{R: in, N: maxExtractedBinaryBytes + 1}
	written, copyErr := io.Copy(out, limited)
	closeErr := out.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(outPath)
		if copyErr != nil {
			return "", "", fmt.Errorf("extract zip member %s: %w", member.Name, copyErr)
		}
		return "", "", fmt.Errorf("close extracted binary: %w", closeErr)
	}
	if written > maxExtractedBinaryBytes {
		_ = os.Remove(outPath)
		return "", "", fmt.Errorf("zip member %s exceeds maximum extracted size", member.Name)
	}
	if written == 0 {
		_ = os.Remove(outPath)
		return "", "", fmt.Errorf("zip member %s is empty", member.Name)
	}
	if err := os.Chmod(outPath, 0o700); err != nil {
		_ = os.Remove(outPath)
		return "", "", fmt.Errorf("chmod extracted binary: %w", err)
	}
	return outPath, normalizeArchiveMemberPath(member.Name), nil
}

func selectZipBinaryMember(files []*zip.File, serviceCode, preferredMember string) (*zip.File, error) {
	preferred := normalizeArchiveMemberPath(preferredMember)
	if preferred != "" && !safeArchiveMemberPath(preferred) {
		return nil, fmt.Errorf("archive_binary_path %q is not safe", preferredMember)
	}
	var selected *zip.File
	selectedRank := 1 << 30
	for _, file := range files {
		name := normalizeArchiveMemberPath(file.Name)
		if !usableZipFileMember(file, name) {
			continue
		}
		if preferred != "" {
			if name == preferred || (!strings.Contains(preferred, "/") && path.Base(name) == preferred) {
				return file, nil
			}
			continue
		}
		for _, candidate := range defaultArchiveBinaryCandidates(serviceCode) {
			rank := archiveCandidateRank(name, candidate)
			if rank < selectedRank {
				selected = file
				selectedRank = rank
			}
		}
	}
	if preferred != "" {
		return nil, fmt.Errorf("archive_binary_path %q was not found in zip artifact", preferredMember)
	}
	if selected == nil {
		return nil, fmt.Errorf("zip artifact does not contain a known executable for %s; set archive_binary_path metadata", serviceCode)
	}
	return selected, nil
}

func usableZipFileMember(file *zip.File, name string) bool {
	if file == nil || file.FileInfo().IsDir() {
		return false
	}
	return safeArchiveMemberPath(name)
}

func normalizeArchiveMemberPath(value string) string {
	value = filepath.ToSlash(strings.TrimSpace(value))
	for strings.HasPrefix(value, "./") {
		value = strings.TrimPrefix(value, "./")
	}
	return value
}

func safeArchiveMemberPath(value string) bool {
	value = normalizeArchiveMemberPath(value)
	return value != "" &&
		value != "." &&
		value != ".." &&
		!strings.HasPrefix(value, "/") &&
		!strings.HasPrefix(value, "../") &&
		!strings.Contains(value, "/../") &&
		!strings.Contains(value, "\\") &&
		!strings.Contains(value, "\x00")
}

func defaultArchiveBinaryCandidates(serviceCode string) []string {
	switch strings.TrimSpace(serviceCode) {
	case "xray-core":
		return []string{"xray"}
	case "shadowsocks":
		return []string{"ss-server"}
	default:
		return nil
	}
}

func archiveCandidateRank(member, candidate string) int {
	if candidate == "" {
		return 1 << 30
	}
	if member == candidate {
		return 0
	}
	if path.Base(member) == candidate {
		return strings.Count(member, "/") + 1
	}
	return 1 << 30
}

func safeExtractedBinaryName(value string) string {
	value = strings.TrimSpace(value)
	var b strings.Builder
	for _, r := range value {
		allowed := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-'
		if allowed {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), ".-_")
	if out == "" {
		return "binary"
	}
	return out
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
