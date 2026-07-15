package http

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/platform/id"
)

func TestPostgresIntegrationNodeBootstrapBundleRevealDownloadHTTP(t *testing.T) {
	store, pool, ctx := setupHTTPPostgresIntegrationStore(t)
	attachHTTPPostgresIntegrationSecretService(t, store)

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	admin, _, err := store.EnsureBootstrapPlatformUser(ctx, "it-bundle-admin-"+suffix, "it-bundle-admin-"+suffix+"@example.invalid", "Integration Bundle Admin", "integration-password-hash")
	if err != nil {
		t.Fatalf("create bootstrap admin: %v", err)
	}
	adminToken := createHTTPPostgresIntegrationSession(t, ctx, store, admin.ID)
	readonly, err := store.CreatePlatformUser(ctx, "it-bundle-readonly-"+suffix, "it-bundle-readonly-"+suffix+"@example.invalid", "Integration Bundle Readonly", "integration-password-hash", []string{"readonly"}, &admin.ID)
	if err != nil {
		t.Fatalf("create readonly user: %v", err)
	}
	readonlyToken := createHTTPPostgresIntegrationSession(t, ctx, store, readonly.ID)

	nodeA, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-http-bundle-a-" + suffix,
		Kind:          "remote",
		Role:          "ingress",
		Status:        "draft",
		Address:       "10.52.0.10",
		OSFamily:      "linux",
		OSVersion:     "ubuntu-24.04",
		Architecture:  "amd64",
		ExecutionMode: "ssh_bootstrap",
		AgentStatus:   "unknown",
	})
	if err != nil {
		t.Fatalf("create node A: %v", err)
	}
	nodeB, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-http-bundle-b-" + suffix,
		Kind:          "remote",
		Role:          "egress",
		Status:        "draft",
		Address:       "10.52.0.11",
		OSFamily:      "linux",
		OSVersion:     "ubuntu-24.04",
		Architecture:  "amd64",
		ExecutionMode: "ssh_bootstrap",
		AgentStatus:   "unknown",
	})
	if err != nil {
		t.Fatalf("create node B: %v", err)
	}

	runID := id.New()
	missingBundleRunID := id.New()
	bundle := []byte("MEGAVPN_TEST_ONLY=1\r\nMEGAVPN_TEST_NODE_ID=" + nodeA.ID + "\nMEGAVPN_TEST_RUN_ID=" + runID + "\n")
	secret, err := store.CreateSecretRef(ctx, "opaque", bundle, map[string]any{
		"scope":            "node_bootstrap",
		"node_id":          nodeA.ID,
		"bootstrap_run_id": runID,
		"bootstrap_mode":   "manual_bundle",
		"material":         "agent_bootstrap_env",
	})
	if err != nil {
		t.Fatalf("create manual bundle secret ref: %v", err)
	}
	insertHTTPPostgresBootstrapRun(t, ctx, pool, nodeA.ID, runID, &secret.ID, true)
	insertHTTPPostgresBootstrapRun(t, ctx, pool, nodeA.ID, missingBundleRunID, nil, false)

	var logs bytes.Buffer
	handler := New(slog.New(slog.NewTextHandler(&logs, nil)), store, Options{
		Version:           "test",
		SessionCookieName: "megavpn_session",
	})

	listRec := sendHTTPPostgresBundleRequest(t, handler, nethttp.MethodGet, "/api/v1/nodes/"+nodeA.ID+"/bootstrap-runs", adminToken, false, "")
	if listRec.Code != 200 {
		t.Fatalf("bootstrap run list status = %d, want 200; body=%s", listRec.Code, listRec.Body.String())
	}
	var runs []domain.NodeBootstrapRun
	if err := json.Unmarshal(listRec.Body.Bytes(), &runs); err != nil {
		t.Fatalf("decode bootstrap runs: %v", err)
	}
	run := findHTTPPostgresBootstrapRun(t, runs, runID)
	if !run.ManualBundleAvailable || run.BootstrapMode != "manual_bundle" || run.Status != "succeeded" {
		t.Fatalf("bootstrap run projection mismatch: id=%s status=%s mode=%s manual_available=%t", run.ID, run.Status, run.BootstrapMode, run.ManualBundleAvailable)
	}
	assertHTTPPostgresPayloadNotContains(t, "bootstrap run list", listRec.Body.Bytes(), []string{
		string(bundle),
		"MEGAVPN_TEST_ONLY",
		nodeBootstrapBundleSecretRefKey,
		secret.ID,
		"ciphertext",
		"key_version",
		"nonce",
	})

	var storedSecretType, storedKeyVersion string
	var storedCiphertext, storedNonce []byte
	var storedMeta map[string]any
	var storedMetaRaw []byte
	if err := pool.QueryRow(ctx, `select secret_type,ciphertext,key_version,nonce,meta_json from secret_refs where id=$1`, secret.ID).
		Scan(&storedSecretType, &storedCiphertext, &storedKeyVersion, &storedNonce, &storedMetaRaw); err != nil {
		t.Fatalf("load secret ref from PostgreSQL: %v", err)
	}
	if err := json.Unmarshal(storedMetaRaw, &storedMeta); err != nil {
		t.Fatalf("decode secret ref metadata: %v", err)
	}
	if storedSecretType != "opaque" || strings.TrimSpace(storedKeyVersion) == "" || len(storedCiphertext) == 0 || len(storedNonce) == 0 {
		t.Fatalf("stored secret ref metadata invalid: type=%q key_version_present=%t ciphertext_len=%d nonce_len=%d",
			storedSecretType, strings.TrimSpace(storedKeyVersion) != "", len(storedCiphertext), len(storedNonce))
	}
	if storedMeta["node_id"] != nodeA.ID || storedMeta["bootstrap_run_id"] != runID || storedMeta["material"] != "agent_bootstrap_env" {
		t.Fatalf("stored secret ref scope metadata mismatch: %#v", storedMeta)
	}
	if bytes.Equal(storedCiphertext, bundle) || bytes.Contains(storedCiphertext, []byte("MEGAVPN_TEST_ONLY")) {
		t.Fatal("stored bootstrap bundle ciphertext contains plaintext fixture content")
	}
	resolvedRef, resolvedBundle, err := store.ResolveSecretValue(ctx, secret.ID)
	if err != nil {
		t.Fatalf("resolve secret ref from PostgreSQL: %v", err)
	}
	if resolvedRef.ID != secret.ID || !bytes.Equal(resolvedBundle, bundle) {
		t.Fatalf("resolved bootstrap bundle mismatch: ref_match=%t got_len=%d want_len=%d",
			resolvedRef.ID == secret.ID, len(resolvedBundle), len(bundle))
	}

	auditBefore := countHTTPPostgresBundleAudits(t, ctx, pool, nodeA.ID)
	revealRec := sendHTTPPostgresBundleRequest(t, handler, nethttp.MethodPost, "/api/v1/nodes/"+nodeA.ID+"/bootstrap-runs/"+runID+"/bundle/reveal", adminToken, true, `{}`)
	if revealRec.Code != 200 {
		t.Fatalf("reveal status = %d, want 200; body=%s", revealRec.Code, revealRec.Body.String())
	}
	assertNodeBootstrapBundleNoStoreHeaders(t, revealRec)
	var revealPayload struct {
		NodeID            string    `json:"node_id"`
		BootstrapRunID    string    `json:"bootstrap_run_id"`
		Filename          string    `json:"filename"`
		AgentBootstrapEnv string    `json:"agent_bootstrapenv"`
		RevealedAt        time.Time `json:"revealed_at"`
	}
	if err := json.Unmarshal(revealRec.Body.Bytes(), &revealPayload); err != nil {
		t.Fatalf("decode reveal response: %v", err)
	}
	if revealPayload.NodeID != nodeA.ID || revealPayload.BootstrapRunID != runID || revealPayload.AgentBootstrapEnv != string(bundle) || revealPayload.RevealedAt.IsZero() {
		t.Fatalf("reveal response mismatch: node_match=%t run_match=%t bundle_len=%d want_len=%d revealed_at_zero=%t",
			revealPayload.NodeID == nodeA.ID, revealPayload.BootstrapRunID == runID, len(revealPayload.AgentBootstrapEnv), len(bundle), revealPayload.RevealedAt.IsZero())
	}
	assertSafeHTTPPostgresBundleFilename(t, revealPayload.Filename)
	assertHTTPPostgresPayloadNotContains(t, "reveal response metadata", revealRec.Body.Bytes(), []string{
		nodeBootstrapBundleSecretRefKey,
		secret.ID,
		"ciphertext",
		"key_version",
		"nonce",
	})

	downloadRec := sendHTTPPostgresBundleRequest(t, handler, nethttp.MethodPost, "/api/v1/nodes/"+nodeA.ID+"/bootstrap-runs/"+runID+"/bundle/download", adminToken, true, "")
	if downloadRec.Code != 200 {
		t.Fatalf("download status = %d, want 200; body_len=%d", downloadRec.Code, downloadRec.Body.Len())
	}
	assertNodeBootstrapBundleNoStoreHeaders(t, downloadRec)
	if got := downloadRec.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("download Content-Type = %q", got)
	}
	disposition, params, err := mime.ParseMediaType(downloadRec.Header().Get("Content-Disposition"))
	if err != nil {
		t.Fatalf("parse download Content-Disposition: %v", err)
	}
	if disposition != "attachment" {
		t.Fatalf("download Content-Disposition = %q, want attachment", disposition)
	}
	assertSafeHTTPPostgresBundleFilename(t, params["filename"])
	if !bytes.Equal(downloadRec.Body.Bytes(), bundle) {
		t.Fatalf("download body mismatch: got len %d want len %d", downloadRec.Body.Len(), len(bundle))
	}

	repeatRevealRec := sendHTTPPostgresBundleRequest(t, handler, nethttp.MethodPost, "/api/v1/nodes/"+nodeA.ID+"/bootstrap-runs/"+runID+"/bundle/reveal", adminToken, true, `{}`)
	if repeatRevealRec.Code != 200 {
		t.Fatalf("repeat reveal status = %d, want 200; body=%s", repeatRevealRec.Code, repeatRevealRec.Body.String())
	}
	assertHTTPPostgresBundleAuditEvents(t, ctx, pool, nodeA.ID, admin.ID, runID, secret.ID, auditBefore+3)

	readonlyReveal := sendHTTPPostgresBundleRequest(t, handler, nethttp.MethodPost, "/api/v1/nodes/"+nodeA.ID+"/bootstrap-runs/"+runID+"/bundle/reveal", readonlyToken, true, `{}`)
	if readonlyReveal.Code != 403 {
		t.Fatalf("readonly reveal status = %d, want 403; body=%s", readonlyReveal.Code, readonlyReveal.Body.String())
	}
	assertHTTPPostgresPayloadNotContains(t, "readonly reveal denial", readonlyReveal.Body.Bytes(), []string{string(bundle), secret.ID, nodeBootstrapBundleSecretRefKey})
	readonlyDownload := sendHTTPPostgresBundleRequest(t, handler, nethttp.MethodPost, "/api/v1/nodes/"+nodeA.ID+"/bootstrap-runs/"+runID+"/bundle/download", readonlyToken, true, "")
	if readonlyDownload.Code != 403 {
		t.Fatalf("readonly download status = %d, want 403; body=%s", readonlyDownload.Code, readonlyDownload.Body.String())
	}
	assertHTTPPostgresPayloadNotContains(t, "readonly download denial", readonlyDownload.Body.Bytes(), []string{string(bundle), secret.ID, nodeBootstrapBundleSecretRefKey})

	missingCSRFReveal := sendHTTPPostgresBundleRequest(t, handler, nethttp.MethodPost, "/api/v1/nodes/"+nodeA.ID+"/bootstrap-runs/"+runID+"/bundle/reveal", adminToken, false, `{}`)
	if missingCSRFReveal.Code != 403 || !strings.Contains(missingCSRFReveal.Body.String(), "csrf protection required") {
		t.Fatalf("missing csrf reveal response mismatch: status=%d body=%s", missingCSRFReveal.Code, missingCSRFReveal.Body.String())
	}
	assertHTTPPostgresPayloadNotContains(t, "missing csrf reveal denial", missingCSRFReveal.Body.Bytes(), []string{string(bundle), secret.ID, nodeBootstrapBundleSecretRefKey})
	missingCSRFDownload := sendHTTPPostgresBundleRequest(t, handler, nethttp.MethodPost, "/api/v1/nodes/"+nodeA.ID+"/bootstrap-runs/"+runID+"/bundle/download", adminToken, false, "")
	if missingCSRFDownload.Code != 403 || !strings.Contains(missingCSRFDownload.Body.String(), "csrf protection required") {
		t.Fatalf("missing csrf download response mismatch: status=%d body=%s", missingCSRFDownload.Code, missingCSRFDownload.Body.String())
	}
	assertHTTPPostgresPayloadNotContains(t, "missing csrf download denial", missingCSRFDownload.Body.Bytes(), []string{string(bundle), secret.ID, nodeBootstrapBundleSecretRefKey})

	crossNodeReveal := sendHTTPPostgresBundleRequest(t, handler, nethttp.MethodPost, "/api/v1/nodes/"+nodeB.ID+"/bootstrap-runs/"+runID+"/bundle/reveal", adminToken, true, `{}`)
	if crossNodeReveal.Code != 404 {
		t.Fatalf("cross-node reveal status = %d, want 404; body=%s", crossNodeReveal.Code, crossNodeReveal.Body.String())
	}
	missingBundleReveal := sendHTTPPostgresBundleRequest(t, handler, nethttp.MethodPost, "/api/v1/nodes/"+nodeA.ID+"/bootstrap-runs/"+missingBundleRunID+"/bundle/reveal", adminToken, true, `{}`)
	if missingBundleReveal.Code != 404 {
		t.Fatalf("missing bundle reveal status = %d, want 404; body=%s", missingBundleReveal.Code, missingBundleReveal.Body.String())
	}
	assertHTTPPostgresPayloadNotContains(t, "cross-node reveal denial", crossNodeReveal.Body.Bytes(), []string{string(bundle), secret.ID, nodeBootstrapBundleSecretRefKey})
	assertHTTPPostgresPayloadNotContains(t, "missing-bundle reveal denial", missingBundleReveal.Body.Bytes(), []string{string(bundle), secret.ID, nodeBootstrapBundleSecretRefKey})
	if got := countHTTPPostgresBundleAudits(t, ctx, pool, nodeA.ID); got != auditBefore+3 {
		t.Fatalf("bundle audit count after rejected requests = %d, want %d", got, auditBefore+3)
	}
	assertHTTPPostgresPayloadNotContains(t, "server logs", []byte(logs.String()), []string{
		string(bundle),
		"MEGAVPN_TEST_ONLY",
		secret.ID,
		nodeBootstrapBundleSecretRefKey,
		"ciphertext",
		"key_version",
		"nonce",
	})
}

