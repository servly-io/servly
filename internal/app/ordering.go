package app

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/servly-io/servly/internal/apperr"
	"github.com/servly-io/servly/internal/dining"
	"github.com/servly-io/servly/internal/dining/diningsql"
	"github.com/servly-io/servly/internal/ids"
	"github.com/servly-io/servly/internal/menu"
	"github.com/servly-io/servly/internal/menu/menusql"
	"github.com/servly-io/servly/internal/orders"
	"github.com/servly-io/servly/internal/orders/ordersql"
	"github.com/servly-io/servly/internal/payments"
	"github.com/servly-io/servly/internal/platform/database"
	"github.com/servly-io/servly/internal/tenant"
	"github.com/servly-io/servly/internal/tenant/tenantsql"
)

const (
	idempotencyScope = "public_order_create"
	idempotencyTTL   = 24 * time.Hour
)

var (
	// ErrMenuNotFound covers every reason a public token does not open a
	// menu, so a guest learns nothing about why.
	ErrMenuNotFound = apperr.NotFound("menu_not_found", "this menu is not available")
	ErrBadIdemKey   = apperr.Invalid("idempotency_key_required", "send an Idempotency-Key header of 8-128 characters [A-Za-z0-9_-]")
)

// PublicMenu is what a guest sees after scanning a table's QR code.
type PublicMenu struct {
	Location   tenant.Location
	Dining     dining.Location
	Categories []PublicCategory
}

type PublicCategory struct {
	menu.Category
	Items []menu.Item
}

// resolveDining opens the dining location of a public token, with its
// restaurant, if every part is active.
func resolveDining(ctx context.Context, q database.Querier, token string) (*dining.Location, *tenant.Location, error) {
	if !ids.IsPublicToken(token) {
		return nil, nil, ErrMenuNotFound
	}
	d, err := diningsql.New(q).GetByToken(ctx, token)
	if err != nil || !d.IsEnabled {
		return nil, nil, ErrMenuNotFound
	}
	st := tenantsql.New(q)
	t, err := st.GetTenant(ctx, d.TenantID)
	if err != nil || t.Status != tenant.StatusActive {
		return nil, nil, ErrMenuNotFound
	}
	loc, err := st.GetLocation(ctx, d.TenantID, d.LocationID)
	if err != nil || loc.Status != tenant.LocationActive {
		return nil, nil, ErrMenuNotFound
	}
	return d, loc, nil
}

// GetPublicMenu returns the guest menu: enabled categories with their enabled
// items; sold-out items are included and marked unavailable.
func (a *App) GetPublicMenu(ctx context.Context, token string) (*PublicMenu, error) {
	d, loc, err := resolveDining(ctx, a.DB, token)
	if err != nil {
		return nil, err
	}
	st := menusql.New(a.DB)
	cats, err := st.ListCategories(ctx, loc.TenantID, loc.ID)
	if err != nil {
		return nil, err
	}
	items, err := st.ListItems(ctx, loc.TenantID, loc.ID)
	if err != nil {
		return nil, err
	}
	pm := &PublicMenu{Location: *loc, Dining: *d}
	for _, c := range cats {
		if !c.IsEnabled {
			continue
		}
		pc := PublicCategory{Category: c}
		for _, it := range items {
			if it.CategoryID == c.ID && it.IsEnabled {
				pc.Items = append(pc.Items, it)
			}
		}
		if len(pc.Items) > 0 {
			pm.Categories = append(pm.Categories, pc)
		}
	}
	return pm, nil
}

// PlaceOrderInput is a guest's order. RequestHash identifies the request body
// for idempotency; the HTTP layer computes it.
type PlaceOrderInput struct {
	DiningToken    string
	Notes          string
	Lines          []orders.LineRequest
	IdempotencyKey string
	RequestHash    string
}

// PlacedOrder is the result of PlaceOrder.
type PlacedOrder struct {
	Order      *orders.Order
	DiningName string
	// Replayed is true when the idempotency key matched an earlier
	// submission and that order is returned again.
	Replayed bool
}

