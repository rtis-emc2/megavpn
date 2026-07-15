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

type nodeAgentRevokePostgresFixture struct {
	node             domain.Node
	usedEnrollment   domain.NodeEnrollmentToken
	activeEnrollment domain.NodeEnrollmentToken
	expiredTokenID   string
	agentToken       string
	rotateToken      string
	inventoryID      string
	jobID            string
	rotationJobID    string
}

func TestPostgresIntegrationRevokeNodeAgentIdentityAtomic(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)
	actorUserID := createPostgresIntegrationRevokeActor(t, ctx, store)
	fixture := createPostgresIntegrationRevokeFixture(t, ctx, store, "atomic")

	before, err := store.GetNode(ctx, fixture.node.ID)
	if err != nil {
		t.Fatalf("get node before revoke: %v", err)
	}
	jobsBefore := countPostgresIntegrationRevokeRows(t, ctx, store, `select count(*) from jobs where node_id=$1`, fixture.node.ID)
	inventoryBefore := countPostgresIntegrationRevokeRows(t, ctx, store, `select count(*) from node_inventory_snapshots where node_id=$1`, fixture.node.ID)

	result, err := store.RevokeNodeAgentIdentity(ctx, fixture.node.ID, domain.NodeAgentIdentityRevokeInput{
		Confirmation: "  " + fixture.node.Name + "  ",
		Reason:       "  incident response rotation  ",
		ActorUserID:  &actorUserID,
	})
	if err != nil {
		t.Fatalf("revoke agent identity: %v", err)
	}
	if result.Status != "revoked" || result.NodeID != fixture.node.ID || result.AgentStatus != "revoked" || result.AlreadyRevoked {
		t.Fatalf("revoke result = %#v", result)
	}
	if result.RevokedAt.IsZero() {
		t.Fatal("revoked_at must be set")
	}
	if result.RevokedEnrollmentTokens != 1 {
		t.Fatalf("revoked enrollment tokens = %d, want 1", result.RevokedEnrollmentTokens)
	}
	assertPostgresIntegrationRevokedAgentIdentity(t, ctx, store, fixture.node.ID, result.RevokedAt)
	if store.ValidateAgentToken(ctx, fixture.node.ID, fixture.agentToken) {
		t.Fatal("old agent token must not validate after revoke")
	}
	if store.ValidateAgentToken(ctx, fixture.node.ID, fixture.rotateToken) {
		t.Fatal("pending rotation token must not validate after revoke")
	}

	assertEnrollmentTokenStatus(t, ctx, store, fixture.usedEnrollment.ID, "used")
	assertEnrollmentTokenStatus(t, ctx, store, fixture.activeEnrollment.ID, "revoked")
	assertEnrollmentTokenStatus(t, ctx, store, fixture.expiredTokenID, "expired")

	after, err := store.GetNode(ctx, fixture.node.ID)
	if err != nil {
		t.Fatalf("get node after revoke: %v", err)
	}
	if after.AgentStatus != "revoked" {
		t.Fatalf("node agent_status = %q, want revoked", after.AgentStatus)
	}
	if after.Name != before.Name || after.Address != before.Address || after.Role != before.Role || after.Status != before.Status || after.ExecutionMode != before.ExecutionMode || after.LocationLabel != before.LocationLabel {
		t.Fatalf("operator-owned node profile changed: before=%#v after=%#v", before, after)
	}
	if !sameNullableTime(after.LastHeartbeatAt, before.LastHeartbeatAt) {
		t.Fatalf("last heartbeat changed: before=%v after=%v", before.LastHeartbeatAt, after.LastHeartbeatAt)
	}
	assertPostgresCount(t, ctx, store, `select count(*) from node_inventory_snapshots where node_id=$1`, inventoryBefore, fixture.node.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from node_inventory_snapshots where id=$1`, 1, fixture.inventoryID)
	assertPostgresCount(t, ctx, store, `select count(*) from jobs where node_id=$1`, jobsBefore, fixture.node.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from jobs where id in ($1,$2) and status='queued'`, 2, fixture.jobID, fixture.rotationJobID)
	assertPostgresCount(t, ctx, store, `select count(*) from node_agents where node_id=$1`, 1, fixture.node.ID)
	assertPostgresIntegrationRevokeAudit(t, ctx, store, fixture.node.ID, actorUserID, 1, fixture.agentToken, fixture.rotateToken, fixture.activeEnrollment.Token)

	_, _, err = store.RegisterAgentWithEnrollmentVersion(ctx, fixture.node.ID, fixture.activeEnrollment.Token, fixture.node.Name, fixture.node.Address, "agent-after-revoke", "v1")
	if !errors.Is(err, domain.ErrAgentRegistrationInvalidCredential) {
		t.Fatalf("registration with revoked enrollment token error = %v, want invalid credential", err)
	}
	recoveryEnrollment, err := store.CreateNodeEnrollmentToken(ctx, fixture.node.ID, time.Hour)
	if err != nil {
		t.Fatalf("create explicit recovery enrollment token: %v", err)
	}
	if recoveryEnrollment.Token == "" || recoveryEnrollment.Status != "active" {
		t.Fatalf("recovery enrollment token = %#v", recoveryEnrollment)
	}
	assertPostgresCount(t, ctx, store, `select count(*) from node_agents where node_id=$1`, 1, fixture.node.ID)
}

