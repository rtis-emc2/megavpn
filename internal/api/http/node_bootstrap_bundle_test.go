package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/infra/postgres"
)

type nodeBootstrapBundleHTTPTestStore struct {
	Store

	permissions []string

	node      domain.Node
	nodeErr   error
	run       domain.NodeBootstrapRun
	runErr    error
	unsafeRun bool
	secret    domain.SecretRef
	bundle    []byte
	secretErr error
	auditErr  error

	getRunCalls      int
	auditActions     []string
	auditResourceIDs []string
	auditSummaries   []string
}

func newNodeBootstrapBundleHTTPTestStore() *nodeBootstrapBundleHTTPTestStore {
	return &nodeBootstrapBundleHTTPTestStore{
		permissions: []string{"node.bootstrap"},
		node: domain.Node{
			ID:   "node-1",
			Name: "Prod Node / West\r\n",
		},
		run: domain.NodeBootstrapRun{
			ID:            "run-1",
			NodeID:        "node-1",
			Status:        "succeeded",
			BootstrapMode: "manual_bundle",
			ResultPayload: map[string]any{
				"agent_bootstrapenv_secret_ref_id": "secret-1",
				"agent_env":                        "MEGAVPN_AGENT_CONTROL_PLANE_URL=https://control.example.test",
				"enrollment_hint":                  "hint-only",
			},
			CreatedAt: time.Unix(100, 0).UTC(),
		},
		secret: domain.SecretRef{
			ID:         "secret-1",
			SecretType: "opaque",
			Meta: map[string]any{
				"node_id":  "node-1",
				"material": "agent_bootstrap_env",
			},
			CreatedAt: time.Unix(100, 0).UTC(),
		},
		bundle: []byte("TEST_BOOTSTRAP_BUNDLE_TOKEN=synthetic-token\nTEST_BOOTSTRAP_NODE_ID=node-1\n"),
	}
}

func (s *nodeBootstrapBundleHTTPTestStore) ResolveAuthContext(context.Context, string) (domain.AuthContext, error) {
	return domain.AuthContext{
		User:            domain.PlatformUser{ID: "user-1", Username: "operator"},
		Session:         domain.UserSession{ID: "session-1", UserID: "user-1", ExpiresAt: time.Now().Add(time.Hour)},
		PermissionCodes: append([]string(nil), s.permissions...),
	}, nil
}

func (s *nodeBootstrapBundleHTTPTestStore) GetNode(context.Context, string) (domain.Node, error) {
	if s.nodeErr != nil {
		return domain.Node{}, s.nodeErr
	}
	return s.node, nil
}

func (s *nodeBootstrapBundleHTTPTestStore) GetNodeBootstrapRun(_ context.Context, nodeID string, runID string) (domain.NodeBootstrapRun, error) {
	s.getRunCalls++
	if s.runErr != nil {
		return domain.NodeBootstrapRun{}, s.runErr
	}
	if !s.unsafeRun && (nodeID != s.run.NodeID || runID != s.run.ID) {
		return domain.NodeBootstrapRun{}, pgx.ErrNoRows
	}
	return s.run, nil
}

func (s *nodeBootstrapBundleHTTPTestStore) ResolveSecretValue(context.Context, string) (domain.SecretRef, []byte, error) {
	if s.secretErr != nil {
		return domain.SecretRef{}, nil, s.secretErr
	}
	return s.secret, append([]byte(nil), s.bundle...), nil
}

func (s *nodeBootstrapBundleHTTPTestStore) CreateAuditForUser(_ context.Context, _ *string, action string, _ string, resourceID *string, summary string) (domain.AuditEvent, error) {
	if s.auditErr != nil {
		return domain.AuditEvent{}, s.auditErr
	}
	s.auditActions = append(s.auditActions, action)
	if resourceID != nil {
		s.auditResourceIDs = append(s.auditResourceIDs, *resourceID)
	}
	s.auditSummaries = append(s.auditSummaries, summary)
	return domain.AuditEvent{ID: "audit-1", Action: action, CreatedAt: time.Now().UTC()}, nil
}

