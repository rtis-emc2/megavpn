package postgres

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/jobschema"
	"github.com/rtis-emc2/megavpn/internal/platform/id"
	secretstore "github.com/rtis-emc2/megavpn/internal/secrets"
	"github.com/rtis-emc2/megavpn/internal/service/driver"
)

type Store struct {
	db           *pgxpool.Pool
	secretSvc    *secretstore.Service
	artifactRoot string
}

const (
	jobLeaseDuration              = 2 * time.Minute
	longNodeJobLeaseDuration      = 15 * time.Minute
	legacyRunningJobRecoveryAfter = 10 * time.Minute
)

func New(pool *pgxpool.Pool) *Store {
	return &Store{db: pool, artifactRoot: defaultArtifactRoot}
}

func (s *Store) SetSecretService(secretSvc *secretstore.Service) {
	if s == nil {
		return
	}
	s.secretSvc = secretSvc
}

func (s *Store) SetArtifactRoot(root string) {
	if s == nil {
		return
	}
	root = strings.TrimSpace(root)
	if root == "" {
		return
	}
	s.artifactRoot = filepath.Clean(root)
}

func (s *Store) ArtifactRoot() string {
	if s == nil || strings.TrimSpace(s.artifactRoot) == "" {
		return defaultArtifactRoot
	}
	return s.artifactRoot
}

func (s *Store) Ping(ctx context.Context) error { return s.db.Ping(ctx) }

func (s *Store) SchemaMigrationStatus(ctx context.Context) (string, int, error) {
	var latest string
	var applied int64
	err := s.db.QueryRow(ctx, `select coalesce(max(version), ''), count(*) from schema_migrations`).Scan(&latest, &applied)
	return latest, int(applied), err
}

func (s *Store) Dashboard(ctx context.Context, version string) (domain.Dashboard, error) {
	var d domain.Dashboard
	d.Version = version
	row := s.db.QueryRow(ctx, `select
		(select count(*) from nodes where status <> 'retired'),
		(select count(*) from nodes where status='online'),
		(select count(*) from instances where status <> 'deleted'),
		(select count(*) from instances where status='active'),
		(select count(*) from client_accounts where status <> 'deleted'),
		(select count(*) from client_accounts where status='active'),
		(select count(*) from jobs where status='queued'),
		(select count(*) from jobs where status='running'),
		(select count(*) from jobs where status='failed'),
		(select count(*) from audit_events)`)
	err := row.Scan(&d.NodesTotal, &d.NodesOnline, &d.InstancesTotal, &d.InstancesActive, &d.ClientsTotal, &d.ClientsActive, &d.JobsQueued, &d.JobsRunning, &d.JobsFailed, &d.AuditTotal)
	return d, err
}

func (s *Store) ListNodes(ctx context.Context) ([]domain.Node, error) {
	rows, err := s.db.Query(ctx, `select
		n.id,n.name,n.kind,n.role,n.status,n.address,coalesce(n.location_label,''),n.latitude,n.longitude,n.accuracy_radius_km,
		coalesce(n.geoip_provider,''),coalesce(n.geoip_status,''),coalesce(n.geoip_ip,''),coalesce(n.geoip_country_code,''),coalesce(n.geoip_country_name,''),coalesce(n.geoip_region,''),coalesce(n.geoip_city,''),coalesce(n.geoip_org,''),coalesce(n.geoip_asn,''),n.geoip_resolved_at,coalesce(n.geoip_error,''),
		n.os_family,n.os_version,n.architecture,n.execution_mode,n.agent_status,n.last_heartbeat_at,n.created_at,n.updated_at,
		coalesce(na.agent_version,''),coalesce(na.protocol_version,''),na.registered_at,na.last_seen_at
	from nodes n
	left join node_agents na on na.node_id=n.id
	where n.status <> 'retired'
	order by n.created_at desc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Node{}
	for rows.Next() {
		n, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) GetNode(ctx context.Context, nodeID string) (domain.Node, error) {
	row := s.db.QueryRow(ctx, `select
		n.id,n.name,n.kind,n.role,n.status,n.address,coalesce(n.location_label,''),n.latitude,n.longitude,n.accuracy_radius_km,
		coalesce(n.geoip_provider,''),coalesce(n.geoip_status,''),coalesce(n.geoip_ip,''),coalesce(n.geoip_country_code,''),coalesce(n.geoip_country_name,''),coalesce(n.geoip_region,''),coalesce(n.geoip_city,''),coalesce(n.geoip_org,''),coalesce(n.geoip_asn,''),n.geoip_resolved_at,coalesce(n.geoip_error,''),
		n.os_family,n.os_version,n.architecture,n.execution_mode,n.agent_status,n.last_heartbeat_at,n.created_at,n.updated_at,
		coalesce(na.agent_version,''),coalesce(na.protocol_version,''),na.registered_at,na.last_seen_at
	from nodes n
	left join node_agents na on na.node_id=n.id
	where n.id=$1`, nodeID)
	return scanNode(row)
}

func scanNode(row jobScanner) (domain.Node, error) {
	var n domain.Node
	if err := row.Scan(
		&n.ID,
		&n.Name,
		&n.Kind,
		&n.Role,
		&n.Status,
		&n.Address,
		&n.LocationLabel,
		&n.Latitude,
		&n.Longitude,
		&n.AccuracyRadiusKM,
		&n.GeoIPProvider,
		&n.GeoIPStatus,
		&n.GeoIPIP,
		&n.GeoIPCountryCode,
		&n.GeoIPCountryName,
		&n.GeoIPRegion,
		&n.GeoIPCity,
		&n.GeoIPOrg,
		&n.GeoIPASN,
		&n.GeoIPResolvedAt,
		&n.GeoIPError,
		&n.OSFamily,
		&n.OSVersion,
		&n.Architecture,
		&n.ExecutionMode,
		&n.AgentStatus,
		&n.LastHeartbeatAt,
		&n.CreatedAt,
		&n.UpdatedAt,
		&n.AgentVersion,
		&n.AgentProtocolVersion,
		&n.AgentRegisteredAt,
		&n.AgentLastSeenAt,
	); err != nil {
		return domain.Node{}, err
	}
	return n, nil
}

func (s *Store) CreateNode(ctx context.Context, n domain.Node) (domain.Node, error) {
	n.Name = strings.TrimSpace(n.Name)
	n.Address = strings.TrimSpace(n.Address)
	if err := domain.ValidateRequiredSingleLine("node name", n.Name); err != nil {
		return n, err
	}
	if err := domain.ValidateRequiredSingleLine("node address", n.Address); err != nil {
		return n, err
	}
	if n.ID == "" {
		n.ID = id.New()
	}
	now := time.Now().UTC()
	n.CreatedAt, n.UpdatedAt = now, now
	if n.Kind == "" {
		n.Kind = "local"
	}
	if n.Role == "" {
		n.Role = "egress"
	}
	if n.Status == "" {
		n.Status = "draft"
	}
	if n.OSFamily == "" {
		n.OSFamily = "linux"
	}
	if n.OSVersion == "" {
		n.OSVersion = "unknown"
	}
	if n.Architecture == "" {
		n.Architecture = "amd64"
	}
	if n.ExecutionMode == "" {
		n.ExecutionMode = "local_managed"
	}
	if n.AgentStatus == "" {
		n.AgentStatus = "unknown"
	}
	if n.GeoIPStatus == "" {
		n.GeoIPStatus = "pending"
	}
	if err := normalizeNodeLocation(&n); err != nil {
		return n, err
	}
	_, err := s.db.Exec(ctx, `insert into nodes(
		id,name,kind,role,status,address,location_label,latitude,longitude,accuracy_radius_km,
		geoip_provider,geoip_status,geoip_ip,geoip_country_code,geoip_country_name,geoip_region,geoip_city,geoip_org,geoip_asn,geoip_resolved_at,geoip_error,
		os_family,os_version,architecture,execution_mode,agent_status,created_at,updated_at
	) values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28)`,
		n.ID, n.Name, n.Kind, n.Role, n.Status, n.Address, n.LocationLabel, n.Latitude, n.Longitude, n.AccuracyRadiusKM,
		n.GeoIPProvider, n.GeoIPStatus, n.GeoIPIP, n.GeoIPCountryCode, n.GeoIPCountryName, n.GeoIPRegion, n.GeoIPCity, n.GeoIPOrg, n.GeoIPASN, n.GeoIPResolvedAt, n.GeoIPError,
		n.OSFamily, n.OSVersion, n.Architecture, n.ExecutionMode, n.AgentStatus, n.CreatedAt, n.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err, "nodes_name_key", "nodes_name_active_key") {
			return n, fmt.Errorf("node name %q is already used by an active node", n.Name)
		}
		return n, err
	}
	_, _ = s.CreateAudit(ctx, "system", "node.create", "node", &n.ID, "node created")
	return n, nil
}

func (s *Store) UpdateNode(ctx context.Context, nodeID string, n domain.Node) (domain.Node, error) {
	current, err := s.GetNode(ctx, nodeID)
	if err != nil {
		return domain.Node{}, err
	}
	if current.Status == "retired" {
		return domain.Node{}, fmt.Errorf("retired node cannot be updated")
	}
	n.Name = strings.TrimSpace(n.Name)
	n.Kind = strings.TrimSpace(n.Kind)
	n.Role = strings.TrimSpace(n.Role)
	n.Address = strings.TrimSpace(n.Address)
	n.LocationLabel = strings.TrimSpace(n.LocationLabel)
	n.OSFamily = strings.TrimSpace(n.OSFamily)
	n.OSVersion = strings.TrimSpace(n.OSVersion)
	n.Architecture = strings.TrimSpace(n.Architecture)
	n.ExecutionMode = strings.TrimSpace(n.ExecutionMode)
	if n.Name == "" {
		return domain.Node{}, errors.New("node name is required")
	}
	if n.Address == "" {
		return domain.Node{}, errors.New("node address is required")
	}
	if err := domain.ValidateRequiredSingleLine("node name", n.Name); err != nil {
		return domain.Node{}, err
	}
	if err := domain.ValidateRequiredSingleLine("node address", n.Address); err != nil {
		return domain.Node{}, err
	}
	if n.Kind == "" {
		n.Kind = current.Kind
	}
	if n.Role == "" {
		n.Role = current.Role
	}
	if n.OSFamily == "" {
		n.OSFamily = current.OSFamily
	}
	if n.OSVersion == "" {
		n.OSVersion = current.OSVersion
	}
	if n.Architecture == "" {
		n.Architecture = current.Architecture
	}
	if n.ExecutionMode == "" {
		n.ExecutionMode = current.ExecutionMode
	}
	if n.GeoIPStatus == "" {
		copyNodeGeoIP(&n, current)
	}
	if err := normalizeNodeLocation(&n); err != nil {
		return domain.Node{}, err
	}
	cmd, err := s.db.Exec(ctx, `update nodes set
		name=$2,kind=$3,role=$4,address=$5,location_label=$6,latitude=$7,longitude=$8,accuracy_radius_km=$9,
		geoip_provider=$10,geoip_status=$11,geoip_ip=$12,geoip_country_code=$13,geoip_country_name=$14,geoip_region=$15,geoip_city=$16,geoip_org=$17,geoip_asn=$18,geoip_resolved_at=$19,geoip_error=$20,
		os_family=$21,os_version=$22,architecture=$23,execution_mode=$24,updated_at=now()
		where id=$1 and status <> 'retired'`,
		nodeID, n.Name, n.Kind, n.Role, n.Address, n.LocationLabel, n.Latitude, n.Longitude, n.AccuracyRadiusKM,
		n.GeoIPProvider, n.GeoIPStatus, n.GeoIPIP, n.GeoIPCountryCode, n.GeoIPCountryName, n.GeoIPRegion, n.GeoIPCity, n.GeoIPOrg, n.GeoIPASN, n.GeoIPResolvedAt, n.GeoIPError,
		n.OSFamily, n.OSVersion, n.Architecture, n.ExecutionMode)
	if err != nil {
		if isUniqueViolation(err, "nodes_name_key", "nodes_name_active_key") {
			return domain.Node{}, fmt.Errorf("node name %q is already used by an active node", n.Name)
		}
		return domain.Node{}, err
	}
	if cmd.RowsAffected() == 0 {
		return domain.Node{}, pgx.ErrNoRows
	}
	_, _ = s.CreateAudit(ctx, "system", "node.update", "node", &nodeID, "node profile updated")
	return s.GetNode(ctx, nodeID)
}

func (s *Store) UpdateNodeGeoIP(ctx context.Context, nodeID string, geo domain.NodeGeoIP) (domain.Node, error) {
	if strings.TrimSpace(nodeID) == "" {
		return domain.Node{}, errors.New("node id is required")
	}
	status := strings.TrimSpace(geo.Status)
	if status == "" {
		status = "pending"
	}
	cmd, err := s.db.Exec(ctx, `update nodes set
		location_label=$2,latitude=$3,longitude=$4,accuracy_radius_km=$5,
		geoip_provider=$6,geoip_status=$7,geoip_ip=$8,geoip_country_code=$9,geoip_country_name=$10,geoip_region=$11,geoip_city=$12,geoip_org=$13,geoip_asn=$14,geoip_resolved_at=$15,geoip_error=$16,
		updated_at=now()
		where id=$1 and status <> 'retired'`,
		nodeID, strings.TrimSpace(geo.LocationLabel), geo.Latitude, geo.Longitude, geo.AccuracyRadiusKM,
		strings.TrimSpace(geo.Provider), status, strings.TrimSpace(geo.IP), strings.TrimSpace(geo.CountryCode), strings.TrimSpace(geo.CountryName), strings.TrimSpace(geo.Region), strings.TrimSpace(geo.City), strings.TrimSpace(geo.Org), strings.TrimSpace(geo.ASN), geo.ResolvedAt, strings.TrimSpace(geo.Error))
	if err != nil {
		return domain.Node{}, err
	}
	if cmd.RowsAffected() == 0 {
		return domain.Node{}, pgx.ErrNoRows
	}
	return s.GetNode(ctx, nodeID)
}

func copyNodeGeoIP(dst *domain.Node, src domain.Node) {
	dst.LocationLabel = src.LocationLabel
	dst.Latitude = src.Latitude
	dst.Longitude = src.Longitude
	dst.AccuracyRadiusKM = src.AccuracyRadiusKM
	dst.GeoIPProvider = src.GeoIPProvider
	dst.GeoIPStatus = src.GeoIPStatus
	dst.GeoIPIP = src.GeoIPIP
	dst.GeoIPCountryCode = src.GeoIPCountryCode
	dst.GeoIPCountryName = src.GeoIPCountryName
	dst.GeoIPRegion = src.GeoIPRegion
	dst.GeoIPCity = src.GeoIPCity
	dst.GeoIPOrg = src.GeoIPOrg
	dst.GeoIPASN = src.GeoIPASN
	dst.GeoIPResolvedAt = src.GeoIPResolvedAt
	dst.GeoIPError = src.GeoIPError
}

func normalizeNodeLocation(n *domain.Node) error {
	n.LocationLabel = strings.TrimSpace(n.LocationLabel)
	if n.Latitude == nil && n.Longitude == nil {
		if n.AccuracyRadiusKM != nil {
			return errors.New("node accuracy radius requires latitude and longitude")
		}
		return nil
	}
	if n.Latitude == nil || n.Longitude == nil {
		return errors.New("node latitude and longitude must be provided together")
	}
	if *n.Latitude < -90 || *n.Latitude > 90 {
		return errors.New("node latitude must be between -90 and 90")
	}
	if *n.Longitude < -180 || *n.Longitude > 180 {
		return errors.New("node longitude must be between -180 and 180")
	}
	if n.AccuracyRadiusKM != nil {
		if *n.AccuracyRadiusKM < 0 || *n.AccuracyRadiusKM > 20000 {
			return errors.New("node accuracy radius must be between 0 and 20000 km")
		}
	}
	return nil
}

func isUniqueViolation(err error, constraints ...string) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return false
	}
	for _, constraint := range constraints {
		if pgErr.ConstraintName == constraint {
			return true
		}
	}
	return len(constraints) == 0
}

func (s *Store) RetireNode(ctx context.Context, nodeID string) (domain.Node, error) {
	var activeInstances int
	if err := s.db.QueryRow(ctx, `select count(*) from instances where node_id=$1 and status not in ('deleted','disabled')`, nodeID).Scan(&activeInstances); err != nil {
		return domain.Node{}, err
	}
	if activeInstances > 0 {
		return domain.Node{}, fmt.Errorf("node has %d active instances", activeInstances)
	}
	cmd, err := s.db.Exec(ctx, `update nodes set status='retired',agent_status='offline',updated_at=now() where id=$1`, nodeID)
	if err != nil {
		return domain.Node{}, err
	}
	if cmd.RowsAffected() == 0 {
		return domain.Node{}, pgx.ErrNoRows
	}
	if _, err := s.db.Exec(ctx, `update node_agents set status='revoked',revoked_at=now() where node_id=$1`, nodeID); err != nil {
		return domain.Node{}, fmt.Errorf("revoke node agents during retire: %w", err)
	}
	_, _ = s.CreateAudit(ctx, "system", "node.retire", "node", &nodeID, "node retired")
	return s.GetNode(ctx, nodeID)
}

func (s *Store) SetNodeMaintenance(ctx context.Context, nodeID string, enabled bool) (domain.Node, error) {
	status := "online"
	if enabled {
		status = "maintenance"
	}
	cmd, err := s.db.Exec(ctx, `update nodes set status=$2,updated_at=now() where id=$1 and status <> 'retired'`, nodeID, status)
	if err != nil {
		return domain.Node{}, err
	}
	if cmd.RowsAffected() == 0 {
		return domain.Node{}, pgx.ErrNoRows
	}
	action := "node.maintenance.disable"
	if enabled {
		action = "node.maintenance.enable"
	}
	_, _ = s.CreateAudit(ctx, "system", action, "node", &nodeID, action)
	return s.GetNode(ctx, nodeID)
}

func (s *Store) UpsertAgentNode(ctx context.Context, name, address, token string) (domain.Node, error) {
	return s.UpsertAgentNodeWithVersion(ctx, name, address, token, "", "")
}

func (s *Store) UpsertAgentNodeWithVersion(ctx context.Context, name, address, token, agentVersion, protocolVersion string) (domain.Node, error) {
	if name == "" {
		return domain.Node{}, errors.New("agent node name is empty")
	}
	row := s.db.QueryRow(ctx, `select
		n.id,n.name,n.kind,n.role,n.status,n.address,n.os_family,n.os_version,n.architecture,n.execution_mode,n.agent_status,n.last_heartbeat_at,n.created_at,n.updated_at,
		coalesce(na.agent_version,''),coalesce(na.protocol_version,''),na.registered_at,na.last_seen_at
	from nodes n
	left join node_agents na on na.node_id=n.id
	where n.name=$1`, name)
	n, err := scanNode(row)
	if errors.Is(err, pgx.ErrNoRows) {
		n = domain.Node{Name: name, Kind: "remote", Role: "egress", Status: "online", Address: address, OSFamily: "linux", OSVersion: "unknown", Architecture: "amd64", ExecutionMode: "agent_managed", AgentStatus: "online"}
		n, err = s.CreateNode(ctx, n)
		if err != nil {
			return n, err
		}
	} else if err != nil {
		return n, err
	}
	now := time.Now().UTC()
	fingerprint := token
	if fingerprint == "" {
		fingerprint = "agent:" + name
	}
	agentVersion = normalizeAgentRuntimeVersion(agentVersion, "dev")
	protocolVersion = normalizeAgentRuntimeVersion(protocolVersion, "v1")
	_, err = s.db.Exec(ctx, `insert into node_agents(id,node_id,status,agent_version,protocol_version,fingerprint,registered_at,last_seen_at)
		values($1,$2,'active',$3,$4,$5,$6,$6)
		on conflict(node_id) do update
		set status='active',
		    agent_version=excluded.agent_version,
		    protocol_version=excluded.protocol_version,
		    last_seen_at=excluded.last_seen_at,
		    revoked_at=null`, id.New(), n.ID, agentVersion, protocolVersion, fingerprint, now)
	if err != nil {
		return n, err
	}
	_, err = s.db.Exec(ctx, `update nodes set status='online', agent_status='online', address=coalesce(nullif($2,''),address), last_heartbeat_at=$3, updated_at=$3 where id=$1`, n.ID, address, now)
	n.LastHeartbeatAt = &now
	n.Status = "online"
	n.AgentStatus = "online"
	return n, err
}

func (s *Store) Heartbeat(ctx context.Context, nodeName string) error {
	return s.HeartbeatWithVersion(ctx, nodeName, "", "")
}

func (s *Store) HeartbeatWithVersion(ctx context.Context, nodeName, agentVersion, protocolVersion string) error {
	now := time.Now().UTC()
	cmd, err := s.db.Exec(ctx, `update nodes set status='online',agent_status='online',last_heartbeat_at=$2,updated_at=$2 where name=$1 and status <> 'retired'`, nodeName, now)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	if _, err := s.db.Exec(ctx, `update node_agents
		set last_seen_at=$2,
		    status='active',
		    agent_version=coalesce(nullif($3,''),agent_version),
		    protocol_version=coalesce(nullif($4,''),protocol_version)
		where node_id=(select id from nodes where name=$1)`, nodeName, now, strings.TrimSpace(agentVersion), strings.TrimSpace(protocolVersion)); err != nil {
		return err
	}
	return nil
}

func (s *Store) ListNodeCapabilityInstallEvents(ctx context.Context, nodeID string, limit int) ([]domain.NodeCapabilityInstallEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `select id,node_id,job_id,capability_code,strategy,status,summary,payload_json,created_at from node_capability_install_events where node_id=$1 order by created_at desc limit $2`, nodeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.NodeCapabilityInstallEvent{}
	for rows.Next() {
		var x domain.NodeCapabilityInstallEvent
		var raw []byte
		if err := rows.Scan(&x.ID, &x.NodeID, &x.JobID, &x.CapabilityCode, &x.Strategy, &x.Status, &x.Summary, &raw, &x.CreatedAt); err != nil {
			return nil, err
		}
		if err := decodeJSONField(raw, &x.Payload, "node_capability_install_events.payload_json"); err != nil {
			return nil, err
		}
		if x.Payload == nil {
			x.Payload = map[string]any{}
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) LatestNodeInventory(ctx context.Context, nodeID string) (domain.NodeInventorySnapshot, error) {
	row := s.db.QueryRow(ctx, `select id,node_id,payload_json,created_at from node_inventory_snapshots where node_id=$1 order by created_at desc limit 1`, nodeID)
	return scanNodeInventory(row)
}

func (s *Store) ListNodeCapabilities(ctx context.Context, nodeID string) ([]domain.NodeCapability, error) {
	rows, err := s.db.Query(ctx, `select id,node_id,capability_code,coalesce(version,''),status,source,detected_at from node_capabilities where node_id=$1 order by capability_code`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.NodeCapability{}
	for rows.Next() {
		var c domain.NodeCapability
		if err := rows.Scan(&c.ID, &c.NodeID, &c.CapabilityCode, &c.Version, &c.Status, &c.Source, &c.DetectedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) ListAllNodeCapabilities(ctx context.Context) (map[string][]domain.NodeCapability, error) {
	rows, err := s.db.Query(ctx, `select id,node_id,capability_code,coalesce(version,''),status,source,detected_at from node_capabilities order by node_id,capability_code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]domain.NodeCapability{}
	for rows.Next() {
		var c domain.NodeCapability
		if err := rows.Scan(&c.ID, &c.NodeID, &c.CapabilityCode, &c.Version, &c.Status, &c.Source, &c.DetectedAt); err != nil {
			return nil, err
		}
		out[c.NodeID] = append(out[c.NodeID], c)
	}
	return out, rows.Err()
}

