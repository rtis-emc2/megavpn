package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const testBackupID = "backup-20260811T120000Z-a1b2c3d4"

func TestManagerRejectsInvalidBackupIDs(t *testing.T) {
	manager, err := New(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"../etc/passwd", "backup.dump", "", testBackupID + "/child"} {
		if _, _, err := manager.Open(id); err == nil {
			t.Fatalf("Open(%q) unexpectedly succeeded", id)
		}
	}
}

func TestManagerRejectsSymlinkBackupArtifacts(t *testing.T) {
	root := t.TempDir()
	manager, err := New(root, "")
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "outside.dump")
	if err := os.WriteFile(target, []byte("not a backup"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, testBackupID+".dump")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.Open(testBackupID); err == nil {
		t.Fatal("opening a symlink backup unexpectedly succeeded")
	}
	if err := manager.Delete(testBackupID); err == nil {
		t.Fatal("deleting a symlink backup unexpectedly succeeded")
	}
	records, err := manager.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 {
		t.Fatalf("symlink backup appeared in list: %+v", records)
	}
}

func TestManagerVerifyRejectsSymlinkChecksum(t *testing.T) {
	root := t.TempDir()
	manager, err := New(root, "")
	if err != nil {
		t.Fatal(err)
	}
	dump := []byte("archive")
	if err := os.WriteFile(filepath.Join(root, testBackupID+".dump"), dump, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(dump)
	outside := filepath.Join(t.TempDir(), "outside.sha256")
	if err := os.WriteFile(outside, []byte(hex.EncodeToString(sum[:])+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, testBackupID+".sha256")); err != nil {
		t.Fatal(err)
	}
	manager.run = func(context.Context, *exec.Cmd) error {
		t.Fatal("pg_restore must not run for an unsafe checksum path")
		return nil
	}
	if _, err := manager.Verify(context.Background(), testBackupID); err == nil {
		t.Fatal("verification with a symlink checksum unexpectedly succeeded")
	}
}

func TestManagerOpenRejectsChecksumMismatch(t *testing.T) {
	root := t.TempDir()
	manager, err := New(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, testBackupID+".dump"), []byte("modified archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	validButWrong := sha256.Sum256([]byte("original archive"))
	if err := os.WriteFile(filepath.Join(root, testBackupID+".sha256"), []byte(hex.EncodeToString(validButWrong[:])+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.Open(testBackupID); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("opening a backup with a mismatched checksum returned %v, want ErrIntegrity", err)
	}
	records, err := manager.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Verified {
		t.Fatalf("checksum mismatch must be visible in listing: %+v", records)
	}
}

func TestManagerRejectsNonHexChecksum(t *testing.T) {
	root := t.TempDir()
	manager, err := New(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, testBackupID+".dump"), []byte("archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, testBackupID+".sha256"), []byte(strings.Repeat("z", sha256.Size*2)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.Open(testBackupID); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("opening a backup with a non-hex checksum returned %v, want ErrIntegrity", err)
	}
}

func TestNewRejectsSymlinkRoot(t *testing.T) {
	target := t.TempDir()
	root := filepath.Join(t.TempDir(), "backups")
	if err := os.Symlink(target, root); err != nil {
		t.Fatal(err)
	}
	if _, err := New(root, ""); err == nil {
		t.Fatal("symlink backup root unexpectedly accepted")
	}
}