func TestNodeBootstrapBundleRejectsUnauthenticatedRequests(t *testing.T) {
	t.Parallel()

	store := newNodeBootstrapBundleHTTPTestStore()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(nethttp.MethodPost, "/api/v1/nodes/node-1/bootstrap-runs/run-1/bundle/reveal", strings.NewReader(`{}`))
	newNodeBootstrapBundleTestHandler(store).ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Fatalf("status = %d, want 401; body=%s", rec.Code, rec.Body.String())
	}
	if store.getRunCalls != 0 {
		t.Fatal("store must not resolve bundle without authentication")
	}
	assertNodeBootstrapBundleNoStoreHeaders(t, rec)
}

func TestNodeBootstrapBundlePostEndpointsRequireNodeBootstrap(t *testing.T) {
	t.Parallel()

	for _, path := range []string{
		"/api/v1/nodes/node-1/bootstrap-runs/run-1/bundle/reveal",
		"/api/v1/nodes/node-1/bootstrap-runs/run-1/bundle/download",
	} {
		path := path
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			store := newNodeBootstrapBundleHTTPTestStore()
			store.permissions = []string{"node.read"}
			rec := httptest.NewRecorder()
			req := newNodeBootstrapBundleRequest(nethttp.MethodPost, path, `{}`)
			newNodeBootstrapBundleTestHandler(store).ServeHTTP(rec, req)
			if rec.Code != 403 {
				t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
			}
			if store.getRunCalls != 0 {
				t.Fatal("store must not resolve bundle without node.bootstrap permission")
			}
		})
	}
}

func TestNodeBootstrapBundlePostEndpointsCookieAuthRequiresCSRF(t *testing.T) {
	t.Parallel()

	for _, path := range []string{
		"/api/v1/nodes/node-1/bootstrap-runs/run-1/bundle/reveal",
		"/api/v1/nodes/node-1/bootstrap-runs/run-1/bundle/download",
	} {
		path := path
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			store := newNodeBootstrapBundleHTTPTestStore()
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(nethttp.MethodPost, path, strings.NewReader(`{}`))
			req.AddCookie(&nethttp.Cookie{Name: "megavpn_session", Value: "test-session"})
			newNodeBootstrapBundleTestHandler(store).ServeHTTP(rec, req)
			if rec.Code != 403 {
				t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "csrf protection required") {
				t.Fatalf("csrf error body = %s", rec.Body.String())
			}
			if store.getRunCalls != 0 {
				t.Fatal("store must not resolve bundle when CSRF rejects request")
			}
			assertNodeBootstrapBundleNoStoreHeaders(t, rec)
		})
	}
}

func TestNodeBootstrapBundlePostRevealRejectsCallerFields(t *testing.T) {
	t.Parallel()

	store := newNodeBootstrapBundleHTTPTestStore()
	rec := httptest.NewRecorder()
	req := newNodeBootstrapBundleRequest(nethttp.MethodPost, "/api/v1/nodes/node-1/bootstrap-runs/run-1/bundle/reveal", `{"secret_ref_id":"attacker","filename":"x.env","content":"x"}`)
	newNodeBootstrapBundleTestHandler(store).ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if store.getRunCalls != 0 {
		t.Fatal("store must not resolve bundle for caller-supplied fields")
	}
	assertNodeBootstrapBundleNoStoreHeaders(t, rec)
}

