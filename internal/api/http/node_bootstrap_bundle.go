package http

import (
	nethttp "net/http"
	"strings"
)

func (s *Server) getNodeBootstrapBundle(w nethttp.ResponseWriter, r *nethttp.Request) {
	nodeID := idParam(r)
	runID := strings.TrimSpace(r.PathValue("run_id"))
	if nodeID == "" || runID == "" {
		writeErr(w, 400, "node_id and run_id are required")
		return
	}

	runs, err := s.store.ListNodeBootstrapRuns(r.Context(), nodeID, 200)
	if err != nil {
		writeErr(w, 500, "list node bootstrap runs failed")
		return
	}
	var runPayload map[string]any
	for _, run := range runs {
		if run.ID == runID {
			runPayload = run.ResultPayload
			break
		}
	}
	if runPayload == nil {
		writeErr(w, 404, "bootstrap run not found")
		return
	}

	secretRefID := strings.TrimSpace(stringifyHTTP(runPayload["agent_bootstrapenv_secret_ref_id"]))
	if secretRefID == "" {
		writeErr(w, 404, "manual bootstrap bundle is not available for this run")
		return
	}
	secretRef, rawBundle, err := s.store.ResolveSecretValue(r.Context(), secretRefID)
	if err != nil {
		writeErr(w, 409, "manual bootstrap bundle cannot be resolved")
		return
	}
	if strings.TrimSpace(stringifyHTTP(secretRef.Meta["node_id"])) != nodeID ||
		strings.TrimSpace(stringifyHTTP(secretRef.Meta["material"])) != "agent_bootstrap_env" {
		writeErr(w, 403, "manual bootstrap bundle secret scope mismatch")
		return
	}

	if authCtx, ok := authFromRequest(r); ok {
		_, _ = s.store.CreateAuditForUser(r.Context(), &authCtx.User.ID, "node.bootstrap_bundle.reveal", "node", &nodeID, "manual bootstrap bundle revealed")
	}
	writeJSON(w, 200, response{
		"node_id":            nodeID,
		"bootstrap_run_id":   runID,
		"agent_env":          stringifyHTTP(runPayload["agent_env"]),
		"agent_bootstrapenv": string(rawBundle),
		"enrollment_hint":    stringifyHTTP(runPayload["enrollment_hint"]),
	})
}
