// Package ordersql implements orders.Repository and orders.IdempotencyStore
// on SQLite and PostgreSQL.
package ordersql

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/servly-io/servly/internal/ids"
	"github.com/servly-io/servly/internal/orders"
	"github.com/servly-io/servly/internal/payments"
	"github.com/servly-io/servly/internal/platform/database"
)

type Store struct{ q database.Querier }

var (
	_ orders.Repository       = (*Store)(nil)
	_ orders.IdempotencyStore = (*Store)(nil)
)

func New(q database.Querier) *Store { return &Store{q: q} }

// NextNumber increments the location's counter for the day. The upsert takes
// a row lock (PostgreSQL) or runs under the write lock (SQLite), so
// concurrent orders get distinct numbers.
func (s *Store) NextNumber(ctx context.Context, tenantID, locationID uuid.UUID, businessDate string) (int, error) {
	var n int
	err := s.q.QueryRowContext(ctx,
		`INSERT INTO order_number_counters (tenant_id, location_id, business_date, last_number) VALUES ($1, $2, $3, 1)
		 ON CONFLICT (tenant_id, location_id, business_date) DO UPDATE SET last_number = order_number_counters.last_number + 1
		 RETURNING last_number`, tenantID, locationID, businessDate).Scan(&n)
	return n, err
}

func (s *Store) Create(ctx context.Context, o *orders.Order) error {
	if _, err := s.q.ExecContext(ctx,
		`INSERT INTO orders (id, tenant_id, location_id, dining_location_id, order_number, business_date, public_token, mode,
		                     status, payment_status, currency, subtotal_minor, total_minor, notes, version, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)`,
		o.ID, o.TenantID, o.LocationID, o.DiningLocationID, o.Number, o.BusinessDate, o.PublicToken, o.Mode,
		o.Status, o.PaymentStatus, o.Currency, o.Subtotal, o.Total, o.Notes, o.Version, o.CreatedAt, o.UpdatedAt); err != nil {
		return err
	}
	for li := range o.Lines {
		l := &o.Lines[li]
		l.ID, l.TenantID, l.OrderID = ids.New(), o.TenantID, o.ID
		if _, err := s.q.ExecContext(ctx,
			`INSERT INTO order_items (id, tenant_id, order_id, menu_item_id, position, name, quantity, unit_price_minor,
			                          options_total_minor, line_total_minor, notes)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			l.ID, l.TenantID, l.OrderID, l.MenuItemID, l.Position, l.Name, l.Quantity, l.UnitPrice, l.OptionsTotal, l.LineTotal, l.Notes); err != nil {
			return err
		}
		for oi := range l.Options {
			op := &l.Options[oi]
			op.ID, op.TenantID, op.LineID = ids.New(), o.TenantID, l.ID
			if _, err := s.q.ExecContext(ctx,
				`INSERT INTO order_item_options (id, tenant_id, order_item_id, option_id, group_name, option_name, price_delta_minor)
				 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
				op.ID, op.TenantID, op.LineID, op.OptionID, op.GroupName, op.OptionName, op.PriceDelta); err != nil {
				return err
			}
		}
	}
	return s.insertEvent(ctx, &orders.StatusEvent{ID: ids.New(), TenantID: o.TenantID, OrderID: o.ID, To: o.Status, CreatedAt: o.CreatedAt})
}

