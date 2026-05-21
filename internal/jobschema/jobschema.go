package jobschema

import (
	"errors"
	"fmt"
	"strings"
)

type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func IsValidationError(err error) bool {
	var target *ValidationError
	return errors.As(err, &target)
}

func Normalize(jobType string, payload map[string]any) (map[string]any, error) {
	jobType = strings.TrimSpace(jobType)
	if payload == nil {
		payload = map[string]any{}
	}
	normalized := cloneMap(payload)
	switch jobType {
	case "node.bootstrap":
		nodeID, err := requireString(payload, "node_id")
		if err != nil {
			return nil, err
		}
		mode, err := optionalString(payload, "bootstrap_mode")
		if err != nil {
			return nil, err
		}
		if mode == "" {
			mode = "ssh_bootstrap"
		}
		if mode != "ssh_bootstrap" && mode != "manual_bundle" {
			return nil, validationf("payload.bootstrap_mode must be ssh_bootstrap or manual_bundle")
		}
		normalized["node_id"] = nodeID
		normalized["bootstrap_mode"] = mode
		if v, ok, err := optionalBool(payload, "reinstall_agent"); err != nil {
			return nil, err
		} else if ok {
			normalized["reinstall_agent"] = v
		}
		if v, ok, err := optionalBool(payload, "force_reenroll"); err != nil {
			return nil, err
		} else if ok {
			normalized["force_reenroll"] = v
		}
	case "node.capability.install":
		nodeID, err := requireString(payload, "node_id")
		if err != nil {
			return nil, err
		}
		serviceCode, err := requireString(payload, "service_code")
		if err != nil {
			return nil, err
		}
		strategy, err := requireString(payload, "strategy")
		if err != nil {
			return nil, err
		}
		channel, err := requireString(payload, "channel")
		if err != nil {
			return nil, err
		}
		normalized["node_id"] = nodeID
		normalized["service_code"] = strings.ToLower(serviceCode)
		normalized["strategy"] = strategy
		normalized["channel"] = channel
	case "node.capability.verify":
		nodeID, err := requireString(payload, "node_id")
		if err != nil {
			return nil, err
		}
		serviceCode, err := requireString(payload, "service_code")
		if err != nil {
			return nil, err
		}
		normalized["node_id"] = nodeID
		normalized["service_code"] = strings.ToLower(serviceCode)
	case "node.inventory", "node.inventory.sync", "node.services.discover", "node.channel.probe":
		nodeID, err := requireString(payload, "node_id")
		if err != nil {
			return nil, err
		}
		normalized["node_id"] = nodeID
	case "node.agent.rotate_token":
		nodeID, err := requireString(payload, "node_id")
		if err != nil {
			return nil, err
		}
		newToken, err := requireString(payload, "new_agent_token")
		if err != nil {
			return nil, err
		}
		normalized["node_id"] = nodeID
		normalized["new_agent_token"] = newToken
		if hint, err := optionalString(payload, "new_token_hint"); err != nil {
			return nil, err
		} else if hint != "" {
			normalized["new_token_hint"] = hint
		}
	case "instance.apply", "instance.restart", "instance.start", "instance.stop", "instance.enable", "instance.disable":
		instanceID, err := requireString(payload, "instance_id")
		if err != nil {
			return nil, err
		}
		action := strings.TrimPrefix(jobType, "instance.")
		normalized["instance_id"] = instanceID
		normalized["action"] = action
		if serviceCode, err := optionalString(payload, "service_code"); err != nil {
			return nil, err
		} else if serviceCode != "" {
			normalized["service_code"] = strings.ToLower(serviceCode)
		}
		if runtimeCode, err := optionalString(payload, "runtime_service_code"); err != nil {
			return nil, err
		} else if runtimeCode != "" {
			normalized["runtime_service_code"] = strings.ToLower(runtimeCode)
		}
		if stringifyAny(normalized["service_code"]) == "" && stringifyAny(normalized["runtime_service_code"]) == "" && strings.TrimSpace(stringifyAny(payload["systemd_unit"])) == "" {
			return nil, validationf("payload.service_code, payload.runtime_service_code or payload.systemd_unit is required")
		}
		if action == "apply" {
			spec, err := requireMap(payload, "spec")
			if err != nil {
				return nil, err
			}
			normalized["spec"] = spec
		}
		trimOptionalStrings(normalized, payload, "name", "slug", "systemd_unit", "endpoint_host")
		if v, ok, err := optionalInt(payload, "endpoint_port"); err != nil {
			return nil, err
		} else if ok {
			normalized["endpoint_port"] = v
		}
		if v, ok, err := optionalBool(payload, "enabled"); err != nil {
			return nil, err
		} else if ok {
			normalized["enabled"] = v
		}
	case "client.provision":
		clientID, err := requireString(payload, "client_id")
		if err != nil {
			return nil, err
		}
		normalized["client_id"] = clientID
		if ids, ok, err := optionalStringSlice(payload, "instance_ids"); err != nil {
			return nil, err
		} else if ok {
			normalized["instance_ids"] = ids
		}
	case "client.revoke":
		clientID, err := requireString(payload, "client_id")
		if err != nil {
			return nil, err
		}
		normalized["client_id"] = clientID
	}
	return normalized, nil
}

