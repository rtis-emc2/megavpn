package auth

import "testing"

func TestNewSessionTokenReturnsTokenAndHash(t *testing.T) {
	token, tokenHash, err := NewSessionToken()
	if err != nil {
		t.Fatalf("new session token: %v", err)
	}
	if token == "" {
		t.Fatal("expected session token")
	}
	if tokenHash == "" {
		t.Fatal("expected session token hash")
	}
	if token == tokenHash {
		t.Fatal("token hash must not equal raw token")
	}
	if got := HashSessionToken(token); got != tokenHash {
		t.Fatalf("hash mismatch: got %q want %q", got, tokenHash)
	}
}

