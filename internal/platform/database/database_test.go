package database_test

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/servly-io/servly/internal/platform/database"
	"github.com/servly-io/servly/internal/platform/database/dbtest"
)

// The tables of docs/docs/domain-model.md. Both migration sets must create
// exactly these.
var wantTables = []string{
	"assets", "audit_events", "dining_locations", "idempotency_records", "installation",
	"menu_categories", "menu_items", "option_groups", "options",
	"order_item_options", "order_items", "order_number_counters",
	"order_status_events", "orders", "payment_accounts", "payment_checkouts", "payment_events",
	"payments", "restaurant_locations",
	"sessions", "tenant_domains", "tenant_memberships", "tenants", "users",
}

func TestMigrateIsCompleteAndRepeatable(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *database.DB) {
		ctx := context.Background()
		state, err := db.MigrationState(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if state.Pending() || state.Latest < 1 {
			t.Fatalf("after migrate: %+v", state)
		}
		again, err := db.Migrate(ctx)
		if err != nil {
			t.Fatalf("second migrate: %v", err)
		}
		if again != state {
			t.Fatalf("second migrate changed state: %+v -> %+v", state, again)
		}
		if got := tables(t, db); !slices.Equal(got, wantTables) {
			t.Fatalf("tables:\n got %v\nwant %v", got, wantTables)
		}
	})
}

func TestEmptyDatabaseReportsPending(t *testing.T) {
	db := dbtest.OpenEmpty(t, database.SQLite)
	state, err := db.MigrationState(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !state.Pending() {
		t.Fatalf("empty database not pending: %+v", state)
	}
}

func TestTimestampRoundTrip(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *database.DB) {
		ctx := context.Background()
		id := uuid.New()
		at := database.Timestamp{Time: time.Date(2026, 10, 10, 18, 30, 1, 123456789, time.FixedZone("CEST", 2*3600))}
		if _, err := db.ExecContext(ctx,
			`INSERT INTO tenants (id, slug, name, status, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $5)`,
			id, "t", "T", "active", at); err != nil {
			t.Fatal(err)
		}
		var got database.Timestamp
		if err := db.QueryRowContext(ctx, `SELECT created_at FROM tenants WHERE id = $1`, id).Scan(&got); err != nil {
			t.Fatal(err)
		}
		want := at.UTC().Truncate(time.Microsecond)
		if !got.Equal(want) || got.Location() != time.UTC {
			t.Fatalf("got %v, want %v", got.Time, want)
		}
	})
}

func TestInTx(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *database.DB) {
		ctx := context.Background()
		insert := func(tx *sql.Tx, slug string) error {
			now := database.Now()
			_, err := tx.ExecContext(ctx,
				`INSERT INTO tenants (id, slug, name, status, created_at, updated_at) VALUES ($1, $2, $2, 'active', $3, $3)`,
				uuid.New(), slug, now)
			return err
		}
		boom := errors.New("boom")
		err := db.InTx(ctx, func(tx *sql.Tx) error {
			if err := insert(tx, "rolled-back"); err != nil {
				return err
			}
			return boom
		})
		if !errors.Is(err, boom) {
			t.Fatalf("InTx error = %v, want boom", err)
		}
		if err := db.InTx(ctx, func(tx *sql.Tx) error { return insert(tx, "committed") }); err != nil {
			t.Fatal(err)
		}
		var slugs []string
		rows, err := db.QueryContext(ctx, `SELECT slug FROM tenants ORDER BY slug`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				t.Fatal(err)
			}
			slugs = append(slugs, s)
		}
		if !slices.Equal(slugs, []string{"committed"}) {
			t.Fatalf("tenants = %v", slugs)
		}
	})
}

// The composite (tenant_id, …) foreign keys make it impossible to attach a
// record to a parent that belongs to another tenant.
func TestSchemaRejectsCrossTenantReferences(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *database.DB) {
		ctx := context.Background()
		now := database.Now()
		tenantA, tenantB, locationA := uuid.New(), uuid.New(), uuid.New()
		mustExec(t, db, `INSERT INTO tenants (id, slug, name, status, created_at, updated_at) VALUES ($1, 'a', 'A', 'active', $2, $2)`, tenantA, now)
		mustExec(t, db, `INSERT INTO tenants (id, slug, name, status, created_at, updated_at) VALUES ($1, 'b', 'B', 'active', $2, $2)`, tenantB, now)
		mustExec(t, db, `INSERT INTO restaurant_locations (id, tenant_id, name, currency, timezone, status, created_at, updated_at)
			VALUES ($1, $2, 'A', 'EUR', 'Europe/Berlin', 'active', $3, $3)`, locationA, tenantA, now)

		insertCategory := `INSERT INTO menu_categories (id, tenant_id, location_id, name, position, created_at, updated_at)
			VALUES ($1, $2, $3, 'Pizza', 1, $4, $4)`
		if _, err := db.ExecContext(ctx, insertCategory, uuid.New(), tenantB, locationA, now); err == nil {
			t.Fatal("category of tenant B under location of tenant A was accepted")
		}
		mustExec(t, db, insertCategory, uuid.New(), tenantA, locationA, now)
	})
}

func mustExec(t *testing.T, db *database.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), query, args...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

func tables(t *testing.T, db *database.DB) []string {
	t.Helper()
	query := `SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' AND name <> 'goose_db_version' ORDER BY name`
	if db.Dialect == database.Postgres {
		query = `SELECT table_name FROM information_schema.tables WHERE table_schema = current_schema() AND table_name <> 'goose_db_version' ORDER BY table_name`
	}
	rows, err := db.QueryContext(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		names = append(names, n)
	}
	return names
}

func TestZeroTimestampIsNull(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *database.DB) {
		ctx := context.Background()
		var got database.Timestamp
		got.Time = time.Now()
		if err := db.QueryRowContext(ctx, `SELECT archived_at FROM menu_items WHERE 1 = 0 UNION ALL SELECT NULL`).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if !got.IsZero() {
			t.Fatalf("NULL scanned as %v", got.Time)
		}
		_, err := db.ExecContext(ctx,
			`INSERT INTO tenants (id, slug, name, status, created_at, updated_at) VALUES ($1, 'z', 'Z', 'active', $2, $2)`,
			uuid.New(), database.Timestamp{})
		if err == nil {
			t.Fatal("zero timestamp accepted in NOT NULL column")
		}
	})
}
