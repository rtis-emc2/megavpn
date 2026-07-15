package postgres

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/platform/id"
)

const nodeAgentRotateTokenJobType = "node.agent.rotate_token"

type staleRotationQueryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type staleRotationExecer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

type nodeStaleRotationAgentEvidence struct {
	Exists              bool
	Status              string
	RevokedAt           *time.Time
	LastSeenAt          *time.Time
	LastJobPollAt       *time.Time
	LastJobClaimAt      *time.Time
	LastJobClaimJobID   *string
	LastJobClaimType    string
	LastJobResultAt     *time.Time
	LastJobResultJobID  *string
	LastJobResultType   string
	LastJobResultStatus string
	LastInventorySyncAt *time.Time
	LastDiscoverySyncAt *time.Time
	LastRuntimeSyncAt   *time.Time
}

func (s *Store) PreviewNodeStaleRotation(ctx context.Context, nodeID string) (domain.NodeStaleRotationPreview, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return domain.NodeStaleRotationPreview{}, domain.ValidationError{Message: "node id is required"}
	}
	diag, err := s.GetNodeDiagnostics(ctx, nodeID)
	if err != nil {
		return domain.NodeStaleRotationPreview{}, err
	}
	now := time.Now().UTC()
	agent, err := loadNodeStaleRotationAgentEvidence(ctx, s.db, nodeID, false)
	if err != nil {
		return domain.NodeStaleRotationPreview{}, err
	}
	jobs, err := loadNodeStaleRotationJobs(ctx, s.db, nodeID, false)
	if err != nil {
		return domain.NodeStaleRotationPreview{}, err
	}
	candidates, err := s.classifyNodeStaleRotationCandidates(ctx, s.db, nodeID, agent, jobs, now)
	if err != nil {
		return domain.NodeStaleRotationPreview{}, err
	}
	return domain.NodeStaleRotationPreview{
		NodeID:                nodeID,
		StaleRotationDetected: len(safeNodeStaleRotationCandidateIDs(candidates)) > 0,
		TokenRotationStatus:   diag.Agent.TokenRotationStatus,
		EvaluatedAt:           now,
		Candidates:            candidates,
	}, nil
}

