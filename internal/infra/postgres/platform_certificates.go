package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/pki"
	"github.com/rtis-emc2/megavpn/internal/platform/id"
)

func (s *Store) ListPlatformCertificates(ctx context.Context) ([]domain.PlatformCertificate, error) {
	rows, err := s.db.Query(ctx, `select
		id,
		name,
		description,
		source,
		kind,
		status,
		common_name,
		san_json,
		issuer_name,
		parent_certificate_id,
		cert_secret_ref_id,
		key_secret_ref_id,
		chain_secret_ref_id,
		not_before,
		not_after,
		is_default,
		meta_json,
		created_at,
		updated_at
	from platform_certificates
	order by is_default desc, kind asc, created_at desc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []domain.PlatformCertificate{}
	for rows.Next() {
		var item domain.PlatformCertificate
		var sansRaw, metaRaw []byte
		if err := rows.Scan(
			&item.ID,
			&item.Name,
			&item.Description,
			&item.Source,
			&item.Kind,
			&item.Status,
			&item.CommonName,
			&sansRaw,
			&item.IssuerName,
			&item.ParentCertificateID,
			&item.CertSecretRefID,
			&item.KeySecretRefID,
			&item.ChainSecretRefID,
			&item.NotBefore,
			&item.NotAfter,
			&item.IsDefault,
			&metaRaw,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(sansRaw, &item.SANs)
		_ = json.Unmarshal(metaRaw, &item.Meta)
		if item.SANs == nil {
			item.SANs = []string{}
		}
		if item.Meta == nil {
			item.Meta = map[string]any{}
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) GetPlatformCertificate(ctx context.Context, certificateID string) (domain.PlatformCertificate, error) {
	var item domain.PlatformCertificate
	var sansRaw, metaRaw []byte
	err := s.db.QueryRow(ctx, `select
		id,
		name,
		description,
		source,
		kind,
		status,
		common_name,
		san_json,
		issuer_name,
		parent_certificate_id,
		cert_secret_ref_id,
		key_secret_ref_id,
		chain_secret_ref_id,
		not_before,
		not_after,
		is_default,
		meta_json,
		created_at,
		updated_at
	from platform_certificates
	where id=$1`, strings.TrimSpace(certificateID)).
		Scan(
			&item.ID,
			&item.Name,
			&item.Description,
			&item.Source,
			&item.Kind,
			&item.Status,
			&item.CommonName,
			&sansRaw,
			&item.IssuerName,
			&item.ParentCertificateID,
			&item.CertSecretRefID,
			&item.KeySecretRefID,
			&item.ChainSecretRefID,
			&item.NotBefore,
			&item.NotAfter,
			&item.IsDefault,
			&metaRaw,
			&item.CreatedAt,
			&item.UpdatedAt,
		)
	if err != nil {
		return domain.PlatformCertificate{}, err
	}
	_ = json.Unmarshal(sansRaw, &item.SANs)
	_ = json.Unmarshal(metaRaw, &item.Meta)
	if item.SANs == nil {
		item.SANs = []string{}
	}
	if item.Meta == nil {
		item.Meta = map[string]any{}
	}
	return item, nil
}

func (s *Store) ImportPlatformCertificate(ctx context.Context, name, description string, certPEM, keyPEM, chainPEM []byte, isDefault bool) (domain.PlatformCertificate, error) {
	desc, err := pki.DescribeCertificatePEM(certPEM)
	if err != nil {
		return domain.PlatformCertificate{}, err
	}
	if desc.IsCA {
		return domain.PlatformCertificate{}, errors.New("imported leaf certificate must not be a CA")
	}
	if len(keyPEM) == 0 {
		return domain.PlatformCertificate{}, errors.New("private key is required for imported certificate")
	}
	certRef, err := s.CreateSecretRef(ctx, "certificate", certPEM, map[string]any{"scope": "platform_certificate", "material": "certificate"})
	if err != nil {
		return domain.PlatformCertificate{}, err
	}
	keyRef, err := s.CreateSecretRef(ctx, "private_key", keyPEM, map[string]any{"scope": "platform_certificate", "material": "private_key"})
	if err != nil {
		return domain.PlatformCertificate{}, err
	}
	var chainRefID *string
	if len(chainPEM) > 0 {
		chainRef, err := s.CreateSecretRef(ctx, "certificate", chainPEM, map[string]any{"scope": "platform_certificate", "material": "certificate_chain"})
		if err != nil {
			return domain.PlatformCertificate{}, err
		}
		chainRefID = &chainRef.ID
	}
	keyRefID := keyRef.ID
	return s.insertPlatformCertificate(ctx, domain.PlatformCertificate{
		ID:               id.New(),
		Name:             firstString(name, desc.CommonName, "imported-certificate"),
		Description:      strings.TrimSpace(description),
		Source:           "imported",
		Kind:             "leaf",
		Status:           "active",
		CommonName:       desc.CommonName,
		SANs:             append([]string(nil), desc.DNSNames...),
		IssuerName:       desc.IssuerName,
		CertSecretRefID:  certRef.ID,
		KeySecretRefID:   &keyRefID,
		ChainSecretRefID: chainRefID,
		NotBefore:        &desc.NotBefore,
		NotAfter:         &desc.NotAfter,
		IsDefault:        isDefault,
		Meta:             map[string]any{"provider": "manual_import"},
	})
}

func (s *Store) CreateSelfSignedPlatformCertificate(ctx context.Context, name, description, commonName string, dnsNames []string, validDays int, isDefault bool) (domain.PlatformCertificate, error) {
	certPEM, keyPEM, err := pki.GenerateSelfSignedCertificate(commonName, dnsNames, validDays)
	if err != nil {
		return domain.PlatformCertificate{}, err
	}
	desc, err := pki.DescribeCertificatePEM(certPEM)
	if err != nil {
		return domain.PlatformCertificate{}, err
	}
	certRef, err := s.CreateSecretRef(ctx, "certificate", certPEM, map[string]any{"scope": "platform_certificate", "material": "self_signed_certificate"})
	if err != nil {
		return domain.PlatformCertificate{}, err
	}
	keyRef, err := s.CreateSecretRef(ctx, "private_key", keyPEM, map[string]any{"scope": "platform_certificate", "material": "self_signed_private_key"})
	if err != nil {
		return domain.PlatformCertificate{}, err
	}
	keyRefID := keyRef.ID
	return s.insertPlatformCertificate(ctx, domain.PlatformCertificate{
		ID:              id.New(),
		Name:            firstString(name, commonName, "self-signed-certificate"),
		Description:     strings.TrimSpace(description),
		Source:          "self_signed",
		Kind:            "leaf",
		Status:          "active",
		CommonName:      desc.CommonName,
		SANs:            append([]string(nil), desc.DNSNames...),
		IssuerName:      desc.IssuerName,
		CertSecretRefID: certRef.ID,
		KeySecretRefID:  &keyRefID,
		NotBefore:       &desc.NotBefore,
		NotAfter:        &desc.NotAfter,
		IsDefault:       isDefault,
		Meta:            map[string]any{"provider": "self_signed", "valid_days": validDays},
	})
}

func (s *Store) CreateManagedPlatformCertificateAuthority(ctx context.Context, name, description, commonName string) (domain.PlatformCertificate, error) {
	certPEM, keyPEM, err := pki.GenerateCertificateAuthority(commonName)
	if err != nil {
		return domain.PlatformCertificate{}, err
	}
	desc, err := pki.DescribeCertificatePEM(certPEM)
	if err != nil {
		return domain.PlatformCertificate{}, err
	}
	certRef, err := s.CreateSecretRef(ctx, "certificate", certPEM, map[string]any{"scope": "platform_certificate", "material": "managed_ca_cert"})
	if err != nil {
		return domain.PlatformCertificate{}, err
	}
	keyRef, err := s.CreateSecretRef(ctx, "private_key", keyPEM, map[string]any{"scope": "platform_certificate", "material": "managed_ca_key"})
	if err != nil {
		return domain.PlatformCertificate{}, err
	}
	keyRefID := keyRef.ID
	return s.insertPlatformCertificate(ctx, domain.PlatformCertificate{
		ID:              id.New(),
		Name:            firstString(name, commonName, "managed-ca"),
		Description:     strings.TrimSpace(description),
		Source:          "managed_ca",
		Kind:            "ca",
		Status:          "active",
		CommonName:      desc.CommonName,
		SANs:            append([]string(nil), desc.DNSNames...),
		IssuerName:      desc.IssuerName,
		CertSecretRefID: certRef.ID,
		KeySecretRefID:  &keyRefID,
		NotBefore:       &desc.NotBefore,
		NotAfter:        &desc.NotAfter,
		Meta:            map[string]any{"provider": "managed_ca"},
	})
}

func (s *Store) IssuePlatformCertificateFromAuthority(ctx context.Context, authorityCertificateID, name, description, commonName string, dnsNames []string, validDays int, isDefault bool) (domain.PlatformCertificate, error) {
	authority, caCertPEM, caKeyPEM, _, err := s.ResolvePlatformCertificateMaterial(ctx, authorityCertificateID)
	if err != nil {
		return domain.PlatformCertificate{}, err
	}
	if authority.Kind != "ca" {
		return domain.PlatformCertificate{}, errors.New("selected certificate is not a CA")
	}
	if len(caKeyPEM) == 0 {
		return domain.PlatformCertificate{}, errors.New("selected CA does not have a private key")
	}
	certPEM, keyPEM, err := pki.IssueSignedCertificateWithOptions(caCertPEM, caKeyPEM, commonName, dnsNames, true, validDays)
	if err != nil {
		return domain.PlatformCertificate{}, err
	}
	desc, err := pki.DescribeCertificatePEM(certPEM)
	if err != nil {
		return domain.PlatformCertificate{}, err
	}
	certRef, err := s.CreateSecretRef(ctx, "certificate", certPEM, map[string]any{"scope": "platform_certificate", "material": "ca_issued_cert", "parent_certificate_id": authority.ID})
	if err != nil {
		return domain.PlatformCertificate{}, err
	}
	keyRef, err := s.CreateSecretRef(ctx, "private_key", keyPEM, map[string]any{"scope": "platform_certificate", "material": "ca_issued_key", "parent_certificate_id": authority.ID})
	if err != nil {
		return domain.PlatformCertificate{}, err
	}
	keyRefID := keyRef.ID
	parentID := authority.ID
	return s.insertPlatformCertificate(ctx, domain.PlatformCertificate{
		ID:                  id.New(),
		Name:                firstString(name, commonName, "issued-certificate"),
		Description:         strings.TrimSpace(description),
		Source:              "ca_issued",
		Kind:                "leaf",
		Status:              "active",
		CommonName:          desc.CommonName,
		SANs:                append([]string(nil), desc.DNSNames...),
		IssuerName:          authority.CommonName,
		ParentCertificateID: &parentID,
		CertSecretRefID:     certRef.ID,
		KeySecretRefID:      &keyRefID,
		NotBefore:           &desc.NotBefore,
		NotAfter:            &desc.NotAfter,
		IsDefault:           isDefault,
		Meta:                map[string]any{"provider": "managed_ca_issue", "authority_certificate_id": authority.ID, "valid_days": validDays},
	})
}

func (s *Store) ResolvePlatformCertificateMaterial(ctx context.Context, certificateID string) (domain.PlatformCertificate, []byte, []byte, []byte, error) {
	item, err := s.GetPlatformCertificate(ctx, certificateID)
	if err != nil {
		return domain.PlatformCertificate{}, nil, nil, nil, err
	}
	_, certPEM, err := s.ResolveSecretValue(ctx, item.CertSecretRefID)
	if err != nil {
		return domain.PlatformCertificate{}, nil, nil, nil, err
	}
	var keyPEM []byte
	if item.KeySecretRefID != nil && strings.TrimSpace(*item.KeySecretRefID) != "" {
		_, keyPEM, err = s.ResolveSecretValue(ctx, *item.KeySecretRefID)
		if err != nil {
			return domain.PlatformCertificate{}, nil, nil, nil, err
		}
	}
	var chainPEM []byte
	if item.ChainSecretRefID != nil && strings.TrimSpace(*item.ChainSecretRefID) != "" {
		_, chainPEM, err = s.ResolveSecretValue(ctx, *item.ChainSecretRefID)
		if err != nil {
			return domain.PlatformCertificate{}, nil, nil, nil, err
		}
	} else if item.ParentCertificateID != nil && strings.TrimSpace(*item.ParentCertificateID) != "" {
		parent, parentCertPEM, _, _, err := s.ResolvePlatformCertificateMaterial(ctx, *item.ParentCertificateID)
		if err == nil && parent.ID != "" && len(parentCertPEM) > 0 {
			chainPEM = parentCertPEM
		}
	}
	return item, certPEM, keyPEM, chainPEM, nil
}

func (s *Store) insertPlatformCertificate(ctx context.Context, item domain.PlatformCertificate) (domain.PlatformCertificate, error) {
	item.Name = strings.TrimSpace(item.Name)
	item.Description = strings.TrimSpace(item.Description)
	item.Source = strings.TrimSpace(item.Source)
	item.Kind = strings.TrimSpace(item.Kind)
	item.Status = firstString(item.Status, "active")
	item.CommonName = strings.TrimSpace(item.CommonName)
	item.IssuerName = strings.TrimSpace(item.IssuerName)
	if item.ID == "" {
		item.ID = id.New()
	}
	now := time.Now().UTC()
	item.CreatedAt = now
	item.UpdatedAt = now
	if item.SANs == nil {
		item.SANs = []string{}
	}
	if item.Meta == nil {
		item.Meta = map[string]any{}
	}
	if item.Kind != "leaf" {
		item.IsDefault = false
	}
	if item.IsDefault {
		if _, err := s.db.Exec(ctx, `update platform_certificates set is_default=false, updated_at=$1 where kind='leaf' and status='active' and is_default=true`, now); err != nil {
			return domain.PlatformCertificate{}, err
		}
	}
	if _, err := s.db.Exec(ctx, `insert into platform_certificates(
		id,name,description,source,kind,status,common_name,san_json,issuer_name,parent_certificate_id,cert_secret_ref_id,key_secret_ref_id,chain_secret_ref_id,not_before,not_after,is_default,meta_json,created_at,updated_at
	) values(
		$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19
	)`,
		item.ID,
		item.Name,
		item.Description,
		item.Source,
		item.Kind,
		item.Status,
		item.CommonName,
		mustJSON(item.SANs),
		item.IssuerName,
		item.ParentCertificateID,
		item.CertSecretRefID,
		item.KeySecretRefID,
		item.ChainSecretRefID,
		item.NotBefore,
		item.NotAfter,
		item.IsDefault,
		mustJSON(item.Meta),
		item.CreatedAt,
		item.UpdatedAt,
	); err != nil {
		return domain.PlatformCertificate{}, err
	}
	_, _ = s.CreateAudit(ctx, "system", "platform_certificate.create", "platform_certificate", &item.ID, "platform certificate created")
	return item, nil
}
