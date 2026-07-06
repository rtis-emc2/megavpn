package postgres

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/service/driver"
)

var managedNginxServerNamePattern = regexp.MustCompile(`^(\*\.)?[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*\.?$`)

func (s *Store) GetInstanceWithSpec(ctx context.Context, instanceID string) (domain.Instance, error) {
	instance, err := s.GetInstance(ctx, instanceID)
	if err != nil {
		return domain.Instance{}, err
	}
	spec, err := s.latestInstanceSpec(ctx, instanceID)
	if err != nil {
		return domain.Instance{}, err
	}
	instance.Spec = spec
	return instance, nil
}

func (s *Store) ReplaceInstanceSpec(ctx context.Context, instanceID, source string, spec map[string]any) (domain.InstanceRevision, error) {
	if strings.TrimSpace(instanceID) == "" {
		return domain.InstanceRevision{}, fmt.Errorf("instance id is required")
	}
	if strings.TrimSpace(source) == "" {
		source = "system"
	}
	return s.createInstanceRevision(ctx, instanceID, source, "validated", spec)
}

func (s *Store) RollbackInstanceRevision(ctx context.Context, instanceID, revisionID, source string) (domain.InstanceRevision, error) {
	if strings.TrimSpace(instanceID) == "" {
		return domain.InstanceRevision{}, fmt.Errorf("instance id is required")
	}
	if strings.TrimSpace(source) == "" {
		source = "system"
	}
	instance, err := s.GetInstance(ctx, instanceID)
	if err != nil {
		return domain.InstanceRevision{}, err
	}
	revisionID = strings.TrimSpace(revisionID)
	if revisionID == "" {
		revisionID = derefString(instance.LastAppliedRevisionID)
	}
	if revisionID == "" {
		return domain.InstanceRevision{}, fmt.Errorf("rollback target revision is required")
	}
	if instance.CurrentRevisionID != nil && strings.TrimSpace(*instance.CurrentRevisionID) == revisionID {
		return domain.InstanceRevision{}, fmt.Errorf("selected revision is already current")
	}
	var specRaw []byte
	var status string
	err = s.db.QueryRow(ctx, `select spec_json,status from instance_revisions where id=$1 and instance_id=$2`, revisionID, instanceID).Scan(&specRaw, &status)
	if err != nil {
		return domain.InstanceRevision{}, err
	}
	if !in(strings.TrimSpace(status), "validated", "applied", "superseded") {
		return domain.InstanceRevision{}, fmt.Errorf("selected revision is not rollback-ready; status=%s", strings.TrimSpace(status))
	}
	var spec map[string]any
	_ = json.Unmarshal(specRaw, &spec)
	if spec == nil {
		spec = map[string]any{}
	}
	return s.createInstanceRevision(ctx, instanceID, "rollback:"+source, "validated", spec)
}

func (s *Store) validateInstanceRevisionSpec(ctx context.Context, instance domain.Instance, spec map[string]any) (string, string, []any) {
	rendered, err := s.renderInstancePayloadSpec(ctx, instance, spec)
	if err != nil {
		return "draft", "", []any{map[string]any{"stage": "render", "message": err.Error()}}
	}
	errors := staticInstanceValidationErrors(rendered)
	hash := renderedInstanceSpecHash(rendered)
	if len(errors) > 0 {
		return "draft", hash, errors
	}
	return "validated", hash, []any{}
}

