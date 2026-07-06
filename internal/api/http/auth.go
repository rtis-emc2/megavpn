package http

import (
	"context"
	"errors"
	"net"
	nethttp "net/http"
	"strings"
	"time"

	authn "github.com/rtis-emc2/megavpn/internal/auth"
	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/rbac"
)

type authContextKey struct{}

type loginRequest struct {
	Login    string `json:"login"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (s *Server) withPermission(permission string, next nethttp.Handler) nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		token := strings.TrimSpace(bearerToken(r))
		cookieAuth := false
		if token == "" && s.sessionCookieName != "" {
			if cookie, err := r.Cookie(s.sessionCookieName); err == nil {
				token = strings.TrimSpace(cookie.Value)
				cookieAuth = token != ""
			}
		}
		if token == "" {
			writeErr(w, 401, "authentication required")
			return
		}
		if cookieAuth && isUnsafeMethod(r.Method) && !hasCSRFHeader(r) {
			writeErr(w, 403, "csrf protection required")
			return
		}

		authCtx, err := s.store.ResolveAuthContext(r.Context(), authn.HashSessionToken(token))
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				writeErr(w, 503, "authentication unavailable")
				return
			}
			writeErr(w, 401, "invalid session")
			return
		}
		if !authContextHasPermission(authCtx, permission) {
			writeErr(w, 403, "forbidden")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), authContextKey{}, authCtx)))
	})
}

func authContextHasPermission(authCtx domain.AuthContext, permission string) bool {
	permission = strings.TrimSpace(permission)
	if permission == "" {
		return true
	}
	if platformUserHasRole(authCtx.RoleCodes, "superadmin") {
		return true
	}
	return rbac.HasPermission(authCtx.PermissionCodes, permission)
}

func authFromRequest(r *nethttp.Request) (domain.AuthContext, bool) {
	x, ok := r.Context().Value(authContextKey{}).(domain.AuthContext)
	return x, ok
}

func isUnsafeMethod(method string) bool {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case "", nethttp.MethodGet, nethttp.MethodHead, nethttp.MethodOptions, nethttp.MethodTrace:
		return false
	default:
		return true
	}
}

func hasCSRFHeader(r *nethttp.Request) bool {
	v := strings.ToLower(strings.TrimSpace(r.Header.Get("X-MegaVPN-CSRF")))
	return v == "1" || v == "true"
}

func (s *Server) sessionCookieSecureForRequest(r *nethttp.Request) bool {
	if !s.sessionCookieSecure {
		return false
	}
	if s.sessionCookieSecureSet {
		return true
	}
	if r.TLS != nil {
		return true
	}
	if s.trustProxyHeaders {
		proto := strings.ToLower(strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0]))
		return proto == "https"
	}
	return false
}

func (s *Server) login(w nethttp.ResponseWriter, r *nethttp.Request) {
	var req loginRequest
	if !decode(r, &req) {
		writeErr(w, 400, "invalid login payload")
		return
	}
	login := strings.TrimSpace(req.Login)
	if login == "" {
		login = strings.TrimSpace(req.Email)
	}
	if login == "" || req.Password == "" {
		writeErr(w, 400, "invalid login payload")
		return
	}

	record, err := s.store.GetPlatformUserForAuth(r.Context(), login)
	if err != nil {
		_, _ = s.store.CreateAuditForUser(r.Context(), nil, "auth.login_failed", "platform_user", nil, "operator login failed")
		writeErr(w, 401, "invalid credentials")
		return
	}
	if record.User.Status != "active" || !authn.VerifyPassword(req.Password, record.PasswordHash) {
		_, _ = s.store.CreateAuditForUser(r.Context(), &record.User.ID, "auth.login_failed", "platform_user", &record.User.ID, "operator login failed")
		writeErr(w, 401, "invalid credentials")
		return
	}

	token, tokenHash, err := authn.NewSessionToken()
	if err != nil {
		writeErr(w, 500, "session token generation failed")
		return
	}
	expiresAt := time.Now().UTC().Add(s.sessionTTL)
	sess, err := s.store.CreateUserSession(r.Context(), record.User.ID, tokenHash, s.clientIP(r), r.UserAgent(), expiresAt)
	if err != nil {
		writeErr(w, 500, "session creation failed")
		return
	}
	_ = s.store.TouchPlatformUserLogin(r.Context(), record.User.ID)
	_, _ = s.store.CreateAuditForUser(r.Context(), &record.User.ID, "auth.login", "platform_user", &record.User.ID, "operator logged in")

	nethttp.SetCookie(w, &nethttp.Cookie{
		Name:     s.sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.sessionCookieSecureForRequest(r),
		SameSite: nethttp.SameSiteLaxMode,
		Expires:  sess.ExpiresAt,
	})

	authCtx, err := s.store.ResolveAuthContext(r.Context(), tokenHash)
	if err != nil {
		writeErr(w, 500, "session resolution failed")
		return
	}
	writeJSON(w, 200, response{
		"status":      "ok",
		"user":        authCtx.User,
		"roles":       authCtx.RoleCodes,
		"permissions": authCtx.PermissionCodes,
		"session":     publicSessionResponse(authCtx.Session),
	})
}

func publicSessionResponse(session domain.UserSession) response {
	return response{
		"id":         session.ID,
		"expires_at": session.ExpiresAt,
	}
}

func (s *Server) logout(w nethttp.ResponseWriter, r *nethttp.Request) {
	authCtx, ok := authFromRequest(r)
	if !ok {
		writeErr(w, 401, "authentication required")
		return
	}
	if err := s.store.RevokeUserSession(r.Context(), authCtx.Session.ID); err != nil {
		writeErr(w, 500, "session revoke failed")
		return
	}
	_, _ = s.store.CreateAuditForUser(r.Context(), &authCtx.User.ID, "auth.logout", "platform_user", &authCtx.User.ID, "operator logged out")
	nethttp.SetCookie(w, &nethttp.Cookie{
		Name:     s.sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   s.sessionCookieSecureForRequest(r),
		SameSite: nethttp.SameSiteLaxMode,
		Expires:  time.Unix(0, 0).UTC(),
		MaxAge:   -1,
	})
	writeJSON(w, 200, response{"status": "ok"})
}

func (s *Server) me(w nethttp.ResponseWriter, r *nethttp.Request) {
	authCtx, ok := authFromRequest(r)
	if !ok {
		writeErr(w, 401, "authentication required")
		return
	}
	writeJSON(w, 200, response{
		"user":        authCtx.User,
		"roles":       authCtx.RoleCodes,
		"permissions": authCtx.PermissionCodes,
		"session":     authCtx.Session,
	})
}

func (s *Server) changePassword(w nethttp.ResponseWriter, r *nethttp.Request) {
	authCtx, ok := authFromRequest(r)
	if !ok {
		writeErr(w, 401, "authentication required")
		return
	}
	var req changePasswordRequest
	if !decode(r, &req) || strings.TrimSpace(req.CurrentPassword) == "" || strings.TrimSpace(req.NewPassword) == "" {
		writeErr(w, 400, "invalid password change payload")
		return
	}

	record, err := s.store.GetPlatformUserByIDForAuth(r.Context(), authCtx.User.ID)
	if err != nil {
		writeErr(w, 500, "current operator lookup failed")
		return
	}
	if !authn.VerifyPassword(req.CurrentPassword, record.PasswordHash) {
		writeErr(w, 401, "invalid current password")
		return
	}

	passwordHash, err := authn.HashPassword(req.NewPassword)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := s.store.UpdatePlatformUserPassword(r.Context(), authCtx.User.ID, passwordHash, &authCtx.User.ID); err != nil {
		writeErr(w, 500, "password update failed")
		return
	}
	if err := s.store.RevokeUserSessionsByUser(r.Context(), authCtx.User.ID); err != nil {
		writeErr(w, 500, "session revoke failed")
		return
	}
	_, _ = s.store.CreateAuditForUser(r.Context(), &authCtx.User.ID, "auth.password.change", "platform_user", &authCtx.User.ID, "operator changed own password")
	nethttp.SetCookie(w, &nethttp.Cookie{
		Name:     s.sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   s.sessionCookieSecureForRequest(r),
		SameSite: nethttp.SameSiteLaxMode,
		Expires:  time.Unix(0, 0).UTC(),
		MaxAge:   -1,
	})
	writeJSON(w, 200, response{"status": "ok", "relogin_required": true})
}

func (s *Server) clientIP(r *nethttp.Request) string {
	if s.trustProxyHeaders {
		if host := lastForwardedIP(r.Header.Get("X-Forwarded-For")); host != "" {
			return host
		}
		if host := validIP(r.Header.Get("X-Real-IP")); host != "" {
			return host
		}
	}
	return remoteIP(r)
}

func remoteIP(r *nethttp.Request) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func lastForwardedIP(header string) string {
	parts := strings.Split(header, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		if host := validIP(parts[i]); host != "" {
			return host
		}
	}
	return ""
}

func validIP(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if ip := net.ParseIP(value); ip != nil {
		return value
	}
	return ""
}
