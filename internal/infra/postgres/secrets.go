package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/platform/id"
)

var ErrSecretServiceUnavailable = errors.New("secret service is not configured")

type preparedSecretRef struct {
	ref        domain.SecretRef
	ciphertext []byte
	nonce      []byte
}

type secretRefInsertExecutor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func (s *Store) prepareSecretRef(secretType string, rawValue []byte, meta map[string]any) (preparedSecretRef, error) {
	secretType = strings.TrimSpace(secretType)
	if secretType == "" {
		return preparedSecretRef{}, errors.New("secret_type is required")
	}
	if len(rawValue) == 0 {
		return preparedSecretRef{}, errors.New("secret value is required")
	}
	if !isAllowedSecretType(secretType) {
		return preparedSecretRef{}, errors.New("unsupported secret_type")
	}
	if s.secretSvc == nil {
		return preparedSecretRef{}, ErrSecretServiceUnavailable
	}
	if meta == nil {
		meta = map[string]any{}
	}

	ciphertext, nonce, keyVersion, err := s.secretSvc.Encrypt(rawValue)
	if err != nil {
		return preparedSecretRef{}, err
	}

	ref := domain.SecretRef{
		ID:         id.New(),
		SecretType: secretType,
		KeyVersion: keyVersion,
		Meta:       meta,
		CreatedAt:  time.Now().UTC(),
	}
	return preparedSecretRef{ref: ref, ciphertext: ciphertext, nonce: nonce}, nil
}

func insertPreparedSecretRef(ctx context.Context, exec secretRefInsertExecutor, prepared preparedSecretRef) error {
	_, err := exec.Exec(ctx, `insert into secret_refs(id,secret_type,ciphertext,key_version,nonce,meta_json,created_at) values($1,$2,$3,$4,$5,$6,$7)`,
		prepared.ref.ID, prepared.ref.SecretType, prepared.ciphertext, prepared.ref.KeyVersion, prepared.nonce, mustJSON(prepared.ref.Meta), prepared.ref.CreatedAt)
	return err
}

func (s *Store) CreateSecretRef(ctx context.Context, secretType string, rawValue []byte, meta map[string]any) (domain.SecretRef, error) {
	prepared, err := s.prepareSecretRef(secretType, rawValue, meta)
	if err != nil {
		return domain.SecretRef{}, err
	}
	if err := insertPreparedSecretRef(ctx, s.db, prepared); err != nil {
		return domain.SecretRef{}, err
	}
	_, _ = s.CreateAudit(ctx, "system", "secret_ref.create", "secret_ref", &prepared.ref.ID, "secret ref stored")
	return prepared.ref, nil
}

func (s *Store) GetSecretRef(ctx context.Context, secretRefID string) (domain.SecretRef, error) {
	var ref domain.SecretRef
	var metaRaw []byte
	err := s.db.QueryRow(ctx, `select id,secret_type,key_version,meta_json,created_at,rotated_at from secret_refs where id=$1`, strings.TrimSpace(secretRefID)).
		Scan(&ref.ID, &ref.SecretType, &ref.KeyVersion, &metaRaw, &ref.CreatedAt, &ref.RotatedAt)
	if err != nil {
		return domain.SecretRef{}, err
	}
	if err := decodeJSONField(metaRaw, &ref.Meta, "secret_refs.meta_json"); err != nil {
		return domain.SecretRef{}, err
	}
	if ref.Meta == nil {
		ref.Meta = map[string]any{}
	}
	return ref, nil
}

func (s *Store) ResolveSecretValue(ctx context.Context, secretRefID string) (domain.SecretRef, []byte, error) {
	if s.secretSvc == nil {
		return domain.SecretRef{}, nil, ErrSecretServiceUnavailable
	}
	var ref domain.SecretRef
	var metaRaw, ciphertext, nonce []byte
	err := s.db.QueryRow(ctx, `select id,secret_type,key_version,meta_json,ciphertext,nonce,created_at,rotated_at from secret_refs where id=$1`, strings.TrimSpace(secretRefID)).
		Scan(&ref.ID, &ref.SecretType, &ref.KeyVersion, &metaRaw, &ciphertext, &nonce, &ref.CreatedAt, &ref.RotatedAt)
	if err != nil {
		return domain.SecretRef{}, nil, err
	}
	if err := decodeJSONField(metaRaw, &ref.Meta, "secret_refs.meta_json"); err != nil {
		return domain.SecretRef{}, nil, err
	}
	if ref.Meta == nil {
		ref.Meta = map[string]any{}
	}
	plaintext, err := s.secretSvc.Decrypt(ciphertext, nonce)
	if err != nil {
		return domain.SecretRef{}, nil, err
	}
	return ref, plaintext, nil
}

func isAllowedSecretType(secretType string) bool {
	switch secretType {
	case "password", "uuid", "private_key", "public_key", "certificate", "psk", "ssh_key", "api_token", "opaque":
		return true
	default:
		return false
	}
}
