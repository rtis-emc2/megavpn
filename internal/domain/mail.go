package domain

import "time"

type PlatformMailSettings struct {
	Enabled                 bool       `json:"enabled"`
	Provider                string     `json:"provider"`
	SMTPHost                string     `json:"smtp_host"`
	SMTPPort                int        `json:"smtp_port"`
	SMTPUsername            string     `json:"smtp_username"`
	SMTPPasswordSecretRefID *string    `json:"smtp_password_secret_ref_id,omitempty"`
	SMTPPasswordConfigured  bool       `json:"smtp_password_configured"`
	SMTPAuthMode            string     `json:"smtp_auth_mode"`
	SMTPTLSMode             string     `json:"smtp_tls_mode"`
	FromEmail               string     `json:"from_email"`
	FromName                string     `json:"from_name"`
	ReplyToEmail            string     `json:"reply_to_email"`
	InviteURLBase           string     `json:"invite_url_base"`
	LastTestAt              *time.Time `json:"last_test_at,omitempty"`
	LastError               string     `json:"last_error"`
	CreatedAt               time.Time  `json:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at"`
}

type PlatformUserInvite struct {
	ID            string     `json:"id"`
	UserID        string     `json:"user_id"`
	Username      string     `json:"username"`
	Email         string     `json:"email"`
	DisplayName   string     `json:"display_name"`
	Token         string     `json:"token,omitempty"`
	TokenHint     string     `json:"token_hint"`
	Status        string     `json:"status"`
	ExpiresAt     time.Time  `json:"expires_at"`
	SentAt        *time.Time `json:"sent_at,omitempty"`
	AcceptedAt    *time.Time `json:"accepted_at,omitempty"`
	DeliveryError string     `json:"delivery_error"`
	CreatedBy     *string    `json:"created_by,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

type ClientEmailDelivery struct {
	ID              string         `json:"id"`
	ClientAccountID string         `json:"client_account_id"`
	Email           string         `json:"email"`
	Subject         string         `json:"subject"`
	Status          string         `json:"status"`
	ArtifactIDs     []string       `json:"artifact_ids"`
	ShareLinkIDs    []string       `json:"share_link_ids"`
	Payload         map[string]any `json:"payload"`
	ErrorText       string         `json:"error_text"`
	CreatedBy       *string        `json:"created_by,omitempty"`
	SentAt          *time.Time     `json:"sent_at,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
}

type ClientDeliveryHistoryItem struct {
	ID                  string     `json:"id"`
	ClientAccountID     string     `json:"client_account_id"`
	DeliveryType        string     `json:"delivery_type"`
	Channel             string     `json:"channel"`
	DestinationHint     string     `json:"destination_hint"`
	Status              string     `json:"status"`
	ArtifactCount       int        `json:"artifact_count"`
	ShareLinkCount      int        `json:"share_link_count"`
	SafeErrorSummary    string     `json:"safe_error_summary,omitempty"`
	RelatedArtifactIDs  []string   `json:"related_artifact_ids,omitempty"`
	RelatedShareLinkIDs []string   `json:"related_share_link_ids,omitempty"`
	CreatedBy           *string    `json:"created_by,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	SentAt              *time.Time `json:"sent_at,omitempty"`
	CompletedAt         *time.Time `json:"completed_at,omitempty"`
	FailedAt            *time.Time `json:"failed_at,omitempty"`
}
