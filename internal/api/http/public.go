package http

import (
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

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
	path := strings.TrimSpace(artifact.StoragePath)
	if path == "" {
		writeErr(w, 404, "artifact storage path is empty")
		return
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		writeErr(w, 404, "artifact file not found")
		return
	}
	filename := filepath.Base(path)
	contentType := mime.TypeByExtension(filepath.Ext(filename))
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	http.ServeFile(w, r, path)
}