// ValidIdempotencyKey reports whether k is an acceptable Idempotency-Key.
func ValidIdempotencyKey(k string) bool {
	if len(k) < 8 || len(k) > 128 {
		return false
	}
	for _, c := range k {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

// PlaceOrder validates, prices and stores a guest's order atomically. A
// retry with the same idempotency key and body returns the original order.
func (a *App) PlaceOrder(ctx context.Context, in PlaceOrderInput, now time.Time) (*PlacedOrder, error) {
	if !ValidIdempotencyKey(in.IdempotencyKey) {
		return nil, ErrBadIdemKey
	}
	res, err := a.placeOrder(ctx, in, now)
	if database.IsUniqueViolation(err) {
		// Another request with the same key committed first; replay it.
		res, err = a.placeOrder(ctx, in, now)
	}
	if err != nil {
		return nil, err
	}
	if !res.Replayed {
		a.publish(OrderEvent{Kind: EventOrderCreated, TenantID: res.Order.TenantID, LocationID: res.Order.LocationID,
			OrderID: res.Order.ID, Version: res.Order.Version})
	}
	return res, nil
}

func (a *App) placeOrder(ctx context.Context, in PlaceOrderInput, now time.Time) (*PlacedOrder, error) {
	var out *PlacedOrder
	err := a.DB.InTx(ctx, func(tx *sql.Tx) error {
		d, loc, err := resolveDining(ctx, tx, in.DiningToken)
		if err != nil {
			return err
		}
		st := ordersql.New(tx)
		ts := database.Timestamp{Time: now.UTC().Truncate(time.Microsecond)}

		rec, err := st.GetIdempotency(ctx, loc.TenantID, idempotencyScope, in.IdempotencyKey, ts)
		if err != nil {
			return err
		}
		if rec != nil {
			if rec.RequestHash != in.RequestHash {
				return orders.ErrIdempotencyMismatch
			}
			o, err := st.Get(ctx, loc.TenantID, rec.ResourceID)
			if err != nil {
				return err
			}
			out = &PlacedOrder{Order: o, DiningName: d.Name, Replayed: true}
			return nil
		}

		m, err := guestMenu(ctx, tx, loc)
		if err != nil {
			return err
		}
		lines, subtotal, err := orders.Price(in.Lines, in.Notes, m)
		if err != nil {
			return err
		}
		tz, err := time.LoadLocation(loc.Timezone)
		if err != nil {
			return err
		}
		bd := orders.BusinessDate(now, tz)
		number, err := st.NextNumber(ctx, loc.TenantID, loc.ID, bd)
		if err != nil {
			return err
		}
		mode := orders.ModeDineIn
		if d.Kind == dining.KindPickup {
			mode = orders.ModePickup
		}
		o := &orders.Order{
			ID: ids.New(), TenantID: loc.TenantID, LocationID: loc.ID,
			DiningLocationID: uuid.NullUUID{UUID: d.ID, Valid: true},
			Number:           number, BusinessDate: bd, PublicToken: ids.PublicToken(), Mode: mode,
			Status: orders.StatusNew, PaymentStatus: payments.StatusUnpaid, Currency: loc.Currency,
			Subtotal: subtotal, Total: subtotal, Notes: strings.TrimSpace(in.Notes), Version: 1,
			Lines: lines, CreatedAt: ts, UpdatedAt: ts,
		}
		if err := st.Create(ctx, o); err != nil {
			return err
		}
		if err := st.PutIdempotency(ctx, &orders.IdempotencyRecord{
			TenantID: loc.TenantID, Scope: idempotencyScope, Key: in.IdempotencyKey, RequestHash: in.RequestHash,
			ResourceID: o.ID, CreatedAt: ts, ExpiresAt: database.Timestamp{Time: ts.Add(idempotencyTTL)},
		}); err != nil {
			return err
		}
		out = &PlacedOrder{Order: o, DiningName: d.Name}
		return nil
	})
	return out, err
}

// guestMenu loads what pricing needs: orderable items and enabled categories.
func guestMenu(ctx context.Context, q database.Querier, loc *tenant.Location) (orders.Menu, error) {
	st := menusql.New(q)
	cats, err := st.ListCategories(ctx, loc.TenantID, loc.ID)
	if err != nil {
		return orders.Menu{}, err
	}
	items, err := st.ListItems(ctx, loc.TenantID, loc.ID)
	if err != nil {
		return orders.Menu{}, err
	}
	m := orders.Menu{Items: map[uuid.UUID]*menu.Item{}, EnabledCategories: map[uuid.UUID]bool{}, NotesEnabled: loc.OrderNotesEnabled}
	for _, c := range cats {
		m.EnabledCategories[c.ID] = c.IsEnabled
	}
	for i := range items {
		m.Items[items[i].ID] = &items[i]
	}
	return m, nil
}

// GetPublicOrder returns an order by its public token, if the restaurant lets
// guests follow their order.
func (a *App) GetPublicOrder(ctx context.Context, token string) (*PlacedOrder, error) {
	if !ids.IsPublicToken(token) {
		return nil, orders.ErrNotFound
	}
	o, err := ordersql.New(a.DB).GetByPublicToken(ctx, token)
	if err != nil {
		return nil, err
	}
	loc, err := tenantsql.New(a.DB).GetLocation(ctx, o.TenantID, o.LocationID)
	if err != nil {
		return nil, err
	}
	if !loc.PublicStatusEnabled {
		return nil, orders.ErrNotFound
	}
	name := ""
	if o.DiningLocationID.Valid {
		if d, err := diningsql.New(a.DB).Get(ctx, o.TenantID, o.DiningLocationID.UUID); err == nil {
			name = d.Name
		}
	}
	return &PlacedOrder{Order: o, DiningName: name}, nil
}

// Quote prices a guest's cart against the live menu without storing
// anything, so the review page shows server-confirmed totals.
func (a *App) QuoteOrder(ctx context.Context, token, notes string, lines []orders.LineRequest) (*PlacedOrder, error) {
	d, loc, err := resolveDining(ctx, a.DB, token)
	if err != nil {
		return nil, err
	}
	m, err := guestMenu(ctx, a.DB, loc)
	if err != nil {
		return nil, err
	}
	priced, subtotal, err := orders.Price(lines, notes, m)
	if err != nil {
		return nil, err
	}
	mode := orders.ModeDineIn
	if d.Kind == dining.KindPickup {
		mode = orders.ModePickup
	}
	return &PlacedOrder{DiningName: d.Name, Order: &orders.Order{
		Mode: mode, Status: orders.StatusNew, PaymentStatus: payments.StatusUnpaid, Currency: loc.Currency,
		Subtotal: subtotal, Total: subtotal, Notes: strings.TrimSpace(notes), Lines: priced,
	}}, nil
}
