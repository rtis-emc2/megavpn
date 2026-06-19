package main

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/rtis-emc2/megavpn/internal/service/driver"
)

type instanceJobPayload struct {
	InstanceID         string         `json:"instance_id"`
	Action             string         `json:"action"`
	ServiceCode        string         `json:"service_code"`
	RuntimeServiceCode string         `json:"runtime_service_code"`
	Name               string         `json:"name"`
	Slug               string         `json:"slug"`
	SystemdUnit        string         `json:"systemd_unit"`
	EndpointHost       string         `json:"endpoint_host"`
	EndpointPort       int            `json:"endpoint_port"`
	Enabled            bool           `json:"enabled"`
	Spec               map[string]any `json:"spec"`
}

type managedFileSpec struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Mode    string `json:"mode"`
}

type instanceRuntimeExecutor struct{}

func (c client) handleInstanceJob(ctx context.Context, j job) (string, map[string]any) {
	payload, err := decodeInstanceJobPayload(j.Payload)
	if err != nil {
		return "failed", map[string]any{"error": err.Error(), "job_type": j.Type}
	}
	return instanceRuntimeExecutor{}.Execute(ctx, payload)
}

func decodeInstanceJobPayload(raw map[string]any) (instanceJobPayload, error) {
	b, err := json.Marshal(raw)
	if err != nil {
		return instanceJobPayload{}, err
	}
	var payload instanceJobPayload
	if err := json.Unmarshal(b, &payload); err != nil {
		return instanceJobPayload{}, err
	}
	payload.ServiceCode = driver.NormalizeCode(first(payload.RuntimeServiceCode, payload.ServiceCode))
	if payload.RuntimeServiceCode == "" {
		payload.RuntimeServiceCode = payload.ServiceCode
	}
	payload.Action = driver.NormalizeOperation(payload.Action)
	if payload.Action == "" {
		payload.Action = driver.OperationApply
	}
	if payload.Slug == "" {
		payload.Slug = slugifyLocal(first(payload.Name, payload.InstanceID, "instance"))
	}
	if payload.ServiceCode == "ipsec" && strings.TrimSpace(payload.SystemdUnit) == "strongswan" {
		payload.SystemdUnit = "strongswan-starter"
	}
	if payload.SystemdUnit == "" {
		payload.SystemdUnit = defaultInstanceSystemdUnit(payload)
	}
	if payload.Spec == nil {
		payload.Spec = map[string]any{}
	}
	return payload, nil
}

func slugifyLocal(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "instance"
	}
	return out
}
