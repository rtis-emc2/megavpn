package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	neturl "net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/infra/postgres"
	"github.com/rtis-emc2/megavpn/internal/platform/config"
)

type bootstrapLogger interface {
	Info(string, ...any)
	Error(string, ...any)
}

type sshSession struct {
	method  domain.NodeAccessMethod
	secret  string
	keyPath string
}

func handleNodeBootstrapJob(ctx context.Context, log bootstrapLogger, store *postgres.Store, cfg config.Config, job domain.Job) (string, map[string]any) {
	nodeID := strings.TrimSpace(stringify(job.Payload["node_id"]))
	if nodeID == "" && job.NodeID != nil {
		nodeID = strings.TrimSpace(*job.NodeID)
	}
	if nodeID == "" {
		return "failed", map[string]any{"error": "node_id is required"}
	}

	node, err := store.GetNode(ctx, nodeID)
	if err != nil {
		return "failed", map[string]any{"error": err.Error(), "node_id": nodeID}
	}
	methods, err := store.ListNodeAccessMethods(ctx, nodeID)
	if err != nil {
		return "failed", map[string]any{"error": err.Error(), "node_id": nodeID, "stage": "list_access_methods"}
	}
	bootstrapMode := strings.TrimSpace(stringify(job.Payload["bootstrap_mode"]))
	if bootstrapMode == "" {
		bootstrapMode = "ssh_bootstrap"
	}
	reinstallAgent := truthyLocal(job.Payload["reinstall_agent"])
	forceReenroll := truthyLocal(job.Payload["force_reenroll"])

	publicBaseURL := strings.TrimSpace(cfg.API.PublicBaseURL)
	if publicBaseURL == "" {
		publicBaseURL = "http://127.0.0.1:8080"
	}
	if err := validateControlPlaneReachability(publicBaseURL, node); err != nil {
		return "failed", map[string]any{"error": err.Error(), "node_id": nodeID, "control_plane_url": publicBaseURL}
	}

	token, err := store.CreateNodeEnrollmentToken(ctx, nodeID, 24*time.Hour)
	if err != nil {
		return "failed", map[string]any{"error": err.Error(), "node_id": nodeID, "stage": "create_enrollment_token"}
	}
	agentEnv, bootstrapEnv := renderBootstrapEnvFiles(cfg, publicBaseURL, node, token.Token)

	if bootstrapMode == "manual_bundle" {
		return "succeeded", map[string]any{
			"message":            "manual bootstrap bundle generated",
			"node_id":            nodeID,
			"bootstrap_mode":     bootstrapMode,
			"reinstall_agent":    reinstallAgent,
			"force_reenroll":     forceReenroll,
			"control_plane_url":  publicBaseURL,
			"enrollment_hint":    token.TokenHint,
			"agent_env":          agentEnv,
			"agent_bootstrapenv": bootstrapEnv,
		}
	}

	method, err := selectBootstrapMethod(methods)
	if err != nil {
		return "failed", map[string]any{"error": err.Error(), "node_id": nodeID}
	}
	secret := ""
	if method.SecretRefID != nil && strings.TrimSpace(*method.SecretRefID) != "" {
		_, rawSecret, err := store.ResolveSecretValue(ctx, *method.SecretRefID)
		if err != nil {
			if errors.Is(err, postgres.ErrSecretServiceUnavailable) {
				return "failed", map[string]any{"error": "bootstrap secret resolution requires MEGAVPN_MASTER_KEY_PATH", "node_id": nodeID}
			}
			return "failed", map[string]any{"error": err.Error(), "node_id": nodeID, "stage": "resolve_bootstrap_secret"}
		}
		secret = string(rawSecret)
	}

	session, err := newSSHSession(method, secret)
	if err != nil {
		return "failed", map[string]any{"error": err.Error(), "node_id": nodeID, "stage": "prepare_ssh"}
	}
	defer session.Close()

	agentBinary, err := resolveBootstrapAsset("megavpn-agent", []string{
		"/opt/megavpn/bin/megavpn-agent",
		"bin/megavpn-agent",
		filepath.Join(filepath.Dir(os.Args[0]), "megavpn-agent"),
		filepath.Join(filepath.Dir(os.Args[0]), "..", "bin", "megavpn-agent"),
	})
	if err != nil {
		return "failed", map[string]any{"error": err.Error(), "node_id": nodeID, "stage": "resolve_agent_binary"}
	}
	agentUnit, err := resolveBootstrapAsset("megavpn-agent.service", []string{
		"/opt/megavpn/deploy/systemd/megavpn-agent.service",
		"/etc/systemd/system/megavpn-agent.service",
		"deploy/systemd/megavpn-agent.service",
		filepath.Join(filepath.Dir(os.Args[0]), "..", "deploy", "systemd", "megavpn-agent.service"),
	})
	if err != nil {
		return "failed", map[string]any{"error": err.Error(), "node_id": nodeID, "stage": "resolve_agent_unit"}
	}

	workDir, err := os.MkdirTemp("", "megavpn-bootstrap-*")
	if err != nil {
		return "failed", map[string]any{"error": err.Error(), "node_id": nodeID, "stage": "local_tmpdir"}
	}
	defer os.RemoveAll(workDir)

	localAgentEnv := filepath.Join(workDir, "agent.env")
	localBootstrapEnv := filepath.Join(workDir, "agent-bootstrap.env")
	if err := os.WriteFile(localAgentEnv, []byte(agentEnv), 0o640); err != nil {
		return "failed", map[string]any{"error": err.Error(), "node_id": nodeID, "stage": "write_agent_env"}
	}
	if err := os.WriteFile(localBootstrapEnv, []byte(bootstrapEnv), 0o600); err != nil {
		return "failed", map[string]any{"error": err.Error(), "node_id": nodeID, "stage": "write_bootstrap_env"}
	}

	remoteTmpDir := fmt.Sprintf("/tmp/megavpn-bootstrap-%s", sanitizeRemoteToken(nodeID))
	if err := session.run(ctx, "mkdir remote tmp", remoteCommand(method, fmt.Sprintf("install -d -m 0755 %s", shellQuote(remoteTmpDir)))); err != nil {
		return "failed", map[string]any{"error": err.Error(), "node_id": nodeID, "stage": "remote_tmpdir"}
	}
	if err := session.copy(ctx, agentBinary, remoteTmpDir+"/megavpn-agent"); err != nil {
		return "failed", map[string]any{"error": err.Error(), "node_id": nodeID, "stage": "copy_agent_binary"}
	}
	if err := session.copy(ctx, agentUnit, remoteTmpDir+"/megavpn-agent.service"); err != nil {
		return "failed", map[string]any{"error": err.Error(), "node_id": nodeID, "stage": "copy_agent_unit"}
	}
	if err := session.copy(ctx, localAgentEnv, remoteTmpDir+"/agent.env"); err != nil {
		return "failed", map[string]any{"error": err.Error(), "node_id": nodeID, "stage": "copy_agent_env"}
	}
	if err := session.copy(ctx, localBootstrapEnv, remoteTmpDir+"/agent-bootstrap.env"); err != nil {
		return "failed", map[string]any{"error": err.Error(), "node_id": nodeID, "stage": "copy_bootstrap_env"}
	}

	installSteps := []string{
		"set -e",
		"install -d -m 0755 /opt/megavpn /opt/megavpn/bin /etc/megavpn /var/lib/megavpn/agent",
	}
	if reinstallAgent {
		installSteps = append(installSteps, "systemctl stop megavpn-agent.service || true")
	}
	if forceReenroll {
		installSteps = append(installSteps,
			"systemctl stop megavpn-agent.service || true",
			"rm -f /var/lib/megavpn/agent/state.json",
			"rm -f /etc/megavpn/agent-bootstrap.env",
		)
	}
	installSteps = append(installSteps,
		fmt.Sprintf("install -m 0755 %s/megavpn-agent /opt/megavpn/bin/megavpn-agent", shellQuote(remoteTmpDir)),
		fmt.Sprintf("install -m 0644 %s/megavpn-agent.service /etc/systemd/system/megavpn-agent.service", shellQuote(remoteTmpDir)),
		fmt.Sprintf("install -m 0640 %s/agent.env /etc/megavpn/agent.env", shellQuote(remoteTmpDir)),
		fmt.Sprintf("install -m 0600 %s/agent-bootstrap.env /etc/megavpn/agent-bootstrap.env", shellQuote(remoteTmpDir)),
		"chmod 0700 /var/lib/megavpn/agent",
		"systemctl daemon-reload",
		"systemctl enable --now megavpn-agent.service",
		"systemctl restart megavpn-agent.service",
		"systemctl is-active megavpn-agent.service",
		fmt.Sprintf("rm -rf %s", shellQuote(remoteTmpDir)),
	)
	installScript := strings.Join(installSteps, " && ")
	if err := session.run(ctx, "install agent runtime", remoteCommand(method, installScript)); err != nil {
		return "failed", map[string]any{"error": err.Error(), "node_id": nodeID, "stage": "install_agent_runtime"}
	}

	log.Info("node bootstrap completed", "node_id", nodeID, "host", method.SSHHost, "bootstrap_mode", bootstrapMode)
	return "succeeded", map[string]any{
		"message":           "remote agent bootstrap completed",
		"node_id":           nodeID,
		"bootstrap_mode":    bootstrapMode,
		"reinstall_agent":   reinstallAgent,
		"force_reenroll":    forceReenroll,
		"ssh_host":          method.SSHHost,
		"ssh_user":          method.SSHUser,
		"control_plane_url": publicBaseURL,
		"enrollment_hint":   token.TokenHint,
		"agent_binary":      agentBinary,
		"agent_unit":        agentUnit,
	}
}

