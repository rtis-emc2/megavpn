package main

import (
	"errors"
	"testing"
)

func TestRandomStrongPasswordShape(t *testing.T) {
	password, err := randomStrongPassword(32)
	if err != nil {
		t.Fatalf("randomStrongPassword failed: %v", err)
	}
	if len(password) != 32 {
		t.Fatalf("password length = %d, want 32", len(password))
	}
	if !containsAny(password, "abcdefghijklmnopqrstuvwxyz") {
		t.Fatalf("password does not include a lowercase character: %q", password)
	}
	if !containsAny(password, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") {
		t.Fatalf("password does not include an uppercase character: %q", password)
	}
	if !containsAny(password, "0123456789") {
		t.Fatalf("password does not include a digit: %q", password)
	}
	if !containsAny(password, "!#$%&()*+,-.:=?@_~") {
		t.Fatalf("password does not include a symbol: %q", password)
	}
}

type failingRandomReader struct{}

func (failingRandomReader) Read([]byte) (int, error) {
	return 0, errors.New("entropy unavailable")
}

func TestRandomHexStringFailsClosedWhenEntropyIsUnavailable(t *testing.T) {
	if value, err := randomHexStringFrom(failingRandomReader{}, 12); err == nil || value != "" {
		t.Fatalf("randomHexStringFrom() = %q, %v; want empty value and error", value, err)
	}
}

func containsAny(value, candidates string) bool {
	for _, ch := range value {
		for _, candidate := range candidates {
			if ch == candidate {
				return true
			}
		}
	}
	return false
}
