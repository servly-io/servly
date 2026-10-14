package midtrans

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/servly-io/servly/internal/payments/provider"
)

const key = "SB-Mid-server-abc"

var cr = provider.Credentials{SecretKey: key}

func fake(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &Client{SnapSandbox: srv.URL, APISandbox: srv.URL, SnapLive: "http://live.invalid", APILive: "http://live.invalid", HTTP: srv.Client()}
}

func TestVerify(t *testing.T) {
	c := fake(t, func(w http.ResponseWriter, r *http.Request) {
		if u, _, _ := r.BasicAuth(); u != key {
			fmt.Fprint(w, `{"status_code":"401","status_message":"unauthorized"}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"status_code":"404","status_message":"Transaction doesn't exist."}`)
	})
	if a, err := c.Verify(context.Background(), cr); err != nil || a.Live {
		t.Fatalf("%+v %v", a, err)
	}
	if _, err := c.Verify(context.Background(), provider.Credentials{SecretKey: "SB-Mid-server-wrong"}); !errors.Is(err, provider.ErrBadCredentials) {
		t.Fatalf("wrong key: %v", err)
	}
	if _, err := c.Verify(context.Background(), provider.Credentials{SecretKey: "SB-Mid-client-abc"}); !errors.Is(err, provider.ErrBadCredentials) {
		t.Fatalf("client key: %v", err)
	}
}

func TestCreateAndLookup(t *testing.T) {
	state := `{"status_code":"404"}`
	c := fake(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/snap/v1/transactions":
			var b map[string]any
			_ = json.NewDecoder(r.Body).Decode(&b)
			td := b["transaction_details"].(map[string]any)
			if td["order_id"] != "ck-1" || td["gross_amount"].(float64) != 17000 {
				t.Errorf("request %v", b)
			}
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{"token":"t","redirect_url":"https://app.sandbox.midtrans.com/snap/v4/redirection/t"}`)
		case r.URL.Path == "/v2/ck-1/status":
			fmt.Fprint(w, state)
		}
	})
	ck, err := c.CreateCheckout(context.Background(), cr, provider.CheckoutInput{Reference: "ck-1", Amount: 17000, Currency: "IDR",
		Description: "Roma · Order 3", ReturnURL: "http://x/pay/t", ExpiresAt: time.Now().Add(31 * time.Minute)}, "ck-1")
	if err != nil || ck.Ref != "ck-1" || !strings.Contains(ck.URL, "redirection") {
		t.Fatalf("%+v %v", ck, err)
	}
	for body, want := range map[string]provider.State{
		`{"status_code":"404"}`: provider.StateOpen,
		`{"status_code":"201","transaction_status":"pending","gross_amount":"17000.00","currency":"IDR","payment_type":"qris"}`:                                             provider.StateOpen,
		`{"status_code":"200","transaction_status":"settlement","gross_amount":"17000.00","currency":"IDR","payment_type":"qris","issuer":"dana"}`:                          provider.StatePaid,
		`{"status_code":"200","transaction_status":"capture","fraud_status":"challenge","gross_amount":"17000.00","payment_type":"credit_card"}`:                            provider.StateOpen,
		`{"status_code":"200","transaction_status":"capture","fraud_status":"accept","gross_amount":"17000.00","payment_type":"credit_card","masked_card":"48111111-1114"}`: provider.StatePaid,
		`{"status_code":"202","transaction_status":"deny","gross_amount":"17000.00"}`:                                                                                       provider.StateFailed,
		`{"status_code":"407","transaction_status":"expire","gross_amount":"17000.00"}`:                                                                                     provider.StateExpired,
	} {
		state = body
		res, err := c.Lookup(context.Background(), cr, "ck-1")
		if err != nil || res.State != want {
			t.Errorf("%s: %+v %v", body, res, err)
		}
		if want == provider.StatePaid && (res.Amount != 17000 || res.Currency != "IDR" || res.Detail == "") {
			t.Errorf("%s: %+v", body, res)
		}
	}
	state = `{"status_code":"200","transaction_status":"settlement","gross_amount":"17000.00","currency":"IDR","payment_type":"qris","issuer":"dana"}`
	if res, _ := c.Lookup(context.Background(), cr, "ck-1"); res.Detail != "QRIS · Dana" {
		t.Errorf("detail %q", res.Detail)
	}
}

func sign(orderID, status, gross string) string {
	s := sha512.Sum512([]byte(orderID + status + gross + key))
	return hex.EncodeToString(s[:])
}

func TestParseWebhook(t *testing.T) {
	c := New()
	body := fmt.Sprintf(`{"order_id":"ck-1","status_code":"200","gross_amount":"17000.00","transaction_id":"tx-1","transaction_status":"settlement","signature_key":"%s"}`,
		sign("ck-1", "200", "17000.00"))
	ev, err := c.ParseWebhook(cr, http.Header{}, []byte(body), time.Now())
	if err != nil || ev.CheckoutRef != "ck-1" || ev.ID == "" {
		t.Fatalf("%+v %v", ev, err)
	}
	forged := strings.Replace(body, `"gross_amount":"17000.00"`, `"gross_amount":"1.00"`, 1)
	if _, err := c.ParseWebhook(cr, http.Header{}, []byte(forged), time.Now()); !errors.Is(err, provider.ErrBadSignature) {
		t.Fatalf("forged: %v", err)
	}
}
