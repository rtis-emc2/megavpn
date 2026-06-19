package http

import (
	"net/http"
	"strings"

	"github.com/rtis-emc2/megavpn/internal/backhaul"
	"github.com/rtis-emc2/megavpn/internal/domain"
)

type createBackhaulLinkRequest struct {
	Name          string   `json:"name"`
	IngressNodeID string   `json:"ingress_node_id"`
	EgressNodeID  string   `json:"egress_node_id"`
	DesiredDriver string   `json:"desired_driver"`
	EndpointHost  string   `json:"endpoint_host"`
	TunnelCIDR    string   `json:"tunnel_cidr"`
	RoutingTable  string   `json:"routing_table"`
	RouteMetric   int      `json:"route_metric"`
	Drivers       []string `json:"drivers"`
}

func (s *Server) listBackhaulDrivers(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, backhaul.Catalog())
}

func (s *Server) listBackhaulLinks(w http.ResponseWriter, r *http.Request) {
	links, err := s.store.ListBackhaulLinks(r.Context())
	if err != nil {
		writeErr(w, 500, "list backhaul links failed")
		return
	}
	writeJSON(w, 200, links)
}

func (s *Server) getBackhaulLink(w http.ResponseWriter, r *http.Request) {
	link, err := s.store.GetBackhaulLink(r.Context(), idParam(r))
	if err != nil {
		writeErr(w, 404, "backhaul link not found")
		return
	}
	writeJSON(w, 200, link)
}

func (s *Server) createBackhaulLink(w http.ResponseWriter, r *http.Request) {
	var req createBackhaulLinkRequest
	if !decode(r, &req) {
		writeErr(w, 400, "invalid backhaul payload")
		return
	}
	metadata := map[string]any{}
	if strings.TrimSpace(req.EndpointHost) != "" {
		metadata["endpoint_host"] = strings.TrimSpace(req.EndpointHost)
	}
	if strings.TrimSpace(req.TunnelCIDR) != "" {
		metadata["tunnel_cidr"] = strings.TrimSpace(req.TunnelCIDR)
	}
	if len(req.Drivers) > 0 {
		drivers := make([]any, 0, len(req.Drivers))
		for _, driver := range req.Drivers {
			if strings.TrimSpace(driver) != "" {
				drivers = append(drivers, strings.TrimSpace(driver))
			}
		}
		metadata["drivers"] = drivers
	}
	link, err := s.store.CreateBackhaulLink(r.Context(), domain.BackhaulLink{
		Name:          req.Name,
		IngressNodeID: req.IngressNodeID,
		EgressNodeID:  req.EgressNodeID,
		DesiredDriver: req.DesiredDriver,
		RoutingTable:  req.RoutingTable,
		RouteMetric:   req.RouteMetric,
		Metadata:      metadata,
	})
	if err != nil {
		writeErr(w, classifyBackhaulErrStatus(err), err.Error())
		return
	}
	writeJSON(w, 201, link)
}

func (s *Server) applyBackhaulLink(w http.ResponseWriter, r *http.Request) {
	jobs, err := s.store.CreateBackhaulApplyJobs(r.Context(), idParam(r))
	if err != nil {
		writeErr(w, classifyBackhaulErrStatus(err), err.Error())
		return
	}
	out := make([]domain.Job, 0, len(jobs))
	for _, job := range jobs {
		out = append(out, redactedJob(job))
	}
	writeJSON(w, 202, response{"jobs": out, "job_count": len(out)})
}

func (s *Server) deleteBackhaulLink(w http.ResponseWriter, r *http.Request) {
	link, err := s.store.DeleteBackhaulLink(r.Context(), idParam(r))
	if err != nil {
		writeErr(w, classifyBackhaulErrStatus(err), err.Error())
		return
	}
	writeJSON(w, 200, link)
}

func classifyBackhaulErrStatus(err error) int {
	switch {
	case err == nil:
		return 500
	case strings.Contains(err.Error(), "not found"):
		return 404
	case strings.Contains(err.Error(), "required"),
		strings.Contains(err.Error(), "unsupported"),
		strings.Contains(err.Error(), "invalid"),
		strings.Contains(err.Error(), "must"):
		return 400
	default:
		return 409
	}
}
