package pki

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"time"
)

type CertificateDescription struct {
	CommonName string
	DNSNames   []string
	NotBefore  time.Time
	NotAfter   time.Time
	IsCA       bool
	IssuerName string
}

func GenerateCertificateAuthority(commonName string) ([]byte, []byte, error) {
	return GenerateCertificateAuthorityWithOptions(commonName, 365*30)
}

func GenerateCertificateAuthorityWithOptions(commonName string, validDays int) ([]byte, []byte, error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	serialNumber, err := randomSerialNumber()
	if err != nil {
		return nil, nil, err
	}
	if validDays <= 0 {
		validDays = 365 * 30
	}
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: commonName,
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(time.Duration(validDays) * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		IsCA:                  true,
		BasicConstraintsValid: true,
		MaxPathLen:            1,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, privateKey.Public(), privateKey)
	if err != nil {
		return nil, nil, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return nil, nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM, nil
}

func IssueSignedCertificate(caCertPEM, caKeyPEM []byte, commonName string, server bool) ([]byte, []byte, error) {
	return IssueSignedCertificateWithOptions(caCertPEM, caKeyPEM, commonName, nil, server, 365*5)
}

func IssueSignedCertificateWithOptions(caCertPEM, caKeyPEM []byte, commonName string, dnsNames []string, server bool, validDays int) ([]byte, []byte, error) {
	caCert, err := parseCertificatePEM(caCertPEM)
	if err != nil {
		return nil, nil, err
	}
	caKey, err := parseECPrivateKeyPEM(caKeyPEM)
	if err != nil {
		return nil, nil, err
	}
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	serialNumber, err := randomSerialNumber()
	if err != nil {
		return nil, nil, err
	}
	if validDays <= 0 {
		validDays = 365 * 5
	}
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: commonName,
		},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(time.Duration(validDays) * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		SubjectKeyId: []byte(commonName),
		DNSNames:     normalizeDNSNames(dnsNames),
	}
	if server {
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
	}
	der, err := x509.CreateCertificate(rand.Reader, template, caCert, privateKey.Public(), caKey)
	if err != nil {
		return nil, nil, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return nil, nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM, nil
}

func GenerateSelfSignedCertificate(commonName string, dnsNames []string, validDays int) ([]byte, []byte, error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	serialNumber, err := randomSerialNumber()
	if err != nil {
		return nil, nil, err
	}
	if validDays <= 0 {
		validDays = 365
	}
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: commonName,
		},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(time.Duration(validDays) * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     normalizeDNSNames(dnsNames),
		SubjectKeyId: []byte(commonName),
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, privateKey.Public(), privateKey)
	if err != nil {
		return nil, nil, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return nil, nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM, nil
}

func DescribeCertificatePEM(raw []byte) (CertificateDescription, error) {
	cert, err := parseCertificatePEM(raw)
	if err != nil {
		return CertificateDescription{}, err
	}
	return CertificateDescription{
		CommonName: cert.Subject.CommonName,
		DNSNames:   append([]string(nil), cert.DNSNames...),
		NotBefore:  cert.NotBefore,
		NotAfter:   cert.NotAfter,
		IsCA:       cert.IsCA,
		IssuerName: cert.Issuer.CommonName,
	}, nil
}

func ValidateCertificateKeyPair(certPEM, keyPEM []byte) error {
	cert, err := parseCertificatePEM(certPEM)
	if err != nil {
		return err
	}
	key, err := parsePrivateKeyPEM(keyPEM)
	if err != nil {
		return err
	}
	keyPublic, err := publicKeyFromPrivateKey(key)
	if err != nil {
		return err
	}
	if !publicKeysEqual(cert.PublicKey, keyPublic) {
		return fmt.Errorf("certificate and private key do not match")
	}
	return nil
}

func DescribePrivateKeyPEM(raw []byte) (string, error) {
	key, err := parsePrivateKeyPEM(raw)
	if err != nil {
		return "", err
	}
	switch key.(type) {
	case *rsa.PrivateKey:
		return "rsa", nil
	case *ecdsa.PrivateKey:
		return "ecdsa", nil
	case ed25519.PrivateKey:
		return "ed25519", nil
	default:
		return "", fmt.Errorf("unsupported private key type")
	}
}

func CountCertificatesPEM(raw []byte) int {
	count := 0
	rest := raw
	for {
		block, remaining := pem.Decode(rest)
		if block == nil {
			return count
		}
		if block.Type == "CERTIFICATE" {
			count++
		}
		rest = remaining
	}
}

func parseCertificatePEM(raw []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("certificate pem decode failed")
	}
	return x509.ParseCertificate(block.Bytes)
}

func parseECPrivateKeyPEM(raw []byte) (*ecdsa.PrivateKey, error) {
	parsed, err := parsePrivateKeyPEM(raw)
	if err != nil {
		return nil, err
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key is not ecdsa")
	}
	return key, nil
}

func parsePrivateKeyPEM(raw []byte) (any, error) {
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("private key pem decode failed")
	}
	if x509.IsEncryptedPEMBlock(block) {
		return nil, fmt.Errorf("encrypted private keys are not supported; decrypt the key before import")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	if key, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("unsupported private key format: expected PKCS#1 RSA, EC or PKCS#8 private key")
	}
	switch key := parsed.(type) {
	case *rsa.PrivateKey, *ecdsa.PrivateKey, ed25519.PrivateKey:
		return key, nil
	default:
		return nil, fmt.Errorf("unsupported private key type")
	}
}

func publicKeyFromPrivateKey(key any) (any, error) {
	switch typed := key.(type) {
	case *rsa.PrivateKey:
		return &typed.PublicKey, nil
	case *ecdsa.PrivateKey:
		return &typed.PublicKey, nil
	case ed25519.PrivateKey:
		return typed.Public(), nil
	default:
		return nil, fmt.Errorf("unsupported private key type")
	}
}

func publicKeysEqual(certPublic, keyPublic any) bool {
	switch certKey := certPublic.(type) {
	case *rsa.PublicKey:
		key, ok := keyPublic.(*rsa.PublicKey)
		return ok && certKey.E == key.E && certKey.N.Cmp(key.N) == 0
	case *ecdsa.PublicKey:
		key, ok := keyPublic.(*ecdsa.PublicKey)
		return ok && certKey.Curve == key.Curve && certKey.X.Cmp(key.X) == 0 && certKey.Y.Cmp(key.Y) == 0
	case ed25519.PublicKey:
		key, ok := keyPublic.(ed25519.PublicKey)
		return ok && bytes.Equal(certKey, key)
	default:
		return false
	}
}

func randomSerialNumber() (*big.Int, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	return rand.Int(rand.Reader, limit)
}

func normalizeDNSNames(names []string) []string {
	out := make([]string, 0, len(names))
	seen := map[string]bool{}
	for _, item := range names {
		name := item
		if ip := net.ParseIP(name); ip != nil {
			name = ip.String()
		}
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}
