package main

import (
	"context"
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/infra/postgres"
	"github.com/rtis-emc2/megavpn/internal/pki"
)

const (
	controlPlaneTLSMaterialDir = "/etc/megavpn/control-plane-tls"
	controlPlaneNginxConfPath  = "/etc/nginx/conf.d/megavpn-control-plane.conf"
)

var controlPlaneTLSServerNamePattern = regexp.MustCompile(`^(\*\.)?[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*\.?$`)

func handleControlPlaneTLSApplyJob(ctx context.Context, store *postgres.Store, job domain.Job) (string, map[string]any) {
	settings, err := store.GetControlPlaneTLSSettings(ctx)
	if err != nil {
		return controlPlaneTLSApplyFailed(ctx, store, "load_settings", err)
	}
	if !settings.Enabled {
		return controlPlaneTLSApplyFailed(ctx, store, "validate_settings", fmt.Errorf("control plane tls profile is disabled"))
	}
	if err := validateControlPlaneTLSApplySettings(settings); err != nil {
		return controlPlaneTLSApplyFailed(ctx, store, "validate_settings", err)
	}

	fullchainPEM, keyPEM, certSource, err := resolveControlPlaneTLSMaterial(ctx, store, settings)
	if err != nil {
		return controlPlaneTLSApplyFailed(ctx, store, "resolve_certificate", err)
	}
	if err := os.MkdirAll(controlPlaneTLSMaterialDir, 0o700); err != nil {
		return controlPlaneTLSApplyFailed(ctx, store, "prepare_material_dir", err)
	}
	certPath := filepath.Join(controlPlaneTLSMaterialDir, "fullchain.pem")
	keyPath := filepath.Join(controlPlaneTLSMaterialDir, "privkey.pem")
	if err := writeFileAtomic(certPath, fullchainPEM, 0o600); err != nil {
		return controlPlaneTLSApplyFailed(ctx, store, "write_certificate", err)
	}
	if err := writeFileAtomic(keyPath, keyPEM, 0o600); err != nil {
		return controlPlaneTLSApplyFailed(ctx, store, "write_private_key", err)
	}

	conf := renderControlPlaneNginxConfig(settings, certPath, keyPath)
	previousConf, hadPreviousConf, _ := readOptionalFile(controlPlaneNginxConfPath)
	if err := writeFileAtomic(controlPlaneNginxConfPath, []byte(conf), 0o644); err != nil {
		return controlPlaneTLSApplyFailed(ctx, store, "write_nginx_config", err)
	}
	if out, err := runCommand(ctx, "nginx", "-t"); err != nil {
		restoreControlPlaneNginxConfig(previousConf, hadPreviousConf)
		return controlPlaneTLSApplyFailed(ctx, store, "nginx_test", fmt.Errorf("%w: %s", err, out))
	}
	if out, err := reloadNginx(ctx); err != nil {
		return controlPlaneTLSApplyFailed(ctx, store, "nginx_reload", fmt.Errorf("%w: %s", err, out))
	}
	_ = store.MarkControlPlaneTLSApplyResult(ctx, true, "")
	return "succeeded", map[string]any{
		"message":          "control plane tls edge applied",
		"job_id":           job.ID,
		"public_base_url":  settings.PublicBaseURL,
		"server_name":      settings.ServerName,
		"listen_port":      settings.ListenPort,
		"upstream_url":     settings.UpstreamURL,
		"certificate_path": certPath,
		"key_path":         keyPath,
		"nginx_conf_path":  controlPlaneNginxConfPath,
		"certificate":      certSource,
	}
}

func controlPlaneTLSApplyFailed(ctx context.Context, store *postgres.Store, stage string, err error) (string, map[string]any) {
	errText := strings.TrimSpace(err.Error())
	_ = store.MarkControlPlaneTLSApplyResult(ctx, false, stage+": "+errText)
	return "failed", map[string]any{"stage": stage, "error": errText}
}

