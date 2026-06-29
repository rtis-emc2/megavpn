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
	switch normalized {
	case "token", "agent_token", "new_agent_token", "new_agent_token_hash", "enrollment_token",
		"password", "smtp_password", "private_key", "secret", "psk",
		"agent_bootstrapenv", "agent_bootstrap_env", "bootstrap_env":
		return true
	}
	if strings.HasSuffix(normalized, "_token") || strings.HasSuffix(normalized, "_token_hash") {
		return true
	}
	if strings.Contains(normalized, "password") || strings.Contains(normalized, "private_key") {
		return true
	}
	if strings.Contains(normalized, "secret") {
		return true
	}
	return false
}
