package http

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	nethttp "net/http"
	"net/http/httptest"
	"strconv"
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
	agentOnboardingVersion = "it-agent-onboarding/4b2"
	agentRecoveredVersion  = "it-agent-recovered/4b2"
	agentHeartbeatVersion  = "it-agent-heartbeat/4b2"
	agentProtocolVersion   = "v1"
)

func TestPostgresIntegrationAgentOnboardingRegisterHeartbeatInventoryDiagnosticsHTTP(t *testing.T) {
	store, pool, ctx := setupHTTPPostgresIntegrationStore(t)
	fixture := newAgentOnboardingHTTPFixture(t, ctx, store)
	handler, logs := newHTTPPostgresAgentOnboardingHandler(store)

	node := createAgentOnboardingNodeViaHTTP(t, handler, fixture.adminToken, "primary")
	readonlyCreate := sendOperatorJSON(t, handler, nethttp.MethodPost, "/api/v1/nodes", fixture.readonlyToken, true, agentOnboardingNodePayload("readonly-denied"), nil)
	if readonlyCreate.Code != nethttp.StatusForbidden {
		t.Fatalf("readonly node create status = %d, want 403", readonlyCreate.Code)
	}

	readonlyEnrollment := sendOperatorJSON(t, handler, nethttp.MethodPost, "/api/v1/nodes/"+node.ID+"/enrollment-token?ttl_hours=1", fixture.readonlyToken, true, nil, nil)
	if readonlyEnrollment.Code != nethttp.StatusForbidden {
		t.Fatalf("readonly enrollment status = %d, want 403", readonlyEnrollment.Code)
	}
	missingCSRFEnrollment := sendOperatorJSON(t, handler, nethttp.MethodPost, "/api/v1/nodes/"+node.ID+"/enrollment-token?ttl_hours=1", fixture.adminToken, false, nil, nil)
	if missingCSRFEnrollment.Code != nethttp.StatusForbidden {
		t.Fatalf("missing csrf enrollment status = %d, want 403", missingCSRFEnrollment.Code)
	}
	unauthenticatedEnrollment := sendOperatorJSON(t, handler, nethttp.MethodPost, "/api/v1/nodes/"+node.ID+"/enrollment-token?ttl_hours=1", "", true, nil, nil)
	if unauthenticatedEnrollment.Code != nethttp.StatusUnauthorized {
		t.Fatalf("unauthenticated enrollment status = %d, want 401", unauthenticatedEnrollment.Code)
	}

	sibling := createAgentOnboardingEnrollmentTokenHTTP(t, handler, node.ID, fixture.adminToken)
	enrollment := createAgentOnboardingEnrollmentTokenHTTP(t, handler, node.ID, fixture.adminToken)
	assertHTTPPostgresEnrollmentTokenStatus(t, ctx, pool, sibling.ID, "revoked")
	if enrollment.Token == "" || enrollment.TokenHint == "" {
		t.Fatal("enrollment endpoint must return one-time plaintext token and safe hint")
	}

	reg := registerAgentOnboardingHTTP(t, handler, node.ID, enrollment.Token, "agent-supplied-name", "192.0.2.250", agentOnboardingVersion)
	if reg.Status != "registered" || reg.AgentToken == "" || reg.TokenHint == "" {
		t.Fatalf("registration response missing compatibility fields: status=%q token_present=%t hint_present=%t", reg.Status, reg.AgentToken != "", reg.TokenHint != "")
	}
	if reg.Node.ID != node.ID || reg.Node.Name != node.Name || reg.Node.Address != node.Address {
		t.Fatalf("registered node projection = %#v, want persisted operator node profile", reg.Node)
	}
	assertAgentRegisterNoStoreHeaders(t, reg.rec)
	assertHTTPPostgresPayloadNotContains(t, "registration response", reg.rec.Body.Bytes(), []string{"agent_token_hash", "token_hash", "token_id", "postgres", "sql"})

	assertHTTPPostgresEnrollmentTokenStatus(t, ctx, pool, enrollment.ID, "used")
	assertHTTPPostgresActiveEnrollmentTokenCount(t, ctx, pool, node.ID, 0)
	agentState := assertHTTPPostgresSingleNodeAgentIdentity(t, ctx, pool, node.ID, reg.AgentToken)
	if agentState.AgentVersion != agentOnboardingVersion || agentState.ProtocolVersion != agentProtocolVersion || agentState.RegisteredAt == nil || agentState.LastSeenAt == nil {
		t.Fatalf("registered agent state mismatch: version=%q protocol=%q registered=%t seen=%t",
			agentState.AgentVersion, agentState.ProtocolVersion, agentState.RegisteredAt != nil, agentState.LastSeenAt != nil)
	}
	assertHTTPPostgresAgentRegisterAudit(t, ctx, pool, node.ID, 1, enrollment.Token, reg.AgentToken)
	assertHTTPPostgresNodeProfilePreserved(t, ctx, store, node)
	assertHTTPPostgresDatabaseTextExcludes(t, ctx, pool, "agent registration storage", node.ID, []string{enrollment.Token, reg.AgentToken})

	consumed := registerAgentOnboardingHTTPRaw(t, handler, node.ID, enrollment.Token, "agent-retry", "192.0.2.251", agentOnboardingVersion)
	if consumed.Code != nethttp.StatusUnauthorized {
		t.Fatalf("consumed token retry status = %d, want 401", consumed.Code)
	}
	assertAgentRegisterErrorCode(t, consumed, agentEnrollmentReissueRequired)
	assertHTTPPostgresPayloadNotContains(t, "consumed token response", consumed.Body.Bytes(), []string{enrollment.Token, reg.AgentToken, "used", "revoked", "expired", "invalid credential"})
	assertHTTPPostgresAgentRegisterAudit(t, ctx, pool, node.ID, 1, enrollment.Token, reg.AgentToken)

	invalidEnrollment := registerAgentOnboardingHTTPRaw(t, handler, node.ID, "synthetic-invalid-enrollment-token", "agent", "192.0.2.252", agentOnboardingVersion)
	if invalidEnrollment.Code != nethttp.StatusUnauthorized {
		t.Fatalf("invalid enrollment status = %d, want 401", invalidEnrollment.Code)
	}
	assertAgentRegisterErrorCode(t, invalidEnrollment, agentEnrollmentReissueRequired)
	assertHTTPPostgresPayloadNotContains(t, "invalid enrollment response", invalidEnrollment.Body.Bytes(), []string{"synthetic-invalid-enrollment-token", reg.AgentToken, "token_hash", "sql"})

	retiredNode := createAgentOnboardingNodeViaHTTP(t, handler, fixture.adminToken, "retired")
	retiredEnrollment := createAgentOnboardingEnrollmentTokenHTTP(t, handler, retiredNode.ID, fixture.adminToken)
	if _, err := store.RetireNode(ctx, retiredNode.ID); err != nil {
		t.Fatalf("retire fixture node: %v", err)
	}
	retired := registerAgentOnboardingHTTPRaw(t, handler, retiredNode.ID, retiredEnrollment.Token, "agent-retired", "192.0.2.253", agentOnboardingVersion)
	if retired.Code != nethttp.StatusConflict {
		t.Fatalf("retired registration status = %d, want 409", retired.Code)
	}
	assertAgentRegisterErrorCode(t, retired, agentEnrollmentNodeRetired)
	assertHTTPPostgresPayloadNotContains(t, "retired registration response", retired.Body.Bytes(), []string{retiredEnrollment.Token, reg.AgentToken, "token_hash", "sql"})

	crossNode := createAgentOnboardingNodeViaHTTP(t, handler, fixture.adminToken, "cross")
	crossEnrollment := createAgentOnboardingEnrollmentTokenHTTP(t, handler, crossNode.ID, fixture.adminToken)
	crossReg := registerAgentOnboardingHTTP(t, handler, crossNode.ID, crossEnrollment.Token, "cross-agent", "192.0.2.254", agentOnboardingVersion)

	heartbeatReq := map[string]any{"node_id": node.ID, "name": node.Name, "agent_version": agentHeartbeatVersion, "protocol_version": agentProtocolVersion}
	heartbeatRec := sendSignedAgentJSON(t, handler, nethttp.MethodPost, "/agent/heartbeat", reg.AgentToken, heartbeatReq, nil)
	if heartbeatRec.rec.Code != nethttp.StatusOK {
		t.Fatalf("heartbeat status = %d, want 200", heartbeatRec.rec.Code)
	}
	verifySignedAgentResponse(t, heartbeatRec.req, heartbeatRec.rec, reg.AgentToken)
	assertHTTPPostgresPayloadNotContains(t, "heartbeat response", heartbeatRec.rec.Body.Bytes(), []string{reg.AgentToken, enrollment.Token, "token_hash"})
	assertHTTPPostgresNoStoreHeader(t, heartbeatRec.rec)

	afterHeartbeat := assertHTTPPostgresSingleNodeAgentIdentity(t, ctx, pool, node.ID, reg.AgentToken)
	if afterHeartbeat.AgentVersion != agentHeartbeatVersion || afterHeartbeat.ProtocolVersion != agentProtocolVersion {
		t.Fatalf("heartbeat runtime version projection = %q/%q, want %q/%q",
			afterHeartbeat.AgentVersion, afterHeartbeat.ProtocolVersion, agentHeartbeatVersion, agentProtocolVersion)
	}
	heartbeatAt := assertHTTPPostgresNodeHeartbeatSet(t, ctx, pool, node.ID)
	assertHTTPPostgresNodeProfilePreserved(t, ctx, store, node)

	inventoryBefore := countHTTPPostgresNodeInventorySnapshots(t, ctx, pool, node.ID)
	assertAgentSignatureBoundaryDoesNotMutate(t, handler, ctx, pool, node.ID, reg.AgentToken, crossReg.AgentToken, heartbeatReq, heartbeatAt, afterHeartbeat.LastSeenAt, inventoryBefore)

	// A later valid heartbeat proves recovery from prior rejected requests and keeps
	// diagnostics from being dominated by an earlier auth-failure timestamp.
	secondHeartbeatRec := sendSignedAgentJSON(t, handler, nethttp.MethodPost, "/agent/heartbeat", reg.AgentToken, heartbeatReq, nil)
	if secondHeartbeatRec.rec.Code != nethttp.StatusOK {
		t.Fatalf("second heartbeat status = %d, want 200", secondHeartbeatRec.rec.Code)
	}
	verifySignedAgentResponse(t, secondHeartbeatRec.req, secondHeartbeatRec.rec, reg.AgentToken)

	readonlyInventoryJob := sendOperatorJSON(t, handler, nethttp.MethodPost, "/api/v1/nodes/"+node.ID+"/inventory/sync", fixture.readonlyToken, true, nil, nil)
	if readonlyInventoryJob.Code != nethttp.StatusForbidden {
		t.Fatalf("readonly inventory sync status = %d, want 403", readonlyInventoryJob.Code)
	}
	missingCSRFInventoryJob := sendOperatorJSON(t, handler, nethttp.MethodPost, "/api/v1/nodes/"+node.ID+"/inventory/sync", fixture.adminToken, false, nil, nil)
	if missingCSRFInventoryJob.Code != nethttp.StatusForbidden {
		t.Fatalf("missing csrf inventory sync status = %d, want 403", missingCSRFInventoryJob.Code)
	}
	var job domain.Job
	jobRec := sendOperatorJSON(t, handler, nethttp.MethodPost, "/api/v1/nodes/"+node.ID+"/inventory/sync", fixture.adminToken, true, nil, &job)
	if jobRec.Code != nethttp.StatusAccepted {
		t.Fatalf("inventory sync job status = %d, want 202", jobRec.Code)
	}
	if job.ID == "" || job.Type != "node.inventory.sync" || job.NodeID == nil || *job.NodeID != node.ID || job.ScopeID == nil || *job.ScopeID != node.ID {
		t.Fatalf("inventory sync job projection mismatch: %#v", job)
	}
	assertHTTPPostgresPayloadNotContains(t, "inventory sync job response", jobRec.Body.Bytes(), []string{reg.AgentToken, enrollment.Token, "enrollment_token", "agent_token"})

	var claimed domain.Job
	nextRec := sendSignedAgentJSON(t, handler, nethttp.MethodGet, "/agent/jobs/next?node_id="+node.ID, reg.AgentToken, nil, &claimed)
	if nextRec.rec.Code != nethttp.StatusOK {
		t.Fatalf("agent next job status = %d, want 200", nextRec.rec.Code)
	}
	verifySignedAgentResponse(t, nextRec.req, nextRec.rec, reg.AgentToken)
	if claimed.ID != job.ID || claimed.Type != "node.inventory.sync" || claimed.NodeID == nil || *claimed.NodeID != node.ID || claimed.Status != "running" {
		t.Fatalf("claimed job mismatch: %#v", claimed)
	}
	assertHTTPPostgresPayloadNotContains(t, "agent next job response", nextRec.rec.Body.Bytes(), []string{reg.AgentToken, enrollment.Token, "enrollment_token", "agent_token"})

	replayRec := httptest.NewRecorder()
	handler.ServeHTTP(replayRec, nextRec.req)
	if replayRec.Code != nethttp.StatusUnauthorized {
		t.Fatalf("replayed job poll status = %d, want 401", replayRec.Code)
	}
	assertHTTPPostgresPayloadNotContains(t, "replayed job poll response", replayRec.Body.Bytes(), []string{reg.AgentToken, enrollment.Token, "signature", "nonce"})

	malformedInventoryRec := sendSignedAgentJSON(t, handler, nethttp.MethodPost, "/agent/inventory", reg.AgentToken, map[string]any{"node_id": node.ID, "source": "inventory"}, nil)
	if malformedInventoryRec.rec.Code != nethttp.StatusBadRequest {
		t.Fatalf("malformed inventory status = %d, want 400", malformedInventoryRec.rec.Code)
	}
	assertHTTPPostgresInventorySnapshotCount(t, ctx, pool, node.ID, inventoryBefore)

	inventoryPayload := syntheticAgentOnboardingInventoryPayload()
	inventoryReq := map[string]any{"node_id": node.ID, "source": "inventory", "inventory": inventoryPayload}
	var inventoryResp struct {
		Status       string                       `json:"status"`
		Snapshot     domain.NodeInventorySnapshot `json:"snapshot"`
		Capabilities []domain.NodeCapability      `json:"capabilities"`
	}
	inventoryRec := sendSignedAgentJSON(t, handler, nethttp.MethodPost, "/agent/inventory", reg.AgentToken, inventoryReq, &inventoryResp)
	if inventoryRec.rec.Code != nethttp.StatusOK {
		t.Fatalf("inventory status = %d, want 200", inventoryRec.rec.Code)
	}
	verifySignedAgentResponse(t, inventoryRec.req, inventoryRec.rec, reg.AgentToken)
	if inventoryResp.Status != "accepted" || inventoryResp.Snapshot.NodeID != node.ID || inventoryResp.Snapshot.ID == "" {
		t.Fatalf("inventory response mismatch: status=%q snapshot=%#v", inventoryResp.Status, inventoryResp.Snapshot)
	}
	assertHTTPPostgresPayloadNotContains(t, "inventory response", inventoryRec.rec.Body.Bytes(), []string{reg.AgentToken, enrollment.Token, "authorization", "signature"})
	assertHTTPPostgresInventoryPersisted(t, ctx, pool, node.ID, inventoryPayload)

	resultReq := map[string]any{"status": "succeeded", "result": map[string]any{
		"message":     "synthetic inventory collected",
		"source":      "inventory",
		"snapshot_id": inventoryResp.Snapshot.ID,
	}}
	resultRec := sendSignedAgentJSON(t, handler, nethttp.MethodPost, "/agent/jobs/"+job.ID+"/result", reg.AgentToken, resultReq, nil)
	if resultRec.rec.Code != nethttp.StatusOK {
		t.Fatalf("job result status = %d, want 200", resultRec.rec.Code)
	}
	verifySignedAgentResponse(t, resultRec.req, resultRec.rec, reg.AgentToken)
	assertHTTPPostgresJobSucceeded(t, ctx, pool, job.ID, reg.AgentToken, enrollment.Token)

	var operatorInventory domain.NodeInventorySnapshot
	inventoryRead := sendOperatorJSON(t, handler, nethttp.MethodGet, "/api/v1/nodes/"+node.ID+"/inventory", fixture.adminToken, false, nil, &operatorInventory)
	if inventoryRead.Code != nethttp.StatusOK {
		t.Fatalf("operator inventory read status = %d, want 200", inventoryRead.Code)
	}
	if operatorInventory.NodeID != node.ID || operatorInventory.ID != inventoryResp.Snapshot.ID || stringifyHTTP(operatorInventory.Payload["hostname"]) != stringifyHTTP(inventoryPayload["hostname"]) {
		t.Fatalf("operator inventory read mismatch: %#v", operatorInventory)
	}
	assertHTTPPostgresPayloadNotContains(t, "operator inventory response", inventoryRead.Body.Bytes(), []string{reg.AgentToken, enrollment.Token, "token_hash", "authorization", "signature", "nonce"})
	readonlyInventoryRead := sendOperatorJSON(t, handler, nethttp.MethodGet, "/api/v1/nodes/"+node.ID+"/inventory", fixture.readonlyToken, false, nil, nil)
	if readonlyInventoryRead.Code != nethttp.StatusOK {
		t.Fatalf("readonly inventory read status = %d, want 200", readonlyInventoryRead.Code)
	}

	var diag domain.NodeDiagnostics
	diagRec := sendOperatorJSON(t, handler, nethttp.MethodGet, "/api/v1/nodes/"+node.ID+"/diagnostics", fixture.adminToken, false, nil, &diag)
	if diagRec.Code != nethttp.StatusOK {
		t.Fatalf("diagnostics status = %d, want 200", diagRec.Code)
	}
	assertAgentOnboardingDiagnostics(t, diag, node.ID, job.ID, agentHeartbeatVersion, "inventory_ok")
	assertHTTPPostgresPayloadNotContains(t, "diagnostics response", diagRec.Body.Bytes(), []string{
		reg.AgentToken,
		enrollment.Token,
		"token_hash",
		"agent_token",
		"Authorization",
		agentauth.HeaderSignature,
		agentauth.HeaderNonce,
		"secret_ref_id",
	})

	assertHTTPPostgresAgentRegisterAudit(t, ctx, pool, node.ID, 1, enrollment.Token, reg.AgentToken)
	assertHTTPPostgresAuditAndLogsRedacted(t, ctx, pool, logs.Bytes(), node.ID, []string{enrollment.Token, reg.AgentToken, crossReg.AgentToken})
	assertHTTPPostgresDatabaseTextExcludes(t, ctx, pool, "primary final storage", node.ID, []string{enrollment.Token, reg.AgentToken, crossReg.AgentToken})
}

