package http

import (
	"bytes"
	"encoding/json"
	nethttp "net/http"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/rtis-emc2/megavpn/internal/agentauth"
	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/platform/id"
)

const (
	staleRotationClearReason  = "Disposable stale rotation recovery verification"
	agentIdentityRevokeReason = "Disposable identity revocation after stale rotation recovery"
)

type lifecycleTrustRotation struct {
	job              domain.Job
	secretRefID      string
	pendingTokenHash string
	pendingToken     string
	operatorBody     []byte
	poll             signedAgentHTTPResult
}

type lifecycleTrustSequenceEvidence struct {
	fixture                nodeLifecycleHTTPFixture
	rotation               lifecycleTrustRotation
	previewBody            []byte
	clearBody              []byte
	revokeBody             []byte
	activeEnrollmentBody   []byte
	recoveryEnrollmentBody []byte
	instanceID             string
	activeEnrollment       domain.NodeEnrollmentToken
	recoveryEnrollment     domain.NodeEnrollmentToken
	activeBeforeClear      signedAgentHTTPResult
	pendingBeforeClear     signedAgentHTTPResult
	randomRejected         signedAgentHTTPResult
	activeAfterClear       signedAgentHTTPResult
	pendingAfterClear      signedAgentHTTPResult
	activeAfterRevoke      signedAgentHTTPResult
	pollAfterRevoke        signedAgentHTTPResult
	markerRejected         signedAgentHTTPResult
	registrationFailure    []byte
}

func TestPostgresIntegrationNodeLifecycleStaleRotationClearRevokeHTTP(t *testing.T) {
	evidence := executeLifecycleTrustClearRevokeSequence(t, "sequence")

	assertHTTPPostgresCount(t, evidence.fixture.ctx, evidence.fixture.pool, `select count(*) from jobs where id=$1 and status='cancelled'`, 1, evidence.rotation.job.ID)
	assertHTTPPostgresCount(t, evidence.fixture.ctx, evidence.fixture.pool, `select count(*) from audit_events where resource_id=$1 and action in ('node.agent_token.clear_stale','node.agent_identity.revoke')`, 2, evidence.fixture.node.ID)
	assertHTTPPostgresCount(t, evidence.fixture.ctx, evidence.fixture.pool, `select count(*) from node_inventory_snapshots where node_id=$1`, evidence.fixture.inventoryCount, evidence.fixture.node.ID)
}

func TestPostgresIntegrationNodeLifecycleStaleRotationPreviewChangedHTTP(t *testing.T) {
	fixture := newLifecycleTrustHTTPFixture(t, "preview-changed")
	rotation := queueAndClaimLifecycleTrustRotation(t, fixture, true)
	markLifecycleTrustRotationStale(t, fixture, rotation.job.ID)
	preview, previewBody := previewLifecycleTrustStaleRotation(t, fixture, rotation.job.ID)
	expectedIDs := lifecycleTrustSafeCandidateIDs(preview)

	if _, err := fixture.pool.Exec(fixture.ctx, `update node_agents set last_job_poll_at=now() where node_id=$1`, fixture.node.ID); err != nil {
		t.Fatalf("install fresh post-preview agent progress: %v", err)
	}
	clearRec := sendOperatorJSON(t, fixture.handler, nethttp.MethodPost, "/api/v1/nodes/"+fixture.node.ID+"/diagnostics/clear-stale-rotation", fixture.adminToken, true, lifecycleTrustClearPayload(fixture.node.Name, expectedIDs), nil)
	if clearRec.Code != nethttp.StatusConflict {
		t.Fatalf("preview-changed clear status = %d, want 409", clearRec.Code)
	}
	assertLifecycleErrorCode(t, clearRec, nodeStaleRotationPreviewChanged)
	assertLifecycleTrustBodyExcludes(t, "preview-changed response", clearRec.Body.Bytes(), lifecycleTrustForbiddenValues(fixture, rotation, nil))
	assertLifecycleTrustBodyExcludes(t, "preview response", previewBody, lifecycleTrustForbiddenValues(fixture, rotation, nil))

	assertHTTPPostgresCount(t, fixture.ctx, fixture.pool, `select count(*) from jobs where id=$1 and status='running' and locked_by is not null and locked_until is not null`, 1, rotation.job.ID)
	assertHTTPPostgresCount(t, fixture.ctx, fixture.pool, `select count(*) from resource_locks where job_id=$1`, 1, rotation.job.ID)
	assertHTTPPostgresCount(t, fixture.ctx, fixture.pool, `select count(*) from secret_refs where id=$1`, 1, rotation.secretRefID)
	assertHTTPPostgresCount(t, fixture.ctx, fixture.pool, `select count(*) from job_logs where job_id=$1 and message='stale agent token rotation cancelled by operator'`, 0, rotation.job.ID)
	assertHTTPPostgresCount(t, fixture.ctx, fixture.pool, `select count(*) from audit_events where resource_id=$1 and action='node.agent_token.clear_stale'`, 0, fixture.node.ID)

	active := sendSignedAgentJSON(t, fixture.handler, nethttp.MethodPost, "/agent/heartbeat", fixture.registration.AgentToken, lifecycleTrustHeartbeat(fixture.node), nil)
	if active.rec.Code != nethttp.StatusOK {
		t.Fatalf("active token after preview conflict status = %d, want 200", active.rec.Code)
	}
	verifySignedAgentResponse(t, active.req, active.rec, fixture.registration.AgentToken)
	pending := sendSignedAgentJSON(t, fixture.handler, nethttp.MethodPost, "/agent/heartbeat", rotation.pendingToken, lifecycleTrustHeartbeat(fixture.node), nil)
	if pending.rec.Code != nethttp.StatusOK {
		t.Fatalf("pending token after preview conflict status = %d, want 200", pending.rec.Code)
	}
	verifySignedAgentResponse(t, pending.req, pending.rec, rotation.pendingToken)
	if !fixture.store.ValidateAgentToken(fixture.ctx, fixture.node.ID, fixture.registration.AgentToken) || !fixture.store.ValidateAgentToken(fixture.ctx, fixture.node.ID, rotation.pendingToken) {
		t.Fatal("preview conflict changed active or pending token validity")
	}
}

