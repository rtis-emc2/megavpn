package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rtis-emc2/megavpn/internal/platform/config"
)

type enrollmentTestLogger struct {
	mu      sync.Mutex
	entries []string
}

func (l *enrollmentTestLogger) Info(msg string, args ...any) {
	l.record(msg, args...)
}

func (l *enrollmentTestLogger) Error(msg string, args ...any) {
	l.record(msg, args...)
}

func (l *enrollmentTestLogger) record(msg string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	var b strings.Builder
	b.WriteString(msg)
	for i := 0; i+1 < len(args); i += 2 {
		b.WriteString(" ")
		b.WriteString(stringify(args[i]))
		b.WriteString("=")
		b.WriteString(stringify(args[i+1]))
	}
	l.entries = append(l.entries, b.String())
}

func (l *enrollmentTestLogger) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(l.entries, "\n")
}

func TestEnrollmentErrorClassification(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want enrollmentErrorClass
	}{
		{name: "request invalid", err: &agentAPIError{StatusCode: http.StatusBadRequest, Code: "agent_registration_request_invalid"}, want: enrollmentErrorTerminalConfiguration},
		{name: "forbidden", err: &agentAPIError{StatusCode: http.StatusForbidden}, want: enrollmentErrorTerminalConfiguration},
		{name: "not found", err: &agentAPIError{StatusCode: http.StatusNotFound}, want: enrollmentErrorTerminalConfiguration},
		{name: "retired", err: &agentAPIError{StatusCode: http.StatusConflict, Code: agentCodeEnrollmentNodeRetired}, want: enrollmentErrorTerminalConfiguration},
		{name: "unknown 4xx", err: &agentAPIError{StatusCode: http.StatusTeapot}, want: enrollmentErrorTerminalConfiguration},
		{name: "reissue", err: &agentAPIError{StatusCode: http.StatusUnauthorized, Code: agentCodeEnrollmentReissueRequired}, want: enrollmentErrorReissueRequired},
		{name: "timeout", err: &agentAPIError{StatusCode: http.StatusRequestTimeout}, want: enrollmentErrorTransient},
		{name: "rate limited", err: &agentAPIError{StatusCode: http.StatusTooManyRequests, Code: agentCodeRegistrationRateLimited}, want: enrollmentErrorTransient},
		{name: "internal", err: &agentAPIError{StatusCode: http.StatusInternalServerError}, want: enrollmentErrorTransient},
		{name: "bad gateway", err: &agentAPIError{StatusCode: http.StatusBadGateway}, want: enrollmentErrorTransient},
		{name: "unavailable", err: &agentAPIError{StatusCode: http.StatusServiceUnavailable}, want: enrollmentErrorTransient},
		{name: "gateway timeout", err: &agentAPIError{StatusCode: http.StatusGatewayTimeout}, want: enrollmentErrorTransient},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyEnrollmentError(context.Background(), tc.err); got != tc.want {
				t.Fatalf("classification = %s, want %s", got, tc.want)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := classifyEnrollmentError(ctx, context.Canceled); got != enrollmentErrorContextCancelled {
		t.Fatalf("context cancellation classification = %s, want %s", got, enrollmentErrorContextCancelled)
	}
	if got := boundedEnrollmentRetryAfter(10 * time.Minute); got != maxEnrollmentRetryAfter {
		t.Fatalf("bounded retry after = %s, want %s", got, maxEnrollmentRetryAfter)
	}
}

func TestEnrollWithRetryResponseLossStopsForReissueAndPreservesBootstrap(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	bootstrapPath := filepath.Join(dir, "agent-bootstrap.env")
	oldEnrollmentToken := "old-enrollment-token-secret"
	if err := os.WriteFile(bootstrapPath, []byte("MEGAVPN_AGENT_ENROLLMENT_TOKEN="+oldEnrollmentToken+"\n"), 0o600); err != nil {
		t.Fatalf("write bootstrap: %v", err)
	}
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := atomic.AddInt32(&attempts, 1)
		_, _ = io.Copy(io.Discard, r.Body)
		if attempt == 1 {
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("response recorder does not support hijack")
			}
			conn, _, err := hijacker.Hijack()
			if err != nil {
				t.Fatalf("hijack response: %v", err)
			}
			_ = conn.Close()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"status":"error","code":"agent_enrollment_reissue_required","error":"agent enrollment requires a new enrollment token"}`))
	}))
	defer server.Close()
	sleeps := captureAgentSleeps(t)
	log := &enrollmentTestLogger{}

	_, err := enrollWithRetry(context.Background(), log, config.Config{Agent: config.AgentConfig{
		StatePath:     statePath,
		BootstrapPath: bootstrapPath,
	}}, bootstrapConfig{
		NodeID:          "11111111-1111-4111-8111-111111111111",
		NodeName:        "node-one",
		NodeAddress:     "10.0.0.10",
		ControlPlaneURL: server.URL,
		EnrollmentToken: oldEnrollmentToken,
	})
	if !errors.Is(err, ErrEnrollmentReissueRequired) {
		t.Fatalf("enroll error = %v, want ErrEnrollmentReissueRequired", err)
	}
	if attempts != 2 {
		t.Fatalf("registration attempts = %d, want 2", attempts)
	}
	if len(sleeps()) != 1 {
		t.Fatalf("sleep count = %d, want one transient retry sleep", len(sleeps()))
	}
	if _, err := os.Stat(statePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("state file exists after reissue-required: %v", err)
	}
	if _, err := os.Stat(bootstrapPath); err != nil {
		t.Fatalf("bootstrap file should remain after reissue-required: %v", err)
	}
	combined := log.String() + "\n" + err.Error()
	if strings.Contains(combined, oldEnrollmentToken) {
		t.Fatal("enrollment token leaked in retry logs or error")
	}
}

func TestEnrollWithRetryRespectsRetryAfterThenStopsForReissue(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	bootstrapPath := filepath.Join(dir, "agent-bootstrap.env")
	if err := os.WriteFile(bootstrapPath, []byte("bootstrap"), 0o600); err != nil {
		t.Fatalf("write bootstrap: %v", err)
	}
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := atomic.AddInt32(&attempts, 1)
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		if attempt == 1 {
			w.Header().Set("Retry-After", "90")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"status":"error","code":"agent_registration_rate_limited","error":"rate limit exceeded"}`))
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"status":"error","code":"agent_enrollment_reissue_required","error":"agent enrollment requires a new enrollment token"}`))
	}))
	defer server.Close()
	sleeps := captureAgentSleeps(t)

	_, err := enrollWithRetry(context.Background(), &enrollmentTestLogger{}, config.Config{Agent: config.AgentConfig{
		StatePath:     statePath,
		BootstrapPath: bootstrapPath,
	}}, bootstrapConfig{
		NodeID:          "11111111-1111-4111-8111-111111111111",
		NodeName:        "node-one",
		NodeAddress:     "10.0.0.10",
		ControlPlaneURL: server.URL,
		EnrollmentToken: "enrollment-token",
	})
	if !errors.Is(err, ErrEnrollmentReissueRequired) {
		t.Fatalf("enroll error = %v, want ErrEnrollmentReissueRequired", err)
	}
	gotSleeps := sleeps()
	if len(gotSleeps) != 1 || gotSleeps[0] != 90*time.Second {
		t.Fatalf("retry sleeps = %v, want [90s]", gotSleeps)
	}
}

func TestEnrollWithRetryOperatorReissueRecoveryWritesStateAndRemovesBootstrap(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	bootstrapPath := filepath.Join(dir, "agent-bootstrap.env")
	oldEnrollmentToken := "old-enrollment-token-secret"
	newEnrollmentToken := "new-enrollment-token-secret"
	if err := os.WriteFile(bootstrapPath, []byte("MEGAVPN_AGENT_ENROLLMENT_TOKEN="+oldEnrollmentToken+"\n"), 0o600); err != nil {
		t.Fatalf("write bootstrap: %v", err)
	}
	var successTokenSeen atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(string(body), oldEnrollmentToken) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"status":"error","code":"agent_enrollment_reissue_required","error":"agent enrollment requires a new enrollment token"}`))
			return
		}
		if !strings.Contains(string(body), newEnrollmentToken) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"status":"error","code":"agent_registration_request_invalid","error":"invalid agent register payload"}`))
			return
		}
		successTokenSeen.Store(true)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"registered","node":{"id":"11111111-1111-4111-8111-111111111111","name":"operator-node","address":"10.0.0.20"},"agent_token":"new-agent-token","token_hint":"new-age...token"}`))
	}))
	defer server.Close()
	captureAgentSleeps(t)

	_, err := enrollWithRetry(context.Background(), &enrollmentTestLogger{}, config.Config{Agent: config.AgentConfig{
		StatePath:     statePath,
		BootstrapPath: bootstrapPath,
	}}, bootstrapConfig{
		NodeID:          "11111111-1111-4111-8111-111111111111",
		NodeName:        "node-one",
		NodeAddress:     "10.0.0.10",
		ControlPlaneURL: server.URL,
		EnrollmentToken: oldEnrollmentToken,
	})
	if !errors.Is(err, ErrEnrollmentReissueRequired) {
		t.Fatalf("first enroll error = %v, want ErrEnrollmentReissueRequired", err)
	}
	if _, err := os.Stat(bootstrapPath); err != nil {
		t.Fatalf("bootstrap should remain before operator reissue: %v", err)
	}
	if err := os.WriteFile(bootstrapPath, []byte("MEGAVPN_AGENT_ENROLLMENT_TOKEN="+newEnrollmentToken+"\n"), 0o600); err != nil {
		t.Fatalf("write reissued bootstrap: %v", err)
	}
	st, err := enrollWithRetry(context.Background(), &enrollmentTestLogger{}, config.Config{Agent: config.AgentConfig{
		StatePath:     statePath,
		BootstrapPath: bootstrapPath,
	}}, bootstrapConfig{
		NodeID:          "11111111-1111-4111-8111-111111111111",
		NodeName:        "node-one",
		NodeAddress:     "10.0.0.10",
		ControlPlaneURL: server.URL,
		EnrollmentToken: newEnrollmentToken,
	})
	if err != nil {
		t.Fatalf("second enroll error = %v", err)
	}
	if !successTokenSeen.Load() || st.AgentToken != "new-agent-token" {
		t.Fatalf("state token = %q, want newly returned agent token", st.AgentToken)
	}
	info, err := os.Stat(statePath)
	if err != nil {
		t.Fatalf("state file missing: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("state file mode = %o, want 0600", mode)
	}
	persisted, err := loadState(statePath)
	if err != nil {
		t.Fatalf("load persisted state: %v", err)
	}
	if persisted.AgentToken != "new-agent-token" {
		t.Fatalf("persisted agent token = %q, want new token", persisted.AgentToken)
	}
	if _, err := os.Stat(bootstrapPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("bootstrap file should be removed after successful state persistence: %v", err)
	}
}

