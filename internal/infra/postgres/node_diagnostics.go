package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rtis-emc2/megavpn/internal/domain"
)

func (s *Store) GetNodeDiagnostics(ctx context.Context, nodeID string) (domain.NodeDiagnostics, error) {
	node, err := s.GetNode(ctx, nodeID)
	if err != nil {
		return domain.NodeDiagnostics{}, err
	}

	diag := domain.NodeDiagnostics{
		Node:               node,
		HeartbeatState:     "unknown",
		CommunicationState: "unknown",
		Agent:              domain.NodeAgentState{NodeID: nodeID, Status: "missing", TokenRotationStatus: "missing"},
		DiscoverySummary:   domain.NodeServiceDiscoverySummary{NodeID: nodeID, ByService: map[string]int{}},
		RecentDiscoveries:  []domain.NodeServiceDiscovery{},
	}
	diag.HeartbeatState, diag.HeartbeatDriftSeconds = heartbeatState(node.LastHeartbeatAt)

	agent, err := s.getNodeAgentState(ctx, nodeID)
	if err != nil {
		return diag, err
	}
	diag.Agent = agent

	pendingRotateAt, err := s.latestPendingAgentTokenRotationAt(ctx, nodeID)
	if err != nil {
		return diag, err
	}

	if inventory, err := s.LatestNodeInventory(ctx, nodeID); err == nil {
		diag.LatestInventory = &inventory
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return diag, err
	}

	if summary, err := s.NodeServiceDiscoverySummary(ctx, nodeID); err == nil {
		diag.DiscoverySummary = summary
	} else {
		return diag, err
	}

	discoveries, err := s.listRecentNodeDiscoveries(ctx, nodeID, 8)
	if err != nil {
		return diag, err
	}
	diag.RecentDiscoveries = discoveries

	runs, err := s.ListNodeBootstrapRuns(ctx, nodeID, 20)
	if err != nil {
		return diag, err
	}
	for idx := range runs {
		run := runs[idx]
		if diag.LastBootstrap == nil {
			diag.LastBootstrap = &run
		}
		if diag.LastSuccessfulBootstrap == nil && isSuccessfulBootstrapStatus(run.Status) {
			diag.LastSuccessfulBootstrap = &run
		}
		if diag.LastFailedBootstrap == nil && isFailedBootstrapStatus(run.Status) {
			diag.LastFailedBootstrap = &run
		}
	}

	tokens, err := s.ListNodeEnrollmentTokens(ctx, nodeID)
	if err != nil {
		return diag, err
	}
	if len(tokens) > 0 {
		diag.LatestEnrollmentToken = &tokens[0]
	}
	for idx := range tokens {
		token := tokens[idx]
		if strings.EqualFold(token.Status, "active") {
			diag.ActiveEnrollmentToken = &token
			break
		}
	}

	diag.Agent.TokenRotationStatus = deriveTokenRotationStatus(diag.Agent, diag.LastBootstrap, diag.ActiveEnrollmentToken, pendingRotateAt)
	diag.CommunicationState, diag.CommunicationHint = deriveCommunicationState(diag)
	return diag, nil
}

func (s *Store) getNodeAgentState(ctx context.Context, nodeID string) (domain.NodeAgentState, error) {
	state := domain.NodeAgentState{
		NodeID:              nodeID,
		Status:              "missing",
		TokenRotationStatus: "missing",
	}
	var registeredAt time.Time
	err := s.db.QueryRow(ctx, `select node_id,status,coalesce(agent_version,''),coalesce(protocol_version,''),coalesce(fingerprint,''),registered_at,last_seen_at,revoked_at,coalesce(token_hint,''),
		last_auth_failure_at,coalesce(last_auth_failure_reason,''),last_job_poll_at,last_job_claim_at,last_job_claim_job_id,coalesce(last_job_claim_type,''),
		last_job_result_at,last_job_result_job_id,coalesce(last_job_result_type,''),coalesce(last_job_result_status,''),last_inventory_sync_at,last_discovery_sync_at,last_runtime_sync_at
			from node_agents where node_id=$1`, nodeID).
		Scan(&state.NodeID, &state.Status, &state.AgentVersion, &state.ProtocolVersion, &state.Fingerprint, &registeredAt, &state.LastSeenAt, &state.RevokedAt, &state.TokenHint,
			&state.LastAuthFailureAt, &state.LastAuthFailureReason, &state.LastJobPollAt, &state.LastJobClaimAt, &state.LastJobClaimJobID, &state.LastJobClaimType,
			&state.LastJobResultAt, &state.LastJobResultJobID, &state.LastJobResultType, &state.LastJobResultStatus, &state.LastInventorySyncAt, &state.LastDiscoverySyncAt, &state.LastRuntimeSyncAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	state.RegisteredAt = &registeredAt
	if state.Status == "" {
		state.Status = "unknown"
	}
	return state, nil
}

func (s *Store) listRecentNodeDiscoveries(ctx context.Context, nodeID string, limit int) ([]domain.NodeServiceDiscovery, error) {
	if limit <= 0 || limit > 50 {
		limit = 8
	}
	rows, err := s.db.Query(ctx, `select id,node_id,service_code,name,coalesce(systemd_unit,''),coalesce(config_path,''),status,source,coalesce(confidence,50),coalesce(endpoint_host,''),coalesce(endpoint_port,0),managed_instance_id,payload_json,detected_at
		from node_service_discoveries
		where node_id=$1
		order by detected_at desc, service_code asc, name asc
		limit $2`, nodeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.NodeServiceDiscovery, 0, limit)
	for rows.Next() {
		item, err := scanNodeServiceDiscovery(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func heartbeatState(lastHeartbeatAt *time.Time) (string, *int64) {
	if lastHeartbeatAt == nil {
		return "unknown", nil
	}
	drift := time.Since(lastHeartbeatAt.UTC())
	if drift < 0 {
		drift = 0
	}
	seconds := int64(drift / time.Second)
	switch {
	case drift <= 30*time.Second:
		return "online", &seconds
	case drift <= 5*time.Minute:
		return "degraded", &seconds
	default:
		return "offline", &seconds
	}
}

func isSuccessfulBootstrapStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "succeeded", "success", "completed":
		return true
	default:
		return false
	}
}

func isFailedBootstrapStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failed", "cancelled":
		return true
	default:
		return false
	}
}

