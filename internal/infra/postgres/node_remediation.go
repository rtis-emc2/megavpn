package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/platform/id"
	"github.com/rtis-emc2/megavpn/internal/service/driver"
)

const (
	stuckNodeJobAfter       = 2 * time.Minute
	stalePendingRotateAfter = 2 * time.Minute
	// Matches the heartbeatState offline threshold in node_diagnostics.go.
	nodeRebootChannelEvidenceMaxAge = 5 * time.Minute
)

func (s *Store) CreateNodeChannelProbeJob(ctx context.Context, nodeID string) (domain.Job, error) {
	if _, err := s.GetNode(ctx, nodeID); err != nil {
		return domain.Job{}, err
	}
	payload := map[string]any{
		"node_id":            nodeID,
		"probe_requested_at": time.Now().UTC().Format(time.RFC3339),
	}
	job, err := s.CreateJob(ctx, domain.Job{
		Type:      "node.channel.probe",
		ScopeType: "node",
		ScopeID:   &nodeID,
		NodeID:    &nodeID,
		Priority:  5,
		Payload:   payload,
	})
	if err != nil {
		return job, err
	}
	_, _ = s.CreateAudit(ctx, "system", "node.channel.probe", "node", &nodeID, "node channel probe queued")
	return job, nil
}

func (s *Store) CreateNodeEmergencyCleanupJob(ctx context.Context, nodeID string, input domain.NodeEmergencyCleanupJobCreateInput) (domain.NodeEmergencyCleanupJobCreateResult, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return domain.NodeEmergencyCleanupJobCreateResult{}, domain.ValidationError{Message: "node id is required"}
	}
	if err := input.NormalizeAndValidate(); err != nil {
		return domain.NodeEmergencyCleanupJobCreateResult{}, err
	}

	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.NodeEmergencyCleanupJobCreateResult{}, err
	}
	defer tx.Rollback(ctx)

	var nodeName string
	var nodeStatus string
	var lastHeartbeatAt *time.Time
	if err := tx.QueryRow(ctx, `select name,status,last_heartbeat_at from nodes where id=$1 for update`, nodeID).Scan(&nodeName, &nodeStatus, &lastHeartbeatAt); err != nil {
		return domain.NodeEmergencyCleanupJobCreateResult{}, err
	}
	if isTerminalNodeRebootStatus(nodeStatus) {
		return domain.NodeEmergencyCleanupJobCreateResult{}, domain.ErrNodeEmergencyCleanupConflict
	}
	if strings.TrimSpace(nodeStatus) != "maintenance" {
		return domain.NodeEmergencyCleanupJobCreateResult{}, domain.ErrNodeEmergencyCleanupMaintenanceRequired
	}

	expectedConfirmation := strings.TrimSpace(nodeName)
	if expectedConfirmation == "" {
		expectedConfirmation = nodeID
	}
	if input.Confirmation != expectedConfirmation {
		return domain.NodeEmergencyCleanupJobCreateResult{}, domain.ErrNodeEmergencyCleanupConfirmationMismatch
	}

	if err := validateNodeEmergencyCleanupAgentReadyTx(ctx, tx, nodeID, lastHeartbeatAt); err != nil {
		return domain.NodeEmergencyCleanupJobCreateResult{}, err
	}

	conflict, err := nodeHasActiveEmergencyCleanupConflictTx(ctx, tx, nodeID)
	if err != nil {
		return domain.NodeEmergencyCleanupJobCreateResult{}, err
	}
	if conflict {
		return domain.NodeEmergencyCleanupJobCreateResult{}, domain.ErrNodeEmergencyCleanupConflict
	}

	plan, err := buildNodeEmergencyCleanupPlanTx(ctx, tx, nodeID, input.CleanupScope, input.IncludeAgent)
	if err != nil {
		return domain.NodeEmergencyCleanupJobCreateResult{}, err
	}

	now := time.Now().UTC()
	payload := map[string]any{
		"schema":               "node.emergency_cleanup.v1",
		"node_id":              nodeID,
		"node_name":            expectedConfirmation,
		"cleanup_scope":        input.CleanupScope,
		"include_agent":        input.IncludeAgent,
		"confirmation":         input.Confirmation,
		"reason":               input.Reason,
		"requested_at":         now.Format(time.RFC3339),
		"cleanup_plan_version": domain.NodeEmergencyCleanupPlanVersion,
		"instances":            plan.instances,
	}
	job, payloadJSON, err := normalizeJobForInsert(domain.Job{
		Type:      "node.emergency_cleanup",
		ScopeType: "node",
		ScopeID:   &nodeID,
		NodeID:    &nodeID,
		Priority:  1,
		Payload:   payload,
		CreatedAt: now,
	})
	if err != nil {
		return domain.NodeEmergencyCleanupJobCreateResult{}, err
	}
	if err := insertJobRow(ctx, tx, job, payloadJSON); err != nil {
		return domain.NodeEmergencyCleanupJobCreateResult{}, err
	}
	if err := acquireJobResourceLockTx(ctx, tx, job); err != nil {
		return domain.NodeEmergencyCleanupJobCreateResult{}, domain.ErrNodeEmergencyCleanupConflict
	}
	auditSummary := fmt.Sprintf(
		"node emergency cleanup job queued: node_id=%s node_name=%s job_id=%s cleanup_scope=%s include_agent=%t instance_targets=%d reason=%s",
		nodeID,
		expectedConfirmation,
		job.ID,
		input.CleanupScope,
		input.IncludeAgent,
		plan.summary.InstanceTargetCount,
		input.Reason,
	)
	if _, err := insertAuditForUser(ctx, tx, input.ActorUserID, "node.emergency_cleanup.queue", "node", &nodeID, auditSummary); err != nil {
		return domain.NodeEmergencyCleanupJobCreateResult{}, fmt.Errorf("%w: %v", domain.ErrNodeEmergencyCleanupAuditFailed, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.NodeEmergencyCleanupJobCreateResult{}, err
	}
	return domain.NodeEmergencyCleanupJobCreateResult{Job: job, PlanSummary: plan.summary}, nil
}

