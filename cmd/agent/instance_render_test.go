package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteManagedFileRejectsSymlinkParent(t *testing.T) {
	t.Parallel()

	root := managedWriteTestDir(t)
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	err := writeManagedFile(managedFileSpec{Path: filepath.Join(link, "managed.conf"), Content: "x\n", Mode: "0644"})
	if err == nil {
		t.Fatal("expected symlink parent to be rejected")
	}
}

func TestWriteManagedFileRejectsSymlinkTarget(t *testing.T) {
	t.Parallel()

	root := managedWriteTestDir(t)
	target := filepath.Join(root, "target.conf")
	if err := os.WriteFile(target, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "managed.conf")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	err := writeManagedFile(managedFileSpec{Path: link, Content: "new\n", Mode: "0644"})
	if err == nil {
		t.Fatal("expected symlink target to be rejected")
	}
}

func TestRollbackManagedFilesRestoresExistingAndRemovesNew(t *testing.T) {
	t.Parallel()

	root := managedWriteTestDir(t)
	existing := filepath.Join(root, "existing.conf")
	created := filepath.Join(root, "created.conf")
	if err := os.WriteFile(existing, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	backups, err := snapshotManagedFiles([]managedFileSpec{
		{Path: existing, Content: "new\n", Mode: "0644"},
		{Path: created, Content: "created\n", Mode: "0644"},
	})
	if err != nil {
		t.Fatalf("snapshotManagedFiles returned error: %v", err)
	}
	if err := writeManagedFile(managedFileSpec{Path: existing, Content: "new\n", Mode: "0644"}); err != nil {
		t.Fatal(err)
	}
	if err := writeManagedFile(managedFileSpec{Path: created, Content: "created\n", Mode: "0644"}); err != nil {
		t.Fatal(err)
	}

	result := rollbackManagedFiles(backups)
	if result["ok"] != true {
		t.Fatalf("rollback result = %#v, want ok", result)
	}
	content, err := os.ReadFile(existing)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "old\n" {
		t.Fatalf("existing content = %q, want old", string(content))
	}
	info, err := os.Stat(existing)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("existing mode = %o, want 0600", got)
	}
	if _, err := os.Stat(created); !os.IsNotExist(err) {
		t.Fatalf("created file still exists or stat failed unexpectedly: %v", err)
	}
}

func managedWriteTestDir(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp(wd, ".managed-write-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	return root
}