func validateControlPlaneReachability(publicBaseURL string, node domain.Node) error {
	u, err := neturl.Parse(strings.TrimSpace(publicBaseURL))
	if err != nil {
		return fmt.Errorf("invalid public base url: %w", err)
	}
	host := strings.TrimSpace(u.Hostname())
	if host == "" {
		return errors.New("public base url hostname is required")
	}
	if isLoopbackHost(host) && !isLoopbackHost(strings.TrimSpace(node.Address)) {
		return fmt.Errorf("control plane url %q is loopback-only and cannot be used by remote node %q; set MEGAVPN_PUBLIC_BASE_URL to a reachable host", publicBaseURL, node.Name)
	}
	return nil
}

func renderBootstrapEnvFiles(cfg config.Config, publicBaseURL string, node domain.Node, enrollmentToken string) (string, string) {
	agentEnv := strings.Join([]string{
		"MEGAVPN_AGENT_CONTROL_PLANE_URL=" + publicBaseURL,
		"MEGAVPN_AGENT_STATE_PATH=/var/lib/megavpn/agent/state.json",
		"MEGAVPN_AGENT_BOOTSTRAP_PATH=/etc/megavpn/agent-bootstrap.env",
		"MEGAVPN_AGENT_POLL_INTERVAL=" + cfg.Agent.PollInterval.String(),
		"",
	}, "\n")
	bootstrapEnv := strings.Join([]string{
		"MEGAVPN_AGENT_NODE_ID=" + node.ID,
		"MEGAVPN_AGENT_NODE_NAME=" + node.Name,
		"MEGAVPN_AGENT_NODE_ADDRESS=" + firstNonEmpty(strings.TrimSpace(node.Address), strings.TrimSpace(node.Name)),
		"MEGAVPN_AGENT_CONTROL_PLANE_URL=" + publicBaseURL,
		"MEGAVPN_AGENT_ENROLLMENT_TOKEN=" + enrollmentToken,
		"",
	}, "\n")
	return agentEnv, bootstrapEnv
}

