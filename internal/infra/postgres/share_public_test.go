package postgres

import "testing"

func TestShareLinkTokenHashingPrimitives(t *testing.T) {
	t.Parallel()

	token := randomToken(32)
	if len(token) < 64 {
		t.Fatalf("share token should have at least 256 bits encoded as hex, got len=%d", len(token))
	}
	hash := hashToken(token)
	if hash == token {
		t.Fatal("token hash must not equal plaintext token")
	}
	if got := len(hash); got != 64 {
		t.Fatalf("sha256 token hash length = %d, want 64 hex chars", got)
	}
	if hint := tokenHint(token); hint == token || len(hint) >= len(token) {
		t.Fatalf("token hint must not expose full token: token=%q hint=%q", token, hint)
	}
}
