package externalegress

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"
)

type CertificateMaterial struct {
	CASubject      string
	PrivateKeyType string
}

func ValidateCertificateMaterial(caPEM, certificatePEM, privateKeyPEM string) (CertificateMaterial, error) {
	var material CertificateMaterial
	if caPEM == "" || certificatePEM == "" || privateKeyPEM == "" {
		return material, fmt.Errorf("certificate authentication requires CA certificate, client certificate and private key")
	}
	caCertificates, err := parseCertificates(caPEM, "IPsec CA certificate")
	if err != nil {
		return material, err
	}
	clientCertificates, err := parseCertificates(certificatePEM, "IPsec client certificate")
	if err != nil {
		return material, err
	}
	leaf := clientCertificates[0]
	roots := x509.NewCertPool()
	intermediates := x509.NewCertPool()
	for _, certificate := range caCertificates {
		if !certificate.IsCA {
			return material, fmt.Errorf("IPsec CA certificate bundle contains a non-CA certificate")
		}
		roots.AddCert(certificate)
	}
	for _, certificate := range clientCertificates[1:] {
		intermediates.AddCert(certificate)
	}
	if _, err := leaf.Verify(x509.VerifyOptions{
		Roots: roots, Intermediates: intermediates,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	}); err != nil {
		return material, fmt.Errorf("IPsec client certificate chain is invalid: %w", err)
	}
	signer, keyType, err := parsePrivateKey(privateKeyPEM)
	if err != nil {
		return material, err
	}
	certificatePublicKey, err := x509.MarshalPKIXPublicKey(leaf.PublicKey)
	if err != nil {
		return material, fmt.Errorf("encode IPsec client certificate public key: %w", err)
	}
	privatePublicKey, err := x509.MarshalPKIXPublicKey(signer.Public())
	if err != nil {
		return material, fmt.Errorf("encode IPsec private key public key: %w", err)
	}
	if !bytes.Equal(certificatePublicKey, privatePublicKey) {
		return material, fmt.Errorf("IPsec private key does not match the client certificate")
	}
	material.CASubject = caCertificates[0].Subject.String()
	material.PrivateKeyType = keyType
	return material, nil
}

func parseCertificates(value, label string) ([]*x509.Certificate, error) {
	remaining := []byte(value)
	certificates := []*x509.Certificate{}
	for len(remaining) > 0 {
		block, rest := pem.Decode(remaining)
		if block == nil {
			break
		}
		remaining = rest
		if block.Type != "CERTIFICATE" {
			continue
		}
		certificate, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("%s is invalid: %w", label, err)
		}
		certificates = append(certificates, certificate)
	}
	if len(certificates) == 0 || strings.TrimSpace(string(remaining)) != "" {
		return nil, fmt.Errorf("%s must contain valid PEM certificates only", label)
	}
	return certificates, nil
}

func parsePrivateKey(value string) (crypto.Signer, string, error) {
	block, remaining := pem.Decode([]byte(value))
	if block == nil || strings.TrimSpace(string(remaining)) != "" {
		return nil, "", fmt.Errorf("IPsec private key must contain one PEM private key")
	}
	var key any
	var err error
	switch block.Type {
	case "RSA PRIVATE KEY":
		key, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	case "EC PRIVATE KEY":
		key, err = x509.ParseECPrivateKey(block.Bytes)
	case "PRIVATE KEY":
		key, err = x509.ParsePKCS8PrivateKey(block.Bytes)
	default:
		return nil, "", fmt.Errorf("IPsec private key type %q is not supported", block.Type)
	}
	if err != nil {
		return nil, "", fmt.Errorf("IPsec private key is invalid: %w", err)
	}
	switch signer := key.(type) {
	case *rsa.PrivateKey:
		return signer, "RSA", nil
	case *ecdsa.PrivateKey:
		return signer, "ECDSA", nil
	default:
		return nil, "", fmt.Errorf("IPsec private key must use RSA or ECDSA")
	}
}
