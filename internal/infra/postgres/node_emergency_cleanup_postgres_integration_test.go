package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/platform/id"
)

type nodeEmergencyCleanupPostgresFixture struct {
	node             domain.Node
	usedEnrollment   domain.NodeEnrollmentToken
	activeEnrollment domain.NodeEnrollmentToken
	agentToken       string
	inventoryID      string
	safeJobID        string
	instances        []domain.Instance
	deletedInstance  domain.Instance
	otherInstance    domain.Instance
}

func TestPostgresIntegrationCreateNodeEmergencyCleanupJobAtomic(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)
	actorUserID := createPostgresIntegrationNodeEmergencyCleanupActor(t, ctx, store)
	fixture := createPostgresIntegrationNodeEmergencyCleanupFixture(t, ctx, store, "atomic")

	before, err := store.GetNode(ctx, fixture.node.ID)
	if err != nil {
		t.Fatalf("get node before cleanup queue: %v", err)
	}
	jobsBefore := countPostgresIntegrationEmergencyCleanupRows(t, ctx, store, `select count(*) from jobs where node_id=$1`, fixture.node.ID)
	inventoryBefore := countPostgresIntegrationEmergencyCleanupRows(t, ctx, store, `select count(*) from node_inventory_snapshots where node_id=$1`, fixture.node.ID)
	instanceRowsBefore := countPostgresIntegrationEmergencyCleanupRows(t, ctx, store, `select count(*) from instances where node_id=$1 and status <> 'deleted'`, fixture.node.ID)

	result, err := store.CreateNodeEmergencyCleanupJob(ctx, fixture.node.ID, domain.NodeEmergencyCleanupJobCreateInput{
		CleanupScope:                  domain.NodeEmergencyCleanupScopeServicesOnly,
		IncludeAgent:                  false,
		Confirmation:                  "  " + fixture.node.Name + "  ",
		Reason:                        "  maintenance window  ",
		AcknowledgeDestructiveCleanup: true,
		ActorUserID:                   &actorUserID,
	})
	if err != nil {
		t.Fatalf("create node emergency cleanup job: %v", err)
	}
	job := result.Job
	if job.ID == "" || job.Type != "node.emergency_cleanup" || job.Status != "queued" || job.Priority != 1 || job.NodeID == nil || *job.NodeID != fixture.node.ID || job.ScopeID == nil || *job.ScopeID != fixture.node.ID {
		t.Fatalf("job projection mismatch: %#v", job)
	}
	assertPostgresIntegrationNodeEmergencyCleanupPayloadSafe(t, job, fixture, domain.NodeEmergencyCleanupScopeServicesOnly, false)
	assertPostgresIntegrationNodeEmergencyCleanupSummary(t, result.PlanSummary, domain.NodeEmergencyCleanupScopeServicesOnly, false, len(fixture.instances))
	assertResourceLockCount(t, ctx, store, job.ID, "node", "bootstrap", 1)
	assertPostgresIntegrationNodeEmergencyCleanupAudit(t, ctx, store, fixture.node.ID, actorUserID, 1, fixture.agentToken, fixture.activeEnrollment.Token)

	assertPostgresCount(t, ctx, store, `select count(*) from jobs where node_id=$1`, jobsBefore+1, fixture.node.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from jobs where id=$1 and status='queued'`, 1, fixture.safeJobID)
	assertPostgresCount(t, ctx, store, `select count(*) from node_inventory_snapshots where node_id=$1`, inventoryBefore, fixture.node.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from node_inventory_snapshots where id=$1`, 1, fixture.inventoryID)
	assertPostgresCount(t, ctx, store, `select count(*) from instances where node_id=$1 and status <> 'deleted'`, instanceRowsBefore, fixture.node.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from instances where id=$1 and status='deleted'`, 1, fixture.deletedInstance.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from instances where id=$1 and status <> 'deleted'`, 1, fixture.otherInstance.ID)
	assertEnrollmentTokenStatus(t, ctx, store, fixture.usedEnrollment.ID, "used")
	assertEnrollmentTokenStatus(t, ctx, store, fixture.activeEnrollment.ID, "active")
	assertPostgresCount(t, ctx, store, `select count(*) from node_agents where node_id=$1 and status='active' and revoked_at is null and agent_token_hash is not null`, 1, fixture.node.ID)
	if !store.ValidateAgentToken(ctx, fixture.node.ID, fixture.agentToken) {
		t.Fatal("agent token must remain valid after queuing emergency cleanup")
	}
	after, err := store.GetNode(ctx, fixture.node.ID)
	if err != nil {
		t.Fatalf("get node after cleanup queue: %v", err)
	}
	if after.Name != before.Name || after.Address != before.Address || after.Role != before.Role || after.Status != before.Status || after.ExecutionMode != before.ExecutionMode || after.LocationLabel != before.LocationLabel {
		t.Fatalf("operator-owned node profile changed: before=%#v after=%#v", before, after)
	}

	_, err = store.CreateNodeEmergencyCleanupJob(ctx, fixture.node.ID, domain.NodeEmergencyCleanupJobCreateInput{
		CleanupScope:                  domain.NodeEmergencyCleanupScopeServicesOnly,
		Confirmation:                  fixture.node.Name,
		Reason:                        "second maintenance window",
		AcknowledgeDestructiveCleanup: true,
		ActorUserID:                   &actorUserID,
	})
	if !errors.Is(err, domain.ErrNodeEmergencyCleanupConflict) {
		t.Fatalf("second active cleanup error = %v, want ErrNodeEmergencyCleanupConflict", err)
	}
	markPostgresIntegrationJobTerminalWithoutSideEffects(t, ctx, store, job.ID, "cancelled")
	if err := store.HeartbeatWithVersion(ctx, fixture.node.Name, "agent-after-cleanup-test", "v1"); err != nil {
		t.Fatalf("refresh heartbeat evidence: %v", err)
	}
	nextResult, err := store.CreateNodeEmergencyCleanupJob(ctx, fixture.node.ID, domain.NodeEmergencyCleanupJobCreateInput{
		CleanupScope:                  domain.NodeEmergencyCleanupScopeFullNode,
		IncludeAgent:                  true,
		Confirmation:                  fixture.node.Name,
		Reason:                        "follow up maintenance window",
		AcknowledgeDestructiveCleanup: true,
		AcknowledgeAgentRemoval:       true,
		ActorUserID:                   &actorUserID,
	})
	if err != nil {
		t.Fatalf("create future cleanup after terminal job and fresh evidence: %v", err)
	}
	if nextResult.Job.ID == job.ID || nextResult.Job.Type != "node.emergency_cleanup" {
		t.Fatalf("future cleanup job mismatch: first=%s next=%#v", job.ID, nextResult.Job)
	}
	assertPostgresIntegrationNodeEmergencyCleanupPayloadSafe(t, nextResult.Job, fixture, domain.NodeEmergencyCleanupScopeFullNode, true)
	assertPostgresIntegrationNodeEmergencyCleanupPreconditions(t, ctx, store, actorUserID)
}

