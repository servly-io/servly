// Package tenantsql implements tenant.Repository on SQLite and PostgreSQL.
package tenantsql

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"

	"github.com/servly-io/servly/internal/platform/database"
	"github.com/servly-io/servly/internal/tenant"
)

// Store is a tenant.Repository over a database handle or transaction.
type Store struct{ q database.Querier }

var _ tenant.Repository = (*Store)(nil)

func New(q database.Querier) *Store { return &Store{q: q} }

func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return tenant.ErrNotFound
	}
	return err
}

func (s *Store) CreateTenant(ctx context.Context, t *tenant.Tenant) error {
	_, err := s.q.ExecContext(ctx,
		`INSERT INTO tenants (id, slug, name, status, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6)`,
		t.ID, t.Slug, t.Name, t.Status, t.CreatedAt, t.UpdatedAt)
	if database.IsUniqueViolation(err) {
		return tenant.ErrSlugTaken
	}
	return err
}

func (s *Store) GetTenant(ctx context.Context, id uuid.UUID) (*tenant.Tenant, error) {
	var t tenant.Tenant
	err := s.q.QueryRowContext(ctx,
		`SELECT id, slug, name, status, created_at, updated_at FROM tenants WHERE id = $1`, id).
		Scan(&t.ID, &t.Slug, &t.Name, &t.Status, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, notFound(err)
	}
	return &t, nil
}

func (s *Store) UpdateTenant(ctx context.Context, t *tenant.Tenant) error {
	return expectOne(s.q.ExecContext(ctx,
		`UPDATE tenants SET name = $2, status = $3, updated_at = $4 WHERE id = $1`,
		t.ID, t.Name, t.Status, t.UpdatedAt))
}

const userCols = `id, email, password_hash, display_name, is_platform_admin, status, created_at, updated_at`

func scanUser(row interface{ Scan(...any) error }) (*tenant.User, error) {
	var u tenant.User
	if err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.DisplayName, &u.IsPlatformAdmin, &u.Status, &u.CreatedAt, &u.UpdatedAt); err != nil {
		return nil, notFound(err)
	}
	return &u, nil
}

func (s *Store) CreateUser(ctx context.Context, u *tenant.User) error {
	_, err := s.q.ExecContext(ctx,
		`INSERT INTO users (`+userCols+`) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		u.ID, u.Email, u.PasswordHash, u.DisplayName, u.IsPlatformAdmin, u.Status, u.CreatedAt, u.UpdatedAt)
	if database.IsUniqueViolation(err) {
		return tenant.ErrEmailTaken
	}
	return err
}

func (s *Store) GetUser(ctx context.Context, id uuid.UUID) (*tenant.User, error) {
	return scanUser(s.q.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE id = $1`, id))
}