func renderedInstanceSpecHash(spec map[string]any) string {
	b, err := json.Marshal(spec)
	if err != nil {
		return ""
	}
	sum := sha1.Sum(b)
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func staticInstanceValidationErrors(spec map[string]any) []any {
	if spec == nil {
		return []any{map[string]any{"stage": "static_validation", "message": "rendered spec is empty"}}
	}
	if filesRaw := spec["files"]; filesRaw != nil {
		return staticManagedFileErrors(filesRaw)
	}
	if _, hasContent, err := staticSingleConfigContent(spec); err != nil {
		return []any{map[string]any{"stage": "static_validation", "message": err.Error()}}
	} else if !hasContent {
		return []any{map[string]any{"stage": "static_validation", "message": "rendered spec does not contain config_content, config_json or spec.files"}}
	}
	return []any{}
}

func staticManagedFileErrors(raw any) []any {
	list, ok := managedFileItems(raw)
	if !ok {
		return []any{map[string]any{"stage": "static_validation", "message": "spec.files must be an array"}}
	}
	if len(list) == 0 {
		return []any{map[string]any{"stage": "static_validation", "message": "spec.files must not be empty"}}
	}
	errors := make([]any, 0)
	for idx, item := range list {
		fileMap, ok := item.(map[string]any)
		if !ok {
			errors = append(errors, map[string]any{"stage": "static_validation", "message": fmt.Sprintf("spec.files[%d] must be an object", idx)})
			continue
		}
		path := strings.TrimSpace(stringify(fileMap["path"]))
		if path == "" {
			errors = append(errors, map[string]any{"stage": "static_validation", "message": fmt.Sprintf("spec.files[%d].path is required", idx)})
			continue
		}
		content := strings.TrimSpace(stringify(fileMap["content"]))
		if content == "" && fileMap["json"] == nil {
			errors = append(errors, map[string]any{"stage": "static_validation", "message": fmt.Sprintf("spec.files[%d] content/json is required for %s", idx, path)})
		}
	}
	return errors
}

func managedFileItems(raw any) ([]any, bool) {
	switch list := raw.(type) {
	case []any:
		return list, true
	case []map[string]any:
		out := make([]any, len(list))
		for idx := range list {
			out[idx] = list[idx]
		}
		return out, true
	default:
		return nil, false
	}
}

func staticSingleConfigContent(spec map[string]any) (string, bool, error) {
	if content := stringify(spec["config_content"]); content != "" {
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		return content, true, nil
	}
	if rawJSON, ok := spec["config_json"]; ok {
		b, err := json.MarshalIndent(rawJSON, "", "  ")
		if err != nil {
			return "", false, err
		}
		return string(b) + "\n", true, nil
	}
	if rawConfig, ok := spec["config"]; ok {
		switch cfg := rawConfig.(type) {
		case string:
			if strings.TrimSpace(cfg) == "" {
				return "", false, nil
			}
			if !strings.HasSuffix(cfg, "\n") {
				cfg += "\n"
			}
			return cfg, true, nil
		default:
			b, err := json.MarshalIndent(cfg, "", "  ")
			if err != nil {
				return "", false, err
			}
			return string(b) + "\n", true, nil
		}
	}
	return "", false, nil
}

func (s *Store) ListProvisioningAccessesByInstance(ctx context.Context, instanceID string) ([]domain.ProvisioningAccess, error) {
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
	where sa.instance_id=$1 and sa.status in ('pending','active')
	order by ca.username asc, sa.created_at asc`, instanceID)
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

func (s *Store) renderInstancePayloadSpec(ctx context.Context, instance domain.Instance, spec map[string]any) (map[string]any, error) {
	var rendered map[string]any
	var err error
	switch normalizeInstanceRuntimeCode(instance.ServiceCode) {
	case "xray-core":
		rendered, err = s.renderXrayPayloadSpec(ctx, instance, spec)
	case "mtproto":
		rendered, err = s.renderMTProtoPayloadSpec(ctx, instance, spec)
	case "nginx":
		rendered, err = s.renderNginxPayloadSpec(ctx, instance, spec)
	case "http_proxy":
		rendered, err = s.renderHTTPProxyPayloadSpec(ctx, instance, spec)
	case "openvpn":
		rendered, err = s.renderOpenVPNPayloadSpec(ctx, instance, spec)
	case "wireguard":
		rendered, err = s.renderWireGuardPayloadSpec(ctx, instance, spec)
	case "ipsec":
		rendered, err = s.renderIPSecPayloadSpec(ctx, instance, spec)
	case "xl2tpd":
		rendered, err = s.renderXL2TPDPayloadSpec(ctx, instance, spec)
	case "shadowsocks":
		rendered, err = s.renderShadowsocksPayloadSpec(ctx, instance, spec)
	default:
		rendered = cloneMap(spec)
	}
	if err != nil {
		return nil, err
	}
	return attachDefaultNetworkPolicy(instance, rendered), nil
}

func (s *Store) renderXrayPayloadSpec(ctx context.Context, instance domain.Instance, spec map[string]any) (map[string]any, error) {
	spec = cloneMap(spec)
	if privateKey, err := s.resolveSecretText(ctx, spec["reality_private_key_secret_ref_id"], spec["reality_private_key"]); err != nil {
		return nil, err
	} else if privateKey != "" {
		spec["reality_private_key"] = privateKey
	}
	unitName := firstString(spec["systemd_unit"], instance.SystemdUnit, serviceDefaultSystemdUnit("xray-core", instance.Slug))
	configPath := firstString(spec["config_path"], xrayConfigPath(instance, spec))
	if err := validateSystemdExecPathArg(configPath); err != nil {
		return nil, err
	}
	configMode := firstString(spec["config_mode"], "0640")
	files := []map[string]any{}
	if strings.EqualFold(firstString(spec["security"]), "tls") {
		if certID := firstString(spec["certificate_id"]); certID != "" {
			certFiles, certPath, keyPath, err := s.materializePlatformCertificateFiles(ctx, certID, "/etc/megavpn/certs/"+firstString(instance.Slug, "xray"))
			if err != nil {
				return nil, err
			}
			spec["tls_cert_path"] = certPath
			spec["tls_key_path"] = keyPath
			files = append(files, certFiles...)
		}
	}
	if err := s.hydrateXrayVLESSGroupTargetRules(ctx, spec); err != nil {
		return nil, err
	}
	config, err := buildXrayServerConfig(instance, spec)
	if err != nil {
		return nil, err
	}
	files = append(files,
		map[string]any{
			"path": configPath,
			"json": config,
			"mode": configMode,
		},
		map[string]any{
			"path":    "/etc/systemd/system/" + unitName + ".service",
			"content": buildXrayUnitFile(unitName, configPath, instance),
			"mode":    "0644",
		},
	)
	spec["files"] = files
	return spec, nil
}

func (s *Store) renderMTProtoPayloadSpec(ctx context.Context, instance domain.Instance, spec map[string]any) (map[string]any, error) {
	spec = cloneMap(spec)
	unitName := firstString(spec["systemd_unit"], instance.SystemdUnit, serviceDefaultSystemdUnit("mtproto", instance.Slug))
	configPath := firstString(spec["config_path"], mtprotoConfigPath(instance, spec))
	if err := validateSystemdExecPathArg(configPath); err != nil {
		return nil, err
	}
	configMode := firstString(spec["config_mode"], "0640")
	config, err := buildMTProtoServerConfig(instance, spec)
	if err != nil {
		return nil, err
	}
	spec["files"] = []map[string]any{
		{
			"path": configPath,
			"json": config,
			"mode": configMode,
		},
		{
			"path":    "/etc/systemd/system/" + unitName + ".service",
			"content": buildMTProtoUnitFile(unitName, configPath, instance),
			"mode":    "0644",
		},
	}
	return spec, nil
}

func (s *Store) renderNginxPayloadSpec(ctx context.Context, instance domain.Instance, spec map[string]any) (map[string]any, error) {
	spec = cloneMap(spec)
	configPath := firstString(spec["config_path"], "/etc/nginx/conf.d/megavpn-"+firstString(spec["slug"], instance.Slug, "edge")+".conf")
	configMode := firstString(spec["config_mode"], "0644")
	files := []map[string]any{}
	if certID := firstString(spec["certificate_id"]); certID != "" {
		certFiles, certPath, keyPath, err := s.materializePlatformCertificateFiles(ctx, certID, "/etc/megavpn/certs/"+firstString(instance.Slug, "edge"))
		if err != nil {
			return nil, err
		}
		spec["tls_enabled"] = true
		spec["tls_cert_path"] = certPath
		spec["tls_key_path"] = keyPath
		files = append(files, certFiles...)
	}
	config, err := buildNginxServerConfig(instance, spec)
	if err != nil {
		return nil, err
	}
	files = append(files, map[string]any{
		"path":    configPath,
		"content": config,
		"mode":    configMode,
	})
	spec["files"] = files
	return spec, nil
}

func (s *Store) renderHTTPProxyPayloadSpec(ctx context.Context, instance domain.Instance, spec map[string]any) (map[string]any, error) {
	spec = cloneMap(spec)
	unitName := firstString(spec["systemd_unit"], instance.SystemdUnit, serviceDefaultSystemdUnit("http_proxy", instance.Slug))
	configPath := firstString(spec["config_path"], httpProxyConfigPath(instance, spec))
	if err := validateSystemdExecPathArg(configPath); err != nil {
		return nil, err
	}
	configMode := firstString(spec["config_mode"], "0644")
	passwdPath := firstString(spec["passwd_path"], httpProxyPasswdPath(instance, spec))
	passwdMode := firstString(spec["passwd_mode"], "0600")
	config, passwdBody, err := buildHTTPProxyServerConfig(instance, spec, passwdPath)
	if err != nil {
		return nil, err
	}
	files := []map[string]any{{
		"path":    configPath,
		"content": config,
		"mode":    configMode,
	}, {
		"path":    "/etc/systemd/system/" + unitName + ".service",
		"content": buildHTTPProxyUnitFile(unitName, configPath, instance),
		"mode":    "0644",
	}}
	if passwdBody != "" {
		files = append(files, map[string]any{
			"path":    passwdPath,
			"content": passwdBody,
			"mode":    passwdMode,
		})
	}
	spec["files"] = files
	return spec, nil
}

func (s *Store) renderOpenVPNPayloadSpec(ctx context.Context, instance domain.Instance, spec map[string]any) (map[string]any, error) {
	spec = cloneMap(spec)
	var err error
	spec, err = s.EnsureOpenVPNInstanceServerPKI(ctx, instance, spec)
	if err != nil {
		return nil, err
	}
	configPath := openVPNConfigPath(instance, spec)
	baseDir := firstString(spec["runtime_dir"], "/etc/openvpn/server/megavpn-"+firstString(spec["slug"], instance.Slug, "server"))
	caPEM, err := s.resolveSecretText(ctx, spec["ca_cert_secret_ref_id"], spec["ca_pem"])
	if err != nil {
		return nil, err
	}
	serverCertPEM, err := s.resolveSecretText(ctx, spec["server_cert_secret_ref_id"], spec["server_cert_pem"])
	if err != nil {
		return nil, err
	}
	serverKeyPEM, err := s.resolveSecretText(ctx, spec["server_key_secret_ref_id"], spec["server_key_pem"])
	if err != nil {
		return nil, err
	}
	if caPEM == "" || serverCertPEM == "" || serverKeyPEM == "" {
		return nil, fmt.Errorf("openvpn instance pki is incomplete")
	}
	serverConfig := buildOpenVPNServerConfig(instance, spec, baseDir)
	files := []map[string]any{
		{"path": configPath, "content": serverConfig, "mode": firstString(spec["config_mode"], "0644")},
		{"path": baseDir + "/ca.crt", "content": caPEM, "mode": "0644"},
		{"path": baseDir + "/server.crt", "content": serverCertPEM, "mode": "0644"},
		{"path": baseDir + "/server.key", "content": serverKeyPEM, "mode": "0600"},
	}
	if tlsCryptKey, err := s.resolveSecretText(ctx, spec["tls_crypt_secret_ref_id"], spec["tls_crypt_key"]); err == nil && tlsCryptKey != "" {
		files = append(files, map[string]any{"path": baseDir + "/tls-crypt.key", "content": tlsCryptKey, "mode": "0600"})
	} else if err != nil {
		return nil, err
	}
	spec["files"] = files
	return spec, nil
}

func (s *Store) renderWireGuardPayloadSpec(ctx context.Context, instance domain.Instance, spec map[string]any) (map[string]any, error) {
	spec = cloneMap(spec)
	interfaceName := firstString(spec["interface_name"])
	if interfaceName == "" && strings.HasPrefix(instance.SystemdUnit, "wg-quick@") {
		interfaceName = strings.TrimPrefix(instance.SystemdUnit, "wg-quick@")
	}
	if interfaceName == "" {
		interfaceName = driver.WireGuardInterfaceName(firstString(spec["slug"], instance.Slug, instance.Name, instance.ID))
	}
	interfaceName = driver.WireGuardInterfaceName(interfaceName)
	spec["interface_name"] = interfaceName
	if firstString(spec["systemd_unit"]) == "" {
		spec["systemd_unit"] = "wg-quick@" + interfaceName
	}
	configPath := firstString(spec["config_path"], "/etc/wireguard/"+interfaceName+".conf")
	configMode := firstString(spec["config_mode"], "0600")
	config, err := s.buildWireGuardServerConfig(ctx, instance, spec)
	if err != nil {
		return nil, err
	}
	spec["files"] = []map[string]any{{
		"path":    configPath,
		"content": config,
		"mode":    configMode,
	}}
	return spec, nil
}

func openVPNConfigPath(instance domain.Instance, spec map[string]any) string {
	slug := firstString(spec["slug"], instance.Slug, "server")
	defaultPath := "/etc/openvpn/server/" + slug + ".conf"
	configPath := firstString(spec["config_path"])
	if configPath == "" {
		return defaultPath
	}
	if slug != "server" && configPath == "/etc/openvpn/server/server.conf" {
		return defaultPath
	}
	return configPath
}

func (s *Store) buildWireGuardServerConfig(ctx context.Context, instance domain.Instance, spec map[string]any) (string, error) {
	if raw := firstString(spec["config_content"]); raw != "" {
		if !strings.HasSuffix(raw, "\n") {
			raw += "\n"
		}
		return raw, nil
	}

	privateKey, err := s.resolveSecretText(ctx, spec["server_private_key_secret_ref_id"], spec["server_private_key"])
	if err != nil {
		return "", err
	}
	if privateKey == "" {
		return "", fmt.Errorf("wireguard server private key is required")
	}

	address := firstString(spec["server_address"])
	if address == "" {
		return "", fmt.Errorf("wireguard server_address is required")
	}

	listenPort := firstIntValue(spec["listen_port"], spec["server_port"], spec["port"], instance.EndpointPort)
	if listenPort <= 0 {
		listenPort = 51820
	}

	lines := []string{
		"[Interface]",
		"Address = " + address,
		"ListenPort = " + strconv.Itoa(listenPort),
		"PrivateKey = " + privateKey,
	}
	if mtu := firstIntValue(spec["mtu"]); mtu > 0 {
		lines = append(lines, "MTU = "+strconv.Itoa(mtu))
	}
	lines = append(lines, extraServerLines(spec["interface_extra_lines"])...)

	peerList, _ := spec["managed_peers"].([]any)
	for _, item := range peerList {
		peer, _ := cloneAny(item).(map[string]any)
		if peer == nil {
			continue
		}
		publicKey := firstString(peer["public_key"], peer["client_public_key"], peer["wireguard_client_public_key"])
		allowedIPs := firstString(peer["allowed_ips"], peer["client_address"], peer["wireguard_client_address"])
		if publicKey == "" || allowedIPs == "" {
			continue
		}
		lines = append(lines, "", "[Peer]")
		lines = append(lines, "PublicKey = "+publicKey)
		if presharedKey, err := s.resolveSecretText(ctx, peer["preshared_key_secret_ref_id"], peer["preshared_key"]); err != nil {
			return "", err
		} else if presharedKey != "" {
			lines = append(lines, "PresharedKey = "+presharedKey)
		}
		lines = append(lines, "AllowedIPs = "+allowedIPs)
		lines = append(lines, extraServerLines(peer["peer_extra_lines"])...)
	}
	return strings.Join(lines, "\n") + "\n", nil
}

func (s *Store) renderIPSecPayloadSpec(ctx context.Context, instance domain.Instance, spec map[string]any) (map[string]any, error) {
	spec = cloneMap(spec)
	psk, err := s.resolveSecretText(ctx, spec["psk_secret_ref_id"], spec["psk"])
	if err != nil {
		return nil, err
	}
	configPath := firstString(spec["config_path"], "/etc/ipsec.conf")
	secretsPath := firstString(spec["secrets_path"], "/etc/ipsec.secrets")
	config, secrets, err := buildIPSecServerConfig(instance, spec, psk)
	if err != nil {
		return nil, err
	}
	spec["files"] = []map[string]any{
		{"path": configPath, "content": config, "mode": firstString(spec["config_mode"], "0644")},
		{"path": secretsPath, "content": secrets, "mode": firstString(spec["secrets_mode"], "0600")},
	}
	return spec, nil
}

func (s *Store) renderXL2TPDPayloadSpec(ctx context.Context, instance domain.Instance, spec map[string]any) (map[string]any, error) {
	spec = cloneMap(spec)
	configPath := firstString(spec["config_path"], "/etc/xl2tpd/xl2tpd.conf")
	optionsPath := firstString(spec["options_path"], "/etc/ppp/options.xl2tpd")
	chapSecretsPath := firstString(spec["chap_secrets_path"], "/etc/ppp/chap-secrets")
	config, options, chapSecrets, err := buildXL2TPDServerConfig(instance, spec)
	if err != nil {
		return nil, err
	}
	spec["files"] = []map[string]any{
		{"path": configPath, "content": config, "mode": firstString(spec["config_mode"], "0644")},
		{"path": optionsPath, "content": options, "mode": firstString(spec["options_mode"], "0644")},
		{"path": chapSecretsPath, "content": chapSecrets, "mode": firstString(spec["chap_secrets_mode"], "0600")},
	}
	return spec, nil
}

func (s *Store) renderShadowsocksPayloadSpec(ctx context.Context, instance domain.Instance, spec map[string]any) (map[string]any, error) {
	spec = cloneMap(spec)
	if password, err := s.resolveSecretText(ctx, firstString(spec["server_password_secret_ref_id"], spec["password_secret_ref_id"]), firstString(spec["server_password"], spec["password"])); err != nil {
		return nil, err
	} else if password != "" {
		spec["server_password"] = password
	}
	unitName := firstString(spec["systemd_unit"], instance.SystemdUnit, serviceDefaultSystemdUnit("shadowsocks", instance.Slug))
	configPath := firstString(spec["config_path"], shadowsocksConfigPath(instance, spec))
	if err := validateSystemdExecPathArg(configPath); err != nil {
		return nil, err
	}
	configMode := firstString(spec["config_mode"], "0640")
	config, err := buildShadowsocksServerConfig(instance, spec)
	if err != nil {
		return nil, err
	}
	spec["files"] = []map[string]any{
		{
			"path": configPath,
			"json": config,
			"mode": configMode,
		},
		{
			"path":    "/etc/systemd/system/" + unitName + ".service",
			"content": buildShadowsocksUnitFile(unitName, configPath, instance),
			"mode":    "0644",
		},
	}
	return spec, nil
}

func xrayConfigPath(instance domain.Instance, spec map[string]any) string {
	slug := firstString(spec["slug"], instance.Slug, "xray")
	defaultPath := "/usr/local/etc/xray/" + slug + ".json"
	configPath := firstString(spec["config_path"])
	if configPath == "" || configPath == "/usr/local/etc/xray/config.json" {
		return defaultPath
	}
	return configPath
}

func shadowsocksConfigPath(instance domain.Instance, spec map[string]any) string {
	slug := firstString(spec["slug"], instance.Slug, "shadowsocks")
	defaultPath := "/etc/shadowsocks-libev/" + slug + ".json"
	configPath := firstString(spec["config_path"])
	if configPath == "" || configPath == "/etc/shadowsocks-libev/config.json" {
		return defaultPath
	}
	return configPath
}

func (s *Store) materializePlatformCertificateFiles(ctx context.Context, certificateID, baseDir string) ([]map[string]any, string, string, error) {
	certificateID = strings.TrimSpace(certificateID)
	if certificateID == "" {
		return nil, "", "", fmt.Errorf("certificate_id is required")
	}
	item, certPEM, keyPEM, chainPEM, err := s.ResolvePlatformCertificateMaterial(ctx, certificateID)
	if err != nil {
		return nil, "", "", err
	}
	if item.Kind != "leaf" {
		return nil, "", "", fmt.Errorf("certificate_id must reference a leaf certificate")
	}
	if len(keyPEM) == 0 {
		return nil, "", "", fmt.Errorf("selected certificate does not include a private key")
	}
	fullchain := string(certPEM)
	if len(chainPEM) > 0 {
		if !strings.HasSuffix(fullchain, "\n") {
			fullchain += "\n"
		}
		fullchain += string(chainPEM)
	}
	certPath := strings.TrimRight(baseDir, "/") + "/fullchain.pem"
	keyPath := strings.TrimRight(baseDir, "/") + "/privkey.pem"
	files := []map[string]any{
		{
			"path":    certPath,
			"content": fullchain,
			"mode":    "0644",
		},
		{
			"path":    keyPath,
			"content": string(keyPEM),
			"mode":    "0600",
		},
	}
	if len(chainPEM) > 0 {
		files = append(files, map[string]any{
			"path":    strings.TrimRight(baseDir, "/") + "/chain.pem",
			"content": string(chainPEM),
			"mode":    "0644",
		})
	}
	return files, certPath, keyPath, nil
}

func buildXrayServerConfig(instance domain.Instance, spec map[string]any) (map[string]any, error) {
	if spec["config_json"] == nil {
		if rawText := firstString(spec["config_content"]); rawText != "" {
			var parsed map[string]any
			if err := json.Unmarshal([]byte(rawText), &parsed); err == nil && parsed != nil {
				spec["config_json"] = parsed
			}
		}
	}
	if raw := spec["config_json"]; raw != nil {
		cfg, ok := cloneAny(raw).(map[string]any)
		if !ok {
			return nil, fmt.Errorf("xray config_json must be an object")
		}
		managedClients := xrayManagedClientSpecs(spec["managed_clients"])
		clients := xrayManagedClients(spec["managed_clients"])
		tag := firstString(spec["managed_inbound_tag"], "vless-in")
		inbounds, _ := cfg["inbounds"].([]any)
		if len(inbounds) == 0 {
			return nil, fmt.Errorf("xray config_json must contain at least one inbound")
		}
		targetIdx := 0
		for idx, inbound := range inbounds {
			m, _ := inbound.(map[string]any)
			if strings.TrimSpace(stringify(m["tag"])) == tag {
				targetIdx = idx
				break
			}
		}
		inbound, _ := inbounds[targetIdx].(map[string]any)
		if inbound == nil {
			return nil, fmt.Errorf("xray inbound at index %d is invalid", targetIdx)
		}
		settings, _ := inbound["settings"].(map[string]any)
		if settings == nil {
			settings = map[string]any{}
		}
		settings["clients"] = clients
		inbound["settings"] = settings
		if strings.TrimSpace(stringify(inbound["listen"])) == "" {
			inbound["listen"] = firstString(spec["listen"], "0.0.0.0")
		}
		if port := firstIntValue(spec["server_port"], spec["port"], instance.EndpointPort); port > 0 && inbound["port"] == nil {
			inbound["port"] = port
		}
		inbounds[targetIdx] = inbound
		cfg["inbounds"] = inbounds
		applyXrayVLESSGroupRouting(cfg, spec, managedClients)
		applyXrayStatsAPI(cfg, instance, spec)
		return cfg, nil
	}

	port := firstIntValue(spec["server_port"], spec["port"], instance.EndpointPort)
	if port <= 0 {
		port = 443
	}
	managedClients := xrayManagedClientSpecs(spec["managed_clients"])
	clients := xrayManagedClients(spec["managed_clients"])
	network := firstString(spec["network"], spec["type"], spec["transport"], "tcp")
	security := firstString(spec["security"], "reality")
	streamSettings := map[string]any{
		"network":  network,
		"security": security,
	}
	switch security {
	case "reality":
		privateKey := firstString(spec["reality_private_key"])
		if privateKey == "" {
			return nil, fmt.Errorf("xray reality_private_key is required")
		}
		shortIDs := stringList(spec["short_ids"])
		if len(shortIDs) == 0 {
			if shortID := firstString(spec["short_id"]); shortID != "" {
				shortIDs = []string{shortID}
			}
		}
		if len(shortIDs) == 0 {
			return nil, fmt.Errorf("xray short_id is required")
		}
		serverNames := stringList(spec["server_names"])
		if len(serverNames) == 0 {
			if serverName := firstString(spec["server_name"], spec["sni"], instance.EndpointHost); serverName != "" {
				serverNames = []string{serverName}
			}
		}
		if len(serverNames) == 0 {
			return nil, fmt.Errorf("xray server_name is required")
		}
		streamSettings["realitySettings"] = map[string]any{
			"show":        false,
			"dest":        firstString(spec["dest"], "www.cloudflare.com:443"),
			"xver":        0,
			"serverNames": serverNames,
			"privateKey":  privateKey,
			"shortIds":    shortIDs,
		}
	case "tls":
		certPath := firstString(spec["tls_cert_path"])
		keyPath := firstString(spec["tls_key_path"])
		if certPath == "" || keyPath == "" {
			return nil, fmt.Errorf("xray tls_cert_path and tls_key_path are required when security=tls")
		}
		streamSettings["tlsSettings"] = map[string]any{
			"certificates": []any{
				map[string]any{
					"certificateFile": certPath,
					"keyFile":         keyPath,
				},
			},
		}
	}
	switch network {
	case "grpc":
		streamSettings["grpcSettings"] = map[string]any{
			"serviceName": firstString(spec["service_name"], "vless-grpc"),
		}
	case "ws", "http", "httpupgrade":
		streamSettings["network"] = "ws"
		wsPath := firstString(spec["path"], "/ws")
		streamSettings["wsSettings"] = map[string]any{
			"path": wsPath,
			"headers": map[string]any{
				"Host": firstString(spec["server_name"], spec["sni"], instance.EndpointHost),
			},
		}
	}
	cfg := map[string]any{
		"log": map[string]any{
			"loglevel": firstString(spec["loglevel"], "warning"),
		},
		"inbounds": []any{
			map[string]any{
				"tag":      firstString(spec["managed_inbound_tag"], "vless-in"),
				"listen":   firstString(spec["listen"], "0.0.0.0"),
				"port":     port,
				"protocol": "vless",
				"settings": map[string]any{
					"clients":    clients,
					"decryption": "none",
				},
				"streamSettings": streamSettings,
			},
		},
		"outbounds": xrayOutboundsForSpec(nil, spec),
	}
	applyXrayVLESSGroupRouting(cfg, spec, managedClients)
	applyXrayStatsAPI(cfg, instance, spec)
	if sniffing := truthy(spec["sniffing_enabled"]); sniffing || spec["sniffing_enabled"] == nil {
		inbounds := cfg["inbounds"].([]any)
		inbound := inbounds[0].(map[string]any)
		inbound["sniffing"] = map[string]any{
			"enabled":      true,
			"destOverride": []any{"http", "tls", "quic"},
		}
		inbounds[0] = inbound
		cfg["inbounds"] = inbounds
	}
	if alpn := stringList(spec["alpn"]); len(alpn) > 0 {
		inbounds := cfg["inbounds"].([]any)
		inbound := inbounds[0].(map[string]any)
		streamSettings, _ := inbound["streamSettings"].(map[string]any)
		if realitySettings, _ := streamSettings["realitySettings"].(map[string]any); realitySettings != nil {
			realitySettings["alpn"] = alpn
			streamSettings["realitySettings"] = realitySettings
		} else if tlsSettings, _ := streamSettings["tlsSettings"].(map[string]any); tlsSettings != nil {
			tlsSettings["alpn"] = alpn
			streamSettings["tlsSettings"] = tlsSettings
		}
		inbound["streamSettings"] = streamSettings
		inbounds[0] = inbound
		cfg["inbounds"] = inbounds
	}
	return cfg, nil
}

func buildMTProtoServerConfig(instance domain.Instance, spec map[string]any) (map[string]any, error) {
	if spec["config_json"] == nil {
		if rawText := firstString(spec["config_content"]); rawText != "" {
			var parsed map[string]any
			if err := json.Unmarshal([]byte(rawText), &parsed); err == nil && parsed != nil {
				spec["config_json"] = parsed
			}
		}
	}
	if raw := spec["config_json"]; raw != nil {
		cfg, ok := cloneAny(raw).(map[string]any)
		if !ok {
			return nil, fmt.Errorf("mtproto config_json must be an object")
		}
		users := mtprotoManagedUsers(spec["managed_users"])
		inbounds, _ := cfg["inbounds"].([]any)
		if len(inbounds) == 0 {
			return nil, fmt.Errorf("mtproto config_json must contain at least one inbound")
		}
		inbound, _ := inbounds[0].(map[string]any)
		if inbound == nil {
			return nil, fmt.Errorf("mtproto config_json inbound is invalid")
		}
		settings, _ := inbound["settings"].(map[string]any)
		if settings == nil {
			settings = map[string]any{}
		}
		settings["users"] = users
		inbound["settings"] = settings
		if strings.TrimSpace(stringify(inbound["listen"])) == "" {
			inbound["listen"] = firstString(spec["listen"], "0.0.0.0")
		}
		if port := firstIntValue(spec["server_port"], spec["port"], instance.EndpointPort); port > 0 && inbound["port"] == nil {
			inbound["port"] = port
		}
		inbounds[0] = inbound
		cfg["inbounds"] = inbounds
		return cfg, nil
	}

	users := mtprotoManagedUsers(spec["managed_users"])
	if len(users) == 0 {
		return nil, fmt.Errorf("mtproto managed_users are empty")
	}
	port := firstIntValue(spec["server_port"], spec["port"], instance.EndpointPort)
	if port <= 0 {
		port = 443
	}
	cfg := map[string]any{
		"log": map[string]any{
			"loglevel": firstString(spec["loglevel"], "warning"),
		},
		"inbounds": []any{
			map[string]any{
				"tag":      firstString(spec["managed_inbound_tag"], "mtproto-in"),
				"listen":   firstString(spec["listen"], "0.0.0.0"),
				"port":     port,
				"protocol": "mtproto",
				"settings": map[string]any{
					"users": users,
				},
			},
		},
		"outbounds": []any{
			map[string]any{"protocol": "freedom", "tag": "direct"},
			map[string]any{"protocol": "blackhole", "tag": "block"},
		},
	}
	return cfg, nil
}

func buildHTTPProxyServerConfig(instance domain.Instance, spec map[string]any, passwdPath string) (string, string, error) {
	if raw := firstString(spec["config_content"]); raw != "" {
		if !strings.HasSuffix(raw, "\n") {
			raw += "\n"
		}
		return raw, firstString(spec["passwd_body"]), nil
	}
	port := firstIntValue(spec["listen_port"], spec["server_port"], spec["port"], instance.EndpointPort)
	if port <= 0 {
		port = 3128
	}
	managedAccounts := httpProxyManagedAccounts(spec["managed_accounts"])
	httpAccessRule := firstString(spec["http_access_rule"], "allow authenticated_users")
	lines := []string{
		"http_port " + strconv.Itoa(port),
		"visible_hostname " + firstString(spec["visible_hostname"], instance.EndpointHost, instance.Name, "megavpn-proxy"),
		"access_log " + firstString(spec["access_log"], httpProxyAccessLogPath(instance, spec)),
		"cache_log " + firstString(spec["cache_log"], httpProxyCacheLogPath(instance, spec)),
		"pid_filename " + firstString(spec["pid_filename"], httpProxyPIDPath(instance, spec)),
	}
	passwdLines := []string{}
	if len(managedAccounts) > 0 {
		authHelperPath := firstString(spec["auth_helper_path"], "/usr/lib/squid/basic_ncsa_auth")
		lines = append(lines,
			"auth_param basic program "+authHelperPath+" "+passwdPath,
			"auth_param basic realm "+firstString(spec["auth_realm"], "RTIS MegaVPN HTTP Proxy"),
			"acl authenticated_users proxy_auth REQUIRED",
			"http_access "+httpAccessRule,
			"http_access deny all",
		)
		for _, account := range managedAccounts {
			passwdLines = append(passwdLines, account.Username+":"+account.PasswordHash)
		}
	} else {
		lines = append(lines, "http_access allow all")
	}
	lines = append(lines,
		"request_header_access X-Forwarded-For deny all",
		"via off",
		"forwarded_for delete",
	)
	lines = append(lines, extraServerLines(spec["config_extra_lines"])...)
	return strings.Join(lines, "\n") + "\n", strings.Join(passwdLines, "\n"), nil
}

func buildOpenVPNServerConfig(instance domain.Instance, spec map[string]any, baseDir string) string {
	if raw := firstString(spec["config_content"]); raw != "" {
		replacer := strings.NewReplacer(
			"{{CA_CERT_PATH}}", baseDir+"/ca.crt",
			"{{SERVER_CERT_PATH}}", baseDir+"/server.crt",
			"{{SERVER_KEY_PATH}}", baseDir+"/server.key",
			"{{TLS_CRYPT_PATH}}", baseDir+"/tls-crypt.key",
			"{{INSTANCE_SLUG}}", firstString(instance.Slug, "server"),
		)
		cfg := replacer.Replace(raw)
		if !strings.Contains(cfg, "\nca ") && !strings.Contains(cfg, "\nca\t") && !strings.HasPrefix(cfg, "ca ") {
			cfg += "\nca " + baseDir + "/ca.crt"
		}
		if !strings.Contains(cfg, "\ncert ") && !strings.Contains(cfg, "\ncert\t") && !strings.HasPrefix(cfg, "cert ") {
			cfg += "\ncert " + baseDir + "/server.crt"
		}
		if !strings.Contains(cfg, "\nkey ") && !strings.Contains(cfg, "\nkey\t") && !strings.HasPrefix(cfg, "key ") {
			cfg += "\nkey " + baseDir + "/server.key"
		}
		if firstString(spec["tls_crypt_secret_ref_id"], spec["tls_crypt_key"]) != "" && !strings.Contains(cfg, "tls-crypt ") {
			cfg += "\ntls-crypt " + baseDir + "/tls-crypt.key"
		}
		if !strings.HasSuffix(cfg, "\n") {
			cfg += "\n"
		}
		return cfg
	}

	port := firstIntValue(spec["server_port"], spec["port"], instance.EndpointPort)
	if port <= 0 {
		port = 1194
	}
	proto := firstString(spec["proto"], "tcp")
	dev := firstString(spec["dev"], "tun")
	serverNetwork := firstString(spec["server_network"], "10.8.0.0")
	serverNetmask := firstString(spec["server_netmask"], "255.255.255.0")
	lines := []string{
		"port " + strconv.Itoa(port),
		"proto " + proto,
		"dev " + dev,
		"topology subnet",
		"server " + serverNetwork + " " + serverNetmask,
		"ifconfig-pool-persist /var/lib/megavpn/openvpn/" + firstString(instance.Slug, "server") + "/ipp.txt",
		"persist-key",
		"persist-tun",
		"keepalive 10 120",
		"user nobody",
		"group nogroup",
		"ca " + baseDir + "/ca.crt",
		"cert " + baseDir + "/server.crt",
		"key " + baseDir + "/server.key",
		"dh none",
		"ecdh-curve prime256v1",
		"client-to-client",
		"duplicate-cn",
		"verb 3",
	}
	if cipher := firstString(spec["cipher"]); cipher != "" {
		lines = append(lines, "cipher "+cipher)
	} else {
		lines = append(lines, "data-ciphers AES-256-GCM:AES-128-GCM")
	}
	if auth := firstString(spec["auth"]); auth != "" {
		lines = append(lines, "auth "+auth)
	}
	if tlsCrypt := firstString(spec["tls_crypt_secret_ref_id"], spec["tls_crypt_key"]); tlsCrypt != "" {
		lines = append(lines, "tls-crypt "+baseDir+"/tls-crypt.key")
	}
	lines = append(lines, extraServerLines(spec["server_extra_lines"])...)
	return strings.Join(lines, "\n") + "\n"
}

func buildIPSecServerConfig(instance domain.Instance, spec map[string]any, psk string) (string, string, error) {
	if raw := firstString(spec["config_content"]); raw != "" {
		secrets := firstString(spec["secrets_content"])
		if secrets == "" {
			if psk == "" {
				return "", "", fmt.Errorf("ipsec psk is required")
			}
			secrets = `%any %any : PSK "` + psk + `"` + "\n"
		}
		if !strings.HasSuffix(raw, "\n") {
			raw += "\n"
		}
		if !strings.HasSuffix(secrets, "\n") {
			secrets += "\n"
		}
		return raw, secrets, nil
	}
	if psk == "" {
		return "", "", fmt.Errorf("ipsec psk is required")
	}

	left := firstString(spec["left"], "%defaultroute")
	leftID := firstString(spec["leftid"], spec["server_id"], instance.EndpointHost)
	right := firstString(spec["right"], "%any")
	connName := firstString(spec["conn_name"], "megavpn-l2tp")
	lines := []string{
		"config setup",
		"    uniqueids=no",
		"",
		"conn " + connName,
		"    auto=" + firstString(spec["auto"], "add"),
		"    keyexchange=" + firstString(spec["keyexchange"], "ikev1"),
		"    authby=" + firstString(spec["authby"], "secret"),
		"    type=" + firstString(spec["type"], "transport"),
		"    left=" + left,
	}
	if leftID != "" {
		lines = append(lines, "    leftid="+leftID)
	}
	lines = append(lines,
		"    leftprotoport="+firstString(spec["leftprotoport"], "17/1701"),
		"    right="+right,
		"    rightprotoport="+firstString(spec["rightprotoport"], "17/%any"),
		"    ike="+firstString(spec["ike"], "aes256-sha1-modp1024"),
		"    esp="+firstString(spec["esp"], "aes256-sha1"),
		"    dpdaction="+firstString(spec["dpdaction"], "clear"),
		"    rekey="+firstString(spec["rekey"], "no"),
		"    forceencaps="+firstString(spec["forceencaps"], "yes"),
	)
	lines = append(lines, extraIndentedLines(spec["config_extra_lines"], "    ")...)
	secrets := firstString(spec["secrets_content"])
	if secrets == "" {
		leftSecretID := leftID
		if leftSecretID == "" {
			leftSecretID = "%any"
		}
		secrets = leftSecretID + ` %any : PSK "` + psk + `"` + "\n"
	} else if !strings.HasSuffix(secrets, "\n") {
		secrets += "\n"
	}
	return strings.Join(lines, "\n") + "\n", secrets, nil
}

func buildXL2TPDServerConfig(instance domain.Instance, spec map[string]any) (string, string, string, error) {
	if raw := firstString(spec["config_content"]); raw != "" {
		options := firstString(spec["options_content"])
		if options == "" {
			options = defaultXL2TPDOptions(spec)
		}
		chapSecrets := firstString(spec["chap_secrets_content"])
		if chapSecrets == "" {
			chapSecrets = defaultXL2TPDChapSecrets(spec)
		}
		if strings.TrimSpace(chapSecrets) == "" {
			return "", "", "", fmt.Errorf("xl2tpd chap secrets are required")
		}
		if !strings.HasSuffix(raw, "\n") {
			raw += "\n"
		}
		if !strings.HasSuffix(options, "\n") {
			options += "\n"
		}
		if !strings.HasSuffix(chapSecrets, "\n") {
			chapSecrets += "\n"
		}
		return raw, options, chapSecrets, nil
	}

	localIP := firstString(spec["local_ip"], "10.20.0.1")
	rangeStart := firstString(spec["ip_range_start"], "10.20.0.10")
	rangeEnd := firstString(spec["ip_range_end"], "10.20.0.200")
	pppOpt := firstString(spec["pppoptfile"], firstString(spec["options_path"], "/etc/ppp/options.xl2tpd"))
	lines := []string{
		"[global]",
		"port = " + strconv.Itoa(firstIntValue(spec["listen_port"], spec["server_port"], spec["port"], instance.EndpointPort, 1701)),
		"",
		"[lns default]",
		"ip range = " + rangeStart + "-" + rangeEnd,
		"local ip = " + localIP,
		"require chap = " + firstString(spec["require_chap"], "yes"),
		"refuse pap = " + firstString(spec["refuse_pap"], "yes"),
		"require authentication = " + firstString(spec["require_authentication"], "yes"),
		"name = " + firstString(spec["name"], "megavpn-l2tpd"),
		"pppoptfile = " + pppOpt,
		"length bit = " + firstString(spec["length_bit"], "yes"),
	}
	lines = append(lines, extraIndentedLines(spec["config_extra_lines"], "")...)
	options := firstString(spec["options_content"])
	if options == "" {
		options = defaultXL2TPDOptions(spec)
	}
	chapSecrets := firstString(spec["chap_secrets_content"])
	if chapSecrets == "" {
		chapSecrets = defaultXL2TPDChapSecrets(spec)
	}
	if strings.TrimSpace(chapSecrets) == "" {
		return "", "", "", fmt.Errorf("xl2tpd chap secrets are required")
	}
	if !strings.HasSuffix(options, "\n") {
		options += "\n"
	}
	if !strings.HasSuffix(chapSecrets, "\n") {
		chapSecrets += "\n"
	}
	return strings.Join(lines, "\n") + "\n", options, chapSecrets, nil
}

func buildNginxServerConfig(instance domain.Instance, spec map[string]any) (string, error) {
	if raw := firstString(spec["config_content"]); raw != "" {
		if !strings.HasSuffix(raw, "\n") {
			raw += "\n"
		}
		return raw, nil
	}

	mode := firstString(spec["mode"], "reverse_proxy")
	listenPort := firstIntValue(spec["listen_port"], spec["server_port"], spec["port"], instance.EndpointPort)
	if listenPort <= 0 {
		listenPort = 8080
	}
	serverName := firstString(spec["server_name"], instance.EndpointHost, "_")
	if err := validateNginxServerName(serverName); err != nil {
		return "", err
	}
	lines := []string{}
	if truthy(spec["tls_enabled"]) {
		if nginxHTTPRedirectEnabled(spec) {
			redirectServerName := firstString(spec["http_redirect_server_name"], spec["redirect_server_name"], serverName)
			if err := validateNginxServerName(redirectServerName); err != nil {
				return "", err
			}
			lines = appendNginxHTTPRedirectServer(lines, redirectServerName, listenPort)
		}
		lines = append(lines, "server {")
		certPath := firstString(spec["tls_cert_path"])
		keyPath := firstString(spec["tls_key_path"])
		if certPath == "" || keyPath == "" {
			return "", fmt.Errorf("nginx tls_cert_path and tls_key_path are required when tls_enabled=true")
		}
		listenLine := "    listen " + strconv.Itoa(listenPort) + " ssl"
		if mode == "grpc_proxy" || truthy(spec["http2_enabled"]) {
			listenLine += " http2"
		}
		lines = append(lines, listenLine+";")
		lines = append(lines, "    ssl_certificate "+certPath+";")
		lines = append(lines, "    ssl_certificate_key "+keyPath+";")
	} else {
		lines = append(lines, "server {")
		lines = append(lines, "    listen "+strconv.Itoa(listenPort)+";")
	}
	lines = append(lines, "    server_name "+serverName+";")
	if clientMaxBodySize := firstString(spec["client_max_body_size"]); clientMaxBodySize != "" {
		lines = append(lines, "    client_max_body_size "+clientMaxBodySize+";")
	}
	if accessLog := firstString(spec["access_log"]); accessLog != "" {
		lines = append(lines, "    access_log "+accessLog+";")
	}
	if errorLog := firstString(spec["error_log"]); errorLog != "" {
		lines = append(lines, "    error_log "+errorLog+";")
	}

	switch mode {
	case "reverse_proxy":
		upstreamURL := firstString(spec["upstream_url"], spec["proxy_pass"])
		if upstreamURL == "" {
			return "", fmt.Errorf("nginx upstream_url is required for reverse_proxy mode")
		}
		if err := validateNginxDirectiveValue("upstream_url", upstreamURL); err != nil {
			return "", err
		}
		locationPath := firstString(spec["location_path"], "/")
		if err := validateNginxLocationPath(locationPath); err != nil {
			return "", err
		}
		if err := validateNginxFallbackSpec(spec, instance, serverName); err != nil {
			return "", err
		}
		lines = appendNginxReverseProxyLocation(lines, locationPath, upstreamURL, spec, "location_extra_lines")
		lines = appendNginxFallbackLocation(lines, locationPath, spec)
	case "grpc_proxy":
		upstreamURL := firstString(spec["upstream_url"], spec["grpc_pass"])
		if upstreamURL == "" {
			return "", fmt.Errorf("nginx upstream_url is required for grpc_proxy mode")
		}
		if err := validateNginxDirectiveValue("upstream_url", upstreamURL); err != nil {
			return "", err
		}
		locationPath := firstString(spec["location_path"], "/")
		if err := validateNginxLocationPath(locationPath); err != nil {
			return "", err
		}
		if err := validateNginxFallbackSpec(spec, instance, serverName); err != nil {
			return "", err
		}
		lines = append(lines, "    location "+locationPath+" {")
		lines = append(lines, "        grpc_pass "+upstreamURL+";")
		lines = append(lines,
			"        grpc_set_header Host $host;",
			"        grpc_set_header X-Real-IP $remote_addr;",
			"        grpc_set_header X-Forwarded-For $proxy_add_x_forwarded_for;",
			"        grpc_set_header X-Forwarded-Proto $scheme;",
		)
		if timeout := firstString(spec["grpc_read_timeout"]); timeout != "" {
			lines = append(lines, "        grpc_read_timeout "+timeout+";")
		}
		if timeout := firstString(spec["grpc_send_timeout"]); timeout != "" {
			lines = append(lines, "        grpc_send_timeout "+timeout+";")
		}
		lines = append(lines, extraIndentedLines(spec["location_extra_lines"], "        ")...)
		lines = append(lines, "    }")
		lines = appendNginxFallbackLocation(lines, locationPath, spec)
	case "static":
		rootDir := firstString(spec["root_dir"])
		if rootDir == "" {
			return "", fmt.Errorf("nginx root_dir is required for static mode")
		}
		lines = append(lines, "    root "+rootDir+";")
		lines = append(lines, "    index "+firstString(spec["index_files"], "index.html index.htm")+";")
		lines = append(lines,
			"    location / {",
			"        try_files $uri $uri/ =404;",
		)
		lines = append(lines, extraIndentedLines(spec["location_extra_lines"], "        ")...)
		lines = append(lines, "    }")
	default:
		return "", fmt.Errorf("unsupported nginx mode %q", mode)
	}

	lines = append(lines, extraIndentedLines(spec["server_extra_lines"], "    ")...)
	lines = append(lines, "}")
	return strings.Join(lines, "\n") + "\n", nil
}

func nginxHTTPRedirectEnabled(spec map[string]any) bool {
	for _, key := range []string{"http_to_https_redirect", "redirect_http_to_https", "http_redirect_enabled", "http_redirect"} {
		if raw, ok := spec[key]; ok {
			return truthy(raw)
		}
	}
	return true
}

func appendNginxHTTPRedirectServer(lines []string, serverName string, httpsPort int) []string {
	redirectTarget := "https://$host$request_uri"
	if httpsPort > 0 && httpsPort != 443 {
		redirectTarget = "https://$host:" + strconv.Itoa(httpsPort) + "$request_uri"
	}
	return append(lines,
		"server {",
		"    listen 80;",
		"    server_name "+serverName+";",
		"    return 301 "+redirectTarget+";",
		"}",
		"",
	)
}

func appendNginxReverseProxyLocation(lines []string, locationPath, upstreamURL string, spec map[string]any, extraLinesKey string) []string {
	lines = append(lines, "    location "+locationPath+" {")
	lines = append(lines, "        proxy_pass "+upstreamURL+";")
	if spec["proxy_http_version"] != nil || truthy(spec["proxy_headers_enabled"]) || spec["proxy_headers_enabled"] == nil {
		lines = append(lines, "        proxy_http_version "+firstString(spec["proxy_http_version"], "1.1")+";")
	}
	if truthy(spec["proxy_headers_enabled"]) || spec["proxy_headers_enabled"] == nil {
		lines = append(lines,
			"        proxy_set_header Host $host;",
			"        proxy_set_header X-Real-IP $remote_addr;",
			"        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;",
			"        proxy_set_header X-Forwarded-Proto $scheme;",
		)
	}
	if truthy(spec["websocket_upgrade"]) || truthy(spec["websocket_upgrade_enabled"]) || truthy(spec["ws_upgrade"]) {
		lines = append(lines,
			"        proxy_set_header Upgrade $http_upgrade;",
			"        proxy_set_header Connection \"upgrade\";",
			"        proxy_read_timeout "+firstString(spec["websocket_read_timeout"], spec["proxy_read_timeout"], "3600s")+";",
			"        proxy_send_timeout "+firstString(spec["websocket_send_timeout"], spec["proxy_send_timeout"], "3600s")+";",
			"        proxy_buffering off;",
		)
	}
	lines = append(lines, extraIndentedLines(spec[extraLinesKey], "        ")...)
	lines = append(lines, "    }")
	return lines
}

func appendNginxFallbackLocation(lines []string, primaryLocation string, spec map[string]any) []string {
	fallbackURL := firstString(spec["fallback_upstream_url"], spec["fallback_proxy_pass"])
	if fallbackURL == "" || primaryLocation == "/" {
		return lines
	}
	fallbackHost := firstString(spec["fallback_host_header"], spec["fallback_host"])
	fallbackSNI := firstString(spec["fallback_sni"], fallbackHost)
	lines = append(lines, "    location / {")
	lines = append(lines, "        proxy_pass "+fallbackURL+";")
	lines = append(lines, "        proxy_http_version "+firstString(spec["fallback_proxy_http_version"], "1.1")+";")
	if fallbackHost != "" {
		lines = append(lines, "        proxy_set_header Host "+fallbackHost+";")
	} else {
		lines = append(lines, "        proxy_set_header Host $host;")
	}
	lines = append(lines,
		"        proxy_set_header X-Real-IP $remote_addr;",
		"        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;",
		"        proxy_set_header X-Forwarded-Proto $scheme;",
	)
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(fallbackURL)), "https://") {
		lines = append(lines, "        proxy_ssl_server_name on;")
		if fallbackSNI != "" {
			lines = append(lines, "        proxy_ssl_name "+fallbackSNI+";")
		}
	}
	lines = append(lines, extraIndentedLines(spec["fallback_location_extra_lines"], "        ")...)
	lines = append(lines, "    }")
	return lines
}

func validateNginxLocationPath(path string) error {
	path = strings.TrimSpace(path)
	if path == "" || !strings.HasPrefix(path, "/") {
		return fmt.Errorf("nginx location_path must start with /")
	}
	if strings.ContainsAny(path, "{};\"'`\\#?\r\n\t ") {
		return fmt.Errorf("nginx location_path contains unsafe characters")
	}
	return nil
}

