package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	nethttp "net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/infra/postgres"
)

const (
	maxNodeBootstrapBundleBytes       = 256 * 1024
	nodeBootstrapBundleRevealAction   = "node.bootstrap_bundle.reveal"
	nodeBootstrapBundleDownloadAction = "node.bootstrap_bundle.download"
	nodeBootstrapBundleMaterial       = "agent_bootstrap_env"
	nodeBootstrapBundleSecretType     = "opaque"
)

type nodeBootstrapBundle struct {
	Node       domain.Node
	Run        domain.NodeBootstrapRun
	Filename   string
	Bundle     []byte
	RevealedAt time.Time
}

type nodeBootstrapBundleRevealResponse struct {
	NodeID            string    `json:"node_id"`
	BootstrapRunID    string    `json:"bootstrap_run_id"`
	Filename          string    `json:"filename"`
	AgentBootstrapEnv string    `json:"agent_bootstrapenv"`
	RevealedAt        time.Time `json:"revealed_at"`
}

type nodeBootstrapBundleError struct {
	status  int
	message string
}

func (e nodeBootstrapBundleError) Error() string { return e.message }

func (s *Server) getNodeBootstrapBundle(w nethttp.ResponseWriter, r *nethttp.Request) {
	setSecretBearingResponseHeaders(w)
	bundle, err := s.resolveNodeBootstrapBundle(r.Context(), idParam(r), r.PathValue("run_id"))
	if err != nil {
		writeNodeBootstrapBundleError(w, err)
		return
	}
	defer zeroNodeBootstrapBundle(bundle.Bundle)
	if !s.auditNodeBootstrapBundleAccess(w, r, bundle, nodeBootstrapBundleRevealAction) {
		return
	}
	writeJSON(w, 200, nodeBootstrapBundleRevealDTO(bundle))
}

func (s *Server) revealNodeBootstrapBundle(w nethttp.ResponseWriter, r *nethttp.Request) {
	setSecretBearingResponseHeaders(w)
	if !decodeEmptyJSONBody(r) {
		writeErr(w, 400, "manual bootstrap bundle request body must be empty or {}")
		return
	}
	bundle, err := s.resolveNodeBootstrapBundle(r.Context(), idParam(r), r.PathValue("run_id"))
	if err != nil {
		writeNodeBootstrapBundleError(w, err)
		return
	}
	defer zeroNodeBootstrapBundle(bundle.Bundle)
	if !s.auditNodeBootstrapBundleAccess(w, r, bundle, nodeBootstrapBundleRevealAction) {
		return
	}
	writeJSON(w, 200, nodeBootstrapBundleRevealDTO(bundle))
}

func (s *Server) downloadNodeBootstrapBundle(w nethttp.ResponseWriter, r *nethttp.Request) {
	setSecretBearingResponseHeaders(w)
	if !decodeEmptyJSONBody(r) {
		writeErr(w, 400, "manual bootstrap bundle request body must be empty or {}")
		return
	}
	bundle, err := s.resolveNodeBootstrapBundle(r.Context(), idParam(r), r.PathValue("run_id"))
	if err != nil {
		writeNodeBootstrapBundleError(w, err)
		return
	}
	defer zeroNodeBootstrapBundle(bundle.Bundle)
	if !s.auditNodeBootstrapBundleAccess(w, r, bundle, nodeBootstrapBundleDownloadAction) {
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/plain; charset=utf-8")
	h.Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": bundle.Filename}))
	h.Del("ETag")
	h.Del("Last-Modified")
	w.WriteHeader(200)
	_, _ = w.Write(bundle.Bundle)
}

func decodeEmptyJSONBody(r *nethttp.Request) bool {
	body, err := requestBodySnapshot(r)
	if err != nil {
		return false
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return true
	}
	var raw map[string]json.RawMessage
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&raw); err != nil {
		return false
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return false
	}
	return raw != nil && len(raw) == 0
}

