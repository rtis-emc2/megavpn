package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyFileSHA256(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "installer.sh")
	content := []byte("#!/usr/bin/env sh\nexit 0\n")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	if err := verifyFileSHA256(path, hex.EncodeToString(sum[:])); err != nil {
		t.Fatalf("expected checksum to match: %v", err)
	}
	if err := verifyFileSHA256(path, "0000000000000000000000000000000000000000000000000000000000000000"); err == nil {
		t.Fatal("expected checksum mismatch")
	}
	if err := verifyFileSHA256(path, "not-a-sha"); err == nil {
		t.Fatal("expected malformed checksum to be rejected")
	}
}
