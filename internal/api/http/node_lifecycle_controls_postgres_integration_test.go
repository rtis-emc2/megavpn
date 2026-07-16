package http

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	nethttp "net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rtis-emc2/megavpn/internal/agentauth"
	"github.com/rtis-emc2/megavpn/internal/domain"
	pgstore "github.com/rtis-emc2/megavpn/internal/infra/postgres"
	"github.com/rtis-emc2/megavpn/internal/platform/id"
)

const (
	lifecycleRebootReason  = "Disposable lifecycle protocol verification"
	lifecycleCleanupReason = "Disposable managed-runtime cleanup protocol verification"
)

type nodeLifecycleHTTPFixture struct {
	store          *pgstore.Store
	pool           *pgxpool.Pool
	ctx            context.Context
	handler        nethttp.Handler
	logs           *bytes.Buffer
	adminUserID    string
	adminToken     string
	node           domain.Node
	enrollment     domain.NodeEnrollmentToken
	registration   agentOnboardingRegisterResult
	heartbeatAt    *time.Time
	inventoryCount int
	agentTokenHash string
	markers        map[string]string
}

type nodeLifecycleHTTPSequenceEvidence struct {
	rebootJobID          string
	cleanupJobID         string
	instanceID           string
	rebootOperatorBody   []byte
	rebootPoll           signedAgentHTTPResult
	rebootResult         signedAgentHTTPResult
	cleanupOperatorBody  []byte
	cleanupPoll          signedAgentHTTPResult
	cleanupResult        signedAgentHTTPResult
	cleanupReplayBody    []byte
	cleanupResultPayload map[string]any
}

type lifecycleForbiddenValue struct {
	category string
	value    string
}

func TestPostgresIntegrationNodeLifecycleRebootEmergencyCleanupHTTP(t *testing.T) {
	fixture := newNodeLifecycleHTTPFixture(t, "sequence")
	evidence := executeNodeLifecycleHTTPSequence(t, fixture)

	if evidence.rebootJobID == "" || evidence.cleanupJobID == "" || evidence.instanceID == "" {
		t.Fatal("lifecycle sequence did not retain safe resource identifiers")
	}
	assertHTTPPostgresCount(t, fixture.ctx, fixture.pool, `select count(*) from jobs where id in ($1,$2) and status='succeeded'`, 2, evidence.rebootJobID, evidence.cleanupJobID)
	assertHTTPPostgresCount(t, fixture.ctx, fixture.pool, `select count(*) from audit_events where resource_id=$1 and action in ('node.reboot.queue','node.emergency_cleanup.queue')`, 2, fixture.node.ID)
	assertHTTPPostgresCount(t, fixture.ctx, fixture.pool, `select count(*) from node_inventory_snapshots where node_id=$1`, fixture.inventoryCount, fixture.node.ID)
}

func TestPostgresIntegrationNodeLifecycleRebootEmergencyCleanupConflictHTTP(t *testing.T) {
	fixture := newNodeLifecycleHTTPFixture(t, "conflict")

	var maintenance domain.Node
	maintenanceRec := sendOperatorJSON(t, fixture.handler, nethttp.MethodPost, "/api/v1/nodes/"+fixture.node.ID+"/maintenance/enable", fixture.adminToken, true, nil, &maintenance)
	if maintenanceRec.Code != nethttp.StatusOK || maintenance.Status != "maintenance" {
		t.Fatalf("maintenance enable status/state = %d/%q, want 200/maintenance", maintenanceRec.Code, maintenance.Status)
	}

	var rebootJob domain.Job
	rebootRec := sendOperatorJSON(t, fixture.handler, nethttp.MethodPost, "/api/v1/nodes/"+fixture.node.ID+"/reboot", fixture.adminToken, true, map[string]any{
		"confirmation": fixture.node.Name,
		"reason":       lifecycleRebootReason,
	}, &rebootJob)
	if rebootRec.Code != nethttp.StatusAccepted {
		t.Fatalf("reboot queue status = %d, want 202; body=%s", rebootRec.Code, rebootRec.Body.String())
	}
	assertOperatorLifecycleJobMetadata(t, "reboot conflict fixture", rebootRec.Body.Bytes(), rebootJob, fixture.node.ID, "node.reboot")

	var rebootPayloadBefore string
	if err := fixture.pool.QueryRow(fixture.ctx, `select payload_json::text from jobs where id=$1 and status='queued'`, rebootJob.ID).Scan(&rebootPayloadBefore); err != nil {
		t.Fatalf("query queued reboot payload before conflict: %v", err)
	}

	cleanupRec := sendOperatorJSON(t, fixture.handler, nethttp.MethodPost, "/api/v1/nodes/"+fixture.node.ID+"/emergency-cleanup", fixture.adminToken, true, lifecycleCleanupRequest(fixture.node.Name), nil)
	if cleanupRec.Code != nethttp.StatusConflict {
		t.Fatalf("cleanup conflict status = %d, want 409; body=%s", cleanupRec.Code, cleanupRec.Body.String())
	}
	assertLifecycleErrorCode(t, cleanupRec, nodeEmergencyCleanupConflict)
	assertLifecycleBodyExcludes(t, "cleanup conflict response", cleanupRec.Body.Bytes(), []lifecycleForbiddenValue{
		{category: "conflicting job identifier", value: rebootJob.ID},
		{category: "agent credential", value: fixture.registration.AgentToken},
		{category: "enrollment credential", value: fixture.enrollment.Token},
		{category: "resource-lock metadata", value: "locked_by"},
		{category: "job payload metadata", value: "payload"},
	})

	assertHTTPPostgresCount(t, fixture.ctx, fixture.pool, `select count(*) from jobs where node_id=$1 and type='node.emergency_cleanup'`, 0, fixture.node.ID)
	assertHTTPPostgresCount(t, fixture.ctx, fixture.pool, `select count(*) from audit_events where resource_id=$1 and action='node.emergency_cleanup.queue'`, 0, fixture.node.ID)
	assertHTTPPostgresCount(t, fixture.ctx, fixture.pool, `select count(*) from resource_locks where resource_type='node' and resource_id=$1 and lock_kind='bootstrap'`, 1, fixture.node.ID)
	assertHTTPPostgresCount(t, fixture.ctx, fixture.pool, `select count(*) from resource_locks where job_id=$1 and resource_type='node' and resource_id=$2 and lock_kind='bootstrap'`, 1, rebootJob.ID, fixture.node.ID)

	var rebootStatus string
	var rebootPayloadAfter string
	if err := fixture.pool.QueryRow(fixture.ctx, `select status,payload_json::text from jobs where id=$1`, rebootJob.ID).Scan(&rebootStatus, &rebootPayloadAfter); err != nil {
		t.Fatalf("query reboot after cleanup conflict: %v", err)
	}
	if rebootStatus != "queued" || rebootPayloadAfter != rebootPayloadBefore {
		t.Fatalf("active reboot changed after cleanup conflict: status=%q payload_same=%t", rebootStatus, rebootPayloadAfter == rebootPayloadBefore)
	}
	assertLifecycleQueueAudit(t, fixture, "node.reboot.queue", rebootJob.ID, lifecycleRebootReason, 1)
}

