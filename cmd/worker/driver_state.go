package main

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"fmt"
	"net/netip"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/infra/postgres"
	"github.com/rtis-emc2/megavpn/internal/pki"
)

func ensureXrayInstanceDriverState(ctx context.Context, store *postgres.Store, instanceID string) error {
	instance, err := store.GetInstanceWithSpec(ctx, instanceID)
	if err != nil {
		return err
	}
	spec := cloneMapLocal(instance.Spec)
	changed := false

	if firstNonEmpty(stringify(spec["reality_private_key_secret_ref_id"])) == "" || firstNonEmpty(stringify(spec["reality_public_key"])) == "" {
		privateKey := firstNonEmpty(stringify(spec["reality_private_key"]))
		publicKey := firstNonEmpty(stringify(spec["reality_public_key"]))
		if privateKey == "" || publicKey == "" {
			var genErr error
			privateKey, publicKey, genErr = generateRealityKeyPair()
			if genErr != nil {
				return genErr
			}
		}
		privateKeyRef, err := store.CreateSecretRef(ctx, "private_key", []byte(privateKey), map[string]any{"scope": "instance", "instance_id": instanceID, "material": "xray_reality_private_key"})
		if err != nil {
			return err
		}
		spec["reality_private_key_secret_ref_id"] = privateKeyRef.ID
		spec["reality_public_key"] = publicKey
		delete(spec, "reality_private_key")
		changed = true
	}
	if firstNonEmpty(stringify(spec["short_id"])) == "" && len(stringListLocal(spec["short_ids"])) == 0 {
		spec["short_id"] = randomHexString(4)
		changed = true
	}
	if firstNonEmpty(stringify(spec["server_name"]), stringify(spec["sni"])) == "" && len(stringListLocal(spec["server_names"])) == 0 {
		if host := firstNonEmpty(instance.EndpointHost); host != "" {
			spec["server_name"] = host
			changed = true
		}
	}

	accesses, err := store.ListProvisioningAccessesByInstance(ctx, instanceID)
	if err != nil {
		return err
	}
	managedClients := make([]any, 0, len(accesses))
	for _, access := range accesses {
		uuid := firstNonEmpty(stringify(access.Access.Metadata["xray_uuid"]), stringify(access.Access.Metadata["uuid"]))
		if uuid == "" {
			continue
		}
		client := map[string]any{
			"id":    uuid,
			"email": firstNonEmpty(access.Client.Username, access.Client.DisplayName, access.Access.ID),
		}
		if flow := firstNonEmpty(stringify(access.Access.Policy["flow"]), stringify(access.Access.Metadata["flow"])); flow != "" {
			client["flow"] = flow
		}
		managedClients = append(managedClients, client)
	}
	spec["managed_clients"] = managedClients
	changed = true

	if changed {
		if _, err := store.ReplaceInstanceSpec(ctx, instanceID, "worker:xray-driver", spec); err != nil {
			return err
		}
	}
	return nil
}

