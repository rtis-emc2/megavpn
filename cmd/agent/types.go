package main

import (
	"net/http"
	"time"

	"github.com/rtis-emc2/megavpn/internal/platform/version"
)

const appVersion = version.Version

type agentLogger interface {
	Info(string, ...any)
	Error(string, ...any)
}

type client struct {
	baseURL        string
	token          string
	statePath      string
	http           httpDoer
	responseReplay *responseReplayCache
}

type httpDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type agentState struct {
	NodeID          string    `json:"node_id"`
	NodeName        string    `json:"node_name"`
	NodeAddress     string    `json:"node_address"`
	ControlPlaneURL string    `json:"control_plane_url"`
	AgentToken      string    `json:"agent_token"`
	RegisteredAt    time.Time `json:"registered_at"`
}

type registerResp struct {
	Status     string    `json:"status"`
	Node       stateNode `json:"node"`
	AgentToken string    `json:"agent_token"`
	TokenHint  string    `json:"token_hint"`
}

type stateNode struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Address string `json:"address"`
}

type job struct {
	ID      string         `json:"id"`
	Type    string         `json:"type"`
	Payload map[string]any `json:"payload"`
}

type instanceRuntimeTargetsResp struct {
	Targets []instanceRuntimeTarget `json:"targets"`
}

type instanceRuntimeTarget struct {
	InstanceID        string  `json:"instance_id"`
	NodeID            string  `json:"node_id"`
	ServiceCode       string  `json:"service_code"`
	Slug              string  `json:"slug"`
	SystemdUnit       string  `json:"systemd_unit"`
	ConfigPath        string  `json:"config_path"`
	EndpointHost      string  `json:"endpoint_host"`
	EndpointPort      int     `json:"endpoint_port"`
	DesiredStatus     string  `json:"desired_status"`
	DesiredEnabled    bool    `json:"desired_enabled"`
	CurrentRevisionID *string `json:"current_revision_id,omitempty"`
	AppliedRevisionID *string `json:"applied_revision_id,omitempty"`
}

type instanceRuntimeReport struct {
	InstanceID         string           `json:"instance_id"`
	ServiceCode        string           `json:"service_code"`
	SystemdUnit        string           `json:"systemd_unit"`
	ConfigPath         string           `json:"config_path"`
	ConfigHash         string           `json:"config_hash"`
	ActiveState        string           `json:"active_state"`
	EnabledState       string           `json:"enabled_state"`
	ObservedRevisionID *string          `json:"observed_revision_id,omitempty"`
	ListeningPorts     []map[string]any `json:"listening_ports"`
	ErrorText          string           `json:"error_text"`
	CheckedAt          *time.Time       `json:"checked_at,omitempty"`
}

type bootstrapConfig struct {
	NodeID          string
	NodeName        string
	NodeAddress     string
	ControlPlaneURL string
	EnrollmentToken string
	DevToken        string
	AllowAuto       bool
}
