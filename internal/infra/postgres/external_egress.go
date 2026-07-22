package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/externalegress"
	"github.com/rtis-emc2/megavpn/internal/platform/id"
)

var externalEgressKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{1,62}[a-z0-9]$`)

var externalEgressSecretTypes = map[string]string{
	"config": "opaque", "username": "opaque", "password": "password",
	"private_key": "private_key", "public_key": "public_key",
	"certificate": "certificate", "ca_certificate": "certificate",
	"preshared_key": "psk", "tls_auth_key": "psk", "tls_crypt_key": "psk",
	"pkcs12": "opaque", "tls_crypt_v2_key": "psk", "static_key": "psk", "uuid": "uuid",
}

func (s *Store) ListExternalEgressProfiles(ctx context.Context) ([]domain.ExternalEgressProfile, error) {
	rows, err := s.db.Query(ctx, externalEgressProfileSelect+` where p.status <> 'deleted' order by p.display_name asc, p.created_at asc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.ExternalEgressProfile{}
	for rows.Next() {
		profile, err := scanExternalEgressProfile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, profile)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if err := s.populateExternalEgressProfile(ctx, &out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *Store) GetExternalEgressProfile(ctx context.Context, profileID string) (domain.ExternalEgressProfile, error) {
	profile, err := scanExternalEgressProfile(s.db.QueryRow(ctx, externalEgressProfileSelect+` where p.id=$1`, strings.TrimSpace(profileID)))
	if errors.Is(err, pgx.ErrNoRows) {
		return profile, domain.ErrExternalEgressProfileNotFound
	}
	if err != nil {
		return profile, err
	}
	if err := s.populateExternalEgressProfile(ctx, &profile); err != nil {
		return profile, err
	}
	return profile, nil
}

const externalEgressProfileSelect = `select
	p.id::text,p.profile_key,p.display_name,p.description,p.protocol,p.transport,p.status,p.import_format,
	p.endpoint_host,coalesce(p.endpoint_port,0),p.config_json,p.created_at,p.updated_at,p.deleted_at
	from external_egress_profiles p`

func scanExternalEgressProfile(row interface{ Scan(...any) error }) (domain.ExternalEgressProfile, error) {
	var profile domain.ExternalEgressProfile
	var configRaw []byte
	if err := row.Scan(
		&profile.ID, &profile.ProfileKey, &profile.DisplayName, &profile.Description,
		&profile.Protocol, &profile.Transport, &profile.Status, &profile.ImportFormat,
		&profile.EndpointHost, &profile.EndpointPort, &configRaw,
		&profile.CreatedAt, &profile.UpdatedAt, &profile.DeletedAt,
	); err != nil {
		return profile, err
	}
	if len(configRaw) == 0 {
		configRaw = []byte(`{}`)
	}
	profile.ConfigJSON = json.RawMessage(configRaw)
	if def, ok := externalegress.Definition(profile.Protocol); ok {
		profile.RuntimeSupport = def.RuntimeSupport
	}
	return profile, nil
}

