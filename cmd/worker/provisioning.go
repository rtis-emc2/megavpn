package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/infra/postgres"
	"github.com/rtis-emc2/megavpn/internal/platform/id"
	"github.com/rtis-emc2/megavpn/internal/service/driver"
)

type generatedArtifactFile struct {
	ArtifactType    string
	ServiceAccessID *string
	Filename        string
	Content         []byte
}

func handleClientProvisionJob(ctx context.Context, store *postgres.Store, job domain.Job) (string, map[string]any) {
	clientID := strings.TrimSpace(stringify(job.Payload["client_id"]))
	if clientID == "" && job.ScopeID != nil {
		clientID = strings.TrimSpace(*job.ScopeID)
	}
	if clientID == "" {
		return "failed", map[string]any{"error": "client_id is required"}
	}
	requestedArtifactType := normalizeRequestedArtifactType(stringify(job.Payload["artifact_type"]))
	if requestedArtifactType == "" {
		requestedArtifactType = normalizeRequestedArtifactType(stringify(job.Payload["type"]))
	}
	if requestedArtifactType == "" {
		requestedArtifactType = "all"
	}

	records, err := store.ListProvisioningAccesses(ctx, clientID)
	if err != nil {
		return "failed", map[string]any{"error": err.Error(), "client_id": clientID}
	}
	if len(records) == 0 {
		return "failed", map[string]any{"error": "no service accesses found", "client_id": clientID}
	}

	selectedInstances := selectedInstanceSet(job.Payload["instance_ids"])
	filtered := make([]domain.ProvisioningAccess, 0, len(records))
	for _, record := range records {
		if len(selectedInstances) > 0 && !selectedInstances[record.Instance.ID] {
			continue
		}
		if record.Access.Status == "revoked" {
			continue
		}
		filtered = append(filtered, record)
	}
	if len(filtered) == 0 {
		return "failed", map[string]any{"error": "no matching service accesses for provisioning", "client_id": clientID}
	}

	generatedFiles := make([]generatedArtifactFile, 0, len(filtered)*2)
	results := make([]map[string]any, 0, len(filtered))
	applyTargets := map[string]string{}

	for idx := range filtered {
		record := &filtered[idx]
		serviceCode := normalizeServiceCode(record.Instance.ServiceCode)
		var files []generatedArtifactFile
		switch serviceCode {
		case "openvpn":
			err = ensureOpenVPNInstanceAndClientState(ctx, store, record)
			if err == nil {
				files, err = buildOpenVPNArtifacts(ctx, store, *record)
			}
			applyTargets[record.Instance.ID] = serviceCode
		case "wireguard":
			if ensureErr := ensureWireGuardInstanceDriverState(ctx, store, record.Instance.ID); ensureErr != nil {
				err = ensureErr
			}
			if err == nil {
				if refreshed, refreshErr := store.GetInstanceWithSpec(ctx, record.Instance.ID); refreshErr == nil {
					record.Instance = refreshed
				}
				if refreshedAccesses, listErr := store.ListProvisioningAccesses(ctx, clientID); listErr == nil {
					for _, refreshed := range refreshedAccesses {
						if refreshed.Access.ID == record.Access.ID {
							record.Access = refreshed.Access
							break
						}
					}
				}
				files, err = buildWireGuardArtifacts(ctx, store, *record)
			}
			applyTargets[record.Instance.ID] = serviceCode
		case "mtproto":
			if ensureErr := ensureMTProtoInstanceDriverState(ctx, store, record.Instance.ID); ensureErr != nil {
				err = ensureErr
			}
			if err == nil {
				if refreshed, refreshErr := store.GetInstanceWithSpec(ctx, record.Instance.ID); refreshErr == nil {
					record.Instance = refreshed
				}
				if refreshedAccesses, listErr := store.ListProvisioningAccesses(ctx, clientID); listErr == nil {
					for _, refreshed := range refreshedAccesses {
						if refreshed.Access.ID == record.Access.ID {
							record.Access = refreshed.Access
							break
						}
					}
				}
				files, err = buildMTProtoArtifacts(*record)
			}
			applyTargets[record.Instance.ID] = serviceCode
		case "xray-core":
			if xrayUUID := strings.TrimSpace(stringify(record.Access.Metadata["xray_uuid"])); xrayUUID == "" {
				record.Access.Metadata["xray_uuid"] = id.New()
			}
			if updateErr := store.UpdateServiceAccessMetadata(ctx, record.Access.ID, record.Access.Metadata); updateErr != nil {
				err = updateErr
			}
			if err == nil {
				if ensureErr := ensureXrayInstanceDriverState(ctx, store, record.Instance.ID); ensureErr != nil {
					err = ensureErr
				}
			}
			if err == nil {
				if refreshed, refreshErr := store.GetInstanceWithSpec(ctx, record.Instance.ID); refreshErr == nil {
					record.Instance = refreshed
				}
				files, err = buildXrayArtifacts(*record)
			}
			applyTargets[record.Instance.ID] = serviceCode
		case "ipsec":
			var familyTargets map[string]string
			familyTargets, err = ensureIPSecL2TPInstanceDriverState(ctx, store, record)
			if err == nil {
				if refreshed, refreshErr := store.GetInstanceWithSpec(ctx, record.Instance.ID); refreshErr == nil {
					record.Instance = refreshed
				}
				if refreshedAccesses, listErr := store.ListProvisioningAccesses(ctx, clientID); listErr == nil {
					for _, refreshed := range refreshedAccesses {
						if refreshed.Access.ID == record.Access.ID {
							record.Access = refreshed.Access
							break
						}
					}
				}
				files, err = buildIPSecBundleArtifacts(ctx, store, *record)
			}
			for instanceID, runtimeCode := range familyTargets {
				applyTargets[instanceID] = runtimeCode
			}
		case "shadowsocks":
			if ensureErr := ensureShadowsocksInstanceDriverState(ctx, store, record.Instance.ID); ensureErr != nil {
				err = ensureErr
			}
			if err == nil {
				if refreshed, refreshErr := store.GetInstanceWithSpec(ctx, record.Instance.ID); refreshErr == nil {
					record.Instance = refreshed
				}
				if refreshedAccesses, listErr := store.ListProvisioningAccesses(ctx, clientID); listErr == nil {
					for _, refreshed := range refreshedAccesses {
						if refreshed.Access.ID == record.Access.ID {
							record.Access = refreshed.Access
							break
						}
					}
				}
				files, err = buildShadowsocksArtifacts(*record)
			}
			applyTargets[record.Instance.ID] = serviceCode
		case "http_proxy":
			if ensureErr := ensureHTTPProxyInstanceDriverState(ctx, store, record.Instance.ID); ensureErr != nil {
				err = ensureErr
			}
			if err == nil {
				if refreshed, refreshErr := store.GetInstanceWithSpec(ctx, record.Instance.ID); refreshErr == nil {
					record.Instance = refreshed
				}
				if refreshedAccesses, listErr := store.ListProvisioningAccesses(ctx, clientID); listErr == nil {
					for _, refreshed := range refreshedAccesses {
						if refreshed.Access.ID == record.Access.ID {
							record.Access = refreshed.Access
							break
						}
					}
				}
				files, err = buildHTTPProxyArtifacts(*record)
			}
			applyTargets[record.Instance.ID] = serviceCode
		default:
			err = fmt.Errorf("provisioning driver is not implemented for %s", serviceCode)
		}
		if err != nil {
			return "failed", map[string]any{
				"error":             err.Error(),
				"client_id":         clientID,
				"instance_id":       record.Instance.ID,
				"service_access_id": record.Access.ID,
				"service_code":      record.Instance.ServiceCode,
			}
		}
		generatedFiles = append(generatedFiles, files...)
		results = append(results, map[string]any{
			"service_access_id": record.Access.ID,
			"instance_id":       record.Instance.ID,
			"service_code":      serviceCode,
			"artifacts":         artifactNames(files),
		})
	}

	queuedApplies := make([]map[string]any, 0, len(applyTargets))
	for instanceID, serviceCode := range applyTargets {
		switch serviceCode {
		case "mtproto":
			if err := ensureMTProtoInstanceDriverState(ctx, store, instanceID); err != nil {
				return "failed", map[string]any{"error": err.Error(), "instance_id": instanceID, "service_code": serviceCode}
			}
		case "http_proxy":
			if err := ensureHTTPProxyInstanceDriverState(ctx, store, instanceID); err != nil {
				return "failed", map[string]any{"error": err.Error(), "instance_id": instanceID, "service_code": serviceCode}
			}
		case "wireguard":
			if err := ensureWireGuardInstanceDriverState(ctx, store, instanceID); err != nil {
				return "failed", map[string]any{"error": err.Error(), "instance_id": instanceID, "service_code": serviceCode}
			}
		case "xray-core":
			if err := ensureXrayInstanceDriverState(ctx, store, instanceID); err != nil {
				return "failed", map[string]any{"error": err.Error(), "instance_id": instanceID, "service_code": serviceCode}
			}
		case "openvpn":
			// PKI/state already ensured during per-access provisioning.
		case "ipsec", "xl2tpd":
			// Family state already ensured during per-access provisioning.
		case "shadowsocks":
			if err := ensureShadowsocksInstanceDriverState(ctx, store, instanceID); err != nil {
				return "failed", map[string]any{"error": err.Error(), "instance_id": instanceID, "service_code": serviceCode}
			}
		}
		jobRecord, err := store.UpdateInstanceStatus(ctx, instanceID, "apply")
		if err != nil {
			return "failed", map[string]any{"error": err.Error(), "instance_id": instanceID, "service_code": serviceCode, "phase": "queue_instance_apply"}
		}
		queuedApplies = append(queuedApplies, map[string]any{
			"instance_id":  instanceID,
			"service_code": serviceCode,
			"job_id":       jobRecord.ID,
		})
	}

	if len(generatedFiles) == 0 {
		return "failed", map[string]any{"error": "no artifacts generated", "client_id": clientID}
	}

	includeZip := requestedArtifactType == "all" || requestedArtifactType == "zip_bundle"
	filesToSave := selectArtifactFiles(generatedFiles, requestedArtifactType)
	if requestedArtifactType == "zip_bundle" {
		filesToSave = nil
	}
	if len(filesToSave) == 0 && !includeZip {
		return "failed", map[string]any{"error": "requested artifact type was not generated", "client_id": clientID, "artifact_type": requestedArtifactType}
	}

	saved := make([]domain.Artifact, 0, len(filesToSave)+1)
	for _, file := range filesToSave {
		artifact, err := store.SaveArtifactContent(ctx, clientID, file.ServiceAccessID, file.ArtifactType, file.Filename, file.Content)
		if err != nil {
			return "failed", map[string]any{
				"error":             err.Error(),
				"client_id":         clientID,
				"artifact_type":     file.ArtifactType,
				"service_access_id": derefString(file.ServiceAccessID),
				"artifact_filename": file.Filename,
			}
		}
		saved = append(saved, artifact)
	}

	if includeZip {
		clientName := firstNonEmpty(filtered[0].Client.Username, filtered[0].Client.DisplayName, "client")
		bundleName := sanitizeLocalFilename(clientName) + "-bundle.zip"
		bundleContent, err := buildZipBundle(clientName, generatedFiles)
		if err != nil {
			return "failed", map[string]any{"error": err.Error(), "client_id": clientID, "artifact_type": "zip_bundle"}
		}
		bundle, err := store.SaveArtifactContent(ctx, clientID, nil, "zip_bundle", bundleName, bundleContent)
		if err != nil {
			return "failed", map[string]any{"error": err.Error(), "client_id": clientID, "artifact_type": "zip_bundle"}
		}
		saved = append(saved, bundle)
	}

	return "succeeded", map[string]any{
		"message":             "client artifacts generated",
		"client_id":           clientID,
		"artifact_type":       requestedArtifactType,
		"access_count":        len(filtered),
		"results":             results,
		"artifacts":           artifactSummaries(saved),
		"instance_apply_jobs": queuedApplies,
	}
}

