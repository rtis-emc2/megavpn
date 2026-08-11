package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

type inviteAcceptanceFailureStore struct {
	Store
	err error
}

func (s inviteAcceptanceFailureStore) AcceptPlatformUserInvite(context.Context, string, string) (domain.PlatformUserInvite, domain.PlatformUserRecord, error) {
	return domain.PlatformUserInvite{}, domain.PlatformUserRecord{}, s.err
}

func TestAcceptPlatformInviteRedactsPersistenceFailures(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "invalid token", err: domain.ErrPlatformInviteInvalid},
		{name: "database failure", err: errors.New(`ERROR: relation "platform_user_invites" does not exist`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/invites/secret/accept", strings.NewReader(`{"password":"a-valid-password-123"}`))
			req.SetPathValue("token", "secret")
			rec := httptest.NewRecorder()
			s := &Server{store: inviteAcceptanceFailureStore{err: tt.err}}

			s.acceptPlatformInvite(rec, req)

			if rec.Code != http.StatusConflict {
				t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusConflict, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), domain.ErrPlatformInviteInvalid.Error()) {
				t.Fatalf("body = %q, want generic invite error", rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "platform_user_invites") {
				t.Fatalf("response leaked persistence details: %q", rec.Body.String())
			}
		})
	}
}