func (s *Store) ForceRetireNode(ctx context.Context, nodeID, confirmation, reason string) (domain.Node, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return domain.Node{}, errors.New("node id is required")
	}
	node, err := s.GetNode(ctx, nodeID)
	if err != nil {
		return domain.Node{}, err
	}
	expectedConfirmation := strings.TrimSpace(node.Name)
	if expectedConfirmation == "" {
		expectedConfirmation = nodeID
	}
	if strings.TrimSpace(confirmation) != expectedConfirmation {
		return domain.Node{}, fmt.Errorf("confirmation must match node name %q", expectedConfirmation)
	}
	if strings.EqualFold(node.Status, "retired") {
		return node, nil
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "operator force retired lost node"
	}

	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.Node{}, err
	}
	defer tx.Rollback(ctx)

	instanceIDs, err := nodeInstanceIDsForForceRetire(ctx, tx, nodeID)
	if err != nil {
		return domain.Node{}, err
	}
	backhaulLinkIDs, err := nodeBackhaulLinkIDsForForceRetire(ctx, tx, nodeID)
	if err != nil {
		return domain.Node{}, err
	}

	artifactCleanupPaths := make([]string, 0)
	for _, instanceID := range instanceIDs {
		cleanupPaths, err := s.cleanupInstanceControlPlaneRows(ctx, tx, instanceID)
		if err != nil {
			return domain.Node{}, err
		}
		artifactCleanupPaths = append(artifactCleanupPaths, cleanupPaths...)
	}

	if len(backhaulLinkIDs) > 0 {
		if _, err := tx.Exec(ctx, `delete from secret_refs
			where meta_json->>'scope'='backhaul'
			  and meta_json->>'link_id' = any($1::text[])`, backhaulLinkIDs); err != nil {
			return domain.Node{}, err
		}
		if _, err := tx.Exec(ctx, `update backhaul_links
			set status='deleted',
			    selected_transport_id=null,
			    updated_at=now()
			where id::text = any($1::text[])`, backhaulLinkIDs); err != nil {
			return domain.Node{}, err
		}
	}

	now := time.Now().UTC()
	cancelResult := mustJSON(map[string]any{
		"message":          "job cancelled by force node retire",
		"node_id":          nodeID,
		"reason":           reason,
		"force_retired_at": now.Format(time.RFC3339),
	})
	cancelledJobIDs, err := cancelNodeJobsForForceRetire(ctx, tx, nodeID, instanceIDs, backhaulLinkIDs, now, cancelResult)
	if err != nil {
		return domain.Node{}, err
	}

	if _, err := tx.Exec(ctx, `update node_bootstrap_runs
		set status='cancelled',
		    result_payload_json=$2,
		    finished_at=now()
		where node_id=$1 and status in ('queued','running')`, nodeID, cancelResult); err != nil {
		return domain.Node{}, err
	}
	if _, err := tx.Exec(ctx, `delete from firewall_node_state where node_id=$1`, nodeID); err != nil {
		return domain.Node{}, err
	}
	if _, err := tx.Exec(ctx, `delete from resource_locks where resource_type='node' and resource_id=$1`, nodeID); err != nil {
		return domain.Node{}, err
	}
	if _, err := tx.Exec(ctx, `update node_agents set status='revoked',revoked_at=now() where node_id=$1`, nodeID); err != nil {
		return domain.Node{}, err
	}
	cmd, err := tx.Exec(ctx, `update nodes
		set status='retired',
		    agent_status='offline',
		    last_heartbeat_at=null,
		    updated_at=now()
		where id=$1`, nodeID)
	if err != nil {
		return domain.Node{}, err
	}
	if cmd.RowsAffected() == 0 {
		return domain.Node{}, pgx.ErrNoRows
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Node{}, err
	}

	if len(artifactCleanupPaths) > 0 {
		_, fileErrors := s.removeManagedArtifactFiles(artifactCleanupPaths)
		if len(fileErrors) > 0 {
			_, _ = s.CreateAudit(ctx, "system", "node.force_retire.artifact_warning", "node", &nodeID, fmt.Sprintf("force retire left %d artifact file cleanup warnings", len(fileErrors)))
		}
	}
	for _, jobID := range cancelledJobIDs {
		_ = s.AddJobLog(ctx, jobID, "warn", "job cancelled by force node retire", map[string]any{"node_id": nodeID, "reason": reason})
	}
	_, _ = s.CreateAudit(ctx, "system", "node.force_retire", "node", &nodeID, fmt.Sprintf("node force retired; instances=%d jobs_cancelled=%d backhaul_links=%d", len(instanceIDs), len(cancelledJobIDs), len(backhaulLinkIDs)))
	return s.GetNode(ctx, nodeID)
}

