package http

import (
	"context"
	"errors"
	"fmt"
	nethttp "net/http"
	"net/netip"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type nodeSSHHostKeyScanRequest struct {
	SSHHost string `json:"ssh_host"`
	SSHPort int    `json:"ssh_port"`
}

type nodeSSHHostKeyScanEntry struct {
	Fingerprint   string `json:"fingerprint"`
	Algorithm     string `json:"algorithm"`
	Bits          int    `json:"bits"`
	KnownHostLine string `json:"known_host_line"`
}

type nodeSSHHostKeyScanResponse struct {
	Host         string                    `json:"host"`
	Port         int                       `json:"port"`
	Fingerprints []nodeSSHHostKeyScanEntry `json:"fingerprints"`
}

func (s *Server) scanNodeSSHHostKey(w nethttp.ResponseWriter, r *nethttp.Request) {
	node, err := s.store.GetNode(r.Context(), idParam(r))
	if err != nil {
		writeErr(w, 404, "node not found")
		return
	}
	var req nodeSSHHostKeyScanRequest
	if !decodeOptional(r, &req) {
		writeErr(w, 400, "invalid ssh host key scan payload")
		return
	}
	host := strings.TrimSpace(req.SSHHost)
	if host == "" {
		host = strings.TrimSpace(node.Address)
	}
	port := req.SSHPort
	if port == 0 {
		port = 22
	}
	result, err := scanSSHHostKey(r.Context(), host, port)
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 200, result)
}

func scanSSHHostKey(ctx context.Context, host string, port int) (nodeSSHHostKeyScanResponse, error) {
	host = strings.Trim(strings.TrimSpace(host), "[]")
	if !isSafeSSHHostKeyScanHost(host) {
		return nodeSSHHostKeyScanResponse{}, errors.New("ssh_host contains unsafe characters")
	}
	if port < 1 || port > 65535 {
		return nodeSSHHostKeyScanResponse{}, errors.New("ssh_port must be between 1 and 65535")
	}
	if _, err := exec.LookPath("ssh-keyscan"); err != nil {
		return nodeSSHHostKeyScanResponse{}, errors.New("ssh-keyscan is required on the API host")
	}
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		return nodeSSHHostKeyScanResponse{}, errors.New("ssh-keygen is required on the API host")
	}

	scanCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	scanCmd := exec.CommandContext(scanCtx, "ssh-keyscan", "-p", strconv.Itoa(port), "-T", "10", host)
	scanOut, err := scanCmd.CombinedOutput()
	if err != nil {
		return nodeSSHHostKeyScanResponse{}, fmt.Errorf("ssh host key scan failed: %w: %s", err, strings.TrimSpace(string(scanOut)))
	}
	knownHostLines := sshKnownHostKeyLines(string(scanOut))
	if len(knownHostLines) == 0 {
		return nodeSSHHostKeyScanResponse{}, errors.New("ssh host key scan returned no keys")
	}

	tmp, err := os.CreateTemp("", "megavpn-ssh-host-key-scan-*")
	if err != nil {
		return nodeSSHHostKeyScanResponse{}, err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.WriteString(strings.Join(knownHostLines, "\n") + "\n"); err != nil {
		tmp.Close()
		return nodeSSHHostKeyScanResponse{}, err
	}
	if err := tmp.Close(); err != nil {
		return nodeSSHHostKeyScanResponse{}, err
	}

	fpOut, err := exec.CommandContext(scanCtx, "ssh-keygen", "-lf", tmpPath, "-E", "sha256").CombinedOutput()
	if err != nil {
		return nodeSSHHostKeyScanResponse{}, fmt.Errorf("ssh host key fingerprint failed: %w: %s", err, strings.TrimSpace(string(fpOut)))
	}
	entries := parseSSHHostKeyFingerprints(string(fpOut), knownHostLines)
	if len(entries) == 0 {
		return nodeSSHHostKeyScanResponse{}, errors.New("ssh host key fingerprint parser returned no SHA256 fingerprints")
	}
	return nodeSSHHostKeyScanResponse{Host: host, Port: port, Fingerprints: entries}, nil
}

func sshKnownHostKeyLines(output string) []string {
	lines := []string{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func parseSSHHostKeyFingerprints(output string, knownHostLines []string) []nodeSSHHostKeyScanEntry {
	out := []nodeSSHHostKeyScanEntry{}
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 || !strings.HasPrefix(fields[1], "SHA256:") {
			continue
		}
		entry := nodeSSHHostKeyScanEntry{Fingerprint: fields[1]}
		if bits, err := strconv.Atoi(fields[0]); err == nil {
			entry.Bits = bits
		}
		if len(out) < len(knownHostLines) {
			entry.KnownHostLine = knownHostLines[len(out)]
			parts := strings.Fields(entry.KnownHostLine)
			if len(parts) >= 2 {
				entry.Algorithm = parts[1]
			}
		}
		out = append(out, entry)
	}
	return out
}

func isSafeSSHHostKeyScanHost(host string) bool {
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