func (s *Store) ClearNodeStalePendingRotation(ctx context.Context, nodeID string, input domain.NodeStaleRotationClearInput) (domain.NodeStaleRotationClearResult, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return domain.NodeStaleRotationClearResult{}, domain.ValidationError{Message: "node id is required"}
	}
	if err := input.NormalizeAndValidate(); err != nil {
		return domain.NodeStaleRotationClearResult{}, err
	}

	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.NodeStaleRotationClearResult{}, err
	}
	defer tx.Rollback(ctx)

	var nodeName string
	var nodeStatus string
	if err := tx.QueryRow(ctx, `select name,status from nodes where id=$1 for update`, nodeID).Scan(&nodeName, &nodeStatus); err != nil {
		return domain.NodeStaleRotationClearResult{}, err
	}
	if strings.EqualFold(strings.TrimSpace(nodeStatus), "retired") {
		return domain.NodeStaleRotationClearResult{}, domain.ErrNodeStaleRotationConflict
	}
	expectedConfirmation := strings.TrimSpace(nodeName)
	if expectedConfirmation == "" {
		expectedConfirmation = nodeID
	}
	if input.Confirmation != expectedConfirmation {
		return domain.NodeStaleRotationClearResult{}, domain.ErrNodeStaleRotationConfirmationMismatch
	}

	agent, err := loadNodeStaleRotationAgentEvidence(ctx, tx, nodeID, true)
	if err != nil {
		return domain.NodeStaleRotationClearResult{}, err
	}
	jobs, err := loadNodeStaleRotationJobs(ctx, tx, nodeID, true)
	if err != nil {
		return domain.NodeStaleRotationClearResult{}, err
	}
	now := time.Now().UTC()
	candidates, err := s.classifyNodeStaleRotationCandidates(ctx, tx, nodeID, agent, jobs, now)
	if err != nil {
		return domain.NodeStaleRotationClearResult{}, err
	}
	if err := nodeStaleRotationExpectedCandidateError(candidates, input.ExpectedJobIDs); err != nil {
		return domain.NodeStaleRotationClearResult{}, err
	}
	currentSafeIDs := safeNodeStaleRotationCandidateIDs(candidates)
	if len(currentSafeIDs) == 0 {
		if len(candidates) > 0 {
			return domain.NodeStaleRotationClearResult{}, domain.ErrNodeStaleRotationPreviewChanged
		}
		return domain.NodeStaleRotationClearResult{}, domain.ErrNodeStaleRotationNotFound
	}
	if !sameSortedStrings(currentSafeIDs, input.ExpectedJobIDs) {
		return domain.NodeStaleRotationClearResult{}, domain.ErrNodeStaleRotationPreviewChanged
	}

	jobByID := make(map[string]domain.Job, len(jobs))
	candidateByID := make(map[string]domain.NodeStaleRotationCandidate, len(candidates))
	for _, job := range jobs {
		jobByID[job.ID] = job
	}
	for _, candidate := range candidates {
		if candidate.SafeToClear {
			candidateByID[candidate.JobID] = candidate
		}
	}

	cleared := make([]domain.NodeStaleRotationClearedJob, 0, len(currentSafeIDs))
	secretRefsCleared := int64(0)
	for _, jobID := range currentSafeIDs {
		job, ok := jobByID[jobID]
		if !ok {
			return domain.NodeStaleRotationClearResult{}, domain.ErrNodeStaleRotationPreviewChanged
		}
		candidate := candidateByID[jobID]
		secretRefID, err := validateNodeStaleRotationSecretRef(ctx, tx, nodeID, job, true)
		if err != nil {
			return domain.NodeStaleRotationClearResult{}, err
		}
		resultJSON := mustJSON(map[string]any{
			"message":      "stale agent token rotation cancelled by operator",
			"code":         "stale_rotation_cleared",
			"cleared_at":   now.Format(time.RFC3339),
			"stale_reason": candidate.StaleReason,
		})
		tag, err := tx.Exec(ctx, `update jobs
			set status='cancelled',
			    finished_at=$2,
			    locked_by=null,
			    locked_until=null,
			    result_json=$3
			where id=$1
			  and type='node.agent.rotate_token'
			  and status in ('queued','running','retrying')`,
			job.ID, now, resultJSON)
		if err != nil {
			return domain.NodeStaleRotationClearResult{}, err
		}
		if tag.RowsAffected() != 1 {
			return domain.NodeStaleRotationClearResult{}, domain.ErrNodeStaleRotationPreviewChanged
		}
		if _, err := tx.Exec(ctx, `delete from resource_locks where job_id=$1`, job.ID); err != nil {
			return domain.NodeStaleRotationClearResult{}, err
		}
		tag, err = tx.Exec(ctx, `delete from secret_refs where id=$1`, secretRefID)
		if err != nil {
			return domain.NodeStaleRotationClearResult{}, err
		}
		if tag.RowsAffected() != 1 {
			return domain.NodeStaleRotationClearResult{}, domain.ErrNodeStaleRotationPendingStateAmbiguous
		}
		secretRefsCleared += tag.RowsAffected()
		if err := insertJobLog(ctx, tx, job.ID, "warn", "stale agent token rotation cancelled by operator", map[string]any{
			"node_id":      nodeID,
			"stale_reason": candidate.StaleReason,
			"reason":       input.Reason,
		}); err != nil {
			return domain.NodeStaleRotationClearResult{}, fmt.Errorf("%w: %v", domain.ErrNodeStaleRotationJobLogFailed, err)
		}
		cleared = append(cleared, domain.NodeStaleRotationClearedJob{
			JobID:          job.ID,
			PreviousStatus: job.Status,
			Status:         "cancelled",
			StaleReason:    candidate.StaleReason,
			FinishedAt:     now,
		})
	}
	auditSummary := fmt.Sprintf(
		"stale agent token rotation cleared: node_id=%s node_name=%s cleared_jobs=%s reasons=%s pending_rotation_state_cleared=%t reason=%s",
		nodeID,
		expectedConfirmation,
		strings.Join(currentSafeIDs, ","),
		strings.Join(nodeStaleRotationReasons(cleared), ","),
		secretRefsCleared > 0,
		input.Reason,
	)
	if _, err := insertAuditForUser(ctx, tx, input.ActorUserID, "node.agent_token.clear_stale", "node", &nodeID, auditSummary); err != nil {
		return domain.NodeStaleRotationClearResult{}, fmt.Errorf("%w: %v", domain.ErrNodeStaleRotationAuditFailed, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.NodeStaleRotationClearResult{}, err
	}
	return domain.NodeStaleRotationClearResult{
		Status:                       "cleared",
		NodeID:                       nodeID,
		ClearedCount:                 len(cleared),
		ClearedJobs:                  cleared,
		PendingRotationStateCleared:  secretRefsCleared > 0,
		ActiveAgentIdentityPreserved: true,
	}, nil
}

