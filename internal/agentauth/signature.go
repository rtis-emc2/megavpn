package agentauth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	HeaderSignature = "X-MegaVPN-Agent-Signature"
	HeaderTimestamp = "X-MegaVPN-Agent-Timestamp"
	HeaderNonce     = "X-MegaVPN-Agent-Nonce"
	HeaderBodyHash  = "X-MegaVPN-Agent-Body-SHA256"

	signaturePrefix = "v1="
)

var (
	ErrUnsigned          = errors.New("agent request is not signed")
	ErrInvalidSignature  = errors.New("agent request signature is invalid")
	ErrInvalidTimestamp  = errors.New("agent request signature timestamp is invalid")
	ErrTimestampOutdated = errors.New("agent request signature timestamp is outside allowed window")
	ErrInvalidNonce      = errors.New("agent request signature nonce is invalid")
	ErrInvalidBodyHash   = errors.New("agent request body hash is invalid")
)

func NewNonce() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

func BodyHash(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func CanonicalString(method, requestURI, timestamp, nonce, bodyHash string) string {
	return strings.ToUpper(strings.TrimSpace(method)) + "\n" +
		strings.TrimSpace(requestURI) + "\n" +
		strings.TrimSpace(timestamp) + "\n" +
		strings.TrimSpace(nonce) + "\n" +
		strings.TrimSpace(bodyHash)
}

func Sign(token, method, requestURI, timestamp, nonce string, body []byte) (string, string) {
	bodyHash := BodyHash(body)
	signature := SignBodyHash(token, method, requestURI, timestamp, nonce, bodyHash)
	return signature, bodyHash
}

func SignBodyHash(token, method, requestURI, timestamp, nonce, bodyHash string) string {
	mac := hmac.New(sha256.New, []byte(token))
	_, _ = mac.Write([]byte(CanonicalString(method, requestURI, timestamp, nonce, bodyHash)))
	return signaturePrefix + hex.EncodeToString(mac.Sum(nil))
}

func Verify(token, method, requestURI, timestamp, nonce, bodyHash, signature string, body []byte, now time.Time, window time.Duration) error {
	return VerifyBodyHash(token, method, requestURI, timestamp, nonce, bodyHash, signature, BodyHash(body), now, window)
}

func VerifyBodyHash(token, method, requestURI, timestamp, nonce, bodyHash, signature, actualBodyHash string, now time.Time, window time.Duration) error {
	if strings.TrimSpace(signature) == "" || strings.TrimSpace(timestamp) == "" || strings.TrimSpace(nonce) == "" || strings.TrimSpace(bodyHash) == "" {
		return ErrUnsigned
	}
	if strings.TrimSpace(token) == "" {
		return ErrInvalidSignature
	}
	if len(strings.TrimSpace(nonce)) < 12 || len(strings.TrimSpace(nonce)) > 128 {
		return ErrInvalidNonce
	}
	ts, err := strconv.ParseInt(strings.TrimSpace(timestamp), 10, 64)
	if err != nil || ts <= 0 {
		return ErrInvalidTimestamp
	}
	if window <= 0 {
		window = 5 * time.Minute
	}
	issuedAt := time.Unix(ts, 0).UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if issuedAt.Before(now.Add(-window)) || issuedAt.After(now.Add(window)) {
		return ErrTimestampOutdated
	}
	if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(bodyHash)), []byte(actualBodyHash)) != 1 {
		return ErrInvalidBodyHash
	}
	wantSignature := SignBodyHash(token, method, requestURI, timestamp, nonce, bodyHash)
	if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(signature)), []byte(wantSignature)) != 1 {
		return ErrInvalidSignature
	}
	return nil
}
