package main

import (
	"strings"
	"sync"
	"time"
)

type responseReplayCache struct {
	mu      sync.Mutex
	window  time.Duration
	entries map[string]time.Time
}

func newResponseReplayCache(window time.Duration) *responseReplayCache {
	if window <= 0 {
		window = 5 * time.Minute
	}
	return &responseReplayCache{window: window, entries: map[string]time.Time{}}
}

func (c *responseReplayCache) accept(key string, now time.Time) bool {
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