func (s *Server) resolveNodeBootstrapBundle(ctx context.Context, nodeID string, runID string) (nodeBootstrapBundle, error) {
	nodeID = strings.TrimSpace(nodeID)
	runID = strings.TrimSpace(runID)
	if nodeID == "" || runID == "" {
		return nodeBootstrapBundle{}, nodeBootstrapBundleError{status: 400, message: "node_id and run_id are required"}
	}
	node, err := s.store.GetNode(ctx, nodeID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nodeBootstrapBundle{}, nodeBootstrapBundleError{status: 404, message: "node not found"}
		}
		return nodeBootstrapBundle{}, nodeBootstrapBundleError{status: 500, message: "load node failed"}
	}
	run, err := s.store.GetNodeBootstrapRun(ctx, nodeID, runID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nodeBootstrapBundle{}, nodeBootstrapBundleError{status: 404, message: "bootstrap run not found"}
		}
		return nodeBootstrapBundle{}, nodeBootstrapBundleError{status: 500, message: "load bootstrap run failed"}
	}
	if strings.TrimSpace(run.NodeID) != nodeID {
		return nodeBootstrapBundle{}, nodeBootstrapBundleError{status: 403, message: "bootstrap run node scope mismatch"}
	}
	if strings.TrimSpace(run.BootstrapMode) != "manual_bundle" {
		return nodeBootstrapBundle{}, nodeBootstrapBundleError{status: 404, message: "manual bootstrap bundle is not available for this run"}
	}
	if !strings.EqualFold(strings.TrimSpace(run.Status), "succeeded") {
		return nodeBootstrapBundle{}, nodeBootstrapBundleError{status: 409, message: "manual bootstrap bundle is only available after a succeeded bootstrap run"}
	}
	secretRefID := strings.TrimSpace(stringifyHTTP(run.ResultPayload[nodeBootstrapBundleSecretRefKey]))
	if secretRefID == "" {
		return nodeBootstrapBundle{}, nodeBootstrapBundleError{status: 404, message: "manual bootstrap bundle is not available for this run"}
	}
	secretRef, rawBundle, err := s.store.ResolveSecretValue(ctx, secretRefID)
	if err != nil {
		if errors.Is(err, postgres.ErrSecretServiceUnavailable) {
			return nodeBootstrapBundle{}, nodeBootstrapBundleError{status: 503, message: "manual bootstrap bundle secret service unavailable"}
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return nodeBootstrapBundle{}, nodeBootstrapBundleError{status: 409, message: "manual bootstrap bundle secret is missing"}
		}
		return nodeBootstrapBundle{}, nodeBootstrapBundleError{status: 409, message: "manual bootstrap bundle cannot be resolved"}
	}
	if strings.TrimSpace(secretRef.SecretType) != nodeBootstrapBundleSecretType {
		zeroNodeBootstrapBundle(rawBundle)
		return nodeBootstrapBundle{}, nodeBootstrapBundleError{status: 409, message: "manual bootstrap bundle secret type mismatch"}
	}
	if err := validateNodeBootstrapBundleSecretScope(secretRef, nodeID, runID); err != nil {
		zeroNodeBootstrapBundle(rawBundle)
		return nodeBootstrapBundle{}, err
	}
	if len(rawBundle) == 0 {
		return nodeBootstrapBundle{}, nodeBootstrapBundleError{status: 409, message: "manual bootstrap bundle is empty"}
	}
	if len(rawBundle) > maxNodeBootstrapBundleBytes {
		zeroNodeBootstrapBundle(rawBundle)
		return nodeBootstrapBundle{}, nodeBootstrapBundleError{status: 413, message: "manual bootstrap bundle is too large"}
	}
	return nodeBootstrapBundle{
		Node:       node,
		Run:        run,
		Filename:   safeNodeBootstrapBundleFilename(node),
		Bundle:     rawBundle,
		RevealedAt: time.Now().UTC(),
	}, nil
}

