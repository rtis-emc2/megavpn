package http

import (
	"strings"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

const redactedValue = "[redacted]"

func redactedJob(job domain.Job) domain.Job {
	job.Payload = redactSensitiveMap(job.Payload)
	job.Result = redactSensitiveMap(job.Result)
	return job
}

func redactedJobs(jobs []domain.Job) []domain.Job {
	if jobs == nil {
		return nil
	}
	out := make([]domain.Job, len(jobs))
	for i := range jobs {
		out[i] = redactedJob(jobs[i])
	}
	return out
}

func redactedServiceAccesses(accesses []domain.ServiceAccess) []domain.ServiceAccess {
	if accesses == nil {
		return nil
	}
	out := make([]domain.ServiceAccess, len(accesses))
	for i := range accesses {
		out[i] = accesses[i]
		out[i].Policy = redactSensitiveMap(accesses[i].Policy)
		out[i].Metadata = redactSensitiveMap(accesses[i].Metadata)
	}
	return out
}

func redactedInstance(instance domain.Instance) domain.Instance {
	instance.Spec = redactInstanceSpec(instance.Spec)
	return instance
}

func redactedInstances(instances []domain.Instance) []domain.Instance {
	if instances == nil {
		return nil
	}
	out := make([]domain.Instance, len(instances))
	for i := range instances {
		out[i] = redactedInstance(instances[i])
	}
	return out
}

func redactedInstanceRevision(revision domain.InstanceRevision) domain.InstanceRevision {
	revision.Spec = redactInstanceSpec(revision.Spec)
	return revision
}

func redactedInstanceRevisions(revisions []domain.InstanceRevision) []domain.InstanceRevision {
	if revisions == nil {
		return nil
	}
	out := make([]domain.InstanceRevision, len(revisions))
	for i := range revisions {
		out[i] = redactedInstanceRevision(revisions[i])
	}
	return out
}

func redactedInstanceRuntimeObservations(observations []domain.InstanceRuntimeObservation) []domain.InstanceRuntimeObservation {
	if observations == nil {
		return nil
	}
	out := make([]domain.InstanceRuntimeObservation, len(observations))
	for i := range observations {
		out[i] = observations[i]
		out[i].Result = redactSensitiveMap(observations[i].Result)
	}
	return out
}

func redactedBootstrapRun(run domain.NodeBootstrapRun) domain.NodeBootstrapRun {
	run.RequestPayload = redactSensitiveMap(run.RequestPayload)
	run.ResultPayload = redactSensitiveMap(run.ResultPayload)
	return run
}

func redactedBootstrapRuns(runs []domain.NodeBootstrapRun) []domain.NodeBootstrapRun {
	if runs == nil {
		return nil
	}
	out := make([]domain.NodeBootstrapRun, len(runs))
	for i := range runs {
		out[i] = redactedBootstrapRun(runs[i])
	}
	return out
}

func redactedNodeDiagnostics(diag domain.NodeDiagnostics) domain.NodeDiagnostics {
	if diag.LastBootstrap != nil {
		run := redactedBootstrapRun(*diag.LastBootstrap)
		diag.LastBootstrap = &run
	}
	if diag.LastSuccessfulBootstrap != nil {
		run := redactedBootstrapRun(*diag.LastSuccessfulBootstrap)
		diag.LastSuccessfulBootstrap = &run
	}
	if diag.LastFailedBootstrap != nil {
		run := redactedBootstrapRun(*diag.LastFailedBootstrap)
		diag.LastFailedBootstrap = &run
	}
	return diag
}

func redactedNodeCapabilityInstallEvent(event domain.NodeCapabilityInstallEvent) domain.NodeCapabilityInstallEvent {
	event.Payload = redactSensitiveMap(event.Payload)
	return event
}

func redactedNodeCapabilityInstallEvents(events []domain.NodeCapabilityInstallEvent) []domain.NodeCapabilityInstallEvent {
	if events == nil {
		return nil
	}
	out := make([]domain.NodeCapabilityInstallEvent, len(events))
	for i := range events {
		out[i] = redactedNodeCapabilityInstallEvent(events[i])
	}
	return out
}

func redactSensitiveMap(src map[string]any) map[string]any {
	return redactMapWithKeyPolicy(src, isSensitiveResponseKey)
}

func redactInstanceSpec(src map[string]any) map[string]any {
	out := redactMapWithKeyPolicy(src, isSensitiveInstanceSpecKey)
	clients, ok := out["managed_clients"].([]any)
	if !ok {
		return out
	}
	for _, item := range clients {
		client, ok := item.(map[string]any)
		if !ok {
			continue
		}
		for _, key := range []string{"id", "uuid", "xray_uuid", "vless_uuid"} {
			if _, exists := client[key]; exists {
				client[key] = redactedValue
			}
		}
	}
	return out
}

func redactMapWithKeyPolicy(src map[string]any, sensitiveKey func(string) bool) map[string]any {
	if src == nil {
		return nil
	}
	out := make(map[string]any, len(src))
	for key, value := range src {
		if sensitiveKey(key) {
			out[key] = redactedValue
			continue
		}
		out[key] = redactValueWithKeyPolicy(value, sensitiveKey)
	}
	return out
}

func redactValueWithKeyPolicy(value any, sensitiveKey func(string) bool) any {
	switch typed := value.(type) {
	case map[string]any:
		return redactMapWithKeyPolicy(typed, sensitiveKey)
	case []any:
		out := make([]any, len(typed))
		for i := range typed {
			out[i] = redactValueWithKeyPolicy(typed[i], sensitiveKey)
		}
		return out
	case string:
		if containsSensitiveResponseString(typed) {
			return redactedValue
		}
		return typed
	default:
		return value
	}
}

func isSensitiveInstanceSpecKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	switch normalized {
	case "content", "json", "config", "config_json", "config_content":
		return false
	default:
		return isSensitiveResponseKey(key)
	}
}

func isSensitiveResponseKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	if normalized == "" {
		return false
	}
	if strings.Contains(normalized, "hint") {
		return false
	}
	if strings.HasSuffix(normalized, "secret_ref_id") {
		return false
	}
	compact := strings.ReplaceAll(normalized, "_", "")
	switch normalized {
	case "token", "agent_token", "new_agent_token", "new_agent_token_hash", "enrollment_token",
		"password", "smtp_password", "private_key", "secret", "psk",
		"uuid", "xray_uuid", "vless_uuid",
		"agent_bootstrapenv", "agent_bootstrap_env", "bootstrap_env",
		"content", "json", "config", "config_json", "config_content", "privatekey", "presharedkey",
		"reality_private_key", "wireguard_private_key", "openvpn_private_key", "tls_private_key":
		return true
	}
	switch compact {
	case "privatekey", "presharedkey", "agentbootstrapenv", "bootstrapenv", "configjson", "configcontent":
		return true
	}
	if strings.HasSuffix(normalized, "_token") || strings.HasSuffix(normalized, "_token_hash") {
		return true
	}
	if strings.Contains(normalized, "password") || strings.Contains(normalized, "private_key") || strings.Contains(compact, "privatekey") {
		return true
	}
	if strings.Contains(normalized, "secret") {
		return true
	}
	return false
}

func containsSensitiveResponseString(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return false
	}
	switch {
	case strings.Contains(normalized, "-----begin ") && strings.Contains(normalized, " private key-----"):
		return true
	case strings.Contains(normalized, "privatekey ="):
		return true
	case strings.Contains(normalized, "presharedkey ="):
		return true
	case strings.Contains(normalized, "megavpn_agent_enrollment_token="):
		return true
	case strings.Contains(normalized, "password:") || strings.Contains(normalized, `"password"`):
		return true
	default:
		return false
	}
}