func TestPostgresIntegrationNodeLifecycleRebootEmergencyCleanupRedactionHTTP(t *testing.T) {
	fixture := newNodeLifecycleHTTPFixture(t, "redaction")
	evidence := executeNodeLifecycleHTTPSequence(t, fixture)

	forbidden := []lifecycleForbiddenValue{
		{category: "enrollment credential", value: fixture.enrollment.Token},
		{category: "agent credential", value: fixture.registration.AgentToken},
		{category: "agent credential hash", value: fixture.agentTokenHash},
		{category: "authorization header", value: "Bearer " + fixture.registration.AgentToken},
		{category: "secret reference marker", value: fixture.markers["secret_ref"]},
		{category: "private-key marker", value: fixture.markers["private_key"]},
		{category: "credential marker", value: fixture.markers["credential"]},
		{category: "configuration marker", value: fixture.markers["configuration"]},
		{category: "raw command-output marker", value: fixture.markers["command_output"]},
	}
	for _, signed := range []signedAgentHTTPResult{evidence.rebootPoll, evidence.rebootResult, evidence.cleanupPoll, evidence.cleanupResult} {
		forbidden = append(forbidden,
			lifecycleForbiddenValue{category: "request signature", value: signed.req.Header.Get(agentauth.HeaderSignature)},
			lifecycleForbiddenValue{category: "request nonce", value: signed.req.Header.Get(agentauth.HeaderNonce)},
		)
	}

	responses := []struct {
		label string
		body  []byte
	}{
		{"reboot operator response", evidence.rebootOperatorBody},
		{"reboot signed-agent poll response", evidence.rebootPoll.rec.Body.Bytes()},
		{"reboot signed-agent result acknowledgement", evidence.rebootResult.rec.Body.Bytes()},
		{"cleanup operator response", evidence.cleanupOperatorBody},
		{"cleanup signed-agent poll response", evidence.cleanupPoll.rec.Body.Bytes()},
		{"cleanup signed-agent result acknowledgement", evidence.cleanupResult.rec.Body.Bytes()},
		{"replayed result rejection", evidence.cleanupReplayBody},
	}
	for _, response := range responses {
		assertLifecycleBodyExcludes(t, response.label, response.body, forbidden)
	}

	queries := []struct {
		label string
		sql   string
	}{
		{"operator audit", `select coalesce(string_agg(row_to_json(x)::text, E'\n'), '') from audit_events x where resource_id=$1`},
		{"lifecycle jobs", `select coalesce(string_agg(row_to_json(x)::text, E'\n'), '') from jobs x where node_id=$1`},
		{"lifecycle job logs", `select coalesce(string_agg(row_to_json(x)::text, E'\n'), '') from job_logs x join jobs j on j.id=x.job_id where j.node_id=$1`},
	}
	for _, query := range queries {
		var stored string
		if err := fixture.pool.QueryRow(fixture.ctx, query.sql, fixture.node.ID).Scan(&stored); err != nil {
			t.Fatalf("query %s redaction evidence: %v", query.label, err)
		}
		assertLifecycleBodyExcludes(t, query.label, []byte(stored), forbidden)
	}
	assertLifecycleBodyExcludes(t, "captured application logs", fixture.logs.Bytes(), forbidden)
}

