package http

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	authn "github.com/rtis-emc2/megavpn/internal/auth"
	"github.com/rtis-emc2/megavpn/internal/domain"
)

type platformUserCreateRequest struct {
	Username    string   `json:"username"`
	Email       string   `json:"email"`
	DisplayName string   `json:"display_name"`
	Password    string   `json:"password"`
	RoleCodes   []string `json:"role_codes"`
}

type platformUserStatusRequest struct {
	Status string `json:"status"`
}

type platformUserUpdateRequest struct {
	Email       string   `json:"email"`
	DisplayName string   `json:"display_name"`
	RoleCodes   []string `json:"role_codes"`
}

type platformUserPasswordResetRequest struct {
	Password string `json:"password"`
}

func (s *Server) listPlatformUsers(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	x, err := s.store.ListPlatformUsers(r.Context(), limit)
	if err != nil {
		writeErr(w, 500, "list platform users failed")
		return
	}
	if x == nil {
		x = []domain.PlatformUserRecord{}
	}
	writeJSON(w, 200, x)
}

func (s *Server) createPlatformUser(w http.ResponseWriter, r *http.Request) {
	authCtx, ok := requireSuperadmin(w, r)
	if !ok {
		return
	}
	var req platformUserCreateRequest
	if !decode(r, &req) || strings.TrimSpace(req.Username) == "" || req.Password == "" {
		writeErr(w, 400, "invalid platform user payload")
		return
	}
	hash, err := authn.HashPassword(req.Password)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	x, err := s.store.CreatePlatformUser(r.Context(), req.Username, req.Email, req.DisplayName, hash, req.RoleCodes, &authCtx.User.ID)
	if err != nil {
		s.writePlatformUserMutationError(w, "platform.user.create", err, "platform user creation failed")
		return
	}
	writeJSON(w, 201, x)
}

func (s *Server) listUserSessions(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	x, err := s.store.ListUserSessions(r.Context(), limit)
	if err != nil {
		writeErr(w, 500, "list user sessions failed")
		return
	}
	if x == nil {
		x = []domain.UserSessionRecord{}
	}
	writeJSON(w, 200, x)
}

func (s *Server) updatePlatformUser(w http.ResponseWriter, r *http.Request) {
	authCtx, ok := requireSuperadmin(w, r)
	if !ok {
		return
	}
	var req platformUserUpdateRequest
	if !decode(r, &req) || strings.TrimSpace(req.Email) == "" || strings.TrimSpace(req.DisplayName) == "" || len(req.RoleCodes) == 0 {
		writeErr(w, 400, "email, display_name and role_codes are required")
		return
	}
	x, err := s.store.UpdatePlatformUser(r.Context(), idParam(r), req.Email, req.DisplayName, req.RoleCodes, &authCtx.User.ID)
	if err != nil {
		s.writePlatformUserMutationError(w, "platform.user.update", err, "platform user update failed")
		return
	}
	writeJSON(w, 200, x)
}

func (s *Server) revokePlatformSession(w http.ResponseWriter, r *http.Request) {
	authCtx, ok := requireSuperadmin(w, r)
	if !ok {
		return
	}
	sessionID := idParam(r)
	if strings.TrimSpace(sessionID) == strings.TrimSpace(authCtx.Session.ID) {
		writeErr(w, 409, "use logout to revoke the current session")
		return
	}
	if err := s.store.RevokeUserSession(r.Context(), sessionID); err != nil {
		if errors.Is(err, domain.ErrUserSessionNotFound) {
			writeErr(w, 404, domain.ErrUserSessionNotFound.Error())
			return
		}
		s.logPersistenceFailure("platform.session.revoke", err, "session_id", sessionID)
		writeErr(w, 500, "session revoke failed")
		return
	}
	s.auditBestEffort(r.Context(), &authCtx.User.ID, "auth.session.revoke", "user_session", &sessionID, "platform session revoked")
	writeJSON(w, 200, response{"status": "ok"})
}

func (s *Server) revokeAllPlatformSessions(w http.ResponseWriter, r *http.Request) {
	authCtx, ok := requireSuperadmin(w, r)
	if !ok {
		return
	}
	count, err := s.store.RevokeAllUserSessions(r.Context(), authCtx.Session.ID)
	if err != nil {
		writeErr(w, 500, "session revoke failed")
		return
	}
	s.auditBestEffort(r.Context(), &authCtx.User.ID, "auth.sessions.revoke_all", "user_session", nil, "all other platform sessions revoked")
	writeJSON(w, 200, response{"status": "ok", "revoked": count, "current_session_preserved": true})
}