func TestPostgresIntegrationAgentOnboardingReissueRecoveryHTTP(t *testing.T) {
	store, pool, ctx := setupHTTPPostgresIntegrationStore(t)
	fixture := newAgentOnboardingHTTPFixture(t, ctx, store)
	handler, logs := newHTTPPostgresAgentOnboardingHandler(store)

	node := createAgentOnboardingNodeViaHTTP(t, handler, fixture.adminToken, "reissue")
	firstEnrollment := createAgentOnboardingEnrollmentTokenHTTP(t, handler, node.ID, fixture.adminToken)

	firstRegister := registerAgentOnboardingHTTPRaw(t, handler, node.ID, firstEnrollment.Token, "lost-response-agent", "192.0.2.240", agentOnboardingVersion)
	if firstRegister.Code != nethttp.StatusOK {
		t.Fatalf("first registration status = %d, want 200", firstRegister.Code)
	}
	firstRegister.Body.Reset()
	assertHTTPPostgresEnrollmentTokenStatus(t, ctx, pool, firstEnrollment.ID, "used")
	firstHash := assertHTTPPostgresSingleNodeAgentIdentityWithoutPlaintext(t, ctx, pool, node.ID, firstEnrollment.Token)
	assertHTTPPostgresAgentRegisterAudit(t, ctx, pool, node.ID, 1, firstEnrollment.Token)

	retryConsumed := registerAgentOnboardingHTTPRaw(t, handler, node.ID, firstEnrollment.Token, "lost-response-agent", "192.0.2.240", agentOnboardingVersion)
	if retryConsumed.Code != nethttp.StatusUnauthorized {
		t.Fatalf("consumed retry status = %d, want 401", retryConsumed.Code)
	}
	assertAgentRegisterErrorCode(t, retryConsumed, agentEnrollmentReissueRequired)
	assertHTTPPostgresPayloadNotContains(t, "consumed retry response", retryConsumed.Body.Bytes(), []string{firstEnrollment.Token, "used", "revoked", "expired", "token_hash"})
	assertHTTPPostgresAgentRegisterAudit(t, ctx, pool, node.ID, 1, firstEnrollment.Token)
	assertHTTPPostgresNodeAgentCount(t, ctx, pool, node.ID, 1)

	readonlyReplacement := sendOperatorJSON(t, handler, nethttp.MethodPost, "/api/v1/nodes/"+node.ID+"/enrollment-token?ttl_hours=1", fixture.readonlyToken, true, nil, nil)
	if readonlyReplacement.Code != nethttp.StatusForbidden {
		t.Fatalf("readonly replacement token status = %d, want 403", readonlyReplacement.Code)
	}
	missingCSRFReplacement := sendOperatorJSON(t, handler, nethttp.MethodPost, "/api/v1/nodes/"+node.ID+"/enrollment-token?ttl_hours=1", fixture.adminToken, false, nil, nil)
	if missingCSRFReplacement.Code != nethttp.StatusForbidden {
		t.Fatalf("missing csrf replacement token status = %d, want 403", missingCSRFReplacement.Code)
	}

	replacement := createAgentOnboardingEnrollmentTokenHTTP(t, handler, node.ID, fixture.adminToken)
	recovered := registerAgentOnboardingHTTP(t, handler, node.ID, replacement.Token, "recovered-agent", "192.0.2.241", agentRecoveredVersion)
	if recovered.AgentToken == "" || recovered.AgentToken == replacement.Token {
		t.Fatal("recovered registration must return a new distinct agent token")
	}
	secondState := assertHTTPPostgresSingleNodeAgentIdentity(t, ctx, pool, node.ID, recovered.AgentToken)
	if secondState.TokenHash == firstHash {
		t.Fatal("recovered registration must replace the stored token hash")
	}
	assertHTTPPostgresCount(t, ctx, pool, `select count(*) from node_agents where node_id=$1 and agent_token_hash=$2 and status='active' and revoked_at is null`, 0, node.ID, firstHash)
	if secondState.AgentVersion != agentRecoveredVersion || secondState.ProtocolVersion != agentProtocolVersion {
		t.Fatalf("recovered agent version/protocol = %q/%q", secondState.AgentVersion, secondState.ProtocolVersion)
	}
	assertHTTPPostgresEnrollmentTokenStatus(t, ctx, pool, replacement.ID, "used")
	assertHTTPPostgresAgentRegisterAudit(t, ctx, pool, node.ID, 2, firstEnrollment.Token, replacement.Token, recovered.AgentToken)
	assertHTTPPostgresDatabaseTextExcludes(t, ctx, pool, "recovered registration storage", node.ID, []string{firstEnrollment.Token, replacement.Token, recovered.AgentToken})

	oldHashHeartbeat := sendSignedAgentJSONWithBearer(t, handler, nethttp.MethodPost, "/agent/heartbeat", firstHash, firstHash, map[string]any{
		"node_id": node.ID, "name": node.Name, "agent_version": agentRecoveredVersion, "protocol_version": agentProtocolVersion,
	}, nil)
	if oldHashHeartbeat.rec.Code != nethttp.StatusUnauthorized {
		t.Fatalf("stored hash as token status = %d, want 401", oldHashHeartbeat.rec.Code)
	}

	heartbeatRec := sendSignedAgentJSON(t, handler, nethttp.MethodPost, "/agent/heartbeat", recovered.AgentToken, map[string]any{
		"node_id": node.ID, "name": node.Name, "agent_version": agentRecoveredVersion, "protocol_version": agentProtocolVersion,
	}, nil)
	if heartbeatRec.rec.Code != nethttp.StatusOK {
		t.Fatalf("recovered heartbeat status = %d, want 200", heartbeatRec.rec.Code)
	}
	verifySignedAgentResponse(t, heartbeatRec.req, heartbeatRec.rec, recovered.AgentToken)

	var job domain.Job
	jobRec := sendOperatorJSON(t, handler, nethttp.MethodPost, "/api/v1/nodes/"+node.ID+"/inventory/sync", fixture.adminToken, true, nil, &job)
	if jobRec.Code != nethttp.StatusAccepted {
		t.Fatalf("recovered inventory job status = %d, want 202", jobRec.Code)
	}
	var claimed domain.Job
	nextRec := sendSignedAgentJSON(t, handler, nethttp.MethodGet, "/agent/jobs/next?node_id="+node.ID, recovered.AgentToken, nil, &claimed)
	if nextRec.rec.Code != nethttp.StatusOK {
		t.Fatalf("recovered job poll status = %d, want 200", nextRec.rec.Code)
	}
	verifySignedAgentResponse(t, nextRec.req, nextRec.rec, recovered.AgentToken)
	if claimed.ID != job.ID {
		t.Fatalf("recovered claimed job id = %s, want %s", claimed.ID, job.ID)
	}

	inventoryPayload := syntheticAgentOnboardingInventoryPayload()
	var inventoryResp struct {
		Status   string                       `json:"status"`
		Snapshot domain.NodeInventorySnapshot `json:"snapshot"`
	}
	inventoryRec := sendSignedAgentJSON(t, handler, nethttp.MethodPost, "/agent/inventory", recovered.AgentToken, map[string]any{
		"node_id": node.ID, "source": "inventory", "inventory": inventoryPayload,
	}, &inventoryResp)
	if inventoryRec.rec.Code != nethttp.StatusOK {
		t.Fatalf("recovered inventory status = %d, want 200", inventoryRec.rec.Code)
	}
	verifySignedAgentResponse(t, inventoryRec.req, inventoryRec.rec, recovered.AgentToken)
	resultRec := sendSignedAgentJSON(t, handler, nethttp.MethodPost, "/agent/jobs/"+job.ID+"/result", recovered.AgentToken, map[string]any{
		"status": "succeeded",
		"result": map[string]any{"message": "recovered synthetic inventory collected", "snapshot_id": inventoryResp.Snapshot.ID},
	}, nil)
	if resultRec.rec.Code != nethttp.StatusOK {
		t.Fatalf("recovered job result status = %d, want 200", resultRec.rec.Code)
	}
	verifySignedAgentResponse(t, resultRec.req, resultRec.rec, recovered.AgentToken)
	assertHTTPPostgresJobSucceeded(t, ctx, pool, job.ID, recovered.AgentToken, replacement.Token)

	var diag domain.NodeDiagnostics
	diagRec := sendOperatorJSON(t, handler, nethttp.MethodGet, "/api/v1/nodes/"+node.ID+"/diagnostics", fixture.adminToken, false, nil, &diag)
	if diagRec.Code != nethttp.StatusOK {
		t.Fatalf("recovered diagnostics status = %d, want 200", diagRec.Code)
	}
	assertAgentOnboardingDiagnostics(t, diag, node.ID, job.ID, agentRecoveredVersion, "inventory_ok")
	assertHTTPPostgresPayloadNotContains(t, "recovered diagnostics response", diagRec.Body.Bytes(), []string{
		firstEnrollment.Token,
		replacement.Token,
		recovered.AgentToken,
		"token_hash",
		"agent_token",
		agentauth.HeaderSignature,
		agentauth.HeaderNonce,
	})
	assertHTTPPostgresAuditAndLogsRedacted(t, ctx, pool, logs.Bytes(), node.ID, []string{firstEnrollment.Token, replacement.Token, recovered.AgentToken})
	assertHTTPPostgresDatabaseTextExcludes(t, ctx, pool, "recovered final storage", node.ID, []string{firstEnrollment.Token, replacement.Token, recovered.AgentToken})
}

