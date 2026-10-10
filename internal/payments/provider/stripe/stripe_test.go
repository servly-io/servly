package stripe

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/servly-io/servly/internal/money"
	"github.com/servly-io/servly/internal/payments/provider"
)

var cr = provider.Credentials{SecretKey: "sk_test_abc", WebhookSecret: "whsec_test"}

func fake(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &Client{BaseURL: srv.URL, HTTP: srv.Client()}
}

func TestCreateCheckout(t *testing.T) {
	c := fake(t, func(w http.ResponseWriter, r *http.Request) {
		if user, _, _ := r.BasicAuth(); user != "sk_test_abc" || r.Header.Get("Idempotency-Key") != "ck-1" {
			t.Errorf("auth %q idem %q", user, r.Header.Get("Idempotency-Key"))
		}
		_ = r.ParseForm()
		for k, want := range map[string]string{
			"mode": "payment", "client_reference_id": "ck-1", "line_items[0][price_data][currency]": "eur",
			"line_items[0][price_data][unit_amount]": "2450", "success_url": "http://x/pay/t", "locale": "auto",
		} {
			if got := r.PostForm.Get(k); got != want {
				t.Errorf("%s = %q, want %q", k, got, want)
			}
		}
		fmt.Fprint(w, `{"id":"cs_test_1","url":"https://checkout.stripe.com/c/pay/cs_test_1"}`)
	})
	got, err := c.CreateCheckout(context.Background(), cr, provider.CheckoutInput{
		Reference: "ck-1", Amount: 2450, Currency: "EUR", Description: "Table 4", ReturnURL: "http://x/pay/t", ExpiresAt: time.Now().Add(time.Hour),
	}, "ck-1")
	if err != nil || got.Ref != "cs_test_1" || !strings.HasPrefix(got.URL, "https://checkout.stripe.com") {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestLookupStates(t *testing.T) {
	cases := map[string]struct {
		body   string
		state  provider.State
		detail string
	}{
		"paid by Apple Pay": {`{"status":"complete","payment_status":"paid","amount_total":2450,"currency":"eur",
			"payment_intent":{"id":"pi_1","status":"succeeded","latest_charge":{"payment_method_details":{"type":"card",
			"card":{"brand":"visa","last4":"4242","wallet":{"type":"apple_pay"}}}}}}`, provider.StatePaid, "Apple Pay · Visa •••• 4242"},
		"paid by PayNow": {`{"status":"complete","payment_status":"paid","amount_total":2450,"currency":"sgd",
			"payment_intent":{"id":"pi_2","status":"succeeded","latest_charge":{"payment_method_details":{"type":"paynow"}}}}`, provider.StatePaid, "PayNow"},
		"still open":     {`{"status":"open","payment_status":"unpaid","amount_total":2450,"currency":"eur"}`, provider.StateOpen, ""},
		"delayed failed": {`{"status":"complete","payment_status":"unpaid","amount_total":2450,"currency":"eur","payment_intent":{"id":"pi_3","status":"requires_payment_method"}}`, provider.StateFailed, ""},
		"expired":        {`{"status":"expired","payment_status":"unpaid","amount_total":2450,"currency":"eur"}`, provider.StateExpired, ""},
	}
	for name, tc := range cases {
		c := fake(t, func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, "/v1/checkout/sessions/cs_1") || r.URL.Query().Get("expand[]") != "payment_intent.latest_charge" {
				t.Errorf("%s: path %s?%s", name, r.URL.Path, r.URL.RawQuery)
			}
			fmt.Fprint(w, tc.body)
		})
		got, err := c.Lookup(context.Background(), cr, "cs_1")
		if err != nil || got.State != tc.state || got.Detail != tc.detail || got.Amount != 2450 {
			t.Errorf("%s: %+v %v", name, got, err)
		}
	}
}

func TestErrors(t *testing.T) {
	c := fake(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/account":
			w.WriteHeader(http.StatusUnauthorized)
		default:
			w.WriteHeader(http.StatusBadGateway)
		}
	})
	if _, err := c.Verify(context.Background(), cr); !errors.Is(err, provider.ErrBadCredentials) {
		t.Fatalf("401: %v", err)
	}
	if _, err := c.Lookup(context.Background(), cr, "cs_1"); !errors.Is(err, provider.ErrUnavailable) {
		t.Fatalf("502: %v", err)
	}
	if _, err := c.Verify(context.Background(), provider.Credentials{SecretKey: "pk_test_publishable"}); !errors.Is(err, provider.ErrBadCredentials) {
		t.Fatalf("publishable key: %v", err)
	}
}

func TestVerifyRestrictedKey(t *testing.T) {
	c := fake(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/account" {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"error":{"message":"needs rak_account_read"}}`)
			return
		}
		fmt.Fprint(w, `{"data":[]}`)
	})
	a, err := c.Verify(context.Background(), provider.Credentials{SecretKey: "rk_live_x"})
	if err != nil || !a.Live {
		t.Fatalf("%+v %v", a, err)
	}
}

func sign(secret string, ts int64, body string) string {
	m := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(m, "%d.%s", ts, body)
	return fmt.Sprintf("t=%d,v1=%s", ts, hex.EncodeToString(m.Sum(nil)))
}

func TestParseWebhook(t *testing.T) {
	c := New()
	now := time.Unix(1_800_000_000, 0)
	body := `{"id":"evt_1","type":"checkout.session.completed","data":{"object":{"id":"cs_1","object":"checkout.session"}}}`
	h := http.Header{}
	h.Set("Stripe-Signature", sign("whsec_test", now.Unix(), body))
	ev, err := c.ParseWebhook(cr, h, []byte(body), now)
	if err != nil || ev.ID != "evt_1" || ev.CheckoutRef != "cs_1" {
		t.Fatalf("%+v %v", ev, err)
	}
	for name, hdr := range map[string]string{
		"wrong secret": sign("whsec_other", now.Unix(), body),
		"too old":      sign("whsec_test", now.Add(-10*time.Minute).Unix(), body),
		"missing":      "",
	} {
		h.Set("Stripe-Signature", hdr)
		if _, err := c.ParseWebhook(cr, h, []byte(body), now); !errors.Is(err, provider.ErrBadSignature) {
			t.Errorf("%s: %v", name, err)
		}
	}
	h.Set("Stripe-Signature", sign("whsec_test", now.Unix(), body))
	if _, err := c.ParseWebhook(cr, h, []byte(body+" "), now); !errors.Is(err, provider.ErrBadSignature) {
		t.Errorf("changed body: %v", err)
	}
}

func TestCurrencyUnits(t *testing.T) {
	for cur, ok := range map[money.Currency]bool{"EUR": true, "SGD": true, "MYR": true, "USD": true, "IDR": true, "JPY": true,
		"ISK": false, "HUF": false, "KWD": false} {
		_, err := toStripe(100, cur)
		if (err == nil) != ok {
			t.Errorf("%s: %v", cur, err)
		}
	}
}