func TestPostgresIntegrationNodeLifecycleStaleRotationClearRevokeRedactionHTTP(t *testing.T) {
	evidence := executeLifecycleTrustClearRevokeSequence(t, "redaction")
	fixture := evidence.fixture
	rotation := evidence.rotation

	forbidden := lifecycleTrustForbiddenValues(fixture, rotation, &evidence)
	for _, signed := range []signedAgentHTTPResult{
		rotation.poll,
		evidence.activeBeforeClear,
		evidence.pendingBeforeClear,
		evidence.randomRejected,
		evidence.activeAfterClear,
		evidence.pendingAfterClear,
		evidence.activeAfterRevoke,
		evidence.pollAfterRevoke,
		evidence.markerRejected,
	} {
		forbidden = append(forbidden,
			lifecycleForbiddenValue{category: "request signature", value: signed.req.Header.Get(agentauth.HeaderSignature)},
			lifecycleForbiddenValue{category: "request nonce", value: signed.req.Header.Get(agentauth.HeaderNonce)},
		)
	}
	operatorResponses := []struct {
		label string
		body  []byte
	}{
		{"rotation operator response", rotation.operatorBody},
		{"stale preview response", evidence.previewBody},
		{"stale clear response", evidence.clearBody},
		{"identity revoke response", evidence.revokeBody},
		{"pending token rejection", evidence.pendingAfterClear.rec.Body.Bytes()},
		{"active token rejection after revoke", evidence.activeAfterRevoke.rec.Body.Bytes()},
		{"job poll rejection after revoke", evidence.pollAfterRevoke.rec.Body.Bytes()},
		{"enrollment registration rejection", evidence.registrationFailure},
		{"marker-bearing unauthorized result", evidence.markerRejected.rec.Body.Bytes()},
	}
	for _, response := range operatorResponses {
		assertLifecycleTrustBodyExcludes(t, response.label, response.body, forbidden)
	}

	if !bytes.Contains(rotation.poll.rec.Body.Bytes(), []byte(rotation.pendingToken)) {
		t.Fatal("signed-agent rotation claim did not contain required pending execution credential")
	}
	pollForbidden := []lifecycleForbiddenValue{
		{category: "active agent credential", value: fixture.registration.AgentToken},
		{category: "original enrollment credential", value: fixture.enrollment.Token},
		{category: "secret reference identifier", value: rotation.secretRefID},
		{category: "pending credential hash", value: rotation.pendingTokenHash},
		{category: "active unused enrollment credential", value: evidence.activeEnrollment.Token},
		{category: "replacement enrollment credential", value: evidence.recoveryEnrollment.Token},
		{category: "request signature", value: rotation.poll.req.Header.Get(agentauth.HeaderSignature)},
		{category: "request nonce", value: rotation.poll.req.Header.Get(agentauth.HeaderNonce)},
	}
	assertLifecycleTrustBodyExcludes(t, "signed-agent rotation claim", rotation.poll.rec.Body.Bytes(), pollForbidden)
	assertLifecycleTrustOneTimeResponse(t, "active unused enrollment response", evidence.activeEnrollmentBody, evidence.activeEnrollment.Token, forbidden)
	assertLifecycleTrustOneTimeResponse(t, "replacement enrollment response", evidence.recoveryEnrollmentBody, evidence.recoveryEnrollment.Token, forbidden)

	var auditText string
	if err := fixture.pool.QueryRow(fixture.ctx, `select coalesce(string_agg(row_to_json(x)::text, E'\n'), '') from audit_events x where resource_id=$1`, fixture.node.ID).Scan(&auditText); err != nil {
		t.Fatalf("query lifecycle trust audit redaction evidence: %v", err)
	}
	var jobLogText string
	if err := fixture.pool.QueryRow(fixture.ctx, `select coalesce(string_agg(row_to_json(x)::text, E'\n'), '') from job_logs x join jobs j on j.id=x.job_id where j.node_id=$1`, fixture.node.ID).Scan(&jobLogText); err != nil {
		t.Fatalf("query lifecycle trust job-log redaction evidence: %v", err)
	}
	assertLifecycleTrustBodyExcludes(t, "audit history", []byte(auditText), forbidden)
	assertLifecycleTrustBodyExcludes(t, "job-log history", []byte(jobLogText), forbidden)
	assertLifecycleTrustBodyExcludes(t, "captured application logs", fixture.logs.Bytes(), forbidden)
}

