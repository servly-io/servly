// Package stripe is the Stripe adapter: Checkout Sessions for guests,
// refunds through the Payment Intent, and signed webhooks. It talks to the
// REST API directly (form-encoded), so servly carries no Stripe SDK.
//
// Payment methods are Stripe's dynamic payment methods: the restaurant turns
// cards, Apple Pay, Google Pay, PayNow, iDEAL and the rest on or off in its
// Stripe dashboard, and Checkout offers what fits the guest.
package stripe

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/servly-io/servly/internal/money"
	"github.com/servly-io/servly/internal/payments/provider"
)

// Client is the Stripe adapter. The zero value is not usable; call New.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

var _ provider.Provider = (*Client)(nil)

func New() *Client {
	return &Client{BaseURL: "https://api.stripe.com", HTTP: &http.Client{Timeout: 20 * time.Second}}
}

func (c *Client) ID() string { return "stripe" }

// webhookTolerance is how old a signed webhook may be (Stripe's own default).
const webhookTolerance = 5 * time.Minute

type apiError struct {
	Error struct {
		Type    string `json:"type"`
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// do calls the API. Network failures and 5xx are ErrUnavailable; a rejected
// key is ErrBadCredentials; other 4xx carry Stripe's message.
func (c *Client) do(ctx context.Context, key, method, path string, form url.Values, idem string, out any) error {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, body)
	if err != nil {
		return err
	}
	req.SetBasicAuth(key, "")
	req.Header.Set("Stripe-Version", "2024-06-20")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if idem != "" {
		req.Header.Set("Idempotency-Key", idem)
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return provider.ErrUnavailable
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return provider.ErrUnavailable
	}
	switch {
	case res.StatusCode == http.StatusUnauthorized:
		return provider.ErrBadCredentials
	case res.StatusCode >= 500 || res.StatusCode == http.StatusTooManyRequests:
		return provider.ErrUnavailable
	case res.StatusCode >= 400:
		var e apiError
		_ = json.Unmarshal(raw, &e)
		if res.StatusCode == http.StatusForbidden {
			return errForbidden{msg: e.Error.Message}
		}
		return provider.ErrRejected.WithMessage("Stripe: %s", e.Error.Message)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

// errForbidden is a restricted key without the permission for one call.
type errForbidden struct{ msg string }

func (e errForbidden) Error() string { return "stripe: permission denied: " + e.msg }

// Verify checks the key. A restricted key may lack "Account: read"; it is
// then checked against Checkout Sessions, which it needs anyway.
func (c *Client) Verify(ctx context.Context, cr provider.Credentials) (*provider.Account, error) {
	k := cr.SecretKey
	if !(strings.HasPrefix(k, "sk_") || strings.HasPrefix(k, "rk_")) {
		return nil, provider.ErrBadCredentials.WithMessage("a Stripe secret key starts with sk_ or a restricted key with rk_")
	}
	live := strings.HasPrefix(k, "sk_live_") || strings.HasPrefix(k, "rk_live_")
	var acct struct {
		ID              string `json:"id"`
		Country         string `json:"country"`
		BusinessProfile struct {
			Name string `json:"name"`
		} `json:"business_profile"`
		Settings struct {
			Dashboard struct {
				DisplayName string `json:"display_name"`
			} `json:"dashboard"`
		} `json:"settings"`
	}
	err := c.do(ctx, k, http.MethodGet, "/v1/account", nil, "", &acct)
	var forbidden errForbidden
	if errors.As(err, &forbidden) {
		if err := c.do(ctx, k, http.MethodGet, "/v1/checkout/sessions?limit=1", nil, "", nil); err != nil {
			return nil, permissionHint(err)
		}
		return &provider.Account{Name: "Stripe account", Live: live}, nil
	}
	if err != nil {
		return nil, err
	}
	name := acct.Settings.Dashboard.DisplayName
	if name == "" {
		name = acct.BusinessProfile.Name
	}
	return &provider.Account{Ref: acct.ID, Name: name, Country: strings.ToUpper(acct.Country), Live: live}, nil
}

func permissionHint(err error) error {
	var forbidden errForbidden
	if errors.As(err, &forbidden) {
		return provider.ErrBadCredentials.WithMessage("the restricted key needs write access to Checkout Sessions and Refunds, and read access to Payment Intents")
	}
	return err
}

func (c *Client) CreateCheckout(ctx context.Context, cr provider.Credentials, in provider.CheckoutInput, idem string) (*provider.Checkout, error) {
	amount, err := toStripe(in.Amount, in.Currency)
	if err != nil {
		return nil, err
	}
	locale := in.Locale
	if locale == "" {
		locale = "auto"
	}
	f := url.Values{
		"mode":                      {"payment"},
		"success_url":               {in.ReturnURL},
		"cancel_url":                {in.ReturnURL},
		"client_reference_id":       {in.Reference},
		"metadata[servly_checkout]": {in.Reference},
		"payment_intent_data[metadata][servly_checkout]": {in.Reference},
		"line_items[0][quantity]":                        {"1"},
		"line_items[0][price_data][currency]":            {strings.ToLower(string(in.Currency))},
		"line_items[0][price_data][unit_amount]":         {strconv.FormatInt(amount, 10)},
		"line_items[0][price_data][product_data][name]":  {in.Description},
		"expires_at": {strconv.FormatInt(in.ExpiresAt.Unix(), 10)},
		"locale":     {locale},
	}
	var s struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := c.do(ctx, cr.SecretKey, http.MethodPost, "/v1/checkout/sessions", f, idem, &s); err != nil {
		return nil, permissionHint(err)
	}
	return &provider.Checkout{Ref: s.ID, URL: s.URL}, nil
}

type session struct {
	ID            string `json:"id"`
	Status        string `json:"status"`         // open, complete, expired
	PaymentStatus string `json:"payment_status"` // paid, unpaid, no_payment_required
	AmountTotal   int64  `json:"amount_total"`
	Currency      string `json:"currency"`
	PaymentIntent *struct {
		ID           string `json:"id"`
		Status       string `json:"status"`
		LatestCharge *struct {
			PaymentMethodDetails struct {
				Type string `json:"type"`
				Card *card  `json:"card"`
			} `json:"payment_method_details"`
		} `json:"latest_charge"`
	} `json:"payment_intent"`
}

func (c *Client) Lookup(ctx context.Context, cr provider.Credentials, ref string) (*provider.Result, error) {
	var s session
	path := "/v1/checkout/sessions/" + url.PathEscape(ref) + "?expand[]=payment_intent.latest_charge"
	if err := c.do(ctx, cr.SecretKey, http.MethodGet, path, nil, "", &s); err != nil {
		return nil, permissionHint(err)
	}
	cur := money.Currency(strings.ToUpper(s.Currency))
	amount, err := fromStripe(s.AmountTotal, cur)
	if err != nil {
		return nil, err
	}
	r := &provider.Result{State: provider.StateOpen, Amount: amount, Currency: cur}
	switch {
	case s.Status == "expired":
		r.State = provider.StateExpired
	case s.Status == "complete" && s.PaymentStatus == "paid":
		r.State = provider.StatePaid
	case s.Status == "complete" && s.PaymentIntent != nil &&
		(s.PaymentIntent.Status == "requires_payment_method" || s.PaymentIntent.Status == "canceled"):
		// A delayed method (a bank transfer, for example) that failed after checkout.
		r.State = provider.StateFailed
	}
	if s.PaymentIntent != nil {
		r.PaymentRef = s.PaymentIntent.ID
		if ch := s.PaymentIntent.LatestCharge; ch != nil {
			r.Detail = detail(ch.PaymentMethodDetails.Type, ch.PaymentMethodDetails.Card)
		}
	}
	return r, nil
}

// detail names how the guest paid: "Apple Pay · Visa •••• 4242", "PayNow".
type card struct {
	Brand  string `json:"brand"`
	Last4  string `json:"last4"`
	Wallet *struct {
		Type string `json:"type"`
	} `json:"wallet"`
}

func detail(typ string, c *card) string {
	if c != nil {
		d := methodNames[c.Brand]
		if d == "" {
			d = c.Brand
		}
		d += " •••• " + c.Last4
		if c.Wallet != nil {
			if w := methodNames[c.Wallet.Type]; w != "" {
				d = w + " · " + d
			}
		}
		return d
	}
	if n := methodNames[typ]; n != "" {
		return n
	}
	return typ
}

var methodNames = map[string]string{
	"visa": "Visa", "mastercard": "Mastercard", "amex": "American Express", "jcb": "JCB", "unionpay": "UnionPay",
	"discover": "Discover", "diners": "Diners Club", "cartes_bancaires": "Cartes Bancaires",
	"apple_pay": "Apple Pay", "google_pay": "Google Pay", "link": "Link", "samsung_pay": "Samsung Pay",
	"paynow": "PayNow", "grabpay": "GrabPay", "fpx": "FPX", "ideal": "iDEAL", "bancontact": "Bancontact",
	"blik": "BLIK", "eps": "EPS", "p24": "Przelewy24", "paypal": "PayPal", "klarna": "Klarna",
	"cashapp": "Cash App Pay", "alipay": "Alipay", "wechat_pay": "WeChat Pay", "twint": "TWINT", "mobilepay": "MobilePay",
}

// ParseWebhook verifies Stripe's signature: Stripe-Signature carries
// t=<unix> and one or more v1=<hex hmac-sha256 of "t.body">.
func (c *Client) ParseWebhook(cr provider.Credentials, h http.Header, body []byte, now time.Time) (*provider.Event, error) {
	if cr.WebhookSecret == "" {
		return nil, provider.ErrBadSignature.WithMessage("no webhook signing secret is configured for this account")
	}
	var ts int64
	var sigs []string
	for _, part := range strings.Split(h.Get("Stripe-Signature"), ",") {
		k, v, _ := strings.Cut(strings.TrimSpace(part), "=")
		switch k {
		case "t":
			ts, _ = strconv.ParseInt(v, 10, 64)
		case "v1":
			sigs = append(sigs, v)
		}
	}
	if ts == 0 || len(sigs) == 0 {
		return nil, provider.ErrBadSignature
	}
	if d := now.Sub(time.Unix(ts, 0)); d > webhookTolerance || d < -webhookTolerance {
		return nil, provider.ErrBadSignature
	}
	mac := hmac.New(sha256.New, []byte(cr.WebhookSecret))
	fmt.Fprintf(mac, "%d.%s", ts, body)
	want := mac.Sum(nil)
	ok := false
	for _, s := range sigs {
		if got, err := hex.DecodeString(s); err == nil && hmac.Equal(got, want) {
			ok = true
		}
	}
	if !ok {
		return nil, provider.ErrBadSignature
	}
	var ev struct {
		ID   string `json:"id"`
		Type string `json:"type"`
		Data struct {
			Object struct {
				ID     string `json:"id"`
				Object string `json:"object"`
			} `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &ev); err != nil || ev.ID == "" {
		return nil, provider.ErrBadSignature.WithMessage("webhook body is not a Stripe event")
	}
	out := &provider.Event{ID: ev.ID, Type: ev.Type}
	if ev.Data.Object.Object == "checkout.session" {
		out.CheckoutRef = ev.Data.Object.ID
	}
	return out, nil
}

func (c *Client) Refund(ctx context.Context, cr provider.Credentials, paymentRef string, amount money.Amount, cur money.Currency, idem string) error {
	a, err := toStripe(amount, cur)
	if err != nil {
		return err
	}
	var r struct {
		Status string `json:"status"`
	}
	f := url.Values{"payment_intent": {paymentRef}, "amount": {strconv.FormatInt(a, 10)}}
	if err := c.do(ctx, cr.SecretKey, http.MethodPost, "/v1/refunds", f, idem, &r); err != nil {
		return permissionHint(err)
	}
	if r.Status == "failed" || r.Status == "canceled" {
		return provider.ErrRejected.WithMessage("Stripe could not refund this payment (%s)", r.Status)
	}
	return nil
}

// Currency units. servly stores minor units by its own table (money.Digits).
// Stripe counts in its smallest unit: none for its zero-decimal currencies,
// cents for the rest, so servly's whole rupiah become Stripe's hundredths.
// shortcut: HUF, TWD and ISK (special rounding rules at Stripe) and
// three-decimal currencies are refused; add them when a restaurant needs one.
var (
	stripeZeroDecimal = set("BIF", "CLP", "DJF", "GNF", "JPY", "KMF", "KRW", "MGA", "PYG", "RWF", "UGX", "VND", "VUV", "XAF", "XOF", "XPF")
	stripeSpecial     = set("HUF", "TWD", "ISK")
)

func set(cs ...string) map[money.Currency]bool {
	m := map[money.Currency]bool{}
	for _, c := range cs {
		m[money.Currency(c)] = true
	}
	return m
}

// scale is what servly's amount is multiplied by to get Stripe's.
func scale(cur money.Currency) (int64, error) {
	servly, stripe := money.Digits(cur), 2
	if stripeZeroDecimal[cur] {
		stripe = 0
	}
	if stripeSpecial[cur] || servly == 3 || stripe < servly {
		return 0, provider.ErrCurrency.WithDetails(map[string]any{"currency": cur})
	}
	f := int64(1)
	for range stripe - servly {
		f *= 10
	}
	return f, nil
}

func toStripe(a money.Amount, cur money.Currency) (int64, error) {
	f, err := scale(cur)
	return int64(a) * f, err
}

func fromStripe(a int64, cur money.Currency) (money.Amount, error) {
	f, err := scale(cur)
	if err != nil {
		return 0, err
	}
	if a%f != 0 {
		return 0, provider.ErrCurrency.WithMessage("Stripe amount %d %s is not a whole %s amount", a, cur, cur)
	}
	return money.Amount(a / f), nil
}
