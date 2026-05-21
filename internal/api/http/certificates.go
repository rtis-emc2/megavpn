package http

import (
	"net/http"
	"strings"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

type importPlatformCertificateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Certificate string `json:"certificate"`
	PrivateKey  string `json:"private_key"`
	Chain       string `json:"chain"`
	IsDefault   bool   `json:"is_default"`
}

type selfSignedPlatformCertificateRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	CommonName  string   `json:"common_name"`
	DNSNames    []string `json:"dns_names"`
	ValidDays   int      `json:"valid_days"`
	IsDefault   bool     `json:"is_default"`
}

type managedPlatformCertificateAuthorityRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	CommonName  string `json:"common_name"`
	ValidDays   int    `json:"valid_days"`
}

type issuePlatformCertificateFromAuthorityRequest struct {
	AuthorityCertificateID string   `json:"authority_certificate_id"`
	Name                   string   `json:"name"`
	Description            string   `json:"description"`
	CommonName             string   `json:"common_name"`
	DNSNames               []string `json:"dns_names"`
	ValidDays              int      `json:"valid_days"`
	IsDefault              bool     `json:"is_default"`
}

type managedPlatformServicePKIRootRequest struct {
	ServiceCode string `json:"service_code"`
	PKIProfile  string `json:"pki_profile"`
	CommonName  string `json:"common_name"`
	ValidDays   int    `json:"valid_days"`
}

func (s *Server) listPlatformCertificates(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListPlatformCertificates(r.Context())
	if err != nil {
		writeErr(w, 500, "list platform certificates failed")
		return
	}
	if items == nil {
		items = []domain.PlatformCertificate{}
	}
	writeJSON(w, 200, items)
}

func (s *Server) importPlatformCertificate(w http.ResponseWriter, r *http.Request) {
	var req importPlatformCertificateRequest
	if !decode(r, &req) || strings.TrimSpace(req.Certificate) == "" || strings.TrimSpace(req.PrivateKey) == "" {
		writeErr(w, 400, "invalid platform certificate import payload")
		return
	}
	item, err := s.store.ImportPlatformCertificate(
		r.Context(),
		req.Name,
		req.Description,
		[]byte(req.Certificate),
		[]byte(req.PrivateKey),
		[]byte(req.Chain),
		req.IsDefault,
	)
	if err != nil {
		writeErr(w, classifyCertificateErrStatus(err), err.Error())
		return
	}
	writeJSON(w, 201, item)
}

func (s *Server) createSelfSignedPlatformCertificate(w http.ResponseWriter, r *http.Request) {
	var req selfSignedPlatformCertificateRequest
	if !decode(r, &req) || strings.TrimSpace(req.CommonName) == "" {
		writeErr(w, 400, "invalid self-signed certificate payload")
		return
	}
	item, err := s.store.CreateSelfSignedPlatformCertificate(r.Context(), req.Name, req.Description, req.CommonName, req.DNSNames, req.ValidDays, req.IsDefault)
	if err != nil {
		writeErr(w, classifyCertificateErrStatus(err), err.Error())
		return
	}
	writeJSON(w, 201, item)
}

func (s *Server) createManagedPlatformCertificateAuthority(w http.ResponseWriter, r *http.Request) {
	var req managedPlatformCertificateAuthorityRequest
	if !decode(r, &req) || strings.TrimSpace(req.CommonName) == "" {
		writeErr(w, 400, "invalid managed ca payload")
		return
	}
	item, err := s.store.CreateManagedPlatformCertificateAuthority(r.Context(), req.Name, req.Description, req.CommonName, req.ValidDays)
	if err != nil {
		writeErr(w, classifyCertificateErrStatus(err), err.Error())
		return
	}
	writeJSON(w, 201, item)
}

func (s *Server) issuePlatformCertificateFromAuthority(w http.ResponseWriter, r *http.Request) {
	var req issuePlatformCertificateFromAuthorityRequest
	if !decode(r, &req) || strings.TrimSpace(req.AuthorityCertificateID) == "" || strings.TrimSpace(req.CommonName) == "" {
		writeErr(w, 400, "invalid issue-from-ca payload")
		return
	}
	item, err := s.store.IssuePlatformCertificateFromAuthority(
		r.Context(),
		req.AuthorityCertificateID,
		req.Name,
		req.Description,
		req.CommonName,
		req.DNSNames,
		req.ValidDays,
		req.IsDefault,
	)
	if err != nil {
		writeErr(w, classifyCertificateErrStatus(err), err.Error())
		return
	}
	writeJSON(w, 201, item)
}

func (s *Server) createManagedPlatformServicePKIRoot(w http.ResponseWriter, r *http.Request) {
	var req managedPlatformServicePKIRootRequest
	if !decode(r, &req) || strings.TrimSpace(req.ServiceCode) == "" {
		writeErr(w, 400, "invalid platform pki root payload")
		return
	}
	root, err := s.store.CreateManagedPlatformServicePKIRoot(r.Context(), req.ServiceCode, req.PKIProfile, req.CommonName, req.ValidDays)
	if err != nil {
		writeErr(w, classifyCertificateErrStatus(err), err.Error())
		return
	}
	writeJSON(w, 201, root)
}

func classifyCertificateErrStatus(err error) int {
	switch {
	case err == nil:
		return 500
	case strings.Contains(err.Error(), "required"),
		strings.Contains(err.Error(), "invalid"),
		strings.Contains(err.Error(), "unsupported"),
		strings.Contains(err.Error(), "must not"),
		strings.Contains(err.Error(), "not a CA"):
		return 400
	case strings.Contains(err.Error(), "already exists"):
		return 409
	default:
		return 500
	}
}