func validateControlPlaneTLSApplySettings(settings domain.ControlPlaneTLSSettings) error {
	if strings.TrimSpace(settings.PublicBaseURL) == "" {
		return fmt.Errorf("public_base_url is required")
	}
	u, err := url.Parse(settings.PublicBaseURL)
	if err != nil {
		return fmt.Errorf("invalid public_base_url: %w", err)
	}
	if !strings.EqualFold(u.Scheme, "https") {
		return fmt.Errorf("public_base_url must use https")
	}
	if strings.TrimSpace(settings.ServerName) == "" {
		return fmt.Errorf("server_name is required")
	}
	if err := validateControlPlaneTLSServerName(settings.ServerName); err != nil {
		return err
	}
	if settings.ListenPort <= 0 || settings.ListenPort > 65535 {
		return fmt.Errorf("listen_port must be between 1 and 65535")
	}
	return nil
}

func validateControlPlaneTLSServerName(serverName string) error {
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
	if !controlPlaneTLSServerNamePattern.MatchString(serverName) {
		return fmt.Errorf("server_name must be a DNS name, wildcard DNS name, IP literal, or _")
	}
	return nil
}

func resolveControlPlaneTLSMaterial(ctx context.Context, store *postgres.Store, settings domain.ControlPlaneTLSSettings) ([]byte, []byte, string, error) {
	if settings.Mode == "self_signed_fallback" && (settings.CertificateID == nil || strings.TrimSpace(*settings.CertificateID) == "") {
		commonName := firstNonEmpty(settings.SelfSignedCommonName, settings.ServerName)
		dnsNames := settings.SelfSignedDNSNames
		if len(dnsNames) == 0 && settings.ServerName != "" {
			dnsNames = []string{settings.ServerName}
		}
		certPEM, keyPEM, err := pki.GenerateSelfSignedCertificate(commonName, dnsNames, 365)
		return certPEM, keyPEM, "self_signed_fallback", err
	}
	if settings.CertificateID == nil || strings.TrimSpace(*settings.CertificateID) == "" {
		return nil, nil, "", fmt.Errorf("certificate_id is required")
	}
	item, certPEM, keyPEM, chainPEM, err := store.ResolvePlatformCertificateMaterial(ctx, *settings.CertificateID)
	if err != nil {
		return nil, nil, "", err
	}
	if item.Kind != "leaf" {
		return nil, nil, "", fmt.Errorf("selected certificate must be a leaf certificate")
	}
	if len(keyPEM) == 0 {
		return nil, nil, "", fmt.Errorf("selected certificate does not include a private key")
	}
	fullchain := append([]byte{}, certPEM...)
	if len(chainPEM) > 0 {
		if !strings.HasSuffix(string(fullchain), "\n") {
			fullchain = append(fullchain, '\n')
		}
		fullchain = append(fullchain, chainPEM...)
	}
	return fullchain, keyPEM, item.Source + ":" + item.ID, nil
}

func renderControlPlaneNginxConfig(settings domain.ControlPlaneTLSSettings, certPath, keyPath string) string {
	return strings.TrimSpace(fmt.Sprintf(`
map $http_upgrade $megavpn_control_plane_connection_upgrade {
    default upgrade;
    '' close;
}

server {
    listen %d ssl http2;
    server_name %s;

    ssl_certificate %s;
    ssl_certificate_key %s;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_prefer_server_ciphers off;
    ssl_session_cache shared:MEGAVPN_CONTROL_PLANE_TLS:10m;
    ssl_session_timeout 1d;

    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
    client_max_body_size 16m;

    location / {
        proxy_pass %s;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $megavpn_control_plane_connection_upgrade;
        proxy_buffering off;
        proxy_read_timeout 2h;
        proxy_send_timeout 2h;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Host $host;
        proxy_set_header X-Forwarded-Port %d;
        proxy_set_header X-Forwarded-Proto https;
    }
}
`, settings.ListenPort, settings.ServerName, certPath, keyPath, settings.UpstreamURL, settings.ListenPort)) + "\n"
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

func readOptionalFile(path string) ([]byte, bool, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	return b, err == nil, err
}

func restoreControlPlaneNginxConfig(previous []byte, hadPrevious bool) {
	if hadPrevious {
		_ = writeFileAtomic(controlPlaneNginxConfPath, previous, 0o644)
		return
	}
	_ = os.Remove(controlPlaneNginxConfPath)
}

func reloadNginx(ctx context.Context) (string, error) {
	if out, err := runCommand(ctx, "systemctl", "reload", "nginx"); err == nil {
		return out, nil
	}
	return runCommand(ctx, "nginx", "-s", "reload")
}

func runCommand(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}
