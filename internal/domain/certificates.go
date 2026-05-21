package domain

import "time"

type PlatformCertificate struct {
	ID                  string         `json:"id"`
	Name                string         `json:"name"`
	Description         string         `json:"description"`
	Source              string         `json:"source"`
	Kind                string         `json:"kind"`
	Status              string         `json:"status"`
	CommonName          string         `json:"common_name"`
	SANs                []string       `json:"sans"`
	IssuerName          string         `json:"issuer_name"`
	ParentCertificateID *string        `json:"parent_certificate_id,omitempty"`
	CertSecretRefID     string         `json:"cert_secret_ref_id"`
	KeySecretRefID      *string        `json:"key_secret_ref_id,omitempty"`
	ChainSecretRefID    *string        `json:"chain_secret_ref_id,omitempty"`
	NotBefore           *time.Time     `json:"not_before,omitempty"`
	NotAfter            *time.Time     `json:"not_after,omitempty"`
	IsDefault           bool           `json:"is_default"`
	Meta                map[string]any `json:"meta,omitempty"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
}
