package http

import (
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
	authCtx, ok := authFromRequest(r)
	if !ok {
		writeErr(w, 401, "authentication required")
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
		writeErr(w, 409, err.Error())
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

func (s *Server) revokePlatformSession(w http.ResponseWriter, r *http.Request) {
	if err := s.store.RevokeUserSession(r.Context(), idParam(r)); err != nil {
		writeErr(w, 500, "session revoke failed")
		return
	}
	writeJSON(w, 200, response{"status": "ok"})
}

func (s *Server) updatePlatformUserStatus(w http.ResponseWriter, r *http.Request) {
	authCtx, ok := authFromRequest(r)
	if !ok {
		writeErr(w, 401, "authentication required")
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
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, x)
}

func (s *Server) resetPlatformUserPassword(w http.ResponseWriter, r *http.Request) {
	authCtx, ok := authFromRequest(r)
	if !ok {
		writeErr(w, 401, "authentication required")
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
		writeErr(w, 400, err.Error())
		return
	}
	if err := s.store.RevokeUserSessionsByUser(r.Context(), userID); err != nil {
		writeErr(w, 500, "session revoke failed")
		return
	}
	_, _ = s.store.CreateAuditForUser(r.Context(), &authCtx.User.ID, "auth.user.reset_password", "platform_user", &userID, "platform user password reset")
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
		writeErr(w, 409, err.Error())
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
