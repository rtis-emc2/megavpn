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
