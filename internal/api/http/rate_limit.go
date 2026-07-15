package http

import (
	"net/http"
	"strconv"
	"sync"
	"time"
)

type rateLimiter struct {
	mu      sync.Mutex
	entries map[string]rateLimitEntry
	now     func() time.Time
}

type rateLimitEntry struct {
	Count     int
	ResetAt   time.Time
	LastSeen  time.Time
	ExpiresAt time.Time
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{
		entries: map[string]rateLimitEntry{},
		now:     time.Now,
	}
}

func (l *rateLimiter) allow(key string, limit int, window time.Duration) (bool, time.Duration) {
	if l == nil || limit <= 0 || window <= 0 {
		return true, 0
	}
	now := l.now().UTC()
	l.mu.Lock()
	defer l.mu.Unlock()

	if len(l.entries) > 4096 {
		l.gcLocked(now)
	}

	entry := l.entries[key]
	if entry.ResetAt.IsZero() || !now.Before(entry.ResetAt) {
		entry = rateLimitEntry{
			Count:     0,
			ResetAt:   now.Add(window),
			ExpiresAt: now.Add(window * 2),
		}
	}
	entry.Count++
	entry.LastSeen = now
	l.entries[key] = entry

	if entry.Count <= limit {
		return true, 0
	}
	retryAfter := entry.ResetAt.Sub(now)
	if retryAfter < time.Second {
		retryAfter = time.Second
	}
	return false, retryAfter
}

func (l *rateLimiter) gcLocked(now time.Time) {
	for key, entry := range l.entries {
		if !entry.ExpiresAt.IsZero() && now.After(entry.ExpiresAt) {
			delete(l.entries, key)
		}
	}
}

func (s *Server) withRateLimit(scope string, limit int, window time.Duration, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.rateLimiter == nil {
			next(w, r)
			return
		}
		key := scope + ":" + s.clientIP(r)
		ok, retryAfter := s.rateLimiter.allow(key, limit, window)
		if !ok {
			w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
			if scope == "agent_register" {
				writeSensitiveCodeErr(w, http.StatusTooManyRequests, agentRegistrationRateLimited, "rate limit exceeded")
				return
			}
			writeErr(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		next(w, r)
	}
}