func (s *Store) ForceDeleteInstance(ctx context.Context, instanceID, confirmation, reason string) (domain.Instance, error) {
	instanceID = strings.TrimSpace(instanceID)
	if instanceID == "" {
		return domain.Instance{}, errors.New("instance id is required")
	}
	instance, err := s.GetInstance(ctx, instanceID)
	if err != nil {
		return domain.Instance{}, err
	}
	expectedConfirmation := strings.TrimSpace(instance.Name)
	if expectedConfirmation == "" {
		expectedConfirmation = instanceID
	}
	if strings.TrimSpace(confirmation) != expectedConfirmation {
		return domain.Instance{}, fmt.Errorf("confirmation must match instance name %q", expectedConfirmation)
	}
	if strings.EqualFold(instance.Status, "deleted") {
		return instance, nil
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "operator force deleted instance without agent cleanup"
	}

	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.Instance{}, err
	}
	defer tx.Rollback(ctx)

	var lockedInstanceID string
	if err := tx.QueryRow(ctx, `select id::text from instances where id=$1 for update`, instanceID).Scan(&lockedInstanceID); err != nil {
		return domain.Instance{}, err
	}
	artifactCleanupPaths, err := s.cleanupInstanceControlPlaneRows(ctx, tx, instanceID)
	if err != nil {
		return domain.Instance{}, err
	}

	now := time.Now().UTC()
	cancelResult := mustJSON(map[string]any{
		"message":           "job cancelled by force instance delete",
		"instance_id":       instanceID,
		"node_id":           instance.NodeID,
		"reason":            reason,
		"force_deleted_at":  now.Format(time.RFC3339),
		"remote_cleanup_ok": false,
	})
	cancelledJobIDs, err := cancelInstanceJobsForForceDelete(ctx, tx, instanceID, now, cancelResult)
	if err != nil {
		return domain.Instance{}, err
	}
	if _, err := tx.Exec(ctx, `delete from resource_locks where resource_type='instance' and resource_id::text=$1`, instanceID); err != nil {
		return domain.Instance{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Instance{}, err
	}

	if len(artifactCleanupPaths) > 0 {
		_, fileErrors := s.removeManagedArtifactFiles(artifactCleanupPaths)
		if len(fileErrors) > 0 {
			_, _ = s.CreateAudit(ctx, "system", "instance.force_delete.artifact_warning", "instance", &instanceID, fmt.Sprintf("force delete left %d artifact file cleanup warnings", len(fileErrors)))
		}
	}
	for _, jobID := range cancelledJobIDs {
		_ = s.AddJobLog(ctx, jobID, "warn", "job cancelled by force instance delete", map[string]any{"instance_id": instanceID, "node_id": instance.NodeID, "reason": reason})
	}
	_, _ = s.CreateAudit(ctx, "system", "instance.force_delete", "instance", &instanceID, fmt.Sprintf("instance force deleted without agent cleanup; jobs_cancelled=%d", len(cancelledJobIDs)))
	return s.GetInstance(ctx, instanceID)
}

func (s *Store) cleanupInstanceControlPlaneRows(ctx context.Context, tx pgx.Tx, instanceID string) ([]string, error) {
	cleanup, err := s.cleanupInstanceClientServiceAccesses(ctx, tx, instanceID)
	if err != nil {
		return nil, err
	}
	if err := releaseInstanceAddressPoolAllocationsTx(ctx, tx, instanceID); err != nil && !isAddressPoolCatalogUnavailable(err) {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `delete from instance_runtime_observations where instance_id=$1`, instanceID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `delete from instance_runtime_states where instance_id=$1`, instanceID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `delete from secret_refs
		where meta_json->>'scope'='instance'
		  and meta_json->>'instance_id'=$1`, instanceID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `update instances set status='deleted',enabled=false,updated_at=now() where id=$1`, instanceID); err != nil {
		return nil, err
	}
	return cleanup.paths, nil
}

