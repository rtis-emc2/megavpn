package http

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	nethttp "net/http"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

const (
	terminalSessionTTL     = 90 * time.Second
	terminalMaxDuration    = 2 * time.Hour
	terminalMaxClientFrame = 64 * 1024
	terminalWebSocketGUID  = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	terminalHostKeyAlgos   = "ssh-ed25519,ecdsa-sha2-nistp256,rsa-sha2-512,rsa-sha2-256"
	websocketOpText        = 1
	websocketOpBinary      = 2
	websocketOpClose       = 8
	websocketOpPing        = 9
	websocketOpPong        = 10
)

var (
	terminalSSHUserPattern          = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]{0,63}$`)
	terminalSSHHostPattern          = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*\.?$`)
	terminalSSHHostKeySHA256Pattern = regexp.MustCompile(`^SHA256:[A-Za-z0-9+/]{32,64}={0,2}$`)
)

type terminalSessionTicket struct {
	ID        string
	NodeID    string
	UserID    string
	SessionID string
	ExpiresAt time.Time
}

type terminalSessionStore struct {
	mu       sync.Mutex
	sessions map[string]terminalSessionTicket
}

func newTerminalSessionStore() *terminalSessionStore {
	return &terminalSessionStore{sessions: map[string]terminalSessionTicket{}}
}

