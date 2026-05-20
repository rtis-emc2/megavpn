package http

import (
	nethttp "net/http"
	"strconv"
	"time"
)

func (s *Server) rotateNodeAgentToken(w nethttp.ResponseWriter, r *nethttp.Request) {
	jobRecord, err := s.store.CreateNodeAgentTokenRotateJob(r.Context(), idParam(r))
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 202, jobRecord)
}

func (s *Server) rotateNodeEnrollmentToken(w nethttp.ResponseWriter, r *nethttp.Request) {
	ttlHours, _ := strconv.Atoi(r.URL.Query().Get("ttl_hours"))
	ttl := time.Duration(ttlHours) * time.Hour
	token, err := s.store.RotateNodeEnrollmentToken(r.Context(), idParam(r), ttl)
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 200, token)
}

func (s *Server) revokeNodeAgentIdentity(w nethttp.ResponseWriter, r *nethttp.Request) {
	node, err := s.store.RevokeNodeAgentIdentity(r.Context(), idParam(r))
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 200, node)
}