func TestNodeBootstrapBundlePostRevealReturnsSafeDTO(t *testing.T) {
	t.Parallel()

	store := newNodeBootstrapBundleHTTPTestStore()
	rec := httptest.NewRecorder()
	req := newNodeBootstrapBundleRequest(nethttp.MethodPost, "/api/v1/nodes/node-1/bootstrap-runs/run-1/bundle/reveal", `{}`)
	newNodeBootstrapBundleTestHandler(store).ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	assertNodeBootstrapBundleNoStoreHeaders(t, rec)
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["node_id"] != "node-1" || payload["bootstrap_run_id"] != "run-1" {
		t.Fatalf("unexpected identity fields: %#v", payload)
	}
	if payload["agent_bootstrapenv"] != string(store.bundle) {
		t.Fatalf("agent_bootstrapenv mismatch: %#v", payload)
	}
	if payload["filename"] != "megavpn-agent-Prod-Node-West-bootstrap.env" {
		t.Fatalf("filename = %#v", payload["filename"])
	}
	for _, forbidden := range []string{"secret_ref_id", "secret-1", "agent_env", "enrollment_hint", "ciphertext", "key_version", "meta"} {
		if strings.Contains(rec.Body.String(), forbidden) {
			t.Fatalf("response leaked %q: %s", forbidden, rec.Body.String())
		}
	}
	if len(store.auditActions) != 1 || store.auditActions[0] != "node.bootstrap_bundle.reveal" {
		t.Fatalf("audit actions = %#v", store.auditActions)
	}
	if len(store.auditResourceIDs) != 1 || store.auditResourceIDs[0] != "node-1" {
		t.Fatalf("audit resource ids = %#v", store.auditResourceIDs)
	}
	if len(store.auditSummaries) != 1 || !strings.Contains(store.auditSummaries[0], "run-1") || strings.Contains(store.auditSummaries[0], "secret-1") {
		t.Fatalf("audit summaries = %#v", store.auditSummaries)
	}
}

func TestNodeBootstrapBundleGetCompatibilityUsesHardenedReveal(t *testing.T) {
	t.Parallel()

	store := newNodeBootstrapBundleHTTPTestStore()
	rec := httptest.NewRecorder()
	req := newNodeBootstrapBundleRequest(nethttp.MethodGet, "/api/v1/nodes/node-1/bootstrap-runs/run-1/bundle", "")
	newNodeBootstrapBundleTestHandler(store).ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	assertNodeBootstrapBundleNoStoreHeaders(t, rec)
	if strings.Contains(rec.Body.String(), "secret_ref_id") || strings.Contains(rec.Body.String(), "agent_env") || strings.Contains(rec.Body.String(), "enrollment_hint") {
		t.Fatalf("GET compatibility response leaked internal fields: %s", rec.Body.String())
	}
	if len(store.auditActions) != 1 || store.auditActions[0] != "node.bootstrap_bundle.reveal" {
		t.Fatalf("audit actions = %#v", store.auditActions)
	}
}

func TestNodeBootstrapBundleDownloadReturnsExactBundleBytes(t *testing.T) {
	t.Parallel()

	store := newNodeBootstrapBundleHTTPTestStore()
	store.bundle = []byte("A=1\r\nB=2\n")
	rec := httptest.NewRecorder()
	req := newNodeBootstrapBundleRequest(nethttp.MethodPost, "/api/v1/nodes/node-1/bootstrap-runs/run-1/bundle/download", ``)
	newNodeBootstrapBundleTestHandler(store).ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Equal(rec.Body.Bytes(), []byte("A=1\r\nB=2\n")) {
		t.Fatalf("download body = %#v", rec.Body.Bytes())
	}
	assertNodeBootstrapBundleNoStoreHeaders(t, rec)
	if got := rec.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := rec.Header().Get("Content-Disposition"); !strings.Contains(got, `attachment`) || !strings.Contains(got, `megavpn-agent-Prod-Node-West-bootstrap.env`) {
		t.Fatalf("Content-Disposition = %q", got)
	}
	if len(store.auditActions) != 1 || store.auditActions[0] != "node.bootstrap_bundle.download" {
		t.Fatalf("audit actions = %#v", store.auditActions)
	}
}

