package driver

import "strings"

const (
	HealthCheckSystemdActive     = "systemd_active"
	HealthCheckEndpointListening = "endpoint_listening"
	HealthCheckConfigObserved    = "config_observed"
)

type HealthCheckSpec struct {
	Code             string `json:"code"`
	DisplayName      string `json:"display_name"`
	Signal           string `json:"signal"`
	Source           string `json:"source"`
	Required         bool   `json:"required"`
	PortSource       string `json:"port_source,omitempty"`
	ConfigPathSource string `json:"config_path_source,omitempty"`
	TimeoutSeconds   int    `json:"timeout_seconds"`
	DegradedOnFail   bool   `json:"degraded_on_fail"`
}

func HealthChecksFor(code string) []HealthCheckSpec {
	contract, ok := contracts[NormalizeCode(code)]
	if !ok {
		return nil
	}
	out := make([]HealthCheckSpec, 0, 3)
	if strings.TrimSpace(contract.DefaultUnitPattern) != "" {
		out = append(out, HealthCheckSpec{
			Code:           HealthCheckSystemdActive,
			DisplayName:    "Systemd unit active",
			Signal:         "active_state",
			Source:         "agent_runtime_report",
			Required:       true,
			TimeoutSeconds: 5,
			DegradedOnFail: true,
		})
	}
	if strings.TrimSpace(contract.DefaultConfigPath) != "" {
		out = append(out, HealthCheckSpec{
			Code:             HealthCheckConfigObserved,
			DisplayName:      "Config observed",
			Signal:           "config_hash",
			Source:           "agent_runtime_report",
			Required:         true,
			ConfigPathSource: "driver_default_or_revision_spec",
			TimeoutSeconds:   5,
			DegradedOnFail:   true,
		})
	}
	if supportsEndpointListeningCheck(contract.Code) {
		out = append(out, HealthCheckSpec{
			Code:           HealthCheckEndpointListening,
			DisplayName:    "Endpoint port listening",
			Signal:         "listening_ports",
			Source:         "agent_runtime_report",
			Required:       true,
			PortSource:     "instance.endpoint_port",
			TimeoutSeconds: 5,
			DegradedOnFail: true,
		})
	}
	return out
}

func supportsEndpointListeningCheck(code string) bool {
	switch NormalizeCode(code) {
	case XrayCore, OpenVPN, WireGuard, IPSec, HTTPProxy, MTProto, Shadowsocks, Nginx:
		return true
	default:
		return false
	}
}
