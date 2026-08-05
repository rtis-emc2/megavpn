package accesspolicy

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

const (
	maxVLESSPolicyBytes = 64 * 1024
	maxVLESSPolicyRules = 128
	maxVLESSRuleValues  = 256
)

// NormalizeVLESS validates and canonicalizes operator-controlled Xray policy.
// It is used on both write and render paths so a stale or manually modified
// database row cannot bypass the policy boundary.
func NormalizeVLESS(raw json.RawMessage, externalProvider bool) (json.RawMessage, error) {
	if len(strings.TrimSpace(string(raw))) == 0 {
		raw = json.RawMessage(`{}`)
	}
	if len(raw) > maxVLESSPolicyBytes {
		return nil, fmt.Errorf("policy_json must not exceed %d bytes", maxVLESSPolicyBytes)
	}
	policy := map[string]any{}
	if err := json.Unmarshal(raw, &policy); err != nil {
		return nil, fmt.Errorf("invalid policy_json: %w", err)
	}
	allowed := map[string]struct{}{
		"access_mode": {}, "egress_mode": {}, "egress_node_id": {},
		"target_instance_id": {}, "outbound_tag": {}, "ad_block": {},
		"rules": {}, "extra_rules": {},
	}
	for key := range policy {
		if _, ok := allowed[key]; !ok {
			return nil, fmt.Errorf("policy_json field %q is not supported", key)
		}
	}

	accessMode, err := policyString(policy, "access_mode", "instance_default")
	if err != nil {
		return nil, err
	}
	accessMode = strings.ToLower(accessMode)
	egressMode, err := policyString(policy, "egress_mode", "default")
	if err != nil {
		return nil, err
	}
	egressMode = strings.ToLower(egressMode)
	egressNodeID, err := policyString(policy, "egress_node_id", "")
	if err != nil {
		return nil, err
	}
	egressNodeID = strings.ToLower(egressNodeID)
	targetInstanceID, err := policyString(policy, "target_instance_id", "")
	if err != nil {
		return nil, err
	}
	targetInstanceID = strings.ToLower(targetInstanceID)
	outboundTag, err := policyString(policy, "outbound_tag", "direct")
	if err != nil {
		return nil, err
	}
	if !oneOf(accessMode, "instance_default", "local_breakout", "egress_node", "instance_only", "block") {
		return nil, fmt.Errorf("unsupported access_mode %q", accessMode)
	}
	if !oneOf(egressMode, "default", "local_breakout", "egress_node", "instance_only", "block") {
		return nil, fmt.Errorf("unsupported egress_mode %q", egressMode)
	}
	if !runtimeIdentifier(outboundTag, 64) {
		return nil, fmt.Errorf("outbound_tag must contain only letters, digits, dot, underscore, colon or hyphen")
	}
	if egressNodeID != "" && !uuidText(egressNodeID) {
		return nil, fmt.Errorf("egress_node_id must be a UUID")
	}
	if targetInstanceID != "" && !uuidText(targetInstanceID) {
		return nil, fmt.Errorf("target_instance_id must be a UUID")
	}
	if !externalProvider {
		switch accessMode {
		case "egress_node":
			if egressMode != "egress_node" || egressNodeID == "" {
				return nil, fmt.Errorf("egress_node access requires egress_mode=egress_node and egress_node_id")
			}
		case "instance_only":
			if egressMode != "instance_only" || targetInstanceID == "" {
				return nil, fmt.Errorf("instance_only access requires egress_mode=instance_only and target_instance_id")
			}
		case "local_breakout":
			if egressMode != "local_breakout" {
				return nil, fmt.Errorf("local_breakout access requires egress_mode=local_breakout")
			}
		case "block":
			if egressMode != "block" || outboundTag != "block" {
				return nil, fmt.Errorf("block access requires egress_mode=block and outbound_tag=block")
			}
		case "instance_default":
			if egressMode != "default" {
				return nil, fmt.Errorf("instance_default access requires egress_mode=default")
			}
		}
	}

	adBlock := false
	if value, ok := policy["ad_block"]; ok {
		var valid bool
		adBlock, valid = value.(bool)
		if !valid {
			return nil, fmt.Errorf("policy_json.ad_block must be a boolean")
		}
	}
	rules, err := validateRules(policy["rules"], "rules")
	if err != nil {
		return nil, err
	}
	extraRules, err := validateRules(policy["extra_rules"], "extra_rules")
	if err != nil {
		return nil, err
	}
	normalized := map[string]any{
		"access_mode": accessMode, "egress_mode": egressMode,
		"egress_node_id": egressNodeID, "target_instance_id": targetInstanceID,
		"outbound_tag": outboundTag, "ad_block": adBlock,
		"rules": rules, "extra_rules": extraRules,
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return nil, fmt.Errorf("encode policy_json: %w", err)
	}
	return encoded, nil
}

