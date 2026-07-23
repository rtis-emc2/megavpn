package http

import (
	"context"
	"encoding/json"
	"errors"
	nethttp "net/http"
	"strings"

	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/externalegress"
)

type externalEgressImportPreviewRequest struct {
	Protocol string `json:"protocol"`
	Format   string `json:"format"`
	Content  string `json:"content"`
}

func (s *Server) listExternalEgressCatalog(w nethttp.ResponseWriter, _ *nethttp.Request) {
	writeJSON(w, nethttp.StatusOK, externalegress.AvailableCatalog())
}

func (s *Server) previewExternalEgressImport(w nethttp.ResponseWriter, r *nethttp.Request) {
	var req externalEgressImportPreviewRequest
	if !decode(r, &req) {
		writeErr(w, nethttp.StatusBadRequest, "invalid external egress import payload")
		return
	}
	protocol := externalegress.NormalizeProtocol(req.Protocol)
	def, ok := externalegress.Definition(protocol)
	if !ok {
		writeErr(w, nethttp.StatusBadRequest, "unsupported external egress protocol")
		return
	}
	preview := domain.ExternalEgressImportPreview{
		Protocol: protocol, RuntimeSupport: def.RuntimeSupport,
		ImportFormat: strings.ToLower(strings.TrimSpace(req.Format)),
	}
	if preview.ImportFormat == "" {
		preview.ImportFormat = "structured"
	}
	var normalized any
	var err error
	switch protocol {
	case "openvpn":
		var parsed externalegress.OpenVPNPreview
		parsed, err = externalegress.ParseOpenVPN([]byte(req.Content))
		if err == nil {
			preview.Transport, preview.EndpointHost, preview.EndpointPort = parsed.Transport, parsed.EndpointHost, parsed.EndpointPort
			preview.RequiredSecrets, preview.InlineBlocks, preview.Warnings = parsed.RequiredSecrets, parsed.InlineBlocks, parsed.Warnings
			normalized = parsed
		}
	case "wireguard":
		var parsed externalegress.WireGuardPreview
		parsed, err = externalegress.ParseWireGuard([]byte(req.Content))
		if err == nil {
			preview.Transport, preview.EndpointHost, preview.EndpointPort = "udp", parsed.EndpointHost, parsed.EndpointPort
			preview.RequiredSecrets = parsed.RequiredSecrets
			preview.Warnings = parsed.Warnings
			normalized = parsed
		}
	case "shadowsocks":
		var parsed externalegress.ShadowsocksPreview
		parsed, err = externalegress.ParseShadowsocks([]byte(req.Content))
		if err == nil {
			preview.Transport, preview.EndpointHost, preview.EndpointPort = parsed.Transport, parsed.EndpointHost, parsed.EndpointPort
			preview.RequiredSecrets, preview.Warnings = parsed.RequiredSecrets, parsed.Warnings
			normalized = parsed
		}
	case "vless":
		var parsed externalegress.VLESSPreview
		parsed, err = externalegress.ParseVLESS([]byte(req.Content))
		if err == nil {
			preview.Transport, preview.EndpointHost, preview.EndpointPort = parsed.Transport, parsed.EndpointHost, parsed.EndpointPort
			preview.RequiredSecrets, preview.Warnings = parsed.RequiredSecrets, parsed.Warnings
			normalized = parsed
		}
	case "l2tp_ipsec":
		var parsed externalegress.L2TPIPsecPreview
		parsed, err = externalegress.ParseL2TPIPsec([]byte(req.Content))
		if err == nil {
			preview.Transport, preview.EndpointHost, preview.EndpointPort = parsed.Transport, parsed.EndpointHost, parsed.EndpointPort
			preview.RequiredSecrets, preview.Warnings = parsed.RequiredSecrets, parsed.Warnings
			normalized = parsed
		}
	default:
		err = errors.New("this protocol currently supports structured profile creation only")
	}
	if err != nil {
		writeErr(w, nethttp.StatusUnprocessableEntity, err.Error())
		return
	}
	if raw, marshalErr := json.Marshal(normalized); marshalErr == nil {
		preview.Normalized = raw
	}
	writeJSON(w, nethttp.StatusOK, preview)
}

func (s *Server) listExternalEgressProfiles(w nethttp.ResponseWriter, r *nethttp.Request) {
	profiles, err := s.store.ListExternalEgressProfiles(r.Context())
	if err != nil {
		writeErr(w, nethttp.StatusInternalServerError, "list external egress profiles failed")
		return
	}
	writeJSON(w, nethttp.StatusOK, profiles)
}

