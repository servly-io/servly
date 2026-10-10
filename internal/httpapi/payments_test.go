package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/servly-io/servly/internal/payments/provider"
	"github.com/servly-io/servly/internal/payments/provider/sandbox"
	"github.com/servly-io/servly/internal/payments/provider/stripe"
	"github.com/servly-io/servly/internal/platform/database"
	"github.com/servly-io/servly/internal/platform/database/dbtest"
	"github.com/servly-io/servly/internal/platform/secretbox"
)

const testSecretKey = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="

// enablePayments gives the shop's server a secret key and the sandbox (and
// optionally a Stripe adapter), and rebuilds the handler so the sandbox
// routes exist. Sessions survive: they live in the database.
func (s *shop) enablePayments(t *testing.T, extra ...provider.Provider) *sandbox.Sandbox {
	t.Helper()
	sb := sandbox.New()
	box, err := secretbox.Parse(testSecretKey)
	if err != nil {
		t.Fatal(err)
	}
	s.app.Secrets = box
	s.app.Providers = map[string]provider.Provider{"sandbox": sb}
	for _, p := range extra {
		s.app.Providers[p.ID()] = p
	}
	s.h = NewHandler(Deps{Log: quiet, App: s.app, Now: func() time.Time { return s.now }})
	return sb
}

func (s *shop) connect(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	return mustCode(t, s.owner.do(http.MethodPut, "/api/admin/payments/account", body), http.StatusOK)
}

func (s *shop) pickupCart() map[string]any {
	return map[string]any{"dining_location_token": s.pickupToken, "notes": "",
		"items": []any{map[string]any{"menu_item_id": s.cola["id"], "quantity": 2}}}
}

// later moves the clock, so guest polls look the checkout up again.
func (s *shop) later() { s.now = s.now.Add(5 * time.Second) }

func sandboxRef(t *testing.T, redirect string) string {
	t.Helper()
	ref, ok := strings.CutPrefix(redirect, "/api/payments/sandbox/")
	if !ok {
		t.Fatalf("redirect %q is not a sandbox page", redirect)
	}
	return ref
}

func (s *shop) payOnSandbox(t *testing.T, ref, result string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/payments/sandbox/"+ref, strings.NewReader(url.Values{"result": {result}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.h.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "/pay/") {
		t.Fatalf("sandbox %s: %d %s", result, rec.Code, rec.Header().Get("Location"))
	}
}

func TestPaymentSetup(t *testing.T) {
	s := newShop(t, dbtest.Open(t, database.SQLite))
	if got := mustCode(t, s.owner.do(http.MethodGet, "/api/admin/payments", nil), http.StatusOK); got["available"] != false || got["account"] != nil {
		t.Fatalf("without secret key: %v", got)
	}
	body := map[string]any{"provider": "sandbox", "secret_key": "sandbox", "table_flow": "after", "pickup_flow": "before"}
	if r := s.owner.do(http.MethodPut, "/api/admin/payments/account", body); r.code() != "secret_key_missing" {
		t.Fatalf("connect without secret key: %d %s", r.Code, r.Body.String())
	}
	s.enablePayments(t)
	if r := s.owner.do(http.MethodPut, "/api/admin/payments/account", map[string]any{"provider": "sandbox", "secret_key": "nope", "table_flow": "after", "pickup_flow": "before"}); r.code() != "payment_credentials_invalid" {
		t.Fatalf("bad key: %d %s", r.Code, r.Body.String())
	}
	if r := s.owner.do(http.MethodPut, "/api/admin/payments/account", map[string]any{"provider": "sandbox", "secret_key": "sandbox", "table_flow": "maybe", "pickup_flow": "before"}); r.code() != "invalid_payment_flow" {
		t.Fatalf("bad flow: %d %s", r.Code, r.Body.String())
	}
	acct := s.connect(t, body)
	if acct["provider"] != "sandbox" || acct["mode"] != "test" || !strings.Contains(acct["webhook_url"].(string), "/api/payments/webhooks/sandbox/") {
		t.Fatalf("account: %v", acct)
	}
	staff := s.staffClient(t, "bob@example.com", "staff")
	mustCode(t, staff.do(http.MethodGet, "/api/admin/payments", nil), http.StatusForbidden)

	menu := func(token string) any {
		return mustCode(t, s.client().do(http.MethodGet, "/api/public/menu/"+token, nil), http.StatusOK)["payment"]
	}
	if menu(s.tableToken) != "after" || menu(s.pickupToken) != "before" {
		t.Fatalf("guest payment: table %v pickup %v", menu(s.tableToken), menu(s.pickupToken))
	}
	// Pay-first pickup points take no unpaid orders.
	if r := s.order("pickup-unpaid", s.pickupCart()); r.code() != "payment_required" {
		t.Fatalf("unpaid pickup order: %d %s", r.Code, r.Body.String())
	}
	mustCode(t, s.owner.do(http.MethodPatch, "/api/admin/payments/account", map[string]any{"table_flow": "staff"}), http.StatusOK)
	if menu(s.tableToken) != "none" {
		t.Fatalf("staff-only tables: %v", menu(s.tableToken))
	}
	mustCode(t, s.owner.do(http.MethodDelete, "/api/admin/payments/account", nil), http.StatusNoContent)
	if menu(s.pickupToken) != "none" || mustCode(t, s.owner.do(http.MethodGet, "/api/admin/payments", nil), http.StatusOK)["account"] != nil {
		t.Fatal("still connected after disconnect")
	}
}