func loadNodeStaleRotationAgentEvidence(ctx context.Context, q staleRotationQueryer, nodeID string, forUpdate bool) (nodeStaleRotationAgentEvidence, error) {
	query := `select status,revoked_at,last_seen_at,last_job_poll_at,last_job_claim_at,last_job_claim_job_id,coalesce(last_job_claim_type,''),
		last_job_result_at,last_job_result_job_id,coalesce(last_job_result_type,''),coalesce(last_job_result_status,''),last_inventory_sync_at,last_discovery_sync_at,last_runtime_sync_at
		from node_agents where node_id=$1`
	if forUpdate {
		query += ` for update`
	}
	var x nodeStaleRotationAgentEvidence
	err := q.QueryRow(ctx, query, nodeID).Scan(
		&x.Status, &x.RevokedAt, &x.LastSeenAt, &x.LastJobPollAt, &x.LastJobClaimAt, &x.LastJobClaimJobID, &x.LastJobClaimType,
		&x.LastJobResultAt, &x.LastJobResultJobID, &x.LastJobResultType, &x.LastJobResultStatus, &x.LastInventorySyncAt, &x.LastDiscoverySyncAt, &x.LastRuntimeSyncAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return x, nil
	}
	if err != nil {
		return x, err
	}
	x.Exists = true
	return x, nil
}

func loadNodeStaleRotationJobs(ctx context.Context, q staleRotationQueryer, nodeID string, forUpdate bool) ([]domain.Job, error) {
	query := `select id,type,scope_type,scope_id,node_id,instance_id,status,priority,payload_json,coalesce(result_json,'{}'::jsonb),locked_by,locked_until,created_at,started_at,finished_at
		from jobs
		where node_id=$1
		  and type='node.agent.rotate_token'
		  and status in ('queued','running','retrying','succeeded')
		order by created_at asc, id asc`
	if forUpdate {
		query += ` for update`
	}
	rows, err := q.Query(ctx, query, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := make([]domain.Job, 0)
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (s *Store) classifyNodeStaleRotationCandidates(ctx context.Context, q staleRotationQueryer, nodeID string, agent nodeStaleRotationAgentEvidence, jobs []domain.Job, now time.Time) ([]domain.NodeStaleRotationCandidate, error) {
	candidates := make([]domain.NodeStaleRotationCandidate, 0)
	for _, job := range jobs {
		if !isActiveNodeStaleRotationStatus(job.Status) {
			continue
		}
		candidate := classifyNodeStaleRotationCandidate(job, agent, jobs, now)
		if candidate.SafeToClear {
			if _, err := validateNodeStaleRotationSecretRef(ctx, q, nodeID, job, false); err != nil {
				if errors.Is(err, domain.ErrNodeStaleRotationPendingStateAmbiguous) {
					candidate.SafeToClear = false
					candidate.StaleReason = domain.NodeStaleRotationReasonPendingStateAmbiguous
				} else {
					return nil, err
				}
			}
		}
		candidates = append(candidates, candidate)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].CreatedAt.Equal(candidates[j].CreatedAt) {
			return candidates[i].JobID < candidates[j].JobID
		}
		return candidates[i].CreatedAt.Before(candidates[j].CreatedAt)
	})
	return candidates, nil
}

func classifyNodeStaleRotationCandidate(job domain.Job, agent nodeStaleRotationAgentEvidence, jobs []domain.Job, now time.Time) domain.NodeStaleRotationCandidate {
	age := int64(0)
	if job.CreatedAt.Before(now) {
		age = int64(now.Sub(job.CreatedAt) / time.Second)
	}
	candidate := domain.NodeStaleRotationCandidate{
		JobID:        job.ID,
		Status:       job.Status,
		CreatedAt:    job.CreatedAt,
		StartedAt:    job.StartedAt,
		LastClaimAt:  agent.LastJobClaimAt,
		LastResultAt: agent.LastJobResultAt,
		LastSeenAt:   agent.LastSeenAt,
		LastPollAt:   agent.LastJobPollAt,
		AgeSeconds:   age,
		StaleReason:  domain.NodeStaleRotationReasonEvidenceAmbiguous,
		SafeToClear:  false,
	}
	if !agent.Exists || !strings.EqualFold(strings.TrimSpace(agent.Status), "active") || agent.RevokedAt != nil {
		candidate.StaleReason = domain.NodeStaleRotationReasonAgentIdentityInactive
		return candidate
	}
	if hasNewerNodeTokenRotation(job, jobs) {
		candidate.StaleReason = domain.NodeStaleRotationReasonSuperseded
		return candidate
	}
	if now.Sub(job.CreatedAt) < stalePendingRotateAfter {
		candidate.StaleReason = domain.NodeStaleRotationReasonFresh
		return candidate
	}

	switch strings.ToLower(strings.TrimSpace(job.Status)) {
	case "queued":
		return classifyQueuedNodeStaleRotationCandidate(candidate, job, agent)
	case "running", "retrying":
		return classifyClaimedNodeStaleRotationCandidate(candidate, job, agent, now)
	default:
		return candidate
	}
}