func ensureOpenVPNInstanceAndClientState(ctx context.Context, store *postgres.Store, record *domain.ProvisioningAccess) error {
	instance, err := store.GetInstanceWithSpec(ctx, record.Instance.ID)
	if err != nil {
		return err
	}
	spec := cloneMapLocal(instance.Spec)
	specChanged := false

	caCertRefID := firstNonEmpty(stringify(spec["ca_cert_secret_ref_id"]))
	caKeyRefID := firstNonEmpty(stringify(spec["ca_key_secret_ref_id"]))
	pkiScope := strings.ToLower(firstNonEmpty(stringify(spec["pki_scope"]), stringify(spec["ca_scope"]), "platform"))
	pkiProfile := normalizePKIProfile(firstNonEmpty(stringify(spec["pki_profile"]), stringify(spec["ca_profile"]), "default"))
	if pkiScope != "instance" {
		root, err := ensureOpenVPNPlatformPKIRoot(ctx, store, pkiProfile)
		if err != nil {
			return err
		}
		if caCertRefID != root.CACertSecretRefID || caKeyRefID != root.CAKeySecretRefID {
			caCertRefID = root.CACertSecretRefID
			caKeyRefID = root.CAKeySecretRefID
			spec["ca_cert_secret_ref_id"] = caCertRefID
			spec["ca_key_secret_ref_id"] = caKeyRefID
			spec["pki_scope"] = "platform"
			spec["pki_profile"] = root.PKIProfile
			spec["platform_pki_root_id"] = root.ID
			delete(spec, "ca_pem")
			delete(spec, "ca_key_pem")
			delete(spec, "server_cert_secret_ref_id")
			delete(spec, "server_key_secret_ref_id")
			delete(spec, "server_common_name")
			specChanged = true
		}
	}
	serverCertRefID := firstNonEmpty(stringify(spec["server_cert_secret_ref_id"]))
	serverKeyRefID := firstNonEmpty(stringify(spec["server_key_secret_ref_id"]))
	if caCertRefID == "" || caKeyRefID == "" || serverCertRefID == "" || serverKeyRefID == "" {
		if caCertRefID == "" || caKeyRefID == "" {
			caCertPEM, caKeyPEM, err := pki.GenerateCertificateAuthority("MegaVPN OpenVPN CA " + firstNonEmpty(instance.Name, instance.Slug, instance.ID))
			if err != nil {
				return err
			}
			caCertRef, err := store.CreateSecretRef(ctx, "certificate", caCertPEM, map[string]any{"scope": "instance", "instance_id": instance.ID, "material": "openvpn_ca_cert"})
			if err != nil {
				return err
			}
			caKeyRef, err := store.CreateSecretRef(ctx, "private_key", caKeyPEM, map[string]any{"scope": "instance", "instance_id": instance.ID, "material": "openvpn_ca_key"})
			if err != nil {
				return err
			}
			caCertRefID = caCertRef.ID
			caKeyRefID = caKeyRef.ID
			spec["ca_cert_secret_ref_id"] = caCertRefID
			spec["ca_key_secret_ref_id"] = caKeyRefID
			spec["pki_scope"] = "instance"
		}
		_, caCertPEM, err := store.ResolveSecretValue(ctx, caCertRefID)
		if err != nil {
			return err
		}
		_, caKeyPEM, err := store.ResolveSecretValue(ctx, caKeyRefID)
		if err != nil {
			return err
		}
		serverCN := "server_" + randomHexString(8)
		serverCertPEM, serverKeyPEM, err := pki.IssueSignedCertificate(caCertPEM, caKeyPEM, serverCN, true)
		if err != nil {
			return err
		}
		serverCertRef, err := store.CreateSecretRef(ctx, "certificate", serverCertPEM, map[string]any{"scope": "instance", "instance_id": instance.ID, "material": "openvpn_server_cert", "common_name": serverCN})
		if err != nil {
			return err
		}
		serverKeyRef, err := store.CreateSecretRef(ctx, "private_key", serverKeyPEM, map[string]any{"scope": "instance", "instance_id": instance.ID, "material": "openvpn_server_key", "common_name": serverCN})
		if err != nil {
			return err
		}
		spec["server_cert_secret_ref_id"] = serverCertRef.ID
		spec["server_key_secret_ref_id"] = serverKeyRef.ID
		spec["server_common_name"] = serverCN
		if firstNonEmpty(stringify(spec["server_port"])) == "" && instance.EndpointPort > 0 {
			spec["server_port"] = instance.EndpointPort
		}
		if firstNonEmpty(stringify(spec["server_name"])) == "" && instance.EndpointHost != "" {
			spec["server_name"] = instance.EndpointHost
		}
		specChanged = true
	}

	clientMeta := record.Access.Metadata
	if clientMeta == nil {
		clientMeta = map[string]any{}
		record.Access.Metadata = clientMeta
	}
	rotate := truthyLocal(clientMeta["rotate_credentials"]) || truthyLocal(record.Access.Policy["rotate_credentials"])
	clientCertRefID := firstNonEmpty(stringify(clientMeta["openvpn_client_cert_secret_ref_id"]))
	clientKeyRefID := firstNonEmpty(stringify(clientMeta["openvpn_client_key_secret_ref_id"]))
	clientCARefID := firstNonEmpty(stringify(clientMeta["openvpn_ca_cert_secret_ref_id"]))
	currentCARefID := firstNonEmpty(stringify(spec["ca_cert_secret_ref_id"]))
	if clientCertRefID != "" && clientKeyRefID != "" && clientCARefID == "" {
		rotate = true
	}
	if clientCARefID != "" && clientCARefID != currentCARefID {
		rotate = true
	}
	if rotate || clientCertRefID == "" || clientKeyRefID == "" {
		_, caCertValue, err := store.ResolveSecretValue(ctx, firstNonEmpty(stringify(spec["ca_cert_secret_ref_id"])))
		if err != nil {
			return err
		}
		_, caKeyValue, err := store.ResolveSecretValue(ctx, firstNonEmpty(stringify(spec["ca_key_secret_ref_id"])))
		if err != nil {
			return err
		}
		clientCN := sanitizeCommonName(firstNonEmpty(record.Client.Username, record.Client.DisplayName, record.Access.ID))
		clientCertPEM, clientKeyPEM, err := pki.IssueSignedCertificate(caCertValue, caKeyValue, clientCN, false)
		if err != nil {
			return err
		}
		clientCertRef, err := store.CreateSecretRef(ctx, "certificate", clientCertPEM, map[string]any{"scope": "service_access", "service_access_id": record.Access.ID, "material": "openvpn_client_cert", "common_name": clientCN})
		if err != nil {
			return err
		}
		clientKeyRef, err := store.CreateSecretRef(ctx, "private_key", clientKeyPEM, map[string]any{"scope": "service_access", "service_access_id": record.Access.ID, "material": "openvpn_client_key", "common_name": clientCN})
		if err != nil {
			return err
		}
		clientMeta["openvpn_client_cert_secret_ref_id"] = clientCertRef.ID
		clientMeta["openvpn_client_key_secret_ref_id"] = clientKeyRef.ID
		clientMeta["openvpn_client_common_name"] = clientCN
		clientMeta["openvpn_ca_cert_secret_ref_id"] = currentCARefID
		clientMeta["openvpn_pki_scope"] = firstNonEmpty(stringify(spec["pki_scope"]), "platform")
		clientMeta["openvpn_pki_profile"] = firstNonEmpty(stringify(spec["pki_profile"]), "default")
		delete(clientMeta, "rotate_credentials")
		if err := store.UpdateServiceAccessMetadata(ctx, record.Access.ID, clientMeta); err != nil {
			return err
		}
	}

	if specChanged {
		if _, err := store.ReplaceInstanceSpec(ctx, instance.ID, "worker:openvpn-driver", spec); err != nil {
			return err
		}
	}
	record.Instance = instance
	record.Instance.Spec = spec
	return nil
}

