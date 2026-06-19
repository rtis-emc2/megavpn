package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	nethttp "net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rtis-emc2/megavpn/internal/agentauth"
)

const defaultAgentSignatureWindow = 5 * time.Minute

type agentSignatureReplayCache struct {
	mu      sync.Mutex
	window  time.Duration
	entries map[string]time.Time
}

func newAgentSignatureReplayCache(window time.Duration) *agentSignatureReplayCache {
	if window <= 0 {
		window = defaultAgentSignatureWindow
	}
	return &agentSignatureReplayCache{window: window, entries: map[string]time.Time{}}
}

func (c *agentSignatureReplayCache) accept(key string, now time.Time) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	if c == nil {
		return true
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for existing, expiresAt := range c.entries {
		if !expiresAt.After(now) {
			delete(c.entries, existing)
		}
	}
	if _, ok := c.entries[key]; ok {
		return false
	}
	c.entries[key] = now.Add(c.window)
	return true
}

func (s *Server) verifyAgentSignature(r *nethttp.Request, scope, token string) bool {
	if r == nil {
		return false
	}
	signed := agentRequestHasSignature(r)
	if !signed && !s.agentSignatureEnforce {
		return true
	}
	body, err := requestBodySnapshot(r)
	if err != nil {
		if s.log != nil {
			s.log.Warn("agent signature body read failed", "scope", scope, "error", err)
		}
		return false
	}
	now := time.Now().UTC()
	window := s.agentSignatureWindow
	if window <= 0 {
		window = defaultAgentSignatureWindow
	}
	err = agentauth.Verify(
		token,
		r.Method,
		r.URL.RequestURI(),
		r.Header.Get(agentauth.HeaderTimestamp),
		r.Header.Get(agentauth.HeaderNonce),
		r.Header.Get(agentauth.HeaderBodyHash),
		r.Header.Get(agentauth.HeaderSignature),
		body,
		now,
		window,
	)
	if err != nil {
		if errors.Is(err, agentauth.ErrUnsigned) && !s.agentSignatureEnforce {
			return true
		}
		if s.log != nil {
			s.log.Warn("agent signature verification failed", "scope", scope, "error", err)
		}
		return false
	}
	replayKey := strings.TrimSpace(scope) + ":" + strings.TrimSpace(r.Header.Get(agentauth.HeaderNonce))
	replay := s.agentSignatureReplay
	if replay == nil {
		replay = newAgentSignatureReplayCache(window)
		s.agentSignatureReplay = replay
	}
	if !replay.accept(replayKey, now) {
		if s.log != nil {
			s.log.Warn("agent signature replay rejected", "scope", scope)
		}
		return false
	}
	return true
}

func agentRequestHasSignature(r *nethttp.Request) bool {
	return strings.TrimSpace(r.Header.Get(agentauth.HeaderSignature)) != "" ||
		strings.TrimSpace(r.Header.Get(agentauth.HeaderTimestamp)) != "" ||
		strings.TrimSpace(r.Header.Get(agentauth.HeaderNonce)) != "" ||
		strings.TrimSpace(r.Header.Get(agentauth.HeaderBodyHash)) != ""
}

func requestBodySnapshot(r *nethttp.Request) ([]byte, error) {
	if r.Body == nil || r.Body == nethttp.NoBody {
		return nil, nil
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	return body, nil
}

func writeSignedAgentJSON(w nethttp.ResponseWriter, r *nethttp.Request, token string, code int, v any) {
	if strings.TrimSpace(token) == "" {
		writeJSON(w, code, v)
		return
	}
	body, err := json.Marshal(v)
	if err != nil {
		writeErr(w, 500, "encode signed agent response failed")
		return
	}
	body = append(body, '\n')
	timestamp := strconv.FormatInt(time.Now().UTC().Unix(), 10)
	nonce := agentauth.NewNonce()
	signature, bodyHash := agentauth.Sign(token, "RESPONSE", r.URL.RequestURI(), timestamp, nonce, body)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set(agentauth.HeaderTimestamp, timestamp)
	w.Header().Set(agentauth.HeaderNonce, nonce)
	w.Header().Set(agentauth.HeaderBodyHash, bodyHash)
	w.Header().Set(agentauth.HeaderSignature, signature)
	w.WriteHeader(code)
	_, _ = w.Write(body)
}