func nodeInstanceIDsForForceRetire(ctx context.Context, tx pgx.Tx, nodeID string) ([]string, error) {
	rows, err := tx.Query(ctx, `select id::text from instances where node_id=$1 and status <> 'deleted' for update`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func nodeBackhaulLinkIDsForForceRetire(ctx context.Context, tx pgx.Tx, nodeID string) ([]string, error) {
	rows, err := tx.Query(ctx, `select id::text
from backhaul_links
where status <> 'deleted'
  and (ingress_node_id=$1 or egress_node_id=$1)
for update`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func cancelNodeJobsForForceRetire(ctx context.Context, tx pgx.Tx, nodeID string, instanceIDs, backhaulLinkIDs []string, finishedAt time.Time, resultJSON []byte) ([]string, error) {
	rows, err := tx.Query(ctx, `with target_jobs as (
	select id
	from jobs
	where status in ('queued','running','retrying')
	  and (
	    node_id=$1
	    or instance_id::text = any($2::text[])
	    or (scope_type='backhaul' and scope_id::text = any($3::text[]))
	    or (type in ('client.provision','artifact.build') and (payload_json->'instance_ids') ?| $4::text[])
	  )
	for update
),
deleted_locks as (
	delete from resource_locks
	where job_id in (select id from target_jobs)
),
updated_jobs as (
	update jobs
	set status='cancelled',
	    finished_at=$5,
	    locked_by=null,
	    locked_until=null,
	    result_json=$6
	where id in (select id from target_jobs)
	returning id::text
)
select id from updated_jobs`, nodeID, instanceIDs, backhaulLinkIDs, instanceIDs, finishedAt, resultJSON)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func cancelInstanceJobsForForceDelete(ctx context.Context, tx pgx.Tx, instanceID string, finishedAt time.Time, resultJSON []byte) ([]string, error) {
	rows, err := tx.Query(ctx, `with target_jobs as (
	select id
	from jobs
	where status in ('queued','running','retrying')
	  and (
	    instance_id::text=$1
	    or (scope_type='instance' and scope_id::text=$1)
	    or (type in ('client.provision','artifact.build') and (payload_json->'instance_ids') ? $1)
	  )
	for update
),
deleted_locks as (
	delete from resource_locks
	where job_id in (select id from target_jobs)
),
updated_jobs as (
	update jobs
	set status='cancelled',
	    finished_at=$2,
	    locked_by=null,
	    locked_until=null,
	    result_json=$3
	where id in (select id from target_jobs)
	returning id::text
)
select id from updated_jobs`, instanceID, finishedAt, resultJSON)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s *Store) CreateNodeRebootJob(ctx context.Context, nodeID string, input domain.NodeRebootJobCreateInput) (domain.Job, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return domain.Job{}, domain.ValidationError{Message: "node id is required"}
	}
	if err := input.NormalizeAndValidate(); err != nil {
		return domain.Job{}, err
	}

	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.Job{}, err
	}
	defer tx.Rollback(ctx)

	var nodeName string
	var nodeStatus string
	var lastHeartbeatAt *time.Time
	if err := tx.QueryRow(ctx, `select name,status,last_heartbeat_at from nodes where id=$1 for update`, nodeID).Scan(&nodeName, &nodeStatus, &lastHeartbeatAt); err != nil {
		return domain.Job{}, err
	}
	if isTerminalNodeRebootStatus(nodeStatus) {
		return domain.Job{}, domain.ErrNodeRebootConflict
	}

	expectedConfirmation := strings.TrimSpace(nodeName)
	if expectedConfirmation == "" {
		expectedConfirmation = nodeID
	}
	if input.Confirmation != expectedConfirmation {
		return domain.Job{}, domain.ErrNodeRebootConfirmationMismatch
	}

	var agentStatus string
	var revokedAt *time.Time
	var tokenHash string
	var lastSeenAt *time.Time
	var lastAuthFailureAt *time.Time
	var lastJobPollAt *time.Time
	var lastJobResultAt *time.Time
	var lastInventorySyncAt *time.Time
	var lastDiscoverySyncAt *time.Time
	var lastRuntimeSyncAt *time.Time
	if err := tx.QueryRow(ctx, `select status,revoked_at,coalesce(agent_token_hash,''),last_seen_at,last_auth_failure_at,last_job_poll_at,last_job_result_at,last_inventory_sync_at,last_discovery_sync_at,last_runtime_sync_at
		from node_agents
		where node_id=$1
		for update`, nodeID).Scan(&agentStatus, &revokedAt, &tokenHash, &lastSeenAt, &lastAuthFailureAt, &lastJobPollAt, &lastJobResultAt, &lastInventorySyncAt, &lastDiscoverySyncAt, &lastRuntimeSyncAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Job{}, domain.ErrNodeRebootAgentMissing
		}
		return domain.Job{}, err
	}
	if agentStatus != "active" || revokedAt != nil || strings.TrimSpace(tokenHash) == "" {
		return domain.Job{}, domain.ErrNodeRebootAgentUnavailable
	}
	now := time.Now().UTC()
	lastEvidence := latestTime(lastHeartbeatAt, lastSeenAt, lastJobPollAt, lastJobResultAt, lastInventorySyncAt, lastDiscoverySyncAt, lastRuntimeSyncAt)
	if lastEvidence == nil || now.Sub(lastEvidence.UTC()) > nodeRebootChannelEvidenceMaxAge {
		return domain.Job{}, domain.ErrNodeRebootAgentUnavailable
	}
	latestOK := latestTime(lastHeartbeatAt, lastSeenAt, lastJobResultAt, lastInventorySyncAt, lastDiscoverySyncAt, lastRuntimeSyncAt)
	if lastAuthFailureAt != nil && (latestOK == nil || lastAuthFailureAt.After(latestOK.UTC())) {
		return domain.Job{}, domain.ErrNodeRebootAgentUnavailable
	}

	conflict, err := nodeHasActiveRebootConflictTx(ctx, tx, nodeID)
	if err != nil {
		return domain.Job{}, err
	}
	if conflict {
		return domain.Job{}, domain.ErrNodeRebootConflict
	}

	payload := map[string]any{
		"schema":       "node.reboot.v1",
		"node_id":      nodeID,
		"node_name":    expectedConfirmation,
		"confirmation": input.Confirmation,
		"reason":       input.Reason,
		"requested_at": now.Format(time.RFC3339),
	}
	job, payloadJSON, err := normalizeJobForInsert(domain.Job{
		Type:      "node.reboot",
		ScopeType: "node",
		ScopeID:   &nodeID,
		NodeID:    &nodeID,
		Priority:  1,
		Payload:   payload,
		CreatedAt: now,
	})
	if err != nil {
		return domain.Job{}, err
	}
	if err := insertJobRow(ctx, tx, job, payloadJSON); err != nil {
		return domain.Job{}, err
	}
	if err := acquireJobResourceLockTx(ctx, tx, job); err != nil {
		return domain.Job{}, domain.ErrNodeRebootConflict
	}
	auditSummary := fmt.Sprintf("node reboot job queued: node_id=%s node_name=%s job_id=%s reason=%s", nodeID, expectedConfirmation, job.ID, input.Reason)
	if _, err := insertAuditForUser(ctx, tx, input.ActorUserID, "node.reboot.queue", "node", &nodeID, auditSummary); err != nil {
		return domain.Job{}, fmt.Errorf("%w: %v", domain.ErrNodeRebootAuditFailed, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Job{}, err
	}
	return job, nil
}

func isTerminalNodeRebootStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "retired", "deleted", "deleting", "archived":
		return true
	default:
		return false
	}
}

func nodeHasActiveRebootConflictTx(ctx context.Context, tx pgx.Tx, nodeID string) (bool, error) {
	var ok bool
	err := tx.QueryRow(ctx, `select exists(
		select 1
		from jobs
		where node_id=$1
		  and status in ('queued','running','retrying')
		  and type in (
		    'node.reboot',
		    'node.emergency_cleanup',
		    'node.agent.rotate_token',
		    'node.bootstrap',
		    'node.capability.install',
		    'node.backhaul.apply',
		    'node.backhaul.cleanup',
		    'node.route_policy.apply',
		    'node.route_policy.cleanup',
		    'node.firewall.apply',
		    'node.firewall.disable',
		    'instance.apply',
		    'instance.restart',
		    'instance.start',
		    'instance.stop',
		    'instance.enable',
		    'instance.disable',
		    'instance.delete'
		  )
	)`, nodeID).Scan(&ok)
	return ok, err
}

func validateNodeEmergencyCleanupAgentReadyTx(ctx context.Context, tx pgx.Tx, nodeID string, lastHeartbeatAt *time.Time) error {
	var agentStatus string
	var revokedAt *time.Time
	var tokenHash string
	var lastSeenAt *time.Time
	var lastAuthFailureAt *time.Time
	var lastJobPollAt *time.Time
	var lastJobResultAt *time.Time
	var lastInventorySyncAt *time.Time
	var lastDiscoverySyncAt *time.Time
	var lastRuntimeSyncAt *time.Time
	if err := tx.QueryRow(ctx, `select status,revoked_at,coalesce(agent_token_hash,''),last_seen_at,last_auth_failure_at,last_job_poll_at,last_job_result_at,last_inventory_sync_at,last_discovery_sync_at,last_runtime_sync_at
		from node_agents
		where node_id=$1
		for update`, nodeID).Scan(&agentStatus, &revokedAt, &tokenHash, &lastSeenAt, &lastAuthFailureAt, &lastJobPollAt, &lastJobResultAt, &lastInventorySyncAt, &lastDiscoverySyncAt, &lastRuntimeSyncAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNodeEmergencyCleanupAgentMissing
		}
		return err
	}
	if agentStatus != "active" || revokedAt != nil || strings.TrimSpace(tokenHash) == "" {
		return domain.ErrNodeEmergencyCleanupAgentUnavailable
	}
	now := time.Now().UTC()
	lastEvidence := latestTime(lastHeartbeatAt, lastSeenAt, lastJobPollAt, lastJobResultAt, lastInventorySyncAt, lastDiscoverySyncAt, lastRuntimeSyncAt)
	if lastEvidence == nil || now.Sub(lastEvidence.UTC()) > nodeRebootChannelEvidenceMaxAge {
		return domain.ErrNodeEmergencyCleanupAgentUnavailable
	}
	latestOK := latestTime(lastHeartbeatAt, lastSeenAt, lastJobResultAt, lastInventorySyncAt, lastDiscoverySyncAt, lastRuntimeSyncAt)
	if lastAuthFailureAt != nil && (latestOK == nil || lastAuthFailureAt.After(latestOK.UTC())) {
		return domain.ErrNodeEmergencyCleanupAgentUnavailable
	}
	return nil
}

