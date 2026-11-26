// Package database owns the Postgres connection pool and the schema migrations.
package database

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"

	"github.com/v1d1tt/talent-bridge/db"
	"github.com/v1d1tt/talent-bridge/internal/store"
)

// DB is the app's database handle.
type DB struct {
	pool    *pgxpool.Pool
	Queries *store.Queries

	mu            sync.Mutex
	schemaVersion int64 // 0 until the migrations ran in this process
}

// Open parses the URL and builds a pool. It does not connect: the first query
// does. So the server can start (and show a red status) while the database is
// down or asleep.
func Open(ctx context.Context, url string) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	// Cloud Run scales to zero and runs few instances; a small pool per instance
	// stays far below the Neon connection limit.
	cfg.MaxConns = 4
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	return &DB{pool: pool, Queries: store.New(pool)}, nil
}

// Close closes all connections.
func (d *DB) Close() { d.pool.Close() }

// Probe makes sure the schema is migrated, then runs a real query against it.
// It returns the schema version.
func (d *DB) Probe(ctx context.Context) (int64, error) {
	v, err := d.migrate(ctx)
	if err != nil {
		return 0, err
	}
	if _, err := d.Queries.CountUsers(ctx); err != nil {
		return 0, fmt.Errorf("query users: %w", err)
	}
	return v, nil
}

// migrate applies pending goose migrations once per process. A failed attempt
// (for example, the database was asleep) is retried on the next call.
func (d *DB) migrate(ctx context.Context) (int64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.schemaVersion > 0 {
		return d.schemaVersion, nil
	}

	// A Postgres advisory lock makes two instances that start together take
	// turns, so each migration runs exactly once.
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return 0, err
	}
	sqlDB := stdlib.OpenDBFromPool(d.pool)
	// Closing this wrapper does not close the shared pool.
	defer func() { _ = sqlDB.Close() }()

	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, db.Migrations(),
		goose.WithSessionLocker(locker))
	if err != nil {
		return 0, err
	}
	if _, err := provider.Up(ctx); err != nil {
		return 0, fmt.Errorf("migrate: %w", err)
	}
	v, err := provider.GetDBVersion(ctx)
	if err != nil {
		return 0, err
	}
	d.schemaVersion = v
	return v, nil
}

// ErrNotConfigured is the probe error when DATABASE_URL is empty.
var ErrNotConfigured = errors.New("DATABASE_URL is not set")

// NotConfigured is the probe the server uses when there is no database URL.
type NotConfigured struct{}

// Probe always fails with ErrNotConfigured.
func (NotConfigured) Probe(context.Context) (int64, error) { return 0, ErrNotConfigured }
