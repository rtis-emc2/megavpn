package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/platform/id"
)

const defaultArtifactRoot = "/var/lib/megavpn/artifacts"

func (s *Store) ListProvisioningAccesses(ctx context.Context, clientID string) ([]domain.ProvisioningAccess, error) {
	rows, err := s.db.Query(ctx, `select
		sa.id,
		sa.client_account_id,
		sa.instance_id,
		sa.status,
		sa.provision_mode,
		sa.policy_json,
		sa.metadata_json,
		sa.created_at,
		sa.updated_at,
		ca.id,
		ca.username,
		coalesce(ca.display_name,''),
		coalesce(ca.email,''),
		ca.status,
		coalesce(ca.notes,''),
		ca.expires_at,
		ca.created_at,
		ca.updated_at,
		i.id,
		i.node_id,
		sd.code,
		i.name,
		i.slug,
		coalesce(i.systemd_unit,''),
		i.status,
		i.enabled,
		coalesce(i.endpoint_host,''),
		coalesce(i.endpoint_port,0),
		i.created_at,
		i.updated_at,
		coalesce((select spec_json from instance_revisions where instance_id=i.id order by revision_no desc limit 1), '{}'::jsonb)
	from service_accesses sa
	join client_accounts ca on ca.id=sa.client_account_id
	join instances i on i.id=sa.instance_id
	join service_definitions sd on sd.id=i.service_definition_id
	where sa.client_account_id=$1
	order by sa.created_at asc`, clientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]domain.ProvisioningAccess, 0)
	for rows.Next() {
		var rec domain.ProvisioningAccess
		var policyRaw, metadataRaw, specRaw []byte
		if err := rows.Scan(
			&rec.Access.ID,
			&rec.Access.ClientAccountID,
			&rec.Access.InstanceID,
			&rec.Access.Status,
			&rec.Access.ProvisionMode,
			&policyRaw,
			&metadataRaw,
			&rec.Access.CreatedAt,
			&rec.Access.UpdatedAt,
			&rec.Client.ID,
			&rec.Client.Username,
			&rec.Client.DisplayName,
			&rec.Client.Email,
			&rec.Client.Status,
			&rec.Client.Notes,
			&rec.Client.ExpiresAt,
			&rec.Client.CreatedAt,
			&rec.Client.UpdatedAt,
			&rec.Instance.ID,
			&rec.Instance.NodeID,
			&rec.Instance.ServiceCode,
			&rec.Instance.Name,
			&rec.Instance.Slug,
			&rec.Instance.SystemdUnit,
			&rec.Instance.Status,
			&rec.Instance.Enabled,
			&rec.Instance.EndpointHost,
			&rec.Instance.EndpointPort,
			&rec.Instance.CreatedAt,
			&rec.Instance.UpdatedAt,
			&specRaw,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(policyRaw, &rec.Access.Policy)
		_ = json.Unmarshal(metadataRaw, &rec.Access.Metadata)
		_ = json.Unmarshal(specRaw, &rec.Instance.Spec)
		if rec.Access.Policy == nil {
			rec.Access.Policy = map[string]any{}
		}
		if rec.Access.Metadata == nil {
			rec.Access.Metadata = map[string]any{}
		}
		if rec.Instance.Spec == nil {
			rec.Instance.Spec = map[string]any{}
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (s *Store) UpdateServiceAccessMetadata(ctx context.Context, accessID string, metadata map[string]any) error {
	if strings.TrimSpace(accessID) == "" {
		return fmt.Errorf("service access id is required")
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	_, err := s.db.Exec(ctx, `update service_accesses set metadata_json=$2,updated_at=now() where id=$1`, accessID, mustJSON(metadata))
	return err
}

func (s *Store) EnsureXrayServiceAccessUUID(ctx context.Context, accessID string) (map[string]any, error) {
	accessID = strings.TrimSpace(accessID)
	if accessID == "" {
		return nil, fmt.Errorf("service access id is required")
	}

	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var clientID string
	var metadataRaw []byte
	if err := tx.QueryRow(ctx, `select client_account_id::text,metadata_json from service_accesses where id=$1 for update`, accessID).Scan(&clientID, &metadataRaw); err != nil {
		return nil, err
	}
	metadata := map[string]any{}
	_ = json.Unmarshal(metadataRaw, &metadata)
	if metadata == nil {
		metadata = map[string]any{}
	}

	forceNew := xrayServiceAccessUUIDRotationRequested(metadata)
	profileKey := xrayClientIdentityProfileKey(metadata)
	uuid := firstString(metadata["xray_uuid"], metadata["uuid"])
	if !forceNew && uuid == "" {
		reusableUUID, err := lookupReusableXrayClientUUIDFromServiceAccessTx(ctx, tx, clientID, accessID)
		if err != nil {
			return nil, err
		}
		uuid = reusableUUID
	}
	identityUUID, err := ensureXrayClientIdentityUUIDTx(ctx, tx, clientID, profileKey, uuid, forceNew)
	if err != nil {
		return nil, err
	}
	if identityUUID != "" && (uuid != identityUUID || forceNew || metadata["xray_identity_key"] == nil) {
		uuid = identityUUID
		metadata["xray_uuid"] = uuid
		metadata["xray_identity_key"] = profileKey
		delete(metadata, "uuid")
		delete(metadata, "rotate_credentials")
		delete(metadata, "force_new_xray_uuid")
		if _, err := tx.Exec(ctx, `update service_accesses set metadata_json=$2,updated_at=now() where id=$1`, accessID, mustJSON(metadata)); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return metadata, nil
}

func (s *Store) serviceAccessForcesNewXrayUUID(ctx context.Context, clientID, instanceID string) (bool, error) {
	clientID = strings.TrimSpace(clientID)
	instanceID = strings.TrimSpace(instanceID)
	if clientID == "" || instanceID == "" {
		return false, nil
	}
	var metadataRaw []byte
	err := s.db.QueryRow(ctx, `select metadata_json from service_accesses where client_account_id=$1 and instance_id=$2`, clientID, instanceID).Scan(&metadataRaw)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	metadata := map[string]any{}
	_ = json.Unmarshal(metadataRaw, &metadata)
	return xrayServiceAccessUUIDRotationRequested(metadata), nil
}

func (s *Store) lookupReusableXrayClientUUID(ctx context.Context, clientID, excludedAccessID string) (string, error) {
	if uuid, err := lookupXrayClientIdentityUUIDTx(ctx, s.db, clientID, defaultXrayClientIdentityProfileKey); err != nil {
		return "", err
	} else if uuid != "" {
		return uuid, nil
	}
	return lookupReusableXrayClientUUIDTx(ctx, s.db, clientID, excludedAccessID)
}

type xrayClientUUIDQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func lookupReusableXrayClientUUIDTx(ctx context.Context, q clientServiceIdentityQuerier, clientID, excludedAccessID string) (string, error) {
	if uuid, err := lookupXrayClientIdentityUUIDTx(ctx, q, clientID, defaultXrayClientIdentityProfileKey); err != nil {
		return "", err
	} else if uuid != "" {
		return uuid, nil
	}
	return lookupReusableXrayClientUUIDFromServiceAccessTx(ctx, q, clientID, excludedAccessID)
}

func lookupReusableXrayClientUUIDFromServiceAccessTx(ctx context.Context, q xrayClientUUIDQuerier, clientID, excludedAccessID string) (string, error) {
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return "", nil
	}
	var uuid string
	err := q.QueryRow(ctx, `select coalesce(nullif(sa.metadata_json->>'xray_uuid',''), nullif(sa.metadata_json->>'uuid',''), '') as client_uuid
		from service_accesses sa
		join instances i on i.id=sa.instance_id
		join service_definitions sd on sd.id=i.service_definition_id
		where sa.client_account_id=$1
		  and ($2::uuid is null or sa.id <> $2::uuid)
		  and sa.status in ('active','pending')
		  and i.status <> 'deleted'
		  and sd.code in ('xray-core','xray','xray_core')
		  and coalesce(nullif(sa.metadata_json->>'xray_uuid',''), nullif(sa.metadata_json->>'uuid',''), '') <> ''
		  and lower(coalesce(sa.metadata_json->>'rotate_credentials','false')) not in ('true','1','yes','on')
		  and lower(coalesce(sa.metadata_json->>'force_new_xray_uuid','false')) not in ('true','1','yes','on')
		order by case sa.status when 'active' then 0 when 'pending' then 1 else 2 end, sa.updated_at desc
		limit 1`, clientID, nullIfEmpty(strings.TrimSpace(excludedAccessID))).Scan(&uuid)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(uuid), nil
}

func xrayServiceAccessUUIDRotationRequested(metadata map[string]any) bool {
	if metadata == nil {
		return false
	}
	return truthy(metadata["rotate_credentials"]) || truthy(metadata["force_new_xray_uuid"])
}

func (s *Store) SaveArtifactContent(ctx context.Context, clientID string, serviceAccessID *string, artifactType, filename string, content []byte) (domain.Artifact, error) {
	clientID = strings.TrimSpace(clientID)
	if !safeArtifactPathSegment(clientID) {
		return domain.Artifact{}, fmt.Errorf("client id is required")
	}
	artifactType = strings.TrimSpace(artifactType)
	if artifactType == "" {
		return domain.Artifact{}, fmt.Errorf("artifact type is required")
	}
	filename = strings.TrimSpace(filename)
	if filename == "" {
		return domain.Artifact{}, fmt.Errorf("artifact filename is required")
	}
	storageDir := filepath.Join(s.ArtifactRoot(), clientID)
	if serviceAccessID != nil && strings.TrimSpace(*serviceAccessID) != "" {
		accessID := strings.TrimSpace(*serviceAccessID)
		if !safeArtifactPathSegment(accessID) {
			return domain.Artifact{}, fmt.Errorf("service access id is invalid")
		}
		storageDir = filepath.Join(storageDir, accessID)
		serviceAccessID = &accessID
	}
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		return domain.Artifact{}, err
	}
	storagePath := filepath.Join(storageDir, sanitizeArtifactFilename(filename))
	if err := atomicWriteFile(storagePath, content, 0o640); err != nil {
		return domain.Artifact{}, err
	}

	sum := sha256.Sum256(content)
	hash := hex.EncodeToString(sum[:])
	size := int64(len(content))
	now := time.Now().UTC()

	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.Artifact{}, err
	}
	defer tx.Rollback(ctx)

	query := `select id,storage_path from artifacts where client_account_id=$1 and artifact_type=$2 and service_access_id is null`
	args := []any{clientID, artifactType}
	if serviceAccessID != nil && strings.TrimSpace(*serviceAccessID) != "" {
		query = `select id,storage_path from artifacts where client_account_id=$1 and artifact_type=$2 and service_access_id=$3`
		args = append(args, *serviceAccessID)
	}
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return domain.Artifact{}, err
	}
	var existingIDs []string
	var existingPaths []string
	for rows.Next() {
		var existingID string
		var existingPath string
		if err := rows.Scan(&existingID, &existingPath); err != nil {
			rows.Close()
			return domain.Artifact{}, err
		}
		existingIDs = append(existingIDs, existingID)
		existingPaths = append(existingPaths, existingPath)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return domain.Artifact{}, err
	}
	if len(existingIDs) > 0 {
		if _, err := tx.Exec(ctx, `delete from artifacts where id = any($1::uuid[])`, existingIDs); err != nil {
			return domain.Artifact{}, err
		}
	}

	var dbServiceAccessID any
	if serviceAccessID != nil && strings.TrimSpace(*serviceAccessID) != "" {
		dbServiceAccessID = *serviceAccessID
	}
	artifact := domain.Artifact{
		ID:              id.New(),
		ClientAccountID: clientID,
		ArtifactType:    artifactType,
		StoragePath:     storagePath,
		Status:          "ready",
		ServiceAccessID: serviceAccessID,
		ContentHash:     hash,
		SizeBytes:       size,
		CreatedAt:       now,
	}
	_, err = tx.Exec(ctx, `insert into artifacts(id,client_account_id,service_access_id,artifact_type,storage_path,content_hash,size_bytes,status,created_at) values($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		artifact.ID, artifact.ClientAccountID, dbServiceAccessID, artifact.ArtifactType, artifact.StoragePath, artifact.ContentHash, artifact.SizeBytes, artifact.Status, artifact.CreatedAt)
	if err != nil {
		return domain.Artifact{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Artifact{}, err
	}

	for _, oldPath := range existingPaths {
		oldPath = strings.TrimSpace(oldPath)
		if oldPath == "" || oldPath == storagePath {
			continue
		}
		_ = os.Remove(oldPath)
	}
	_, _ = s.CreateAudit(ctx, "system", "artifact.create", "artifact", &artifact.ID, "artifact generated")
	return artifact, nil
}

func safeArtifactPathSegment(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || value == "." || value == ".." {
		return false
	}
	if strings.Contains(value, "/") || strings.Contains(value, "\\") || strings.Contains(value, "..") {
		return false
	}
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			continue
		case r >= 'A' && r <= 'Z':
			continue
		case r >= '0' && r <= '9':
			continue
		case r == '-' || r == '_' || r == '.':
			continue
		default:
			return false
		}
	}
	return true
}

func atomicWriteFile(path string, content []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, content, mode); err != nil {
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func sanitizeArtifactFilename(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "artifact.bin"
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := strings.Trim(strings.TrimSpace(b.String()), "._-")
	if out == "" {
		return "artifact.bin"
	}
	return out
}
