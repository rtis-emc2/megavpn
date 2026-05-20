package main

import (
	"bufio"
	"bytes"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

func collectInventory() map[string]any {
	host, _ := os.Hostname()
	osInfo := readOSRelease()
	kernel := strings.TrimSpace(runOutput("uname", "-r"))
	arch := runtime.GOARCH
	if arch != "amd64" && arch != "arm64" {
		arch = runtime.GOARCH
	}
	services := collectServices([]string{"xray", "xray-megavpn", "rtis.xray", "openvpn", "openvpn-server@server", "megavpn-openvpn", "nginx", "wg-quick", "strongswan", "strongswan-starter", "ipsec", "xl2tpd", "shadowsocks-libev", "shadowsocks", "squid"})
	mergeMap(services, collectInterestingSystemdUnits())
	return map[string]any{
		"agent_version": appVersion,
		"hostname":      host,
		"os":            osInfo,
		"kernel":        kernel,
		"arch":          arch,
		"systemd":       systemdAvailable(),
		"interfaces":    collectInterfaces(),
		"binaries":      collectBinaries([]string{"systemctl", "xray", "openvpn", "nginx", "wg", "wg-quick", "ip", "ipsec", "strongswan", "xl2tpd", "ss", "ss-server", "squid", "iptables", "nft"}),
		"services":      services,
		"processes":     collectProcesses(),
		"config_files":  collectConfigFiles(),
		"ports":         collectListeningPorts(),
		"collected_at":  time.Now().UTC().Format(time.RFC3339),
	}
}

func readOSRelease() map[string]any {
	out := map[string]any{"family": runtime.GOOS, "pretty_name": runtime.GOOS}
	b, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return out
	}
	s := bufio.NewScanner(bytes.NewReader(b))
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		v = strings.Trim(strings.TrimSpace(v), "\"")
		switch k {
		case "ID":
			out["id"] = v
		case "VERSION_ID":
			out["version_id"] = v
		case "VERSION_CODENAME", "UBUNTU_CODENAME":
			out["version_codename"] = v
		case "PRETTY_NAME":
			out["pretty_name"] = v
		}
	}
	return out
}

func systemdAvailable() bool {
	if _, err := os.Stat("/run/systemd/system"); err == nil {
		return true
	}
	_, err := exec.LookPath("systemctl")
	return err == nil
}

func collectInterfaces() []map[string]any {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil
	}
	out := make([]map[string]any, 0, len(ifs))
	for _, iface := range ifs {
		addrs, _ := iface.Addrs()
		addrStrings := make([]string, 0, len(addrs))
		for _, a := range addrs {
			addrStrings = append(addrStrings, a.String())
		}
		out = append(out, map[string]any{
			"name":          iface.Name,
			"mtu":           iface.MTU,
			"flags":         iface.Flags.String(),
			"hardware_addr": iface.HardwareAddr.String(),
			"addrs":         addrStrings,
		})
	}
	return out
}

func collectBinaries(names []string) map[string]any {
	out := map[string]any{}
	for _, name := range names {
		path, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		version := strings.TrimSpace(firstLine(runOutput(name, "--version")))
		if version == "" && name == "nginx" {
			version = strings.TrimSpace(firstLine(runCombinedOutput(name, "-v")))
		}
		if version == "" {
			version = path
		}
		out[name] = version
	}
	return out
}

