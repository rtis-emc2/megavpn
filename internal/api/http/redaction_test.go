package http

import (
	"testing"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

func TestRedactedJobRemovesSensitivePayloadAndResultFields(t *testing.T) {
	job := domain.Job{
		Payload: map[string]any{
			"node_id":              "node-1",
			"new_agent_token":      "secret-token",
			"new_agent_token_hash": "secret-verifier",
			"new_token_hint":       "abcd...1234",
			"nested": map[string]any{
				"password":       "secret-password",
				"download_token": "secret-download-ticket",
			},
		},
		Result: map[string]any{
			"agent_bootstrapenv": "MEGAVPN_AGENT_ENROLLMENT_TOKEN=secret",
			"enrollment_hint":    "abcd...1234",
		},
	}

	got := redactedJob(job)
	if got.Payload["new_agent_token"] != redactedValue {
		t.Fatalf("new_agent_token was not redacted: %#v", got.Payload)
	}
	if got.Payload["new_agent_token_hash"] != redactedValue {
		t.Fatalf("new_agent_token_hash was not redacted: %#v", got.Payload)
	}
	if got.Payload["new_token_hint"] != "abcd...1234" {
		t.Fatalf("new_token_hint should remain visible: %#v", got.Payload)
	}
	nested, ok := got.Payload["nested"].(map[string]any)
	if !ok || nested["password"] != redactedValue {
		t.Fatalf("nested password was not redacted: %#v", got.Payload["nested"])
	}
	if nested["download_token"] != redactedValue {
		t.Fatalf("nested download token was not redacted: %#v", got.Payload["nested"])
	}
	if got.Result["agent_bootstrapenv"] != redactedValue {
		t.Fatalf("agent_bootstrapenv was not redacted: %#v", got.Result)
	}
}

func TestRedactedNodeCapabilityInstallEventsRemoveSensitivePayload(t *testing.T) {
	events := []domain.NodeCapabilityInstallEvent{{
		Payload: map[string]any{
			"binary_repository": map[string]any{
				"download_token": "secret-ticket",
				"ticket_hint":    "abcd...wxyz",
			},
		},
	}}

	got := redactedNodeCapabilityInstallEvents(events)
	repo, ok := got[0].Payload["binary_repository"].(map[string]any)
	if !ok {
		t.Fatalf("binary_repository type = %T", got[0].Payload["binary_repository"])
	}
	if repo["download_token"] != redactedValue {
		t.Fatalf("download_token was not redacted: %#v", repo)
	}
	if repo["ticket_hint"] != "abcd...wxyz" {
		t.Fatalf("ticket_hint should remain visible: %#v", repo)
	}
}

func TestRedactedBootstrapRunRemovesManualBundleSecret(t *testing.T) {
	run := domain.NodeBootstrapRun{
		ResultPayload: map[string]any{
			"agent_env":                        "MEGAVPN_AGENT_CONTROL_PLANE_URL=https://control.example.com",
			"agent_bootstrapenv":               "MEGAVPN_AGENT_ENROLLMENT_TOKEN=secret",
			"agent_bootstrapenv_secret_ref_id": "secret-ref-id",
		},
	}

	got := redactedBootstrapRun(run)
	if got.ResultPayload["agent_bootstrapenv"] != nil {
		t.Fatalf("agent_bootstrapenv should be removed: %#v", got.ResultPayload)
	}
	if got.ResultPayload["agent_bootstrapenv_secret_ref_id"] != nil {
		t.Fatalf("secret ref id should be removed: %#v", got.ResultPayload)
	}
	if !got.ManualBundleAvailable {
		t.Fatalf("manual_bundle_available = false, want true: %#v", got)
	}
}

func TestRedactedBootstrapRunManualBundleUnavailable(t *testing.T) {
	run := domain.NodeBootstrapRun{
		ResultPayload: map[string]any{
			"agent_env":                    "MEGAVPN_AGENT_CONTROL_PLANE_URL=https://control.example.com",
			"agent_bootstrapenv_available": true,
		},
	}

	got := redactedBootstrapRun(run)
	if got.ManualBundleAvailable {
		t.Fatalf("manual_bundle_available = true, want false: %#v", got)
	}
	if got.ResultPayload["agent_env"] != "MEGAVPN_AGENT_CONTROL_PLANE_URL=https://control.example.com" {
		t.Fatalf("non-secret agent_env should remain visible: %#v", got.ResultPayload)
	}
}

func TestRedactedNodeDiagnosticsUsesSafeBootstrapProjection(t *testing.T) {
	run := domain.NodeBootstrapRun{
		ResultPayload: map[string]any{
			"agent_bootstrapenv_secret_ref_id": "secret-ref-id",
			"agent_bootstrapenv":               "MEGAVPN_AGENT_ENROLLMENT_TOKEN=secret",
		},
	}
	diag := domain.NodeDiagnostics{
		LastBootstrap:           &run,
		LastSuccessfulBootstrap: &run,
	}

	got := redactedNodeDiagnostics(diag)
	if got.LastBootstrap == nil || !got.LastBootstrap.ManualBundleAvailable {
		t.Fatalf("last bootstrap projection = %#v", got.LastBootstrap)
	}
	if got.LastSuccessfulBootstrap == nil || !got.LastSuccessfulBootstrap.ManualBundleAvailable {
		t.Fatalf("last successful bootstrap projection = %#v", got.LastSuccessfulBootstrap)
	}
	for _, projected := range []*domain.NodeBootstrapRun{got.LastBootstrap, got.LastSuccessfulBootstrap} {
		if projected.ResultPayload["agent_bootstrapenv_secret_ref_id"] != nil || projected.ResultPayload["agent_bootstrapenv"] != nil {
			t.Fatalf("diagnostics leaked bundle fields: %#v", projected.ResultPayload)
		}
	}
}

func TestRedactedJobRemovesRuntimeFileSecrets(t *testing.T) {
	job := domain.Job{
		Result: map[string]any{
			"files": []any{
				map[string]any{"path": "/etc/wireguard/wg0.conf", "content": "PrivateKey = very-secret"},
			},
			"settings": map[string]any{
				"privateKey":   "xray-reality-secret",
				"content_hash": "safe-hash",
				"pem":          "-----BEGIN PRIVATE KEY-----\nsecret\n-----END PRIVATE KEY-----",
			},
		},
	}

	got := redactedJob(job)
	files := got.Result["files"].([]any)
	file := files[0].(map[string]any)
	if file["content"] != redactedValue {
		t.Fatalf("file content was not redacted: %#v", file)
	}
	config := got.Result["settings"].(map[string]any)
	if config["privateKey"] != redactedValue {
		t.Fatalf("camel-case privateKey was not redacted: %#v", config)
	}
	if config["pem"] != redactedValue {
		t.Fatalf("PEM private key content was not redacted: %#v", config)
	}
	if config["content_hash"] != "safe-hash" {
		t.Fatalf("content_hash should remain visible: %#v", config)
	}
}
