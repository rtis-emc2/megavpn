package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"hash"
	"strings"
)

const (
	passwordScheme     = "pbkdf2_sha256"
	passwordIterations = 600_000
	passwordSaltBytes  = 16
	passwordKeyLen     = 32
	minPasswordLen     = 12
)

func HashPassword(password string) (string, error) {
	password = strings.TrimSpace(password)
	if len(password) < minPasswordLen {
		return "", fmt.Errorf("password must be at least %d characters", minPasswordLen)
	}
	salt := make([]byte, passwordSaltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := pbkdf2Key(sha256.New, []byte(password), salt, passwordIterations, passwordKeyLen)
	return fmt.Sprintf(
		"%s$%d$%s$%s",
		passwordScheme,
		passwordIterations,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

func VerifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != passwordScheme {
		return false
	}
	var iterations int
	if _, err := fmt.Sscanf(parts[1], "%d", &iterations); err != nil || iterations <= 0 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(want) == 0 {
		return false
	}
	got := pbkdf2Key(sha256.New, []byte(password), salt, iterations, len(want))
	return subtle.ConstantTimeCompare(got, want) == 1
}

func pbkdf2Key(h func() hash.Hash, password, salt []byte, iterations, keyLen int) []byte {
	prf := hmac.New(h, password)
	hashLen := prf.Size()
	numBlocks := (keyLen + hashLen - 1) / hashLen
	out := make([]byte, 0, numBlocks*hashLen)
	blockBuf := make([]byte, len(salt)+4)
	copy(blockBuf, salt)

	for block := 1; block <= numBlocks; block++ {
		binary.BigEndian.PutUint32(blockBuf[len(salt):], uint32(block))
		prf.Reset()
		_, _ = prf.Write(blockBuf)
		u := prf.Sum(nil)
		t := append([]byte(nil), u...)
		for i := 1; i < iterations; i++ {
			prf.Reset()
			_, _ = prf.Write(u)
			u = prf.Sum(nil)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		out = append(out, t...)
	}
	return out[:keyLen]
}
