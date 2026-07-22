package externalegress

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"unicode"
)

type ShadowsocksPreview struct {
	Protocol        string   `json:"protocol"`
	Transport       string   `json:"transport"`
	EndpointHost    string   `json:"endpoint_host"`
	EndpointPort    int      `json:"endpoint_port"`
	Method          string   `json:"method"`
	HasPassword     bool     `json:"has_password"`
	RequiredSecrets []string `json:"required_secrets,omitempty"`
	Warnings        []string `json:"warnings,omitempty"`
	Password        string   `json:"-"`
}

var supportedShadowsocksMethods = map[string]bool{
	"2022-blake3-aes-128-gcm":       true,
	"2022-blake3-aes-256-gcm":       true,
	"2022-blake3-chacha20-poly1305": true,
	"aes-128-gcm":                   true,
	"aes-256-gcm":                   true,
	"chacha20-poly1305":             true,
	"chacha20-ietf-poly1305":        true,
	"xchacha20-poly1305":            true,
	"xchacha20-ietf-poly1305":       true,
}

func ParseShadowsocks(raw []byte) (ShadowsocksPreview, error) {
	preview := ShadowsocksPreview{Protocol: "shadowsocks", Transport: "tcp_udp"}
	content, err := validateImportedText(raw, "Shadowsocks")
	if err != nil {
		return preview, err
	}
	if strings.HasPrefix(strings.ToLower(content), "ss://") {
		err = parseShadowsocksURL(content, &preview)
	} else {
		err = parseShadowsocksJSON(content, &preview)
	}
	if err != nil {
		return preview, err
	}
	if !supportedShadowsocksMethods[preview.Method] {
		return preview, fmt.Errorf("Shadowsocks method %q is not supported by the managed Xray runtime", preview.Method)
	}
	if preview.Password == "" {
		preview.RequiredSecrets = []string{"password"}
	} else {
		if len(preview.Password) > 4096 || strings.IndexFunc(preview.Password, unicode.IsControl) >= 0 {
			return preview, fmt.Errorf("Shadowsocks password contains control characters or exceeds the managed limit")
		}
		preview.HasPassword = true
	}
	return preview, nil
}

func parseShadowsocksURL(content string, preview *ShadowsocksPreview) error {
	u, err := url.Parse(content)
	if err != nil || !strings.EqualFold(u.Scheme, "ss") {
		return fmt.Errorf("invalid Shadowsocks URL")
	}
	if plugin := strings.TrimSpace(u.Query().Get("plugin")); plugin != "" {
		return fmt.Errorf("Shadowsocks SIP003 plugins are not allowed in a managed profile")
	}
	for key := range u.Query() {
		if key != "plugin" {
			return fmt.Errorf("unsupported Shadowsocks URL parameter %q", key)
		}
	}
	userinfo := ""
	hostport := u.Host
	if u.User != nil {
		userinfo = u.User.String()
	} else {
		decoded, decodeErr := decodeURLBase64(strings.TrimPrefix(content, "ss://"))
		if decodeErr != nil {
			return fmt.Errorf("invalid Shadowsocks credentials encoding")
		}
		if hash := strings.IndexByte(decoded, '#'); hash >= 0 {
			decoded = decoded[:hash]
		}
		at := strings.LastIndex(decoded, "@")
		if at <= 0 {
			return fmt.Errorf("Shadowsocks URL does not contain credentials and endpoint")
		}
		userinfo, hostport = decoded[:at], decoded[at+1:]
	}
	if decoded, decodeErr := url.QueryUnescape(userinfo); decodeErr == nil {
		userinfo = decoded
	}
	if !strings.Contains(userinfo, ":") {
		decoded, decodeErr := decodeURLBase64(userinfo)
		if decodeErr != nil {
			return fmt.Errorf("invalid Shadowsocks credentials encoding")
		}
		userinfo = decoded
	}
	method, password, ok := strings.Cut(userinfo, ":")
	if !ok || strings.TrimSpace(method) == "" || password == "" {
		return fmt.Errorf("Shadowsocks URL must contain method and password")
	}
	host, port, err := splitManagedEndpoint(hostport)
	if err != nil {
		return fmt.Errorf("invalid Shadowsocks endpoint: %w", err)
	}
	preview.EndpointHost, preview.EndpointPort = host, port
	preview.Method, preview.Password = strings.ToLower(strings.TrimSpace(method)), password
	return nil
}

