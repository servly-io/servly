// Package paymentsql implements payments.Repository on SQLite and PostgreSQL.
package paymentsql

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"

	"github.com/servly-io/servly/internal/payments"
	"github.com/servly-io/servly/internal/platform/database"
)

type Store struct{ q database.Querier }

var _ payments.Repository = (*Store)(nil)

func New(q database.Querier) *Store { return &Store{q: q} }

const cols = `id, tenant_id, order_id, method, status, amount_minor, refunded_minor, currency, reference, recorded_by_user_id, created_at, updated_at`

func scan(row interface{ Scan(...any) error }) (*payments.Payment, error) {
	var p payments.Payment
	err := row.Scan(&p.ID, &p.TenantID, &p.OrderID, &p.Method, &p.Status, &p.Amount, &p.Refunded, &p.Currency, &p.Reference,
		&p.RecordedByUserID, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, payments.ErrNotFound
	}
	return &p, err
}

func (s *Store) Create(ctx context.Context, p *payments.Payment) error {
	_, err := s.q.ExecContext(ctx, `INSERT INTO payments (`+cols+`) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		p.ID, p.TenantID, p.OrderID, p.Method, p.Status, p.Amount, p.Refunded, p.Currency, p.Reference, p.RecordedByUserID, p.CreatedAt, p.UpdatedAt)
	return err
}

func (s *Store) ListForOrder(ctx context.Context, tenantID, orderID uuid.UUID) ([]payments.Payment, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT `+cols+` FROM payments WHERE tenant_id = $1 AND order_id = $2 ORDER BY created_at, id`, tenantID, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []payments.Payment
	for rows.Next() {
		p, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

func (s *Store) Get(ctx context.Context, tenantID, id uuid.UUID) (*payments.Payment, error) {
	return scan(s.q.QueryRowContext(ctx, `SELECT `+cols+` FROM payments WHERE tenant_id = $1 AND id = $2`, tenantID, id))
}

var errConflict = payments.ErrConflict

func (s *Store) Update(ctx context.Context, p *payments.Payment, expected payments.Status) error {
	res, err := s.q.ExecContext(ctx,
		`UPDATE payments SET status = $4, refunded_minor = $5, updated_at = $6 WHERE tenant_id = $1 AND id = $2 AND status = $3`,
		p.TenantID, p.ID, expected, p.Status, p.Refunded, p.UpdatedAt)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		if _, err := s.Get(ctx, p.TenantID, p.ID); err != nil {
			return err
		}
		return errConflict
	}
	return nil
}
