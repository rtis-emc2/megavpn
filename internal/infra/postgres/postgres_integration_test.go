package postgres

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/platform/id"
)

func TestPostgresIntegrationJobsLocksProvisioningAccessRoutes(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	node, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-node-" + suffix,
		Kind:          "remote",
		Role:          "egress",
		Status:        "online",
		Address:       "10.50.0.10",
		OSFamily:      "linux",
		OSVersion:     "ubuntu-24.04",
		Architecture:  "amd64",
		ExecutionMode: "agent_managed",
		AgentStatus:   "online",
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}

	instance, err := store.CreateInstance(ctx, domain.Instance{
		NodeID:       node.ID,
		ServiceCode:  "wireguard",
		Name:         "wg-" + suffix,
		Slug:         "wg-" + suffix,
		EndpointHost: "198.51.100.10",
		EndpointPort: 51820,
		Spec: map[string]any{
			"config_content": "[Interface]\nAddress = 10.99.0.1/24\nListenPort = 51820\nPrivateKey = integration-test\n",
		},
	})
	if err != nil {
		t.Fatalf("create instance: %v", err)
	}
	if instance.SystemdUnit != "wg-quick@wg-"+suffix {
		t.Fatalf("instance systemd_unit = %q, want wg-quick@wg-%s", instance.SystemdUnit, suffix)
	}

	applyJob, ok, err := store.AgentNextJob(ctx, node.ID)
	if err != nil {
		t.Fatalf("claim instance apply job: %v", err)
	}
	if !ok {
		t.Fatal("expected queued instance apply job")
	}
	if applyJob.Type != "instance.apply" {
		t.Fatalf("claimed job type = %q, want instance.apply", applyJob.Type)
	}
	assertResourceLockCount(t, ctx, store, applyJob.ID, "instance", "apply", 1)

	if err := store.CompleteJob(ctx, applyJob.ID, "succeeded", map[string]any{"active_state": "active"}); err != nil {
		t.Fatalf("complete instance apply job: %v", err)
	}
	assertResourceLockCount(t, ctx, store, applyJob.ID, "instance", "apply", 0)

	runtimeState, err := store.GetInstanceRuntimeState(ctx, instance.ID)
	if err != nil {
		t.Fatalf("get runtime state: %v", err)
	}
	if runtimeState.RuntimeStatus != "active" || runtimeState.HealthStatus != "healthy" || runtimeState.DriftStatus != "in_sync" {
		t.Fatalf("runtime projection = runtime:%s health:%s drift:%s, want active/healthy/in_sync", runtimeState.RuntimeStatus, runtimeState.HealthStatus, runtimeState.DriftStatus)
	}
	observations, err := store.ListInstanceRuntimeObservations(ctx, instance.ID, 10)
	if err != nil {
		t.Fatalf("list runtime observations after apply: %v", err)
	}
	if len(observations) != 1 || observations[0].Source != "job" || observations[0].RuntimeStatus != "active" {
		t.Fatalf("runtime observations after apply = %#v, want one active job observation", observations)
	}

	targets, err := store.ListAgentInstanceRuntimeTargets(ctx, node.ID)
	if err != nil {
		t.Fatalf("list agent runtime targets: %v", err)
	}
	if len(targets) != 1 || targets[0].InstanceID != instance.ID {
		t.Fatalf("runtime targets = %#v, want one target for instance %s", targets, instance.ID)
	}
	if targets[0].ConfigPath != "/etc/wireguard/wg-"+suffix+".conf" {
		t.Fatalf("runtime target config_path = %q, want default wireguard config path", targets[0].ConfigPath)
	}
	reportRevisionID := runtimeState.AppliedRevisionID
	agentStates, err := store.SubmitAgentInstanceRuntimeReports(ctx, node.ID, []domain.AgentInstanceRuntimeReport{{
		InstanceID:         instance.ID,
		ServiceCode:        "wireguard",
		SystemdUnit:        instance.SystemdUnit,
		ConfigPath:         targets[0].ConfigPath,
		ConfigHash:         "sha256:integration-runtime",
		ActiveState:        "active",
		EnabledState:       "enabled",
		ObservedRevisionID: reportRevisionID,
		ListeningPorts:     []map[string]any{{"network": "udp", "state": "UNCONN", "local_address": "0.0.0.0:51820", "port": 51820}},
	}})
	if err != nil {
		t.Fatalf("submit agent runtime report: %v", err)
	}
	if len(agentStates) != 1 {
		t.Fatalf("agent runtime states len = %d, want 1", len(agentStates))
	}
	if agentStates[0].ConfigHash != "sha256:integration-runtime" || agentStates[0].EnabledState != "enabled" || agentStates[0].AgentReportedAt == nil {
		t.Fatalf("agent runtime state = %#v, want hash/enabled/agent_reported_at", agentStates[0])
	}
	if agentStates[0].RuntimeStatus != "active" || agentStates[0].HealthStatus != "healthy" || agentStates[0].DriftStatus != "in_sync" {
		t.Fatalf("agent runtime projection = runtime:%s health:%s drift:%s, want active/healthy/in_sync", agentStates[0].RuntimeStatus, agentStates[0].HealthStatus, agentStates[0].DriftStatus)
	}
	observations, err = store.ListInstanceRuntimeObservations(ctx, instance.ID, 10)
	if err != nil {
		t.Fatalf("list runtime observations after agent report: %v", err)
	}
	if len(observations) < 2 || observations[0].Source != "agent" || observations[0].ConfigHash != "sha256:integration-runtime" {
		t.Fatalf("runtime observations after agent report = %#v, want latest agent observation with config hash", observations)
	}
	if len(observations[0].ListeningPorts) != 1 || observations[0].ListeningPorts[0]["local_address"] != "0.0.0.0:51820" {
		t.Fatalf("agent observation listening ports = %#v, want expected endpoint port", observations[0].ListeningPorts)
	}

	// Complete the route-policy job queued by instance.apply before creating the
	// client, so the next route-policy payload proves it contains access routes.
	if routeJob, ok, err := store.AgentNextJob(ctx, node.ID); err != nil {
		t.Fatalf("claim post-apply route policy job: %v", err)
	} else if ok {
		if routeJob.Type != "node.route_policy.apply" {
			t.Fatalf("claimed post-apply job type = %q, want node.route_policy.apply", routeJob.Type)
		}
		if err := store.CompleteJob(ctx, routeJob.ID, "succeeded", map[string]any{"active_state": "active"}); err != nil {
			t.Fatalf("complete post-apply route policy job: %v", err)
		}
	}

	client, err := store.CreateClient(ctx, domain.Client{
		Username:    "it-client-" + suffix,
		DisplayName: "Integration Client",
		Email:       "it-client-" + suffix + "@example.invalid",
	})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	provisionJob, err := store.ProvisionClient(ctx, client.ID, []string{instance.ID})
	if err != nil {
		t.Fatalf("provision client: %v", err)
	}
	if provisionJob.Type != "client.provision" {
		t.Fatalf("provision job type = %q, want client.provision", provisionJob.Type)
	}

	accesses, err := store.ListServiceAccesses(ctx, client.ID)
	if err != nil {
		t.Fatalf("list service accesses: %v", err)
	}
	if len(accesses) != 1 || accesses[0].Status != "pending" || accesses[0].InstanceID != instance.ID {
		t.Fatalf("service accesses = %#v, want one pending access for instance", accesses)
	}
	routes, err := store.ListClientAccessRoutes(ctx, client.ID)
	if err != nil {
		t.Fatalf("list client access routes: %v", err)
	}
	if len(routes) != 1 || routes[0].ServiceAccessID == nil || routes[0].NodeID == nil || *routes[0].NodeID != node.ID {
		t.Fatalf("client access routes = %#v, want one baseline route for node", routes)
	}

	workerJob, ok, err := store.ClaimJob(ctx, "integration-worker")
	if err != nil {
		t.Fatalf("claim worker job: %v", err)
	}
	if !ok {
		t.Fatal("expected queued client provisioning worker job")
	}
	if workerJob.ID != provisionJob.ID || workerJob.Type != "client.provision" {
		t.Fatalf("worker claimed %s/%s, want %s/client.provision", workerJob.ID, workerJob.Type, provisionJob.ID)
	}
	assertResourceLockCount(t, ctx, store, workerJob.ID, "client", "provision", 1)
	if err := store.CompleteJob(ctx, workerJob.ID, "succeeded", map[string]any{"message": "integration provisioning complete"}); err != nil {
		t.Fatalf("complete client provision job: %v", err)
	}
	assertResourceLockCount(t, ctx, store, workerJob.ID, "client", "provision", 0)

	queuedRouteJob, err := store.CreateNodeRoutePolicyApplyJob(ctx, node.ID)
	if err != nil {
		t.Fatalf("create route policy apply job: %v", err)
	}
	agentRouteJob, ok, err := store.AgentNextJob(ctx, node.ID)
	if err != nil {
		t.Fatalf("claim route policy apply job: %v", err)
	}
	if !ok {
		t.Fatal("expected queued route policy apply job")
	}
	if agentRouteJob.ID != queuedRouteJob.ID {
		t.Fatalf("agent claimed route job %s, want %s", agentRouteJob.ID, queuedRouteJob.ID)
	}
	if routeCount, ok := agentRouteJob.Payload["route_count"].(float64); !ok || int(routeCount) != 1 {
		t.Fatalf("route policy route_count = %#v, want 1", agentRouteJob.Payload["route_count"])
	}
}

