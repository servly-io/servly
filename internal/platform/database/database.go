// Package database opens the configured SQL backend, applies migrations and
// provides the transaction helper the repositories build on.
//
// Repositories write one SQL text for both dialects where the semantics
// match, using $1-style placeholders (both drivers accept them), and keep a
// per-dialect variant only where they differ. See docs/docs/architecture.md.
package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
	_ "modernc.org/sqlite" // registers the "sqlite" driver

	"github.com/servly-io/servly/migrations"
)

// Dialect names a supported SQL backend.
type Dialect string

const (
	SQLite   Dialect = "sqlite"
	Postgres Dialect = "postgres"
)

// DB is an open database handle together with its dialect.
type DB struct {
	*sql.DB
	Dialect Dialect
	log     *slog.Logger
}

// Querier is satisfied by *sql.DB and *sql.Tx. Repositories take a Querier so
// the same code runs inside or outside a transaction.
type Querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// OpenSQLite opens (creating if needed) the SQLite database file at path.
func OpenSQLite(ctx context.Context, path string, log *slog.Logger) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}
	q := url.Values{}
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "synchronous(NORMAL)")
	q.Add("_pragma", "busy_timeout(5000)")
	// BEGIN IMMEDIATE: a write transaction takes the write lock up front,
	// instead of failing with SQLITE_BUSY when it upgrades from a read.
	q.Set("_txlock", "immediate")
	return open(ctx, SQLite, "sqlite", "file:"+path+"?"+q.Encode(), log)
}

// OpenPostgres opens a PostgreSQL connection pool for the given URL.
func OpenPostgres(ctx context.Context, dsn string, log *slog.Logger) (*DB, error) {
	return open(ctx, Postgres, "pgx", dsn, log)
}

func open(ctx context.Context, dialect Dialect, driver, dsn string, log *slog.Logger) (*DB, error) {
	sqlDB, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", dialect, err)
	}
	if dialect == SQLite {
		// One writer at a time is a property of SQLite; a small pool keeps
		// WAL readers concurrent without piling up blocked writers.
		sqlDB.SetMaxOpenConns(4)
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("connect to %s: %w", dialect, err)
	}
	return &DB{DB: sqlDB, Dialect: dialect, log: log}, nil
}

// InTx runs fn in a transaction. It commits when fn returns nil and rolls back
// otherwise, including when fn panics.
func (db *DB) InTx(ctx context.Context, fn func(tx *sql.Tx) error) (err error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
				err = errors.Join(err, fmt.Errorf("rollback: %w", rbErr))
			}
		}
	}()
	if err = fn(tx); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// MigrationState reports the applied and the latest available schema version.
type MigrationState struct {
	Current int64
	Latest  int64
}

// Pending reports whether migrations remain to be applied.
func (s MigrationState) Pending() bool { return s.Current < s.Latest }

// Migrate applies every pending migration and returns the resulting state.
func (db *DB) Migrate(ctx context.Context) (MigrationState, error) {
	p, err := db.provider()
	if err != nil {
		return MigrationState{}, err
	}
	results, err := p.Up(ctx)
	for _, r := range results {
		db.log.Info("migration applied", "version", r.Source.Version, "path", r.Source.Path, "duration", r.Duration)
	}
	if err != nil {
		return MigrationState{}, fmt.Errorf("apply migrations: %w", err)
	}
	return db.migrationState(ctx, p)
}

// MigrationState reports the schema version without changing anything.
func (db *DB) MigrationState(ctx context.Context) (MigrationState, error) {
	p, err := db.provider()
	if err != nil {
		return MigrationState{}, err
	}
	return db.migrationState(ctx, p)
}

func (db *DB) migrationState(ctx context.Context, p *goose.Provider) (MigrationState, error) {
	current, latest, err := p.GetVersions(ctx)
	if err != nil {
		return MigrationState{}, fmt.Errorf("read schema version: %w", err)
	}
	return MigrationState{Current: current, Latest: latest}, nil
}

func (db *DB) provider() (*goose.Provider, error) {
	var (
		gd   goose.Dialect
		fsys fs.FS
		opts []goose.ProviderOption
	)
	switch db.Dialect {
	case SQLite:
		gd, fsys = goose.DialectSQLite3, migrations.SQLite()
	case Postgres:
		gd, fsys = goose.DialectPostgres, migrations.Postgres()
		// Several instances may start against one database; an advisory
		// lock keeps them from migrating concurrently.
		locker, err := lock.NewPostgresSessionLocker()
		if err != nil {
			return nil, fmt.Errorf("migration lock: %w", err)
		}
		opts = append(opts, goose.WithSessionLocker(locker))
	default:
		return nil, fmt.Errorf("unsupported dialect %q", db.Dialect)
	}
	p, err := goose.NewProvider(gd, db.DB, fsys, opts...)
	if err != nil {
		return nil, fmt.Errorf("load migrations: %w", err)
	}
	return p, nil
}
