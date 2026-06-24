package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

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
  megavpn-admin version

Commands:
  reset-password  Reset an existing local platform user's password.

`)
}

func resetPassword(args []string) error {
	fs := flag.NewFlagSet("reset-password", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		databaseDSN = fs.String("database-dsn", "", "PostgreSQL DSN; defaults to MEGAVPN_DATABASE_DSN")
		login       = fs.String("login", "superadmin", "platform username or email")
		passwordEnv = fs.String("password-env", "MEGAVPN_ADMIN_PASSWORD", "environment variable containing the new password")
		activate    = fs.Bool("activate", false, "set the user status to active after reset")
		timeout     = fs.Duration("timeout", 10*time.Second, "database operation timeout")
	)
	if err := fs.Parse(args); err != nil {
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

	dsn := strings.TrimSpace(*databaseDSN)
	if dsn == "" {
		dsn = strings.TrimSpace(config.Load().Database.DSN)
	}
	if dsn == "" {
		return fmt.Errorf("database DSN is empty: set MEGAVPN_DATABASE_DSN or pass --database-dsn")
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