func validateNginxServerName(serverName string) error {
	serverName = strings.TrimSpace(serverName)
	if serverName == "" {
		return fmt.Errorf("nginx server_name is required")
	}
	if strings.ContainsAny(serverName, " \t\r\n;{}") {
		return fmt.Errorf("nginx server_name contains unsafe directive characters")
	}
	if serverName == "_" {
		return nil
	}
	ipLiteral := serverName
	if strings.HasPrefix(serverName, "[") || strings.HasSuffix(serverName, "]") {
		if !strings.HasPrefix(serverName, "[") || !strings.HasSuffix(serverName, "]") {
			return fmt.Errorf("nginx server_name must be a DNS name, wildcard DNS name, IP literal, or _")
		}
		ipLiteral = strings.TrimPrefix(strings.TrimSuffix(serverName, "]"), "[")
	}
	if _, err := netip.ParseAddr(ipLiteral); err == nil {
		return nil
	}
	if !managedNginxServerNamePattern.MatchString(serverName) {
		return fmt.Errorf("nginx server_name must be a DNS name, wildcard DNS name, IP literal, or _")
	}
	return nil
}

func validateNginxFallbackSpec(spec map[string]any, instance domain.Instance, serverName string) error {
	fallbackURL := firstString(spec["fallback_upstream_url"], spec["fallback_proxy_pass"])
	if err := validateNginxFallbackURL(fallbackURL); err != nil {
		return err
	}
	for _, item := range []struct {
		name  string
		value string
	}{
		{name: "fallback_host_header", value: firstString(spec["fallback_host_header"], spec["fallback_host"])},
		{name: "fallback_sni", value: firstString(spec["fallback_sni"])},
	} {
		if err := validateNginxDirectiveValue(item.name, item.value); err != nil {
			return err
		}
	}
	if nginxFallbackLoopGuardEnabled(spec) {
		if err := validateNginxFallbackLoopGuard(spec, fallbackURL, instance, serverName); err != nil {
			return err
		}
	}
	return nil
}

