package http

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

func TestValidateArtifactPathRequiresArtifactRootContainment(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	server := &Server{artifactRoot: root}

	insideDir := filepath.Join(root, "client-1")
	if err := os.MkdirAll(insideDir, 0o700); err != nil {
		t.Fatalf("mkdir inside dir: %v", err)
	}
	inside := filepath.Join(insideDir, "client.ovpn")
	if err := os.WriteFile(inside, []byte("client\n"), 0o600); err != nil {
		t.Fatalf("write inside artifact: %v", err)
	}
	if err := server.validateArtifactPath(inside); err != nil {
		t.Fatalf("inside artifact rejected: %v", err)
	}

	outside := filepath.Join(t.TempDir(), "outside.ovpn")
	if err := os.WriteFile(outside, []byte("outside\n"), 0o600); err != nil {
		t.Fatalf("write outside artifact: %v", err)
	}
	if err := server.validateArtifactPath(outside); err == nil {
		t.Fatal("outside artifact path must be rejected")
	}

	symlink := filepath.Join(root, "linked-outside.ovpn")
	if err := os.Symlink(outside, symlink); err != nil {
		t.Skipf("symlink not supported in test environment: %v", err)
	}
	if err := server.validateArtifactPath(symlink); err == nil {
		t.Fatal("artifact symlink escaping artifact root must be rejected")
	}
}

func TestResolveArtifactFileRejectsNonReadyArtifacts(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "client.ovpn")
	if err := os.WriteFile(path, []byte("client\n"), 0o600); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	server := &Server{artifactRoot: root}
	if _, _, _, err := server.resolveArtifactFile(domain.Artifact{
		Status:      "pending",
		StoragePath: path,
	}); err == nil {
		t.Fatal("non-ready artifact must not be served")
	}
}

func TestPreviewableArtifactTypes(t *testing.T) {
	t.Parallel()

	for _, artifactType := range []string{"ovpn", "vless_url", "wg_conf", "mtproto_url", "http_proxy_bundle", "ss_url", "ipsec_bundle"} {
		if !isPreviewableArtifactType(artifactType) {
			t.Fatalf("%s should be previewable", artifactType)
		}
	}
	if isPreviewableArtifactType("zip_bundle") {
		t.Fatal("zip_bundle must not be inline-previewable")
	}
}
