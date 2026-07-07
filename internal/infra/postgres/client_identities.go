package postgres

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rtis-emc2/megavpn/internal/platform/id"
)

const (
	xrayClientIdentityServiceCode       = "xray-core"
	defaultXrayClientIdentityProfileKey = "vless"
)

type clientServiceIdentityQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func xrayClientIdentityProfileKey(metadata map[string]any) string {
	key := normalizeClientServiceIdentityKey(firstString(
		metadata["xray_identity_key"],
		metadata["vless_identity_key"],
		metadata["client_identity_key"],
		metadata["profile_key"],
	))
	if key == "" {
		return defaultXrayClientIdentityProfileKey
	}
	return key
}

func normalizeClientServiceIdentityKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		case r == ' ':
			b.WriteByte('_')
		}
	}
	return strings.Trim(b.String(), "._-")
}

func lookupXrayClientIdentityUUIDTx(ctx context.Context, q clientServiceIdentityQuerier, clientID, profileKey string) (string, error) {
	clientID = strings.TrimSpace(clientID)
	profileKey = normalizeClientServiceIdentityKey(profileKey)
	if clientID == "" || profileKey == "" {
		return "", nil
	}
	var uuid string
	err := q.QueryRow(ctx, `select coalesce(credential_json->>'xray_uuid','')
		from client_service_identities
		where client_account_id=$1
		  and service_code=$2
		  and profile_key=$3
		  and status='active'
		limit 1`, clientID, xrayClientIdentityServiceCode, profileKey).Scan(&uuid)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(uuid), nil
}

func ensureXrayClientIdentityUUIDTx(ctx context.Context, q clientServiceIdentityQuerier, clientID, profileKey, requestedUUID string, forceNew bool) (string, error) {
	clientID = strings.TrimSpace(clientID)
	profileKey = normalizeClientServiceIdentityKey(profileKey)
	requestedUUID = strings.TrimSpace(requestedUUID)
	if clientID == "" || profileKey == "" {
		return "", nil
	}
	if !forceNew {
		existing, err := lookupXrayClientIdentityUUIDTx(ctx, q, clientID, profileKey)
		if err != nil {
			return "", err
		}
		if existing != "" {
			return existing, nil
		}
	}
	uuid := requestedUUID
	if uuid == "" || forceNew {
		uuid = id.New()
	}
	_, err := q.Exec(ctx, `insert into client_service_identities(
			id, client_account_id, service_code, profile_key, credential_json, status, created_at, updated_at
		) values($1,$2,$3,$4,jsonb_build_object('xray_uuid',$5),'active',now(),now())
		on conflict(client_account_id, service_code, profile_key) do update set
			credential_json=client_service_identities.credential_json || excluded.credential_json,
			status='active',
			updated_at=now()`,
		id.New(), clientID, xrayClientIdentityServiceCode, profileKey, uuid)
	if err != nil {
		return "", err
	}
	return uuid, nil
}