func ensureOpenVPNPlatformPKIRoot(ctx context.Context, store *postgres.Store, profile string) (domain.PlatformServicePKIRoot, error) {
	profile = normalizePKIProfile(profile)
	root, err := store.GetActivePlatformServicePKIRoot(ctx, "openvpn", profile)
	if err == nil {
		return root, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.PlatformServicePKIRoot{}, err
	}
	commonName := "MegaVPN OpenVPN Platform CA"
	if profile != "default" {
		commonName += " " + profile
	}
	caCertPEM, caKeyPEM, err := pki.GenerateCertificateAuthority(commonName)
	if err != nil {
		return domain.PlatformServicePKIRoot{}, err
	}
	caCertRef, err := store.CreateSecretRef(ctx, "certificate", caCertPEM, map[string]any{"scope": "platform", "service_code": "openvpn", "pki_profile": profile, "material": "openvpn_ca_cert"})
	if err != nil {
		return domain.PlatformServicePKIRoot{}, err
	}
	caKeyRef, err := store.CreateSecretRef(ctx, "private_key", caKeyPEM, map[string]any{"scope": "platform", "service_code": "openvpn", "pki_profile": profile, "material": "openvpn_ca_key"})
	if err != nil {
		return domain.PlatformServicePKIRoot{}, err
	}
	root, err = store.CreatePlatformServicePKIRoot(ctx, "openvpn", profile, commonName, caCertRef.ID, caKeyRef.ID)
	if err == nil {
		return root, nil
	}
	refetched, refetchErr := store.GetActivePlatformServicePKIRoot(ctx, "openvpn", profile)
	if refetchErr == nil {
		return refetched, nil
	}
	return domain.PlatformServicePKIRoot{}, err
}

func ensureShadowsocksInstanceDriverState(ctx context.Context, store *postgres.Store, instanceID string) error {
	instance, err := store.GetInstanceWithSpec(ctx, instanceID)
	if err != nil {
		return err
	}
	spec := cloneMapLocal(instance.Spec)
	changed := false

	if firstNonEmpty(stringify(spec["listen"]), stringify(spec["server"])) == "" {
		spec["listen"] = "0.0.0.0"
		changed = true
	}
	if firstNonEmpty(stringify(spec["method"])) == "" {
		spec["method"] = "chacha20-ietf-poly1305"
		changed = true
	}
	if firstNonEmpty(stringify(spec["mode"])) == "" {
		spec["mode"] = "tcp_and_udp"
		changed = true
	}
	if firstInt(spec["timeout"]) <= 0 {
		spec["timeout"] = 300
		changed = true
	}

	accesses, err := store.ListProvisioningAccessesByInstance(ctx, instanceID)
	if err != nil {
		return err
	}
	if len(accesses) == 0 {
		if changed {
			if _, err := store.ReplaceInstanceSpec(ctx, instanceID, "worker:shadowsocks-driver", spec); err != nil {
				return err
			}
		}
		return nil
	}

	basePort := firstInt(spec["access_port_base"], spec["server_port"], instance.EndpointPort)
	if basePort <= 0 {
		basePort = 8388
		spec["access_port_base"] = basePort
		changed = true
	}

	usedPorts := map[int]bool{}
	for _, access := range accesses {
		if port := firstInt(access.Access.Metadata["server_port"], access.Access.Metadata["port"]); port > 0 {
			usedPorts[port] = true
		}
	}

	managedAccounts := make([]any, 0, len(accesses))
	nextPort := basePort
	for _, access := range accesses {
		meta := access.Access.Metadata
		if meta == nil {
			meta = map[string]any{}
		}
		metaChanged := false
		rotate := truthyLocal(meta["rotate_credentials"]) || truthyLocal(access.Access.Policy["rotate_credentials"])

		password := firstNonEmpty(stringify(meta["password"]), stringify(meta["shadowsocks_password"]))
		if rotate || password == "" {
			password = randomHexString(16)
			meta["password"] = password
			meta["shadowsocks_password"] = password
			delete(meta, "rotate_credentials")
			metaChanged = true
		}

		port := firstInt(meta["server_port"], meta["port"])
		if port <= 0 {
			port = nextAvailablePort(usedPorts, nextPort)
			meta["server_port"] = port
			meta["port"] = port
			metaChanged = true
		}
		usedPorts[port] = true
		if port >= nextPort {
			nextPort = port + 1
		}

		if host := firstNonEmpty(instance.EndpointHost); host != "" && firstNonEmpty(stringify(meta["server_host"])) == "" {
			meta["server_host"] = host
			metaChanged = true
		}

		if metaChanged {
			if err := store.UpdateServiceAccessMetadata(ctx, access.Access.ID, meta); err != nil {
				return err
			}
		}

		managedAccounts = append(managedAccounts, map[string]any{
			"service_access_id": access.Access.ID,
			"username":          firstNonEmpty(access.Client.Username, access.Client.DisplayName, access.Access.ID),
			"server_port":       port,
			"password":          password,
		})
	}

	spec["managed_accounts"] = managedAccounts
	changed = true
	if _, err := store.ReplaceInstanceSpec(ctx, instanceID, "worker:shadowsocks-driver", spec); err != nil {
		return err
	}
	return nil
}