func TestNodeBootstrapBundleAuditFailureDoesNotWriteSecret(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "reveal", method: nethttp.MethodPost, path: "/api/v1/nodes/node-1/bootstrap-runs/run-1/bundle/reveal", body: `{}`},
		{name: "download", method: nethttp.MethodPost, path: "/api/v1/nodes/node-1/bootstrap-runs/run-1/bundle/download", body: `{}`},
		{name: "get", method: nethttp.MethodGet, path: "/api/v1/nodes/node-1/bootstrap-runs/run-1/bundle"},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store := newNodeBootstrapBundleHTTPTestStore()
			store.auditErr = errors.New("audit unavailable")
			rec := httptest.NewRecorder()
			req := newNodeBootstrapBundleRequest(tc.method, tc.path, tc.body)
			newNodeBootstrapBundleTestHandler(store).ServeHTTP(rec, req)
			if rec.Code != 500 {
				t.Fatalf("status = %d, want 500; body=%s", rec.Code, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "synthetic-token") || strings.Contains(rec.Body.String(), "A=1") {
				t.Fatalf("audit failure leaked secret body: %s", rec.Body.String())
			}
			assertNodeBootstrapBundleNoStoreHeaders(t, rec)
		})
	}
}

func TestNodeBootstrapBundleRepeatedRevealsAreAudited(t *testing.T) {
	t.Parallel()

	store := newNodeBootstrapBundleHTTPTestStore()
	handler := newNodeBootstrapBundleTestHandler(store)
	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		req := newNodeBootstrapBundleRequest(nethttp.MethodPost, "/api/v1/nodes/node-1/bootstrap-runs/run-1/bundle/reveal", `{}`)
		handler.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("attempt %d status = %d, want 200; body=%s", i+1, rec.Code, rec.Body.String())
		}
	}
	if len(store.auditActions) != 2 {
		t.Fatalf("audit actions = %#v, want 2 reveal audits", store.auditActions)
	}
}

func TestNodeBootstrapBundleLogsDoNotContainBundleContent(t *testing.T) {
	t.Parallel()

	store := newNodeBootstrapBundleHTTPTestStore()
	var logs bytes.Buffer
	handler := New(slog.New(slog.NewTextHandler(&logs, nil)), store, Options{
		Version:           "test",
		SessionCookieName: "megavpn_session",
	})
	for _, path := range []string{
		"/api/v1/nodes/node-1/bootstrap-runs/run-1/bundle/reveal",
		"/api/v1/nodes/node-1/bootstrap-runs/run-1/bundle/download",
	} {
		rec := httptest.NewRecorder()
		req := newNodeBootstrapBundleRequest(nethttp.MethodPost, path, `{}`)
		handler.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("%s status = %d, want 200; body=%s", path, rec.Code, rec.Body.String())
		}
	}
	if strings.Contains(logs.String(), "synthetic-token") || strings.Contains(logs.String(), "TEST_BOOTSTRAP_BUNDLE_TOKEN") {
		t.Fatalf("request logs leaked bundle content: %s", logs.String())
	}
}