func insertHTTPPostgresBootstrapRun(t *testing.T, ctx context.Context, pool *pgxpool.Pool, nodeID string, runID string, secretRefID *string, includeAgentEnv bool) {
	t.Helper()

	result := map[string]any{}
	if secretRefID != nil {
		result[nodeBootstrapBundleSecretRefKey] = *secretRefID
	}
	if includeAgentEnv {
		result["agent_env"] = "MEGAVPN_AGENT_CONTROL_PLANE_URL=https://control.example.test"
		result["agent_bootstrapenv_available"] = true
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal bootstrap run result payload: %v", err)
	}
	if _, err := pool.Exec(ctx, `insert into node_bootstrap_runs(
  id,node_id,status,bootstrap_mode,request_payload_json,result_payload_json,started_at,finished_at,created_at
) values(
  $1,$2,'succeeded','manual_bundle','{}'::jsonb,$3::jsonb,now(),now(),now()
)`, runID, nodeID, string(resultJSON)); err != nil {
		t.Fatalf("insert bootstrap run fixture: %v", err)
	}
}

func sendHTTPPostgresBundleRequest(t *testing.T, handler nethttp.Handler, method, path, sessionToken string, csrf bool, body string) *httptest.ResponseRecorder {
	t.Helper()

	var reader io.Reader = nethttp.NoBody
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.AddCookie(&nethttp.Cookie{Name: "megavpn_session", Value: sessionToken})
	if csrf {
		req.Header.Set("X-MegaVPN-CSRF", "1")
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func findHTTPPostgresBootstrapRun(t *testing.T, runs []domain.NodeBootstrapRun, runID string) domain.NodeBootstrapRun {
	t.Helper()

	for _, run := range runs {
		if run.ID == runID {
			return run
		}
	}
	t.Fatalf("bootstrap run %s not found in response", runID)
	return domain.NodeBootstrapRun{}
}

func assertHTTPPostgresPayloadNotContains(t *testing.T, label string, payload []byte, forbidden []string) {
	t.Helper()

	for i, item := range forbidden {
		if item == "" {
			continue
		}
		if bytes.Contains(payload, []byte(item)) {
			t.Fatalf("%s leaked forbidden marker index=%d len=%d", label, i, len(item))
		}
	}
}

func assertSafeHTTPPostgresBundleFilename(t *testing.T, filename string) {
	t.Helper()

	if strings.TrimSpace(filename) == "" {
		t.Fatal("bundle filename is empty")
	}
	if strings.ContainsAny(filename, "\r\n/\\") || strings.Contains(filename, "..") || !strings.HasSuffix(filename, ".env") {
		t.Fatalf("unsafe bundle filename %q", filename)
	}
}

func countHTTPPostgresBundleAudits(t *testing.T, ctx context.Context, pool *pgxpool.Pool, nodeID string) int {
	t.Helper()

	var count int
	if err := pool.QueryRow(ctx, `select count(*) from audit_events where resource_type='node' and resource_id=$1 and action in ($2,$3)`,
		nodeID, nodeBootstrapBundleRevealAction, nodeBootstrapBundleDownloadAction).Scan(&count); err != nil {
		t.Fatalf("count bootstrap bundle audits: %v", err)
	}
	return count
}

func assertHTTPPostgresBundleAuditEvents(t *testing.T, ctx context.Context, pool *pgxpool.Pool, nodeID, actorUserID, runID, secretRefID string, wantCount int) {
	t.Helper()

	rows, err := pool.Query(ctx, `select actor_user_id,action,resource_type,resource_id,summary,payload_json::text
from audit_events
where resource_type='node' and resource_id=$1 and action in ($2,$3)
order by created_at,id`, nodeID, nodeBootstrapBundleRevealAction, nodeBootstrapBundleDownloadAction)
	if err != nil {
		t.Fatalf("load bootstrap bundle audit events: %v", err)
	}
	defer rows.Close()

	got := 0
	revealCount := 0
	downloadCount := 0
	for rows.Next() {
		got++
		var actor *string
		var action, resourceType, resourceID, summary, payload string
		if err := rows.Scan(&actor, &action, &resourceType, &resourceID, &summary, &payload); err != nil {
			t.Fatalf("scan bootstrap bundle audit event: %v", err)
		}
		if actor == nil || *actor != actorUserID || resourceType != "node" || resourceID != nodeID || !strings.Contains(summary, runID) {
			t.Fatalf("bootstrap bundle audit event metadata mismatch: actor_match=%t resource_type=%q resource_match=%t summary_has_run=%t",
				actor != nil && *actor == actorUserID, resourceType, resourceID == nodeID, strings.Contains(summary, runID))
		}
		assertHTTPPostgresPayloadNotContains(t, "audit event", []byte(summary+"\n"+payload), []string{
			"MEGAVPN_TEST_ONLY",
			secretRefID,
			nodeBootstrapBundleSecretRefKey,
			"agent_bootstrapenv",
			"ciphertext",
			"key_version",
			"nonce",
		})
		switch action {
		case nodeBootstrapBundleRevealAction:
			revealCount++
		case nodeBootstrapBundleDownloadAction:
			downloadCount++
		default:
			t.Fatalf("unexpected bootstrap bundle audit action %q", action)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate bootstrap bundle audit events: %v", err)
	}
	if got != wantCount || revealCount != 2 || downloadCount != 1 {
		t.Fatalf("bootstrap bundle audit counts got total=%d reveal=%d download=%d want total=%d reveal=2 download=1", got, revealCount, downloadCount, wantCount)
	}
}
