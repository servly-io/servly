package app

import (
	"context"
	"database/sql"
	"strings"

	"github.com/google/uuid"

	"github.com/servly-io/servly/internal/dining/diningsql"
	"github.com/servly-io/servly/internal/ids"
	"github.com/servly-io/servly/internal/money"
	"github.com/servly-io/servly/internal/orders"
	"github.com/servly-io/servly/internal/orders/ordersql"
	"github.com/servly-io/servly/internal/payments"
	"github.com/servly-io/servly/internal/payments/paymentsql"
	"github.com/servly-io/servly/internal/platform/database"
	"github.com/servly-io/servly/internal/tenant/tenantsql"
)

// ListOrders lists the active location's orders, newest first.
func (a *App) ListOrders(ctx context.Context, p *Principal, f orders.ListFilter) ([]orders.Order, string, error) {
	f.LocationID = p.Location.ID
	for _, s := range f.Statuses {
		if !s.Valid() {
			return nil, "", orders.ErrInvalidTransition.WithMessage("unknown status %q", s)
		}
	}
	return ordersql.New(a.DB).List(ctx, p.TenantID(), f)
}

// OrderDetail is an order with its history and payments.
type OrderDetail struct {
	Order       *orders.Order
	Events      []orders.StatusEvent
	Payments    []payments.Payment
	Outstanding money.Amount
	// Actors names the staff who appear in Events and Payments.
	Actors map[uuid.UUID]string
}

func (a *App) GetOrder(ctx context.Context, p *Principal, id uuid.UUID) (*OrderDetail, error) {
	return a.orderDetail(ctx, a.DB, p.TenantID(), id)
}

