package domain

import "time"

const TrafficAccountingRetentionDays = 180

type AgentTrafficAccountingSample struct {
	SampleKey       string         `json:"sample_key,omitempty"`
	InstanceID      string         `json:"instance_id,omitempty"`
	ServiceAccessID string         `json:"service_access_id,omitempty"`
	ClientAccountID string         `json:"client_account_id,omitempty"`
	Source          string         `json:"source,omitempty"`
	Protocol        string         `json:"protocol,omitempty"`
	Direction       string         `json:"direction,omitempty"`
	BucketStart     *time.Time     `json:"bucket_start,omitempty"`
	BucketEnd       *time.Time     `json:"bucket_end,omitempty"`
	RxBytes         int64          `json:"rx_bytes"`
	TxBytes         int64          `json:"tx_bytes"`
	RxPackets       int64          `json:"rx_packets,omitempty"`
	TxPackets       int64          `json:"tx_packets,omitempty"`
	FlowCount       int64          `json:"flow_count,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
	ObservedAt      *time.Time     `json:"observed_at,omitempty"`
}

type TrafficAccountingSample struct {
	ID              string         `json:"id"`
	NodeID          string         `json:"node_id"`
	NodeName        string         `json:"node_name"`
	InstanceID      string         `json:"instance_id,omitempty"`
	InstanceName    string         `json:"instance_name,omitempty"`
	ServiceAccessID string         `json:"service_access_id,omitempty"`
	ClientAccountID string         `json:"client_account_id,omitempty"`
	ClientUsername  string         `json:"client_username,omitempty"`
	Source          string         `json:"source"`
	Protocol        string         `json:"protocol"`
	Direction       string         `json:"direction"`
	BucketStart     time.Time      `json:"bucket_start"`
	BucketEnd       time.Time      `json:"bucket_end"`
	RxBytes         int64          `json:"rx_bytes"`
	TxBytes         int64          `json:"tx_bytes"`
	RxPackets       int64          `json:"rx_packets"`
	TxPackets       int64          `json:"tx_packets"`
	FlowCount       int64          `json:"flow_count"`
	Metadata        map[string]any `json:"metadata"`
	ObservedAt      time.Time      `json:"observed_at"`
	ReceivedAt      time.Time      `json:"received_at"`
}

type TrafficAccountingSummary struct {
	RetentionDays         int        `json:"retention_days"`
	RetentionCutoff       *time.Time `json:"retention_cutoff,omitempty"`
	ExpiredSampleCount    int64      `json:"expired_sample_count"`
	PruneBatchSize        int        `json:"prune_batch_size"`
	PruneBatchesPerIngest int        `json:"prune_batches_per_ingest"`
	MaxPrunePerIngest     int        `json:"max_prune_per_ingest"`
	SampleCount           int64      `json:"sample_count"`
	ClientCount           int64      `json:"client_count"`
	NodeCount             int64      `json:"node_count"`
	RxBytes               int64      `json:"rx_bytes"`
	TxBytes               int64      `json:"tx_bytes"`
	FlowCount             int64      `json:"flow_count"`
	OldestBucketStart     *time.Time `json:"oldest_bucket_start,omitempty"`
	NewestBucketEnd       *time.Time `json:"newest_bucket_end,omitempty"`
}

type TrafficAccountingCollectorStatus struct {
	NodeID                 string    `json:"node_id"`
	NodeName               string    `json:"node_name"`
	Source                 string    `json:"source"`
	Protocol               string    `json:"protocol"`
	Status                 string    `json:"status"`
	SampleCount            int64     `json:"sample_count"`
	ClientCount            int64     `json:"client_count"`
	RxBytes                int64     `json:"rx_bytes"`
	TxBytes                int64     `json:"tx_bytes"`
	FlowCount              int64     `json:"flow_count"`
	LastBucketEnd          time.Time `json:"last_bucket_end"`
	LastReceivedAt         time.Time `json:"last_received_at"`
	LastReceivedAgeSeconds int64     `json:"last_received_age_seconds"`
}

type TrafficAccountingOverview struct {
	Summary    TrafficAccountingSummary           `json:"summary"`
	Samples    []TrafficAccountingSample          `json:"samples"`
	Collectors []TrafficAccountingCollectorStatus `json:"collectors"`
}

type TrafficAccountingExportFilter struct {
	Limit           int
	From            *time.Time
	To              *time.Time
	ClientAccountID string
	NodeID          string
	Protocol        string
}

type TrafficAccountingIngestResult struct {
	Status        string   `json:"status"`
	Accepted      int      `json:"accepted"`
	Inserted      int      `json:"inserted"`
	Rejected      int      `json:"rejected"`
	Pruned        int64    `json:"pruned"`
	RetentionDays int      `json:"retention_days"`
	Errors        []string `json:"errors,omitempty"`
}
