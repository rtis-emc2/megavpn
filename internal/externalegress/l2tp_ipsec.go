package externalegress

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"
)

type L2TPIPsecPreview struct {
	Protocol        string   `json:"protocol"`
	Transport       string   `json:"transport"`
	EndpointHost    string   `json:"endpoint_host"`
	EndpointPort    int      `json:"endpoint_port"`
	RemoteID        string   `json:"remote_id,omitempty"`
	IKEProposal     string   `json:"ike_proposal"`
	ESPProposal     string   `json:"esp_proposal"`
	HasUsername     bool     `json:"has_username"`
	HasPassword     bool     `json:"has_password"`
	HasPSK          bool     `json:"has_psk"`
	RequiredSecrets []string `json:"required_secrets,omitempty"`
	Warnings        []string `json:"warnings,omitempty"`
	Username        string   `json:"-"`
	Password        string   `json:"-"`
	PSK             string   `json:"-"`
}

func ParseL2TPIPsec(raw []byte) (L2TPIPsecPreview, error) {
	preview := L2TPIPsecPreview{
		Protocol: "l2tp_ipsec", Transport: "udp_ipsec", EndpointPort: 1701,
		IKEProposal: "aes256-sha256-modp2048,aes256-sha1-modp2048",
		ESPProposal: "aes256-sha256,aes256-sha1,aes128-sha1",
	}
	content, err := validateImportedText(raw, "L2TP/IPsec")
	if err != nil {
		return preview, err
	}
	values := map[string]string{}
	if strings.HasPrefix(content, "{") {
		var object map[string]any
		if err := json.Unmarshal([]byte(content), &object); err != nil {
			return preview, fmt.Errorf("invalid L2TP/IPsec JSON configuration")
		}
		for key, value := range object {
			key = strings.ToLower(strings.TrimSpace(key))
			if !allowedL2TPIPsecKey(key) {
				return preview, fmt.Errorf("unsupported L2TP/IPsec field %q", key)
			}
			if text, ok := value.(string); ok {
				if l2tpIPsecSecretKey(key) {
					values[key] = text
				} else {
					values[key] = strings.TrimSpace(text)
				}
			} else {
				return preview, fmt.Errorf("L2TP/IPsec field %q must be a string", key)
			}
		}
	} else {
		seen := map[string]bool{}
		for lineNo, line := range strings.Split(content, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
				continue
			}
			key, value, ok := strings.Cut(line, "=")
			if !ok {
				return preview, fmt.Errorf("L2TP/IPsec line %d must use key=value", lineNo+1)
			}
			key = strings.ToLower(strings.TrimSpace(key))
			if !allowedL2TPIPsecKey(key) {
				return preview, fmt.Errorf("unsupported L2TP/IPsec field %q", key)
			}
			if seen[key] {
				return preview, fmt.Errorf("duplicate L2TP/IPsec field %q", key)
			}
			seen[key] = true
			values[key] = strings.TrimSpace(value)
		}
	}
	preview.EndpointHost = firstMapValue(values, "server", "endpoint", "host", "address")
	preview.RemoteID = firstMapValue(values, "remote_id", "rightid")
	preview.Username = firstMapSecretValue(values, "username", "user", "name")
	preview.Password = firstMapSecretValue(values, "password", "pass")
	preview.PSK = firstMapSecretValue(values, "preshared_key", "pre_shared_key", "psk", "ipsec_psk")
	preview.IKEProposal = firstNonEmpty(firstMapValue(values, "ike", "ike_proposal"), preview.IKEProposal)
	preview.ESPProposal = firstNonEmpty(firstMapValue(values, "esp", "esp_proposal"), preview.ESPProposal)
	if err := validateManagedEndpoint(preview.EndpointHost, preview.EndpointPort); err != nil {
		return preview, err
	}
	if !safeIPsecProposalList(preview.IKEProposal) || !safeIPsecProposalList(preview.ESPProposal) {
		return preview, fmt.Errorf("L2TP/IPsec proposal contains unsupported characters")
	}
	if preview.RemoteID != "" && !safeIPsecRemoteID(preview.RemoteID) {
		return preview, fmt.Errorf("L2TP/IPsec remote identity is invalid")
	}
	preview.HasUsername, preview.HasPassword, preview.HasPSK = preview.Username != "", preview.Password != "", preview.PSK != ""
	if !preview.HasUsername {
		preview.RequiredSecrets = append(preview.RequiredSecrets, "username")
	}
	if !preview.HasPassword {
		preview.RequiredSecrets = append(preview.RequiredSecrets, "password")
	}
	if !preview.HasPSK {
		preview.RequiredSecrets = append(preview.RequiredSecrets, "preshared_key")
	}
	if strings.Contains(preview.IKEProposal, "modp1024") {
		preview.Warnings = append(preview.Warnings, "modp1024 is retained only as a legacy provider fallback")
	}
	return preview, nil
}

func safeIPsecRemoteID(value string) bool {
	value = strings.TrimSpace(strings.TrimPrefix(value, "@"))
	return validDNSName(value) || net.ParseIP(value) != nil
}

func allowedL2TPIPsecKey(key string) bool {
	switch key {
	case "server", "endpoint", "host", "address", "remote_id", "rightid", "username", "user", "name", "password", "pass", "preshared_key", "pre_shared_key", "psk", "ipsec_psk", "ike", "ike_proposal", "esp", "esp_proposal":
		return true
	default:
		return false
	}
}

func l2tpIPsecSecretKey(key string) bool {
	switch key {
	case "username", "user", "name", "password", "pass", "preshared_key", "pre_shared_key", "psk", "ipsec_psk":
		return true
	default:
		return false
	}
}

func safeIPsecProposalList(value string) bool {
	if value == "" || len(value) > 512 {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == ',' {
			continue
		}
		return false
	}
	return true
}

func firstMapValue(values map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(values[key]); value != "" {
			return value
		}
	}
	return ""
}

func firstMapSecretValue(values map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := values[key]; value != "" {
			return value
		}
	}
	return ""
}