func executeLifecycleTrustClearRevokeSequence(t *testing.T, label string) lifecycleTrustSequenceEvidence {
	t.Helper()

	fixture := newLifecycleTrustHTTPFixture(t, "trust-"+label)
	instance := createNodeLifecycleManagedInstance(t, fixture)
	rotation := queueAndClaimLifecycleTrustRotation(t, fixture, true)
	evidence := lifecycleTrustSequenceEvidence{fixture: fixture, rotation: rotation, instanceID: instance.ID}

	evidence.activeBeforeClear = sendSignedAgentJSON(t, fixture.handler, nethttp.MethodPost, "/agent/heartbeat", fixture.registration.AgentToken, lifecycleTrustHeartbeat(fixture.node), nil)
	if evidence.activeBeforeClear.rec.Code != nethttp.StatusOK {
		t.Fatalf("active pre-clear heartbeat status = %d, want 200", evidence.activeBeforeClear.rec.Code)
	}
	verifySignedAgentResponse(t, evidence.activeBeforeClear.req, evidence.activeBeforeClear.rec, fixture.registration.AgentToken)
	evidence.pendingBeforeClear = sendSignedAgentJSON(t, fixture.handler, nethttp.MethodPost, "/agent/heartbeat", rotation.pendingToken, lifecycleTrustHeartbeat(fixture.node), nil)
	if evidence.pendingBeforeClear.rec.Code != nethttp.StatusOK {
		t.Fatalf("pending pre-clear heartbeat status = %d, want 200", evidence.pendingBeforeClear.rec.Code)
	}
	verifySignedAgentResponse(t, evidence.pendingBeforeClear.req, evidence.pendingBeforeClear.rec, rotation.pendingToken)

	randomToken := strings.ReplaceAll(id.New(), "-", "") + strings.ReplaceAll(id.New(), "-", "")
	evidence.randomRejected = sendSignedAgentJSON(t, fixture.handler, nethttp.MethodPost, "/agent/heartbeat", randomToken, lifecycleTrustHeartbeat(fixture.node), nil)
	if evidence.randomRejected.rec.Code != nethttp.StatusUnauthorized {
		t.Fatalf("unrelated credential heartbeat status = %d, want 401", evidence.randomRejected.rec.Code)
	}
	assertLifecycleTrustBodyExcludes(t, "unrelated credential rejection", evidence.randomRejected.rec.Body.Bytes(), []lifecycleForbiddenValue{{category: "unrelated credential", value: randomToken}})

	markerPayload := map[string]any{
		"status": "succeeded",
		"result": map[string]any{
			"private_key": fixture.markers["private_key"],
			"config":      fixture.markers["configuration"],
		},
	}
	evidence.markerRejected = sendSignedAgentJSON(t, fixture.handler, nethttp.MethodPost, "/agent/jobs/"+rotation.job.ID+"/result", randomToken, markerPayload, nil)
	if evidence.markerRejected.rec.Code != nethttp.StatusUnauthorized {
		t.Fatalf("marker-bearing unauthorized result status = %d, want 401", evidence.markerRejected.rec.Code)
	}

	markLifecycleTrustRotationStale(t, fixture, rotation.job.ID)
	preview, previewBody := previewLifecycleTrustStaleRotation(t, fixture, rotation.job.ID)
	evidence.previewBody = previewBody
	expectedIDs := lifecycleTrustSafeCandidateIDs(preview)

	missingCSRF := sendOperatorJSON(t, fixture.handler, nethttp.MethodPost, "/api/v1/nodes/"+fixture.node.ID+"/diagnostics/clear-stale-rotation", fixture.adminToken, false, lifecycleTrustClearPayload(fixture.node.Name, expectedIDs), nil)
	if missingCSRF.Code != nethttp.StatusForbidden {
		t.Fatalf("stale clear without csrf status = %d, want 403", missingCSRF.Code)
	}
	var clearResult domain.NodeStaleRotationClearResult
	clearRec := sendOperatorJSON(t, fixture.handler, nethttp.MethodPost, "/api/v1/nodes/"+fixture.node.ID+"/diagnostics/clear-stale-rotation", fixture.adminToken, true, lifecycleTrustClearPayload(fixture.node.Name, expectedIDs), &clearResult)
	if clearRec.Code != nethttp.StatusOK {
		t.Fatalf("stale clear status = %d, want 200", clearRec.Code)
	}
	assertHTTPPostgresNoStoreHeader(t, clearRec)
	if clearResult.Status != "cleared" || clearResult.NodeID != fixture.node.ID || clearResult.ClearedCount != 1 || !clearResult.PendingRotationStateCleared || !clearResult.ActiveAgentIdentityPreserved {
		t.Fatal("stale clear response metadata mismatch")
	}
	if len(clearResult.ClearedJobs) != 1 || clearResult.ClearedJobs[0].JobID != rotation.job.ID || clearResult.ClearedJobs[0].PreviousStatus != "running" || clearResult.ClearedJobs[0].Status != "cancelled" || clearResult.ClearedJobs[0].StaleReason != domain.NodeStaleRotationReasonClaimedInactive {
		t.Fatal("stale clear response job set mismatch")
	}
	evidence.clearBody = append([]byte(nil), clearRec.Body.Bytes()...)

	assertHTTPPostgresCount(t, fixture.ctx, fixture.pool, `select count(*) from jobs where id=$1 and status='cancelled' and locked_by is null and locked_until is null`, 1, rotation.job.ID)
	assertHTTPPostgresCount(t, fixture.ctx, fixture.pool, `select count(*) from resource_locks where job_id=$1`, 0, rotation.job.ID)
	assertHTTPPostgresCount(t, fixture.ctx, fixture.pool, `select count(*) from secret_refs where id=$1`, 0, rotation.secretRefID)
	assertHTTPPostgresCount(t, fixture.ctx, fixture.pool, `select count(*) from job_logs where job_id=$1 and message='stale agent token rotation cancelled by operator'`, 1, rotation.job.ID)
	assertLifecycleTrustAudit(t, fixture, "node.agent_token.clear_stale", 1)
	assertLifecycleTrustAuditSummary(t, fixture, "node.agent_token.clear_stale", rotation.job.ID, domain.NodeStaleRotationReasonClaimedInactive, staleRotationClearReason)
	if got := lifecycleTrustActiveTokenHash(t, fixture); got != fixture.agentTokenHash {
		t.Fatal("stale clear changed the active agent token hash")
	}
	assertHTTPPostgresCount(t, fixture.ctx, fixture.pool, `select count(*) from node_agents where node_id=$1 and status='active' and revoked_at is null and agent_token_hash is not null`, 1, fixture.node.ID)
	assertHTTPPostgresEnrollmentTokenStatus(t, fixture.ctx, fixture.pool, fixture.enrollment.ID, "used")

	evidence.activeAfterClear = sendSignedAgentJSON(t, fixture.handler, nethttp.MethodPost, "/agent/heartbeat", fixture.registration.AgentToken, lifecycleTrustHeartbeat(fixture.node), nil)
	if evidence.activeAfterClear.rec.Code != nethttp.StatusOK {
		t.Fatalf("active post-clear heartbeat status = %d, want 200", evidence.activeAfterClear.rec.Code)
	}
	verifySignedAgentResponse(t, evidence.activeAfterClear.req, evidence.activeAfterClear.rec, fixture.registration.AgentToken)
	heartbeatAfterActive := assertHTTPPostgresNodeHeartbeatSet(t, fixture.ctx, fixture.pool, fixture.node.ID)
	nodeBeforePendingRejection, err := fixture.store.GetNode(fixture.ctx, fixture.node.ID)
	if err != nil {
		t.Fatalf("get node before pending-token rejection: %v", err)
	}
	evidence.pendingAfterClear = sendSignedAgentJSON(t, fixture.handler, nethttp.MethodPost, "/agent/heartbeat", rotation.pendingToken, lifecycleTrustHeartbeat(fixture.node), nil)
	if evidence.pendingAfterClear.rec.Code != nethttp.StatusUnauthorized {
		t.Fatalf("cleared pending credential status = %d, want 401", evidence.pendingAfterClear.rec.Code)
	}
	assertHTTPPostgresNodeHeartbeatEquals(t, fixture.ctx, fixture.pool, fixture.node.ID, heartbeatAfterActive)
	nodeAfterPendingRejection, err := fixture.store.GetNode(fixture.ctx, fixture.node.ID)
	if err != nil {
		t.Fatalf("get node after pending-token rejection: %v", err)
	}
	if nodeAfterPendingRejection.Status != nodeBeforePendingRejection.Status || nodeAfterPendingRejection.AgentStatus != nodeBeforePendingRejection.AgentStatus || !sameHTTPPostgresNullableTime(nodeAfterPendingRejection.LastHeartbeatAt, nodeBeforePendingRejection.LastHeartbeatAt) {
		t.Fatal("rejected pending credential mutated node state")
	}
	assertHTTPPostgresCount(t, fixture.ctx, fixture.pool, `select count(*) from jobs where id=$1 and status='cancelled'`, 1, rotation.job.ID)
	if !fixture.store.ValidateAgentToken(fixture.ctx, fixture.node.ID, fixture.registration.AgentToken) || fixture.store.ValidateAgentToken(fixture.ctx, fixture.node.ID, rotation.pendingToken) {
		t.Fatal("stale clear did not preserve active-token/invalidate pending-token boundary")
	}
	var afterClear domain.NodeStaleRotationPreview
	afterClearRec := sendOperatorJSON(t, fixture.handler, nethttp.MethodGet, "/api/v1/nodes/"+fixture.node.ID+"/diagnostics/stale-rotation", fixture.adminToken, false, nil, &afterClear)
	if afterClearRec.Code != nethttp.StatusOK || afterClear.StaleRotationDetected || afterClear.TokenRotationStatus == "rotating" || len(afterClear.Candidates) != 0 {
		t.Fatal("stale rotation diagnostics did not converge after clear")
	}

	activeEnrollmentRec := sendOperatorJSON(t, fixture.handler, nethttp.MethodPost, "/api/v1/nodes/"+fixture.node.ID+"/enrollment-token?ttl_hours=1", fixture.adminToken, true, nil, &evidence.activeEnrollment)
	if activeEnrollmentRec.Code != nethttp.StatusCreated || evidence.activeEnrollment.ID == "" || evidence.activeEnrollment.NodeID != fixture.node.ID || evidence.activeEnrollment.Token == "" || evidence.activeEnrollment.Status != "active" {
		t.Fatal("active unused enrollment credential response mismatch")
	}
	assertHTTPPostgresNoStoreHeader(t, activeEnrollmentRec)
	evidence.activeEnrollmentBody = append([]byte(nil), activeEnrollmentRec.Body.Bytes()...)
	assertHTTPPostgresEnrollmentTokenStatus(t, fixture.ctx, fixture.pool, evidence.activeEnrollment.ID, "active")

	missingRevokeCSRF := sendOperatorJSON(t, fixture.handler, nethttp.MethodPost, "/api/v1/nodes/"+fixture.node.ID+"/agent-identity/revoke", fixture.adminToken, false, lifecycleTrustRevokePayload(fixture.node.Name), nil)
	if missingRevokeCSRF.Code != nethttp.StatusForbidden {
		t.Fatalf("identity revoke without csrf status = %d, want 403", missingRevokeCSRF.Code)
	}
	var revokeResult domain.NodeAgentIdentityRevokeResult
	revokeRec := sendOperatorJSON(t, fixture.handler, nethttp.MethodPost, "/api/v1/nodes/"+fixture.node.ID+"/agent-identity/revoke", fixture.adminToken, true, lifecycleTrustRevokePayload(fixture.node.Name), &revokeResult)
	if revokeRec.Code != nethttp.StatusOK {
		t.Fatalf("identity revoke status = %d, want 200", revokeRec.Code)
	}
	assertHTTPPostgresNoStoreHeader(t, revokeRec)
	if revokeResult.Status != "revoked" || revokeResult.NodeID != fixture.node.ID || revokeResult.AgentStatus != "revoked" || revokeResult.AlreadyRevoked || revokeResult.RevokedAt.IsZero() || revokeResult.RevokedEnrollmentTokens != 1 {
		t.Fatal("identity revoke response metadata mismatch")
	}
	evidence.revokeBody = append([]byte(nil), revokeRec.Body.Bytes()...)
	assertLifecycleTrustAudit(t, fixture, "node.agent_identity.revoke", 1)
	assertLifecycleTrustAuditSummary(t, fixture, "node.agent_identity.revoke", "enrollment_tokens_revoked=1", agentIdentityRevokeReason)
	assertHTTPPostgresCount(t, fixture.ctx, fixture.pool, `select count(*) from node_agents where node_id=$1 and status='revoked' and revoked_at is not null and agent_token_hash is null`, 1, fixture.node.ID)
	assertHTTPPostgresEnrollmentTokenStatus(t, fixture.ctx, fixture.pool, evidence.activeEnrollment.ID, "revoked")
	assertHTTPPostgresCount(t, fixture.ctx, fixture.pool, `select count(*) from nodes where id=$1`, 1, fixture.node.ID)
	assertHTTPPostgresCount(t, fixture.ctx, fixture.pool, `select count(*) from node_inventory_snapshots where node_id=$1`, fixture.inventoryCount, fixture.node.ID)
	assertHTTPPostgresCount(t, fixture.ctx, fixture.pool, `select count(*) from instances where id=$1 and node_id=$2 and status='active'`, 1, evidence.instanceID, fixture.node.ID)

	nodeBeforeRejectedCalls, err := fixture.store.GetNode(fixture.ctx, fixture.node.ID)
	if err != nil {
		t.Fatalf("get revoked node before rejected calls: %v", err)
	}
	evidence.activeAfterRevoke = sendSignedAgentJSON(t, fixture.handler, nethttp.MethodPost, "/agent/heartbeat", fixture.registration.AgentToken, lifecycleTrustHeartbeat(fixture.node), nil)
	if evidence.activeAfterRevoke.rec.Code != nethttp.StatusUnauthorized {
		t.Fatalf("revoked active heartbeat status = %d, want 401", evidence.activeAfterRevoke.rec.Code)
	}
	evidence.pollAfterRevoke = sendSignedAgentJSON(t, fixture.handler, nethttp.MethodGet, "/agent/jobs/next?node_id="+fixture.node.ID, fixture.registration.AgentToken, nil, nil)
	if evidence.pollAfterRevoke.rec.Code != nethttp.StatusUnauthorized {
		t.Fatalf("revoked active job poll status = %d, want 401", evidence.pollAfterRevoke.rec.Code)
	}
	if evidence.activeAfterRevoke.rec.Header().Get(agentauth.HeaderSignature) != "" || evidence.pollAfterRevoke.rec.Header().Get(agentauth.HeaderSignature) != "" {
		t.Fatal("unauthorized signed-agent responses unexpectedly carried response signatures")
	}
	nodeAfterRejectedCalls, err := fixture.store.GetNode(fixture.ctx, fixture.node.ID)
	if err != nil {
		t.Fatalf("get revoked node after rejected calls: %v", err)
	}
	if !sameHTTPPostgresNullableTime(nodeBeforeRejectedCalls.LastHeartbeatAt, nodeAfterRejectedCalls.LastHeartbeatAt) || nodeAfterRejectedCalls.AgentStatus != "revoked" {
		t.Fatal("rejected post-revoke calls mutated node heartbeat or identity state")
	}
	assertHTTPPostgresCount(t, fixture.ctx, fixture.pool, `select count(*) from jobs where id=$1 and status='cancelled'`, 1, rotation.job.ID)

	registrationFailure := registerAgentOnboardingHTTPRaw(t, fixture.handler, fixture.node.ID, evidence.activeEnrollment.Token, "revoked-agent", "192.0.2.98", agentOnboardingVersion)
	if registrationFailure.Code != nethttp.StatusUnauthorized {
		t.Fatalf("revoked enrollment registration status = %d, want 401", registrationFailure.Code)
	}
	assertAgentRegisterErrorCode(t, registrationFailure, agentEnrollmentReissueRequired)
	evidence.registrationFailure = append([]byte(nil), registrationFailure.Body.Bytes()...)
	assertHTTPPostgresEnrollmentTokenStatus(t, fixture.ctx, fixture.pool, evidence.activeEnrollment.ID, "revoked")
	assertHTTPPostgresCount(t, fixture.ctx, fixture.pool, `select count(*) from node_agents where node_id=$1`, 1, fixture.node.ID)

	recoveryEnrollmentRec := sendOperatorJSON(t, fixture.handler, nethttp.MethodPost, "/api/v1/nodes/"+fixture.node.ID+"/enrollment-token?ttl_hours=1", fixture.adminToken, true, nil, &evidence.recoveryEnrollment)
	if recoveryEnrollmentRec.Code != nethttp.StatusCreated || evidence.recoveryEnrollment.ID == "" || evidence.recoveryEnrollment.NodeID != fixture.node.ID || evidence.recoveryEnrollment.Token == "" || evidence.recoveryEnrollment.Status != "active" {
		t.Fatal("explicit recovery enrollment credential response mismatch")
	}
	assertHTTPPostgresNoStoreHeader(t, recoveryEnrollmentRec)
	evidence.recoveryEnrollmentBody = append([]byte(nil), recoveryEnrollmentRec.Body.Bytes()...)
	assertHTTPPostgresEnrollmentTokenStatus(t, fixture.ctx, fixture.pool, evidence.recoveryEnrollment.ID, "active")
	assertHTTPPostgresCount(t, fixture.ctx, fixture.pool, `select count(*) from node_agents where node_id=$1 and status='revoked' and agent_token_hash is null`, 1, fixture.node.ID)
	assertHTTPPostgresCount(t, fixture.ctx, fixture.pool, `select count(*) from audit_events where resource_id=$1 and action='node.agent_token.clear_stale'`, 1, fixture.node.ID)
	assertHTTPPostgresCount(t, fixture.ctx, fixture.pool, `select count(*) from audit_events where resource_id=$1 and action='node.agent_identity.revoke'`, 1, fixture.node.ID)
	assertHTTPPostgresCount(t, fixture.ctx, fixture.pool, `select count(*) from job_logs where job_id=$1`, 2, rotation.job.ID)
	return evidence
}

