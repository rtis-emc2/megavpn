package domain

import "time"

type PlatformServicePKIRoot struct {
	ID                string     `json:"id"`
	ServiceCode       string     `json:"service_code"`
	PKIProfile        string     `json:"pki_profile"`
	Status            string     `json:"status"`
	CACertSecretRefID string     `json:"ca_cert_secret_ref_id"`
	CAKeySecretRefID  string     `json:"ca_key_secret_ref_id"`
	CommonName        string     `json:"common_name"`
	CreatedAt         time.Time  `json:"created_at"`
	RotatedAt         *time.Time `json:"rotated_at,omitempty"`
}
