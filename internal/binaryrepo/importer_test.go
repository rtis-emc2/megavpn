package binaryrepo

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportReaderStoresArtifactAndCalculatesSHA(t *testing.T) {
	root := t.TempDir()
	body := []byte("#!/bin/sh\nexit 0\n")
	artifact, err := ImportReader(root, bytes.NewReader(body), ImportRequest{
		SourceFilename: "xray-install.sh",
		ServiceCode:    "xray-core",
		Version:        "1.8.24",
		Architecture:   "amd64",
		InstallMode:    "xray_install_script",
	})
	if err != nil {
		t.Fatalf("ImportReader() error = %v", err)
	}
	stored, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(artifact.StoragePath)))
	if err != nil {
		t.Fatalf("read stored artifact: %v", err)
	}
	if string(stored) != string(body) {
		t.Fatalf("stored content = %q, want %q", string(stored), string(body))
	}
	sum := sha256.Sum256(body)
	if artifact.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("sha256 = %q, want %q", artifact.SHA256, hex.EncodeToString(sum[:]))
	}
	if artifact.Metadata["install_mode"] != "xray_install_script" {
		t.Fatalf("metadata = %#v, want install_mode", artifact.Metadata)
	}
}

func TestImportReaderRejectsExpectedSHAMismatch(t *testing.T) {
	root := t.TempDir()
	_, err := ImportReader(root, strings.NewReader("one"), ImportRequest{
		SourceFilename: "runtime.bin",
		ServiceCode:    "shadowsocks",
		Version:        "1.0.0",
		Architecture:   "amd64",
		ExpectedSHA256: strings.Repeat("0", 64),
	})
	if err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("ImportReader() error = %v, want sha mismatch", err)
	}
}

func TestCopyArtifactRejectsTraversalAndSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	if _, err := CleanRelativePath("../xray"); err == nil {
		t.Fatal("CleanRelativePath traversal succeeded, want error")
	}
	outside := t.TempDir()
	link := filepath.Join(root, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	_, _, err := CopyArtifact(root, strings.NewReader("binary"), "link/runtime.bin", CopyOptions{})
	if err == nil || !strings.Contains(err.Error(), "outside artifact root") {
		t.Fatalf("CopyArtifact symlink escape error = %v, want outside root", err)
	}
}

func TestCopyArtifactRejectsTooLargeUpload(t *testing.T) {
	root := t.TempDir()
	_, _, err := CopyArtifact(root, strings.NewReader("abcdef"), "runtime-repository/runtime.bin", CopyOptions{MaxBytes: 3})
	if err == nil || !strings.Contains(err.Error(), "maximum upload size") {
		t.Fatalf("CopyArtifact too large error = %v, want max size rejection", err)
	}
}