type agentOnboardingHTTPFixture struct {
	adminToken    string
	readonlyToken string
}

type agentOnboardingRegisterResult struct {
	Status string `json:"status"`
	Node   struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Address string `json:"address"`
	} `json:"node"`
	AgentToken string `json:"agent_token"`
	TokenHint  string `json:"token_hint"`
	rec        *httptest.ResponseRecorder
}

type signedAgentHTTPResult struct {
	req *nethttp.Request
	rec *httptest.ResponseRecorder
}

type httpPostgresNodeAgentState struct {
	TokenHash           string
	TokenHint           string
	Status              string
	AgentVersion        string
	ProtocolVersion     string
	RegisteredAt        *time.Time
	LastSeenAt          *time.Time
	LastInventorySyncAt *time.Time
	LastDiscoverySyncAt *time.Time
	LastJobPollAt       *time.Time
	LastJobClaimAt      *time.Time
	LastJobClaimJobID   *string
	LastJobResultAt     *time.Time
	LastJobResultJobID  *string
	LastJobResultStatus string
	LastAuthFailureAt   *time.Time
}

func newAgentOnboardingHTTPFixture(t *testing.T, ctx context.Context, store *pgstore.Store) agentOnboardingHTTPFixture {
	t.Helper()

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	admin, _, err := store.EnsureBootstrapPlatformUser(ctx, "it-onboarding-admin-"+suffix, "it-onboarding-admin-"+suffix+"@example.invalid", "Integration Onboarding Admin", "integration-password-hash")
	if err != nil {
		t.Fatalf("create bootstrap admin: %v", err)
	}
	readonly, err := store.CreatePlatformUser(ctx, "it-onboarding-readonly-"+suffix, "it-onboarding-readonly-"+suffix+"@example.invalid", "Integration Onboarding Readonly", "integration-password-hash", []string{"readonly"}, &admin.ID)
	if err != nil {
		t.Fatalf("create readonly user: %v", err)
	}
	return agentOnboardingHTTPFixture{
		adminToken:    createHTTPPostgresIntegrationSession(t, ctx, store, admin.ID),
		readonlyToken: createHTTPPostgresIntegrationSession(t, ctx, store, readonly.ID),
	}
}

