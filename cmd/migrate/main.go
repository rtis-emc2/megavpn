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
)

func main() {
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
	files, err := filepath.Glob("migrations/*.up.sql")
	if err != nil {
		log.Fatal(err)
	}
	sort.Strings(files)
	if len(files) == 0 {
		fmt.Println("no migrations found")
		return
	}
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
