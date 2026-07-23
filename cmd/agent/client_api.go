package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/rtis-emc2/megavpn/internal/agentauth"
)

func newClient(baseURL, token, statePath string) *client {
	return &client{
		baseURL:                      strings.TrimRight(baseURL, "/"),
		token:                        token,
		statePath:                    statePath,
		http:                         &http.Client{Timeout: 10 * time.Second},
		responseReplay:               newResponseReplayCache(5 * time.Minute),
		trafficReportInterval:        time.Minute,
		xrayTrafficCounterState:      map[string]int64{},
		wireGuardTrafficCounterState: map[string]int64{},
		openVPNTrafficCounterState:   map[string]int64{},
	}
}

func (c client) register(ctx context.Context, b bootstrapConfig) (*agentState, error) {
	payload := map[string]any{
		"node_id":          b.NodeID,
		"name":             b.NodeName,
		"address":          b.NodeAddress,
		"token":            b.DevToken,
		"enrollment_token": b.EnrollmentToken,
		"agent_version":    appVersion,
		"protocol_version": "v1",
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
	return c.post(ctx, "/agent/heartbeat", map[string]any{
		"node_id":          nodeID,
		"name":             name,
		"agent_version":    appVersion,
		"protocol_version": "v1",
	}, nil)
}

func (c client) submitInventory(ctx context.Context, nodeID, source string, inv map[string]any) error {
	return c.post(ctx, "/agent/inventory", map[string]any{"node_id": nodeID, "source": source, "inventory": inv}, nil)
}

func (c client) listRuntimeTargets(ctx context.Context, nodeID string) ([]instanceRuntimeTarget, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/agent/runtime/instances?node_id="+url.QueryEscape(nodeID), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	c.signRequest(req, nil)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(b))
	}
	body, err := c.readSignedResponseBody(req, resp, true)
	if err != nil {
		return nil, err
	}
	var out instanceRuntimeTargetsResp
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return out.Targets, nil
}

func (c client) submitRuntimeReports(ctx context.Context, nodeID string, reports []instanceRuntimeReport) error {
	return c.post(ctx, "/agent/runtime/instances", map[string]any{"node_id": nodeID, "reports": reports}, nil)
}

func (c client) submitTrafficAccounting(ctx context.Context, nodeID string, samples []trafficAccountingSample) error {
	if len(samples) == 0 {
		return nil
	}
	return c.post(ctx, "/agent/traffic/accounting", map[string]any{"node_id": nodeID, "samples": samples}, nil)
}

func (c client) nextJob(ctx context.Context, nodeID string) (job, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/agent/jobs/next?node_id="+nodeID, nil)
	if err != nil {
		return job{}, false, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	c.signRequest(req, nil)
	resp, err := c.http.Do(req)
	if err != nil {
		return job{}, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		if _, err := c.readSignedResponseBody(req, resp, true); err != nil {
			return job{}, false, err
		}
		return job{}, false, nil
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return job{}, false, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(b))
	}
	body, err := c.readSignedResponseBody(req, resp, true)
	if err != nil {
		return job{}, false, err
	}
	var j job
	if err := json.Unmarshal(body, &j); err != nil {
		return job{}, false, err
	}
	return j, true, nil
}

func (c client) submit(ctx context.Context, id, status string, result map[string]any) error {
	return c.post(ctx, "/agent/jobs/"+id+"/result", map[string]any{"status": status, "result": result}, nil)
}

func (c client) post(ctx context.Context, path string, payload any, out any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal agent request payload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	c.signRequest(req, b)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, readErr := c.readSignedResponseBody(req, resp, path != "/agent/register")
	if readErr != nil {
		return readErr
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}
	if out != nil {
		return json.Unmarshal(body, out)
	}
	return nil
}

func (c client) signRequest(req *http.Request, body []byte) {
	if req == nil || strings.TrimSpace(c.token) == "" {
		return
	}
	timestamp := strconv.FormatInt(time.Now().UTC().Unix(), 10)
	nonce := agentauth.NewNonce()
	signature, bodyHash := agentauth.Sign(c.token, req.Method, req.URL.RequestURI(), timestamp, nonce, body)
	req.Header.Set(agentauth.HeaderTimestamp, timestamp)
	req.Header.Set(agentauth.HeaderNonce, nonce)
	req.Header.Set(agentauth.HeaderBodyHash, bodyHash)
	req.Header.Set(agentauth.HeaderSignature, signature)
}

func (c client) readSignedResponseBody(req *http.Request, resp *http.Response, requireSignature bool) ([]byte, error) {
	if resp == nil {
		return nil, errors.New("response is nil")
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if !responseHasAgentSignature(resp) {
		if requireSignature {
			return nil, unsignedAgentResponseError(resp, body)
		}
		return body, nil
	}
	if strings.TrimSpace(c.token) == "" {
		return nil, errors.New("signed agent response received without local agent token")
	}
	err = agentauth.Verify(
		c.token,
		"RESPONSE",
		req.URL.RequestURI(),
		resp.Header.Get(agentauth.HeaderTimestamp),
		resp.Header.Get(agentauth.HeaderNonce),
		resp.Header.Get(agentauth.HeaderBodyHash),
		resp.Header.Get(agentauth.HeaderSignature),
		body,
		time.Now().UTC(),
		5*time.Minute,
	)
	if err != nil {
		return nil, fmt.Errorf("agent response signature verification failed: %w", err)
	}
	if c.responseReplay == nil {
		c.responseReplay = newResponseReplayCache(5 * time.Minute)
	}
	replayKey := req.URL.RequestURI() + ":" + strings.TrimSpace(resp.Header.Get(agentauth.HeaderNonce))
	if !c.responseReplay.accept(replayKey, time.Now().UTC()) {
		return nil, errors.New("agent response signature replay rejected")
	}
	return body, nil
}

func unsignedAgentResponseError(resp *http.Response, body []byte) error {
	statusCode := 0
	contentType := ""
	if resp != nil {
		statusCode = resp.StatusCode
		contentType = strings.TrimSpace(resp.Header.Get("Content-Type"))
	}
	preview := strings.Join(strings.Fields(string(body)), " ")
	const maxPreviewBytes = 256
	if len(preview) > maxPreviewBytes {
		preview = preview[:maxPreviewBytes] + "..."
	}
	if preview == "" {
		return fmt.Errorf("unsigned agent response rejected: status=%d content_type=%q", statusCode, contentType)
	}
	return fmt.Errorf("unsigned agent response rejected: status=%d content_type=%q body=%q", statusCode, contentType, preview)
}

func responseHasAgentSignature(resp *http.Response) bool {
	return resp != nil && (strings.TrimSpace(resp.Header.Get(agentauth.HeaderSignature)) != "" ||
		strings.TrimSpace(resp.Header.Get(agentauth.HeaderTimestamp)) != "" ||
		strings.TrimSpace(resp.Header.Get(agentauth.HeaderNonce)) != "" ||
		strings.TrimSpace(resp.Header.Get(agentauth.HeaderBodyHash)) != "")
}
