// Package store owns the PostgreSQL connection pool and the embedded schema
// migrations. It is the only package that opens a database connection.
//
// Migrations are numbered, forward-only goose SQL files under ./migrations,
// embedded in the binary so a container needs no migration volume. They are
// applied at start-up, before the reference seed runs.
package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Store wraps the connection pool and the migration entry points.
type Store struct {
	pool *pgxpool.Pool
}

// Open parses the connection string, creates the pool and verifies it can
// reach the database. The caller owns Close.
func Open(ctx context.Context, databaseURL string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return &Store{pool: pool}, nil
}

// Pool exposes the underlying pool for repositories in other packages.
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// Close releases the pool.
func (s *Store) Close() {
	if s.pool != nil {
		s.pool.Close()
	}
}

// Migrate applies every embedded migration that has not run yet. It is
// idempotent: goose records applied versions in its own table.
//
// goose operates on a database/sql handle, so this opens a short-lived one via
// the pgx stdlib driver. The long-lived application queries still use the pgx
// pool; only migrations go through database/sql.
func (s *Store) Migrate(ctx context.Context) error {
	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}

	connStr := s.pool.Config().ConnString()
	db, err := sql.Open("pgx", connStr)
	if err != nil {
		return fmt.Errorf("open sql handle for migrations: %w", err)
	}
	defer db.Close()

	if err := goose.UpContext(ctx, db, "migrations"); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}
