package postgres

import (
	"context"
	"errors"
	"fmt"
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

func (s *Store) RevokeNodeAgentIdentity(ctx context.Context, nodeID string, input domain.NodeAgentIdentityRevokeInput) (domain.NodeAgentIdentityRevokeResult, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return domain.NodeAgentIdentityRevokeResult{}, domain.ValidationError{Message: "node id is required"}
	}
	if err := input.NormalizeAndValidate(); err != nil {
		return domain.NodeAgentIdentityRevokeResult{}, err
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.NodeAgentIdentityRevokeResult{}, err
	}
	defer tx.Rollback(ctx)

	var nodeName string
	if err := tx.QueryRow(ctx, `select name from nodes where id=$1 for update`, nodeID).Scan(&nodeName); err != nil {
		return domain.NodeAgentIdentityRevokeResult{}, err
	}
	expectedConfirmation := strings.TrimSpace(nodeName)
	if expectedConfirmation == "" {
		expectedConfirmation = nodeID
	}
	if input.Confirmation != expectedConfirmation {
		return domain.NodeAgentIdentityRevokeResult{}, domain.ErrNodeAgentRevokeConfirmationMismatch
	}

	var agentID string
	var agentStatus string
	var revokedAt *time.Time
	if err := tx.QueryRow(ctx, `select id,status,revoked_at from node_agents where node_id=$1 for update`, nodeID).Scan(&agentID, &agentStatus, &revokedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NodeAgentIdentityRevokeResult{}, domain.ErrNodeAgentIdentityMissing
		}
		return domain.NodeAgentIdentityRevokeResult{}, err
	}
	if agentStatus == "revoked" {
		if revokedAt == nil {
			return domain.NodeAgentIdentityRevokeResult{}, domain.ErrNodeAgentRevokeConflict
		}
		return domain.NodeAgentIdentityRevokeResult{
			Status:                  "revoked",
			NodeID:                  nodeID,
			AgentStatus:             "revoked",
			RevokedAt:               *revokedAt,
			AlreadyRevoked:          true,
			RevokedEnrollmentTokens: 0,
		}, nil
	}
	if agentStatus != "active" || revokedAt != nil {
		return domain.NodeAgentIdentityRevokeResult{}, domain.ErrNodeAgentRevokeConflict
	}

	now := time.Now().UTC()
	var persistedRevokedAt time.Time
	err = tx.QueryRow(ctx, `update node_agents
		set status='revoked',
		    revoked_at=$2,
		    agent_token_hash=null
		where id=$1 and status='active' and revoked_at is null
		returning revoked_at`, agentID, now).Scan(&persistedRevokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.NodeAgentIdentityRevokeResult{}, domain.ErrNodeAgentRevokeConflict
	}
	if err != nil {
		return domain.NodeAgentIdentityRevokeResult{}, err
	}

	cmd, err := tx.Exec(ctx, `update node_enrollment_tokens
		set status='revoked'
		where node_id=$1 and status='active' and used_at is null and expires_at > $2`, nodeID, now)
	if err != nil {
		return domain.NodeAgentIdentityRevokeResult{}, err
	}
	revokedEnrollmentTokens := cmd.RowsAffected()

	if _, err := tx.Exec(ctx, `update nodes
		set agent_status='revoked',
		    updated_at=$2
		where id=$1`, nodeID, now); err != nil {
		return domain.NodeAgentIdentityRevokeResult{}, err
	}
	auditSummary := fmt.Sprintf("agent identity revoked: node_id=%s node_name=%s enrollment_tokens_revoked=%d reason=%s", nodeID, nodeName, revokedEnrollmentTokens, input.Reason)
	if _, err := insertAuditForUser(ctx, tx, input.ActorUserID, "node.agent_identity.revoke", "node", &nodeID, auditSummary); err != nil {
		return domain.NodeAgentIdentityRevokeResult{}, fmt.Errorf("%w: %v", domain.ErrNodeAgentRevokeAuditFailed, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.NodeAgentIdentityRevokeResult{}, err
	}
	return domain.NodeAgentIdentityRevokeResult{
		Status:                  "revoked",
		NodeID:                  nodeID,
		AgentStatus:             "revoked",
		RevokedAt:               persistedRevokedAt,
		AlreadyRevoked:          false,
		RevokedEnrollmentTokens: revokedEnrollmentTokens,
	}, nil
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