func (s *terminalSessionStore) create(nodeID, userID, sessionID string, now time.Time) (terminalSessionTicket, error) {
	token, err := randomTerminalToken()
	if err != nil {
		return terminalSessionTicket{}, err
	}
	ticket := terminalSessionTicket{
		ID:        token,
		NodeID:    nodeID,
		UserID:    userID,
		SessionID: sessionID,
		ExpiresAt: now.Add(terminalSessionTTL),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, item := range s.sessions {
		if !item.ExpiresAt.After(now) {
			delete(s.sessions, id)
		}
	}
	s.sessions[token] = ticket
	return ticket, nil
}

func (s *terminalSessionStore) consume(token string, now time.Time) (terminalSessionTicket, bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return terminalSessionTicket{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ticket, ok := s.sessions[token]
	if ok {
		delete(s.sessions, token)
	}
	if !ok || !ticket.ExpiresAt.After(now) {
		return terminalSessionTicket{}, false
	}
	return ticket, true
}

func randomTerminalToken() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func (s *Server) createNodeSSHTerminalSession(w nethttp.ResponseWriter, r *nethttp.Request) {
	authCtx, ok := authFromRequest(r)
	if !ok {
		writeErr(w, 401, "authentication required")
		return
	}
	nodeID := idParam(r)
	if _, err := s.store.GetNode(r.Context(), nodeID); err != nil {
		writeErr(w, 404, "node not found")
		return
	}
	method, err := s.enabledSSHTerminalMethod(r.Context(), nodeID)
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	if strings.TrimSpace(method.AuthType) == "token" {
		writeErr(w, 409, "token-based SSH terminal is not supported")
		return
	}
	if err := terminalRuntimePreflight(method); err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	ticket, err := s.terminalSessions.create(nodeID, authCtx.User.ID, authCtx.Session.ID, time.Now().UTC())
	if err != nil {
		writeErr(w, 500, "ssh terminal ticket creation failed")
		return
	}
	_, _ = s.store.CreateAuditForUser(r.Context(), &authCtx.User.ID, "node.ssh_terminal.ticket", "node", &nodeID, "web ssh terminal ticket created")
	writeJSON(w, 201, response{
		"session_id": ticket.ID,
		"expires_at": ticket.ExpiresAt,
		"node_id":    nodeID,
		"endpoint":   terminalEndpointSummary(method),
	})
}

func (s *Server) nodeSSHTerminal(w nethttp.ResponseWriter, r *nethttp.Request) {
	authCtx, ok := authFromRequest(r)
	if !ok {
		writeErr(w, 401, "authentication required")
		return
	}
	nodeID := idParam(r)
	ticket, ok := s.terminalSessions.consume(r.URL.Query().Get("session"), time.Now().UTC())
	if !ok || ticket.NodeID != nodeID || ticket.UserID != authCtx.User.ID || ticket.SessionID != authCtx.Session.ID {
		writeErr(w, 403, "invalid or expired ssh terminal session")
		return
	}
	ws, err := upgradeTerminalWebSocket(w, r)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	defer ws.Close()
	if err := s.runNodeSSHTerminal(r.Context(), ws, nodeID, authCtx); err != nil {
		_ = ws.WriteJSON(response{"type": "error", "message": err.Error()})
	}
}

func (s *Server) enabledSSHTerminalMethod(ctx context.Context, nodeID string) (domain.NodeAccessMethod, error) {
	methods, err := s.store.ListNodeAccessMethods(ctx, nodeID)
	if err != nil {
		return domain.NodeAccessMethod{}, errors.New("list node access methods failed")
	}
	for _, method := range methods {
		if method.IsEnabled && method.Method == "ssh" {
			if method.SecretRefID == nil || strings.TrimSpace(*method.SecretRefID) == "" {
				return domain.NodeAccessMethod{}, errors.New("enabled ssh access method is missing secret_ref_id")
			}
			if err := validateTerminalSSHTarget(method); err != nil {
				return domain.NodeAccessMethod{}, err
			}
			if method.SSHPort == 0 {
				method.SSHPort = 22
			}
			return method, nil
		}
	}
	return domain.NodeAccessMethod{}, errors.New("enabled ssh access method is not configured")
}

func terminalRuntimePreflight(method domain.NodeAccessMethod) error {
	for _, name := range []string{"ssh", "ssh-keyscan", "ssh-keygen"} {
		if _, err := exec.LookPath(name); err != nil {
			return fmt.Errorf("%s is required on the API host for web ssh terminal", name)
		}
	}
	if strings.TrimSpace(method.AuthType) == "password" {
		if _, err := exec.LookPath("sshpass"); err != nil {
			return errors.New("sshpass is required on the API host for password-based web ssh terminal")
		}
	}
	return nil
}

func terminalEndpointSummary(method domain.NodeAccessMethod) response {
	port := method.SSHPort
	if port == 0 {
		port = 22
	}
	return response{
		"ssh_host":               method.SSHHost,
		"ssh_port":               port,
		"ssh_user":               method.SSHUser,
		"auth_type":              method.AuthType,
		"ssh_host_key_sha256":    method.SSHHostKeySHA256,
		"secret_available":       method.SecretRefID != nil,
		"server_side_proxy_only": true,
	}
}

func (s *Server) runNodeSSHTerminal(parent context.Context, ws *terminalWebSocket, nodeID string, authCtx domain.AuthContext) error {
	ctx, cancel := context.WithTimeout(parent, terminalMaxDuration)
	defer cancel()

	method, err := s.enabledSSHTerminalMethod(ctx, nodeID)
	if err != nil {
		return err
	}
	_, secretValue, err := s.store.ResolveSecretValue(ctx, *method.SecretRefID)
	if err != nil {
		return errors.New("resolve ssh secret failed")
	}
	runtime, err := prepareTerminalSSHRuntime(method, string(secretValue))
	if err != nil {
		return err
	}
	defer runtime.Close()

	prog, args, env := runtime.Command()
	cmd := exec.CommandContext(ctx, prog, args...)
	cmd.Env = env
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("ssh start failed: %w", err)
	}
	_, _ = s.store.CreateAuditForUser(ctx, &authCtx.User.ID, "node.ssh_terminal.start", "node", &nodeID, "web ssh terminal started")
	_ = ws.WriteJSON(response{"type": "status", "message": "connected", "endpoint": terminalEndpointSummary(method)})

	var once sync.Once
	closeSession := func() {
		once.Do(func() {
			cancel()
			_ = stdin.Close()
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
		})
	}

	var pumps sync.WaitGroup
	pumps.Add(2)
	go func() {
		defer pumps.Done()
		pumpTerminalOutput(ws, stdout)
	}()
	go func() {
		defer pumps.Done()
		pumpTerminalOutput(ws, stderr)
	}()
	go func() {
		defer closeSession()
		readTerminalInput(ws, stdin)
	}()

	err = cmd.Wait()
	pumps.Wait()
	closeSession()
	exitPayload := response{"type": "exit", "message": "ssh session closed"}
	if err != nil {
		exitPayload["message"] = err.Error()
	}
	_ = ws.WriteJSON(exitPayload)
	auditCtx, auditCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer auditCancel()
	_, _ = s.store.CreateAuditForUser(auditCtx, &authCtx.User.ID, "node.ssh_terminal.end", "node", &nodeID, "web ssh terminal closed")
	return nil
}

func pumpTerminalOutput(ws *terminalWebSocket, r io.Reader) {
	buf := make([]byte, 8192)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			_ = ws.WriteJSON(response{"type": "output", "data": string(buf[:n])})
		}
		if err != nil {
			return
		}
	}
}

