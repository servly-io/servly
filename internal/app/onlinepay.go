package app

// Online payments through providers (ADR 0007, ADR 0008,
// docs/docs/payments.md). A payment counts only when servly reads it from
// the provider (API or verified webhook), and settle applies it exactly once.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/servly-io/servly/internal/audit"
	"github.com/servly-io/servly/internal/dining"
	"github.com/servly-io/servly/internal/dining/diningsql"
	"github.com/servly-io/servly/internal/ids"
	"github.com/servly-io/servly/internal/money"
	"github.com/servly-io/servly/internal/orders"
	"github.com/servly-io/servly/internal/orders/ordersql"
	"github.com/servly-io/servly/internal/payments"
	"github.com/servly-io/servly/internal/payments/paymentsql"
	"github.com/servly-io/servly/internal/payments/provider"
	"github.com/servly-io/servly/internal/platform/database"
	"github.com/servly-io/servly/internal/tenant"
	"github.com/servly-io/servly/internal/tenant/tenantsql"
)

const (
	// checkoutTTL is how long a hosted payment stays payable. Stripe needs
	// at least 30 minutes; a minute more absorbs clock differences.
	checkoutTTL = 31 * time.Minute
	// reuseMargin: an open checkout this close to expiry is not handed out again.
	reuseMargin = 2 * time.Minute
	// guestRecheck throttles provider lookups triggered by a guest's polling.
	guestRecheck = 2 * time.Second
	// expiryGrace: a checkout the provider still calls open this long after
	// its expiry is closed as expired.
	expiryGrace = 10 * time.Minute
)

// ---- owner: connect, update, disconnect -----------------------------------

// PaymentProviders lists the providers this server offers, by id.
func (a *App) PaymentProviders() []string {
	out := make([]string, 0, len(a.Providers))
	for id := range a.Providers {
		out = append(out, id)
	}
	return out
}

// OnlinePaymentsAvailable reports whether the operator configured the secret
// key that provider credentials are sealed with.
func (a *App) OnlinePaymentsAvailable() bool { return a.Secrets != nil }

// GetPaymentAccount returns the active location's connected account.
func (a *App) GetPaymentAccount(ctx context.Context, p *Principal) (*payments.Account, error) {
	acct, err := paymentsql.New(a.DB).AccountForLocation(ctx, p.TenantID(), p.Location.ID)
	if err != nil {
		return nil, err
	}
	if acct.Status != payments.AccountActive {
		return nil, payments.ErrNoAccount
	}
	return acct, nil
}

// ConnectInput connects a provider account to the active location.
type ConnectInput struct {
	Provider    string
	Credentials provider.Credentials
	TableFlow   payments.Flow
	PickupFlow  payments.Flow
}

// ConnectPayments verifies the credentials with the provider, seals them and
// makes the account the location's payment account.
func (a *App) ConnectPayments(ctx context.Context, p *Principal, in ConnectInput) (*payments.Account, error) {
	if a.Secrets == nil {
		return nil, payments.ErrSecretKeyMissing
	}
	prov, ok := a.Providers[in.Provider]
	if !ok {
		return nil, payments.ErrUnknownProvider
	}
	if !payments.ValidTableFlow(in.TableFlow) || !payments.ValidPickupFlow(in.PickupFlow) {
		return nil, payments.ErrInvalidFlow
	}
	in.Credentials.SecretKey = strings.TrimSpace(in.Credentials.SecretKey)
	in.Credentials.WebhookSecret = strings.TrimSpace(in.Credentials.WebhookSecret)
	info, err := prov.Verify(ctx, in.Credentials)
	if err != nil {
		return nil, err
	}
	sealed, err := a.seal(in.Credentials)
	if err != nil {
		return nil, err
	}
	now := database.Now()
	acct := &payments.Account{ID: ids.New(), TenantID: p.TenantID(), LocationID: p.Location.ID, Provider: in.Provider,
		Mode: "test", Status: payments.AccountActive, AccountRef: info.Ref, AccountName: info.Name, Country: info.Country,
		TableFlow: in.TableFlow, PickupFlow: in.PickupFlow, Credentials: sealed, CreatedAt: now, UpdatedAt: now}
	if info.Live {
		acct.Mode = "live"
	}
	err = a.DB.InTx(ctx, func(tx *sql.Tx) error {
		st := paymentsql.New(tx)
		if old, err := st.AccountForLocation(ctx, p.TenantID(), p.Location.ID); err == nil {
			acct.ID, acct.CreatedAt = old.ID, old.CreatedAt
			// Open checkouts are looked up with the old account's credentials.
			if n, err := st.CountOpen(ctx, p.TenantID(), old.ID); err != nil {
				return err
			} else if n > 0 {
				return payments.ErrOpenCheckouts
			}
		} else if !errors.Is(err, payments.ErrNoAccount) {
			return err
		}
		if err := st.SaveAccount(ctx, acct); err != nil {
			return err
		}
		return a.audit(ctx, tx, p, "payments.connected", "payment_account", acct.ID, map[string]any{
			"provider": acct.Provider, "mode": acct.Mode, "account_ref": acct.AccountRef, "country": acct.Country})
	})
	if err != nil {
		return nil, err
	}
	return acct, nil
}

