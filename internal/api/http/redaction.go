package http

import (
	"strings"
	"time"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

type operatorJobMetadata struct {
	ID         string     `json:"id"`
	Type       string     `json:"type"`
	ScopeType  string     `json:"scope_type"`
	Status     string     `json:"status"`
	ScopeID    *string    `json:"scope_id"`
	NodeID     *string    `json:"node_id"`
	InstanceID *string    `json:"instance_id"`
	Priority   int        `json:"priority"`
	CreatedAt  time.Time  `json:"created_at"`
	StartedAt  *time.Time `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
}

func operatorLifecycleJobMetadata(job domain.Job) operatorJobMetadata {
	return operatorJobMetadata{
		ID:         job.ID,
		Type:       job.Type,
		ScopeType:  job.ScopeType,
		Status:     job.Status,
		ScopeID:    job.ScopeID,
		NodeID:     job.NodeID,
		InstanceID: job.InstanceID,
		Priority:   job.Priority,
		CreatedAt:  job.CreatedAt,
		StartedAt:  job.StartedAt,
		FinishedAt: job.FinishedAt,
	}
}

const redactedValue = "[redacted]"
const nodeBootstrapBundleSecretRefKey = "agent_bootstrapenv_secret_ref_id"

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

func redactedBootstrapRun(run domain.NodeBootstrapRun) domain.NodeBootstrapRun {
	run.ManualBundleAvailable = bootstrapManualBundleAvailable(run.ResultPayload)
	run.RequestPayload = redactSensitiveMap(run.RequestPayload)
	run.ResultPayload = redactBootstrapRunResultPayload(run.ResultPayload)
	return run
}

func bootstrapManualBundleAvailable(payload map[string]any) bool {
	if payload == nil {
		return false
	}
	return strings.TrimSpace(stringifyHTTP(payload[nodeBootstrapBundleSecretRefKey])) != ""
}

func redactBootstrapRunResultPayload(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	out := redactSensitiveMap(src)
	delete(out, nodeBootstrapBundleSecretRefKey)
	delete(out, "agent_bootstrapenv")
	delete(out, "agent_bootstrap_env")
	return out
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
	if src == nil {
		return nil
	}
	out := make(map[string]any, len(src))
	for key, value := range src {
		if isSensitiveResponseKey(key) {
			out[key] = redactedValue
			continue
		}
		out[key] = redactSensitiveValue(value)
	}
	return out
}

func redactSensitiveValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return redactSensitiveMap(typed)
	case []any:
		out := make([]any, len(typed))
		for i := range typed {
			out[i] = redactSensitiveValue(typed[i])
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
