package postgres

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/platform/id"
	secretstore "github.com/rtis-emc2/megavpn/internal/secrets"
)

type Store struct {
	db        *pgxpool.Pool
	secretSvc *secretstore.Service
}

func New(pool *pgxpool.Pool) *Store { return &Store{db: pool} }

func (s *Store) SetSecretService(secretSvc *secretstore.Service) {
	if s == nil {
		return
	}
	s.secretSvc = secretSvc
}

func (s *Store) Ping(ctx context.Context) error { return s.db.Ping(ctx) }

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
	rows, err := s.db.Query(ctx, `select id,name,kind,role,status,address,os_family,os_version,architecture,execution_mode,agent_status,last_heartbeat_at,created_at,updated_at from nodes order by created_at desc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Node
	for rows.Next() {
		var n domain.Node
		if err := rows.Scan(&n.ID, &n.Name, &n.Kind, &n.Role, &n.Status, &n.Address, &n.OSFamily, &n.OSVersion, &n.Architecture, &n.ExecutionMode, &n.AgentStatus, &n.LastHeartbeatAt, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) GetNode(ctx context.Context, nodeID string) (domain.Node, error) {
	var n domain.Node
	err := s.db.QueryRow(ctx, `select id,name,kind,role,status,address,os_family,os_version,architecture,execution_mode,agent_status,last_heartbeat_at,created_at,updated_at from nodes where id=$1`, nodeID).Scan(&n.ID, &n.Name, &n.Kind, &n.Role, &n.Status, &n.Address, &n.OSFamily, &n.OSVersion, &n.Architecture, &n.ExecutionMode, &n.AgentStatus, &n.LastHeartbeatAt, &n.CreatedAt, &n.UpdatedAt)
	return n, err
}

func (s *Store) CreateNode(ctx context.Context, n domain.Node) (domain.Node, error) {
	if strings.TrimSpace(n.Name) == "" {
		return n, errors.New("node name is required")
	}
	if strings.TrimSpace(n.Address) == "" {
		return n, errors.New("node address is required")
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
	_, err := s.db.Exec(ctx, `insert into nodes(id,name,kind,role,status,address,os_family,os_version,architecture,execution_mode,agent_status,created_at,updated_at) values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, n.ID, n.Name, n.Kind, n.Role, n.Status, n.Address, n.OSFamily, n.OSVersion, n.Architecture, n.ExecutionMode, n.AgentStatus, n.CreatedAt, n.UpdatedAt)
	if err != nil {
		return n, err
	}
	_, _ = s.CreateAudit(ctx, "system", "node.create", "node", &n.ID, "node created")
	return n, nil
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
	_, _ = s.db.Exec(ctx, `update node_agents set status='revoked',revoked_at=now() where node_id=$1`, nodeID)
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
	if name == "" {
		return domain.Node{}, errors.New("agent node name is empty")
	}
	var n domain.Node
	err := s.db.QueryRow(ctx, `select id,name,kind,role,status,address,os_family,os_version,architecture,execution_mode,agent_status,last_heartbeat_at,created_at,updated_at from nodes where name=$1`, name).Scan(&n.ID, &n.Name, &n.Kind, &n.Role, &n.Status, &n.Address, &n.OSFamily, &n.OSVersion, &n.Architecture, &n.ExecutionMode, &n.AgentStatus, &n.LastHeartbeatAt, &n.CreatedAt, &n.UpdatedAt)
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
	_, err = s.db.Exec(ctx, `insert into node_agents(id,node_id,status,agent_version,protocol_version,fingerprint,registered_at,last_seen_at) values($1,$2,'active','dev','v1',$3,$4,$4) on conflict(node_id) do update set status='active', last_seen_at=excluded.last_seen_at, revoked_at=null`, id.New(), n.ID, fingerprint, now)
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
	now := time.Now().UTC()
	cmd, err := s.db.Exec(ctx, `update nodes set status='online',agent_status='online',last_heartbeat_at=$2,updated_at=$2 where name=$1 and status <> 'retired'`, nodeName, now)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	_, _ = s.db.Exec(ctx, `update node_agents set last_seen_at=$2,status='active' where node_id=(select id from nodes where name=$1)`, nodeName, now)
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
		_ = json.Unmarshal(raw, &x.Payload)
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
	b, _ := json.Marshal(payload)
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
		_, _ = tx.Exec(ctx, `update nodes set os_family='linux', os_version=$2, architecture=$3, updated_at=now() where id=$1`, nodeID, osVersion, arch)
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
		db, _ := json.Marshal(d.Payload)
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
	serviceCode = normalizeCapabilityCode(serviceCode)
	strategy = strings.TrimSpace(strategy)
	channel = strings.TrimSpace(channel)
	if _, err := s.GetNode(ctx, nodeID); err != nil {
		return domain.Job{}, err
	}
	if serviceCode == "" {
		return domain.Job{}, errors.New("service_code is required")
	}
	if strategy == "" {
		switch serviceCode {
		case "nginx":
			strategy = "nginx_org_repo"
		case "xray-core":
			strategy = "xtls_install_release"
		default:
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
	j, err := s.CreateJob(ctx, domain.Job{Type: "node.capability.install", ScopeType: "node", ScopeID: &nodeID, NodeID: &nodeID, Payload: payload})
	if err != nil {
		return j, err
	}
	_, _ = s.db.Exec(ctx, `insert into node_capability_install_events(id,node_id,job_id,capability_code,strategy,status,summary,payload_json,created_at) values($1,$2,$3,$4,$5,'queued','capability install queued',$6,now())`, id.New(), nodeID, j.ID, serviceCode, strategy, mustJSON(payload))
	_, _ = s.CreateAudit(ctx, "system", "node.capability.install", "node", &nodeID, "capability install queued: "+serviceCode)
	return j, nil
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
	_, _ = s.db.Exec(ctx, `insert into node_capability_install_events(id,node_id,job_id,capability_code,strategy,status,summary,payload_json,created_at) values($1,$2,$3,$4,'verify','queued','capability verify queued',$5,now())`, id.New(), nodeID, j.ID, serviceCode, mustJSON(payload))
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
		_, _ = s.db.Exec(ctx, `update node_service_discoveries set status='imported', managed_instance_id=$3 where node_id=$1 and id=$2`, d.NodeID, d.ID, existingID)
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
		if err := s.db.QueryRow(ctx, `select exists(select 1 from instances where slug=$1)`, slug).Scan(&exists); err != nil {
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
	b, _ := json.Marshal(spec)
	_, err = tx.Exec(ctx, `insert into instance_revisions(id,instance_id,revision_no,source,status,spec_json,created_at) values($1,$2,1,'import','validated',$3,now())`, id.New(), instanceID, b)
	if err != nil {
		return domain.Instance{}, err
	}
	_, err = tx.Exec(ctx, `update node_service_discoveries set status='imported', managed_instance_id=$3 where node_id=$1 and id=$2`, d.NodeID, d.ID, instanceID)
	if err != nil {
		return domain.Instance{}, err
	}
	_, _ = tx.Exec(ctx, `insert into node_service_discovery_events(id,node_id,discovery_id,event_type,summary,payload_json,created_at) values($1,$2,$3,'imported',$4,$5,now())`, id.New(), d.NodeID, d.ID, "service discovery imported", mustJSON(map[string]any{"instance_id": instanceID, "service_code": d.ServiceCode}))
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
	rows, err := s.db.Query(ctx, `select i.id,i.node_id,sd.code,i.name,i.slug,coalesce(i.systemd_unit,''),i.status,i.enabled,coalesce(i.endpoint_host,''),coalesce(i.endpoint_port,0),i.created_at,i.updated_at from instances i join service_definitions sd on sd.id=i.service_definition_id where i.status <> 'deleted' order by i.created_at desc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Instance
	for rows.Next() {
		var x domain.Instance
		if err := rows.Scan(&x.ID, &x.NodeID, &x.ServiceCode, &x.Name, &x.Slug, &x.SystemdUnit, &x.Status, &x.Enabled, &x.EndpointHost, &x.EndpointPort, &x.CreatedAt, &x.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) GetInstance(ctx context.Context, instanceID string) (domain.Instance, error) {
	var x domain.Instance
	err := s.db.QueryRow(ctx, `select i.id,i.node_id,sd.code,i.name,i.slug,coalesce(i.systemd_unit,''),i.status,i.enabled,coalesce(i.endpoint_host,''),coalesce(i.endpoint_port,0),i.created_at,i.updated_at from instances i join service_definitions sd on sd.id=i.service_definition_id where i.id=$1`, instanceID).Scan(&x.ID, &x.NodeID, &x.ServiceCode, &x.Name, &x.Slug, &x.SystemdUnit, &x.Status, &x.Enabled, &x.EndpointHost, &x.EndpointPort, &x.CreatedAt, &x.UpdatedAt)
	return x, err
}

func (s *Store) FindNodeInstanceByService(ctx context.Context, nodeID, serviceCode string) (domain.Instance, error) {
	serviceCode = normalizeInstanceRuntimeCode(serviceCode)
	var x domain.Instance
	err := s.db.QueryRow(ctx, `select i.id,i.node_id,sd.code,i.name,i.slug,coalesce(i.systemd_unit,''),i.status,i.enabled,coalesce(i.endpoint_host,''),coalesce(i.endpoint_port,0),i.created_at,i.updated_at
		from instances i
		join service_definitions sd on sd.id=i.service_definition_id
		where i.node_id=$1 and sd.code=$2 and i.status <> 'deleted'
		order by i.enabled desc, i.created_at asc
		limit 1`, nodeID, serviceCode).
		Scan(&x.ID, &x.NodeID, &x.ServiceCode, &x.Name, &x.Slug, &x.SystemdUnit, &x.Status, &x.Enabled, &x.EndpointHost, &x.EndpointPort, &x.CreatedAt, &x.UpdatedAt)
	return x, err
}

func (s *Store) CreateInstance(ctx context.Context, x domain.Instance) (domain.Instance, error) {
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
	if x.SystemdUnit == "" {
		x.SystemdUnit = serviceDefaultSystemdUnit(x.ServiceCode, x.Slug)
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
	_, _ = s.createInstanceRevision(ctx, x.ID, "system", "validated", spec)
	payload, payloadErr := s.buildInstanceJobPayload(ctx, x, "apply")
	if payloadErr == nil {
		_, _ = s.CreateJob(ctx, domain.Job{Type: "instance.apply", ScopeType: "instance", ScopeID: &x.ID, NodeID: &x.NodeID, InstanceID: &x.ID, Payload: payload})
	}
	_, _ = s.CreateAudit(ctx, "system", "instance.create", "instance", &x.ID, "instance created")
	return x, nil
}

func (s *Store) UpdateInstanceStatus(ctx context.Context, instanceID, action string) (domain.Job, error) {
	x, err := s.GetInstance(ctx, instanceID)
	if err != nil {
		return domain.Job{}, err
	}
	jobType := "instance." + action
	status := x.Status
	enabled := x.Enabled
	switch action {
	case "apply":
		status = "provisioning"
	case "restart":
	case "start":
		status = "active"
		enabled = true
	case "stop":
		status = "disabled"
		enabled = false
	case "disable":
		status = "disabled"
		enabled = false
	case "enable":
		status = "active"
		enabled = true
	default:
		return domain.Job{}, fmt.Errorf("unsupported instance action %q", action)
	}
	x.Status = status
	x.Enabled = enabled
	payload, payloadErr := s.buildInstanceJobPayload(ctx, x, action)
	if payloadErr != nil {
		if action == "apply" {
			return domain.Job{}, payloadErr
		}
		payload = map[string]any{"instance_id": instanceID, "action": action, "service_code": x.ServiceCode, "systemd_unit": x.SystemdUnit}
	}
	_, err = s.db.Exec(ctx, `update instances set status=$2,enabled=$3,updated_at=now() where id=$1`, instanceID, status, enabled)
	if err != nil {
		return domain.Job{}, err
	}
	j, err := s.CreateJob(ctx, domain.Job{Type: jobType, ScopeType: "instance", ScopeID: &instanceID, NodeID: &x.NodeID, InstanceID: &instanceID, Payload: payload})
	_, _ = s.CreateAudit(ctx, "system", jobType, "instance", &instanceID, "instance action queued")
	return j, err
}

func (s *Store) DeleteInstance(ctx context.Context, instanceID string) (domain.Instance, error) {
	var accesses int
	if err := s.db.QueryRow(ctx, `select count(*) from service_accesses where instance_id=$1 and status in ('pending','active','disabled')`, instanceID).Scan(&accesses); err != nil {
		return domain.Instance{}, err
	}
	if accesses > 0 {
		return domain.Instance{}, fmt.Errorf("instance has %d service accesses", accesses)
	}
	_, err := s.db.Exec(ctx, `update instances set status='deleted',enabled=false,updated_at=now() where id=$1`, instanceID)
	if err != nil {
		return domain.Instance{}, err
	}
	_, _ = s.CreateAudit(ctx, "system", "instance.delete", "instance", &instanceID, "instance soft deleted")
	return s.GetInstance(ctx, instanceID)
}

func (s *Store) ListClients(ctx context.Context) ([]domain.Client, error) {
	rows, err := s.db.Query(ctx, `select id,username,coalesce(display_name,''),coalesce(email,''),status,coalesce(notes,''),expires_at,created_at,updated_at from client_accounts where status <> 'deleted' order by created_at desc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Client
	for rows.Next() {
		var c domain.Client
		if err := rows.Scan(&c.ID, &c.Username, &c.DisplayName, &c.Email, &c.Status, &c.Notes, &c.ExpiresAt, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
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
		_, _ = s.db.Exec(ctx, `update service_accesses set status='revoked',updated_at=now() where client_account_id=$1`, clientID)
		_, _ = s.db.Exec(ctx, `update share_links set status='revoked' where client_account_id=$1 and status='active'`, clientID)
	}
	_, _ = s.CreateAudit(ctx, "system", "client."+status, "client", &clientID, "client status changed")
	return s.GetClient(ctx, clientID)
}

func (s *Store) ProvisionClient(ctx context.Context, clientID string, instanceIDs []string) (domain.Job, error) {
	if _, err := s.GetClient(ctx, clientID); err != nil {
		return domain.Job{}, err
	}
	if len(instanceIDs) == 0 {
		rows, err := s.db.Query(ctx, `select i.id from instances i join service_definitions sd on sd.id=i.service_definition_id where i.enabled=true and i.status in ('active','draft','provisioning') and sd.supports_accounts=true order by i.created_at asc`)
		if err != nil {
			return domain.Job{}, err
		}
		defer rows.Close()
		for rows.Next() {
			var iid string
			if err := rows.Scan(&iid); err != nil {
				return domain.Job{}, err
			}
			instanceIDs = append(instanceIDs, iid)
		}
		if err := rows.Err(); err != nil {
			return domain.Job{}, err
		}
	}
	for _, iid := range instanceIDs {
		_, _ = s.db.Exec(ctx, `insert into service_accesses(id,client_account_id,instance_id,status,provision_mode,created_at,updated_at) values($1,$2,$3,'pending','manual',now(),now()) on conflict(client_account_id, instance_id) do update set status='pending', updated_at=now()`, id.New(), clientID, iid)
	}
	j, err := s.CreateJob(ctx, domain.Job{Type: "client.provision", ScopeType: "client", ScopeID: &clientID, Payload: map[string]any{"client_id": clientID, "instance_ids": instanceIDs}})
	_, _ = s.CreateAudit(ctx, "system", "client.provision", "client", &clientID, "client provisioning queued")
	return j, err
}

func (s *Store) RevokeClient(ctx context.Context, clientID string) (domain.Job, error) {
	if _, err := s.GetClient(ctx, clientID); err != nil {
		return domain.Job{}, err
	}
	_, _ = s.db.Exec(ctx, `update client_accounts set status='revoked',updated_at=now() where id=$1`, clientID)
	_, _ = s.db.Exec(ctx, `update service_accesses set status='revoked',updated_at=now() where client_account_id=$1`, clientID)
	_, _ = s.db.Exec(ctx, `update share_links set status='revoked' where client_account_id=$1 and status='active'`, clientID)
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
		_ = json.Unmarshal(p, &a.Policy)
		_ = json.Unmarshal(m, &a.Metadata)
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
	link := domain.ShareLink{ID: id.New(), ClientAccountID: clientID, TargetType: "artifact", TargetID: targetID, Token: randomToken(24), Status: "active", ExpiresAt: time.Now().UTC().Add(ttl), CreatedAt: time.Now().UTC()}
	_, err := s.db.Exec(ctx, `insert into share_links(id,client_account_id,target_type,target_id,token,status,expires_at,created_at) values($1,$2,$3,$4,$5,$6,$7,$8)`, link.ID, link.ClientAccountID, link.TargetType, link.TargetID, link.Token, link.Status, link.ExpiresAt, link.CreatedAt)
	if err != nil {
		return link, err
	}
	_, _ = s.CreateAudit(ctx, "system", "share_link.publish", "share_link", &link.ID, "share link published")
	return link, nil
}

func (s *Store) ListShareLinks(ctx context.Context, clientID string) ([]domain.ShareLink, error) {
	query := `select id,client_account_id,target_type,target_id,token,status,expires_at,download_count,created_at from share_links`
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
		if err := rows.Scan(&l.ID, &l.ClientAccountID, &l.TargetType, &l.TargetID, &l.Token, &l.Status, &l.ExpiresAt, &l.DownloadCount, &l.CreatedAt); err != nil {
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
		returning id,client_account_id,target_type,target_id,token,status,expires_at,download_count,created_at`,
		linkID, clientID,
	).Scan(&link.ID, &link.ClientAccountID, &link.TargetType, &link.TargetID, &link.Token, &link.Status, &link.ExpiresAt, &link.DownloadCount, &link.CreatedAt)
	if err != nil {
		return domain.ShareLink{}, err
	}
	_, _ = s.CreateAudit(ctx, "system", "share_link.revoke", "share_link", &link.ID, "share link revoked")
	return link, nil
}

func (s *Store) CreateJob(ctx context.Context, j domain.Job) (domain.Job, error) {
	if strings.TrimSpace(j.Type) == "" {
		return j, errors.New("job type is required")
	}
	if j.ID == "" {
		j.ID = id.New()
	}
	if j.Status == "" {
		j.Status = "queued"
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
	if !in(j.Status, "queued", "running", "succeeded", "failed", "cancelled", "retrying") {
		return j, fmt.Errorf("unsupported job status %q", j.Status)
	}
	b, _ := json.Marshal(j.Payload)
	now := time.Now().UTC()
	j.CreatedAt = now
	_, err := s.db.Exec(ctx, `insert into jobs(id,type,scope_type,scope_id,node_id,instance_id,status,priority,payload_json,created_at) values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, j.ID, j.Type, j.ScopeType, j.ScopeID, j.NodeID, j.InstanceID, j.Status, j.Priority, b, now)
	if err != nil {
		return j, err
	}
	_, _ = s.CreateAudit(ctx, "system", "job.create", "job", &j.ID, "job queued")
	return j, nil
}

func (s *Store) ListJobs(ctx context.Context, limit int) ([]domain.Job, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
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
		_ = json.Unmarshal(b, &l.Payload)
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *Store) CancelJob(ctx context.Context, jobID string) (domain.Job, error) {
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
	return s.claimJob(ctx, workerID, `type not in ('node.inventory','node.inventory.sync','node.services.discover','node.capability.install','node.capability.verify','node.channel.probe','instance.restart','instance.apply','instance.start','instance.stop','instance.enable','instance.disable')`, nil)
}

func (s *Store) CompleteJob(ctx context.Context, idv, status string, result map[string]any) error {
	if !in(status, "succeeded", "failed", "cancelled") {
		return fmt.Errorf("unsupported completion status %q", status)
	}
	if result == nil {
		result = map[string]any{}
	}
	b, err := json.Marshal(result)
	if err != nil {
		result = map[string]any{"message": "result marshal failed", "error": err.Error()}
		b, _ = json.Marshal(result)
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
	if err := tx.QueryRow(ctx, `select type,scope_id,instance_id,status,payload_json from jobs where id=$1 for update`, idv).Scan(&jobType, &scopeID, &instanceID, &currentStatus, &payloadRaw); err != nil {
		return err
	}
	if in(currentStatus, "succeeded", "failed", "cancelled") {
		return fmt.Errorf("job already finished with status %s", currentStatus)
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

	payload := map[string]any{}
	_ = json.Unmarshal(payloadRaw, &payload)
	if payload == nil {
		payload = map[string]any{}
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
		}
	}

	if status == "succeeded" {
		switch jobType {
		case "node.bootstrap":
			if scopeID != nil {
				_, _ = s.db.Exec(ctx, `update nodes set status='bootstrapping',agent_status='starting',updated_at=now() where id=$1 and status='bootstrapping'`, *scopeID)
			}
		case "node.agent.rotate_token":
			if scopeID != nil {
				newToken := stringify(payload["new_agent_token"])
				newHint := stringify(payload["new_token_hint"])
				if newToken != "" {
					_, _ = s.db.Exec(ctx, `update node_agents
						set agent_token_hash=$2,
						    token_hint=$3,
						    status='active',
						    revoked_at=null
						where node_id=$1`, *scopeID, hashToken(newToken), newHint)
					_, _ = s.db.Exec(ctx, `update nodes set agent_status='online',updated_at=now() where id=$1`, *scopeID)
				}
			}
		case "instance.apply", "instance.start", "instance.enable":
			if instanceID != nil {
				_, _ = s.db.Exec(ctx, `update instances set status='active',enabled=true,last_applied_revision_id=current_revision_id,updated_at=now() where id=$1`, *instanceID)
				_, _ = s.db.Exec(ctx, `update service_accesses set status='active',updated_at=now() where instance_id=$1 and status='pending'`, *instanceID)
			}
		case "instance.stop", "instance.disable":
			if instanceID != nil {
				_, _ = s.db.Exec(ctx, `update instances set status='disabled',enabled=false,updated_at=now() where id=$1`, *instanceID)
			}
		case "client.revoke":
			if scopeID != nil {
				_, _ = s.db.Exec(ctx, `update client_accounts set status='revoked',updated_at=now() where id=$1`, *scopeID)
				_, _ = s.db.Exec(ctx, `update service_accesses set status='revoked',updated_at=now() where client_account_id=$1`, *scopeID)
			}
		}
	} else if jobType == "node.bootstrap" && scopeID != nil {
		_, _ = s.db.Exec(ctx, `update nodes set status='draft',updated_at=now() where id=$1 and status='bootstrapping'`, *scopeID)
	}

	if jobType == "node.bootstrap" {
		if _, err := s.db.Exec(ctx, `update node_bootstrap_runs set status=$2,result_payload_json=$3,finished_at=now() where job_id=$1`, idv, status, resultJSON); err != nil {
			return err
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
	_, err := s.db.Exec(ctx, `insert into node_capability_install_events(id,node_id,job_id,capability_code,strategy,status,summary,payload_json,created_at) values($1,$2,$3,$4,$5,$6,$7,$8,now())`, id.New(), nodeID, jobID, capCode, strategy, status, summary, payloadJSON)
	return err
}

func (s *Store) upsertNodeCapability(ctx context.Context, nodeID, capCode, version, status, source string) error {
	if capCode == "" {
		return nil
	}
	if status == "" {
		status = "unknown"
	}
	if source == "" {
		source = "installer"
	}
	_, err := s.db.Exec(ctx, `insert into node_capabilities(id,node_id,capability_code,version,status,detected_at,source) values($1,$2,$3,$4,$5,now(),$6) on conflict(node_id, capability_code) do update set version=excluded.version,status=excluded.status,detected_at=excluded.detected_at,source=excluded.source`, id.New(), nodeID, capCode, version, status, source)
	return err
}

func (s *Store) AddJobLog(ctx context.Context, jobID, level, msg string, payload map[string]any) error {
	if !in(level, "debug", "info", "warn", "error") {
		level = "info"
	}
	if payload == nil {
		payload = map[string]any{}
	}
	b, _ := json.Marshal(payload)
	_, err := s.db.Exec(ctx, `insert into job_logs(id,job_id,level,message,payload_json,created_at) values($1,$2,$3,$4,$5,now())`, id.New(), jobID, level, msg, b)
	return err
}

func (s *Store) AgentNextJob(ctx context.Context, nodeRef string) (domain.Job, bool, error) {
	var nodeID string
	if err := s.db.QueryRow(ctx, `select id from nodes where (id::text=$1 or name=$1) and status <> 'retired'`, nodeRef).Scan(&nodeID); err != nil {
		return domain.Job{}, false, err
	}
	where := `(node_id=$1 or node_id is null) and type in ('node.inventory','node.inventory.sync','node.services.discover','node.capability.install','node.capability.verify','node.channel.probe','instance.restart','instance.apply','instance.start','instance.stop','instance.enable','instance.disable')`
	return s.claimJob(ctx, "agent:"+nodeID, where, []any{nodeID})
}

func (s *Store) claimJob(ctx context.Context, owner, where string, args []any) (domain.Job, bool, error) {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.Job{}, false, err
	}
	defer tx.Rollback(ctx)

	_, _ = tx.Exec(ctx, `delete from resource_locks where expires_at < now()`)
	_, _ = tx.Exec(ctx, `update jobs set status='retrying', locked_by=null, locked_until=null where status='running' and locked_until is not null and locked_until < now()`)

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
		_, err = tx.Exec(ctx, `update jobs set status='running',started_at=coalesce(started_at,$2),locked_by=$3,locked_until=$2 + interval '2 minutes' where id=$1 and status in ('queued','retrying')`, j.ID, now, owner)
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
		lockedUntil := now.Add(2 * time.Minute)
		j.LockedUntil = &lockedUntil
		if j.StartedAt == nil {
			j.StartedAt = &now
		}
		_ = s.AddJobLog(context.Background(), j.ID, "info", "job claimed", map[string]any{"owner": owner})
		return j, true, nil
	}
	return domain.Job{}, false, nil
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
	_, err := s.db.Exec(ctx, `insert into nodes(id,name,kind,role,status,address,os_family,os_version,architecture,execution_mode,agent_status,created_at,updated_at) values($1,'local','local','egress','online','127.0.0.1','linux','unknown','amd64','local_managed','unknown',now(),now()) on conflict(name) do nothing`, id.New())
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
	var out []domain.NodeEnrollmentToken
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
	_, err = tx.Exec(ctx, `insert into node_agents(id,node_id,status,agent_version,protocol_version,fingerprint,registered_at,last_seen_at,agent_token_hash,token_hint) values($1,$2,'active','alpha','v1',$3,$4,$4,$5,$6) on conflict(node_id) do update set status='active', agent_version='alpha', protocol_version='v1', fingerprint=excluded.fingerprint, last_seen_at=excluded.last_seen_at, agent_token_hash=excluded.agent_token_hash, token_hint=excluded.token_hint, revoked_at=null`, id.New(), nodeID, fingerprint, now, agentHash, agentHint)
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
	now := time.Now().UTC()
	cmd, err := s.db.Exec(ctx, `update nodes set status='online',agent_status='online',last_heartbeat_at=$2,updated_at=$2 where id=$1 and status <> 'retired'`, nodeID, now)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	_, _ = s.db.Exec(ctx, `update node_agents set last_seen_at=$2,status='active' where node_id=$1`, nodeID, now)
	return nil
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
	_ = json.Unmarshal(p, &j.Payload)
	_ = json.Unmarshal(r, &j.Result)
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
	_, err := tx.Exec(ctx, `insert into resource_locks(id,resource_type,resource_id,lock_kind,job_id,acquired_at,expires_at) values($1,$2,$3,$4,$5,now(),now()+interval '2 minutes')`, id.New(), resourceType, resourceID, lockKind, j.ID)
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
	if j.ScopeID != nil && *j.ScopeID != "" {
		switch {
		case strings.HasPrefix(j.Type, "client."):
			return "client", *j.ScopeID, "provision", true
		case strings.HasPrefix(j.Type, "node."):
			return "node", *j.ScopeID, "bootstrap", true
		}
	}
	if j.NodeID != nil && *j.NodeID != "" && strings.HasPrefix(j.Type, "node.") {
		return "node", *j.NodeID, "bootstrap", true
	}
	return "", "", "", false
}

type nodeServiceDiscoveryScanner interface{ Scan(dest ...any) error }

func scanNodeServiceDiscovery(row nodeServiceDiscoveryScanner) (domain.NodeServiceDiscovery, error) {
	var x domain.NodeServiceDiscovery
	var b []byte
	if err := row.Scan(&x.ID, &x.NodeID, &x.ServiceCode, &x.Name, &x.SystemdUnit, &x.ConfigPath, &x.Status, &x.Source, &x.Confidence, &x.EndpointHost, &x.EndpointPort, &x.ManagedInstanceID, &b, &x.DetectedAt); err != nil {
		return domain.NodeServiceDiscovery{}, err
	}
	_ = json.Unmarshal(b, &x.Payload)
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
	_ = json.Unmarshal(b, &x.Payload)
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

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte(`{}`)
	}
	return b
}

func normalizeCapabilityCode(code string) string {
	code = strings.ToLower(strings.TrimSpace(code))
	switch code {
	case "xray", "xray-core", "xray_core":
		return "xray-core"
	case "openvpn":
		return "openvpn"
	case "wireguard", "wg", "wg-quick":
		return "wireguard"
	case "mtproto", "telegram-mtproto":
		return "mtproto"
	case "nginx":
		return "nginx"
	case "ipsec", "strongswan":
		return "ipsec"
	case "http_proxy", "http-proxy", "squid":
		return "http_proxy"
	case "l2tp", "l2tpd", "xl2tpd":
		return "xl2tpd"
	case "shadowsocks", "shadowsocks-libev", "ss-server":
		return "shadowsocks"
	default:
		return code
	}
}

func validInstallStrategy(serviceCode, strategy string) bool {
	switch serviceCode {
	case "nginx":
		return in(strategy, "nginx_org_repo", "ubuntu_repo", "manual_present")
	case "xray-core":
		return in(strategy, "xtls_install_release", "manual_present")
	case "openvpn", "wireguard", "ipsec", "http_proxy", "xl2tpd", "shadowsocks":
		return in(strategy, "ubuntu_repo", "manual_present")
	default:
		return false
	}
}

func normalizeInstanceRuntimeCode(serviceCode string) string {
	serviceCode = strings.ToLower(strings.TrimSpace(serviceCode))
	switch serviceCode {
	case "xray", "xray-core", "xray_core":
		return "xray-core"
	case "openvpn":
		return "openvpn"
	case "wireguard", "wg", "wg-quick":
		return "wireguard"
	case "mtproto", "telegram-mtproto":
		return "mtproto"
	case "nginx":
		return "nginx"
	case "ipsec", "strongswan":
		return "ipsec"
	case "http_proxy", "http-proxy", "squid":
		return "http_proxy"
	case "l2tp", "l2tpd", "xl2tpd":
		return "xl2tpd"
	case "shadowsocks", "shadowsocks-libev", "ss-server":
		return "shadowsocks"
	default:
		return serviceCode
	}
}

func serviceDefaultSystemdUnit(serviceCode, slug string) string {
	switch normalizeInstanceRuntimeCode(serviceCode) {
	case "xray-core":
		if slug == "" {
			slug = "xray"
		}
		return "megavpn-xray-" + slug
	case "nginx":
		return "nginx"
	case "openvpn":
		if slug != "" {
			return "openvpn-server@" + slug
		}
		return "openvpn-server@server"
	case "mtproto":
		if slug == "" {
			slug = "mtproto"
		}
		return "megavpn-mtproto-" + slug
	case "http_proxy":
		if slug == "" {
			slug = "proxy"
		}
		return "megavpn-http-proxy-" + slug
	case "wireguard":
		if slug != "" {
			return "wg-quick@" + slug
		}
		return "wg-quick@wg0"
	case "ipsec":
		return "strongswan-starter"
	case "xl2tpd":
		return "xl2tpd"
	case "shadowsocks":
		if slug == "" {
			slug = "shadowsocks"
		}
		return "megavpn-shadowsocks-" + slug
	default:
		if slug == "" {
			slug = "instance"
		}
		return "megavpn-" + slug
	}
}

func (s *Store) latestInstanceSpec(ctx context.Context, instanceID string) (map[string]any, error) {
	var raw []byte
	err := s.db.QueryRow(ctx, `select spec_json from instance_revisions where instance_id=$1 order by revision_no desc limit 1`, instanceID).Scan(&raw)
	if err != nil {
		return nil, err
	}
	var spec map[string]any
	_ = json.Unmarshal(raw, &spec)
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
	rows, err := s.db.Query(ctx, `select id,instance_id,source,status,coalesce(rendered_hash,''),revision_no,spec_json,coalesce(validation_errors_json,'[]'::jsonb),created_at,applied_at
		from instance_revisions
		where instance_id=$1
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
		if err := rows.Scan(&rev.ID, &rev.InstanceID, &rev.Source, &rev.Status, &rev.RenderedHash, &rev.RevisionNo, &specRaw, &validationRaw, &rev.CreatedAt, &rev.AppliedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(specRaw, &rev.Spec)
		_ = json.Unmarshal(validationRaw, &rev.ValidationErrors)
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
	var revisionNo int
	_ = s.db.QueryRow(ctx, `select coalesce(max(revision_no),0)+1 from instance_revisions where instance_id=$1`, instanceID).Scan(&revisionNo)
	b, _ := json.Marshal(spec)
	rev := domain.InstanceRevision{ID: id.New(), InstanceID: instanceID, RevisionNo: revisionNo, Source: source, Status: status, Spec: spec, CreatedAt: time.Now().UTC()}
	if _, err := s.db.Exec(ctx, `insert into instance_revisions(id,instance_id,revision_no,source,status,spec_json,created_at) values($1,$2,$3,$4,$5,$6,now())`, rev.ID, rev.InstanceID, rev.RevisionNo, rev.Source, rev.Status, b); err != nil {
		return rev, err
	}
	_, err := s.db.Exec(ctx, `update instances set current_revision_id=$2,updated_at=now() where id=$1`, instanceID, rev.ID)
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