func normalizeRequestedArtifactType(value string) string {
	return driver.NormalizeArtifactType(value)
}

func selectArtifactFiles(files []generatedArtifactFile, artifactType string) []generatedArtifactFile {
	artifactType = normalizeRequestedArtifactType(artifactType)
	if artifactType == "" || artifactType == "all" || artifactType == "zip_bundle" {
		return files
	}
	out := make([]generatedArtifactFile, 0, len(files))
	for _, file := range files {
		if file.ArtifactType == artifactType {
			out = append(out, file)
		}
	}
	return out
}

func buildOpenVPNArtifacts(ctx context.Context, store *postgres.Store, record domain.ProvisioningAccess) ([]generatedArtifactFile, error) {
	spec := record.Instance.Spec
	meta := record.Access.Metadata
	if spec == nil {
		spec = map[string]any{}
	}
	if meta == nil {
		meta = map[string]any{}
	}

	inline := firstNonEmpty(stringify(meta["ovpn_inline"]), stringify(spec["ovpn_inline"]), stringify(spec["client_ovpn_inline"]))
	clientName := firstNonEmpty(record.Client.Username, record.Client.DisplayName, record.Access.ID)
	filename := sanitizeLocalFilename(clientName) + "--" + sanitizeLocalFilename(firstNonEmpty(record.Instance.Slug, record.Instance.Name, "openvpn")) + ".ovpn"

	var body string
	if inline != "" {
		body = renderTemplateVariables(inline, record)
		if !strings.HasSuffix(body, "\n") {
			body += "\n"
		}
	} else {
		remoteHost := firstNonEmpty(stringify(meta["remote_host"]), stringify(spec["remote_host"]), stringify(spec["server_host"]), record.Instance.EndpointHost)
		if remoteHost == "" {
			return nil, fmt.Errorf("openvpn remote host is required")
		}
		remotePort := firstInt(meta["remote_port"], spec["remote_port"], spec["server_port"], record.Instance.EndpointPort)
		if remotePort <= 0 {
			remotePort = 1194
		}
		proto := firstNonEmpty(stringify(meta["proto"]), stringify(spec["proto"]), "tcp")
		dev := firstNonEmpty(stringify(meta["dev"]), stringify(spec["dev"]), "tun")
		caPEM, err := resolveOpenVPNText(ctx, store, meta["ca_cert_secret_ref_id"], spec["ca_cert_secret_ref_id"], meta["ca_pem"], spec["ca_pem"])
		if err != nil {
			return nil, err
		}
		certPEM, err := resolveOpenVPNText(ctx, store, meta["openvpn_client_cert_secret_ref_id"], meta["client_cert_secret_ref_id"], meta["client_cert_pem"], spec["client_cert_pem"])
		if err != nil {
			return nil, err
		}
		keyPEM, err := resolveOpenVPNText(ctx, store, meta["openvpn_client_key_secret_ref_id"], meta["client_key_secret_ref_id"], meta["client_key_pem"], spec["client_key_pem"])
		if err != nil {
			return nil, err
		}
		if caPEM == "" || certPEM == "" || keyPEM == "" {
			return nil, fmt.Errorf("openvpn client materials are incomplete; ca_pem, client_cert_pem and client_key_pem are required")
		}
		var lines []string
		lines = append(lines,
			"client",
			"nobind",
			"persist-key",
			"persist-tun",
			"remote-cert-tls "+firstNonEmpty(stringify(spec["remote_cert_tls"]), "server"),
			"verb 3",
			"dev "+dev,
			"proto "+proto,
			fmt.Sprintf("remote %s %d", remoteHost, remotePort),
		)
		proxyMode := strings.ToLower(firstNonEmpty(stringify(meta["client_proxy_mode"]), stringify(spec["client_proxy_mode"]), stringify(meta["proxy_mode"]), stringify(spec["proxy_mode"])))
		switch proxyMode {
		case "socks", "socks5":
			proxyHost := firstNonEmpty(stringify(meta["socks_proxy_host"]), stringify(spec["socks_proxy_host"]), stringify(meta["proxy_host"]), stringify(spec["proxy_host"]), "127.0.0.1")
			proxyPort := firstInt(meta["socks_proxy_port"], spec["socks_proxy_port"], meta["proxy_port"], spec["proxy_port"])
			if proxyPort <= 0 {
				return nil, fmt.Errorf("openvpn socks proxy port is required")
			}
			lines = append(lines,
				"# Route OpenVPN TCP through the local proxy exposed by the VLESS client.",
				fmt.Sprintf("socks-proxy %s %d", proxyHost, proxyPort),
			)
		case "http", "http-connect":
			proxyHost := firstNonEmpty(stringify(meta["http_proxy_host"]), stringify(spec["http_proxy_host"]), stringify(meta["proxy_host"]), stringify(spec["proxy_host"]), "127.0.0.1")
			proxyPort := firstInt(meta["http_proxy_port"], spec["http_proxy_port"], meta["proxy_port"], spec["proxy_port"])
			if proxyPort <= 0 {
				return nil, fmt.Errorf("openvpn http proxy port is required")
			}
			lines = append(lines,
				"# Route OpenVPN TCP through the local HTTP CONNECT proxy exposed by the VLESS client.",
				fmt.Sprintf("http-proxy %s %d", proxyHost, proxyPort),
			)
		case "", "none", "direct":
		default:
			return nil, fmt.Errorf("unsupported openvpn client proxy mode %q", proxyMode)
		}
		if cipher := firstNonEmpty(stringify(spec["cipher"]), stringify(meta["cipher"])); cipher != "" {
			lines = append(lines, "cipher "+cipher)
		}
		if auth := firstNonEmpty(stringify(spec["auth"]), stringify(meta["auth"])); auth != "" {
			lines = append(lines, "auth "+auth)
		}
		lines = append(lines, strings.TrimRight(normalizePEMBlock("ca", caPEM), "\n"))
		lines = append(lines, strings.TrimRight(normalizePEMBlock("cert", certPEM), "\n"))
		lines = append(lines, strings.TrimRight(normalizePEMBlock("key", keyPEM), "\n"))
		tlsCrypt, err := resolveOpenVPNText(ctx, store, meta["tls_crypt_secret_ref_id"], spec["tls_crypt_secret_ref_id"], meta["tls_crypt_key"], spec["tls_crypt_key"])
		if err != nil {
			return nil, err
		}
		if tlsCrypt != "" {
			lines = append(lines, strings.TrimRight(normalizePEMBlock("tls-crypt", tlsCrypt), "\n"))
		} else if tlsAuth, err := resolveOpenVPNText(ctx, store, meta["tls_auth_secret_ref_id"], spec["tls_auth_secret_ref_id"], meta["tls_auth_key"], spec["tls_auth_key"]); err != nil {
			return nil, err
		} else if tlsAuth != "" {
			lines = append(lines, "key-direction 1")
			lines = append(lines, strings.TrimRight(normalizePEMBlock("tls-auth", tlsAuth), "\n"))
		}
		lines = append(lines, extraConfigLines(spec["client_extra_lines"], meta["client_extra_lines"])...)
		body = strings.Join(lines, "\n") + "\n"
	}

	accessID := record.Access.ID
	return []generatedArtifactFile{{
		ArtifactType:    "ovpn",
		ServiceAccessID: &accessID,
		Filename:        filename,
		Content:         []byte(body),
	}}, nil
}

