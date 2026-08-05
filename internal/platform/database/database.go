package database

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
)

type DB struct{ Pool *pgxpool.Pool }

func Open(ctx context.Context, dsn string) (*DB, error) {
	if dsn == "" {
		return nil, errors.New("database DSN is empty: set MEGAVPN_DATABASE_DSN")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &DB{Pool: pool}, nil
}
func (d *DB) Close() {
	if d != nil && d.Pool != nil {
		d.Pool.Close()
	}
}
func (d *DB) Ping(ctx context.Context) error { return d.Pool.Ping(ctx) }