func nodeHasActiveEmergencyCleanupConflictTx(ctx context.Context, tx pgx.Tx, nodeID string) (bool, error) {
	return nodeHasActiveRebootConflictTx(ctx, tx, nodeID)
}

type nodeEmergencyCleanupPlan struct {
	instances []map[string]any
	summary   domain.NodeEmergencyCleanupPlanSummary
}

func buildNodeEmergencyCleanupPlanTx(ctx context.Context, tx pgx.Tx, nodeID, cleanupScope string, includeAgent bool) (nodeEmergencyCleanupPlan, error) {
	rows, err := tx.Query(ctx, `select
		i.id::text,
		sd.code,
		i.name,
		i.slug,
		coalesce(i.systemd_unit,''),
		coalesce(i.endpoint_host,''),
		coalesce(i.endpoint_port,0)
	from instances i
	join service_definitions sd on sd.id=i.service_definition_id
	where i.node_id=$1
	  and i.status <> 'deleted'
	order by lower(sd.code), lower(i.slug), lower(i.name), i.created_at, i.id
	for share of i`, nodeID)
	if err != nil {
		return nodeEmergencyCleanupPlan{}, err
	}
	defer rows.Close()

	instances := make([]map[string]any, 0)
	serviceCounts := map[string]int{}
	for rows.Next() {
		var instanceID string
		var serviceCode string
		var name string
		var slug string
		var systemdUnit string
		var endpointHost string
		var endpointPort int
		if err := rows.Scan(&instanceID, &serviceCode, &name, &slug, &systemdUnit, &endpointHost, &endpointPort); err != nil {
			return nodeEmergencyCleanupPlan{}, err
		}
		target, err := safeNodeEmergencyCleanupTarget(instanceID, serviceCode, name, slug, systemdUnit, endpointHost, endpointPort)
		if err != nil {
			return nodeEmergencyCleanupPlan{}, fmt.Errorf("%w: %v", domain.ErrNodeEmergencyCleanupPlanInvalid, err)
		}
		instances = append(instances, target)
		serviceCounts[normalizeInstanceRuntimeCode(serviceCode)]++
	}
	if err := rows.Err(); err != nil {
		return nodeEmergencyCleanupPlan{}, err
	}

	summary := domain.NodeEmergencyCleanupPlanSummary{
		CleanupScope:          cleanupScope,
		IncludeAgent:          includeAgent,
		InstanceTargetCount:   len(instances),
		ServiceCounts:         serviceCounts,
		NodeRuntimeCleanup:    cleanupScope == domain.NodeEmergencyCleanupScopeFullNode,
		AgentRemovalRequested: includeAgent,
	}
	return nodeEmergencyCleanupPlan{instances: instances, summary: summary}, nil
}

