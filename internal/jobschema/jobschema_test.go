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

func TestNormalizeControlPlaneTLSApply(t *testing.T) {
	payload, err := Normalize("platform.control_plane_tls.apply", map[string]any{
		"public_base_url": "https://control.example.com:58765",
	})
	if err != nil {
		t.Fatalf("Normalize returned error: %v", err)
	}
	if got := payload["public_base_url"]; got != "https://control.example.com:58765" {
		t.Fatalf("public_base_url = %v, want input preserved", got)
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
		"service_code": "xray",
		"systemd_unit": "megavpn-xray",
	})
	if err != nil {
		t.Fatalf("Normalize returned error: %v", err)
	}
	if got := payload["action"]; got != "restart" {
		t.Fatalf("action = %v, want restart", got)
	}
	if got := payload["service_code"]; got != "xray-core" {
		t.Fatalf("service_code = %v, want xray-core", got)
	}
}

func TestNormalizeInstanceOperationRejectsUnsupportedDriver(t *testing.T) {
	_, err := Normalize("instance.restart", map[string]any{
		"instance_id":  "instance-1",
		"service_code": "unknown-driver",
		"systemd_unit": "megavpn-unknown",
	})
	if err == nil {
		t.Fatal("expected validation error for unsupported operation driver")
	}
	if !IsValidationError(err) {
		t.Fatalf("expected validation error, got %T", err)
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

func TestNormalizeAgentTokenRotationRequiresSecretRef(t *testing.T) {
	payload, err := Normalize("node.agent.rotate_token", map[string]any{
		"node_id":                       "node-1",
		"new_agent_token_secret_ref_id": "secret-ref-1",
		"new_agent_token_hash":          "token-hash",
		"new_token_hint":                "abcd...1234",
	})
	if err != nil {
		t.Fatalf("Normalize returned error: %v", err)
	}
	if got := payload["new_agent_token_secret_ref_id"]; got != "secret-ref-1" {
		t.Fatalf("new_agent_token_secret_ref_id = %v, want secret-ref-1", got)
	}
	if _, ok := payload["new_agent_token"]; ok {
		t.Fatalf("new_agent_token must not be normalized into stored payload: %#v", payload)
	}
}

func TestNormalizeAgentTokenRotationRejectsPlaintextToken(t *testing.T) {
	_, err := Normalize("node.agent.rotate_token", map[string]any{
		"node_id":         "node-1",
		"new_agent_token": "secret-token",
		"new_token_hint":  "abcd...1234",
	})
	if err == nil {
		t.Fatal("expected validation error for plaintext new_agent_token")
	}
	if !IsValidationError(err) {
		t.Fatalf("expected validation error, got %T", err)
	}
}

func TestNormalizeNodeBackhaulApply(t *testing.T) {
	payload, err := Normalize("node.backhaul.apply", map[string]any{
		"node_id":       " ingress-node ",
		"link_id":       " link-1 ",
		"transport_id":  " transport-1 ",
		"role":          " Ingress ",
		"driver":        "openvpn-udp",
		"endpoint_port": 11950,
		"activate":      "true",
		"files": []any{
			map[string]any{"path": "/etc/megavpn/backhaul/link/openvpn.conf", "content": "redacted", "mode": "0600"},
		},
	})
	if err != nil {
		t.Fatalf("Normalize returned error: %v", err)
	}
	if got := payload["role"]; got != "ingress" {
		t.Fatalf("role = %v, want ingress", got)
	}
	if got := payload["driver"]; got != "openvpn_udp" {
		t.Fatalf("driver = %v, want openvpn_udp", got)
	}
	if got := payload["endpoint_port"]; got != 11950 {
		t.Fatalf("endpoint_port = %v, want 11950", got)
	}
	if got := payload["activate"]; got != true {
		t.Fatalf("activate = %v, want true", got)
	}
}

func TestNormalizeNodeBackhaulApplyRejectsInvalidRole(t *testing.T) {
	_, err := Normalize("node.backhaul.apply", map[string]any{
		"node_id":      "node-1",
		"link_id":      "link-1",
		"transport_id": "transport-1",
		"role":         "control",
		"driver":       "wireguard",
	})
	if err == nil {
		t.Fatal("expected validation error for invalid backhaul role")
	}
	if !IsValidationError(err) {
		t.Fatalf("expected validation error, got %T", err)
	}
}

func TestNormalizeNodeBackhaulApplyRejectsInvalidDriver(t *testing.T) {
	_, err := Normalize("node.backhaul.apply", map[string]any{
		"node_id":      "node-1",
		"link_id":      "link-1",
		"transport_id": "transport-1",
		"role":         "ingress",
		"driver":       "unknown-vpn",
	})
	if err == nil {
		t.Fatal("expected validation error for invalid backhaul driver")
	}
	if !IsValidationError(err) {
		t.Fatalf("expected validation error, got %T", err)
	}
}

func TestNormalizeNodeBackhaulProbe(t *testing.T) {
	payload, err := Normalize("node.backhaul.probe", map[string]any{
		"node_id":        " node-1 ",
		"link_id":        " link-1 ",
		"transport_id":   " transport-1 ",
		"role":           " Egress ",
		"driver":         "wireguard",
		"interface_name": " mgbh0 ",
		"systemd_unit":   " megavpn-backhaul-link.service ",
		"peer_address":   "10.240.0.1",
		"probe_count":    3,
	})
	if err != nil {
		t.Fatalf("Normalize returned error: %v", err)
	}
	if got := payload["role"]; got != "egress" {
		t.Fatalf("role = %v, want egress", got)
	}
	if got := payload["interface_name"]; got != "mgbh0" {
		t.Fatalf("interface_name = %v, want mgbh0", got)
	}
}

func TestNormalizeNodeBackhaulCleanup(t *testing.T) {
	payload, err := Normalize("node.backhaul.cleanup", map[string]any{
		"node_id":         "node-1",
		"link_id":         "link-1",
		"transport_id":    "transport-1",
		"role":            "ingress",
		"driver":          "openvpn_udp",
		"delete_batch_id": "batch-1",
		"systemd_units":   []any{" megavpn-backhaul-link.service "},
		"paths":           []any{"/etc/systemd/system/megavpn-backhaul-link.service"},
		"directories":     []any{"/etc/megavpn/backhaul/link"},
	})
	if err != nil {
		t.Fatalf("Normalize returned error: %v", err)
	}
	units, ok := payload["systemd_units"].([]string)
	if !ok || len(units) != 1 || units[0] != "megavpn-backhaul-link.service" {
		t.Fatalf("systemd_units = %#v, want normalized string slice", payload["systemd_units"])
	}
}

func TestNormalizeArtifactBuild(t *testing.T) {
	payload, err := Normalize("artifact.build", map[string]any{
		"client_id":     " client-1 ",
		"artifact_type": " wg_conf ",
		"instance_ids":  []any{"instance-1", " instance-2 "},
	})
	if err != nil {
		t.Fatalf("Normalize returned error: %v", err)
	}
	if got := payload["client_id"]; got != "client-1" {
		t.Fatalf("client_id = %v, want client-1", got)
	}
	if got := payload["artifact_type"]; got != "wg_conf" {
		t.Fatalf("artifact_type = %v, want wg_conf", got)
	}
	ids, ok := payload["instance_ids"].([]string)
	if !ok || len(ids) != 2 || ids[1] != "instance-2" {
		t.Fatalf("instance_ids = %#v, want normalized ids", payload["instance_ids"])
	}
}

func TestNormalizeArtifactBuildRejectsUnsupportedType(t *testing.T) {
	_, err := Normalize("artifact.build", map[string]any{
		"client_id":     "client-1",
		"artifact_type": "raw_secret_dump",
	})
	if err == nil {
		t.Fatal("expected validation error for unsupported artifact type")
	}
	if !IsValidationError(err) {
		t.Fatalf("expected validation error, got %T", err)
	}
}
