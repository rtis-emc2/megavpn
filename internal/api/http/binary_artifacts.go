package http

import (
	"net/http"
	"strings"

	"github.com/rtis-emc2/megavpn/internal/binaryrepo"
	"github.com/rtis-emc2/megavpn/internal/domain"
)

const maxBinaryArtifactUploadBytes int64 = 512 * 1024 * 1024

func (s *Server) listBinaryArtifacts(w http.ResponseWriter, r *http.Request) {
	includeInactive := r.URL.Query().Get("include_inactive") == "1"
	items, err := s.store.ListBinaryArtifacts(r.Context(), includeInactive)
	if err != nil {
		writeErr(w, 500, "list binary artifacts failed")
		return
	}
	if items == nil {
		items = []domain.BinaryArtifact{}
	}
	writeJSON(w, 200, items)
}

func (s *Server) createBinaryArtifact(w http.ResponseWriter, r *http.Request) {
	var req domain.BinaryArtifact
	if !decode(r, &req) {
		writeErr(w, 400, "invalid binary artifact payload")
		return
	}
	item, err := s.store.CreateBinaryArtifact(r.Context(), req)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 201, item)
}

func (s *Server) importBinaryArtifact(w http.ResponseWriter, r *http.Request) {
	root := strings.TrimSpace(s.artifactRoot)
	if root == "" {
		writeErr(w, 500, "artifact root is not configured")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBinaryArtifactUploadBytes+1024*1024)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeErr(w, 400, "invalid artifact upload form: "+err.Error())
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeErr(w, 400, "artifact file is required")
		return
	}
	defer file.Close()
	artifact, err := binaryrepo.ImportReader(root, file, binaryrepo.ImportRequest{
		SourceFilename: strings.TrimSpace(header.Filename),
		Name:           formValue(r, "name"),
		Kind:           formValue(r, "kind"),
		ServiceCode:    formValue(r, "service_code"),
		Version:        formValue(r, "version"),
		OSFamily:       formValue(r, "os_family"),
		OSVersion:      formValue(r, "os_version"),
		Architecture:   formValue(r, "architecture"),
		InstallMode:    formValue(r, "install_mode"),
		InstallPath:    formValue(r, "install_path"),
		Signature:      formValue(r, "signature"),
		StoragePath:    formValue(r, "storage_path"),
		ExpectedSHA256: formValue(r, "expected_sha256"),
		ReplaceFile:    formBool(r, "replace_file"),
		MaxBytes:       maxBinaryArtifactUploadBytes,
	})
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	item, err := s.store.CreateBinaryArtifact(r.Context(), artifact)
	if err != nil {
		binaryrepo.RemoveStoredArtifact(root, artifact)
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 201, item)
}

func formValue(r *http.Request, key string) string {
	return strings.TrimSpace(r.FormValue(key))
}

func formBool(r *http.Request, key string) bool {
	switch strings.ToLower(strings.TrimSpace(r.FormValue(key))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
