package main

import (
	"os"
	"os/exec"
	"strings"
)

func findExecutable(name string, candidates ...string) string {
	path, _ := resolveExecutable(name, candidates...)
	return path
}

func resolveExecutable(name string, candidates ...string) (string, bool) {
	name = strings.TrimSpace(name)
	if name != "" {
		if path, err := exec.LookPath(name); err == nil && path != "" {
			return path, true
		}
	}
	for _, candidate := range append(candidates, defaultExecutableCandidates(name)...) {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return candidate, true
		}
	}
	return name, false
}

func defaultExecutableCandidates(name string) []string {
	switch strings.TrimSpace(name) {
	case "nginx":
		return []string{"/usr/sbin/nginx", "/usr/bin/nginx", "/usr/local/sbin/nginx", "/usr/local/bin/nginx", "/usr/local/nginx/sbin/nginx", "/opt/nginx/sbin/nginx"}
	case "systemctl":
		return []string{"/usr/bin/systemctl", "/bin/systemctl"}
	case "xray":
		return []string{"/usr/local/bin/xray", "/usr/bin/xray", "/opt/xray/xray"}
	case "openvpn":
		return []string{"/usr/sbin/openvpn", "/usr/bin/openvpn", "/usr/local/sbin/openvpn", "/usr/local/bin/openvpn"}
	case "wg", "wg-quick":
		return []string{"/usr/bin/" + name, "/usr/sbin/" + name, "/usr/local/bin/" + name, "/usr/local/sbin/" + name}
	case "ss-server":
		return []string{"/usr/bin/ss-server", "/usr/local/bin/ss-server", "/usr/sbin/ss-server", "/usr/local/sbin/ss-server"}
	case "squid":
		return []string{"/usr/sbin/squid", "/usr/bin/squid", "/usr/local/sbin/squid", "/usr/local/bin/squid"}
	case "ip", "ss", "iptables", "nft", "ipsec", "strongswan", "pppd", "xl2tpd":
		return []string{"/usr/sbin/" + name, "/usr/bin/" + name, "/sbin/" + name, "/bin/" + name, "/usr/local/sbin/" + name, "/usr/local/bin/" + name}
	default:
		return nil
	}
}

func xrayExecutablePath() string {
	return findExecutable("xray", "/usr/local/bin/xray", "/usr/bin/xray", "/opt/xray/xray")
}
