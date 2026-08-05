package postgres

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/platform/id"
)

var controlPlaneServerNamePattern = regexp.MustCompile(`^(\*\.)?[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*\.?$`)

func (s *Store) GetControlPlaneTLSSettings(ctx context.Context) (domain.ControlPlaneTLSSettings, error) {
	var x domain.ControlPlaneTLSSettings
	var sansRaw []byte
	err := s.db.QueryRow(ctx, `select enabled,mode,public_base_url,server_name,listen_port,upstream_url,certificate_id,self_signed_common_name,self_signed_san_json,last_applied_at,last_error,created_at,updated_at from platform_control_plane_tls_settings where id=true`).
		Scan(&x.Enabled, &x.Mode, &x.PublicBaseURL, &x.ServerName, &x.ListenPort, &x.UpstreamURL, &x.CertificateID, &x.SelfSignedCommonName, &sansRaw, &x.LastAppliedAt, &x.LastError, &x.CreatedAt, &x.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return defaultControlPlaneTLSSettings(), nil
		}
		return x, err
	}
	if err := decodeJSONField(sansRaw, &x.SelfSignedDNSNames, "platform_control_plane_tls_settings.self_signed_san_json"); err != nil {
		return x, err
	}
	if x.SelfSignedDNSNames == nil {
		x.SelfSignedDNSNames = []string{}
	}
	return x, nil
}

func defaultControlPlaneTLSSettings() domain.ControlPlaneTLSSettings {
	return domain.ControlPlaneTLSSettings{
		Enabled:     true,
		Mode:        "managed_certificate",
		ListenPort:  443,
		UpstreamURL: "http://127.0.0.1:8080",
	}
}

func (s *Store) UpsertControlPlaneTLSSettings(ctx context.Context, x domain.ControlPlaneTLSSettings, updatedBy *string) (domain.ControlPlaneTLSSettings, error) {
	x.Mode = strings.TrimSpace(x.Mode)
	if x.Mode == "" {
		x.Mode = "managed_certificate"
	}
	if x.Mode != "managed_certificate" && x.Mode != "self_signed_fallback" {
		return domain.ControlPlaneTLSSettings{}, fmt.Errorf("unsupported control plane tls mode %q", x.Mode)
	}
	x.PublicBaseURL = strings.TrimRight(strings.TrimSpace(x.PublicBaseURL), "/")
	x.ServerName = strings.TrimSpace(x.ServerName)
	x.UpstreamURL = strings.TrimRight(strings.TrimSpace(x.UpstreamURL), "/")
	x.SelfSignedCommonName = strings.TrimSpace(x.SelfSignedCommonName)
	x.SelfSignedDNSNames = normalizeStringSlice(x.SelfSignedDNSNames)
	if x.ListenPort <= 0 {
		x.ListenPort = 443
	}
	if x.UpstreamURL == "" {
		x.UpstreamURL = "http://127.0.0.1:8080"
	}
	if x.Enabled {
		if err := validateControlPlaneTLSSettings(x); err != nil {
			return domain.ControlPlaneTLSSettings{}, err
		}
		if x.Mode == "managed_certificate" {
			if x.CertificateID == nil || strings.TrimSpace(*x.CertificateID) == "" {
				return domain.ControlPlaneTLSSettings{}, fmt.Errorf("certificate_id is required for managed certificate mode")
			}
			cert, err := s.GetPlatformCertificate(ctx, *x.CertificateID)
			if err != nil {
				return domain.ControlPlaneTLSSettings{}, err
			}
			if cert.Kind != "leaf" || cert.Status != "active" || cert.KeySecretRefID == nil || strings.TrimSpace(*cert.KeySecretRefID) == "" {
				return domain.ControlPlaneTLSSettings{}, fmt.Errorf("control plane tls certificate must be an active leaf certificate with a private key")
			}
		}
		if x.Mode == "self_signed_fallback" {
			if x.SelfSignedCommonName == "" {
				x.SelfSignedCommonName = x.ServerName
			}
			if len(x.SelfSignedDNSNames) == 0 && x.ServerName != "" {
				x.SelfSignedDNSNames = []string{x.ServerName}
			}
		}
	}
	if _, err := s.db.Exec(ctx, `insert into platform_control_plane_tls_settings(id,enabled,mode,public_base_url,server_name,listen_port,upstream_url,certificate_id,self_signed_common_name,self_signed_san_json,last_error,created_at,updated_at)
		values(true,$1,$2,$3,$4,$5,$6,$7,$8,$9,'',now(),now())
		on conflict(id) do update set
		  enabled=excluded.enabled,
		  mode=excluded.mode,
		  public_base_url=excluded.public_base_url,
		  server_name=excluded.server_name,
		  listen_port=excluded.listen_port,
		  upstream_url=excluded.upstream_url,
		  certificate_id=excluded.certificate_id,
		  self_signed_common_name=excluded.self_signed_common_name,
		  self_signed_san_json=excluded.self_signed_san_json,
		  last_error='',
		  updated_at=now()`,
		x.Enabled, x.Mode, x.PublicBaseURL, x.ServerName, x.ListenPort, x.UpstreamURL, x.CertificateID, x.SelfSignedCommonName, mustJSON(x.SelfSignedDNSNames)); err != nil {
		return domain.ControlPlaneTLSSettings{}, err
	}
	_, _ = s.CreateAuditForUser(ctx, updatedBy, "platform.control_plane_tls.update", "platform_control_plane_tls_settings", nil, "control plane tls settings updated")
	return s.GetControlPlaneTLSSettings(ctx)
}

