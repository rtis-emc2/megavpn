package domain

import "time"

type NodeAgentState struct {
	NodeID                string     `json:"node_id"`
	Status                string     `json:"status"`
	AgentVersion          string     `json:"agent_version"`
	ProtocolVersion       string     `json:"protocol_version"`
	Fingerprint           string     `json:"fingerprint"`
	RegisteredAt          *time.Time `json:"registered_at,omitempty"`
	LastSeenAt            *time.Time `json:"last_seen_at,omitempty"`
	RevokedAt             *time.Time `json:"revoked_at,omitempty"`
	TokenHint             string     `json:"token_hint"`
	TokenRotationStatus   string     `json:"token_rotation_status"`
	LastAuthFailureAt     *time.Time `json:"last_auth_failure_at,omitempty"`
	LastAuthFailureReason string     `json:"last_auth_failure_reason"`
	LastJobPollAt         *time.Time `json:"last_job_poll_at,omitempty"`
	LastJobClaimAt        *time.Time `json:"last_job_claim_at,omitempty"`
	LastJobClaimJobID     *string    `json:"last_job_claim_job_id,omitempty"`
	LastJobClaimType      string     `json:"last_job_claim_type"`
	LastJobResultAt       *time.Time `json:"last_job_result_at,omitempty"`
	LastJobResultJobID    *string    `json:"last_job_result_job_id,omitempty"`
	LastJobResultType     string     `json:"last_job_result_type"`
	LastJobResultStatus   string     `json:"last_job_result_status"`
	LastInventorySyncAt   *time.Time `json:"last_inventory_sync_at,omitempty"`
	LastDiscoverySyncAt   *time.Time `json:"last_discovery_sync_at,omitempty"`
	LastRuntimeSyncAt     *time.Time `json:"last_runtime_sync_at,omitempty"`
}

type NodeDiagnostics struct {
	Node                    Node                        `json:"node"`
	HeartbeatState          string                      `json:"heartbeat_state"`
	HeartbeatDriftSeconds   *int64                      `json:"heartbeat_drift_seconds,omitempty"`
	CommunicationState      string                      `json:"communication_state"`
	CommunicationHint       string                      `json:"communication_hint"`
	Agent                   NodeAgentState              `json:"agent"`
	LatestInventory         *NodeInventorySnapshot      `json:"latest_inventory,omitempty"`
	DiscoverySummary        NodeServiceDiscoverySummary `json:"discovery_summary"`
	RecentDiscoveries       []NodeServiceDiscovery      `json:"recent_discoveries"`
	LastBootstrap           *NodeBootstrapRun           `json:"last_bootstrap,omitempty"`
	LastSuccessfulBootstrap *NodeBootstrapRun           `json:"last_successful_bootstrap,omitempty"`
	LastFailedBootstrap     *NodeBootstrapRun           `json:"last_failed_bootstrap,omitempty"`
	ActiveEnrollmentToken   *NodeEnrollmentToken        `json:"active_enrollment_token,omitempty"`
	LatestEnrollmentToken   *NodeEnrollmentToken        `json:"latest_enrollment_token,omitempty"`
}
