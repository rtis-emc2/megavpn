package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/pki"
	"github.com/rtis-emc2/megavpn/internal/platform/id"
)

func (s *Store) GetActivePlatformServicePKIRoot(ctx context.Context, serviceCode, profile string) (domain.PlatformServicePKIRoot, error) {
	serviceCode = normalizeServicePKIValue(serviceCode, "")
	profile = normalizeServicePKIValue(profile, "default")
	var root domain.PlatformServicePKIRoot
	err := s.db.QueryRow(ctx, `select id,service_code,pki_profile,status,ca_cert_secret_ref_id,ca_key_secret_ref_id,common_name,created_at,rotated_at
from platform_service_pki_roots
where service_code=$1 and pki_profile=$2 and status='active'
order by created_at desc
limit 1`, serviceCode, profile).
		Scan(&root.ID, &root.ServiceCode, &root.PKIProfile, &root.Status, &root.CACertSecretRefID, &root.CAKeySecretRefID, &root.CommonName, &root.CreatedAt, &root.RotatedAt)
	return root, err
}

func (s *Store) ListPlatformServicePKIRoots(ctx context.Context) ([]domain.PlatformServicePKIRoot, error) {
	rows, err := s.db.Query(ctx, `select id,service_code,pki_profile,status,ca_cert_secret_ref_id,ca_key_secret_ref_id,common_name,created_at,rotated_at
from platform_service_pki_roots
order by service_code,pki_profile,created_at desc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.PlatformServicePKIRoot{}
	for rows.Next() {
		var root domain.PlatformServicePKIRoot
		if err := rows.Scan(&root.ID, &root.ServiceCode, &root.PKIProfile, &root.Status, &root.CACertSecretRefID, &root.CAKeySecretRefID, &root.CommonName, &root.CreatedAt, &root.RotatedAt); err != nil {
			return nil, err
		}
		out = append(out, root)
	}
	return out, rows.Err()
}

func (s *Store) CreatePlatformServicePKIRoot(ctx context.Context, serviceCode, profile, commonName, caCertRefID, caKeyRefID string) (domain.PlatformServicePKIRoot, error) {
	serviceCode = normalizeServicePKIValue(serviceCode, "")
	profile = normalizeServicePKIValue(profile, "default")
	commonName = strings.TrimSpace(commonName)
	caCertRefID = strings.TrimSpace(caCertRefID)
	caKeyRefID = strings.TrimSpace(caKeyRefID)
	if serviceCode == "" {
		return domain.PlatformServicePKIRoot{}, errors.New("service_code is required")
	}
	if commonName == "" || caCertRefID == "" || caKeyRefID == "" {
		return domain.PlatformServicePKIRoot{}, errors.New("pki root common name and ca secret refs are required")
	}
	root := domain.PlatformServicePKIRoot{
		ID:                id.New(),
		ServiceCode:       serviceCode,
		PKIProfile:        profile,
		Status:            "active",
		CACertSecretRefID: caCertRefID,
		CAKeySecretRefID:  caKeyRefID,
		CommonName:        commonName,
		CreatedAt:         time.Now().UTC(),
	}
	if _, err := s.db.Exec(ctx, `insert into platform_service_pki_roots(id,service_code,pki_profile,status,ca_cert_secret_ref_id,ca_key_secret_ref_id,common_name,created_at)
values($1,$2,$3,$4,$5,$6,$7,$8)`,
		root.ID, root.ServiceCode, root.PKIProfile, root.Status, root.CACertSecretRefID, root.CAKeySecretRefID, root.CommonName, root.CreatedAt); err != nil {
		return domain.PlatformServicePKIRoot{}, err
	}
	_, _ = s.CreateAudit(ctx, "system", "platform_service_pki_root.create", "platform_service_pki_root", &root.ID, "platform service pki root created")
	return root, nil
}

func (s *Store) EnsureOpenVPNPlatformPKIRoot(ctx context.Context, profile string) (domain.PlatformServicePKIRoot, error) {
	profile = normalizeServicePKIValue(profile, "default")
	root, err := s.GetActivePlatformServicePKIRoot(ctx, "openvpn", profile)
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
	caCertRef, err := s.CreateSecretRef(ctx, "certificate", caCertPEM, map[string]any{"scope": "platform", "service_code": "openvpn", "pki_profile": profile, "material": "openvpn_ca_cert"})
	if err != nil {
		return domain.PlatformServicePKIRoot{}, err
	}
	caKeyRef, err := s.CreateSecretRef(ctx, "private_key", caKeyPEM, map[string]any{"scope": "platform", "service_code": "openvpn", "pki_profile": profile, "material": "openvpn_ca_key"})
	if err != nil {
		return domain.PlatformServicePKIRoot{}, err
	}
	root, err = s.CreatePlatformServicePKIRoot(ctx, "openvpn", profile, commonName, caCertRef.ID, caKeyRef.ID)
	if err == nil {
		return root, nil
	}
	refetched, refetchErr := s.GetActivePlatformServicePKIRoot(ctx, "openvpn", profile)
	if refetchErr == nil {
		return refetched, nil
	}
	return domain.PlatformServicePKIRoot{}, err
}

func (s *Store) CreateManagedPlatformServicePKIRoot(ctx context.Context, serviceCode, profile, commonName string) (domain.PlatformServicePKIRoot, error) {
	serviceCode = normalizeServicePKIValue(serviceCode, "")
	profile = normalizeServicePKIValue(profile, "default")
	if serviceCode == "" {
		return domain.PlatformServicePKIRoot{}, errors.New("service_code is required")
	}
	if existing, err := s.GetActivePlatformServicePKIRoot(ctx, serviceCode, profile); err == nil && existing.ID != "" {
		return domain.PlatformServicePKIRoot{}, errors.New("active platform pki root already exists for service/profile")
	} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return domain.PlatformServicePKIRoot{}, err
	}
	commonName = strings.TrimSpace(commonName)
	if commonName == "" {
		commonName = "MegaVPN " + strings.ToUpper(serviceCode) + " Platform CA"
		if profile != "default" {
			commonName += " " + profile
		}
	}
	caCertPEM, caKeyPEM, err := pki.GenerateCertificateAuthority(commonName)
	if err != nil {
		return domain.PlatformServicePKIRoot{}, err
	}
	caCertRef, err := s.CreateSecretRef(ctx, "certificate", caCertPEM, map[string]any{"scope": "platform", "service_code": serviceCode, "pki_profile": profile, "material": serviceCode + "_ca_cert"})
	if err != nil {
		return domain.PlatformServicePKIRoot{}, err
	}
	caKeyRef, err := s.CreateSecretRef(ctx, "private_key", caKeyPEM, map[string]any{"scope": "platform", "service_code": serviceCode, "pki_profile": profile, "material": serviceCode + "_ca_key"})
	if err != nil {
		return domain.PlatformServicePKIRoot{}, err
	}
	return s.CreatePlatformServicePKIRoot(ctx, serviceCode, profile, commonName, caCertRef.ID, caKeyRef.ID)
}

func normalizeServicePKIValue(value, fallback string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		value = fallback
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
	value = strings.Trim(out.String(), ".-_")
	if value == "" {
		return fallback
	}
	return value
}
