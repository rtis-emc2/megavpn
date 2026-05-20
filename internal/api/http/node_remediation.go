package http

import nethttp "net/http"

func (s *Server) retryNodeInventorySync(w nethttp.ResponseWriter, r *nethttp.Request) {
	job, err := s.store.CreateNodeInventoryJob(r.Context(), idParam(r))
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 202, response{
		"status":  "queued",
		"message": "inventory sync queued",
		"job":     job,
	})
}

func (s *Server) retryNodeDiscoverySync(w nethttp.ResponseWriter, r *nethttp.Request) {
	job, err := s.store.CreateNodeServiceDiscoveryJob(r.Context(), idParam(r))
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 202, response{
		"status":  "queued",
		"message": "discovery sync queued",
		"job":     job,
	})
}

func (s *Server) requeueNodeStuckJob(w nethttp.ResponseWriter, r *nethttp.Request) {
	job, err := s.store.RequeueNodeStuckJob(r.Context(), idParam(r))
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 202, response{
		"status":  "queued",
		"message": "stuck node job requeued",
		"job":     job,
	})
}

func (s *Server) probeNodeChannel(w nethttp.ResponseWriter, r *nethttp.Request) {
	job, err := s.store.CreateNodeChannelProbeJob(r.Context(), idParam(r))
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 202, response{
		"status":  "queued",
		"message": "agent channel probe queued",
		"job":     job,
	})
}

func (s *Server) clearNodeStaleRotation(w nethttp.ResponseWriter, r *nethttp.Request) {
	jobs, err := s.store.ClearNodeStalePendingRotation(r.Context(), idParam(r))
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 200, response{
		"status":        "ok",
		"message":       "stale rotation jobs cleared",
		"cleared_count": len(jobs),
		"jobs":          jobs,
	})
}