func newNodeLifecycleHTTPFixture(t *testing.T, label string) nodeLifecycleHTTPFixture {
	t.Helper()

	store, pool, ctx := setupHTTPPostgresIntegrationStore(t)
	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	admin, _, err := store.EnsureBootstrapPlatformUser(ctx, "it-lifecycle-admin-"+suffix, "it-lifecycle-admin-"+suffix+"@example.invalid", "Lifecycle Integration Admin", "integration-password-hash")
	if err != nil {
		t.Fatalf("create lifecycle integration actor: %v", err)
	}
	adminToken := createHTTPPostgresIntegrationSession(t, ctx, store, admin.ID)
	logs := bytes.NewBuffer(nil)
	handler := New(slog.New(slog.NewTextHandler(logs, nil)), store, Options{
		Version:               "test",
		SessionCookieName:     "megavpn_session",
		AgentSignatureEnforce: true,
		AgentSignatureWindow:  time.Minute,
	})

	node := createAgentOnboardingNodeViaHTTP(t, handler, adminToken, "lifecycle-"+label)
	enrollment := createAgentOnboardingEnrollmentTokenHTTP(t, handler, node.ID, adminToken)
	registration := registerAgentOnboardingHTTP(t, handler, node.ID, enrollment.Token, "lifecycle-agent-"+suffix, "192.0.2.96", "it-lifecycle-agent/1")

	heartbeat := sendSignedAgentJSON(t, handler, nethttp.MethodPost, "/agent/heartbeat", registration.AgentToken, map[string]any{
		"node_id":          node.ID,
		"name":             node.Name,
		"agent_version":    "it-lifecycle-agent/1",
		"protocol_version": agentProtocolVersion,
	}, nil)
	if heartbeat.rec.Code != nethttp.StatusOK {
		t.Fatalf("signed lifecycle heartbeat status = %d, want 200", heartbeat.rec.Code)
	}
	verifySignedAgentResponse(t, heartbeat.req, heartbeat.rec, registration.AgentToken)

	inventory := sendSignedAgentJSON(t, handler, nethttp.MethodPost, "/agent/inventory", registration.AgentToken, map[string]any{
		"node_id": node.ID,
		"source":  "inventory",
		"inventory": map[string]any{
			"hostname":     "lifecycle-disposable-node",
			"arch":         "amd64",
			"kernel":       "6.6.0-integration",
			"collected_at": "2026-07-16T00:00:00Z",
			"capabilities": map[string]any{"test_only": true},
		},
	}, nil)
	if inventory.rec.Code != nethttp.StatusOK {
		t.Fatalf("signed lifecycle inventory status = %d, want 200", inventory.rec.Code)
	}
	verifySignedAgentResponse(t, inventory.req, inventory.rec, registration.AgentToken)

	emptyPoll := sendSignedAgentJSON(t, handler, nethttp.MethodGet, "/agent/jobs/next?node_id="+node.ID, registration.AgentToken, nil, nil)
	if emptyPoll.rec.Code != nethttp.StatusNoContent {
		t.Fatalf("signed lifecycle readiness poll status = %d, want 204; body=%s", emptyPoll.rec.Code, emptyPoll.rec.Body.String())
	}
	verifySignedAgentResponse(t, emptyPoll.req, emptyPoll.rec, registration.AgentToken)

	var diagnostics domain.NodeDiagnostics
	diagnosticsRec := sendOperatorJSON(t, handler, nethttp.MethodGet, "/api/v1/nodes/"+node.ID+"/diagnostics", adminToken, false, nil, &diagnostics)
	if diagnosticsRec.Code != nethttp.StatusOK {
		t.Fatalf("lifecycle diagnostics status = %d, want 200", diagnosticsRec.Code)
	}
	if diagnostics.Node.ID != node.ID || diagnostics.HeartbeatState != "online" || diagnostics.Agent.Status != "active" || diagnostics.Agent.LastSeenAt == nil || diagnostics.Agent.LastJobPollAt == nil {
		t.Fatalf("lifecycle diagnostics do not show an active current agent channel: node_match=%t heartbeat=%q agent=%q seen=%t poll=%t",
			diagnostics.Node.ID == node.ID, diagnostics.HeartbeatState, diagnostics.Agent.Status, diagnostics.Agent.LastSeenAt != nil, diagnostics.Agent.LastJobPollAt != nil)
	}

	currentNode, err := store.GetNode(ctx, node.ID)
	if err != nil {
		t.Fatalf("get lifecycle node after signed setup: %v", err)
	}
	heartbeatAt := assertHTTPPostgresNodeHeartbeatSet(t, ctx, pool, node.ID)
	var tokenHash string
	if err := pool.QueryRow(ctx, `select coalesce(agent_token_hash,'') from node_agents where node_id=$1 and status='active' and revoked_at is null`, node.ID).Scan(&tokenHash); err != nil {
		t.Fatalf("query lifecycle agent token hash: %v", err)
	}
	if tokenHash == "" || tokenHash == registration.AgentToken {
		t.Fatal("lifecycle agent identity does not contain a non-plaintext credential hash")
	}

	return nodeLifecycleHTTPFixture{
		store:          store,
		pool:           pool,
		ctx:            ctx,
		handler:        handler,
		logs:           logs,
		adminUserID:    admin.ID,
		adminToken:     adminToken,
		node:           currentNode,
		enrollment:     enrollment,
		registration:   registration,
		heartbeatAt:    heartbeatAt,
		inventoryCount: countHTTPPostgresNodeInventorySnapshots(t, ctx, pool, node.ID),
		agentTokenHash: tokenHash,
		markers: map[string]string{
			"private_key":    "LIFECYCLE_PRIVATE_KEY_SENTINEL_" + suffix,
			"secret_ref":     "LIFECYCLE_SECRET_REF_SENTINEL_" + suffix,
			"credential":     "LIFECYCLE_CREDENTIAL_SENTINEL_" + suffix,
			"configuration":  "LIFECYCLE_CONFIGURATION_SENTINEL_" + suffix,
			"command_output": "LIFECYCLE_COMMAND_OUTPUT_SENTINEL_" + suffix,
		},
	}
}

