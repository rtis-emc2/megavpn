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

type nodeStaleRotationPostgresFixture struct {
	node          domain.Node
	agentToken    string
	rotationJob   domain.Job
	rotationToken string
	secretRefID   string
}

func TestPostgresIntegrationClearNodeStaleRotationAtomic(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)
	actorUserID := createPostgresIntegrationNodeStaleRotationActor(t, ctx, store)
	fixture := createPostgresIntegrationNodeStaleRotationFixture(t, ctx, store, "atomic")
	markPostgresIntegrationStaleRotationClaimed(t, ctx, store, fixture.node.ID, fixture.rotationJob.ID)
	insertPostgresIntegrationRotationLock(t, ctx, store, fixture.node.ID, fixture.rotationJob.ID)

	if !store.ValidateAgentToken(ctx, fixture.node.ID, fixture.agentToken) {
		t.Fatal("active agent token must validate before cleanup")
	}
	if !store.ValidateAgentToken(ctx, fixture.node.ID, fixture.rotationToken) {
		t.Fatal("pending rotation token must validate before cleanup")
	}
	preview, err := store.PreviewNodeStaleRotation(ctx, fixture.node.ID)
	if err != nil {
		t.Fatalf("preview stale rotation: %v", err)
	}
	if !preview.StaleRotationDetected || preview.TokenRotationStatus != "rotating" || len(preview.Candidates) != 1 || !preview.Candidates[0].SafeToClear {
		t.Fatalf("preview mismatch: %#v", preview)
	}

	result, err := store.ClearNodeStalePendingRotation(ctx, fixture.node.ID, domain.NodeStaleRotationClearInput{
		Confirmation:              "  " + fixture.node.Name + "  ",
		Reason:                    "  maintenance window  ",
		AcknowledgeCancelRotation: true,
		ExpectedJobIDs:            []string{fixture.rotationJob.ID},
		ActorUserID:               &actorUserID,
	})
	if err != nil {
		t.Fatalf("clear stale rotation: %v", err)
	}
	if result.Status != "cleared" || result.NodeID != fixture.node.ID || result.ClearedCount != 1 || !result.PendingRotationStateCleared || !result.ActiveAgentIdentityPreserved {
		t.Fatalf("clear result mismatch: %#v", result)
	}
	if len(result.ClearedJobs) != 1 || result.ClearedJobs[0].JobID != fixture.rotationJob.ID || result.ClearedJobs[0].PreviousStatus != "running" || result.ClearedJobs[0].Status != "cancelled" {
		t.Fatalf("cleared job result mismatch: %#v", result.ClearedJobs)
	}
	assertPostgresCount(t, ctx, store, `select count(*) from jobs where id=$1 and status='cancelled' and locked_by is null and locked_until is null`, 1, fixture.rotationJob.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from resource_locks where job_id=$1`, 0, fixture.rotationJob.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from secret_refs where id=$1`, 0, fixture.secretRefID)
	assertPostgresCount(t, ctx, store, `select count(*) from job_logs where job_id=$1 and message='stale agent token rotation cancelled by operator'`, 1, fixture.rotationJob.ID)
	assertPostgresIntegrationNodeStaleRotationAudit(t, ctx, store, fixture.node.ID, actorUserID, 1, fixture.agentToken, fixture.rotationToken, fixture.secretRefID)
	assertPostgresIntegrationNodeStaleRotationResultSafe(t, ctx, store, fixture.rotationJob.ID, fixture.agentToken, fixture.rotationToken, fixture.secretRefID)

	if !store.ValidateAgentToken(ctx, fixture.node.ID, fixture.agentToken) {
		t.Fatal("active agent token must remain valid after cleanup")
	}
	if store.ValidateAgentToken(ctx, fixture.node.ID, fixture.rotationToken) {
		t.Fatal("pending rotation token must not validate after cleanup")
	}
	assertPostgresCount(t, ctx, store, `select count(*) from node_agents where node_id=$1 and status='active' and revoked_at is null and agent_token_hash is not null`, 1, fixture.node.ID)
	diag, err := store.GetNodeDiagnostics(ctx, fixture.node.ID)
	if err != nil {
		t.Fatalf("diagnostics after cleanup: %v", err)
	}
	if diag.Agent.TokenRotationStatus == "rotating" {
		t.Fatalf("diagnostics still reports rotating after cleanup: %#v", diag.Agent)
	}
}

