package agentauth

import (
	"errors"
	"strconv"
	"testing"
	"time"
)

func TestSignVerifyRoundTrip(t *testing.T) {
	t.Parallel()

	body := []byte(`{"status":"succeeded"}`)
	timestamp := strconv.FormatInt(time.Now().UTC().Unix(), 10)
	nonce := NewNonce()
	signature, bodyHash := Sign("agent-token", "POST", "/agent/jobs/job-1/result", timestamp, nonce, body)

	if err := Verify("agent-token", "POST", "/agent/jobs/job-1/result", timestamp, nonce, bodyHash, signature, body, time.Now().UTC(), time.Minute); err != nil {
		t.Fatalf("Verify signed request error = %v, want nil", err)
	}
}

func TestVerifyRejectsTamperedBody(t *testing.T) {
	t.Parallel()

	body := []byte(`{"status":"succeeded"}`)
	timestamp := strconv.FormatInt(time.Now().UTC().Unix(), 10)
	nonce := NewNonce()
	signature, bodyHash := Sign("agent-token", "POST", "/agent/jobs/job-1/result", timestamp, nonce, body)

	err := Verify("agent-token", "POST", "/agent/jobs/job-1/result", timestamp, nonce, bodyHash, signature, []byte(`{"status":"failed"}`), time.Now().UTC(), time.Minute)
	if !errors.Is(err, ErrInvalidBodyHash) {
		t.Fatalf("Verify tampered body error = %v, want ErrInvalidBodyHash", err)
	}
}

func TestSignVerifyBodyHashRoundTrip(t *testing.T) {
	t.Parallel()

	bodyHash := BodyHash([]byte("runtime artifact"))
	timestamp := strconv.FormatInt(time.Now().UTC().Unix(), 10)
	nonce := NewNonce()
	signature := SignBodyHash("agent-token", "RESPONSE", "/agent/binary-artifacts/artifact-1/download?node_id=node-1&job_id=job-1", timestamp, nonce, bodyHash)

	err := VerifyBodyHash(
		"agent-token",
		"RESPONSE",
		"/agent/binary-artifacts/artifact-1/download?node_id=node-1&job_id=job-1",
		timestamp,
		nonce,
		bodyHash,
		signature,
		bodyHash,
		time.Now().UTC(),
		time.Minute,
	)
	if err != nil {
		t.Fatalf("VerifyBodyHash signed response error = %v, want nil", err)
	}
}

func TestVerifyBodyHashRejectsMismatchedActualHash(t *testing.T) {
	t.Parallel()

	bodyHash := BodyHash([]byte("runtime artifact"))
	timestamp := strconv.FormatInt(time.Now().UTC().Unix(), 10)
	nonce := NewNonce()
	signature := SignBodyHash("agent-token", "RESPONSE", "/agent/binary-artifacts/artifact-1/download", timestamp, nonce, bodyHash)

	err := VerifyBodyHash("agent-token", "RESPONSE", "/agent/binary-artifacts/artifact-1/download", timestamp, nonce, bodyHash, signature, BodyHash([]byte("tampered")), time.Now().UTC(), time.Minute)
	if !errors.Is(err, ErrInvalidBodyHash) {
		t.Fatalf("VerifyBodyHash mismatched body hash error = %v, want ErrInvalidBodyHash", err)
	}
}

func TestVerifyRejectsExpiredTimestamp(t *testing.T) {
	t.Parallel()

	body := []byte(`{}`)
	timestamp := strconv.FormatInt(time.Now().UTC().Add(-10*time.Minute).Unix(), 10)
	nonce := NewNonce()
	signature, bodyHash := Sign("agent-token", "GET", "/agent/jobs/next?node_id=node-1", timestamp, nonce, body)

	err := Verify("agent-token", "GET", "/agent/jobs/next?node_id=node-1", timestamp, nonce, bodyHash, signature, body, time.Now().UTC(), time.Minute)
	if !errors.Is(err, ErrTimestampOutdated) {
		t.Fatalf("Verify expired timestamp error = %v, want ErrTimestampOutdated", err)
	}
}
