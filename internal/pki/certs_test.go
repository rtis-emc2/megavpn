package pki

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"
)

func TestValidateCertificateKeyPairAcceptsMatchingPair(t *testing.T) {
	certPEM, keyPEM, err := GenerateSelfSignedCertificate("edge.example.com", []string{"edge.example.com"}, 365)
	if err != nil {
		t.Fatalf("generate certificate: %v", err)
	}
	if err := ValidateCertificateKeyPair(certPEM, keyPEM); err != nil {
		t.Fatalf("matching cert/key rejected: %v", err)
	}
}

func TestValidateCertificateKeyPairRejectsMismatchedPair(t *testing.T) {
	certPEM, _, err := GenerateSelfSignedCertificate("edge.example.com", []string{"edge.example.com"}, 365)
	if err != nil {
		t.Fatalf("generate certificate: %v", err)
	}
	_, otherKeyPEM, err := GenerateSelfSignedCertificate("other.example.com", []string{"other.example.com"}, 365)
	if err != nil {
		t.Fatalf("generate second certificate: %v", err)
	}
	if err := ValidateCertificateKeyPair(certPEM, otherKeyPEM); err == nil {
		t.Fatal("mismatched cert/key pair should be rejected")
	}
}

func TestValidateCertificateKeyPairAcceptsRSAPKCS1Key(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(42),
		Subject: pkix.Name{
			CommonName: "edge-rsa.example.com",
		},
		NotBefore: time.Now().Add(-time.Hour),
		NotAfter:  time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:  x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		DNSNames:  []string{"edge-rsa.example.com"},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create rsa certificate: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})

	if err := ValidateCertificateKeyPair(certPEM, keyPEM); err != nil {
		t.Fatalf("rsa pkcs1 cert/key rejected: %v", err)
	}
	keyType, err := DescribePrivateKeyPEM(keyPEM)
	if err != nil {
		t.Fatalf("describe rsa key: %v", err)
	}
	if keyType != "rsa" {
		t.Fatalf("unexpected key type: %s", keyType)
	}
}