func (s *Store) insertEvent(ctx context.Context, ev *orders.StatusEvent) error {
	var from sql.NullString
	if ev.From != "" {
		from = sql.NullString{String: string(ev.From), Valid: true}
	}
	_, err := s.q.ExecContext(ctx,
		`INSERT INTO order_status_events (id, tenant_id, order_id, from_status, to_status, actor_user_id, reason, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		ev.ID, ev.TenantID, ev.OrderID, from, ev.To, ev.ActorUserID, ev.Reason, ev.CreatedAt)
	return err
}

const orderCols = `id, tenant_id, location_id, dining_location_id, order_number, business_date, public_token, mode, status,
	payment_status, currency, subtotal_minor, total_minor, notes, version, created_at, updated_at`

func scanOrder(row interface{ Scan(...any) error }) (*orders.Order, error) {
	var o orders.Order
	err := row.Scan(&o.ID, &o.TenantID, &o.LocationID, &o.DiningLocationID, &o.Number, &o.BusinessDate, &o.PublicToken, &o.Mode,
		&o.Status, &o.PaymentStatus, &o.Currency, &o.Subtotal, &o.Total, &o.Notes, &o.Version, &o.CreatedAt, &o.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, orders.ErrNotFound
	}
	return &o, err
}

func (s *Store) Get(ctx context.Context, tenantID, id uuid.UUID) (*orders.Order, error) {
	return s.one(ctx, `SELECT `+orderCols+` FROM orders WHERE tenant_id = $1 AND id = $2`, tenantID, id)
}

func (s *Store) GetByPublicToken(ctx context.Context, token string) (*orders.Order, error) {
	return s.one(ctx, `SELECT `+orderCols+` FROM orders WHERE public_token = $1`, token)
}

func (s *Store) one(ctx context.Context, query string, args ...any) (*orders.Order, error) {
	o, err := scanOrder(s.q.QueryRowContext(ctx, query, args...))
	if err != nil {
		return nil, err
	}
	list := []orders.Order{*o}
	if err := s.loadLines(ctx, list); err != nil {
		return nil, err
	}
	return &list[0], nil
}

// List returns orders newest first. The cursor encodes the last row's
// (created_at, id), so pages are stable while new orders arrive.
func (s *Store) List(ctx context.Context, tenantID uuid.UUID, f orders.ListFilter) ([]orders.Order, string, error) {
	limit := f.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	where := []string{"tenant_id = $1", "location_id = $2"}
	args := []any{tenantID, f.LocationID}
	arg := func(v any) string {
		args = append(args, v)
		return "$" + strconv.Itoa(len(args))
	}
	if len(f.Statuses) > 0 {
		ph := make([]string, len(f.Statuses))
		for i, st := range f.Statuses {
			ph[i] = arg(st)
		}
		where = append(where, "status IN ("+strings.Join(ph, ", ")+")")
	}
	if f.Mode != "" {
		where = append(where, "mode = "+arg(f.Mode))
	}
	if f.Cursor != "" {
		at, id, err := decodeCursor(f.Cursor)
		if err != nil {
			return nil, "", orders.ErrBadCursor
		}
		a, b := arg(at), arg(id)
		where = append(where, "(created_at < "+a+" OR (created_at = "+a+" AND id < "+b+"))")
	}
	rows, err := s.q.QueryContext(ctx,
		`SELECT `+orderCols+` FROM orders WHERE `+strings.Join(where, " AND ")+
			` ORDER BY created_at DESC, id DESC LIMIT `+strconv.Itoa(limit+1), args...)
	if err != nil {
		return nil, "", err
	}
	var out []orders.Order
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil {
			rows.Close()
			return nil, "", err
		}
		out = append(out, *o)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	next := ""
	if len(out) > limit {
		out = out[:limit]
		last := out[len(out)-1]
		next = encodeCursor(last.CreatedAt, last.ID)
	}
	if err := s.loadLines(ctx, out); err != nil {
		return nil, "", err
	}
	return out, next, nil
}

func encodeCursor(at database.Timestamp, id uuid.UUID) string {
	v, _ := at.Value()
	return base64.RawURLEncoding.EncodeToString([]byte(v.(string) + "|" + id.String()))
}

func decodeCursor(c string) (database.Timestamp, uuid.UUID, error) {
	b, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return database.Timestamp{}, uuid.Nil, err
	}
	at, idText, ok := strings.Cut(string(b), "|")
	if !ok {
		return database.Timestamp{}, uuid.Nil, errors.New("bad cursor")
	}
	var ts database.Timestamp
	if err := ts.Scan(at); err != nil {
		return database.Timestamp{}, uuid.Nil, err
	}
	id, err := uuid.Parse(idText)
	return ts, id, err
}

// loadLines fills lines and their options for a batch of orders.
func (s *Store) loadLines(ctx context.Context, list []orders.Order) error {
	if len(list) == 0 {
		return nil
	}
	byOrder := map[uuid.UUID]int{}
	args := []any{list[0].TenantID}
	ph := make([]string, len(list))
	for i := range list {
		byOrder[list[i].ID] = i
		args = append(args, list[i].ID)
		ph[i] = "$" + strconv.Itoa(i+2)
	}
	in := strings.Join(ph, ", ")
	rows, err := s.q.QueryContext(ctx,
		`SELECT id, tenant_id, order_id, menu_item_id, position, name, quantity, unit_price_minor, options_total_minor, line_total_minor, notes
		   FROM order_items WHERE tenant_id = $1 AND order_id IN (`+in+`) ORDER BY position`, args...)
	if err != nil {
		return err
	}
	type ref struct{ order, line int }
	lineAt := map[uuid.UUID]ref{}
	for rows.Next() {
		var l orders.Line
		if err := rows.Scan(&l.ID, &l.TenantID, &l.OrderID, &l.MenuItemID, &l.Position, &l.Name, &l.Quantity, &l.UnitPrice,
			&l.OptionsTotal, &l.LineTotal, &l.Notes); err != nil {
			rows.Close()
			return err
		}
		oi := byOrder[l.OrderID]
		list[oi].Lines = append(list[oi].Lines, l)
		lineAt[l.ID] = ref{oi, len(list[oi].Lines) - 1}
	}
	rows.Close()
	if err := rows.Err(); err != nil || len(lineAt) == 0 {
		return err
	}
	rows, err = s.q.QueryContext(ctx,
		`SELECT o.id, o.tenant_id, o.order_item_id, o.option_id, o.group_name, o.option_name, o.price_delta_minor
		   FROM order_item_options o JOIN order_items l ON l.tenant_id = o.tenant_id AND l.id = o.order_item_id
		  WHERE o.tenant_id = $1 AND l.order_id IN (`+in+`) ORDER BY o.id`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var op orders.LineOption
		if err := rows.Scan(&op.ID, &op.TenantID, &op.LineID, &op.OptionID, &op.GroupName, &op.OptionName, &op.PriceDelta); err != nil {
			return err
		}
		r := lineAt[op.LineID]
		l := &list[r.order].Lines[r.line]
		l.Options = append(l.Options, op)
	}
	return rows.Err()
}

func (s *Store) UpdateStatus(ctx context.Context, tenantID, id uuid.UUID, expectedVersion int64, ev orders.StatusEvent) (*orders.Order, error) {
	res, err := s.q.ExecContext(ctx,
		`UPDATE orders SET status = $4, version = version + 1, updated_at = $5
		  WHERE tenant_id = $1 AND id = $2 AND version = $3`,
		tenantID, id, expectedVersion, ev.To, ev.CreatedAt)
	if err != nil {
		return nil, err
	}
	if n, err := res.RowsAffected(); err != nil {
		return nil, err
	} else if n == 0 {
		if _, err := s.Get(ctx, tenantID, id); err != nil {
			return nil, err
		}
		return nil, orders.ErrConflict
	}
	ev.TenantID, ev.OrderID = tenantID, id
	if ev.ID == uuid.Nil {
		ev.ID = ids.New()
	}
	if err := s.insertEvent(ctx, &ev); err != nil {
		return nil, err
	}
	return s.Get(ctx, tenantID, id)
}

func (s *Store) SetPaymentStatus(ctx context.Context, tenantID, id uuid.UUID, st payments.Status) error {
	res, err := s.q.ExecContext(ctx,
		`UPDATE orders SET payment_status = $3, version = version + 1, updated_at = $4 WHERE tenant_id = $1 AND id = $2`,
		tenantID, id, st, database.Now())
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		if err != nil {
			return err
		}
		return orders.ErrNotFound
	}
	return nil
}

func (s *Store) Events(ctx context.Context, tenantID, id uuid.UUID) ([]orders.StatusEvent, error) {
	rows, err := s.q.QueryContext(ctx,
		`SELECT id, tenant_id, order_id, from_status, to_status, actor_user_id, reason, created_at
		   FROM order_status_events WHERE tenant_id = $1 AND order_id = $2 ORDER BY created_at, id`, tenantID, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []orders.StatusEvent
	for rows.Next() {
		var ev orders.StatusEvent
		var from sql.NullString
		if err := rows.Scan(&ev.ID, &ev.TenantID, &ev.OrderID, &from, &ev.To, &ev.ActorUserID, &ev.Reason, &ev.CreatedAt); err != nil {
			return nil, err
		}
		ev.From = orders.Status(from.String)
		out = append(out, ev)
	}
	return out, rows.Err()
}

// Idempotency records.

func (s *Store) GetIdempotency(ctx context.Context, tenantID uuid.UUID, scope, key string, now database.Timestamp) (*orders.IdempotencyRecord, error) {
	var r orders.IdempotencyRecord
	err := s.q.QueryRowContext(ctx,
		`SELECT tenant_id, scope, key, request_hash, resource_id, created_at, expires_at
		   FROM idempotency_records WHERE tenant_id = $1 AND scope = $2 AND key = $3 AND expires_at > $4`,
		tenantID, scope, key, now).Scan(&r.TenantID, &r.Scope, &r.Key, &r.RequestHash, &r.ResourceID, &r.CreatedAt, &r.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &r, err
}

// PutIdempotency inserts a record. A concurrent first use of the same key
// fails with a unique violation; the caller rolls back and retries, and then
// finds this record.
func (s *Store) PutIdempotency(ctx context.Context, r *orders.IdempotencyRecord) error {
	// An expired record with the same key may still exist: replace it.
	if _, err := s.q.ExecContext(ctx,
		`DELETE FROM idempotency_records WHERE tenant_id = $1 AND scope = $2 AND key = $3 AND expires_at <= $4`,
		r.TenantID, r.Scope, r.Key, r.CreatedAt); err != nil {
		return err
	}
	_, err := s.q.ExecContext(ctx,
		`INSERT INTO idempotency_records (tenant_id, scope, key, request_hash, resource_id, created_at, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		r.TenantID, r.Scope, r.Key, r.RequestHash, r.ResourceID, r.CreatedAt, r.ExpiresAt)
	return err
}

func (s *Store) DeleteExpiredIdempotency(ctx context.Context, before database.Timestamp) (int64, error) {
	res, err := s.q.ExecContext(ctx, `DELETE FROM idempotency_records WHERE expires_at <= $1`, before)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