func parseShadowsocksJSON(content string, preview *ShadowsocksPreview) error {
	var value map[string]any
	if err := json.Unmarshal([]byte(content), &value); err != nil {
		return fmt.Errorf("Shadowsocks configuration must be a SIP002 URL or JSON object")
	}
	if value["plugin"] != nil || value["plugin_opts"] != nil {
		return fmt.Errorf("Shadowsocks plugins are not allowed in a managed profile")
	}
	stringFields := map[string]bool{
		"server": true, "address": true, "host": true, "method": true,
		"cipher": true, "password": true,
	}
	for key, item := range value {
		switch {
		case stringFields[key]:
			if _, ok := item.(string); !ok {
				return fmt.Errorf("Shadowsocks field %q must be a string", key)
			}
		case key == "server_port" || key == "port":
			switch typed := item.(type) {
			case string:
			case float64:
				if typed != float64(int(typed)) {
					return fmt.Errorf("Shadowsocks field %q must be an integer", key)
				}
			default:
				return fmt.Errorf("Shadowsocks field %q must be a string or integer", key)
			}
		case key == "plugin" || key == "plugin_opts":
			return fmt.Errorf("Shadowsocks plugins are not allowed in a managed profile")
		default:
			return fmt.Errorf("unsupported Shadowsocks field %q", key)
		}
	}
	preview.EndpointHost = firstImportedString(value, "server", "address", "host")
	preview.EndpointPort = firstImportedInt(value, "server_port", "port")
	preview.Method = strings.ToLower(firstImportedString(value, "method", "cipher"))
	preview.Password = firstImportedSecretString(value, "password")
	return validateManagedEndpoint(preview.EndpointHost, preview.EndpointPort)
}

func decodeURLBase64(value string) (string, error) {
	value = strings.TrimSpace(strings.SplitN(strings.SplitN(value, "?", 2)[0], "#", 2)[0])
	for _, encoding := range []*base64.Encoding{base64.RawURLEncoding, base64.URLEncoding, base64.RawStdEncoding, base64.StdEncoding} {
		if decoded, err := encoding.DecodeString(value); err == nil {
			return string(decoded), nil
		}
	}
	return "", fmt.Errorf("invalid base64")
}

func splitManagedEndpoint(value string) (string, int, error) {
	host, portValue, err := net.SplitHostPort(strings.TrimSpace(value))
	if err != nil {
		return "", 0, err
	}
	port, err := strconv.Atoi(portValue)
	if err != nil {
		return "", 0, fmt.Errorf("invalid port")
	}
	if err := validateManagedEndpoint(host, port); err != nil {
		return "", 0, err
	}
	return host, port, nil
}

func validateManagedEndpoint(host string, port int) error {
	host = strings.TrimSpace(host)
	if (net.ParseIP(host) == nil && !validDNSName(host)) || port < 1 || port > 65535 {
		return fmt.Errorf("endpoint host or port is invalid")
	}
	return nil
}

func validateImportedText(raw []byte, protocol string) (string, error) {
	if len(raw) == 0 {
		return "", fmt.Errorf("%s config is empty", protocol)
	}
	if len(raw) > MaxImportedConfigBytes || strings.IndexByte(string(raw), 0) >= 0 {
		return "", fmt.Errorf("%s config is invalid or too large", protocol)
	}
	return strings.TrimSpace(string(raw)), nil
}

func firstImportedString(value map[string]any, keys ...string) string {
	for _, key := range keys {
		if item, ok := value[key]; ok {
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
				return strings.TrimSpace(text)
			}
		}
	}
	return ""
}

func firstImportedSecretString(value map[string]any, keys ...string) string {
	for _, key := range keys {
		if item, ok := value[key]; ok {
			if text, ok := item.(string); ok && text != "" {
				return text
			}
		}
	}
	return ""
}

func firstImportedInt(value map[string]any, keys ...string) int {
	for _, key := range keys {
		switch item := value[key].(type) {
		case float64:
			return int(item)
		case string:
			if parsed, err := strconv.Atoi(strings.TrimSpace(item)); err == nil {
				return parsed
			}
		}
	}
	return 0
}
