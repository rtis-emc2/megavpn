package postgres

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/platform/id"
)

type nodeRebootPostgresFixture struct {
	node             domain.Node
	usedEnrollment   domain.NodeEnrollmentToken
	activeEnrollment domain.NodeEnrollmentToken
	agentToken       string
	inventoryID      string
	safeJobID        string
}

func TestPostgresIntegrationCreateNodeRebootJobAtomic(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)
	actorUserID := createPostgresIntegrationNodeRebootActor(t, ctx, store)
	fixture := createPostgresIntegrationNodeRebootFixture(t, ctx, store, "atomic")

	before, err := store.GetNode(ctx, fixture.node.ID)
	if err != nil {
		t.Fatalf("get node before reboot queue: %v", err)
	}
	inventoryBefore := countPostgresIntegrationRebootRows(t, ctx, store, `select count(*) from node_inventory_snapshots where node_id=$1`, fixture.node.ID)
	jobsBefore := countPostgresIntegrationRebootRows(t, ctx, store, `select count(*) from jobs where node_id=$1`, fixture.node.ID)

	job, err := store.CreateNodeRebootJob(ctx, fixture.node.ID, domain.NodeRebootJobCreateInput{
		Confirmation: "  " + fixture.node.Name + "  ",
		Reason:       "  maintenance window  ",
		ActorUserID:  &actorUserID,
	})
	if err != nil {
		t.Fatalf("create node reboot job: %v", err)
	}
	if job.ID == "" || job.Type != "node.reboot" || job.Status != "queued" || job.Priority != 1 || job.NodeID == nil || *job.NodeID != fixture.node.ID || job.ScopeID == nil || *job.ScopeID != fixture.node.ID {
		t.Fatalf("job projection mismatch: %#v", job)
	}
	assertPostgresIntegrationNodeRebootPayloadSafe(t, job, fixture)
	assertResourceLockCount(t, ctx, store, job.ID, "node", "bootstrap", 1)
	assertPostgresIntegrationNodeRebootAudit(t, ctx, store, fixture.node.ID, actorUserID, 1, fixture.agentToken, fixture.activeEnrollment.Token)

	assertPostgresCount(t, ctx, store, `select count(*) from jobs where node_id=$1`, jobsBefore+1, fixture.node.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from jobs where id=$1 and status='queued'`, 1, fixture.safeJobID)
	assertPostgresCount(t, ctx, store, `select count(*) from node_inventory_snapshots where node_id=$1`, inventoryBefore, fixture.node.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from node_inventory_snapshots where id=$1`, 1, fixture.inventoryID)
	assertEnrollmentTokenStatus(t, ctx, store, fixture.usedEnrollment.ID, "used")
	assertEnrollmentTokenStatus(t, ctx, store, fixture.activeEnrollment.ID, "active")
	assertPostgresCount(t, ctx, store, `select count(*) from node_agents where node_id=$1 and status='active' and revoked_at is null and agent_token_hash is not null`, 1, fixture.node.ID)
	if !store.ValidateAgentToken(ctx, fixture.node.ID, fixture.agentToken) {
		t.Fatal("agent token must remain valid after queuing reboot")
	}
	after, err := store.GetNode(ctx, fixture.node.ID)
	if err != nil {
		t.Fatalf("get node after reboot queue: %v", err)
	}
	if after.Name != before.Name || after.Address != before.Address || after.Role != before.Role || after.Status != before.Status || after.ExecutionMode != before.ExecutionMode || after.LocationLabel != before.LocationLabel {
		t.Fatalf("operator-owned node profile changed: before=%#v after=%#v", before, after)
	}

	_, err = store.CreateNodeRebootJob(ctx, fixture.node.ID, domain.NodeRebootJobCreateInput{
		Confirmation: fixture.node.Name,
		Reason:       "second maintenance window",
		ActorUserID:  &actorUserID,
	})
	if !errors.Is(err, domain.ErrNodeRebootConflict) {
		t.Fatalf("second active reboot error = %v, want ErrNodeRebootConflict", err)
	}
	if err := store.CompleteJob(ctx, job.ID, "succeeded", map[string]any{"scheduled": true, "message": "agent scheduled command only"}); err != nil {
		t.Fatalf("complete reboot job: %v", err)
	}
	assertResourceLockCount(t, ctx, store, job.ID, "node", "bootstrap", 0)
	if err := store.HeartbeatWithVersion(ctx, fixture.node.Name, "agent-after-reboot-test", "v1"); err != nil {
		t.Fatalf("refresh heartbeat evidence: %v", err)
	}
	nextJob, err := store.CreateNodeRebootJob(ctx, fixture.node.ID, domain.NodeRebootJobCreateInput{
		Confirmation: fixture.node.Name,
		Reason:       "follow up maintenance window",
		ActorUserID:  &actorUserID,
	})
	if err != nil {
		t.Fatalf("create future reboot after terminal job and fresh evidence: %v", err)
	}
	if nextJob.ID == job.ID || nextJob.Type != "node.reboot" {
		t.Fatalf("future reboot job mismatch: first=%s next=%#v", job.ID, nextJob)
	}

	assertPostgresIntegrationNodeRebootPreconditions(t, ctx, store, actorUserID)
}

