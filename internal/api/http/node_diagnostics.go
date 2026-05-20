package http

import nethttp "net/http"

func (s *Server) getNodeDiagnostics(w nethttp.ResponseWriter, r *nethttp.Request) {
	x, err := s.store.GetNodeDiagnostics(r.Context(), idParam(r))
	if err != nil {
		writeErr(w, 404, "node diagnostics not found")
		return
	}
	writeJSON(w, 200, x)
}
