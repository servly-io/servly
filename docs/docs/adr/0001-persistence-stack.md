---
id: 0001-persistence-stack
title: "0001: Persistence stack"
---

**Status:** accepted, 2026-10-10

## Context

MASTER_PLAN.md requires SQLite (local) and PostgreSQL (cloud) behind `database/sql`, with explicit SQL, versioned migrations, and the same behavior on both. The release builds use `CGO_ENABLED=0` for static cross-compiled binaries, and that needs to stay.

## Decision

* **SQLite driver:** `modernc.org/sqlite`, pure Go, so no cgo. Opened with `foreign_keys=ON`, `journal_mode=WAL`, `synchronous=NORMAL`, `busy_timeout=5000` and `_txlock=immediate`, with at most 4 open connections.
* **PostgreSQL driver:** `github.com/jackc/pgx/v5/stdlib`, used through `database/sql`.
* **Migrations:** `github.com/pressly/goose/v3`, with the SQL embedded. There are two directories with identical version numbers: `migrations/sqlite` and `migrations/postgres`. Migrations run at startup by default, or with `servly migrate up`. PostgreSQL uses goose's advisory session lock.
* **SQL style:** one SQL text for both dialects where the semantics match, with `$1` placeholders (both drivers accept them, verified by test). Write a per-dialect variant only where the semantics differ.
* **Portable types:** `database.Timestamp` writes fixed-width UTC text that PostgreSQL parses into `timestamptz`, and reads both. `uuid.UUID` for ids. Booleans as Go `bool` (SQLite stores 0/1). `business_date` as `YYYY-MM-DD` text in both dialects.
* **Shared test harness:** `dbtest.Each` runs every persistence test on both backends. PostgreSQL runs in CI as a service container, and locally through `make test-integration`.

## Alternatives

* `mattn/go-sqlite3`: needs cgo, which complicates cross-compilation and static images.
* `sqlc`: the plan allows it. It generates per-dialect code, and the generator needs its own config per engine. It can be adopted later per package without changing the schema.
* `golang-migrate`: comparable. goose has a cleaner provider API for `embed.FS` and a PostgreSQL lock.
* An ORM: the plan rules it out (explicit SQL).

## Consequences

* modernc is slower than the cgo driver. That is irrelevant at restaurant load.
* Schema changes are written twice, and the test `TestMigrateIsCompleteAndRepeatable` checks that both dialects create the same tables. Columns are checked by the repository tests that run on both backends.
* `TEXT` timestamps in SQLite order correctly only because of the fixed-width layout. Always use `database.Timestamp`; never pass a raw `time.Time`.
