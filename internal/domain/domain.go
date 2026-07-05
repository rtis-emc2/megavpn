package domain

import (
	"errors"
	"time"
)

var ErrServicePackNotFound = errors.New("service pack not found")
var ErrVLESSGroupTemplateNotFound = errors.New("vless group template not found")

type Node struct {
	ID                   string     `json:"id"`
	Name                 string     `json:"name"`
	Kind                 string     `json:"kind"`
	Role                 string     `json:"role"`
	Status               string     `json:"status"`
	Address              string     `json:"address"`
	LocationLabel        string     `json:"location_label,omitempty"`
	Latitude             *float64   `json:"latitude,omitempty"`
	Longitude            *float64   `json:"longitude,omitempty"`
	AccuracyRadiusKM     *float64   `json:"accuracy_radius_km,omitempty"`
	GeoIPProvider        string     `json:"geoip_provider,omitempty"`
	GeoIPStatus          string     `json:"geoip_status,omitempty"`
	GeoIPIP              string     `json:"geoip_ip,omitempty"`
	GeoIPCountryCode     string     `json:"geoip_country_code,omitempty"`
	GeoIPCountryName     string     `json:"geoip_country_name,omitempty"`
	GeoIPRegion          string     `json:"geoip_region,omitempty"`
	GeoIPCity            string     `json:"geoip_city,omitempty"`
	GeoIPOrg             string     `json:"geoip_org,omitempty"`
	GeoIPASN             string     `json:"geoip_asn,omitempty"`
	GeoIPResolvedAt      *time.Time `json:"geoip_resolved_at,omitempty"`
	GeoIPError           string     `json:"geoip_error,omitempty"`
	OSFamily             string     `json:"os_family"`
	OSVersion            string     `json:"os_version"`
	Architecture         string     `json:"architecture"`
	ExecutionMode        string     `json:"execution_mode"`
	AgentStatus          string     `json:"agent_status"`
	AgentVersion         string     `json:"agent_version"`
	AgentProtocolVersion string     `json:"agent_protocol_version"`
	AgentRegisteredAt    *time.Time `json:"agent_registered_at,omitempty"`
	AgentLastSeenAt      *time.Time `json:"agent_last_seen_at,omitempty"`
	LastHeartbeatAt      *time.Time `json:"last_heartbeat_at"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

type NodeGeoIP struct {
	Provider         string
	Status           string
	IP               string
	CountryCode      string
	CountryName      string
	Region           string
	City             string
	Org              string
	ASN              string
	LocationLabel    string
	Latitude         *float64
	Longitude        *float64
	AccuracyRadiusKM *float64
	ResolvedAt       *time.Time
	Error            string
}

type NodeAccessMethod struct {
	ID               string    `json:"id"`
	NodeID           string    `json:"node_id"`
	Method           string    `json:"method"`
	IsEnabled        bool      `json:"is_enabled"`
	SSHHost          string    `json:"ssh_host"`
	SSHPort          int       `json:"ssh_port"`
	SSHUser          string    `json:"ssh_user"`
	SSHHostKeySHA256 string    `json:"ssh_host_key_sha256"`
	AuthType         string    `json:"auth_type"`
	SecretRefID      *string   `json:"secret_ref_id,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type NodeBootstrapRun struct {
	ID             string         `json:"id"`
	NodeID         string         `json:"node_id"`
	JobID          *string        `json:"job_id,omitempty"`
	Status         string         `json:"status"`
	BootstrapMode  string         `json:"bootstrap_mode"`
	RequestPayload map[string]any `json:"request_payload"`
	ResultPayload  map[string]any `json:"result_payload,omitempty"`
	StartedAt      *time.Time     `json:"started_at,omitempty"`
	FinishedAt     *time.Time     `json:"finished_at,omitempty"`
	CreatedBy      *string        `json:"created_by,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
}

type NodeEnrollmentToken struct {
	ID        string     `json:"id"`
	NodeID    string     `json:"node_id"`
	Token     string     `json:"token,omitempty"`
	TokenHint string     `json:"token_hint"`
	Status    string     `json:"status"`
	ExpiresAt time.Time  `json:"expires_at"`
	UsedAt    *time.Time `json:"used_at"`
	CreatedAt time.Time  `json:"created_at"`
}

type NodeCapability struct {
	ID             string    `json:"id"`
	NodeID         string    `json:"node_id"`
	CapabilityCode string    `json:"capability_code"`
	Version        string    `json:"version"`
	Status         string    `json:"status"`
	Source         string    `json:"source"`
	DetectedAt     time.Time `json:"detected_at"`
}

type NodeCapabilityInstallEvent struct {
	ID             string         `json:"id"`
	NodeID         string         `json:"node_id"`
	JobID          *string        `json:"job_id,omitempty"`
	CapabilityCode string         `json:"capability_code"`
	Strategy       string         `json:"strategy"`
	Status         string         `json:"status"`
	Summary        string         `json:"summary"`
	Payload        map[string]any `json:"payload"`
	CreatedAt      time.Time      `json:"created_at"`
}

type NodeInventorySnapshot struct {
	ID        string         `json:"id"`
	NodeID    string         `json:"node_id"`
	Payload   map[string]any `json:"payload"`
	CreatedAt time.Time      `json:"created_at"`
}

type NodeServiceDiscovery struct {
	ID                string         `json:"id"`
	NodeID            string         `json:"node_id"`
	ServiceCode       string         `json:"service_code"`
	Name              string         `json:"name"`
	SystemdUnit       string         `json:"systemd_unit"`
	ConfigPath        string         `json:"config_path"`
	Status            string         `json:"status"`
	Source            string         `json:"source"`
	Confidence        int            `json:"confidence"`
	EndpointHost      string         `json:"endpoint_host"`
	EndpointPort      int            `json:"endpoint_port"`
	ManagedInstanceID *string        `json:"managed_instance_id,omitempty"`
	Payload           map[string]any `json:"payload"`
	DetectedAt        time.Time      `json:"detected_at"`
}

type NodeServiceDiscoverySummary struct {
	NodeID          string         `json:"node_id"`
	Total           int            `json:"total"`
	Available       int            `json:"available"`
	Discovered      int            `json:"discovered"`
	Imported        int            `json:"imported"`
	Ignored         int            `json:"ignored"`
	ByService       map[string]int `json:"by_service"`
	ImportableCount int            `json:"importable_count"`
}

type ServiceDefinition struct {
	ID                string    `json:"id"`
	Code              string    `json:"code"`
	Name              string    `json:"name"`
	Category          string    `json:"category"`
	Tier              string    `json:"tier"`
	SupportsAccounts  bool      `json:"supports_accounts"`
	SupportsArtifacts bool      `json:"supports_artifacts"`
	SupportsInstall   bool      `json:"supports_install"`
	SupportsInstances bool      `json:"supports_instances"`
	Enabled           bool      `json:"enabled"`
	CreatedAt         time.Time `json:"created_at"`
}

type ServicePackComponent struct {
	Label                string         `json:"label"`
	Description          string         `json:"description,omitempty"`
	ServiceCode          string         `json:"service_code"`
	PresetKey            string         `json:"preset_key"`
	NameSuffix           string         `json:"name_suffix"`
	SlugSuffix           string         `json:"slug_suffix"`
	EndpointPort         int            `json:"endpoint_port"`
	RequiresEndpointHost bool           `json:"requires_endpoint_host"`
	Spec                 map[string]any `json:"spec"`
}

type ServicePackDefinition struct {
	Key                  string                 `json:"key"`
	Label                string                 `json:"label"`
	Description          string                 `json:"description"`
	BaseNameTemplate     string                 `json:"base_name_template"`
	EndpointHint         string                 `json:"endpoint_hint"`
	RequiresEndpointHost bool                   `json:"requires_endpoint_host"`
	PlatformNotes        []string               `json:"platform_notes"`
	Recommendations      []string               `json:"recommendations"`
	Components           []ServicePackComponent `json:"components"`
	Status               string                 `json:"status,omitempty"`
	Source               string                 `json:"source,omitempty"`
	Version              int                    `json:"version,omitempty"`
	DisplayOrder         int                    `json:"display_order,omitempty"`
}

type VLESSGroupTemplate struct {
	Key              string           `json:"key"`
	Label            string           `json:"label"`
	Description      string           `json:"description"`
	AccessMode       string           `json:"access_mode"`
	EgressMode       string           `json:"egress_mode"`
	EgressNodeID     string           `json:"egress_node_id,omitempty"`
	TargetInstanceID string           `json:"target_instance_id,omitempty"`
	OutboundTag      string           `json:"outbound_tag"`
	AdBlock          bool             `json:"ad_block"`
	Rules            []map[string]any `json:"rules"`
	ExtraRules       []map[string]any `json:"extra_rules,omitempty"`
	Status           string           `json:"status,omitempty"`
	Source           string           `json:"source,omitempty"`
	Version          int              `json:"version,omitempty"`
	DisplayOrder     int              `json:"display_order,omitempty"`
}

type VLESSGroupCatalogSyncResult struct {
	ScannedInstances   int                             `json:"scanned_instances"`
	ChangedInstances   int                             `json:"changed_instances"`
	UnchangedInstances int                             `json:"unchanged_instances"`
	QueuedApplyJobs    int                             `json:"queued_apply_jobs"`
	SkippedApplyJobs   int                             `json:"skipped_apply_jobs"`
	FailedInstances    []VLESSGroupCatalogSyncFailure  `json:"failed_instances,omitempty"`
	Instances          []VLESSGroupCatalogSyncInstance `json:"instances,omitempty"`
}

type VLESSGroupCatalogSyncInstance struct {
	InstanceID   string `json:"instance_id"`
	NodeID       string `json:"node_id,omitempty"`
	Status       string `json:"status,omitempty"`
	Changed      bool   `json:"changed"`
	ApplyQueued  bool   `json:"apply_queued"`
	ApplyJobID   string `json:"apply_job_id,omitempty"`
	ApplyJobType string `json:"apply_job_type,omitempty"`
	RevisionID   string `json:"revision_id,omitempty"`
	Skipped      bool   `json:"skipped,omitempty"`
	Reason       string `json:"reason,omitempty"`
}

type VLESSGroupCatalogSyncFailure struct {
	InstanceID string `json:"instance_id"`
	NodeID     string `json:"node_id,omitempty"`
	Stage      string `json:"stage"`
	Error      string `json:"error"`
}

type Instance struct {
	ID                    string         `json:"id"`
	NodeID                string         `json:"node_id"`
	ServiceCode           string         `json:"service_code"`
	Name                  string         `json:"name"`
	Slug                  string         `json:"slug"`
	SystemdUnit           string         `json:"systemd_unit"`
	Status                string         `json:"status"`
	Enabled               bool           `json:"enabled"`
	EndpointHost          string         `json:"endpoint_host"`
	EndpointPort          int            `json:"endpoint_port"`
	CurrentRevisionID     *string        `json:"current_revision_id,omitempty"`
	LastAppliedRevisionID *string        `json:"last_applied_revision_id,omitempty"`
	Spec                  map[string]any `json:"spec,omitempty"`
	CreatedAt             time.Time      `json:"created_at"`
	UpdatedAt             time.Time      `json:"updated_at"`
}

type InstanceRevision struct {
	ID               string         `json:"id"`
	InstanceID       string         `json:"instance_id"`
	Source           string         `json:"source"`
	Status           string         `json:"status"`
	RenderedHash     string         `json:"rendered_hash"`
	RevisionNo       int            `json:"revision_no"`
	Spec             map[string]any `json:"spec"`
	ValidationErrors []any          `json:"validation_errors"`
	CreatedAt        time.Time      `json:"created_at"`
	AppliedAt        *time.Time     `json:"applied_at"`
	IsCurrent        bool           `json:"is_current"`
	IsLastApplied    bool           `json:"is_last_applied"`
}

type InstanceRuntimeState struct {
	ID                 string           `json:"id"`
	InstanceID         string           `json:"instance_id"`
	NodeID             *string          `json:"node_id,omitempty"`
	ServiceCode        string           `json:"service_code"`
	SystemdUnit        string           `json:"systemd_unit"`
	DesiredStatus      string           `json:"desired_status"`
	RuntimeStatus      string           `json:"runtime_status"`
	HealthStatus       string           `json:"health_status"`
	DriftStatus        string           `json:"drift_status"`
	HealthChecks       []RuntimeCheck   `json:"health_checks"`
	HealthReasons      []string         `json:"health_reasons"`
	DriftReasons       []string         `json:"drift_reasons"`
	ActiveState        string           `json:"active_state"`
	EnabledState       string           `json:"enabled_state"`
	ConfigHash         string           `json:"config_hash"`
	LastJobID          *string          `json:"last_job_id,omitempty"`
	LastJobType        string           `json:"last_job_type"`
	LastJobStatus      string           `json:"last_job_status"`
	AppliedRevisionID  *string          `json:"applied_revision_id,omitempty"`
	ObservedRevisionID *string          `json:"observed_revision_id,omitempty"`
	EndpointHost       string           `json:"endpoint_host"`
	EndpointPort       int              `json:"endpoint_port"`
	ListeningPorts     []map[string]any `json:"listening_ports"`
	Result             map[string]any   `json:"result"`
	ErrorText          string           `json:"error_text"`
	AgentReportedAt    *time.Time       `json:"agent_reported_at,omitempty"`
	CheckedAt          time.Time        `json:"checked_at"`
	UpdatedAt          time.Time        `json:"updated_at"`
}

type RuntimeCheck struct {
	Code        string `json:"code"`
	DisplayName string `json:"display_name"`
	Signal      string `json:"signal"`
	Source      string `json:"source"`
	Required    bool   `json:"required"`
	Status      string `json:"status"`
	Message     string `json:"message"`
	Expected    any    `json:"expected,omitempty"`
	Observed    any    `json:"observed,omitempty"`
}

type InstanceRuntimeObservation struct {
	ID                 string           `json:"id"`
	InstanceID         string           `json:"instance_id"`
	NodeID             *string          `json:"node_id,omitempty"`
	Source             string           `json:"source"`
	ServiceCode        string           `json:"service_code"`
	SystemdUnit        string           `json:"systemd_unit"`
	DesiredStatus      string           `json:"desired_status"`
	RuntimeStatus      string           `json:"runtime_status"`
	HealthStatus       string           `json:"health_status"`
	DriftStatus        string           `json:"drift_status"`
	HealthChecks       []RuntimeCheck   `json:"health_checks"`
	HealthReasons      []string         `json:"health_reasons"`
	DriftReasons       []string         `json:"drift_reasons"`
	ActiveState        string           `json:"active_state"`
	EnabledState       string           `json:"enabled_state"`
	ConfigHash         string           `json:"config_hash"`
	LastJobID          *string          `json:"last_job_id,omitempty"`
	LastJobType        string           `json:"last_job_type"`
	LastJobStatus      string           `json:"last_job_status"`
	AppliedRevisionID  *string          `json:"applied_revision_id,omitempty"`
	ObservedRevisionID *string          `json:"observed_revision_id,omitempty"`
	EndpointHost       string           `json:"endpoint_host"`
	EndpointPort       int              `json:"endpoint_port"`
	ListeningPorts     []map[string]any `json:"listening_ports"`
	Result             map[string]any   `json:"result"`
	ErrorText          string           `json:"error_text"`
	ObservedAt         time.Time        `json:"observed_at"`
	ReceivedAt         time.Time        `json:"received_at"`
}

type AgentInstanceRuntimeTarget struct {
	InstanceID        string  `json:"instance_id"`
	NodeID            string  `json:"node_id"`
	ServiceCode       string  `json:"service_code"`
	Slug              string  `json:"slug"`
	SystemdUnit       string  `json:"systemd_unit"`
	ConfigPath        string  `json:"config_path"`
	EndpointHost      string  `json:"endpoint_host"`
	EndpointPort      int     `json:"endpoint_port"`
	DesiredStatus     string  `json:"desired_status"`
	DesiredEnabled    bool    `json:"desired_enabled"`
	CurrentRevisionID *string `json:"current_revision_id,omitempty"`
	AppliedRevisionID *string `json:"applied_revision_id,omitempty"`
}

type AgentInstanceRuntimeReport struct {
	InstanceID         string           `json:"instance_id"`
	ServiceCode        string           `json:"service_code"`
	SystemdUnit        string           `json:"systemd_unit"`
	ConfigPath         string           `json:"config_path"`
	ConfigHash         string           `json:"config_hash"`
	ActiveState        string           `json:"active_state"`
	EnabledState       string           `json:"enabled_state"`
	ObservedRevisionID *string          `json:"observed_revision_id,omitempty"`
	ListeningPorts     []map[string]any `json:"listening_ports"`
	ErrorText          string           `json:"error_text"`
	CheckedAt          *time.Time       `json:"checked_at,omitempty"`
}

type Client struct {
	ID          string         `json:"id"`
	Username    string         `json:"username"`
	DisplayName string         `json:"display_name"`
	Email       string         `json:"email"`
	Status      string         `json:"status"`
	Notes       string         `json:"notes"`
	ExpiresAt   *time.Time     `json:"expires_at"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	Summary     *ClientSummary `json:"summary,omitempty"`
}

