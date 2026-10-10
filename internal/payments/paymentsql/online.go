package paymentsql

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"

	"github.com/servly-io/servly/internal/payments"
	"github.com/servly-io/servly/internal/platform/database"
)

const accountCols = `id, tenant_id, location_id, provider, mode, status, account_ref, account_name, country,
	table_flow, pickup_flow, credentials, created_at, updated_at`

func scanAccount(row interface{ Scan(...any) error }) (*payments.Account, error) {
	var a payments.Account
	err := row.Scan(&a.ID, &a.TenantID, &a.LocationID, &a.Provider, &a.Mode, &a.Status, &a.AccountRef, &a.AccountName,
		&a.Country, &a.TableFlow, &a.PickupFlow, &a.Credentials, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, payments.ErrNoAccount
	}
	return &a, err
}

// AccountForLocation returns the location's account, connected or not.
func (s *Store) AccountForLocation(ctx context.Context, tenantID, locationID uuid.UUID) (*payments.Account, error) {
	return scanAccount(s.q.QueryRowContext(ctx,
		`SELECT `+accountCols+` FROM payment_accounts WHERE tenant_id = $1 AND location_id = $2`, tenantID, locationID))
}

// AccountByID finds an account across tenants: webhooks name the account in
// their path before any tenant is known. Callers verify the webhook against
// the account's own credentials before trusting anything.
func (s *Store) AccountByID(ctx context.Context, id uuid.UUID) (*payments.Account, error) {
	return scanAccount(s.q.QueryRowContext(ctx, `SELECT `+accountCols+` FROM payment_accounts WHERE id = $1`, id))
}

// SaveAccount inserts the location's account, or replaces the one it had.
func (s *Store) SaveAccount(ctx context.Context, a *payments.Account) error {
	_, err := s.q.ExecContext(ctx,
		`INSERT INTO payment_accounts (`+accountCols+`) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		 ON CONFLICT (tenant_id, location_id) DO UPDATE SET provider = excluded.provider, mode = excluded.mode,
		   status = excluded.status, account_ref = excluded.account_ref, account_name = excluded.account_name,
		   country = excluded.country, table_flow = excluded.table_flow, pickup_flow = excluded.pickup_flow,
		   credentials = excluded.credentials, updated_at = excluded.updated_at`,
		a.ID, a.TenantID, a.LocationID, a.Provider, a.Mode, a.Status, a.AccountRef, a.AccountName, a.Country,
		a.TableFlow, a.PickupFlow, a.Credentials, a.CreatedAt, a.UpdatedAt)
	return err
}

const checkoutCols = `id, tenant_id, location_id, account_id, public_token, kind, order_id, dining_location_id, idempotency_key,
	draft, provider, provider_ref, redirect_url, amount_minor, currency, status, attempts, next_check_at, expires_at,
	created_at, updated_at`

func scanCheckout(row interface{ Scan(...any) error }) (*payments.Checkout, error) {
	var c payments.Checkout
	err := row.Scan(&c.ID, &c.TenantID, &c.LocationID, &c.AccountID, &c.PublicToken, &c.Kind, &c.OrderID, &c.DiningLocationID,
		&c.IdempotencyKey, &c.Draft, &c.Provider, &c.ProviderRef, &c.RedirectURL, &c.Amount, &c.Currency, &c.Status,
		&c.Attempts, &c.NextCheckAt, &c.ExpiresAt, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, payments.ErrNotFound
	}
	return &c, err
}

func (s *Store) CreateCheckout(ctx context.Context, c *payments.Checkout) error {
	_, err := s.q.ExecContext(ctx, `INSERT INTO payment_checkouts (`+checkoutCols+`)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)`,
		c.ID, c.TenantID, c.LocationID, c.AccountID, c.PublicToken, c.Kind, c.OrderID, c.DiningLocationID, c.IdempotencyKey,
		c.Draft, c.Provider, c.ProviderRef, c.RedirectURL, c.Amount, c.Currency, c.Status, c.Attempts, c.NextCheckAt,
		c.ExpiresAt, c.CreatedAt, c.UpdatedAt)
	return err
}

func (s *Store) one(ctx context.Context, where string, args ...any) (*payments.Checkout, error) {
	return scanCheckout(s.q.QueryRowContext(ctx, `SELECT `+checkoutCols+` FROM payment_checkouts WHERE `+where, args...))
}

func (s *Store) Checkout(ctx context.Context, id uuid.UUID) (*payments.Checkout, error) {
	return s.one(ctx, `id = $1`, id)
}

func (s *Store) CheckoutByToken(ctx context.Context, token string) (*payments.Checkout, error) {
	return s.one(ctx, `public_token = $1`, token)
}

