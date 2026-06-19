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
				"password": "secret-password",
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
	if got.Result["agent_bootstrapenv"] != redactedValue {
		t.Fatalf("agent_bootstrapenv was not redacted: %#v", got.Result)
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
	if got.ResultPayload["agent_bootstrapenv"] != redactedValue {
		t.Fatalf("agent_bootstrapenv was not redacted: %#v", got.ResultPayload)
	}
	if got.ResultPayload["agent_bootstrapenv_secret_ref_id"] != "secret-ref-id" {
		t.Fatalf("secret ref id should remain visible: %#v", got.ResultPayload)
	}
}
