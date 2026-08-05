package domain

import "time"

type SecretRef struct {
	ID         string         `json:"id"`
	SecretType string         `json:"secret_type"`
	KeyVersion string         `json:"key_version"`
	Meta       map[string]any `json:"meta"`
	CreatedAt  time.Time      `json:"created_at"`
	RotatedAt  *time.Time     `json:"rotated_at,omitempty"`
}