func TestPrepayCheckout(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *database.DB) {
		s := newShop(t, db)
		sb := s.enablePayments(t)
		s.connect(t, map[string]any{"provider": "sandbox", "secret_key": "sandbox", "table_flow": "after", "pickup_flow": "before"})

		guest := s.client()
		start := mustCode(t, guest.do(http.MethodPost, "/api/public/checkouts", s.pickupCart(), "Idempotency-Key", "cart-key-0001"), http.StatusOK)
		again := mustCode(t, guest.do(http.MethodPost, "/api/public/checkouts", s.pickupCart(), "Idempotency-Key", "cart-key-0001"), http.StatusOK)
		if again["checkout_token"] != start["checkout_token"] {
			t.Fatal("same key, new checkout")
		}
		if n := s.countOrders(); n != 0 {
			t.Fatalf("unpaid prepay created %d orders", n)
		}
		token := start["checkout_token"].(string)
		ref := sandboxRef(t, start["redirect_url"].(string))
		if ss, _ := sb.Session(ref); ss.Amount != 600 || ss.Currency != "EUR" {
			t.Fatalf("checkout amount: %+v", ss)
		}
		if got := mustCode(t, guest.do(http.MethodGet, "/api/public/checkouts/"+token, nil), http.StatusOK); got["status"] != "open" {
			t.Fatalf("before paying: %v", got)
		}

		s.payOnSandbox(t, ref, "paid")
		s.later()
		got := mustCode(t, guest.do(http.MethodGet, "/api/public/checkouts/"+token, nil), http.StatusOK)
		if got["status"] != "paid" || got["order_token"] == nil {
			t.Fatalf("after paying: %v", got)
		}
		order := mustCode(t, guest.do(http.MethodGet, "/api/public/orders/"+got["order_token"].(string), nil), http.StatusOK)
		if order["payment_status"] != "paid" || order["status"] != "new" || order["mode"] != "pickup" || order["total_minor"].(float64) != 600 || order["pay_online"] != false {
			t.Fatalf("order: %v", order)
		}

		// The reconciler and a webhook-less retry find nothing left to do.
		s.later()
		s.app.ReconcilePayments(context.Background(), s.now)
		mustCode(t, guest.do(http.MethodGet, "/api/public/checkouts/"+token, nil), http.StatusOK)
		var payments int
		_ = db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM payments WHERE method = 'online'`).Scan(&payments)
		if s.countOrders() != 1 || payments != 1 {
			t.Fatalf("orders %d, online payments %d; want 1 and 1", s.countOrders(), payments)
		}
		list := mustCode(t, s.owner.do(http.MethodGet, "/api/admin/orders?mode=pickup", nil), http.StatusOK)["items"].([]any)
		d := mustCode(t, s.owner.do(http.MethodGet, "/api/admin/orders/"+list[0].(map[string]any)["id"].(string), nil), http.StatusOK)
		pm := d["payments"].([]any)[0].(map[string]any)
		if pm["method"] != "online" || pm["provider"] != "sandbox" || pm["detail"] != "Sandbox card •••• 4242" || pm["recorded_by"] != nil {
			t.Fatalf("payment: %v", pm)
		}

		// Disconnecting waits for open checkouts.
		mustCode(t, guest.do(http.MethodPost, "/api/public/checkouts", s.pickupCart(), "Idempotency-Key", "cart-key-0002"), http.StatusOK)
		if r := s.owner.do(http.MethodDelete, "/api/admin/payments/account", nil); r.code() != "payments_in_progress" {
			t.Fatalf("disconnect with open checkout: %d %s", r.Code, r.Body.String())
		}
	})
}

func TestCheckoutDeclinedAndWrongAmount(t *testing.T) {
	s := newShop(t, dbtest.Open(t, database.SQLite))
	sb := s.enablePayments(t)
	s.connect(t, map[string]any{"provider": "sandbox", "secret_key": "sandbox", "table_flow": "after", "pickup_flow": "before"})
	guest := s.client()

	declined := mustCode(t, guest.do(http.MethodPost, "/api/public/checkouts", s.pickupCart(), "Idempotency-Key", "cart-decline-1"), http.StatusOK)
	s.payOnSandbox(t, sandboxRef(t, declined["redirect_url"].(string)), "failed")
	s.later()
	if got := mustCode(t, guest.do(http.MethodGet, "/api/public/checkouts/"+declined["checkout_token"].(string), nil), http.StatusOK); got["status"] != "failed" || got["order_token"] != nil {
		t.Fatalf("declined: %v", got)
	}

	wrong := mustCode(t, guest.do(http.MethodPost, "/api/public/checkouts", s.pickupCart(), "Idempotency-Key", "cart-wrong-01"), http.StatusOK)
	ref := sandboxRef(t, wrong["redirect_url"].(string))
	sb.SetAmount(ref, 1)
	s.payOnSandbox(t, ref, "paid")
	s.later()
	if got := mustCode(t, guest.do(http.MethodGet, "/api/public/checkouts/"+wrong["checkout_token"].(string), nil), http.StatusOK); got["status"] != "failed" {
		t.Fatalf("wrong amount: %v", got)
	}
	var mismatches int
	_ = s.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM audit_events WHERE action = 'payment.online_mismatch'`).Scan(&mismatches)
	if s.countOrders() != 0 || mismatches != 1 {
		t.Fatalf("orders %d, mismatch audits %d", s.countOrders(), mismatches)
	}
}