func newHTTPPostgresAgentOnboardingHandler(store *pgstore.Store) (nethttp.Handler, *bytes.Buffer) {
	logs := bytes.NewBuffer(nil)
	return New(slog.New(slog.NewTextHandler(logs, nil)), store, Options{
		Version:               "test",
		SessionCookieName:     "megavpn_session",
		AgentSignatureEnforce: true,
		AgentSignatureWindow:  time.Minute,
	}), logs
}

func agentOnboardingNodePayload(label string) domain.Node {
	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	return domain.Node{
		Name:          "it-agent-onboarding-" + label + "-" + suffix,
		Kind:          "remote",
		Role:          "ingress",
		Status:        "maintenance",
		Address:       "203.0.113.20",
		LocationLabel: "lab-rack-" + label,
		OSFamily:      "linux",
		OSVersion:     "ubuntu-24.04-operator",
		Architecture:  "amd64",
		ExecutionMode: "manual_bundle",
		AgentStatus:   "unknown",
	}
}

func createAgentOnboardingNodeViaHTTP(t *testing.T, handler nethttp.Handler, sessionToken, label string) domain.Node {
	t.Helper()

	var node domain.Node
	rec := sendOperatorJSON(t, handler, nethttp.MethodPost, "/api/v1/nodes", sessionToken, true, agentOnboardingNodePayload(label), &node)
	if rec.Code != nethttp.StatusCreated {
		t.Fatalf("node create status = %d, want 201", rec.Code)
	}
	if node.ID == "" || node.Name == "" || node.Address == "" || node.LocationLabel == "" {
		t.Fatalf("created node projection missing required profile fields: %#v", node)
	}
	return node
}