func buildWireGuardArtifacts(ctx context.Context, store *postgres.Store, record domain.ProvisioningAccess) ([]generatedArtifactFile, error) {
	spec := record.Instance.Spec
	meta := record.Access.Metadata
	if spec == nil {
		spec = map[string]any{}
	}
	if meta == nil {
		meta = map[string]any{}
	}

	privateKey, err := resolveWorkerSecretText(ctx, store, meta["wireguard_client_private_key_secret_ref_id"], meta["wireguard_client_private_key"])
	if err != nil {
		return nil, err
	}
	if privateKey == "" {
		return nil, fmt.Errorf("wireguard client private key is required")
	}
	address := firstNonEmpty(stringify(meta["wireguard_client_address"]))
	if address == "" {
		return nil, fmt.Errorf("wireguard client address is required")
	}
	serverPublicKey := firstNonEmpty(stringify(meta["wireguard_server_public_key"]), stringify(spec["server_public_key"]))
	if serverPublicKey == "" {
		return nil, fmt.Errorf("wireguard server public key is required")
	}
	serverHost := firstNonEmpty(stringify(meta["server_host"]), stringify(spec["server_host"]), stringify(spec["public_host"]), record.Instance.EndpointHost)
	if serverHost == "" {
		return nil, fmt.Errorf("wireguard server host is required")
	}
	serverPort := firstInt(meta["server_port"], spec["listen_port"], spec["server_port"], record.Instance.EndpointPort)
	if serverPort <= 0 {
		serverPort = 51820
	}
	allowedIPs := firstNonEmpty(stringify(meta["allowed_ips"]), stringify(spec["client_allowed_ips"]), "0.0.0.0/0, ::/0")
	dnsServers := firstNonEmpty(stringify(meta["dns_servers"]), stringify(spec["client_dns"]))
	persistentKeepalive := firstInt(meta["persistent_keepalive"], spec["persistent_keepalive"], 25)

	lines := []string{
		"[Interface]",
		"PrivateKey = " + privateKey,
		"Address = " + address,
	}
	if dnsServers != "" {
		lines = append(lines, "DNS = "+dnsServers)
	}
	lines = append(lines,
		"",
		"[Peer]",
		"PublicKey = "+serverPublicKey,
		fmt.Sprintf("Endpoint = %s:%d", serverHost, serverPort),
		"AllowedIPs = "+allowedIPs,
	)
	if presharedKey, err := resolveWorkerSecretText(ctx, store, meta["wireguard_preshared_key_secret_ref_id"], meta["wireguard_preshared_key"]); err != nil {
		return nil, err
	} else if presharedKey != "" {
		lines = append(lines, "PresharedKey = "+presharedKey)
	}
	if persistentKeepalive > 0 {
		lines = append(lines, fmt.Sprintf("PersistentKeepalive = %d", persistentKeepalive))
	}

	accessID := record.Access.ID
	filename := sanitizeLocalFilename(firstNonEmpty(record.Client.Username, record.Client.DisplayName, "client")) + "--" + sanitizeLocalFilename(firstNonEmpty(record.Instance.Slug, record.Instance.Name, "wireguard")) + ".conf"
	return []generatedArtifactFile{{
		ArtifactType:    "wg_conf",
		ServiceAccessID: &accessID,
		Filename:        filename,
		Content:         []byte(strings.Join(lines, "\n") + "\n"),
	}}, nil
}