func nginxFallbackLoopGuardEnabled(spec map[string]any) bool {
	profile := strings.ToLower(strings.TrimSpace(firstString(spec["service_profile"], spec["edge_profile"], spec["profile"])))
	switch profile {
	case "ws_camouflage_edge", "grpc_edge":
		return true
	}
	return truthy(spec["traffic_camouflage"]) || truthy(spec["fallback_loop_guard"])
}

func validateNginxFallbackLoopGuard(spec map[string]any, fallbackURL string, instance domain.Instance, serverName string) error {
	edgeHosts := []string{serverName, instance.EndpointHost}
	if fallbackURL != "" {
		if parsed, err := url.Parse(strings.TrimSpace(fallbackURL)); err == nil && parsed != nil {
			if nginxHostMatchesEdge(parsed.Hostname(), edgeHosts...) {
				return fmt.Errorf("nginx fallback_upstream_url must not target the public edge host; choose a separate fallback website")
			}
		}
	}
	for _, item := range []struct {
		name  string
		value string
	}{
		{name: "fallback_host_header", value: firstString(spec["fallback_host_header"], spec["fallback_host"])},
		{name: "fallback_sni", value: firstString(spec["fallback_sni"])},
	} {
		if nginxHostMatchesEdge(item.value, edgeHosts...) {
			return fmt.Errorf("nginx %s must not target the public edge host", item.name)
		}
	}
	return nil
}