func TestPostgresIntegrationRecoverStaleJobLeases(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	node, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-stale-node-" + suffix,
		Kind:          "remote",
		Role:          "ingress",
		Status:        "online",
		Address:       "10.50.1.10",
		OSFamily:      "linux",
		OSVersion:     "ubuntu-24.04",
		Architecture:  "amd64",
		ExecutionMode: "agent_managed",
		AgentStatus:   "online",
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}

	job, err := store.CreateJob(ctx, domain.Job{
		Type:      "node.route_policy.apply",
		ScopeType: "node",
		ScopeID:   &node.ID,
		NodeID:    &node.ID,
		Priority:  20,
		Payload: map[string]any{
			"node_id": node.ID,
		},
	})
	if err != nil {
		t.Fatalf("create route policy job: %v", err)
	}

	claimed, ok, err := store.AgentNextJob(ctx, node.ID)
	if err != nil {
		t.Fatalf("claim route policy job: %v", err)
	}
	if !ok || claimed.ID != job.ID || claimed.Status != "running" || claimed.LockedUntil == nil {
		t.Fatalf("claimed job = %#v, want running claimed job %s", claimed, job.ID)
	}
	assertResourceLockCount(t, ctx, store, claimed.ID, "node", "bootstrap", 1)

	if _, err := store.db.Exec(ctx, `update jobs set locked_until=$2 where id=$1`, claimed.ID, time.Now().UTC().Add(-time.Minute)); err != nil {
		t.Fatalf("expire job lease: %v", err)
	}
	recoveredCount, err := store.RecoverStaleJobLeases(ctx)
	if err != nil {
		t.Fatalf("recover stale job leases: %v", err)
	}
	if recoveredCount != 1 {
		t.Fatalf("recovered job count = %d, want 1", recoveredCount)
	}
	recovered, err := store.GetJob(ctx, claimed.ID)
	if err != nil {
		t.Fatalf("get recovered job: %v", err)
	}
	if recovered.Status != "retrying" || recovered.LockedBy != nil || recovered.LockedUntil != nil {
		t.Fatalf("recovered job state = status:%s locked_by:%v locked_until:%v, want retrying with no lease", recovered.Status, recovered.LockedBy, recovered.LockedUntil)
	}
	assertResourceLockCount(t, ctx, store, claimed.ID, "node", "bootstrap", 0)

	cancelled, err := store.CancelJob(ctx, claimed.ID)
	if err != nil {
		t.Fatalf("cancel recovered job: %v", err)
	}
	if cancelled.Status != "cancelled" || cancelled.FinishedAt == nil {
		t.Fatalf("cancelled job = %#v, want terminal cancelled job", cancelled)
	}
}