func TestNodeBootstrapBundleResolverRejectsUnsafeStatesAndScopes(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		mutate     func(*nodeBootstrapBundleHTTPTestStore)
		wantStatus int
	}{
		{
			name: "node not found",
			mutate: func(s *nodeBootstrapBundleHTTPTestStore) {
				s.nodeErr = pgx.ErrNoRows
			},
			wantStatus: 404,
		},
		{
			name: "run not found",
			mutate: func(s *nodeBootstrapBundleHTTPTestStore) {
				s.runErr = pgx.ErrNoRows
			},
			wantStatus: 404,
		},
		{
			name: "defensive run node mismatch",
			mutate: func(s *nodeBootstrapBundleHTTPTestStore) {
				s.unsafeRun = true
				s.run.NodeID = "node-2"
			},
			wantStatus: 403,
		},
		{
			name: "wrong mode",
			mutate: func(s *nodeBootstrapBundleHTTPTestStore) {
				s.run.BootstrapMode = "ssh_bootstrap"
			},
			wantStatus: 404,
		},
		{
			name: "not succeeded",
			mutate: func(s *nodeBootstrapBundleHTTPTestStore) {
				s.run.Status = "running"
			},
			wantStatus: 409,
		},
		{
			name: "missing secret ref",
			mutate: func(s *nodeBootstrapBundleHTTPTestStore) {
				s.run.ResultPayload = map[string]any{}
			},
			wantStatus: 404,
		},
		{
			name: "missing secret",
			mutate: func(s *nodeBootstrapBundleHTTPTestStore) {
				s.secretErr = pgx.ErrNoRows
			},
			wantStatus: 409,
		},
		{
			name: "secret service unavailable",
			mutate: func(s *nodeBootstrapBundleHTTPTestStore) {
				s.secretErr = postgres.ErrSecretServiceUnavailable
			},
			wantStatus: 503,
		},
		{
			name: "wrong type",
			mutate: func(s *nodeBootstrapBundleHTTPTestStore) {
				s.secret.SecretType = "private_key"
			},
			wantStatus: 409,
		},
		{
			name: "wrong node metadata",
			mutate: func(s *nodeBootstrapBundleHTTPTestStore) {
				s.secret.Meta["node_id"] = "node-2"
			},
			wantStatus: 403,
		},
		{
			name: "wrong material metadata",
			mutate: func(s *nodeBootstrapBundleHTTPTestStore) {
				s.secret.Meta["material"] = "agent_env"
			},
			wantStatus: 403,
		},
		{
			name: "wrong run metadata",
			mutate: func(s *nodeBootstrapBundleHTTPTestStore) {
				s.secret.Meta["bootstrap_run_id"] = "run-2"
			},
			wantStatus: 403,
		},
		{
			name: "oversized bundle",
			mutate: func(s *nodeBootstrapBundleHTTPTestStore) {
				s.bundle = bytes.Repeat([]byte("x"), maxNodeBootstrapBundleBytes+1)
			},
			wantStatus: 413,
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store := newNodeBootstrapBundleHTTPTestStore()
			tc.mutate(store)
			rec := httptest.NewRecorder()
			req := newNodeBootstrapBundleRequest(nethttp.MethodPost, "/api/v1/nodes/node-1/bootstrap-runs/run-1/bundle/reveal", `{}`)
			newNodeBootstrapBundleTestHandler(store).ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if len(store.auditActions) != 0 {
				t.Fatalf("audit should not be written after failed resolver: %#v", store.auditActions)
			}
			assertNodeBootstrapBundleNoStoreHeaders(t, rec)
		})
	}
}

func newNodeBootstrapBundleRequest(method, path, body string) *nethttp.Request {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Authorization", "Bearer test-session")
	return req
}

func newNodeBootstrapBundleTestHandler(store *nodeBootstrapBundleHTTPTestStore) nethttp.Handler {
	return New(slog.New(slog.NewTextHandler(io.Discard, nil)), store, Options{
		Version:           "test",
		SessionCookieName: "megavpn_session",
	})
}

func assertNodeBootstrapBundleNoStoreHeaders(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if got := rec.Header().Get("Cache-Control"); got != "no-store, private, max-age=0" {
		t.Fatalf("Cache-Control = %q", got)
	}
	if got := rec.Header().Get("Pragma"); got != "no-cache" {
		t.Fatalf("Pragma = %q", got)
	}
	if got := rec.Header().Get("Expires"); got != "0" {
		t.Fatalf("Expires = %q", got)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q", got)
	}
	if got := rec.Header().Get("Referrer-Policy"); got != "no-referrer" {
		t.Fatalf("Referrer-Policy = %q", got)
	}
	if got := rec.Header().Get("ETag"); got != "" {
		t.Fatalf("ETag = %q", got)
	}
	if got := rec.Header().Get("Last-Modified"); got != "" {
		t.Fatalf("Last-Modified = %q", got)
	}
}
