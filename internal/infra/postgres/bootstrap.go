package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/jobschema"
	"github.com/rtis-emc2/megavpn/internal/platform/id"
)

var (
	sshUserPattern          = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]{0,63}$`)
	sshHostPattern          = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*\.?$`)
	sshHostKeySHA256Pattern = regexp.MustCompile(`^SHA256:[A-Za-z0-9+/]{32,64}={0,2}$`)
)

func (s *Store) ListNodeAccessMethods(ctx context.Context, nodeID string) ([]domain.NodeAccessMethod, error) {
	rows, err := s.db.Query(ctx, `select id,node_id,method,is_enabled,coalesce(ssh_host,''),coalesce(ssh_port,0),coalesce(ssh_user,''),coalesce(ssh_host_key_sha256,''),coalesce(auth_type,''),secret_ref_id,created_at,updated_at from node_access_methods where node_id=$1 order by created_at asc`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.NodeAccessMethod{}
	for rows.Next() {
		var x domain.NodeAccessMethod
		if err := rows.Scan(&x.ID, &x.NodeID, &x.Method, &x.IsEnabled, &x.SSHHost, &x.SSHPort, &x.SSHUser, &x.SSHHostKeySHA256, &x.AuthType, &x.SecretRefID, &x.CreatedAt, &x.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) ReplaceNodeAccessMethods(ctx context.Context, nodeID string, methods []domain.NodeAccessMethod) ([]domain.NodeAccessMethod, error) {
	if _, err := s.GetNode(ctx, nodeID); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	for i := range methods {
		if methods[i].ID == "" {
			methods[i].ID = id.New()
		}
		methods[i].NodeID = nodeID
		methods[i].Method = strings.TrimSpace(methods[i].Method)
		methods[i].AuthType = strings.TrimSpace(methods[i].AuthType)
		methods[i].SSHHost = strings.TrimSpace(methods[i].SSHHost)
		methods[i].SSHUser = strings.TrimSpace(methods[i].SSHUser)
		methods[i].SSHHostKeySHA256 = strings.TrimSpace(methods[i].SSHHostKeySHA256)
		if methods[i].SecretRefID != nil {
			secretRefID := strings.TrimSpace(*methods[i].SecretRefID)
			if secretRefID == "" {
				methods[i].SecretRefID = nil
			} else {
				methods[i].SecretRefID = &secretRefID
			}
		}
		if methods[i].Method == "" {
			return nil, errors.New("access method is required")
		}
		if !in(methods[i].Method, "local", "ssh", "manual_bundle", "agent") {
			return nil, fmt.Errorf("unsupported access method %q", methods[i].Method)
		}
		if methods[i].Method == "ssh" {
			if methods[i].SSHHost == "" {
				return nil, errors.New("ssh_host is required for ssh access method")
			}
			if methods[i].SSHUser == "" {
				return nil, errors.New("ssh_user is required for ssh access method")
			}
			if !sshUserPattern.MatchString(methods[i].SSHUser) {
				return nil, errors.New("ssh_user contains unsafe characters")
			}
			if !isSafeSSHAccessHost(methods[i].SSHHost) {
				return nil, errors.New("ssh_host contains unsafe characters")
			}
			if methods[i].SSHHostKeySHA256 == "" || !sshHostKeySHA256Pattern.MatchString(methods[i].SSHHostKeySHA256) {
				return nil, errors.New("ssh_host_key_sha256 is required for ssh bootstrap")
			}
			if methods[i].SSHPort == 0 {
				methods[i].SSHPort = 22
			}
			if methods[i].AuthType == "" {
				methods[i].AuthType = "ssh_key"
			}
			if methods[i].AuthType == "none" {
				return nil, errors.New("auth_type none is not allowed for ssh access method")
			}
			if methods[i].SecretRefID == nil {
				return nil, errors.New("secret_ref_id is required for ssh access method")
			}
			ref, err := s.GetSecretRef(ctx, *methods[i].SecretRefID)
			if err != nil {
				return nil, errors.New("secret_ref_id does not exist")
			}
			if methods[i].AuthType == "ssh_key" {
				if err := validateSSHAccessSecretRef(nodeID, ref); err != nil {
					return nil, err
				}
			}
		} else {
			methods[i].SSHPort = 0
			methods[i].SSHHost = ""
			methods[i].SSHUser = ""
			methods[i].SSHHostKeySHA256 = ""
			methods[i].SecretRefID = nil
			if methods[i].AuthType == "" {
				methods[i].AuthType = "none"
			}
		}
		if !in(methods[i].AuthType, "ssh_key", "password", "token", "none") {
			return nil, fmt.Errorf("unsupported auth_type %q", methods[i].AuthType)
		}
		methods[i].CreatedAt = now
		methods[i].UpdatedAt = now
	}

	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `delete from node_access_methods where node_id=$1`, nodeID); err != nil {
		return nil, err
	}
	for _, x := range methods {
		if _, err := tx.Exec(ctx, `insert into node_access_methods(id,node_id,method,is_enabled,ssh_host,ssh_port,ssh_user,ssh_host_key_sha256,auth_type,secret_ref_id,created_at,updated_at) values($1,$2,$3,$4,nullif($5,''),nullif($6,0),nullif($7,''),nullif($8,''),nullif($9,''),$10,$11,$12)`,
			x.ID, x.NodeID, x.Method, x.IsEnabled, x.SSHHost, x.SSHPort, x.SSHUser, x.SSHHostKeySHA256, x.AuthType, x.SecretRefID, x.CreatedAt, x.UpdatedAt); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	_, _ = s.CreateAudit(ctx, "system", "node.access_methods.replace", "node", &nodeID, "node access methods updated")
	return s.ListNodeAccessMethods(ctx, nodeID)
}

func isSafeSSHAccessHost(host string) bool {
	host = strings.TrimSpace(host)
	if host == "" || strings.HasPrefix(host, "-") || strings.ContainsAny(host, " \t\r\n;{}") {
		return false
	}
	ipLiteral := host
	if strings.HasPrefix(host, "[") || strings.HasSuffix(host, "]") {
		if !strings.HasPrefix(host, "[") || !strings.HasSuffix(host, "]") {
			return false
		}
		ipLiteral = strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")
	}
	if _, err := netip.ParseAddr(ipLiteral); err == nil {
		return true
	}
	return sshHostPattern.MatchString(host)
}

func validateSSHAccessSecretRef(nodeID string, ref domain.SecretRef) error {
	if strings.TrimSpace(ref.SecretType) != "ssh_key" {
		return errors.New("secret_ref_id must reference an ssh_key secret")
	}
	if metaNodeID := strings.TrimSpace(stringify(ref.Meta["node_id"])); metaNodeID != "" && metaNodeID != strings.TrimSpace(nodeID) {
		return errors.New("secret_ref_id belongs to another node")
	}
	if metaAuthType := strings.TrimSpace(stringify(ref.Meta["auth_type"])); metaAuthType != "" && metaAuthType != "ssh_key" {
		return errors.New("secret_ref_id auth_type is not ssh_key")
	}
	return nil
}

func (s *Store) CreateNodeBootstrapJob(ctx context.Context, nodeID, bootstrapMode string, options map[string]any) (domain.Job, domain.NodeBootstrapRun, error) {
	n, err := s.GetNode(ctx, nodeID)
	if err != nil {
		return domain.Job{}, domain.NodeBootstrapRun{}, err
	}
	bootstrapMode = strings.TrimSpace(bootstrapMode)
	if bootstrapMode == "" {
		bootstrapMode = "ssh_bootstrap"
	}
	if !in(bootstrapMode, "ssh_bootstrap", "manual_bundle") {
		return domain.Job{}, domain.NodeBootstrapRun{}, fmt.Errorf("unsupported bootstrap mode %q", bootstrapMode)
	}
	methods, err := s.ListNodeAccessMethods(ctx, nodeID)
	if err != nil {
		return domain.Job{}, domain.NodeBootstrapRun{}, err
	}
	if bootstrapMode == "ssh_bootstrap" {
		ok := false
		for _, m := range methods {
			if m.IsEnabled && m.Method == "ssh" {
				ok = true
				break
			}
		}
		if !ok {
			return domain.Job{}, domain.NodeBootstrapRun{}, errors.New("enabled ssh access method is required for ssh_bootstrap")
		}
		if err := s.ensureFirewallAllowsSSHBootstrap(ctx, nodeID, methods); err != nil {
			return domain.Job{}, domain.NodeBootstrapRun{}, err
		}
	}

	payload := map[string]any{
		"node_id":         nodeID,
		"node_name":       n.Name,
		"node_role":       n.Role,
		"bootstrap_mode":  bootstrapMode,
		"execution_mode":  n.ExecutionMode,
		"enabled_methods": enabledMethodSummary(methods),
	}
	for key, value := range options {
		payload[key] = value
	}
	payload, err = jobschema.Normalize("node.bootstrap", payload)
	if err != nil {
		return domain.Job{}, domain.NodeBootstrapRun{}, err
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.Job{}, domain.NodeBootstrapRun{}, err
	}
	defer tx.Rollback(ctx)

	jobID := id.New()
	now := time.Now().UTC()
	job := domain.Job{
		ID:        jobID,
		Type:      "node.bootstrap",
		ScopeType: "node",
		ScopeID:   &nodeID,
		NodeID:    &nodeID,
		Status:    "queued",
		Priority:  40,
		Payload:   payload,
		CreatedAt: now,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return domain.Job{}, domain.NodeBootstrapRun{}, fmt.Errorf("marshal bootstrap job payload: %w", err)
	}
	if _, err := tx.Exec(ctx, `insert into jobs(id,type,scope_type,scope_id,node_id,instance_id,status,priority,payload_json,created_at) values($1,$2,$3,$4,$5,null,$6,$7,$8,$9)`,
		job.ID, job.Type, job.ScopeType, job.ScopeID, job.NodeID, job.Status, job.Priority, b, job.CreatedAt); err != nil {
		return domain.Job{}, domain.NodeBootstrapRun{}, err
	}

	run := domain.NodeBootstrapRun{
		ID:             id.New(),
		NodeID:         nodeID,
		JobID:          &jobID,
		Status:         "queued",
		BootstrapMode:  bootstrapMode,
		RequestPayload: payload,
		CreatedAt:      now,
	}
	if _, err := tx.Exec(ctx, `insert into node_bootstrap_runs(id,node_id,job_id,status,bootstrap_mode,request_payload_json,created_at) values($1,$2,$3,$4,$5,$6,$7)`,
		run.ID, run.NodeID, run.JobID, run.Status, run.BootstrapMode, mustJSON(run.RequestPayload), run.CreatedAt); err != nil {
		return domain.Job{}, domain.NodeBootstrapRun{}, err
	}

	if _, err := tx.Exec(ctx, `update nodes set status='bootstrapping',updated_at=now() where id=$1 and status='draft'`, nodeID); err != nil {
		return domain.Job{}, domain.NodeBootstrapRun{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.Job{}, domain.NodeBootstrapRun{}, err
	}
	_, _ = s.CreateAudit(ctx, "system", "node.bootstrap", "node", &nodeID, "node bootstrap queued")
	return job, run, nil
}

func (s *Store) ensureFirewallAllowsSSHBootstrap(ctx context.Context, nodeID string, methods []domain.NodeAccessMethod) error {
	ports := sshBootstrapPorts(methods)
	if len(ports) == 0 {
		return nil
	}
	status, enforcement, policyID, err := s.nodeFirewallApplyState(ctx, nodeID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("check node firewall state before bootstrap: %w", err)
	}
	if status != "applied" || enforcement != "enforced" {
		return nil
	}
	if policyID == "" {
		return fmt.Errorf("node firewall is enforced but has no policy reference; use Firewall -> Node apply -> Disable before SSH bootstrap or reinstall")
	}
	for _, port := range ports {
		allowed, err := s.firewallPolicyAllowsStrictSSHBootstrapPort(ctx, policyID, port)
		if err != nil {
			return fmt.Errorf("check node firewall SSH allow rules before bootstrap: %w", err)
		}
		if allowed {
			return nil
		}
	}
	return fmt.Errorf("node firewall is enforced and does not include an active input accept rule for SSH bootstrap port(s) %s; use Firewall -> Node apply -> Disable before bootstrap/reinstall, or add a source-scoped SSH allow rule for trusted_control_plane/trusted_operators and apply the policy", joinIntList(ports))
}

func sshBootstrapPorts(methods []domain.NodeAccessMethod) []int {
	seen := map[int]bool{}
	out := []int{}
	for _, method := range methods {
		if !method.IsEnabled || method.Method != "ssh" {
			continue
		}
		port := method.SSHPort
		if port == 0 {
			port = 22
		}
		if port < 1 || port > 65535 || seen[port] {
			continue
		}
		seen[port] = true
		out = append(out, port)
	}
	return out
}

func (s *Store) nodeFirewallApplyState(ctx context.Context, nodeID string) (status, enforcement, policyID string, err error) {
	err = s.db.QueryRow(ctx, `select status,coalesce(observed_json->>'default_policy_enforcement',''),coalesce(policy_id::text,'') from firewall_node_state where node_id=$1`, nodeID).
		Scan(&status, &enforcement, &policyID)
	status = strings.ToLower(strings.TrimSpace(status))
	enforcement = strings.ToLower(strings.TrimSpace(enforcement))
	policyID = strings.TrimSpace(policyID)
	return status, enforcement, policyID, err
}

func (s *Store) firewallPolicyAllowsInputTCPPort(ctx context.Context, policyID string, port int) (bool, error) {
	rows, err := s.db.Query(ctx, `select protocol,coalesce(dst_ports,''),coalesce(src_cidr,''),coalesce(src_list_id::text,''),state_match
from firewall_rules
where policy_id=$1
  and status='active'
  and enabled=true
  and chain='input'
  and action='accept'
  and protocol in ('tcp','any')`, policyID)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var protocol, dstPorts, srcCIDR, srcListID string
		var stateMatch []string
		if err := rows.Scan(&protocol, &dstPorts, &srcCIDR, &srcListID, &stateMatch); err != nil {
			return false, err
		}
		if !firewallRuleAllowsNewState(stateMatch) {
			continue
		}
		if protocol == "tcp" && !firewallPortListContains(dstPorts, port) {
			continue
		}
		if protocol == "any" && strings.TrimSpace(dstPorts) != "" {
			continue
		}
		if strings.TrimSpace(srcListID) != "" {
			hasEntries, err := s.firewallAddressListHasRenderableEntries(ctx, srcListID)
			if err != nil {
				return false, err
			}
			if !hasEntries {
				continue
			}
		}
		if strings.TrimSpace(srcCIDR) != "" {
			if _, err := netip.ParsePrefix(strings.TrimSpace(srcCIDR)); err != nil {
				continue
			}
		}
		return true, nil
	}
	return false, rows.Err()
}

func (s *Store) firewallPolicyAllowsStrictSSHBootstrapPort(ctx context.Context, policyID string, port int) (bool, error) {
	managedSources, err := s.firewallHasRenderableTrustedManagementSources(ctx)
	if err != nil {
		return false, err
	}
	if managedSources {
		return true, nil
	}
	rows, err := s.db.Query(ctx, `select r.protocol,coalesce(r.dst_ports,''),coalesce(r.src_cidr,''),coalesce(r.src_list_id::text,''),coalesce(l.key,''),r.state_match
from firewall_rules r
left join firewall_address_lists l on l.id=r.src_list_id
where r.policy_id=$1
  and r.status='active'
  and r.enabled=true
  and r.chain='input'
  and r.action='accept'
  and r.protocol in ('tcp','any')`, policyID)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var protocol, dstPorts, srcCIDR, srcListID, srcListKey string
		var stateMatch []string
		if err := rows.Scan(&protocol, &dstPorts, &srcCIDR, &srcListID, &srcListKey, &stateMatch); err != nil {
			return false, err
		}
		if !firewallRuleAllowsNewState(stateMatch) {
			continue
		}
		if protocol == "tcp" && !firewallPortListContains(dstPorts, port) {
			continue
		}
		if protocol == "any" && strings.TrimSpace(dstPorts) != "" {
			continue
		}
		if firewallRuleHasTrustedControlSource(ctx, s, srcCIDR, srcListID, srcListKey) {
			return true, nil
		}
	}
	return false, rows.Err()
}

func firewallRuleHasTrustedControlSource(ctx context.Context, s *Store, srcCIDR, srcListID, srcListKey string) bool {
	if strings.TrimSpace(srcCIDR) != "" {
		if prefix, err := netip.ParsePrefix(strings.TrimSpace(srcCIDR)); err == nil && !firewallAddressValueIsAny(prefix.String(), "cidr") {
			return true
		}
		return false
	}
	if strings.TrimSpace(srcListID) == "" {
		return false
	}
	srcListKey = strings.ToLower(strings.TrimSpace(srcListKey))
	if srcListKey != "trusted_control_plane" && srcListKey != "trusted_operators" {
		return false
	}
	hasEntries, err := s.firewallAddressListHasRenderableNonAnyEntries(ctx, srcListID, srcListKey)
	return err == nil && hasEntries
}

func firewallRuleAllowsNewState(values []string) bool {
	if len(values) == 0 {
		return true
	}
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), "new") {
			return true
		}
	}
	return false
}