func readTerminalInput(ws *terminalWebSocket, stdin io.WriteCloser) {
	for {
		op, payload, err := ws.ReadMessage()
		if err != nil {
			return
		}
		if op != websocketOpText && op != websocketOpBinary {
			continue
		}
		var msg struct {
			Type string `json:"type"`
			Data string `json:"data"`
		}
		if err := json.Unmarshal(payload, &msg); err != nil {
			continue
		}
		switch msg.Type {
		case "input":
			_, _ = io.WriteString(stdin, msg.Data)
		case "close":
			return
		}
	}
}

type terminalSSHRuntime struct {
	method         domain.NodeAccessMethod
	tempDir        string
	keyPath        string
	knownHostsPath string
	secret         string
}

func prepareTerminalSSHRuntime(method domain.NodeAccessMethod, secret string) (*terminalSSHRuntime, error) {
	tempDir, err := os.MkdirTemp("", "megavpn-ssh-terminal-*")
	if err != nil {
		return nil, err
	}
	runtime := &terminalSSHRuntime{
		method:         method,
		tempDir:        tempDir,
		knownHostsPath: filepath.Join(tempDir, "known_hosts"),
		secret:         secret,
	}
	authType := strings.TrimSpace(method.AuthType)
	if authType == "" {
		authType = "ssh_key"
	}
	switch authType {
	case "ssh_key":
		keyMaterial, err := normalizeTerminalSSHPrivateKey(secret)
		if err != nil {
			runtime.Close()
			return nil, err
		}
		runtime.keyPath = filepath.Join(tempDir, "id_ssh")
		if err := os.WriteFile(runtime.keyPath, []byte(keyMaterial), 0o600); err != nil {
			runtime.Close()
			return nil, err
		}
	case "password":
		if strings.TrimSpace(secret) == "" {
			runtime.Close()
			return nil, errors.New("ssh password is empty")
		}
		if _, err := exec.LookPath("sshpass"); err != nil {
			runtime.Close()
			return nil, errors.New("sshpass is required on the API host for password-based web ssh terminal")
		}
	default:
		runtime.Close()
		return nil, fmt.Errorf("unsupported ssh auth_type %q for web terminal", method.AuthType)
	}
	if err := writePinnedTerminalKnownHost(method, runtime.knownHostsPath); err != nil {
		runtime.Close()
		return nil, err
	}
	return runtime, nil
}

func (r *terminalSSHRuntime) Close() {
	if r.tempDir != "" {
		_ = os.RemoveAll(r.tempDir)
	}
}

