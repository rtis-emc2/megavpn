package http

import (
	"context"
	"encoding/json"
	"log/slog"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

type nodeAgentTokenRotateHTTPTestStore struct {
	Store
	called bool
}

func (s *nodeAgentTokenRotateHTTPTestStore) ResolveAuthContext(context.Context, string) (domain.AuthContext, error) {
	return domain.AuthContext{
		User:            domain.PlatformUser{ID: "user-1", Username: "operator"},
		Session:         domain.UserSession{ID: "session-1", UserID: "user-1", ExpiresAt: time.Now().Add(time.Hour)},
		PermissionCodes: []string{"node.bootstrap"},
	}, nil
}

func (s *nodeAgentTokenRotateHTTPTestStore) CreateNodeAgentTokenRotateJob(_ context.Context, nodeID string) (domain.Job, error) {
	s.called = true
	return domain.Job{
		ID:        "job-1",
		Type:      "node.agent.rotate_token",
		ScopeType: "node",
		ScopeID:   &nodeID,
		NodeID:    &nodeID,
		Status:    "queued",
		Priority:  15,
		Payload: map[string]any{
			"node_id":                       nodeID,
			"new_agent_token_secret_ref_id": "secret-ref-internal",
			"new_agent_token_hash":          "token-hash-internal",
			"new_token_hint":                "hint-internal",
		},
		Result:      map[string]any{"internal": true},
		LockedBy:    stringPointer("internal-owner"),
		LockedUntil: timePointer(time.Now().Add(time.Minute)),
		CreatedAt:   time.Unix(100, 0).UTC(),
	}, nil
}

func TestRotateNodeAgentTokenReturnsMetadataOnly(t *testing.T) {
	t.Parallel()

	store := &nodeAgentTokenRotateHTTPTestStore{}
	handler := New(slog.New(slog.NewTextHandler(&strings.Builder{}, nil)), store, Options{SessionCookieName: "megavpn_session"})
	req := httptest.NewRequest(nethttp.MethodPost, "/api/v1/nodes/node-1/agent-token/rotate", nethttp.NoBody)
	req.AddCookie(&nethttp.Cookie{Name: "megavpn_session", Value: "test-session"})
	req.Header.Set("X-MegaVPN-CSRF", "1")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != nethttp.StatusAccepted || !store.called {
		t.Fatalf("rotation response status/called = %d/%t, want 202/true", rec.Code, store.called)
	}
	assertHTTPPostgresNoStoreHeader(t, rec)
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode rotation metadata response: %v", err)
	}
	for _, forbidden := range []string{"payload", "result", "locked_by", "locked_until"} {
		if _, ok := body[forbidden]; ok {
			t.Fatalf("rotation metadata response exposed %s", forbidden)
		}
	}
	if body["id"] != "job-1" || body["type"] != "node.agent.rotate_token" || body["status"] != "queued" || body["node_id"] != "node-1" {
		t.Fatalf("rotation metadata response mismatch: id/type/status/node metadata not preserved")
	}
}

func stringPointer(value string) *string {
	return &value
}

func timePointer(value time.Time) *time.Time {
	return &value
}
