package main

import (
	"context"
	"strings"
)

func (c *client) rotateAgentToken(ctx context.Context, j job, st *agentState) (string, map[string]any) {
	newToken := strings.TrimSpace(stringify(j.Payload["new_agent_token"]))
	newHint := strings.TrimSpace(stringify(j.Payload["new_token_hint"]))
	if newToken == "" {
		return "failed", map[string]any{"error": "new_agent_token is required"}
	}
	nextState := *st
	nextState.AgentToken = newToken
	if err := saveState(c.statePath, nextState); err != nil {
		return "failed", map[string]any{"error": err.Error(), "stage": "save_state"}
	}
	*st = nextState
	c.token = newToken
	return "succeeded", map[string]any{
		"message":        "agent token rotated locally",
		"new_token_hint": newHint,
		"node_id":        st.NodeID,
	}
}