func safeNodeEmergencyCleanupTarget(instanceID, serviceCode, name, slug, systemdUnit, endpointHost string, endpointPort int) (map[string]any, error) {
	instanceID = strings.TrimSpace(instanceID)
	serviceCode = normalizeInstanceRuntimeCode(serviceCode)
	name = strings.TrimSpace(name)
	slug = strings.TrimSpace(slug)
	systemdUnit = strings.TrimSpace(systemdUnit)
	endpointHost = strings.TrimSpace(endpointHost)

	if err := validateNodeEmergencyCleanupIdentifier("instance_id", instanceID, 128); err != nil {
		return nil, err
	}
	if err := validateNodeEmergencyCleanupIdentifier("service_code", serviceCode, 64); err != nil {
		return nil, err
	}
	if err := validateNodeEmergencyCleanupSingleLine("name", name, 256); err != nil {
		return nil, err
	}
	if err := validateNodeEmergencyCleanupSingleLine("slug", slug, 256); err != nil {
		return nil, err
	}
	if systemdUnit != "" {
		if err := validateNodeEmergencyCleanupSystemdUnit(systemdUnit); err != nil {
			return nil, err
		}
	}
	if endpointHost != "" {
		if err := validateNodeEmergencyCleanupSingleLine("endpoint_host", endpointHost, 253); err != nil {
			return nil, err
		}
	}
	if endpointPort < 0 || endpointPort > 65535 {
		return nil, fmt.Errorf("endpoint_port is invalid")
	}

	return map[string]any{
		"instance_id":          instanceID,
		"action":               "delete",
		"service_code":         serviceCode,
		"runtime_service_code": serviceCode,
		"name":                 name,
		"slug":                 slug,
		"systemd_unit":         systemdUnit,
		"endpoint_host":        endpointHost,
		"endpoint_port":        endpointPort,
		"enabled":              false,
	}, nil
}

func validateNodeEmergencyCleanupSingleLine(field, value string, maxLen int) error {
	if len(value) > maxLen {
		return fmt.Errorf("%s must be at most %d characters", field, maxLen)
	}
	if err := domain.ValidateSingleLine(field, value); err != nil {
		return err
	}
	return nil
}

func validateNodeEmergencyCleanupIdentifier(field, value string, maxLen int) error {
	if value == "" {
		return fmt.Errorf("%s is required", field)
	}
	if err := validateNodeEmergencyCleanupSingleLine(field, value, maxLen); err != nil {
		return err
	}
	for _, r := range value {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.') {
			return fmt.Errorf("%s contains unsafe characters", field)
		}
	}
	return nil
}

func validateNodeEmergencyCleanupSystemdUnit(unit string) error {
	if err := validateNodeEmergencyCleanupSingleLine("systemd_unit", unit, 256); err != nil {
		return err
	}
	for _, r := range unit {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' || r == '@' || r == ':') {
			return fmt.Errorf("systemd_unit contains unsafe characters")
		}
	}
	return nil
}

