package postgres

import (
	"context"
	"strings"

	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/pki"
)

func (s *Store) EnsureOpenVPNInstanceServerPKI(ctx context.Context, instance domain.Instance, spec map[string]any) (map[string]any, error) {
	spec = cloneMap(spec)
	changed := false
	caCertRefID := firstString(spec["ca_cert_secret_ref_id"])
	caKeyRefID := firstString(spec["ca_key_secret_ref_id"])
	pkiScope := strings.ToLower(firstString(spec["pki_scope"], spec["ca_scope"], "platform"))
	pkiProfile := normalizeServicePKIValue(firstString(spec["pki_profile"], spec["ca_profile"], "default"), "default")

	if pkiScope != "instance" {
		root, err := s.EnsureOpenVPNPlatformPKIRoot(ctx, pkiProfile)
		if err != nil {
			return nil, err
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
			changed = true
		}
	}

	if caCertRefID == "" || caKeyRefID == "" {
		caCertPEM, caKeyPEM, err := pki.GenerateCertificateAuthority("MegaVPN OpenVPN CA " + firstString(instance.Name, instance.Slug, instance.ID))
		if err != nil {
			return nil, err
		}
		caCertRef, err := s.CreateSecretRef(ctx, "certificate", caCertPEM, map[string]any{"scope": "instance", "instance_id": instance.ID, "material": "openvpn_ca_cert"})
		if err != nil {
			return nil, err
		}
		caKeyRef, err := s.CreateSecretRef(ctx, "private_key", caKeyPEM, map[string]any{"scope": "instance", "instance_id": instance.ID, "material": "openvpn_ca_key"})
		if err != nil {
			return nil, err
		}
		caCertRefID = caCertRef.ID
		caKeyRefID = caKeyRef.ID
		spec["ca_cert_secret_ref_id"] = caCertRefID
		spec["ca_key_secret_ref_id"] = caKeyRefID
		spec["pki_scope"] = "instance"
		changed = true
	}

	if firstString(spec["server_cert_secret_ref_id"]) == "" || firstString(spec["server_key_secret_ref_id"]) == "" {
		_, caCertPEM, err := s.ResolveSecretValue(ctx, caCertRefID)
		if err != nil {
			return nil, err
		}
		_, caKeyPEM, err := s.ResolveSecretValue(ctx, caKeyRefID)
		if err != nil {
			return nil, err
		}
		serverCN := "server_" + randomToken(8)
		serverCertPEM, serverKeyPEM, err := pki.IssueSignedCertificate(caCertPEM, caKeyPEM, serverCN, true)
		if err != nil {
			return nil, err
		}
		serverCertRef, err := s.CreateSecretRef(ctx, "certificate", serverCertPEM, map[string]any{"scope": "instance", "instance_id": instance.ID, "material": "openvpn_server_cert", "common_name": serverCN})
		if err != nil {
			return nil, err
		}
		serverKeyRef, err := s.CreateSecretRef(ctx, "private_key", serverKeyPEM, map[string]any{"scope": "instance", "instance_id": instance.ID, "material": "openvpn_server_key", "common_name": serverCN})
		if err != nil {
			return nil, err
		}
		spec["server_cert_secret_ref_id"] = serverCertRef.ID
		spec["server_key_secret_ref_id"] = serverKeyRef.ID
		spec["server_common_name"] = serverCN
		changed = true
	}

	if firstString(spec["server_port"]) == "" && instance.EndpointPort > 0 {
		spec["server_port"] = instance.EndpointPort
		changed = true
	}
	if firstString(spec["server_name"]) == "" && instance.EndpointHost != "" {
		spec["server_name"] = instance.EndpointHost
		changed = true
	}
	if changed {
		if _, err := s.ReplaceInstanceSpec(ctx, instance.ID, "worker:openvpn-pki", spec); err != nil {
			return nil, err
		}
	}
	return spec, nil
}