func TestPostgresIntegrationCreateNodeRebootJobAuditRollback(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)
	actorUserID := createPostgresIntegrationNodeRebootActor(t, ctx, store)
	fixture := createPostgresIntegrationNodeRebootFixture(t, ctx, store, "rollback")
	before, err := store.GetNode(ctx, fixture.node.ID)
	if err != nil {
		t.Fatalf("get node before rollback: %v", err)
	}
	installNodeRebootAuditFailureTrigger(t, ctx, store)

	_, err = store.CreateNodeRebootJob(ctx, fixture.node.ID, domain.NodeRebootJobCreateInput{
		Confirmation: fixture.node.Name,
		Reason:       "maintenance window",
		ActorUserID:  &actorUserID,
	})
	if !errors.Is(err, domain.ErrNodeRebootAuditFailed) {
		t.Fatalf("create reboot error = %v, want ErrNodeRebootAuditFailed", err)
	}
	assertPostgresCount(t, ctx, store, `select count(*) from jobs where node_id=$1 and type='node.reboot'`, 0, fixture.node.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from resource_locks rl join jobs j on j.id=rl.job_id where j.node_id=$1 and j.type='node.reboot'`, 0, fixture.node.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from audit_events where resource_id=$1 and action='node.reboot.queue'`, 0, fixture.node.ID)
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
}