func (s *Store) QueueNodeRuntimeReconcile(ctx context.Context, nodeID, source string) ([]domain.Job, []string, error) {
	nodeID = strings.TrimSpace(nodeID)
	node, err := s.GetNode(ctx, nodeID)
	if err != nil {
		return nil, nil, err
	}
	if strings.EqualFold(node.Status, "retired") {
		return nil, nil, errors.New("node is retired")
	}
	source = strings.TrimSpace(source)
	if source == "" {
		source = "operator"
	}
	jobs := []domain.Job{}
	warnings := []string{}
	addJob := func(job domain.Job, err error, label string) {
		if err != nil {
			warnings = append(warnings, label+": "+err.Error())
			return
		}
		jobs = append(jobs, job)
	}

	job, err := s.CreateNodeInventoryJob(ctx, nodeID)
	addJob(job, err, "inventory sync")
	job, err = s.CreateNodeServiceDiscoveryJob(ctx, nodeID)
	addJob(job, err, "service discovery")

	instanceIDs, err := s.reconcilableInstanceIDsForNode(ctx, nodeID)
	if err != nil {
		warnings = append(warnings, "list instances: "+err.Error())
	} else {
		for _, instanceID := range instanceIDs {
			job, err := s.UpdateInstanceStatus(ctx, instanceID, driver.OperationApply)
			if err == nil {
				_ = s.setQueuedJobPriority(ctx, job.ID, 60)
				job.Priority = 60
			}
			addJob(job, err, "instance apply "+instanceID)
		}
	}

	backhaulIDs, err := s.reconcilableBackhaulLinkIDsForNode(ctx, nodeID)
	if err != nil {
		warnings = append(warnings, "list backhaul links: "+err.Error())
	} else {
		for _, linkID := range backhaulIDs {
			queued, err := s.CreateBackhaulApplyJobs(ctx, linkID)
			if err != nil {
				warnings = append(warnings, "backhaul apply "+linkID+": "+err.Error())
				continue
			}
			jobs = append(jobs, queued...)
		}
	}

	if routeJob, err := s.CreateNodeRoutePolicyApplyJob(ctx, nodeID); err != nil {
		warnings = append(warnings, "route policy apply: "+err.Error())
	} else {
		jobs = append(jobs, routeJob)
	}

	if firewallPolicyID, err := s.lastFirewallPolicyIDForNode(ctx, nodeID); err != nil {
		warnings = append(warnings, "firewall state lookup: "+err.Error())
	} else if firewallPolicyID != "" {
		job, err := s.CreateFirewallApplyJob(ctx, nodeID, firewallPolicyID, false)
		addJob(job, err, "firewall apply")
	}

	if len(jobs) > 0 {
		_, _ = s.CreateAudit(ctx, "system", "node.runtime_reconcile", "node", &nodeID, fmt.Sprintf("node runtime reconcile queued from %s", source))
	}
	return jobs, warnings, nil
}

