package http

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/infra/postgres"
)

type nodeSSHAccessMethodHTTPTestStore struct {
	Store
	permissions []string
	createErr   error
	called      bool
	input       domain.NodeSSHAccessMethodCreateInput
}

func (s *nodeSSHAccessMethodHTTPTestStore) ResolveAuthContext(context.Context, string) (domain.AuthContext, error) {
	return domain.AuthContext{
		User:            domain.PlatformUser{ID: "user-1", Username: "operator"},
		Session:         domain.UserSession{ID: "session-1", UserID: "user-1", ExpiresAt: time.Now().Add(time.Hour)},
		PermissionCodes: append([]string(nil), s.permissions...),
	}, nil
}

func (s *nodeSSHAccessMethodHTTPTestStore) CreateNodeSSHAccessMethod(_ context.Context, nodeID string, input domain.NodeSSHAccessMethodCreateInput) (domain.NodeAccessMethod, error) {
	s.called = true
	s.input = input
	if s.createErr != nil {
		return domain.NodeAccessMethod{}, s.createErr
	}
	secretID := "11111111-1111-1111-1111-111111111111"
	return domain.NodeAccessMethod{
		ID:               "22222222-2222-2222-2222-222222222222",
		NodeID:           nodeID,
		Method:           "ssh",
		IsEnabled:        input.IsEnabled,
		SSHHost:          strings.TrimSpace(input.SSHHost),
		SSHPort:          22,
		SSHUser:          strings.TrimSpace(input.SSHUser),
		SSHHostKeySHA256: strings.TrimSpace(input.SSHHostKeySHA256),
		AuthType:         "ssh_key",
		SecretRefID:      &secretID,
		CreatedAt:        time.Unix(100, 0).UTC(),
		UpdatedAt:        time.Unix(100, 0).UTC(),
	}, nil
}

func TestCreateNodeSSHAccessMethodRouteRequiresNodeBootstrap(t *testing.T) {
	t.Parallel()

	store := &nodeSSHAccessMethodHTTPTestStore{permissions: []string{"node.read"}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(nethttp.MethodPost, "/api/v1/nodes/node-1/access-methods/ssh", strings.NewReader(validNodeSSHAccessMethodPayload()))
	req.Header.Set("Authorization", "Bearer test-session")
	newNodeSSHAccessMethodTestHandler(store).ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
	if store.called {
		t.Fatal("store must not be called without node.bootstrap permission")
	}
}

func TestCreateNodeSSHAccessMethodCookieAuthRequiresCSRF(t *testing.T) {
	t.Parallel()

	store := &nodeSSHAccessMethodHTTPTestStore{permissions: []string{"node.bootstrap"}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(nethttp.MethodPost, "/api/v1/nodes/node-1/access-methods/ssh", strings.NewReader(validNodeSSHAccessMethodPayload()))
	req.AddCookie(&nethttp.Cookie{Name: "megavpn_session", Value: "test-session"})
	newNodeSSHAccessMethodTestHandler(store).ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "csrf protection required") {
		t.Fatalf("csrf error body = %s", rec.Body.String())
	}
	if store.called {
		t.Fatal("store must not be called when CSRF protection rejects request")
	}
}

func TestCreateNodeSSHAccessMethodRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	store := &nodeSSHAccessMethodHTTPTestStore{permissions: []string{"node.bootstrap"}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(nethttp.MethodPost, "/api/v1/nodes/node-1/access-methods/ssh", strings.NewReader(`{"ssh_host":"host.example","ssh_user":"support","ssh_host_key_sha256":"SHA256:abcdefghijklmnopqrstuvwxyzABCDEFGH1234567890+/=","private_key":"key","secret_ref_id":"caller-supplied"}`))
	req.Header.Set("Authorization", "Bearer test-session")
	newNodeSSHAccessMethodTestHandler(store).ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if store.called {
		t.Fatal("store must not be called for unknown request fields")
	}
}

func TestCreateNodeSSHAccessMethodReturnsRedactedResponse(t *testing.T) {
	t.Parallel()

	store := &nodeSSHAccessMethodHTTPTestStore{permissions: []string{"node.bootstrap"}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(nethttp.MethodPost, "/api/v1/nodes/node-1/access-methods/ssh", strings.NewReader(validNodeSSHAccessMethodPayload()))
	req.Header.Set("Authorization", "Bearer test-session")
	newNodeSSHAccessMethodTestHandler(store).ServeHTTP(rec, req)
	if rec.Code != 201 {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	if !store.called {
		t.Fatal("store should be called")
	}
	if store.input.ActorUserID == nil || *store.input.ActorUserID != "user-1" {
		t.Fatalf("actor user id = %#v", store.input.ActorUserID)
	}
	body := rec.Body.String()
	for _, forbidden := range []string{"private_key", "secret_ref_id", "ciphertext", "nonce", "key_version", "11111111-1111-1111-1111-111111111111"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("response leaked %q: %s", forbidden, body)
		}
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["secret_configured"] != true {
		t.Fatalf("secret_configured = %#v, want true", payload["secret_configured"])
	}
	if payload["method"] != "ssh" || payload["auth_type"] != "ssh_key" {
		t.Fatalf("response method/auth_type = %#v", payload)
	}
}

func TestCreateNodeSSHAccessMethodMapsSecretServiceUnavailable(t *testing.T) {
	t.Parallel()

	store := &nodeSSHAccessMethodHTTPTestStore{permissions: []string{"node.bootstrap"}, createErr: postgres.ErrSecretServiceUnavailable}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(nethttp.MethodPost, "/api/v1/nodes/node-1/access-methods/ssh", strings.NewReader(validNodeSSHAccessMethodPayload()))
	req.Header.Set("Authorization", "Bearer test-session")
	newNodeSSHAccessMethodTestHandler(store).ServeHTTP(rec, req)
	if rec.Code != 503 {
		t.Fatalf("status = %d, want 503; body=%s", rec.Code, rec.Body.String())
	}
}

func newNodeSSHAccessMethodTestHandler(store *nodeSSHAccessMethodHTTPTestStore) nethttp.Handler {
	return New(slog.New(slog.NewTextHandler(io.Discard, nil)), store, Options{
		Version:           "test",
		SessionCookieName: "megavpn_session",
	})
}

func validNodeSSHAccessMethodPayload() string {
	return `{"ssh_host":"203.0.113.10","ssh_port":22,"ssh_user":"support","ssh_host_key_sha256":"SHA256:abcdefghijklmnopqrstuvwxyzABCDEFGH1234567890+/=","private_key":"test-key-material","is_enabled":true}`
}