func TestPostgresIntegrationRevokeNodeAgentIdentityAuditRollback(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)
	actorUserID := createPostgresIntegrationRevokeActor(t, ctx, store)
	fixture := createPostgresIntegrationRevokeFixture(t, ctx, store, "rollback")
	before, err := store.GetNode(ctx, fixture.node.ID)
	if err != nil {
		t.Fatalf("get node before revoke: %v", err)
	}
	installNodeAgentRevokeAuditFailureTrigger(t, ctx, store)

	_, err = store.RevokeNodeAgentIdentity(ctx, fixture.node.ID, domain.NodeAgentIdentityRevokeInput{
		Confirmation: fixture.node.Name,
		Reason:       "incident response rotation",
		ActorUserID:  &actorUserID,
	})
	if !errors.Is(err, domain.ErrNodeAgentRevokeAuditFailed) {
		t.Fatalf("revoke error = %v, want ErrNodeAgentRevokeAuditFailed", err)
	}
	if !store.ValidateAgentToken(ctx, fixture.node.ID, fixture.agentToken) {
		t.Fatal("old agent token should still validate after rollback")
	}
	if !store.ValidateAgentToken(ctx, fixture.node.ID, fixture.rotateToken) {
		t.Fatal("pending rotation token should still validate after rollback")
	}
	assertEnrollmentTokenStatus(t, ctx, store, fixture.activeEnrollment.ID, "active")
	assertPostgresCount(t, ctx, store, `select count(*) from node_agents where node_id=$1 and status='active' and revoked_at is null and agent_token_hash is not null`, 1, fixture.node.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from audit_events where resource_id=$1 and action='node.agent_identity.revoke'`, 0, fixture.node.ID)

	after, err := store.GetNode(ctx, fixture.node.ID)
	if err != nil {
		t.Fatalf("get node after rollback: %v", err)
	}
	if after.AgentStatus != before.AgentStatus || after.Status != before.Status || !sameNullableTime(after.LastHeartbeatAt, before.LastHeartbeatAt) {
		t.Fatalf("node projection changed despite rollback: before=%#v after=%#v", before, after)
	}
}

func TestPostgresIntegrationRevokeNodeAgentIdentityIdempotentAndConcurrent(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)
	actorUserID := createPostgresIntegrationRevokeActor(t, ctx, store)
	fixture := createPostgresIntegrationRevokeFixture(t, ctx, store, "concurrent")

	const attempts = 6
	var wg sync.WaitGroup
	results := make(chan domain.NodeAgentIdentityRevokeResult, attempts)
	errs := make(chan error, attempts)
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := store.RevokeNodeAgentIdentity(context.Background(), fixture.node.ID, domain.NodeAgentIdentityRevokeInput{
				Confirmation: fixture.node.Name,
				Reason:       "incident response rotation",
				ActorUserID:  &actorUserID,
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

	failures := 0
	for err := range errs {
		if err != nil {
			failures++
			t.Errorf("concurrent revoke failed: %v", err)
		}
	}
	if failures != 0 {
		t.Fatalf("concurrent revoke failures = %d", failures)
	}
	firstTransitions := 0
	idempotentResults := 0
	var revokedAt time.Time
	for result := range results {
		if result.Status != "revoked" || result.NodeID != fixture.node.ID || result.AgentStatus != "revoked" {
			t.Fatalf("concurrent revoke result = %#v", result)
		}
		if revokedAt.IsZero() {
			revokedAt = result.RevokedAt
		}
		if !result.RevokedAt.Equal(revokedAt) {
			t.Fatalf("revoked_at changed across idempotent results: first=%v got=%v", revokedAt, result.RevokedAt)
		}
		if result.AlreadyRevoked {
			idempotentResults++
		} else {
			firstTransitions++
		}
	}
	if firstTransitions != 1 || idempotentResults != attempts-1 {
		t.Fatalf("transition/idempotent counts = %d/%d, want 1/%d", firstTransitions, idempotentResults, attempts-1)
	}
	assertPostgresIntegrationRevokedAgentIdentity(t, ctx, store, fixture.node.ID, revokedAt)
	assertPostgresIntegrationRevokeAudit(t, ctx, store, fixture.node.ID, actorUserID, 1, fixture.agentToken, fixture.rotateToken, fixture.activeEnrollment.Token)
	assertPostgresCount(t, ctx, store, `select count(*) from node_agents where node_id=$1`, 1, fixture.node.ID)

	repeated, err := store.RevokeNodeAgentIdentity(ctx, fixture.node.ID, domain.NodeAgentIdentityRevokeInput{
		Confirmation: fixture.node.Name,
		Reason:       "incident response rotation",
		ActorUserID:  &actorUserID,
	})
	if err != nil {
		t.Fatalf("repeated revoke: %v", err)
	}
	if !repeated.AlreadyRevoked {
		t.Fatalf("repeated revoke already_revoked = false: %#v", repeated)
	}
	if !repeated.RevokedAt.Equal(revokedAt) {
		t.Fatalf("repeated revoked_at = %v, want %v", repeated.RevokedAt, revokedAt)
	}
	assertPostgresIntegrationRevokeAudit(t, ctx, store, fixture.node.ID, actorUserID, 1, fixture.agentToken, fixture.rotateToken, fixture.activeEnrollment.Token)
	assertPostgresCount(t, ctx, store, `select count(*) from node_agents where node_id=$1`, 1, fixture.node.ID)
}

func createPostgresIntegrationRevokeFixture(t *testing.T, ctx context.Context, store *Store, label string) nodeAgentRevokePostgresFixture {
	t.Helper()

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	node, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-agent-revoke-" + label + "-" + suffix,
		Kind:          "remote",
		Role:          "egress",
		Status:        "maintenance",
		Address:       "10.70.0.10",
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
	registered, agentToken, err := store.RegisterAgentWithEnrollmentVersion(ctx, node.ID, enrollment.Token, "agent-name-"+suffix, "192.0.2.70", "agent-build", "v1")
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
	expiredTokenID := id.New()
	if _, err := store.db.Exec(ctx, `insert into node_enrollment_tokens(id,node_id,token_hash,token_hint,status,expires_at,created_at) values($1,$2,$3,$4,'expired',$5,$6)`,
		expiredTokenID, node.ID, hashToken(randomToken(32)), "expired-test", time.Now().UTC().Add(-time.Hour), time.Now().UTC().Add(-2*time.Hour)); err != nil {
		t.Fatalf("insert expired enrollment token: %v", err)
	}
	snap, _, err := store.SubmitNodeInventory(ctx, node.ID, map[string]any{"collector": "revoke-test"})
	if err != nil {
		t.Fatalf("submit inventory fixture: %v", err)
	}
	job, err := store.CreateNodeInventoryJob(ctx, node.ID)
	if err != nil {
		t.Fatalf("create inventory job fixture: %v", err)
	}
	rotationJob, err := store.CreateNodeAgentTokenRotateJob(ctx, node.ID)
	if err != nil {
		t.Fatalf("create token rotation job fixture: %v", err)
	}
	rotateToken := resolvePostgresIntegrationRotateToken(t, ctx, store, rotationJob)
	return nodeAgentRevokePostgresFixture{
		node:             node,
		usedEnrollment:   enrollment,
		activeEnrollment: activeEnrollment,
		expiredTokenID:   expiredTokenID,
		agentToken:       agentToken,
		rotateToken:      rotateToken,
		inventoryID:      snap.ID,
		jobID:            job.ID,
		rotationJobID:    rotationJob.ID,
	}
}

func createPostgresIntegrationRevokeActor(t *testing.T, ctx context.Context, store *Store) string {
	t.Helper()

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	user, _, err := store.EnsureBootstrapPlatformUser(ctx, "it-revoke-admin-"+suffix, "it-revoke-admin-"+suffix+"@example.invalid", "Revoke Admin", "integration-password-hash")
	if err != nil {
		t.Fatalf("create revoke actor: %v", err)
	}
	return user.ID
}

func resolvePostgresIntegrationRotateToken(t *testing.T, ctx context.Context, store *Store, job domain.Job) string {
	t.Helper()

	secretID, _ := job.Payload["new_agent_token_secret_ref_id"].(string)
	if strings.TrimSpace(secretID) == "" {
		t.Fatalf("rotation job missing secret ref")
	}
	_, value, err := store.ResolveSecretValue(ctx, secretID)
	if err != nil {
		t.Fatalf("resolve rotation token secret: %v", err)
	}
	token := string(value)
	if token == "" {
		t.Fatal("rotation token secret is empty")
	}
	return token
}

func assertPostgresIntegrationRevokedAgentIdentity(t *testing.T, ctx context.Context, store *Store, nodeID string, wantRevokedAt time.Time) {
	t.Helper()

	var status string
	var revokedAt time.Time
	var tokenHash string
	if err := store.db.QueryRow(ctx, `select status,revoked_at,coalesce(agent_token_hash,'') from node_agents where node_id=$1`, nodeID).Scan(&status, &revokedAt, &tokenHash); err != nil {
		t.Fatalf("query node agent identity: %v", err)
	}
	if status != "revoked" {
		t.Fatalf("node agent status = %q, want revoked", status)
	}
	if revokedAt.IsZero() || !revokedAt.Equal(wantRevokedAt) {
		t.Fatalf("node agent revoked_at = %v, want %v", revokedAt, wantRevokedAt)
	}
	if tokenHash != "" {
		t.Fatal("revoked identity must not retain current agent token hash")
	}
}

func assertPostgresIntegrationRevokeAudit(t *testing.T, ctx context.Context, store *Store, nodeID, actorUserID string, want int, forbidden ...string) {
	t.Helper()

	var count int
	if err := store.db.QueryRow(ctx, `select count(*) from audit_events where resource_id=$1 and action='node.agent_identity.revoke'`, nodeID).Scan(&count); err != nil {
		t.Fatalf("count revoke audits: %v", err)
	}
	if count != want {
		t.Fatalf("revoke audit count = %d, want %d", count, want)
	}
	if want == 0 {
		return
	}
	var actor *string
	var actorType string
	var resourceType string
	var summary string
	var payload string
	if err := store.db.QueryRow(ctx, `select actor_user_id,actor_type,resource_type,summary,payload_json::text from audit_events where resource_id=$1 and action='node.agent_identity.revoke' order by created_at desc limit 1`, nodeID).Scan(&actor, &actorType, &resourceType, &summary, &payload); err != nil {
		t.Fatalf("query revoke audit: %v", err)
	}
	if actor == nil || *actor != actorUserID || actorType != "platform_user" || resourceType != "node" {
		t.Fatalf("audit actor/resource = actor:%v actor_type:%q resource:%q", actor, actorType, resourceType)
	}
	auditText := summary + "\n" + payload
	if strings.Contains(auditText, "agent_token") || strings.Contains(auditText, "token_hash") || strings.Contains(auditText, "Authorization") {
		t.Fatalf("audit contains forbidden token metadata: %s", auditText)
	}
	for _, value := range forbidden {
		if value != "" && strings.Contains(auditText, value) {
			t.Fatalf("audit leaked forbidden secret value")
		}
	}
}

func installNodeAgentRevokeAuditFailureTrigger(t *testing.T, ctx context.Context, store *Store) {
	t.Helper()

	_, err := store.db.Exec(ctx, `
create or replace function fail_node_agent_revoke_audit_test() returns trigger
language plpgsql
as $$
begin
  if new.action = 'node.agent_identity.revoke' then
    raise exception 'forced node.agent_identity.revoke audit failure';
  end if;
  return new;
end;
$$;
create trigger fail_node_agent_revoke_audit_test
before insert on audit_events
for each row execute function fail_node_agent_revoke_audit_test();
`)
	if err != nil {
		t.Fatalf("install revoke audit failure trigger: %v", err)
	}
	t.Cleanup(func() {
		_, _ = store.db.Exec(context.Background(), `drop trigger if exists fail_node_agent_revoke_audit_test on audit_events; drop function if exists fail_node_agent_revoke_audit_test();`)
	})
}

func countPostgresIntegrationRevokeRows(t *testing.T, ctx context.Context, store *Store, query string, args ...any) int {
	t.Helper()

	var got int
	if err := store.db.QueryRow(ctx, query, args...).Scan(&got); err != nil {
		t.Fatalf("count query %q: %v", query, err)
	}
	return got
}

func sameNullableTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}