func (s *Store) reconcilableInstanceIDsForNode(ctx context.Context, nodeID string) ([]string, error) {
	rows, err := s.db.Query(ctx, `select i.id::text
from instances i
join instance_revisions r on r.id=i.current_revision_id
where i.node_id=$1
  and i.status <> 'deleted'
  and (i.enabled=true or i.status in ('active','degraded','failed','provisioning'))
  and r.status in ('validated','applied')
order by i.created_at asc`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s *Store) reconcilableBackhaulLinkIDsForNode(ctx context.Context, nodeID string) ([]string, error) {
	rows, err := s.db.Query(ctx, `select id::text
from backhaul_links
where status in ('active','failed','pending_apply')
  and (ingress_node_id=$1 or egress_node_id=$1)
order by created_at asc`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s *Store) lastFirewallPolicyIDForNode(ctx context.Context, nodeID string) (string, error) {
	var policyID string
	err := s.db.QueryRow(ctx, `select coalesce(policy_id::text,'')
from firewall_node_state
where node_id=$1 and policy_id is not null
order by updated_at desc
limit 1`, nodeID).Scan(&policyID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return strings.TrimSpace(policyID), err
}

func (s *Store) setQueuedJobPriority(ctx context.Context, jobID string, priority int) error {
	if strings.TrimSpace(jobID) == "" || priority <= 0 {
		return nil
	}
	_, err := s.db.Exec(ctx, `update jobs set priority=$2 where id=$1 and status='queued'`, jobID, priority)
	return err
}

func normalizeNodeCleanupScope(value string, includeAgent bool) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "services_only", "service", "services", "instances", "instance_runtime":
		return "services_only"
	case "full_node", "full", "node", "all", "wipe":
		return "full_node"
	}
	if includeAgent {
		return "full_node"
	}
	return "services_only"
}

func (s *Store) RequeueNodeStuckJob(ctx context.Context, nodeID string) (domain.Job, error) {
	if _, err := s.GetNode(ctx, nodeID); err != nil {
		return domain.Job{}, err
	}

	type staleClaim struct {
		job         domain.Job
		lastClaimAt time.Time
		lastResult  *time.Time
	}

	stale, err := s.loadNodeStaleClaim(ctx, nodeID)
	if err != nil {
		return domain.Job{}, err
	}
	if stale == nil {
		return domain.Job{}, errors.New("no stale claimed job is waiting for requeue")
	}

	now := time.Now().UTC()
	requeued := stale.job
	requeued.ID = id.New()
	requeued.Status = "queued"
	requeued.LockedBy = nil
	requeued.LockedUntil = nil
	requeued.CreatedAt = now
	requeued.StartedAt = nil
	requeued.FinishedAt = nil
	requeued.Result = map[string]any{}

	payloadJSON := mustJSON(requeued.Payload)
	resultJSON := mustJSON(map[string]any{
		"message":            "job was marked stale and superseded by a new queued job",
		"requeued_job_id":    requeued.ID,
		"stale_last_claim":   stale.lastClaimAt.Format(time.RFC3339),
		"stale_wait_seconds": int64(time.Since(stale.lastClaimAt) / time.Second),
	})

	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.Job{}, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `update jobs
		set status='cancelled',
		    finished_at=$2,
		    locked_by=null,
		    locked_until=null,
		    result_json=$3
		where id=$1 and status in ('running','retrying')`,
		stale.job.ID, now, resultJSON); err != nil {
		return domain.Job{}, err
	}
	if _, err := tx.Exec(ctx, `delete from resource_locks where job_id=$1`, stale.job.ID); err != nil {
		return domain.Job{}, err
	}
	if _, err := tx.Exec(ctx, `insert into jobs(id,type,scope_type,scope_id,node_id,instance_id,status,priority,payload_json,result_json,created_at,started_at,finished_at,locked_by,locked_until)
		values($1,$2,$3,$4,$5,$6,$7,$8,$9,'{}'::jsonb,$10,null,null,null,null)`,
		requeued.ID, requeued.Type, requeued.ScopeType, requeued.ScopeID, requeued.NodeID, requeued.InstanceID, requeued.Status, requeued.Priority, payloadJSON, requeued.CreatedAt); err != nil {
		return domain.Job{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Job{}, err
	}

	_ = s.AddJobLog(ctx, stale.job.ID, "warn", "stale agent job was cancelled and superseded", map[string]any{
		"requeued_job_id": requeued.ID,
		"last_claim_at":   stale.lastClaimAt,
	})
	_ = s.AddJobLog(ctx, requeued.ID, "info", "job requeued from stale agent claim", map[string]any{
		"superseded_job_id": stale.job.ID,
	})
	_, _ = s.CreateAudit(ctx, "system", "node.job.requeue_stale", "node", &nodeID, "stale node job requeued")
	return s.GetJob(ctx, requeued.ID)
}

func (s *Store) loadNodeStaleClaim(ctx context.Context, nodeID string) (*struct {
	job         domain.Job
	lastClaimAt time.Time
	lastResult  *time.Time
}, error) {
	row := s.db.QueryRow(ctx, `select
		j.id,j.type,j.scope_type,j.scope_id,j.node_id,j.instance_id,j.status,j.priority,j.payload_json,coalesce(j.result_json,'{}'::jsonb),j.locked_by,j.locked_until,j.created_at,j.started_at,j.finished_at,
		na.last_job_claim_at,
		na.last_job_result_at
	from node_agents na
	join jobs j on j.id=na.last_job_claim_job_id
	where na.node_id=$1
	  and na.last_job_claim_at is not null`, nodeID)

	var payloadRaw, resultRaw []byte
	var x struct {
		job         domain.Job
		lastClaimAt time.Time
		lastResult  *time.Time
	}
	if err := row.Scan(&x.job.ID, &x.job.Type, &x.job.ScopeType, &x.job.ScopeID, &x.job.NodeID, &x.job.InstanceID, &x.job.Status, &x.job.Priority, &payloadRaw, &resultRaw, &x.job.LockedBy, &x.job.LockedUntil, &x.job.CreatedAt, &x.job.StartedAt, &x.job.FinishedAt, &x.lastClaimAt, &x.lastResult); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if err := decodeJSONField(payloadRaw, &x.job.Payload, "jobs.payload_json"); err != nil {
		return nil, err
	}
	if err := decodeJSONField(resultRaw, &x.job.Result, "jobs.result_json"); err != nil {
		return nil, err
	}
	if x.job.Payload == nil {
		x.job.Payload = map[string]any{}
	}
	if x.job.Result == nil {
		x.job.Result = map[string]any{}
	}

	if !isAgentHandledJobType(x.job.Type) {
		return nil, nil
	}
	if !strings.EqualFold(x.job.Status, "running") && !strings.EqualFold(x.job.Status, "retrying") {
		return nil, nil
	}
	if x.lastResult != nil && !x.lastResult.Before(x.lastClaimAt) {
		return nil, nil
	}
	if time.Since(x.lastClaimAt) < stuckNodeJobAfter {
		return nil, nil
	}
	return &x, nil
}

func isAgentHandledJobType(jobType string) bool {
	switch strings.TrimSpace(jobType) {
	case "node.inventory",
		"node.inventory.sync",
		"node.services.discover",
		"node.capability.install",
		"node.capability.verify",
		"node.channel.probe",
		"node.agent.rotate_token",
		"node.emergency_cleanup",
		"node.reboot",
		"node.backhaul.apply",
		"node.backhaul.probe",
		"node.backhaul.cleanup",
		"node.route_policy.apply",
		"node.route_policy.cleanup",
		"node.firewall.preview",
		"node.firewall.apply",
		"node.firewall.observe",
		"node.firewall.disable",
		"instance.restart",
		"instance.apply",
		"instance.start",
		"instance.stop",
		"instance.enable",
		"instance.disable",
		"instance.diagnose",
		"instance.delete":
		return true
	default:
		return false
	}
}