func firewallPortListContains(ports string, target int) bool {
	if strings.TrimSpace(ports) == "" {
		return true
	}
	for _, part := range strings.Split(ports, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "-") {
			left, right, _ := strings.Cut(part, "-")
			from, err := strconv.Atoi(strings.TrimSpace(left))
			if err != nil {
				continue
			}
			to, err := strconv.Atoi(strings.TrimSpace(right))
			if err != nil {
				continue
			}
			if from <= target && target <= to {
				return true
			}
			continue
		}
		port, err := strconv.Atoi(part)
		if err != nil {
			continue
		}
		if port == target {
			return true
		}
	}
	return false
}

func (s *Store) firewallAddressListHasRenderableEntries(ctx context.Context, listID string) (bool, error) {
	listID = strings.TrimSpace(listID)
	if listID == "" {
		return false, nil
	}
	var ok bool
	err := s.db.QueryRow(ctx, `select exists(
	select 1
	from firewall_address_entries e
	join firewall_address_lists l on l.id=e.list_id
	where e.list_id=$1::uuid
	  and e.status='active'
	  and l.status='active'
	  and e.value_type in ('cidr','address','range')
)`, listID).Scan(&ok)
	return ok, err
}

func joinIntList(values []int) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, strconv.Itoa(value))
	}
	return strings.Join(parts, ",")
}