func ensureHTTPProxyInstanceDriverState(ctx context.Context, store *postgres.Store, instanceID string) error {
	instance, err := store.GetInstanceWithSpec(ctx, instanceID)
	if err != nil {
		return err
	}
	spec := cloneMapLocal(instance.Spec)
	changed := false

	if firstInt(spec["listen_port"], spec["server_port"]) <= 0 {
		spec["listen_port"] = firstInt(instance.EndpointPort, 3128)
		changed = true
	}
	if firstNonEmpty(stringify(spec["auth_realm"])) == "" {
		spec["auth_realm"] = "RTIS MegaVPN HTTP Proxy"
		changed = true
	}
	if firstNonEmpty(stringify(spec["auth_helper_path"])) == "" {
		spec["auth_helper_path"] = "/usr/lib/squid/basic_ncsa_auth"
		changed = true
	}
	if firstNonEmpty(stringify(spec["visible_hostname"])) == "" && instance.EndpointHost != "" {
		spec["visible_hostname"] = instance.EndpointHost
		changed = true
	}

	accesses, err := store.ListProvisioningAccessesByInstance(ctx, instanceID)
	if err != nil {
		return err
	}
	if len(accesses) == 0 {
		if changed {
			if _, err := store.ReplaceInstanceSpec(ctx, instanceID, "worker:http-proxy-driver", spec); err != nil {
				return err
			}
		}
		return nil
	}

	managedAccounts := make([]any, 0, len(accesses))
	for _, access := range accesses {
		meta := access.Access.Metadata
		if meta == nil {
			meta = map[string]any{}
		}
		metaChanged := false
		rotate := truthyLocal(meta["rotate_credentials"]) || truthyLocal(access.Access.Policy["rotate_credentials"])

		username := firstNonEmpty(stringify(meta["username"]), stringify(meta["proxy_username"]), stringify(meta["http_proxy_username"]))
		if rotate || username == "" {
			username = sanitizeCommonName(firstNonEmpty(access.Client.Username, access.Client.DisplayName, access.Access.ID))
			meta["username"] = username
			meta["proxy_username"] = username
			meta["http_proxy_username"] = username
			metaChanged = true
		}

		password := firstNonEmpty(stringify(meta["password"]), stringify(meta["proxy_password"]), stringify(meta["http_proxy_password"]))
		if rotate || password == "" {
			password = randomHexString(12)
			meta["password"] = password
			meta["proxy_password"] = password
			meta["http_proxy_password"] = password
			metaChanged = true
		}

		if host := firstNonEmpty(instance.EndpointHost); host != "" && firstNonEmpty(stringify(meta["server_host"])) == "" {
			meta["server_host"] = host
			metaChanged = true
		}
		if firstInt(meta["server_port"]) <= 0 {
			port := firstInt(spec["listen_port"], spec["server_port"], instance.EndpointPort, 3128)
			meta["server_port"] = port
			metaChanged = true
		}

		delete(meta, "rotate_credentials")
		if metaChanged {
			if err := store.UpdateServiceAccessMetadata(ctx, access.Access.ID, meta); err != nil {
				return err
			}
		}

		managedAccounts = append(managedAccounts, map[string]any{
			"service_access_id": access.Access.ID,
			"username":          username,
			"password_hash":     httpProxyPasswordHash(password),
		})
	}
	spec["managed_accounts"] = managedAccounts
	changed = true
	if changed {
		if _, err := store.ReplaceInstanceSpec(ctx, instanceID, "worker:http-proxy-driver", spec); err != nil {
			return err
		}
	}
	return nil
}

func ensureMTProtoInstanceDriverState(ctx context.Context, store *postgres.Store, instanceID string) error {
	instance, err := store.GetInstanceWithSpec(ctx, instanceID)
	if err != nil {
		return err
	}
	spec := cloneMapLocal(instance.Spec)
	changed := false

	if firstInt(spec["server_port"], spec["listen_port"]) <= 0 {
		spec["server_port"] = firstInt(instance.EndpointPort, 443)
		changed = true
	}
	if firstNonEmpty(stringify(spec["listen"])) == "" {
		spec["listen"] = "0.0.0.0"
		changed = true
	}

	accesses, err := store.ListProvisioningAccessesByInstance(ctx, instanceID)
	if err != nil {
		return err
	}
	if len(accesses) == 0 {
		if changed {
			if _, err := store.ReplaceInstanceSpec(ctx, instanceID, "worker:mtproto-driver", spec); err != nil {
				return err
			}
		}
		return nil
	}

	managedUsers := make([]any, 0, len(accesses))
	for _, access := range accesses {
		meta := access.Access.Metadata
		if meta == nil {
			meta = map[string]any{}
		}
		metaChanged := false
		rotate := truthyLocal(meta["rotate_credentials"]) || truthyLocal(access.Access.Policy["rotate_credentials"])

		secret := firstNonEmpty(stringify(meta["mtproto_secret"]), stringify(meta["secret"]))
		if rotate || secret == "" {
			secret = randomHexString(16)
			meta["mtproto_secret"] = secret
			meta["secret"] = secret
			metaChanged = true
		}
		if host := firstNonEmpty(instance.EndpointHost); host != "" && firstNonEmpty(stringify(meta["server_host"])) == "" {
			meta["server_host"] = host
			metaChanged = true
		}
		if firstInt(meta["server_port"]) <= 0 {
			meta["server_port"] = firstInt(spec["server_port"], instance.EndpointPort, 443)
			metaChanged = true
		}

		delete(meta, "rotate_credentials")
		if metaChanged {
			if err := store.UpdateServiceAccessMetadata(ctx, access.Access.ID, meta); err != nil {
				return err
			}
		}
		managedUsers = append(managedUsers, map[string]any{
			"service_access_id": access.Access.ID,
			"username":          firstNonEmpty(access.Client.Username, access.Client.DisplayName, access.Access.ID),
			"secret":            secret,
		})
	}

	spec["managed_users"] = managedUsers
	changed = true
	if changed {
		if _, err := store.ReplaceInstanceSpec(ctx, instanceID, "worker:mtproto-driver", spec); err != nil {
			return err
		}
	}
	return nil
}