func createAgentOnboardingEnrollmentTokenHTTP(t *testing.T, handler nethttp.Handler, nodeID, sessionToken string) domain.NodeEnrollmentToken {
	t.Helper()

	var token domain.NodeEnrollmentToken
	rec := sendOperatorJSON(t, handler, nethttp.MethodPost, "/api/v1/nodes/"+nodeID+"/enrollment-token?ttl_hours=1", sessionToken, true, nil, &token)
	if rec.Code != nethttp.StatusCreated {
		t.Fatalf("enrollment token create status = %d, want 201", rec.Code)
	}
	if token.ID == "" || token.NodeID != nodeID || token.Token == "" || token.TokenHint == "" || token.Status != "active" {
		t.Fatalf("enrollment token projection mismatch: id_present=%t node_match=%t token_present=%t hint_present=%t status=%q",
			token.ID != "", token.NodeID == nodeID, token.Token != "", token.TokenHint != "", token.Status)
	}
	return token
}

func registerAgentOnboardingHTTP(t *testing.T, handler nethttp.Handler, nodeID, enrollmentToken, name, address, version string) agentOnboardingRegisterResult {
	t.Helper()

	rec := registerAgentOnboardingHTTPRaw(t, handler, nodeID, enrollmentToken, name, address, version)
	if rec.Code != nethttp.StatusOK {
		t.Fatalf("agent register status = %d, want 200", rec.Code)
	}
	var out agentOnboardingRegisterResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode registration response: %v", err)
	}
	out.rec = rec
	return out
}

func registerAgentOnboardingHTTPRaw(t *testing.T, handler nethttp.Handler, nodeID, enrollmentToken, name, address, version string) *httptest.ResponseRecorder {
	t.Helper()

	body, err := json.Marshal(map[string]any{
		"node_id":          nodeID,
		"name":             name,
		"address":          address,
		"enrollment_token": enrollmentToken,
		"agent_version":    version,
		"protocol_version": agentProtocolVersion,
	})
	if err != nil {
		t.Fatalf("marshal registration request: %v", err)
	}
	req := httptest.NewRequest(nethttp.MethodPost, "/agent/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func sendOperatorJSON(t *testing.T, handler nethttp.Handler, method, path, sessionToken string, csrf bool, payload any, out any) *httptest.ResponseRecorder {
	t.Helper()

	var body []byte
	var reader io.Reader = nethttp.NoBody
	if payload != nil {
		var err error
		body, err = json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal operator payload: %v", err)
		}
		reader = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if strings.TrimSpace(sessionToken) != "" {
		req.AddCookie(&nethttp.Cookie{Name: "megavpn_session", Value: sessionToken})
	}
	if csrf {
		req.Header.Set("X-MegaVPN-CSRF", "1")
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if out != nil && rec.Code >= 200 && rec.Code < 300 {
		if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
			t.Fatalf("decode operator response status=%d: %v", rec.Code, err)
		}
	}
	return rec
}

func sendSignedAgentJSON(t *testing.T, handler nethttp.Handler, method, path, token string, payload any, out any) signedAgentHTTPResult {
	t.Helper()
	return sendSignedAgentJSONWithBearer(t, handler, method, path, token, token, payload, out)
}

func sendSignedAgentJSONWithBearer(t *testing.T, handler nethttp.Handler, method, path, bearer, signingToken string, payload any, out any) signedAgentHTTPResult {
	t.Helper()

	body := mustMarshalOptionalJSON(t, payload)
	req := newSignedAgentRequest(t, method, path, bearer, signingToken, body, time.Now().UTC(), agentauth.NewNonce())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if out != nil && rec.Code >= 200 && rec.Code < 300 {
		if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
			t.Fatalf("decode signed agent response status=%d: %v", rec.Code, err)
		}
	}
	return signedAgentHTTPResult{req: req, rec: rec}
}

func newSignedAgentRequest(t *testing.T, method, path, bearer, signingToken string, body []byte, at time.Time, nonce string) *nethttp.Request {
	t.Helper()

	var reader io.Reader = nethttp.NoBody
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if strings.TrimSpace(bearer) != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	signAgentRequestAt(t, req, signingToken, body, at, nonce)
	return req
}

func signAgentRequestAt(t *testing.T, req *nethttp.Request, token string, body []byte, at time.Time, nonce string) {
	t.Helper()

	if at.IsZero() {
		at = time.Now().UTC()
	}
	if nonce == "" {
		nonce = agentauth.NewNonce()
	}
	timestamp := strconv.FormatInt(at.UTC().Unix(), 10)
	signature, bodyHash := agentauth.Sign(token, req.Method, req.URL.RequestURI(), timestamp, nonce, body)
	req.Header.Set(agentauth.HeaderTimestamp, timestamp)
	req.Header.Set(agentauth.HeaderNonce, nonce)
	req.Header.Set(agentauth.HeaderBodyHash, bodyHash)
	req.Header.Set(agentauth.HeaderSignature, signature)
}

func mustMarshalOptionalJSON(t *testing.T, payload any) []byte {
	t.Helper()

	if payload == nil {
		return nil
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal agent payload: %v", err)
	}
	return body
}

func verifySignedAgentResponse(t *testing.T, req *nethttp.Request, rec *httptest.ResponseRecorder, token string) {
	t.Helper()

	err := agentauth.Verify(
		token,
		"RESPONSE",
		req.URL.RequestURI(),
		rec.Header().Get(agentauth.HeaderTimestamp),
		rec.Header().Get(agentauth.HeaderNonce),
		rec.Header().Get(agentauth.HeaderBodyHash),
		rec.Header().Get(agentauth.HeaderSignature),
		rec.Body.Bytes(),
		time.Now().UTC(),
		time.Minute,
	)
	if err != nil {
		t.Fatalf("verify signed agent response: %v", err)
	}
}