func classifyQueuedNodeStaleRotationCandidate(candidate domain.NodeStaleRotationCandidate, job domain.Job, agent nodeStaleRotationAgentEvidence) domain.NodeStaleRotationCandidate {
	if job.StartedAt != nil || job.LockedBy != nil || job.LockedUntil != nil {
		candidate.StaleReason = domain.NodeStaleRotationReasonEvidenceAmbiguous
		return candidate
	}
	if sameStringPtr(agent.LastJobClaimJobID, job.ID) || sameStringPtr(agent.LastJobResultJobID, job.ID) {
		candidate.StaleReason = domain.NodeStaleRotationReasonEvidenceAmbiguous
		return candidate
	}
	if hasAgentProgressAtOrAfter(job.CreatedAt, agent.LastSeenAt, agent.LastJobPollAt, agent.LastJobClaimAt, agent.LastJobResultAt, agent.LastInventorySyncAt, agent.LastDiscoverySyncAt, agent.LastRuntimeSyncAt) {
		candidate.StaleReason = domain.NodeStaleRotationReasonAgentProgressAfterCreation
		return candidate
	}
	candidate.StaleReason = domain.NodeStaleRotationReasonUnclaimedInactive
	candidate.SafeToClear = true
	return candidate
}

func classifyClaimedNodeStaleRotationCandidate(candidate domain.NodeStaleRotationCandidate, job domain.Job, agent nodeStaleRotationAgentEvidence, now time.Time) domain.NodeStaleRotationCandidate {
	if agent.LastJobClaimAt == nil || !sameStringPtr(agent.LastJobClaimJobID, job.ID) || strings.TrimSpace(agent.LastJobClaimType) != nodeAgentRotateTokenJobType {
		candidate.StaleReason = domain.NodeStaleRotationReasonClaimMissing
		return candidate
	}
	if now.Sub(*agent.LastJobClaimAt) < stalePendingRotateAfter || (job.LockedUntil != nil && job.LockedUntil.After(now)) {
		candidate.StaleReason = domain.NodeStaleRotationReasonClaimOrLeaseActive
		return candidate
	}
	if sameStringPtr(agent.LastJobResultJobID, job.ID) && agent.LastJobResultAt != nil {
		if !agent.LastJobResultAt.Before(*agent.LastJobClaimAt) {
			candidate.StaleReason = domain.NodeStaleRotationReasonResultSubmitted
			return candidate
		}
		candidate.StaleReason = domain.NodeStaleRotationReasonEvidenceAmbiguous
		return candidate
	}
	if hasAgentProgressAtOrAfter(*agent.LastJobClaimAt, agent.LastSeenAt, agent.LastJobPollAt, agent.LastInventorySyncAt, agent.LastDiscoverySyncAt, agent.LastRuntimeSyncAt) {
		candidate.StaleReason = domain.NodeStaleRotationReasonAgentProgressAfterClaim
		return candidate
	}
	candidate.StaleReason = domain.NodeStaleRotationReasonClaimedInactive
	candidate.SafeToClear = true
	return candidate
}