func buildMTProtoArtifacts(record domain.ProvisioningAccess) ([]generatedArtifactFile, error) {
	spec := record.Instance.Spec
	meta := record.Access.Metadata
	if spec == nil {
		spec = map[string]any{}
	}
	if meta == nil {
		meta = map[string]any{}
	}
	secret := firstNonEmpty(stringify(meta["mtproto_secret"]), stringify(meta["secret"]))
	if secret == "" {
		return nil, fmt.Errorf("mtproto secret is required")
	}
	host := firstNonEmpty(stringify(meta["server_host"]), stringify(spec["server_host"]), stringify(spec["public_host"]), record.Instance.EndpointHost)
	if host == "" {
		return nil, fmt.Errorf("mtproto server host is required")
	}
	port := firstInt(meta["server_port"], spec["server_port"], spec["listen_port"], record.Instance.EndpointPort)
	if port <= 0 {
		port = 443
	}
	mtprotoURL := fmt.Sprintf("tg://proxy?server=%s&port=%d&secret=%s", host, port, secret)
	accessID := record.Access.ID
	filename := sanitizeLocalFilename(firstNonEmpty(record.Client.Username, record.Client.DisplayName, "client")) + "--" + sanitizeLocalFilename(firstNonEmpty(record.Instance.Slug, record.Instance.Name, "mtproto")) + ".mtproto.txt"
	return []generatedArtifactFile{{
		ArtifactType:    "mtproto_url",
		ServiceAccessID: &accessID,
		Filename:        filename,
		Content:         []byte(mtprotoURL + "\n"),
	}}, nil
}

