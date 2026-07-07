package jobschema

import (
	"errors"
	"fmt"
	"strings"

	"github.com/rtis-emc2/megavpn/internal/backhaul"
	"github.com/rtis-emc2/megavpn/internal/service/driver"
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
	case "platform.control_plane_tls.apply":
		// Settings are read from the database by the worker at execution time.
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
		if ids, ok, err := optionalStringSlice(payload, "dependent_instance_ids"); err != nil {
			return nil, err
		} else if ok {
			normalized["dependent_instance_ids"] = ids
		}
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
	case "node.emergency_cleanup":
		nodeID, err := requireString(payload, "node_id")
		if err != nil {
			return nil, err
		}
		confirmation, err := requireString(payload, "confirmation")
		if err != nil {
			return nil, err
		}
		normalized["node_id"] = nodeID
		normalized["confirmation"] = confirmation
		if nodeName, err := optionalString(payload, "node_name"); err != nil {
			return nil, err
		} else if nodeName != "" {
			normalized["node_name"] = nodeName
		}
		if v, ok, err := optionalBool(payload, "include_agent"); err != nil {
			return nil, err
		} else if ok {
			normalized["include_agent"] = v
		} else {
			normalized["include_agent"] = false
		}
		cleanupScope, err := optionalString(payload, "cleanup_scope")
		if err != nil {
			return nil, err
		}
		includeAgent, _ := normalized["include_agent"].(bool)
		cleanupScope = normalizeNodeCleanupScope(cleanupScope, includeAgent)
		normalized["cleanup_scope"] = cleanupScope
	case "node.reboot":
		nodeID, err := requireString(payload, "node_id")
		if err != nil {
			return nil, err
		}
		confirmation, err := requireString(payload, "confirmation")
		if err != nil {
			return nil, err
		}
		normalized["node_id"] = nodeID
		normalized["confirmation"] = confirmation
		if nodeName, err := optionalString(payload, "node_name"); err != nil {
			return nil, err
		} else if nodeName != "" {
			normalized["node_name"] = nodeName
		}
		if reason, err := optionalString(payload, "reason"); err != nil {
			return nil, err
		} else if reason != "" {
			normalized["reason"] = reason
		}
	case "node.backhaul.apply":
		nodeID, err := requireString(payload, "node_id")
		if err != nil {
			return nil, err
		}
		linkID, err := requireString(payload, "link_id")
		if err != nil {
			return nil, err
		}
		transportID, err := requireString(payload, "transport_id")
		if err != nil {
			return nil, err
		}
		role, err := requireString(payload, "role")
		if err != nil {
			return nil, err
		}
		role = strings.ToLower(role)
		if role != "ingress" && role != "egress" {
			return nil, validationf("payload.role must be ingress or egress")
		}
		driver, err := requireString(payload, "driver")
		if err != nil {
			return nil, err
		}
		driver = backhaul.NormalizeDriver(driver)
		if err := backhaul.ValidateDriver(driver); err != nil {
			return nil, validationf("%s", err.Error())
		}
		normalized["node_id"] = nodeID
		normalized["link_id"] = linkID
		normalized["transport_id"] = transportID
		normalized["role"] = role
		normalized["driver"] = driver
		trimOptionalStrings(normalized, payload, "link_name", "interface_name", "routing_table", "endpoint_host", "protocol", "tunnel_cidr", "ingress_address", "egress_address", "output_path", "systemd_unit", "activation_mode", "driver_layer")
		if v, ok, err := optionalInt(payload, "endpoint_port"); err != nil {
			return nil, err
		} else if ok {
			normalized["endpoint_port"] = v
		}
		if v, ok, err := optionalInt(payload, "route_metric"); err != nil {
			return nil, err
		} else if ok {
			normalized["route_metric"] = v
		}
		if v, ok, err := optionalBool(payload, "activate"); err != nil {
			return nil, err
		} else if ok {
			normalized["activate"] = v
		}
		if rawFiles, ok := payload["files"]; ok {
			files, ok := rawFiles.([]any)
			if !ok {
				return nil, validationf("payload.files must be an array")
			}
			normalized["files"] = files
		} else {
			normalized["files"] = []any{}
		}
		if enforcement, err := optionalMap(payload, "enforcement"); err != nil {
			return nil, err
		} else if enforcement != nil {
			normalized["enforcement"] = enforcement
		}
		if warnings, ok, err := optionalStringSlice(payload, "warnings"); err != nil {
			return nil, err
		} else if ok {
			normalized["warnings"] = warnings
		}
	case "node.backhaul.probe":
		nodeID, err := requireString(payload, "node_id")
		if err != nil {
			return nil, err
		}
		linkID, err := requireString(payload, "link_id")
		if err != nil {
			return nil, err
		}
		transportID, err := requireString(payload, "transport_id")
		if err != nil {
			return nil, err
		}
		role, err := requireString(payload, "role")
		if err != nil {
			return nil, err
		}
		role = strings.ToLower(role)
		if role != "ingress" && role != "egress" {
			return nil, validationf("payload.role must be ingress or egress")
		}
		driverCode, err := requireString(payload, "driver")
		if err != nil {
			return nil, err
		}
		driverCode = backhaul.NormalizeDriver(driverCode)
		if err := backhaul.ValidateDriver(driverCode); err != nil {
			return nil, validationf("%s", err.Error())
		}
		normalized["node_id"] = nodeID
		normalized["link_id"] = linkID
		normalized["transport_id"] = transportID
		normalized["role"] = role
		normalized["driver"] = driverCode
		trimOptionalStrings(normalized, payload, "interface_name", "systemd_unit", "peer_address")
		if v, ok, err := optionalInt(payload, "probe_count"); err != nil {
			return nil, err
		} else if ok {
			normalized["probe_count"] = v
		}
	case "node.backhaul.cleanup":
		nodeID, err := requireString(payload, "node_id")
		if err != nil {
			return nil, err
		}
		linkID, err := requireString(payload, "link_id")
		if err != nil {
			return nil, err
		}
		transportID, err := requireString(payload, "transport_id")
		if err != nil {
			return nil, err
		}
		role, err := requireString(payload, "role")
		if err != nil {
			return nil, err
		}
		role = strings.ToLower(role)
		if role != "ingress" && role != "egress" {
			return nil, validationf("payload.role must be ingress or egress")
		}
		driverCode, err := requireString(payload, "driver")
		if err != nil {
			return nil, err
		}
		driverCode = backhaul.NormalizeDriver(driverCode)
		if err := backhaul.ValidateDriver(driverCode); err != nil {
			return nil, validationf("%s", err.Error())
		}
		normalized["node_id"] = nodeID
		normalized["link_id"] = linkID
		normalized["transport_id"] = transportID
		normalized["role"] = role
		normalized["driver"] = driverCode
		trimOptionalStrings(normalized, payload, "interface_name", "delete_batch_id", "route_disable_batch_id")
		if routeAction, err := optionalString(payload, "route_action"); err != nil {
			return nil, err
		} else if routeAction != "" {
			routeAction = strings.ToLower(routeAction)
			if routeAction != "disable" {
				return nil, validationf("payload.route_action must be disable")
			}
			normalized["route_action"] = routeAction
		}
		if systemdUnits, ok, err := optionalStringSlice(payload, "systemd_units"); err != nil {
			return nil, err
		} else if ok {
			normalized["systemd_units"] = systemdUnits
		}
		if paths, ok, err := optionalStringSlice(payload, "paths"); err != nil {
			return nil, err
		} else if ok {
			normalized["paths"] = paths
		}
		if directories, ok, err := optionalStringSlice(payload, "directories"); err != nil {
			return nil, err
		} else if ok {
			normalized["directories"] = directories
		}
	case "node.route_policy.apply":
		nodeID, err := requireString(payload, "node_id")
		if err != nil {
			return nil, err
		}
		normalized["node_id"] = nodeID
		if revision, err := optionalString(payload, "revision"); err != nil {
			return nil, err
		} else if revision != "" {
			normalized["revision"] = revision
		}
		if generatedAt, err := optionalString(payload, "generated_at"); err != nil {
			return nil, err
		} else if generatedAt != "" {
			normalized["generated_at"] = generatedAt
		}
		if outputPath, err := optionalString(payload, "output_path"); err != nil {
			return nil, err
		} else if outputPath != "" {
			normalized["output_path"] = outputPath
		}
		if enforcementMode, err := optionalString(payload, "enforcement_mode"); err != nil {
			return nil, err
		} else if enforcementMode != "" {
			normalized["enforcement_mode"] = strings.ToLower(enforcementMode)
		}
		if rawRoutes, ok := payload["routes"]; ok {
			routes, err := normalizeObjectArray(rawRoutes, "payload.routes")
			if err != nil {
				return nil, err
			}
			normalized["routes"] = routes
		} else {
			normalized["routes"] = []any{}
		}
		if rawRoutes, ok := payload["system_routes"]; ok {
			routes, err := normalizeObjectArray(rawRoutes, "payload.system_routes")
			if err != nil {
				return nil, err
			}
			normalized["system_routes"] = routes
		} else {
			normalized["system_routes"] = []any{}
		}
		if v, ok, err := optionalInt(payload, "route_count"); err != nil {
			return nil, err
		} else if ok {
			normalized["route_count"] = v
		}
		if v, ok, err := optionalInt(payload, "active_route_count"); err != nil {
			return nil, err
		} else if ok {
			normalized["active_route_count"] = v
		}
		if v, ok, err := optionalInt(payload, "system_route_count"); err != nil {
			return nil, err
		} else if ok {
			normalized["system_route_count"] = v
		}
		if v, ok, err := optionalInt(payload, "active_system_route_count"); err != nil {
			return nil, err
		} else if ok {
			normalized["active_system_route_count"] = v
		}
	case "node.route_policy.cleanup":
		nodeID, err := requireString(payload, "node_id")
		if err != nil {
			return nil, err
		}
		normalized["node_id"] = nodeID
		trimOptionalStrings(normalized, payload, "revision", "generated_at", "output_path", "cleanup_reason")
		if rawRoutes, ok := payload["routes"]; ok {
			routes, err := normalizeObjectArray(rawRoutes, "payload.routes")
			if err != nil {
				return nil, err
			}
			normalized["routes"] = routes
		} else {
			normalized["routes"] = []any{}
		}
		if rawRoutes, ok := payload["system_routes"]; ok {
			routes, err := normalizeObjectArray(rawRoutes, "payload.system_routes")
			if err != nil {
				return nil, err
			}
			normalized["system_routes"] = routes
		} else {
			normalized["system_routes"] = []any{}
		}
	case "node.firewall.preview", "node.firewall.apply", "node.firewall.observe", "node.firewall.disable":
		nodeID, err := requireString(payload, "node_id")
		if err != nil {
			return nil, err
		}
		normalized["node_id"] = nodeID
		trimOptionalStrings(normalized, payload, "policy_id", "policy_key", "revision_id", "disable_reason")
		if v, ok, err := optionalInt(payload, "revision_no"); err != nil {
			return nil, err
		} else if ok {
			normalized["revision_no"] = v
		}
		for _, key := range []string{"default_input_policy", "default_forward_policy", "default_output_policy"} {
			value, err := optionalString(payload, key)
			if err != nil {
				return nil, err
			}
			value = strings.ToLower(value)
			if value == "" {
				value = "accept"
			}
			if value != "accept" && value != "drop" && value != "reject" {
				return nil, validationf("payload.%s must be accept, drop or reject", key)
			}
			normalized[key] = value
		}
		if v, ok, err := optionalBool(payload, "enforce_default_policy"); err != nil {
			return nil, err
		} else if ok {
			normalized["enforce_default_policy"] = v
		} else {
			normalized["enforce_default_policy"] = false
		}
		if rawRules, ok := payload["rules"]; ok {
			rules, ok := rawRules.([]any)
			if !ok {
				return nil, validationf("payload.rules must be an array")
			}
			normalized["rules"] = rules
		} else {
			normalized["rules"] = []any{}
		}
		if rawLists, ok := payload["address_lists"]; ok {
			lists, ok := rawLists.([]any)
			if !ok {
				return nil, validationf("payload.address_lists must be an array")
			}
			normalized["address_lists"] = lists
		} else {
			normalized["address_lists"] = []any{}
		}
	case "node.agent.rotate_token":
		nodeID, err := requireString(payload, "node_id")
		if err != nil {
			return nil, err
		}
		newTokenRefID, err := optionalString(payload, "new_agent_token_secret_ref_id")
		if err != nil {
			return nil, err
		}
		newTokenHash, err := optionalString(payload, "new_agent_token_hash")
		if err != nil {
			return nil, err
		}
		legacyNewToken, err := optionalString(payload, "new_agent_token")
		if err != nil {
			return nil, err
		}
		if newTokenRefID == "" || newTokenHash == "" {
			return nil, validationf("payload.new_agent_token_secret_ref_id and payload.new_agent_token_hash are required")
		}
		if legacyNewToken != "" {
			return nil, validationf("payload.new_agent_token must not be stored in job payload; use payload.new_agent_token_secret_ref_id")
		}
		normalized["node_id"] = nodeID
		normalized["new_agent_token_secret_ref_id"] = newTokenRefID
		normalized["new_agent_token_hash"] = newTokenHash
		if hint, err := optionalString(payload, "new_token_hint"); err != nil {
			return nil, err
		} else if hint != "" {
			normalized["new_token_hint"] = hint
		}
	case "instance.apply", "instance.restart", "instance.start", "instance.stop", "instance.enable", "instance.disable", "instance.delete":
		instanceID, err := requireString(payload, "instance_id")
		if err != nil {
			return nil, err
		}
		action, ok := driver.OperationFromJobType(jobType)
		if !ok {
			return nil, validationf("unsupported instance job type %q", jobType)
		}
		normalized["instance_id"] = instanceID
		normalized["action"] = action
		if serviceCode, err := optionalString(payload, "service_code"); err != nil {
			return nil, err
		} else if serviceCode != "" {
			normalizedServiceCode := driver.NormalizeCode(serviceCode)
			if !driver.SupportsOperation(normalizedServiceCode, action) {
				return nil, validationf("driver %s does not support instance operation %s", normalizedServiceCode, action)
			}
			normalized["service_code"] = normalizedServiceCode
		}
		if runtimeCode, err := optionalString(payload, "runtime_service_code"); err != nil {
			return nil, err
		} else if runtimeCode != "" {
			normalizedRuntimeCode := driver.NormalizeCode(runtimeCode)
			if !driver.SupportsOperation(normalizedRuntimeCode, action) {
				return nil, validationf("runtime driver %s does not support instance operation %s", normalizedRuntimeCode, action)
			}
			normalized["runtime_service_code"] = normalizedRuntimeCode
		}
		if stringifyAny(normalized["service_code"]) == "" && stringifyAny(normalized["runtime_service_code"]) == "" && strings.TrimSpace(stringifyAny(payload["systemd_unit"])) == "" {
			return nil, validationf("payload.service_code, payload.runtime_service_code or payload.systemd_unit is required")
		}
		if action == "apply" || action == "delete" {
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
	case "instance.diagnose":
		instanceID, err := requireString(payload, "instance_id")
		if err != nil {
			return nil, err
		}
		normalized["instance_id"] = instanceID
		normalized["action"] = "diagnose"
		if serviceCode, err := optionalString(payload, "service_code"); err != nil {
			return nil, err
		} else if serviceCode != "" {
			normalized["service_code"] = driver.NormalizeCode(serviceCode)
		}
		if runtimeCode, err := optionalString(payload, "runtime_service_code"); err != nil {
			return nil, err
		} else if runtimeCode != "" {
			normalized["runtime_service_code"] = driver.NormalizeCode(runtimeCode)
		}
		if stringifyAny(normalized["service_code"]) == "" && stringifyAny(normalized["runtime_service_code"]) == "" && strings.TrimSpace(stringifyAny(payload["systemd_unit"])) == "" {
			return nil, validationf("payload.service_code, payload.runtime_service_code or payload.systemd_unit is required")
		}
		if spec, err := optionalMap(payload, "spec"); err != nil {
			return nil, err
		} else if spec != nil {
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
		if options, err := optionalMap(payload, "service_options"); err != nil {
			return nil, err
		} else if options != nil {
			normalized["service_options"] = options
		}
	case "artifact.build":
		clientID, err := requireString(payload, "client_id")
		if err != nil {
			return nil, err
		}
		artifactType, err := optionalString(payload, "artifact_type")
		if err != nil {
			return nil, err
		}
		if artifactType == "" {
			artifactType, err = optionalString(payload, "type")
			if err != nil {
				return nil, err
			}
		}
		artifactType = strings.ToLower(strings.TrimSpace(artifactType))
		if artifactType == "" {
			artifactType = "all"
		}
		if !isAllowedArtifactType(artifactType) {
			return nil, validationf("payload.artifact_type is unsupported")
		}
		normalized["client_id"] = clientID
		normalized["artifact_type"] = artifactType
		delete(normalized, "type")
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
	default:
		return nil, validationf("unsupported job type %q", jobType)
	}
	return normalized, nil
}

func isAllowedArtifactType(artifactType string) bool {
	return driver.IsSupportedArtifactType(artifactType)
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

func normalizeNodeCleanupScope(value string, includeAgent bool) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "services_only", "service", "services", "instances", "instance_runtime":
		return "services_only"
	case "full_node", "full", "node", "all", "wipe":
		return "full_node"
	}
	if includeAgent {
		return "full_node"
	}
	return "services_only"
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
	if ok {
		return value, nil
	}
	if typed, ok := raw.(map[string]map[string]any); ok {
		value := make(map[string]any, len(typed))
		for nestedKey, nestedValue := range typed {
			value[nestedKey] = nestedValue
		}
		return value, nil
	}
	return nil, validationf("payload.%s must be an object", key)
}

func optionalMap(payload map[string]any, key string) (map[string]any, error) {
	raw, ok := payload[key]
	if !ok || raw == nil {
		return nil, nil
	}
	value, ok := raw.(map[string]any)
	if ok {
		return value, nil
	}
	if typed, ok := raw.(map[string]map[string]any); ok {
		value := make(map[string]any, len(typed))
		for nestedKey, nestedValue := range typed {
			value[nestedKey] = nestedValue
		}
		return value, nil
	}
	return nil, validationf("payload.%s must be an object", key)
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

func normalizeObjectArray(value any, field string) ([]any, error) {
	switch items := value.(type) {
	case []any:
		out := make([]any, 0, len(items))
		for idx, item := range items {
			if _, ok := item.(map[string]any); !ok {
				return nil, validationf("%s[%d] must be an object", field, idx)
			}
			out = append(out, item)
		}
		return out, nil
	case []map[string]any:
		out := make([]any, 0, len(items))
		for _, item := range items {
			out = append(out, item)
		}
		return out, nil
	default:
		return nil, validationf("%s must be an array", field)
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