func executeNodeLifecycleHTTPSequence(t *testing.T, fixture nodeLifecycleHTTPFixture) nodeLifecycleHTTPSequenceEvidence {
	t.Helper()

	beforeNode := fixture.node
	beforeAgentHash := fixture.agentTokenHash

	unauthenticated := sendOperatorJSON(t, fixture.handler, nethttp.MethodPost, "/api/v1/nodes/"+fixture.node.ID+"/reboot", "", true, map[string]any{
		"confirmation": fixture.node.Name,
		"reason":       lifecycleRebootReason,
	}, nil)
	if unauthenticated.Code != nethttp.StatusUnauthorized {
		t.Fatalf("unauthenticated reboot status = %d, want 401", unauthenticated.Code)
	}
	missingCSRF := sendOperatorJSON(t, fixture.handler, nethttp.MethodPost, "/api/v1/nodes/"+fixture.node.ID+"/reboot", fixture.adminToken, false, map[string]any{
		"confirmation": fixture.node.Name,
		"reason":       lifecycleRebootReason,
	}, nil)
	if missingCSRF.Code != nethttp.StatusForbidden {
		t.Fatalf("reboot without CSRF status = %d, want 403", missingCSRF.Code)
	}
	assertHTTPPostgresCount(t, fixture.ctx, fixture.pool, `select count(*) from jobs where node_id=$1 and type='node.reboot'`, 0, fixture.node.ID)

	var rebootJob domain.Job
	rebootRec := sendOperatorJSON(t, fixture.handler, nethttp.MethodPost, "/api/v1/nodes/"+fixture.node.ID+"/reboot", fixture.adminToken, true, map[string]any{
		"confirmation": fixture.node.Name,
		"reason":       lifecycleRebootReason,
	}, &rebootJob)
	if rebootRec.Code != nethttp.StatusAccepted {
		t.Fatalf("reboot operator status = %d, want 202; body=%s", rebootRec.Code, rebootRec.Body.String())
	}
	assertOperatorLifecycleJobMetadata(t, "reboot operator response", rebootRec.Body.Bytes(), rebootJob, fixture.node.ID, "node.reboot")
	assertLifecycleQueuedJob(t, fixture, rebootJob.ID, "node.reboot")
	assertLifecycleQueueAudit(t, fixture, "node.reboot.queue", rebootJob.ID, lifecycleRebootReason, 1)
	assertLifecycleNodeAndAgentPreserved(t, fixture, beforeNode, beforeAgentHash)
	assertHTTPPostgresNodeHeartbeatEquals(t, fixture.ctx, fixture.pool, fixture.node.ID, fixture.heartbeatAt)

	var claimedReboot domain.Job
	rebootPoll := sendSignedAgentJSON(t, fixture.handler, nethttp.MethodGet, "/agent/jobs/next?node_id="+fixture.node.ID, fixture.registration.AgentToken, nil, &claimedReboot)
	if rebootPoll.rec.Code != nethttp.StatusOK {
		t.Fatalf("signed-agent reboot poll status = %d, want 200; body=%s", rebootPoll.rec.Code, rebootPoll.rec.Body.String())
	}
	verifySignedAgentResponse(t, rebootPoll.req, rebootPoll.rec, fixture.registration.AgentToken)
	assertLifecycleClaimedJob(t, fixture, claimedReboot, rebootJob.ID, "node.reboot")
	assertRebootAgentPayload(t, claimedReboot.Payload, fixture.node.ID, fixture.node.Name)

	rebootResultPayload := simulatedSignedAgentRebootResult(fixture.node)
	rebootResult := sendSignedAgentJSON(t, fixture.handler, nethttp.MethodPost, "/agent/jobs/"+rebootJob.ID+"/result", fixture.registration.AgentToken, map[string]any{
		"status": "succeeded",
		"result": rebootResultPayload,
	}, nil)
	if rebootResult.rec.Code != nethttp.StatusOK {
		t.Fatalf("simulated signed-agent reboot result status = %d, want 200; body=%s", rebootResult.rec.Code, rebootResult.rec.Body.String())
	}
	verifySignedAgentResponse(t, rebootResult.req, rebootResult.rec, fixture.registration.AgentToken)
	assertLifecycleAcceptedResult(t, rebootResult.rec)
	assertLifecycleTerminalJob(t, fixture, rebootJob.ID, rebootResultPayload)
	assertLifecycleNodeAndAgentPreserved(t, fixture, beforeNode, beforeAgentHash)
	assertHTTPPostgresNodeHeartbeatEquals(t, fixture.ctx, fixture.pool, fixture.node.ID, fixture.heartbeatAt)

	var cleanupNode domain.Node
	maintenanceRec := sendOperatorJSON(t, fixture.handler, nethttp.MethodPost, "/api/v1/nodes/"+fixture.node.ID+"/maintenance/enable", fixture.adminToken, true, nil, &cleanupNode)
	if maintenanceRec.Code != nethttp.StatusOK || cleanupNode.Status != "maintenance" {
		t.Fatalf("maintenance enable before cleanup status/state = %d/%q, want 200/maintenance", maintenanceRec.Code, cleanupNode.Status)
	}

	instance := createNodeLifecycleManagedInstance(t, fixture)
	var instanceStatusBefore string
	var instanceEnabledBefore bool
	if err := fixture.pool.QueryRow(fixture.ctx, `select status,enabled from instances where id=$1`, instance.ID).Scan(&instanceStatusBefore, &instanceEnabledBefore); err != nil {
		t.Fatalf("query cleanup fixture instance: %v", err)
	}

	cleanupWithoutCSRF := sendOperatorJSON(t, fixture.handler, nethttp.MethodPost, "/api/v1/nodes/"+fixture.node.ID+"/emergency-cleanup", fixture.adminToken, false, lifecycleCleanupRequest(fixture.node.Name), nil)
	if cleanupWithoutCSRF.Code != nethttp.StatusForbidden {
		t.Fatalf("cleanup without CSRF status = %d, want 403", cleanupWithoutCSRF.Code)
	}
	assertHTTPPostgresCount(t, fixture.ctx, fixture.pool, `select count(*) from jobs where node_id=$1 and type='node.emergency_cleanup'`, 0, fixture.node.ID)

	var cleanupResponse struct {
		Status      string                                 `json:"status"`
		Message     string                                 `json:"message"`
		Job         domain.Job                             `json:"job"`
		PlanSummary domain.NodeEmergencyCleanupPlanSummary `json:"plan_summary"`
	}
	cleanupRec := sendOperatorJSON(t, fixture.handler, nethttp.MethodPost, "/api/v1/nodes/"+fixture.node.ID+"/emergency-cleanup", fixture.adminToken, true, lifecycleCleanupRequest(fixture.node.Name), &cleanupResponse)
	if cleanupRec.Code != nethttp.StatusAccepted {
		t.Fatalf("cleanup operator status = %d, want 202; body=%s", cleanupRec.Code, cleanupRec.Body.String())
	}
	if cleanupResponse.Status != "queued" || cleanupResponse.Message != "emergency cleanup job queued" {
		t.Fatalf("cleanup operator queue projection = status:%q message:%q", cleanupResponse.Status, cleanupResponse.Message)
	}
	assertOperatorLifecycleJobMetadata(t, "cleanup operator response", lifecycleNestedJSON(t, cleanupRec.Body.Bytes(), "job"), cleanupResponse.Job, fixture.node.ID, "node.emergency_cleanup")
	assertCleanupOperatorResponseSafe(t, cleanupRec.Body.Bytes(), cleanupResponse.PlanSummary, 1)
	assertLifecycleQueuedJob(t, fixture, cleanupResponse.Job.ID, "node.emergency_cleanup")
	assertLifecycleQueueAudit(t, fixture, "node.emergency_cleanup.queue", cleanupResponse.Job.ID, lifecycleCleanupReason, 1)
	assertHTTPPostgresCount(t, fixture.ctx, fixture.pool, `select count(*) from instances where id=$1 and status=$2 and enabled=$3`, 1, instance.ID, instanceStatusBefore, instanceEnabledBefore)

	var claimedCleanup domain.Job
	cleanupPoll := sendSignedAgentJSON(t, fixture.handler, nethttp.MethodGet, "/agent/jobs/next?node_id="+fixture.node.ID, fixture.registration.AgentToken, nil, &claimedCleanup)
	if cleanupPoll.rec.Code != nethttp.StatusOK {
		t.Fatalf("signed-agent cleanup poll status = %d, want 200; body=%s", cleanupPoll.rec.Code, cleanupPoll.rec.Body.String())
	}
	verifySignedAgentResponse(t, cleanupPoll.req, cleanupPoll.rec, fixture.registration.AgentToken)
	assertLifecycleClaimedJob(t, fixture, claimedCleanup, cleanupResponse.Job.ID, "node.emergency_cleanup")
	assertCleanupAgentPayload(t, claimedCleanup.Payload, fixture, instance)

	cleanupResultPayload := simulatedSignedAgentCleanupResult(fixture.node.ID, instance)
	cleanupResult := sendSignedAgentJSON(t, fixture.handler, nethttp.MethodPost, "/agent/jobs/"+cleanupResponse.Job.ID+"/result", fixture.registration.AgentToken, map[string]any{
		"status": "succeeded",
		"result": cleanupResultPayload,
	}, nil)
	if cleanupResult.rec.Code != nethttp.StatusOK {
		t.Fatalf("simulated signed-agent cleanup result status = %d, want 200; body=%s", cleanupResult.rec.Code, cleanupResult.rec.Body.String())
	}
	verifySignedAgentResponse(t, cleanupResult.req, cleanupResult.rec, fixture.registration.AgentToken)
	assertLifecycleAcceptedResult(t, cleanupResult.rec)
	assertLifecycleTerminalJob(t, fixture, cleanupResponse.Job.ID, cleanupResultPayload)
	assertHTTPPostgresCount(t, fixture.ctx, fixture.pool, `select count(*) from resource_locks where job_id=$1`, 0, cleanupResponse.Job.ID)
	assertHTTPPostgresCount(t, fixture.ctx, fixture.pool, `select count(*) from instances where id=$1 and status='deleted' and enabled=false`, 1, instance.ID)
	assertLifecycleNodeAndAgentPreserved(t, fixture, cleanupNode, beforeAgentHash)
	assertHTTPPostgresNodeHeartbeatEquals(t, fixture.ctx, fixture.pool, fixture.node.ID, fixture.heartbeatAt)
	assertHTTPPostgresCount(t, fixture.ctx, fixture.pool, `select count(*) from node_inventory_snapshots where node_id=$1`, fixture.inventoryCount, fixture.node.ID)

	resultLogCount := lifecycleCount(t, fixture, `select count(*) from job_logs where job_id=$1 and message='agent submitted result'`, cleanupResponse.Job.ID)
	replayRec := httptest.NewRecorder()
	fixture.handler.ServeHTTP(replayRec, cleanupResult.req)
	if replayRec.Code != nethttp.StatusUnauthorized {
		t.Fatalf("replayed signed cleanup result status = %d, want 401", replayRec.Code)
	}
	if afterReplay := lifecycleCount(t, fixture, `select count(*) from job_logs where job_id=$1 and message='agent submitted result'`, cleanupResponse.Job.ID); afterReplay != resultLogCount {
		t.Fatalf("replayed result mutated accepted-result log count: before=%d after=%d", resultLogCount, afterReplay)
	}
	assertHTTPPostgresCount(t, fixture.ctx, fixture.pool, `select count(*) from jobs where id=$1 and status='succeeded'`, 1, cleanupResponse.Job.ID)

	return nodeLifecycleHTTPSequenceEvidence{
		rebootJobID:          rebootJob.ID,
		cleanupJobID:         cleanupResponse.Job.ID,
		instanceID:           instance.ID,
		rebootOperatorBody:   append([]byte(nil), rebootRec.Body.Bytes()...),
		rebootPoll:           rebootPoll,
		rebootResult:         rebootResult,
		cleanupOperatorBody:  append([]byte(nil), cleanupRec.Body.Bytes()...),
		cleanupPoll:          cleanupPoll,
		cleanupResult:        cleanupResult,
		cleanupReplayBody:    append([]byte(nil), replayRec.Body.Bytes()...),
		cleanupResultPayload: cleanupResultPayload,
	}
}