func selectBootstrapMethod(methods []domain.NodeAccessMethod) (domain.NodeAccessMethod, error) {
	for _, method := range methods {
		if method.IsEnabled && method.Method == "ssh" {
			return method, nil
		}
	}
	return domain.NodeAccessMethod{}, errors.New("enabled ssh access method is required for node bootstrap")
}

func newSSHSession(method domain.NodeAccessMethod, secret string) (*sshSession, error) {
	if strings.TrimSpace(method.SSHHost) == "" || strings.TrimSpace(method.SSHUser) == "" {
		return nil, errors.New("ssh_host and ssh_user are required")
	}
	session := &sshSession{method: method, secret: secret}
	switch strings.TrimSpace(method.AuthType) {
	case "", "ssh_key":
		if strings.TrimSpace(secret) == "" {
			return nil, errors.New("ssh private key is empty")
		}
		keyFile, err := os.CreateTemp("", "megavpn-ssh-key-*")
		if err != nil {
			return nil, err
		}
		if _, err := keyFile.WriteString(secret); err != nil {
			keyFile.Close()
			return nil, err
		}
		if err := keyFile.Close(); err != nil {
			return nil, err
		}
		if err := os.Chmod(keyFile.Name(), 0o600); err != nil {
			return nil, err
		}
		session.keyPath = keyFile.Name()
	case "password":
		if strings.TrimSpace(secret) == "" {
			return nil, errors.New("ssh password is empty")
		}
		if _, err := exec.LookPath("sshpass"); err != nil {
			return nil, errors.New("sshpass is required for password-based ssh bootstrap")
		}
	default:
		return nil, fmt.Errorf("unsupported ssh auth_type %q for bootstrap", method.AuthType)
	}
	if session.method.SSHPort == 0 {
		session.method.SSHPort = 22
	}
	return session, nil
}