func (r *terminalSSHRuntime) Command() (string, []string, []string) {
	port := r.method.SSHPort
	if port == 0 {
		port = 22
	}
	base := []string{
		"-tt",
		"-o", "StrictHostKeyChecking=yes",
		"-o", "UserKnownHostsFile=" + r.knownHostsPath,
		"-o", "GlobalKnownHostsFile=/dev/null",
		"-o", "ConnectTimeout=10",
		"-o", "ServerAliveInterval=15",
		"-o", "ServerAliveCountMax=2",
		"-o", "LogLevel=ERROR",
		"-o", "HostKeyAlgorithms=" + terminalHostKeyAlgos,
		"-p", strconv.Itoa(port),
	}
	env := append(os.Environ(), "TERM=xterm-256color")
	switch strings.TrimSpace(r.method.AuthType) {
	case "", "ssh_key":
		base = append(base, "-i", r.keyPath, "-o", "IdentitiesOnly=yes", "-o", "BatchMode=yes")
		return "ssh", append(base, "--", terminalSSHTarget(r.method)), env
	case "password":
		env = append(env, "SSHPASS="+r.secret)
		args := append([]string{"-e", "ssh"}, base...)
		args = append(args, "-o", "PreferredAuthentications=password", "-o", "PubkeyAuthentication=no", "--", terminalSSHTarget(r.method))
		return "sshpass", args, env
	default:
		return "ssh", append(base, "--", terminalSSHTarget(r.method)), env
	}
}

func validateTerminalSSHTarget(method domain.NodeAccessMethod) error {
	user := strings.TrimSpace(method.SSHUser)
	host := strings.TrimSpace(method.SSHHost)
	pin := strings.TrimSpace(method.SSHHostKeySHA256)
	if !terminalSSHUserPattern.MatchString(user) {
		return errors.New("ssh_user contains unsafe characters")
	}
	if !isSafeTerminalSSHHost(host) {
		return errors.New("ssh_host contains unsafe characters")
	}
	if !terminalSSHHostKeySHA256Pattern.MatchString(pin) {
		return errors.New("ssh_host_key_sha256 is required for web ssh terminal")
	}
	if method.SSHPort < 0 || method.SSHPort > 65535 {
		return errors.New("ssh_port is out of range")
	}
	return nil
}

func isSafeTerminalSSHHost(host string) bool {
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
	return terminalSSHHostPattern.MatchString(host)
}

func writePinnedTerminalKnownHost(method domain.NodeAccessMethod, path string) error {
	host := strings.Trim(strings.TrimSpace(method.SSHHost), "[]")
	port := method.SSHPort
	if port == 0 {
		port = 22
	}
	scanCmd := exec.Command("ssh-keyscan", "-p", strconv.Itoa(port), "-T", "10", host)
	scanOut, err := scanCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ssh host key scan failed: %w: %s", err, strings.TrimSpace(string(scanOut)))
	}
	lines := strings.TrimSpace(string(scanOut))
	if lines == "" {
		return errors.New("ssh host key scan returned no keys")
	}
	tmpPath := path + ".scan"
	if err := os.WriteFile(tmpPath, []byte(lines+"\n"), 0o600); err != nil {
		return err
	}
	defer os.Remove(tmpPath)
	fpOut, err := exec.Command("ssh-keygen", "-lf", tmpPath, "-E", "sha256").CombinedOutput()
	if err != nil {
		return fmt.Errorf("ssh host key fingerprint failed: %w: %s", err, strings.TrimSpace(string(fpOut)))
	}
	if !terminalKnownHostFingerprintMatches(string(fpOut), method.SSHHostKeySHA256) {
		return fmt.Errorf("ssh host key fingerprint mismatch for %s", method.SSHHost)
	}
	return os.WriteFile(path, []byte(lines+"\n"), 0o600)
}

func terminalKnownHostFingerprintMatches(output, pin string) bool {
	pin = strings.TrimSpace(pin)
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == pin {
			return true
		}
	}
	return false
}

func terminalSSHTarget(method domain.NodeAccessMethod) string {
	return strings.TrimSpace(method.SSHUser) + "@" + strings.TrimSpace(method.SSHHost)
}

