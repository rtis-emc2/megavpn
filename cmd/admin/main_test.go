package main

import (
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