func collectConfigFiles() map[string]any {
	patterns := []string{
		"/etc/xray/*.json",
		"/usr/local/etc/xray/*.json",
		"/opt/xray/conf/*.json",
		"/opt/megavpn/xray/conf/*.json",
		"/etc/openvpn/*.conf",
		"/etc/openvpn/server/*.conf",
		"/opt/megavpn/openvpn/*.conf",
		"/opt/megavpn/openvpn/server*.conf",
		"/etc/nginx/nginx.conf",
		"/etc/nginx/sites-enabled/*",
		"/etc/wireguard/*.conf",
		"/etc/ipsec.conf",
		"/etc/ipsec.d/*.conf",
		"/etc/strongswan.conf",
		"/etc/strongswan.d/*.conf",
		"/etc/xl2tpd/xl2tpd.conf",
		"/etc/shadowsocks*.json",
		"/etc/shadowsocks-libev/*.json",
		"/opt/megavpn/shadowsocks/*.json",
		"/etc/squid/squid.conf",
	}
	out := map[string]any{}
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}
		for _, path := range matches {
			if len(out) >= 100 {
				return out
			}
			info, err := os.Stat(path)
			if err != nil || info.IsDir() {
				continue
			}
			out[path] = map[string]any{
				"size_bytes":  info.Size(),
				"mode":        info.Mode().String(),
				"modified_at": info.ModTime().UTC().Format(time.RFC3339),
			}
		}
	}
	return out
}

func mergeMap(dst, src map[string]any) {
	for k, v := range src {
		dst[k] = v
	}
}

func collectInterestingSystemdUnits() map[string]any {
	out := map[string]any{}
	if !systemdAvailable() {
		return out
	}
	raw := strings.TrimSpace(runOutput("systemctl", "list-unit-files", "--type=service", "--no-legend", "--no-pager"))
	if raw == "" {
		return out
	}
	interesting := []string{"xray", "openvpn", "nginx", "wg-quick", "strongswan", "ipsec", "xl2tpd", "shadowsocks", "squid", "megavpn"}
	for _, line := range strings.Split(raw, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 1 {
			continue
		}
		unit := strings.TrimSuffix(fields[0], ".service")
		matched := false
		for _, needle := range interesting {
			if strings.Contains(unit, needle) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		active := strings.TrimSpace(runOutput("systemctl", "is-active", unit))
		enabled := ""
		if len(fields) >= 2 {
			enabled = fields[1]
		} else {
			enabled = strings.TrimSpace(runOutput("systemctl", "is-enabled", unit))
		}
		out[unit] = map[string]any{
			"active_state":  normalizeSystemctlState(active),
			"enabled_state": normalizeSystemctlState(enabled),
			"source":        "unit-files",
		}
	}
	return out
}

func collectProcesses() []map[string]any {
	raw := strings.TrimSpace(runOutput("ps", "-eo", "pid=,comm=,args="))
	if raw == "" {
		return nil
	}
	interesting := []string{"xray", "openvpn", "nginx", "wg-quick", "strongswan", "charon", "xl2tpd", "ss-server", "squid", "megavpn"}
	out := []map[string]any{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		matched := false
		for _, needle := range interesting {
			if strings.Contains(line, needle) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		out = append(out, map[string]any{"pid": fields[0], "command": fields[1], "args": line})
		if len(out) >= 100 {
			break
		}
	}
	return out
}

func collectServices(names []string) map[string]any {
	out := map[string]any{}
	if !systemdAvailable() {
		return out
	}
	for _, name := range names {
		active := strings.TrimSpace(runOutput("systemctl", "is-active", name))
		enabled := strings.TrimSpace(runOutput("systemctl", "is-enabled", name))
		if active == "" && enabled == "" {
			continue
		}
		out[name] = map[string]any{
			"active_state":  normalizeSystemctlState(active),
			"enabled_state": normalizeSystemctlState(enabled),
		}
	}
	return out
}

func collectListeningPorts() []map[string]any {
	outRaw := strings.TrimSpace(runOutput("ss", "-lntuH"))
	if outRaw == "" {
		return nil
	}
	lines := strings.Split(outRaw, "\n")
	out := make([]map[string]any, 0, len(lines))
	for i, line := range lines {
		if i >= 200 {
			break
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		out = append(out, map[string]any{"network": fields[0], "state": fields[1], "local_address": fields[4]})
	}
	return out
}

func capabilitySummary(inv map[string]any) []string {
	binaries, _ := inv["binaries"].(map[string]any)
	keys := make([]string, 0, len(binaries))
	for k := range binaries {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
