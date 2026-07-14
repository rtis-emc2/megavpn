package http

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"io"
	"log/slog"
	nethttp "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	authn "github.com/rtis-emc2/megavpn/internal/auth"
	"github.com/rtis-emc2/megavpn/internal/domain"
	pgstore "github.com/rtis-emc2/megavpn/internal/infra/postgres"
	"github.com/rtis-emc2/megavpn/internal/platform/id"
	"github.com/rtis-emc2/megavpn/internal/secrets"
	"golang.org/x/crypto/ssh"
)

func TestPostgresIntegrationCreateNodeSSHAccessMethodHTTP(t *testing.T) {
	store, pool, ctx := setupHTTPPostgresIntegrationStore(t)
	attachHTTPPostgresIntegrationSecretService(t, store)

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	admin, _, err := store.EnsureBootstrapPlatformUser(ctx, "it-admin-"+suffix, "it-admin-"+suffix+"@example.invalid", "Integration Admin", "integration-password-hash")
	if err != nil {
		t.Fatalf("create bootstrap admin: %v", err)
	}
	adminToken := createHTTPPostgresIntegrationSession(t, ctx, store, admin.ID)
	readonly, err := store.CreatePlatformUser(ctx, "it-readonly-"+suffix, "it-readonly-"+suffix+"@example.invalid", "Integration Readonly", "integration-password-hash", []string{"readonly"}, &admin.ID)
	if err != nil {
		t.Fatalf("create readonly user: %v", err)
	}
	readonlyToken := createHTTPPostgresIntegrationSession(t, ctx, store, readonly.ID)

	node, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-http-ssh-access-" + suffix,
		Kind:          "remote",
		Role:          "egress",
		Status:        "draft",
		Address:       "203.0.113.51",
		OSFamily:      "linux",
		OSVersion:     "ubuntu-24.04",
		Architecture:  "amd64",
		ExecutionMode: "ssh_bootstrap",
		AgentStatus:   "unknown",
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}

	handler := newHTTPPostgresNodeSSHAccessMethodHandler(store)
	privateKey := generatedHTTPOpenSSHPrivateKey(t)
	payload := map[string]any{
		"ssh_host":            "203.0.113.51",
		"ssh_port":            22,
		"ssh_user":            "support",
		"ssh_host_key_sha256": "SHA256:abcdefghijklmnopqrstuvwxyzABCDEFGH1234567890+/=",
		"private_key":         privateKey,
		"is_enabled":          true,
	}

	rec := postHTTPPostgresNodeSSHAccessMethod(t, handler, node.ID, adminToken, true, payload)
	if rec.Code != 201 {
		t.Fatalf("create status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, forbidden := range []string{"private_key", "secret_ref_id", "ciphertext", "nonce", "key_version"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("response leaked forbidden field %q", forbidden)
		}
	}
	if strings.Contains(body, "BEGIN OPENSSH PRIVATE KEY") || strings.Contains(body, privateKey) {
		t.Fatal("response leaked submitted private key material")
	}

	var response map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if response["secret_configured"] != true {
		t.Fatalf("secret_configured = %#v, want true", response["secret_configured"])
	}
	if response["method"] != "ssh" || response["auth_type"] != "ssh_key" || response["node_id"] != node.ID {
		t.Fatalf("unexpected response metadata: %#v", response)
	}

	assertHTTPPostgresCount(t, ctx, pool, `select count(*) from node_access_methods where node_id=$1 and method='ssh'`, 1, node.ID)
	assertHTTPPostgresCount(t, ctx, pool, `select count(*) from secret_refs where secret_type='ssh_key' and meta_json->>'node_id'=$1 and meta_json->>'purpose'='node_ssh_bootstrap'`, 1, node.ID)

	var accessMethodID string
	var secretRefID string
	if err := pool.QueryRow(ctx, `select id, secret_ref_id from node_access_methods where node_id=$1 and method='ssh' and ssh_host=$2 and ssh_user=$3`, node.ID, "203.0.113.51", "support").Scan(&accessMethodID, &secretRefID); err != nil {
		t.Fatalf("select created ssh access method: %v", err)
	}
	if response["id"] != accessMethodID {
		t.Fatalf("response id = %#v, want persisted access method id", response["id"])
	}

	var secretType string
	var ciphertext []byte
	var keyVersion string
	var nonce []byte
	if err := pool.QueryRow(ctx, `select secret_type, ciphertext, key_version, nonce from secret_refs where id=$1`, secretRefID).Scan(&secretType, &ciphertext, &keyVersion, &nonce); err != nil {
		t.Fatalf("select secret ref: %v", err)
	}
	if secretType != "ssh_key" {
		t.Fatalf("secret_type = %q, want ssh_key", secretType)
	}
	if keyVersion == "" || len(nonce) == 0 {
		t.Fatal("secret ref must include key version and nonce")
	}
	if bytes.Equal(ciphertext, []byte(privateKey)) {
		t.Fatal("stored ciphertext must not equal submitted private key")
	}
	ref, plaintext, err := store.ResolveSecretValue(ctx, secretRefID)
	if err != nil {
		t.Fatalf("resolve secret value: %v", err)
	}
	if ref.SecretType != "ssh_key" || string(plaintext) != privateKey {
		t.Fatalf("resolved secret mismatch: ref type=%q", ref.SecretType)
	}

	var auditSummary string
	var auditPayload string
	if err := pool.QueryRow(ctx, `select summary, payload_json::text from audit_events where action='node.ssh_access_method.create' and resource_id=$1 order by created_at desc limit 1`, node.ID).Scan(&auditSummary, &auditPayload); err != nil {
		t.Fatalf("select create audit event: %v", err)
	}
	auditText := auditSummary + "\n" + auditPayload
	if !strings.Contains(auditSummary, accessMethodID) {
		t.Fatalf("audit summary does not reference access method id: %q", auditSummary)
	}
	if strings.Contains(auditText, "BEGIN OPENSSH PRIVATE KEY") || strings.Contains(auditText, privateKey) {
		t.Fatal("audit event leaked submitted private key material")
	}

	duplicatePayload := cloneHTTPPostgresSSHAccessPayload(payload)
	duplicatePayload["private_key"] = generatedHTTPOpenSSHPrivateKey(t)
	rec = postHTTPPostgresNodeSSHAccessMethod(t, handler, node.ID, adminToken, true, duplicatePayload)
	if rec.Code != 409 {
		t.Fatalf("duplicate status = %d, want 409; body=%s", rec.Code, rec.Body.String())
	}
	assertHTTPPostgresCount(t, ctx, pool, `select count(*) from node_access_methods where node_id=$1 and method='ssh'`, 1, node.ID)
	assertHTTPPostgresCount(t, ctx, pool, `select count(*) from secret_refs where secret_type='ssh_key' and meta_json->>'node_id'=$1 and meta_json->>'purpose'='node_ssh_bootstrap'`, 1, node.ID)

	invalidPayload := cloneHTTPPostgresSSHAccessPayload(payload)
	invalidPayload["ssh_host"] = "203.0.113.52"
	invalidPayload["private_key"] = "not-a-private-key"
	rec = postHTTPPostgresNodeSSHAccessMethod(t, handler, node.ID, adminToken, true, invalidPayload)
	if rec.Code != 400 {
		t.Fatalf("invalid key status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	assertHTTPPostgresCount(t, ctx, pool, `select count(*) from node_access_methods where node_id=$1 and method='ssh' and ssh_host=$2`, 0, node.ID, "203.0.113.52")
	assertHTTPPostgresCount(t, ctx, pool, `select count(*) from secret_refs where secret_type='ssh_key' and meta_json->>'node_id'=$1 and meta_json->>'purpose'='node_ssh_bootstrap'`, 1, node.ID)

	noPermissionPayload := cloneHTTPPostgresSSHAccessPayload(payload)
	noPermissionPayload["ssh_host"] = "203.0.113.53"
	noPermissionPayload["private_key"] = generatedHTTPOpenSSHPrivateKey(t)
	rec = postHTTPPostgresNodeSSHAccessMethod(t, handler, node.ID, readonlyToken, true, noPermissionPayload)
	if rec.Code != 403 {
		t.Fatalf("no-permission status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
	assertHTTPPostgresCount(t, ctx, pool, `select count(*) from node_access_methods where node_id=$1 and method='ssh' and ssh_host=$2`, 0, node.ID, "203.0.113.53")

	missingCSRFPayload := cloneHTTPPostgresSSHAccessPayload(payload)
	missingCSRFPayload["ssh_host"] = "203.0.113.54"
	missingCSRFPayload["private_key"] = generatedHTTPOpenSSHPrivateKey(t)
	rec = postHTTPPostgresNodeSSHAccessMethod(t, handler, node.ID, adminToken, false, missingCSRFPayload)
	if rec.Code != 403 {
		t.Fatalf("missing-csrf status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "csrf protection required") {
		t.Fatalf("missing-csrf body = %s", rec.Body.String())
	}
	assertHTTPPostgresCount(t, ctx, pool, `select count(*) from node_access_methods where node_id=$1 and method='ssh'`, 1, node.ID)
	assertHTTPPostgresCount(t, ctx, pool, `select count(*) from secret_refs where secret_type='ssh_key' and meta_json->>'node_id'=$1 and meta_json->>'purpose'='node_ssh_bootstrap'`, 1, node.ID)
}

func setupHTTPPostgresIntegrationStore(t *testing.T) (*pgstore.Store, *pgxpool.Pool, context.Context) {
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
	schema := "megavpn_http_it_" + strings.ReplaceAll(id.New(), "-", "")[:16]
	if _, err := adminPool.Exec(ctx, "create schema "+quoteHTTPPostgresIdentifier(schema)); err != nil {
		adminPool.Close()
		t.Fatalf("create test schema: %v", err)
	}
	t.Cleanup(func() {
		dropCtx, dropCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer dropCancel()
		_, _ = adminPool.Exec(dropCtx, "drop schema if exists "+quoteHTTPPostgresIdentifier(schema)+" cascade")
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

	applyHTTPPostgresIntegrationMigrations(t, ctx, pool)
	return pgstore.New(pool), pool, ctx
}

func attachHTTPPostgresIntegrationSecretService(t *testing.T, store *pgstore.Store) {
	t.Helper()

	keyPath := filepath.Join(t.TempDir(), "master.key")
	rawKey := make([]byte, 32)
	if _, err := rand.Read(rawKey); err != nil {
		t.Fatalf("generate test master key: %v", err)
	}
	if err := os.WriteFile(keyPath, []byte(hex.EncodeToString(rawKey)), 0o600); err != nil {
		t.Fatalf("write test master key: %v", err)
	}
	secretSvc, err := secrets.LoadFromFile(keyPath, "test-v1")
	if err != nil {
		t.Fatalf("load test secret service: %v", err)
	}
	store.SetSecretService(secretSvc)
}

func applyHTTPPostgresIntegrationMigrations(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
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

func createHTTPPostgresIntegrationSession(t *testing.T, ctx context.Context, store *pgstore.Store, userID string) string {
	t.Helper()

	token, tokenHash, err := authn.NewSessionToken()
	if err != nil {
		t.Fatalf("create session token: %v", err)
	}
	if _, err := store.CreateUserSession(ctx, userID, tokenHash, "127.0.0.1", "postgres-integration-test", time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatalf("create user session: %v", err)
	}
	return token
}

func postHTTPPostgresNodeSSHAccessMethod(t *testing.T, handler nethttp.Handler, nodeID, sessionToken string, csrf bool, payload map[string]any) *httptest.ResponseRecorder {
	t.Helper()

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal request payload: %v", err)
	}
	req := httptest.NewRequest(nethttp.MethodPost, "/api/v1/nodes/"+nodeID+"/access-methods/ssh", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&nethttp.Cookie{Name: "megavpn_session", Value: sessionToken})
	if csrf {
		req.Header.Set("X-MegaVPN-CSRF", "1")
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func newHTTPPostgresNodeSSHAccessMethodHandler(store *pgstore.Store) nethttp.Handler {
	return New(slog.New(slog.NewTextHandler(io.Discard, nil)), store, Options{
		Version:           "test",
		SessionCookieName: "megavpn_session",
	})
}

func assertHTTPPostgresCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, query string, want int, args ...any) {
	t.Helper()

	var got int
	if err := pool.QueryRow(ctx, query, args...).Scan(&got); err != nil {
		t.Fatalf("count query failed: %v", err)
	}
	if got != want {
		t.Fatalf("count = %d, want %d", got, want)
	}
}

func quoteHTTPPostgresIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func cloneHTTPPostgresSSHAccessPayload(payload map[string]any) map[string]any {
	out := make(map[string]any, len(payload))
	for key, value := range payload {
		out[key] = value
	}
	return out
}

func generatedHTTPOpenSSHPrivateKey(t *testing.T) string {
	t.Helper()

	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	block, err := ssh.MarshalPrivateKey(key, "generated-http-integration-key")
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	return string(pem.EncodeToMemory(block))
}
