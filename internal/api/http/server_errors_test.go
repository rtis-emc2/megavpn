package http

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPublicErrorMessageHidesInternalFailures(t *testing.T) {
	tests := []struct {
		name string
		code int
		msg  string
		want string
	}{
		{name: "server error", code: 500, msg: `open /etc/megavpn/private: permission denied`, want: "internal server error"},
		{name: "postgres state", code: 409, msg: `ERROR: relation "secret_refs" does not exist (SQLSTATE 42P01)`, want: "request could not be processed"},
		{name: "prepared statement", code: 400, msg: `cannot insert multiple commands into a prepared statement`, want: "request could not be processed"},
		{name: "operator correction", code: 409, msg: `client access group "blocked" is not active`, want: `client access group "blocked" is not active`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := publicErrorMessage(tt.code, tt.msg); got != tt.want {
				t.Fatalf("publicErrorMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPublicErrorMessageNormalizesAndBoundsClientErrors(t *testing.T) {
	got := publicErrorMessage(400, strings.Repeat("x", 600)+"\nsecret")
	if len(got) != 512 {
		t.Fatalf("len(publicErrorMessage()) = %d, want 512", len(got))
	}
	if strings.ContainsAny(got, "\r\n") {
		t.Fatalf("publicErrorMessage() contains control newline: %q", got)
	}
}

func TestPublicBearerResourceFailureDoesNotExposeTokenState(t *testing.T) {
	for _, privateReason := range []string{
		"share token expired at 2026-08-05T00:00:00Z",
		"subscription was revoked by operator@example.com",
		"token hash does not exist",
	} {
		if strings.Contains(publicBearerResourceUnavailable, privateReason) {
			t.Fatalf("public bearer failure exposes private state %q", privateReason)
		}
	}
	if publicBearerResourceUnavailable != "requested resource is unavailable" {
		t.Fatalf("unexpected public bearer failure: %q", publicBearerResourceUnavailable)
	}
}

func TestRequestLoggerKeepsPrivateServerErrorOutOfResponse(t *testing.T) {
	var logs bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logs, nil))
	handler := requestLogger(log, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeErr(w, http.StatusInternalServerError, "database password leaked\nsecond line")
	}))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/test", nil))

	var body response
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got := body["error"]; got != "internal server error" {
		t.Fatalf("public error = %q, want internal server error", got)
	}
	if strings.Contains(recorder.Body.String(), "database password") {
		t.Fatalf("private error leaked to response: %s", recorder.Body.String())
	}
	if !strings.Contains(logs.String(), `"level":"ERROR"`) || !strings.Contains(logs.String(), `"error":"database password leaked second line"`) {
		t.Fatalf("private error was not retained in structured log: %s", logs.String())
	}
}