func newLifecycleTrustHTTPFixture(t *testing.T, label string) nodeLifecycleHTTPFixture {
	t.Helper()
	fixture := newNodeLifecycleHTTPFixture(t, label)
	attachHTTPPostgresIntegrationSecretService(t, fixture.store)
	return fixture
}

func queueAndClaimLifecycleTrustRotation(t *testing.T, fixture nodeLifecycleHTTPFixture, verifyOperatorBoundary bool) lifecycleTrustRotation {
	t.Helper()

	path := "/api/v1/nodes/" + fixture.node.ID + "/agent-token/rotate"
	if verifyOperatorBoundary {
		unauthenticated := sendOperatorJSON(t, fixture.handler, nethttp.MethodPost, path, "", true, nil, nil)
		if unauthenticated.Code != nethttp.StatusUnauthorized {
			t.Fatalf("unauthenticated rotation status = %d, want 401", unauthenticated.Code)
		}
		missingCSRF := sendOperatorJSON(t, fixture.handler, nethttp.MethodPost, path, fixture.adminToken, false, nil, nil)
		if missingCSRF.Code != nethttp.StatusForbidden {
			t.Fatalf("rotation without csrf status = %d, want 403", missingCSRF.Code)
		}
	}

	var job domain.Job
	rec := sendOperatorJSON(t, fixture.handler, nethttp.MethodPost, path, fixture.adminToken, true, nil, &job)
	if rec.Code != nethttp.StatusAccepted {
		t.Fatalf("rotation queue status = %d, want 202", rec.Code)
	}
	assertHTTPPostgresNoStoreHeader(t, rec)
	if job.ID == "" || job.Type != "node.agent.rotate_token" || job.Status != "queued" || job.NodeID == nil || *job.NodeID != fixture.node.ID {
		t.Fatal("rotation operator response metadata mismatch")
	}
	var metadata map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &metadata); err != nil {
		t.Fatalf("decode rotation operator metadata: %v", err)
	}
	assertLifecycleExactKeys(t, "rotation operator metadata", metadata, []string{"id", "type", "scope_type", "status", "scope_id", "node_id", "instance_id", "priority", "created_at", "started_at", "finished_at"})

	var secretRefID string
	var pendingHash string
	if err := fixture.pool.QueryRow(fixture.ctx, `select coalesce(payload_json->>'new_agent_token_secret_ref_id',''),coalesce(payload_json->>'new_agent_token_hash','') from jobs where id=$1`, job.ID).Scan(&secretRefID, &pendingHash); err != nil {
		t.Fatalf("query persisted rotation correlation: %v", err)
	}
	if secretRefID == "" || pendingHash == "" {
		t.Fatal("persisted rotation job is missing pending secret correlation")
	}
	assertLifecycleTrustBodyExcludes(t, "rotation operator response", rec.Body.Bytes(), []lifecycleForbiddenValue{
		{category: "job payload", value: "\"payload\""},
		{category: "secret reference identifier", value: secretRefID},
		{category: "pending token hash", value: pendingHash},
		{category: "active agent credential", value: fixture.registration.AgentToken},
		{category: "active agent token hash", value: fixture.agentTokenHash},
	})
	assertHTTPPostgresCount(t, fixture.ctx, fixture.pool, `select count(*) from jobs where id=$1 and node_id=$2 and type='node.agent.rotate_token' and status='queued'`, 1, job.ID, fixture.node.ID)
	assertHTTPPostgresCount(t, fixture.ctx, fixture.pool, `select count(*) from secret_refs where id=$1`, 1, secretRefID)
	if got := lifecycleTrustActiveTokenHash(t, fixture); got != fixture.agentTokenHash {
		t.Fatal("rotation queue changed active token hash")
	}

	var claimed domain.Job
	poll := sendSignedAgentJSON(t, fixture.handler, nethttp.MethodGet, "/agent/jobs/next?node_id="+fixture.node.ID, fixture.registration.AgentToken, nil, &claimed)
	if poll.rec.Code != nethttp.StatusOK {
		t.Fatalf("rotation signed-agent poll status = %d, want 200", poll.rec.Code)
	}
	verifySignedAgentResponse(t, poll.req, poll.rec, fixture.registration.AgentToken)
	if claimed.ID != job.ID || claimed.Type != "node.agent.rotate_token" || claimed.Status != "running" || claimed.NodeID == nil || *claimed.NodeID != fixture.node.ID {
		t.Fatal("signed-agent rotation claim metadata mismatch")
	}
	assertLifecycleExactKeys(t, "rotation execution payload", claimed.Payload, []string{"node_id", "new_agent_token", "new_token_hint"})
	pendingToken, _ := claimed.Payload["new_agent_token"].(string)
	if strings.TrimSpace(pendingToken) == "" || claimed.Payload["node_id"] != fixture.node.ID {
		t.Fatal("signed-agent rotation claim missing required execution material")
	}
	if strings.TrimSpace(stringifyHTTP(claimed.Payload["new_token_hint"])) == "" {
		t.Fatal("signed-agent rotation claim missing safe token hint")
	}
	assertHTTPPostgresCount(t, fixture.ctx, fixture.pool, `select count(*) from jobs where id=$1 and status='running' and locked_by=$2 and locked_until>now() and started_at is not null`, 1, job.ID, "agent:"+fixture.node.ID)
	assertHTTPPostgresCount(t, fixture.ctx, fixture.pool, `select count(*) from resource_locks where job_id=$1 and resource_type='node' and resource_id=$2`, 1, job.ID, fixture.node.ID)
	assertHTTPPostgresCount(t, fixture.ctx, fixture.pool, `select count(*) from job_logs where job_id=$1 and message='job claimed'`, 1, job.ID)
	if got := lifecycleTrustActiveTokenHash(t, fixture); got != fixture.agentTokenHash {
		t.Fatal("rotation claim changed active token hash")
	}
	return lifecycleTrustRotation{
		job:              job,
		secretRefID:      secretRefID,
		pendingTokenHash: pendingHash,
		pendingToken:     pendingToken,
		operatorBody:     append([]byte(nil), rec.Body.Bytes()...),
		poll:             poll,
	}
}