func validateNodeBootstrapBundleSecretScope(secretRef domain.SecretRef, nodeID string, runID string) error {
	metaNodeID := strings.TrimSpace(stringifyHTTP(secretRef.Meta["node_id"]))
	if metaNodeID != nodeID {
		return nodeBootstrapBundleError{status: 403, message: "manual bootstrap bundle secret node scope mismatch"}
	}
	if material := strings.TrimSpace(stringifyHTTP(secretRef.Meta["material"])); material != nodeBootstrapBundleMaterial {
		return nodeBootstrapBundleError{status: 403, message: "manual bootstrap bundle secret material mismatch"}
	}
	if metaRunID := strings.TrimSpace(stringifyHTTP(secretRef.Meta["bootstrap_run_id"])); metaRunID != "" && metaRunID != runID {
		return nodeBootstrapBundleError{status: 403, message: "manual bootstrap bundle secret run scope mismatch"}
	}
	return nil
}

func (s *Server) auditNodeBootstrapBundleAccess(w nethttp.ResponseWriter, r *nethttp.Request, bundle nodeBootstrapBundle, action string) bool {
	authCtx, ok := authFromRequest(r)
	if !ok {
		writeErr(w, 401, "authentication required")
		return false
	}
	nodeID := bundle.Node.ID
	if strings.TrimSpace(nodeID) == "" {
		nodeID = bundle.Run.NodeID
	}
	summary := "manual bootstrap bundle accessed"
	if runID := strings.TrimSpace(bundle.Run.ID); runID != "" {
		summary += " for bootstrap run " + runID
	}
	if _, err := s.store.CreateAuditForUser(r.Context(), &authCtx.User.ID, action, "node", &nodeID, summary); err != nil {
		writeErr(w, 500, "manual bootstrap bundle audit failed")
		return false
	}
	return true
}

func nodeBootstrapBundleRevealDTO(bundle nodeBootstrapBundle) nodeBootstrapBundleRevealResponse {
	return nodeBootstrapBundleRevealResponse{
		NodeID:            bundle.Run.NodeID,
		BootstrapRunID:    bundle.Run.ID,
		Filename:          bundle.Filename,
		AgentBootstrapEnv: string(bundle.Bundle),
		RevealedAt:        bundle.RevealedAt,
	}
}

func writeNodeBootstrapBundleError(w nethttp.ResponseWriter, err error) {
	var bundleErr nodeBootstrapBundleError
	if errors.As(err, &bundleErr) {
		writeErr(w, bundleErr.status, bundleErr.message)
		return
	}
	writeErr(w, 500, "manual bootstrap bundle access failed")
}

func setSecretBearingResponseHeaders(w nethttp.ResponseWriter) {
	h := w.Header()
	h.Set("Cache-Control", "no-store, private, max-age=0")
	h.Set("Pragma", "no-cache")
	h.Set("Expires", "0")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	h.Del("ETag")
	h.Del("Last-Modified")
}

func secretBearingHeadersMiddleware(next nethttp.Handler) nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		setSecretBearingResponseHeaders(w)
		next.ServeHTTP(w, r)
	})
}

func safeNodeBootstrapBundleFilename(node domain.Node) string {
	component := sanitizeNodeBootstrapBundleFilenameComponent(node.Name)
	if component == "" {
		component = sanitizeNodeBootstrapBundleFilenameComponent(node.ID)
	}
	if component == "" {
		component = "node"
	}
	return "megavpn-agent-" + component + "-bootstrap.env"
}

func sanitizeNodeBootstrapBundleFilenameComponent(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		var out rune
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			out = r
		case r == '.', r == '_':
			out = r
		case r == '-', r == ' ', r == '\t':
			out = '-'
		default:
			continue
		}
		if out == '-' {
			if lastDash {
				continue
			}
			lastDash = true
		} else {
			lastDash = false
		}
		b.WriteRune(out)
		if b.Len() >= 64 {
			break
		}
	}
	return strings.Trim(b.String(), ".-_")
}

func zeroNodeBootstrapBundle(buf []byte) {
	for i := range buf {
		buf[i] = 0
	}
}