func TestPostgresIntegrationCreateNodeEmergencyCleanupJobAuditRollback(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)
	actorUserID := createPostgresIntegrationNodeEmergencyCleanupActor(t, ctx, store)
	fixture := createPostgresIntegrationNodeEmergencyCleanupFixture(t, ctx, store, "rollback")
	before, err := store.GetNode(ctx, fixture.node.ID)
	if err != nil {
		t.Fatalf("get node before rollback: %v", err)
	}
	installNodeEmergencyCleanupAuditFailureTrigger(t, ctx, store)

	_, err = store.CreateNodeEmergencyCleanupJob(ctx, fixture.node.ID, domain.NodeEmergencyCleanupJobCreateInput{
		CleanupScope:                  domain.NodeEmergencyCleanupScopeServicesOnly,
		Confirmation:                  fixture.node.Name,
		Reason:                        "maintenance window",
		AcknowledgeDestructiveCleanup: true,
		ActorUserID:                   &actorUserID,
	})
	if !errors.Is(err, domain.ErrNodeEmergencyCleanupAuditFailed) {
		t.Fatalf("create cleanup error = %v, want ErrNodeEmergencyCleanupAuditFailed", err)
	}
	assertPostgresCount(t, ctx, store, `select count(*) from jobs where node_id=$1 and type='node.emergency_cleanup'`, 0, fixture.node.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from resource_locks rl join jobs j on j.id=rl.job_id where j.node_id=$1 and j.type='node.emergency_cleanup'`, 0, fixture.node.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from audit_events where resource_id=$1 and action='node.emergency_cleanup.queue'`, 0, fixture.node.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from jobs where id=$1 and status='queued'`, 1, fixture.safeJobID)
	assertPostgresCount(t, ctx, store, `select count(*) from node_agents where node_id=$1 and status='active' and revoked_at is null and agent_token_hash is not null`, 1, fixture.node.ID)
	assertEnrollmentTokenStatus(t, ctx, store, fixture.activeEnrollment.ID, "active")
	if !store.ValidateAgentToken(ctx, fixture.node.ID, fixture.agentToken) {
		t.Fatal("agent token must remain valid after rollback")
	}
	after, err := store.GetNode(ctx, fixture.node.ID)
	if err != nil {
		t.Fatalf("get node after rollback: %v", err)
	}
	if after.AgentStatus != before.AgentStatus || after.Status != before.Status || !sameNullableTime(after.LastHeartbeatAt, before.LastHeartbeatAt) {
		t.Fatalf("node projection changed despite rollback: before=%#v after=%#v", before, after)
	}
	for _, instance := range fixture.instances {
		assertPostgresCount(t, ctx, store, `select count(*) from instances where id=$1 and status=$2 and enabled=$3`, 1, instance.ID, instance.Status, instance.Enabled)
	}
}

func TestPostgresIntegrationCreateNodeEmergencyCleanupJobConcurrentConflict(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)
	actorUserID := createPostgresIntegrationNodeEmergencyCleanupActor(t, ctx, store)
	fixture := createPostgresIntegrationNodeEmergencyCleanupFixture(t, ctx, store, "concurrent")

	const attempts = 2
	var wg sync.WaitGroup
	results := make(chan domain.NodeEmergencyCleanupJobCreateResult, attempts)
	errs := make(chan error, attempts)
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := store.CreateNodeEmergencyCleanupJob(context.Background(), fixture.node.ID, domain.NodeEmergencyCleanupJobCreateInput{
				CleanupScope:                  domain.NodeEmergencyCleanupScopeServicesOnly,
				Confirmation:                  fixture.node.Name,
				Reason:                        "maintenance window",
				AcknowledgeDestructiveCleanup: true,
				ActorUserID:                   &actorUserID,
			})
			if err == nil {
				results <- result
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)

	successes := 0
	conflicts := 0
	for err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, domain.ErrNodeEmergencyCleanupConflict):
			conflicts++
		default:
			t.Fatalf("unexpected concurrent cleanup error: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent outcomes success/conflict = %d/%d, want 1/1", successes, conflicts)
	}
	var queued domain.NodeEmergencyCleanupJobCreateResult
	for result := range results {
		queued = result
	}
	if queued.Job.ID == "" {
		t.Fatal("successful concurrent cleanup job was not captured")
	}
	assertPostgresCount(t, ctx, store, `select count(*) from jobs where node_id=$1 and type='node.emergency_cleanup' and status='queued'`, 1, fixture.node.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from audit_events where resource_id=$1 and action='node.emergency_cleanup.queue'`, 1, fixture.node.ID)
	assertResourceLockCount(t, ctx, store, queued.Job.ID, "node", "bootstrap", 1)
	assertPostgresIntegrationNodeEmergencyCleanupAudit(t, ctx, store, fixture.node.ID, actorUserID, 1, fixture.agentToken, fixture.activeEnrollment.Token)
}

