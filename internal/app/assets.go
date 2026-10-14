package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/google/uuid"

	"github.com/servly-io/servly/internal/apperr"
	"github.com/servly-io/servly/internal/ids"
	"github.com/servly-io/servly/internal/platform/database"
	"github.com/servly-io/servly/internal/storage"
)

// DefaultMaxImageBytes is the upload limit when the operator sets none
// (uploads.max_image_bytes): a well-compressed menu photo fits in 1 MiB.
const DefaultMaxImageBytes = 1 << 20

var (
	ErrAssetTooLarge = apperr.New(apperr.KindTooLarge, "asset_too_large", "the image is too large")
	ErrAssetType     = apperr.Invalid("unsupported_media_type", "upload a JPEG, PNG or WebP image")
	ErrAssetNotFound = apperr.NotFound("not_found", "not found")
)

// allowedTypes are detected from the content, never from the file name or
// the client's Content-Type.
var allowedTypes = map[string]bool{"image/jpeg": true, "image/png": true, "image/webp": true}

// Asset is an uploaded image's record. The image bytes live in the same row
// (ADR 0009), so every server instance can serve them.
type Asset struct {
	ID          uuid.UUID
	TenantID    uuid.UUID
	StorageKey  string
	ContentType string
	Size        int64
	SHA256      string
	CreatedAt   database.Timestamp
}

// ImageLimit is the configured upload limit, or the default.
func (a *App) ImageLimit() int64 {
	if a.MaxImageBytes > 0 {
		return a.MaxImageBytes
	}
	return DefaultMaxImageBytes
}

// UploadAsset stores an image for the active tenant. It reads at most the
// limit plus one byte from r.
func (a *App) UploadAsset(ctx context.Context, p *Principal, r io.Reader) (*Asset, error) {
	limit := a.ImageLimit()
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, ErrAssetTooLarge.WithMessage("images can be at most %s", humanBytes(limit))
	}
	ctype := http.DetectContentType(data)
	if !allowedTypes[ctype] {
		return nil, ErrAssetType
	}
	sum := sha256.Sum256(data)
	as := &Asset{ID: ids.New(), TenantID: p.TenantID(), ContentType: ctype, Size: int64(len(data)),
		SHA256: hex.EncodeToString(sum[:]), CreatedAt: database.Now()}
	as.StorageKey = storage.Key(as.TenantID, as.ID)
	err = a.DB.InTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO assets (id, tenant_id, storage_key, content_type, size_bytes, sha256, created_at, data) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			as.ID, as.TenantID, as.StorageKey, as.ContentType, as.Size, as.SHA256, as.CreatedAt, data); err != nil {
			return err
		}
		return a.audit(ctx, tx, p, "asset.uploaded", "asset", as.ID, map[string]any{"content_type": ctype, "size_bytes": as.Size})
	})
	if err != nil {
		return nil, err
	}
	return as, nil
}

func humanBytes(n int64) string {
	if n%(1<<20) == 0 {
		return fmt.Sprintf("%d MB", n>>20)
	}
	return fmt.Sprintf("%d KB", n>>10)
}

const assetCols = `id, tenant_id, storage_key, content_type, size_bytes, sha256, created_at, data`

func scanAsset(row *sql.Row) (*Asset, []byte, error) {
	var as Asset
	var data []byte
	err := row.Scan(&as.ID, &as.TenantID, &as.StorageKey, &as.ContentType, &as.Size, &as.SHA256, &as.CreatedAt, &data)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && data == nil) {
		// No bytes: an old on-disk image that was lost before it was copied in.
		return nil, nil, ErrAssetNotFound
	}
	return &as, data, err
}

// AdminAsset returns an image of the active tenant, attached or not.
func (a *App) AdminAsset(ctx context.Context, p *Principal, id uuid.UUID) (*Asset, []byte, error) {
	return scanAsset(a.DB.QueryRowContext(ctx, `SELECT `+assetCols+` FROM assets WHERE tenant_id = $1 AND id = $2`, p.TenantID(), id))
}

// PublicAsset returns an image that a live (not archived) menu item shows.
// Uploads that no item uses are not public.
func (a *App) PublicAsset(ctx context.Context, id uuid.UUID) (*Asset, []byte, error) {
	return scanAsset(a.DB.QueryRowContext(ctx, `SELECT `+assetCols+` FROM assets a WHERE a.id = $1 AND EXISTS (
		SELECT 1 FROM menu_items i WHERE i.tenant_id = a.tenant_id AND i.image_asset_id = a.id AND i.archived_at IS NULL)`, id))
}

// ImportAssetFiles copies images that older versions kept on disk into the
// database, once; files that are already gone are skipped and reported.
func (a *App) ImportAssetFiles(ctx context.Context, files *storage.Local) error {
	rows, err := a.DB.QueryContext(ctx, `SELECT id, storage_key FROM assets WHERE data IS NULL`)
	if err != nil {
		return err
	}
	type pending struct {
		id  uuid.UUID
		key string
	}
	var todo []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.id, &p.key); err != nil {
			rows.Close()
			return err
		}
		todo = append(todo, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, p := range todo {
		f, err := files.Open(p.key)
		if err != nil {
			a.Log.WarnContext(ctx, "image file missing; upload it again", "asset_id", p.id)
			continue
		}
		data, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			return err
		}
		if _, err := a.DB.ExecContext(ctx, `UPDATE assets SET data = $2 WHERE id = $1 AND data IS NULL`, p.id, data); err != nil {
			return err
		}
	}
	if len(todo) > 0 {
		a.Log.InfoContext(ctx, "copied image files into the database", "count", len(todo))
	}
	return nil
}
