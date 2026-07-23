package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	nethttp "net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rtis-emc2/megavpn/internal/agentauth"
)

const defaultAgentSignatureWindow = 5 * time.Minute
const maxAgentSignatureReplayEntries = 65536

var errAgentSignatureReplay = errors.New("agent request signature replay rejected")

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
	if len(c.entries) >= maxAgentSignatureReplayEntries {
		return false
	}
	c.entries[key] = now.Add(c.window)
	return true
}

func (s *Server) verifyAgentSignature(r *nethttp.Request, scope, token string) bool {
	return s.verifyAgentSignatureError(r, scope, token) == nil
}

func (s *Server) verifyAgentSignatureError(r *nethttp.Request, scope, token string) error {
	if r == nil {
		return errors.New("agent request is nil")
	}
	signed := agentRequestHasSignature(r)
	if !signed && !s.agentSignatureEnforce {
		return nil
	}
	body, err := requestBodySnapshot(r)
	if err != nil {
		if s.log != nil {
			s.log.Warn("agent signature body read failed", "scope", scope, "error", err)
		}
		return fmt.Errorf("read agent request body: %w", err)
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
			return nil
		}
		err = describeAgentSignatureFailure(err, r.Header.Get(agentauth.HeaderTimestamp), now)
		if s.log != nil {
			s.log.Warn("agent signature verification failed", "scope", scope, "error", err)
		}
		return err
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
		return errAgentSignatureReplay
	}
	return nil
}

func describeAgentSignatureFailure(err error, timestamp string, now time.Time) error {
	if !errors.Is(err, agentauth.ErrTimestampOutdated) {
		return err
	}
	issuedUnix, parseErr := strconv.ParseInt(strings.TrimSpace(timestamp), 10, 64)
	if parseErr != nil || issuedUnix <= 0 {
		return err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	skew := now.Sub(time.Unix(issuedUnix, 0).UTC())
	direction := "ahead of"
	if skew > 0 {
		direction = "behind"
	}
	return fmt.Errorf(
		"%w: agent clock is %s control-plane clock by %s; synchronize NTP on both hosts",
		err,
		direction,
		formatClockSkew(skew),
	)
}

func formatClockSkew(skew time.Duration) time.Duration {
	if skew < 0 {
		skew = -skew
	}
	return skew.Round(time.Second)
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

func writeSignedAgentError(w nethttp.ResponseWriter, r *nethttp.Request, code int, message string) {
	writeSignedAgentJSON(w, r, bearerToken(r), code, response{"error": message})
}

func writeSignedAgentNoContent(w nethttp.ResponseWriter, r *nethttp.Request, token string) {
	if strings.TrimSpace(token) == "" {
		w.WriteHeader(nethttp.StatusNoContent)
		return
	}
	body := []byte{}
	timestamp := strconv.FormatInt(time.Now().UTC().Unix(), 10)
	nonce := agentauth.NewNonce()
	signature, bodyHash := agentauth.Sign(token, "RESPONSE", r.URL.RequestURI(), timestamp, nonce, body)
	w.Header().Set(agentauth.HeaderTimestamp, timestamp)
	w.Header().Set(agentauth.HeaderNonce, nonce)
	w.Header().Set(agentauth.HeaderBodyHash, bodyHash)
	w.Header().Set(agentauth.HeaderSignature, signature)
	w.WriteHeader(nethttp.StatusNoContent)
}

func setSignedAgentResponseHeaders(w nethttp.ResponseWriter, r *nethttp.Request, token, bodyHash string) bool {
	token = strings.TrimSpace(token)
	bodyHash = strings.ToLower(strings.TrimSpace(bodyHash))
	if token == "" || bodyHash == "" {
		return false
	}
	timestamp := strconv.FormatInt(time.Now().UTC().Unix(), 10)
	nonce := agentauth.NewNonce()
	signature := agentauth.SignBodyHash(token, "RESPONSE", r.URL.RequestURI(), timestamp, nonce, bodyHash)
	w.Header().Set(agentauth.HeaderTimestamp, timestamp)
	w.Header().Set(agentauth.HeaderNonce, nonce)
	w.Header().Set(agentauth.HeaderBodyHash, bodyHash)
	w.Header().Set(agentauth.HeaderSignature, signature)
	return true
}
