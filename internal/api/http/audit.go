package http

import (
	"context"
	"fmt"
)

func (s *Server) auditBestEffort(ctx context.Context, actorUserID *string, action, targetType string, targetID *string, summary string) {
	if err := s.auditRequired(ctx, actorUserID, action, targetType, targetID, summary); err != nil && s.log != nil {
		s.log.Error(
			"audit event persistence failed",
			"action", action,
			"target_type", targetType,
			"target_id", stringPointerValue(targetID),
			"actor_user_id", stringPointerValue(actorUserID),
			"error", err,
		)
	}
}

func (s *Server) auditRequired(ctx context.Context, actorUserID *string, action, targetType string, targetID *string, summary string) error {
	if s == nil || s.store == nil {
		return fmt.Errorf("audit store is unavailable")
	}
	if _, err := s.store.CreateAuditForUser(ctx, actorUserID, action, targetType, targetID, summary); err != nil {
		return fmt.Errorf("persist audit event %s: %w", action, err)
	}
	return nil
}

func (s *Server) logPersistenceFailure(operation string, err error, attrs ...any) {
	if s == nil || s.log == nil || err == nil {
		return
	}
	attrs = append(attrs, "operation", operation, "error", err)
	s.log.Error("control-plane persistence failed", attrs...)
}

func stringPointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
