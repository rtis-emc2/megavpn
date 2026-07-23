package http

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveWebAssetRejectsTraversalAndSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "assets", "app.js")
	if err := os.MkdirAll(filepath.Dir(inside), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inside, []byte("inside"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "assets", "outside.js")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}

	server := &Server{webRoot: root}
	if got, ok := server.resolveWebAsset("assets/app.js"); !ok || got != inside {
		t.Fatalf("managed asset = %q, %v; want %q, true", got, ok, inside)
	}
	for _, path := range []string{"../secret.txt", "assets/../../secret.txt", "assets/outside.js"} {
		if got, ok := server.resolveWebAsset(path); ok {
			t.Fatalf("unsafe asset %q resolved to %q", path, got)
		}
	}
}
