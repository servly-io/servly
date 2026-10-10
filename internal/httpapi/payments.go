package httpapi

import (
	"errors"
	"html/template"
	"io"
	"net/http"
	"slices"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/servly-io/servly/internal/app"
	"github.com/servly-io/servly/internal/money"
	"github.com/servly-io/servly/internal/payments"
	"github.com/servly-io/servly/internal/payments/provider"
	"github.com/servly-io/servly/internal/payments/provider/sandbox"
	"github.com/servly-io/servly/internal/tenant"
)

// Online payments (docs/docs/api.md#online-payments).

func (s *server) paymentAdminRoutes(r chi.Router) {
	owner := func(h http.HandlerFunc) http.HandlerFunc { return s.can(tenant.PermManageSettings, h) }
	r.Get("/payments", owner(s.getPayments))
	r.Put("/payments/account", owner(s.connectPayments))
	r.Patch("/payments/account", owner(s.updatePayments))
	r.Delete("/payments/account", owner(s.disconnectPayments))
}

func (s *server) paymentPublicRoutes(r chi.Router) {
	r.Post("/orders/{token}/checkout", s.startBillCheckout)
	r.Post("/checkouts", s.startPrepayCheckout)
	r.Get("/checkouts/{token}", s.getCheckout)
}

// paymentProviderRoutes are called by providers and the sandbox page, not by
// servly's own frontend: no session, no CSRF token.
func (s *server) paymentProviderRoutes(r chi.Router) {
	r.Post("/webhooks/{provider}/{account}", s.paymentWebhook)
	if sb, ok := s.App.Providers["sandbox"].(*sandbox.Sandbox); ok {
		r.Get("/sandbox/{ref}", s.sandboxPage(sb))
		r.Post("/sandbox/{ref}", s.sandboxFinish(sb))
	}
}

type paymentAccountJSON struct {
	Provider    string        `json:"provider"`
	Mode        string        `json:"mode"`
	AccountName string        `json:"account_name"`
	AccountRef  string        `json:"account_ref"`
	Country     string        `json:"country"`
	TableFlow   payments.Flow `json:"table_flow"`
	PickupFlow  payments.Flow `json:"pickup_flow"`
	// WebhookURL is what the owner registers with the provider.
	WebhookURL string `json:"webhook_url"`
	// Webhooks: a signing secret is stored. Without it servly polls.
	Webhooks  bool      `json:"webhooks"`
	UpdatedAt time.Time `json:"updated_at"`
}

type marketJSON struct {
	payments.Market
	Providers []catalogProviderJSON `json:"providers"`
}

type catalogProviderJSON struct {
	payments.CatalogProvider
	// Available: servly can connect this provider today.
	Available bool `json:"available"`
}

func (s *server) paymentAccountView(r *http.Request, a *payments.Account) paymentAccountJSON {
	v := paymentAccountJSON{Provider: a.Provider, Mode: a.Mode, AccountName: a.AccountName, AccountRef: a.AccountRef,
		Country: a.Country, TableFlow: a.TableFlow, PickupFlow: a.PickupFlow, UpdatedAt: a.UpdatedAt.Time,
		WebhookURL: s.publicBase(r) + "/api/payments/webhooks/" + a.Provider + "/" + a.ID.String()}
	v.Webhooks = s.App.HasWebhookSecret(a)
	return v
}