func ensureWireGuardInstanceDriverState(ctx context.Context, store *postgres.Store, instanceID string) error {
	instance, err := store.GetInstanceWithSpec(ctx, instanceID)
	if err != nil {
		return err
	}
	spec := cloneMapLocal(instance.Spec)
	changed := false

	networkCIDR := firstNonEmpty(stringify(spec["network_cidr"]), "10.66.0.0/24")
	if firstNonEmpty(stringify(spec["network_cidr"])) == "" {
		spec["network_cidr"] = networkCIDR
		changed = true
	}
	prefix, err := netip.ParsePrefix(networkCIDR)
	if err != nil {
		return fmt.Errorf("wireguard network_cidr is invalid: %w", err)
	}
	if !prefix.Addr().Is4() {
		return fmt.Errorf("wireguard network_cidr must be IPv4 for the current driver")
	}

	if firstNonEmpty(stringify(spec["server_private_key_secret_ref_id"])) == "" || firstNonEmpty(stringify(spec["server_public_key"])) == "" {
		privateKey := firstNonEmpty(stringify(spec["server_private_key"]))
		publicKey := firstNonEmpty(stringify(spec["server_public_key"]))
		if privateKey == "" || publicKey == "" {
			var genErr error
			privateKey, publicKey, genErr = generateWireGuardKeyPair()
			if genErr != nil {
				return genErr
			}
		}
		privateKeyRef, err := store.CreateSecretRef(ctx, "private_key", []byte(privateKey), map[string]any{"scope": "instance", "instance_id": instanceID, "material": "wireguard_server_private_key"})
		if err != nil {
			return err
		}
		spec["server_private_key_secret_ref_id"] = privateKeyRef.ID
		spec["server_public_key"] = publicKey
		delete(spec, "server_private_key")
		changed = true
	}
	if firstNonEmpty(stringify(spec["server_address"])) == "" {
		serverAddress, err := wireGuardHostCIDR(prefix, firstInt(spec["server_host_index"], 1))
		if err != nil {
			return err
		}
		spec["server_address"] = serverAddress
		changed = true
	}
	if firstInt(spec["listen_port"], spec["server_port"]) <= 0 {
		spec["listen_port"] = firstInt(instance.EndpointPort, 51820)
		changed = true
	}
	if firstInt(spec["client_address_start"]) <= 0 {
		spec["client_address_start"] = 10
		changed = true
	}
	if firstNonEmpty(stringify(spec["client_allowed_ips"])) == "" {
		spec["client_allowed_ips"] = "0.0.0.0/0, ::/0"
		changed = true
	}
	if firstInt(spec["persistent_keepalive"]) <= 0 {
		spec["persistent_keepalive"] = 25
		changed = true
	}

	accesses, err := store.ListProvisioningAccessesByInstance(ctx, instanceID)
	if err != nil {
		return err
	}
	usedAddresses := map[string]bool{}
	if serverIP := wireGuardBareIP(firstNonEmpty(stringify(spec["server_address"]))); serverIP != "" {
		usedAddresses[serverIP] = true
	}
	for _, access := range accesses {
		if clientIP := wireGuardBareIP(firstNonEmpty(stringify(access.Access.Metadata["wireguard_client_address"]))); clientIP != "" {
			usedAddresses[clientIP] = true
		}
	}

	managedPeers := make([]any, 0, len(accesses))
	nextHost := firstInt(spec["client_address_start"], 10)
	serverPublicKey := firstNonEmpty(stringify(spec["server_public_key"]))
	serverHost := firstNonEmpty(instance.EndpointHost)
	serverPort := firstInt(spec["listen_port"], spec["server_port"], instance.EndpointPort, 51820)

	for _, access := range accesses {
		meta := access.Access.Metadata
		if meta == nil {
			meta = map[string]any{}
		}
		metaChanged := false
		rotate := truthyLocal(meta["rotate_credentials"]) || truthyLocal(access.Access.Policy["rotate_credentials"])

		clientPrivateKeyRefID := firstNonEmpty(stringify(meta["wireguard_client_private_key_secret_ref_id"]))
		clientPublicKey := firstNonEmpty(stringify(meta["wireguard_client_public_key"]))
		if rotate || clientPrivateKeyRefID == "" || clientPublicKey == "" {
			privateKey, publicKey, err := generateWireGuardKeyPair()
			if err != nil {
				return err
			}
			privateKeyRef, err := store.CreateSecretRef(ctx, "private_key", []byte(privateKey), map[string]any{"scope": "service_access", "service_access_id": access.Access.ID, "material": "wireguard_client_private_key"})
			if err != nil {
				return err
			}
			meta["wireguard_client_private_key_secret_ref_id"] = privateKeyRef.ID
			meta["wireguard_client_public_key"] = publicKey
			clientPrivateKeyRefID = privateKeyRef.ID
			clientPublicKey = publicKey
			metaChanged = true
		}

		clientAddress := firstNonEmpty(stringify(meta["wireguard_client_address"]))
		if rotate || clientAddress == "" || !wireGuardAddressInPrefix(prefix, clientAddress) {
			address, nextValue, err := nextWireGuardClientAddress(prefix, usedAddresses, nextHost)
			if err != nil {
				return err
			}
			nextHost = nextValue
			meta["wireguard_client_address"] = address
			clientAddress = address
			metaChanged = true
		}
		if clientIP := wireGuardBareIP(clientAddress); clientIP != "" {
			usedAddresses[clientIP] = true
		}

		if serverHost != "" && firstNonEmpty(stringify(meta["server_host"])) == "" {
			meta["server_host"] = serverHost
			metaChanged = true
		}
		if firstInt(meta["server_port"]) <= 0 && serverPort > 0 {
			meta["server_port"] = serverPort
			metaChanged = true
		}
		if serverPublicKey != "" && firstNonEmpty(stringify(meta["wireguard_server_public_key"])) != serverPublicKey {
			meta["wireguard_server_public_key"] = serverPublicKey
			metaChanged = true
		}
		if firstNonEmpty(stringify(meta["allowed_ips"])) == "" {
			allowedIPs := firstNonEmpty(stringify(access.Access.Policy["allowed_ips"]), stringify(spec["client_allowed_ips"]), "0.0.0.0/0, ::/0")
			meta["allowed_ips"] = allowedIPs
			metaChanged = true
		}
		if firstNonEmpty(stringify(meta["dns_servers"])) == "" {
			if dns := firstNonEmpty(stringify(access.Access.Policy["dns_servers"]), stringify(spec["client_dns"])); dns != "" {
				meta["dns_servers"] = dns
				metaChanged = true
			}
		}
		if firstInt(meta["persistent_keepalive"]) <= 0 {
			keepalive := firstInt(access.Access.Policy["persistent_keepalive"], spec["persistent_keepalive"], 25)
			meta["persistent_keepalive"] = keepalive
			metaChanged = true
		}

		delete(meta, "rotate_credentials")
		if metaChanged {
			if err := store.UpdateServiceAccessMetadata(ctx, access.Access.ID, meta); err != nil {
				return err
			}
		}

		managedPeers = append(managedPeers, map[string]any{
			"service_access_id": access.Access.ID,
			"username":          firstNonEmpty(access.Client.Username, access.Client.DisplayName, access.Access.ID),
			"public_key":        clientPublicKey,
			"allowed_ips":       clientAddress,
		})
	}

	spec["managed_peers"] = managedPeers
	changed = true
	if changed {
		if _, err := store.ReplaceInstanceSpec(ctx, instanceID, "worker:wireguard-driver", spec); err != nil {
			return err
		}
	}
	return nil
}