func nginxHostMatchesEdge(candidate string, edgeHosts ...string) bool {
	normalizedCandidate := normalizeNginxHost(candidate)
	if normalizedCandidate == "" {
		return false
	}
	for _, edgeHost := range edgeHosts {
		normalizedEdge := normalizeNginxHost(edgeHost)
		if normalizedEdge == "" || normalizedEdge == "_" {
			continue
		}
		if strings.HasPrefix(normalizedEdge, "*.") {
			suffix := strings.TrimPrefix(normalizedEdge, "*.")
			if normalizedCandidate == suffix || strings.HasSuffix(normalizedCandidate, "."+suffix) {
				return true
			}
			continue
		}
		if normalizedCandidate == normalizedEdge {
			return true
		}
	}
	return false
}

func normalizeNginxHost(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.Contains(value, "://") {
		if parsed, err := url.Parse(value); err == nil && parsed != nil {
			value = parsed.Hostname()
		}
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	}
	value = strings.TrimPrefix(strings.TrimSuffix(value, "]"), "[")
	value = strings.TrimSuffix(value, ".")
	return strings.ToLower(value)
}

func validateNginxFallbackURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if err := validateNginxDirectiveValue("fallback_upstream_url", raw); err != nil {
		return err
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed == nil {
		return fmt.Errorf("nginx fallback_upstream_url is invalid")
	}
	if parsed.User != nil {
		return fmt.Errorf("nginx fallback_upstream_url must not contain credentials")
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("nginx fallback_upstream_url must be an absolute http or https URL")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
	default:
		return fmt.Errorf("nginx fallback_upstream_url scheme must be http or https")
	}
	if parsed.Fragment != "" {
		return fmt.Errorf("nginx fallback_upstream_url must not contain a fragment")
	}
	return nil
}

func validateNginxDirectiveValue(name, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if strings.ContainsAny(value, "{};\"'`\\#\r\n\t ") {
		return fmt.Errorf("nginx %s contains unsafe characters", name)
	}
	return nil
}

func buildShadowsocksServerConfig(instance domain.Instance, spec map[string]any) (map[string]any, error) {
	if spec["config_json"] == nil {
		if rawText := firstString(spec["config_content"]); rawText != "" {
			var parsed map[string]any
			if err := json.Unmarshal([]byte(rawText), &parsed); err == nil && parsed != nil {
				spec["config_json"] = parsed
			}
		}
	}

	var cfg map[string]any
	if raw := spec["config_json"]; raw != nil {
		parsed, ok := cloneAny(raw).(map[string]any)
		if !ok {
			return nil, fmt.Errorf("shadowsocks config_json must be an object")
		}
		cfg = parsed
	} else {
		cfg = map[string]any{}
	}

	listen := firstString(spec["listen"], spec["server"], "0.0.0.0")
	method := firstString(spec["method"], "chacha20-ietf-poly1305")
	mode := firstString(spec["mode"], "tcp_and_udp")
	timeout := firstIntValue(spec["timeout"], 300)
	managedAccounts := shadowsocksManagedAccounts(spec["managed_accounts"])

	cfg["server"] = listen
	cfg["method"] = method
	cfg["mode"] = mode
	cfg["timeout"] = timeout
	if spec["fast_open"] != nil {
		cfg["fast_open"] = truthy(spec["fast_open"])
	}
	if spec["reuse_port"] != nil {
		cfg["reuse_port"] = truthy(spec["reuse_port"])
	}
	if spec["no_delay"] != nil {
		cfg["no_delay"] = truthy(spec["no_delay"])
	}

	if len(managedAccounts) > 0 {
		portPassword := map[string]string{}
		for _, account := range managedAccounts {
			port := firstIntValue(account["server_port"], account["port"])
			password := firstString(account["password"], account["shadowsocks_password"])
			if port <= 0 || password == "" {
				continue
			}
			portPassword[strconv.Itoa(port)] = password
		}
		if len(portPassword) == 0 {
			return nil, fmt.Errorf("shadowsocks managed_accounts are empty")
		}
		cfg["port_password"] = portPassword
		delete(cfg, "server_port")
		delete(cfg, "password")
		return cfg, nil
	}

	serverPort := firstIntValue(spec["server_port"], spec["port"], instance.EndpointPort)
	if serverPort <= 0 {
		return nil, fmt.Errorf("shadowsocks server_port is required")
	}
	password := firstString(spec["password"], spec["server_password"])
	if password == "" {
		return nil, fmt.Errorf("shadowsocks password is required")
	}
	cfg["server_port"] = serverPort
	cfg["password"] = password
	delete(cfg, "port_password")
	return cfg, nil
}

func attachDefaultNetworkPolicy(instance domain.Instance, spec map[string]any) map[string]any {
	spec = cloneMap(spec)
	if explicitlyFalse(spec["network_policy_enabled"]) {
		return spec
	}

	serviceCode := normalizeInstanceRuntimeCode(instance.ServiceCode)
	if requiresIPForwarding(serviceCode) {
		sysctl := mapFromAny(spec["sysctl"])
		if _, exists := sysctl["net.ipv4.ip_forward"]; !exists {
			sysctl["net.ipv4.ip_forward"] = "1"
		}
		spec["sysctl"] = sysctl
	}

	rules := sliceFromAny(spec["firewall_rules"])
	for _, rule := range defaultFirewallRulesForInstance(instance, spec) {
		rules = appendFirewallRuleIfMissing(rules, rule)
	}
	if len(rules) > 0 {
		spec["firewall_rules"] = rules
	}
	natRules := sliceFromAny(spec["nat_rules"])
	for _, rule := range defaultNATRulesForInstance(instance, spec) {
		natRules = appendNATRuleIfMissing(natRules, rule)
	}
	if len(natRules) > 0 {
		spec["nat_rules"] = natRules
	}
	return spec
}

func requiresIPForwarding(serviceCode string) bool {
	return driver.RequiresIPForwarding(serviceCode)
}

