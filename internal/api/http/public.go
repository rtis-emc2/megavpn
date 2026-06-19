package http

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

const maxArtifactPreviewBytes int64 = 512 * 1024

func (s *Server) publicShareDownload(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.PathValue("token"))
	if token == "" {
		writeErr(w, 400, "share token is required")
		return
	}
	_, artifact, err := s.store.ResolveShareLinkArtifact(r.Context(), token)
	if err != nil {
		writeErr(w, 404, err.Error())
		return
	}
	if err := s.serveArtifactDownload(w, r, artifact); err != nil {
		writeErr(w, artifactHTTPStatus(err), err.Error())
	}
}

func (s *Server) clientArtifactDownload(w http.ResponseWriter, r *http.Request) {
	artifact, err := s.store.GetArtifact(r.Context(), idParam(r), strings.TrimSpace(r.PathValue("artifact_id")))
	if err != nil {
		writeErr(w, 404, err.Error())
		return
	}
	if err := s.serveArtifactDownload(w, r, artifact); err != nil {
		writeErr(w, artifactHTTPStatus(err), err.Error())
	}
}

func (s *Server) clientArtifactContent(w http.ResponseWriter, r *http.Request) {
	artifact, err := s.store.GetArtifact(r.Context(), idParam(r), strings.TrimSpace(r.PathValue("artifact_id")))
	if err != nil {
		writeErr(w, 404, err.Error())
		return
	}
	if !isPreviewableArtifactType(artifact.ArtifactType) {
		writeErr(w, 415, "artifact type is not previewable; download the file instead")
		return
	}
	path, filename, info, err := s.resolveArtifactFile(artifact)
	if err != nil {
		writeErr(w, artifactHTTPStatus(err), err.Error())
		return
	}
	if info.Size() > maxArtifactPreviewBytes {
		writeErr(w, 413, "artifact is too large for inline preview; download the file instead")
		return
	}
	f, err := os.Open(path)
	if err != nil {
		writeErr(w, 404, "artifact file not found")
		return
	}
	defer f.Close()
	content, err := io.ReadAll(io.LimitReader(f, maxArtifactPreviewBytes+1))
	if err != nil {
		writeErr(w, 500, "read artifact failed")
		return
	}
	if int64(len(content)) > maxArtifactPreviewBytes {
		writeErr(w, 413, "artifact is too large for inline preview; download the file instead")
		return
	}
	writeJSON(w, 200, response{
		"id":                artifact.ID,
		"client_account_id": artifact.ClientAccountID,
		"service_access_id": artifact.ServiceAccessID,
		"artifact_type":     artifact.ArtifactType,
		"filename":          filename,
		"size_bytes":        info.Size(),
		"content":           string(content),
	})
}

func (s *Server) serveArtifactDownload(w http.ResponseWriter, r *http.Request, artifact domain.Artifact) error {
	path, filename, _, err := s.resolveArtifactFile(artifact)
	if err != nil {
		return err
	}
	contentType := mime.TypeByExtension(filepath.Ext(filename))
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	http.ServeFile(w, r, path)
	return nil
}

func (s *Server) resolveArtifactFile(artifact domain.Artifact) (string, string, os.FileInfo, error) {
	if strings.TrimSpace(artifact.Status) != "ready" {
		return "", "", nil, fmt.Errorf("artifact is not ready")
	}
	path := strings.TrimSpace(artifact.StoragePath)
	if path == "" {
		return "", "", nil, fmt.Errorf("artifact storage path is empty")
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return "", "", nil, fmt.Errorf("artifact file not found")
	}
	if err := s.validateArtifactPath(path); err != nil {
		return "", "", nil, err
	}
	return path, filepath.Base(path), info, nil
}

func (s *Server) validateArtifactPath(path string) error {
	root := strings.TrimSpace(s.artifactRoot)
	if root == "" {
		return nil
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("artifact root is invalid")
	}
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("artifact path is invalid")
	}
	rootReal, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		rootReal = rootAbs
	}
	pathReal, err := filepath.EvalSymlinks(pathAbs)
	if err != nil {
		return fmt.Errorf("artifact file not found")
	}
	rel, err := filepath.Rel(rootReal, pathReal)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("artifact path is outside artifact root")
	}
	return nil
}

func isPreviewableArtifactType(artifactType string) bool {
	switch strings.TrimSpace(artifactType) {
	case "ovpn", "vless_url", "wg_conf", "mtproto_url", "http_proxy_bundle", "ss_url", "ipsec_bundle":
		return true
	default:
		return false
	}
}

func artifactHTTPStatus(err error) int {
	if err == nil {
		return 500
	}
	switch {
	case strings.Contains(err.Error(), "outside artifact root"):
		return 403
	case strings.Contains(err.Error(), "not ready"):
		return 409
	case strings.Contains(err.Error(), "not found"),
		strings.Contains(err.Error(), "empty"):
		return 404
	default:
		return 500
	}
}
