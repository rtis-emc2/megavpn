package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func newClient(baseURL, token, statePath string) *client {
	return &client{
		baseURL:   strings.TrimRight(baseURL, "/"),
		token:     token,
		statePath: statePath,
		http:      &http.Client{Timeout: 10 * time.Second},
	}
}

func (c client) register(ctx context.Context, b bootstrapConfig) (*agentState, error) {
	payload := map[string]any{
		"node_id":          b.NodeID,
		"name":             b.NodeName,
		"address":          b.NodeAddress,
		"token":            b.DevToken,
		"enrollment_token": b.EnrollmentToken,
	}
	var out registerResp
	if err := c.post(ctx, "/agent/register", payload, &out); err != nil {
		return nil, err
	}
	if out.AgentToken == "" && b.DevToken != "" {
		out.AgentToken = b.DevToken
	}
	if out.AgentToken == "" {
		return nil, errors.New("control plane did not return agent_token")
	}
	nodeID := first(out.Node.ID, b.NodeID)
	nodeName := first(out.Node.Name, b.NodeName)
	addr := first(out.Node.Address, b.NodeAddress)
	return &agentState{
		NodeID:          nodeID,
		NodeName:        nodeName,
		NodeAddress:     addr,
		ControlPlaneURL: b.ControlPlaneURL,
		AgentToken:      out.AgentToken,
		RegisteredAt:    time.Now().UTC(),
	}, nil
}

func (c client) heartbeat(ctx context.Context, nodeID, name string) error {
	return c.post(ctx, "/agent/heartbeat", map[string]any{"node_id": nodeID, "name": name}, nil)
}

func (c client) submitInventory(ctx context.Context, nodeID, source string, inv map[string]any) error {
	return c.post(ctx, "/agent/inventory", map[string]any{"node_id": nodeID, "source": source, "inventory": inv}, nil)
}

func (c client) nextJob(ctx context.Context, nodeID string) (job, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/agent/jobs/next?node_id="+nodeID, nil)
	if err != nil {
		return job{}, false, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.http.Do(req)
	if err != nil {
		return job{}, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return job{}, false, nil
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return job{}, false, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(b))
	}
	var j job
	if err := json.NewDecoder(resp.Body).Decode(&j); err != nil {
		return job{}, false, err
	}
	return j, true, nil
}

func (c client) submit(ctx context.Context, id, status string, result map[string]any) error {
	return c.post(ctx, "/agent/jobs/"+id+"/result", map[string]any{"status": status, "result": result}, nil)
}

func (c client) post(ctx context.Context, path string, payload any, out any) error {
	b, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}
