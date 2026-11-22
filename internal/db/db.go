// Package db owns the connection pool and schema migrations.
package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Open creates a pgx pool and verifies connectivity.
func Open(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	cfg.MaxConns = 10
	cfg.MaxConnIdleTime = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}

// Migrate applies all pending embedded migrations.
func Migrate(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger) error {
	sqlDB := stdlib.OpenDBFromPool(pool)
	defer sqlDB.Close()
	return migrate(ctx, sqlDB, log)
}

// MigrateRoundTrip applies all migrations, rolls every one back, then applies
// them again. Used by tests to enforce that migrations are reversible.
func MigrateRoundTrip(ctx context.Context, pool *pgxpool.Pool) error {
	sqlDB := stdlib.OpenDBFromPool(pool)
	defer sqlDB.Close()
	p, err := newProvider(sqlDB)
	if err != nil {
		return err
	}
	if _, err := p.Up(ctx); err != nil {
		return fmt.Errorf("up: %w", err)
	}
	if _, err := p.DownTo(ctx, 0); err != nil {
		return fmt.Errorf("down: %w", err)
	}
	if _, err := p.Up(ctx); err != nil {
		return fmt.Errorf("up again: %w", err)
	}
	return nil
}

func newProvider(sqlDB *sql.DB) (*goose.Provider, error) {
	p, err := goose.NewProvider(goose.DialectPostgres, sqlDB, mustSub(migrationsFS, "migrations"))
	if err != nil {
		return nil, fmt.Errorf("goose provider: %w", err)
	}
	return p, nil
}

func migrate(ctx context.Context, sqlDB *sql.DB, log *slog.Logger) error {
	provider, err := newProvider(sqlDB)
	if err != nil {
		return err
	}
	results, err := provider.Up(ctx)
	if err != nil {
		return fmt.Errorf("migrate up: %w", err)
	}
	for _, r := range results {
		log.Info("migration applied", "version", r.Source.Version, "file", r.Source.Path, "duration", r.Duration)
	}
	return nil
}
