package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/platform/id"
)

func (s *Store) CreateNodeAgentTokenRotateJob(ctx context.Context, nodeID string) (domain.Job, error) {
	if _, err := s.GetNode(ctx, nodeID); err != nil {
		return domain.Job{}, err
	}
	var active bool
	if err := s.db.QueryRow(ctx, `select exists(select 1 from node_agents where node_id=$1 and revoked_at is null and status='active')`, nodeID).Scan(&active); err != nil {
		return domain.Job{}, err
	}
	if !active {
		return domain.Job{}, errors.New("active agent identity is required before token rotation")
	}
	newToken := randomToken(32)
	newTokenHint := tokenHint(newToken)
	newTokenRef, err := s.CreateSecretRef(ctx, "api_token", []byte(newToken), map[string]any{
		"scope":    "node_agent_token_rotation",
		"node_id":  nodeID,
		"material": "new_agent_token",
		"hint":     newTokenHint,
	})
	if err != nil {
		if errors.Is(err, ErrSecretServiceUnavailable) {
			return domain.Job{}, errors.New("agent token rotation requires MEGAVPN_MASTER_KEY_PATH")
		}
		return domain.Job{}, err
	}
	job := domain.Job{
		ID:        id.New(),
		Type:      "node.agent.rotate_token",
		ScopeType: "node",
		ScopeID:   &nodeID,
		NodeID:    &nodeID,
		Status:    "queued",
		Priority:  15,
		Payload: map[string]any{
			"node_id":                       nodeID,
			"new_agent_token_secret_ref_id": newTokenRef.ID,
			"new_agent_token_hash":          hashToken(newToken),
			"new_token_hint":                newTokenHint,
		},
		CreatedAt: time.Now().UTC(),
	}
	job, err = s.CreateJob(ctx, job)
	if err != nil {
		return job, err
	}
	_, _ = s.CreateAudit(ctx, "system", "node.agent_token.rotate", "node", &nodeID, "agent token rotation queued")
	return job, nil
}

func (s *Store) RotateNodeEnrollmentToken(ctx context.Context, nodeID string, ttl time.Duration) (domain.NodeEnrollmentToken, error) {
	token, err := s.CreateNodeEnrollmentToken(ctx, nodeID, ttl)
	if err != nil {
		return token, err
	}
	_, _ = s.CreateAudit(ctx, "system", "node.enrollment_token.rotate", "node", &nodeID, "node enrollment token rotated")
	return token, nil
}

func (s *Store) RevokeNodeAgentIdentity(ctx context.Context, nodeID string) (domain.Node, error) {
	if _, err := s.GetNode(ctx, nodeID); err != nil {
		return domain.Node{}, err
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.Node{}, err
	}
	defer tx.Rollback(ctx)

	now := time.Now().UTC()
	if _, err := tx.Exec(ctx, `update node_agents
		set status='revoked',
		    revoked_at=$2,
		    agent_token_hash=null
		where node_id=$1`, nodeID, now); err != nil {
		return domain.Node{}, err
	}
	if _, err := tx.Exec(ctx, `update node_enrollment_tokens
		set status='revoked'
		where node_id=$1 and status='active'`, nodeID); err != nil {
		return domain.Node{}, err
	}
	if _, err := tx.Exec(ctx, `update nodes
		set agent_status='revoked',
		    status=case when status in ('retired','maintenance') then status else 'degraded' end,
		    updated_at=$2
		where id=$1`, nodeID, now); err != nil {
		return domain.Node{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Node{}, err
	}
	_, _ = s.CreateAudit(ctx, "system", "node.agent_identity.revoke", "node", &nodeID, "agent identity revoked")
	return s.GetNode(ctx, nodeID)
}

func (s *Store) hasPendingAgentTokenRotation(ctx context.Context, nodeID string) (bool, error) {
	var ok bool
	err := s.db.QueryRow(ctx, `select exists(
		select 1
		from jobs
		where node_id=$1
		  and type='node.agent.rotate_token'
		  and status in ('queued','running','retrying')
	)`, nodeID).Scan(&ok)
	return ok, err
}

func (s *Store) latestPendingAgentTokenRotationAt(ctx context.Context, nodeID string) (*time.Time, error) {
	var at time.Time
	err := s.db.QueryRow(ctx, `select created_at
		from jobs
		where node_id=$1
		  and type='node.agent.rotate_token'
		  and status in ('queued','running','retrying')
		order by created_at desc
		limit 1`, nodeID).Scan(&at)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &at, nil
}

func rotatedAgentTokenMatchesSQL() string {
	return `exists(
		select 1
		from jobs
		where node_id=$1
		  and type='node.agent.rotate_token'
		  and status in ('queued','running','retrying')
		  and (
		    coalesce(payload_json->>'new_agent_token_hash','')=$2
		    or encode(digest(coalesce(payload_json->>'new_agent_token',''),'sha256'),'hex')=$2
		  )
	)`
}

func normalizeNodeAgentStatus(status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	if status == "" {
		return "unknown"
	}
	return status
}
