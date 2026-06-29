package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

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
	item.Architecture = strings.ToLower(strings.TrimSpace(item.Architecture))
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
	_ = json.Unmarshal(metadataRaw, &item.Metadata)
	if item.Metadata == nil {
		item.Metadata = map[string]any{}
	}
	return item, nil
}