func (s *Store) SubmitNodeInventory(ctx context.Context, nodeID string, payload map[string]any) (domain.NodeInventorySnapshot, []domain.NodeCapability, error) {
	if nodeID == "" {
		return domain.NodeInventorySnapshot{}, nil, errors.New("node_id is required")
	}
	if payload == nil {
		payload = map[string]any{}
	}
	if _, err := s.GetNode(ctx, nodeID); err != nil {
		return domain.NodeInventorySnapshot{}, nil, err
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return domain.NodeInventorySnapshot{}, nil, fmt.Errorf("marshal node inventory payload: %w", err)
	}
	snap := domain.NodeInventorySnapshot{ID: id.New(), NodeID: nodeID, Payload: payload, CreatedAt: time.Now().UTC()}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return snap, nil, err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `insert into node_inventory_snapshots(id,node_id,payload_json,created_at) values($1,$2,$3,$4)`, snap.ID, snap.NodeID, b, snap.CreatedAt)
	if err != nil {
		return snap, nil, err
	}
	if osVersion := stringFromPath(payload, "os", "pretty_name"); osVersion != "" {
		arch := stringFromPath(payload, "arch")
		if arch == "" {
			arch = "amd64"
		}
		if !in(arch, "amd64", "arm64") {
			arch = "amd64"
		}
		if _, err := tx.Exec(ctx, `update nodes set os_family='linux', os_version=$2, architecture=$3, updated_at=now() where id=$1`, nodeID, osVersion, arch); err != nil {
			return snap, nil, err
		}
	}
	caps := deriveCapabilities(nodeID, payload, snap.CreatedAt)
	for _, c := range caps {
		_, err = tx.Exec(ctx, `insert into node_capabilities(id,node_id,capability_code,version,status,detected_at,source) values($1,$2,$3,$4,$5,$6,$7)
			on conflict(node_id, capability_code) do update set version=excluded.version,status=excluded.status,detected_at=excluded.detected_at,source=excluded.source`, c.ID, c.NodeID, c.CapabilityCode, c.Version, c.Status, c.DetectedAt, c.Source)
		if err != nil {
			return snap, nil, err
		}
	}
	discoveries := deriveServiceDiscoveries(nodeID, payload, snap.CreatedAt)
	for _, d := range discoveries {
		db, err := json.Marshal(d.Payload)
		if err != nil {
			return snap, nil, fmt.Errorf("marshal service discovery payload: %w", err)
		}
		_, err = tx.Exec(ctx, `insert into node_service_discoveries(id,node_id,service_code,name,systemd_unit,config_path,status,source,confidence,endpoint_host,endpoint_port,payload_json,detected_at) values($1,$2,$3,$4,nullif($5,''),nullif($6,''),$7,$8,$9,nullif($10,''),nullif($11,0),$12,$13)
			on conflict(node_id, service_code, name) do update set systemd_unit=excluded.systemd_unit,config_path=excluded.config_path,status=case when node_service_discoveries.status='imported' then node_service_discoveries.status else excluded.status end,source=excluded.source,confidence=excluded.confidence,endpoint_host=excluded.endpoint_host,endpoint_port=excluded.endpoint_port,payload_json=excluded.payload_json,detected_at=excluded.detected_at`, d.ID, d.NodeID, d.ServiceCode, d.Name, d.SystemdUnit, d.ConfigPath, d.Status, d.Source, d.Confidence, d.EndpointHost, d.EndpointPort, db, d.DetectedAt)
		if err != nil {
			return snap, nil, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return snap, nil, err
	}
	_, _ = s.CreateAudit(ctx, "agent", "node.inventory.submit", "node", &nodeID, "node inventory submitted")
	return snap, caps, nil
}

func (s *Store) CreateNodeInventoryJob(ctx context.Context, nodeID string) (domain.Job, error) {
	if _, err := s.GetNode(ctx, nodeID); err != nil {
		return domain.Job{}, err
	}
	j, err := s.CreateJob(ctx, domain.Job{Type: "node.inventory.sync", ScopeType: "node", ScopeID: &nodeID, NodeID: &nodeID, Payload: map[string]any{"node_id": nodeID}})
	if err != nil {
		return j, err
	}
	_, _ = s.CreateAudit(ctx, "system", "node.inventory.sync", "node", &nodeID, "node inventory sync queued")
	return j, nil
}

