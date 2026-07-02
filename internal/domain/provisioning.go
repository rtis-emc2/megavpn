package domain

import "time"

type ProvisioningAccess struct {
	Access   ServiceAccess `json:"access"`
	Client   Client        `json:"client"`
	Instance Instance      `json:"instance"`
}

type ClientSubscription struct {
	ID              string     `json:"id"`
	ClientAccountID string     `json:"client_account_id"`
	Token           string     `json:"token,omitempty"`
	TokenHint       string     `json:"token_hint"`
	Status          string     `json:"status"`
	ExpiresAt       time.Time  `json:"expires_at"`
	DownloadCount   int64      `json:"download_count"`
	LastUsedAt      *time.Time `json:"last_used_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type ClientSubscriptionProfile struct {
	ServiceAccessID string `json:"service_access_id"`
	InstanceID      string `json:"instance_id"`
	InstanceName    string `json:"instance_name"`
	InstanceSlug    string `json:"instance_slug"`
	NodeID          string `json:"node_id"`
	ServiceCode     string `json:"service_code"`
	EndpointHost    string `json:"endpoint_host"`
	EndpointPort    int    `json:"endpoint_port"`
	OutboundGroup   string `json:"outbound_group"`
	URI             string `json:"uri"`
}

type ClientSubscriptionDocument struct {
	Subscription ClientSubscription          `json:"subscription"`
	Client       Client                      `json:"client"`
	Profiles     []ClientSubscriptionProfile `json:"profiles"`
	GeneratedAt  time.Time                   `json:"generated_at"`
}