func lifecycleCleanupRequest(nodeName string) map[string]any {
	return map[string]any{
		"cleanup_scope":                   domain.NodeEmergencyCleanupScopeServicesOnly,
		"include_agent":                   false,
		"confirmation":                    nodeName,
		"reason":                          lifecycleCleanupReason,
		"acknowledge_destructive_cleanup": true,
		"acknowledge_agent_removal":       false,
	}
}

func createNodeLifecycleManagedInstance(t *testing.T, fixture nodeLifecycleHTTPFixture) domain.Instance {
	t.Helper()

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	instance, err := fixture.store.CreateInstanceDraft(fixture.ctx, domain.Instance{
		NodeID:       fixture.node.ID,
		ServiceCode:  "xray-core",
		Name:         "lifecycle-xray-" + suffix,
		Slug:         "lifecycle-xray-" + suffix,
		Status:       "active",
		EndpointHost: "198.51.100.96",
		EndpointPort: 443,
		Spec: map[string]any{
			"private_key":        fixture.markers["private_key"],
			"secret_ref_id":      fixture.markers["secret_ref"],
			"credential":         fixture.markers["credential"],
			"config_content":     fixture.markers["configuration"],
			"raw_command_output": fixture.markers["command_output"],
		},
	})
	if err != nil {
		t.Fatalf("create disposable cleanup instance fixture: %v", err)
	}
	return instance
}

