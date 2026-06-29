package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestParseAdminEnvValue(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "single quoted",
			raw:  "'postgres://user:password@127.0.0.1:5432/megavpn?sslmode=disable'",
			want: "postgres://user:password@127.0.0.1:5432/megavpn?sslmode=disable",
		},
		{
			name: "single quote escape",
			raw:  "'postgres://user:p'\\''wd@127.0.0.1:5432/megavpn?sslmode=disable'",
			want: "postgres://user:p'wd@127.0.0.1:5432/megavpn?sslmode=disable",
		},
		{
			name: "unquoted comment",
			raw:  "value # comment",
			want: "value",
		},
		{
			name: "hash inside token",
			raw:  "value#fragment",
			want: "value#fragment",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseAdminEnvValue(tt.raw)
			if err != nil {
				t.Fatalf("parseAdminEnvValue() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("parseAdminEnvValue() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLoadAdminEnvFileDoesNotOverrideExistingEnv(t *testing.T) {
	t.Setenv("MEGAVPN_DATABASE_DSN", "from-shell")
	envPath := filepath.Join(t.TempDir(), "runtime.env")
	content := "MEGAVPN_DATABASE_DSN='from-file'\nMEGAVPN_PUBLIC_BASE_URL='https://control.example.com'\n"
	if err := os.WriteFile(envPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}
	if err := loadAdminEnvFile(envPath); err != nil {
		t.Fatalf("loadAdminEnvFile() error = %v", err)
	}
	if got := os.Getenv("MEGAVPN_DATABASE_DSN"); got != "from-shell" {
		t.Fatalf("MEGAVPN_DATABASE_DSN = %q, want shell value", got)
	}
}

func TestLoadAdminEnvFileSetsMissingEnv(t *testing.T) {
	key := "MEGAVPN_TEST_ADMIN_ENV_FILE_VALUE"
	old, hadOld := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unset env: %v", err)
	}
	t.Cleanup(func() {
		if hadOld {
			_ = os.Setenv(key, old)
		} else {
			_ = os.Unsetenv(key)
		}
	})
	envPath := filepath.Join(t.TempDir(), "runtime.env")
	if err := os.WriteFile(envPath, []byte(key+"='from-file'\n"), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}
	if err := loadAdminEnvFile(envPath); err != nil {
		t.Fatalf("loadAdminEnvFile() error = %v", err)
	}
	if got := os.Getenv(key); got != "from-file" {
		t.Fatalf("%s = %q, want from-file", key, got)
	}
}

func TestPrepareBinaryArtifactImportCopiesFileAndCalculatesSHA(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(t.TempDir(), "xray-install.sh")
	content := []byte("#!/bin/sh\nexit 0\n")
	if err := os.WriteFile(source, content, 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	artifact, err := prepareBinaryArtifactImport(root, binaryArtifactImportRequest{
		SourceFile:   source,
		ServiceCode:  "xray-core",
		Version:      "1.8.24",
		Architecture: "amd64",
		InstallMode:  "xray_install_script",
	})
	if err != nil {
		t.Fatalf("prepare import: %v", err)
	}
	if artifact.Kind != "script" {
		t.Fatalf("kind = %q, want script", artifact.Kind)
	}
	if artifact.StoragePath == "" || filepath.IsAbs(artifact.StoragePath) {
		t.Fatalf("storage_path = %q, want relative path", artifact.StoragePath)
	}
	got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(artifact.StoragePath)))
	if err != nil {
		t.Fatalf("read copied artifact: %v", err)
	}
	if string(got) != string(content) {
		t.Fatalf("copied content = %q, want %q", string(got), string(content))
	}
	sum := sha256.Sum256(content)
	if artifact.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("sha256 = %q, want %q", artifact.SHA256, hex.EncodeToString(sum[:]))
	}
	if artifact.SizeBytes != int64(len(content)) {
		t.Fatalf("size_bytes = %d, want %d", artifact.SizeBytes, len(content))
	}
	if artifact.Metadata["install_mode"] != "xray_install_script" {
		t.Fatalf("metadata = %#v, want install_mode", artifact.Metadata)
	}
}

func TestCleanRelativeRepositoryPathRejectsTraversal(t *testing.T) {
	for _, path := range []string{"../xray", "runtime/../../xray", "/absolute/xray", ""} {
		if _, err := cleanRelativeRepositoryPath(path); err == nil {
			t.Fatalf("cleanRelativeRepositoryPath(%q) succeeded, want error", path)
		}
	}
}

func TestPrepareBinaryArtifactImportRejectsExistingFileWithoutReplace(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(t.TempDir(), "runtime.bin")
	if err := os.WriteFile(source, []byte("one"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	req := binaryArtifactImportRequest{
		SourceFile:   source,
		ServiceCode:  "shadowsocks",
		Version:      "1.0.0",
		Architecture: "amd64",
		StoragePath:  "runtime-repository/shadowsocks/amd64/runtime.bin",
	}
	if _, err := prepareBinaryArtifactImport(root, req); err != nil {
		t.Fatalf("initial import: %v", err)
	}
	if _, err := prepareBinaryArtifactImport(root, req); err == nil {
		t.Fatal("second import succeeded without replace, want error")
	}
	req.ReplaceFile = true
	if _, err := prepareBinaryArtifactImport(root, req); err != nil {
		t.Fatalf("replace import: %v", err)
	}
}