func buildXrayArtifacts(record domain.ProvisioningAccess) ([]generatedArtifactFile, error) {
	spec := record.Instance.Spec
	meta := record.Access.Metadata
	if spec == nil {
		spec = map[string]any{}
	}
	if meta == nil {
		meta = map[string]any{}
		record.Access.Metadata = meta
	}

	xrayUUID := firstNonEmpty(stringify(meta["xray_uuid"]), stringify(meta["uuid"]), stringify(spec["xray_uuid"]))
	if xrayUUID == "" {
		xrayUUID = id.New()
		meta["xray_uuid"] = xrayUUID
	}
	host := firstNonEmpty(stringify(meta["public_host"]), stringify(meta["server_host"]), stringify(spec["public_host"]), stringify(spec["server_host"]), record.Instance.EndpointHost)
	if host == "" {
		return nil, fmt.Errorf("xray server host is required")
	}
	port := firstInt(meta["public_port"], meta["server_port"], spec["public_port"], spec["server_port"], spec["port"], record.Instance.EndpointPort)
	if port <= 0 {
		port = 443
	}
	security := firstNonEmpty(stringify(meta["public_security"]), stringify(meta["security"]), stringify(spec["public_security"]), stringify(spec["security"]), "reality")
	query := url.Values{}
	query.Set("encryption", "none")
	query.Set("security", security)
	if flow := firstNonEmpty(stringify(meta["public_flow"]), stringify(meta["flow"]), stringify(spec["public_flow"]), stringify(spec["flow"])); flow != "" {
		query.Set("flow", flow)
	}
	if sni := firstNonEmpty(stringify(meta["public_sni"]), stringify(meta["sni"]), stringify(spec["public_sni"]), stringify(spec["sni"]), stringify(spec["public_server_name"]), stringify(spec["server_name"]), host); sni != "" {
		query.Set("sni", sni)
	}
	if fp := firstNonEmpty(stringify(meta["public_fingerprint"]), stringify(meta["fingerprint"]), stringify(spec["public_fingerprint"]), stringify(spec["fingerprint"])); fp != "" || security == "reality" {
		if fp == "" {
			fp = "chrome"
		}
		query.Set("fp", fp)
	}
	if pbk := firstNonEmpty(stringify(meta["reality_public_key"]), stringify(spec["reality_public_key"]), stringify(spec["public_key"]), stringify(spec["pbk"])); pbk != "" && security == "reality" {
		query.Set("pbk", pbk)
	}
	if sid := firstNonEmpty(stringify(meta["short_id"]), stringify(spec["short_id"]), stringify(spec["sid"])); sid != "" && security == "reality" {
		query.Set("sid", sid)
	}
	network := firstNonEmpty(stringify(meta["public_network"]), stringify(meta["type"]), stringify(spec["public_network"]), stringify(spec["type"]), stringify(spec["transport"]), stringify(spec["network"]), "tcp")
	query.Set("type", network)
	if path := firstNonEmpty(stringify(meta["public_path"]), stringify(meta["path"]), stringify(spec["public_path"]), stringify(spec["path"])); path != "" {
		query.Set("path", path)
	}
	if serviceName := firstNonEmpty(stringify(meta["public_service_name"]), stringify(meta["service_name"]), stringify(spec["public_service_name"]), stringify(spec["service_name"])); serviceName != "" {
		query.Set("serviceName", serviceName)
	}
	if hostHeader := firstNonEmpty(stringify(meta["public_host_header"]), stringify(spec["public_host_header"])); hostHeader != "" {
		query.Set("host", hostHeader)
	}
	if alpn := firstNonEmpty(stringify(meta["public_alpn"]), stringify(meta["alpn"]), stringify(spec["public_alpn"]), stringify(spec["alpn"])); alpn != "" {
		query.Set("alpn", alpn)
	}
	rawLabel := firstNonEmpty(stringify(spec["client_label_prefix"]), record.Instance.Name, record.Instance.Slug, "xray") + "-" + firstNonEmpty(record.Client.Username, record.Access.ID)
	label := url.QueryEscape(rawLabel)
	vlessURL := fmt.Sprintf("vless://%s@%s:%d?%s#%s", xrayUUID, host, port, query.Encode(), label)
	content, err := buildVLESSClientArtifactContent(record, vlessURL, rawLabel, host, port, security, network, query)
	if err != nil {
		return nil, err
	}

	accessID := record.Access.ID
	filename := sanitizeLocalFilename(firstNonEmpty(record.Client.Username, record.Client.DisplayName, "client")) + "--" + sanitizeLocalFilename(firstNonEmpty(record.Instance.Slug, record.Instance.Name, "xray")) + ".vless.txt"
	return []generatedArtifactFile{{
		ArtifactType:    "vless_url",
		ServiceAccessID: &accessID,
		Filename:        filename,
		Content:         content,
	}}, nil
}