func (s *Server) updatePlatformUserStatus(w http.ResponseWriter, r *http.Request) {
	authCtx, ok := requireSuperadmin(w, r)
	if !ok {
		return
	}
	var req platformUserStatusRequest
	if !decode(r, &req) {
		writeErr(w, 400, "invalid platform user status payload")
		return
	}
	status := strings.TrimSpace(req.Status)
	if status != "active" && status != "disabled" && status != "locked" {
		writeErr(w, 400, "invalid platform user status")
		return
	}
	x, err := s.store.UpdatePlatformUserStatus(r.Context(), idParam(r), status, &authCtx.User.ID)
	if err != nil {
		s.writePlatformUserMutationError(w, "platform.user.status", err, "platform user status update failed")
		return
	}
	writeJSON(w, 200, x)
}

func (s *Server) resetPlatformUserPassword(w http.ResponseWriter, r *http.Request) {
	authCtx, ok := requireSuperadmin(w, r)
	if !ok {
		return
	}
	var req platformUserPasswordResetRequest
	if !decode(r, &req) || strings.TrimSpace(req.Password) == "" {
		writeErr(w, 400, "invalid platform user password reset payload")
		return
	}
	passwordHash, err := authn.HashPassword(req.Password)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	userID := idParam(r)
	if err := s.store.UpdatePlatformUserPassword(r.Context(), userID, passwordHash, &authCtx.User.ID); err != nil {
		s.writePlatformUserMutationError(w, "platform.user.password", err, "platform user password reset failed")
		return
	}
	s.auditBestEffort(r.Context(), &authCtx.User.ID, "auth.user.reset_password", "platform_user", &userID, "platform user password reset")
	writeJSON(w, 200, response{"status": "ok", "sessions_revoked": true})
}

func (s *Server) deletePlatformUser(w http.ResponseWriter, r *http.Request) {
	authCtx, ok := authFromRequest(r)
	if !ok {
		writeErr(w, 401, "authentication required")
		return
	}
	if !platformUserHasRole(authCtx.RoleCodes, "superadmin") {
		writeErr(w, 403, "superadmin role is required")
		return
	}
	userID := idParam(r)
	if strings.TrimSpace(userID) == strings.TrimSpace(authCtx.User.ID) {
		writeErr(w, 409, "cannot delete the current operator")
		return
	}
	if err := s.store.DeletePlatformUser(r.Context(), userID, &authCtx.User.ID); err != nil {
		s.writePlatformUserMutationError(w, "platform.user.delete", err, "platform user deletion failed")
		return
	}
	writeJSON(w, 200, response{"status": "ok", "deleted_user_id": userID})
}

func platformUserHasRole(roleCodes []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, roleCode := range roleCodes {
		if strings.TrimSpace(roleCode) == target {
			return true
		}
	}
	return false
}

func (s *Server) writePlatformUserMutationError(w http.ResponseWriter, operation string, err error, fallback string) {
	switch {
	case errors.Is(err, domain.ErrPlatformUserNotFound):
		writeErr(w, http.StatusNotFound, domain.ErrPlatformUserNotFound.Error())
	case errors.Is(err, domain.ErrPlatformUserConflict):
		writeErr(w, http.StatusConflict, domain.ErrPlatformUserConflict.Error())
	case errors.Is(err, domain.ErrUnknownPlatformRole):
		writeErr(w, http.StatusBadRequest, "one or more platform roles are not available")
	case errors.Is(err, domain.ErrLastActiveSuperadmin),
		errors.Is(err, domain.ErrLastSuperadminRole),
		errors.Is(err, domain.ErrLastSuperadmin),
		errors.Is(err, domain.ErrCurrentOperatorDelete):
		writeErr(w, http.StatusConflict, err.Error())
	default:
		s.logPersistenceFailure(operation, err)
		writeErr(w, http.StatusInternalServerError, fallback)
	}
}

func requireSuperadmin(w http.ResponseWriter, r *http.Request) (domain.AuthContext, bool) {
	authCtx, ok := authFromRequest(r)
	if !ok {
		writeErr(w, 401, "authentication required")
		return domain.AuthContext{}, false
	}
	if !platformUserHasRole(authCtx.RoleCodes, "superadmin") {
		writeErr(w, 403, "superadmin role is required")
		return domain.AuthContext{}, false
	}
	return authCtx, true
}