func TestBillCheckoutAndRefund(t *testing.T) {
	s := newShop(t, dbtest.Open(t, database.SQLite))
	sb := s.enablePayments(t)
	s.connect(t, map[string]any{"provider": "sandbox", "secret_key": "sandbox", "table_flow": "after", "pickup_flow": "either"})
	placed := mustCode(t, s.order("bill-0001", s.basicOrder()), http.StatusCreated)
	token := placed["order_token"].(string)
	guest := s.client()
	if got := mustCode(t, guest.do(http.MethodGet, "/api/public/orders/"+token, nil), http.StatusOK); got["pay_online"] != true {
		t.Fatalf("pay_online before paying: %v", got)
	}
	start := mustCode(t, guest.do(http.MethodPost, "/api/public/orders/"+token+"/checkout", nil), http.StatusOK)
	again := mustCode(t, guest.do(http.MethodPost, "/api/public/orders/"+token+"/checkout", nil), http.StatusOK)
	if again["checkout_token"] != start["checkout_token"] {
		t.Fatal("second tap opened a second checkout")
	}
	ref := sandboxRef(t, start["redirect_url"].(string))
	s.payOnSandbox(t, ref, "paid")
	s.later()
	s.app.ReconcilePayments(context.Background(), s.now.Add(time.Minute))
	got := mustCode(t, guest.do(http.MethodGet, "/api/public/orders/"+token, nil), http.StatusOK)
	if got["payment_status"] != "paid" || got["pay_online"] != false {
		t.Fatalf("after paying: %v", got)
	}
	if r := guest.do(http.MethodPost, "/api/public/orders/"+token+"/checkout", nil); r.code() != "nothing_to_pay" {
		t.Fatalf("pay twice: %d %s", r.Code, r.Body.String())
	}

	// A manager's refund goes through the provider first.
	manager := s.staffClient(t, "mia@example.com", "manager")
	id := mustCode(t, manager.do(http.MethodGet, "/api/admin/orders?mode=dine_in", nil), http.StatusOK)["items"].([]any)[0].(map[string]any)["id"].(string)
	d := mustCode(t, manager.do(http.MethodGet, "/api/admin/orders/"+id, nil), http.StatusOK)
	pid := d["payments"].([]any)[0].(map[string]any)["id"].(string)
	d = mustCode(t, manager.do(http.MethodPost, "/api/admin/payments/"+pid+"/refund", map[string]any{"amount_minor": 300, "reason": "cold cola"}), http.StatusOK)
	if ss, _ := sb.Session(ref); ss.Refunded != 300 || d["payment_status"] != "partially_refunded" {
		t.Fatalf("refund: provider %d, order %v", ss.Refunded, d["payment_status"])
	}
}

// fakeStripe answers like Stripe for one checkout session that is paid.
type fakeStripe struct {
	mu   sync.Mutex
	paid bool
}

