package http

import (
	"context"
	"fmt"
	"net"
	nethttp "net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type runtimePreflightResponse struct {
	Status         string                  `json:"status"`
	Version        string                  `json:"version"`
	ProductionMode bool                    `json:"production_mode"`
	GeneratedAt    time.Time               `json:"generated_at"`
	Checks         []runtimePreflightCheck `json:"checks"`
}

type runtimePreflightCheck struct {
	Code    string `json:"code"`
	Status  string `json:"status"`
	Summary string `json:"summary"`
	Detail  string `json:"detail,omitempty"`
}

func (s *Server) runtimePreflight(w nethttp.ResponseWriter, r *nethttp.Request) {
	checks := s.runtimePreflightChecks(r.Context())
	writeJSON(w, 200, runtimePreflightResponse{
		Status:         runtimePreflightStatus(checks),
		Version:        s.version,
		ProductionMode: s.productionMode,
		GeneratedAt:    time.Now().UTC(),
		Checks:         checks,
	})
}

func (s *Server) runtimePreflightChecks(ctx context.Context) []runtimePreflightCheck {
	return []runtimePreflightCheck{
		s.databasePreflightCheck(ctx),
		s.schemaMigrationPreflightCheck(ctx),
		s.secretStoragePreflightCheck(),
		s.publicBaseURLPreflightCheck(),
		s.controlPlaneTLSPreflightCheck(ctx),
		s.webRootPreflightCheck(),
		s.artifactStoragePreflightCheck(),
		s.proxyHeadersPreflightCheck(),
		s.requestLimitPreflightCheck(),
		s.agentRegistrationPreflightCheck(),
	}
}

func (s *Server) databasePreflightCheck(ctx context.Context) runtimePreflightCheck {
	if err := s.store.Ping(ctx); err != nil {
		return failedPreflight("database", "database is not reachable", "database ping failed")
	}
	return okPreflight("database", "database is reachable", "")
}

func (s *Server) schemaMigrationPreflightCheck(ctx context.Context) runtimePreflightCheck {
	latest, applied, err := s.store.SchemaMigrationStatus(ctx)
	if err != nil {
		return failedPreflight("schema_migrations", "schema migration status is unavailable", "schema_migrations table lookup failed")
	}
	if applied == 0 {
		return failedPreflight("schema_migrations", "no schema migrations are applied", "run cmd/migrate before starting production traffic")
	}
	return okPreflight("schema_migrations", "schema migrations are applied", fmt.Sprintf("%d applied, latest=%s", applied, latest))
}

func (s *Server) secretStoragePreflightCheck() runtimePreflightCheck {
	if s.secretStorageReady {
		return okPreflight("secret_storage", "secret storage is configured", "master key was loaded during API startup")
	}
	return failedPreflight("secret_storage", "secret storage is not configured", "set MEGAVPN_MASTER_KEY_PATH before using bootstrap bundles and token rotation")
}

func (s *Server) publicBaseURLPreflightCheck() runtimePreflightCheck {
	raw := strings.TrimSpace(s.publicBaseURL)
	if raw == "" {
		return warningPreflight("public_base_url", "public base URL is not configured", "share links, generated URLs and operator instructions may be incomplete")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return failedPreflight("public_base_url", "public base URL is invalid", "expected an absolute http or https URL")
	}
	if parsed.Scheme != "https" {
		if s.productionMode {
			return failedPreflight("public_base_url", "public base URL is not HTTPS", "production control-plane traffic must be exposed over HTTPS")
		}
		return warningPreflight("public_base_url", "public base URL is not HTTPS", "production control-plane traffic should be exposed over HTTPS")
	}
	if isLocalControlPlaneHost(parsed.Hostname()) {
		if s.productionMode {
			return failedPreflight("public_base_url", "public base URL points to a local address", "production agents and operators need a routable public HTTPS endpoint")
		}
		return warningPreflight("public_base_url", "public base URL points to a local address", "external clients and share links may not be reachable")
	}
	return okPreflight("public_base_url", "public base URL is production-shaped", raw)
}

func (s *Server) controlPlaneTLSPreflightCheck(ctx context.Context) runtimePreflightCheck {
	settings, err := s.store.GetControlPlaneTLSSettings(ctx)
	if err != nil {
		return failedPreflight("control_plane_tls", "control-plane TLS settings are unavailable", "settings lookup failed")
	}
	if !settings.Enabled {
		if s.productionMode && s.externalControlPlaneTLSAssumed() {
			return okPreflight("control_plane_tls", "external control-plane TLS edge is configured", "managed TLS is disabled; HTTPS public URL, loopback API listener and trusted proxy headers indicate external TLS termination")
		}
		return warningPreflight("control_plane_tls", "managed control-plane TLS is disabled", "this is acceptable only when TLS terminates at a trusted external reverse proxy")
	}
	if strings.TrimSpace(settings.LastError) != "" {
		return failedPreflight("control_plane_tls", "last control-plane TLS apply failed", settings.LastError)
	}
	mode := strings.TrimSpace(settings.Mode)
	if mode == "" {
		mode = "unspecified"
	}
	return okPreflight("control_plane_tls", "managed control-plane TLS is enabled", fmt.Sprintf("mode=%s", mode))
}

