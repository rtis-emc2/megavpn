package http

import (
	nethttp "net/http"
	"strconv"
	"strings"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

func (s *Server) listInstanceVLESSGroupMembers(w nethttp.ResponseWriter, r *nethttp.Request) {
	out, err := s.store.ListInstanceVLESSGroupMembers(r.Context(), idParam(r))
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 200, out)
}

func (s *Server) listInstanceVLESSGroupMemberClients(w nethttp.ResponseWriter, r *nethttp.Request) {
	out, err := s.store.ListInstanceVLESSGroupMemberClients(
		r.Context(),
		idParam(r),
		strings.TrimSpace(r.PathValue("group_key")),
		strings.TrimSpace(r.URL.Query().Get("search")),
		strings.TrimSpace(r.URL.Query().Get("status")),
		boundedQueryInt(r, "limit", 50, 1, 500),
		boundedQueryInt(r, "offset", 0, 0, 100000),
	)
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 200, out)
}

func (s *Server) listInstanceVLESSGroupAvailableClients(w nethttp.ResponseWriter, r *nethttp.Request) {
	assignment := strings.TrimSpace(r.URL.Query().Get("assignment"))
	if assignment == "" {
		assignment = "unassigned"
	}
	out, err := s.store.ListInstanceVLESSGroupAvailableClients(
		r.Context(),
		idParam(r),
		strings.TrimSpace(r.URL.Query().Get("search")),
		assignment,
		boundedQueryInt(r, "limit", 50, 1, 500),
		boundedQueryInt(r, "offset", 0, 0, 100000),
	)
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 200, out)
}

func (s *Server) addInstanceVLESSGroupMembers(w nethttp.ResponseWriter, r *nethttp.Request) {
	var req domain.VLESSGroupMembershipRequest
	if !decode(r, &req) {
		writeErr(w, 400, "invalid vless group membership payload")
		return
	}
	out, err := s.store.AddInstanceVLESSGroupMembers(r.Context(), idParam(r), strings.TrimSpace(r.PathValue("group_key")), req)
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 202, out)
}

func (s *Server) moveInstanceVLESSGroupMember(w nethttp.ResponseWriter, r *nethttp.Request) {
	var req struct {
		GroupKey string `json:"group_key"`
	}
	if !decode(r, &req) || strings.TrimSpace(req.GroupKey) == "" {
		writeErr(w, 400, "group_key is required")
		return
	}
	out, err := s.store.MoveInstanceVLESSGroupMember(r.Context(), idParam(r), strings.TrimSpace(r.PathValue("client_id")), req.GroupKey)
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 202, out)
}

func (s *Server) removeInstanceVLESSGroupMember(w nethttp.ResponseWriter, r *nethttp.Request) {
	out, err := s.store.RemoveInstanceVLESSGroupMember(r.Context(), idParam(r), strings.TrimSpace(r.PathValue("client_id")))
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 202, out)
}

func boundedQueryInt(r *nethttp.Request, key string, def, min, max int) int {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return def
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