func markLifecycleTrustRotationStale(t *testing.T, fixture nodeLifecycleHTTPFixture, jobID string) {
	t.Helper()

	now := time.Now().UTC()
	createdAt := now.Add(-20 * time.Minute)
	startedAt := now.Add(-19 * time.Minute)
	claimAt := now.Add(-18 * time.Minute)
	progressAt := now.Add(-30 * time.Minute)
	leaseExpiredAt := now.Add(-10 * time.Minute)
	owner := "agent:" + fixture.node.ID
	if _, err := fixture.pool.Exec(fixture.ctx, `update jobs
		set status='running',created_at=$2,started_at=$3,finished_at=null,locked_by=$4,locked_until=$5
		where id=$1 and type='node.agent.rotate_token'`, jobID, createdAt, startedAt, owner, leaseExpiredAt); err != nil {
		t.Fatalf("age claimed rotation job: %v", err)
	}
	if _, err := fixture.pool.Exec(fixture.ctx, `update node_agents
		set last_seen_at=$2,last_job_poll_at=$2,last_job_claim_at=$3,last_job_claim_job_id=$4,last_job_claim_type='node.agent.rotate_token',
		    last_job_result_at=null,last_job_result_job_id=null,last_job_result_type='',last_job_result_status='',
		    last_inventory_sync_at=$2,last_discovery_sync_at=$2,last_runtime_sync_at=$2
		where node_id=$1`, fixture.node.ID, progressAt, claimAt, jobID); err != nil {
		t.Fatalf("age agent rotation progress evidence: %v", err)
	}
	if _, err := fixture.pool.Exec(fixture.ctx, `update nodes set last_heartbeat_at=$2 where id=$1`, fixture.node.ID, progressAt); err != nil {
		t.Fatalf("age node heartbeat evidence: %v", err)
	}
	if _, err := fixture.pool.Exec(fixture.ctx, `update resource_locks set acquired_at=$2,expires_at=$3 where job_id=$1`, jobID, startedAt, leaseExpiredAt); err != nil {
		t.Fatalf("age rotation resource lock: %v", err)
	}
}

