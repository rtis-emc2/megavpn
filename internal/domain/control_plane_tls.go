package domain

import "time"

type ControlPlaneTLSSettings struct {
	Enabled              bool       `json:"enabled"`
	Mode                 string     `json:"mode"`
	PublicBaseURL        string     `json:"public_base_url"`
	ServerName           string     `json:"server_name"`
	ListenPort           int        `json:"listen_port"`
	UpstreamURL          string     `json:"upstream_url"`
	CertificateID        *string    `json:"certificate_id,omitempty"`
	SelfSignedCommonName string     `json:"self_signed_common_name"`
	SelfSignedDNSNames   []string   `json:"self_signed_dns_names"`
	LastAppliedAt        *time.Time `json:"last_applied_at,omitempty"`
	LastError            string     `json:"last_error"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}
