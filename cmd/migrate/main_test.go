package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveMigrationsDirUsesConfiguredPath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeMigrationFile(t, dir, "000001_test.up.sql")

	got, err := resolveMigrationsDir(dir)
	if err != nil {
		t.Fatalf("resolveMigrationsDir() error = %v", err)
	}
	if got != dir {
		t.Fatalf("resolveMigrationsDir() = %q, want %q", got, dir)
	}
}

func TestResolveMigrationsDirRejectsEmptyConfiguredPath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	got, err := resolveMigrationsDir(filepath.Join(dir, "missing"))
	if err == nil {
		t.Fatalf("resolveMigrationsDir() = %q, want error", got)
	}
}

func TestMigrationDirCandidatesDeduplicate(t *testing.T) {
	t.Parallel()

	dir := filepath.Clean("migrations")
	candidates := migrationDirCandidates(dir)
	seen := map[string]bool{}
	for _, candidate := range candidates {
		if seen[candidate] {
			t.Fatalf("duplicate candidate %q in %#v", candidate, candidates)
		}
		seen[candidate] = true
	}
}

func writeMigrationFile(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("select 1;\n"), 0o600); err != nil {
		t.Fatalf("write migration file: %v", err)
	}
}
