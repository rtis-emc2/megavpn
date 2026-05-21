package pki

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
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

func parseCertificatePEM(raw []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("certificate pem decode failed")
	}
	return x509.ParseCertificate(block.Bytes)
}

func parseECPrivateKeyPEM(raw []byte) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("private key pem decode failed")
	}
	if key, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key is not ecdsa")
	}
	return key, nil
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