func validateNodeStaleRotationSecretRef(ctx context.Context, q staleRotationQueryer, nodeID string, job domain.Job, forUpdate bool) (string, error) {
	secretRefID := strings.TrimSpace(stringify(job.Payload["new_agent_token_secret_ref_id"]))
	tokenHash := strings.TrimSpace(stringify(job.Payload["new_agent_token_hash"]))
	if secretRefID == "" || tokenHash == "" {
		return "", domain.ErrNodeStaleRotationPendingStateAmbiguous
	}
	query := `select secret_type,meta_json from secret_refs where id=$1`
	if forUpdate {
		query += ` for update`
	}
	var secretType string
	var metaRaw []byte
	err := q.QueryRow(ctx, query, secretRefID).Scan(&secretType, &metaRaw)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrNodeStaleRotationPendingStateAmbiguous
	}
	if err != nil {
		return "", domain.ErrNodeStaleRotationPendingStateAmbiguous
	}
	meta := map[string]any{}
	if err := decodeJSONField(metaRaw, &meta, "secret_refs.meta_json"); err != nil {
		return "", err
	}
	if strings.TrimSpace(secretType) != "api_token" ||
		strings.TrimSpace(stringify(meta["scope"])) != "node_agent_token_rotation" ||
		strings.TrimSpace(stringify(meta["node_id"])) != strings.TrimSpace(nodeID) ||
		strings.TrimSpace(stringify(meta["material"])) != "new_agent_token" {
		return "", domain.ErrNodeStaleRotationPendingStateAmbiguous
	}
	return secretRefID, nil
}

func insertJobLog(ctx context.Context, exec staleRotationExecer, jobID, level, msg string, payload map[string]any) error {
	if !in(level, "debug", "info", "warn", "error") {
		level = "info"
	}
	if payload == nil {
		payload = map[string]any{}
	}
	redacted := redactSensitiveMapForStorage(payload)
	b := mustJSON(redacted)
	_, err := exec.Exec(ctx, `insert into job_logs(id,job_id,level,message,payload_json,created_at) values($1,$2,$3,$4,$5,now())`, id.New(), jobID, level, msg, b)
	return err
}

func safeNodeStaleRotationCandidateIDs(candidates []domain.NodeStaleRotationCandidate) []string {
	ids := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.SafeToClear {
			ids = append(ids, candidate.JobID)
		}
	}
	sort.Strings(ids)
	return ids
}

func nodeStaleRotationExpectedCandidateError(candidates []domain.NodeStaleRotationCandidate, expectedJobIDs []string) error {
	expected := make(map[string]struct{}, len(expectedJobIDs))
	for _, id := range expectedJobIDs {
		expected[strings.TrimSpace(id)] = struct{}{}
	}
	for _, candidate := range candidates {
		if _, ok := expected[candidate.JobID]; !ok || candidate.SafeToClear {
			continue
		}
		switch candidate.StaleReason {
		case domain.NodeStaleRotationReasonPendingStateAmbiguous:
			return domain.ErrNodeStaleRotationPendingStateAmbiguous
		case domain.NodeStaleRotationReasonEvidenceAmbiguous,
			domain.NodeStaleRotationReasonClaimMissing,
			domain.NodeStaleRotationReasonAgentIdentityInactive:
			return domain.ErrNodeStaleRotationEvidenceAmbiguous
		default:
			continue
		}
	}
	return nil
}

func sameSortedStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for idx := range a {
		if a[idx] != b[idx] {
			return false
		}
	}
	return true
}

func hasNewerNodeTokenRotation(job domain.Job, jobs []domain.Job) bool {
	for _, other := range jobs {
		if other.ID == job.ID || !isTerminalOrActiveNodeStaleRotationStatus(other.Status) {
			continue
		}
		if other.CreatedAt.After(job.CreatedAt) || (other.CreatedAt.Equal(job.CreatedAt) && other.ID > job.ID) {
			return true
		}
	}
	return false
}

func isActiveNodeStaleRotationStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "queued", "running", "retrying":
		return true
	default:
		return false
	}
}

func isTerminalOrActiveNodeStaleRotationStatus(status string) bool {
	return isActiveNodeStaleRotationStatus(status) || strings.EqualFold(strings.TrimSpace(status), "succeeded")
}

func sameStringPtr(value *string, want string) bool {
	return value != nil && strings.TrimSpace(*value) == strings.TrimSpace(want)
}

func hasAgentProgressAtOrAfter(reference time.Time, values ...*time.Time) bool {
	for _, value := range values {
		if value == nil {
			continue
		}
		if !value.Before(reference) {
			return true
		}
	}
	return false
}

func nodeStaleRotationReasons(jobs []domain.NodeStaleRotationClearedJob) []string {
	reasons := make([]string, 0, len(jobs))
	seen := map[string]struct{}{}
	for _, job := range jobs {
		reason := strings.TrimSpace(job.StaleReason)
		if reason == "" {
			continue
		}
		if _, ok := seen[reason]; ok {
			continue
		}
		seen[reason] = struct{}{}
		reasons = append(reasons, reason)
	}
	sort.Strings(reasons)
	return reasons
}