func assertAgentSignatureBoundaryDoesNotMutate(t *testing.T, handler nethttp.Handler, ctx context.Context, pool *pgxpool.Pool, nodeID, agentToken, crossNodeToken string, heartbeatPayload map[string]any, heartbeatBefore, seenBefore *time.Time, inventoryBefore int) {
	t.Helper()

	body := mustMarshalOptionalJSON(t, heartbeatPayload)
	cases := []struct {
		name string
		req  *nethttp.Request
	}{
		{
			name: "missing bearer token",
			req: func() *nethttp.Request {
				req := newSignedAgentRequest(t, nethttp.MethodPost, "/agent/heartbeat", "", agentToken, body, time.Now().UTC(), agentauth.NewNonce())
				return req
			}(),
		},
		{
			name: "wrong agent token",
			req:  newSignedAgentRequest(t, nethttp.MethodPost, "/agent/heartbeat", "wrong-agent-token", "wrong-agent-token", body, time.Now().UTC(), agentauth.NewNonce()),
		},
		{
			name: "cross node token",
			req:  newSignedAgentRequest(t, nethttp.MethodPost, "/agent/heartbeat", crossNodeToken, crossNodeToken, body, time.Now().UTC(), agentauth.NewNonce()),
		},
		{
			name: "missing signature headers",
			req: func() *nethttp.Request {
				req := httptest.NewRequest(nethttp.MethodPost, "/agent/heartbeat", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer "+agentToken)
				return req
			}(),
		},
		{
			name: "invalid body hash",
			req: func() *nethttp.Request {
				req := newSignedAgentRequest(t, nethttp.MethodPost, "/agent/heartbeat", agentToken, agentToken, body, time.Now().UTC(), agentauth.NewNonce())
				req.Header.Set(agentauth.HeaderBodyHash, agentauth.BodyHash([]byte("modified")))
				return req
			}(),
		},
		{
			name: "invalid signature",
			req: func() *nethttp.Request {
				req := newSignedAgentRequest(t, nethttp.MethodPost, "/agent/heartbeat", agentToken, agentToken, body, time.Now().UTC(), agentauth.NewNonce())
				req.Header.Set(agentauth.HeaderSignature, "v1=0000000000000000000000000000000000000000000000000000000000000000")
				return req
			}(),
		},
		{
			name: "stale timestamp",
			req:  newSignedAgentRequest(t, nethttp.MethodPost, "/agent/heartbeat", agentToken, agentToken, body, time.Now().UTC().Add(-10*time.Minute), agentauth.NewNonce()),
		},
		{
			name: "body modified after signing",
			req: func() *nethttp.Request {
				req := newSignedAgentRequest(t, nethttp.MethodPost, "/agent/heartbeat", agentToken, agentToken, body, time.Now().UTC(), agentauth.NewNonce())
				req.Body = io.NopCloser(bytes.NewReader([]byte(`{"node_id":"` + nodeID + `","agent_version":"tampered","protocol_version":"v1"}`)))
				return req
			}(),
		},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, tc.req)
		if rec.Code != nethttp.StatusUnauthorized {
			t.Fatalf("%s status = %d, want 401", tc.name, rec.Code)
		}
		assertHTTPPostgresPayloadNotContains(t, tc.name+" response", rec.Body.Bytes(), []string{agentToken, crossNodeToken, "token_hash", "signature", "nonce"})
		assertHTTPPostgresNodeHeartbeatEquals(t, ctx, pool, nodeID, heartbeatBefore)
		state := assertHTTPPostgresSingleNodeAgentIdentity(t, ctx, pool, nodeID, agentToken)
		if !timePtrEqual(state.LastSeenAt, seenBefore) {
			t.Fatalf("%s mutated last_seen_at", tc.name)
		}
		assertHTTPPostgresInventorySnapshotCount(t, ctx, pool, nodeID, inventoryBefore)
	}

	replayReq := newSignedAgentRequest(t, nethttp.MethodPost, "/agent/heartbeat", agentToken, agentToken, body, time.Now().UTC(), agentauth.NewNonce())
	first := httptest.NewRecorder()
	handler.ServeHTTP(first, replayReq)
	if first.Code != nethttp.StatusOK {
		t.Fatalf("initial replay fixture heartbeat status = %d, want 200", first.Code)
	}
	second := httptest.NewRecorder()
	handler.ServeHTTP(second, replayReq)
	if second.Code != nethttp.StatusUnauthorized {
		t.Fatalf("replayed heartbeat status = %d, want 401", second.Code)
	}
	assertHTTPPostgresInventorySnapshotCount(t, ctx, pool, nodeID, inventoryBefore)
}

func syntheticAgentOnboardingInventoryPayload() map[string]any {
	return map[string]any{
		"hostname":     "it-agent-onboarding-node",
		"arch":         "amd64",
		"kernel":       "6.6.0-megavpn-test",
		"collected_at": "2026-07-15T00:00:00Z",
		"os": map[string]any{
			"family":      "linux",
			"version":     "1.0-test",
			"pretty_name": "MegaVPN Synthetic Test Linux 1.0",
		},
		"interfaces": []any{
			map[string]any{
				"name":       "eth-test0",
				"mac":        "02:00:5e:00:53:01",
				"addrs":      []any{"192.0.2.10/24", "2001:db8::10/64"},
				"mtu":        1500,
				"admin_up":   true,
				"oper_state": "up",
			},
		},
		"ports": []any{
			map[string]any{"protocol": "tcp", "local_address": "192.0.2.10:443", "state": "listen"},
		},
		"binaries": map[string]any{
			"systemctl": "255-test",
			"nginx":     "1.26.0-test",
			"xray":      "24.9.30-test",
		},
		"services": map[string]any{
			"nginx": map[string]any{
				"active_state":  "active",
				"enabled_state": "enabled",
				"unit":          "nginx.service",
			},
			"xray": map[string]any{
				"active_state":  "active",
				"enabled_state": "enabled",
				"unit":          "xray.service",
			},
		},
		"config_files": map[string]any{
			"/etc/nginx/nginx.conf": map[string]any{"present": true, "mode": "0644"},
			"/etc/xray/config.json": map[string]any{"present": true, "mode": "0644"},
		},
		"capabilities": map[string]any{
			"test_only": true,
		},
		"packages": []any{
			map[string]any{"name": "megavpn-test-package", "version": "1.0.0-test"},
		},
	}
}

func assertHTTPPostgresEnrollmentTokenStatus(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tokenID, want string) {
	t.Helper()

	var got string
	var used bool
	if err := pool.QueryRow(ctx, `select status, used_at is not null from node_enrollment_tokens where id=$1`, tokenID).Scan(&got, &used); err != nil {
		t.Fatalf("query enrollment token status: %v", err)
	}
	if got != want {
		t.Fatalf("enrollment token status = %q, want %q", got, want)
	}
	if want == "used" && !used {
		t.Fatalf("enrollment token status is used but used_at is null")
	}
}

func assertHTTPPostgresActiveEnrollmentTokenCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, nodeID string, want int) {
	t.Helper()

	assertHTTPPostgresCount(t, ctx, pool, `select count(*) from node_enrollment_tokens where node_id=$1 and status='active'`, want, nodeID)
}

func assertHTTPPostgresNodeAgentCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, nodeID string, want int) {
	t.Helper()

	assertHTTPPostgresCount(t, ctx, pool, `select count(*) from node_agents where node_id=$1`, want, nodeID)
}

func assertHTTPPostgresSingleNodeAgentIdentity(t *testing.T, ctx context.Context, pool *pgxpool.Pool, nodeID, plaintextToken string) httpPostgresNodeAgentState {
	t.Helper()

	state := assertHTTPPostgresSingleNodeAgentState(t, ctx, pool, nodeID)
	if state.TokenHash == "" || state.TokenHash == plaintextToken {
		t.Fatalf("stored agent token hash is empty or plaintext")
	}
	if state.TokenHint == "" || state.TokenHint == plaintextToken {
		t.Fatalf("stored token hint is empty or plaintext")
	}
	var valid bool
	if err := pool.QueryRow(ctx, `select exists(select 1 from node_agents where node_id=$1 and status='active' and revoked_at is null and agent_token_hash=encode(digest($2,'sha256'),'hex'))`, nodeID, plaintextToken).Scan(&valid); err != nil {
		t.Fatalf("validate token hash from postgres: %v", err)
	}
	if !valid {
		t.Fatalf("stored token hash does not validate generated agent token")
	}
	return state
}