func DecodeVLESS(group domain.ClientAccessGroup) (domain.VLESSAccessPolicy, error) {
	external := group.ExternalEgressProfileID != nil && strings.TrimSpace(*group.ExternalEgressProfileID) != ""
	normalized, err := NormalizeVLESS(group.PolicyJSON, external)
	if err != nil {
		return domain.VLESSAccessPolicy{}, err
	}
	var policy struct {
		AccessMode       string           `json:"access_mode"`
		EgressMode       string           `json:"egress_mode"`
		EgressNodeID     string           `json:"egress_node_id"`
		TargetInstanceID string           `json:"target_instance_id"`
		OutboundTag      string           `json:"outbound_tag"`
		AdBlock          bool             `json:"ad_block"`
		Rules            []map[string]any `json:"rules"`
		ExtraRules       []map[string]any `json:"extra_rules"`
	}
	if err := json.Unmarshal(normalized, &policy); err != nil {
		return domain.VLESSAccessPolicy{}, fmt.Errorf("decode normalized policy_json: %w", err)
	}
	key := strings.TrimSpace(group.GroupKey)
	if key == "" {
		return domain.VLESSAccessPolicy{}, fmt.Errorf("group key is empty")
	}
	label := strings.TrimSpace(group.DisplayName)
	if label == "" {
		label = key
	}
	return domain.VLESSAccessPolicy{
		Key: key, Label: label, Description: strings.TrimSpace(group.Description),
		AccessMode: policy.AccessMode, EgressMode: policy.EgressMode,
		EgressNodeID: policy.EgressNodeID, TargetInstanceID: policy.TargetInstanceID,
		OutboundTag: policy.OutboundTag, AdBlock: policy.AdBlock,
		Rules: policy.Rules, ExtraRules: policy.ExtraRules,
		Status: group.Status, Source: "client_access_group", Version: 1, DisplayOrder: 100,
	}, nil
}

func policyString(policy map[string]any, key, fallback string) (string, error) {
	value, ok := policy[key]
	if !ok || value == nil {
		return fallback, nil
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("policy_json.%s must be a string", key)
	}
	return strings.TrimSpace(text), nil
}

func validateRules(raw any, field string) ([]map[string]any, error) {
	if raw == nil {
		return []map[string]any{}, nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("policy_json.%s must be an array", field)
	}
	if len(items) > maxVLESSPolicyRules {
		return nil, fmt.Errorf("policy_json.%s must not contain more than %d rules", field, maxVLESSPolicyRules)
	}
	allowed := map[string]struct{}{
		"type": {}, "domain": {}, "ip": {}, "port": {}, "sourcePort": {},
		"source_port": {}, "network": {}, "source": {}, "protocol": {},
		"attrs": {}, "outboundTag": {}, "outbound_tag": {},
	}
	out := make([]map[string]any, 0, len(items))
	for index, item := range items {
		rule, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("policy_json.%s[%d] must be an object", field, index)
		}
		if len(rule) > len(allowed) {
			return nil, fmt.Errorf("policy_json.%s[%d] contains too many fields", field, index)
		}
		clean := make(map[string]any, len(rule))
		for key, value := range rule {
			if _, ok := allowed[key]; !ok {
				return nil, fmt.Errorf("policy_json.%s[%d].%s is not supported", field, index, key)
			}
			if key == "type" {
				typeName, ok := value.(string)
				if !ok || (strings.TrimSpace(typeName) != "" && !strings.EqualFold(strings.TrimSpace(typeName), "field")) {
					return nil, fmt.Errorf("policy_json.%s[%d].type must be field", field, index)
				}
				clean[key] = "field"
				continue
			}
			if key == "outboundTag" || key == "outbound_tag" {
				tag, ok := value.(string)
				if !ok || !runtimeIdentifier(strings.TrimSpace(tag), 64) {
					return nil, fmt.Errorf("policy_json.%s[%d].%s is invalid", field, index, key)
				}
				clean["outbound_tag"] = strings.TrimSpace(tag)
				continue
			}
			if err := validateRuleValue(value); err != nil {
				return nil, fmt.Errorf("policy_json.%s[%d].%s: %w", field, index, key, err)
			}
			clean[key] = value
		}
		if _, ok := clean["type"]; !ok {
			clean["type"] = "field"
		}
		out = append(out, clean)
	}
	return out, nil
}

func validateRuleValue(value any) error {
	switch typed := value.(type) {
	case string:
		if len(typed) > 2048 {
			return fmt.Errorf("string value is too long")
		}
		return nil
	case []any:
		if len(typed) > maxVLESSRuleValues {
			return fmt.Errorf("array contains more than %d values", maxVLESSRuleValues)
		}
		for _, item := range typed {
			text, ok := item.(string)
			if !ok || len(text) > 2048 {
				return fmt.Errorf("array values must be strings up to 2048 bytes")
			}
		}
		return nil
	default:
		return fmt.Errorf("value must be a string or an array of strings")
	}
}

func runtimeIdentifier(value string, max int) bool {
	if value == "" || len(value) > max {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || strings.ContainsRune("_.:-", r) {
			continue
		}
		return false
	}
	return true
}

func uuidText(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	for index, r := range value {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			continue
		}
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}

func oneOf(value string, options ...string) bool {
	for _, option := range options {
		if value == option {
			return true
		}
	}
	return false
}
