package http

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiterAllowsUntilLimitAndThenResets(t *testing.T) {
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	limiter := newRateLimiter()
	limiter.now = func() time.Time { return now }

	if ok, _ := limiter.allow("login:127.0.0.1", 2, time.Minute); !ok {
		t.Fatal("first request should be allowed")
	}
	if ok, _ := limiter.allow("login:127.0.0.1", 2, time.Minute); !ok {
		t.Fatal("second request should be allowed")
	}
	if ok, retryAfter := limiter.allow("login:127.0.0.1", 2, time.Minute); ok || retryAfter <= 0 {
		t.Fatalf("third request should be rejected with retry-after, ok=%v retry_after=%s", ok, retryAfter)
	}

	now = now.Add(time.Minute)
	if ok, _ := limiter.allow("login:127.0.0.1", 2, time.Minute); !ok {
		t.Fatal("request after reset should be allowed")
	}
}

func TestWithRateLimitRejectsExcessRequests(t *testing.T) {
	s := &Server{rateLimiter: newRateLimiter()}
	handler := s.withRateLimit("test", 1, time.Minute, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "/limited", nil)
	req.RemoteAddr = "192.0.2.10:12345"

	first := httptest.NewRecorder()
	handler(first, req)
	if first.Code != http.StatusNoContent {
		t.Fatalf("first status = %d, want %d", first.Code, http.StatusNoContent)
	}

	second := httptest.NewRecorder()
	handler(second, req)
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want %d", second.Code, http.StatusTooManyRequests)
	}
	if second.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header")
	}
}