func buildVLESSClientArtifactContent(record domain.ProvisioningAccess, vlessURL, label, host string, port int, security, network string, query url.Values) ([]byte, error) {
	meta := record.Access.Metadata
	if meta == nil {
		meta = map[string]any{}
	}
	inbound, _ := meta["inbound_service"].(map[string]any)
	stream := map[string]any{
		"network":     network,
		"security":    security,
		"sni":         query.Get("sni"),
		"fingerprint": query.Get("fp"),
		"alpn":        query.Get("alpn"),
	}
	if security == "reality" {
		stream["reality_public_key"] = query.Get("pbk")
		stream["short_id"] = query.Get("sid")
	}
	if path := query.Get("path"); path != "" {
		stream["path"] = path
	}
	if serviceName := query.Get("serviceName"); serviceName != "" {
		stream["service_name"] = serviceName
	}
	if hostHeader := query.Get("host"); hostHeader != "" {
		stream["host_header"] = hostHeader
	}
	outboundGroup := firstNonEmpty(
		stringify(meta["vless_group"]),
		stringify(meta["xray_group"]),
		stringify(meta["outbound_group"]),
		stringify(inbound["vless_group"]),
		stringify(inbound["xray_group"]),
		stringify(inbound["outbound_group"]),
		stringify(record.Instance.Spec["default_vless_group"]),
		"default",
	)
	credential := map[string]any{
		"id":         firstNonEmpty(stringify(meta["xray_uuid"]), stringify(meta["uuid"])),
		"encryption": "none",
	}
	if flow := query.Get("flow"); flow != "" {
		credential["flow"] = flow
	}
	profile := map[string]any{
		"name":              label,
		"protocol":          "vless",
		"service_access_id": record.Access.ID,
		"client": map[string]any{
			"id":       record.Client.ID,
			"username": record.Client.Username,
			"email":    record.Client.Email,
		},
		"instance": map[string]any{
			"id":           record.Instance.ID,
			"name":         record.Instance.Name,
			"slug":         record.Instance.Slug,
			"service_code": record.Instance.ServiceCode,
		},
		"inbound_service": inbound,
		"outbound_group":  outboundGroup,
		"endpoint": map[string]any{
			"host": host,
			"port": port,
		},
		"credential": credential,
		"stream":     stream,
		"uri":        vlessURL,
	}
	profileJSON, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return nil, err
	}
	lines := []string{
		"VLESS client access",
		"",
		"Client: " + firstNonEmpty(record.Client.Username, record.Client.DisplayName, record.Client.ID),
		"Instance: " + firstNonEmpty(record.Instance.Name, record.Instance.Slug, record.Instance.ID),
		fmt.Sprintf("Endpoint: %s:%d", host, port),
		"Network: " + network,
		"Security: " + security,
	}
	if query.Get("sni") != "" {
		lines = append(lines, "SNI: "+query.Get("sni"))
	}
	if query.Get("fp") != "" {
		lines = append(lines, "Fingerprint: "+query.Get("fp"))
	}
	if query.Get("pbk") != "" {
		lines = append(lines, "Reality public key: "+query.Get("pbk"))
	}
	if query.Get("sid") != "" {
		lines = append(lines, "Reality short id: "+query.Get("sid"))
	}
	if query.Get("flow") != "" {
		lines = append(lines, "Flow: "+query.Get("flow"))
	}
	if outboundGroup != "" {
		lines = append(lines, "Access group: "+outboundGroup)
	}
	lines = append(lines,
		"",
		"Import URI:",
		vlessURL,
		"",
		"Client JSON:",
		string(profileJSON),
		"",
	)
	return []byte(strings.Join(lines, "\n")), nil
}

func buildShadowsocksArtifacts(record domain.ProvisioningAccess) ([]generatedArtifactFile, error) {
	spec := record.Instance.Spec
	meta := record.Access.Metadata
	if spec == nil {
		spec = map[string]any{}
	}
	if meta == nil {
		meta = map[string]any{}
	}

	method := firstNonEmpty(stringify(meta["method"]), stringify(spec["method"]), "chacha20-ietf-poly1305")
	password := firstNonEmpty(stringify(meta["password"]), stringify(meta["shadowsocks_password"]), stringify(spec["password"]), stringify(spec["server_password"]))
	account := shadowsocksManagedAccountForAccess(spec["managed_accounts"], record.Access.ID)
	if password == "" {
		password = firstNonEmpty(stringify(account["password"]), stringify(account["shadowsocks_password"]))
	}
	if password == "" {
		return nil, fmt.Errorf("shadowsocks password is required")
	}
	host := firstNonEmpty(stringify(meta["server_host"]), stringify(spec["server_host"]), stringify(spec["public_host"]), record.Instance.EndpointHost)
	if host == "" {
		return nil, fmt.Errorf("shadowsocks server host is required")
	}
	port := firstInt(meta["server_port"], meta["port"], account["server_port"], account["port"], spec["server_port"], record.Instance.EndpointPort)
	if port <= 0 {
		return nil, fmt.Errorf("shadowsocks server port is required")
	}

	credentials := method + ":" + password
	encoded := base64.RawURLEncoding.EncodeToString([]byte(credentials))
	label := url.QueryEscape(firstNonEmpty(record.Instance.Name, record.Instance.Slug, "shadowsocks") + "-" + firstNonEmpty(record.Client.Username, record.Access.ID))
	ssURL := fmt.Sprintf("ss://%s@%s:%d#%s", encoded, host, port, label)

	accessID := record.Access.ID
	filename := sanitizeLocalFilename(firstNonEmpty(record.Client.Username, record.Client.DisplayName, "client")) + "--" + sanitizeLocalFilename(firstNonEmpty(record.Instance.Slug, record.Instance.Name, "shadowsocks")) + ".ss.txt"
	return []generatedArtifactFile{{
		ArtifactType:    "ss_url",
		ServiceAccessID: &accessID,
		Filename:        filename,
		Content:         []byte(ssURL + "\n"),
	}}, nil
}

func shadowsocksManagedAccountForAccess(raw any, accessID string) map[string]any {
	accessID = strings.TrimSpace(accessID)
	if accessID == "" {
		return nil
	}
	switch accounts := raw.(type) {
	case []any:
		for _, item := range accounts {
			account, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if strings.TrimSpace(stringify(account["service_access_id"])) == accessID {
				return account
			}
		}
	case []map[string]any:
		for _, account := range accounts {
			if strings.TrimSpace(stringify(account["service_access_id"])) == accessID {
				return account
			}
		}
	}
	return nil
}

