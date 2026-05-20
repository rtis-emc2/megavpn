package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
)

type Service struct {
	key        []byte
	keyVersion string
}

func LoadFromFile(path, version string) (*Service, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("master key path is empty")
	}
	if strings.TrimSpace(version) == "" {
		return nil, errors.New("master key version is empty")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	key, err := parseKey(strings.TrimSpace(string(raw)))
	if err != nil {
		return nil, err
	}
	return &Service{key: key, keyVersion: version}, nil
}

func (s *Service) KeyVersion() string {
	if s == nil {
		return ""
	}
	return s.keyVersion
}

func (s *Service) Encrypt(plaintext []byte) (ciphertext []byte, nonce []byte, keyVersion string, err error) {
	if s == nil {
		return nil, nil, "", errors.New("secret service is nil")
	}
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return nil, nil, "", err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, "", err
	}
	nonce = make([]byte, aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, nil, "", err
	}
	ciphertext = aead.Seal(nil, nonce, plaintext, nil)
	return ciphertext, nonce, s.keyVersion, nil
}

func (s *Service) Decrypt(ciphertext, nonce []byte) ([]byte, error) {
	if s == nil {
		return nil, errors.New("secret service is nil")
	}
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(nonce) != aead.NonceSize() {
		return nil, fmt.Errorf("invalid nonce size %d", len(nonce))
	}
	return aead.Open(nil, nonce, ciphertext, nil)
}

func parseKey(raw string) ([]byte, error) {
	if raw == "" {
		return nil, errors.New("master key file is empty")
	}
	if b, err := hex.DecodeString(raw); err == nil && len(b) == 32 {
		return b, nil
	}
	if b, err := base64.RawStdEncoding.DecodeString(raw); err == nil && len(b) == 32 {
		return b, nil
	}
	if b, err := base64.StdEncoding.DecodeString(raw); err == nil && len(b) == 32 {
		return b, nil
	}
	if len(raw) == 32 {
		return []byte(raw), nil
	}
	return nil, errors.New("master key must be 32 bytes, base64 or hex encoded")
}
