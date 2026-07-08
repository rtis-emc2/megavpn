package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/platform/id"
	"github.com/rtis-emc2/megavpn/internal/service/driver"
)

func (s *Store) ListBinaryArtifacts(ctx context.Context, includeInactive bool) ([]domain.BinaryArtifact, error) {
	where := `status = 'active'`
	if includeInactive {
		where = `status <> 'deleted'`
	}
	rows, err := s.db.Query(ctx, `select id,name,kind,service_code,version,os_family,os_version,architecture,storage_path,size_bytes,sha256,signature,status,metadata_json,created_at,updated_at from binary_artifacts where `+where+` order by kind, service_code, name, version desc, created_at desc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.BinaryArtifact{}
	for rows.Next() {
		item, err := scanBinaryArtifact(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) GetBinaryArtifact(ctx context.Context, artifactID string) (domain.BinaryArtifact, error) {
	row := s.db.QueryRow(ctx, `select id,name,kind,service_code,version,os_family,os_version,architecture,storage_path,size_bytes,sha256,signature,status,metadata_json,created_at,updated_at from binary_artifacts where id=$1 and status <> 'deleted'`, artifactID)
	return scanBinaryArtifact(row)
}

func (s *Store) FindBinaryRuntimeArtifactForNode(ctx context.Context, nodeID, serviceCode string) (domain.BinaryArtifact, error) {
	nodeID = strings.TrimSpace(nodeID)
	serviceCode = driver.NormalizeCode(serviceCode)
	if nodeID == "" {
		return domain.BinaryArtifact{}, errors.New("node_id is required")
	}
	if serviceCode == "" {
		return domain.BinaryArtifact{}, errors.New("service_code is required")
	}
	var osFamily, osVersion, arch string
	err := s.db.QueryRow(ctx, `select coalesce(os_family,''),coalesce(os_version,''),coalesce(architecture,'') from nodes where id=$1 and status <> 'retired'`, nodeID).Scan(&osFamily, &osVersion, &arch)
	if err != nil {
		return domain.BinaryArtifact{}, err
	}
	osFamily = strings.ToLower(strings.TrimSpace(osFamily))
	if osFamily == "" {
		osFamily = "linux"
	}
	arch = normalizeBinaryArtifactArchitecture(arch)
	row := s.db.QueryRow(ctx, `
		select id,name,kind,service_code,version,os_family,os_version,architecture,storage_path,size_bytes,sha256,signature,status,metadata_json,created_at,updated_at
		from binary_artifacts
		where status='active'
		  and service_code=$1
		  and kind in ('runtime','package','script','bundle')
		  and os_family=$2
		  and architecture=$3
		  and (os_version='' or os_version=$4)
		order by
		  case when os_version=$4 and os_version <> '' then 0 else 1 end,
		  created_at desc,
		  version desc
		limit 1`, serviceCode, osFamily, arch, strings.TrimSpace(osVersion))
	return scanBinaryArtifact(row)
}

func (s *Store) CreateBinaryArtifact(ctx context.Context, item domain.BinaryArtifact) (domain.BinaryArtifact, error) {
	if err := normalizeBinaryArtifact(&item); err != nil {
		return domain.BinaryArtifact{}, err
	}
	if item.ID == "" {
		item.ID = id.New()
	}
	now := time.Now().UTC()
	item.CreatedAt = now
	item.UpdatedAt = now
	if item.Metadata == nil {
		item.Metadata = map[string]any{}
	}
	if _, err := s.db.Exec(ctx, `insert into binary_artifacts(id,name,kind,service_code,version,os_family,os_version,architecture,storage_path,size_bytes,sha256,signature,status,metadata_json,created_at,updated_at)
		values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`,
		item.ID, item.Name, item.Kind, item.ServiceCode, item.Version, item.OSFamily, item.OSVersion, item.Architecture, item.StoragePath, item.SizeBytes, item.SHA256, item.Signature, item.Status, mustJSON(item.Metadata), item.CreatedAt, item.UpdatedAt); err != nil {
		return domain.BinaryArtifact{}, err
	}
	_, _ = s.CreateAudit(ctx, "system", "binary_artifact.create", "binary_artifact", &item.ID, "binary artifact registered: "+item.Name)
	return item, nil
}

func (s *Store) CreateBinaryDownloadTicket(ctx context.Context, artifactID, nodeID string, jobID *string, ttl time.Duration) (domain.BinaryDownloadTicket, error) {
	ticket, err := newBinaryDownloadTicket(artifactID, nodeID, jobID, ttl)
	if err != nil {
		return domain.BinaryDownloadTicket{}, err
	}
	if err := insertBinaryDownloadTicket(ctx, s.db, ticket); err != nil {
		return domain.BinaryDownloadTicket{}, err
	}
	return ticket, nil
}

func newBinaryDownloadTicket(artifactID, nodeID string, jobID *string, ttl time.Duration) (domain.BinaryDownloadTicket, error) {
	artifactID = strings.TrimSpace(artifactID)
	nodeID = strings.TrimSpace(nodeID)
	if artifactID == "" {
		return domain.BinaryDownloadTicket{}, errors.New("artifact_id is required")
	}
	if nodeID == "" {
		return domain.BinaryDownloadTicket{}, errors.New("node_id is required")
	}
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	var normalizedJobID *string
	if jobID != nil {
		value := strings.TrimSpace(*jobID)
		if value == "" {
			return domain.BinaryDownloadTicket{}, errors.New("job_id cannot be empty")
		}
		normalizedJobID = &value
	}
	token := randomToken(32)
	now := time.Now().UTC()
	ticket := domain.BinaryDownloadTicket{
		ID:         id.New(),
		ArtifactID: artifactID,
		NodeID:     &nodeID,
		JobID:      normalizedJobID,
		Token:      token,
		TokenHint:  tokenHint(token),
		Status:     "active",
		ExpiresAt:  now.Add(ttl),
		CreatedAt:  now,
	}
	return ticket, nil
}

type binaryDownloadTicketInserter interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func insertBinaryDownloadTicket(ctx context.Context, db binaryDownloadTicketInserter, ticket domain.BinaryDownloadTicket) error {
	if strings.TrimSpace(ticket.Token) == "" {
		return errors.New("download ticket token is required")
	}
	_, err := db.Exec(ctx, `insert into binary_download_tickets(id,artifact_id,node_id,job_id,token_hash,token_hint,status,expires_at,created_at)
		values($1,$2,$3,$4,$5,$6,$7,$8,$9)`, ticket.ID, ticket.ArtifactID, ticket.NodeID, ticket.JobID, hashToken(ticket.Token), ticket.TokenHint, ticket.Status, ticket.ExpiresAt, ticket.CreatedAt)
	return err
}

func (s *Store) ResolveBinaryDownloadTicket(ctx context.Context, token, artifactID, nodeID, jobID string) (domain.BinaryDownloadTicket, domain.BinaryArtifact, error) {
	token = strings.TrimSpace(token)
	artifactID = strings.TrimSpace(artifactID)
	nodeID = strings.TrimSpace(nodeID)
	jobID = strings.TrimSpace(jobID)
	if token == "" {
		return domain.BinaryDownloadTicket{}, domain.BinaryArtifact{}, errors.New("download token is required")
	}
	if artifactID == "" || nodeID == "" || jobID == "" {
		return domain.BinaryDownloadTicket{}, domain.BinaryArtifact{}, errors.New("artifact_id, node_id and job_id are required")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return domain.BinaryDownloadTicket{}, domain.BinaryArtifact{}, err
	}
	defer tx.Rollback(ctx)

	var ticket domain.BinaryDownloadTicket
	var rowJobID sql.NullString
	err = tx.QueryRow(ctx, `select id,artifact_id,node_id::text,job_id::text,coalesce(token_hint,''),status,expires_at,used_at,created_at
		from binary_download_tickets
		where token_hash=$1 and artifact_id=$2 and node_id=$3
		for update`, hashToken(token), artifactID, nodeID).
		Scan(&ticket.ID, &ticket.ArtifactID, &ticket.NodeID, &rowJobID, &ticket.TokenHint, &ticket.Status, &ticket.ExpiresAt, &ticket.UsedAt, &ticket.CreatedAt)
	if err != nil {
		return domain.BinaryDownloadTicket{}, domain.BinaryArtifact{}, err
	}
	if rowJobID.Valid {
		value := strings.TrimSpace(rowJobID.String)
		ticket.JobID = &value
		if value != jobID {
			return domain.BinaryDownloadTicket{}, domain.BinaryArtifact{}, errors.New("download ticket is not bound to this job")
		}
	}
	now := time.Now().UTC()
	if ticket.Status != "active" {
		return domain.BinaryDownloadTicket{}, domain.BinaryArtifact{}, errors.New("download ticket is not active")
	}
	if ticket.ExpiresAt.Before(now) {
		_, _ = tx.Exec(ctx, `update binary_download_tickets set status='expired' where id=$1 and status='active'`, ticket.ID)
		return domain.BinaryDownloadTicket{}, domain.BinaryArtifact{}, errors.New("download ticket has expired")
	}
	artifact, err := scanBinaryArtifact(tx.QueryRow(ctx, `select id,name,kind,service_code,version,os_family,os_version,architecture,storage_path,size_bytes,sha256,signature,status,metadata_json,created_at,updated_at from binary_artifacts where id=$1 and status='active'`, artifactID))
	if err != nil {
		return domain.BinaryDownloadTicket{}, domain.BinaryArtifact{}, err
	}
	if ticket.JobID == nil {
		ticket.JobID = &jobID
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.BinaryDownloadTicket{}, domain.BinaryArtifact{}, err
	}
	return ticket, artifact, nil
}

func (s *Store) MarkBinaryDownloadTicketUsed(ctx context.Context, ticketID, jobID string) error {
	ticketID = strings.TrimSpace(ticketID)
	jobID = strings.TrimSpace(jobID)
	if ticketID == "" || jobID == "" {
		return errors.New("ticket_id and job_id are required")
	}
	now := time.Now().UTC()
	tag, err := s.db.Exec(ctx, `update binary_download_tickets
		set status='used', used_at=coalesce(used_at,$3), job_id=coalesce(job_id,$2)
		where id=$1
		  and status in ('active','used')
		  and (job_id is null or job_id=$2)`,
		ticketID, jobID, now)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("download ticket cannot be marked used")
	}
	return nil
}

func (s *Store) markBinaryDownloadTicketUsedFromJobResult(ctx context.Context, jobID string, result map[string]any) error {
	repo, _ := result["binary_repository"].(map[string]any)
	if repo == nil {
		return nil
	}
	if !truthy(repo["download_verified"]) {
		return nil
	}
	ticketID := strings.TrimSpace(stringify(repo["download_ticket_id"]))
	if ticketID == "" {
		ticketID = strings.TrimSpace(stringify(repo["ticket_id"]))
	}
	if ticketID == "" {
		return nil
	}
	return s.MarkBinaryDownloadTicketUsed(ctx, ticketID, jobID)
}

func (s *Store) CleanupBinaryDownloadTickets(ctx context.Context, retention time.Duration) (expired int64, deleted int64, err error) {
	if retention <= 0 {
		retention = 7 * 24 * time.Hour
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback(ctx)

	now := time.Now().UTC()
	expireTag, err := tx.Exec(ctx, `update binary_download_tickets
		set status='expired'
		where status='active'
		  and expires_at < $1`, now)
	if err != nil {
		return 0, 0, err
	}
	cutoff := now.Add(-retention)
	deleteTag, err := tx.Exec(ctx, `delete from binary_download_tickets
		where status in ('used','expired','revoked')
		  and coalesce(used_at, expires_at, created_at) < $1`, cutoff)
	if err != nil {
		return 0, 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, 0, err
	}
	return expireTag.RowsAffected(), deleteTag.RowsAffected(), nil
}

func normalizeBinaryArtifact(item *domain.BinaryArtifact) error {
	if item == nil {
		return errors.New("binary artifact is required")
	}
	item.Name = strings.TrimSpace(item.Name)
	item.Kind = strings.ToLower(strings.TrimSpace(item.Kind))
	item.ServiceCode = driver.NormalizeCode(item.ServiceCode)
	item.Version = strings.TrimSpace(item.Version)
	item.OSFamily = strings.ToLower(strings.TrimSpace(item.OSFamily))
	item.OSVersion = strings.TrimSpace(item.OSVersion)
	item.Architecture = normalizeBinaryArtifactArchitecture(item.Architecture)
	item.StoragePath = strings.TrimSpace(item.StoragePath)
	item.SHA256 = strings.ToLower(strings.TrimSpace(item.SHA256))
	item.Signature = strings.TrimSpace(item.Signature)
	item.Status = strings.ToLower(strings.TrimSpace(item.Status))
	if item.OSFamily == "" {
		item.OSFamily = "linux"
	}
	if item.Architecture == "" {
		item.Architecture = "amd64"
	}
	if item.Status == "" {
		item.Status = "active"
	}
	if item.Name == "" {
		return errors.New("name is required")
	}
	if !in(item.Kind, "agent", "runtime", "package", "script", "bundle") {
		return fmt.Errorf("unsupported binary artifact kind %q", item.Kind)
	}
	if item.Version == "" {
		return errors.New("version is required")
	}
	if !in(item.OSFamily, "linux") {
		return fmt.Errorf("unsupported os_family %q", item.OSFamily)
	}
	if !in(item.Architecture, "amd64", "arm64") {
		return fmt.Errorf("unsupported architecture %q", item.Architecture)
	}
	if item.StoragePath == "" || strings.Contains(item.StoragePath, "\x00") || strings.Contains(item.StoragePath, "..") {
		return errors.New("storage_path must be a stable repository path without traversal")
	}
	if len(item.SHA256) != 64 || strings.Trim(item.SHA256, "0123456789abcdef") != "" {
		return errors.New("sha256 must be 64 lowercase hex characters")
	}
	if !in(item.Status, "active", "disabled") {
		return fmt.Errorf("unsupported status %q", item.Status)
	}
	if item.SizeBytes < 0 {
		return errors.New("size_bytes cannot be negative")
	}
	return nil
}

func normalizeBinaryArtifactArchitecture(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "x86_64":
		return "amd64"
	case "aarch64":
		return "arm64"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

type binaryArtifactScanner interface{ Scan(dest ...any) error }

func scanBinaryArtifact(row binaryArtifactScanner) (domain.BinaryArtifact, error) {
	var item domain.BinaryArtifact
	var metadataRaw []byte
	if err := row.Scan(
		&item.ID,
		&item.Name,
		&item.Kind,
		&item.ServiceCode,
		&item.Version,
		&item.OSFamily,
		&item.OSVersion,
		&item.Architecture,
		&item.StoragePath,
		&item.SizeBytes,
		&item.SHA256,
		&item.Signature,
		&item.Status,
		&metadataRaw,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return domain.BinaryArtifact{}, err
	}
	if err := decodeJSONField(metadataRaw, &item.Metadata, "binary_artifacts.metadata_json"); err != nil {
		return domain.BinaryArtifact{}, err
	}
	if item.Metadata == nil {
		item.Metadata = map[string]any{}
	}
	return item, nil
}