func previewLifecycleTrustStaleRotation(t *testing.T, fixture nodeLifecycleHTTPFixture, expectedJobID string) (domain.NodeStaleRotationPreview, []byte) {
	t.Helper()

	unauthenticated := sendOperatorJSON(t, fixture.handler, nethttp.MethodGet, "/api/v1/nodes/"+fixture.node.ID+"/diagnostics/stale-rotation", "", false, nil, nil)
	if unauthenticated.Code != nethttp.StatusUnauthorized {
		t.Fatalf("unauthenticated stale preview status = %d, want 401", unauthenticated.Code)
	}
	var preview domain.NodeStaleRotationPreview
	rec := sendOperatorJSON(t, fixture.handler, nethttp.MethodGet, "/api/v1/nodes/"+fixture.node.ID+"/diagnostics/stale-rotation", fixture.adminToken, false, nil, &preview)
	if rec.Code != nethttp.StatusOK {
		t.Fatalf("stale preview status = %d, want 200", rec.Code)
	}
	assertHTTPPostgresNoStoreHeader(t, rec)
	if preview.NodeID != fixture.node.ID || !preview.StaleRotationDetected || preview.TokenRotationStatus != "rotating" || len(preview.Candidates) != 1 {
		t.Fatal("stale preview summary mismatch")
	}
	candidate := preview.Candidates[0]
	if candidate.JobID != expectedJobID || candidate.Status != "running" || !candidate.SafeToClear || candidate.StaleReason != domain.NodeStaleRotationReasonClaimedInactive || candidate.LastClaimAt == nil {
		t.Fatal("stale preview candidate mismatch")
	}
	return preview, append([]byte(nil), rec.Body.Bytes()...)
}

