// Package dbtest creates a throwaway, fully migrated database per test.
//
// Set TEST_DATABASE_URL (or DATABASE_URL) to a server where the user may
// CREATE DATABASE. Tests are skipped when neither is set.
package dbtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"log/slog"
	"net/url"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ChandraShekar2001/application-tracker/internal/db"
)

// URL returns the base server URL or skips the test.
func URL(t testing.TB) string {
	t.Helper()
	u := os.Getenv("TEST_DATABASE_URL")
	if u == "" {
		u = os.Getenv("DATABASE_URL")
	}
	if u == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database test")
	}
	return u
}

// NewDatabase creates an empty database and returns its URL. It is dropped when the test ends.
func NewDatabase(t testing.TB) string {
	t.Helper()
	base := URL(t)
	ctx := context.Background()

	var b [6]byte
	_, _ = rand.Read(b[:])
	name := "test_" + hex.EncodeToString(b[:])

	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("create database: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
		_ = admin.Close(context.Background())
	})

	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	u.Path = "/" + name
	return u.String()
}

// New returns a pool on a fresh, migrated database.
func New(t testing.TB) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	pool, err := db.Open(ctx, NewDatabase(t))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := db.Migrate(ctx, pool, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return pool
}
