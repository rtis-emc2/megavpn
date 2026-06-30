package main

import "testing"

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