func TestPostgresIntegrationAgentVersionProjection(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	node, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-agent-version-" + suffix,
		Kind:          "remote",
		Role:          "ingress",
		Status:        "draft",
		Address:       "10.50.2.10",
		OSFamily:      "linux",
		OSVersion:     "ubuntu-24.04",
		Architecture:  "amd64",
		ExecutionMode: "agent_managed",
		AgentStatus:   "unknown",
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	token, err := store.CreateNodeEnrollmentToken(ctx, node.ID, time.Hour)
	if err != nil {
		t.Fatalf("create enrollment token: %v", err)
	}
	if _, _, err := store.RegisterAgentWithEnrollmentVersion(ctx, node.ID, token.Token, node.Name, node.Address, "0.6.10.5-alpha", "v1"); err != nil {
		t.Fatalf("register agent: %v", err)
	}
	registered, err := store.GetNode(ctx, node.ID)
	if err != nil {
		t.Fatalf("get registered node: %v", err)
	}
	if registered.AgentVersion != "0.6.10.5-alpha" || registered.AgentProtocolVersion != "v1" || registered.AgentRegisteredAt == nil || registered.AgentLastSeenAt == nil {
		t.Fatalf("registered node agent projection = version:%q protocol:%q registered:%v last_seen:%v", registered.AgentVersion, registered.AgentProtocolVersion, registered.AgentRegisteredAt, registered.AgentLastSeenAt)
	}

	if err := store.HeartbeatByNodeIDWithVersion(ctx, node.ID, "0.6.10.6-alpha", "v1"); err != nil {
		t.Fatalf("heartbeat with version: %v", err)
	}
	nodes, err := store.ListNodes(ctx)
	if err != nil {
		t.Fatalf("list nodes: %v", err)
	}
	var projected *domain.Node
	for idx := range nodes {
		if nodes[idx].ID == node.ID {
			projected = &nodes[idx]
			break
		}
	}
	if projected == nil {
		t.Fatal("registered node missing from list nodes")
	}
	if projected.AgentVersion != "0.6.10.6-alpha" {
		t.Fatalf("projected agent version = %q, want heartbeat version", projected.AgentVersion)
	}
}