func defaultFirewallRulesForInstance(instance domain.Instance, spec map[string]any) []map[string]any {
	serviceCode := normalizeInstanceRuntimeCode(instance.ServiceCode)
	endpointPort := firstIntValue(spec["listen_port"], spec["server_port"], spec["port"], instance.EndpointPort)
	rules := []map[string]any{}
	add := func(protocol string, port int) {
		if port <= 0 || strings.TrimSpace(protocol) == "" {
			return
		}
		rules = append(rules, map[string]any{
			"direction": "input",
			"action":    "allow",
			"protocol":  protocol,
			"port":      port,
			"comment":   "default " + serviceCode + " ingress",
		})
	}

	switch serviceCode {
	case "xray-core", "nginx", "http_proxy", "mtproto":
		add("tcp", endpointPort)
	case "wireguard":
		if endpointPort <= 0 {
			endpointPort = 51820
		}
		add("udp", endpointPort)
	case "openvpn":
		if endpointPort <= 0 {
			endpointPort = 1194
		}
		proto := strings.ToLower(firstString(spec["proto"], "tcp"))
		if proto != "udp" {
			proto = "tcp"
		}
		add(proto, endpointPort)
	case "ipsec":
		add("udp", 500)
		add("udp", 4500)
		if endpointPort > 0 && endpointPort != 500 && endpointPort != 4500 {
			add("udp", endpointPort)
		}
	case "xl2tpd":
		if endpointPort <= 0 {
			endpointPort = 1701
		}
		add("udp", endpointPort)
	case "shadowsocks":
		if endpointPort <= 0 {
			endpointPort = firstIntValue(spec["server_port"], spec["port"])
		}
		mode := strings.ToLower(firstString(spec["mode"], "tcp_and_udp"))
		if mode == "udp_only" || mode == "udp" {
			add("udp", endpointPort)
		} else if mode == "tcp_only" || mode == "tcp" {
			add("tcp", endpointPort)
		} else {
			add("tcp", endpointPort)
			add("udp", endpointPort)
		}
	}
	return rules
}

func defaultNATRulesForInstance(instance domain.Instance, spec map[string]any) []map[string]any {
	serviceCode := normalizeInstanceRuntimeCode(instance.ServiceCode)
	if serviceCode != "openvpn" || explicitlyFalse(spec["nat_enabled"]) || !openVPNFullTunnelPushEnabled(spec) {
		return nil
	}
	sourceCIDR := openVPNClientPoolCIDR(spec)
	if sourceCIDR == "" {
		return nil
	}
	return []map[string]any{{
		"type":    "masquerade",
		"family":  "ip",
		"source":  sourceCIDR,
		"comment": "default openvpn client egress masquerade",
	}}
}

func openVPNFullTunnelPushEnabled(spec map[string]any) bool {
	for _, line := range extraServerLines(spec["server_extra_lines"]) {
		if strings.Contains(strings.ToLower(line), "redirect-gateway") {
			return true
		}
	}
	return false
}

func openVPNClientPoolCIDR(spec map[string]any) string {
	cidr := strings.TrimSpace(firstString(spec["address_pool_cidr"]))
	if prefix, err := netip.ParsePrefix(cidr); err == nil && prefix.Addr().Is4() {
		return prefix.Masked().String()
	}
	cidr, err := cidrFromIPv4NetworkAndNetmask(
		firstString(spec["server_network"], "10.8.0.0"),
		firstString(spec["server_netmask"], "255.255.255.0"),
	)
	if err != nil {
		return ""
	}
	prefix, err := netip.ParsePrefix(cidr)
	if err != nil || !prefix.Addr().Is4() {
		return ""
	}
	return prefix.Masked().String()
}

func appendFirewallRuleIfMissing(rules []any, rule map[string]any) []any {
	for _, item := range rules {
		existing, _ := item.(map[string]any)
		if existing == nil {
			continue
		}
		if firewallRuleKey(existing) == firewallRuleKey(rule) {
			return rules
		}
	}
	return append(rules, rule)
}

func appendNATRuleIfMissing(rules []any, rule map[string]any) []any {
	for _, item := range rules {
		existing, _ := item.(map[string]any)
		if existing == nil {
			continue
		}
		if natRuleKey(existing) == natRuleKey(rule) {
			return rules
		}
	}
	return append(rules, rule)
}

func firewallRuleKey(rule map[string]any) string {
	return strings.Join([]string{
		strings.ToLower(firstString(rule["direction"], "input")),
		strings.ToLower(firstString(rule["action"], "allow")),
		strings.ToLower(firstString(rule["protocol"])),
		strconv.Itoa(firstIntValue(rule["port"], rule["dport"], rule["listen_port"])),
		firstString(rule["source"], rule["src"]),
		firstString(rule["interface"], rule["iifname"], rule["dev"]),
	}, "|")
}

func natRuleKey(rule map[string]any) string {
	return strings.Join([]string{
		strings.ToLower(firstString(rule["type"], "masquerade")),
		strings.ToLower(firstString(rule["family"], "ip")),
		firstString(rule["source"], rule["src"], rule["cidr"]),
		firstString(rule["out_interface"], rule["oifname"], rule["interface"]),
	}, "|")
}

func (s *Store) resolveSecretText(ctx context.Context, refRaw, inlineRaw any) (string, error) {
	if refID := strings.TrimSpace(stringify(refRaw)); refID != "" {
		_, value, err := s.ResolveSecretValue(ctx, refID)
		if err != nil {
			return "", err
		}
		return string(value), nil
	}
	return firstString(inlineRaw), nil
}

type xrayVLESSGroup struct {
	Key         string
	Label       string
	EgressMode  string
	OutboundTag string
	TargetID    string
	Outbound    map[string]any
	AdBlock     bool
	Rules       []map[string]any
}

func xrayManagedClientSpecs(raw any) []map[string]any {
	list, _ := raw.([]any)
	out := make([]map[string]any, 0, len(list))
	for _, item := range list {
		client, _ := cloneAny(item).(map[string]any)
		if client == nil {
			continue
		}
		if strings.TrimSpace(stringify(client["id"])) == "" {
			continue
		}
		out = append(out, client)
	}
	return out
}

func xrayManagedClients(raw any) []any {
	specs := xrayManagedClientSpecs(raw)
	out := make([]any, 0, len(specs))
	for _, client := range specs {
		rendered := map[string]any{
			"id": strings.TrimSpace(stringify(client["id"])),
		}
		if email := firstString(client["email"]); email != "" {
			rendered["email"] = email
		}
		if flow := firstString(client["flow"]); flow != "" {
			rendered["flow"] = flow
		}
		if level := firstIntValue(client["level"]); level >= 0 {
			if _, ok := client["level"]; ok {
				rendered["level"] = level
			}
		}
		out = append(out, rendered)
	}
	return out
}

func xrayVLESSGroups(spec map[string]any) []xrayVLESSGroup {
	if spec == nil {
		return nil
	}
	raw := spec["vless_groups"]
	if raw == nil {
		raw = spec["xray_groups"]
	}
	if raw == nil {
		raw = spec["outbound_groups"]
	}
	list, _ := raw.([]any)
	groups := make([]xrayVLESSGroup, 0, len(list))
	seen := make(map[string]struct{}, len(list))
	for _, item := range list {
		groupRaw, _ := cloneAny(item).(map[string]any)
		if groupRaw == nil {
			continue
		}
		key := normalizeXrayVLESSGroupKey(firstString(groupRaw["key"], groupRaw["name"], groupRaw["id"]))
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		outbound, _ := cloneAny(groupRaw["outbound"]).(map[string]any)
		if outbound != nil && firstString(outbound["protocol"]) == "" {
			outbound = nil
		}
		outboundTag := normalizeXrayOutboundTag(firstString(groupRaw["outbound_tag"], groupRaw["outboundTag"], groupRaw["tag"]))
		if outbound != nil {
			if tag := normalizeXrayOutboundTag(firstString(outbound["tag"])); tag != "" {
				outboundTag = tag
			} else if outboundTag != "" {
				outbound["tag"] = outboundTag
			}
		}
		if outboundTag == "" {
			outboundTag = "direct"
		}
		if outbound == nil && !isBuiltInXrayOutboundTag(outboundTag) {
			outboundTag = "direct"
		}
		rules := xrayVLESSGroupRules(groupRaw["rules"])
		if extraRules := xrayVLESSGroupRules(groupRaw["extra_rules"]); len(extraRules) > 0 {
			rules = append(rules, extraRules...)
		}
		group := xrayVLESSGroup{
			Key:         key,
			Label:       firstString(groupRaw["label"], groupRaw["title"], key),
			EgressMode:  strings.ToLower(firstString(groupRaw["egress_mode"], groupRaw["access_mode"], groupRaw["mode"])),
			OutboundTag: outboundTag,
			TargetID:    firstString(groupRaw["target_instance_id"], groupRaw["targetInstanceID"]),
			Outbound:    outbound,
			AdBlock:     xrayVLESSGroupAdBlock(groupRaw),
			Rules:       rules,
		}
		groups = append(groups, group)
		seen[key] = struct{}{}
	}
	return groups
}

func xrayVLESSGroupRules(raw any) []map[string]any {
	list := sliceRuleItemsFromAny(raw)
	out := make([]map[string]any, 0, len(list))
	for _, item := range list {
		rule, _ := cloneAny(item).(map[string]any)
		if rule == nil {
			continue
		}
		if xrayVLESSRuleIsManagedAdBlock(rule) {
			continue
		}
		out = append(out, rule)
	}
	return out
}

func xrayVLESSGroupAdBlock(group map[string]any) bool {
	if group == nil {
		return false
	}
	if truthy(group["ad_block"]) || truthy(group["adBlock"]) || truthy(group["block_ads"]) || truthy(group["blockAds"]) {
		return true
	}
	list := sliceRuleItemsFromAny(group["rules"])
	for _, item := range list {
		rule, _ := cloneAny(item).(map[string]any)
		if xrayVLESSRuleIsManagedAdBlock(rule) {
			return true
		}
	}
	return false
}

func sliceRuleItemsFromAny(raw any) []any {
	if raw == nil {
		return nil
	}
	switch value := raw.(type) {
	case []any:
		return value
	case []map[string]any:
		out := make([]any, 0, len(value))
		for _, item := range value {
			out = append(out, item)
		}
		return out
	default:
		return nil
	}
}

func (s *Store) hydrateXrayVLESSGroupTargetRules(ctx context.Context, spec map[string]any) error {
	if spec == nil {
		return nil
	}
	groups, ok := spec["vless_groups"].([]any)
	if !ok || len(groups) == 0 {
		return nil
	}
	updated := make([]any, 0, len(groups))
	for _, item := range groups {
		group, _ := cloneAny(item).(map[string]any)
		if group == nil {
			updated = append(updated, item)
			continue
		}
		targetID := firstString(group["target_instance_id"], group["targetInstanceID"])
		mode := strings.ToLower(firstString(group["egress_mode"], group["access_mode"], group["mode"]))
		if targetID == "" || mode != "instance_only" {
			updated = append(updated, group)
			continue
		}
		target, err := s.GetInstance(ctx, targetID)
		if err != nil {
			return fmt.Errorf("vless group %q target instance lookup failed: %w", firstString(group["key"], group["name"], "unknown"), err)
		}
		rule, err := xrayVLESSTargetInstanceRule(target, firstString(group["allow_outbound_tag"], "direct"))
		if err != nil {
			return fmt.Errorf("vless group %q target instance is not routable: %w", firstString(group["key"], group["name"], "unknown"), err)
		}
		rules := sliceMapRulesFromAny(group["rules"])
		rules = append(rules, rule)
		group["rules"] = rules
		group["outbound_tag"] = "block"
		updated = append(updated, group)
	}
	spec["vless_groups"] = updated
	return nil
}

func xrayVLESSTargetInstanceRule(target domain.Instance, outboundTag string) (map[string]any, error) {
	host := strings.TrimSpace(target.EndpointHost)
	port := target.EndpointPort
	if host == "" {
		return nil, fmt.Errorf("target instance %s has empty endpoint host", target.ID)
	}
	if port <= 0 || port > 65535 {
		return nil, fmt.Errorf("target instance %s has invalid endpoint port", target.ID)
	}
	tag := normalizeXrayOutboundTag(outboundTag)
	if tag == "" {
		tag = "direct"
	}
	rule := map[string]any{
		"type":        "field",
		"port":        strconv.Itoa(port),
		"outboundTag": tag,
	}
	if _, err := netip.ParseAddr(host); err == nil {
		rule["ip"] = []any{host}
	} else {
		rule["domain"] = []any{host}
	}
	return rule, nil
}

func sliceMapRulesFromAny(raw any) []any {
	if raw == nil {
		return []any{}
	}
	switch value := raw.(type) {
	case []any:
		return sliceFromAny(value)
	case []map[string]any:
		out := make([]any, 0, len(value))
		for _, item := range value {
			out = append(out, cloneMap(item))
		}
		return out
	default:
		return []any{}
	}
}

func xrayVLESSRuleIsManagedAdBlock(rule map[string]any) bool {
	if rule == nil {
		return false
	}
	tag := normalizeXrayOutboundTag(firstString(rule["outboundTag"], rule["outbound_tag"]))
	if tag != "block" {
		return false
	}
	domains, _ := rule["domain"].([]any)
	for _, raw := range domains {
		if strings.EqualFold(strings.TrimSpace(stringify(raw)), "geosite:category-ads-all") {
			return true
		}
	}
	return false
}

func xrayDefaultVLESSGroupKey(spec map[string]any, groups []xrayVLESSGroup) string {
	requested := normalizeXrayVLESSGroupKey(firstString(spec["default_vless_group"], spec["default_xray_group"], spec["default_outbound_group"]))
	if requested != "" {
		for _, group := range groups {
			if group.Key == requested {
				return requested
			}
		}
	}
	if len(groups) > 0 {
		return groups[0].Key
	}
	if requested != "" {
		return requested
	}
	return "default"
}