// PaymentUpdate changes when guests pay, or adds the webhook signing secret.
type PaymentUpdate struct {
	TableFlow     *payments.Flow
	PickupFlow    *payments.Flow
	WebhookSecret *string
}

func (a *App) UpdatePayments(ctx context.Context, p *Principal, in PaymentUpdate) (*payments.Account, error) {
	var acct *payments.Account
	err := a.DB.InTx(ctx, func(tx *sql.Tx) error {
		st := paymentsql.New(tx)
		var err error
		if acct, err = st.AccountForLocation(ctx, p.TenantID(), p.Location.ID); err != nil {
			return err
		}
		if acct.Status != payments.AccountActive {
			return payments.ErrNoAccount
		}
		changed := map[string]any{}
		if in.TableFlow != nil {
			if !payments.ValidTableFlow(*in.TableFlow) {
				return payments.ErrInvalidFlow
			}
			acct.TableFlow, changed["table_flow"] = *in.TableFlow, *in.TableFlow
		}
		if in.PickupFlow != nil {
			if !payments.ValidPickupFlow(*in.PickupFlow) {
				return payments.ErrInvalidFlow
			}
			acct.PickupFlow, changed["pickup_flow"] = *in.PickupFlow, *in.PickupFlow
		}
		if in.WebhookSecret != nil {
			creds, _, err := a.credentials(acct)
			if err != nil {
				return err
			}
			creds.WebhookSecret = strings.TrimSpace(*in.WebhookSecret)
			if acct.Credentials, err = a.seal(creds); err != nil {
				return err
			}
			changed["webhook_secret"] = creds.WebhookSecret != ""
		}
		acct.UpdatedAt = database.Now()
		if err := st.SaveAccount(ctx, acct); err != nil {
			return err
		}
		return a.audit(ctx, tx, p, "payments.updated", "payment_account", acct.ID, changed)
	})
	if err != nil {
		return nil, err
	}
	return acct, nil
}

// DisconnectPayments forgets the credentials. Past payments stay recorded.
func (a *App) DisconnectPayments(ctx context.Context, p *Principal) error {
	return a.DB.InTx(ctx, func(tx *sql.Tx) error {
		st := paymentsql.New(tx)
		acct, err := st.AccountForLocation(ctx, p.TenantID(), p.Location.ID)
		if err != nil {
			return err
		}
		if n, err := st.CountOpen(ctx, p.TenantID(), acct.ID); err != nil {
			return err
		} else if n > 0 {
			return payments.ErrOpenCheckouts
		}
		acct.Status, acct.Credentials, acct.UpdatedAt = payments.AccountDisabled, "", database.Now()
		if err := st.SaveAccount(ctx, acct); err != nil {
			return err
		}
		return a.audit(ctx, tx, p, "payments.disconnected", "payment_account", acct.ID, map[string]any{"provider": acct.Provider})
	})
}

// HasWebhookSecret reports whether webhooks can be verified for the account;
// without a secret servly relies on polling alone.
func (a *App) HasWebhookSecret(acct *payments.Account) bool {
	c, _, err := a.credentials(acct)
	return err == nil && c.WebhookSecret != ""
}

