package jobschema

import "testing"

func TestNormalizeNodeBootstrap(t *testing.T) {
	payload, err := Normalize("node.bootstrap", map[string]any{
		"node_id":         " node-1 ",
		"bootstrap_mode":  " manual_bundle ",
		"reinstall_agent": "true",
		"force_reenroll":  false,
	})
	if err != nil {
		t.Fatalf("Normalize returned error: %v", err)
	}
	if got := payload["node_id"]; got != "node-1" {
		t.Fatalf("node_id = %v, want node-1", got)
	}
	if got := payload["bootstrap_mode"]; got != "manual_bundle" {
		t.Fatalf("bootstrap_mode = %v, want manual_bundle", got)
	}
	if got := payload["reinstall_agent"]; got != true {
		t.Fatalf("reinstall_agent = %v, want true", got)
	}
}

func TestNormalizeInstanceApplyRequiresSpec(t *testing.T) {
	_, err := Normalize("instance.apply", map[string]any{
		"instance_id":  "instance-1",
		"service_code": "xray-core",
	})
	if err == nil {
		t.Fatal("expected validation error for missing spec")
	}
	if !IsValidationError(err) {
		t.Fatalf("expected validation error, got %T", err)
	}
}

func TestNormalizeInstanceRestartInfersAction(t *testing.T) {
	payload, err := Normalize("instance.restart", map[string]any{
		"instance_id":  "instance-1",
		"systemd_unit": "megavpn-xray",
	})
	if err != nil {
		t.Fatalf("Normalize returned error: %v", err)
	}
	if got := payload["action"]; got != "restart" {
		t.Fatalf("action = %v, want restart", got)
	}
}

func TestNormalizeClientProvisionInstanceIDs(t *testing.T) {
	payload, err := Normalize("client.provision", map[string]any{
		"client_id":    "client-1",
		"instance_ids": []any{"instance-a", " instance-b "},
	})
	if err != nil {
		t.Fatalf("Normalize returned error: %v", err)
	}
	ids, ok := payload["instance_ids"].([]string)
	if !ok {
		t.Fatalf("instance_ids type = %T, want []string", payload["instance_ids"])
	}
	if len(ids) != 2 || ids[0] != "instance-a" || ids[1] != "instance-b" {
		t.Fatalf("instance_ids = %#v, want normalized values", ids)
	}
}