func (s *Store) CheckoutByIdempotency(ctx context.Context, tenantID uuid.UUID, key string) (*payments.Checkout, error) {
	return s.one(ctx, `tenant_id = $1 AND idempotency_key = $2`, tenantID, key)
}

// CheckoutByRef finds an account's checkout by the provider's id.
func (s *Store) CheckoutByRef(ctx context.Context, accountID uuid.UUID, ref string) (*payments.Checkout, error) {
	return s.one(ctx, `account_id = $1 AND provider_ref = $2`, accountID, ref)
}

// OpenCheckoutForOrder returns the order's open checkout that is not about
// to expire, if any, so a guest tapping "Pay" twice gets the same page.
func (s *Store) OpenCheckoutForOrder(ctx context.Context, tenantID, orderID uuid.UUID, validUntil database.Timestamp) (*payments.Checkout, error) {
	return s.one(ctx, `tenant_id = $1 AND order_id = $2 AND status = 'open' AND expires_at > $3 AND provider_ref <> ''
		ORDER BY created_at DESC LIMIT 1`, tenantID, orderID, validUntil)
}

// SetProvider stores what the provider returned for a new checkout.
func (s *Store) SetProvider(ctx context.Context, id uuid.UUID, ref, url string, now database.Timestamp) error {
	_, err := s.q.ExecContext(ctx, `UPDATE payment_checkouts SET provider_ref = $2, redirect_url = $3, updated_at = $4 WHERE id = $1`,
		id, ref, url, now)
	return err
}

// Claim takes an open checkout for settling: it succeeds for exactly one
// caller between commits, so a webhook and the reconciler never both apply
// a result. In PostgreSQL the update also locks the row until commit.
func (s *Store) Claim(ctx context.Context, id uuid.UUID, now database.Timestamp) (bool, error) {
	res, err := s.q.ExecContext(ctx, `UPDATE payment_checkouts SET updated_at = $2 WHERE id = $1 AND status = 'open'`, id, now)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// Finish closes a checkout with its final status and, for a prepay, the order it created.
func (s *Store) Finish(ctx context.Context, id uuid.UUID, st payments.CheckoutStatus, orderID uuid.NullUUID, now database.Timestamp) error {
	_, err := s.q.ExecContext(ctx,
		`UPDATE payment_checkouts SET status = $2, order_id = COALESCE($3, order_id), updated_at = $4 WHERE id = $1 AND status = 'open'`,
		id, st, orderID, now)
	return err
}

// Reschedule sets when the reconciler looks at an open checkout next. It
// only succeeds if nobody rescheduled it since `seen` was read, which is how
// two processes avoid checking the same checkout at once.
func (s *Store) Reschedule(ctx context.Context, id uuid.UUID, seen, next database.Timestamp) (bool, error) {
	res, err := s.q.ExecContext(ctx,
		`UPDATE payment_checkouts SET next_check_at = $3, attempts = attempts + 1 WHERE id = $1 AND status = 'open' AND next_check_at = $2`,
		id, seen, next)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// DueCheckouts lists open checkouts whose next check is due.
func (s *Store) DueCheckouts(ctx context.Context, now database.Timestamp, limit int) ([]payments.Checkout, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT `+checkoutCols+` FROM payment_checkouts
		WHERE status = 'open' AND next_check_at <= $1 ORDER BY next_check_at LIMIT $2`, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []payments.Checkout
	for rows.Next() {
		c, err := scanCheckout(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

// CountOpen counts a location's open checkouts.
func (s *Store) CountOpen(ctx context.Context, tenantID, accountID uuid.UUID) (int, error) {
	var n int
	err := s.q.QueryRowContext(ctx, `SELECT COUNT(*) FROM payment_checkouts WHERE tenant_id = $1 AND account_id = $2 AND status = 'open'`,
		tenantID, accountID).Scan(&n)
	return n, err
}

// RecordEvent stores a provider event once. It reports false for an event
// already seen (a retried webhook).
func (s *Store) RecordEvent(ctx context.Context, id, tenantID, accountID uuid.UUID, provider, ref, typ string, now database.Timestamp) (bool, error) {
	_, err := s.q.ExecContext(ctx, `INSERT INTO payment_events (id, tenant_id, account_id, provider, event_ref, type, received_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`, id, tenantID, accountID, provider, ref, typ, now)
	if database.IsUniqueViolation(err) {
		return false, nil
	}
	return err == nil, err
}

// DeleteEventsBefore purges old event records; they only serve deduplication.
func (s *Store) DeleteEventsBefore(ctx context.Context, t database.Timestamp) (int64, error) {
	res, err := s.q.ExecContext(ctx, `DELETE FROM payment_events WHERE received_at < $1`, t)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