func (s *Store) populateExternalEgressProfile(ctx context.Context, profile *domain.ExternalEgressProfile) error {
	if profile == nil {
		return nil
	}
	rows, err := s.db.Query(ctx, `select purpose from external_egress_profile_secrets where profile_id=$1 order by purpose`, profile.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var purpose string
		if err := rows.Scan(&purpose); err != nil {
			rows.Close()
			return err
		}
		profile.SecretPurposes = append(profile.SecretPurposes, purpose)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	deployments, err := s.listExternalEgressDeployments(ctx, profile.ID)
	if err != nil {
		return err
	}
	profile.Deployments = deployments
	return nil
}

func (s *Store) CreateExternalEgressProfile(ctx context.Context, input domain.ExternalEgressProfileInput, userID *string) (domain.ExternalEgressProfile, error) {
	normalized, err := normalizeExternalEgressProfileInput(input, true)
	if err != nil {
		return domain.ExternalEgressProfile{}, err
	}
	profileID := id.New()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return domain.ExternalEgressProfile{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `insert into external_egress_profiles(
		id,profile_key,display_name,description,protocol,transport,status,import_format,endpoint_host,endpoint_port,config_json,created_by,updated_by,created_at,updated_at
	) values($1,$2,$3,$4,$5,$6,$7,$8,$9,nullif($10,0),$11,nullif($12,'')::uuid,nullif($12,'')::uuid,now(),now())`,
		profileID, normalized.ProfileKey, normalized.DisplayName, normalized.Description, normalized.Protocol,
		normalized.Transport, normalized.Status, normalized.ImportFormat, normalized.EndpointHost,
		normalized.EndpointPort, normalized.ConfigJSON, derefUserID(userID)); err != nil {
		return domain.ExternalEgressProfile{}, err
	}
	for purpose, value := range normalized.Secrets {
		if err := s.putExternalEgressSecretTx(ctx, tx, profileID, purpose, value); err != nil {
			return domain.ExternalEgressProfile{}, err
		}
	}
	if normalized.Status == "active" {
		if err := s.validateExternalEgressProfileSecretsTx(ctx, tx, profileID, normalized.Protocol); err != nil {
			return domain.ExternalEgressProfile{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.ExternalEgressProfile{}, err
	}
	_, _ = s.CreateAudit(ctx, "system", "external_egress.profile.create", "external_egress", &profileID, "external egress profile created")
	return s.GetExternalEgressProfile(ctx, profileID)
}

func (s *Store) UpdateExternalEgressProfile(ctx context.Context, profileID string, input domain.ExternalEgressProfileInput, userID *string) (domain.ExternalEgressProfile, error) {
	existing, err := s.GetExternalEgressProfile(ctx, profileID)
	if err != nil {
		return domain.ExternalEgressProfile{}, err
	}
	if strings.TrimSpace(input.ProfileKey) == "" {
		input.ProfileKey = existing.ProfileKey
	}
	if strings.TrimSpace(input.DisplayName) == "" {
		input.DisplayName = existing.DisplayName
	}
	if strings.TrimSpace(input.Protocol) == "" {
		input.Protocol = existing.Protocol
	}
	if strings.TrimSpace(input.Transport) == "" {
		input.Transport = existing.Transport
	}
	if strings.TrimSpace(input.Status) == "" {
		input.Status = existing.Status
	}
	if strings.TrimSpace(input.ImportFormat) == "" {
		input.ImportFormat = existing.ImportFormat
	}
	if strings.TrimSpace(input.EndpointHost) == "" {
		input.EndpointHost = existing.EndpointHost
	}
	if input.EndpointPort == 0 {
		input.EndpointPort = existing.EndpointPort
	}
	if len(strings.TrimSpace(string(input.ConfigJSON))) == 0 {
		input.ConfigJSON = existing.ConfigJSON
	}
	normalized, err := normalizeExternalEgressProfileInput(input, false)
	if err != nil {
		return domain.ExternalEgressProfile{}, err
	}
	if normalized.Protocol != existing.Protocol {
		return domain.ExternalEgressProfile{}, fmt.Errorf("external egress protocol cannot be changed; create a new profile")
	}
	if normalized.Secrets["config"] == "" && externalEgressRuntimeMetadataChanged(normalized, existing) {
		return domain.ExternalEgressProfile{}, fmt.Errorf("endpoint, transport and import format can change only with a replacement provider config")
	}
	if existing.Status == "active" && normalized.Status != "active" {
		var activeGroups, liveDeployments int
		if err := s.db.QueryRow(ctx, `select count(*) from client_access_groups
			where external_egress_profile_id=$1 and deleted_at is null and status <> 'deleted'`, profileID).Scan(&activeGroups); err != nil {
			return domain.ExternalEgressProfile{}, err
		}
		if err := s.db.QueryRow(ctx, `select count(*) from external_egress_deployments
			where profile_id=$1 and status not in ('inactive','deleted')`, profileID).Scan(&liveDeployments); err != nil {
			return domain.ExternalEgressProfile{}, err
		}
		if activeGroups > 0 || liveDeployments > 0 {
			return domain.ExternalEgressProfile{}, fmt.Errorf("remove this profile from access groups and cleanup all deployments before disabling it")
		}
	}
	runtimeChanged := normalized.Transport != existing.Transport ||
		normalized.ImportFormat != existing.ImportFormat ||
		normalized.EndpointHost != existing.EndpointHost ||
		normalized.EndpointPort != existing.EndpointPort ||
		strings.TrimSpace(string(normalized.ConfigJSON)) != strings.TrimSpace(string(existing.ConfigJSON)) ||
		len(normalized.Secrets) > 0
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return domain.ExternalEgressProfile{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `update external_egress_profiles set
		profile_key=$2,display_name=$3,description=$4,transport=$5,status=$6,import_format=$7,
		endpoint_host=$8,endpoint_port=nullif($9,0),config_json=$10,updated_by=nullif($11,'')::uuid,updated_at=now()
		where id=$1 and status <> 'deleted'`, profileID, normalized.ProfileKey, normalized.DisplayName,
		normalized.Description, normalized.Transport, normalized.Status, normalized.ImportFormat,
		normalized.EndpointHost, normalized.EndpointPort, normalized.ConfigJSON, derefUserID(userID)); err != nil {
		return domain.ExternalEgressProfile{}, err
	}
	for purpose, value := range normalized.Secrets {
		if err := s.putExternalEgressSecretTx(ctx, tx, profileID, purpose, value); err != nil {
			return domain.ExternalEgressProfile{}, err
		}
	}
	if normalized.Status == "active" {
		if err := s.validateExternalEgressProfileSecretsTx(ctx, tx, profileID, normalized.Protocol); err != nil {
			return domain.ExternalEgressProfile{}, err
		}
	}
	if normalized.Status == "active" && runtimeChanged {
		if _, err := tx.Exec(ctx, `update external_egress_deployments
			set status='pending',last_error='profile updated; apply required',updated_at=now()
			where profile_id=$1 and desired_status='active' and status not in ('inactive','deleted')`, profileID); err != nil {
			return domain.ExternalEgressProfile{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.ExternalEgressProfile{}, err
	}
	_, _ = s.CreateAudit(ctx, "system", "external_egress.profile.update", "external_egress", &profileID, "external egress profile updated")
	return s.GetExternalEgressProfile(ctx, profileID)
}

func externalEgressRuntimeMetadataChanged(input domain.ExternalEgressProfileInput, existing domain.ExternalEgressProfile) bool {
	return input.Transport != existing.Transport ||
		input.ImportFormat != existing.ImportFormat ||
		input.EndpointHost != existing.EndpointHost ||
		input.EndpointPort != existing.EndpointPort
}

func (s *Store) DeleteExternalEgressProfile(ctx context.Context, profileID string, userID *string) (domain.ExternalEgressProfile, error) {
	profile, err := s.GetExternalEgressProfile(ctx, profileID)
	if err != nil {
		return profile, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return profile, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var currentStatus string
	if err := tx.QueryRow(ctx, `select status from external_egress_profiles where id=$1 for update`, profileID).Scan(&currentStatus); err != nil {
		return profile, err
	}
	if currentStatus == "deleted" {
		return profile, domain.ErrExternalEgressProfileNotFound
	}
	var activeGroups int
	if err := tx.QueryRow(ctx, `select count(*) from client_access_groups where external_egress_profile_id=$1 and deleted_at is null and status <> 'deleted'`, profileID).Scan(&activeGroups); err != nil {
		return profile, err
	}
	if activeGroups > 0 {
		return profile, fmt.Errorf("external egress profile is referenced by %d active client access groups", activeGroups)
	}
	var liveDeployments int
	if err := tx.QueryRow(ctx, `select count(*) from external_egress_deployments where profile_id=$1 and status not in ('inactive','deleted')`, profileID).Scan(&liveDeployments); err != nil {
		return profile, err
	}
	if liveDeployments > 0 {
		return profile, fmt.Errorf("cleanup all external egress deployments before deleting the profile")
	}
	rows, err := tx.Query(ctx, `select secret_ref_id::text from external_egress_profile_secrets where profile_id=$1 for update`, profileID)
	if err != nil {
		return profile, err
	}
	secretRefIDs := []string{}
	for rows.Next() {
		var secretRefID string
		if err := rows.Scan(&secretRefID); err != nil {
			rows.Close()
			return profile, err
		}
		secretRefIDs = append(secretRefIDs, secretRefID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return profile, err
	}
	rows.Close()
	if _, err := tx.Exec(ctx, `delete from external_egress_profile_secrets where profile_id=$1`, profileID); err != nil {
		return profile, err
	}
	for _, secretRefID := range secretRefIDs {
		if _, err := tx.Exec(ctx, `delete from secret_refs where id=$1`, secretRefID); err != nil {
			return profile, fmt.Errorf("delete external egress secret: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `update external_egress_deployments
		set desired_status='deleted',status='deleted',last_job_id=null,last_error='',updated_at=now()
		where profile_id=$1 and status in ('inactive','deleted')`, profileID); err != nil {
		return profile, err
	}
	if _, err := tx.Exec(ctx, `update external_egress_profiles set status='deleted',deleted_at=now(),updated_by=nullif($2,'')::uuid,updated_at=now() where id=$1`, profileID, derefUserID(userID)); err != nil {
		return profile, err
	}
	if err := tx.Commit(ctx); err != nil {
		return profile, err
	}
	profile.Status = "deleted"
	_, _ = s.CreateAudit(ctx, "system", "external_egress.profile.delete", "external_egress", &profileID, "external egress profile deleted")
	return profile, nil
}

func (s *Store) putExternalEgressSecretTx(ctx context.Context, tx pgx.Tx, profileID, purpose, value string) error {
	purpose = strings.ToLower(strings.TrimSpace(purpose))
	secretType, ok := externalEgressSecretTypes[purpose]
	if !ok {
		return fmt.Errorf("unsupported external egress secret purpose %q", purpose)
	}
	if value == "" {
		return fmt.Errorf("external egress secret %q is empty", purpose)
	}
	if len(value) > externalegress.MaxImportedConfigBytes {
		return fmt.Errorf("external egress secret %q is too large", purpose)
	}
	if s.secretSvc == nil {
		return ErrSecretServiceUnavailable
	}
	var previousSecretID string
	if err := tx.QueryRow(ctx, `select secret_ref_id::text from external_egress_profile_secrets where profile_id=$1 and purpose=$2 for update`, profileID, purpose).Scan(&previousSecretID); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	ciphertext, nonce, keyVersion, err := s.secretSvc.Encrypt([]byte(value))
	if err != nil {
		return err
	}
	secretID := id.New()
	meta := map[string]any{"external_egress_profile_id": profileID, "purpose": purpose}
	if _, err := tx.Exec(ctx, `insert into secret_refs(id,secret_type,ciphertext,key_version,nonce,meta_json,created_at) values($1,$2,$3,$4,$5,$6,now())`,
		secretID, secretType, ciphertext, keyVersion, nonce, mustJSON(meta)); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `insert into external_egress_profile_secrets(profile_id,purpose,secret_ref_id,created_at,updated_at)
		values($1,$2,$3,now(),now()) on conflict(profile_id,purpose) do update set secret_ref_id=excluded.secret_ref_id,updated_at=now()`,
		profileID, purpose, secretID); err != nil {
		return err
	}
	if previousSecretID != "" && previousSecretID != secretID {
		if _, err := tx.Exec(ctx, `delete from secret_refs where id=$1`, previousSecretID); err != nil {
			return fmt.Errorf("delete replaced external egress secret: %w", err)
		}
	}
	return nil
}

func (s *Store) validateExternalEgressProfileSecretsTx(ctx context.Context, tx pgx.Tx, profileID, protocol string) error {
	if s.secretSvc == nil {
		return ErrSecretServiceUnavailable
	}
	var ciphertext, nonce []byte
	if err := tx.QueryRow(ctx, `select sr.ciphertext,sr.nonce
		from external_egress_profile_secrets es join secret_refs sr on sr.id=es.secret_ref_id
		where es.profile_id=$1 and es.purpose='config'`, profileID).Scan(&ciphertext, &nonce); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("active %s profile requires an imported config", protocol)
		}
		return err
	}
	config, err := s.secretSvc.Decrypt(ciphertext, nonce)
	if err != nil {
		return fmt.Errorf("decrypt external egress config for validation: %w", err)
	}
	rows, err := tx.Query(ctx, `select purpose from external_egress_profile_secrets where profile_id=$1`, profileID)
	if err != nil {
		return err
	}
	purposes := map[string]bool{}
	for rows.Next() {
		var purpose string
		if err := rows.Scan(&purpose); err != nil {
			rows.Close()
			return err
		}
		purposes[purpose] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	return validateExternalEgressRuntimeProfile(protocol, config, purposes)
}

func validateExternalEgressRuntimeProfile(protocol string, config []byte, purposes map[string]bool) error {
	required := []string{}
	switch protocol {
	case "openvpn":
		preview, err := externalegress.ParseOpenVPN(config)
		if err != nil {
			return err
		}
		required = preview.RequiredSecrets
	case "wireguard":
		preview, err := externalegress.ParseWireGuard(config)
		if err != nil {
			return err
		}
		if !preview.HasDefaultRoute {
			return fmt.Errorf("active WireGuard external egress profile requires AllowedIPs to contain 0.0.0.0/0")
		}
		required = preview.RequiredSecrets
	case "shadowsocks":
		preview, err := externalegress.ParseShadowsocks(config)
		if err != nil {
			return err
		}
		required = preview.RequiredSecrets
	case "vless":
		preview, err := externalegress.ParseVLESS(config)
		if err != nil {
			return err
		}
		required = preview.RequiredSecrets
	case "l2tp_ipsec":
		preview, err := externalegress.ParseL2TPIPsec(config)
		if err != nil {
			return err
		}
		required = preview.RequiredSecrets
	default:
		return fmt.Errorf("protocol %s runtime is not available", protocol)
	}
	for _, purpose := range required {
		if purpose == "username_password" {
			if !purposes["username"] || !purposes["password"] {
				return fmt.Errorf("active OpenVPN profile requires separate username and password secrets")
			}
			continue
		}
		if purposes["config"] && externalEgressConfigContainsPurpose(protocol, config, purpose) {
			continue
		}
		if !purposes[purpose] {
			return fmt.Errorf("active %s profile requires secret %q", protocol, purpose)
		}
	}
	return nil
}

func externalEgressConfigContainsPurpose(protocol string, config []byte, purpose string) bool {
	switch protocol {
	case "shadowsocks":
		preview, err := externalegress.ParseShadowsocks(config)
		return err == nil && purpose == "password" && preview.HasPassword
	case "vless":
		preview, err := externalegress.ParseVLESS(config)
		return err == nil && purpose == "uuid" && preview.HasCredential
	case "l2tp_ipsec":
		preview, err := externalegress.ParseL2TPIPsec(config)
		if err != nil {
			return false
		}
		return (purpose == "username" && preview.HasUsername) ||
			(purpose == "password" && preview.HasPassword) ||
			(purpose == "preshared_key" && preview.HasPSK)
	default:
		return false
	}
}

func normalizeExternalEgressProfileInput(input domain.ExternalEgressProfileInput, create bool) (domain.ExternalEgressProfileInput, error) {
	input.ProfileKey = strings.ToLower(strings.TrimSpace(input.ProfileKey))
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.Description = strings.TrimSpace(input.Description)
	input.Protocol = externalegress.NormalizeProtocol(input.Protocol)
	input.Transport = strings.ToLower(strings.TrimSpace(input.Transport))
	input.Status = strings.ToLower(strings.TrimSpace(input.Status))
	input.ImportFormat = strings.ToLower(strings.TrimSpace(input.ImportFormat))
	input.EndpointHost = strings.TrimSpace(input.EndpointHost)
	if !externalEgressKeyPattern.MatchString(input.ProfileKey) {
		return input, fmt.Errorf("profile_key must be 3-64 lowercase letters, digits or underscores")
	}
	if input.DisplayName == "" {
		return input, fmt.Errorf("display_name is required")
	}
	def, ok := externalegress.Definition(input.Protocol)
	if !ok {
		return input, fmt.Errorf("unsupported external egress protocol %q", input.Protocol)
	}
	if input.Status == "" {
		input.Status = "draft"
	}
	if input.Status != "draft" && input.Status != "active" && input.Status != "disabled" {
		return input, fmt.Errorf("invalid external egress profile status %q", input.Status)
	}
	if input.Status == "active" && def.RuntimeSupport != externalegress.RuntimeReady {
		return input, fmt.Errorf("protocol %s runtime is %s; create the profile as draft", input.Protocol, def.RuntimeSupport)
	}
	if input.ImportFormat == "" {
		input.ImportFormat = "structured"
	}
	if input.EndpointPort < 0 || input.EndpointPort > 65535 {
		return input, fmt.Errorf("endpoint_port is invalid")
	}
	if len(strings.TrimSpace(string(input.ConfigJSON))) == 0 {
		input.ConfigJSON = []byte(`{}`)
	}
	var config map[string]any
	if err := json.Unmarshal(input.ConfigJSON, &config); err != nil {
		return input, fmt.Errorf("invalid config_json: %w", err)
	}
	if config == nil {
		input.ConfigJSON = []byte(`{}`)
	}
	normalizedSecrets := make(map[string]string, len(input.Secrets))
	for purpose, value := range input.Secrets {
		purpose = strings.ToLower(strings.TrimSpace(purpose))
		if _, ok := externalEgressSecretTypes[purpose]; !ok {
			return input, fmt.Errorf("unsupported external egress secret purpose %q", purpose)
		}
		if strings.TrimSpace(value) == "" {
			return input, fmt.Errorf("external egress secret %q is empty", purpose)
		}
		normalizedSecrets[purpose] = value
	}
	input.Secrets = normalizedSecrets
	configValue := input.Secrets["config"]
	if configValue != "" {
		switch input.Protocol {
		case "openvpn":
			preview, err := externalegress.ParseOpenVPN([]byte(configValue))
			if err != nil {
				return input, err
			}
			input.Transport, input.EndpointHost, input.EndpointPort = preview.Transport, preview.EndpointHost, preview.EndpointPort
		case "wireguard":
			preview, err := externalegress.ParseWireGuard([]byte(configValue))
			if err != nil {
				return input, err
			}
			input.Transport, input.EndpointHost, input.EndpointPort = "udp", preview.EndpointHost, preview.EndpointPort
		case "shadowsocks":
			preview, err := externalegress.ParseShadowsocks([]byte(configValue))
			if err != nil {
				return input, err
			}
			input.Transport, input.EndpointHost, input.EndpointPort = preview.Transport, preview.EndpointHost, preview.EndpointPort
		case "vless":
			preview, err := externalegress.ParseVLESS([]byte(configValue))
			if err != nil {
				return input, err
			}
			input.Transport, input.EndpointHost, input.EndpointPort = preview.Transport, preview.EndpointHost, preview.EndpointPort
		case "l2tp_ipsec":
			preview, err := externalegress.ParseL2TPIPsec([]byte(configValue))
			if err != nil {
				return input, err
			}
			input.Transport, input.EndpointHost, input.EndpointPort = preview.Transport, preview.EndpointHost, preview.EndpointPort
		}
	}
	if input.Status == "active" && def.RuntimeSupport == externalegress.RuntimeReady && configValue == "" && create {
		return input, fmt.Errorf("active %s profile requires an imported config", input.Protocol)
	}
	return input, nil
}

func (s *Store) CreateExternalEgressDeployment(ctx context.Context, profileID string, input domain.ExternalEgressDeploymentInput) (domain.ExternalEgressDeployment, error) {
	profile, err := s.GetExternalEgressProfile(ctx, profileID)
	if err != nil {
		return domain.ExternalEgressDeployment{}, err
	}
	if profile.Status != "active" {
		return domain.ExternalEgressDeployment{}, fmt.Errorf("external egress profile must be active before deployment")
	}
	if profile.RuntimeSupport != externalegress.RuntimeReady {
		return domain.ExternalEgressDeployment{}, fmt.Errorf("protocol %s runtime is not available", profile.Protocol)
	}
	node, err := s.GetNode(ctx, strings.TrimSpace(input.NodeID))
	if err != nil {
		return domain.ExternalEgressDeployment{}, fmt.Errorf("egress node not found: %w", err)
	}
	if node.Status == "retired" {
		return domain.ExternalEgressDeployment{}, fmt.Errorf("external egress deployment requires a non-retired managed node")
	}
	requestedTable := 0
	routingTableInput := strings.TrimSpace(input.RoutingTable)
	if routingTableInput != "" && !strings.EqualFold(routingTableInput, "auto") {
		parsed, err := strconv.Atoi(routingTableInput)
		if err != nil || parsed < 40000 || parsed > 48999 {
			return domain.ExternalEgressDeployment{}, fmt.Errorf("routing_table must be auto or a number between 40000 and 48999")
		}
		requestedTable = parsed
	}
	if input.RouteMetric <= 0 {
		input.RouteMetric = 100
	}
	if input.RouteMetric > 32767 {
		return domain.ExternalEgressDeployment{}, fmt.Errorf("route_metric must be between 1 and 32767")
	}
	if len(strings.TrimSpace(string(input.ConfigJSON))) == 0 {
		input.ConfigJSON = []byte(`{}`)
	}
	var deploymentConfig map[string]any
	if err := json.Unmarshal(input.ConfigJSON, &deploymentConfig); err != nil || deploymentConfig == nil {
		return domain.ExternalEgressDeployment{}, fmt.Errorf("deployment config_json must be a JSON object")
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return domain.ExternalEgressDeployment{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `select pg_advisory_xact_lock(hashtextextended($1,0))`, node.ID); err != nil {
		return domain.ExternalEgressDeployment{}, err
	}
	if profile.Protocol == "l2tp_ipsec" {
		var existing int
		if err := tx.QueryRow(ctx, `select count(*) from external_egress_deployments d
			join external_egress_profiles p on p.id=d.profile_id
			where d.node_id=$1 and d.profile_id<>$2 and p.protocol='l2tp_ipsec'
			and d.desired_status='active' and d.status<>'deleted'`, node.ID, profile.ID).Scan(&existing); err != nil {
			return domain.ExternalEgressDeployment{}, err
		}
		if existing > 0 {
			return domain.ExternalEgressDeployment{}, fmt.Errorf("only one active L2TP/IPsec external egress deployment is supported per node")
		}
		var serverInstanceExists bool
		if err := tx.QueryRow(ctx, `select exists(
			select 1 from instances i
			join service_definitions sd on sd.id=i.service_definition_id
			where i.node_id=$1 and i.enabled=true and i.status<>'deleted' and sd.code='xl2tpd'
		)`, node.ID).Scan(&serverInstanceExists); err != nil {
			return domain.ExternalEgressDeployment{}, err
		}
		if serverInstanceExists {
			return domain.ExternalEgressDeployment{}, fmt.Errorf("L2TP/IPsec external egress cannot share a node with a managed XL2TPD server instance")
		}
	}

	deploymentID := ""
	interfaceName := ""
	routingTable := 0
	fwmark := 0
	err = tx.QueryRow(ctx, `select id::text,interface_name,routing_table::integer,fwmark
		from external_egress_deployments where profile_id=$1 and node_id=$2 for update`, profile.ID, node.ID).
		Scan(&deploymentID, &interfaceName, &routingTable, &fwmark)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return domain.ExternalEgressDeployment{}, err
	}
	if errors.Is(err, pgx.ErrNoRows) {
		deploymentID = id.New()
		allocatedInterface, allocatedTable, allocatedMark, err := allocateExternalEgressNodeResources(ctx, tx, profile.ID, node.ID, requestedTable)
		if err != nil {
			return domain.ExternalEgressDeployment{}, err
		}
		interfaceName, routingTable, fwmark = allocatedInterface, allocatedTable, allocatedMark
		if _, err := tx.Exec(ctx, `insert into external_egress_deployments(
			id,profile_id,node_id,desired_status,status,interface_name,routing_table,fwmark,route_metric,config_json,health_json,created_at,updated_at
		) values($1,$2,$3,'active','pending',$4,$5,$6,$7,$8,'{}'::jsonb,now(),now())`,
			deploymentID, profile.ID, node.ID, interfaceName, strconv.Itoa(routingTable), fwmark, input.RouteMetric, input.ConfigJSON); err != nil {
			return domain.ExternalEgressDeployment{}, err
		}
	} else {
		if requestedTable != 0 && requestedTable != routingTable {
			var conflict bool
			if err := tx.QueryRow(ctx, `select exists(select 1 from external_egress_deployments where node_id=$1 and routing_table=$2 and id<>$3)`, node.ID, strconv.Itoa(requestedTable), deploymentID).Scan(&conflict); err != nil {
				return domain.ExternalEgressDeployment{}, err
			}
			if conflict {
				return domain.ExternalEgressDeployment{}, fmt.Errorf("routing table %d is already reserved on node %s", requestedTable, node.Name)
			}
			routingTable = requestedTable
		}
		if _, err := tx.Exec(ctx, `update external_egress_deployments set desired_status='active',status='pending',
			routing_table=$2,route_metric=$3,config_json=$4,last_error='',updated_at=now() where id=$1`,
			deploymentID, strconv.Itoa(routingTable), input.RouteMetric, input.ConfigJSON); err != nil {
			return domain.ExternalEgressDeployment{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.ExternalEgressDeployment{}, err
	}
	deployment, err := s.getExternalEgressDeploymentByProfileNode(ctx, profile.ID, node.ID)
	if err == nil {
		_, _ = s.CreateAudit(ctx, "system", "external_egress.deployment.create", "external_egress", &deployment.ID, "external egress deployment planned")
	}
	return deployment, err
}

func (s *Store) GetExternalEgressDeployment(ctx context.Context, deploymentID string) (domain.ExternalEgressDeployment, error) {
	deployment, err := scanExternalEgressDeployment(s.db.QueryRow(ctx, externalEgressDeploymentSelect+` where d.id=$1`, strings.TrimSpace(deploymentID)))
	if errors.Is(err, pgx.ErrNoRows) {
		return deployment, domain.ErrExternalEgressDeploymentNotFound
	}
	return deployment, err
}

func (s *Store) getExternalEgressDeploymentByProfileNode(ctx context.Context, profileID, nodeID string) (domain.ExternalEgressDeployment, error) {
	deployment, err := scanExternalEgressDeployment(s.db.QueryRow(ctx, externalEgressDeploymentSelect+` where d.profile_id=$1 and d.node_id=$2`, profileID, nodeID))
	if errors.Is(err, pgx.ErrNoRows) {
		return deployment, domain.ErrExternalEgressDeploymentNotFound
	}
	return deployment, err
}

func (s *Store) listExternalEgressDeployments(ctx context.Context, profileID string) ([]domain.ExternalEgressDeployment, error) {
	rows, err := s.db.Query(ctx, externalEgressDeploymentSelect+` where d.profile_id=$1 and d.status <> 'deleted' order by d.created_at asc`, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.ExternalEgressDeployment{}
	for rows.Next() {
		deployment, err := scanExternalEgressDeployment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, deployment)
	}
	return out, rows.Err()
}

const externalEgressDeploymentSelect = `select
	d.id::text,d.profile_id::text,d.node_id::text,n.name,d.desired_status,d.status,d.interface_name,d.routing_table,
	d.fwmark,d.route_metric,d.config_json,d.health_json,d.last_job_id::text,d.last_error,d.applied_at,d.observed_at,d.created_at,d.updated_at
	from external_egress_deployments d join nodes n on n.id=d.node_id`

func scanExternalEgressDeployment(row interface{ Scan(...any) error }) (domain.ExternalEgressDeployment, error) {
	var deployment domain.ExternalEgressDeployment
	var configRaw, healthRaw []byte
	if err := row.Scan(
		&deployment.ID, &deployment.ProfileID, &deployment.NodeID, &deployment.NodeName,
		&deployment.DesiredStatus, &deployment.Status, &deployment.InterfaceName, &deployment.RoutingTable,
		&deployment.FWMark, &deployment.RouteMetric, &configRaw, &healthRaw, &deployment.LastJobID, &deployment.LastError,
		&deployment.AppliedAt, &deployment.ObservedAt, &deployment.CreatedAt, &deployment.UpdatedAt,
	); err != nil {
		return deployment, err
	}
	deployment.ConfigJSON = json.RawMessage(configRaw)
	deployment.HealthJSON = json.RawMessage(healthRaw)
	return deployment, nil
}

func (s *Store) CreateExternalEgressApplyJob(ctx context.Context, deploymentID string) (domain.Job, error) {
	return s.createExternalEgressJob(ctx, deploymentID, "node.external_egress.apply")
}

func (s *Store) CreateExternalEgressProbeJob(ctx context.Context, deploymentID string) (domain.Job, error) {
	return s.createExternalEgressJob(ctx, deploymentID, "node.external_egress.probe")
}

func (s *Store) CreateExternalEgressCleanupJob(ctx context.Context, deploymentID string) (domain.Job, error) {
	return s.createExternalEgressJob(ctx, deploymentID, "node.external_egress.cleanup")
}

func (s *Store) createExternalEgressJob(ctx context.Context, deploymentID, jobType string) (domain.Job, error) {
	deploymentID = strings.TrimSpace(deploymentID)
	if deploymentID == "" {
		return domain.Job{}, domain.ErrExternalEgressDeploymentNotFound
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return domain.Job{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `select pg_advisory_xact_lock(hashtextextended($1,0))`, deploymentID); err != nil {
		return domain.Job{}, err
	}
	var lockedProfileID string
	if err := tx.QueryRow(ctx, `select profile_id::text from external_egress_deployments where id=$1`, deploymentID).Scan(&lockedProfileID); errors.Is(err, pgx.ErrNoRows) {
		return domain.Job{}, domain.ErrExternalEgressDeploymentNotFound
	} else if err != nil {
		return domain.Job{}, err
	}
	profile, err := scanExternalEgressProfile(tx.QueryRow(ctx, externalEgressProfileSelect+` where p.id=$1 for update`, lockedProfileID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Job{}, domain.ErrExternalEgressProfileNotFound
	}
	if err != nil {
		return domain.Job{}, err
	}
	deployment, err := scanExternalEgressDeployment(tx.QueryRow(ctx, externalEgressDeploymentSelect+` where d.id=$1 for update of d`, deploymentID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Job{}, domain.ErrExternalEgressDeploymentNotFound
	}
	if err != nil {
		return domain.Job{}, err
	}
	if deployment.ProfileID != profile.ID {
		return domain.Job{}, fmt.Errorf("external egress deployment profile changed while queuing job")
	}
	if jobType != "node.external_egress.cleanup" {
		if profile.Status != "active" || deployment.DesiredStatus != "active" {
			return domain.Job{}, fmt.Errorf("external egress profile and deployment must be active")
		}
		if profile.RuntimeSupport != externalegress.RuntimeReady {
			return domain.Job{}, fmt.Errorf("protocol %s runtime is not available", profile.Protocol)
		}
	}
	activeJob, err := scanJob(tx.QueryRow(ctx, `select id,type,scope_type,scope_id,node_id,instance_id,status,priority,
		payload_json,coalesce(result_json,'{}'::jsonb),locked_by,locked_until,created_at,started_at,finished_at
		from jobs where scope_type='external_egress' and scope_id=$1 and status in ('queued','running','retrying')
		order by created_at asc limit 1 for update`, deployment.ID))
	if err == nil {
		if activeJob.Type == jobType {
			return activeJob, nil
		}
		return domain.Job{}, fmt.Errorf("external egress deployment already has active job %s", activeJob.Type)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Job{}, err
	}
	secretRefs := map[string]any{}
	if jobType == "node.external_egress.apply" {
		rows, queryErr := tx.Query(ctx, `select purpose,secret_ref_id::text from external_egress_profile_secrets where profile_id=$1`, profile.ID)
		if queryErr != nil {
			return domain.Job{}, queryErr
		}
		for rows.Next() {
			var purpose, refID string
			if scanErr := rows.Scan(&purpose, &refID); scanErr != nil {
				rows.Close()
				return domain.Job{}, scanErr
			}
			secretRefs[purpose] = refID
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return domain.Job{}, err
		}
	}
	payload := map[string]any{
		"node_id": deployment.NodeID, "profile_id": profile.ID, "deployment_id": deployment.ID,
		"protocol": profile.Protocol, "transport": profile.Transport, "interface_name": deployment.InterfaceName,
		"routing_table": deployment.RoutingTable, "route_metric": deployment.RouteMetric,
		"endpoint_host": profile.EndpointHost, "endpoint_port": profile.EndpointPort,
		"fwmark":      deployment.FWMark,
		"secret_refs": secretRefs,
	}
	if externalEgressUsesLoopbackProxy(profile.Protocol) {
		port, portErr := externalEgressLoopbackPort(deployment.RoutingTable)
		if portErr != nil {
			return domain.Job{}, portErr
		}
		payload["proxy_port"] = port
	}
	job, payloadJSON, err := normalizeJobForInsert(domain.Job{Type: jobType, ScopeType: "external_egress", ScopeID: &deployment.ID, NodeID: &deployment.NodeID, Priority: 45, Payload: payload})
	if err != nil {
		return domain.Job{}, err
	}
	if err := insertJobRow(ctx, tx, job, payloadJSON); err != nil {
		return domain.Job{}, err
	}
	status := "queued"
	if jobType == "node.external_egress.cleanup" {
		if _, err := tx.Exec(ctx, `update external_egress_deployments set desired_status='inactive',status=$2,last_job_id=$3,last_error='',updated_at=now() where id=$1`, deployment.ID, status, job.ID); err != nil {
			return domain.Job{}, err
		}
	} else if _, err := tx.Exec(ctx, `update external_egress_deployments set status=$2,last_job_id=$3,last_error='',updated_at=now() where id=$1`, deployment.ID, status, job.ID); err != nil {
		return domain.Job{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Job{}, err
	}
	_, _ = s.CreateAudit(ctx, "system", "job.create", "job", &job.ID, "job queued")
	_, _ = s.CreateAudit(ctx, "system", jobType, "external_egress", &deployment.ID, "external egress node job queued")
	return job, nil
}

func (s *Store) externalEgressSecretRefs(ctx context.Context, profileID string) (map[string]any, error) {
	rows, err := s.db.Query(ctx, `select purpose,secret_ref_id::text from external_egress_profile_secrets where profile_id=$1`, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]any{}
	for rows.Next() {
		var purpose, refID string
		if err := rows.Scan(&purpose, &refID); err != nil {
			return nil, err
		}
		out[purpose] = refID
	}
	return out, rows.Err()
}

func (s *Store) ApplyExternalEgressJobResult(ctx context.Context, job domain.Job, status string, result map[string]any) error {
	deploymentID := strings.TrimSpace(stringify(job.Payload["deployment_id"]))
	if deploymentID == "" && job.ScopeID != nil {
		deploymentID = *job.ScopeID
	}
	if deploymentID == "" {
		return fmt.Errorf("external egress deployment id is missing from job result")
	}
	nextStatus := "failed"
	if status == "succeeded" {
		switch job.Type {
		case "node.external_egress.cleanup":
			nextStatus = "inactive"
		case "node.external_egress.probe", "node.external_egress.apply":
			nextStatus = "active"
		}
	}
	lastError := ""
	if nextStatus == "failed" {
		lastError = firstString(result["error"], result["message"], "external egress job failed")
	}
	health := result["health"]
	if health == nil {
		health = map[string]any{"status": nextStatus}
	}
	tag, err := s.db.Exec(ctx, `update external_egress_deployments set status=$2,health_json=$3,last_error=$4,
		applied_at=case when $2='active' then now() else applied_at end,observed_at=now(),updated_at=now()
		where id=$1 and last_job_id=$5`,
		deploymentID, nextStatus, mustJSON(health), lastError, job.ID)
	if err != nil {
		return err
	}
	// A newer job may have been queued after this job reached a terminal state.
	// Its deployment status must not be overwritten by a stale completion.
	if tag.RowsAffected() == 0 {
		return nil
	}
	return nil
}

func (s *Store) ensureExternalEgressGroupRouting(ctx context.Context, instance domain.Instance, group domain.ClientAccessGroup, spec map[string]any) (map[string]any, error) {
	profileID := derefOptionalString(group.ExternalEgressProfileID)
	if profileID == "" {
		return spec, nil
	}
	profile, err := s.GetExternalEgressProfile(ctx, profileID)
	if err != nil {
		return nil, err
	}
	deployment, err := s.getExternalEgressDeploymentByProfileNode(ctx, profileID, instance.NodeID)
	if err != nil {
		if errors.Is(err, domain.ErrExternalEgressDeploymentNotFound) {
			return nil, fmt.Errorf("external egress profile %q is not deployed on node %s", profile.DisplayName, instance.NodeID)
		}
		return nil, err
	}
	if deployment.Status != "active" {
		return nil, fmt.Errorf("external egress deployment %q on node %s is %s", profile.DisplayName, instance.NodeID, deployment.Status)
	}

	next, changed, err := applyExternalEgressGroupRoutingSpec(spec, group, profile, deployment)
	if err != nil {
		return nil, err
	}
	if !changed {
		return next, nil
	}
	materialized, err := s.materializeInstanceDriverSpecDefaults(ctx, instance, next)
	if err != nil {
		return nil, fmt.Errorf("materialize external egress routing: %w", err)
	}
	status, _, validationErrors := s.validateInstanceRevisionSpec(ctx, instance, materialized)
	if !in(status, "validated", "applied") {
		return nil, fmt.Errorf("external egress revision is not apply-ready; status=%s errors=%v", status, validationErrors)
	}
	revision, err := s.ReplaceInstanceSpec(ctx, instance.ID, "system:external-egress-group", materialized)
	if err != nil {
		return nil, fmt.Errorf("persist external egress group routing: %w", err)
	}
	return revision.Spec, nil
}

func applyExternalEgressGroupRoutingSpec(spec map[string]any, group domain.ClientAccessGroup, profile domain.ExternalEgressProfile, deployment domain.ExternalEgressDeployment) (map[string]any, bool, error) {
	next, _ := cloneAny(spec).(map[string]any)
	if next == nil {
		next = map[string]any{}
	}
	groupListKey := "vless_groups"
	groups, _ := next[groupListKey].([]any)
	if groups == nil {
		for _, key := range []string{"xray_groups", "outbound_groups"} {
			if candidate, ok := next[key].([]any); ok {
				groupListKey, groups = key, candidate
				break
			}
		}
	}
	if len(groups) == 0 {
		return nil, false, fmt.Errorf("vless group %q is not materialized", group.GroupKey)
	}
	outboundTag := normalizeXrayOutboundTag("external_" + group.GroupKey)
	found := false
	for index, raw := range groups {
		item, _ := cloneAny(raw).(map[string]any)
		if item == nil || normalizeXrayVLESSGroupKey(firstString(item["key"], item["name"], item["id"])) != group.GroupKey {
			continue
		}
		item["outbound_tag"] = outboundTag
		if externalEgressUsesLoopbackProxy(profile.Protocol) {
			port, err := externalEgressLoopbackPort(deployment.RoutingTable)
			if err != nil {
				return nil, false, err
			}
			item["outbound"] = map[string]any{
				"tag": outboundTag, "protocol": "socks",
				"settings": map[string]any{"servers": []any{
					map[string]any{"address": "127.0.0.1", "port": port},
				}},
			}
		} else {
			item["outbound"] = map[string]any{
				"tag":      outboundTag,
				"protocol": "freedom",
				"settings": map[string]any{"domainStrategy": "UseIP"},
				"streamSettings": map[string]any{
					"sockopt": map[string]any{"mark": deployment.FWMark},
				},
			}
		}
		item["egress"] = map[string]any{
			"mode":                       "external_egress",
			"external_egress_profile_id": profile.ID,
			"deployment_id":              deployment.ID,
			"routing_table":              deployment.RoutingTable,
			"interface":                  deployment.InterfaceName,
			"fwmark":                     deployment.FWMark,
		}
		groups[index] = item
		found = true
		break
	}
	if !found {
		return nil, false, fmt.Errorf("vless group %q is not available", group.GroupKey)
	}
	next[groupListKey] = groups
	if string(mustJSON(next)) == string(mustJSON(spec)) {
		return next, false, nil
	}
	return next, true, nil
}

func externalEgressUsesLoopbackProxy(protocol string) bool {
	return protocol == "vless" || protocol == "shadowsocks"
}

func externalEgressLoopbackPort(routingTable string) (int, error) {
	table, err := strconv.Atoi(strings.TrimSpace(routingTable))
	if err != nil || table < 40000 || table > 48999 {
		return 0, fmt.Errorf("external egress routing table is invalid")
	}
	return 20000 + table - 40000, nil
}

func allocateExternalEgressNodeResources(ctx context.Context, tx pgx.Tx, profileID, nodeID string, requestedTable int) (string, int, int, error) {
	rows, err := tx.Query(ctx, `select interface_name,routing_table::integer,fwmark from external_egress_deployments where node_id=$1`, nodeID)
	if err != nil {
		return "", 0, 0, err
	}
	interfaces := map[string]bool{}
	tables := map[int]bool{}
	marks := map[int]bool{}
	for rows.Next() {
		var interfaceName string
		var table, mark int
		if err := rows.Scan(&interfaceName, &table, &mark); err != nil {
			rows.Close()
			return "", 0, 0, err
		}
		interfaces[interfaceName], tables[table], marks[mark] = true, true, true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return "", 0, 0, err
	}
	rows.Close()
	interfaceName := ""
	for attempt := 0; attempt < 65536; attempt++ {
		candidate := externalEgressInterfaceName(profileID, nodeID, attempt)
		if !interfaces[candidate] {
			interfaceName = candidate
			break
		}
	}
	if interfaceName == "" {
		return "", 0, 0, fmt.Errorf("no external egress interface name is available on node")
	}
	routingTable := requestedTable
	if routingTable != 0 && tables[routingTable] {
		return "", 0, 0, fmt.Errorf("routing table %d is already reserved on node", routingTable)
	}
	if routingTable == 0 {
		for attempt := 0; attempt < 9000; attempt++ {
			candidate := externalEgressRoutingTable(profileID, nodeID, attempt)
			if !tables[candidate] {
				routingTable = candidate
				break
			}
		}
	}
	if routingTable == 0 {
		return "", 0, 0, fmt.Errorf("no external egress routing table is available on node")
	}
	fwmark := 0
	for attempt := 0; attempt < 65536; attempt++ {
		candidate := externalEgressFWMark(profileID, attempt)
		if !marks[candidate] {
			fwmark = candidate
			break
		}
	}
	if fwmark == 0 {
		return "", 0, 0, fmt.Errorf("no external egress fwmark is available on node")
	}
	return interfaceName, routingTable, fwmark, nil
}

func externalEgressInterfaceName(profileID, nodeID string, attempt int) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|interface|%d", profileID, nodeID, attempt)))
	return "mgev" + hex.EncodeToString(sum[:5])
}

func externalEgressRoutingTable(profileID, nodeID string, attempt int) int {
	sum := sha256.Sum256([]byte(profileID + "|" + nodeID + "|table"))
	value := int(sum[0])<<8 | int(sum[1])
	return 40000 + (value+attempt)%9000
}

func externalEgressFWMark(profileID string, attempt int) int {
	sum := sha256.Sum256([]byte(profileID + "|fwmark"))
	value := int(sum[0])<<8 | int(sum[1])
	return 0x4d590000 | (value+attempt)&0xffff
}