func TestPostgresIntegrationClearNodeStaleRotationAuditRollback(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)
	actorUserID := createPostgresIntegrationNodeStaleRotationActor(t, ctx, store)
	fixture := createPostgresIntegrationNodeStaleRotationFixture(t, ctx, store, "audit-rollback")
	markPostgresIntegrationStaleRotationClaimed(t, ctx, store, fixture.node.ID, fixture.rotationJob.ID)
	insertPostgresIntegrationRotationLock(t, ctx, store, fixture.node.ID, fixture.rotationJob.ID)
	installNodeStaleRotationAuditFailureTrigger(t, ctx, store)

	_, err := store.ClearNodeStalePendingRotation(ctx, fixture.node.ID, domain.NodeStaleRotationClearInput{
		Confirmation:              fixture.node.Name,
		Reason:                    "maintenance window",
		AcknowledgeCancelRotation: true,
		ExpectedJobIDs:            []string{fixture.rotationJob.ID},
		ActorUserID:               &actorUserID,
	})
	if !errors.Is(err, domain.ErrNodeStaleRotationAuditFailed) {
		t.Fatalf("clear error = %v, want ErrNodeStaleRotationAuditFailed", err)
	}
	assertPostgresCount(t, ctx, store, `select count(*) from jobs where id=$1 and status='running'`, 1, fixture.rotationJob.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from resource_locks where job_id=$1`, 1, fixture.rotationJob.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from secret_refs where id=$1`, 1, fixture.secretRefID)
	assertPostgresCount(t, ctx, store, `select count(*) from job_logs where job_id=$1`, 0, fixture.rotationJob.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from audit_events where resource_id=$1 and action='node.agent_token.clear_stale'`, 0, fixture.node.ID)
	if !store.ValidateAgentToken(ctx, fixture.node.ID, fixture.agentToken) {
		t.Fatal("active token must remain valid after rollback")
	}
	if !store.ValidateAgentToken(ctx, fixture.node.ID, fixture.rotationToken) {
		t.Fatal("pending token must remain valid after rollback")
	}
}

func TestPostgresIntegrationClearNodeStaleRotationPreviewChanged(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)
	actorUserID := createPostgresIntegrationNodeStaleRotationActor(t, ctx, store)
	fixture := createPostgresIntegrationNodeStaleRotationFixture(t, ctx, store, "preview-changed")
	markPostgresIntegrationStaleRotationClaimed(t, ctx, store, fixture.node.ID, fixture.rotationJob.ID)
	insertPostgresIntegrationRotationLock(t, ctx, store, fixture.node.ID, fixture.rotationJob.ID)

	preview, err := store.PreviewNodeStaleRotation(ctx, fixture.node.ID)
	if err != nil {
		t.Fatalf("preview stale rotation: %v", err)
	}
	if len(safeNodeStaleRotationCandidateIDs(preview.Candidates)) != 1 {
		t.Fatalf("expected one safe candidate before progress: %#v", preview.Candidates)
	}
	if _, err := store.db.Exec(ctx, `update node_agents set last_job_poll_at=now() where node_id=$1`, fixture.node.ID); err != nil {
		t.Fatalf("mark fresh poll evidence: %v", err)
	}

	_, err = store.ClearNodeStalePendingRotation(ctx, fixture.node.ID, domain.NodeStaleRotationClearInput{
		Confirmation:              fixture.node.Name,
		Reason:                    "maintenance window",
		AcknowledgeCancelRotation: true,
		ExpectedJobIDs:            []string{fixture.rotationJob.ID},
		ActorUserID:               &actorUserID,
	})
	if !errors.Is(err, domain.ErrNodeStaleRotationPreviewChanged) {
		t.Fatalf("clear error = %v, want ErrNodeStaleRotationPreviewChanged", err)
	}
	assertPostgresCount(t, ctx, store, `select count(*) from jobs where id=$1 and status='running'`, 1, fixture.rotationJob.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from resource_locks where job_id=$1`, 1, fixture.rotationJob.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from secret_refs where id=$1`, 1, fixture.secretRefID)
}

func TestPostgresIntegrationClearNodeStaleRotationPendingStateAmbiguousRollback(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)
	actorUserID := createPostgresIntegrationNodeStaleRotationActor(t, ctx, store)
	fixture := createPostgresIntegrationNodeStaleRotationFixture(t, ctx, store, "pending-ambiguous")
	markPostgresIntegrationStaleRotationClaimed(t, ctx, store, fixture.node.ID, fixture.rotationJob.ID)
	insertPostgresIntegrationRotationLock(t, ctx, store, fixture.node.ID, fixture.rotationJob.ID)

	if _, err := store.db.Exec(ctx, `delete from secret_refs where id=$1`, fixture.secretRefID); err != nil {
		t.Fatalf("delete rotation secret ref before cleanup: %v", err)
	}
	_, err := store.ClearNodeStalePendingRotation(ctx, fixture.node.ID, domain.NodeStaleRotationClearInput{
		Confirmation:              fixture.node.Name,
		Reason:                    "maintenance window",
		AcknowledgeCancelRotation: true,
		ExpectedJobIDs:            []string{fixture.rotationJob.ID},
		ActorUserID:               &actorUserID,
	})
	if !errors.Is(err, domain.ErrNodeStaleRotationPendingStateAmbiguous) {
		t.Fatalf("clear error = %v, want ErrNodeStaleRotationPendingStateAmbiguous", err)
	}
	assertPostgresCount(t, ctx, store, `select count(*) from jobs where id=$1 and status='running'`, 1, fixture.rotationJob.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from resource_locks where job_id=$1`, 1, fixture.rotationJob.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from job_logs where job_id=$1`, 0, fixture.rotationJob.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from audit_events where resource_id=$1 and action='node.agent_token.clear_stale'`, 0, fixture.node.ID)
	if !store.ValidateAgentToken(ctx, fixture.node.ID, fixture.agentToken) {
		t.Fatal("active token must remain valid after pending ambiguity")
	}
}