type ClientSummary struct {
	ServiceAccessCount        int        `json:"service_access_count"`
	ActiveServiceAccessCount  int        `json:"active_service_access_count"`
	PendingServiceAccessCount int        `json:"pending_service_access_count"`
	RouteCount                int        `json:"route_count"`
	ActiveRouteCount          int        `json:"active_route_count"`
	ArtifactCount             int        `json:"artifact_count"`
	ReadyArtifactCount        int        `json:"ready_artifact_count"`
	ShareLinkCount            int        `json:"share_link_count"`
	ActiveShareLinkCount      int        `json:"active_share_link_count"`
	LastArtifactAt            *time.Time `json:"last_artifact_at,omitempty"`
	NextShareLinkExpiresAt    *time.Time `json:"next_share_link_expires_at,omitempty"`
}

type ServiceAccess struct {
	ID              string         `json:"id"`
	ClientAccountID string         `json:"client_account_id"`
	InstanceID      string         `json:"instance_id"`
	Status          string         `json:"status"`
	ProvisionMode   string         `json:"provision_mode"`
	Policy          map[string]any `json:"policy"`
	Metadata        map[string]any `json:"metadata"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

type ClientAccessRoute struct {
	ID              string         `json:"id"`
	ClientAccountID string         `json:"client_account_id"`
	ServiceAccessID *string        `json:"service_access_id,omitempty"`
	InstanceID      *string        `json:"instance_id,omitempty"`
	NodeID          *string        `json:"node_id,omitempty"`
	Name            string         `json:"name"`
	Status          string         `json:"status"`
	Action          string         `json:"action"`
	DestinationType string         `json:"destination_type"`
	Destination     string         `json:"destination"`
	Protocol        string         `json:"protocol"`
	Ports           string         `json:"ports"`
	Description     string         `json:"description"`
	Policy          map[string]any `json:"policy"`
	Metadata        map[string]any `json:"metadata"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

type BackhaulLink struct {
	ID                  string              `json:"id"`
	Name                string              `json:"name"`
	IngressNodeID       string              `json:"ingress_node_id"`
	EgressNodeID        string              `json:"egress_node_id"`
	Status              string              `json:"status"`
	SelectedTransportID *string             `json:"selected_transport_id,omitempty"`
	DesiredDriver       string              `json:"desired_driver"`
	RoutingTable        string              `json:"routing_table"`
	RouteMetric         int                 `json:"route_metric"`
	FailoverPolicy      map[string]any      `json:"failover_policy"`
	Metadata            map[string]any      `json:"metadata"`
	Transports          []BackhaulTransport `json:"transports,omitempty"`
	CreatedAt           time.Time           `json:"created_at"`
	UpdatedAt           time.Time           `json:"updated_at"`
}

type BackhaulTransport struct {
	ID               string         `json:"id"`
	LinkID           string         `json:"link_id"`
	Driver           string         `json:"driver"`
	Priority         int            `json:"priority"`
	Status           string         `json:"status"`
	EndpointHost     string         `json:"endpoint_host"`
	EndpointPort     int            `json:"endpoint_port"`
	Protocol         string         `json:"protocol"`
	InterfaceName    string         `json:"interface_name"`
	TunnelCIDR       string         `json:"tunnel_cidr"`
	IngressAddress   string         `json:"ingress_address"`
	EgressAddress    string         `json:"egress_address"`
	Config           map[string]any `json:"config"`
	SecretRefs       map[string]any `json:"secret_refs"`
	Health           map[string]any `json:"health"`
	AppliedIngressAt *time.Time     `json:"applied_ingress_at,omitempty"`
	AppliedEgressAt  *time.Time     `json:"applied_egress_at,omitempty"`
	LastError        string         `json:"last_error"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

type Artifact struct {
	ID              string    `json:"id"`
	ClientAccountID string    `json:"client_account_id"`
	ArtifactType    string    `json:"artifact_type"`
	StoragePath     string    `json:"storage_path"`
	Status          string    `json:"status"`
	ServiceAccessID *string   `json:"service_access_id"`
	ContentHash     string    `json:"content_hash"`
	SizeBytes       int64     `json:"size_bytes"`
	CreatedAt       time.Time `json:"created_at"`
}

type BinaryArtifact struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	Kind         string         `json:"kind"`
	ServiceCode  string         `json:"service_code"`
	Version      string         `json:"version"`
	OSFamily     string         `json:"os_family"`
	OSVersion    string         `json:"os_version"`
	Architecture string         `json:"architecture"`
	StoragePath  string         `json:"storage_path"`
	SizeBytes    int64          `json:"size_bytes"`
	SHA256       string         `json:"sha256"`
	Signature    string         `json:"signature,omitempty"`
	Status       string         `json:"status"`
	Metadata     map[string]any `json:"metadata"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

type BinaryManifest struct {
	ID         string         `json:"id"`
	ArtifactID string         `json:"artifact_id"`
	Channel    string         `json:"channel"`
	Manifest   map[string]any `json:"manifest"`
	Status     string         `json:"status"`
	CreatedAt  time.Time      `json:"created_at"`
}

type BinaryDownloadTicket struct {
	ID         string     `json:"id"`
	ArtifactID string     `json:"artifact_id"`
	NodeID     *string    `json:"node_id,omitempty"`
	JobID      *string    `json:"job_id,omitempty"`
	Token      string     `json:"token,omitempty"`
	TokenHint  string     `json:"token_hint"`
	Status     string     `json:"status"`
	ExpiresAt  time.Time  `json:"expires_at"`
	UsedAt     *time.Time `json:"used_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

type ShareLink struct {
	ID              string    `json:"id"`
	ClientAccountID string    `json:"client_account_id"`
	TargetType      string    `json:"target_type"`
	TargetID        string    `json:"target_id"`
	Token           string    `json:"token,omitempty"`
	TokenHint       string    `json:"token_hint"`
	Status          string    `json:"status"`
	ExpiresAt       time.Time `json:"expires_at"`
	DownloadCount   int64     `json:"download_count"`
	CreatedAt       time.Time `json:"created_at"`
}

type Job struct {
	ID          string         `json:"id"`
	Type        string         `json:"type"`
	ScopeType   string         `json:"scope_type"`
	Status      string         `json:"status"`
	ScopeID     *string        `json:"scope_id"`
	NodeID      *string        `json:"node_id"`
	InstanceID  *string        `json:"instance_id"`
	Priority    int            `json:"priority"`
	Payload     map[string]any `json:"payload"`
	Result      map[string]any `json:"result"`
	LockedBy    *string        `json:"locked_by,omitempty"`
	LockedUntil *time.Time     `json:"locked_until,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	StartedAt   *time.Time     `json:"started_at"`
	FinishedAt  *time.Time     `json:"finished_at"`
}

type JobLog struct {
	ID        string         `json:"id"`
	JobID     string         `json:"job_id"`
	Level     string         `json:"level"`
	Message   string         `json:"message"`
	Payload   map[string]any `json:"payload"`
	CreatedAt time.Time      `json:"created_at"`
}

type AuditEvent struct {
	ID           string    `json:"id"`
	ActorUserID  *string   `json:"actor_user_id,omitempty"`
	ActorType    string    `json:"actor_type"`
	Action       string    `json:"action"`
	ResourceType string    `json:"resource_type"`
	Summary      string    `json:"summary"`
	ResourceID   *string   `json:"resource_id"`
	CreatedAt    time.Time `json:"created_at"`
}

type Dashboard struct {
	NodesTotal      int    `json:"nodes_total"`
	NodesOnline     int    `json:"nodes_online"`
	InstancesTotal  int    `json:"instances_total"`
	InstancesActive int    `json:"instances_active"`
	ClientsTotal    int    `json:"clients_total"`
	ClientsActive   int    `json:"clients_active"`
	JobsQueued      int    `json:"jobs_queued"`
	JobsRunning     int    `json:"jobs_running"`
	JobsFailed      int    `json:"jobs_failed"`
	AuditTotal      int    `json:"audit_total"`
	Version         string `json:"version"`
}
