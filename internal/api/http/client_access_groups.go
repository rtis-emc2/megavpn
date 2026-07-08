package http

import (
	"errors"
	nethttp "net/http"
	"strings"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

func (s *Server) listClientAccessGroups(w nethttp.ResponseWriter, r *nethttp.Request) {
	out, err := s.store.ListClientAccessGroups(r.Context(), strings.TrimSpace(r.URL.Query().Get("service_code")))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, out)
}

func (s *Server) getClientAccessGroup(w nethttp.ResponseWriter, r *nethttp.Request) {
	out, err := s.store.GetClientAccessGroup(r.Context(), strings.TrimSpace(r.PathValue("group_id")))
	if errors.Is(err, domain.ErrClientAccessGroupNotFound) {
		writeErr(w, 404, "client access group not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, out)
}

func (s *Server) createClientAccessGroup(w nethttp.ResponseWriter, r *nethttp.Request) {
	var input domain.ClientAccessGroupInput
	if !decode(r, &input) {
		writeErr(w, 400, "invalid client access group payload")
		return
	}
	var userID *string
	if authCtx, ok := authFromRequest(r); ok {
		userID = &authCtx.User.ID
	}
	out, err := s.store.CreateClientAccessGroup(r.Context(), input, userID)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 201, out)
}

func (s *Server) updateClientAccessGroup(w nethttp.ResponseWriter, r *nethttp.Request) {
	var input domain.ClientAccessGroupInput
	if !decode(r, &input) {
		writeErr(w, 400, "invalid client access group payload")
		return
	}
	var userID *string
	if authCtx, ok := authFromRequest(r); ok {
		userID = &authCtx.User.ID
	}
	out, err := s.store.UpdateClientAccessGroup(r.Context(), strings.TrimSpace(r.PathValue("group_id")), input, userID)
	if errors.Is(err, domain.ErrClientAccessGroupNotFound) {
		writeErr(w, 404, "client access group not found")
		return
	}
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, out)
}

func (s *Server) setClientAccessGroupStatus(status string) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		var userID *string
		if authCtx, ok := authFromRequest(r); ok {
			userID = &authCtx.User.ID
		}
		out, err := s.store.SetClientAccessGroupStatus(r.Context(), strings.TrimSpace(r.PathValue("group_id")), status, userID)
		if errors.Is(err, domain.ErrClientAccessGroupNotFound) {
			writeErr(w, 404, "client access group not found")
			return
		}
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, out)
	}
}

func (s *Server) deleteClientAccessGroup(w nethttp.ResponseWriter, r *nethttp.Request) {
	var userID *string
	if authCtx, ok := authFromRequest(r); ok {
		userID = &authCtx.User.ID
	}
	out, err := s.store.DeleteClientAccessGroupSoft(r.Context(), strings.TrimSpace(r.PathValue("group_id")), userID)
	if errors.Is(err, domain.ErrClientAccessGroupNotFound) {
		writeErr(w, 404, "client access group not found")
		return
	}
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, out)
}

func (s *Server) listClientAccessGroupAvailableClients(w nethttp.ResponseWriter, r *nethttp.Request) {
	assignment := strings.TrimSpace(r.URL.Query().Get("assignment"))
	if assignment == "" {
		assignment = "unassigned"
	}
	out, err := s.store.ListClientAccessGroupAvailableClients(
		r.Context(),
		strings.TrimSpace(r.URL.Query().Get("service_code")),
		strings.TrimSpace(r.URL.Query().Get("search")),
		assignment,
		strings.TrimSpace(r.URL.Query().Get("status")),
		boundedQueryInt(r, "limit", 50, 1, 500),
		boundedQueryInt(r, "offset", 0, 0, 1000000),
	)
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 200, out)
}

func (s *Server) listClientAccessGroupMembers(w nethttp.ResponseWriter, r *nethttp.Request) {
	out, err := s.store.ListClientAccessGroupMembers(
		r.Context(),
		strings.TrimSpace(r.PathValue("group_id")),
		strings.TrimSpace(r.URL.Query().Get("search")),
		strings.TrimSpace(r.URL.Query().Get("status")),
		boundedQueryInt(r, "limit", 50, 1, 500),
		boundedQueryInt(r, "offset", 0, 0, 1000000),
	)
	if errors.Is(err, domain.ErrClientAccessGroupNotFound) {
		writeErr(w, 404, "client access group not found")
		return
	}
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 200, out)
}

func (s *Server) previewClientAccessGroupMembers(w nethttp.ResponseWriter, r *nethttp.Request) {
	req, ok := decodeClientAccessGroupMembershipRequest(w, r)
	if !ok {
		return
	}
	out, err := s.store.PreviewClientAccessGroupMembers(r.Context(), strings.TrimSpace(r.PathValue("group_id")), req)
	writeClientAccessGroupMembershipResponse(w, out, err, 200)
}

func (s *Server) bulkAddClientAccessGroupMembers(w nethttp.ResponseWriter, r *nethttp.Request) {
	req, ok := decodeClientAccessGroupMembershipRequest(w, r)
	if !ok {
		return
	}
	out, err := s.store.BulkAddClientAccessGroupMembers(r.Context(), strings.TrimSpace(r.PathValue("group_id")), req)
	writeClientAccessGroupMembershipResponse(w, out, err, 202)
}

