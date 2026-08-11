package http

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

func TestWritePlatformUserMutationError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantBody   string
	}{
		{
			name:       "missing user",
			err:        domain.ErrPlatformUserNotFound,
			wantStatus: http.StatusNotFound,
			wantBody:   domain.ErrPlatformUserNotFound.Error(),
		},
		{
			name:       "duplicate user",
			err:        domain.ErrPlatformUserConflict,
			wantStatus: http.StatusConflict,
			wantBody:   domain.ErrPlatformUserConflict.Error(),
		},
		{
			name:       "unknown role",
			err:        errors.Join(domain.ErrUnknownPlatformRole, errors.New("private role code")),
			wantStatus: http.StatusBadRequest,
			wantBody:   "one or more platform roles are not available",
		},
		{
			name:       "last superadmin",
			err:        domain.ErrLastSuperadmin,
			wantStatus: http.StatusConflict,
			wantBody:   domain.ErrLastSuperadmin.Error(),
		},
		{
			name:       "persistence failure is redacted",
			err:        errors.New(`ERROR: duplicate key value violates unique constraint "users_email_key"`),
			wantStatus: http.StatusInternalServerError,
			wantBody:   "internal server error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			s := &Server{}
			s.writePlatformUserMutationError(rr, "platform.user.update", tt.err, "platform user update failed")

			if rr.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rr.Code, tt.wantStatus)
			}
			if !strings.Contains(rr.Body.String(), tt.wantBody) {
				t.Fatalf("body = %q, want it to contain %q", rr.Body.String(), tt.wantBody)
			}
			if strings.Contains(rr.Body.String(), "users_email_key") {
				t.Fatalf("response leaked persistence details: %q", rr.Body.String())
			}
		})
	}
}
