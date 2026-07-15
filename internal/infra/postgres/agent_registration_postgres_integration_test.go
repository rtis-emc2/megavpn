package postgres

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/platform/id"
)

func TestPostgresIntegrationRegisterAgentWithEnrollmentAtomic(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)

	node, err := store.CreateNode(ctx, domain.Node{
		Name:            "it-agent-register-" + id.New()[:8],
		Kind:            "local",
		Role:            "ingress",
		Status:          "maintenance",
		Address:         "10.60.0.10",
		LocationLabel:   "operator-owned-rack",
		OSFamily:        "linux",
		OSVersion:       "ubuntu-24.04",
		Architecture:    "arm64",
		ExecutionMode:   "manual_bundle",
		AgentStatus:     "unknown",
		GeoIPProvider:   "manual",
		GeoIPStatus:     "disabled",
		GeoIPIP:         "198.51.100.20",
		GeoIPRegion:     "operator-region",
		GeoIPCity:       "operator-city",
		GeoIPOrg:        "operator-org",
		GeoIPASN:        "64512",
		GeoIPError:      "operator-note",
		LastHeartbeatAt: nil,
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	enrollment, err := store.CreateNodeEnrollmentToken(ctx, node.ID, time.Hour)
	if err != nil {
		t.Fatalf("create enrollment token: %v", err)
	}
	registered, agentToken, err := store.RegisterAgentWithEnrollmentVersion(ctx, node.ID, enrollment.Token, "agent-renamed", "192.0.2.200", "agent-build-1", "v1")
	if err != nil {
		t.Fatalf("register agent: %v", err)
	}
	if agentToken == "" || agentToken == enrollment.Token {
		t.Fatalf("agent token should be a new one-time token distinct from enrollment token")
	}
	if registered.ID != node.ID || registered.Name != node.Name || registered.Address != node.Address {
		t.Fatalf("registered node projection = %#v, want persisted operator name/address", registered)
	}
	if registered.Status != "maintenance" {
		t.Fatalf("registered status = %q, want maintenance preserved", registered.Status)
	}
	if registered.Kind != "remote" || registered.ExecutionMode != "agent_managed" || registered.AgentStatus != "online" {
		t.Fatalf("registered lifecycle = kind:%s execution:%s agent:%s, want remote/agent_managed/online", registered.Kind, registered.ExecutionMode, registered.AgentStatus)
	}
	if registered.Role != node.Role || registered.LocationLabel != node.LocationLabel || registered.OSVersion != node.OSVersion || registered.Architecture != node.Architecture {
		t.Fatalf("operator-owned profile changed: before=%#v after=%#v", node, registered)
	}
	if registered.AgentVersion != "agent-build-1" || registered.AgentProtocolVersion != "v1" || registered.AgentRegisteredAt == nil || registered.AgentLastSeenAt == nil {
		t.Fatalf("agent projection = version:%q protocol:%q registered:%v seen:%v", registered.AgentVersion, registered.AgentProtocolVersion, registered.AgentRegisteredAt, registered.AgentLastSeenAt)
	}
	assertEnrollmentTokenStatus(t, ctx, store, enrollment.ID, "used")
	assertActiveEnrollmentTokenCount(t, ctx, store, node.ID, 0)
	agentHash := assertSingleNodeAgent(t, ctx, store, node.ID, agentToken)
	assertAuditCount(t, ctx, store, node.ID, "agent.register", 1)
	if !store.ValidateAgentToken(ctx, node.ID, agentToken) {
		t.Fatal("new agent token should validate against stored hash")
	}
	if store.ValidateAgentToken(ctx, node.ID, enrollment.Token) {
		t.Fatal("enrollment token must not validate as an agent token")
	}

	nextEnrollment, err := store.CreateNodeEnrollmentToken(ctx, node.ID, time.Hour)
	if err != nil {
		t.Fatalf("create second enrollment token: %v", err)
	}
	_, nextAgentToken, err := store.RegisterAgentWithEnrollmentVersion(ctx, node.ID, nextEnrollment.Token, "second-agent-rename", "192.0.2.201", "agent-build-2", "v1")
	if err != nil {
		t.Fatalf("re-register agent: %v", err)
	}
	nextHash := assertSingleNodeAgent(t, ctx, store, node.ID, nextAgentToken)
	if nextHash == agentHash {
		t.Fatalf("re-enrollment did not replace agent token hash")
	}
	if store.ValidateAgentToken(ctx, node.ID, agentToken) {
		t.Fatal("old agent token should no longer validate after re-enrollment")
	}
	if !store.ValidateAgentToken(ctx, node.ID, nextAgentToken) {
		t.Fatal("new re-enrollment agent token should validate")
	}
	assertAuditCount(t, ctx, store, node.ID, "agent.register", 2)
}

func TestPostgresIntegrationRegisterAgentWithEnrollmentRollbackOnAuditFailure(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)

	node, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-agent-audit-rollback-" + id.New()[:8],
		Kind:          "remote",
		Role:          "egress",
		Status:        "draft",
		Address:       "10.60.0.20",
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
	installAgentRegisterAuditFailureTrigger(t, ctx, store)

	_, _, err = store.RegisterAgentWithEnrollmentVersion(ctx, node.ID, enrollment.Token, node.Name, node.Address, "agent-build", "v1")
	if !errors.Is(err, domain.ErrAgentRegistrationAuditFailed) {
		t.Fatalf("register error = %v, want ErrAgentRegistrationAuditFailed", err)
	}
	assertEnrollmentTokenStatus(t, ctx, store, enrollment.ID, "active")
	assertActiveEnrollmentTokenCount(t, ctx, store, node.ID, 1)
	assertNodeAgentCount(t, ctx, store, node.ID, 0)
	assertAuditCount(t, ctx, store, node.ID, "agent.register", 0)
	after, err := store.GetNode(ctx, node.ID)
	if err != nil {
		t.Fatalf("get node after rollback: %v", err)
	}
	if after.Status != "draft" || after.ExecutionMode != "manual_bundle" || after.AgentStatus != "unknown" || after.LastHeartbeatAt != nil {
		t.Fatalf("node changed despite rollback: %#v", after)
	}
}