func TestPostgresIntegrationCreateNodeEmergencyCleanupJobPlanFailureRollback(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)
	actorUserID := createPostgresIntegrationNodeEmergencyCleanupActor(t, ctx, store)
	fixture := createPostgresIntegrationNodeEmergencyCleanupFixture(t, ctx, store, "plan-failure")
	if _, err := store.db.Exec(ctx, `update instances set systemd_unit=$2 where id=$1`, fixture.instances[0].ID, "bad unit with spaces.service"); err != nil {
		t.Fatalf("poison systemd unit: %v", err)
	}

	_, err := store.CreateNodeEmergencyCleanupJob(ctx, fixture.node.ID, domain.NodeEmergencyCleanupJobCreateInput{
		CleanupScope:                  domain.NodeEmergencyCleanupScopeServicesOnly,
		Confirmation:                  fixture.node.Name,
		Reason:                        "maintenance window",
		AcknowledgeDestructiveCleanup: true,
		ActorUserID:                   &actorUserID,
	})
	if !errors.Is(err, domain.ErrNodeEmergencyCleanupPlanInvalid) {
		t.Fatalf("create cleanup error = %v, want ErrNodeEmergencyCleanupPlanInvalid", err)
	}
	assertPostgresCount(t, ctx, store, `select count(*) from jobs where node_id=$1 and type='node.emergency_cleanup'`, 0, fixture.node.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from resource_locks rl join jobs j on j.id=rl.job_id where j.node_id=$1 and j.type='node.emergency_cleanup'`, 0, fixture.node.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from audit_events where resource_id=$1 and action='node.emergency_cleanup.queue'`, 0, fixture.node.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from jobs where id=$1 and status='queued'`, 1, fixture.safeJobID)
	assertPostgresCount(t, ctx, store, `select count(*) from instances where id=$1 and systemd_unit=$2`, 1, fixture.instances[0].ID, "bad unit with spaces.service")
}

func createPostgresIntegrationNodeEmergencyCleanupFixture(t *testing.T, ctx context.Context, store *Store, label string) nodeEmergencyCleanupPostgresFixture {
	t.Helper()

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	node, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-node-cleanup-" + label + "-" + suffix,
		Kind:          "remote",
		Role:          "egress",
		Status:        "maintenance",
		Address:       "10.73.0.10",
		LocationLabel: "rack-" + label,
		OSFamily:      "linux",
		OSVersion:     "ubuntu-24.04",
		Architecture:  "amd64",
		ExecutionMode: "manual_bundle",
		AgentStatus:   "unknown",
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	enrollment, err := store.CreateNodeEnrollmentToken(ctx, node.ID, time.Hour)
	if err != nil {
		t.Fatalf("create enrollment token: %v", err)
	}
	registered, agentToken, err := store.RegisterAgentWithEnrollmentVersion(ctx, node.ID, enrollment.Token, "agent-name-"+suffix, "192.0.2.73", "agent-build", "v1")
	if err != nil {
		t.Fatalf("register agent: %v", err)
	}
	if registered.Name != node.Name || registered.Address != node.Address || registered.Status != "maintenance" {
		t.Fatalf("registration changed operator profile: registered=%#v node=%#v", registered, node)
	}
	activeEnrollment, err := store.CreateNodeEnrollmentToken(ctx, node.ID, time.Hour)
	if err != nil {
		t.Fatalf("create active enrollment token: %v", err)
	}
	xray := createPostgresIntegrationEmergencyCleanupInstance(t, ctx, store, node.ID, "xray-core", "xray-"+suffix, "active")
	wg := createPostgresIntegrationEmergencyCleanupInstance(t, ctx, store, node.ID, "wireguard", "wg-"+suffix, "active")
	deleted := createPostgresIntegrationEmergencyCleanupInstance(t, ctx, store, node.ID, "openvpn", "deleted-"+suffix, "active")
	if _, err := store.db.Exec(ctx, `update instances set status='deleted',enabled=false,updated_at=now() where id=$1`, deleted.ID); err != nil {
		t.Fatalf("mark deleted instance: %v", err)
	}
	otherNode := createPostgresIntegrationEmergencyCleanupBareNode(t, ctx, store, "other-"+label)
	otherInstance := createPostgresIntegrationEmergencyCleanupInstance(t, ctx, store, otherNode.ID, "xray-core", "other-"+suffix, "active")
	snap, _, err := store.SubmitNodeInventory(ctx, node.ID, map[string]any{"collector": "cleanup-test"})
	if err != nil {
		t.Fatalf("submit inventory fixture: %v", err)
	}
	safeJob, err := store.CreateNodeInventoryJob(ctx, node.ID)
	if err != nil {
		t.Fatalf("create safe inventory job fixture: %v", err)
	}
	return nodeEmergencyCleanupPostgresFixture{
		node:             node,
		usedEnrollment:   enrollment,
		activeEnrollment: activeEnrollment,
		agentToken:       agentToken,
		inventoryID:      snap.ID,
		safeJobID:        safeJob.ID,
		instances:        []domain.Instance{xray, wg},
		deletedInstance:  deleted,
		otherInstance:    otherInstance,
	}
}

func createPostgresIntegrationEmergencyCleanupInstance(t *testing.T, ctx context.Context, store *Store, nodeID, serviceCode, slug, status string) domain.Instance {
	t.Helper()

	instance, err := store.CreateInstanceDraft(ctx, domain.Instance{
		NodeID:       nodeID,
		ServiceCode:  serviceCode,
		Name:         slug,
		Slug:         slug,
		Status:       status,
		EndpointHost: "198.51.100.73",
		EndpointPort: 443,
		Spec: map[string]any{
			"non_secret_setting": "integration-test",
			"private_key":        "synthetic-secret-that-must-not-enter-cleanup-plan",
			"secret_ref_id":      "synthetic-secret-ref-that-must-not-enter-cleanup-plan",
			"config_content":     "synthetic-config-that-must-not-enter-cleanup-plan",
		},
	})
	if err != nil {
		t.Fatalf("create %s instance: %v", serviceCode, err)
	}
	return instance
}

func createPostgresIntegrationNodeEmergencyCleanupActor(t *testing.T, ctx context.Context, store *Store) string {
	t.Helper()

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	user, _, err := store.EnsureBootstrapPlatformUser(ctx, "it-cleanup-admin-"+suffix, "it-cleanup-admin-"+suffix+"@example.invalid", "Cleanup Admin", "integration-password-hash")
	if err != nil {
		t.Fatalf("create cleanup actor: %v", err)
	}
	return user.ID
}

func createPostgresIntegrationEmergencyCleanupBareNode(t *testing.T, ctx context.Context, store *Store, label string) domain.Node {
	t.Helper()

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	node, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-node-cleanup-" + label + "-" + suffix,
		Kind:          "remote",
		Role:          "egress",
		Status:        "maintenance",
		Address:       "10.74.0.10",
		OSFamily:      "linux",
		OSVersion:     "ubuntu-24.04",
		Architecture:  "amd64",
		ExecutionMode: "manual_bundle",
		AgentStatus:   "unknown",
	})
	if err != nil {
		t.Fatalf("create bare node: %v", err)
	}
	return node
}

func assertPostgresIntegrationNodeEmergencyCleanupPayloadSafe(t *testing.T, job domain.Job, fixture nodeEmergencyCleanupPostgresFixture, cleanupScope string, includeAgent bool) {
	t.Helper()

	allowed := map[string]bool{
		"schema":               true,
		"node_id":              true,
		"node_name":            true,
		"cleanup_scope":        true,
		"include_agent":        true,
		"confirmation":         true,
		"reason":               true,
		"requested_at":         true,
		"cleanup_plan_version": true,
		"instances":            true,
	}
	for key := range job.Payload {
		if !allowed[key] {
			t.Fatalf("cleanup payload contains unexpected key %q: %#v", key, job.Payload)
		}
	}
	if job.Payload["schema"] != "node.emergency_cleanup.v1" || job.Payload["node_id"] != fixture.node.ID || job.Payload["node_name"] != fixture.node.Name || job.Payload["cleanup_scope"] != cleanupScope || job.Payload["include_agent"] != includeAgent || job.Payload["confirmation"] != fixture.node.Name || job.Payload["reason"] != "maintenance window" && job.Payload["reason"] != "follow up maintenance window" {
		t.Fatalf("cleanup payload mismatch: %#v", job.Payload)
	}
	if job.Payload["cleanup_plan_version"] != domain.NodeEmergencyCleanupPlanVersion || job.Payload["requested_at"] == nil {
		t.Fatalf("cleanup payload missing version/requested_at: %#v", job.Payload)
	}
	instances, ok := job.Payload["instances"].([]any)
	if !ok {
		t.Fatalf("instances type = %T, want []any", job.Payload["instances"])
	}
	if len(instances) != len(fixture.instances) {
		t.Fatalf("instances len = %d, want %d; payload=%#v", len(instances), len(fixture.instances), job.Payload)
	}
	seen := map[string]bool{}
	for _, raw := range instances {
		target, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("target type = %T, want map[string]any", raw)
		}
		targetAllowed := map[string]bool{
			"instance_id":          true,
			"action":               true,
			"service_code":         true,
			"runtime_service_code": true,
			"name":                 true,
			"slug":                 true,
			"systemd_unit":         true,
			"endpoint_host":        true,
			"endpoint_port":        true,
			"enabled":              true,
		}
		for key := range target {
			if !targetAllowed[key] {
				t.Fatalf("target contains unexpected key %q: %#v", key, target)
			}
		}
		instanceID := stringify(target["instance_id"])
		seen[instanceID] = true
		if target["action"] != "delete" || target["enabled"] != false {
			t.Fatalf("target delete fields mismatch: %#v", target)
		}
	}
	for _, instance := range fixture.instances {
		if !seen[instance.ID] {
			t.Fatalf("cleanup plan missing instance %s: %#v", instance.ID, instances)
		}
	}
	if seen[fixture.deletedInstance.ID] || seen[fixture.otherInstance.ID] {
		t.Fatalf("cleanup plan included deleted/other-node target: %#v", instances)
	}
	payloadBytes, err := json.Marshal(job.Payload)
	if err != nil {
		t.Fatalf("marshal cleanup payload: %v", err)
	}
	payloadText := string(payloadBytes)
	for _, forbidden := range []string{"agent_token", "token_hash", "authorization", "private_key", "secret_ref", "config_content", "render_error", fixture.agentToken, fixture.activeEnrollment.Token} {
		if forbidden != "" && strings.Contains(payloadText, forbidden) {
			t.Fatalf("cleanup payload leaked forbidden value/key %q: %s", forbidden, payloadText)
		}
	}
}

func assertPostgresIntegrationNodeEmergencyCleanupSummary(t *testing.T, summary domain.NodeEmergencyCleanupPlanSummary, cleanupScope string, includeAgent bool, wantTargets int) {
	t.Helper()

	if summary.CleanupScope != cleanupScope || summary.IncludeAgent != includeAgent || summary.AgentRemovalRequested != includeAgent || summary.NodeRuntimeCleanup != (cleanupScope == domain.NodeEmergencyCleanupScopeFullNode) {
		t.Fatalf("summary flags mismatch: %#v", summary)
	}
	if summary.InstanceTargetCount != wantTargets {
		t.Fatalf("summary target count = %d, want %d", summary.InstanceTargetCount, wantTargets)
	}
	if summary.ServiceCounts["wireguard"] != 1 || summary.ServiceCounts["xray-core"] != 1 {
		t.Fatalf("summary service counts mismatch: %#v", summary.ServiceCounts)
	}
}

func assertPostgresIntegrationNodeEmergencyCleanupAudit(t *testing.T, ctx context.Context, store *Store, nodeID, actorUserID string, want int, forbidden ...string) {
	t.Helper()

	var count int
	if err := store.db.QueryRow(ctx, `select count(*) from audit_events where resource_id=$1 and action='node.emergency_cleanup.queue'`, nodeID).Scan(&count); err != nil {
		t.Fatalf("count cleanup audits: %v", err)
	}
	if count != want {
		t.Fatalf("cleanup audit count = %d, want %d", count, want)
	}
	if want == 0 {
		return
	}
	var actor *string
	var actorType string
	var resourceType string
	var summary string
	var payload string
	if err := store.db.QueryRow(ctx, `select actor_user_id,actor_type,resource_type,summary,payload_json::text from audit_events where resource_id=$1 and action='node.emergency_cleanup.queue' order by created_at desc limit 1`, nodeID).Scan(&actor, &actorType, &resourceType, &summary, &payload); err != nil {
		t.Fatalf("query cleanup audit: %v", err)
	}
	if actor == nil || *actor != actorUserID || actorType != "platform_user" || resourceType != "node" {
		t.Fatalf("audit actor/resource = actor:%v actor_type:%q resource:%q", actor, actorType, resourceType)
	}
	auditText := summary + "\n" + payload
	lowerSummary := strings.ToLower(summary)
	if !strings.Contains(lowerSummary, "queued") || strings.Contains(lowerSummary, "completed") || strings.Contains(lowerSummary, "cleaned") || strings.Contains(lowerSummary, "removed") {
		t.Fatalf("audit summary must describe queueing only: %q", summary)
	}
	for _, marker := range []string{"agent_token", "token_hash", "Authorization", "private_key", "secret_ref", "config_content"} {
		if strings.Contains(auditText, marker) {
			t.Fatalf("audit contains forbidden token metadata: %s", auditText)
		}
	}
	for _, value := range forbidden {
		if value != "" && strings.Contains(auditText, value) {
			t.Fatalf("audit leaked forbidden secret value")
		}
	}
}

func assertPostgresIntegrationNodeEmergencyCleanupPreconditions(t *testing.T, ctx context.Context, store *Store, actorUserID string) {
	t.Helper()

	online := createPostgresIntegrationNodeEmergencyCleanupFixture(t, ctx, store, "not-maintenance")
	if _, err := store.db.Exec(ctx, `update nodes set status='online' where id=$1`, online.node.ID); err != nil {
		t.Fatalf("disable maintenance fixture: %v", err)
	}
	_, err := store.CreateNodeEmergencyCleanupJob(ctx, online.node.ID, domain.NodeEmergencyCleanupJobCreateInput{
		CleanupScope:                  domain.NodeEmergencyCleanupScopeServicesOnly,
		Confirmation:                  online.node.Name,
		Reason:                        "maintenance window",
		AcknowledgeDestructiveCleanup: true,
		ActorUserID:                   &actorUserID,
	})
	if !errors.Is(err, domain.ErrNodeEmergencyCleanupMaintenanceRequired) {
		t.Fatalf("maintenance error = %v, want ErrNodeEmergencyCleanupMaintenanceRequired", err)
	}

	retired := createPostgresIntegrationEmergencyCleanupBareNode(t, ctx, store, "retired")
	if _, err := store.RetireNode(ctx, retired.ID); err != nil {
		t.Fatalf("retire cleanup precondition fixture: %v", err)
	}
	_, err = store.CreateNodeEmergencyCleanupJob(ctx, retired.ID, domain.NodeEmergencyCleanupJobCreateInput{
		CleanupScope:                  domain.NodeEmergencyCleanupScopeServicesOnly,
		Confirmation:                  retired.Name,
		Reason:                        "maintenance window",
		AcknowledgeDestructiveCleanup: true,
		ActorUserID:                   &actorUserID,
	})
	if !errors.Is(err, domain.ErrNodeEmergencyCleanupConflict) {
		t.Fatalf("retired node error = %v, want ErrNodeEmergencyCleanupConflict", err)
	}

	missing := createPostgresIntegrationEmergencyCleanupBareNode(t, ctx, store, "missing")
	_, err = store.CreateNodeEmergencyCleanupJob(ctx, missing.ID, domain.NodeEmergencyCleanupJobCreateInput{
		CleanupScope:                  domain.NodeEmergencyCleanupScopeServicesOnly,
		Confirmation:                  missing.Name,
		Reason:                        "maintenance window",
		AcknowledgeDestructiveCleanup: true,
		ActorUserID:                   &actorUserID,
	})
	if !errors.Is(err, domain.ErrNodeEmergencyCleanupAgentMissing) {
		t.Fatalf("missing agent error = %v, want ErrNodeEmergencyCleanupAgentMissing", err)
	}

	revoked := createPostgresIntegrationNodeEmergencyCleanupFixture(t, ctx, store, "revoked")
	if _, err := store.db.Exec(ctx, `update node_agents set status='revoked', revoked_at=now(), agent_token_hash=null where node_id=$1`, revoked.node.ID); err != nil {
		t.Fatalf("mark agent revoked: %v", err)
	}
	_, err = store.CreateNodeEmergencyCleanupJob(ctx, revoked.node.ID, domain.NodeEmergencyCleanupJobCreateInput{
		CleanupScope:                  domain.NodeEmergencyCleanupScopeServicesOnly,
		Confirmation:                  revoked.node.Name,
		Reason:                        "maintenance window",
		AcknowledgeDestructiveCleanup: true,
		ActorUserID:                   &actorUserID,
	})
	if !errors.Is(err, domain.ErrNodeEmergencyCleanupAgentUnavailable) {
		t.Fatalf("revoked agent error = %v, want ErrNodeEmergencyCleanupAgentUnavailable", err)
	}

	offline := createPostgresIntegrationNodeEmergencyCleanupFixture(t, ctx, store, "offline")
	old := time.Now().UTC().Add(-10 * time.Minute)
	if _, err := store.db.Exec(ctx, `update nodes set last_heartbeat_at=$2 where id=$1`, offline.node.ID, old); err != nil {
		t.Fatalf("stale node heartbeat: %v", err)
	}
	if _, err := store.db.Exec(ctx, `update node_agents
		set last_seen_at=$2,
		    last_job_poll_at=null,
		    last_job_result_at=null,
		    last_inventory_sync_at=null,
		    last_discovery_sync_at=null,
		    last_runtime_sync_at=null
		where node_id=$1`, offline.node.ID, old); err != nil {
		t.Fatalf("stale agent channel evidence: %v", err)
	}
	_, err = store.CreateNodeEmergencyCleanupJob(ctx, offline.node.ID, domain.NodeEmergencyCleanupJobCreateInput{
		CleanupScope:                  domain.NodeEmergencyCleanupScopeServicesOnly,
		Confirmation:                  offline.node.Name,
		Reason:                        "maintenance window",
		AcknowledgeDestructiveCleanup: true,
		ActorUserID:                   &actorUserID,
	})
	if !errors.Is(err, domain.ErrNodeEmergencyCleanupAgentUnavailable) {
		t.Fatalf("offline agent error = %v, want ErrNodeEmergencyCleanupAgentUnavailable", err)
	}

	rebootConflict := createPostgresIntegrationNodeEmergencyCleanupFixture(t, ctx, store, "reboot-conflict")
	if _, err := store.CreateNodeRebootJob(ctx, rebootConflict.node.ID, domain.NodeRebootJobCreateInput{
		Confirmation: rebootConflict.node.Name,
		Reason:       "maintenance window",
		ActorUserID:  &actorUserID,
	}); err != nil {
		t.Fatalf("queue reboot conflict fixture: %v", err)
	}
	_, err = store.CreateNodeEmergencyCleanupJob(ctx, rebootConflict.node.ID, domain.NodeEmergencyCleanupJobCreateInput{
		CleanupScope:                  domain.NodeEmergencyCleanupScopeServicesOnly,
		Confirmation:                  rebootConflict.node.Name,
		Reason:                        "maintenance window",
		AcknowledgeDestructiveCleanup: true,
		ActorUserID:                   &actorUserID,
	})
	if !errors.Is(err, domain.ErrNodeEmergencyCleanupConflict) {
		t.Fatalf("reboot conflict error = %v, want ErrNodeEmergencyCleanupConflict", err)
	}

	tokenConflict := createPostgresIntegrationNodeEmergencyCleanupFixture(t, ctx, store, "token-conflict")
	if _, err := store.CreateNodeAgentTokenRotateJob(ctx, tokenConflict.node.ID); err != nil {
		t.Fatalf("queue token rotation conflict fixture: %v", err)
	}
	_, err = store.CreateNodeEmergencyCleanupJob(ctx, tokenConflict.node.ID, domain.NodeEmergencyCleanupJobCreateInput{
		CleanupScope:                  domain.NodeEmergencyCleanupScopeServicesOnly,
		Confirmation:                  tokenConflict.node.Name,
		Reason:                        "maintenance window",
		AcknowledgeDestructiveCleanup: true,
		ActorUserID:                   &actorUserID,
	})
	if !errors.Is(err, domain.ErrNodeEmergencyCleanupConflict) {
		t.Fatalf("token rotation conflict error = %v, want ErrNodeEmergencyCleanupConflict", err)
	}

	instanceConflict := createPostgresIntegrationNodeEmergencyCleanupFixture(t, ctx, store, "instance-conflict")
	conflictInstance := instanceConflict.instances[0]
	if _, err := store.CreateJob(ctx, domain.Job{
		Type:       "instance.stop",
		ScopeType:  "instance",
		ScopeID:    &conflictInstance.ID,
		NodeID:     &instanceConflict.node.ID,
		InstanceID: &conflictInstance.ID,
		Payload: map[string]any{
			"instance_id":  conflictInstance.ID,
			"service_code": conflictInstance.ServiceCode,
			"systemd_unit": conflictInstance.SystemdUnit,
		},
	}); err != nil {
		t.Fatalf("queue instance conflict fixture: %v", err)
	}
	_, err = store.CreateNodeEmergencyCleanupJob(ctx, instanceConflict.node.ID, domain.NodeEmergencyCleanupJobCreateInput{
		CleanupScope:                  domain.NodeEmergencyCleanupScopeServicesOnly,
		Confirmation:                  instanceConflict.node.Name,
		Reason:                        "maintenance window",
		AcknowledgeDestructiveCleanup: true,
		ActorUserID:                   &actorUserID,
	})
	if !errors.Is(err, domain.ErrNodeEmergencyCleanupConflict) {
		t.Fatalf("instance mutation conflict error = %v, want ErrNodeEmergencyCleanupConflict", err)
	}
}

func installNodeEmergencyCleanupAuditFailureTrigger(t *testing.T, ctx context.Context, store *Store) {
	t.Helper()

	_, err := store.db.Exec(ctx, `
create or replace function fail_node_emergency_cleanup_audit_test() returns trigger
language plpgsql
as $$
begin
  if new.action = 'node.emergency_cleanup.queue' then
    raise exception 'forced node.emergency_cleanup.queue audit failure';
  end if;
  return new;
end;
$$;
create trigger fail_node_emergency_cleanup_audit_test
before insert on audit_events
for each row execute function fail_node_emergency_cleanup_audit_test();
`)
	if err != nil {
		t.Fatalf("install emergency cleanup audit failure trigger: %v", err)
	}
	t.Cleanup(func() {
		_, _ = store.db.Exec(context.Background(), `drop trigger if exists fail_node_emergency_cleanup_audit_test on audit_events; drop function if exists fail_node_emergency_cleanup_audit_test();`)
	})
}

func markPostgresIntegrationJobTerminalWithoutSideEffects(t *testing.T, ctx context.Context, store *Store, jobID, status string) {
	t.Helper()

	if _, err := store.db.Exec(ctx, `update jobs set status=$2,finished_at=now(),locked_by=null,locked_until=null where id=$1`, jobID, status); err != nil {
		t.Fatalf("mark job terminal: %v", err)
	}
	if _, err := store.db.Exec(ctx, `delete from resource_locks where job_id=$1`, jobID); err != nil {
		t.Fatalf("delete resource lock: %v", err)
	}
}

func countPostgresIntegrationEmergencyCleanupRows(t *testing.T, ctx context.Context, store *Store, query string, args ...any) int {
	t.Helper()

	var got int
	if err := store.db.QueryRow(ctx, query, args...).Scan(&got); err != nil {
		t.Fatalf("count query %q: %v", query, err)
	}
	return got
}