func (s *Store) GetUserByEmail(ctx context.Context, email string) (*tenant.User, error) {
	return scanUser(s.q.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE email = $1`, email))
}

func (s *Store) UpdateUser(ctx context.Context, u *tenant.User) error {
	return expectOne(s.q.ExecContext(ctx,
		`UPDATE users SET display_name = $2, password_hash = $3, status = $4, updated_at = $5 WHERE id = $1`,
		u.ID, u.DisplayName, u.PasswordHash, u.Status, u.UpdatedAt))
}

func (s *Store) CreateMembership(ctx context.Context, m *tenant.Membership) error {
	_, err := s.q.ExecContext(ctx,
		`INSERT INTO tenant_memberships (id, tenant_id, user_id, role, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6)`,
		m.ID, m.TenantID, m.UserID, m.Role, m.CreatedAt, m.UpdatedAt)
	if database.IsUniqueViolation(err) {
		return tenant.ErrAlreadyMember
	}
	return err
}

func (s *Store) GetMembership(ctx context.Context, tenantID, userID uuid.UUID) (*tenant.Membership, error) {
	var m tenant.Membership
	err := s.q.QueryRowContext(ctx,
		`SELECT id, tenant_id, user_id, role, created_at, updated_at FROM tenant_memberships WHERE tenant_id = $1 AND user_id = $2`,
		tenantID, userID).Scan(&m.ID, &m.TenantID, &m.UserID, &m.Role, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		return nil, notFound(err)
	}
	return &m, nil
}

func (s *Store) Memberships(ctx context.Context, userID uuid.UUID) ([]tenant.TenantMembership, error) {
	rows, err := s.q.QueryContext(ctx,
		`SELECT m.id, m.tenant_id, m.user_id, m.role, m.created_at, m.updated_at, t.name, t.status
		   FROM tenant_memberships m JOIN tenants t ON t.id = m.tenant_id
		  WHERE m.user_id = $1 ORDER BY t.name, t.id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []tenant.TenantMembership
	for rows.Next() {
		var m tenant.TenantMembership
		if err := rows.Scan(&m.ID, &m.TenantID, &m.UserID, &m.Role, &m.CreatedAt, &m.UpdatedAt, &m.TenantName, &m.TenantStatus); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) ListMembers(ctx context.Context, tenantID uuid.UUID) ([]tenant.Member, error) {
	rows, err := s.q.QueryContext(ctx,
		`SELECT m.id, m.tenant_id, m.user_id, m.role, m.created_at, m.updated_at, u.email, u.display_name, u.status
		   FROM tenant_memberships m JOIN users u ON u.id = m.user_id
		  WHERE m.tenant_id = $1 ORDER BY u.display_name, u.email`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []tenant.Member
	for rows.Next() {
		var m tenant.Member
		if err := rows.Scan(&m.ID, &m.TenantID, &m.UserID, &m.Role, &m.CreatedAt, &m.UpdatedAt, &m.Email, &m.DisplayName, &m.UserStatus); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) UpdateMembership(ctx context.Context, m *tenant.Membership) error {
	return expectOne(s.q.ExecContext(ctx,
		`UPDATE tenant_memberships SET role = $3, updated_at = $4 WHERE tenant_id = $1 AND id = $2`,
		m.TenantID, m.ID, m.Role, m.UpdatedAt))
}

func (s *Store) CountActiveOwners(ctx context.Context, tenantID uuid.UUID) (int, error) {
	var n int
	err := s.q.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tenant_memberships m JOIN users u ON u.id = m.user_id
		  WHERE m.tenant_id = $1 AND m.role = 'owner' AND u.status = 'active'`, tenantID).Scan(&n)
	return n, err
}

const locationCols = `id, tenant_id, name, currency, timezone, order_notes_enabled, public_status_enabled, status, created_at, updated_at`

func scanLocation(row interface{ Scan(...any) error }) (*tenant.Location, error) {
	var l tenant.Location
	if err := row.Scan(&l.ID, &l.TenantID, &l.Name, &l.Currency, &l.Timezone, &l.OrderNotesEnabled,
		&l.PublicStatusEnabled, &l.Status, &l.CreatedAt, &l.UpdatedAt); err != nil {
		return nil, notFound(err)
	}
	return &l, nil
}

func (s *Store) CreateLocation(ctx context.Context, l *tenant.Location) error {
	_, err := s.q.ExecContext(ctx,
		`INSERT INTO restaurant_locations (`+locationCols+`) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		l.ID, l.TenantID, l.Name, l.Currency, l.Timezone, l.OrderNotesEnabled, l.PublicStatusEnabled, l.Status, l.CreatedAt, l.UpdatedAt)
	return err
}

func (s *Store) GetLocation(ctx context.Context, tenantID, id uuid.UUID) (*tenant.Location, error) {
	return scanLocation(s.q.QueryRowContext(ctx,
		`SELECT `+locationCols+` FROM restaurant_locations WHERE tenant_id = $1 AND id = $2`, tenantID, id))
}

func (s *Store) PrimaryLocation(ctx context.Context, tenantID uuid.UUID) (*tenant.Location, error) {
	return scanLocation(s.q.QueryRowContext(ctx,
		`SELECT `+locationCols+` FROM restaurant_locations WHERE tenant_id = $1 ORDER BY created_at, id LIMIT 1`, tenantID))
}

func (s *Store) UpdateLocation(ctx context.Context, l *tenant.Location) error {
	return expectOne(s.q.ExecContext(ctx,
		`UPDATE restaurant_locations
		    SET name = $3, currency = $4, timezone = $5, order_notes_enabled = $6, public_status_enabled = $7, status = $8, updated_at = $9
		  WHERE tenant_id = $1 AND id = $2`,
		l.TenantID, l.ID, l.Name, l.Currency, l.Timezone, l.OrderNotesEnabled, l.PublicStatusEnabled, l.Status, l.UpdatedAt))
}

// expectOne turns an update that matched no row into tenant.ErrNotFound.
func expectOne(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return tenant.ErrNotFound
	}
	return nil
}
