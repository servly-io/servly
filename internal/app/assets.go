package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"net/http"

	"github.com/google/uuid"

	"github.com/servly-io/servly/internal/apperr"
	"github.com/servly-io/servly/internal/ids"
	"github.com/servly-io/servly/internal/platform/database"
	"github.com/servly-io/servly/internal/storage"
)

// MaxAssetBytes caps one uploaded image.
const MaxAssetBytes = 5 << 20

var (
	ErrAssetTooLarge   = apperr.New(apperr.KindTooLarge, "asset_too_large", "images can be at most 5 MiB")
	ErrAssetType       = apperr.Invalid("unsupported_media_type", "upload a JPEG, PNG or WebP image")
	ErrAssetNotFound   = apperr.NotFound("not_found", "not found")
	errUploadsDisabled = errors.New("asset storage not configured")
)

// allowedTypes are detected from the content, never from the file name or
// the client's Content-Type.
var allowedTypes = map[string]bool{"image/jpeg": true, "image/png": true, "image/webp": true}

// Asset is an uploaded file's record.
type Asset struct {
	ID          uuid.UUID
	TenantID    uuid.UUID
	StorageKey  string
	ContentType string
	Size        int64
	SHA256      string
	CreatedAt   database.Timestamp
}

// UploadAsset stores an image for the active tenant. It reads at most
// MaxAssetBytes+1 bytes from r.
func (a *App) UploadAsset(ctx context.Context, p *Principal, r io.Reader) (*Asset, error) {
	if a.Files == nil {
		return nil, errUploadsDisabled
	}
	data, err := io.ReadAll(io.LimitReader(r, MaxAssetBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxAssetBytes {
		return nil, ErrAssetTooLarge
	}
	ctype := http.DetectContentType(data)
	if !allowedTypes[ctype] {
		return nil, ErrAssetType
	}
	sum := sha256.Sum256(data)
	as := &Asset{ID: ids.New(), TenantID: p.TenantID(), ContentType: ctype, Size: int64(len(data)),
		SHA256: hex.EncodeToString(sum[:]), CreatedAt: database.Now()}
	as.StorageKey = storage.Key(as.TenantID, as.ID)

	// File first, row second: a crash in between leaves an orphan file, never
	// a row pointing at nothing.
	if err := a.Files.Put(as.StorageKey, data); err != nil {
		return nil, err
	}
	err = a.DB.InTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO assets (id, tenant_id, storage_key, content_type, size_bytes, sha256, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			as.ID, as.TenantID, as.StorageKey, as.ContentType, as.Size, as.SHA256, as.CreatedAt); err != nil {
			return err
		}
		return a.audit(ctx, tx, p, "asset.uploaded", "asset", as.ID, map[string]any{"content_type": ctype, "size_bytes": as.Size})
	})
	if err != nil {
		_ = a.Files.Delete(as.StorageKey)
		return nil, err
	}
	return as, nil
}

const assetCols = `id, tenant_id, storage_key, content_type, size_bytes, sha256, created_at`

func scanAsset(row *sql.Row) (*Asset, error) {
	var as Asset
	err := row.Scan(&as.ID, &as.TenantID, &as.StorageKey, &as.ContentType, &as.Size, &as.SHA256, &as.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrAssetNotFound
	}
	return &as, err
}

// AdminAsset opens an asset of the active tenant, attached or not.
func (a *App) AdminAsset(ctx context.Context, p *Principal, id uuid.UUID) (*Asset, io.ReadSeekCloser, error) {
	as, err := scanAsset(a.DB.QueryRowContext(ctx, `SELECT `+assetCols+` FROM assets WHERE tenant_id = $1 AND id = $2`, p.TenantID(), id))
	if err != nil {
		return nil, nil, err
	}
	return a.openAsset(as)
}

// PublicAsset opens an image that a live (not archived) menu item shows.
// Uploads that no item uses are not public.
func (a *App) PublicAsset(ctx context.Context, id uuid.UUID) (*Asset, io.ReadSeekCloser, error) {
	as, err := scanAsset(a.DB.QueryRowContext(ctx, `SELECT `+assetCols+` FROM assets a WHERE a.id = $1 AND EXISTS (
		SELECT 1 FROM menu_items i WHERE i.tenant_id = a.tenant_id AND i.image_asset_id = a.id AND i.archived_at IS NULL)`, id))
	if err != nil {
		return nil, nil, err
	}
	return a.openAsset(as)
}

func (a *App) openAsset(as *Asset) (*Asset, io.ReadSeekCloser, error) {
	if a.Files == nil {
		return nil, nil, errUploadsDisabled
	}
	f, err := a.Files.Open(as.StorageKey)
	if err != nil {
		return nil, nil, err
	}
	return as, f, nil
}