func TestPostgresIntegrationCreateNodeRebootJobConcurrentConflict(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)
	actorUserID := createPostgresIntegrationNodeRebootActor(t, ctx, store)
	fixture := createPostgresIntegrationNodeRebootFixture(t, ctx, store, "concurrent")

	const attempts = 2
	var wg sync.WaitGroup
	jobs := make(chan domain.Job, attempts)
	errs := make(chan error, attempts)
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			job, err := store.CreateNodeRebootJob(context.Background(), fixture.node.ID, domain.NodeRebootJobCreateInput{
				Confirmation: fixture.node.Name,
				Reason:       "maintenance window",
				ActorUserID:  &actorUserID,
			})
			if err == nil {
				jobs <- job
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(jobs)
	close(errs)

	successes := 0
	conflicts := 0
	for err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, domain.ErrNodeRebootConflict):
			conflicts++
		default:
			t.Fatalf("unexpected concurrent reboot error: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent outcomes success/conflict = %d/%d, want 1/1", successes, conflicts)
	}
	var queuedJob domain.Job
	for job := range jobs {
		queuedJob = job
	}
	if queuedJob.ID == "" {
		t.Fatal("successful concurrent job was not captured")
	}
	assertPostgresCount(t, ctx, store, `select count(*) from jobs where node_id=$1 and type='node.reboot' and status='queued'`, 1, fixture.node.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from audit_events where resource_id=$1 and action='node.reboot.queue'`, 1, fixture.node.ID)
	assertResourceLockCount(t, ctx, store, queuedJob.ID, "node", "bootstrap", 1)
	assertPostgresIntegrationNodeRebootAudit(t, ctx, store, fixture.node.ID, actorUserID, 1, fixture.agentToken, fixture.activeEnrollment.Token)
}

func createPostgresIntegrationNodeRebootFixture(t *testing.T, ctx context.Context, store *Store, label string) nodeRebootPostgresFixture {
	t.Helper()

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	node, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-node-reboot-" + label + "-" + suffix,
		Kind:          "remote",
		Role:          "egress",
		Status:        "maintenance",
		Address:       "10.71.0.10",
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
	registered, agentToken, err := store.RegisterAgentWithEnrollmentVersion(ctx, node.ID, enrollment.Token, "agent-name-"+suffix, "192.0.2.71", "agent-build", "v1")
	if err != nil {
		t.Fatalf("register agent: %v", err)
	}
	if registered.Name != node.Name || registered.Address != node.Address {
		t.Fatalf("registration changed operator profile: registered=%#v node=%#v", registered, node)
	}
	activeEnrollment, err := store.CreateNodeEnrollmentToken(ctx, node.ID, time.Hour)
	if err != nil {
		t.Fatalf("create active enrollment token: %v", err)
	}
	snap, _, err := store.SubmitNodeInventory(ctx, node.ID, map[string]any{"collector": "reboot-test"})
	if err != nil {
		t.Fatalf("submit inventory fixture: %v", err)
	}
	safeJob, err := store.CreateNodeInventoryJob(ctx, node.ID)
	if err != nil {
		t.Fatalf("create safe inventory job fixture: %v", err)
	}
	return nodeRebootPostgresFixture{
		node:             node,
		usedEnrollment:   enrollment,
		activeEnrollment: activeEnrollment,
		agentToken:       agentToken,
		inventoryID:      snap.ID,
		safeJobID:        safeJob.ID,
	}
}

func createPostgresIntegrationNodeRebootActor(t *testing.T, ctx context.Context, store *Store) string {
	t.Helper()

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	user, _, err := store.EnsureBootstrapPlatformUser(ctx, "it-reboot-admin-"+suffix, "it-reboot-admin-"+suffix+"@example.invalid", "Reboot Admin", "integration-password-hash")
	if err != nil {
		t.Fatalf("create reboot actor: %v", err)
	}
	return user.ID
}

func assertPostgresIntegrationNodeRebootPayloadSafe(t *testing.T, job domain.Job, fixture nodeRebootPostgresFixture) {
	t.Helper()

	allowed := map[string]bool{
		"schema":       true,
		"node_id":      true,
		"node_name":    true,
		"confirmation": true,
		"reason":       true,
		"requested_at": true,
	}
	for key := range job.Payload {
		if !allowed[key] {
			t.Fatalf("reboot payload contains unexpected key %q: %#v", key, job.Payload)
		}
	}
	if job.Payload["node_id"] != fixture.node.ID || job.Payload["node_name"] != fixture.node.Name || job.Payload["confirmation"] != fixture.node.Name || job.Payload["reason"] != "maintenance window" {
		t.Fatalf("reboot payload mismatch: %#v", job.Payload)
	}
	if job.Payload["requested_at"] == nil {
		t.Fatalf("reboot payload missing requested_at: %#v", job.Payload)
	}
	payloadText := stringify(job.Payload["node_id"]) + stringify(job.Payload["node_name"]) + stringify(job.Payload["confirmation"]) + stringify(job.Payload["reason"]) + stringify(job.Payload["requested_at"])
	for _, forbidden := range []string{"agent_token", "token_hash", "authorization", fixture.agentToken, fixture.activeEnrollment.Token} {
		if forbidden != "" && strings.Contains(payloadText, forbidden) {
			t.Fatalf("reboot payload leaked forbidden value/key %q: %#v", forbidden, job.Payload)
		}
	}
}

func assertPostgresIntegrationNodeRebootAudit(t *testing.T, ctx context.Context, store *Store, nodeID, actorUserID string, want int, forbidden ...string) {
	t.Helper()

	var count int
	if err := store.db.QueryRow(ctx, `select count(*) from audit_events where resource_id=$1 and action='node.reboot.queue'`, nodeID).Scan(&count); err != nil {
		t.Fatalf("count reboot audits: %v", err)
	}
	if count != want {
		t.Fatalf("reboot audit count = %d, want %d", count, want)
	}
	if want == 0 {
		return
	}
	var actor *string
	var actorType string
	var resourceType string
	var summary string
	var payload string
	if err := store.db.QueryRow(ctx, `select actor_user_id,actor_type,resource_type,summary,payload_json::text from audit_events where resource_id=$1 and action='node.reboot.queue' order by created_at desc limit 1`, nodeID).Scan(&actor, &actorType, &resourceType, &summary, &payload); err != nil {
		t.Fatalf("query reboot audit: %v", err)
	}
	if actor == nil || *actor != actorUserID || actorType != "platform_user" || resourceType != "node" {
		t.Fatalf("audit actor/resource = actor:%v actor_type:%q resource:%q", actor, actorType, resourceType)
	}
	auditText := summary + "\n" + payload
	if !strings.Contains(summary, "queued") || strings.Contains(strings.ToLower(summary), "rebooted") || strings.Contains(strings.ToLower(summary), "completed") {
		t.Fatalf("audit summary must describe queueing only: %q", summary)
	}
	for _, marker := range []string{"agent_token", "token_hash", "Authorization", "private_key", "secret_ref"} {
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

func assertPostgresIntegrationNodeRebootPreconditions(t *testing.T, ctx context.Context, store *Store, actorUserID string) {
	t.Helper()

	retired := createPostgresIntegrationRebootBareNode(t, ctx, store, "retired")
	if _, err := store.RetireNode(ctx, retired.ID); err != nil {
		t.Fatalf("retire reboot precondition fixture: %v", err)
	}
	_, err := store.CreateNodeRebootJob(ctx, retired.ID, domain.NodeRebootJobCreateInput{
		Confirmation: retired.Name,
		Reason:       "maintenance window",
		ActorUserID:  &actorUserID,
	})
	if !errors.Is(err, domain.ErrNodeRebootConflict) {
		t.Fatalf("retired node error = %v, want ErrNodeRebootConflict", err)
	}

	missing := createPostgresIntegrationRebootBareNode(t, ctx, store, "missing")
	_, err = store.CreateNodeRebootJob(ctx, missing.ID, domain.NodeRebootJobCreateInput{
		Confirmation: missing.Name,
		Reason:       "maintenance window",
		ActorUserID:  &actorUserID,
	})
	if !errors.Is(err, domain.ErrNodeRebootAgentMissing) {
		t.Fatalf("missing agent error = %v, want ErrNodeRebootAgentMissing", err)
	}

	revoked := createPostgresIntegrationNodeRebootFixture(t, ctx, store, "revoked")
	if _, err := store.db.Exec(ctx, `update node_agents set status='revoked', revoked_at=now(), agent_token_hash=null where node_id=$1`, revoked.node.ID); err != nil {
		t.Fatalf("mark agent revoked: %v", err)
	}
	_, err = store.CreateNodeRebootJob(ctx, revoked.node.ID, domain.NodeRebootJobCreateInput{
		Confirmation: revoked.node.Name,
		Reason:       "maintenance window",
		ActorUserID:  &actorUserID,
	})
	if !errors.Is(err, domain.ErrNodeRebootAgentUnavailable) {
		t.Fatalf("revoked agent error = %v, want ErrNodeRebootAgentUnavailable", err)
	}

	offline := createPostgresIntegrationNodeRebootFixture(t, ctx, store, "offline")
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
	_, err = store.CreateNodeRebootJob(ctx, offline.node.ID, domain.NodeRebootJobCreateInput{
		Confirmation: offline.node.Name,
		Reason:       "maintenance window",
		ActorUserID:  &actorUserID,
	})
	if !errors.Is(err, domain.ErrNodeRebootAgentUnavailable) {
		t.Fatalf("offline agent error = %v, want ErrNodeRebootAgentUnavailable", err)
	}
}

func createPostgresIntegrationRebootBareNode(t *testing.T, ctx context.Context, store *Store, label string) domain.Node {
	t.Helper()

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	node, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-node-reboot-" + label + "-" + suffix,
		Kind:          "remote",
		Role:          "egress",
		Status:        "maintenance",
		Address:       "10.72.0.10",
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

func installNodeRebootAuditFailureTrigger(t *testing.T, ctx context.Context, store *Store) {
	t.Helper()

	_, err := store.db.Exec(ctx, `
create or replace function fail_node_reboot_audit_test() returns trigger
language plpgsql
as $$
begin
  if new.action = 'node.reboot.queue' then
    raise exception 'forced node.reboot.queue audit failure';
  end if;
  return new;
end;
$$;
create trigger fail_node_reboot_audit_test
before insert on audit_events
for each row execute function fail_node_reboot_audit_test();
`)
	if err != nil {
		t.Fatalf("install reboot audit failure trigger: %v", err)
	}
	t.Cleanup(func() {
		_, _ = store.db.Exec(context.Background(), `drop trigger if exists fail_node_reboot_audit_test on audit_events; drop function if exists fail_node_reboot_audit_test();`)
	})
}

func countPostgresIntegrationRebootRows(t *testing.T, ctx context.Context, store *Store, query string, args ...any) int {
	t.Helper()

	var got int
	if err := store.db.QueryRow(ctx, query, args...).Scan(&got); err != nil {
		t.Fatalf("count query %q: %v", query, err)
	}
	return got
}