func (s *sshSession) Close() {
	if s.keyPath != "" {
		_ = os.Remove(s.keyPath)
	}
}

func (s *sshSession) run(ctx context.Context, step, remoteCmd string) error {
	prog, args := s.commandArgs("ssh", remoteCmd)
	cmd := exec.CommandContext(ctx, prog, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s failed: %w: %s", step, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (s *sshSession) copy(ctx context.Context, localPath, remotePath string) error {
	prog, args := s.commandArgs("scp", localPath, s.target()+":"+remotePath)
	cmd := exec.CommandContext(ctx, prog, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("scp failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (s *sshSession) commandArgs(tool string, extra ...string) (string, []string) {
	base := []string{
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "ConnectTimeout=10",
		"-P", fmt.Sprintf("%d", s.method.SSHPort),
	}
	switch strings.TrimSpace(s.method.AuthType) {
	case "", "ssh_key":
		base = append(base, "-i", s.keyPath, "-o", "IdentitiesOnly=yes", "-o", "BatchMode=yes")
		if tool == "ssh" {
			return "ssh", append([]string{
				"-o", "StrictHostKeyChecking=no",
				"-o", "UserKnownHostsFile=/dev/null",
				"-o", "ConnectTimeout=10",
				"-p", fmt.Sprintf("%d", s.method.SSHPort),
				"-i", s.keyPath,
				"-o", "IdentitiesOnly=yes",
				"-o", "BatchMode=yes",
				s.target(),
			}, extra...)
		}
		return "scp", append(base, extra...)
	case "password":
		if tool == "ssh" {
			return "sshpass", append([]string{"-p", s.secret, "ssh",
				"-o", "StrictHostKeyChecking=no",
				"-o", "UserKnownHostsFile=/dev/null",
				"-o", "ConnectTimeout=10",
				"-p", fmt.Sprintf("%d", s.method.SSHPort),
				"-o", "PreferredAuthentications=password",
				"-o", "PubkeyAuthentication=no",
				s.target(),
			}, extra...)
		}
		return "sshpass", append([]string{"-p", s.secret, "scp"}, append(base, extra...)...)
	default:
		return tool, extra
	}
}

func (s *sshSession) target() string {
	return strings.TrimSpace(s.method.SSHUser) + "@" + strings.TrimSpace(s.method.SSHHost)
}

func resolveBootstrapAsset(label string, candidates []string) (string, error) {
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		abs := filepath.Clean(candidate)
		if info, err := os.Stat(abs); err == nil && !info.IsDir() {
			return abs, nil
		}
	}
	return "", fmt.Errorf("%s asset was not found on the worker host", label)
}

func remoteCommand(method domain.NodeAccessMethod, command string) string {
	if strings.TrimSpace(method.SSHUser) == "root" {
		return command
	}
	return "sudo -n sh -lc " + shellQuote(command)
}

func shellQuote(text string) string {
	return "'" + strings.ReplaceAll(text, "'", `'\''`) + "'"
}

func sanitizeRemoteToken(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return "node"
	}
	var b strings.Builder
	for _, r := range text {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

func isLoopbackHost(host string) bool {
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	if host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