func lifecycleTrustSafeCandidateIDs(preview domain.NodeStaleRotationPreview) []string {
	ids := make([]string, 0, len(preview.Candidates))
	for _, candidate := range preview.Candidates {
		if candidate.SafeToClear {
			ids = append(ids, candidate.JobID)
		}
	}
	sort.Strings(ids)
	return ids
}

func lifecycleTrustClearPayload(nodeName string, expectedIDs []string) map[string]any {
	return map[string]any{
		"confirmation":                nodeName,
		"reason":                      staleRotationClearReason,
		"acknowledge_cancel_rotation": true,
		"expected_job_ids":            expectedIDs,
	}
}

func lifecycleTrustRevokePayload(nodeName string) map[string]any {
	return map[string]any{"confirmation": nodeName, "reason": agentIdentityRevokeReason}
}

func lifecycleTrustHeartbeat(node domain.Node) map[string]any {
	return map[string]any{
		"node_id":          node.ID,
		"name":             node.Name,
		"agent_version":    "it-lifecycle-trust/1",
		"protocol_version": agentProtocolVersion,
	}
}

func lifecycleTrustActiveTokenHash(t *testing.T, fixture nodeLifecycleHTTPFixture) string {
	t.Helper()
	var tokenHash string
	if err := fixture.pool.QueryRow(fixture.ctx, `select coalesce(agent_token_hash,'') from node_agents where node_id=$1`, fixture.node.ID).Scan(&tokenHash); err != nil {
		t.Fatalf("query lifecycle trust agent token hash: %v", err)
	}
	return tokenHash
}

