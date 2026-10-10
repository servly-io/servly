// Package dbtest gives tests a fresh, migrated database for each backend.
//
// SQLite always runs. PostgreSQL runs when SERVLY_TEST_POSTGRES_URL is set;
// each test then gets its own schema, dropped when the test ends.
// `make test-integration` starts a PostgreSQL container and sets the variable.
package dbtest

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"io"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/servly-io/servly/internal/platform/database"
)

// PostgresURLEnv names the variable holding the PostgreSQL test server URL.
const PostgresURLEnv = "SERVLY_TEST_POSTGRES_URL"

// Each runs fn once per available backend, as a subtest named after the
// dialect, with a freshly migrated database.
func Each(t *testing.T, fn func(t *testing.T, db *database.DB)) {
	t.Helper()
	t.Run(string(database.SQLite), func(t *testing.T) { fn(t, Open(t, database.SQLite)) })
	t.Run(string(database.Postgres), func(t *testing.T) {
		if os.Getenv(PostgresURLEnv) == "" {
			t.Skipf("%s not set", PostgresURLEnv)
		}
		fn(t, Open(t, database.Postgres))
	})
}

// Open returns a freshly migrated database of the given dialect.
func Open(t *testing.T, dialect database.Dialect) *database.DB {
	t.Helper()
	db := OpenEmpty(t, dialect)
	if _, err := db.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// OpenEmpty returns a database of the given dialect with no schema applied.
func OpenEmpty(t *testing.T, dialect database.Dialect) *database.DB {
	t.Helper()
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	var (
		db  *database.DB
		err error
	)
	switch dialect {
	case database.SQLite:
		db, err = database.OpenSQLite(ctx, filepath.Join(t.TempDir(), "test.db"), log)
	case database.Postgres:
		db, err = database.OpenPostgres(ctx, postgresSchema(t), log)
	default:
		t.Fatalf("unknown dialect %q", dialect)
	}
	if err != nil {
		t.Fatalf("open %s: %v", dialect, err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// postgresSchema creates a throwaway schema and returns a URL whose
// connections use it as their search_path.
func postgresSchema(t *testing.T) string {
	t.Helper()
	base := os.Getenv(PostgresURLEnv)
	if base == "" {
		t.Skipf("%s not set", PostgresURLEnv)
	}

	b := make([]byte, 8)
	_, _ = rand.Read(b)
	schema := "test_" + hex.EncodeToString(b)

	admin, err := sql.Open("pgx", base)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	if _, err := admin.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	// Registered after admin's Close, so it runs first: drop, then close.
	t.Cleanup(func() {
		if _, err := admin.Exec("DROP SCHEMA " + schema + " CASCADE"); err != nil {
			t.Errorf("drop schema %s: %v", schema, err)
		}
	})

	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("parse %s: %v", PostgresURLEnv, err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	return u.String()
}