func simulatedSignedAgentRebootResult(node domain.Node) map[string]any {
	return map[string]any{
		"message":       "node reboot command scheduled",
		"node_id":       node.ID,
		"node_name":     node.Name,
		"reason":        lifecycleRebootReason,
		"agent_version": "it-lifecycle-agent/1",
		"scheduled":     true,
		"mechanism":     "disposable-integration-simulator",
		"fallback_used": false,
		"delay_seconds": 5,
		"duration_ms":   1,
	}
}

func simulatedSignedAgentCleanupResult(nodeID string, instance domain.Instance) map[string]any {
	return map[string]any{
		"message":              "emergency cleanup execution finished",
		"outcome":              "succeeded",
		"node_id":              nodeID,
		"cleanup_scope":        domain.NodeEmergencyCleanupScopeServicesOnly,
		"include_agent":        false,
		"cleanup_plan_version": domain.NodeEmergencyCleanupPlanVersion,
		"summary": map[string]any{
			"targets_total":          1,
			"targets_succeeded":      1,
			"targets_already_absent": 0,
			"targets_failed":         0,
			"units_stopped":          0,
			"paths_removed":          0,
			"warnings":               0,
		},
		"targets": []any{map[string]any{
			"instance_id":  instance.ID,
			"service_code": instance.ServiceCode,
			"status":       "succeeded",
			"code":         "removed",
		}},
		"nginx":              map[string]any{"status": "not_required", "remaining_managed_configs": 0},
		"systemd":            map[string]any{"daemon_reload": "not_required"},
		"agent_self_removal": map[string]any{"requested": false, "state": "not_requested"},
		"duration_ms":        1,
	}
}

func assertOperatorLifecycleJobMetadata(t *testing.T, label string, body []byte, job domain.Job, nodeID, jobType string) {
	t.Helper()

	if job.ID == "" || job.Type != jobType || job.Status != "queued" || job.NodeID == nil || *job.NodeID != nodeID || job.ScopeID == nil || *job.ScopeID != nodeID {
		t.Fatalf("%s metadata mismatch: id_present=%t type=%q status=%q node_match=%t scope_match=%t", label, job.ID != "", job.Type, job.Status, job.NodeID != nil && *job.NodeID == nodeID, job.ScopeID != nil && *job.ScopeID == nodeID)
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("decode %s metadata: %v", label, err)
	}
	for _, forbidden := range []string{"payload", "result", "locked_by", "locked_until"} {
		if _, ok := raw[forbidden]; ok {
			t.Fatalf("%s exposed internal field %q", label, forbidden)
		}
	}
}

func assertCleanupOperatorResponseSafe(t *testing.T, body []byte, summary domain.NodeEmergencyCleanupPlanSummary, wantTargets int) {
	t.Helper()

	if summary.CleanupScope != domain.NodeEmergencyCleanupScopeServicesOnly || summary.IncludeAgent || summary.AgentRemovalRequested || summary.NodeRuntimeCleanup || summary.InstanceTargetCount != wantTargets || summary.ServiceCounts["xray-core"] != wantTargets {
		t.Fatalf("cleanup plan summary mismatch: %#v", summary)
	}
	for _, forbidden := range []string{"instances", "systemd_unit", "endpoint_host", "confirmation", "reason", "command", "private_key", "secret_ref", "config_content"} {
		if strings.Contains(strings.ToLower(string(body)), strings.ToLower(forbidden)) {
			t.Fatalf("cleanup operator response exposed %q", forbidden)
		}
	}
}

func assertLifecycleQueuedJob(t *testing.T, fixture nodeLifecycleHTTPFixture, jobID, jobType string) {
	t.Helper()

	var gotType string
	var status string
	var nodeID string
	if err := fixture.pool.QueryRow(fixture.ctx, `select type,status,node_id::text from jobs where id=$1`, jobID).Scan(&gotType, &status, &nodeID); err != nil {
		t.Fatalf("query queued lifecycle job: %v", err)
	}
	if gotType != jobType || status != "queued" || nodeID != fixture.node.ID {
		t.Fatalf("queued lifecycle job mismatch: type=%q status=%q node_match=%t", gotType, status, nodeID == fixture.node.ID)
	}
	assertHTTPPostgresCount(t, fixture.ctx, fixture.pool, `select count(*) from resource_locks where job_id=$1 and resource_type='node' and resource_id=$2 and lock_kind='bootstrap'`, 1, jobID, fixture.node.ID)
}