func setupPostgresIntegrationStore(t *testing.T) (*Store, context.Context) {
	t.Helper()

	dsn := strings.TrimSpace(os.Getenv("MEGAVPN_TEST_DATABASE_DSN"))
	if dsn == "" {
		t.Skip("set MEGAVPN_TEST_DATABASE_DSN to run PostgreSQL integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)

	adminPool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect admin database: %v", err)
	}
	schema := "megavpn_it_" + strings.ReplaceAll(id.New(), "-", "")[:16]
	if _, err := adminPool.Exec(ctx, "create schema "+quotePostgresIdentifier(schema)); err != nil {
		adminPool.Close()
		t.Fatalf("create test schema: %v", err)
	}
	t.Cleanup(func() {
		dropCtx, dropCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer dropCancel()
		_, _ = adminPool.Exec(dropCtx, "drop schema if exists "+quotePostgresIdentifier(schema)+" cascade")
		adminPool.Close()
	})

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse test database dsn: %v", err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	cfg.MaxConns = 4
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connect test database schema: %v", err)
	}
	t.Cleanup(pool.Close)

	applyPostgresIntegrationMigrations(t, ctx, pool)
	return New(pool), ctx
}

func applyPostgresIntegrationMigrations(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()

	if _, err := pool.Exec(ctx, `create table if not exists schema_migrations(version text primary key, applied_at timestamptz not null default now())`); err != nil {
		t.Fatalf("create schema_migrations: %v", err)
	}

	files, err := filepath.Glob(filepath.Join("..", "..", "..", "migrations", "*.up.sql"))
	if err != nil {
		t.Fatalf("list migrations: %v", err)
	}
	sort.Strings(files)
	if len(files) == 0 {
		t.Fatal("no migrations found")
	}
	for _, file := range files {
		version := strings.TrimSuffix(filepath.Base(file), ".up.sql")
		sqlBytes, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read migration %s: %v", version, err)
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin migration %s: %v", version, err)
		}
		if _, err := tx.Exec(ctx, string(sqlBytes)); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("apply migration %s: %v", version, err)
		}
		if _, err := tx.Exec(ctx, `insert into schema_migrations(version, applied_at) values($1, now())`, version); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("record migration %s: %v", version, err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit migration %s: %v", version, err)
		}
	}
}

func assertResourceLockCount(t *testing.T, ctx context.Context, store *Store, jobID, resourceType, lockKind string, want int) {
	t.Helper()

	var got int
	if err := store.db.QueryRow(ctx, `select count(*) from resource_locks where job_id=$1 and resource_type=$2 and lock_kind=$3`, jobID, resourceType, lockKind).Scan(&got); err != nil {
		t.Fatalf("count resource locks: %v", err)
	}
	if got != want {
		t.Fatalf("resource lock count for job=%s resource=%s kind=%s = %d, want %d", jobID, resourceType, lockKind, got, want)
	}
}

func quotePostgresIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}