func (s *Server) bulkMoveClientAccessGroupMembers(w nethttp.ResponseWriter, r *nethttp.Request) {
	req, ok := decodeClientAccessGroupMembershipRequest(w, r)
	if !ok {
		return
	}
	out, err := s.store.BulkMoveClientAccessGroupMembers(r.Context(), strings.TrimSpace(r.PathValue("group_id")), req)
	writeClientAccessGroupMembershipResponse(w, out, err, 202)
}

func (s *Server) removeClientAccessGroupMember(w nethttp.ResponseWriter, r *nethttp.Request) {
	out, err := s.store.RemoveClientAccessGroupMember(
		r.Context(),
		strings.TrimSpace(r.PathValue("group_id")),
		strings.TrimSpace(r.PathValue("client_id")),
	)
	writeClientAccessGroupMembershipResponse(w, out, err, 202)
}

func decodeClientAccessGroupMembershipRequest(w nethttp.ResponseWriter, r *nethttp.Request) (domain.ClientAccessGroupMembershipRequest, bool) {
	var req domain.ClientAccessGroupMembershipRequest
	if !decode(r, &req) {
		writeErr(w, 400, "invalid client access group membership payload")
		return req, false
	}
	return req, true
}

func writeClientAccessGroupMembershipResponse(w nethttp.ResponseWriter, out domain.ClientAccessGroupMembershipResult, err error, successStatus int) {
	if errors.Is(err, domain.ErrClientAccessGroupNotFound) {
		writeErr(w, 404, "client access group not found")
		return
	}
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, successStatus, out)
}

func (s *Server) getClientAccessGroupScope(w nethttp.ResponseWriter, r *nethttp.Request) {
	out, err := s.store.GetClientAccessGroupScope(r.Context(), strings.TrimSpace(r.PathValue("group_id")))
	if errors.Is(err, domain.ErrClientAccessGroupNotFound) {
		writeErr(w, 404, "client access group not found")
		return
	}
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 200, out)
}

func (s *Server) updateClientAccessGroupScope(w nethttp.ResponseWriter, r *nethttp.Request) {
	var req domain.ClientAccessGroupScope
	if !decode(r, &req) {
		writeErr(w, 400, "invalid client access group scope payload")
		return
	}
	var userID *string
	if authCtx, ok := authFromRequest(r); ok {
		userID = &authCtx.User.ID
	}
	out, err := s.store.UpdateClientAccessGroupScope(r.Context(), strings.TrimSpace(r.PathValue("group_id")), req, userID)
	if errors.Is(err, domain.ErrClientAccessGroupNotFound) {
		writeErr(w, 404, "client access group not found")
		return
	}
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 200, out)
}

func (s *Server) previewClientAccessGroupSync(w nethttp.ResponseWriter, r *nethttp.Request) {
	out, err := s.store.PreviewClientAccessGroupSync(r.Context(), strings.TrimSpace(r.PathValue("group_id")))
	if errors.Is(err, domain.ErrClientAccessGroupNotFound) {
		writeErr(w, 404, "client access group not found")
		return
	}
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 200, out)
}

func (s *Server) applyClientAccessGroupSync(w nethttp.ResponseWriter, r *nethttp.Request) {
	out, err := s.store.ApplyClientAccessGroupSync(r.Context(), strings.TrimSpace(r.PathValue("group_id")))
	writeClientAccessGroupMembershipResponse(w, out, err, 202)
}

func (s *Server) listClientAccessGroupSyncState(w nethttp.ResponseWriter, r *nethttp.Request) {
	out, err := s.store.ListClientAccessGroupSyncState(r.Context(), strings.TrimSpace(r.PathValue("group_id")))
	if errors.Is(err, domain.ErrClientAccessGroupNotFound) {
		writeErr(w, 404, "client access group not found")
		return
	}
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 200, out)
}

func (s *Server) listClientAccessGroupMigrationConflicts(w nethttp.ResponseWriter, r *nethttp.Request) {
	out, err := s.store.ListClientAccessGroupMigrationConflicts(r.Context(), boundedQueryInt(r, "limit", 200, 1, 1000))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, out)
}

func (s *Server) clientAccessGroups(w nethttp.ResponseWriter, r *nethttp.Request) {
	out, err := s.store.ListClientAccessGroupsForClient(r.Context(), idParam(r))
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 200, out)
}

func (s *Server) setClientAccessGroupForClient(w nethttp.ResponseWriter, r *nethttp.Request) {
	var req struct {
		GroupID string `json:"group_id"`
	}
	if !decode(r, &req) || strings.TrimSpace(req.GroupID) == "" {
		writeErr(w, 400, "group_id is required")
		return
	}
	var userID *string
	if authCtx, ok := authFromRequest(r); ok {
		userID = &authCtx.User.ID
	}
	out, err := s.store.SetClientAccessGroupForClient(
		r.Context(),
		idParam(r),
		strings.TrimSpace(r.PathValue("service_code")),
		strings.TrimSpace(req.GroupID),
		userID,
	)
	writeClientAccessGroupMembershipResponse(w, out, err, 202)
}