func cloneMap(src map[string]any) map[string]any {
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func trimOptionalStrings(dst, src map[string]any, keys ...string) {
	for _, key := range keys {
		if value, err := optionalString(src, key); err == nil && value != "" {
			dst[key] = value
		}
	}
}

func requireString(payload map[string]any, key string) (string, error) {
	value, err := optionalString(payload, key)
	if err != nil {
		return "", err
	}
	if value == "" {
		return "", validationf("payload.%s is required", key)
	}
	return value, nil
}

func optionalString(payload map[string]any, key string) (string, error) {
	raw, ok := payload[key]
	if !ok || raw == nil {
		return "", nil
	}
	switch value := raw.(type) {
	case string:
		return strings.TrimSpace(value), nil
	case fmt.Stringer:
		return strings.TrimSpace(value.String()), nil
	default:
		return "", validationf("payload.%s must be a string", key)
	}
}

func requireMap(payload map[string]any, key string) (map[string]any, error) {
	raw, ok := payload[key]
	if !ok || raw == nil {
		return nil, validationf("payload.%s is required", key)
	}
	value, ok := raw.(map[string]any)
	if !ok {
		return nil, validationf("payload.%s must be an object", key)
	}
	return value, nil
}

func optionalBool(payload map[string]any, key string) (bool, bool, error) {
	raw, ok := payload[key]
	if !ok || raw == nil {
		return false, false, nil
	}
	switch value := raw.(type) {
	case bool:
		return value, true, nil
	case string:
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "true", "1", "yes":
			return true, true, nil
		case "false", "0", "no":
			return false, true, nil
		}
	case float64:
		if value == 1 {
			return true, true, nil
		}
		if value == 0 {
			return false, true, nil
		}
	}
	return false, false, validationf("payload.%s must be a boolean", key)
}

func optionalInt(payload map[string]any, key string) (int, bool, error) {
	raw, ok := payload[key]
	if !ok || raw == nil {
		return 0, false, nil
	}
	switch value := raw.(type) {
	case int:
		return value, true, nil
	case int32:
		return int(value), true, nil
	case int64:
		return int(value), true, nil
	case float64:
		return int(value), true, nil
	}
	return 0, false, validationf("payload.%s must be a number", key)
}

func optionalStringSlice(payload map[string]any, key string) ([]string, bool, error) {
	raw, ok := payload[key]
	if !ok || raw == nil {
		return nil, false, nil
	}
	switch value := raw.(type) {
	case []string:
		out := make([]string, 0, len(value))
		for _, item := range value {
			item = strings.TrimSpace(item)
			if item != "" {
				out = append(out, item)
			}
		}
		return out, true, nil
	case []any:
		out := make([]string, 0, len(value))
		for _, item := range value {
			s, ok := item.(string)
			if !ok {
				return nil, false, validationf("payload.%s must be an array of strings", key)
			}
			s = strings.TrimSpace(s)
			if s != "" {
				out = append(out, s)
			}
		}
		return out, true, nil
	default:
		return nil, false, validationf("payload.%s must be an array of strings", key)
	}
}

func stringifyAny(v any) string {
	switch value := v.(type) {
	case string:
		return strings.TrimSpace(value)
	case fmt.Stringer:
		return strings.TrimSpace(value.String())
	default:
		return ""
	}
}

func validationf(format string, args ...any) error {
	return &ValidationError{Message: fmt.Sprintf(format, args...)}
}
