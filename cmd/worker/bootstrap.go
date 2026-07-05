package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	neturl "net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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

const sshKnownHostsPath = "/var/lib/megavpn/ssh/known_hosts"

var (
	bootstrapEnvKeyPattern           = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
	bootstrapSSHUserPattern          = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]{0,63}$`)
	bootstrapSSHHostPattern          = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*\.?$`)
	bootstrapSSHHostKeySHA256Pattern = regexp.MustCompile(`^SHA256:[A-Za-z0-9+/]{32,64}={0,2}$`)
)

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
	agentEnv, bootstrapEnv, err := renderBootstrapEnvFiles(cfg, publicBaseURL, node, token.Token)
	if err != nil {
		return "failed", map[string]any{"error": err.Error(), "node_id": nodeID, "stage": "render_bootstrap_env"}
	}

	if bootstrapMode == "manual_bundle" {
		bootstrapRef, err := store.CreateSecretRef(ctx, "opaque", []byte(bootstrapEnv), map[string]any{
			"scope":          "node_bootstrap",
			"node_id":        nodeID,
			"bootstrap_mode": bootstrapMode,
			"material":       "agent_bootstrap_env",
		})
		if err != nil {
			if errors.Is(err, postgres.ErrSecretServiceUnavailable) {
				return "failed", map[string]any{"error": "manual bootstrap bundle requires MEGAVPN_MASTER_KEY_PATH", "node_id": nodeID}
			}
			return "failed", map[string]any{"error": err.Error(), "node_id": nodeID, "stage": "store_manual_bootstrap_bundle"}
		}
		return "succeeded", map[string]any{
			"message":                          "manual bootstrap bundle generated",
			"node_id":                          nodeID,
			"bootstrap_mode":                   bootstrapMode,
			"reinstall_agent":                  reinstallAgent,
			"force_reenroll":                   forceReenroll,
			"control_plane_url":                publicBaseURL,
			"enrollment_hint":                  token.TokenHint,
			"agent_env":                        agentEnv,
			"agent_bootstrapenv_secret_ref_id": bootstrapRef.ID,
			"agent_bootstrapenv_available":     true,
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
	if err := session.run(ctx, "cleanup remote tmp", remoteCommand(method, fmt.Sprintf("rm -rf %s", shellQuote(remoteTmpDir)))); err != nil {
		return "failed", map[string]any{"error": err.Error(), "node_id": nodeID, "stage": "remote_tmp_cleanup"}
	}
	if err := session.run(ctx, "mkdir remote tmp", fmt.Sprintf("install -d -m 0700 %s", shellQuote(remoteTmpDir))); err != nil {
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

func renderBootstrapEnvFiles(cfg config.Config, publicBaseURL string, node domain.Node, enrollmentToken string) (string, string, error) {
	agentEnv, err := renderEnvFile([]envEntry{
		{key: "MEGAVPN_AGENT_CONTROL_PLANE_URL", value: publicBaseURL},
		{key: "MEGAVPN_AGENT_STATE_PATH", value: "/var/lib/megavpn/agent/state.json"},
		{key: "MEGAVPN_AGENT_BOOTSTRAP_PATH", value: "/etc/megavpn/agent-bootstrap.env"},
		{key: "MEGAVPN_AGENT_POLL_INTERVAL", value: cfg.Agent.PollInterval.String()},
	})
	if err != nil {
		return "", "", err
	}
	bootstrapEnv, err := renderEnvFile([]envEntry{
		{key: "MEGAVPN_AGENT_NODE_ID", value: node.ID},
		{key: "MEGAVPN_AGENT_NODE_NAME", value: node.Name},
		{key: "MEGAVPN_AGENT_NODE_ADDRESS", value: firstNonEmpty(strings.TrimSpace(node.Address), strings.TrimSpace(node.Name))},
		{key: "MEGAVPN_AGENT_CONTROL_PLANE_URL", value: publicBaseURL},
		{key: "MEGAVPN_AGENT_ENROLLMENT_TOKEN", value: enrollmentToken},
	})
	if err != nil {
		return "", "", err
	}
	return agentEnv, bootstrapEnv, nil
}

type envEntry struct {
	key   string
	value string
}

func renderEnvFile(entries []envEntry) (string, error) {
	var b strings.Builder
	for _, entry := range entries {
		if !bootstrapEnvKeyPattern.MatchString(entry.key) {
			return "", fmt.Errorf("invalid bootstrap env key %q", entry.key)
		}
		if err := domain.ValidateSingleLine("bootstrap env "+entry.key, entry.value); err != nil {
			return "", err
		}
		b.WriteString(entry.key)
		b.WriteByte('=')
		b.WriteString(entry.value)
		b.WriteByte('\n')
	}
	return b.String(), nil
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
	if err := validateSSHBootstrapTarget(method); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(sshKnownHostsPath), 0o700); err != nil {
		return nil, err
	}
	session := &sshSession{method: method, secret: secret}
	switch strings.TrimSpace(method.AuthType) {
	case "", "ssh_key":
		keyMaterial, err := normalizeSSHPrivateKey(secret)
		if err != nil {
			return nil, err
		}
		if keyMaterial == "" {
			return nil, errors.New("ssh private key is empty")
		}
		keyFile, err := os.CreateTemp("", "megavpn-ssh-key-*")
		if err != nil {
			return nil, err
		}
		if _, err := keyFile.WriteString(keyMaterial); err != nil {
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
	if err := ensurePinnedKnownHost(method); err != nil {
		session.Close()
		return nil, err
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
	s.applySecretEnv(cmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s failed: %w: %s", step, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (s *sshSession) copy(ctx context.Context, localPath, remotePath string) error {
	prog, args := s.commandArgs("scp", localPath, s.target()+":"+remotePath)
	cmd := exec.CommandContext(ctx, prog, args...)
	s.applySecretEnv(cmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("scp failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (s *sshSession) applySecretEnv(cmd *exec.Cmd) {
	if strings.TrimSpace(s.method.AuthType) == "password" {
		cmd.Env = append(os.Environ(), "SSHPASS="+s.secret)
	}
}

func (s *sshSession) commandArgs(tool string, extra ...string) (string, []string) {
	base := []string{
		"-o", "StrictHostKeyChecking=yes",
		"-o", "UserKnownHostsFile=" + sshKnownHostsPath,
		"-o", "GlobalKnownHostsFile=/dev/null",
		"-o", "ConnectTimeout=10",
		"-o", "HostKeyAlgorithms=ssh-ed25519,ecdsa-sha2-nistp256,rsa-sha2-512,rsa-sha2-256",
		"-P", fmt.Sprintf("%d", s.method.SSHPort),
	}
	switch strings.TrimSpace(s.method.AuthType) {
	case "", "ssh_key":
		base = append(base, "-i", s.keyPath, "-o", "IdentitiesOnly=yes", "-o", "BatchMode=yes")
		if tool == "ssh" {
			return "ssh", append([]string{
				"-o", "StrictHostKeyChecking=yes",
				"-o", "UserKnownHostsFile=" + sshKnownHostsPath,
				"-o", "GlobalKnownHostsFile=/dev/null",
				"-o", "ConnectTimeout=10",
				"-o", "HostKeyAlgorithms=ssh-ed25519,ecdsa-sha2-nistp256,rsa-sha2-512,rsa-sha2-256",
				"-p", fmt.Sprintf("%d", s.method.SSHPort),
				"-i", s.keyPath,
				"-o", "IdentitiesOnly=yes",
				"-o", "BatchMode=yes",
				"--",
				s.target(),
			}, extra...)
		}
		return "scp", append(append(base, "--"), extra...)
	case "password":
		if tool == "ssh" {
			return "sshpass", append([]string{"-e", "ssh",
				"-o", "StrictHostKeyChecking=yes",
				"-o", "UserKnownHostsFile=" + sshKnownHostsPath,
				"-o", "GlobalKnownHostsFile=/dev/null",
				"-o", "ConnectTimeout=10",
				"-o", "HostKeyAlgorithms=ssh-ed25519,ecdsa-sha2-nistp256,rsa-sha2-512,rsa-sha2-256",
				"-p", fmt.Sprintf("%d", s.method.SSHPort),
				"-o", "PreferredAuthentications=password",
				"-o", "PubkeyAuthentication=no",
				"--",
				s.target(),
			}, extra...)
		}
		return "sshpass", append([]string{"-e", "scp"}, append(append(base, "--"), extra...)...)
	default:
		return tool, extra
	}
}

func validateSSHBootstrapTarget(method domain.NodeAccessMethod) error {
	user := strings.TrimSpace(method.SSHUser)
	host := strings.TrimSpace(method.SSHHost)
	pin := strings.TrimSpace(method.SSHHostKeySHA256)
	if !bootstrapSSHUserPattern.MatchString(user) {
		return errors.New("ssh_user contains unsafe characters")
	}
	if !isSafeSSHBootstrapHost(host) {
		return errors.New("ssh_host contains unsafe characters")
	}
	if !bootstrapSSHHostKeySHA256Pattern.MatchString(pin) {
		return errors.New("ssh_host_key_sha256 is required for ssh bootstrap")
	}
	return nil
}

func isSafeSSHBootstrapHost(host string) bool {
	host = strings.TrimSpace(host)
	if host == "" || strings.HasPrefix(host, "-") || strings.ContainsAny(host, " \t\r\n;{}") {
		return false
	}
	ipLiteral := host
	if strings.HasPrefix(host, "[") || strings.HasSuffix(host, "]") {
		if !strings.HasPrefix(host, "[") || !strings.HasSuffix(host, "]") {
			return false
		}
		ipLiteral = strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")
	}
	if _, err := netip.ParseAddr(ipLiteral); err == nil {
		return true
	}
	return bootstrapSSHHostPattern.MatchString(host)
}

func ensurePinnedKnownHost(method domain.NodeAccessMethod) error {
	host := strings.Trim(strings.TrimSpace(method.SSHHost), "[]")
	pin := strings.TrimSpace(method.SSHHostKeySHA256)
	port := method.SSHPort
	if port == 0 {
		port = 22
	}
	scanCmd := exec.Command("ssh-keyscan", "-p", fmt.Sprintf("%d", port), "-T", "10", host)
	scanOut, err := scanCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ssh host key scan failed: %w: %s", err, strings.TrimSpace(string(scanOut)))
	}
	lines := strings.TrimSpace(string(scanOut))
	if lines == "" {
		return errors.New("ssh host key scan returned no keys")
	}
	tmp, err := os.CreateTemp("", "megavpn-known-host-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.WriteString(lines + "\n"); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	fpOut, err := exec.Command("ssh-keygen", "-lf", tmpPath).CombinedOutput()
	if err != nil {
		return fmt.Errorf("ssh host key fingerprint failed: %w: %s", err, strings.TrimSpace(string(fpOut)))
	}
	if !knownHostFingerprintMatches(string(fpOut), pin) {
		return fmt.Errorf("ssh host key fingerprint mismatch for %s", method.SSHHost)
	}
	if err := os.MkdirAll(filepath.Dir(sshKnownHostsPath), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(sshKnownHostsPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(lines + "\n")
	return err
}

func knownHostFingerprintMatches(output, pin string) bool {
	pin = strings.TrimSpace(pin)
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == pin {
			return true
		}
	}
	return false
}

func (s *sshSession) target() string {
	return strings.TrimSpace(s.method.SSHUser) + "@" + strings.TrimSpace(s.method.SSHHost)
}

func normalizeSSHPrivateKey(secret string) (string, error) {
	value := strings.TrimSpace(secret)
	if value == "" {
		return "", nil
	}
	if !strings.Contains(value, "\n") && strings.Contains(value, `\n`) {
		value = strings.ReplaceAll(value, `\n`, "\n")
	}
	switch {
	case strings.HasPrefix(value, "ssh-rsa "),
		strings.HasPrefix(value, "ssh-ed25519 "),
		strings.HasPrefix(value, "ecdsa-sha2-"),
		strings.HasPrefix(value, "sk-ssh-"):
		return "", errors.New("ssh access method contains a public key; paste the private key block instead")
	}
	if !strings.Contains(value, "PRIVATE KEY") {
		return "", errors.New("ssh private key must be a PEM/OpenSSH private key block")
	}
	return value + "\n", nil
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
