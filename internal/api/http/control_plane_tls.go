package http

import (
	"net/http"
	"strings"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

type controlPlaneTLSSettingsRequest struct {
	Enabled              bool     `json:"enabled"`
	Mode                 string   `json:"mode"`
	PublicBaseURL        string   `json:"public_base_url"`
	ServerName           string   `json:"server_name"`
	ListenPort           int      `json:"listen_port"`
	UpstreamURL          string   `json:"upstream_url"`
	CertificateID        string   `json:"certificate_id"`
	SelfSignedCommonName string   `json:"self_signed_common_name"`
	SelfSignedDNSNames   []string `json:"self_signed_dns_names"`
}

func (s *Server) getControlPlaneTLSSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.store.GetControlPlaneTLSSettings(r.Context())
	if err != nil {
		writeErr(w, 500, "control plane tls settings lookup failed")
		return
	}
	writeJSON(w, 200, settings)
}

func (s *Server) updateControlPlaneTLSSettings(w http.ResponseWriter, r *http.Request) {
	var req controlPlaneTLSSettingsRequest
	if !decode(r, &req) {
		writeErr(w, 400, "invalid control plane tls settings payload")
		return
	}
	var certificateID *string
	if strings.TrimSpace(req.CertificateID) != "" {
		v := strings.TrimSpace(req.CertificateID)
		certificateID = &v
	}
	authCtx, ok := authFromRequest(r)
	if !ok {
		writeErr(w, 401, "auth required")
		return
	}
	updated, err := s.store.UpsertControlPlaneTLSSettings(r.Context(), domain.ControlPlaneTLSSettings{
		Enabled:              req.Enabled,
		Mode:                 strings.TrimSpace(req.Mode),
		PublicBaseURL:        strings.TrimSpace(req.PublicBaseURL),
		ServerName:           strings.TrimSpace(req.ServerName),
		ListenPort:           req.ListenPort,
		UpstreamURL:          strings.TrimSpace(req.UpstreamURL),
		CertificateID:        certificateID,
		SelfSignedCommonName: strings.TrimSpace(req.SelfSignedCommonName),
		SelfSignedDNSNames:   req.SelfSignedDNSNames,
	}, &authCtx.User.ID)
	if err != nil {
		writeErr(w, classifyControlPlaneTLSErrStatus(err), err.Error())
		return
	}
	writeJSON(w, 200, updated)
}

func (s *Server) applyControlPlaneTLSSettings(w http.ResponseWriter, r *http.Request) {
	job, err := s.store.CreateControlPlaneTLSApplyJob(r.Context())
	if err != nil {
		writeErr(w, classifyControlPlaneTLSErrStatus(err), err.Error())
		return
	}
	writeJSON(w, 202, redactedJob(job))
}

func classifyControlPlaneTLSErrStatus(err error) int {
	switch {
	case err == nil:
		return 500
	case strings.Contains(err.Error(), "required"),
		strings.Contains(err.Error(), "invalid"),
		strings.Contains(err.Error(), "unsupported"),
		strings.Contains(err.Error(), "must"):
		return 400
	default:
		return 500
	}
}
