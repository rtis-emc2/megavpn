package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/rtis-emc2/megavpn/internal/agentauth"
	"github.com/rtis-emc2/megavpn/internal/platform/config"
)

func newClientWithHTTP(baseURL, token, statePath string, httpClient httpDoer) *client {
	return &client{
		baseURL:                      strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		token:                        token,
		statePath:                    statePath,
		http:                         httpClient,
		responseReplay:               newResponseReplayCache(5 * time.Minute),
		trafficReportInterval:        time.Minute,
		xrayTrafficCounterState:      map[string]int64{},
		wireGuardTrafficCounterState: map[string]int64{},
		openVPNTrafficCounterState:   map[string]int64{},
	}
}

func newConfiguredClient(baseURL, token, statePath string, cfg config.AgentConfig) (*client, error) {
	httpClient, err := newAgentHTTPClient(baseURL, cfg)
	if err != nil {
		return nil, err
	}
	return newClientWithHTTP(baseURL, token, statePath, httpClient), nil
}

func newAgentHTTPClient(baseURL string, cfg config.AgentConfig) (*http.Client, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return nil, fmt.Errorf("invalid agent control-plane URL %q", baseURL)
	}
	if parsed.User != nil {
		return nil, errors.New("agent control-plane URL must not contain credentials")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("agent control-plane URL must not contain query parameters or a fragment")
	}
	if parsed.Scheme == "http" && !cfg.AllowInsecureHTTP && !isLoopbackControlPlaneHost(parsed.Hostname()) {
		return nil, errors.New("remote agent control-plane URL must use HTTPS; set MEGAVPN_AGENT_ALLOW_INSECURE_HTTP=true only for an isolated development environment")
	}
	if parsed.Scheme != "https" && (cfg.TLSCAFile != "" || cfg.TLSCertFile != "" || cfg.TLSKeyFile != "") {
		return nil, errors.New("agent TLS files require an HTTPS control-plane URL")
	}
	if (cfg.TLSCertFile == "") != (cfg.TLSKeyFile == "") {
		return nil, errors.New("MEGAVPN_AGENT_TLS_CERT_FILE and MEGAVPN_AGENT_TLS_KEY_FILE must be configured together")
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: parsed.Hostname()}
	if cfg.TLSCAFile != "" {
		caPEM, err := os.ReadFile(cfg.TLSCAFile)
		if err != nil {
			return nil, fmt.Errorf("read agent TLS CA file: %w", err)
		}
		roots, err := x509.SystemCertPool()
		if err != nil || roots == nil {
			roots = x509.NewCertPool()
		}
		if !roots.AppendCertsFromPEM(caPEM) {
			return nil, errors.New("agent TLS CA file contains no valid certificates")
		}
		tlsConfig.RootCAs = roots
	}
	if cfg.TLSCertFile != "" {
		certificate, err := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile)
		if err != nil {
			return nil, fmt.Errorf("load agent TLS client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{certificate}
	}
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          20,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		ExpectContinueTimeout: time.Second,
		TLSClientConfig:       tlsConfig,
	}
	return &http.Client{Transport: transport, Timeout: 30 * time.Second}, nil
}

func isLoopbackControlPlaneHost(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
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
	if err := c.signRequest(req, b); err != nil {
		return err
	}
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

func (c client) signRequest(req *http.Request, body []byte) error {
	if req == nil || strings.TrimSpace(c.token) == "" {
		return nil
	}
	timestamp := strconv.FormatInt(time.Now().UTC().Unix(), 10)
	nonce, err := agentauth.NewNonce()
	if err != nil {
		return fmt.Errorf("generate signed agent request nonce: %w", err)
	}
	signature, bodyHash := agentauth.Sign(c.token, req.Method, req.URL.RequestURI(), timestamp, nonce, body)
	req.Header.Set(agentauth.HeaderTimestamp, timestamp)
	req.Header.Set(agentauth.HeaderNonce, nonce)
	req.Header.Set(agentauth.HeaderBodyHash, bodyHash)
	req.Header.Set(agentauth.HeaderSignature, signature)
	return nil
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
		return nil, fmt.Errorf(
			"agent response signature verification failed: %w",
			describeSignedResponseVerificationFailure(err, resp.Header.Get(agentauth.HeaderTimestamp), time.Now().UTC()),
		)
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

func describeSignedResponseVerificationFailure(err error, timestamp string, now time.Time) error {
	if !errors.Is(err, agentauth.ErrTimestampOutdated) {
		return err
	}
	serverUnix, parseErr := strconv.ParseInt(strings.TrimSpace(timestamp), 10, 64)
	if parseErr != nil || serverUnix <= 0 {
		return err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	skew := time.Unix(serverUnix, 0).UTC().Sub(now)
	direction := "ahead of"
	if skew < 0 {
		direction = "behind"
		skew = -skew
	}
	return fmt.Errorf(
		"%w: observed control-plane clock is %s local clock by %s; synchronize NTP on both hosts",
		err,
		direction,
		skew.Round(time.Second),
	)
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