func (s *Store) CreateControlPlaneTLSApplyJob(ctx context.Context) (domain.Job, error) {
	settings, err := s.GetControlPlaneTLSSettings(ctx)
	if err != nil {
		return domain.Job{}, err
	}
	if !settings.Enabled {
		return domain.Job{}, fmt.Errorf("control plane tls profile is disabled")
	}
	payload := map[string]any{
		"public_base_url": settings.PublicBaseURL,
		"server_name":     settings.ServerName,
		"listen_port":     settings.ListenPort,
		"mode":            settings.Mode,
	}
	job, err := s.CreateJob(ctx, domain.Job{
		ID:        id.New(),
		Type:      "platform.control_plane_tls.apply",
		ScopeType: "platform",
		Status:    "queued",
		Priority:  20,
		Payload:   payload,
	})
	if err != nil {
		return domain.Job{}, err
	}
	_, _ = s.CreateAudit(ctx, "system", "platform.control_plane_tls.apply", "platform", nil, "control plane tls apply queued")
	return job, nil
}

func (s *Store) MarkControlPlaneTLSApplyResult(ctx context.Context, success bool, errText string) error {
	if success {
		_, err := s.db.Exec(ctx, `update platform_control_plane_tls_settings set last_applied_at=now(),last_error='',updated_at=now() where id=true`)
		return err
	}
	_, err := s.db.Exec(ctx, `update platform_control_plane_tls_settings set last_error=$1,updated_at=now() where id=true`, strings.TrimSpace(errText))
	return err
}

func validateControlPlaneTLSSettings(x domain.ControlPlaneTLSSettings) error {
	if x.PublicBaseURL == "" {
		return fmt.Errorf("public_base_url is required")
	}
	u, err := url.Parse(x.PublicBaseURL)
	if err != nil {
		return fmt.Errorf("invalid public_base_url: %w", err)
	}
	if !strings.EqualFold(u.Scheme, "https") {
		return fmt.Errorf("control plane public_base_url must use https")
	}
	if u.Hostname() == "" {
		return fmt.Errorf("public_base_url host is required")
	}
	if x.ServerName == "" {
		return fmt.Errorf("server_name is required")
	}
	if err := validateControlPlaneServerName(x.ServerName); err != nil {
		return err
	}
	if x.ListenPort <= 0 || x.ListenPort > 65535 {
		return fmt.Errorf("listen_port must be between 1 and 65535")
	}
	upstream, err := url.Parse(x.UpstreamURL)
	if err != nil {
		return fmt.Errorf("invalid upstream_url: %w", err)
	}
	if !strings.EqualFold(upstream.Scheme, "http") {
		return fmt.Errorf("upstream_url must use local http behind the TLS edge")
	}
	host := strings.Trim(strings.ToLower(upstream.Hostname()), "[]")
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		return fmt.Errorf("upstream_url must point to loopback only")
	}
	return nil
}

func validateControlPlaneServerName(serverName string) error {
	serverName = strings.TrimSpace(serverName)
	if serverName == "" {
		return fmt.Errorf("server_name is required")
	}
	if strings.ContainsAny(serverName, " \t\r\n;{}") {
		return fmt.Errorf("server_name contains unsafe nginx directive characters")
	}
	if serverName == "_" {
		return nil
	}
	ipLiteral := serverName
	if strings.HasPrefix(serverName, "[") || strings.HasSuffix(serverName, "]") {
		if !strings.HasPrefix(serverName, "[") || !strings.HasSuffix(serverName, "]") {
			return fmt.Errorf("server_name must be a DNS name, wildcard DNS name, IP literal, or _")
		}
		ipLiteral = strings.TrimPrefix(strings.TrimSuffix(serverName, "]"), "[")
	}
	if _, err := netip.ParseAddr(ipLiteral); err == nil {
		return nil
	}
	if !controlPlaneServerNamePattern.MatchString(serverName) {
		return fmt.Errorf("server_name must be a DNS name, wildcard DNS name, IP literal, or _")
	}
	return nil
}

func normalizeStringSlice(items []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}