func (f *fakeStripe) handler(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case r.URL.Path == "/v1/account":
		fmt.Fprint(w, `{"id":"acct_1","country":"DE","settings":{"dashboard":{"display_name":"Roma GmbH"}}}`)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/checkout/sessions":
		fmt.Fprint(w, `{"id":"cs_test_1","url":"https://checkout.stripe.com/c/pay/cs_test_1"}`)
	case r.URL.Path == "/v1/checkout/sessions/cs_test_1":
		if !f.paid {
			fmt.Fprint(w, `{"id":"cs_test_1","status":"open","payment_status":"unpaid","amount_total":600,"currency":"eur"}`)
			return
		}
		fmt.Fprint(w, `{"id":"cs_test_1","status":"complete","payment_status":"paid","amount_total":600,"currency":"eur",
			"payment_intent":{"id":"pi_1","status":"succeeded","latest_charge":{"payment_method_details":{"type":"card","card":{"brand":"mastercard","last4":"4444"}}}}}`)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func stripeSign(secret string, ts time.Time, body string) string {
	m := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(m, "%d.%s", ts.Unix(), body)
	return fmt.Sprintf("t=%d,v1=%s", ts.Unix(), hex.EncodeToString(m.Sum(nil)))
}

func TestStripeWebhook(t *testing.T) {
	fs := &fakeStripe{}
	srv := httptest.NewServer(http.HandlerFunc(fs.handler))
	defer srv.Close()
	st := &stripe.Client{BaseURL: srv.URL, HTTP: srv.Client()}

	s := newShop(t, dbtest.Open(t, database.SQLite))
	s.enablePayments(t, st)
	acct := s.connect(t, map[string]any{"provider": "stripe", "secret_key": "sk_test_1", "webhook_secret": "whsec_1", "table_flow": "after", "pickup_flow": "before"})
	if acct["account_name"] != "Roma GmbH" || acct["country"] != "DE" || acct["webhooks"] != true {
		t.Fatalf("account: %v", acct)
	}
	start := mustCode(t, s.client().do(http.MethodPost, "/api/public/checkouts", s.pickupCart(), "Idempotency-Key", "cart-stripe-1"), http.StatusOK)
	if start["redirect_url"] != "https://checkout.stripe.com/c/pay/cs_test_1" {
		t.Fatalf("start: %v", start)
	}
	hook := func(path, sig, body string) resp {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Stripe-Signature", sig)
		rec := httptest.NewRecorder()
		s.h.ServeHTTP(rec, req)
		return resp{rec}
	}
	path := strings.TrimPrefix(acct["webhook_url"].(string), "http://example.com")
	body := `{"id":"evt_1","type":"checkout.session.completed","data":{"object":{"id":"cs_test_1","object":"checkout.session"}}}`

	if r := hook(path, stripeSign("whsec_wrong", s.now, body), body); r.code() != "payment_webhook_invalid" {
		t.Fatalf("forged: %d %s", r.Code, r.Body.String())
	}
	if r := hook("/api/payments/webhooks/stripe/01a126c6-0040-7183-ba22-497d6ecac94c", stripeSign("whsec_1", s.now, body), body); r.code() != "payment_webhook_invalid" {
		t.Fatalf("unknown account: %d %s", r.Code, r.Body.String())
	}
	fs.mu.Lock()
	fs.paid = true
	fs.mu.Unlock()
	for range 2 { // Stripe retries; the second delivery changes nothing
		if r := hook(path, stripeSign("whsec_1", s.now, body), body); r.Code != http.StatusNoContent {
			t.Fatalf("webhook: %d %s", r.Code, r.Body.String())
		}
	}
	if n := s.countOrders(); n != 1 {
		t.Fatalf("orders after webhook: %d", n)
	}
	got := mustCode(t, s.client().do(http.MethodGet, "/api/public/checkouts/"+start["checkout_token"].(string), nil), http.StatusOK)
	if got["status"] != "paid" || got["order_token"] == nil {
		t.Fatalf("checkout: %v", got)
	}
}

// TestSettleRace: the reconciler and guests polling settle the same paid
// checkout at once; exactly one order and one payment come out.
func TestSettleRace(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *database.DB) {
		s := newShop(t, db)
		s.enablePayments(t)
		s.connect(t, map[string]any{"provider": "sandbox", "secret_key": "sandbox", "table_flow": "after", "pickup_flow": "before"})
		start := mustCode(t, s.client().do(http.MethodPost, "/api/public/checkouts", s.pickupCart(), "Idempotency-Key", "cart-race-01"), http.StatusOK)
		s.payOnSandbox(t, sandboxRef(t, start["redirect_url"].(string)), "paid")
		at := s.now.Add(time.Minute)
		var wg sync.WaitGroup
		for i := range 6 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if i%2 == 0 {
					s.app.ReconcilePayments(context.Background(), at)
				} else {
					_, _ = s.app.GetCheckout(context.Background(), start["checkout_token"].(string), at)
				}
			}()
		}
		wg.Wait()
		var payments int
		_ = db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM payments WHERE method = 'online'`).Scan(&payments)
		if s.countOrders() != 1 || payments != 1 {
			t.Fatalf("orders %d, online payments %d; want 1 and 1", s.countOrders(), payments)
		}
	})
}
