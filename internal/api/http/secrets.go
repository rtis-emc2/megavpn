package http

import (
	"net/http"
	"strings"
)

func (s *Server) createSecretRef(w http.ResponseWriter, r *http.Request) {
	var req secretRefCreateRequest
	if !decode(r, &req) || strings.TrimSpace(req.SecretType) == "" || req.Value == "" {
		writeErr(w, 400, "invalid secret ref payload")
		return
	}
	ref, err := s.store.CreateSecretRef(r.Context(), req.SecretType, []byte(req.Value), req.Meta)
	if err != nil {
		switch {
		case strings.Contains(err.Error(), "not configured"):
			writeErr(w, 503, "secret storage is not configured")
		case strings.Contains(err.Error(), "required"), strings.Contains(err.Error(), "unsupported"):
			writeErr(w, 400, err.Error())
		default:
			writeErr(w, 500, "secret ref create failed")
		}
		return
	}
	writeJSON(w, 201, ref)
}