func TestPostgresIntegrationRegisterAgentWithEnrollmentRejectsRetiredBeforeTokenUse(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)

	node, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-agent-retired-" + id.New()[:8],
		Kind:          "remote",
		Role:          "egress",
		Status:        "draft",
		Address:       "10.60.0.30",
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
	if _, err := store.RetireNode(ctx, node.ID); err != nil {
		t.Fatalf("retire node: %v", err)
	}
	_, _, err = store.RegisterAgentWithEnrollmentVersion(ctx, node.ID, enrollment.Token, node.Name, node.Address, "agent-build", "v1")
	if !errors.Is(err, domain.ErrAgentRegistrationNodeRetired) {
		t.Fatalf("register retired error = %v, want ErrAgentRegistrationNodeRetired", err)
	}
	assertEnrollmentTokenStatus(t, ctx, store, enrollment.ID, "active")
	assertAuditCount(t, ctx, store, node.ID, "agent.register", 0)
	after, err := store.GetNode(ctx, node.ID)
	if err != nil {
		t.Fatalf("get retired node: %v", err)
	}
	if after.Status != "retired" {
		t.Fatalf("retired node status = %q, want retired", after.Status)
	}
}

func TestPostgresIntegrationRegisterAgentWithEnrollmentConcurrentTokenUse(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)

	node, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-agent-concurrent-" + id.New()[:8],
		Kind:          "remote",
		Role:          "egress",
		Status:        "draft",
		Address:       "10.60.0.40",
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
	const attempts = 6
	var wg sync.WaitGroup
	errs := make(chan error, attempts)
	tokens := make(chan string, attempts)
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, token, err := store.RegisterAgentWithEnrollmentVersion(context.Background(), node.ID, enrollment.Token, node.Name, node.Address, "agent-build", "v1")
			if err == nil {
				tokens <- token
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	close(tokens)
	successes := 0
	for err := range errs {
		if err == nil {
			successes++
			continue
		}
		if !errors.Is(err, domain.ErrAgentRegistrationInvalidCredential) {
			t.Fatalf("unexpected concurrent registration error: %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent successes = %d, want 1", successes)
	}
	if len(tokens) != 1 {
		t.Fatalf("successful token count = %d, want 1", len(tokens))
	}
	assertEnrollmentTokenStatus(t, ctx, store, enrollment.ID, "used")
	assertNodeAgentCount(t, ctx, store, node.ID, 1)
	assertAuditCount(t, ctx, store, node.ID, "agent.register", 1)
}

func installAgentRegisterAuditFailureTrigger(t *testing.T, ctx context.Context, store *Store) {
	t.Helper()
	_, err := store.db.Exec(ctx, `
create or replace function fail_agent_register_audit_test() returns trigger
language plpgsql
as $$
begin
  if new.action = 'agent.register' then
    raise exception 'forced agent.register audit failure';
  end if;
  return new;
end;
$$;
create trigger fail_agent_register_audit_test
before insert on audit_events
for each row execute function fail_agent_register_audit_test();
`)
	if err != nil {
		t.Fatalf("install audit failure trigger: %v", err)
	}
	t.Cleanup(func() {
		_, _ = store.db.Exec(context.Background(), `drop trigger if exists fail_agent_register_audit_test on audit_events; drop function if exists fail_agent_register_audit_test();`)
	})
}

func assertEnrollmentTokenStatus(t *testing.T, ctx context.Context, store *Store, tokenID, want string) {
	t.Helper()
	var got string
	var used bool
	if err := store.db.QueryRow(ctx, `select status, used_at is not null from node_enrollment_tokens where id=$1`, tokenID).Scan(&got, &used); err != nil {
		t.Fatalf("query enrollment token status: %v", err)
	}
	if got != want {
		t.Fatalf("enrollment token status = %q, want %q", got, want)
	}
	if want == "used" && !used {
		t.Fatalf("enrollment token %s status used but used_at is null", tokenID)
	}
	if want != "used" && used {
		t.Fatalf("enrollment token %s status %s but used_at is set", tokenID, got)
	}
}

func assertActiveEnrollmentTokenCount(t *testing.T, ctx context.Context, store *Store, nodeID string, want int) {
	t.Helper()
	var got int
	if err := store.db.QueryRow(ctx, `select count(*) from node_enrollment_tokens where node_id=$1 and status='active'`, nodeID).Scan(&got); err != nil {
		t.Fatalf("count active enrollment tokens: %v", err)
	}
	if got != want {
		t.Fatalf("active enrollment tokens = %d, want %d", got, want)
	}
}

func assertSingleNodeAgent(t *testing.T, ctx context.Context, store *Store, nodeID, plaintextToken string) string {
	t.Helper()
	var count int
	var hash, hint, fingerprint string
	var revokedAt *time.Time
	if err := store.db.QueryRow(ctx, `select count(*), max(coalesce(agent_token_hash,'')), max(coalesce(token_hint,'')), max(coalesce(fingerprint,'')), max(revoked_at) from node_agents where node_id=$1`, nodeID).Scan(&count, &hash, &hint, &fingerprint, &revokedAt); err != nil {
		t.Fatalf("query node agent: %v", err)
	}
	if count != 1 {
		t.Fatalf("node_agents count = %d, want 1", count)
	}
	if hash == "" || hash == plaintextToken {
		t.Fatalf("stored agent token hash is empty or plaintext")
	}
	if hash != hashToken(plaintextToken) {
		t.Fatalf("stored agent token hash does not match generated token hash")
	}
	if hint == "" || hint == plaintextToken {
		t.Fatalf("stored token hint = %q, want non-plaintext safe hint", hint)
	}
	if fingerprint != "agent:"+nodeID {
		t.Fatalf("fingerprint = %q, want stable non-secret fingerprint", fingerprint)
	}
	if revokedAt != nil {
		t.Fatalf("revoked_at = %v, want null active identity", revokedAt)
	}
	return hash
}

func assertNodeAgentCount(t *testing.T, ctx context.Context, store *Store, nodeID string, want int) {
	t.Helper()
	var got int
	if err := store.db.QueryRow(ctx, `select count(*) from node_agents where node_id=$1`, nodeID).Scan(&got); err != nil {
		t.Fatalf("count node_agents: %v", err)
	}
	if got != want {
		t.Fatalf("node_agents count = %d, want %d", got, want)
	}
}

func assertAuditCount(t *testing.T, ctx context.Context, store *Store, nodeID, action string, want int) {
	t.Helper()
	var got int
	if err := store.db.QueryRow(ctx, `select count(*) from audit_events where resource_id=$1 and action=$2`, nodeID, action).Scan(&got); err != nil {
		t.Fatalf("count audit events: %v", err)
	}
	if got != want {
		t.Fatalf("%s audit count = %d, want %d", action, got, want)
	}
	var leaked int
	if err := store.db.QueryRow(ctx, `select count(*) from audit_events where resource_id=$1 and action=$2 and payload_json <> '{}'::jsonb`, nodeID, action).Scan(&leaked); err != nil {
		t.Fatalf("count audit payload leaks: %v", err)
	}
	if leaked != 0 {
		t.Fatalf("%s audit payload rows = %d, want empty payload", action, leaked)
	}
}