func (s *server) getPayments(w http.ResponseWriter, r *http.Request) {
	offered := s.App.PaymentProviders()
	markets := make([]marketJSON, 0, len(payments.Catalog))
	for _, m := range payments.Catalog {
		mv := marketJSON{Market: m}
		for _, p := range m.Providers {
			mv.Providers = append(mv.Providers, catalogProviderJSON{CatalogProvider: p, Available: slices.Contains(offered, p.ID)})
		}
		markets = append(markets, mv)
	}
	out := map[string]any{"available": s.App.OnlinePaymentsAvailable(), "markets": markets, "sandbox": slices.Contains(offered, "sandbox"), "account": nil}
	acct, err := s.App.GetPaymentAccount(r.Context(), principal(r))
	switch {
	case err == nil:
		out["account"] = s.paymentAccountView(r, acct)
	case !errors.Is(err, payments.ErrNoAccount):
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

type connectRequest struct {
	Provider      string        `json:"provider"`
	SecretKey     string        `json:"secret_key"`
	WebhookSecret string        `json:"webhook_secret"`
	TableFlow     payments.Flow `json:"table_flow"`
	PickupFlow    payments.Flow `json:"pickup_flow"`
}

func (s *server) connectPayments(w http.ResponseWriter, r *http.Request) {
	var req connectRequest
	if err := decodeJSON(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	acct, err := s.App.ConnectPayments(r.Context(), principal(r), app.ConnectInput{Provider: req.Provider,
		Credentials: provider.Credentials{SecretKey: req.SecretKey, WebhookSecret: req.WebhookSecret},
		TableFlow:   req.TableFlow, PickupFlow: req.PickupFlow})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s.paymentAccountView(r, acct))
}

func (s *server) updatePayments(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TableFlow     *payments.Flow `json:"table_flow"`
		PickupFlow    *payments.Flow `json:"pickup_flow"`
		WebhookSecret *string        `json:"webhook_secret"`
	}
	if err := decodeJSON(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	acct, err := s.App.UpdatePayments(r.Context(), principal(r), app.PaymentUpdate{TableFlow: req.TableFlow,
		PickupFlow: req.PickupFlow, WebhookSecret: req.WebhookSecret})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s.paymentAccountView(r, acct))
}

func (s *server) disconnectPayments(w http.ResponseWriter, r *http.Request) {
	if err := s.App.DisconnectPayments(r.Context(), principal(r)); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type checkoutStartJSON struct {
	CheckoutToken string `json:"checkout_token"`
	RedirectURL   string `json:"redirect_url"`
}

func (s *server) startBillCheckout(w http.ResponseWriter, r *http.Request) {
	if !s.public.ordersPerIP.allow(s.clientIP(r)) {
		s.limited(w, r, s.public.ordersPerIP)
		return
	}
	start, err := s.App.StartBillCheckout(r.Context(), chi.URLParam(r, "token"), s.publicBase(r), s.Now())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, checkoutStartJSON{CheckoutToken: start.Token, RedirectURL: start.RedirectURL})
}

// startPrepayCheckout takes the same body and Idempotency-Key as placing an
// order; the order is created once the payment is confirmed.
func (s *server) startPrepayCheckout(w http.ResponseWriter, r *http.Request) {
	if !s.public.ordersPerIP.allow(s.clientIP(r)) {
		s.limited(w, r, s.public.ordersPerIP)
		return
	}
	var req placeOrderRequest
	if err := decodeJSON(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if !s.public.ordersPerTable.allow(req.DiningLocationToken) {
		s.limited(w, r, s.public.ordersPerTable)
		return
	}
	start, err := s.App.StartPrepayCheckout(r.Context(), app.PlaceOrderInput{DiningToken: req.DiningLocationToken,
		Notes: req.Notes, Lines: req.lines(), IdempotencyKey: r.Header.Get("Idempotency-Key")}, s.publicBase(r), s.Now())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, checkoutStartJSON{CheckoutToken: start.Token, RedirectURL: start.RedirectURL})
}

func (s *server) getCheckout(w http.ResponseWriter, r *http.Request) {
	if !s.public.statusPerIP.allow(s.clientIP(r)) {
		s.limited(w, r, s.public.statusPerIP)
		return
	}
	v, err := s.App.GetCheckout(r.Context(), chi.URLParam(r, "token"), s.Now())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var orderToken *string
	if v.OrderToken != "" {
		orderToken = &v.OrderToken
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": v.Status, "kind": v.Kind, "order_token": orderToken, "redirect_url": v.RedirectURL})
}

// maxWebhookBody: provider events are small; anything bigger is not one.
const maxWebhookBody = 64 << 10

func (s *server) paymentWebhook(w http.ResponseWriter, r *http.Request) {
	accountID, err := uuid.Parse(chi.URLParam(r, "account"))
	if err != nil {
		s.fail(w, r, provider.ErrBadSignature)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxWebhookBody))
	if err != nil {
		s.fail(w, r, errBodyTooLarge)
		return
	}
	if err := s.App.PaymentWebhook(r.Context(), chi.URLParam(r, "provider"), accountID, r.Header, body, s.Now()); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

var sandboxTmpl = template.Must(template.New("sandbox").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Sandbox payment</title>
<style>
*{box-sizing:border-box}
body{font-family:system-ui,sans-serif;margin:0;min-height:100vh;display:grid;place-items:center;background:#f3f3f1;color:#141414}
main{background:#fff;border-radius:16px;padding:32px;width:min(400px,calc(100vw - 32px));box-shadow:0 4px 12px rgb(0 0 0/.08);display:flex;flex-direction:column;gap:16px}
.tag{align-self:flex-start;background:#fff0d4;color:#9a5800;font-weight:700;font-size:.75rem;padding:3px 10px;border-radius:999px}
h1{margin:0;font-size:1.4rem}.amount{font-size:2rem;font-weight:800}p{margin:0;color:#6c6c67}
button{font:inherit;font-weight:700;min-height:52px;border-radius:999px;border:1px solid #cdcdc7;background:#fff;cursor:pointer;width:100%}
.pay{background:#141414;color:#fff;border-color:#141414}
</style></head><body><main>
<span class="tag">SANDBOX · NO REAL MONEY</span>
<h1>{{.Description}}</h1>
<div class="amount">{{.Amount}}</div>
<p>This page stands in for a payment provider's checkout. Nothing is charged.</p>
<form method="post"><input type="hidden" name="result" value="paid"><button class="pay">Pay</button></form>
<form method="post"><input type="hidden" name="result" value="failed"><button>Decline</button></form>
</main></body></html>`))

func (s *server) sandboxPage(sb *sandbox.Sandbox) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ss, ok := sb.Session(chi.URLParam(r, "ref"))
		if !ok || ss.State != provider.StateOpen {
			http.Error(w, "This sandbox payment is finished or unknown.", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = sandboxTmpl.Execute(w, map[string]any{"Description": ss.Description, "Amount": money.Format(ss.Amount, ss.Currency)})
	}
}

func (s *server) sandboxFinish(sb *sandbox.Sandbox) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		back, ok := sb.Finish(chi.URLParam(r, "ref"), r.FormValue("result") == "paid")
		if !ok {
			http.Error(w, "This sandbox payment is finished or unknown.", http.StatusNotFound)
			return
		}
		http.Redirect(w, r, back, http.StatusSeeOther)
	}
}