func TestPostgresIntegrationClearNodeStaleRotationConcurrent(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)
	actorUserID := createPostgresIntegrationNodeStaleRotationActor(t, ctx, store)
	fixture := createPostgresIntegrationNodeStaleRotationFixture(t, ctx, store, "concurrent")
	markPostgresIntegrationStaleRotationClaimed(t, ctx, store, fixture.node.ID, fixture.rotationJob.ID)
	insertPostgresIntegrationRotationLock(t, ctx, store, fixture.node.ID, fixture.rotationJob.ID)

	const attempts = 2
	var wg sync.WaitGroup
	errs := make(chan error, attempts)
	results := make(chan domain.NodeStaleRotationClearResult, attempts)
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := store.ClearNodeStalePendingRotation(context.Background(), fixture.node.ID, domain.NodeStaleRotationClearInput{
				Confirmation:              fixture.node.Name,
				Reason:                    "maintenance window",
				AcknowledgeCancelRotation: true,
				ExpectedJobIDs:            []string{fixture.rotationJob.ID},
				ActorUserID:               &actorUserID,
			})
			if err == nil {
				results <- result
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	close(results)

	successes := 0
	notFoundOrChanged := 0
	for err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, domain.ErrNodeStaleRotationNotFound), errors.Is(err, domain.ErrNodeStaleRotationPreviewChanged):
			notFoundOrChanged++
		default:
			t.Fatalf("unexpected concurrent cleanup error: %v", err)
		}
	}
	if successes != 1 || notFoundOrChanged != attempts-1 {
		t.Fatalf("concurrent outcomes success/not-found-or-changed = %d/%d, want 1/%d", successes, notFoundOrChanged, attempts-1)
	}
	for result := range results {
		if result.ClearedCount != 1 || result.ClearedJobs[0].JobID != fixture.rotationJob.ID {
			t.Fatalf("successful concurrent result mismatch: %#v", result)
		}
	}
	assertPostgresCount(t, ctx, store, `select count(*) from jobs where id=$1 and status='cancelled'`, 1, fixture.rotationJob.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from audit_events where resource_id=$1 and action='node.agent_token.clear_stale'`, 1, fixture.node.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from job_logs where job_id=$1`, 1, fixture.rotationJob.ID)
}

