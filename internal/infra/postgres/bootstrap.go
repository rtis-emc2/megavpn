package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"regexp"
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
			if _, err := s.GetSecretRef(ctx, *methods[i].SecretRefID); err != nil {
				return nil, errors.New("secret_ref_id does not exist")
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
	b, _ := json.Marshal(payload)
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
		var x domain.NodeBootstrapRun
		var reqRaw, resRaw []byte
		if err := rows.Scan(&x.ID, &x.NodeID, &x.JobID, &x.Status, &x.BootstrapMode, &reqRaw, &resRaw, &x.StartedAt, &x.FinishedAt, &x.CreatedBy, &x.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(reqRaw, &x.RequestPayload)
		_ = json.Unmarshal(resRaw, &x.ResultPayload)
		if x.RequestPayload == nil {
			x.RequestPayload = map[string]any{}
		}
		if x.ResultPayload == nil {
			x.ResultPayload = map[string]any{}
		}
		out = append(out, x)
	}
	return out, rows.Err()
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