func (s *Store) ListNodeBootstrapRuns(ctx context.Context, nodeID string, limit int) ([]domain.NodeBootstrapRun, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.Query(ctx, `select id,node_id,job_id,status,bootstrap_mode,request_payload_json,coalesce(result_payload_json,'{}'::jsonb),started_at,finished_at,created_by,created_at from node_bootstrap_runs where node_id=$1 order by created_at desc limit $2`, nodeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.NodeBootstrapRun{}
	for rows.Next() {
		x, err := scanNodeBootstrapRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) GetNodeBootstrapRun(ctx context.Context, nodeID string, runID string) (domain.NodeBootstrapRun, error) {
	row := s.db.QueryRow(ctx, `select id,node_id,job_id,status,bootstrap_mode,request_payload_json,coalesce(result_payload_json,'{}'::jsonb),started_at,finished_at,created_by,created_at from node_bootstrap_runs where node_id=$1 and id=$2`, nodeID, strings.TrimSpace(runID))
	return scanNodeBootstrapRun(row)
}

type nodeBootstrapRunScanner interface {
	Scan(dest ...any) error
}

func scanNodeBootstrapRun(scanner nodeBootstrapRunScanner) (domain.NodeBootstrapRun, error) {
	var x domain.NodeBootstrapRun
	var reqRaw, resRaw []byte
	if err := scanner.Scan(&x.ID, &x.NodeID, &x.JobID, &x.Status, &x.BootstrapMode, &reqRaw, &resRaw, &x.StartedAt, &x.FinishedAt, &x.CreatedBy, &x.CreatedAt); err != nil {
		return domain.NodeBootstrapRun{}, err
	}
	if err := decodeJSONField(reqRaw, &x.RequestPayload, "node_bootstrap_runs.request_payload_json"); err != nil {
		return domain.NodeBootstrapRun{}, err
	}
	if err := decodeJSONField(resRaw, &x.ResultPayload, "node_bootstrap_runs.result_payload_json"); err != nil {
		return domain.NodeBootstrapRun{}, err
	}
	if x.RequestPayload == nil {
		x.RequestPayload = map[string]any{}
	}
	if x.ResultPayload == nil {
		x.ResultPayload = map[string]any{}
	}
	return x, nil
}

func enabledMethodSummary(methods []domain.NodeAccessMethod) []map[string]any {
	out := make([]map[string]any, 0, len(methods))
	for _, m := range methods {
		if !m.IsEnabled {
			continue
		}
		out = append(out, map[string]any{
			"id":            m.ID,
			"method":        m.Method,
			"ssh_host":      m.SSHHost,
			"ssh_port":      m.SSHPort,
			"ssh_user":      m.SSHUser,
			"auth_type":     m.AuthType,
			"secret_ref_id": m.SecretRefID,
		})
	}
	return out
}
