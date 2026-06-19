package auth

import "testing"

func TestHashPasswordRejectsShortPassword(t *testing.T) {
	_, err := HashPassword("short-pass")
	if err == nil {
		t.Fatal("expected short password to be rejected")
	}
}

func TestHashPasswordAndVerifyPassword(t *testing.T) {
	const password = "correct-horse-battery-staple"

	encoded, err := HashPassword(password)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if encoded == "" {
		t.Fatal("expected encoded password hash")
	}
	if !VerifyPassword(password, encoded) {
		t.Fatal("expected password verification to succeed")
	}
	if VerifyPassword(password+"-wrong", encoded) {
		t.Fatal("expected wrong password verification to fail")
	}
}
