package http

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

type failingAuditStore struct {
	Store
	err error
}

func (s failingAuditStore) CreateAuditForUser(context.Context, *string, string, string, *string, string) (domain.AuditEvent, error) {
	return domain.AuditEvent{}, s.err
}

func TestAuditRequiredPropagatesPersistenceFailure(t *testing.T) {
	t.Parallel()

	s := &Server{store: failingAuditStore{err: errors.New("database unavailable")}}
	err := s.auditRequired(context.Background(), nil, "node.secret.reveal", "node", nil, "secret revealed")
	if err == nil || !strings.Contains(err.Error(), "database unavailable") {
		t.Fatalf("expected durable audit failure, got %v", err)
	}
}

func TestAuditBestEffortLogsPersistenceFailure(t *testing.T) {
	t.Parallel()

	var output strings.Builder
	s := &Server{
		store: failingAuditStore{err: errors.New("database unavailable")},
		log:   slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelError})),
	}
	s.auditBestEffort(context.Background(), nil, "auth.login_failed", "platform_user", nil, "login failed")
	if !strings.Contains(output.String(), "audit event persistence failed") || !strings.Contains(output.String(), "database unavailable") {
		t.Fatalf("expected structured audit failure log, got %q", output.String())
	}
}