func (s *Store) ListNodeServiceDiscoveries(ctx context.Context, nodeID string) ([]domain.NodeServiceDiscovery, error) {
	rows, err := s.db.Query(ctx, `select id,node_id,service_code,name,coalesce(systemd_unit,''),coalesce(config_path,''),status,source,coalesce(confidence,50),coalesce(endpoint_host,''),coalesce(endpoint_port,0),managed_instance_id,payload_json,detected_at from node_service_discoveries where node_id=$1 order by service_code,name`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.NodeServiceDiscovery{}
	for rows.Next() {
		x, err := scanNodeServiceDiscovery(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) GetNodeServiceDiscovery(ctx context.Context, nodeID, discoveryID string) (domain.NodeServiceDiscovery, error) {
	row := s.db.QueryRow(ctx, `select id,node_id,service_code,name,coalesce(systemd_unit,''),coalesce(config_path,''),status,source,coalesce(confidence,50),coalesce(endpoint_host,''),coalesce(endpoint_port,0),managed_instance_id,payload_json,detected_at from node_service_discoveries where node_id=$1 and id=$2`, nodeID, discoveryID)
	return scanNodeServiceDiscovery(row)
}

func (s *Store) NodeServiceDiscoverySummary(ctx context.Context, nodeID string) (domain.NodeServiceDiscoverySummary, error) {
	if _, err := s.GetNode(ctx, nodeID); err != nil {
		return domain.NodeServiceDiscoverySummary{}, err
	}
	summary := domain.NodeServiceDiscoverySummary{NodeID: nodeID, ByService: map[string]int{}}
	rows, err := s.db.Query(ctx, `select service_code,status,count(*) from node_service_discoveries where node_id=$1 group by service_code,status`, nodeID)
	if err != nil {
		return summary, err
	}
	defer rows.Close()
	for rows.Next() {
		var serviceCode, status string
		var count int
		if err := rows.Scan(&serviceCode, &status, &count); err != nil {
			return summary, err
		}
		summary.Total += count
		summary.ByService[serviceCode] += count
		switch status {
		case "available":
			summary.Available += count
			summary.ImportableCount += count
		case "discovered":
			summary.Discovered += count
			summary.ImportableCount += count
		case "imported":
			summary.Imported += count
		case "ignored":
			summary.Ignored += count
		}
	}
	return summary, rows.Err()
}

func (s *Store) CreateNodeServiceDiscoveryJob(ctx context.Context, nodeID string) (domain.Job, error) {
	if _, err := s.GetNode(ctx, nodeID); err != nil {
		return domain.Job{}, err
	}
	j, err := s.CreateJob(ctx, domain.Job{Type: "node.services.discover", ScopeType: "node", ScopeID: &nodeID, NodeID: &nodeID, Payload: map[string]any{"node_id": nodeID}})
	if err != nil {
		return j, err
	}
	_, _ = s.CreateAudit(ctx, "system", "node.services.discover", "node", &nodeID, "node service discovery queued")
	return j, nil
}

func (s *Store) CreateNodeCapabilityInstallJob(ctx context.Context, nodeID, serviceCode, strategy, channel string) (domain.Job, error) {
	return s.CreateNodeCapabilityInstallJobWithDependents(ctx, nodeID, serviceCode, strategy, channel, nil)
}

func (s *Store) CreateNodeCapabilityInstallJobWithDependents(ctx context.Context, nodeID, serviceCode, strategy, channel string, dependentInstanceIDs []string) (domain.Job, error) {
	serviceCode = normalizeCapabilityCode(serviceCode)
	strategy = strings.TrimSpace(strategy)
	channel = strings.TrimSpace(channel)
	if _, err := s.GetNode(ctx, nodeID); err != nil {
		return domain.Job{}, err
	}
	if serviceCode == "" {
		return domain.Job{}, errors.New("service_code is required")
	}
	dependentInstanceIDs = normalizedStringSet(dependentInstanceIDs)
	if existing, ok, err := s.findActiveCapabilityInstallJob(ctx, nodeID, serviceCode); err != nil {
		return domain.Job{}, err
	} else if ok {
		if err := s.mergeCapabilityInstallJobDependents(ctx, existing.ID, dependentInstanceIDs); err != nil {
			return domain.Job{}, err
		}
		if err := s.markCapabilityInstallQueued(ctx, nodeID, serviceCode, existing, dependentInstanceIDs); err != nil {
			return domain.Job{}, err
		}
		return s.GetJob(ctx, existing.ID)
	}
	var binaryArtifact domain.BinaryArtifact
	binaryRepositoryAvailable := false
	checkBinaryRepository := strategy == "" || strategy == "binary_repository" || (strategy == "ubuntu_repo" && serviceCode == driver.Shadowsocks)
	if checkBinaryRepository {
		artifact, err := s.FindBinaryRuntimeArtifactForNode(ctx, nodeID, serviceCode)
		switch {
		case err == nil:
			binaryRepositoryAvailable = true
			binaryArtifact = artifact
			if strategy == "" {
				strategy = "binary_repository"
			}
		case strategy == "binary_repository":
			return domain.Job{}, s.binaryRuntimeArtifactUnavailableError(ctx, nodeID, serviceCode, err)
		case !errors.Is(err, pgx.ErrNoRows):
			return domain.Job{}, fmt.Errorf("binary repository preflight failed for %s on node %s: %w", serviceCode, nodeID, err)
		}
	}
	if strategy == "" {
		strategy = defaultCapabilityInstallStrategy(serviceCode)
		if strategy == "" {
			return domain.Job{}, fmt.Errorf("unsupported installable service: %s", serviceCode)
		}
	}
	if channel == "" {
		if serviceCode == "nginx" {
			channel = "stable"
		} else {
			channel = "latest"
		}
	}
	if !validInstallStrategy(serviceCode, strategy) {
		return domain.Job{}, fmt.Errorf("unsupported install strategy %q for %s", strategy, serviceCode)
	}
	payload := map[string]any{"node_id": nodeID, "service_code": serviceCode, "strategy": strategy, "channel": channel}
	if len(dependentInstanceIDs) > 0 {
		payload["dependent_instance_ids"] = dependentInstanceIDs
	}
	if strategy == "binary_repository" {
		if !binaryRepositoryAvailable {
			return domain.Job{}, s.binaryRuntimeArtifactUnavailableError(ctx, nodeID, serviceCode, nil)
		}
		jobID := id.New()
		ticketJobID := jobID
		ticket, err := newBinaryDownloadTicket(binaryArtifact.ID, nodeID, &ticketJobID, 2*time.Hour)
		if err != nil {
			return domain.Job{}, fmt.Errorf("binary repository download ticket create failed: %w", err)
		}
		payload["binary_repository"] = binaryRepositoryPayload(binaryArtifact, ticket)
		j, err := s.createCapabilityInstallJob(ctx, domain.Job{ID: jobID, Type: "node.capability.install", ScopeType: "node", ScopeID: &nodeID, NodeID: &nodeID, Priority: 60, Payload: payload}, &ticket, nodeID, serviceCode, strategy, dependentInstanceIDs)
		return j, err
	}
	if strategy == "ubuntu_repo" && serviceCode == driver.Shadowsocks && binaryRepositoryAvailable {
		jobID := id.New()
		ticketJobID := jobID
		ticket, err := newBinaryDownloadTicket(binaryArtifact.ID, nodeID, &ticketJobID, 2*time.Hour)
		if err != nil {
			return domain.Job{}, fmt.Errorf("binary repository fallback download ticket create failed: %w", err)
		}
		payload["binary_repository_fallback"] = binaryRepositoryPayload(binaryArtifact, ticket)
		j, err := s.createCapabilityInstallJob(ctx, domain.Job{ID: jobID, Type: "node.capability.install", ScopeType: "node", ScopeID: &nodeID, NodeID: &nodeID, Priority: 60, Payload: payload}, &ticket, nodeID, serviceCode, strategy, dependentInstanceIDs)
		return j, err
	}
	return s.createCapabilityInstallJob(ctx, domain.Job{Type: "node.capability.install", ScopeType: "node", ScopeID: &nodeID, NodeID: &nodeID, Priority: 60, Payload: payload}, nil, nodeID, serviceCode, strategy, dependentInstanceIDs)
}

func (s *Store) binaryRuntimeArtifactUnavailableError(ctx context.Context, nodeID, serviceCode string, cause error) error {
	var nodeName, osFamily, osVersion, arch string
	err := s.db.QueryRow(ctx, `select name,coalesce(os_family,''),coalesce(os_version,''),coalesce(architecture,'') from nodes where id=$1 and status <> 'retired'`, nodeID).
		Scan(&nodeName, &osFamily, &osVersion, &arch)
	if err != nil {
		if cause != nil {
			return fmt.Errorf("binary repository artifact is not available for %s on node %s: %w", serviceCode, nodeID, cause)
		}
		return fmt.Errorf("binary repository artifact is not available for %s on node %s", serviceCode, nodeID)
	}
	osFamily = strings.ToLower(strings.TrimSpace(osFamily))
	if osFamily == "" {
		osFamily = "linux"
	}
	osVersion = strings.TrimSpace(osVersion)
	arch = normalizeBinaryArtifactArchitecture(arch)
	if arch == "" {
		arch = "amd64"
	}
	versionRequirement := "empty os_version"
	if osVersion != "" {
		versionRequirement = "os_version=" + osVersion + " or empty os_version"
	}
	return fmt.Errorf("binary repository artifact is not available for %s on node %s (%s; %s/%s). Register an active runtime/package/script/bundle artifact with service_code=%s, os_family=%s, architecture=%s and %s, then retry Install runtime",
		serviceCode, nodeName, nodeID, osFamily, arch, serviceCode, osFamily, arch, versionRequirement)
}

func (s *Store) createCapabilityInstallJob(ctx context.Context, j domain.Job, ticket *domain.BinaryDownloadTicket, nodeID, serviceCode, strategy string, dependentInstanceIDs []string) (domain.Job, error) {
	j, payloadJSON, err := normalizeJobForInsert(j)
	if err != nil {
		return domain.Job{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return domain.Job{}, err
	}
	defer tx.Rollback(ctx)
	if err := insertJobRow(ctx, tx, j, payloadJSON); err != nil {
		return domain.Job{}, err
	}
	if ticket != nil {
		if err := insertBinaryDownloadTicket(ctx, tx, *ticket); err != nil {
			return domain.Job{}, err
		}
	}
	eventPayload := redactSensitiveMapForStorage(j.Payload)
	if _, err := tx.Exec(ctx, `insert into node_capability_install_events(id,node_id,job_id,capability_code,strategy,status,summary,payload_json,created_at) values($1,$2,$3,$4,$5,'queued','capability install queued',$6,now())`, id.New(), nodeID, j.ID, serviceCode, strategy, mustJSON(eventPayload)); err != nil {
		return domain.Job{}, err
	}
	if err := s.markCapabilityInstallQueuedTx(ctx, tx, nodeID, serviceCode, j, dependentInstanceIDs); err != nil {
		return domain.Job{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Job{}, err
	}
	_, _ = s.CreateAudit(ctx, "system", "job.create", "job", &j.ID, "job queued")
	_, _ = s.CreateAudit(ctx, "system", "node.capability.install", "node", &nodeID, "capability install queued: "+serviceCode)
	return j, nil
}

func (s *Store) findActiveCapabilityInstallJob(ctx context.Context, nodeID, serviceCode string) (domain.Job, bool, error) {
	serviceCode = runtimeCapabilityCodeForService(serviceCode)
	if nodeID == "" || serviceCode == "" {
		return domain.Job{}, false, nil
	}
	row := s.db.QueryRow(ctx, `select id,type,scope_type,scope_id,node_id,instance_id,status,priority,payload_json,coalesce(result_json,'{}'::jsonb),locked_by,locked_until,created_at,started_at,finished_at
		from jobs
		where node_id=$1
		  and type='node.capability.install'
		  and status in ('queued','running','retrying')
		  and payload_json->>'service_code'=$2
		order by created_at desc
		limit 1`, nodeID, serviceCode)
	job, err := scanJob(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Job{}, false, nil
	}
	if err != nil {
		return domain.Job{}, false, err
	}
	return job, true, nil
}

func (s *Store) mergeCapabilityInstallJobDependents(ctx context.Context, jobID string, dependentInstanceIDs []string) error {
	dependentInstanceIDs = normalizedStringSet(dependentInstanceIDs)
	if len(dependentInstanceIDs) == 0 {
		return nil
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var raw []byte
	if err := tx.QueryRow(ctx, `select payload_json from jobs where id=$1 and type='node.capability.install' and status in ('queued','running','retrying') for update`, jobID).Scan(&raw); err != nil {
		return err
	}
	payload := map[string]any{}
	if err := decodeJSONField(raw, &payload, "jobs.payload_json"); err != nil {
		return err
	}
	merged := normalizedStringSet(append(stringSetFromAny(payload["dependent_instance_ids"]), dependentInstanceIDs...))
	if len(merged) > 0 {
		payload["dependent_instance_ids"] = merged
	}
	if _, err := tx.Exec(ctx, `update jobs set payload_json=$2 where id=$1`, jobID, mustJSON(payload)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) markCapabilityInstallQueued(ctx context.Context, nodeID, serviceCode string, job domain.Job, dependentInstanceIDs []string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := s.markCapabilityInstallQueuedTx(ctx, tx, nodeID, serviceCode, job, dependentInstanceIDs); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) markCapabilityInstallQueuedTx(ctx context.Context, tx pgx.Tx, nodeID, serviceCode string, job domain.Job, dependentInstanceIDs []string) error {
	serviceCode = runtimeCapabilityCodeForService(serviceCode)
	if nodeID == "" || serviceCode == "" {
		return nil
	}
	if err := upsertNodeCapabilityTx(ctx, tx, nodeID, serviceCode, "", "installing", "installer"); err != nil {
		return err
	}
	for _, instanceID := range normalizedStringSet(dependentInstanceIDs) {
		instance, err := getInstanceTx(ctx, tx, instanceID)
		if err != nil {
			continue
		}
		if strings.TrimSpace(instance.NodeID) != nodeID {
			continue
		}
		if runtimeCapabilityCodeForService(instance.ServiceCode) != serviceCode {
			continue
		}
		if err := s.upsertInstanceRuntimeStateForQueuedJobTx(ctx, tx, instance, job); err != nil {
			return err
		}
	}
	return nil
}

func binaryRepositoryPayload(artifact domain.BinaryArtifact, ticket domain.BinaryDownloadTicket) map[string]any {
	return map[string]any{
		"artifact_id":    artifact.ID,
		"name":           artifact.Name,
		"kind":           artifact.Kind,
		"service_code":   artifact.ServiceCode,
		"version":        artifact.Version,
		"os_family":      artifact.OSFamily,
		"os_version":     artifact.OSVersion,
		"architecture":   artifact.Architecture,
		"sha256":         artifact.SHA256,
		"signature":      artifact.Signature,
		"metadata":       artifact.Metadata,
		"download_path":  "/agent/binary-artifacts/" + artifact.ID + "/download",
		"download_token": ticket.Token,
		"ticket_id":      ticket.ID,
		"ticket_hint":    ticket.TokenHint,
		"expires_at":     ticket.ExpiresAt.Format(time.RFC3339),
	}
}

func defaultCapabilityInstallStrategy(serviceCode string) string {
	switch normalizeCapabilityCode(serviceCode) {
	case driver.Nginx:
		return "nginx_org_repo"
	case driver.XrayCore:
		return "xtls_install_release"
	case driver.OpenVPN, driver.WireGuard, driver.IPSec, driver.XL2TPD, driver.HTTPProxy, driver.Shadowsocks:
		return "ubuntu_repo"
	default:
		return ""
	}
}

func normalizedStringSet(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func (s *Store) CreateNodeCapabilityVerifyJob(ctx context.Context, nodeID, serviceCode string) (domain.Job, error) {
	serviceCode = normalizeCapabilityCode(serviceCode)
	if _, err := s.GetNode(ctx, nodeID); err != nil {
		return domain.Job{}, err
	}
	if serviceCode == "" {
		return domain.Job{}, errors.New("service_code is required")
	}
	payload := map[string]any{"node_id": nodeID, "service_code": serviceCode}
	j, err := s.CreateJob(ctx, domain.Job{Type: "node.capability.verify", ScopeType: "node", ScopeID: &nodeID, NodeID: &nodeID, Payload: payload})
	if err != nil {
		return j, err
	}
	_, _ = s.db.Exec(ctx, `insert into node_capability_install_events(id,node_id,job_id,capability_code,strategy,status,summary,payload_json,created_at) values($1,$2,$3,$4,'verify','queued','capability verify queued',$5,now())`, id.New(), nodeID, j.ID, serviceCode, mustJSON(redactSensitiveMapForStorage(payload)))
	_, _ = s.CreateAudit(ctx, "system", "node.capability.verify", "node", &nodeID, "capability verify queued: "+serviceCode)
	return j, nil
}

func (s *Store) IgnoreNodeServiceDiscovery(ctx context.Context, nodeID, discoveryID string, ignored bool) (domain.NodeServiceDiscovery, error) {
	status := "ignored"
	eventType := "ignored"
	if !ignored {
		status = "discovered"
		eventType = "unignored"
	}
	cmd, err := s.db.Exec(ctx, `update node_service_discoveries set status=$3 where node_id=$1 and id=$2 and status <> 'imported'`, nodeID, discoveryID, status)
	if err != nil {
		return domain.NodeServiceDiscovery{}, err
	}
	if cmd.RowsAffected() == 0 {
		return domain.NodeServiceDiscovery{}, pgx.ErrNoRows
	}
	_, _ = s.db.Exec(ctx, `insert into node_service_discovery_events(id,node_id,discovery_id,event_type,summary,payload_json,created_at) values($1,$2,$3,$4,$5,'{}'::jsonb,now())`, id.New(), nodeID, discoveryID, eventType, "service discovery "+eventType)
	_, _ = s.CreateAudit(ctx, "system", "node.service_discovery."+eventType, "node", &nodeID, "service discovery "+eventType)
	return s.GetNodeServiceDiscovery(ctx, nodeID, discoveryID)
}

func (s *Store) ImportNodeServiceDiscovery(ctx context.Context, nodeID, discoveryID string) (domain.Instance, error) {
	d, err := s.GetNodeServiceDiscovery(ctx, nodeID, discoveryID)
	if err != nil {
		return domain.Instance{}, err
	}
	if d.Status == "imported" && d.ManagedInstanceID != nil && *d.ManagedInstanceID != "" {
		return s.GetInstance(ctx, *d.ManagedInstanceID)
	}
	if d.Status == "ignored" {
		return domain.Instance{}, errors.New("discovery is ignored")
	}
	if !in(d.Status, "available", "discovered") {
		return domain.Instance{}, fmt.Errorf("discovery status %q is not importable", d.Status)
	}
	return s.importDiscoveryAsInstance(ctx, d)
}

func (s *Store) ImportAllNodeServiceDiscoveries(ctx context.Context, nodeID string) ([]domain.Instance, error) {
	items, err := s.ListNodeServiceDiscoveries(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	out := []domain.Instance{}
	for _, d := range items {
		if !in(d.Status, "available", "discovered") {
			continue
		}
		x, err := s.importDiscoveryAsInstance(ctx, d)
		if err != nil {
			_, _ = s.db.Exec(ctx, `insert into node_service_discovery_events(id,node_id,discovery_id,event_type,summary,payload_json,created_at) values($1,$2,$3,'failed',$4,$5,now())`, id.New(), d.NodeID, d.ID, err.Error(), mustJSON(map[string]any{"error": err.Error()}))
			continue
		}
		out = append(out, x)
	}
	return out, nil
}

func (s *Store) importDiscoveryAsInstance(ctx context.Context, d domain.NodeServiceDiscovery) (domain.Instance, error) {
	if d.NodeID == "" || d.ServiceCode == "" || d.Name == "" {
		return domain.Instance{}, errors.New("invalid discovery")
	}
	var existingID string
	err := s.db.QueryRow(ctx, `select id from instances where node_id=$1 and service_definition_id=(select id from service_definitions where code=$2) and (systemd_unit=$3 or name=$4) and status <> 'deleted' limit 1`, d.NodeID, d.ServiceCode, nullIfEmpty(d.SystemdUnit), d.Name).Scan(&existingID)
	if err == nil && existingID != "" {
		if _, err := s.db.Exec(ctx, `update node_service_discoveries set status='imported', managed_instance_id=$3 where node_id=$1 and id=$2`, d.NodeID, d.ID, existingID); err != nil {
			return domain.Instance{}, err
		}
		return s.GetInstance(ctx, existingID)
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return domain.Instance{}, err
	}
	name := strings.TrimSpace(d.Name)
	if name == "" {
		name = d.ServiceCode
	}
	slug := slugify(d.ServiceCode + "-" + name)
	baseSlug := slug
	for i := 2; ; i++ {
		var exists bool
		if err := s.db.QueryRow(ctx, `select exists(select 1 from instances where slug=$1 and status <> 'deleted')`, slug).Scan(&exists); err != nil {
			return domain.Instance{}, err
		}
		if !exists {
			break
		}
		slug = fmt.Sprintf("%s-%d", baseSlug, i)
	}
	now := time.Now().UTC()
	instanceID := id.New()
	spec := map[string]any{
		"source":        "discovery",
		"discovery_id":  d.ID,
		"service_code":  d.ServiceCode,
		"systemd_unit":  d.SystemdUnit,
		"config_path":   d.ConfigPath,
		"endpoint_host": d.EndpointHost,
		"endpoint_port": d.EndpointPort,
		"confidence":    d.Confidence,
		"raw":           d.Payload,
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.Instance{}, err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `insert into instances(id,node_id,service_definition_id,name,slug,systemd_unit,status,enabled,endpoint_host,endpoint_port,created_at,updated_at) values($1,$2,(select id from service_definitions where code=$3),$4,$5,nullif($6,''),'draft',false,nullif($7,''),nullif($8,0),$9,$10)`, instanceID, d.NodeID, d.ServiceCode, name, slug, d.SystemdUnit, d.EndpointHost, d.EndpointPort, now, now)
	if err != nil {
		return domain.Instance{}, err
	}
	b, err := json.Marshal(spec)
	if err != nil {
		return domain.Instance{}, fmt.Errorf("marshal imported instance spec: %w", err)
	}
	_, err = tx.Exec(ctx, `insert into instance_revisions(id,instance_id,revision_no,source,status,spec_json,created_at) values($1,$2,1,'import','validated',$3,now())`, id.New(), instanceID, b)
	if err != nil {
		return domain.Instance{}, err
	}
	_, err = tx.Exec(ctx, `update node_service_discoveries set status='imported', managed_instance_id=$3 where node_id=$1 and id=$2`, d.NodeID, d.ID, instanceID)
	if err != nil {
		return domain.Instance{}, err
	}
	if _, err := tx.Exec(ctx, `insert into node_service_discovery_events(id,node_id,discovery_id,event_type,summary,payload_json,created_at) values($1,$2,$3,'imported',$4,$5,now())`, id.New(), d.NodeID, d.ID, "service discovery imported", mustJSON(map[string]any{"instance_id": instanceID, "service_code": d.ServiceCode})); err != nil {
		return domain.Instance{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.Instance{}, err
	}
	_, _ = s.CreateAudit(ctx, "system", "node.service_discovery.import", "instance", &instanceID, "discovered service imported as instance")
	return s.GetInstance(ctx, instanceID)
}

func (s *Store) ListServiceDefinitions(ctx context.Context) ([]domain.ServiceDefinition, error) {
	rows, err := s.db.Query(ctx, `select id,code,name,category,tier,supports_accounts,supports_artifacts,coalesce(supports_install,false),coalesce(supports_instances,true),enabled,created_at from service_definitions order by tier, code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type rankedServiceDefinition struct {
		def   domain.ServiceDefinition
		score int
	}
	ranked := map[string]rankedServiceDefinition{}
	order := []string{}
	for rows.Next() {
		var x domain.ServiceDefinition
		if err := rows.Scan(&x.ID, &x.Code, &x.Name, &x.Category, &x.Tier, &x.SupportsAccounts, &x.SupportsArtifacts, &x.SupportsInstall, &x.SupportsInstances, &x.Enabled, &x.CreatedAt); err != nil {
			return nil, err
		}
		originalCode := x.Code
		canonicalCode := normalizeInstanceRuntimeCode(originalCode)
		score := 1
		if originalCode == canonicalCode {
			score = 2
		}
		x.Code = canonicalCode
		current, exists := ranked[canonicalCode]
		if !exists {
			order = append(order, canonicalCode)
			ranked[canonicalCode] = rankedServiceDefinition{def: x, score: score}
			continue
		}
		if score > current.score {
			ranked[canonicalCode] = rankedServiceDefinition{def: x, score: score}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]domain.ServiceDefinition, 0, len(order))
	for _, code := range order {
		out = append(out, ranked[code].def)
	}
	return out, nil
}

func (s *Store) ListInstances(ctx context.Context) ([]domain.Instance, error) {
	rows, err := s.db.Query(ctx, `select i.id,i.node_id,sd.code,i.name,i.slug,coalesce(i.systemd_unit,''),i.status,i.enabled,coalesce(i.endpoint_host,''),coalesce(i.endpoint_port,0),i.current_revision_id,i.last_applied_revision_id,i.created_at,i.updated_at from instances i join service_definitions sd on sd.id=i.service_definition_id where i.status <> 'deleted' order by i.created_at desc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Instance
	for rows.Next() {
		var x domain.Instance
		if err := rows.Scan(&x.ID, &x.NodeID, &x.ServiceCode, &x.Name, &x.Slug, &x.SystemdUnit, &x.Status, &x.Enabled, &x.EndpointHost, &x.EndpointPort, &x.CurrentRevisionID, &x.LastAppliedRevisionID, &x.CreatedAt, &x.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) GetInstance(ctx context.Context, instanceID string) (domain.Instance, error) {
	return getInstanceTx(ctx, s.db, instanceID)
}

func getInstanceTx(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, instanceID string) (domain.Instance, error) {
	var x domain.Instance
	err := q.QueryRow(ctx, `select i.id,i.node_id,sd.code,i.name,i.slug,coalesce(i.systemd_unit,''),i.status,i.enabled,coalesce(i.endpoint_host,''),coalesce(i.endpoint_port,0),i.current_revision_id,i.last_applied_revision_id,i.created_at,i.updated_at from instances i join service_definitions sd on sd.id=i.service_definition_id where i.id=$1`, instanceID).Scan(&x.ID, &x.NodeID, &x.ServiceCode, &x.Name, &x.Slug, &x.SystemdUnit, &x.Status, &x.Enabled, &x.EndpointHost, &x.EndpointPort, &x.CurrentRevisionID, &x.LastAppliedRevisionID, &x.CreatedAt, &x.UpdatedAt)
	return x, err
}

func (s *Store) FindNodeInstanceByService(ctx context.Context, nodeID, serviceCode string) (domain.Instance, error) {
	serviceCode = normalizeInstanceRuntimeCode(serviceCode)
	var x domain.Instance
	err := s.db.QueryRow(ctx, `select i.id,i.node_id,sd.code,i.name,i.slug,coalesce(i.systemd_unit,''),i.status,i.enabled,coalesce(i.endpoint_host,''),coalesce(i.endpoint_port,0),i.current_revision_id,i.last_applied_revision_id,i.created_at,i.updated_at
		from instances i
		join service_definitions sd on sd.id=i.service_definition_id
		where i.node_id=$1 and sd.code=$2 and i.status <> 'deleted'
		order by i.enabled desc, i.created_at asc
		limit 1`, nodeID, serviceCode).
		Scan(&x.ID, &x.NodeID, &x.ServiceCode, &x.Name, &x.Slug, &x.SystemdUnit, &x.Status, &x.Enabled, &x.EndpointHost, &x.EndpointPort, &x.CurrentRevisionID, &x.LastAppliedRevisionID, &x.CreatedAt, &x.UpdatedAt)
	return x, err
}

func (s *Store) CreateInstance(ctx context.Context, x domain.Instance) (domain.Instance, error) {
	return s.createInstanceWithOptions(ctx, x, createInstanceOptions{queueInitialApply: true, requireApplyReadyRevision: true})
}

func (s *Store) CreateInstanceDraft(ctx context.Context, x domain.Instance) (domain.Instance, error) {
	return s.createInstanceWithOptions(ctx, x, createInstanceOptions{queueInitialApply: false})
}

func (s *Store) CreateInstanceValidatedDraft(ctx context.Context, x domain.Instance) (domain.Instance, error) {
	return s.createInstanceWithOptions(ctx, x, createInstanceOptions{queueInitialApply: false, requireApplyReadyRevision: true})
}

type createInstanceOptions struct {
	queueInitialApply         bool
	requireApplyReadyRevision bool
}

func (s *Store) createInstanceWithOptions(ctx context.Context, x domain.Instance, opts createInstanceOptions) (domain.Instance, error) {
	if x.ID == "" {
		x.ID = id.New()
	}
	x.ServiceCode = normalizeInstanceRuntimeCode(x.ServiceCode)
	now := time.Now().UTC()
	x.CreatedAt, x.UpdatedAt = now, now
	if x.Status == "" {
		x.Status = "draft"
	}
	x.Enabled = true
	if x.Slug == "" {
		x.Slug = slugify(x.Name)
	}
	if x.ServiceCode == driver.WireGuard {
		if x.Spec == nil {
			x.Spec = map[string]any{}
		}
		if firstString(x.Spec["interface_name"]) == "" {
			x.Spec["interface_name"] = driver.WireGuardInterfaceName(firstString(x.Slug, x.Name, x.ID))
		}
	}
	if x.SystemdUnit == "" {
		x.SystemdUnit = serviceDefaultSystemdUnit(x.ServiceCode, x.Slug)
		if x.ServiceCode == driver.WireGuard {
			x.SystemdUnit = "wg-quick@" + driver.WireGuardInterfaceName(firstString(x.Spec["interface_name"], x.Slug, x.Name, x.ID))
		}
	}
	_, err := s.db.Exec(ctx, `insert into instances(id,node_id,service_definition_id,name,slug,systemd_unit,status,enabled,endpoint_host,endpoint_port,created_at,updated_at) values($1,$2,(select id from service_definitions where code=$3),$4,$5,$6,$7,$8,$9,nullif($10,0),$11,$12)`, x.ID, x.NodeID, x.ServiceCode, x.Name, x.Slug, x.SystemdUnit, x.Status, x.Enabled, x.EndpointHost, x.EndpointPort, x.CreatedAt, x.UpdatedAt)
	if err != nil {
		return x, err
	}
	spec := x.Spec
	if spec == nil {
		spec = map[string]any{}
	}
	spec["service_code"] = x.ServiceCode
	spec["name"] = x.Name
	spec["endpoint_host"] = x.EndpointHost
	spec["endpoint_port"] = x.EndpointPort
	rev, err := s.createInstanceRevision(ctx, x.ID, "system", "validated", spec)
	if err != nil {
		if cleanupErr := s.discardUnqueuedInstanceDraft(ctx, x.ID); cleanupErr != nil {
			return x, errors.Join(err, fmt.Errorf("discard invalid instance draft: %w", cleanupErr))
		}
		return x, err
	}
	if opts.requireApplyReadyRevision && !in(rev.Status, "validated", "applied") {
		applyReadyErr := initialRevisionNotApplyReadyError(rev)
		if cleanupErr := s.discardUnqueuedInstanceDraft(ctx, x.ID); cleanupErr != nil {
			return x, errors.Join(applyReadyErr, fmt.Errorf("discard unqueueable instance draft: %w", cleanupErr))
		}
		return x, applyReadyErr
	}
	if x.ServiceCode == "xray-core" {
		if materialized, err := s.materializeGlobalVLESSGroupMembershipsForInstance(ctx, x.ID); err != nil {
			if cleanupErr := s.discardUnqueuedInstanceDraft(ctx, x.ID); cleanupErr != nil {
				return x, errors.Join(err, fmt.Errorf("discard unqueueable instance draft: %w", cleanupErr))
			}
			return x, fmt.Errorf("materialize global vless group memberships: %w", err)
		} else if materialized > 0 {
			_, _ = s.CreateAudit(ctx, "system", "vless_group_members.materialize", "instance", &x.ID, fmt.Sprintf("global vless group memberships materialized before initial apply: %d", materialized))
		}
	}
	if opts.queueInitialApply {
		payload, payloadErr := s.buildInstanceJobPayload(ctx, x, "apply")
		if payloadErr != nil {
			if cleanupErr := s.discardUnqueuedInstanceDraft(ctx, x.ID); cleanupErr != nil {
				return x, errors.Join(payloadErr, fmt.Errorf("discard unqueueable instance draft: %w", cleanupErr))
			}
			return x, fmt.Errorf("initial instance apply payload is not queueable: %w", payloadErr)
		}
		job, err := s.CreateJob(ctx, domain.Job{Type: "instance.apply", ScopeType: "instance", ScopeID: &x.ID, NodeID: &x.NodeID, InstanceID: &x.ID, Payload: payload})
		if err != nil {
			if cleanupErr := s.discardUnqueuedInstanceDraft(ctx, x.ID); cleanupErr != nil {
				return x, errors.Join(err, fmt.Errorf("discard unqueued instance draft: %w", cleanupErr))
			}
			return x, fmt.Errorf("initial instance apply queue failed: %w", err)
		}
		if _, err := s.db.Exec(ctx, `update instances set status='provisioning',updated_at=now() where id=$1 and status in ('draft','validated','failed')`, x.ID); err != nil {
			if cleanupErr := s.discardUnqueuedInstanceDraft(ctx, x.ID); cleanupErr != nil {
				return x, errors.Join(err, fmt.Errorf("discard unqueued instance draft: %w", cleanupErr))
			}
			return x, fmt.Errorf("mark instance provisioning: %w", err)
		}
		x.Status = "provisioning"
		_ = s.upsertInstanceRuntimeStateForQueuedJob(ctx, x.ID, job)
		_ = s.AddJobLog(ctx, job.ID, "info", "initial instance apply queued", map[string]any{"instance_id": x.ID, "service_code": x.ServiceCode})
		_, _ = s.CreateAudit(ctx, "system", "instance.apply", "instance", &x.ID, "initial instance apply queued")
	}
	_, _ = s.CreateAudit(ctx, "system", "instance.create", "instance", &x.ID, "instance created")
	return x, nil
}

func (s *Store) DiscardInstanceDraft(ctx context.Context, instanceID string) error {
	if strings.TrimSpace(instanceID) == "" {
		return fmt.Errorf("instance id is required")
	}
	var status string
	if err := s.db.QueryRow(ctx, `select status from instances where id=$1`, instanceID).Scan(&status); err != nil {
		return err
	}
	if strings.TrimSpace(status) != "draft" {
		return fmt.Errorf("instance %s is not discardable; status=%s", instanceID, strings.TrimSpace(status))
	}
	var refs int
	if err := s.db.QueryRow(ctx, `select count(*) from service_accesses where instance_id=$1`, instanceID).Scan(&refs); err != nil {
		return err
	}
	if refs > 0 {
		return fmt.Errorf("instance %s is not discardable; service accesses exist", instanceID)
	}
	if err := s.db.QueryRow(ctx, `select count(*) from jobs where instance_id=$1`, instanceID).Scan(&refs); err != nil {
		return err
	}
	if refs > 0 {
		return fmt.Errorf("instance %s is not discardable; jobs exist", instanceID)
	}
	return s.discardUnqueuedInstanceDraft(ctx, instanceID)
}

func (s *Store) discardUnqueuedInstanceDraft(ctx context.Context, instanceID string) error {
	if strings.TrimSpace(instanceID) == "" {
		return nil
	}
	s.releaseInstanceAddressPoolAllocations(ctx, instanceID)
	if _, err := s.db.Exec(ctx, `delete from instance_runtime_observations where instance_id=$1`, instanceID); err != nil {
		return err
	}
	if _, err := s.db.Exec(ctx, `delete from instance_runtime_states where instance_id=$1`, instanceID); err != nil {
		return err
	}
	if _, err := s.db.Exec(ctx, `delete from instance_revisions where instance_id=$1`, instanceID); err != nil {
		return err
	}
	if _, err := s.db.Exec(ctx, `delete from instances where id=$1`, instanceID); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx, `delete from secret_refs
		where meta_json->>'scope'='instance'
		  and meta_json->>'instance_id'=$1`, instanceID)
	return err
}

func initialRevisionNotApplyReadyError(rev domain.InstanceRevision) error {
	status := strings.TrimSpace(rev.Status)
	if status == "" {
		status = "unknown"
	}
	messages := revisionValidationMessages(rev.ValidationErrors)
	if len(messages) == 0 {
		return fmt.Errorf("initial instance revision is not apply-ready; status=%s", status)
	}
	return fmt.Errorf("initial instance revision is not apply-ready; status=%s; validation: %s", status, strings.Join(messages, "; "))
}

func revisionValidationMessages(items []any) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		switch typed := item.(type) {
		case map[string]any:
			stage := strings.TrimSpace(stringify(typed["stage"]))
			message := strings.TrimSpace(stringify(typed["message"]))
			if message == "" {
				message = strings.TrimSpace(stringify(typed["error"]))
			}
			if message == "" {
				continue
			}
			if stage != "" {
				out = append(out, stage+": "+message)
			} else {
				out = append(out, message)
			}
		case string:
			if msg := strings.TrimSpace(typed); msg != "" {
				out = append(out, msg)
			}
		default:
			if msg := strings.TrimSpace(stringify(typed)); msg != "" {
				out = append(out, msg)
			}
		}
		if len(out) >= 4 {
			break
		}
	}
	return out
}

func (s *Store) UpdateInstanceStatus(ctx context.Context, instanceID, action string) (domain.Job, error) {
	x, err := s.GetInstance(ctx, instanceID)
	if err != nil {
		return domain.Job{}, err
	}
	action = driver.NormalizeOperation(action)
	op, ok := driver.OperationFor(x.ServiceCode, action)
	if !ok || op.JobType == "" || !op.AgentExecutable {
		return domain.Job{}, fmt.Errorf("unsupported instance action %q for service %s", action, x.ServiceCode)
	}
	jobType := op.JobType
	status := x.Status
	enabled := x.Enabled
	if op.QueuedStatus != "" {
		status = op.QueuedStatus
	}
	if op.SetsEnabled {
		enabled = op.QueuedEnabled
	}
	x.Status = status
	x.Enabled = enabled
	if action == driver.OperationApply {
		currentRevisionStatus := "draft"
		err = s.db.QueryRow(ctx, `select coalesce((select status from instance_revisions where id=i.current_revision_id), '') from instances i where i.id=$1`, instanceID).Scan(&currentRevisionStatus)
		if err != nil {
			return domain.Job{}, err
		}
		if !in(strings.TrimSpace(currentRevisionStatus), "validated", "applied") {
			return domain.Job{}, fmt.Errorf("current revision is not apply-ready; status=%s", strings.TrimSpace(currentRevisionStatus))
		}
		installNeeded, err := s.runtimeCapabilityInstallNeeded(ctx, x)
		if err != nil {
			return domain.Job{}, err
		}
		if installNeeded {
			job, err := s.CreateNodeCapabilityInstallJobWithDependents(ctx, x.NodeID, x.ServiceCode, "", "", []string{x.ID})
			if err != nil {
				return domain.Job{}, err
			}
			if _, err := s.db.Exec(ctx, `update instances set status=$2,enabled=$3,updated_at=now() where id=$1`, instanceID, status, enabled); err != nil {
				return domain.Job{}, err
			}
			_ = s.AddJobLog(ctx, job.ID, "info", "runtime install queued before instance apply", map[string]any{"instance_id": x.ID, "service_code": x.ServiceCode})
			_, _ = s.CreateAudit(ctx, "system", "node.capability.install", "instance", &instanceID, "runtime install queued before instance apply")
			return job, nil
		}
	}
	payload, payloadErr := s.buildInstanceJobPayload(ctx, x, action)
	if payloadErr != nil {
		if action == driver.OperationApply {
			return domain.Job{}, payloadErr
		}
		payload = map[string]any{"instance_id": instanceID, "action": action, "service_code": x.ServiceCode, "systemd_unit": x.SystemdUnit}
	}
	_, err = s.db.Exec(ctx, `update instances set status=$2,enabled=$3,updated_at=now() where id=$1`, instanceID, status, enabled)
	if err != nil {
		return domain.Job{}, err
	}
	j, err := s.CreateJob(ctx, domain.Job{Type: jobType, ScopeType: "instance", ScopeID: &instanceID, NodeID: &x.NodeID, InstanceID: &instanceID, Payload: payload})
	if err == nil {
		_ = s.upsertInstanceRuntimeStateForQueuedJob(ctx, instanceID, j)
	}
	_, _ = s.CreateAudit(ctx, "system", jobType, "instance", &instanceID, "instance action queued")
	return j, err
}

func (s *Store) CreateInstanceDiagnosticsJob(ctx context.Context, instanceID string) (domain.Job, error) {
	x, err := s.GetInstance(ctx, instanceID)
	if err != nil {
		return domain.Job{}, err
	}
	payload, payloadErr := s.buildInstanceDeleteJobPayload(ctx, x)
	if payloadErr != nil {
		payload = map[string]any{
			"instance_id":          x.ID,
			"action":               "diagnose",
			"service_code":         x.ServiceCode,
			"runtime_service_code": normalizeInstanceRuntimeCode(x.ServiceCode),
			"name":                 x.Name,
			"slug":                 x.Slug,
			"systemd_unit":         x.SystemdUnit,
			"endpoint_host":        x.EndpointHost,
			"endpoint_port":        x.EndpointPort,
			"enabled":              x.Enabled,
			"spec":                 map[string]any{"render_error": payloadErr.Error()},
		}
	}
	payload["action"] = "diagnose"
	payload["diagnostics"] = true
	j, err := s.CreateJob(ctx, domain.Job{
		Type:       "instance.diagnose",
		ScopeType:  "instance",
		ScopeID:    &instanceID,
		NodeID:     &x.NodeID,
		InstanceID: &instanceID,
		Priority:   40,
		Payload:    payload,
	})
	if err != nil {
		return j, err
	}
	_ = s.AddJobLog(ctx, j.ID, "info", "instance diagnostics queued", map[string]any{"instance_id": x.ID, "service_code": x.ServiceCode})
	_, _ = s.CreateAudit(ctx, "system", "instance.diagnose", "instance", &instanceID, "instance diagnostics queued")
	return j, nil
}

func (s *Store) runtimeCapabilityInstallNeeded(ctx context.Context, instance domain.Instance) (bool, error) {
	capCode := runtimeCapabilityCodeForService(instance.ServiceCode)
	if capCode == "" || defaultCapabilityInstallStrategy(capCode) == "" {
		return false, nil
	}
	var status string
	err := s.db.QueryRow(ctx, `select coalesce(status,'') from node_capabilities where node_id=$1 and capability_code=$2`, instance.NodeID, capCode).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "available", "installed", "detected":
		return false, nil
	default:
		return true, nil
	}
}

func (s *Store) DeleteInstance(ctx context.Context, instanceID string) (domain.Instance, error) {
	x, err := s.GetInstance(ctx, instanceID)
	if err != nil {
		return domain.Instance{}, err
	}
	if in(strings.TrimSpace(x.Status), "deleted", "deleting") {
		return x, nil
	}
	payload, payloadErr := s.buildInstanceDeleteJobPayload(ctx, x)
	if payloadErr != nil {
		return domain.Instance{}, payloadErr
	}
	_, err = s.db.Exec(ctx, `update instances set status='deleting',enabled=false,updated_at=now() where id=$1`, instanceID)
	if err != nil {
		return domain.Instance{}, err
	}
	job, err := s.CreateJob(ctx, domain.Job{Type: "instance.delete", ScopeType: "instance", ScopeID: &x.ID, NodeID: &x.NodeID, InstanceID: &x.ID, Payload: payload})
	if err != nil {
		if _, rollbackErr := s.db.Exec(ctx, `update instances set status=$2,enabled=$3,updated_at=now() where id=$1`, instanceID, x.Status, x.Enabled); rollbackErr != nil {
			return domain.Instance{}, errors.Join(err, fmt.Errorf("rollback instance delete state: %w", rollbackErr))
		}
		return domain.Instance{}, err
	}
	_ = s.upsertInstanceRuntimeStateForQueuedJob(ctx, instanceID, job)
	_ = s.AddJobLog(ctx, job.ID, "info", "instance cleanup queued", map[string]any{"instance_id": x.ID, "service_code": x.ServiceCode})
	_, _ = s.CreateAudit(ctx, "system", "instance.delete", "instance", &instanceID, "instance cleanup queued")
	return s.GetInstance(ctx, instanceID)
}

func (s *Store) ListClients(ctx context.Context) ([]domain.Client, error) {
	rows, err := s.db.Query(ctx, `select
		ca.id,
		ca.username,
		coalesce(ca.display_name,''),
		coalesce(ca.email,''),
		ca.status,
		coalesce(ca.notes,''),
		ca.expires_at,
		ca.created_at,
		ca.updated_at,
		coalesce(sa.total_count,0),
		coalesce(sa.active_count,0),
		coalesce(sa.pending_count,0),
		coalesce(car.total_count,0),
		coalesce(car.active_count,0),
		coalesce(ar.total_count,0),
		coalesce(ar.ready_count,0),
		ar.last_created_at,
		coalesce(sl.total_count,0),
		coalesce(sl.active_count,0),
		sl.next_expires_at
	from client_accounts ca
	left join lateral (
		select
			count(*) as total_count,
			count(*) filter (where status='active') as active_count,
			count(*) filter (where status='pending') as pending_count
		from service_accesses
		where client_account_id=ca.id and status <> 'revoked'
	) sa on true
	left join lateral (
		select
			count(*) as total_count,
			count(*) filter (where status='active') as active_count
		from client_access_routes
		where client_account_id=ca.id and status <> 'revoked'
	) car on true
	left join lateral (
		select
			count(*) as total_count,
			count(*) filter (where status='ready') as ready_count,
			max(created_at) as last_created_at
		from artifacts
		where client_account_id=ca.id
	) ar on true
	left join lateral (
		select
			count(*) as total_count,
			count(*) filter (where status='active' and expires_at > now()) as active_count,
			min(expires_at) filter (where status='active' and expires_at > now()) as next_expires_at
		from share_links
		where client_account_id=ca.id and status <> 'revoked'
	) sl on true
	where ca.status <> 'deleted'
	order by ca.created_at desc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Client
	for rows.Next() {
		var c domain.Client
		var summary domain.ClientSummary
		var serviceAccessCount, activeServiceAccessCount, pendingServiceAccessCount int64
		var routeCount, activeRouteCount int64
		var artifactCount, readyArtifactCount int64
		var shareLinkCount, activeShareLinkCount int64
		if err := rows.Scan(
			&c.ID,
			&c.Username,
			&c.DisplayName,
			&c.Email,
			&c.Status,
			&c.Notes,
			&c.ExpiresAt,
			&c.CreatedAt,
			&c.UpdatedAt,
			&serviceAccessCount,
			&activeServiceAccessCount,
			&pendingServiceAccessCount,
			&routeCount,
			&activeRouteCount,
			&artifactCount,
			&readyArtifactCount,
			&summary.LastArtifactAt,
			&shareLinkCount,
			&activeShareLinkCount,
			&summary.NextShareLinkExpiresAt,
		); err != nil {
			return nil, err
		}
		summary.ServiceAccessCount = int(serviceAccessCount)
		summary.ActiveServiceAccessCount = int(activeServiceAccessCount)
		summary.PendingServiceAccessCount = int(pendingServiceAccessCount)
		summary.RouteCount = int(routeCount)
		summary.ActiveRouteCount = int(activeRouteCount)
		summary.ArtifactCount = int(artifactCount)
		summary.ReadyArtifactCount = int(readyArtifactCount)
		summary.ShareLinkCount = int(shareLinkCount)
		summary.ActiveShareLinkCount = int(activeShareLinkCount)
		c.Summary = &summary
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) GetClient(ctx context.Context, clientID string) (domain.Client, error) {
	var c domain.Client
	err := s.db.QueryRow(ctx, `select id,username,coalesce(display_name,''),coalesce(email,''),status,coalesce(notes,''),expires_at,created_at,updated_at from client_accounts where id=$1`, clientID).Scan(&c.ID, &c.Username, &c.DisplayName, &c.Email, &c.Status, &c.Notes, &c.ExpiresAt, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

func (s *Store) CreateClient(ctx context.Context, c domain.Client) (domain.Client, error) {
	if c.ID == "" {
		c.ID = id.New()
	}
	now := time.Now().UTC()
	c.CreatedAt, c.UpdatedAt = now, now
	if c.Status == "" {
		c.Status = "active"
	}
	_, err := s.db.Exec(ctx, `insert into client_accounts(id,username,display_name,email,status,notes,expires_at,created_at,updated_at) values($1,$2,$3,$4,$5,$6,$7,$8,$9)`, c.ID, c.Username, c.DisplayName, c.Email, c.Status, c.Notes, c.ExpiresAt, c.CreatedAt, c.UpdatedAt)
	if err != nil {
		return c, err
	}
	_, _ = s.CreateAudit(ctx, "system", "client.create", "client", &c.ID, "client created")
	return c, nil
}

func (s *Store) SetClientStatus(ctx context.Context, clientID, status string) (domain.Client, error) {
	if !in(status, "active", "suspended", "revoked", "expired", "deleted") {
		return domain.Client{}, fmt.Errorf("unsupported client status %q", status)
	}
	_, err := s.db.Exec(ctx, `update client_accounts set status=$2,updated_at=now() where id=$1`, clientID, status)
	if err != nil {
		return domain.Client{}, err
	}
	if status == "revoked" {
		if _, err := s.db.Exec(ctx, `update service_accesses set status='revoked',updated_at=now() where client_account_id=$1`, clientID); err != nil {
			return domain.Client{}, err
		}
		if _, err := s.db.Exec(ctx, `update client_access_routes set status='revoked',updated_at=now() where client_account_id=$1`, clientID); err != nil {
			return domain.Client{}, err
		}
		if _, err := s.db.Exec(ctx, `update share_links set status='revoked' where client_account_id=$1 and status='active'`, clientID); err != nil {
			return domain.Client{}, err
		}
		if _, err := s.db.Exec(ctx, `update client_subscriptions set status='revoked',updated_at=now() where client_account_id=$1 and status='active'`, clientID); err != nil {
			return domain.Client{}, err
		}
		if err := s.queueRoutePolicyJobsForClient(ctx, clientID); err != nil {
			return domain.Client{}, err
		}
	} else if status == "deleted" {
		if _, err := s.db.Exec(ctx, `update service_accesses set status='revoked',updated_at=now() where client_account_id=$1`, clientID); err != nil {
			return domain.Client{}, err
		}
		if _, err := s.db.Exec(ctx, `update client_access_routes set status='revoked',updated_at=now() where client_account_id=$1`, clientID); err != nil {
			return domain.Client{}, err
		}
		if _, err := s.db.Exec(ctx, `update share_links set status='revoked' where client_account_id=$1 and status='active'`, clientID); err != nil {
			return domain.Client{}, err
		}
		if _, err := s.db.Exec(ctx, `update client_subscriptions set status='revoked',updated_at=now() where client_account_id=$1 and status='active'`, clientID); err != nil {
			return domain.Client{}, err
		}
		if err := s.queueRoutePolicyJobsForClient(ctx, clientID); err != nil {
			return domain.Client{}, err
		}
	}
	_, _ = s.CreateAudit(ctx, "system", "client."+status, "client", &clientID, "client status changed")
	return s.GetClient(ctx, clientID)
}

func (s *Store) ProvisionClient(ctx context.Context, clientID string, instanceIDs []string) (domain.Job, error) {
	return s.ProvisionClientWithOptions(ctx, clientID, instanceIDs, nil)
}

func (s *Store) ProvisionClientWithOptions(ctx context.Context, clientID string, instanceIDs []string, serviceOptions map[string]map[string]any) (domain.Job, error) {
	instanceIDs, err := s.ensureClientProvisioningAccesses(ctx, clientID, instanceIDs, true, serviceOptions)
	if err != nil {
		return domain.Job{}, err
	}
	payload := map[string]any{"client_id": clientID, "instance_ids": instanceIDs}
	effectiveOptions := make(map[string]map[string]any)
	for _, instanceID := range instanceIDs {
		if options := serviceOptions[instanceID]; len(options) > 0 {
			effectiveOptions[instanceID] = options
		}
	}
	if len(effectiveOptions) > 0 {
		payload["service_options"] = effectiveOptions
	}
	j, err := s.CreateJob(ctx, domain.Job{Type: "client.provision", ScopeType: "client", ScopeID: &clientID, Payload: payload})
	_, _ = s.CreateAudit(ctx, "system", "client.provision", "client", &clientID, "client provisioning queued")
	return j, err
}

func (s *Store) CreateArtifactBuildJob(ctx context.Context, clientID, artifactType string, instanceIDs []string) (domain.Job, error) {
	instanceIDs, err := s.ensureClientProvisioningAccesses(ctx, clientID, instanceIDs, false, nil)
	if err != nil {
		return domain.Job{}, err
	}
	artifactType = strings.ToLower(strings.TrimSpace(artifactType))
	if artifactType == "" {
		artifactType = "all"
	}
	j, err := s.CreateJob(ctx, domain.Job{
		Type:      "artifact.build",
		ScopeType: "client",
		ScopeID:   &clientID,
		Payload: map[string]any{
			"client_id":     clientID,
			"instance_ids":  instanceIDs,
			"artifact_type": artifactType,
		},
	})
	_, _ = s.CreateAudit(ctx, "system", "artifact.build", "client", &clientID, "artifact build queued")
	return j, err
}

func (s *Store) ensureClientProvisioningAccesses(ctx context.Context, clientID string, instanceIDs []string, markPending bool, serviceOptions map[string]map[string]any) ([]string, error) {
	if _, err := s.GetClient(ctx, clientID); err != nil {
		return nil, err
	}
	if len(instanceIDs) == 0 {
		rows, err := s.db.Query(ctx, `select i.id from instances i join service_definitions sd on sd.id=i.service_definition_id where i.enabled=true and i.status in ('active','draft','provisioning') and sd.supports_accounts=true order by i.created_at asc`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var iid string
			if err := rows.Scan(&iid); err != nil {
				return nil, err
			}
			instanceIDs = append(instanceIDs, iid)
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
	} else {
		normalized, err := s.validateClientProvisioningInstanceIDs(ctx, instanceIDs)
		if err != nil {
			return nil, err
		}
		instanceIDs = normalized
	}
	if len(instanceIDs) == 0 {
		return nil, fmt.Errorf("at least one provisionable service instance is required")
	}
	for _, iid := range instanceIDs {
		var accessID string
		metadata, err := s.clientProvisioningServiceMetadata(ctx, clientID, iid, serviceOptions[iid])
		if err != nil {
			return nil, err
		}
		err = s.db.QueryRow(ctx, `insert into service_accesses(id,client_account_id,instance_id,status,provision_mode,metadata_json,created_at,updated_at)
			values($1,$2,$3,'pending','manual',$5,now(),now())
			on conflict(client_account_id, instance_id) do update set
				status=case when $4::boolean then 'pending' else service_accesses.status end,
				metadata_json=service_accesses.metadata_json || excluded.metadata_json,
				updated_at=now()
			returning id`, id.New(), clientID, iid, markPending, mustJSON(metadata)).Scan(&accessID)
		if err != nil {
			return nil, err
		}
		if err := s.ensureBaselineClientAccessRoute(ctx, clientID, accessID, iid); err != nil {
			return nil, err
		}
	}
	return instanceIDs, nil
}

func (s *Store) validateClientProvisioningInstanceIDs(ctx context.Context, instanceIDs []string) ([]string, error) {
	seen := make(map[string]struct{}, len(instanceIDs))
	out := make([]string, 0, len(instanceIDs))
	for _, raw := range instanceIDs {
		instanceID := strings.TrimSpace(raw)
		if instanceID == "" {
			continue
		}
		if _, ok := seen[instanceID]; ok {
			continue
		}
		seen[instanceID] = struct{}{}

		var enabled, supportsAccounts bool
		var status, serviceCode string
		err := s.db.QueryRow(ctx, `select i.enabled,i.status,sd.code,sd.supports_accounts
			from instances i
			join service_definitions sd on sd.id=i.service_definition_id
			where i.id=$1`, instanceID).Scan(&enabled, &status, &serviceCode, &supportsAccounts)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, fmt.Errorf("service instance %s not found", instanceID)
			}
			return nil, err
		}
		if !enabled || strings.TrimSpace(status) == "deleted" {
			return nil, fmt.Errorf("service instance %s is not available for client provisioning", instanceID)
		}
		if !supportsAccounts {
			return nil, fmt.Errorf("service %s does not support client provisioning", strings.TrimSpace(serviceCode))
		}
		out = append(out, instanceID)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("at least one service instance is required")
	}
	return out, nil
}

func (s *Store) RevokeClient(ctx context.Context, clientID string) (domain.Job, error) {
	if _, err := s.GetClient(ctx, clientID); err != nil {
		return domain.Job{}, err
	}
	if _, err := s.db.Exec(ctx, `update client_accounts set status='revoked',updated_at=now() where id=$1`, clientID); err != nil {
		return domain.Job{}, err
	}
	if _, err := s.db.Exec(ctx, `update service_accesses set status='revoked',updated_at=now() where client_account_id=$1`, clientID); err != nil {
		return domain.Job{}, err
	}
	if _, err := s.db.Exec(ctx, `update client_access_routes set status='revoked',updated_at=now() where client_account_id=$1`, clientID); err != nil {
		return domain.Job{}, err
	}
	if _, err := s.db.Exec(ctx, `update share_links set status='revoked' where client_account_id=$1 and status='active'`, clientID); err != nil {
		return domain.Job{}, err
	}
	if _, err := s.db.Exec(ctx, `update client_subscriptions set status='revoked',updated_at=now() where client_account_id=$1 and status='active'`, clientID); err != nil {
		return domain.Job{}, err
	}
	if err := s.queueRoutePolicyJobsForClient(ctx, clientID); err != nil {
		return domain.Job{}, err
	}
	j, err := s.CreateJob(ctx, domain.Job{Type: "client.revoke", ScopeType: "client", ScopeID: &clientID, Payload: map[string]any{"client_id": clientID}})
	_, _ = s.CreateAudit(ctx, "system", "client.revoke", "client", &clientID, "client revoke queued")
	return j, err
}

func (s *Store) ListServiceAccesses(ctx context.Context, clientID string) ([]domain.ServiceAccess, error) {
	rows, err := s.db.Query(ctx, `select id,client_account_id,instance_id,status,provision_mode,policy_json,metadata_json,created_at,updated_at from service_accesses where client_account_id=$1 order by created_at desc`, clientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.ServiceAccess
	for rows.Next() {
		var a domain.ServiceAccess
		var p, m []byte
		if err := rows.Scan(&a.ID, &a.ClientAccountID, &a.InstanceID, &a.Status, &a.ProvisionMode, &p, &m, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		if err := decodeJSONField(p, &a.Policy, "service_accesses.policy_json"); err != nil {
			return nil, err
		}
		if err := decodeJSONField(m, &a.Metadata, "service_accesses.metadata_json"); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) ListArtifacts(ctx context.Context, clientID string) ([]domain.Artifact, error) {
	query := `select id,client_account_id,service_access_id,artifact_type,storage_path,coalesce(content_hash,''),coalesce(size_bytes,0),status,created_at from artifacts`
	args := []any{}
	if clientID != "" {
		query += ` where client_account_id=$1`
		args = append(args, clientID)
	}
	query += ` order by created_at desc`
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Artifact
	for rows.Next() {
		var a domain.Artifact
		if err := rows.Scan(&a.ID, &a.ClientAccountID, &a.ServiceAccessID, &a.ArtifactType, &a.StoragePath, &a.ContentHash, &a.SizeBytes, &a.Status, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) GetArtifact(ctx context.Context, clientID, artifactID string) (domain.Artifact, error) {
	clientID = strings.TrimSpace(clientID)
	artifactID = strings.TrimSpace(artifactID)
	if artifactID == "" {
		return domain.Artifact{}, fmt.Errorf("artifact id is required")
	}
	query := `select id,client_account_id,service_access_id,artifact_type,storage_path,coalesce(content_hash,''),coalesce(size_bytes,0),status,created_at from artifacts where id=$1`
	args := []any{artifactID}
	if clientID != "" {
		query += ` and client_account_id=$2`
		args = append(args, clientID)
	}
	var a domain.Artifact
	if err := s.db.QueryRow(ctx, query, args...).Scan(&a.ID, &a.ClientAccountID, &a.ServiceAccessID, &a.ArtifactType, &a.StoragePath, &a.ContentHash, &a.SizeBytes, &a.Status, &a.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Artifact{}, fmt.Errorf("artifact not found")
		}
		return domain.Artifact{}, err
	}
	return a, nil
}

func (s *Store) PublishShareLink(ctx context.Context, clientID string, targetID string, ttl time.Duration) (domain.ShareLink, error) {
	if strings.TrimSpace(clientID) == "" {
		return domain.ShareLink{}, fmt.Errorf("client id is required")
	}
	if ttl <= 0 {
		ttl = 72 * time.Hour
	}
	if targetID == "" {
		arts, err := s.ListArtifacts(ctx, clientID)
		if err != nil {
			return domain.ShareLink{}, err
		}
		if len(arts) == 0 {
			return domain.ShareLink{}, fmt.Errorf("no client artifacts found; run provisioning or artifact export first")
		} else {
			targetID = arts[0].ID
		}
	}
	var verifiedTargetID string
	var verifiedStatus string
	if err := s.db.QueryRow(ctx, `select id,status from artifacts where id=$1 and client_account_id=$2`, targetID, clientID).Scan(&verifiedTargetID, &verifiedStatus); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ShareLink{}, fmt.Errorf("artifact does not belong to the selected client")
		}
		return domain.ShareLink{}, err
	}
	if strings.TrimSpace(verifiedStatus) != "ready" {
		return domain.ShareLink{}, fmt.Errorf("artifact %s is not ready for publishing", verifiedTargetID)
	}
	token := randomToken(32)
	link := domain.ShareLink{ID: id.New(), ClientAccountID: clientID, TargetType: "artifact", TargetID: targetID, Token: token, TokenHint: tokenHint(token), Status: "active", ExpiresAt: time.Now().UTC().Add(ttl), CreatedAt: time.Now().UTC()}
	_, err := s.db.Exec(ctx, `insert into share_links(id,client_account_id,target_type,target_id,token_hash,token_hint,status,expires_at,created_at) values($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		link.ID, link.ClientAccountID, link.TargetType, link.TargetID, hashToken(token), link.TokenHint, link.Status, link.ExpiresAt, link.CreatedAt)
	if err != nil {
		return link, err
	}
	_, _ = s.CreateAudit(ctx, "system", "share_link.publish", "share_link", &link.ID, "share link published")
	return link, nil
}

func (s *Store) ListShareLinks(ctx context.Context, clientID string) ([]domain.ShareLink, error) {
	query := `select id,client_account_id,target_type,target_id,coalesce(token_hint,''),status,expires_at,download_count,created_at from share_links`
	args := []any{}
	if clientID != "" {
		query += ` where client_account_id=$1`
		args = append(args, clientID)
	}
	query += ` order by created_at desc`
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.ShareLink
	for rows.Next() {
		var l domain.ShareLink
		if err := rows.Scan(&l.ID, &l.ClientAccountID, &l.TargetType, &l.TargetID, &l.TokenHint, &l.Status, &l.ExpiresAt, &l.DownloadCount, &l.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *Store) RevokeShareLink(ctx context.Context, clientID, linkID string) (domain.ShareLink, error) {
	if strings.TrimSpace(clientID) == "" || strings.TrimSpace(linkID) == "" {
		return domain.ShareLink{}, fmt.Errorf("client id and link id are required")
	}
	var link domain.ShareLink
	err := s.db.QueryRow(ctx, `update share_links
		set status='revoked'
		where id=$1 and client_account_id=$2 and status in ('active','expired')
		returning id,client_account_id,target_type,target_id,coalesce(token_hint,''),status,expires_at,download_count,created_at`,
		linkID, clientID,
	).Scan(&link.ID, &link.ClientAccountID, &link.TargetType, &link.TargetID, &link.TokenHint, &link.Status, &link.ExpiresAt, &link.DownloadCount, &link.CreatedAt)
	if err != nil {
		return domain.ShareLink{}, err
	}
	_, _ = s.CreateAudit(ctx, "system", "share_link.revoke", "share_link", &link.ID, "share link revoked")
	return link, nil
}

func (s *Store) CreateJob(ctx context.Context, j domain.Job) (domain.Job, error) {
	j, payloadJSON, err := normalizeJobForInsert(j)
	if err != nil {
		return j, err
	}
	if err := insertJobRow(ctx, s.db, j, payloadJSON); err != nil {
		return j, err
	}
	_, _ = s.CreateAudit(ctx, "system", "job.create", "job", &j.ID, "job queued")
	return j, nil
}

type jobInsertExecutor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func normalizeJobForInsert(j domain.Job) (domain.Job, []byte, error) {
	j.Type = strings.TrimSpace(j.Type)
	if j.Type == "" {
		return j, nil, errors.New("job type is required")
	}
	if j.ID == "" {
		j.ID = id.New()
	}
	if j.Status == "" {
		j.Status = "queued"
	}
	if j.Status != "queued" {
		return j, nil, fmt.Errorf("new jobs must start queued")
	}
	if j.ScopeType == "" {
		j.ScopeType = "system"
	}
	if j.Priority == 0 {
		j.Priority = 100
	}
	if j.Payload == nil {
		j.Payload = map[string]any{}
	}
	normalizedPayload, err := jobschema.Normalize(j.Type, j.Payload)
	if err != nil {
		return j, nil, err
	}
	j.Payload = normalizedPayload
	b, err := json.Marshal(j.Payload)
	if err != nil {
		return j, nil, fmt.Errorf("marshal job payload: %w", err)
	}
	if j.CreatedAt.IsZero() {
		j.CreatedAt = time.Now().UTC()
	}
	return j, b, nil
}

func insertJobRow(ctx context.Context, db jobInsertExecutor, j domain.Job, payloadJSON []byte) error {
	_, err := db.Exec(ctx, `insert into jobs(id,type,scope_type,scope_id,node_id,instance_id,status,priority,payload_json,created_at) values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, j.ID, j.Type, j.ScopeType, j.ScopeID, j.NodeID, j.InstanceID, j.Status, j.Priority, payloadJSON, j.CreatedAt)
	return err
}

func (s *Store) ListJobs(ctx context.Context, limit int) ([]domain.Job, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if _, err := s.RecoverStaleJobLeases(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `select id,type,scope_type,scope_id,node_id,instance_id,status,priority,payload_json,coalesce(result_json,'{}'::jsonb),locked_by,locked_until,created_at,started_at,finished_at from jobs order by created_at desc limit $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.Job, 0)
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

func (s *Store) GetJob(ctx context.Context, jobID string) (domain.Job, error) {
	if _, err := s.RecoverStaleJobLeases(ctx); err != nil {
		return domain.Job{}, err
	}
	row := s.db.QueryRow(ctx, `select id,type,scope_type,scope_id,node_id,instance_id,status,priority,payload_json,coalesce(result_json,'{}'::jsonb),locked_by,locked_until,created_at,started_at,finished_at from jobs where id=$1`, jobID)
	return scanJob(row)
}

func (s *Store) ListJobLogs(ctx context.Context, jobID string, limit int) ([]domain.JobLog, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := s.db.Query(ctx, `select id,job_id,level,message,payload_json,created_at from job_logs where job_id=$1 order by created_at asc limit $2`, jobID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.JobLog, 0)
	for rows.Next() {
		var l domain.JobLog
		var b []byte
		if err := rows.Scan(&l.ID, &l.JobID, &l.Level, &l.Message, &b, &l.CreatedAt); err != nil {
			return nil, err
		}
		if err := decodeJSONField(b, &l.Payload, "job_logs.payload_json"); err != nil {
			return nil, err
		}
		l.Payload = redactSensitiveMapForStorage(l.Payload)
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *Store) CancelJob(ctx context.Context, jobID string) (domain.Job, error) {
	if _, err := s.RecoverStaleJobLeases(ctx); err != nil {
		return domain.Job{}, err
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.Job{}, err
	}
	defer tx.Rollback(ctx)
	var status string
	if err := tx.QueryRow(ctx, `select status from jobs where id=$1 for update`, jobID).Scan(&status); err != nil {
		return domain.Job{}, err
	}
	if !in(status, "queued", "retrying") {
		return domain.Job{}, fmt.Errorf("job %s cannot be cancelled from status %s", jobID, status)
	}
	_, err = tx.Exec(ctx, `update jobs set status='cancelled', finished_at=now(), locked_by=null, locked_until=null where id=$1`, jobID)
	if err != nil {
		return domain.Job{}, err
	}
	_, err = tx.Exec(ctx, `delete from resource_locks where job_id=$1`, jobID)
	if err != nil {
		return domain.Job{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.Job{}, err
	}
	_ = s.AddJobLog(ctx, jobID, "warn", "job cancelled", map[string]any{"previous_status": status})
	_, _ = s.CreateAudit(ctx, "system", "job.cancel", "job", &jobID, "job cancelled")
	return s.GetJob(ctx, jobID)
}

func (s *Store) ClaimJob(ctx context.Context, workerID string) (domain.Job, bool, error) {
	return s.claimJob(ctx, workerID, `type not in ('node.inventory','node.inventory.sync','node.services.discover','node.capability.install','node.capability.verify','node.channel.probe','node.agent.rotate_token','node.emergency_cleanup','node.reboot','node.backhaul.apply','node.backhaul.probe','node.backhaul.cleanup','node.route_policy.apply','node.route_policy.cleanup','node.firewall.preview','node.firewall.apply','node.firewall.observe','node.firewall.disable','instance.restart','instance.apply','instance.start','instance.stop','instance.enable','instance.disable','instance.diagnose','instance.delete')`, nil)
}

func (s *Store) RecoverStaleJobLeases(ctx context.Context) (int, error) {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	jobs, err := recoverStaleJobLeasesTx(ctx, tx, time.Now().UTC())
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	for _, job := range jobs {
		_ = s.AddJobLog(ctx, job.ID, "warn", "stale job lease recovered", map[string]any{
			"new_status":                    "retrying",
			"recovery_reason":               "running job exceeded its lease",
			"legacy_recovery_after":         legacyRunningJobRecoveryAfter.String(),
			"job_lease_duration":            jobLeaseDurationForType(job.Type).String(),
			"requires_agent_or_worker_poll": true,
		})
	}
	return len(jobs), nil
}

func (s *Store) CompleteJob(ctx context.Context, idv, status string, result map[string]any) error {
	return s.completeJob(ctx, idv, "", false, status, result)
}

func (s *Store) CompleteAgentJob(ctx context.Context, idv, owner, status string, result map[string]any) error {
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return errors.New("agent job owner is required")
	}
	return s.completeJob(ctx, idv, owner, true, status, result)
}

func (s *Store) completeJob(ctx context.Context, idv, owner string, requireLease bool, status string, result map[string]any) error {
	if !in(status, "succeeded", "failed", "cancelled") {
		return fmt.Errorf("unsupported completion status %q", status)
	}
	if result == nil {
		result = map[string]any{}
	}
	storedResult := redactSensitiveMapForStorage(result)
	b, err := json.Marshal(storedResult)
	if err != nil {
		storedResult = map[string]any{"message": "result marshal failed", "error": err.Error()}
		b, err = json.Marshal(storedResult)
		if err != nil {
			return fmt.Errorf("marshal fallback job result: %w", err)
		}
	}

	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var jobType string
	var scopeID *string
	var instanceID *string
	var currentStatus string
	var payloadRaw []byte
	var lockedBy *string
	var lockedUntil *time.Time
	if err := tx.QueryRow(ctx, `select type,scope_id,instance_id,status,payload_json,locked_by,locked_until from jobs where id=$1 for update`, idv).Scan(&jobType, &scopeID, &instanceID, &currentStatus, &payloadRaw, &lockedBy, &lockedUntil); err != nil {
		return err
	}
	if in(currentStatus, "succeeded", "failed", "cancelled") {
		return fmt.Errorf("job already finished with status %s", currentStatus)
	}
	if requireLease {
		now := time.Now().UTC()
		if currentStatus != "running" {
			return fmt.Errorf("job is not running")
		}
		if lockedBy == nil || strings.TrimSpace(*lockedBy) != owner {
			return fmt.Errorf("job lease owner mismatch")
		}
		if lockedUntil == nil || !lockedUntil.After(now) {
			return fmt.Errorf("job lease expired")
		}
	}
	payload := map[string]any{}
	if err := decodeJSONField(payloadRaw, &payload, "jobs.payload_json"); err != nil {
		return err
	}
	if payload == nil {
		payload = map[string]any{}
	}

	if _, err = tx.Exec(ctx, `update jobs set status=$2,result_json=$3,finished_at=now(),locked_by=null,locked_until=null where id=$1`, idv, status, b); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `delete from resource_locks where job_id=$1`, idv); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}

	if err := s.applyJobCompletionSideEffects(ctx, idv, jobType, status, scopeID, instanceID, payload, result, b); err != nil {
		_ = s.AddJobLog(ctx, idv, "error", "job completion side effect failed", map[string]any{"error": err.Error(), "job_type": jobType, "status": status})
	}

	_, _ = s.CreateAudit(ctx, "system", "job."+status, "job", &idv, "job finished")
	return nil
}

func (s *Store) applyJobCompletionSideEffects(ctx context.Context, idv, jobType, status string, scopeID, instanceID *string, payload, result map[string]any, resultJSON []byte) error {
	if strings.HasPrefix(jobType, "node.capability.") && scopeID != nil {
		capCode := normalizeCapabilityCode(stringify(payload["service_code"]))
		if capCode == "" {
			capCode = normalizeCapabilityCode(stringify(result["service_code"]))
		}
		strategy := stringify(payload["strategy"])
		if strategy == "" {
			strategy = stringify(result["strategy"])
		}
		summary := stringify(result["message"])
		if summary == "" {
			summary = "capability job finished"
		}
		if err := s.insertCapabilityInstallEvent(ctx, *scopeID, idv, capCode, strategy, status, summary, resultJSON); err != nil {
			return err
		}
		if jobType == "node.capability.install" {
			if err := s.markBinaryDownloadTicketUsedFromJobResult(ctx, idv, result); err != nil {
				_ = s.AddJobLog(ctx, idv, "warn", "binary download ticket was not marked used", map[string]any{"error": err.Error()})
			}
		}
		if capCode != "" {
			version := stringify(result["version"])
			capStatus := "available"
			if status == "failed" {
				capStatus = "failed"
			} else if status == "cancelled" {
				capStatus = "unknown"
			}
			if err := s.upsertNodeCapability(ctx, *scopeID, capCode, version, capStatus, "installer"); err != nil {
				return err
			}
			if status == "succeeded" {
				s.queueApplyJobsForCapabilityDependents(ctx, idv, *scopeID, capCode, payload)
			} else if jobType == "node.capability.install" && (status == "failed" || status == "cancelled") {
				if err := s.markCapabilityInstallFailedForDependents(ctx, idv, *scopeID, capCode, status, payload, result); err != nil {
					_ = s.AddJobLog(ctx, idv, "warn", "dependent runtime state was not updated after runtime install failure", map[string]any{"error": err.Error()})
				}
			}
		}
	}

	if strings.HasPrefix(jobType, "instance.") && jobType != "instance.diagnose" && instanceID != nil {
		return s.applyInstanceJobCompletionSideEffects(ctx, *instanceID, idv, jobType, status, result)
	}

	if strings.HasPrefix(jobType, "node.firewall.") {
		return s.ApplyFirewallJobResult(ctx, domain.Job{ID: idv, Type: jobType, ScopeID: scopeID, Payload: payload, Result: result}, status, result)
	}

	if status == "succeeded" {
		switch jobType {
		case "node.backhaul.apply":
			return s.ApplyBackhaulJobResult(ctx, domain.Job{ID: idv, Type: jobType, ScopeID: scopeID, NodeID: nil, Payload: payload, Result: result}, status, result)
		case "node.backhaul.probe":
			return s.ApplyBackhaulProbeResult(ctx, domain.Job{ID: idv, Type: jobType, ScopeID: scopeID, NodeID: nil, Payload: payload, Result: result}, status, result)
		case "node.backhaul.cleanup":
			return s.ApplyBackhaulCleanupResult(ctx, domain.Job{ID: idv, Type: jobType, ScopeID: scopeID, NodeID: nil, Payload: payload, Result: result}, status, result)
		case "node.bootstrap":
			if scopeID != nil && shouldHandoffNodeBootstrapToAgent(payload, result) {
				if _, err := s.db.Exec(ctx, `update nodes
					set status='bootstrapping',
					    execution_mode='agent_managed',
					    agent_status='starting',
					    updated_at=now()
					where id=$1 and status <> 'retired'`, *scopeID); err != nil {
					return err
				}
				jobs, warnings, err := s.QueueNodeRuntimeReconcile(ctx, *scopeID, "node.bootstrap")
				if err != nil {
					_ = s.AddJobLog(ctx, idv, "warn", "runtime reconcile was not queued after bootstrap", map[string]any{"error": err.Error()})
				} else {
					level := "info"
					if len(warnings) > 0 {
						level = "warn"
					}
					_ = s.AddJobLog(ctx, idv, level, "runtime reconcile queued after bootstrap", map[string]any{
						"queued_jobs": len(jobs),
						"warnings":    warnings,
					})
				}
			}
		case "node.agent.rotate_token":
			if scopeID != nil {
				newToken := stringify(payload["new_agent_token"])
				newTokenHash := stringify(payload["new_agent_token_hash"])
				newHint := stringify(payload["new_token_hint"])
				if newTokenHash == "" && newToken != "" {
					newTokenHash = hashToken(newToken)
				}
				if newTokenHash != "" {
					if _, err := s.db.Exec(ctx, `update node_agents
						set agent_token_hash=$2,
						    token_hint=$3,
						    status='active',
						    revoked_at=null
						where node_id=$1`, *scopeID, newTokenHash, newHint); err != nil {
						return err
					}
					if _, err := s.db.Exec(ctx, `update nodes set agent_status='online',updated_at=now() where id=$1`, *scopeID); err != nil {
						return err
					}
				}
			}
		case "node.emergency_cleanup":
			nodeID := ""
			if scopeID != nil {
				nodeID = *scopeID
			}
			if nodeID == "" {
				nodeID = strings.TrimSpace(stringify(payload["node_id"]))
			}
			if nodeID != "" {
				if err := s.applyNodeEmergencyCleanupResult(ctx, nodeID, payload); err != nil {
					return err
				}
			}
		case "client.revoke":
			if scopeID != nil {
				if _, err := s.db.Exec(ctx, `update client_accounts set status='revoked',updated_at=now() where id=$1`, *scopeID); err != nil {
					return err
				}
				if _, err := s.db.Exec(ctx, `update service_accesses set status='revoked',updated_at=now() where client_account_id=$1`, *scopeID); err != nil {
					return err
				}
				if _, err := s.db.Exec(ctx, `update client_access_routes set status='revoked',updated_at=now() where client_account_id=$1`, *scopeID); err != nil {
					return err
				}
				if _, err := s.db.Exec(ctx, `update client_subscriptions set status='revoked',updated_at=now() where client_account_id=$1 and status='active'`, *scopeID); err != nil {
					return err
				}
				if err := s.queueRoutePolicyJobsForClient(ctx, *scopeID); err != nil {
					return err
				}
			}
		case "platform.control_plane_tls.apply":
			if err := s.MarkControlPlaneTLSApplyResult(ctx, true, ""); err != nil {
				return err
			}
		}
	} else if strings.HasPrefix(jobType, "node.backhaul.") {
		switch jobType {
		case "node.backhaul.apply":
			return s.ApplyBackhaulJobResult(ctx, domain.Job{ID: idv, Type: jobType, ScopeID: scopeID, Payload: payload, Result: result}, status, result)
		case "node.backhaul.probe":
			return s.ApplyBackhaulProbeResult(ctx, domain.Job{ID: idv, Type: jobType, ScopeID: scopeID, Payload: payload, Result: result}, status, result)
		case "node.backhaul.cleanup":
			return s.ApplyBackhaulCleanupResult(ctx, domain.Job{ID: idv, Type: jobType, ScopeID: scopeID, Payload: payload, Result: result}, status, result)
		}
	} else if jobType == "platform.control_plane_tls.apply" {
		errText := strings.TrimSpace(stringify(result["error"]))
		if errText == "" {
			errText = strings.TrimSpace(stringify(result["message"]))
		}
		if errText == "" {
			errText = "control plane tls apply failed"
		}
		if err := s.MarkControlPlaneTLSApplyResult(ctx, false, errText); err != nil {
			return err
		}
	} else if jobType == "node.bootstrap" && scopeID != nil {
		if _, err := s.db.Exec(ctx, `update nodes set status='draft',updated_at=now() where id=$1 and status='bootstrapping'`, *scopeID); err != nil {
			return err
		}
	}

	if jobType == "node.bootstrap" {
		if _, err := s.db.Exec(ctx, `update node_bootstrap_runs set status=$2,result_payload_json=$3,finished_at=now() where job_id=$1`, idv, status, resultJSON); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) applyInstanceJobCompletionSideEffects(ctx context.Context, instanceID, jobID, jobType, status string, result map[string]any) error {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	routePolicyNeeded := false
	var artifactCleanupPaths []string
	if status == "succeeded" {
		switch jobType {
		case "instance.apply":
			if _, err := tx.Exec(ctx, `update instance_revisions
				set status='superseded'
				where id=(select last_applied_revision_id from instances where id=$1)
				  and id is distinct from (select current_revision_id from instances where id=$1)
				  and status='applied'`, instanceID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `update instance_revisions
				set status='applied',applied_at=now(),validation_errors_json='[]'::jsonb
				where id=(select current_revision_id from instances where id=$1)`, instanceID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `update instances set status='active',enabled=true,last_applied_revision_id=current_revision_id,updated_at=now() where id=$1`, instanceID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `update service_accesses set status='active',updated_at=now() where instance_id=$1 and status='pending'`, instanceID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `update client_access_routes set status='active',updated_at=now() where instance_id=$1 and status='pending'`, instanceID); err != nil {
				return err
			}
			routePolicyNeeded = true
		case "instance.start", "instance.enable":
			if _, err := tx.Exec(ctx, `update instances set status='active',enabled=true,updated_at=now() where id=$1`, instanceID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `update client_access_routes set status='active',updated_at=now() where instance_id=$1 and status='disabled'`, instanceID); err != nil {
				return err
			}
			routePolicyNeeded = true
		case "instance.stop", "instance.disable":
			if _, err := tx.Exec(ctx, `update instances set status='disabled',enabled=false,updated_at=now() where id=$1`, instanceID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `update client_access_routes set status='disabled',updated_at=now() where instance_id=$1 and status='active'`, instanceID); err != nil {
				return err
			}
			routePolicyNeeded = true
		case "instance.delete":
			if _, err := tx.Exec(ctx, `update instances set status='deleted',enabled=false,updated_at=now() where id=$1`, instanceID); err != nil {
				return err
			}
			cleanup, err := s.cleanupInstanceClientServiceAccesses(ctx, tx, instanceID)
			if err != nil {
				return err
			}
			artifactCleanupPaths = cleanup.paths
			if err := releaseInstanceAddressPoolAllocationsTx(ctx, tx, instanceID); err != nil && !isAddressPoolCatalogUnavailable(err) {
				return err
			}
			routePolicyNeeded = true
		}
	} else if jobType == "instance.apply" {
		validationErrors := []any{}
		if errText := strings.TrimSpace(stringify(result["error"])); errText != "" {
			validationErrors = append(validationErrors, map[string]any{"stage": "apply", "message": errText})
		} else if errText := strings.TrimSpace(stringify(result["message"])); errText != "" {
			validationErrors = append(validationErrors, map[string]any{"stage": "apply", "message": errText})
		} else {
			validationErrors = append(validationErrors, map[string]any{"stage": "apply", "message": "instance apply failed"})
		}
		if _, err := tx.Exec(ctx, `update instance_revisions
			set status='failed',applied_at=null,validation_errors_json=$2
			where id=(select current_revision_id from instances where id=$1)`, instanceID, mustJSON(validationErrors)); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `update instances set status='failed',updated_at=now() where id=$1`, instanceID); err != nil {
			return err
		}
	} else if jobType == "instance.delete" {
		if _, err := tx.Exec(ctx, `update instances set status='failed',enabled=false,updated_at=now() where id=$1`, instanceID); err != nil {
			return err
		}
	}

	instance, err := getInstanceTx(ctx, tx, instanceID)
	if err != nil {
		return err
	}
	if status == "succeeded" && jobType == "instance.apply" {
		runtimePreflight := mapFromPath(result, "runtime_preflight")
		recoveredCapability := runtimeCapabilityCodeForService(stringify(runtimePreflight["recovered_capability"]))
		if recoveredCapability != "" && runtimePreflight["binary_available_after_install"] == true {
			version := stringify(runtimePreflight["version"])
			if err := upsertNodeCapabilityTx(ctx, tx, instance.NodeID, recoveredCapability, version, "available", "instance_apply_recovery"); err != nil {
				return err
			}
		}
	}
	if err := s.upsertInstanceRuntimeStateForJobTx(ctx, tx, instance, jobID, jobType, status, result); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if len(artifactCleanupPaths) > 0 {
		_, fileErrors := s.removeManagedArtifactFiles(artifactCleanupPaths)
		if len(fileErrors) > 0 {
			_ = s.AddJobLog(ctx, jobID, "warn", "instance delete left artifact cleanup warnings", map[string]any{"errors": fileErrors})
		}
	}
	if routePolicyNeeded {
		if err := s.queueRoutePolicyJobForInstance(ctx, instanceID); err != nil {
			return err
		}
	}
	return nil
}

func shouldHandoffNodeBootstrapToAgent(payload, result map[string]any) bool {
	bootstrapMode := strings.TrimSpace(stringify(result["bootstrap_mode"]))
	if bootstrapMode == "" {
		bootstrapMode = strings.TrimSpace(stringify(payload["bootstrap_mode"]))
	}
	return bootstrapMode == "ssh_bootstrap"
}

func (s *Store) applyNodeEmergencyCleanupResult(ctx context.Context, nodeID string, payload map[string]any) error {
	includeAgent := false
	if v, ok := payload["include_agent"].(bool); ok {
		includeAgent = v
	}
	cleanupScope := normalizeNodeCleanupScope(stringify(payload["cleanup_scope"]), includeAgent)
	rows, err := s.db.Query(ctx, `select id from instances where node_id=$1 and status <> 'deleted'`, nodeID)
	if err != nil {
		return err
	}
	instanceIDs := []string{}
	for rows.Next() {
		var instanceID string
		if err := rows.Scan(&instanceID); err != nil {
			rows.Close()
			return err
		}
		instanceIDs = append(instanceIDs, instanceID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, instanceID := range instanceIDs {
		if _, err := s.db.Exec(ctx, `update instances set status='deleted',enabled=false,updated_at=now() where id=$1`, instanceID); err != nil {
			return err
		}
		tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return err
		}
		cleanup, cleanupErr := s.cleanupInstanceClientServiceAccesses(ctx, tx, instanceID)
		if cleanupErr != nil {
			_ = tx.Rollback(ctx)
			return cleanupErr
		}
		if commitErr := tx.Commit(ctx); commitErr != nil {
			_ = tx.Rollback(ctx)
			return commitErr
		}
		if len(cleanup.paths) > 0 {
			_, fileErrors := s.removeManagedArtifactFiles(cleanup.paths)
			if len(fileErrors) > 0 {
				_, _ = s.CreateAudit(ctx, "system", "instance.artifact_cleanup.warning", "instance", &instanceID, strings.Join(fileErrors, "; "))
			}
		}
		s.releaseInstanceAddressPoolAllocations(ctx, instanceID)
	}
	if includeAgent {
		if _, err := s.db.Exec(ctx, `update node_agents set status='revoked',revoked_at=now() where node_id=$1`, nodeID); err != nil {
			return err
		}
		if _, err := s.db.Exec(ctx, `update nodes set status='draft',agent_status='removed',last_heartbeat_at=null,updated_at=now() where id=$1 and status <> 'retired'`, nodeID); err != nil {
			return err
		}
	} else {
		if _, err := s.db.Exec(ctx, `update nodes set updated_at=now() where id=$1 and status <> 'retired'`, nodeID); err != nil {
			return err
		}
		if cleanupScope == "services_only" {
			if _, err := s.CreateNodeRoutePolicyApplyJob(ctx, nodeID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) insertCapabilityInstallEvent(ctx context.Context, nodeID, jobID, capCode, strategy, status, summary string, payloadJSON []byte) error {
	if capCode == "" {
		capCode = "unknown"
	}
	if strategy == "" {
		strategy = "unknown"
	}
	if summary == "" {
		summary = "capability job event"
	}
	var payload map[string]any
	if err := decodeJSONField(payloadJSON, &payload, "node_capability_install_events.payload_json"); err != nil {
		return err
	}
	if payload == nil {
		payload = map[string]any{}
	}
	_, err := s.db.Exec(ctx, `insert into node_capability_install_events(id,node_id,job_id,capability_code,strategy,status,summary,payload_json,created_at) values($1,$2,$3,$4,$5,$6,$7,$8,now())`, id.New(), nodeID, jobID, capCode, strategy, status, summary, mustJSON(redactSensitiveMapForStorage(payload)))
	return err
}

func (s *Store) upsertNodeCapability(ctx context.Context, nodeID, capCode, version, status, source string) error {
	return upsertNodeCapabilityTx(ctx, s.db, nodeID, capCode, version, status, source)
}

func upsertNodeCapabilityTx(ctx context.Context, exec interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, nodeID, capCode, version, status, source string) error {
	if capCode == "" {
		return nil
	}
	if status == "" {
		status = "unknown"
	}
	if source == "" {
		source = "installer"
	}
	_, err := exec.Exec(ctx, `insert into node_capabilities(id,node_id,capability_code,version,status,detected_at,source) values($1,$2,$3,$4,$5,now(),$6) on conflict(node_id, capability_code) do update set version=excluded.version,status=excluded.status,detected_at=excluded.detected_at,source=excluded.source`, id.New(), nodeID, capCode, version, status, source)
	return err
}

func (s *Store) queueApplyJobsForCapabilityDependents(ctx context.Context, parentJobID, nodeID, capCode string, payload map[string]any) {
	capCode = runtimeCapabilityCodeForService(capCode)
	if nodeID == "" || capCode == "" {
		return
	}
	for _, instanceID := range stringSetFromAny(payload["dependent_instance_ids"]) {
		instance, err := s.GetInstance(ctx, instanceID)
		if err != nil {
			continue
		}
		if strings.TrimSpace(instance.NodeID) != nodeID {
			continue
		}
		if runtimeCapabilityCodeForService(instance.ServiceCode) != capCode {
			continue
		}
		job, err := s.UpdateInstanceStatus(ctx, instance.ID, driver.OperationApply)
		if err != nil {
			if parentJobID != "" {
				_ = s.AddJobLog(ctx, parentJobID, "warn", "dependent instance apply was not queued", map[string]any{"instance_id": instance.ID, "error": err.Error()})
			}
			continue
		}
		_ = s.AddJobLog(ctx, job.ID, "info", "instance apply queued after runtime install", map[string]any{"instance_id": instance.ID, "runtime_service_code": capCode})
	}
}

func (s *Store) markCapabilityInstallFailedForDependents(ctx context.Context, jobID, nodeID, capCode, status string, payload, result map[string]any) error {
	capCode = runtimeCapabilityCodeForService(capCode)
	if nodeID == "" || capCode == "" {
		return nil
	}
	dependents := stringSetFromAny(payload["dependent_instance_ids"])
	if len(dependents) == 0 {
		return nil
	}
	for _, instanceID := range dependents {
		instance, err := s.GetInstance(ctx, instanceID)
		if err != nil {
			continue
		}
		if strings.TrimSpace(instance.NodeID) != nodeID {
			continue
		}
		if runtimeCapabilityCodeForService(instance.ServiceCode) != capCode {
			continue
		}
		projection := cloneMap(result)
		if projection == nil {
			projection = map[string]any{}
		}
		if strings.TrimSpace(stringify(projection["message"])) == "" {
			projection["message"] = "runtime install " + status
		}
		projection["service_code"] = capCode
		projection["runtime_install_status"] = status
		if err := s.upsertInstanceRuntimeStateForJob(ctx, instance.ID, jobID, "node.capability.install", status, projection); err != nil {
			return err
		}
	}
	return nil
}

func stringSetFromAny(value any) []string {
	switch items := value.(type) {
	case []string:
		return normalizedStringSet(items)
	case []any:
		out := make([]string, 0, len(items))
		for _, item := range items {
			if text := strings.TrimSpace(stringify(item)); text != "" {
				out = append(out, text)
			}
		}
		return normalizedStringSet(out)
	default:
		return nil
	}
}

func (s *Store) AddJobLog(ctx context.Context, jobID, level, msg string, payload map[string]any) error {
	if !in(level, "debug", "info", "warn", "error") {
		level = "info"
	}
	if payload == nil {
		payload = map[string]any{}
	}
	redacted := redactSensitiveMapForStorage(payload)
	b, err := json.Marshal(redacted)
	if err != nil {
		b, err = json.Marshal(map[string]any{"message": "job log payload marshal failed", "error": err.Error()})
		if err != nil {
			return err
		}
	}
	_, err = s.db.Exec(ctx, `insert into job_logs(id,job_id,level,message,payload_json,created_at) values($1,$2,$3,$4,$5,now())`, id.New(), jobID, level, msg, b)
	return err
}

func (s *Store) AgentNextJob(ctx context.Context, nodeRef string) (domain.Job, bool, error) {
	var nodeID string
	if err := s.db.QueryRow(ctx, `select id from nodes where (id::text=$1 or name=$1) and status <> 'retired'`, nodeRef).Scan(&nodeID); err != nil {
		return domain.Job{}, false, err
	}
	where := `node_id=$1 and type in ('node.inventory','node.inventory.sync','node.services.discover','node.capability.install','node.capability.verify','node.channel.probe','node.agent.rotate_token','node.emergency_cleanup','node.reboot','node.backhaul.apply','node.backhaul.probe','node.backhaul.cleanup','node.route_policy.apply','node.route_policy.cleanup','node.firewall.preview','node.firewall.apply','node.firewall.observe','node.firewall.disable','instance.restart','instance.apply','instance.start','instance.stop','instance.enable','instance.disable','instance.diagnose','instance.delete')`
	job, ok, err := s.claimJob(ctx, "agent:"+nodeID, where, []any{nodeID})
	if err != nil || !ok {
		return job, ok, err
	}
	if err := s.hydrateAgentJobSecrets(ctx, nodeID, &job); err != nil {
		return domain.Job{}, false, err
	}
	return job, true, nil
}

func (s *Store) hydrateAgentJobSecrets(ctx context.Context, nodeID string, job *domain.Job) error {
	if job == nil || job.Type != "node.agent.rotate_token" {
		return nil
	}
	if strings.TrimSpace(stringify(job.Payload["new_agent_token"])) != "" {
		return nil
	}
	secretRefID := strings.TrimSpace(stringify(job.Payload["new_agent_token_secret_ref_id"]))
	if secretRefID == "" {
		return errors.New("new agent token secret ref is missing")
	}
	ref, rawToken, err := s.ResolveSecretValue(ctx, secretRefID)
	if err != nil {
		return err
	}
	if refNodeID := strings.TrimSpace(stringify(ref.Meta["node_id"])); refNodeID != "" && refNodeID != strings.TrimSpace(nodeID) {
		return errors.New("new agent token secret ref node mismatch")
	}
	job.Payload["new_agent_token"] = strings.TrimSpace(string(rawToken))
	return nil
}

func (s *Store) claimJob(ctx context.Context, owner, where string, args []any) (domain.Job, bool, error) {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.Job{}, false, err
	}
	defer tx.Rollback(ctx)

	if _, err := recoverStaleJobLeasesTx(ctx, tx, time.Now().UTC()); err != nil {
		return domain.Job{}, false, err
	}

	query := `select id,type,scope_type,scope_id,node_id,instance_id,status,priority,payload_json,coalesce(result_json,'{}'::jsonb),locked_by,locked_until,created_at,started_at,finished_at from jobs where status in ('queued','retrying') and ` + where + ` order by priority asc, created_at asc limit 10 for update skip locked`
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return domain.Job{}, false, err
	}

	// pgx does not allow issuing another command on the same connection while
	// rows are still open. Load candidate jobs first, close rows, then perform
	// lock/update commands inside the same transaction. Row locks acquired by
	// FOR UPDATE SKIP LOCKED are kept until tx commit/rollback.
	candidates := make([]domain.Job, 0, 10)
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			rows.Close()
			return domain.Job{}, false, err
		}
		candidates = append(candidates, j)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return domain.Job{}, false, err
	}
	rows.Close()

	for _, j := range candidates {
		if err := acquireJobResourceLockTx(ctx, tx, j); err != nil {
			continue
		}
		now := time.Now().UTC()
		lockedUntil := now.Add(jobLeaseDurationForType(j.Type))
		_, err = tx.Exec(ctx, `update jobs set status='running',started_at=coalesce(started_at,$2),locked_by=$3,locked_until=$4 where id=$1 and status in ('queued','retrying')`, j.ID, now, owner, lockedUntil)
		if err != nil {
			return domain.Job{}, false, err
		}
		if j.Type == "node.bootstrap" {
			if _, err = tx.Exec(ctx, `update node_bootstrap_runs set status='running',started_at=coalesce(started_at,$2) where job_id=$1 and status='queued'`, j.ID, now); err != nil {
				return domain.Job{}, false, err
			}
		}
		if err = tx.Commit(ctx); err != nil {
			return domain.Job{}, false, err
		}
		j.Status = "running"
		j.LockedBy = &owner
		j.LockedUntil = &lockedUntil
		if j.StartedAt == nil {
			j.StartedAt = &now
		}
		_ = s.AddJobLog(context.Background(), j.ID, "info", "job claimed", map[string]any{"owner": owner})
		return j, true, nil
	}
	return domain.Job{}, false, nil
}

func recoverStaleJobLeasesTx(ctx context.Context, tx pgx.Tx, now time.Time) ([]domain.Job, error) {
	if _, err := tx.Exec(ctx, `delete from resource_locks where expires_at < $1`, now); err != nil {
		return nil, err
	}
	legacyCutoff := now.Add(-legacyRunningJobRecoveryAfter)
	rows, err := tx.Query(ctx, `update jobs
		set status='retrying',
		    locked_by=null,
		    locked_until=null
		where status='running'
		  and (
		    (locked_until is not null and locked_until < $1)
		    or (locked_until is null and coalesce(started_at, created_at) < $2)
		  )
		returning id,type,scope_type,scope_id,node_id,instance_id,status,priority,payload_json,coalesce(result_json,'{}'::jsonb),locked_by,locked_until,created_at,started_at,finished_at`,
		now, legacyCutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	recovered := make([]domain.Job, 0)
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		recovered = append(recovered, job)
	}
	return recovered, rows.Err()
}
func (s *Store) ListAudit(ctx context.Context, limit int) ([]domain.AuditEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `select id,actor_user_id,actor_type,action,resource_type,resource_id,summary,created_at from audit_events order by created_at desc limit $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.AuditEvent
	for rows.Next() {
		var a domain.AuditEvent
		if err := rows.Scan(&a.ID, &a.ActorUserID, &a.ActorType, &a.Action, &a.ResourceType, &a.ResourceID, &a.Summary, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) CreateAudit(ctx context.Context, actor, action, resource string, resourceID *string, summary string) (domain.AuditEvent, error) {
	a := domain.AuditEvent{ID: id.New(), ActorType: actor, Action: action, ResourceType: resource, ResourceID: resourceID, Summary: summary, CreatedAt: time.Now().UTC()}
	_, err := s.db.Exec(ctx, `insert into audit_events(id,actor_user_id,actor_type,action,resource_type,resource_id,summary,payload_json,created_at) values($1,null,$2,$3,$4,$5,$6,'{}'::jsonb,$7)`, a.ID, a.ActorType, a.Action, a.ResourceType, a.ResourceID, a.Summary, a.CreatedAt)
	return a, err
}

func (s *Store) SeedLocalInventory(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `insert into nodes(id,name,kind,role,status,address,os_family,os_version,architecture,execution_mode,agent_status,created_at,updated_at) values($1,'local','local','egress','online','127.0.0.1','linux','unknown','amd64','local_managed','unknown',now(),now()) on conflict(name) where status <> 'retired' do nothing`, id.New())
	return err
}

func (s *Store) CreateNodeEnrollmentToken(ctx context.Context, nodeID string, ttl time.Duration) (domain.NodeEnrollmentToken, error) {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	if _, err := s.GetNode(ctx, nodeID); err != nil {
		return domain.NodeEnrollmentToken{}, err
	}
	token := randomToken(32)
	hash := hashToken(token)
	hint := tokenHint(token)
	now := time.Now().UTC()
	x := domain.NodeEnrollmentToken{ID: id.New(), NodeID: nodeID, Token: token, TokenHint: hint, Status: "active", ExpiresAt: now.Add(ttl), CreatedAt: now}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return x, err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `update node_enrollment_tokens set status='revoked' where node_id=$1 and status='active'`, nodeID)
	if err != nil {
		return x, err
	}
	_, err = tx.Exec(ctx, `insert into node_enrollment_tokens(id,node_id,token_hash,token_hint,status,expires_at,created_at) values($1,$2,$3,$4,$5,$6,$7)`, x.ID, x.NodeID, hash, x.TokenHint, x.Status, x.ExpiresAt, x.CreatedAt)
	if err != nil {
		return x, err
	}
	if err = tx.Commit(ctx); err != nil {
		return x, err
	}
	_, _ = s.CreateAudit(ctx, "system", "node.enrollment_token.create", "node", &nodeID, "node enrollment token created")
	return x, nil
}

func (s *Store) ListNodeEnrollmentTokens(ctx context.Context, nodeID string) ([]domain.NodeEnrollmentToken, error) {
	rows, err := s.db.Query(ctx, `select id,node_id,token_hint,status,expires_at,used_at,created_at from node_enrollment_tokens where node_id=$1 order by created_at desc`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.NodeEnrollmentToken{}
	for rows.Next() {
		var x domain.NodeEnrollmentToken
		if err := rows.Scan(&x.ID, &x.NodeID, &x.TokenHint, &x.Status, &x.ExpiresAt, &x.UsedAt, &x.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) RegisterAgentWithEnrollment(ctx context.Context, nodeID, token, name, address string) (domain.Node, string, error) {
	return s.RegisterAgentWithEnrollmentVersion(ctx, nodeID, token, name, address, "", "")
}

func (s *Store) RegisterAgentWithEnrollmentVersion(ctx context.Context, nodeID, token, name, address, agentVersion, protocolVersion string) (domain.Node, string, error) {
	if nodeID == "" || token == "" {
		return domain.Node{}, "", errors.New("node_id and enrollment_token are required")
	}
	hash := hashToken(token)
	agentToken := randomToken(32)
	agentHash := hashToken(agentToken)
	agentHint := tokenHint(agentToken)
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.Node{}, "", err
	}
	defer tx.Rollback(ctx)
	var tokID string
	err = tx.QueryRow(ctx, `select id from node_enrollment_tokens where node_id=$1 and token_hash=$2 and status='active' and expires_at > now() for update`, nodeID, hash).Scan(&tokID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Node{}, "", errors.New("invalid or expired enrollment token")
	}
	if err != nil {
		return domain.Node{}, "", err
	}
	now := time.Now().UTC()
	_, err = tx.Exec(ctx, `update node_enrollment_tokens set status='used', used_at=$2 where id=$1`, tokID, now)
	if err != nil {
		return domain.Node{}, "", err
	}
	_, err = tx.Exec(ctx, `update node_enrollment_tokens set status='revoked' where node_id=$1 and status='active'`, nodeID)
	if err != nil {
		return domain.Node{}, "", err
	}
	if name == "" {
		name = "node-" + nodeID[:8]
	}
	if address == "" {
		address = "127.0.0.1"
	}
	_, err = tx.Exec(ctx, `update nodes set name=coalesce(nullif($2,''),name), kind='remote', status='online', address=coalesce(nullif($3,''),address), os_family='linux', os_version=coalesce(nullif(os_version,''),'unknown'), architecture=coalesce(nullif(architecture,''),'amd64'), execution_mode='agent_managed', agent_status='online', last_heartbeat_at=$4, updated_at=$4 where id=$1`, nodeID, name, address, now)
	if err != nil {
		return domain.Node{}, "", err
	}
	fingerprint := "sha256:" + hash[:32]
	agentVersion = normalizeAgentRuntimeVersion(agentVersion, "alpha")
	protocolVersion = normalizeAgentRuntimeVersion(protocolVersion, "v1")
	_, err = tx.Exec(ctx, `insert into node_agents(id,node_id,status,agent_version,protocol_version,fingerprint,registered_at,last_seen_at,agent_token_hash,token_hint)
		values($1,$2,'active',$3,$4,$5,$6,$6,$7,$8)
		on conflict(node_id) do update
		set status='active',
		    agent_version=excluded.agent_version,
		    protocol_version=excluded.protocol_version,
		    fingerprint=excluded.fingerprint,
		    last_seen_at=excluded.last_seen_at,
		    agent_token_hash=excluded.agent_token_hash,
		    token_hint=excluded.token_hint,
		    revoked_at=null`, id.New(), nodeID, agentVersion, protocolVersion, fingerprint, now, agentHash, agentHint)
	if err != nil {
		return domain.Node{}, "", err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.Node{}, "", err
	}
	_, _ = s.CreateAudit(ctx, "agent", "agent.register", "node", &nodeID, "agent registered by enrollment")
	n, err := s.GetNode(ctx, nodeID)
	return n, agentToken, err
}

func (s *Store) HeartbeatByNodeID(ctx context.Context, nodeID string) error {
	return s.HeartbeatByNodeIDWithVersion(ctx, nodeID, "", "")
}

func (s *Store) HeartbeatByNodeIDWithVersion(ctx context.Context, nodeID, agentVersion, protocolVersion string) error {
	now := time.Now().UTC()
	cmd, err := s.db.Exec(ctx, `update nodes set status='online',agent_status='online',last_heartbeat_at=$2,updated_at=$2 where id=$1 and status <> 'retired'`, nodeID, now)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	if _, err := s.db.Exec(ctx, `update node_agents
		set last_seen_at=$2,
		    status='active',
		    agent_version=coalesce(nullif($3,''),agent_version),
		    protocol_version=coalesce(nullif($4,''),protocol_version)
		where node_id=$1`, nodeID, now, strings.TrimSpace(agentVersion), strings.TrimSpace(protocolVersion)); err != nil {
		return err
	}
	return nil
}

func normalizeAgentRuntimeVersion(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}
	return strings.TrimSpace(fallback)
}

type jobScanner interface {
	Scan(dest ...any) error
}

func scanJob(row jobScanner) (domain.Job, error) {
	var j domain.Job
	var p, r []byte
	if err := row.Scan(&j.ID, &j.Type, &j.ScopeType, &j.ScopeID, &j.NodeID, &j.InstanceID, &j.Status, &j.Priority, &p, &r, &j.LockedBy, &j.LockedUntil, &j.CreatedAt, &j.StartedAt, &j.FinishedAt); err != nil {
		return domain.Job{}, err
	}
	if err := decodeJSONField(p, &j.Payload, "jobs.payload_json"); err != nil {
		return domain.Job{}, err
	}
	if err := decodeJSONField(r, &j.Result, "jobs.result_json"); err != nil {
		return domain.Job{}, err
	}
	if j.Payload == nil {
		j.Payload = map[string]any{}
	}
	if j.Result == nil {
		j.Result = map[string]any{}
	}
	return j, nil
}

func acquireJobResourceLockTx(ctx context.Context, tx pgx.Tx, j domain.Job) error {
	resourceType, resourceID, lockKind, ok := jobLockTarget(j)
	if !ok {
		return nil
	}
	_, _ = tx.Exec(ctx, `delete from resource_locks where resource_type=$1 and resource_id=$2 and lock_kind=$3 and expires_at < now()`, resourceType, resourceID, lockKind)
	expiresAt := time.Now().UTC().Add(jobLeaseDurationForType(j.Type))
	_, err := tx.Exec(ctx, `insert into resource_locks(id,resource_type,resource_id,lock_kind,job_id,acquired_at,expires_at) values($1,$2,$3,$4,$5,now(),$6)`, id.New(), resourceType, resourceID, lockKind, j.ID, expiresAt)
	return err
}

func jobLockTarget(j domain.Job) (resourceType string, resourceID string, lockKind string, ok bool) {
	// Inventory collection is read-only from the node execution perspective.
	// Do not acquire a node bootstrap lock for it; stale bootstrap locks must not
	// prevent the agent from collecting and submitting inventory snapshots.
	if j.Type == "node.inventory" || j.Type == "node.inventory.sync" || j.Type == "node.services.discover" {
		return "", "", "", false
	}
	if j.InstanceID != nil && *j.InstanceID != "" {
		kind := "mutate"
		if strings.HasSuffix(j.Type, ".apply") {
			kind = "apply"
		}
		if strings.HasSuffix(j.Type, ".delete") {
			kind = "delete"
		}
		return "instance", *j.InstanceID, kind, true
	}
	if j.NodeID != nil && *j.NodeID != "" && strings.HasPrefix(j.Type, "node.") {
		return "node", *j.NodeID, "bootstrap", true
	}
	if j.ScopeID != nil && *j.ScopeID != "" {
		switch {
		case strings.HasPrefix(j.Type, "client."):
			return "client", *j.ScopeID, "provision", true
		case strings.HasPrefix(j.Type, "node."):
			return "node", *j.ScopeID, "bootstrap", true
		}
	}
	return "", "", "", false
}

func jobLeaseDurationForType(jobType string) time.Duration {
	switch jobType {
	case "node.capability.install", "node.emergency_cleanup", "node.reboot", "node.backhaul.apply", "instance.apply", "instance.delete":
		return longNodeJobLeaseDuration
	default:
		return jobLeaseDuration
	}
}

type nodeServiceDiscoveryScanner interface{ Scan(dest ...any) error }

func scanNodeServiceDiscovery(row nodeServiceDiscoveryScanner) (domain.NodeServiceDiscovery, error) {
	var x domain.NodeServiceDiscovery
	var b []byte
	if err := row.Scan(&x.ID, &x.NodeID, &x.ServiceCode, &x.Name, &x.SystemdUnit, &x.ConfigPath, &x.Status, &x.Source, &x.Confidence, &x.EndpointHost, &x.EndpointPort, &x.ManagedInstanceID, &b, &x.DetectedAt); err != nil {
		return domain.NodeServiceDiscovery{}, err
	}
	if err := decodeJSONField(b, &x.Payload, "node_service_discoveries.payload_json"); err != nil {
		return domain.NodeServiceDiscovery{}, err
	}
	if x.Payload == nil {
		x.Payload = map[string]any{}
	}
	return x, nil
}

type nodeInventoryScanner interface{ Scan(dest ...any) error }

func scanNodeInventory(row nodeInventoryScanner) (domain.NodeInventorySnapshot, error) {
	var x domain.NodeInventorySnapshot
	var b []byte
	if err := row.Scan(&x.ID, &x.NodeID, &b, &x.CreatedAt); err != nil {
		return domain.NodeInventorySnapshot{}, err
	}
	if err := decodeJSONField(b, &x.Payload, "node_inventory_snapshots.payload_json"); err != nil {
		return domain.NodeInventorySnapshot{}, err
	}
	if x.Payload == nil {
		x.Payload = map[string]any{}
	}
	return x, nil
}

func deriveCapabilities(nodeID string, payload map[string]any, detectedAt time.Time) []domain.NodeCapability {
	binaries := mapFromPath(payload, "binaries")
	services := mapFromPath(payload, "services")
	defs := []struct {
		code     string
		binaries []string
		services []string
	}{
		{code: "systemd", binaries: []string{"systemctl"}},
		{code: "xray-core", binaries: []string{"xray"}, services: []string{"xray", "xray-megavpn", "rtis.xray"}},
		{code: "openvpn", binaries: []string{"openvpn"}, services: []string{"openvpn", "openvpn-server@server", "megavpn-openvpn"}},
		{code: "nginx", binaries: []string{"nginx"}, services: []string{"nginx"}},
		{code: "wireguard", binaries: []string{"wg", "wg-quick"}, services: []string{"wg-quick"}},
		{code: "ipsec", binaries: []string{"ipsec", "strongswan"}, services: []string{"strongswan", "strongswan-starter", "ipsec"}},
		{code: "xl2tpd", binaries: []string{"xl2tpd"}, services: []string{"xl2tpd"}},
		{code: "shadowsocks", binaries: []string{"ss-server"}, services: []string{"shadowsocks-libev", "shadowsocks"}},
	}
	out := make([]domain.NodeCapability, 0, len(defs))
	for _, d := range defs {
		status := "missing"
		version := ""
		for _, name := range d.binaries {
			if v, ok := binaries[name]; ok {
				status = "available"
				if version == "" {
					version = stringify(v)
				}
			}
		}
		for _, name := range d.services {
			if v, ok := services[name]; ok {
				m, _ := v.(map[string]any)
				state := stringify(m["active_state"])
				if state == "active" || state == "inactive" || state == "unknown" {
					if status == "missing" && state != "unknown" {
						status = "available"
					}
				}
			}
		}
		out = append(out, domain.NodeCapability{ID: id.New(), NodeID: nodeID, CapabilityCode: d.code, Version: version, Status: status, Source: "inventory", DetectedAt: detectedAt})
	}
	return out
}

func deriveServiceDiscoveries(nodeID string, payload map[string]any, detectedAt time.Time) []domain.NodeServiceDiscovery {
	binaries := mapFromPath(payload, "binaries")
	services := mapFromPath(payload, "services")
	configs := mapFromPath(payload, "config_files")
	ports := portSet(payload)
	primaryAddr := firstNonLoopbackAddress(payload)
	type rule struct {
		code         string
		name         string
		binary       string
		units        []string
		patterns     []string
		defaultPorts []int
	}
	rules := []rule{
		{code: "xray-core", name: "xray-core", binary: "xray", units: []string{"xray", "xray-megavpn", "rtis.xray"}, patterns: []string{"/etc/xray", "/usr/local/etc/xray", "/opt/xray", "/opt/megavpn/xray"}, defaultPorts: []int{443, 8443, 10085}},
		{code: "openvpn", name: "openvpn", binary: "openvpn", units: []string{"openvpn", "openvpn-server@server", "megavpn-openvpn"}, patterns: []string{"/etc/openvpn", "/opt/megavpn/openvpn"}, defaultPorts: []int{1194, 11194}},
		{code: "nginx", name: "nginx", binary: "nginx", units: []string{"nginx"}, patterns: []string{"/etc/nginx"}, defaultPorts: []int{80, 443, 8443}},
		{code: "wireguard", name: "wireguard", binary: "wg", units: []string{"wg-quick"}, patterns: []string{"/etc/wireguard"}, defaultPorts: []int{51820}},
		{code: "ipsec", name: "ipsec", binary: "ipsec", units: []string{"strongswan", "strongswan-starter", "ipsec"}, patterns: []string{"/etc/ipsec", "/etc/strongswan"}, defaultPorts: []int{500, 4500}},
		{code: "ipsec", name: "xl2tpd", binary: "xl2tpd", units: []string{"xl2tpd"}, patterns: []string{"/etc/xl2tpd"}, defaultPorts: []int{1701}},
		{code: "shadowsocks", name: "shadowsocks", binary: "ss-server", units: []string{"shadowsocks-libev", "shadowsocks"}, patterns: []string{"/etc/shadowsocks", "/opt/megavpn/shadowsocks"}},
		{code: "http_proxy", name: "squid", binary: "squid", units: []string{"squid"}, patterns: []string{"/etc/squid"}, defaultPorts: []int{3128}},
	}
	out := []domain.NodeServiceDiscovery{}
	seen := map[string]bool{}
	for _, r := range rules {
		unit, unitPayload, hasUnit := firstKnownService(services, r.units)
		configPath, configPayload, hasConfig := firstMatchingConfig(configs, r.patterns)
		binaryVersion := stringify(binaries[r.binary])
		endpointPort := firstOpenPort(ports, r.defaultPorts)
		if !hasUnit && !hasConfig && binaryVersion == "" && endpointPort == 0 {
			continue
		}
		name := r.name
		if unit != "" {
			name = unit
		} else if configPath != "" {
			name = r.name + "-config"
		}
		key := r.code + ":" + name
		if seen[key] {
			continue
		}
		seen[key] = true
		status := "discovered"
		confidence := 20
		if hasConfig {
			confidence += 25
		}
		if binaryVersion != "" {
			confidence += 25
			status = "available"
		}
		if hasUnit {
			confidence += 35
			status = "available"
		}
		if endpointPort > 0 {
			confidence += 10
		}
		if confidence > 100 {
			confidence = 100
		}
		payload := map[string]any{"binary_version": binaryVersion, "default_port_observed": endpointPort > 0}
		if hasUnit {
			payload["service"] = unitPayload
		}
		if hasConfig {
			payload["config"] = configPayload
		}
		out = append(out, domain.NodeServiceDiscovery{ID: id.New(), NodeID: nodeID, ServiceCode: r.code, Name: name, SystemdUnit: unit, ConfigPath: configPath, Status: status, Source: "inventory", Confidence: confidence, EndpointHost: primaryAddr, EndpointPort: endpointPort, Payload: payload, DetectedAt: detectedAt})
	}
	return out
}

func portSet(payload map[string]any) map[int]bool {
	out := map[int]bool{}
	raw, _ := payload["ports"].([]any)
	for _, item := range raw {
		m, _ := item.(map[string]any)
		addr := stringify(m["local_address"])
		if addr == "" {
			continue
		}
		idx := strings.LastIndex(addr, ":")
		if idx < 0 || idx+1 >= len(addr) {
			continue
		}
		portStr := strings.Trim(addr[idx+1:], "[]")
		var port int
		if _, err := fmt.Sscanf(portStr, "%d", &port); err == nil && port > 0 {
			out[port] = true
		}
	}
	return out
}

func firstOpenPort(ports map[int]bool, candidates []int) int {
	for _, port := range candidates {
		if ports[port] {
			return port
		}
	}
	return 0
}

func firstNonLoopbackAddress(payload map[string]any) string {
	raw, _ := payload["interfaces"].([]any)
	for _, item := range raw {
		iface, _ := item.(map[string]any)
		addrs, _ := iface["addrs"].([]any)
		for _, addrRaw := range addrs {
			addr := stringify(addrRaw)
			if addr == "" || strings.HasPrefix(addr, "127.") || strings.HasPrefix(addr, "::1") || strings.HasPrefix(addr, "fe80:") {
				continue
			}
			if ip, _, ok := strings.Cut(addr, "/"); ok {
				return ip
			}
			return addr
		}
	}
	return ""
}

func firstKnownService(services map[string]any, names []string) (string, any, bool) {
	for _, n := range names {
		v, ok := services[n]
		if !ok {
			continue
		}
		m, _ := v.(map[string]any)
		active := stringify(m["active_state"])
		enabled := stringify(m["enabled_state"])
		if active == "unknown" && enabled == "unknown" {
			continue
		}
		return n, v, true
	}
	return "", nil, false
}

func firstMatchingConfig(configs map[string]any, patterns []string) (string, any, bool) {
	for path, v := range configs {
		for _, p := range patterns {
			if strings.HasPrefix(path, p) {
				return path, v, true
			}
		}
	}
	return "", nil, false
}

func mapFromPath(m map[string]any, path ...string) map[string]any {
	var cur any = m
	for _, p := range path {
		mm, ok := cur.(map[string]any)
		if !ok {
			return map[string]any{}
		}
		cur = mm[p]
	}
	if mm, ok := cur.(map[string]any); ok {
		return mm
	}
	return map[string]any{}
}

func stringFromPath(m map[string]any, path ...string) string {
	var cur any = m
	for _, p := range path {
		mm, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur = mm[p]
	}
	return stringify(cur)
}

func nullIfEmpty(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}

func derefString(v *string) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(*v)
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte(`{}`)
	}
	return b
}

func decodeJSONField(raw []byte, target any, field string) error {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("decode %s: %w", field, err)
	}
	return nil
}

func redactSensitiveMapForStorage(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	out := make(map[string]any, len(src))
	for key, value := range src {
		if isSensitiveStorageKey(key) {
			out[key] = "[redacted]"
			continue
		}
		out[key] = redactSensitiveValueForStorage(value)
	}
	return out
}

func redactSensitiveValueForStorage(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return redactSensitiveMapForStorage(typed)
	case []any:
		out := make([]any, len(typed))
		for i := range typed {
			out[i] = redactSensitiveValueForStorage(typed[i])
		}
		return out
	case string:
		if containsSensitiveStorageString(typed) {
			return "[redacted]"
		}
		return typed
	default:
		return value
	}
}

func isSensitiveStorageKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	if normalized == "" {
		return false
	}
	if strings.Contains(normalized, "hint") {
		return false
	}
	if strings.HasSuffix(normalized, "secret_ref_id") {
		return false
	}
	compact := strings.ReplaceAll(normalized, "_", "")
	switch normalized {
	case "token", "agent_token", "new_agent_token", "new_agent_token_hash", "enrollment_token",
		"password", "smtp_password", "private_key", "secret", "psk",
		"agent_bootstrapenv", "agent_bootstrap_env", "bootstrap_env",
		"content", "json", "config", "config_json", "config_content", "privatekey", "presharedkey",
		"reality_private_key", "wireguard_private_key", "openvpn_private_key", "tls_private_key":
		return true
	}
	switch compact {
	case "privatekey", "presharedkey", "agentbootstrapenv", "bootstrapenv", "configjson", "configcontent":
		return true
	}
	if strings.HasSuffix(normalized, "_token") || strings.HasSuffix(normalized, "_token_hash") {
		return true
	}
	if strings.Contains(normalized, "password") || strings.Contains(normalized, "private_key") || strings.Contains(compact, "privatekey") {
		return true
	}
	if strings.Contains(normalized, "secret") {
		return true
	}
	return false
}

func containsSensitiveStorageString(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return false
	}
	switch {
	case strings.Contains(normalized, "-----begin ") && strings.Contains(normalized, " private key-----"):
		return true
	case strings.Contains(normalized, "privatekey ="):
		return true
	case strings.Contains(normalized, "presharedkey ="):
		return true
	case strings.Contains(normalized, "megavpn_agent_enrollment_token="):
		return true
	case strings.Contains(normalized, "password:") || strings.Contains(normalized, `"password"`):
		return true
	default:
		return false
	}
}

func normalizeCapabilityCode(code string) string {
	return driver.NormalizeCode(code)
}

func runtimeCapabilityCodeForService(serviceCode string) string {
	contract, ok := driver.ContractFor(serviceCode)
	if ok && strings.TrimSpace(contract.RuntimeCode) != "" {
		return driver.NormalizeCode(contract.RuntimeCode)
	}
	return driver.NormalizeCode(serviceCode)
}

func validInstallStrategy(serviceCode, strategy string) bool {
	return driver.IsValidInstallStrategy(serviceCode, strategy)
}

func normalizeInstanceRuntimeCode(serviceCode string) string {
	return driver.NormalizeCode(serviceCode)
}

func serviceDefaultSystemdUnit(serviceCode, slug string) string {
	if unit := driver.DefaultSystemdUnit(serviceCode, slug); unit != "" {
		return unit
	}
	if slug == "" {
		slug = "instance"
	}
	return "megavpn-" + slug
}

func (s *Store) latestInstanceSpec(ctx context.Context, instanceID string) (map[string]any, error) {
	var raw []byte
	err := s.db.QueryRow(ctx, `select spec_json from instance_revisions where instance_id=$1 order by revision_no desc limit 1`, instanceID).Scan(&raw)
	if err != nil {
		return nil, err
	}
	var spec map[string]any
	if err := decodeJSONField(raw, &spec, "instance_revisions.spec_json"); err != nil {
		return nil, err
	}
	if spec == nil {
		spec = map[string]any{}
	}
	return spec, nil
}

func (s *Store) ListInstanceRevisions(ctx context.Context, instanceID string, limit int) ([]domain.InstanceRevision, error) {
	if strings.TrimSpace(instanceID) == "" {
		return nil, fmt.Errorf("instance id is required")
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.db.Query(ctx, `select r.id,r.instance_id,r.source,r.status,coalesce(r.rendered_hash,''),r.revision_no,r.spec_json,coalesce(r.validation_errors_json,'[]'::jsonb),r.created_at,r.applied_at,
			(r.id = i.current_revision_id) as is_current,
			(r.id = i.last_applied_revision_id) as is_last_applied
		from instance_revisions r
		join instances i on i.id=r.instance_id
		where r.instance_id=$1
		order by revision_no desc
		limit $2`, instanceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]domain.InstanceRevision, 0)
	for rows.Next() {
		var rev domain.InstanceRevision
		var specRaw []byte
		var validationRaw []byte
		if err := rows.Scan(&rev.ID, &rev.InstanceID, &rev.Source, &rev.Status, &rev.RenderedHash, &rev.RevisionNo, &specRaw, &validationRaw, &rev.CreatedAt, &rev.AppliedAt, &rev.IsCurrent, &rev.IsLastApplied); err != nil {
			return nil, err
		}
		if err := decodeJSONField(specRaw, &rev.Spec, "instance_revisions.spec_json"); err != nil {
			return nil, err
		}
		if err := decodeJSONField(validationRaw, &rev.ValidationErrors, "instance_revisions.validation_errors_json"); err != nil {
			return nil, err
		}
		if rev.Spec == nil {
			rev.Spec = map[string]any{}
		}
		if rev.ValidationErrors == nil {
			rev.ValidationErrors = []any{}
		}
		out = append(out, rev)
	}
	return out, rows.Err()
}

func (s *Store) buildInstanceJobPayload(ctx context.Context, x domain.Instance, action string) (map[string]any, error) {
	spec, err := s.latestInstanceSpec(ctx, x.ID)
	if err != nil {
		return nil, err
	}
	spec, err = s.renderInstancePayloadSpec(ctx, x, spec)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"instance_id":          x.ID,
		"action":               action,
		"service_code":         x.ServiceCode,
		"runtime_service_code": normalizeInstanceRuntimeCode(x.ServiceCode),
		"name":                 x.Name,
		"slug":                 x.Slug,
		"systemd_unit":         x.SystemdUnit,
		"endpoint_host":        x.EndpointHost,
		"endpoint_port":        x.EndpointPort,
		"enabled":              x.Enabled,
		"spec":                 spec,
	}
	return payload, nil
}

func (s *Store) buildInstanceDeleteJobPayload(ctx context.Context, x domain.Instance) (map[string]any, error) {
	spec, err := s.latestInstanceSpec(ctx, x.ID)
	if err != nil {
		return nil, err
	}
	rendered, renderErr := s.renderInstancePayloadSpec(ctx, x, spec)
	if renderErr == nil {
		spec = rendered
	} else {
		spec = cloneMap(spec)
		spec["render_error"] = renderErr.Error()
	}
	payload := map[string]any{
		"instance_id":          x.ID,
		"action":               "delete",
		"service_code":         x.ServiceCode,
		"runtime_service_code": normalizeInstanceRuntimeCode(x.ServiceCode),
		"name":                 x.Name,
		"slug":                 x.Slug,
		"systemd_unit":         x.SystemdUnit,
		"endpoint_host":        x.EndpointHost,
		"endpoint_port":        x.EndpointPort,
		"enabled":              false,
		"spec":                 spec,
	}
	return payload, nil
}

func stringify(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case fmt.Stringer:
		return strings.TrimSpace(x.String())
	case nil:
		return ""
	default:
		return strings.TrimSpace(fmt.Sprint(x))
	}
}

func AsError(prefix string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", prefix, err)
}

func (s *Store) createInstanceRevision(ctx context.Context, instanceID, source, status string, spec map[string]any) (domain.InstanceRevision, error) {
	if spec == nil {
		spec = map[string]any{}
	}
	instance, err := s.GetInstance(ctx, instanceID)
	if err != nil {
		return domain.InstanceRevision{}, err
	}
	if status == "" {
		status = "draft"
	}
	spec, err = s.materializeInstanceDriverSpecDefaults(ctx, instance, spec)
	if err != nil {
		return domain.InstanceRevision{}, err
	}
	renderedHash := ""
	validationErrors := []any{}
	if status == "validated" || status == "draft" {
		status, renderedHash, validationErrors = s.validateInstanceRevisionSpec(ctx, instance, spec)
	}
	var revisionNo int
	if err := s.db.QueryRow(ctx, `select coalesce(max(revision_no),0)+1 from instance_revisions where instance_id=$1`, instanceID).Scan(&revisionNo); err != nil {
		return domain.InstanceRevision{}, err
	}
	b, err := json.Marshal(spec)
	if err != nil {
		return domain.InstanceRevision{}, err
	}
	rev := domain.InstanceRevision{ID: id.New(), InstanceID: instanceID, RevisionNo: revisionNo, Source: source, Status: status, RenderedHash: renderedHash, Spec: spec, ValidationErrors: validationErrors, CreatedAt: time.Now().UTC(), IsCurrent: true}
	if instance.CurrentRevisionID != nil && strings.TrimSpace(*instance.CurrentRevisionID) != "" {
		if _, err := s.db.Exec(ctx, `update instance_revisions
			set status='superseded'
			where id=$1
			  and id is distinct from $2
			  and status in ('draft','validated','failed')`, *instance.CurrentRevisionID, nullIfEmpty(derefString(instance.LastAppliedRevisionID))); err != nil {
			return domain.InstanceRevision{}, err
		}
	}
	if _, err := s.db.Exec(ctx, `insert into instance_revisions(id,instance_id,revision_no,source,status,spec_json,rendered_hash,validation_errors_json,created_at) values($1,$2,$3,$4,$5,$6,$7,$8,now())`, rev.ID, rev.InstanceID, rev.RevisionNo, rev.Source, rev.Status, b, nullIfEmpty(rev.RenderedHash), mustJSON(rev.ValidationErrors)); err != nil {
		return rev, err
	}
	_, err = s.db.Exec(ctx, `update instances set current_revision_id=$2,updated_at=now() where id=$1`, instanceID, rev.ID)
	return rev, err
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "instance"
	}
	return out
}

func in(v string, allowed ...string) bool {
	for _, x := range allowed {
		if v == x {
			return true
		}
	}
	return false
}

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func tokenHint(token string) string {
	if len(token) <= 14 {
		return token
	}
	return token[:8] + "..." + token[len(token)-6:]
}

func (s *Store) ValidateAgentToken(ctx context.Context, nodeID, token string) bool {
	if nodeID == "" || token == "" {
		return false
	}
	var ok bool
	err := s.db.QueryRow(ctx, `select (
		exists(select 1 from node_agents where node_id=$1 and status='active' and revoked_at is null and agent_token_hash=$2)
		or `+rotatedAgentTokenMatchesSQL()+`
	)`, nodeID, hashToken(token)).Scan(&ok)
	return err == nil && ok
}

func (s *Store) RecordAgentAuthFailure(ctx context.Context, nodeID, reason string) error {
	if strings.TrimSpace(nodeID) == "" {
		return nil
	}
	_, err := s.db.Exec(ctx, `update node_agents
		set last_auth_failure_at=now(),
		    last_auth_failure_reason=$2
		where node_id=$1`, strings.TrimSpace(nodeID), strings.TrimSpace(reason))
	return err
}

func (s *Store) TouchAgentJobPoll(ctx context.Context, nodeID string) error {
	if strings.TrimSpace(nodeID) == "" {
		return nil
	}
	_, err := s.db.Exec(ctx, `update node_agents set last_job_poll_at=now() where node_id=$1`, strings.TrimSpace(nodeID))
	return err
}

func (s *Store) RecordAgentJobClaim(ctx context.Context, nodeID, jobID, jobType string) error {
	if strings.TrimSpace(nodeID) == "" {
		return nil
	}
	_, err := s.db.Exec(ctx, `update node_agents
		set last_job_claim_at=now(),
		    last_job_claim_job_id=$2,
		    last_job_claim_type=$3
		where node_id=$1`, strings.TrimSpace(nodeID), nullableUUID(jobID), strings.TrimSpace(jobType))
	return err
}

func (s *Store) RecordAgentJobResult(ctx context.Context, nodeID, jobID, jobType, status string) error {
	if strings.TrimSpace(nodeID) == "" {
		return nil
	}
	_, err := s.db.Exec(ctx, `update node_agents
		set last_job_result_at=now(),
		    last_job_result_job_id=$2,
		    last_job_result_type=$3,
		    last_job_result_status=$4
		where node_id=$1`, strings.TrimSpace(nodeID), nullableUUID(jobID), strings.TrimSpace(jobType), strings.TrimSpace(status))
	return err
}

func (s *Store) RecordAgentInventorySync(ctx context.Context, nodeID, source string) error {
	if strings.TrimSpace(nodeID) == "" {
		return nil
	}
	source = strings.ToLower(strings.TrimSpace(source))
	if source == "discovery" {
		_, err := s.db.Exec(ctx, `update node_agents
			set last_inventory_sync_at=now(),
			    last_discovery_sync_at=now()
			where node_id=$1`, strings.TrimSpace(nodeID))
		return err
	}
	_, err := s.db.Exec(ctx, `update node_agents set last_inventory_sync_at=now() where node_id=$1`, strings.TrimSpace(nodeID))
	return err
}

func nullableUUID(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func (s *Store) ValidateAgentTokenForJob(ctx context.Context, jobID, token string) bool {
	if jobID == "" || token == "" {
		return false
	}
	var nodeID string
	err := s.db.QueryRow(ctx, `select coalesce(node_id::text,'') from jobs where id=$1`, jobID).Scan(&nodeID)
	if err != nil || nodeID == "" {
		return false
	}
	return s.ValidateAgentToken(ctx, nodeID, token)
}

func randomToken(bytesN int) string {
	b := make([]byte, bytesN)
	if _, err := rand.Read(b); err != nil {
		return strings.ReplaceAll(id.New(), "-", "")
	}
	return hex.EncodeToString(b)
}