func xrayVLESSGroupKeySet(spec map[string]any) map[string]struct{} {
	groups := xrayVLESSGroups(spec)
	set := make(map[string]struct{}, len(groups)+1)
	for _, group := range groups {
		set[group.Key] = struct{}{}
	}
	if len(set) == 0 {
		set[xrayDefaultVLESSGroupKey(spec, groups)] = struct{}{}
	}
	return set
}

func normalizeXrayVLESSGroupKey(raw string) string {
	text := strings.TrimSpace(raw)
	if text == "" {
		return ""
	}
	text = strings.ReplaceAll(text, " ", "_")
	var b strings.Builder
	for _, r := range text {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '_' || r == '-' || r == '.' || r == ':':
			b.WriteRune(r)
		}
		if b.Len() >= 64 {
			break
		}
	}
	return b.String()
}

func normalizeXrayOutboundTag(raw string) string {
	text := strings.TrimSpace(raw)
	if text == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range text {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '_' || r == '-' || r == '.' || r == ':':
			b.WriteRune(r)
		}
		if b.Len() >= 64 {
			break
		}
	}
	return b.String()
}

func isBuiltInXrayOutboundTag(tag string) bool {
	return tag == "direct" || tag == "block"
}

func xrayInstanceDefaultOutbound(spec map[string]any) (map[string]any, string) {
	if spec == nil {
		return nil, ""
	}
	rawOutbound, _ := cloneAny(spec["xray_default_outbound"]).(map[string]any)
	if rawOutbound == nil {
		rawOutbound, _ = cloneAny(spec["default_xray_outbound"]).(map[string]any)
	}
	if rawOutbound == nil {
		egress, _ := spec["xray_egress"].(map[string]any)
		mode := strings.ToLower(firstString(egress["mode"]))
		sendThrough := firstString(egress["send_through"], egress["sendThrough"])
		if mode != "" && mode != "local_breakout" && sendThrough != "" {
			rawOutbound = map[string]any{
				"tag":         "egress-default",
				"protocol":    "freedom",
				"sendThrough": sendThrough,
				"settings": map[string]any{
					"domainStrategy": "UseIP",
				},
			}
		}
	}
	if rawOutbound == nil {
		return nil, ""
	}
	tag := normalizeXrayOutboundTag(firstString(rawOutbound["tag"], "egress-default"))
	if tag == "" {
		return nil, ""
	}
	rawOutbound["tag"] = tag
	if firstString(rawOutbound["protocol"]) == "" {
		rawOutbound["protocol"] = "freedom"
	}
	if sendThrough := firstString(rawOutbound["send_through"]); sendThrough != "" && firstString(rawOutbound["sendThrough"]) == "" {
		rawOutbound["sendThrough"] = sendThrough
		delete(rawOutbound, "send_through")
	}
	return rawOutbound, tag
}

func xrayInstanceDefaultOutboundTag(spec map[string]any) string {
	_, tag := xrayInstanceDefaultOutbound(spec)
	return tag
}

func xrayOutboundsForSpec(raw any, spec map[string]any) []any {
	outbounds := xrayOutboundsWithGroups(raw, xrayVLESSGroups(spec))
	outbound, tag := xrayInstanceDefaultOutbound(spec)
	if outbound == nil || tag == "" {
		return outbounds
	}
	for _, item := range outbounds {
		existing, _ := item.(map[string]any)
		if normalizeXrayOutboundTag(firstString(existing["tag"])) == tag {
			return outbounds
		}
	}
	return append(outbounds, outbound)
}

func xrayOutboundsWithGroups(raw any, groups []xrayVLESSGroup) []any {
	outbounds, _ := cloneAny(raw).([]any)
	if len(outbounds) == 0 {
		outbounds = []any{
			map[string]any{"protocol": "freedom", "tag": "direct"},
			map[string]any{"protocol": "blackhole", "tag": "block"},
		}
	}
	seen := make(map[string]struct{}, len(outbounds)+len(groups))
	for _, item := range outbounds {
		outbound, _ := item.(map[string]any)
		if outbound == nil {
			continue
		}
		if tag := normalizeXrayOutboundTag(firstString(outbound["tag"])); tag != "" {
			seen[tag] = struct{}{}
		}
	}
	for _, group := range groups {
		if group.Outbound == nil {
			continue
		}
		outbound, _ := cloneAny(group.Outbound).(map[string]any)
		if outbound == nil {
			continue
		}
		tag := normalizeXrayOutboundTag(firstString(outbound["tag"], group.OutboundTag))
		if tag == "" {
			continue
		}
		outbound["tag"] = tag
		if _, ok := seen[tag]; ok {
			continue
		}
		outbounds = append(outbounds, outbound)
		seen[tag] = struct{}{}
	}
	return outbounds
}

func applyXrayVLESSGroupRouting(cfg map[string]any, spec map[string]any, clients []map[string]any) {
	groups := xrayVLESSGroups(spec)
	cfg["outbounds"] = xrayOutboundsForSpec(cfg["outbounds"], spec)
	rules := xrayVLESSGroupRoutingRules(spec, groups, clients)
	if tag := xrayInstanceDefaultOutboundTag(spec); tag != "" {
		rules = append(rules, map[string]any{
			"type":        "field",
			"inboundTag":  []any{firstString(spec["managed_inbound_tag"], "vless-in")},
			"outboundTag": tag,
		})
	}
	if len(rules) == 0 {
		return
	}
	routing, _ := cfg["routing"].(map[string]any)
	if routing == nil {
		routing = map[string]any{}
	}
	if firstString(routing["domainStrategy"]) == "" {
		routing["domainStrategy"] = firstString(spec["routing_domain_strategy"], "AsIs")
	}
	existingRules, _ := routing["rules"].([]any)
	routing["rules"] = append(existingRules, rules...)
	cfg["routing"] = routing
}

func applyXrayStatsAPI(cfg map[string]any, instance domain.Instance, spec map[string]any) {
	if !xrayStatsAPIEnabled(spec) {
		return
	}
	apiTag := normalizeXrayOutboundTag(firstString(spec["xray_stats_api_tag"], "api"))
	if apiTag == "" {
		apiTag = "api"
	}
	inboundTag := normalizeXrayOutboundTag(firstString(spec["xray_stats_inbound_tag"], apiTag))
	if inboundTag == "" {
		inboundTag = apiTag
	}
	port := xrayStatsAPIPort(instance, spec)
	if port <= 0 || port > 65535 {
		return
	}

	if _, ok := cfg["stats"].(map[string]any); !ok {
		cfg["stats"] = map[string]any{}
	}
	applyXrayStatsPolicy(cfg)
	applyXrayStatsAPIObject(cfg, apiTag)
	applyXrayStatsInbound(cfg, inboundTag, port)
	applyXrayStatsRoutingRule(cfg, inboundTag, apiTag, spec)
}

func xrayStatsAPIEnabled(spec map[string]any) bool {
	if spec == nil {
		return false
	}
	if explicitlyFalse(spec["traffic_accounting_enabled"]) ||
		explicitlyFalse(spec["xray_stats_enabled"]) ||
		explicitlyFalse(spec["stats_enabled"]) {
		return false
	}
	return truthy(spec["traffic_accounting_enabled"]) ||
		truthy(spec["xray_stats_enabled"]) ||
		truthy(spec["stats_enabled"])
}

func xrayStatsAPIPort(instance domain.Instance, spec map[string]any) int {
	if port := firstIntValue(spec["xray_stats_api_port"], spec["stats_api_port"]); port > 0 && port <= 65535 {
		return port
	}
	basePort := firstIntValue(spec["server_port"], spec["port"], instance.EndpointPort)
	if basePort > 0 && basePort <= 55535 {
		return basePort + 10000
	}
	seed := firstString(spec["slug"], instance.Slug, instance.Name, instance.ID, "xray")
	sum := sha1.Sum([]byte(seed))
	return 18000 + ((int(sum[0])*256 + int(sum[1])) % 20000)
}

func applyXrayStatsPolicy(cfg map[string]any) {
	policy, _ := cfg["policy"].(map[string]any)
	if policy == nil {
		policy = map[string]any{}
	}
	levels, _ := policy["levels"].(map[string]any)
	if levels == nil {
		levels = map[string]any{}
	}
	levelZero, _ := levels["0"].(map[string]any)
	if levelZero == nil {
		levelZero = map[string]any{}
	}
	levelZero["statsUserUplink"] = true
	levelZero["statsUserDownlink"] = true
	levels["0"] = levelZero
	policy["levels"] = levels
	cfg["policy"] = policy
}

func applyXrayStatsAPIObject(cfg map[string]any, apiTag string) {
	api, _ := cfg["api"].(map[string]any)
	if api == nil {
		api = map[string]any{}
	}
	api["tag"] = apiTag
	services := anyStringList(uniqueSortedStrings(append(stringList(api["services"]), "StatsService")))
	if len(services) == 0 {
		services = []any{"StatsService"}
	}
	api["services"] = services
	cfg["api"] = api
}

func applyXrayStatsInbound(cfg map[string]any, inboundTag string, port int) {
	inbounds, _ := cfg["inbounds"].([]any)
	for _, item := range inbounds {
		inbound, _ := item.(map[string]any)
		if normalizeXrayOutboundTag(firstString(inbound["tag"])) == inboundTag {
			inbound["listen"] = "127.0.0.1"
			inbound["port"] = port
			inbound["protocol"] = "dokodemo-door"
			settings, _ := inbound["settings"].(map[string]any)
			if settings == nil {
				settings = map[string]any{}
			}
			settings["address"] = "127.0.0.1"
			inbound["settings"] = settings
			return
		}
	}
	inbounds = append(inbounds, map[string]any{
		"tag":      inboundTag,
		"listen":   "127.0.0.1",
		"port":     port,
		"protocol": "dokodemo-door",
		"settings": map[string]any{
			"address": "127.0.0.1",
		},
	})
	cfg["inbounds"] = inbounds
}

func applyXrayStatsRoutingRule(cfg map[string]any, inboundTag, apiTag string, spec map[string]any) {
	routing, _ := cfg["routing"].(map[string]any)
	if routing == nil {
		routing = map[string]any{}
	}
	if firstString(routing["domainStrategy"]) == "" {
		routing["domainStrategy"] = firstString(spec["routing_domain_strategy"], "AsIs")
	}
	existingRules, _ := routing["rules"].([]any)
	for _, item := range existingRules {
		rule, _ := item.(map[string]any)
		if rule == nil {
			continue
		}
		if firstString(rule["outboundTag"]) != apiTag {
			continue
		}
		for _, tag := range stringList(rule["inboundTag"]) {
			if normalizeXrayOutboundTag(tag) == inboundTag {
				return
			}
		}
	}
	apiRule := map[string]any{
		"type":        "field",
		"inboundTag":  []any{inboundTag},
		"outboundTag": apiTag,
	}
	routing["rules"] = append([]any{apiRule}, existingRules...)
	cfg["routing"] = routing
}

func xrayVLESSGroupRoutingRules(spec map[string]any, groups []xrayVLESSGroup, clients []map[string]any) []any {
	if len(groups) == 0 || len(clients) == 0 {
		return nil
	}
	inboundTag := firstString(spec["managed_inbound_tag"], "vless-in")
	known := make(map[string]xrayVLESSGroup, len(groups))
	for _, group := range groups {
		known[group.Key] = group
	}
	defaultGroup := xrayDefaultVLESSGroupKey(spec, groups)
	usersByGroup := make(map[string][]string, len(groups))
	for _, client := range clients {
		user := firstString(client["email"])
		if user == "" {
			continue
		}
		groupKey := normalizeXrayVLESSGroupKey(firstString(client["vless_group"], client["xray_group"], client["outbound_group"], client["group"]))
		if groupKey == "" {
			groupKey = defaultGroup
		}
		if _, ok := known[groupKey]; !ok {
			groupKey = defaultGroup
		}
		usersByGroup[groupKey] = append(usersByGroup[groupKey], user)
	}
	rules := make([]any, 0)
	for _, group := range groups {
		users := uniqueSortedStrings(usersByGroup[group.Key])
		if len(users) == 0 {
			continue
		}
		if group.AdBlock {
			rules = append(rules, map[string]any{
				"type":        "field",
				"inboundTag":  []any{inboundTag},
				"user":        anyStringList(users),
				"domain":      []any{"geosite:category-ads-all"},
				"outboundTag": "block",
			})
		}
		for _, rawRule := range group.Rules {
			rule, _ := cloneAny(rawRule).(map[string]any)
			if rule == nil {
				continue
			}
			if firstString(rule["type"]) == "" {
				rule["type"] = "field"
			}
			delete(rule, "outbound_tag")
			delete(rule, "group")
			delete(rule, "key")
			rule["user"] = anyStringList(users)
			if firstString(rule["inboundTag"]) == "" && inboundTag != "" {
				rule["inboundTag"] = []any{inboundTag}
			}
			rawRuleTag := normalizeXrayOutboundTag(firstString(rawRule["outboundTag"], rawRule["outbound_tag"]))
			tag := rawRuleTag
			if tag == "" {
				tag = xrayEffectiveVLESSGroupOutboundTag(spec, group)
			}
			rule["outboundTag"] = tag
			rules = append(rules, rule)
		}
		rules = append(rules, map[string]any{
			"type":        "field",
			"inboundTag":  []any{inboundTag},
			"user":        anyStringList(users),
			"outboundTag": xrayEffectiveVLESSGroupOutboundTag(spec, group),
		})
	}
	return rules
}