func buildHTTPProxyArtifacts(record domain.ProvisioningAccess) ([]generatedArtifactFile, error) {
	spec := record.Instance.Spec
	meta := record.Access.Metadata
	if spec == nil {
		spec = map[string]any{}
	}
	if meta == nil {
		meta = map[string]any{}
	}
	username := firstNonEmpty(stringify(meta["username"]), stringify(meta["proxy_username"]), stringify(meta["http_proxy_username"]))
	password := firstNonEmpty(stringify(meta["password"]), stringify(meta["proxy_password"]), stringify(meta["http_proxy_password"]))
	host := firstNonEmpty(stringify(meta["server_host"]), stringify(spec["server_host"]), stringify(spec["public_host"]), record.Instance.EndpointHost)
	port := firstInt(meta["server_port"], spec["listen_port"], spec["server_port"], record.Instance.EndpointPort)
	if username == "" || password == "" {
		return nil, fmt.Errorf("http proxy credentials are incomplete")
	}
	if host == "" {
		return nil, fmt.Errorf("http proxy server host is required")
	}
	if port <= 0 {
		port = 3128
	}
	scheme := firstNonEmpty(stringify(meta["scheme"]), stringify(spec["scheme"]), "http")
	lines := []string{
		"RTIS MegaVPN HTTP Proxy access",
		"",
		"Server: " + host,
		fmt.Sprintf("Port: %d", port),
		"Scheme: " + scheme,
		"Username: " + username,
		"Password: " + password,
		"",
		fmt.Sprintf("Proxy URL: %s://%s:%s@%s:%d", scheme, username, password, host, port),
	}
	accessID := record.Access.ID
	filename := sanitizeLocalFilename(firstNonEmpty(record.Client.Username, record.Client.DisplayName, "client")) + "--" + sanitizeLocalFilename(firstNonEmpty(record.Instance.Slug, record.Instance.Name, "http-proxy")) + ".proxy.txt"
	return []generatedArtifactFile{{
		ArtifactType:    "http_proxy_bundle",
		ServiceAccessID: &accessID,
		Filename:        filename,
		Content:         []byte(strings.Join(lines, "\n") + "\n"),
	}}, nil
}

func buildIPSecBundleArtifacts(ctx context.Context, store *postgres.Store, record domain.ProvisioningAccess) ([]generatedArtifactFile, error) {
	spec := record.Instance.Spec
	meta := record.Access.Metadata
	if spec == nil {
		spec = map[string]any{}
	}
	if meta == nil {
		meta = map[string]any{}
	}

	xl2tpdInstance, err := store.FindNodeInstanceByService(ctx, record.Instance.NodeID, "xl2tpd")
	if err != nil {
		return nil, fmt.Errorf("xl2tpd companion instance is required for ipsec bundle generation: %w", err)
	}
	xl2tpdInstance, err = store.GetInstanceWithSpec(ctx, xl2tpdInstance.ID)
	if err != nil {
		return nil, err
	}

	psk, err := resolveWorkerSecretText(ctx, store, meta["ipsec_psk_secret_ref_id"], spec["psk_secret_ref_id"], meta["ipsec_psk"], spec["psk"])
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(psk) == "" {
		return nil, fmt.Errorf("ipsec psk is required")
	}

	username := firstNonEmpty(stringify(meta["username"]), stringify(meta["l2tp_username"]), stringify(meta["ppp_username"]))
	password := firstNonEmpty(stringify(meta["password"]), stringify(meta["l2tp_password"]), stringify(meta["ppp_password"]))
	if username == "" || password == "" {
		return nil, fmt.Errorf("l2tp client credentials are incomplete")
	}

	serverHost := firstNonEmpty(stringify(meta["server_host"]), stringify(spec["server_host"]), record.Instance.EndpointHost, xl2tpdInstance.EndpointHost)
	if serverHost == "" {
		return nil, fmt.Errorf("ipsec server host is required")
	}
	serverID := firstNonEmpty(stringify(meta["server_id"]), stringify(spec["server_id"]), stringify(spec["leftid"]), serverHost)
	l2tpPort := firstInt(meta["server_port"], meta["l2tp_port"], xl2tpdInstance.Spec["listen_port"], xl2tpdInstance.Spec["server_port"], xl2tpdInstance.EndpointPort)
	if l2tpPort <= 0 {
		l2tpPort = 1701
	}

	lines := []string{
		"MegaVPN IPsec/L2TP access package",
		"",
		"Mode: L2TP over IPsec (PSK)",
		"Client: " + firstNonEmpty(record.Client.Username, record.Client.DisplayName, record.Access.ID),
		"Instance: " + firstNonEmpty(record.Instance.Name, record.Instance.Slug, record.Instance.ID),
		"",
		"Connection settings",
		"Server: " + serverHost,
		"Server ID: " + serverID,
		fmt.Sprintf("L2TP port: %d", l2tpPort),
		"IPsec PSK: " + psk,
		"Username: " + username,
		"Password: " + password,
	}
	if dns1 := firstNonEmpty(stringify(xl2tpdInstance.Spec["ppp_dns_primary"])); dns1 != "" {
		lines = append(lines, "DNS primary: "+dns1)
	}
	if dns2 := firstNonEmpty(stringify(xl2tpdInstance.Spec["ppp_dns_secondary"])); dns2 != "" {
		lines = append(lines, "DNS secondary: "+dns2)
	}
	if ike := firstNonEmpty(stringify(spec["ike"])); ike != "" {
		lines = append(lines, "IKE proposal: "+ike)
	}
	if esp := firstNonEmpty(stringify(spec["esp"])); esp != "" {
		lines = append(lines, "ESP proposal: "+esp)
	}
	lines = append(lines,
		"",
		"Suggested client profile",
		"- VPN type: L2TP/IPsec PSK",
		"- Use the server address above as both L2TP server and IPsec gateway",
		"- Use the PSK value as the shared secret",
		"- Use the username/password pair above for PPP authentication",
	)

	accessID := record.Access.ID
	filename := sanitizeLocalFilename(firstNonEmpty(record.Client.Username, record.Client.DisplayName, "client")) + "--" + sanitizeLocalFilename(firstNonEmpty(record.Instance.Slug, record.Instance.Name, "ipsec")) + ".ipsec.txt"
	return []generatedArtifactFile{{
		ArtifactType:    "ipsec_bundle",
		ServiceAccessID: &accessID,
		Filename:        filename,
		Content:         []byte(strings.Join(lines, "\n") + "\n"),
	}}, nil
}