func assertLifecycleClaimedJob(t *testing.T, fixture nodeLifecycleHTTPFixture, job domain.Job, jobID, jobType string) {
	t.Helper()

	if job.ID != jobID || job.Type != jobType || job.Status != "running" || job.NodeID == nil || *job.NodeID != fixture.node.ID {
		t.Fatalf("claimed lifecycle job mismatch: id_match=%t type=%q status=%q node_match=%t", job.ID == jobID, job.Type, job.Status, job.NodeID != nil && *job.NodeID == fixture.node.ID)
	}
	var status string
	var owner *string
	var lockedUntil *time.Time
	if err := fixture.pool.QueryRow(fixture.ctx, `select status,locked_by,locked_until from jobs where id=$1`, jobID).Scan(&status, &owner, &lockedUntil); err != nil {
		t.Fatalf("query claimed lifecycle job: %v", err)
	}
	wantOwner := "agent:" + fixture.node.ID
	if status != "running" || owner == nil || *owner != wantOwner || lockedUntil == nil || !lockedUntil.After(time.Now().UTC()) {
		t.Fatalf("claimed lifecycle lease mismatch: status=%q owner_match=%t lease_current=%t", status, owner != nil && *owner == wantOwner, lockedUntil != nil && lockedUntil.After(time.Now().UTC()))
	}
	assertHTTPPostgresCount(t, fixture.ctx, fixture.pool, `select count(*) from resource_locks where job_id=$1 and resource_type='node' and resource_id=$2 and lock_kind='bootstrap'`, 1, jobID, fixture.node.ID)
}

func assertRebootAgentPayload(t *testing.T, payload map[string]any, nodeID, nodeName string) {
	t.Helper()

	assertLifecycleExactKeys(t, "signed-agent reboot payload", payload, []string{"schema", "node_id", "node_name", "confirmation", "reason", "requested_at"})
	if payload["schema"] != "node.reboot.v1" || payload["node_id"] != nodeID || payload["node_name"] != nodeName || payload["confirmation"] != nodeName || payload["reason"] != lifecycleRebootReason || strings.TrimSpace(stringifyHTTP(payload["requested_at"])) == "" {
		t.Fatalf("signed-agent reboot payload contract mismatch")
	}
	encoded := mustJSONFromAny(t, payload)
	for _, forbidden := range []string{"token", "hash", "secret", "credential", "private_key", "command", "output"} {
		if strings.Contains(strings.ToLower(string(encoded)), forbidden) {
			t.Fatalf("signed-agent reboot payload exposed forbidden category %q", forbidden)
		}
	}
}

func assertCleanupAgentPayload(t *testing.T, payload map[string]any, fixture nodeLifecycleHTTPFixture, instance domain.Instance) {
	t.Helper()

	assertLifecycleExactKeys(t, "signed-agent cleanup payload", payload, []string{"schema", "node_id", "node_name", "cleanup_scope", "include_agent", "confirmation", "reason", "requested_at", "cleanup_plan_version", "instances"})
	if payload["schema"] != "node.emergency_cleanup.v1" || payload["node_id"] != fixture.node.ID || payload["cleanup_scope"] != domain.NodeEmergencyCleanupScopeServicesOnly || payload["include_agent"] != false || payload["cleanup_plan_version"] != float64(domain.NodeEmergencyCleanupPlanVersion) && payload["cleanup_plan_version"] != domain.NodeEmergencyCleanupPlanVersion {
		t.Fatalf("signed-agent cleanup payload contract mismatch")
	}
	targets, ok := payload["instances"].([]any)
	if !ok || len(targets) != 1 {
		t.Fatalf("signed-agent cleanup target count/type mismatch: type=%T count=%d", payload["instances"], len(targets))
	}
	target, ok := targets[0].(map[string]any)
	if !ok {
		t.Fatalf("signed-agent cleanup target type = %T, want object", targets[0])
	}
	assertLifecycleExactKeys(t, "signed-agent cleanup target", target, []string{"instance_id", "action", "service_code", "runtime_service_code", "name", "slug", "systemd_unit", "endpoint_host", "endpoint_port", "enabled"})
	if target["instance_id"] != instance.ID || target["service_code"] != instance.ServiceCode || target["action"] != "delete" || target["enabled"] != false {
		t.Fatalf("signed-agent cleanup target contract mismatch")
	}
	encoded := mustJSONFromAny(t, payload)
	assertLifecycleBodyExcludes(t, "signed-agent cleanup payload", encoded, []lifecycleForbiddenValue{
		{category: "agent credential", value: fixture.registration.AgentToken},
		{category: "agent credential hash", value: fixture.agentTokenHash},
		{category: "private-key marker", value: fixture.markers["private_key"]},
		{category: "secret-reference marker", value: fixture.markers["secret_ref"]},
		{category: "credential marker", value: fixture.markers["credential"]},
		{category: "configuration marker", value: fixture.markers["configuration"]},
		{category: "raw command-output marker", value: fixture.markers["command_output"]},
	})
}

func assertLifecycleTerminalJob(t *testing.T, fixture nodeLifecycleHTTPFixture, jobID string, expectedResult map[string]any) {
	t.Helper()

	var status string
	var finished bool
	var resultRaw []byte
	if err := fixture.pool.QueryRow(fixture.ctx, `select status,finished_at is not null,result_json from jobs where id=$1`, jobID).Scan(&status, &finished, &resultRaw); err != nil {
		t.Fatalf("query terminal lifecycle job: %v", err)
	}
	if status != "succeeded" || !finished {
		t.Fatalf("terminal lifecycle job state = %q/finished:%t, want succeeded/true", status, finished)
	}
	var stored map[string]any
	if err := json.Unmarshal(resultRaw, &stored); err != nil {
		t.Fatalf("decode terminal lifecycle result: %v", err)
	}
	if stringifyHTTP(stored["node_id"]) != stringifyHTTP(expectedResult["node_id"]) {
		t.Fatal("terminal lifecycle result node ownership mismatch")
	}
	assertHTTPPostgresCount(t, fixture.ctx, fixture.pool, `select count(*) from resource_locks where job_id=$1`, 0, jobID)
	assertHTTPPostgresCount(t, fixture.ctx, fixture.pool, `select count(*) from job_logs where job_id=$1 and message='agent submitted result'`, 1, jobID)
}