func xrayEffectiveVLESSGroupOutboundTag(spec map[string]any, group xrayVLESSGroup) string {
	tag := normalizeXrayOutboundTag(group.OutboundTag)
	if tag == "" {
		tag = "direct"
	}
	switch strings.ToLower(group.EgressMode) {
	case "local", "direct", "local_breakout":
		return "direct"
	case "block", "blocked", "deny":
		return "block"
	}
	if tag == "direct" && group.Outbound == nil {
		if defaultTag := xrayInstanceDefaultOutboundTag(spec); defaultTag != "" {
			return defaultTag
		}
	}
	return tag
}

func uniqueSortedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func anyStringList(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

func shadowsocksManagedAccounts(raw any) []map[string]any {
	list, _ := raw.([]any)
	out := make([]map[string]any, 0, len(list))
	for _, item := range list {
		account, _ := cloneAny(item).(map[string]any)
		if account == nil {
			continue
		}
		if firstIntValue(account["server_port"], account["port"]) <= 0 {
			continue
		}
		if firstString(account["password"], account["shadowsocks_password"]) == "" {
			continue
		}
		out = append(out, account)
	}
	return out
}

type httpProxyManagedAccount struct {
	Username     string
	PasswordHash string
}

func httpProxyManagedAccounts(raw any) []httpProxyManagedAccount {
	list, _ := raw.([]any)
	out := make([]httpProxyManagedAccount, 0, len(list))
	for _, item := range list {
		account, _ := cloneAny(item).(map[string]any)
		if account == nil {
			continue
		}
		username := firstString(account["username"])
		passwordHash := firstString(account["password_hash"])
		if username == "" || passwordHash == "" {
			continue
		}
		out = append(out, httpProxyManagedAccount{Username: username, PasswordHash: passwordHash})
	}
	return out
}

func mtprotoManagedUsers(raw any) []any {
	list, _ := raw.([]any)
	out := make([]any, 0, len(list))
	for _, item := range list {
		user, _ := cloneAny(item).(map[string]any)
		if user == nil {
			continue
		}
		if firstString(user["secret"]) == "" {
			continue
		}
		out = append(out, map[string]any{"secret": firstString(user["secret"])})
	}
	return out
}

func httpProxyPasswordHash(password string) string {
	sum := sha1.Sum([]byte(password))
	return "{SHA}" + base64.StdEncoding.EncodeToString(sum[:])
}

func mtprotoConfigPath(instance domain.Instance, spec map[string]any) string {
	slug := firstString(spec["slug"], instance.Slug, "mtproto")
	return "/usr/local/etc/xray/" + slug + ".json"
}

func httpProxyConfigPath(instance domain.Instance, spec map[string]any) string {
	slug := firstString(spec["slug"], instance.Slug, "proxy")
	return "/etc/squid/" + slug + ".conf"
}

func httpProxyPasswdPath(instance domain.Instance, spec map[string]any) string {
	slug := firstString(spec["slug"], instance.Slug, "proxy")
	return "/etc/squid/" + slug + ".passwd"
}

func httpProxyAccessLogPath(instance domain.Instance, spec map[string]any) string {
	slug := firstString(spec["slug"], instance.Slug, "proxy")
	return "stdio:/var/log/squid/" + slug + "-access.log"
}

func httpProxyCacheLogPath(instance domain.Instance, spec map[string]any) string {
	slug := firstString(spec["slug"], instance.Slug, "proxy")
	return "/var/log/squid/" + slug + "-cache.log"
}

func httpProxyPIDPath(instance domain.Instance, spec map[string]any) string {
	slug := firstString(spec["slug"], instance.Slug, "proxy")
	return "/run/" + slug + ".pid"
}

func buildMTProtoUnitFile(unitName, configPath string, instance domain.Instance) string {
	return strings.Join([]string{
		"[Unit]",
		"Description=RTIS MegaVPN MTProto instance (" + firstString(instance.Name, instance.Slug, unitName) + ")",
		"After=network-online.target",
		"Wants=network-online.target",
		"",
		"[Service]",
		"Type=simple",
		"Environment=PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"ExecStart=/usr/bin/env xray run -config " + configPath,
		"Restart=on-failure",
		"RestartSec=2s",
		"LimitNOFILE=1048576",
		"",
		"[Install]",
		"WantedBy=multi-user.target",
		"",
	}, "\n")
}

func buildXrayUnitFile(unitName, configPath string, instance domain.Instance) string {
	return strings.Join([]string{
		"[Unit]",
		"Description=RTIS MegaVPN Xray instance (" + firstString(instance.Name, instance.Slug, unitName) + ")",
		"After=network-online.target",
		"Wants=network-online.target",
		"",
		"[Service]",
		"Type=simple",
		"Environment=PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"ExecStart=/usr/bin/env xray run -config " + configPath,
		"Restart=on-failure",
		"RestartSec=2s",
		"LimitNOFILE=1048576",
		"",
		"[Install]",
		"WantedBy=multi-user.target",
		"",
	}, "\n")
}

func buildHTTPProxyUnitFile(unitName, configPath string, instance domain.Instance) string {
	return strings.Join([]string{
		"[Unit]",
		"Description=RTIS MegaVPN HTTP Proxy instance (" + firstString(instance.Name, instance.Slug, unitName) + ")",
		"After=network-online.target",
		"Wants=network-online.target",
		"",
		"[Service]",
		"Type=simple",
		"ExecStart=/usr/bin/env squid -f " + configPath + " -N",
		"ExecReload=/usr/bin/env squid -k reconfigure -f " + configPath,
		"ExecStop=/usr/bin/env squid -k shutdown -f " + configPath,
		"Restart=on-failure",
		"RestartSec=2s",
		"",
		"[Install]",
		"WantedBy=multi-user.target",
		"",
	}, "\n")
}

func buildShadowsocksUnitFile(unitName, configPath string, instance domain.Instance) string {
	return strings.Join([]string{
		"[Unit]",
		"Description=RTIS MegaVPN Shadowsocks instance (" + firstString(instance.Name, instance.Slug, unitName) + ")",
		"After=network-online.target",
		"Wants=network-online.target",
		"",
		"[Service]",
		"Type=simple",
		"ExecStart=/usr/bin/env ss-server -c " + configPath,
		"Restart=on-failure",
		"RestartSec=2s",
		"",
		"[Install]",
		"WantedBy=multi-user.target",
		"",
	}, "\n")
}

func validateSystemdExecPathArg(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("systemd exec path is required")
	}
	if !strings.HasPrefix(path, "/") {
		return fmt.Errorf("systemd exec path must be absolute")
	}
	if strings.Contains(path, "\x00") || strings.Contains(path, "..") {
		return fmt.Errorf("systemd exec path contains unsafe traversal")
	}
	if strings.ContainsAny(path, " \t\r\n'\"`$;|&()<>\\") {
		return fmt.Errorf("systemd exec path contains unsafe shell token")
	}
	return nil
}

func cloneMap(src map[string]any) map[string]any {
	if src == nil {
		return map[string]any{}
	}
	dst := make(map[string]any, len(src))
	for key, value := range src {
		dst[key] = cloneAny(value)
	}
	return dst
}

func cloneAny(value any) any {
	switch x := value.(type) {
	case map[string]any:
		return cloneMap(x)
	case []any:
		out := make([]any, len(x))
		for idx := range x {
			out[idx] = cloneAny(x[idx])
		}
		return out
	default:
		return x
	}
}

func firstString(values ...any) string {
	for _, value := range values {
		if text := strings.TrimSpace(stringify(value)); text != "" {
			return text
		}
	}
	return ""
}

func firstIntValue(values ...any) int {
	for _, value := range values {
		switch x := value.(type) {
		case int:
			if x != 0 {
				return x
			}
		case int64:
			if x != 0 {
				return int(x)
			}
		case float64:
			if x != 0 {
				return int(x)
			}
		case string:
			if n, err := strconv.Atoi(strings.TrimSpace(x)); err == nil && n != 0 {
				return n
			}
		}
	}
	return 0
}

func stringList(raw any) []string {
	switch x := raw.(type) {
	case string:
		text := strings.TrimSpace(x)
		if text == "" {
			return nil
		}
		return []string{text}
	case []any:
		out := make([]string, 0, len(x))
		for _, item := range x {
			if text := strings.TrimSpace(stringify(item)); text != "" {
				out = append(out, text)
			}
		}
		sort.Strings(out)
		return out
	case []string:
		out := make([]string, 0, len(x))
		for _, item := range x {
			if text := strings.TrimSpace(item); text != "" {
				out = append(out, text)
			}
		}
		sort.Strings(out)
		return out
	default:
		return nil
	}
}

func truthy(raw any) bool {
	switch x := raw.(type) {
	case bool:
		return x
	case string:
		x = strings.ToLower(strings.TrimSpace(x))
		return x == "1" || x == "true" || x == "yes" || x == "on"
	default:
		return false
	}
}

func explicitlyFalse(raw any) bool {
	switch x := raw.(type) {
	case bool:
		return !x
	case string:
		x = strings.ToLower(strings.TrimSpace(x))
		return x == "0" || x == "false" || x == "no" || x == "off"
	default:
		return false
	}
}

func mapFromAny(raw any) map[string]any {
	if raw == nil {
		return map[string]any{}
	}
	if value, ok := cloneAny(raw).(map[string]any); ok {
		return value
	}
	return map[string]any{}
}

func sliceFromAny(raw any) []any {
	if raw == nil {
		return []any{}
	}
	if value, ok := cloneAny(raw).([]any); ok {
		return value
	}
	return []any{}
}

func extraServerLines(raw any) []string {
	switch x := raw.(type) {
	case string:
		lines := strings.Split(x, "\n")
		out := make([]string, 0, len(lines))
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" {
				out = append(out, line)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(x))
		for _, item := range x {
			if line := strings.TrimSpace(stringify(item)); line != "" {
				out = append(out, line)
			}
		}
		return out
	default:
		return nil
	}
}

func extraIndentedLines(raw any, indent string) []string {
	lines := extraServerLines(raw)
	if len(lines) == 0 {
		return nil
	}
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, indent+line)
	}
	return out
}

func defaultXL2TPDOptions(spec map[string]any) string {
	lines := []string{
		"ipcp-accept-local",
		"ipcp-accept-remote",
		"noccp",
		"auth",
		"mtu " + firstString(spec["mtu"], "1410"),
		"mru " + firstString(spec["mru"], "1410"),
		"lock",
		"connect-delay " + firstString(spec["connect_delay"], "5000"),
	}
	if dns1 := firstString(spec["ppp_dns_primary"], "1.1.1.1"); dns1 != "" {
		lines = append(lines, "ms-dns "+dns1)
	}
	if dns2 := firstString(spec["ppp_dns_secondary"], "1.0.0.1"); dns2 != "" {
		lines = append(lines, "ms-dns "+dns2)
	}
	lines = append(lines, extraServerLines(spec["options_extra_lines"])...)
	return strings.Join(lines, "\n")
}

func defaultXL2TPDChapSecrets(spec map[string]any) string {
	entries := firstString(spec["chap_secrets_entries"], spec["chap_secrets"])
	if entries != "" {
		return entries
	}
	if managed := xl2tpdManagedAccounts(spec["managed_accounts"]); len(managed) > 0 {
		lines := make([]string, 0, len(managed))
		for _, account := range managed {
			if strings.TrimSpace(account.Username) == "" || strings.TrimSpace(account.Password) == "" {
				continue
			}
			lines = append(lines, account.Username+` l2tpd `+account.Password+` *`)
		}
		if len(lines) > 0 {
			return strings.Join(lines, "\n")
		}
	}
	username := firstString(spec["default_username"])
	password := firstString(spec["default_password"])
	if username == "" || password == "" {
		return ""
	}
	return username + ` l2tpd ` + password + ` *`
}

type xl2tpdManagedAccount struct {
	ServiceAccessID string
	Username        string
	Password        string
}

func xl2tpdManagedAccounts(raw any) []xl2tpdManagedAccount {
	list, _ := raw.([]any)
	if len(list) == 0 {
		return nil
	}
	out := make([]xl2tpdManagedAccount, 0, len(list))
	for _, item := range list {
		entry, _ := item.(map[string]any)
		account := xl2tpdManagedAccount{
			ServiceAccessID: firstString(entry["service_access_id"]),
			Username:        firstString(entry["username"]),
			Password:        firstString(entry["password"]),
		}
		if account.Username == "" || account.Password == "" {
			continue
		}
		out = append(out, account)
	}
	return out
}
