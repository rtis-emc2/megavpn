package domain

import "time"

type Node struct {
	ID                   string     `json:"id"`
	Name                 string     `json:"name"`
	Kind                 string     `json:"kind"`
	Role                 string     `json:"role"`
	Status               string     `json:"status"`
	Address              string     `json:"address"`
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

type NodeAccessMethod struct {
	ID          string    `json:"id"`
	NodeID      string    `json:"node_id"`
	Method      string    `json:"method"`
	IsEnabled   bool      `json:"is_enabled"`
	SSHHost     string    `json:"ssh_host"`
	SSHPort     int       `json:"ssh_port"`
	SSHUser     string    `json:"ssh_user"`
	AuthType    string    `json:"auth_type"`
	SecretRefID *string   `json:"secret_ref_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
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
	ID          string     `json:"id"`
	Username    string     `json:"username"`
	DisplayName string     `json:"display_name"`
	Email       string     `json:"email"`
	Status      string     `json:"status"`
	Notes       string     `json:"notes"`
	ExpiresAt   *time.Time `json:"expires_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
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

type ShareLink struct {
	ID              string    `json:"id"`
	ClientAccountID string    `json:"client_account_id"`
	TargetType      string    `json:"target_type"`
	TargetID        string    `json:"target_id"`
	Token           string    `json:"token"`
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