func (a *App) seal(c provider.Credentials) (string, error) {
	if a.Secrets == nil {
		return "", payments.ErrSecretKeyMissing
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return a.Secrets.Seal(raw), nil
}

// credentials opens an account's sealed credentials and finds its provider.
func (a *App) credentials(acct *payments.Account) (provider.Credentials, provider.Provider, error) {
	var c provider.Credentials
	prov, ok := a.Providers[acct.Provider]
	if !ok || a.Secrets == nil || acct.Status != payments.AccountActive {
		return c, nil, payments.ErrNoAccount
	}
	raw, err := a.Secrets.Open(acct.Credentials)
	if err != nil {
		return c, nil, fmt.Errorf("payment account %s: %w", acct.ID, err)
	}
	return c, prov, json.Unmarshal(raw, &c)
}

// ---- guests ---------------------------------------------------------------

// GuestPayment is how guests at one dining location can pay online:
// "none", or the Flow (before, after, either).
type GuestPayment string

const GuestPayNone GuestPayment = "none"

// guestPayment derives what a dining location offers from the account.
func (a *App) guestPayment(ctx context.Context, q database.Querier, tenantID, locationID uuid.UUID, kind dining.Kind) (GuestPayment, *payments.Account) {
	acct, err := paymentsql.New(q).AccountForLocation(ctx, tenantID, locationID)
	if err != nil || acct.Status != payments.AccountActive || a.Secrets == nil || a.Providers[acct.Provider] == nil {
		return GuestPayNone, nil
	}
	f := acct.TableFlow
	if kind == dining.KindPickup {
		f = acct.PickupFlow
	}
	if f == payments.FlowStaff || f == payments.FlowCounter {
		return GuestPayNone, acct
	}
	return GuestPayment(f), acct
}

// CheckoutStart is where to send the guest.
type CheckoutStart struct {
	Token       string
	RedirectURL string
}

// StartBillCheckout lets a guest pay an existing order's outstanding amount.
func (a *App) StartBillCheckout(ctx context.Context, orderToken, base string, now time.Time) (*CheckoutStart, error) {
	if !ids.IsPublicToken(orderToken) {
		return nil, orders.ErrNotFound
	}
	o, err := ordersql.New(a.DB).GetByPublicToken(ctx, orderToken)
	if err != nil {
		return nil, err
	}
	kind := dining.KindTable
	if o.Mode == orders.ModePickup {
		kind = dining.KindPickup
	}
	gp, acct := a.guestPayment(ctx, a.DB, o.TenantID, o.LocationID, kind)
	if !payments.Flow(gp).PayLater() || o.Status == orders.StatusCancelled {
		return nil, payments.ErrOnlineNotAllowed
	}
	ps, err := paymentsql.New(a.DB).ListForOrder(ctx, o.TenantID, o.ID)
	if err != nil {
		return nil, err
	}
	due := payments.Outstanding(o.Total, ps)
	if due <= 0 {
		return nil, payments.ErrNothingToPay
	}
	st := paymentsql.New(a.DB)
	if open, err := st.OpenCheckoutForOrder(ctx, o.TenantID, o.ID, ts(now.Add(reuseMargin))); err == nil && open.Amount == due {
		return &CheckoutStart{Token: open.PublicToken, RedirectURL: open.RedirectURL}, nil
	}
	ck := &payments.Checkout{TenantID: o.TenantID, LocationID: o.LocationID, AccountID: acct.ID, Kind: payments.CheckoutBill,
		OrderID: uuid.NullUUID{UUID: o.ID, Valid: true}, DiningLocationID: o.DiningLocationID, Amount: due, Currency: o.Currency}
	return a.openCheckout(ctx, ck, acct, a.describe(ctx, o.TenantID, o.LocationID, o.DiningLocationID, o.Number), base, now)
}

// prepayDraft is the priced order a prepay checkout creates once paid.
type prepayDraft struct {
	Mode  orders.Mode
	Notes string
	Lines []orders.Line
	Total money.Amount
}

// StartPrepayCheckout prices a cart and starts its payment. The order is
// created only when the payment is confirmed (ADR 0008), so the kitchen never
// sees an unpaid prepaid order. A retry with the same key returns the same
// checkout.
func (a *App) StartPrepayCheckout(ctx context.Context, in PlaceOrderInput, base string, now time.Time) (*CheckoutStart, error) {
	if !ValidIdempotencyKey(in.IdempotencyKey) {
		return nil, ErrBadIdemKey
	}
	d, loc, err := resolveDining(ctx, a.DB, in.DiningToken)
	if err != nil {
		return nil, err
	}
	gp, acct := a.guestPayment(ctx, a.DB, loc.TenantID, loc.ID, d.Kind)
	if gp != GuestPayment(payments.FlowBefore) && gp != GuestPayment(payments.FlowEither) {
		return nil, payments.ErrOnlineNotAllowed
	}
	st := paymentsql.New(a.DB)
	if old, err := st.CheckoutByIdempotency(ctx, loc.TenantID, in.IdempotencyKey); err == nil {
		return &CheckoutStart{Token: old.PublicToken, RedirectURL: old.RedirectURL}, nil
	} else if !errors.Is(err, payments.ErrNotFound) {
		return nil, err
	}
	if err := checkOrderable(ctx, a.DB, loc, d); err != nil {
		return nil, err
	}
	m, err := guestMenu(ctx, a.DB, loc)
	if err != nil {
		return nil, err
	}
	lines, subtotal, err := orders.Price(in.Lines, in.Notes, m)
	if err != nil {
		return nil, err
	}
	if subtotal <= 0 {
		return nil, payments.ErrNothingToPay
	}
	mode := orders.ModeDineIn
	if d.Kind == dining.KindPickup {
		mode = orders.ModePickup
	}
	draft, err := json.Marshal(prepayDraft{Mode: mode, Notes: strings.TrimSpace(in.Notes), Lines: lines, Total: subtotal})
	if err != nil {
		return nil, err
	}
	key := in.IdempotencyKey
	ck := &payments.Checkout{TenantID: loc.TenantID, LocationID: loc.ID, AccountID: acct.ID, Kind: payments.CheckoutPrepay,
		DiningLocationID: uuid.NullUUID{UUID: d.ID, Valid: true}, IdempotencyKey: &key, Draft: string(draft),
		Amount: subtotal, Currency: loc.Currency}
	start, err := a.openCheckout(ctx, ck, acct, fmt.Sprintf("%s · %s", loc.Name, d.Name), base, now)
	if database.IsUniqueViolation(err) {
		// A concurrent retry with the same key won; hand out its checkout.
		if old, err := st.CheckoutByIdempotency(ctx, loc.TenantID, in.IdempotencyKey); err == nil {
			return &CheckoutStart{Token: old.PublicToken, RedirectURL: old.RedirectURL}, nil
		}
	}
	return start, err
}

func (a *App) describe(ctx context.Context, tenantID, locationID uuid.UUID, diningID uuid.NullUUID, number int) string {
	name := ""
	if loc, err := tenantsql.New(a.DB).GetLocation(ctx, tenantID, locationID); err == nil {
		name = loc.Name
	}
	where := ""
	if diningID.Valid {
		if d, err := diningsql.New(a.DB).Get(ctx, tenantID, diningID.UUID); err == nil {
			where = " · " + d.Name
		}
	}
	return fmt.Sprintf("%s · Order %d%s", name, number, where)
}

// openCheckout stores a checkout, then asks the provider for the hosted
// page. The checkout id is the provider's idempotency key, so a retry after a
// lost answer gets the same page.
func (a *App) openCheckout(ctx context.Context, ck *payments.Checkout, acct *payments.Account, desc, base string, now time.Time) (*CheckoutStart, error) {
	creds, prov, err := a.credentials(acct)
	if err != nil {
		return nil, err
	}
	n := ts(now)
	ck.ID, ck.PublicToken, ck.Provider, ck.Status = ids.New(), ids.PublicToken(), acct.Provider, payments.CheckoutOpen
	ck.NextCheckAt, ck.ExpiresAt, ck.CreatedAt, ck.UpdatedAt = ts(now.Add(10*time.Second)), ts(now.Add(checkoutTTL)), n, n
	st := paymentsql.New(a.DB)
	if err := st.CreateCheckout(ctx, ck); err != nil {
		return nil, err
	}
	out, err := prov.CreateCheckout(ctx, creds, provider.CheckoutInput{
		Reference: ck.ID.String(), Amount: ck.Amount, Currency: ck.Currency, Description: desc,
		ReturnURL: base + "/pay/" + ck.PublicToken, ExpiresAt: ck.ExpiresAt.Time,
	}, ck.ID.String())
	if err != nil {
		_ = st.Finish(ctx, ck.ID, payments.CheckoutFailed, uuid.NullUUID{}, ts(now))
		return nil, err
	}
	if err := st.SetProvider(ctx, ck.ID, out.Ref, out.URL, ts(now)); err != nil {
		return nil, err
	}
	return &CheckoutStart{Token: ck.PublicToken, RedirectURL: out.URL}, nil
}

// CheckoutView is what a returning guest sees of a checkout.
type CheckoutView struct {
	Status      payments.CheckoutStatus
	Kind        payments.CheckoutKind
	OrderToken  string
	RedirectURL string
}

// GetCheckout answers the guest's "did it work?" page. An open checkout is
// looked up at the provider right away (at most every few seconds), so the
// guest does not wait for the reconciler.
func (a *App) GetCheckout(ctx context.Context, token string, now time.Time) (*CheckoutView, error) {
	if !ids.IsPublicToken(token) {
		return nil, payments.ErrNotFound
	}
	st := paymentsql.New(a.DB)
	ck, err := st.CheckoutByToken(ctx, token)
	if err != nil {
		return nil, err
	}
	if ck.Status == payments.CheckoutOpen && ck.ProviderRef != "" && now.Sub(ck.UpdatedAt.Time) >= guestRecheck {
		if err := a.settle(ctx, ck.ID, now); err != nil && !errors.Is(err, provider.ErrUnavailable) {
			a.Log.WarnContext(ctx, "settle checkout on guest return", "checkout_id", ck.ID, "error", err)
		}
		if ck, err = st.CheckoutByToken(ctx, token); err != nil {
			return nil, err
		}
	}
	v := &CheckoutView{Status: ck.Status, Kind: ck.Kind, RedirectURL: ck.RedirectURL}
	if ck.OrderID.Valid {
		if o, err := ordersql.New(a.DB).Get(ctx, ck.TenantID, ck.OrderID.UUID); err == nil {
			v.OrderToken = o.PublicToken
		}
	}
	return v, nil
}

// ---- settling ---------------------------------------------------------------

// settle reads a checkout's state from its provider and applies it once:
// paid creates the payment (and, for a prepay, the order); failed and
// expired close it. Webhooks, the reconciler and returning guests all call
// it; Claim lets exactly one of them apply a result.
func (a *App) settle(ctx context.Context, checkoutID uuid.UUID, now time.Time) error {
	st := paymentsql.New(a.DB)
	ck, err := st.Checkout(ctx, checkoutID)
	if err != nil || ck.Status != payments.CheckoutOpen || ck.ProviderRef == "" {
		return err
	}
	acct, err := st.AccountByID(ctx, ck.AccountID)
	if err != nil {
		return err
	}
	creds, prov, err := a.credentials(acct)
	if err != nil {
		return err
	}
	res, err := prov.Lookup(ctx, creds, ck.ProviderRef)
	if err != nil {
		return err
	}
	var ev *OrderEvent
	err = a.DB.InTx(ctx, func(tx *sql.Tx) error {
		ps := paymentsql.New(tx)
		if claimed, err := ps.Claim(ctx, ck.ID, ts(now)); err != nil || !claimed {
			return err // settled by someone else in the meantime
		}
		switch res.State {
		case provider.StateOpen:
			if now.After(ck.ExpiresAt.Add(expiryGrace)) {
				return ps.Finish(ctx, ck.ID, payments.CheckoutExpired, uuid.NullUUID{}, ts(now))
			}
			return nil
		case provider.StateFailed, provider.StateExpired:
			return ps.Finish(ctx, ck.ID, payments.CheckoutStatus(res.State), uuid.NullUUID{}, ts(now))
		}
		// Paid. The provider's amount and currency must be what servly asked for.
		if res.Amount != ck.Amount || res.Currency != ck.Currency {
			a.Log.ErrorContext(ctx, "online payment amount mismatch; not applied", "checkout_id", ck.ID,
				"want_minor", ck.Amount, "want_currency", ck.Currency, "got_minor", res.Amount, "got_currency", res.Currency)
			if err := systemAudit(ctx, tx, ck.TenantID, "payment.online_mismatch", "checkout", ck.ID, map[string]any{
				"want_minor": ck.Amount, "got_minor": res.Amount, "got_currency": res.Currency, "provider_ref": ck.ProviderRef}); err != nil {
				return err
			}
			return ps.Finish(ctx, ck.ID, payments.CheckoutFailed, uuid.NullUUID{}, ts(now))
		}
		orderID := ck.OrderID
		kind := EventOrderUpdated
		if ck.Kind == payments.CheckoutPrepay {
			o, err := a.createPrepaidOrder(ctx, tx, ck, now)
			if err != nil {
				return err
			}
			orderID, kind = uuid.NullUUID{UUID: o.ID, Valid: true}, EventOrderCreated
		}
		o, err := ordersql.New(tx).Get(ctx, ck.TenantID, orderID.UUID)
		if err != nil {
			return err
		}
		pm := &payments.Payment{ID: ids.New(), TenantID: ck.TenantID, OrderID: o.ID, Method: payments.MethodOnline,
			Status: payments.StatusPaid, Amount: res.Amount, Currency: res.Currency, Provider: ck.Provider,
			ProviderRef: res.PaymentRef, CheckoutID: uuid.NullUUID{UUID: ck.ID, Valid: true}, Detail: res.Detail,
			CreatedAt: ts(now), UpdatedAt: ts(now)}
		if err := ps.Create(ctx, pm); err != nil {
			return err
		}
		all, err := ps.ListForOrder(ctx, o.TenantID, o.ID)
		if err != nil {
			return err
		}
		if err := ordersql.New(tx).SetPaymentStatus(ctx, o.TenantID, o.ID, payments.Summarize(o.Total, all)); err != nil {
			return err
		}
		if err := ps.Finish(ctx, ck.ID, payments.CheckoutPaid, orderID, ts(now)); err != nil {
			return err
		}
		ev = &OrderEvent{Kind: kind, TenantID: o.TenantID, LocationID: o.LocationID, OrderID: o.ID, Version: o.Version + 1}
		return systemAudit(ctx, tx, ck.TenantID, "payment.online_paid", "payment", pm.ID, map[string]any{
			"order_id": o.ID, "checkout_id": ck.ID, "provider": ck.Provider, "amount_minor": pm.Amount, "detail": pm.Detail})
	})
	if err == nil && ev != nil {
		a.publish(*ev)
	}
	return err
}

// createPrepaidOrder turns a paid prepay checkout's draft into an order. The
// draft was priced when the guest started paying; that is what they paid.
func (a *App) createPrepaidOrder(ctx context.Context, tx *sql.Tx, ck *payments.Checkout, now time.Time) (*orders.Order, error) {
	var d prepayDraft
	if err := json.Unmarshal([]byte(ck.Draft), &d); err != nil {
		return nil, fmt.Errorf("checkout %s draft: %w", ck.ID, err)
	}
	loc, err := tenantsql.New(tx).GetLocation(ctx, ck.TenantID, ck.LocationID)
	if err != nil {
		return nil, err
	}
	tz, err := time.LoadLocation(loc.Timezone)
	if err != nil {
		return nil, err
	}
	st := ordersql.New(tx)
	bd := orders.BusinessDate(now, tz)
	number, err := st.NextNumber(ctx, ck.TenantID, ck.LocationID, bd)
	if err != nil {
		return nil, err
	}
	o := &orders.Order{ID: ids.New(), TenantID: ck.TenantID, LocationID: ck.LocationID, DiningLocationID: ck.DiningLocationID,
		Number: number, BusinessDate: bd, PublicToken: ids.PublicToken(), Mode: d.Mode, Status: orders.StatusNew,
		PaymentStatus: payments.StatusUnpaid, Currency: ck.Currency, Subtotal: d.Total, Total: d.Total, Notes: d.Notes,
		Version: 1, Lines: d.Lines, CreatedAt: ts(now), UpdatedAt: ts(now)}
	return o, st.Create(ctx, o)
}

func systemAudit(ctx context.Context, q database.Querier, tenantID uuid.UUID, action, entity string, id uuid.UUID, data map[string]any) error {
	tid, actor := audit.Tenant(tenantID, uuid.Nil)
	return audit.Record(ctx, q, audit.Event{TenantID: tid, ActorUserID: actor, Action: action, EntityType: entity, EntityID: id, Data: data})
}

// ---- reconciler and webhooks ------------------------------------------------

// RunPaymentReconciler looks up open checkouts until they settle: the floor
// that works without webhooks, for example on a local install.
func (a *App) RunPaymentReconciler(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		a.ReconcilePayments(ctx, time.Now())
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// ReconcilePayments checks the checkouts that are due, with backoff.
func (a *App) ReconcilePayments(ctx context.Context, now time.Time) {
	st := paymentsql.New(a.DB)
	due, err := st.DueCheckouts(ctx, ts(now), 25)
	if err != nil {
		if ctx.Err() == nil {
			a.Log.WarnContext(ctx, "list due checkouts", "error", err)
		}
		return
	}
	for _, ck := range due {
		next := min(10*time.Second<<min(ck.Attempts, 4), 2*time.Minute)
		if ok, err := st.Reschedule(ctx, ck.ID, ck.NextCheckAt, ts(now.Add(next))); err != nil || !ok {
			continue // another process took it
		}
		if err := a.settle(ctx, ck.ID, now); err != nil && ctx.Err() == nil {
			a.Log.WarnContext(ctx, "settle checkout", "checkout_id", ck.ID, "attempt", ck.Attempts+1, "error", err)
		}
	}
}

// PaymentWebhook verifies a provider webhook with the named account's own
// credentials, records the event once, and settles its checkout. Anything
// that cannot be verified is ErrBadSignature, also an unknown account, so
// the endpoint reveals nothing.
func (a *App) PaymentWebhook(ctx context.Context, providerID string, accountID uuid.UUID, header map[string][]string, body []byte, now time.Time) error {
	st := paymentsql.New(a.DB)
	acct, err := st.AccountByID(ctx, accountID)
	if err != nil || acct.Provider != providerID {
		return provider.ErrBadSignature
	}
	creds, prov, err := a.credentials(acct)
	if err != nil {
		return provider.ErrBadSignature
	}
	ev, err := prov.ParseWebhook(creds, header, body, now)
	if err != nil {
		return err
	}
	if first, err := st.RecordEvent(ctx, ids.New(), acct.TenantID, acct.ID, acct.Provider, ev.ID, ev.Type, ts(now)); err != nil || !first {
		return err
	}
	if ev.CheckoutRef == "" {
		return nil
	}
	ck, err := st.CheckoutByRef(ctx, acct.ID, ev.CheckoutRef)
	if errors.Is(err, payments.ErrNotFound) {
		return nil // not ours, or not created by servly
	} else if err != nil {
		return err
	}
	return a.settle(ctx, ck.ID, now)
}

// refundOnline gives back an online payment through its provider before
// servly records the refund. The key makes a retry after a lost answer safe.
func (a *App) refundOnline(ctx context.Context, p *Principal, pm *payments.Payment, amount money.Amount) error {
	acct, err := paymentsql.New(a.DB).AccountForLocation(ctx, p.TenantID(), p.Location.ID)
	if err != nil || acct.Provider != pm.Provider {
		return payments.ErrNoAccount.WithMessage("reconnect %s in Settings > Payments to refund this payment", pm.Provider)
	}
	creds, prov, err := a.credentials(acct)
	if err != nil {
		return err
	}
	key := fmt.Sprintf("refund-%s-%d-%d", pm.ID, pm.Refunded, amount)
	return prov.Refund(ctx, creds, pm.ProviderRef, amount, pm.Currency, key)
}

func ts(t time.Time) database.Timestamp {
	return database.Timestamp{Time: t.UTC().Truncate(time.Microsecond)}
}

// guestPaymentFor is GetPublicMenu's and GetPublicOrder's view.
func (a *App) guestPaymentFor(ctx context.Context, loc *tenant.Location, kind dining.Kind) GuestPayment {
	gp, _ := a.guestPayment(ctx, a.DB, loc.TenantID, loc.ID, kind)
	return gp
}
