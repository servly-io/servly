package app_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/servly-io/servly/internal/ids"
	"github.com/servly-io/servly/internal/platform/database"
	"github.com/servly-io/servly/internal/platform/database/dbtest"
	"github.com/servly-io/servly/internal/storage"
)

// Images older versions kept on disk are copied into the database at start.
func TestImportAssetFiles(t *testing.T) {
	db := dbtest.Open(t, database.SQLite)
	ctx := context.Background()
	a := newApp(t, db)
	res, err := a.CreateTenant(ctx, roma)
	if err != nil {
		t.Fatal(err)
	}
	files := storage.NewLocal(t.TempDir())
	onDisk, lost := ids.New(), ids.New()
	for _, id := range []uuid.UUID{onDisk, lost} {
		if _, err := db.ExecContext(ctx, `INSERT INTO assets (id, tenant_id, storage_key, content_type, size_bytes, sha256, created_at)
			VALUES ($1, $2, $3, 'image/png', 3, 'x', $4)`, id, res.Tenant.ID, storage.Key(res.Tenant.ID, id), database.Now()); err != nil {
			t.Fatal(err)
		}
	}
	if err := files.Put(storage.Key(res.Tenant.ID, onDisk), []byte("png")); err != nil {
		t.Fatal(err)
	}
	if err := a.ImportAssetFiles(ctx, files); err != nil {
		t.Fatal(err)
	}
	var data []byte
	_ = db.QueryRowContext(ctx, `SELECT data FROM assets WHERE id = $1`, onDisk).Scan(&data)
	if string(data) != "png" {
		t.Fatalf("imported %q", data)
	}
	if err := db.QueryRowContext(ctx, `SELECT data FROM assets WHERE id = $1`, lost).Scan(&data); err != nil || data != nil {
		t.Fatalf("lost file: %q %v", data, err)
	}
}