func assertHTTPPostgresSingleNodeAgentIdentityWithoutPlaintext(t *testing.T, ctx context.Context, pool *pgxpool.Pool, nodeID string, forbidden ...string) string {
	t.Helper()

	state := assertHTTPPostgresSingleNodeAgentState(t, ctx, pool, nodeID)
	if state.TokenHash == "" {
		t.Fatal("stored agent token hash is empty")
	}
	text := assertHTTPPostgresNodeAgentsText(t, ctx, pool, nodeID)
	assertHTTPPostgresPayloadNotContains(t, "node agent identity row", []byte(text), forbidden)
	return state.TokenHash
}

func assertHTTPPostgresSingleNodeAgentState(t *testing.T, ctx context.Context, pool *pgxpool.Pool, nodeID string) httpPostgresNodeAgentState {
	t.Helper()

	var count int
	var state httpPostgresNodeAgentState
	if err := pool.QueryRow(ctx, `select count(*) from node_agents where node_id=$1`, nodeID).Scan(&count); err != nil {
		t.Fatalf("count node agent identities: %v", err)
	}
	if count != 1 {
		t.Fatalf("node agent identity count = %d, want 1", count)
	}
	if err := pool.QueryRow(ctx, `select
		coalesce(agent_token_hash,''),
		coalesce(token_hint,''),
		status,
		coalesce(agent_version,''),
		coalesce(protocol_version,''),
		registered_at,
		last_seen_at,
		last_inventory_sync_at,
		last_discovery_sync_at,
		last_job_poll_at,
		last_job_claim_at,
		last_job_claim_job_id,
		last_job_result_at,
		last_job_result_job_id,
		coalesce(last_job_result_status,''),
		last_auth_failure_at
	from node_agents where node_id=$1`, nodeID).Scan(
		&state.TokenHash,
		&state.TokenHint,
		&state.Status,
		&state.AgentVersion,
		&state.ProtocolVersion,
		&state.RegisteredAt,
		&state.LastSeenAt,
		&state.LastInventorySyncAt,
		&state.LastDiscoverySyncAt,
		&state.LastJobPollAt,
		&state.LastJobClaimAt,
		&state.LastJobClaimJobID,
		&state.LastJobResultAt,
		&state.LastJobResultJobID,
		&state.LastJobResultStatus,
		&state.LastAuthFailureAt,
	); err != nil {
		t.Fatalf("query node agent state: %v", err)
	}
	if state.Status != "active" {
		t.Fatalf("node agent status = %q, want active", state.Status)
	}
	return state
}

func assertHTTPPostgresNodeAgentsText(t *testing.T, ctx context.Context, pool *pgxpool.Pool, nodeID string) string {
	t.Helper()

	var text string
	if err := pool.QueryRow(ctx, `select coalesce(string_agg(row_to_json(na)::text, E'\n'), '') from node_agents na where node_id=$1`, nodeID).Scan(&text); err != nil {
		t.Fatalf("load node_agents text: %v", err)
	}
	return text
}

func assertHTTPPostgresAgentRegisterAudit(t *testing.T, ctx context.Context, pool *pgxpool.Pool, nodeID string, want int, forbidden ...string) {
	t.Helper()

	var count int
	var text string
	if err := pool.QueryRow(ctx, `select count(*), coalesce(string_agg(action || ' ' || actor_type || ' ' || summary || ' ' || payload_json::text, E'\n'), '') from audit_events where action='agent.register' and resource_id=$1`, nodeID).Scan(&count, &text); err != nil {
		t.Fatalf("query agent.register audit: %v", err)
	}
	if count != want {
		t.Fatalf("agent.register audit count = %d, want %d", count, want)
	}
	if want > 0 && (!strings.Contains(text, "agent.register") || !strings.Contains(text, "agent")) {
		t.Fatalf("agent.register audit metadata missing expected actor/action")
	}
	assertHTTPPostgresPayloadNotContains(t, "agent.register audit", []byte(text), append(forbidden, "agent_token", "enrollment_token", "token_hash", "Authorization", agentauth.HeaderSignature))
}

func assertHTTPPostgresAuditAndLogsRedacted(t *testing.T, ctx context.Context, pool *pgxpool.Pool, logs []byte, nodeID string, forbidden []string) {
	t.Helper()

	var auditText string
	if err := pool.QueryRow(ctx, `select coalesce(string_agg(action || ' ' || actor_type || ' ' || summary || ' ' || payload_json::text, E'\n'), '') from audit_events where resource_id=$1`, nodeID).Scan(&auditText); err != nil {
		t.Fatalf("query audit redaction text: %v", err)
	}
	common := append([]string{}, forbidden...)
	common = append(common, "Authorization: Bearer", agentauth.HeaderSignature, "request_signature", "token_hash")
	assertHTTPPostgresPayloadNotContains(t, "audit redaction", []byte(auditText), common)
	assertHTTPPostgresPayloadNotContains(t, "http logs", logs, common)
}

func assertHTTPPostgresDatabaseTextExcludes(t *testing.T, ctx context.Context, pool *pgxpool.Pool, label, nodeID string, forbidden []string) {
	t.Helper()

	queries := []struct {
		label string
		sql   string
	}{
		{"node_agents", `select coalesce(string_agg(row_to_json(x)::text, E'\n'), '') from node_agents x where node_id=$1`},
		{"node_enrollment_tokens", `select coalesce(string_agg(row_to_json(x)::text, E'\n'), '') from node_enrollment_tokens x where node_id=$1`},
		{"node_inventory_snapshots", `select coalesce(string_agg(row_to_json(x)::text, E'\n'), '') from node_inventory_snapshots x where node_id=$1`},
		{"audit_events", `select coalesce(string_agg(row_to_json(x)::text, E'\n'), '') from audit_events x where resource_id=$1`},
		{"jobs", `select coalesce(string_agg(row_to_json(x)::text, E'\n'), '') from jobs x where node_id=$1`},
		{"job_logs", `select coalesce(string_agg(row_to_json(x)::text, E'\n'), '') from job_logs x join jobs j on j.id=x.job_id where j.node_id=$1`},
	}
	for _, query := range queries {
		var text string
		if err := pool.QueryRow(ctx, query.sql, nodeID).Scan(&text); err != nil {
			t.Fatalf("query %s text for %s: %v", query.label, label, err)
		}
		assertHTTPPostgresPayloadNotContains(t, label+" "+query.label, []byte(text), forbidden)
	}
}

func assertHTTPPostgresNodeProfilePreserved(t *testing.T, ctx context.Context, store *pgstore.Store, before domain.Node) {
	t.Helper()

	after, err := store.GetNode(ctx, before.ID)
	if err != nil {
		t.Fatalf("get node after agent operation: %v", err)
	}
	if after.Name != before.Name || after.Address != before.Address || after.Role != before.Role || after.LocationLabel != before.LocationLabel || after.OSVersion != before.OSVersion || after.Architecture != before.Architecture {
		t.Fatalf("operator-owned node profile changed: before=%#v after=%#v", before, after)
	}
	if after.Kind != "remote" || after.ExecutionMode != "agent_managed" || after.AgentStatus != "online" {
		t.Fatalf("agent lifecycle fields mismatch after registration: kind=%q execution=%q agent_status=%q", after.Kind, after.ExecutionMode, after.AgentStatus)
	}
}