func buildZipBundle(clientName string, files []generatedArtifactFile) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	manifest := make([]string, 0, len(files)+2)
	manifest = append(manifest, "MegaVPN client bundle")
	manifest = append(manifest, "Client: "+clientName)

	sort.Slice(files, func(i, j int) bool {
		if files[i].Filename == files[j].Filename {
			return files[i].ArtifactType < files[j].ArtifactType
		}
		return files[i].Filename < files[j].Filename
	})

	for _, file := range files {
		entry, err := zw.Create(file.Filename)
		if err != nil {
			_ = zw.Close()
			return nil, err
		}
		if _, err := entry.Write(file.Content); err != nil {
			_ = zw.Close()
			return nil, err
		}
		manifest = append(manifest, "- "+file.Filename+" ("+file.ArtifactType+")")
	}

	readme, err := zw.Create("README.txt")
	if err != nil {
		_ = zw.Close()
		return nil, err
	}
	if _, err := readme.Write([]byte(strings.Join(manifest, "\n") + "\n")); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func artifactNames(files []generatedArtifactFile) []string {
	out := make([]string, 0, len(files))
	for _, file := range files {
		out = append(out, file.ArtifactType+":"+file.Filename)
	}
	return out
}

func artifactSummaries(artifacts []domain.Artifact) []map[string]any {
	out := make([]map[string]any, 0, len(artifacts))
	for _, artifact := range artifacts {
		out = append(out, map[string]any{
			"id":                artifact.ID,
			"artifact_type":     artifact.ArtifactType,
			"service_access_id": derefString(artifact.ServiceAccessID),
			"storage_path":      artifact.StoragePath,
			"size_bytes":        artifact.SizeBytes,
			"content_hash":      artifact.ContentHash,
		})
	}
	return out
}

func selectedInstanceSet(raw any) map[string]bool {
	list, ok := raw.([]any)
	if !ok || len(list) == 0 {
		return nil
	}
	out := make(map[string]bool, len(list))
	for _, item := range list {
		if value := strings.TrimSpace(stringify(item)); value != "" {
			out[value] = true
		}
	}
	return out
}

func normalizeServiceCode(code string) string {
	return driver.NormalizeCode(code)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func firstInt(values ...any) int {
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

func extraConfigLines(values ...any) []string {
	out := []string{}
	for _, value := range values {
		switch x := value.(type) {
		case string:
			for _, line := range strings.Split(x, "\n") {
				line = strings.TrimSpace(line)
				if line != "" {
					out = append(out, line)
				}
			}
		case []any:
			for _, item := range x {
				if line := strings.TrimSpace(stringify(item)); line != "" {
					out = append(out, line)
				}
			}
		}
	}
	return out
}

func normalizePEMBlock(tag, content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	if strings.Contains(content, "-----BEGIN ") {
		return content + "\n"
	}
	return "<" + tag + ">\n" + content + "\n</" + tag + ">\n"
}

func renderTemplateVariables(template string, record domain.ProvisioningAccess) string {
	replacer := strings.NewReplacer(
		"{{CLIENT_NAME}}", firstNonEmpty(record.Client.Username, record.Client.DisplayName, record.Access.ID),
		"{{CLIENT_USERNAME}}", record.Client.Username,
		"{{INSTANCE_NAME}}", record.Instance.Name,
		"{{INSTANCE_SLUG}}", record.Instance.Slug,
		"{{ENDPOINT_HOST}}", record.Instance.EndpointHost,
		"{{ENDPOINT_PORT}}", strconv.Itoa(record.Instance.EndpointPort),
	)
	return replacer.Replace(template)
}

func sanitizeLocalFilename(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "artifact"
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
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-._")
	if out == "" {
		return "artifact"
	}
	return out
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func resolveOpenVPNText(ctx context.Context, store *postgres.Store, values ...any) (string, error) {
	for idx := 0; idx < len(values); idx++ {
		value := values[idx]
		text := strings.TrimSpace(stringify(value))
		if text == "" {
			continue
		}
		if strings.HasPrefix(text, "-----BEGIN ") || strings.Contains(text, "\n") || strings.HasPrefix(text, "<") {
			return text, nil
		}
		if strings.Contains(strings.ToLower(text), "pem") {
			return text, nil
		}
		_, secretValue, err := store.ResolveSecretValue(ctx, text)
		if err == nil {
			return string(secretValue), nil
		}
		if idx >= 2 {
			return text, nil
		}
	}
	return "", nil
}

func resolveWorkerSecretText(ctx context.Context, store *postgres.Store, values ...any) (string, error) {
	var lastSecretErr error
	for _, value := range values {
		text := strings.TrimSpace(stringify(value))
		if text == "" {
			continue
		}
		_, secretValue, err := store.ResolveSecretValue(ctx, text)
		if err == nil {
			return string(secretValue), nil
		}
		if looksLikeSecretRefID(text) {
			lastSecretErr = err
			continue
		}
		return text, nil
	}
	if lastSecretErr != nil {
		return "", lastSecretErr
	}
	return "", nil
}

func looksLikeSecretRefID(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 36 {
		return false
	}
	for idx, ch := range value {
		if idx == 8 || idx == 13 || idx == 18 || idx == 23 {
			if ch != '-' {
				return false
			}
			continue
		}
		if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')) {
			return false
		}
	}
	return true
}
