package postgres

import (
	"context"
	"encoding/json"
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

func (s *Store) CreateNodeEmergencyCleanupJob(ctx context.Context, nodeID string, includeAgent bool, confirmation, cleanupScope string) (domain.Job, error) {
	node, err := s.GetNode(ctx, nodeID)
	if err != nil {
		return domain.Job{}, err
	}
	if strings.EqualFold(node.Status, "retired") {
		return domain.Job{}, errors.New("node is retired")
	}
	expectedConfirmation := strings.TrimSpace(node.Name)
	if expectedConfirmation == "" {
		expectedConfirmation = strings.TrimSpace(nodeID)
	}
	if strings.TrimSpace(confirmation) != expectedConfirmation {
		return domain.Job{}, fmt.Errorf("confirmation must match node name %q", expectedConfirmation)
	}
	cleanupScope = normalizeNodeCleanupScope(cleanupScope, includeAgent)
	instances, err := s.nodeInstanceCleanupPayloads(ctx, nodeID)
	if err != nil {
		return domain.Job{}, err
	}
	payload := map[string]any{
		"node_id":       nodeID,
		"node_name":     expectedConfirmation,
		"include_agent": includeAgent,
		"cleanup_scope": cleanupScope,
		"confirmation":  strings.TrimSpace(confirmation),
		"instances":     instances,
		"requested_at":  time.Now().UTC().Format(time.RFC3339),
	}
	job, err := s.CreateJob(ctx, domain.Job{
		Type:      "node.emergency_cleanup",
		ScopeType: "node",
		ScopeID:   &nodeID,
		NodeID:    &nodeID,
		Priority:  1,
		Payload:   payload,
	})
	if err != nil {
		return job, err
	}
	_, _ = s.CreateAudit(ctx, "system", "node.emergency_cleanup", "node", &nodeID, "node emergency cleanup queued")
	return job, nil
}

func (s *Store) CreateNodeRebootJob(ctx context.Context, nodeID, confirmation, reason string) (domain.Job, error) {
	node, err := s.GetNode(ctx, nodeID)
	if err != nil {
		return domain.Job{}, err
	}
	expectedConfirmation := strings.TrimSpace(node.Name)
	if expectedConfirmation == "" {
		expectedConfirmation = strings.TrimSpace(nodeID)
	}
	if strings.TrimSpace(confirmation) != expectedConfirmation {
		return domain.Job{}, fmt.Errorf("confirmation must match node name %q", expectedConfirmation)
	}
	payload := map[string]any{
		"node_id":      nodeID,
		"node_name":    expectedConfirmation,
		"confirmation": strings.TrimSpace(confirmation),
		"reason":       strings.TrimSpace(reason),
		"requested_at": time.Now().UTC().Format(time.RFC3339),
	}
	job, err := s.CreateJob(ctx, domain.Job{
		Type:      "node.reboot",
		ScopeType: "node",
		ScopeID:   &nodeID,
		NodeID:    &nodeID,
		Priority:  1,
		Payload:   payload,
	})
	if err != nil {
		return job, err
	}
	_, _ = s.CreateAudit(ctx, "system", "node.reboot", "node", &nodeID, "node reboot queued")
	return job, nil
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

func (s *Store) nodeInstanceCleanupPayloads(ctx context.Context, nodeID string) ([]map[string]any, error) {
	rows, err := s.db.Query(ctx, `select id from instances where node_id=$1 and status <> 'deleted' order by created_at asc`, nodeID)
	if err != nil {
		return nil, err
	}
	instanceIDs := []string{}
	for rows.Next() {
		var instanceID string
		if err := rows.Scan(&instanceID); err != nil {
			rows.Close()
			return nil, err
		}
		instanceIDs = append(instanceIDs, instanceID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	out := []map[string]any{}
	for _, instanceID := range instanceIDs {
		instance, err := s.GetInstance(ctx, instanceID)
		if err != nil {
			return nil, err
		}
		payload, err := s.buildInstanceDeleteJobPayload(ctx, instance)
		if err != nil {
			payload = map[string]any{
				"instance_id":  instance.ID,
				"action":       "delete",
				"service_code": instance.ServiceCode,
				"name":         instance.Name,
				"slug":         instance.Slug,
				"systemd_unit": instance.SystemdUnit,
				"enabled":      false,
				"spec":         map[string]any{"render_error": err.Error()},
			}
		}
		out = append(out, payload)
	}
	return out, nil
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

func (s *Store) ClearNodeStalePendingRotation(ctx context.Context, nodeID string) ([]domain.Job, error) {
	if _, err := s.GetNode(ctx, nodeID); err != nil {
		return nil, err
	}

	rows, err := s.db.Query(ctx, `select
		j.id,j.type,j.scope_type,j.scope_id,j.node_id,j.instance_id,j.status,j.priority,j.payload_json,coalesce(j.result_json,'{}'::jsonb),j.locked_by,j.locked_until,j.created_at,j.started_at,j.finished_at,
		na.last_seen_at
	from jobs j
	left join node_agents na on na.node_id=j.node_id
	where j.node_id=$1
	  and j.type='node.agent.rotate_token'
	  and j.status in ('queued','running','retrying')
	order by j.created_at asc`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	now := time.Now().UTC()
	staleJobs := make([]domain.Job, 0)
	for rows.Next() {
		var job domain.Job
		var payloadRaw, resultRaw []byte
		var lastSeenAt *time.Time
		if err := rows.Scan(&job.ID, &job.Type, &job.ScopeType, &job.ScopeID, &job.NodeID, &job.InstanceID, &job.Status, &job.Priority, &payloadRaw, &resultRaw, &job.LockedBy, &job.LockedUntil, &job.CreatedAt, &job.StartedAt, &job.FinishedAt, &lastSeenAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(payloadRaw, &job.Payload)
		_ = json.Unmarshal(resultRaw, &job.Result)
		if job.Payload == nil {
			job.Payload = map[string]any{}
		}
		if job.Result == nil {
			job.Result = map[string]any{}
		}
		if !isStalePendingRotationJob(job.CreatedAt, lastSeenAt, now) {
			continue
		}
		staleJobs = append(staleJobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(staleJobs) == 0 {
		return nil, errors.New("no stale pending rotation jobs found for this node")
	}

	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	for idx := range staleJobs {
		job := &staleJobs[idx]
		job.Status = "cancelled"
		job.FinishedAt = &now
		job.LockedBy = nil
		job.LockedUntil = nil
		resultJSON := mustJSON(map[string]any{
			"message": "stale pending token rotation was cleared",
		})
		if _, err := tx.Exec(ctx, `update jobs
			set status='cancelled',
			    finished_at=$2,
			    locked_by=null,
			    locked_until=null,
			    result_json=$3
			where id=$1 and status in ('queued','running','retrying')`,
			job.ID, now, resultJSON); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `delete from resource_locks where job_id=$1`, job.ID); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	for _, job := range staleJobs {
		_ = s.AddJobLog(ctx, job.ID, "warn", "stale token rotation was cleared", map[string]any{"node_id": nodeID})
	}
	_, _ = s.CreateAudit(ctx, "system", "node.agent_token.clear_stale", "node", &nodeID, fmt.Sprintf("%d stale token rotation jobs cleared", len(staleJobs)))
	return staleJobs, nil
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
	_ = json.Unmarshal(payloadRaw, &x.job.Payload)
	_ = json.Unmarshal(resultRaw, &x.job.Result)
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

func isStalePendingRotationJob(createdAt time.Time, lastSeenAt *time.Time, now time.Time) bool {
	if now.Sub(createdAt) < stalePendingRotateAfter {
		return false
	}
	if lastSeenAt == nil {
		return true
	}
	return lastSeenAt.Before(createdAt)
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
