package externalegress

import (
	"bufio"
	"fmt"
	"net"
	"strconv"
	"strings"
)

const MaxImportedConfigBytes = 1024 * 1024

type OpenVPNPreview struct {
	Protocol        string   `json:"protocol"`
	Transport       string   `json:"transport"`
	EndpointHost    string   `json:"endpoint_host"`
	EndpointPort    int      `json:"endpoint_port"`
	DeviceType      string   `json:"device_type"`
	AuthUserPass    bool     `json:"auth_user_pass"`
	InlineBlocks    []string `json:"inline_blocks"`
	RequiredSecrets []string `json:"required_secrets"`
	Warnings        []string `json:"warnings,omitempty"`
}

var forbiddenOpenVPNDirectives = map[string]struct{}{
	"askpass": {}, "auth-gen-token-secret": {},
	"auth-user-pass-verify": {}, "cd": {}, "chroot": {}, "client-connect": {},
	"client-disconnect": {}, "config": {}, "daemon": {}, "down": {}, "down-pre": {},
	"extra-certs": {}, "group": {}, "http-proxy-user-pass": {}, "ipchange": {}, "iproute": {}, "learn-address": {}, "log": {}, "log-append": {},
	"management": {}, "management-client-auth": {}, "management-client-group": {},
	"management-client-user": {}, "plugin": {}, "route-pre-down": {}, "route-up": {},
	"pkcs11-id": {}, "pkcs11-id-management": {}, "pkcs11-private-mode": {}, "pkcs11-providers": {},
	"bind-dev": {}, "client-crresponse": {}, "dh": {}, "engine": {}, "mark": {}, "providers": {},
	"script-security": {}, "setcon": {}, "socks-proxy": {}, "status": {}, "syslog": {}, "tls-export-cert": {}, "tls-verify": {}, "tls-crypt-v2-verify": {}, "tmp-dir": {}, "up": {},
	"up-delay": {}, "up-restart": {}, "user": {}, "writepid": {}, "crl-verify": {},
}

var managedOpenVPNPathDirectives = map[string]string{
	"ca": "ca_certificate", "cert": "certificate", "key": "private_key",
	"pkcs12": "pkcs12", "tls-auth": "tls_auth_key", "tls-crypt": "tls_crypt_key",
	"tls-crypt-v2": "tls_crypt_v2_key", "secret": "static_key",
}

var allowedOpenVPNInlineBlocks = map[string]struct{}{
	"auth-user-pass": {}, "ca": {}, "cert": {}, "key": {}, "pkcs12": {},
	"secret": {}, "tls-auth": {}, "tls-crypt": {}, "tls-crypt-v2": {},
}