func ensureIPSecL2TPInstanceDriverState(ctx context.Context, store *postgres.Store, record *domain.ProvisioningAccess) (map[string]string, error) {
	instance, err := store.GetInstanceWithSpec(ctx, record.Instance.ID)
	if err != nil {
		return nil, err
	}
	spec := cloneMapLocal(instance.Spec)
	specChanged := false

	xl2tpdInstance, err := store.FindNodeInstanceByService(ctx, instance.NodeID, "xl2tpd")
	if err != nil {
		return nil, fmt.Errorf("xl2tpd companion instance is required on node %s for ipsec_l2tp provisioning: %w", instance.NodeID, err)
	}
	xl2tpdInstance, err = store.GetInstanceWithSpec(ctx, xl2tpdInstance.ID)
	if err != nil {
		return nil, err
	}
	xl2tpdSpec := cloneMapLocal(xl2tpdInstance.Spec)
	xl2tpdChanged := false

	if firstNonEmpty(stringify(spec["psk_secret_ref_id"]), stringify(spec["psk"])) == "" {
		psk := randomHexString(16)
		pskRef, err := store.CreateSecretRef(ctx, "psk", []byte(psk), map[string]any{"scope": "instance", "instance_id": instance.ID, "material": "ipsec_psk"})
		if err != nil {
			return nil, err
		}
		spec["psk_secret_ref_id"] = pskRef.ID
		delete(spec, "psk")
		specChanged = true
	}
	if firstNonEmpty(stringify(spec["server_id"]), stringify(spec["leftid"])) == "" && instance.EndpointHost != "" {
		spec["server_id"] = instance.EndpointHost
		specChanged = true
	}
	if firstNonEmpty(stringify(spec["left"])) == "" {
		spec["left"] = "%defaultroute"
		specChanged = true
	}
	if firstNonEmpty(stringify(spec["right"])) == "" {
		spec["right"] = "%any"
		specChanged = true
	}

	accesses, err := store.ListProvisioningAccessesByInstance(ctx, instance.ID)
	if err != nil {
		return nil, err
	}
	if len(accesses) == 0 {
		if specChanged {
			if _, err := store.ReplaceInstanceSpec(ctx, instance.ID, "worker:ipsec-driver", spec); err != nil {
				return nil, err
			}
		}
		if xl2tpdChanged {
			if _, err := store.ReplaceInstanceSpec(ctx, xl2tpdInstance.ID, "worker:xl2tpd-driver", xl2tpdSpec); err != nil {
				return nil, err
			}
		}
		return map[string]string{
			instance.ID:       "ipsec",
			xl2tpdInstance.ID: "xl2tpd",
		}, nil
	}

	managedAccounts := make([]any, 0, len(accesses))
	chapSecretLines := make([]string, 0, len(accesses))
	defaultL2TPPort := firstInt(xl2tpdSpec["listen_port"], xl2tpdSpec["server_port"], xl2tpdInstance.EndpointPort, 1701)
	serverHost := firstNonEmpty(instance.EndpointHost, xl2tpdInstance.EndpointHost)

	for _, access := range accesses {
		meta := access.Access.Metadata
		if meta == nil {
			meta = map[string]any{}
		}
		metaChanged := false
		rotate := truthyLocal(meta["rotate_credentials"]) || truthyLocal(access.Access.Policy["rotate_credentials"])

		username := firstNonEmpty(stringify(meta["username"]), stringify(meta["l2tp_username"]), stringify(meta["ppp_username"]))
		if rotate || username == "" {
			username = sanitizeCommonName(firstNonEmpty(access.Client.Username, access.Client.DisplayName, access.Access.ID))
			meta["username"] = username
			meta["l2tp_username"] = username
			meta["ppp_username"] = username
			metaChanged = true
		}

		password := firstNonEmpty(stringify(meta["password"]), stringify(meta["l2tp_password"]), stringify(meta["ppp_password"]))
		if rotate || password == "" {
			password = randomHexString(12)
			meta["password"] = password
			meta["l2tp_password"] = password
			meta["ppp_password"] = password
			metaChanged = true
		}

		if serverHost != "" && firstNonEmpty(stringify(meta["server_host"])) == "" {
			meta["server_host"] = serverHost
			metaChanged = true
		}
		if firstInt(meta["server_port"], meta["l2tp_port"]) <= 0 && defaultL2TPPort > 0 {
			meta["server_port"] = defaultL2TPPort
			meta["l2tp_port"] = defaultL2TPPort
			metaChanged = true
		}
		if pskRefID := firstNonEmpty(stringify(spec["psk_secret_ref_id"])); pskRefID != "" && firstNonEmpty(stringify(meta["ipsec_psk_secret_ref_id"])) == "" {
			meta["ipsec_psk_secret_ref_id"] = pskRefID
			metaChanged = true
		}

		delete(meta, "rotate_credentials")
		if metaChanged {
			if err := store.UpdateServiceAccessMetadata(ctx, access.Access.ID, meta); err != nil {
				return nil, err
			}
		}
		if access.Access.ID == record.Access.ID {
			record.Access.Metadata = meta
		}
		managedAccounts = append(managedAccounts, map[string]any{
			"service_access_id": access.Access.ID,
			"username":          username,
			"password":          password,
			"server_host":       firstNonEmpty(stringify(meta["server_host"]), serverHost),
			"server_port":       firstInt(meta["server_port"], meta["l2tp_port"], defaultL2TPPort),
		})
		chapSecretLines = append(chapSecretLines, username+` l2tpd `+password+` *`)
	}

	spec["managed_accounts"] = managedAccounts
	specChanged = true
	xl2tpdSpec["managed_accounts"] = managedAccounts
	xl2tpdSpec["chap_secrets_entries"] = strings.Join(chapSecretLines, "\n")
	xl2tpdChanged = true
	if len(chapSecretLines) > 0 {
		first := managedAccounts[0].(map[string]any)
		if firstNonEmpty(stringify(xl2tpdSpec["default_username"])) == "" {
			xl2tpdSpec["default_username"] = stringify(first["username"])
			xl2tpdChanged = true
		}
		if firstNonEmpty(stringify(xl2tpdSpec["default_password"])) == "" {
			xl2tpdSpec["default_password"] = stringify(first["password"])
			xl2tpdChanged = true
		}
	}

	if specChanged {
		if _, err := store.ReplaceInstanceSpec(ctx, instance.ID, "worker:ipsec-driver", spec); err != nil {
			return nil, err
		}
	}
	if xl2tpdChanged {
		if _, err := store.ReplaceInstanceSpec(ctx, xl2tpdInstance.ID, "worker:xl2tpd-driver", xl2tpdSpec); err != nil {
			return nil, err
		}
	}

	record.Instance = instance
	record.Instance.Spec = spec
	return map[string]string{
		instance.ID:       "ipsec",
		xl2tpdInstance.ID: "xl2tpd",
	}, nil
}

