package xendit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/servly-io/servly/internal/payments/provider"
)

var cr = provider.Credentials{SecretKey: "xnd_development_abc", WebhookSecret: "cbtoken"}

func fake(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &Client{BaseURL: srv.URL, HTTP: srv.Client()}
}

func TestVerify(t *testing.T) {
	c := fake(t, func(w http.ResponseWriter, r *http.Request) {
		if u, _, _ := r.BasicAuth(); u != "xnd_development_abc" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	a, err := c.Verify(context.Background(), cr)
	if err != nil || a.Live {
		t.Fatalf("%+v %v", a, err)
	}
	if _, err := c.Verify(context.Background(), provider.Credentials{SecretKey: "xnd_production_wrong"}); !errors.Is(err, provider.ErrBadCredentials) {
		t.Fatalf("wrong key: %v", err)
	}
	if _, err := c.Verify(context.Background(), provider.Credentials{SecretKey: "sk_test_x"}); !errors.Is(err, provider.ErrBadCredentials) {
		t.Fatalf("Stripe key: %v", err)
	}
}

func TestCreateAndLookup(t *testing.T) {
	c := fake(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/sessions":
			var b map[string]any
			_ = json.NewDecoder(r.Body).Decode(&b)
			if b["amount"].(float64) != 17000 || b["currency"] != "IDR" || b["reference_id"] != "ck-1" || b["mode"] != "PAYMENT_LINK" ||
				b["success_return_url"] != "https://x/pay/t" || r.Header.Get("Idempotency-key") != "ck-1" {
				t.Errorf("request %v %v", b, r.Header)
			}
			fmt.Fprint(w, `{"payment_session_id":"ps-1","status":"ACTIVE","amount":17000,"currency":"IDR","payment_link_url":"https://xen.to/a"}`)
		case r.URL.Path == "/sessions/ps-1":
			fmt.Fprint(w, `{"payment_session_id":"ps-1","status":"COMPLETED","amount":17000,"currency":"IDR","payment_request_id":"pr-9"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	ck, err := c.CreateCheckout(context.Background(), cr, provider.CheckoutInput{Reference: "ck-1", Amount: 17000, Currency: "IDR",
		ReturnURL: "https://x/pay/t", ExpiresAt: time.Now().Add(31 * time.Minute)}, "ck-1")
	if err != nil || ck.Ref != "ps-1" || ck.URL != "https://xen.to/a" {
		t.Fatalf("%+v %v", ck, err)
	}
	res, err := c.Lookup(context.Background(), cr, "ps-1")
	if err != nil || res.State != provider.StatePaid || res.Amount != 17000 || res.Currency != "IDR" || res.PaymentRef != "pr-9" {
		t.Fatalf("%+v %v", res, err)
	}
	if _, err := c.CreateCheckout(context.Background(), cr, provider.CheckoutInput{Amount: 950, Currency: "EUR"}, "x"); !errors.Is(err, provider.ErrCurrency) {
		t.Fatalf("EUR: %v", err)
	}
}

func TestParseWebhook(t *testing.T) {
	c := New()
	body := []byte(`{"event":"payment_session.completed","data":{"payment_session_id":"ps-1","status":"COMPLETED"}}`)
	h := http.Header{}
	h.Set("x-callback-token", "cbtoken")
	ev, err := c.ParseWebhook(cr, h, body, time.Now())
	if err != nil || ev.CheckoutRef != "ps-1" || ev.ID == "" {
		t.Fatalf("%+v %v", ev, err)
	}
	h.Set("x-callback-token", "wrong")
	if _, err := c.ParseWebhook(cr, h, body, time.Now()); !errors.Is(err, provider.ErrBadSignature) {
		t.Fatalf("wrong token: %v", err)
	}
	if _, err := c.ParseWebhook(provider.Credentials{SecretKey: "x"}, http.Header{}, body, time.Now()); !errors.Is(err, provider.ErrBadSignature) {
		t.Fatalf("no token configured: %v", err)
	}
}

func TestWholeAmount(t *testing.T) {
	for in, want := range map[string]int64{"17000": 17000, "17000.00": 17000, "0": 0} {
		if got, err := provider.WholeAmount(in); err != nil || int64(got) != want {
			t.Errorf("%q: %d %v", in, got, err)
		}
	}
	for _, bad := range []string{"170.50", "", "-1", "1e5", "abc"} {
		if _, err := provider.WholeAmount(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}