func assertLifecycleTrustAudit(t *testing.T, fixture nodeLifecycleHTTPFixture, action string, want int) {
	t.Helper()
	assertHTTPPostgresCount(t, fixture.ctx, fixture.pool, `select count(*) from audit_events where resource_id=$1 and action=$2 and actor_type='platform_user' and actor_user_id=$3`, want, fixture.node.ID, action, fixture.adminUserID)
}

func assertLifecycleTrustAuditSummary(t *testing.T, fixture nodeLifecycleHTTPFixture, action string, expected ...string) {
	t.Helper()
	var summary string
	if err := fixture.pool.QueryRow(fixture.ctx, `select summary from audit_events where resource_id=$1 and action=$2 and actor_type='platform_user' and actor_user_id=$3`, fixture.node.ID, action, fixture.adminUserID).Scan(&summary); err != nil {
		t.Fatalf("query lifecycle trust audit summary for %s: %v", action, err)
	}
	for _, marker := range expected {
		if !strings.Contains(summary, marker) {
			t.Fatalf("%s audit summary missing required safe metadata", action)
		}
	}
}

func lifecycleTrustForbiddenValues(fixture nodeLifecycleHTTPFixture, rotation lifecycleTrustRotation, evidence *lifecycleTrustSequenceEvidence) []lifecycleForbiddenValue {
	values := []lifecycleForbiddenValue{
		{category: "active agent credential", value: fixture.registration.AgentToken},
		{category: "original enrollment credential", value: fixture.enrollment.Token},
		{category: "active agent token hash", value: fixture.agentTokenHash},
		{category: "pending rotation credential", value: rotation.pendingToken},
		{category: "pending rotation credential hash", value: rotation.pendingTokenHash},
		{category: "secret reference identifier", value: rotation.secretRefID},
		{category: "authorization header", value: "Bearer " + fixture.registration.AgentToken},
		{category: "private-key marker", value: fixture.markers["private_key"]},
		{category: "fixture secret-reference marker", value: fixture.markers["secret_ref"]},
		{category: "credential marker", value: fixture.markers["credential"]},
		{category: "configuration marker", value: fixture.markers["configuration"]},
		{category: "command-output marker", value: fixture.markers["command_output"]},
	}
	if evidence != nil {
		values = append(values,
			lifecycleForbiddenValue{category: "active unused enrollment credential", value: evidence.activeEnrollment.Token},
			lifecycleForbiddenValue{category: "replacement enrollment credential", value: evidence.recoveryEnrollment.Token},
		)
	}
	return values
}

func assertLifecycleTrustOneTimeResponse(t *testing.T, label string, body []byte, allowedToken string, forbidden []lifecycleForbiddenValue) {
	t.Helper()
	if allowedToken == "" || !bytes.Contains(body, []byte(allowedToken)) {
		t.Fatalf("%s omitted required one-time credential", label)
	}
	filtered := make([]lifecycleForbiddenValue, 0, len(forbidden))
	for _, item := range forbidden {
		if item.value != allowedToken {
			filtered = append(filtered, item)
		}
	}
	assertLifecycleTrustBodyExcludes(t, label, body, filtered)
}

func assertLifecycleTrustBodyExcludes(t *testing.T, label string, body []byte, forbidden []lifecycleForbiddenValue) {
	t.Helper()
	for _, item := range forbidden {
		if item.value != "" && bytes.Contains(body, []byte(item.value)) {
			t.Fatalf("%s exposed %s", label, item.category)
		}
	}
}