func normalizeTerminalSSHPrivateKey(secret string) (string, error) {
	value := strings.TrimSpace(secret)
	if value == "" {
		return "", errors.New("ssh private key is empty")
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

type terminalWebSocket struct {
	conn net.Conn
	r    *bufio.Reader
	mu   sync.Mutex
}

func upgradeTerminalWebSocket(w nethttp.ResponseWriter, r *nethttp.Request) (*terminalWebSocket, error) {
	if !headerTokenContains(r.Header.Get("Connection"), "upgrade") || !strings.EqualFold(strings.TrimSpace(r.Header.Get("Upgrade")), "websocket") {
		return nil, errors.New("websocket upgrade required")
	}
	if strings.TrimSpace(r.Header.Get("Sec-WebSocket-Version")) != "13" {
		return nil, errors.New("unsupported websocket version")
	}
	key := strings.TrimSpace(r.Header.Get("Sec-WebSocket-Key"))
	if key == "" {
		return nil, errors.New("missing websocket key")
	}
	conn, rw, err := nethttp.NewResponseController(w).Hijack()
	if err != nil {
		return nil, err
	}
	accept := websocketAcceptKey(key)
	_, err = fmt.Fprintf(rw, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", accept)
	if err != nil {
		conn.Close()
		return nil, err
	}
	if err := rw.Flush(); err != nil {
		conn.Close()
		return nil, err
	}
	return &terminalWebSocket{conn: conn, r: rw.Reader}, nil
}

func headerTokenContains(value, token string) bool {
	token = strings.ToLower(strings.TrimSpace(token))
	for _, part := range strings.Split(value, ",") {
		if strings.ToLower(strings.TrimSpace(part)) == token {
			return true
		}
	}
	return false
}

func websocketAcceptKey(key string) string {
	sum := sha1.Sum([]byte(key + terminalWebSocketGUID))
	return base64.StdEncoding.EncodeToString(sum[:])
}

func (ws *terminalWebSocket) Close() error {
	if ws == nil || ws.conn == nil {
		return nil
	}
	_ = ws.WriteClose()
	return ws.conn.Close()
}

func (ws *terminalWebSocket) ReadMessage() (int, []byte, error) {
	header := make([]byte, 2)
	if _, err := io.ReadFull(ws.r, header); err != nil {
		return 0, nil, err
	}
	op := int(header[0] & 0x0f)
	masked := header[1]&0x80 != 0
	length := uint64(header[1] & 0x7f)
	switch length {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(ws.r, ext[:]); err != nil {
			return 0, nil, err
		}
		length = uint64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err := io.ReadFull(ws.r, ext[:]); err != nil {
			return 0, nil, err
		}
		length = binary.BigEndian.Uint64(ext[:])
	}
	if length > terminalMaxClientFrame {
		return 0, nil, errors.New("websocket frame too large")
	}
	var mask [4]byte
	if masked {
		if _, err := io.ReadFull(ws.r, mask[:]); err != nil {
			return 0, nil, err
		}
	} else if op != websocketOpClose {
		return 0, nil, errors.New("client websocket frames must be masked")
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(ws.r, payload); err != nil {
		return 0, nil, err
	}
	if masked {
		for i := range payload {
			payload[i] ^= mask[i%4]
		}
	}
	switch op {
	case websocketOpClose:
		return op, payload, io.EOF
	case websocketOpPing:
		_ = ws.WriteFrame(websocketOpPong, payload)
		return ws.ReadMessage()
	default:
		return op, payload, nil
	}
}

func (ws *terminalWebSocket) WriteJSON(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return ws.WriteFrame(websocketOpText, data)
}

func (ws *terminalWebSocket) WriteClose() error {
	return ws.WriteFrame(websocketOpClose, nil)
}

func (ws *terminalWebSocket) WriteFrame(op int, payload []byte) error {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	header := []byte{0x80 | byte(op)}
	switch {
	case len(payload) < 126:
		header = append(header, byte(len(payload)))
	case len(payload) <= 0xffff:
		header = append(header, 126, byte(len(payload)>>8), byte(len(payload)))
	default:
		var ext [8]byte
		binary.BigEndian.PutUint64(ext[:], uint64(len(payload)))
		header = append(header, 127)
		header = append(header, ext[:]...)
	}
	if _, err := ws.conn.Write(header); err != nil {
		return err
	}
	if len(payload) == 0 {
		return nil
	}
	_, err := ws.conn.Write(payload)
	return err
}
