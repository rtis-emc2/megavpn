package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

func (s *Store) RotateServiceAccess(ctx context.Context, clientID, accessID, driver string) (domain.Job, error) {
	driver = strings.ToLower(strings.TrimSpace(driver))
	if clientID == "" || accessID == "" {
		return domain.Job{}, fmt.Errorf("client id and access id are required")
	}
	if driver != "openvpn" && driver != "xray-core" && driver != "xray" && driver != "wireguard" && driver != "ipsec" && driver != "shadowsocks" {
		return domain.Job{}, fmt.Errorf("unsupported rotation driver %q", driver)
	}

	var access domain.ServiceAccess
	var metadataRaw, policyRaw []byte
	var instanceID string
	var serviceCode string
	err := s.db.QueryRow(ctx, `select sa.id,sa.client_account_id,sa.instance_id,sa.status,sa.provision_mode,sa.policy_json,sa.metadata_json,sa.created_at,sa.updated_at,sd.code
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
	)
	if err != nil {
		return domain.Job{}, err
	}
	instanceID = access.InstanceID
	_ = json.Unmarshal(policyRaw, &access.Policy)
	_ = json.Unmarshal(metadataRaw, &access.Metadata)
	if access.Metadata == nil {
		access.Metadata = map[string]any{}
	}

	switch normalizeInstanceRuntimeCode(driver) {
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
		delete(access.Metadata, "xray_uuid")
		delete(access.Metadata, "uuid")
	case "wireguard":
		if normalizeInstanceRuntimeCode(serviceCode) != "wireguard" {
			return domain.Job{}, fmt.Errorf("service access driver mismatch: %s", serviceCode)
		}
		access.Metadata["rotate_credentials"] = true
		delete(access.Metadata, "wireguard_client_private_key_secret_ref_id")
		delete(access.Metadata, "wireguard_client_private_key")
		delete(access.Metadata, "wireguard_client_public_key")
		delete(access.Metadata, "wireguard_client_address")
	case "shadowsocks":
		if normalizeInstanceRuntimeCode(serviceCode) != "shadowsocks" {
			return domain.Job{}, fmt.Errorf("service access driver mismatch: %s", serviceCode)
		}
		access.Metadata["rotate_credentials"] = true
		delete(access.Metadata, "password")
		delete(access.Metadata, "shadowsocks_password")
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
		return domain.Job{}, fmt.Errorf("unsupported rotation driver %q", driver)
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
