package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	apihttp "github.com/rtis-emc2/megavpn/internal/api/http"
	authn "github.com/rtis-emc2/megavpn/internal/auth"
	"github.com/rtis-emc2/megavpn/internal/infra/postgres"
	"github.com/rtis-emc2/megavpn/internal/platform/config"
	"github.com/rtis-emc2/megavpn/internal/platform/database"
	platformversion "github.com/rtis-emc2/megavpn/internal/platform/version"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "reset-password":
		if err := resetPassword(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "megavpn-admin: %v\n", err)
			os.Exit(1)
		}
	case "seed-service-packs":
		if err := seedServicePacks(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "megavpn-admin: %v\n", err)
			os.Exit(1)
		}
	case "version", "--version", "-version":
		fmt.Println(platformversion.Version)
	case "help", "--help", "-h":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "megavpn-admin: unknown command %q\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `MegaVPN administrative maintenance utility

Usage:
  megavpn-admin reset-password [flags]
  megavpn-admin seed-service-packs [flags]
  megavpn-admin version

Commands:
  reset-password  Reset an existing local platform user's password.
  seed-service-packs
                  Insert built-in service pack templates when the catalog table exists.

`)
}

func resetPassword(args []string) error {
	fs := flag.NewFlagSet("reset-password", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		databaseDSN = fs.String("database-dsn", "", "PostgreSQL DSN; defaults to MEGAVPN_DATABASE_DSN")
		envFile     = fs.String("env-file", "/etc/megavpn/megavpn.env", "runtime environment file loaded before reading env vars; empty disables file loading")
		login       = fs.String("login", "superadmin", "platform username or email")
		passwordEnv = fs.String("password-env", "MEGAVPN_ADMIN_PASSWORD", "environment variable containing the new password")
		activate    = fs.Bool("activate", false, "set the user status to active after reset")
		timeout     = fs.Duration("timeout", 10*time.Second, "database operation timeout")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := loadAdminEnvFile(*envFile); err != nil {
		return err
	}

	loginValue := strings.TrimSpace(*login)
	if loginValue == "" {
		return fmt.Errorf("login is required")
	}

	password := os.Getenv(strings.TrimSpace(*passwordEnv))
	if strings.TrimSpace(password) == "" {
		return fmt.Errorf("new password is required in %s", strings.TrimSpace(*passwordEnv))
	}
	passwordHash, err := authn.HashPassword(password)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	dsn, err := resolveAdminDatabaseDSN(*databaseDSN)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer db.Close()

	store := postgres.New(db.Pool)
	record, err := store.GetPlatformUserForAuth(ctx, loginValue)
	if err != nil {
		return fmt.Errorf("find platform user %q: %w", loginValue, err)
	}
	if err := store.UpdatePlatformUserPassword(ctx, record.User.ID, passwordHash, nil); err != nil {
		return fmt.Errorf("update platform user password: %w", err)
	}
	if *activate && record.User.Status != "active" {
		if _, err := store.UpdatePlatformUserStatus(ctx, record.User.ID, "active", nil); err != nil {
			return fmt.Errorf("activate platform user: %w", err)
		}
		record.User.Status = "active"
	}

	fmt.Printf("password reset completed: username=%s user_id=%s status=%s activate=%t\n", record.User.Username, record.User.ID, record.User.Status, *activate)
	return nil
}

func seedServicePacks(args []string) error {
	fs := flag.NewFlagSet("seed-service-packs", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		databaseDSN = fs.String("database-dsn", "", "PostgreSQL DSN; defaults to MEGAVPN_DATABASE_DSN")
		envFile     = fs.String("env-file", "/etc/megavpn/megavpn.env", "runtime environment file loaded before reading env vars; empty disables file loading")
		timeout     = fs.Duration("timeout", 10*time.Second, "database operation timeout")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := loadAdminEnvFile(*envFile); err != nil {
		return err
	}

	dsn, err := resolveAdminDatabaseDSN(*databaseDSN)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer db.Close()

	store := postgres.New(db.Pool)
	defaults := apihttp.DefaultServicePackDefinitions()
	if err := store.EnsureDefaultServicePacks(ctx, defaults); err != nil {
		return fmt.Errorf("seed service pack templates: %w", err)
	}
	active, err := store.ListServicePacks(ctx)
	if err != nil {
		return fmt.Errorf("verify service pack templates: %w", err)
	}

	fmt.Printf("service pack defaults seeded: defaults=%d active=%d\n", len(defaults), len(active))
	return nil
}

func resolveAdminDatabaseDSN(flagValue string) (string, error) {
	dsn := strings.TrimSpace(flagValue)
	if dsn == "" {
		dsn = strings.TrimSpace(config.Load().Database.DSN)
	}
	if dsn == "" {
		return "", fmt.Errorf("database DSN is empty: set MEGAVPN_DATABASE_DSN, pass --database-dsn, or provide --env-file")
	}
	return dsn, nil
}

func loadAdminEnvFile(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("open env file %s: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for lineNo := 1; scanner.Scan(); lineNo++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		key, raw, found := strings.Cut(line, "=")
		if !found {
			return fmt.Errorf("parse env file %s:%d: expected KEY=VALUE", path, lineNo)
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return fmt.Errorf("parse env file %s:%d: empty key", path, lineNo)
		}
		value, err := parseAdminEnvValue(raw)
		if err != nil {
			return fmt.Errorf("parse env file %s:%d: %w", path, lineNo, err)
		}
		if existing, exists := os.LookupEnv(key); !exists || strings.TrimSpace(existing) == "" {
			if err := os.Setenv(key, value); err != nil {
				return fmt.Errorf("set env %s from %s:%d: %w", key, path, lineNo, err)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read env file %s: %w", path, err)
	}
	return nil
}

func parseAdminEnvValue(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	var out strings.Builder
	for i := 0; i < len(value); {
		switch value[i] {
		case '\'':
			i++
			start := i
			for i < len(value) && value[i] != '\'' {
				i++
			}
			if i >= len(value) {
				return "", fmt.Errorf("unterminated single-quoted value")
			}
			out.WriteString(value[start:i])
			i++
		case '"':
			i++
			for i < len(value) && value[i] != '"' {
				if value[i] == '\\' && i+1 < len(value) {
					i++
				}
				out.WriteByte(value[i])
				i++
			}
			if i >= len(value) {
				return "", fmt.Errorf("unterminated double-quoted value")
			}
			i++
		case '\\':
			if i+1 >= len(value) {
				return "", fmt.Errorf("dangling escape in value")
			}
			i++
			out.WriteByte(value[i])
			i++
		case '#':
			if strings.TrimSpace(value[:i]) == "" || value[i-1] == ' ' || value[i-1] == '\t' {
				return strings.TrimSpace(out.String()), nil
			}
			out.WriteByte(value[i])
			i++
		default:
			out.WriteByte(value[i])
			i++
		}
	}
	return strings.TrimSpace(out.String()), nil
}