func ParseOpenVPN(raw []byte) (OpenVPNPreview, error) {
	preview := OpenVPNPreview{Protocol: "openvpn", Transport: "udp", EndpointPort: 1194, DeviceType: "tun"}
	if len(raw) == 0 {
		return preview, fmt.Errorf("OpenVPN config is empty")
	}
	if len(raw) > MaxImportedConfigBytes {
		return preview, fmt.Errorf("OpenVPN config exceeds %d bytes", MaxImportedConfigBytes)
	}
	if strings.IndexByte(string(raw), 0) >= 0 {
		return preview, fmt.Errorf("OpenVPN config contains a NUL byte")
	}

	scanner := bufio.NewScanner(strings.NewReader(string(raw)))
	scanner.Buffer(make([]byte, 4096), MaxImportedConfigBytes)
	inline := map[string]bool{}
	openInline := ""
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "<") {
			if strings.HasPrefix(line, "</") {
				name := strings.ToLower(strings.Trim(strings.Fields(line)[0], "<>/"))
				if openInline == "" || name != openInline {
					return preview, fmt.Errorf("OpenVPN config line %d closes an unexpected inline block", lineNo)
				}
				openInline = ""
				continue
			}
			name := strings.ToLower(strings.Trim(strings.Fields(line)[0], "<>/"))
			if _, ok := allowedOpenVPNInlineBlocks[name]; !ok || openInline != "" {
				return preview, fmt.Errorf("OpenVPN config line %d contains an unsupported inline block", lineNo)
			}
			inline[name] = true
			openInline = name
			continue
		}
		if openInline != "" {
			continue
		}
		fields, err := splitOpenVPNFields(line)
		if err != nil {
			return preview, fmt.Errorf("OpenVPN config line %d: %w", lineNo, err)
		}
		if len(fields) == 0 {
			continue
		}
		directive := strings.ToLower(strings.TrimLeft(fields[0], "-"))
		if _, forbidden := forbiddenOpenVPNDirectives[directive]; forbidden {
			return preview, fmt.Errorf("OpenVPN directive %q is not allowed in a managed external egress profile", directive)
		}
		switch directive {
		case "remote":
			if len(fields) < 2 {
				return preview, fmt.Errorf("OpenVPN remote requires a host")
			}
			host := strings.TrimSpace(fields[1])
			if net.ParseIP(host) == nil && !validDNSName(host) {
				return preview, fmt.Errorf("OpenVPN remote host is invalid")
			}
			if preview.EndpointHost == "" {
				preview.EndpointHost = host
				if len(fields) >= 3 {
					port, err := strconv.Atoi(fields[2])
					if err != nil || port < 1 || port > 65535 {
						return preview, fmt.Errorf("OpenVPN remote port is invalid")
					}
					preview.EndpointPort = port
				}
			}
		case "proto":
			if len(fields) < 2 {
				return preview, fmt.Errorf("OpenVPN proto requires a value")
			}
			proto := strings.ToLower(fields[1])
			if strings.HasPrefix(proto, "udp") {
				preview.Transport = "udp"
			} else if strings.HasPrefix(proto, "tcp") {
				preview.Transport = "tcp"
			} else {
				return preview, fmt.Errorf("OpenVPN proto %q is not supported", proto)
			}
		case "dev", "dev-type":
			if len(fields) >= 2 {
				dev := strings.ToLower(fields[1])
				if !strings.HasPrefix(dev, "tun") {
					return preview, fmt.Errorf("OpenVPN managed external egress requires a tun device")
				}
				preview.DeviceType = "tun"
			}
		case "auth-user-pass":
			preview.AuthUserPass = true
			if len(fields) > 1 && fields[1] != "[inline]" {
				return preview, fmt.Errorf("OpenVPN auth-user-pass file paths are not allowed; provide credentials separately")
			}
		default:
			if purpose, managed := managedOpenVPNPathDirectives[directive]; managed && len(fields) > 1 && fields[1] != "[inline]" {
				preview.RequiredSecrets = appendUnique(preview.RequiredSecrets, purpose)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return preview, fmt.Errorf("scan OpenVPN config: %w", err)
	}
	if openInline != "" {
		return preview, fmt.Errorf("OpenVPN inline block %q is not closed", openInline)
	}
	if preview.EndpointHost == "" {
		return preview, fmt.Errorf("OpenVPN config does not define a remote endpoint")
	}
	for name := range inline {
		preview.InlineBlocks = append(preview.InlineBlocks, name)
	}
	if preview.AuthUserPass && !inline["auth-user-pass"] {
		preview.RequiredSecrets = appendUnique(preview.RequiredSecrets, "username_password")
	}
	return preview, nil
}

func splitOpenVPNFields(line string) ([]string, error) {
	fields := []string{}
	var current strings.Builder
	var quote rune
	escaped := false
	flush := func() {
		if current.Len() > 0 {
			fields = append(fields, current.String())
			current.Reset()
		}
	}
	for _, r := range line {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				current.WriteRune(r)
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			continue
		}
		if r == ' ' || r == '\t' {
			flush()
			continue
		}
		current.WriteRune(r)
	}
	if escaped || quote != 0 {
		return nil, fmt.Errorf("unterminated escape or quote")
	}
	flush()
	return fields, nil
}

func validDNSName(value string) bool {
	value = strings.TrimSuffix(strings.TrimSpace(value), ".")
	if value == "" || len(value) > 253 {
		return false
	}
	for _, label := range strings.Split(value, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' {
				return false
			}
		}
	}
	return true
}

func appendUnique(values []string, value string) []string {
	for _, item := range values {
		if item == value {
			return values
		}
	}
	return append(values, value)
}
