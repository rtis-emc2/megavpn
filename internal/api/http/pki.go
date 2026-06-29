package http

import (
	"net/http"
	"time"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

type platformServicePKIRootResponse struct {
	ID                string     `json:"id"`
	ServiceCode       string     `json:"service_code"`
	PKIProfile        string     `json:"pki_profile"`
	Status            string     `json:"status"`
	CACertSecretRefID string     `json:"ca_cert_secret_ref_id"`
	CommonName        string     `json:"common_name"`
	NotBefore         *time.Time `json:"not_before,omitempty"`
	NotAfter          *time.Time `json:"not_after,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	RotatedAt         *time.Time `json:"rotated_at,omitempty"`
}

func (s *Server) listPlatformServicePKIRoots(w http.ResponseWriter, r *http.Request) {
	roots, err := s.store.ListPlatformServicePKIRoots(r.Context())
	if err != nil {
		writeErr(w, 500, "list platform pki roots failed")
		return
	}
	resp := make([]platformServicePKIRootResponse, 0, len(roots))
	for _, root := range roots {
		resp = append(resp, platformServicePKIRootToResponse(root))
	}
	writeJSON(w, 200, resp)
}

func platformServicePKIRootToResponse(root domain.PlatformServicePKIRoot) platformServicePKIRootResponse {
	return platformServicePKIRootResponse{
		ID:                root.ID,
		ServiceCode:       root.ServiceCode,
		PKIProfile:        root.PKIProfile,
		Status:            root.Status,
		CACertSecretRefID: root.CACertSecretRefID,
		CommonName:        root.CommonName,
		NotBefore:         root.NotBefore,
		NotAfter:          root.NotAfter,
		CreatedAt:         root.CreatedAt,
		RotatedAt:         root.RotatedAt,
	}
}
