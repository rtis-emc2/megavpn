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
	baseURL   string
	token     string
	statePath string
	http      httpDoer
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

type bootstrapConfig struct {
	NodeID          string
	NodeName        string
	NodeAddress     string
	ControlPlaneURL string
	EnrollmentToken string
	DevToken        string
	AllowAuto       bool
}
