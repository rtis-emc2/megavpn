package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/platform/id"
	servicedriver "github.com/rtis-emc2/megavpn/internal/service/driver"
)

func (s *Store) RotateServiceAccess(ctx context.Context, clientID, accessID, driverCode string) (domain.Job, error) {
	driverCode = servicedriver.NormalizeCode(driverCode)
	if clientID == "" || accessID == "" {
		return domain.Job{}, fmt.Errorf("client id and access id are required")
	}
	if !servicedriver.IsProvisioningSupported(driverCode) || driverCode == servicedriver.XL2TPD {
		return domain.Job{}, fmt.Errorf("unsupported rotation driver %q", driverCode)
	}

	var access domain.ServiceAccess
	var metadataRaw, policyRaw []byte
	var instanceID string
	var serviceCode string
	var instanceStatus string
	err := s.db.QueryRow(ctx, `select sa.id,sa.client_account_id,sa.instance_id,sa.status,sa.provision_mode,sa.policy_json,sa.metadata_json,sa.created_at,sa.updated_at,sd.code
			,i.status
		from service_accesses sa
		join instances i on i.id=sa.instance_id
		join service_definitions sd on sd.id=i.service_definition_id
		where sa.id=$1 and sa.client_account_id=$2`,
		accessID, clientID,
	).Scan(
		&access.ID,
		&access.ClientAccountID,
		&access.InstanceID,
		&access.Status,
		&access.ProvisionMode,
		&policyRaw,
		&metadataRaw,
		&access.CreatedAt,
		&access.UpdatedAt,
		&serviceCode,
		&instanceStatus,
	)
	if err != nil {
		return domain.Job{}, err
	}
	instanceID = access.InstanceID
	if strings.EqualFold(strings.TrimSpace(access.Status), "revoked") {
		return domain.Job{}, fmt.Errorf("service access %s is revoked", accessID)
	}
	if strings.EqualFold(strings.TrimSpace(instanceStatus), "deleted") {
		return domain.Job{}, fmt.Errorf("service instance %s is deleted", instanceID)
	}
	if err := decodeJSONField(policyRaw, &access.Policy, "service_accesses.policy_json"); err != nil {
		return domain.Job{}, err
	}
	if err := decodeJSONField(metadataRaw, &access.Metadata, "service_accesses.metadata_json"); err != nil {
		return domain.Job{}, err
	}
	if access.Metadata == nil {
		access.Metadata = map[string]any{}
	}

	switch normalizeInstanceRuntimeCode(driverCode) {
	case "openvpn":
		if normalizeInstanceRuntimeCode(serviceCode) != "openvpn" {
			return domain.Job{}, fmt.Errorf("service access driver mismatch: %s", serviceCode)
		}
		access.Metadata["rotate_credentials"] = true
		delete(access.Metadata, "openvpn_client_cert_secret_ref_id")
		delete(access.Metadata, "openvpn_client_key_secret_ref_id")
	case "xray-core":
		if normalizeInstanceRuntimeCode(serviceCode) != "xray-core" {
			return domain.Job{}, fmt.Errorf("service access driver mismatch: %s", serviceCode)
		}
		profileKey := xrayClientIdentityProfileKey(access.Metadata)
		instanceIDs, err := s.rotateXrayClientIdentityForServiceAccesses(ctx, clientID, profileKey, id.New())
		if err != nil {
			return domain.Job{}, err
		}
		if len(instanceIDs) == 0 {
			instanceIDs = []string{instanceID}
		}
		jobRecord, err := s.ProvisionClient(ctx, clientID, instanceIDs)
		if err != nil {
			return domain.Job{}, err
		}
		_, _ = s.CreateAudit(ctx, "system", "service_access.rotate", "service_access", &accessID, "xray client identity rotated and propagation queued")
		return jobRecord, nil
	case "wireguard":
		if normalizeInstanceRuntimeCode(serviceCode) != "wireguard" {
			return domain.Job{}, fmt.Errorf("service access driver mismatch: %s", serviceCode)
		}
		access.Metadata["rotate_credentials"] = true
		delete(access.Metadata, "wireguard_client_private_key_secret_ref_id")
		delete(access.Metadata, "wireguard_client_private_key")
		delete(access.Metadata, "wireguard_client_public_key")
		delete(access.Metadata, "wireguard_client_address")
	case "mtproto":
		if normalizeInstanceRuntimeCode(serviceCode) != "mtproto" {
			return domain.Job{}, fmt.Errorf("service access driver mismatch: %s", serviceCode)
		}
		access.Metadata["rotate_credentials"] = true
		delete(access.Metadata, "mtproto_secret")
		delete(access.Metadata, "secret")
	case "shadowsocks":
		if normalizeInstanceRuntimeCode(serviceCode) != "shadowsocks" {
			return domain.Job{}, fmt.Errorf("service access driver mismatch: %s", serviceCode)
		}
		access.Metadata["rotate_credentials"] = true
		delete(access.Metadata, "password")
		delete(access.Metadata, "shadowsocks_password")
	case "http_proxy":
		if normalizeInstanceRuntimeCode(serviceCode) != "http_proxy" {
			return domain.Job{}, fmt.Errorf("service access driver mismatch: %s", serviceCode)
		}
		access.Metadata["rotate_credentials"] = true
		delete(access.Metadata, "username")
		delete(access.Metadata, "proxy_username")
		delete(access.Metadata, "http_proxy_username")
		delete(access.Metadata, "password")
		delete(access.Metadata, "proxy_password")
		delete(access.Metadata, "http_proxy_password")
	case "ipsec":
		if normalizeInstanceRuntimeCode(serviceCode) != "ipsec" {
			return domain.Job{}, fmt.Errorf("service access driver mismatch: %s", serviceCode)
		}
		access.Metadata["rotate_credentials"] = true
		delete(access.Metadata, "username")
		delete(access.Metadata, "l2tp_username")
		delete(access.Metadata, "ppp_username")
		delete(access.Metadata, "password")
		delete(access.Metadata, "l2tp_password")
		delete(access.Metadata, "ppp_password")
	default:
		return domain.Job{}, fmt.Errorf("unsupported rotation driver %q", driverCode)
	}

	if _, err := s.db.Exec(ctx, `update service_accesses set status='pending',metadata_json=$2,updated_at=now() where id=$1`, accessID, mustJSON(access.Metadata)); err != nil {
		return domain.Job{}, err
	}

	jobRecord, err := s.ProvisionClient(ctx, clientID, []string{instanceID})
	if err != nil {
		return domain.Job{}, err
	}
	_, _ = s.CreateAudit(ctx, "system", "service_access.rotate", "service_access", &accessID, "service access rotation queued")
	return jobRecord, nil
}