func assertLifecycleQueueAudit(t *testing.T, fixture nodeLifecycleHTTPFixture, action, jobID, reason string, want int) {
	t.Helper()

	var count int
	if err := fixture.pool.QueryRow(fixture.ctx, `select count(*) from audit_events where resource_id=$1 and action=$2`, fixture.node.ID, action).Scan(&count); err != nil {
		t.Fatalf("count lifecycle queue audit: %v", err)
	}
	if count != want {
		t.Fatalf("lifecycle audit %s count = %d, want %d", action, count, want)
	}
	if want == 0 {
		return
	}
	var actor *string
	var actorType string
	var resourceType string
	var summary string
	var payload string
	if err := fixture.pool.QueryRow(fixture.ctx, `select actor_user_id,actor_type,resource_type,summary,payload_json::text from audit_events where resource_id=$1 and action=$2 order by created_at desc limit 1`, fixture.node.ID, action).Scan(&actor, &actorType, &resourceType, &summary, &payload); err != nil {
		t.Fatalf("query lifecycle queue audit: %v", err)
	}
	lower := strings.ToLower(summary)
	if actor == nil || *actor != fixture.adminUserID || actorType != "platform_user" || resourceType != "node" || !strings.Contains(summary, jobID) || !strings.Contains(summary, reason) || !strings.Contains(lower, "queued") {
		t.Fatalf("lifecycle audit actor/resource/queue metadata mismatch: actor_match=%t actor_type=%q resource=%q job=%t reason=%t queued=%t", actor != nil && *actor == fixture.adminUserID, actorType, resourceType, strings.Contains(summary, jobID), strings.Contains(summary, reason), strings.Contains(lower, "queued"))
	}
	for _, forbidden := range []string{"rebooted", "recovered", "cleanup completed", "files removed", "agent removed", "agent_token", "token_hash", "authorization", "private_key", "secret_ref", "config_content"} {
		if strings.Contains(strings.ToLower(summary+"\n"+payload), strings.ToLower(forbidden)) {
			t.Fatalf("lifecycle queue audit contains forbidden category %q", forbidden)
		}
	}
}

func assertLifecycleNodeAndAgentPreserved(t *testing.T, fixture nodeLifecycleHTTPFixture, before domain.Node, tokenHash string) {
	t.Helper()

	after, err := fixture.store.GetNode(fixture.ctx, before.ID)
	if err != nil {
		t.Fatalf("get node after lifecycle protocol operation: %v", err)
	}
	if after.Name != before.Name || after.Address != before.Address || after.Role != before.Role || after.Status != before.Status || after.Kind != before.Kind || after.ExecutionMode != before.ExecutionMode || after.LocationLabel != before.LocationLabel || after.OSVersion != before.OSVersion || after.Architecture != before.Architecture {
		t.Fatalf("node profile changed during lifecycle protocol evidence")
	}
	var storedHash string
	var status string
	var revoked bool
	if err := fixture.pool.QueryRow(fixture.ctx, `select coalesce(agent_token_hash,''),status,revoked_at is not null from node_agents where node_id=$1`, fixture.node.ID).Scan(&storedHash, &status, &revoked); err != nil {
		t.Fatalf("query agent identity after lifecycle protocol operation: %v", err)
	}
	if storedHash != tokenHash || status != "active" || revoked {
		t.Fatalf("agent identity changed during lifecycle protocol evidence: hash_same=%t status=%q revoked=%t", storedHash == tokenHash, status, revoked)
	}
}

func assertLifecycleAcceptedResult(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode signed-agent result acknowledgement: %v", err)
	}
	if len(payload) != 1 || payload["status"] != "accepted" {
		t.Fatalf("signed-agent result acknowledgement mismatch: %#v", payload)
	}
}

func assertLifecycleErrorCode(t *testing.T, rec *httptest.ResponseRecorder, want string) {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode lifecycle error response: %v", err)
	}
	if payload["code"] != want {
		t.Fatalf("lifecycle error code = %#v, want %q", payload["code"], want)
	}
}

func assertLifecycleExactKeys(t *testing.T, label string, payload map[string]any, expected []string) {
	t.Helper()

	got := make([]string, 0, len(payload))
	for key := range payload {
		got = append(got, key)
	}
	sort.Strings(got)
	want := append([]string(nil), expected...)
	sort.Strings(want)
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("%s keys = %v, want %v", label, got, want)
	}
}

func assertLifecycleBodyExcludes(t *testing.T, label string, body []byte, forbidden []lifecycleForbiddenValue) {
	t.Helper()

	for _, item := range forbidden {
		if item.value != "" && bytes.Contains(body, []byte(item.value)) {
			t.Fatalf("%s exposed %s", label, item.category)
		}
	}
}

func lifecycleCount(t *testing.T, fixture nodeLifecycleHTTPFixture, query string, args ...any) int {
	t.Helper()

	var count int
	if err := fixture.pool.QueryRow(fixture.ctx, query, args...).Scan(&count); err != nil {
		t.Fatalf("count lifecycle rows: %v", err)
	}
	return count
}

func mustJSONFromAny(t *testing.T, value any) []byte {
	t.Helper()

	body, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal lifecycle assertion value: %v", err)
	}
	return body
}

func lifecycleNestedJSON(t *testing.T, body []byte, key string) []byte {
	t.Helper()

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode lifecycle response envelope: %v", err)
	}
	nested, ok := payload[key]
	if !ok {
		t.Fatalf("lifecycle response is missing %q", key)
	}
	return nested
}
