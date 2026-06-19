package http

import (
	"net/http"
	"strings"
	"time"

	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/pki"
)

type importPlatformCertificateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Certificate string `json:"certificate"`
	PrivateKey  string `json:"private_key"`
	Chain       string `json:"chain"`
	IsDefault   bool   `json:"is_default"`
}

type previewPlatformCertificateResponse struct {
	CommonName       string     `json:"common_name"`
	IssuerName       string     `json:"issuer_name"`
	SANs             []string   `json:"sans"`
	NotBefore        *time.Time `json:"not_before,omitempty"`
	NotAfter         *time.Time `json:"not_after,omitempty"`
	IsCA             bool       `json:"is_ca"`
	PrivateKeyType   string     `json:"private_key_type,omitempty"`
	KeyPairValid     bool       `json:"key_pair_valid"`
	ChainCertificate int        `json:"chain_certificate_count"`
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

func (s *Server) previewPlatformCertificate(w http.ResponseWriter, r *http.Request) {
	var req importPlatformCertificateRequest
	if !decode(r, &req) || strings.TrimSpace(req.Certificate) == "" || strings.TrimSpace(req.PrivateKey) == "" {
		writeErr(w, 400, "certificate file and private key file are required")
		return
	}
	desc, err := pki.DescribeCertificatePEM([]byte(req.Certificate))
	if err != nil {
		writeErr(w, classifyCertificateErrStatus(err), err.Error())
		return
	}
	keyType, err := pki.DescribePrivateKeyPEM([]byte(req.PrivateKey))
	if err != nil {
		writeErr(w, classifyCertificateErrStatus(err), err.Error())
		return
	}
	if err := pki.ValidateCertificateKeyPair([]byte(req.Certificate), []byte(req.PrivateKey)); err != nil {
		writeErr(w, classifyCertificateErrStatus(err), err.Error())
		return
	}
	writeJSON(w, 200, previewPlatformCertificateResponse{
		CommonName:       desc.CommonName,
		IssuerName:       desc.IssuerName,
		SANs:             desc.DNSNames,
		NotBefore:        &desc.NotBefore,
		NotAfter:         &desc.NotAfter,
		IsCA:             desc.IsCA,
		PrivateKeyType:   keyType,
		KeyPairValid:     true,
		ChainCertificate: pki.CountCertificatesPEM([]byte(req.Chain)),
	})
}

func (s *Server) importPlatformCertificate(w http.ResponseWriter, r *http.Request) {
	var req importPlatformCertificateRequest
	if !decode(r, &req) || strings.TrimSpace(req.Certificate) == "" || strings.TrimSpace(req.PrivateKey) == "" {
		writeErr(w, 400, "certificate file and private key file are required")
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

func (s *Server) setDefaultPlatformCertificate(w http.ResponseWriter, r *http.Request) {
	result, err := s.store.SetDefaultPlatformCertificate(r.Context(), strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		writeErr(w, classifyCertificateErrStatus(err), err.Error())
		return
	}
	writeJSON(w, 200, result)
}

func (s *Server) revokePlatformCertificate(w http.ResponseWriter, r *http.Request) {
	result, err := s.store.RevokePlatformCertificate(r.Context(), strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		writeErr(w, classifyCertificateErrStatus(err), err.Error())
		return
	}
	writeJSON(w, 200, result)
}

func (s *Server) deletePlatformCertificate(w http.ResponseWriter, r *http.Request) {
	result, err := s.store.DeletePlatformCertificateCascade(r.Context(), strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		writeErr(w, classifyCertificateErrStatus(err), err.Error())
		return
	}
	writeJSON(w, 200, result)
}

func classifyCertificateErrStatus(err error) int {
	switch {
	case err == nil:
		return 500
	case strings.Contains(err.Error(), "required"),
		strings.Contains(err.Error(), "invalid"),
		strings.Contains(err.Error(), "unsupported"),
		strings.Contains(err.Error(), "pem decode"),
		strings.Contains(err.Error(), "private key"),
		strings.Contains(err.Error(), "x509:"),
		strings.Contains(err.Error(), "malformed"),
		strings.Contains(err.Error(), "must not"),
		strings.Contains(err.Error(), "must include"),
		strings.Contains(err.Error(), "do not match"),
		strings.Contains(err.Error(), "not a CA"),
		strings.Contains(err.Error(), "only leaf"),
		strings.Contains(err.Error(), "only CA"),
		strings.Contains(err.Error(), "cannot be revoked"),
		strings.Contains(err.Error(), "not active"),
		strings.Contains(err.Error(), "expired"):
		return 400
	case strings.Contains(err.Error(), "already exists"):
		return 409
	default:
		return 500
	}
}