func (s *Store) rotateXrayClientIdentityForServiceAccesses(ctx context.Context, clientID, profileKey, newUUID string) ([]string, error) {
	clientID = strings.TrimSpace(clientID)
	profileKey = normalizeClientServiceIdentityKey(profileKey)
	newUUID = strings.TrimSpace(newUUID)
	if clientID == "" || profileKey == "" || newUUID == "" {
		return nil, fmt.Errorf("client id, profile key and xray uuid are required")
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := ensureXrayClientIdentityUUIDTx(ctx, tx, clientID, profileKey, newUUID, true); err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx, `select
			sa.id::text,
			sa.instance_id::text,
			sa.metadata_json
		from service_accesses sa
		join instances i on i.id=sa.instance_id
		join service_definitions sd on sd.id=i.service_definition_id
		where sa.client_account_id=$1
		  and sa.status <> 'revoked'
		  and i.status <> 'deleted'
		  and sd.code in ('xray-core','xray','xray_core')
		for update`, clientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	instanceSeen := map[string]struct{}{}
	instanceIDs := make([]string, 0)
	for rows.Next() {
		var accessID, instanceID string
		var metadataRaw []byte
		if err := rows.Scan(&accessID, &instanceID, &metadataRaw); err != nil {
			return nil, err
		}
		metadata := map[string]any{}
		if err := decodeJSONField(metadataRaw, &metadata, "service_accesses.metadata_json"); err != nil {
			return nil, err
		}
		if metadata == nil {
			metadata = map[string]any{}
		}
		if xrayClientIdentityProfileKey(metadata) != profileKey {
			continue
		}
		metadata["xray_uuid"] = newUUID
		metadata["xray_identity_key"] = profileKey
		delete(metadata, "uuid")
		delete(metadata, "rotate_credentials")
		delete(metadata, "force_new_xray_uuid")
		if _, err := tx.Exec(ctx, `update service_accesses set status='pending',metadata_json=$2,updated_at=now() where id=$1`, accessID, mustJSON(metadata)); err != nil {
			return nil, err
		}
		if _, ok := instanceSeen[instanceID]; !ok {
			instanceSeen[instanceID] = struct{}{}
			instanceIDs = append(instanceIDs, instanceID)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return instanceIDs, nil
}
