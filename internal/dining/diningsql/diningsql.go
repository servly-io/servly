// Package diningsql implements dining.Repository on SQLite and PostgreSQL.
package diningsql

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"

	"github.com/servly-io/servly/internal/dining"
	"github.com/servly-io/servly/internal/platform/database"
)

type Store struct{ q database.Querier }

var _ dining.Repository = (*Store)(nil)

func New(q database.Querier) *Store { return &Store{q: q} }

const cols = `id, tenant_id, location_id, name, kind, public_token, is_enabled, created_at, updated_at`

func scan(row interface{ Scan(...any) error }) (*dining.Location, error) {
	var l dining.Location
	err := row.Scan(&l.ID, &l.TenantID, &l.LocationID, &l.Name, &l.Kind, &l.PublicToken, &l.IsEnabled, &l.CreatedAt, &l.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, dining.ErrNotFound
	}
	return &l, err
}

func (s *Store) List(ctx context.Context, tenantID, locationID uuid.UUID) ([]dining.Location, error) {
	rows, err := s.q.QueryContext(ctx,
		`SELECT `+cols+` FROM dining_locations WHERE tenant_id = $1 AND location_id = $2 ORDER BY kind DESC, name, created_at`,
		tenantID, locationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []dining.Location
	for rows.Next() {
		l, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *l)
	}
	return out, rows.Err()
}

func (s *Store) Get(ctx context.Context, tenantID, id uuid.UUID) (*dining.Location, error) {
	return scan(s.q.QueryRowContext(ctx, `SELECT `+cols+` FROM dining_locations WHERE tenant_id = $1 AND id = $2`, tenantID, id))
}

func (s *Store) GetByToken(ctx context.Context, token string) (*dining.Location, error) {
	return scan(s.q.QueryRowContext(ctx, `SELECT `+cols+` FROM dining_locations WHERE public_token = $1`, token))
}

func (s *Store) Create(ctx context.Context, l *dining.Location) error {
	_, err := s.q.ExecContext(ctx, `INSERT INTO dining_locations (`+cols+`) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		l.ID, l.TenantID, l.LocationID, l.Name, l.Kind, l.PublicToken, l.IsEnabled, l.CreatedAt, l.UpdatedAt)
	return err
}

func (s *Store) Update(ctx context.Context, l *dining.Location) error {
	res, err := s.q.ExecContext(ctx,
		`UPDATE dining_locations SET name = $3, is_enabled = $4, public_token = $5, updated_at = $6 WHERE tenant_id = $1 AND id = $2`,
		l.TenantID, l.ID, l.Name, l.IsEnabled, l.PublicToken, l.UpdatedAt)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return firstErr(err, dining.ErrNotFound)
	}
	return nil
}

func firstErr(a, b error) error {
	if a != nil {
		return a
	}
	return b
}
