package http

import (
	"strings"
	"testing"
	"time"

	nethttp "net/http"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

func TestPostgresIntegrationRevokeNodeAgentIdentityRejectsOldSignedAgentHTTP(t *testing.T) {
	store, pool, ctx := setupHTTPPostgresIntegrationStore(t)
	fixture := newAgentOnboardingHTTPFixture(t, ctx, store)
	handler, logs := newHTTPPostgresAgentOnboardingHandler(store)

	node := createAgentOnboardingNodeViaHTTP(t, handler, fixture.adminToken, "revoke")
	enrollment := createAgentOnboardingEnrollmentTokenHTTP(t, handler, node.ID, fixture.adminToken)
	reg := registerAgentOnboardingHTTP(t, handler, node.ID, enrollment.Token, "revoked-agent", "192.0.2.90", agentOnboardingVersion)
	activeEnrollment := createAgentOnboardingEnrollmentTokenHTTP(t, handler, node.ID, fixture.adminToken)

	before, err := store.GetNode(ctx, node.ID)
	if err != nil {
		t.Fatalf("get node before revoke: %v", err)
	}
	var job domain.Job
	jobRec := sendOperatorJSON(t, handler, nethttp.MethodPost, "/api/v1/nodes/"+node.ID+"/inventory/sync", fixture.adminToken, true, nil, &job)
	if jobRec.Code != nethttp.StatusAccepted {
		t.Fatalf("inventory job status = %d, want 202; body=%s", jobRec.Code, jobRec.Body.String())
	}

	var revokeResult domain.NodeAgentIdentityRevokeResult
	revokeRec := sendOperatorJSON(t, handler, nethttp.MethodPost, "/api/v1/nodes/"+node.ID+"/agent-identity/revoke", fixture.adminToken, true, map[string]any{
		"confirmation": node.Name,
		"reason":       "incident response rotation",
	}, &revokeResult)
	if revokeRec.Code != nethttp.StatusOK {
		t.Fatalf("revoke status = %d, want 200; body=%s", revokeRec.Code, revokeRec.Body.String())
	}
	if revokeResult.Status != "revoked" || revokeResult.NodeID != node.ID || revokeResult.AgentStatus != "revoked" || revokeResult.AlreadyRevoked {
		t.Fatalf("revoke result = %#v", revokeResult)
	}
	assertHTTPPostgresPayloadNotContains(t, "revoke response", revokeRec.Body.Bytes(), []string{
		reg.AgentToken,
		activeEnrollment.Token,
		"agent_token",
		"token_hash",
		"token_hint",
		"fingerprint",
		"enrollment_token_id",
		"enrollment_token_hash",
	})

	cases := []struct {
		name    string
		method  string
		path    string
		payload any
	}{
		{
			name:   "heartbeat",
			method: nethttp.MethodPost,
			path:   "/agent/heartbeat",
			payload: map[string]any{
				"node_id": node.ID, "name": node.Name, "agent_version": agentOnboardingVersion, "protocol_version": agentProtocolVersion,
			},
		},
		{
			name:   "inventory",
			method: nethttp.MethodPost,
			path:   "/agent/inventory",
			payload: map[string]any{
				"node_id": node.ID, "source": "inventory", "inventory": map[string]any{"collector": "revoked-agent"},
			},
		},
		{
			name:   "runtime targets",
			method: nethttp.MethodGet,
			path:   "/agent/runtime/instances?node_id=" + node.ID,
		},
		{
			name:    "runtime report",
			method:  nethttp.MethodPost,
			path:    "/agent/runtime/instances",
			payload: map[string]any{"node_id": node.ID, "reports": []any{}},
		},
		{
			name:    "traffic accounting",
			method:  nethttp.MethodPost,
			path:    "/agent/traffic/accounting",
			payload: map[string]any{"node_id": node.ID, "samples": []any{}},
		},
		{
			name:   "job poll",
			method: nethttp.MethodGet,
			path:   "/agent/jobs/next?node_id=" + node.ID,
		},
		{
			name:    "job result",
			method:  nethttp.MethodPost,
			path:    "/agent/jobs/" + job.ID + "/result",
			payload: map[string]any{"status": "succeeded", "result": map[string]any{"ok": true}},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			res := sendSignedAgentJSON(t, handler, tc.method, tc.path, reg.AgentToken, tc.payload, nil)
			if res.rec.Code != nethttp.StatusUnauthorized {
				t.Fatalf("%s status = %d, want 401; body=%s", tc.name, res.rec.Code, res.rec.Body.String())
			}
			assertHTTPPostgresPayloadNotContains(t, tc.name+" unauthorized response", res.rec.Body.Bytes(), []string{reg.AgentToken, activeEnrollment.Token, "token_hash", "agent_token"})
		})
	}

	after, err := store.GetNode(ctx, node.ID)
	if err != nil {
		t.Fatalf("get node after rejected old-agent calls: %v", err)
	}
	if after.AgentStatus != "revoked" || after.Status != before.Status {
		t.Fatalf("node state after rejected old-agent calls = status:%q agent:%q", after.Status, after.AgentStatus)
	}
	if !sameHTTPPostgresNullableTime(after.LastHeartbeatAt, before.LastHeartbeatAt) {
		t.Fatalf("last heartbeat changed after rejected old-agent calls: before=%v after=%v", before.LastHeartbeatAt, after.LastHeartbeatAt)
	}
	assertHTTPPostgresCount(t, ctx, pool, `select count(*) from jobs where id=$1 and status='queued'`, 1, job.ID)
	assertHTTPPostgresCount(t, ctx, pool, `select count(*) from node_agents where node_id=$1 and status='revoked' and revoked_at is not null and agent_token_hash is null`, 1, node.ID)

	retryActiveEnrollment := registerAgentOnboardingHTTPRaw(t, handler, node.ID, activeEnrollment.Token, "revoked-agent", "192.0.2.91", agentOnboardingVersion)
	if retryActiveEnrollment.Code != nethttp.StatusUnauthorized {
		t.Fatalf("revoked enrollment registration status = %d, want 401", retryActiveEnrollment.Code)
	}
	assertAgentRegisterErrorCode(t, retryActiveEnrollment, agentEnrollmentReissueRequired)
	assertHTTPPostgresPayloadNotContains(t, "revoked enrollment response", retryActiveEnrollment.Body.Bytes(), []string{activeEnrollment.Token, reg.AgentToken, "token_hash"})

	recoveryEnrollment := createAgentOnboardingEnrollmentTokenHTTP(t, handler, node.ID, fixture.adminToken)
	if strings.TrimSpace(recoveryEnrollment.Token) == "" || recoveryEnrollment.Status != "active" {
		t.Fatalf("explicit recovery enrollment token = %#v", recoveryEnrollment)
	}
	assertHTTPPostgresAuditAndLogsRedacted(t, ctx, pool, logs.Bytes(), node.ID, []string{enrollment.Token, reg.AgentToken, activeEnrollment.Token})
}

func sameHTTPPostgresNullableTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}