func assertHTTPPostgresNodeHeartbeatSet(t *testing.T, ctx context.Context, pool *pgxpool.Pool, nodeID string) *time.Time {
	t.Helper()

	var heartbeat *time.Time
	if err := pool.QueryRow(ctx, `select last_heartbeat_at from nodes where id=$1`, nodeID).Scan(&heartbeat); err != nil {
		t.Fatalf("query node heartbeat: %v", err)
	}
	if heartbeat == nil {
		t.Fatal("last_heartbeat_at is null after signed heartbeat")
	}
	return heartbeat
}

func assertHTTPPostgresNodeHeartbeatEquals(t *testing.T, ctx context.Context, pool *pgxpool.Pool, nodeID string, want *time.Time) {
	t.Helper()

	var got *time.Time
	if err := pool.QueryRow(ctx, `select last_heartbeat_at from nodes where id=$1`, nodeID).Scan(&got); err != nil {
		t.Fatalf("query node heartbeat: %v", err)
	}
	if !timePtrEqual(got, want) {
		t.Fatalf("last_heartbeat_at changed: got=%v want=%v", got, want)
	}
}

func countHTTPPostgresNodeInventorySnapshots(t *testing.T, ctx context.Context, pool *pgxpool.Pool, nodeID string) int {
	t.Helper()

	var count int
	if err := pool.QueryRow(ctx, `select count(*) from node_inventory_snapshots where node_id=$1`, nodeID).Scan(&count); err != nil {
		t.Fatalf("count inventory snapshots: %v", err)
	}
	return count
}

func assertHTTPPostgresInventorySnapshotCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, nodeID string, want int) {
	t.Helper()

	if got := countHTTPPostgresNodeInventorySnapshots(t, ctx, pool, nodeID); got != want {
		t.Fatalf("inventory snapshot count = %d, want %d", got, want)
	}
}

func assertHTTPPostgresInventoryPersisted(t *testing.T, ctx context.Context, pool *pgxpool.Pool, nodeID string, want map[string]any) {
	t.Helper()

	var payloadRaw []byte
	if err := pool.QueryRow(ctx, `select payload_json from node_inventory_snapshots where node_id=$1 order by created_at desc limit 1`, nodeID).Scan(&payloadRaw); err != nil {
		t.Fatalf("query latest inventory: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(payloadRaw, &payload); err != nil {
		t.Fatalf("decode latest inventory payload: %v", err)
	}
	if stringifyHTTP(payload["hostname"]) != stringifyHTTP(want["hostname"]) || stringifyHTTP(payload["collected_at"]) != stringifyHTTP(want["collected_at"]) {
		t.Fatalf("persisted inventory payload mismatch: %#v", payload)
	}
	state := assertHTTPPostgresSingleNodeAgentState(t, ctx, pool, nodeID)
	if state.LastInventorySyncAt == nil {
		t.Fatal("last_inventory_sync_at is null after source=inventory submission")
	}
	if state.LastDiscoverySyncAt != nil {
		t.Fatalf("last_discovery_sync_at = %v, want nil for source=inventory", state.LastDiscoverySyncAt)
	}
	assertHTTPPostgresCount(t, ctx, pool, `select count(*) from node_capabilities where node_id=$1 and source='inventory'`, 8, nodeID)
}

func assertHTTPPostgresJobSucceeded(t *testing.T, ctx context.Context, pool *pgxpool.Pool, jobID string, forbidden ...string) {
	t.Helper()

	var status string
	var finished bool
	var resultText string
	if err := pool.QueryRow(ctx, `select status, finished_at is not null, coalesce(result_json::text, '{}') from jobs where id=$1`, jobID).Scan(&status, &finished, &resultText); err != nil {
		t.Fatalf("query completed job: %v", err)
	}
	if status != "succeeded" || !finished {
		t.Fatalf("job completion state = status:%q finished:%t, want succeeded/finished", status, finished)
	}
	assertHTTPPostgresPayloadNotContains(t, "job result", []byte(resultText), append(forbidden, "agent_token", "enrollment_token", "token_hash"))
}

func assertAgentOnboardingDiagnostics(t *testing.T, diag domain.NodeDiagnostics, nodeID, jobID, agentVersion, wantCommunication string) {
	t.Helper()

	if diag.Node.ID != nodeID {
		t.Fatalf("diagnostics node id = %s, want %s", diag.Node.ID, nodeID)
	}
	if diag.HeartbeatState != "online" || diag.HeartbeatDriftSeconds == nil || *diag.HeartbeatDriftSeconds > 30 {
		t.Fatalf("diagnostics heartbeat = state:%q drift:%v, want online/current", diag.HeartbeatState, diag.HeartbeatDriftSeconds)
	}
	if diag.Agent.NodeID != nodeID || diag.Agent.Status != "active" || diag.Agent.RegisteredAt == nil || diag.Agent.LastSeenAt == nil {
		t.Fatalf("diagnostics agent state mismatch: %#v", diag.Agent)
	}
	if diag.Agent.AgentVersion != agentVersion || diag.Agent.ProtocolVersion != agentProtocolVersion {
		t.Fatalf("diagnostics agent version/protocol = %q/%q, want %q/%q", diag.Agent.AgentVersion, diag.Agent.ProtocolVersion, agentVersion, agentProtocolVersion)
	}
	if diag.LatestInventory == nil || diag.LatestInventory.NodeID != nodeID || stringifyHTTP(diag.LatestInventory.Payload["hostname"]) == "" {
		t.Fatalf("diagnostics latest inventory missing or wrong: %#v", diag.LatestInventory)
	}
	if diag.Agent.LastInventorySyncAt == nil || diag.Agent.LastJobPollAt == nil || diag.Agent.LastJobClaimAt == nil || diag.Agent.LastJobResultAt == nil {
		t.Fatalf("diagnostics missing agent sync timestamps: %#v", diag.Agent)
	}
	if diag.Agent.LastJobClaimJobID == nil || *diag.Agent.LastJobClaimJobID != jobID || diag.Agent.LastJobResultJobID == nil || *diag.Agent.LastJobResultJobID != jobID || diag.Agent.LastJobResultStatus != "succeeded" {
		t.Fatalf("diagnostics job projection mismatch: %#v", diag.Agent)
	}
	if diag.CommunicationState != wantCommunication {
		t.Fatalf("communication state = %q, want %q; hint=%q", diag.CommunicationState, wantCommunication, diag.CommunicationHint)
	}
	if diag.LatestEnrollmentToken == nil || diag.LatestEnrollmentToken.Status != "used" || diag.LatestEnrollmentToken.Token != "" {
		status := "<nil>"
		tokenExposed := false
		if diag.LatestEnrollmentToken != nil {
			status = diag.LatestEnrollmentToken.Status
			tokenExposed = diag.LatestEnrollmentToken.Token != ""
		}
		t.Fatalf("latest enrollment token projection mismatch: status=%q token_exposed=%t", status, tokenExposed)
	}
	if diag.ActiveEnrollmentToken != nil && diag.ActiveEnrollmentToken.Token != "" {
		t.Fatal("active enrollment token exposed plaintext")
	}
}

func assertHTTPPostgresNoStoreHeader(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()

	if !strings.Contains(strings.ToLower(rec.Header().Get("Cache-Control")), "no-store") {
		t.Fatalf("Cache-Control = %q, want no-store", rec.Header().Get("Cache-Control"))
	}
}

func timePtrEqual(a, b *time.Time) bool {
	switch {
	case a == nil && b == nil:
		return true
	case a == nil || b == nil:
		return false
	default:
		return a.Equal(*b)
	}
}