func generateRealityKeyPair() (string, string, error) {
	curve := ecdh.X25519()
	privateKey, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	publicKey := privateKey.PublicKey()
	return base64.RawURLEncoding.EncodeToString(privateKey.Bytes()), base64.RawURLEncoding.EncodeToString(publicKey.Bytes()), nil
}

func generateWireGuardKeyPair() (string, string, error) {
	curve := ecdh.X25519()
	privateKey, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	publicKey := privateKey.PublicKey()
	return base64.StdEncoding.EncodeToString(privateKey.Bytes()), base64.StdEncoding.EncodeToString(publicKey.Bytes()), nil
}

func randomHexString(n int) string {
	if n <= 0 {
		n = 8
	}
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return strings.Repeat("0", n*2)
	}
	return fmt.Sprintf("%x", buf)
}

func sanitizeCommonName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "client"
	}
	var out strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			out.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			out.WriteRune(r)
		case r >= '0' && r <= '9':
			out.WriteRune(r)
		case r == '.', r == '-', r == '_':
			out.WriteRune(r)
		default:
			out.WriteByte('_')
		}
	}
	text := strings.Trim(out.String(), "._-")
	if text == "" {
		return "client"
	}
	return text
}

func normalizePKIProfile(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "default"
	}
	var out strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			out.WriteRune(r)
		case r >= '0' && r <= '9':
			out.WriteRune(r)
		case r == '.', r == '-', r == '_':
			out.WriteRune(r)
		default:
			out.WriteByte('-')
		}
	}
	text := strings.Trim(out.String(), "._-")
	if text == "" {
		return "default"
	}
	return text
}

