package externalegress

import (
	"encoding/json"
	"fmt"
	"math"
	"net"
	"net/url"
	"regexp"
	"strings"
	"unicode"
)

var managedVLESSUUIDPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
var managedVLESSFingerprintPattern = regexp.MustCompile(`^[A-Za-z0-9-]{1,32}$`)
var managedVLESSRealityKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{32,64}$`)
var managedVLESSShortIDPattern = regexp.MustCompile(`^[0-9A-Fa-f]{2,16}$`)

type VLESSPreview struct {
	Protocol        string   `json:"protocol"`
	Transport       string   `json:"transport"`
	EndpointHost    string   `json:"endpoint_host"`
	EndpointPort    int      `json:"endpoint_port"`
	Security        string   `json:"security"`
	ServerName      string   `json:"server_name,omitempty"`
	Flow            string   `json:"flow,omitempty"`
	Fingerprint     string   `json:"fingerprint,omitempty"`
	Path            string   `json:"path,omitempty"`
	Host            string   `json:"host,omitempty"`
	ServiceName     string   `json:"service_name,omitempty"`
	PublicKey       string   `json:"public_key,omitempty"`
	ShortID         string   `json:"short_id,omitempty"`
	SpiderX         string   `json:"spider_x,omitempty"`
	ALPN            []string `json:"alpn,omitempty"`
	Encryption      string   `json:"encryption"`
	HasCredential   bool     `json:"has_credential"`
	RequiredSecrets []string `json:"required_secrets,omitempty"`
	Warnings        []string `json:"warnings,omitempty"`
	UUID            string   `json:"-"`
}

func ParseVLESS(raw []byte) (VLESSPreview, error) {
	preview := VLESSPreview{Protocol: "vless", Transport: "tcp", Encryption: "none"}
	content, err := validateImportedText(raw, "VLESS")
	if err != nil {
		return preview, err
	}
	if strings.HasPrefix(strings.ToLower(content), "vless://") {
		err = parseVLESSURL(content, &preview)
	} else {
		err = parseVLESSJSON(content, &preview)
	}
	if err != nil {
		return preview, err
	}
	if preview.ServerName == "" && net.ParseIP(preview.EndpointHost) == nil {
		preview.ServerName = preview.EndpointHost
	}
	if preview.UUID == "" {
		preview.RequiredSecrets = []string{"uuid"}
	} else if !managedVLESSUUIDPattern.MatchString(preview.UUID) {
		return preview, fmt.Errorf("VLESS config contains an invalid UUID credential")
	} else {
		preview.HasCredential = true
	}
	if err := validateVLESSPreview(&preview); err != nil {
		return preview, err
	}
	return preview, nil
}

func parseVLESSURL(content string, preview *VLESSPreview) error {
	u, err := url.Parse(content)
	if err != nil || !strings.EqualFold(u.Scheme, "vless") || u.User == nil {
		return fmt.Errorf("invalid VLESS URL")
	}
	port := 0
	if u.Port() != "" {
		parsed := map[string]any{"port": u.Port()}
		port = firstImportedInt(parsed, "port")
	}
	preview.EndpointHost, preview.EndpointPort = u.Hostname(), port
	preview.UUID = u.User.Username()
	query := u.Query()
	preview.Transport = strings.ToLower(firstNonEmpty(query.Get("type"), query.Get("network"), "tcp"))
	if preview.Transport == "raw" {
		preview.Transport = "tcp"
	}
	preview.Security = strings.ToLower(firstNonEmpty(query.Get("security"), "tls"))
	preview.ServerName = firstNonEmpty(query.Get("sni"), query.Get("serverName"))
	preview.Flow = strings.TrimSpace(query.Get("flow"))
	preview.Fingerprint = strings.TrimSpace(query.Get("fp"))
	preview.Path = strings.TrimSpace(query.Get("path"))
	preview.Host = strings.TrimSpace(query.Get("host"))
	preview.ServiceName = firstNonEmpty(query.Get("serviceName"), query.Get("service_name"))
	preview.PublicKey = firstNonEmpty(query.Get("pbk"), query.Get("publicKey"), query.Get("password"))
	preview.ShortID = firstNonEmpty(query.Get("sid"), query.Get("shortId"))
	preview.SpiderX = firstNonEmpty(query.Get("spx"), query.Get("spiderX"))
	preview.ALPN = splitVLESSALPN(query.Get("alpn"))
	preview.Encryption = firstNonEmpty(query.Get("encryption"), "none")
	allowedQuery := map[string]bool{
		"type": true, "network": true, "security": true, "sni": true, "serverName": true,
		"flow": true, "fp": true, "path": true, "host": true, "serviceName": true,
		"service_name": true, "pbk": true, "publicKey": true, "password": true,
		"sid": true, "shortId": true, "spx": true, "spiderX": true, "alpn": true,
		"headerType": true, "encryption": true, "allowInsecure": true,
	}
	for key := range query {
		if !allowedQuery[key] {
			return fmt.Errorf("unsupported VLESS URL parameter %q", key)
		}
	}
	if strings.EqualFold(query.Get("allowInsecure"), "1") || strings.EqualFold(query.Get("allowInsecure"), "true") {
		return fmt.Errorf("VLESS allowInsecure is forbidden in a managed profile")
	}
	if headerType := strings.TrimSpace(query.Get("headerType")); headerType != "" && !strings.EqualFold(headerType, "none") {
		return fmt.Errorf("VLESS headerType %q is not supported by the managed runtime", headerType)
	}
	return validateManagedEndpoint(preview.EndpointHost, preview.EndpointPort)
}

func parseVLESSJSON(content string, preview *VLESSPreview) error {
	var value map[string]any
	if err := json.Unmarshal([]byte(content), &value); err != nil {
		return fmt.Errorf("VLESS configuration must be a vless:// URL or JSON object")
	}
	stringFields := map[string]bool{
		"address": true, "server": true, "host": true, "id": true, "uuid": true,
		"network": true, "type": true, "transport": true, "security": true,
		"server_name": true, "serverName": true, "sni": true, "flow": true,
		"fingerprint": true, "fp": true, "path": true, "ws_host": true,
		"host_header": true, "service_name": true, "serviceName": true,
		"public_key": true, "publicKey": true, "pbk": true, "reality_password": true,
		"short_id": true, "shortId": true, "sid": true, "spider_x": true,
		"spiderX": true, "encryption": true,
	}
	for key, item := range value {
		switch {
		case stringFields[key]:
			if _, ok := item.(string); !ok {
				return fmt.Errorf("VLESS field %q must be a string", key)
			}
		case key == "port" || key == "server_port":
			switch typed := item.(type) {
			case string:
			case float64:
				if math.Trunc(typed) != typed {
					return fmt.Errorf("VLESS field %q must be an integer", key)
				}
			default:
				return fmt.Errorf("VLESS field %q must be a string or integer", key)
			}
		case key == "allow_insecure" || key == "allowInsecure":
			if _, ok := item.(bool); !ok {
				return fmt.Errorf("VLESS field %q must be a boolean", key)
			}
		case key == "alpn":
			switch typed := item.(type) {
			case string:
			case []any:
				for _, protocol := range typed {
					if _, ok := protocol.(string); !ok {
						return fmt.Errorf("VLESS field %q must contain strings", key)
					}
				}
			default:
				return fmt.Errorf("VLESS field %q must be a string or string array", key)
			}
		default:
			return fmt.Errorf("unsupported VLESS field %q", key)
		}
	}
	preview.EndpointHost = firstImportedString(value, "address", "server", "host")
	preview.EndpointPort = firstImportedInt(value, "port", "server_port")
	preview.UUID = firstImportedString(value, "id", "uuid")
	preview.Transport = strings.ToLower(firstNonEmpty(firstImportedString(value, "network", "type", "transport"), "tcp"))
	if preview.Transport == "raw" {
		preview.Transport = "tcp"
	}
	preview.Security = strings.ToLower(firstNonEmpty(firstImportedString(value, "security"), "tls"))
	preview.ServerName = firstImportedString(value, "server_name", "serverName", "sni")
	preview.Flow = firstImportedString(value, "flow")
	preview.Fingerprint = firstImportedString(value, "fingerprint", "fp")
	preview.Path = firstImportedString(value, "path")
	preview.Host = firstImportedString(value, "ws_host", "host_header")
	preview.ServiceName = firstImportedString(value, "service_name", "serviceName")
	preview.PublicKey = firstImportedString(value, "public_key", "publicKey", "pbk", "reality_password")
	preview.ShortID = firstImportedString(value, "short_id", "shortId", "sid")
	preview.SpiderX = firstImportedString(value, "spider_x", "spiderX")
	preview.ALPN = parseVLESSALPNValue(value["alpn"])
	preview.Encryption = firstNonEmpty(firstImportedString(value, "encryption"), "none")
	if insecure, _ := value["allow_insecure"].(bool); insecure {
		return fmt.Errorf("VLESS allow_insecure is forbidden in a managed profile")
	}
	if insecure, _ := value["allowInsecure"].(bool); insecure {
		return fmt.Errorf("VLESS allow_insecure is forbidden in a managed profile")
	}
	return validateManagedEndpoint(preview.EndpointHost, preview.EndpointPort)
}

func validateVLESSPreview(preview *VLESSPreview) error {
	switch preview.Transport {
	case "tcp", "ws", "grpc", "httpupgrade", "xhttp":
	default:
		return fmt.Errorf("VLESS transport %q is not supported by the managed runtime", preview.Transport)
	}
	if preview.Security != "tls" && preview.Security != "reality" {
		return fmt.Errorf("managed VLESS external egress requires TLS or REALITY transport security")
	}
	if preview.ServerName == "" {
		return fmt.Errorf("VLESS TLS/REALITY server name is required")
	}
	if net.ParseIP(preview.ServerName) == nil && !validDNSName(preview.ServerName) {
		return fmt.Errorf("VLESS TLS/REALITY server name is invalid")
	}
	if !safeVLESSValue(preview.Flow, 64) || !safeVLESSValue(preview.Fingerprint, 32) ||
		!safeVLESSValue(preview.Path, 2048) || !safeVLESSValue(preview.Host, 255) ||
		!safeVLESSValue(preview.ServiceName, 512) || !safeVLESSValue(preview.SpiderX, 2048) {
		return fmt.Errorf("VLESS transport settings contain invalid control characters or exceed managed limits")
	}
	if preview.Encryption != "none" {
		return fmt.Errorf("managed VLESS external egress requires encryption=none")
	}
	if preview.Flow != "" && preview.Flow != "xtls-rprx-vision" && preview.Flow != "xtls-rprx-vision-udp443" {
		return fmt.Errorf("VLESS flow %q is not supported", preview.Flow)
	}
	if preview.Fingerprint != "" && !managedVLESSFingerprintPattern.MatchString(preview.Fingerprint) {
		return fmt.Errorf("VLESS fingerprint is invalid")
	}
	if preview.Host != "" && net.ParseIP(preview.Host) == nil && !validDNSName(preview.Host) {
		return fmt.Errorf("VLESS transport Host is invalid")
	}
	if preview.Path != "" && !strings.HasPrefix(preview.Path, "/") {
		return fmt.Errorf("VLESS transport path must start with /")
	}
	if preview.SpiderX != "" && !strings.HasPrefix(preview.SpiderX, "/") {
		return fmt.Errorf("VLESS REALITY spiderX must start with /")
	}
	for _, protocol := range preview.ALPN {
		switch protocol {
		case "h2", "http/1.1", "h3":
		default:
			return fmt.Errorf("VLESS ALPN protocol %q is not supported", protocol)
		}
	}
	if preview.Security == "reality" {
		if preview.PublicKey == "" {
			return fmt.Errorf("VLESS REALITY requires a public key/password")
		}
		if !managedVLESSRealityKeyPattern.MatchString(preview.PublicKey) {
			return fmt.Errorf("VLESS REALITY public key/password is invalid")
		}
		if preview.ShortID != "" && (!managedVLESSShortIDPattern.MatchString(preview.ShortID) || len(preview.ShortID)%2 != 0) {
			return fmt.Errorf("VLESS REALITY short ID must contain 2-16 hexadecimal characters in complete bytes")
		}
		if preview.Transport == "ws" || preview.Transport == "httpupgrade" {
			return fmt.Errorf("VLESS REALITY does not support %s transport", preview.Transport)
		}
	}
	if preview.Transport == "ws" && preview.Path == "" {
		preview.Warnings = append(preview.Warnings, "WebSocket path defaults to /")
	}
	return nil
}

func splitVLESSALPN(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			result = append(result, part)
		}
	}
	return result
}

func parseVLESSALPNValue(value any) []string {
	switch typed := value.(type) {
	case string:
		return splitVLESSALPN(typed)
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if protocol, ok := item.(string); ok {
				if protocol = strings.TrimSpace(protocol); protocol != "" {
					result = append(result, protocol)
				}
			}
		}
		return result
	default:
		return nil
	}
}

func safeVLESSValue(value string, maxLength int) bool {
	if len(value) > maxLength {
		return false
	}
	return strings.IndexFunc(value, unicode.IsControl) < 0
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