func (a *App) orderDetail(ctx context.Context, q database.Querier, tenantID, id uuid.UUID) (*OrderDetail, error) {
	st := ordersql.New(q)
	o, err := st.Get(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	evs, err := st.Events(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	ps, err := paymentsql.New(q).ListForOrder(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	d := &OrderDetail{Order: o, Events: evs, Payments: ps, Outstanding: payments.Outstanding(o.Total, ps), Actors: map[uuid.UUID]string{}}
	users := tenantsql.New(q)
	name := func(id uuid.NullUUID) {
		if !id.Valid {
			return
		}
		if _, ok := d.Actors[id.UUID]; ok {
			return
		}
		if u, err := users.GetUser(ctx, id.UUID); err == nil {
			d.Actors[id.UUID] = u.DisplayName
		}
	}
	for _, e := range evs {
		name(e.ActorUserID)
	}
	for _, pm := range ps {
		name(pm.RecordedByUserID)
	}
	return d, nil
}

// DiningNames maps the active location's dining location ids to names.
func (a *App) DiningNames(ctx context.Context, p *Principal) (map[uuid.UUID]string, error) {
	ls, err := diningsql.New(a.DB).List(ctx, p.TenantID(), p.Location.ID)
	if err != nil {
		return nil, err
	}
	m := make(map[uuid.UUID]string, len(ls))
	for _, l := range ls {
		m[l.ID] = l.Name
	}
	return m, nil
}

// ChangeOrderStatus moves an order along its lifecycle. expectedVersion is
// the version the client saw; a concurrent change makes it ErrConflict.
func (a *App) ChangeOrderStatus(ctx context.Context, p *Principal, id uuid.UUID, to orders.Status, expectedVersion int64, reason string) (*OrderDetail, error) {
	reason = strings.TrimSpace(reason)
	var d *OrderDetail
	err := a.DB.InTx(ctx, func(tx *sql.Tx) error {
		st := ordersql.New(tx)
		o, err := st.Get(ctx, p.TenantID(), id)
		if err != nil {
			return err
		}
		if o.Version != expectedVersion {
			return orders.ErrConflict
		}
		if err := orders.ValidateTransition(o.Status, to, p.Tenant.Role, reason); err != nil {
			return err
		}
		if _, err := st.UpdateStatus(ctx, p.TenantID(), id, expectedVersion, orders.StatusEvent{
			ID: ids.New(), From: o.Status, To: to, ActorUserID: uuid.NullUUID{UUID: p.User.ID, Valid: true},
			Reason: reason, CreatedAt: database.Now(),
		}); err != nil {
			return err
		}
		if to == orders.StatusCancelled {
			if err := a.audit(ctx, tx, p, "order.cancelled", "order", id, map[string]any{"from": o.Status, "reason": reason, "number": o.Number}); err != nil {
				return err
			}
		}
		d, err = a.orderDetail(ctx, tx, p.TenantID(), id)
		return err
	})
	if err != nil {
		return nil, err
	}
	a.publish(OrderEvent{Kind: EventOrderUpdated, TenantID: d.Order.TenantID, LocationID: d.Order.LocationID, OrderID: id, Version: d.Order.Version})
	return d, nil
}

// PaymentInput records a payment taken outside servly.
type PaymentInput struct {
	Method    payments.Method
	Status    payments.Status
	Amount    money.Amount
	Reference string
}

// RecordPayment records a payment against an order. It is a staff statement:
// servly does not verify it with any terminal.
func (a *App) RecordPayment(ctx context.Context, p *Principal, orderID uuid.UUID, in PaymentInput) (*OrderDetail, error) {
	in.Reference = strings.TrimSpace(in.Reference)
	if err := payments.CheckReference(in.Reference); err != nil {
		return nil, err
	}
	return a.changePayments(ctx, p, orderID, func(tx *sql.Tx, o *orders.Order, existing []payments.Payment) error {
		now := database.Now()
		pm := &payments.Payment{ID: ids.New(), TenantID: o.TenantID, OrderID: o.ID, Method: in.Method, Status: in.Status,
			Amount: in.Amount, Currency: o.Currency, Reference: in.Reference,
			RecordedByUserID: uuid.NullUUID{UUID: p.User.ID, Valid: true}, CreatedAt: now, UpdatedAt: now}
		if err := pm.ValidateNew(payments.Outstanding(o.Total, existing)); err != nil {
			return err
		}
		if err := paymentsql.New(tx).Create(ctx, pm); err != nil {
			return err
		}
		return a.audit(ctx, tx, p, "payment.recorded", "payment", pm.ID, map[string]any{
			"order_id": o.ID, "method": pm.Method, "status": pm.Status, "amount_minor": pm.Amount, "reference": pm.Reference})
	})
}

// SettlePayment resolves a pending payment as paid or failed.
func (a *App) SettlePayment(ctx context.Context, p *Principal, paymentID uuid.UUID, to payments.Status) (*OrderDetail, error) {
	pm, err := paymentsql.New(a.DB).Get(ctx, p.TenantID(), paymentID)
	if err != nil {
		return nil, err
	}
	return a.changePayments(ctx, p, pm.OrderID, func(tx *sql.Tx, _ *orders.Order, _ []payments.Payment) error {
		st := paymentsql.New(tx)
		cur, err := st.Get(ctx, p.TenantID(), paymentID)
		if err != nil {
			return err
		}
		if cur.Status != payments.StatusPending {
			return payments.ErrInvalidTransition.WithDetails(map[string]any{"from": cur.Status, "to": to})
		}
		if err := payments.ValidateTransition(cur.Status, to); err != nil {
			return err
		}
		from := cur.Status
		cur.Status, cur.UpdatedAt = to, database.Now()
		if err := st.Update(ctx, cur, from); err != nil {
			return err
		}
		return a.audit(ctx, tx, p, "payment.settled", "payment", cur.ID, map[string]any{"order_id": cur.OrderID, "from": from, "to": to})
	})
}

// RefundPayment records a refund given back outside servly.
func (a *App) RefundPayment(ctx context.Context, p *Principal, paymentID uuid.UUID, amount money.Amount, reason string) (*OrderDetail, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, orders.ErrReasonRequired.WithMessage("a refund needs a reason")
	}
	pm, err := paymentsql.New(a.DB).Get(ctx, p.TenantID(), paymentID)
	if err != nil {
		return nil, err
	}
	return a.changePayments(ctx, p, pm.OrderID, func(tx *sql.Tx, _ *orders.Order, _ []payments.Payment) error {
		st := paymentsql.New(tx)
		cur, err := st.Get(ctx, p.TenantID(), paymentID)
		if err != nil {
			return err
		}
		from := cur.Status
		if err := cur.Refund(amount); err != nil {
			return err
		}
		cur.UpdatedAt = database.Now()
		if err := st.Update(ctx, cur, from); err != nil {
			return err
		}
		return a.audit(ctx, tx, p, "payment.refunded", "payment", cur.ID, map[string]any{
			"order_id": cur.OrderID, "amount_minor": amount, "reason": reason, "status": cur.Status})
	})
}

// changePayments runs a payment change and then stores the order's derived
// payment status, in one transaction, and notifies staff screens.
func (a *App) changePayments(ctx context.Context, p *Principal, orderID uuid.UUID,
	change func(tx *sql.Tx, o *orders.Order, existing []payments.Payment) error) (*OrderDetail, error) {
	var d *OrderDetail
	err := a.DB.InTx(ctx, func(tx *sql.Tx) error {
		o, err := ordersql.New(tx).Get(ctx, p.TenantID(), orderID)
		if err != nil {
			return err
		}
		ps := paymentsql.New(tx)
		existing, err := ps.ListForOrder(ctx, o.TenantID, o.ID)
		if err != nil {
			return err
		}
		if err := change(tx, o, existing); err != nil {
			return err
		}
		after, err := ps.ListForOrder(ctx, o.TenantID, o.ID)
		if err != nil {
			return err
		}
		if err := ordersql.New(tx).SetPaymentStatus(ctx, o.TenantID, o.ID, payments.Summarize(o.Total, after)); err != nil {
			return err
		}
		d, err = a.orderDetail(ctx, tx, o.TenantID, o.ID)
		return err
	})
	if err != nil {
		return nil, err
	}
	a.publish(OrderEvent{Kind: EventOrderUpdated, TenantID: d.Order.TenantID, LocationID: d.Order.LocationID, OrderID: orderID, Version: d.Order.Version})
	return d, nil
}