func (s *Server) getExternalEgressProfile(w nethttp.ResponseWriter, r *nethttp.Request) {
	profile, err := s.store.GetExternalEgressProfile(r.Context(), strings.TrimSpace(r.PathValue("profile_id")))
	if errors.Is(err, domain.ErrExternalEgressProfileNotFound) {
		writeErr(w, nethttp.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		writeErr(w, nethttp.StatusInternalServerError, "get external egress profile failed")
		return
	}
	writeJSON(w, nethttp.StatusOK, profile)
}

func externalEgressActor(r *nethttp.Request) *string {
	if authCtx, ok := authFromRequest(r); ok {
		return &authCtx.User.ID
	}
	return nil
}

func (s *Server) createExternalEgressProfile(w nethttp.ResponseWriter, r *nethttp.Request) {
	var input domain.ExternalEgressProfileInput
	if !decode(r, &input) {
		writeErr(w, nethttp.StatusBadRequest, "invalid external egress profile payload")
		return
	}
	profile, err := s.store.CreateExternalEgressProfile(r.Context(), input, externalEgressActor(r))
	if err != nil {
		writeErr(w, nethttp.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, nethttp.StatusCreated, profile)
}

func (s *Server) updateExternalEgressProfile(w nethttp.ResponseWriter, r *nethttp.Request) {
	var input domain.ExternalEgressProfileInput
	if !decode(r, &input) {
		writeErr(w, nethttp.StatusBadRequest, "invalid external egress profile payload")
		return
	}
	profile, err := s.store.UpdateExternalEgressProfile(r.Context(), strings.TrimSpace(r.PathValue("profile_id")), input, externalEgressActor(r))
	if errors.Is(err, domain.ErrExternalEgressProfileNotFound) {
		writeErr(w, nethttp.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		writeErr(w, nethttp.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, nethttp.StatusOK, profile)
}

func (s *Server) deleteExternalEgressProfile(w nethttp.ResponseWriter, r *nethttp.Request) {
	profile, err := s.store.DeleteExternalEgressProfile(r.Context(), strings.TrimSpace(r.PathValue("profile_id")), externalEgressActor(r))
	if errors.Is(err, domain.ErrExternalEgressProfileNotFound) {
		writeErr(w, nethttp.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		writeErr(w, nethttp.StatusConflict, err.Error())
		return
	}
	writeJSON(w, nethttp.StatusOK, profile)
}

func (s *Server) createExternalEgressDeployment(w nethttp.ResponseWriter, r *nethttp.Request) {
	var input domain.ExternalEgressDeploymentInput
	if !decode(r, &input) {
		writeErr(w, nethttp.StatusBadRequest, "invalid external egress deployment payload")
		return
	}
	deployment, err := s.store.CreateExternalEgressDeployment(r.Context(), strings.TrimSpace(r.PathValue("profile_id")), input)
	if err != nil {
		writeErr(w, nethttp.StatusConflict, err.Error())
		return
	}
	writeJSON(w, nethttp.StatusCreated, deployment)
}

func (s *Server) applyExternalEgressDeployment(w nethttp.ResponseWriter, r *nethttp.Request) {
	s.externalEgressDeploymentJob(w, r, s.store.CreateExternalEgressApplyJob)
}

func (s *Server) probeExternalEgressDeployment(w nethttp.ResponseWriter, r *nethttp.Request) {
	s.externalEgressDeploymentJob(w, r, s.store.CreateExternalEgressProbeJob)
}

func (s *Server) cleanupExternalEgressDeployment(w nethttp.ResponseWriter, r *nethttp.Request) {
	s.externalEgressDeploymentJob(w, r, s.store.CreateExternalEgressCleanupJob)
}

func (s *Server) externalEgressDeploymentJob(w nethttp.ResponseWriter, r *nethttp.Request, create func(context.Context, string) (domain.Job, error)) {
	job, err := create(r.Context(), strings.TrimSpace(r.PathValue("deployment_id")))
	if errors.Is(err, domain.ErrExternalEgressDeploymentNotFound) {
		writeErr(w, nethttp.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		writeErr(w, nethttp.StatusConflict, err.Error())
		return
	}
	writeJSON(w, nethttp.StatusAccepted, redactedJob(job))
}