func (s *Server) webRootPreflightCheck() runtimePreflightCheck {
	if strings.TrimSpace(s.webRoot) == "" {
		return failedPreflight("web_root", "web UI root is not resolved", "API fallback page will be served instead of the control-plane UI")
	}
	indexPath := filepath.Join(s.webRoot, "index.html")
	if info, err := os.Stat(indexPath); err != nil || info.IsDir() {
		return failedPreflight("web_root", "web UI index is missing", indexPath)
	}
	return okPreflight("web_root", "web UI root is available", s.webRoot)
}

func (s *Server) artifactStoragePreflightCheck() runtimePreflightCheck {
	root := strings.TrimSpace(s.artifactRoot)
	if root == "" {
		return failedPreflight("artifact_storage", "artifact storage root is not configured", "set MEGAVPN_ARTIFACT_ROOT")
	}
	root = filepath.Clean(root)
	if info, err := os.Stat(root); err == nil {
		if !info.IsDir() {
			return failedPreflight("artifact_storage", "artifact storage root is not a directory", root)
		}
		return okPreflight("artifact_storage", "artifact storage root exists", root)
	}
	parent := filepath.Dir(root)
	if info, err := os.Stat(parent); err == nil && info.IsDir() {
		if s.productionMode {
			return failedPreflight("artifact_storage", "artifact storage root does not exist", "create "+root+" on persistent storage before enabling production mode")
		}
		return warningPreflight("artifact_storage", "artifact storage root does not exist yet", "worker will create "+root+" on first artifact build")
	}
	return failedPreflight("artifact_storage", "artifact storage parent directory is missing", parent)
}

func (s *Server) proxyHeadersPreflightCheck() runtimePreflightCheck {
	if s.trustProxyHeaders {
		if s.productionMode {
			if isLoopbackListenAddress(s.listenAddr) {
				return okPreflight("trusted_proxy_headers", "trusted proxy headers are enabled behind a loopback API listener", "edge proxy must strip inbound forwarded headers before proxying to the API")
			}
			return failedPreflight("trusted_proxy_headers", "trusted proxy headers are enabled on a non-loopback API listener", "bind MEGAVPN_API_LISTEN_ADDR to 127.0.0.1 or disable MEGAVPN_TRUST_PROXY_HEADERS")
		}
		return warningPreflight("trusted_proxy_headers", "trusted proxy headers are enabled", "only run this behind a controlled reverse proxy that strips untrusted forwarded headers")
	}
	return okPreflight("trusted_proxy_headers", "trusted proxy headers are disabled", "")
}

func (s *Server) requestLimitPreflightCheck() runtimePreflightCheck {
	if s.maxRequestBytes <= 0 {
		if s.productionMode {
			return failedPreflight("request_body_limit", "request body limit is disabled", "set MEGAVPN_API_MAX_REQUEST_BYTES for predictable memory pressure boundaries")
		}
		return warningPreflight("request_body_limit", "request body limit is disabled", "set MEGAVPN_API_MAX_REQUEST_BYTES for predictable memory pressure boundaries")
	}
	return okPreflight("request_body_limit", "request body limit is configured", fmt.Sprintf("%d bytes", s.maxRequestBytes))
}

func (s *Server) agentRegistrationPreflightCheck() runtimePreflightCheck {
	if s.allowAutoRegister {
		if s.productionMode {
			return failedPreflight("agent_auto_register", "agent auto-registration is enabled", "disable shared-token auto-registration and use per-node enrollment tokens in production")
		}
		return warningPreflight("agent_auto_register", "agent auto-registration is enabled", "prefer enrollment-token based registration in production")
	}
	return okPreflight("agent_auto_register", "agent auto-registration is disabled", "")
}

func runtimePreflightStatus(checks []runtimePreflightCheck) string {
	status := "ready"
	for _, check := range checks {
		if check.Status == "failed" {
			return "blocked"
		}
		if check.Status == "warning" {
			status = "degraded"
		}
	}
	return status
}

func runtimePreflightIsReady(checks []runtimePreflightCheck) bool {
	return runtimePreflightStatus(checks) == "ready"
}

func okPreflight(code, summary, detail string) runtimePreflightCheck {
	return runtimePreflightCheck{Code: code, Status: "ok", Summary: summary, Detail: detail}
}

func warningPreflight(code, summary, detail string) runtimePreflightCheck {
	return runtimePreflightCheck{Code: code, Status: "warning", Summary: summary, Detail: detail}
}

func failedPreflight(code, summary, detail string) runtimePreflightCheck {
	return runtimePreflightCheck{Code: code, Status: "failed", Summary: summary, Detail: detail}
}

func isLocalControlPlaneHost(host string) bool {
	host = strings.Trim(strings.ToLower(strings.TrimSpace(host)), "[]")
	if host == "" {
		return false
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsLoopback() || ip.IsUnspecified() || ip.IsPrivate())
}

func (s *Server) externalControlPlaneTLSAssumed() bool {
	raw := strings.TrimSpace(s.publicBaseURL)
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return false
	}
	return s.trustProxyHeaders && isLoopbackListenAddress(s.listenAddr)
}

func isLoopbackListenAddress(addr string) bool {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return false
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = strings.Trim(addr, "[]")
	}
	host = strings.Trim(strings.ToLower(strings.TrimSpace(host)), "[]")
	if host == "" {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
