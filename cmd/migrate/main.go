package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rtis-emc2/megavpn/internal/platform/config"
	"github.com/rtis-emc2/megavpn/internal/platform/database"
	platformversion "github.com/rtis-emc2/megavpn/internal/platform/version"
)

func main() {
	if platformversion.CommandRequested(os.Args[1:]) {
		fmt.Println(platformversion.Version)
		return
	}

	cfg := config.Load()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	db, err := database.Open(ctx, cfg.Database.DSN)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Pool.Exec(ctx, `create table if not exists schema_migrations(version text primary key, applied_at timestamptz not null default now())`); err != nil {
		log.Fatal(err)
	}
	if err := ensureSquashedMigrationCompatibility(ctx, db); err != nil {
		log.Fatal(err)
	}
	migrationsDir, err := resolveMigrationsDir(os.Getenv("MEGAVPN_MIGRATIONS_DIR"))
	if err != nil {
		log.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(migrationsDir, "*.up.sql"))
	if err != nil {
		log.Fatal(err)
	}
	sort.Strings(files)
	if len(files) == 0 {
		fmt.Println("no migrations found", migrationsDir)
		return
	}
	fmt.Println("migrations dir", migrationsDir)
	for _, f := range files {
		version := strings.TrimSuffix(filepath.Base(f), ".up.sql")
		var exists bool
		if err := db.Pool.QueryRow(ctx, `select exists(select 1 from schema_migrations where version=$1)`, version).Scan(&exists); err != nil {
			log.Fatal(err)
		}
		if exists {
			fmt.Println("skip", version)
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			log.Fatal(err)
		}
		tx, err := db.Pool.Begin(ctx)
		if err != nil {
			log.Fatal(err)
		}
		if _, err = tx.Exec(ctx, string(b)); err != nil {
			_ = tx.Rollback(ctx)
			log.Fatalf("migration %s failed: %v", version, err)
		}
		if _, err = tx.Exec(ctx, `insert into schema_migrations(version,applied_at) values($1,now())`, version); err != nil {
			_ = tx.Rollback(ctx)
			log.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			log.Fatal(err)
		}
		fmt.Println("applied", version)
	}
}

func ensureSquashedMigrationCompatibility(ctx context.Context, db *database.DB) error {
	const baselineVersion = "000001_control_plane"
	const latestLegacyVersion = "000044_vless_outbound_groups"
	const latestBaselineAuditAction = "migration.vless_outbound_groups"

	var applied int
	var hasBaseline bool
	var hasLatestLegacy bool
	if err := db.Pool.QueryRow(ctx, `
		select
		  count(*),
		  coalesce(bool_or(version = $1), false),
		  coalesce(bool_or(version = $2), false)
		from schema_migrations
	`, baselineVersion, latestLegacyVersion).Scan(&applied, &hasBaseline, &hasLatestLegacy); err != nil {
		return err
	}
	if applied == 0 || !hasBaseline || hasLatestLegacy {
		return nil
	}

	var hasAuditEvents bool
	if err := db.Pool.QueryRow(ctx, `select to_regclass(current_schema() || '.audit_events') is not null`).Scan(&hasAuditEvents); err != nil {
		return err
	}
	if hasAuditEvents {
		var hasBaselineState bool
		if err := db.Pool.QueryRow(ctx, `select exists(select 1 from audit_events where action = $1)`, latestBaselineAuditAction).Scan(&hasBaselineState); err != nil {
			return err
		}
		if hasBaselineState {
			return nil
		}
	}
	return fmt.Errorf("database has partial legacy migration history before the current migration baseline; apply the previous release migrations through %s before deploying this release", latestLegacyVersion)
}

func resolveMigrationsDir(configured string) (string, error) {
	if strings.TrimSpace(configured) != "" {
		dir := filepath.Clean(strings.TrimSpace(configured))
		if hasMigrationFiles(dir) {
			return dir, nil
		}
		return "", fmt.Errorf("configured migrations directory has no *.up.sql files: %s", dir)
	}
	for _, candidate := range migrationDirCandidates(configured) {
		if hasMigrationFiles(candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("migrations directory not found; set MEGAVPN_MIGRATIONS_DIR")
}

func migrationDirCandidates(configured string) []string {
	candidates := []string{}
	add := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		path = filepath.Clean(path)
		for _, existing := range candidates {
			if existing == path {
				return
			}
		}
		candidates = append(candidates, path)
	}

	add(configured)
	add("migrations")
	add("/opt/megavpn/migrations")
	if wd, err := os.Getwd(); err == nil {
		add(filepath.Join(wd, "migrations"))
	}
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		add(filepath.Join(exeDir, "migrations"))
		add(filepath.Join(exeDir, "..", "migrations"))
	}
	return candidates
}

func hasMigrationFiles(dir string) bool {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return false
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.up.sql"))
	return err == nil && len(files) > 0
}