func TestPostgresIntegrationClearNodeStaleRotationPreservesActiveToken(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)
	actorUserID := createPostgresIntegrationNodeStaleRotationActor(t, ctx, store)
	fixture := createPostgresIntegrationNodeStaleRotationFixture(t, ctx, store, "preserve-active")
	markPostgresIntegrationStaleRotationQueued(t, ctx, store, fixture.node.ID, fixture.rotationJob.ID)
	unrelated := createPostgresIntegrationNodeStaleRotationFixture(t, ctx, store, "unrelated")
	revoked := createPostgresIntegrationNodeStaleRotationFixture(t, ctx, store, "revoked")
	if _, err := store.db.Exec(ctx, `update node_agents set status='revoked', revoked_at=now(), agent_token_hash=null where node_id=$1`, revoked.node.ID); err != nil {
		t.Fatalf("revoke unrelated fixture agent: %v", err)
	}
	if _, err := store.db.Exec(ctx, `update nodes set agent_status='revoked', updated_at=now() where id=$1`, revoked.node.ID); err != nil {
		t.Fatalf("mark unrelated fixture node revoked: %v", err)
	}

	if !store.ValidateAgentToken(ctx, fixture.node.ID, fixture.agentToken) || !store.ValidateAgentToken(ctx, fixture.node.ID, fixture.rotationToken) {
		t.Fatal("target active and pending tokens must validate before cleanup")
	}
	if !store.ValidateAgentToken(ctx, unrelated.node.ID, unrelated.agentToken) {
		t.Fatal("unrelated active token must validate before cleanup")
	}
	if store.ValidateAgentToken(ctx, revoked.node.ID, revoked.agentToken) {
		t.Fatal("revoked identity token must not validate before cleanup")
	}

	_, err := store.ClearNodeStalePendingRotation(ctx, fixture.node.ID, domain.NodeStaleRotationClearInput{
		Confirmation:              fixture.node.Name,
		Reason:                    "maintenance window",
		AcknowledgeCancelRotation: true,
		ExpectedJobIDs:            []string{fixture.rotationJob.ID},
		ActorUserID:               &actorUserID,
	})
	if err != nil {
		t.Fatalf("clear stale queued rotation: %v", err)
	}
	if !store.ValidateAgentToken(ctx, fixture.node.ID, fixture.agentToken) {
		t.Fatal("target active token must validate after cleanup")
	}
	if store.ValidateAgentToken(ctx, fixture.node.ID, fixture.rotationToken) {
		t.Fatal("target pending token must not validate after cleanup")
	}
	if !store.ValidateAgentToken(ctx, unrelated.node.ID, unrelated.agentToken) {
		t.Fatal("unrelated active token must remain valid after cleanup")
	}
	if store.ValidateAgentToken(ctx, revoked.node.ID, revoked.agentToken) {
		t.Fatal("cleanup must not restore a revoked identity")
	}
	assertPostgresCount(t, ctx, store, `select count(*) from node_agents where node_id=$1 and status='active' and revoked_at is null and agent_token_hash is not null`, 1, fixture.node.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from node_agents where node_id=$1 and status='revoked' and revoked_at is not null and agent_token_hash is null`, 1, revoked.node.ID)
}

func createPostgresIntegrationNodeStaleRotationFixture(t *testing.T, ctx context.Context, store *Store, label string) nodeStaleRotationPostgresFixture {
	t.Helper()

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	node, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-stale-rotation-" + label + "-" + suffix,
		Kind:          "remote",
		Role:          "egress",
		Status:        "maintenance",
		Address:       "10.72.0.10",
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
	registered, agentToken, err := store.RegisterAgentWithEnrollmentVersion(ctx, node.ID, enrollment.Token, "agent-name-"+suffix, "192.0.2.72", "agent-build", "v1")
	if err != nil {
		t.Fatalf("register agent: %v", err)
	}
	if registered.Name != node.Name || registered.Address != node.Address {
		t.Fatalf("registration changed operator profile: registered=%#v node=%#v", registered, node)
	}
	rotationJob, err := store.CreateNodeAgentTokenRotateJob(ctx, node.ID)
	if err != nil {
		t.Fatalf("create token rotation job: %v", err)
	}
	rotationToken := resolvePostgresIntegrationRotateToken(t, ctx, store, rotationJob)
	secretRefID := strings.TrimSpace(stringify(rotationJob.Payload["new_agent_token_secret_ref_id"]))
	if secretRefID == "" {
		t.Fatal("rotation job missing secret ref")
	}
	return nodeStaleRotationPostgresFixture{
		node:          node,
		agentToken:    agentToken,
		rotationJob:   rotationJob,
		rotationToken: rotationToken,
		secretRefID:   secretRefID,
	}
}

func createPostgresIntegrationNodeStaleRotationActor(t *testing.T, ctx context.Context, store *Store) string {
	t.Helper()

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	user, _, err := store.EnsureBootstrapPlatformUser(ctx, "it-stale-rotation-admin-"+suffix, "it-stale-rotation-admin-"+suffix+"@example.invalid", "Stale Rotation Admin", "integration-password-hash")
	if err != nil {
		t.Fatalf("create stale rotation actor: %v", err)
	}
	return user.ID
}

func markPostgresIntegrationStaleRotationClaimed(t *testing.T, ctx context.Context, store *Store, nodeID, jobID string) {
	t.Helper()

	now := time.Now().UTC()
	createdAt := now.Add(-12 * time.Minute)
	startedAt := now.Add(-11 * time.Minute)
	claimAt := now.Add(-10 * time.Minute)
	lockedUntil := now.Add(-5 * time.Minute)
	lastSeenAt := now.Add(-30 * time.Minute)
	lastPollAt := now.Add(-20 * time.Minute)
	owner := "agent:" + nodeID
	if _, err := store.db.Exec(ctx, `update jobs
		set status='running',
		    created_at=$2,
		    started_at=$3,
		    locked_by=$4,
		    locked_until=$5
		where id=$1`, jobID, createdAt, startedAt, owner, lockedUntil); err != nil {
		t.Fatalf("mark rotation job claimed: %v", err)
	}
	if _, err := store.db.Exec(ctx, `update node_agents
		set last_seen_at=$2,
		    last_job_poll_at=$3,
		    last_job_claim_at=$4,
		    last_job_claim_job_id=$5,
		    last_job_claim_type='node.agent.rotate_token',
		    last_job_result_at=null,
		    last_job_result_job_id=null,
		    last_job_result_type='',
		    last_job_result_status=''
		where node_id=$1`, nodeID, lastSeenAt, lastPollAt, claimAt, jobID); err != nil {
		t.Fatalf("mark agent stale claimed evidence: %v", err)
	}
	if _, err := store.db.Exec(ctx, `update nodes set last_heartbeat_at=$2 where id=$1`, nodeID, lastSeenAt); err != nil {
		t.Fatalf("mark node heartbeat stale: %v", err)
	}
}

func markPostgresIntegrationStaleRotationQueued(t *testing.T, ctx context.Context, store *Store, nodeID, jobID string) {
	t.Helper()

	now := time.Now().UTC()
	createdAt := now.Add(-12 * time.Minute)
	lastSeenAt := now.Add(-30 * time.Minute)
	if _, err := store.db.Exec(ctx, `update jobs
		set status='queued',
		    created_at=$2,
		    started_at=null,
		    locked_by=null,
		    locked_until=null
		where id=$1`, jobID, createdAt); err != nil {
		t.Fatalf("mark rotation job queued stale: %v", err)
	}
	if _, err := store.db.Exec(ctx, `update node_agents
		set last_seen_at=$2,
		    last_job_poll_at=null,
		    last_job_claim_at=null,
		    last_job_claim_job_id=null,
		    last_job_claim_type='',
		    last_job_result_at=null,
		    last_job_result_job_id=null,
		    last_job_result_type='',
		    last_job_result_status=''
		where node_id=$1`, nodeID, lastSeenAt); err != nil {
		t.Fatalf("mark agent stale queued evidence: %v", err)
	}
	if _, err := store.db.Exec(ctx, `update nodes set last_heartbeat_at=$2 where id=$1`, nodeID, lastSeenAt); err != nil {
		t.Fatalf("mark node heartbeat stale: %v", err)
	}
}

func insertPostgresIntegrationRotationLock(t *testing.T, ctx context.Context, store *Store, nodeID, jobID string) {
	t.Helper()

	if _, err := store.db.Exec(ctx, `insert into resource_locks(id,resource_type,resource_id,lock_kind,job_id,expires_at)
		values($1,'node',$2,'bootstrap',$3,$4)`, id.New(), nodeID, jobID, time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatalf("insert stale rotation resource lock: %v", err)
	}
}

func assertPostgresIntegrationNodeStaleRotationAudit(t *testing.T, ctx context.Context, store *Store, nodeID, actorUserID string, want int, forbidden ...string) {
	t.Helper()

	var count int
	if err := store.db.QueryRow(ctx, `select count(*) from audit_events where resource_id=$1 and action='node.agent_token.clear_stale'`, nodeID).Scan(&count); err != nil {
		t.Fatalf("count stale rotation audits: %v", err)
	}
	if count != want {
		t.Fatalf("stale rotation audit count = %d, want %d", count, want)
	}
	if want == 0 {
		return
	}
	var actor *string
	var actorType string
	var resourceType string
	var summary string
	var payload string
	if err := store.db.QueryRow(ctx, `select actor_user_id,actor_type,resource_type,summary,payload_json::text from audit_events where resource_id=$1 and action='node.agent_token.clear_stale' order by created_at desc limit 1`, nodeID).Scan(&actor, &actorType, &resourceType, &summary, &payload); err != nil {
		t.Fatalf("query stale rotation audit: %v", err)
	}
	if actor == nil || *actor != actorUserID || actorType != "platform_user" || resourceType != "node" {
		t.Fatalf("audit actor/resource = actor:%v actor_type:%q resource:%q", actor, actorType, resourceType)
	}
	auditText := summary + "\n" + payload
	for _, marker := range []string{"agent_token", "token_hash", "Authorization", "secret_ref"} {
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

func assertPostgresIntegrationNodeStaleRotationResultSafe(t *testing.T, ctx context.Context, store *Store, jobID string, forbidden ...string) {
	t.Helper()

	var result string
	if err := store.db.QueryRow(ctx, `select coalesce(result_json,'{}'::jsonb)::text from jobs where id=$1`, jobID).Scan(&result); err != nil {
		t.Fatalf("query stale rotation result json: %v", err)
	}
	for _, marker := range []string{"agent_token", "token_hash", "Authorization", "secret_ref"} {
		if strings.Contains(result, marker) {
			t.Fatalf("result json contains forbidden token metadata: %s", result)
		}
	}
	for _, value := range forbidden {
		if value != "" && strings.Contains(result, value) {
			t.Fatalf("result json leaked forbidden secret value")
		}
	}
}

func installNodeStaleRotationAuditFailureTrigger(t *testing.T, ctx context.Context, store *Store) {
	t.Helper()

	_, err := store.db.Exec(ctx, `
create or replace function fail_node_stale_rotation_audit_test() returns trigger
language plpgsql
as $$
begin
  if new.action = 'node.agent_token.clear_stale' then
    raise exception 'forced node.agent_token.clear_stale audit failure';
  end if;
  return new;
end;
$$;
create trigger fail_node_stale_rotation_audit_test
before insert on audit_events
for each row execute function fail_node_stale_rotation_audit_test();
`)
	if err != nil {
		t.Fatalf("install stale rotation audit failure trigger: %v", err)
	}
	t.Cleanup(func() {
		_, _ = store.db.Exec(context.Background(), `drop trigger if exists fail_node_stale_rotation_audit_test on audit_events; drop function if exists fail_node_stale_rotation_audit_test();`)
	})
}