func cloneMapLocal(src map[string]any) map[string]any {
	if src == nil {
		return map[string]any{}
	}
	dst := make(map[string]any, len(src))
	for key, value := range src {
		switch x := value.(type) {
		case map[string]any:
			dst[key] = cloneMapLocal(x)
		case []any:
			items := make([]any, len(x))
			for idx := range x {
				items[idx] = x[idx]
			}
			dst[key] = items
		default:
			dst[key] = x
		}
	}
	return dst
}

func stringListLocal(raw any) []string {
	switch x := raw.(type) {
	case []any:
		out := make([]string, 0, len(x))
		for _, item := range x {
			if text := strings.TrimSpace(stringify(item)); text != "" {
				out = append(out, text)
			}
		}
		return out
	case string:
		if text := strings.TrimSpace(x); text != "" {
			return []string{text}
		}
	}
	return nil
}

func truthyLocal(raw any) bool {
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

func nextAvailablePort(used map[int]bool, start int) int {
	if start <= 0 {
		start = 8388
	}
	port := start
	for used[port] {
		port++
	}
	return port
}

func httpProxyPasswordHash(password string) string {
	sum := sha1.Sum([]byte(password))
	return "{SHA}" + base64.StdEncoding.EncodeToString(sum[:])
}

func wireGuardHostCIDR(prefix netip.Prefix, host int) (string, error) {
	addr, err := wireGuardHostAddr(prefix, host)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/%d", addr.String(), prefix.Bits()), nil
}

func nextWireGuardClientAddress(prefix netip.Prefix, used map[string]bool, startHost int) (string, int, error) {
	if startHost <= 0 {
		startHost = 10
	}
	totalHosts := 1 << uint(32-prefix.Bits())
	maxHost := totalHosts - 1
	if prefix.Bits() < 31 {
		maxHost = totalHosts - 2
	}
	for host := startHost; host <= maxHost; host++ {
		addr, err := wireGuardHostAddr(prefix, host)
		if err != nil {
			return "", startHost, err
		}
		if used[addr.String()] {
			continue
		}
		return addr.String() + "/32", host + 1, nil
	}
	return "", startHost, fmt.Errorf("wireguard address pool %s is exhausted", prefix.String())
}

func wireGuardHostAddr(prefix netip.Prefix, host int) (netip.Addr, error) {
	if !prefix.Addr().Is4() {
		return netip.Addr{}, fmt.Errorf("wireguard address allocation supports only IPv4 prefixes")
	}
	base := prefix.Masked().Addr().As4()
	baseValue := uint32(base[0])<<24 | uint32(base[1])<<16 | uint32(base[2])<<8 | uint32(base[3])
	addrValue := baseValue + uint32(host)
	addr := netip.AddrFrom4([4]byte{
		byte(addrValue >> 24),
		byte(addrValue >> 16),
		byte(addrValue >> 8),
		byte(addrValue),
	})
	if !prefix.Contains(addr) {
		return netip.Addr{}, fmt.Errorf("address %s is outside of prefix %s", addr.String(), prefix.String())
	}
	return addr, nil
}

func wireGuardBareIP(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if prefix, err := netip.ParsePrefix(value); err == nil {
		return prefix.Addr().String()
	}
	if addr, err := netip.ParseAddr(value); err == nil {
		return addr.String()
	}
	return ""
}

func wireGuardAddressInPrefix(prefix netip.Prefix, value string) bool {
	bare := wireGuardBareIP(value)
	if bare == "" {
		return false
	}
	addr, err := netip.ParseAddr(bare)
	if err != nil {
		return false
	}
	return prefix.Contains(addr)
}