func deriveTokenRotationStatus(agent domain.NodeAgentState, lastBootstrap *domain.NodeBootstrapRun, activeToken *domain.NodeEnrollmentToken, pendingRotateAt *time.Time) string {
	if agent.RevokedAt != nil || strings.EqualFold(agent.Status, "revoked") {
		return "revoked"
	}
	if agent.RegisteredAt == nil {
		if activeToken != nil {
			return "awaiting_enrollment"
		}
		return "missing"
	}
	if lastBootstrap != nil && truthyBootstrap(lastBootstrap.RequestPayload, "force_reenroll") {
		if agent.LastSeenAt == nil || agent.LastSeenAt.Before(lastBootstrap.CreatedAt) {
			return "pending_reenroll"
		}
	}
	if pendingRotateAt != nil {
		if agent.LastSeenAt == nil || agent.LastSeenAt.Before(*pendingRotateAt) {
			return "rotating"
		}
	}
	if activeToken != nil && activeToken.CreatedAt.After(*agent.RegisteredAt) {
		return "enrollment_issued"
	}
	return "active"
}

func truthyBootstrap(payload map[string]any, key string) bool {
	if payload == nil {
		return false
	}
	switch value := payload[key].(type) {
	case bool:
		return value
	case string:
		value = strings.TrimSpace(strings.ToLower(value))
		return value == "1" || value == "true" || value == "yes" || value == "on"
	case float64:
		return value != 0
	default:
		return false
	}
}

func deriveCommunicationState(diag domain.NodeDiagnostics) (string, string) {
	agent := diag.Agent
	if agent.RegisteredAt == nil {
		if diag.ActiveEnrollmentToken != nil {
			return "awaiting_enrollment", "Enrollment token is active but the agent has not registered yet."
		}
		return "missing_agent_identity", "No active agent identity is registered for this node."
	}
	if agent.LastAuthFailureAt != nil {
		latestOK := latestTime(diag.Node.LastHeartbeatAt, agent.LastSeenAt, agent.LastJobResultAt, agent.LastInventorySyncAt, agent.LastDiscoverySyncAt)
		if latestOK == nil || agent.LastAuthFailureAt.After(*latestOK) {
			return "auth_failure", firstNonEmptyHint(agent.LastAuthFailureReason, "Recent agent request was rejected by control plane authorization.")
		}
	}
	if diag.HeartbeatState == "offline" {
		if agent.LastJobPollAt != nil && time.Since(*agent.LastJobPollAt) <= 5*time.Minute {
			return "heartbeat_stalled", "Agent is still polling the control plane but heartbeats are stale."
		}
		return "channel_offline", "Control plane has not seen recent heartbeat or poll activity from the agent."
	}
	if agent.LastJobClaimAt != nil {
		if agent.LastJobResultAt == nil || agent.LastJobResultAt.Before(*agent.LastJobClaimAt) {
			if time.Since(*agent.LastJobClaimAt) > 2*time.Minute {
				return "job_result_stalled", "Agent claimed a job but did not submit the result yet."
			}
			return "job_running", "Agent has a claimed job in progress."
		}
	}
	if agent.LastDiscoverySyncAt != nil && time.Since(*agent.LastDiscoverySyncAt) <= 15*time.Minute {
		return "discovery_ok", "Recent discovery sync reached the control plane."
	}
	if agent.LastInventorySyncAt != nil && time.Since(*agent.LastInventorySyncAt) <= 15*time.Minute {
		return "inventory_ok", "Recent inventory sync reached the control plane."
	}
	if diag.HeartbeatState == "degraded" {
		return "degraded", "Heartbeat is delayed but communication is still partially alive."
	}
	return "healthy", "Heartbeat, auth and recent control-plane communication look healthy."
}

func latestTime(values ...*time.Time) *time.Time {
	var latest *time.Time
	for _, value := range values {
		if value == nil {
			continue
		}
		if latest == nil || value.After(*latest) {
			copyValue := *value
			latest = &copyValue
		}
	}
	return latest
}

func firstNonEmptyHint(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