func TestEnrollWithRetryKeepsBootstrapWhenStatePersistenceFails(t *testing.T) {
	dir := t.TempDir()
	stateParent := filepath.Join(dir, "state-parent")
	statePath := filepath.Join(stateParent, "state.json")
	if err := os.WriteFile(stateParent, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("create state parent blocker: %v", err)
	}
	bootstrapPath := filepath.Join(dir, "agent-bootstrap.env")
	if err := os.WriteFile(bootstrapPath, []byte("MEGAVPN_AGENT_ENROLLMENT_TOKEN=test-token\n"), 0o600); err != nil {
		t.Fatalf("write bootstrap: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"registered","node":{"id":"11111111-1111-4111-8111-111111111111","name":"operator-node","address":"10.0.0.20"},"agent_token":"new-agent-token","token_hint":"new-age...token"}`))
	}))
	defer server.Close()
	captureAgentSleeps(t)

	_, err := enrollWithRetry(context.Background(), &enrollmentTestLogger{}, config.Config{Agent: config.AgentConfig{
		StatePath:     statePath,
		BootstrapPath: bootstrapPath,
	}}, bootstrapConfig{
		NodeID:          "11111111-1111-4111-8111-111111111111",
		NodeName:        "node-one",
		NodeAddress:     "10.0.0.10",
		ControlPlaneURL: server.URL,
		EnrollmentToken: "test-token",
	})
	if err == nil || !strings.Contains(err.Error(), "agent state save failed") {
		t.Fatalf("enroll error = %v, want state save failure", err)
	}
	if _, err := os.Stat(bootstrapPath); err != nil {
		t.Fatalf("bootstrap file should remain when state persistence fails: %v", err)
	}
}

func captureAgentSleeps(t *testing.T) func() []time.Duration {
	t.Helper()
	var mu sync.Mutex
	var sleeps []time.Duration
	previous := agentSleepContext
	agentSleepContext = func(ctx context.Context, d time.Duration) error {
		mu.Lock()
		sleeps = append(sleeps, d)
		mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}
	t.Cleanup(func() {
		agentSleepContext = previous
	})
	return func() []time.Duration {
		mu.Lock()
		defer mu.Unlock()
		out := make([]time.Duration, len(sleeps))
		copy(out, sleeps)
		return out
	}
}
