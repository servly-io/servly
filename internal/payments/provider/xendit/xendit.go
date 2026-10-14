// Package xendit is the Xendit adapter: Payment Sessions in PAYMENT_LINK mode
// (Xendit's hosted checkout, which replaced its Invoice API), refunds through
// the payment request, and webhooks verified with the callback token. It talks
// to the REST API directly.
//
// The checkout offers what the restaurant enabled in its Xendit dashboard:
// QRIS, e-wallets (OVO, DANA, ShopeePay, LinkAja, GoPay), virtual accounts and
// cards.
package xendit

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/servly-io/servly/internal/money"
	"github.com/servly-io/servly/internal/payments/provider"
)

// Client is the Xendit adapter. The zero value is not usable; call New.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

var _ provider.Provider = (*Client)(nil)

func New() *Client {
	return &Client{BaseURL: "https://api.xendit.co", HTTP: &http.Client{Timeout: 20 * time.Second}}
}

func (c *Client) ID() string { return "xendit" }

// do calls the API with the secret key as the Basic user. Network failures
// and 5xx are ErrUnavailable; 401 is ErrBadCredentials; 403 a key without the
// needed permission; other 4xx carry Xendit's message.
func (c *Client) do(ctx context.Context, key, method, path string, in any, idem string, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, body)
	if err != nil {
		return err
	}
	req.SetBasicAuth(key, "")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if idem != "" {
		req.Header.Set("Idempotency-key", idem)
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
	var e struct {
		ErrorCode string `json:"error_code"`
		Message   string `json:"message"`
	}
	switch {
	case res.StatusCode == http.StatusUnauthorized:
		return provider.ErrBadCredentials
	case res.StatusCode == http.StatusForbidden:
		return provider.ErrBadCredentials.WithMessage("the Xendit key needs write permission for Payment Sessions and Refunds (Settings > API keys)")
	case res.StatusCode >= 500 || res.StatusCode == http.StatusTooManyRequests:
		return provider.ErrUnavailable
	case res.StatusCode == http.StatusNotFound:
		return errNotFound
	case res.StatusCode >= 400:
		_ = json.Unmarshal(raw, &e)
		return provider.ErrRejected.WithMessage("Xendit: %s", strings.TrimSpace(e.ErrorCode+" "+e.Message))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

var errNotFound = errors.New("xendit: not found")

// Verify checks the key by asking for a session that cannot exist: a valid
// key gets 404, a wrong one 401.
func (c *Client) Verify(ctx context.Context, cr provider.Credentials) (*provider.Account, error) {
	k := cr.SecretKey
	if !strings.HasPrefix(k, "xnd_development_") && !strings.HasPrefix(k, "xnd_production_") {
		return nil, provider.ErrBadCredentials.WithMessage("a Xendit secret key starts with xnd_development_ (test) or xnd_production_ (live)")
	}
	if err := c.do(ctx, k, http.MethodGet, "/sessions/ps-000000000000000000000000", nil, "", nil); err != nil && !errors.Is(err, errNotFound) {
		return nil, err
	}
	return &provider.Account{Name: "Xendit account", Country: "ID", Live: strings.HasPrefix(k, "xnd_production_")}, nil
}

// session is the part of a Payment Session servly reads.
type session struct {
	ID               string      `json:"payment_session_id"`
	Status           string      `json:"status"` // ACTIVE, COMPLETED, EXPIRED, CANCELED
	Amount           json.Number `json:"amount"`
	Currency         string      `json:"currency"`
	PaymentLinkURL   string      `json:"payment_link_url"`
	PaymentRequestID string      `json:"payment_request_id"`
}

func (c *Client) CreateCheckout(ctx context.Context, cr provider.Credentials, in provider.CheckoutInput, idem string) (*provider.Checkout, error) {
	if err := checkCurrency(in.Currency); err != nil {
		return nil, err
	}
	req := map[string]any{
		"reference_id": in.Reference,
		"session_type": "PAY",
		"mode":         "PAYMENT_LINK",
		"amount":       int64(in.Amount),
		"currency":     string(in.Currency),
		"country":      "ID",
		"description":  in.Description,
		"expires_at":   in.ExpiresAt.UTC().Format(time.RFC3339),
	}
	// Xendit only takes HTTPS return addresses. Over plain HTTP (a LAN install)
	// the guest finishes on Xendit's page and reopens the order from its link.
	if strings.HasPrefix(in.ReturnURL, "https://") {
		req["success_return_url"], req["cancel_return_url"] = in.ReturnURL, in.ReturnURL
	}
	if in.Locale != "" {
		req["locale"] = in.Locale
	}
	var s session
	if err := c.do(ctx, cr.SecretKey, http.MethodPost, "/sessions", req, idem, &s); err != nil {
		return nil, err
	}
	return &provider.Checkout{Ref: s.ID, URL: s.PaymentLinkURL}, nil
}

func (c *Client) Lookup(ctx context.Context, cr provider.Credentials, ref string) (*provider.Result, error) {
	var s session
	if err := c.do(ctx, cr.SecretKey, http.MethodGet, "/sessions/"+url.PathEscape(ref), nil, "", &s); err != nil {
		if errors.Is(err, errNotFound) {
			return nil, provider.ErrRejected.WithMessage("Xendit does not know session %s", ref)
		}
		return nil, err
	}
	amount, err := provider.WholeAmount(string(s.Amount))
	if err != nil {
		return nil, err
	}
	r := &provider.Result{State: provider.StateOpen, Amount: amount, Currency: money.Currency(strings.ToUpper(s.Currency)),
		PaymentRef: s.PaymentRequestID}
	switch s.Status {
	case "COMPLETED":
		r.State = provider.StatePaid
		// shortcut: the session does not say which channel paid (QRIS, OVO, ...);
		// read the payment request when staff need it.
		r.Detail = "Xendit"
	case "EXPIRED":
		r.State = provider.StateExpired
	case "CANCELED":
		r.State = provider.StateFailed
	}
	return r, nil
}

// ParseWebhook checks the x-callback-token header against the account's
// webhook verification token (Xendit dashboard: Settings > Webhooks). Xendit
// signs nothing else, so the event only tells servly which session to look up.
func (c *Client) ParseWebhook(cr provider.Credentials, h http.Header, body []byte, _ time.Time) (*provider.Event, error) {
	got := h.Get("x-callback-token")
	if cr.WebhookSecret == "" || subtle.ConstantTimeCompare([]byte(got), []byte(cr.WebhookSecret)) != 1 {
		return nil, provider.ErrBadSignature
	}
	var ev struct {
		Event string `json:"event"`
		Data  struct {
			SessionID string `json:"payment_session_id"`
			Status    string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &ev); err != nil || ev.Event == "" {
		return nil, provider.ErrBadSignature.WithMessage("webhook body is not a Xendit event")
	}
	id := h.Get("webhook-id")
	if id == "" {
		id = ev.Event + ":" + ev.Data.SessionID + ":" + ev.Data.Status
	}
	out := &provider.Event{ID: id, Type: ev.Event}
	if strings.HasPrefix(ev.Event, "payment_session.") {
		out.CheckoutRef = ev.Data.SessionID
	}
	return out, nil
}

func (c *Client) Refund(ctx context.Context, cr provider.Credentials, paymentRef string, amount money.Amount, cur money.Currency, idem string) error {
	if err := checkCurrency(cur); err != nil {
		return err
	}
	var r struct {
		Status string `json:"status"`
	}
	req := map[string]any{"payment_request_id": paymentRef, "amount": int64(amount), "currency": string(cur), "reason": "REQUESTED_BY_CUSTOMER"}
	if err := c.do(ctx, cr.SecretKey, http.MethodPost, "/refunds", req, idem, &r); err != nil {
		return err
	}
	if r.Status == "FAILED" {
		return provider.ErrRejected.WithMessage("Xendit could not refund this payment")
	}
	return nil
}

// checkCurrency: servly stores rupiah as whole units, which is what Xendit
// takes. shortcut: IDR only; add PHP, THB, VND, MYR with their units when a
// restaurant outside Indonesia uses Xendit.
func checkCurrency(cur money.Currency) error {
	if cur != "IDR" {
		return provider.ErrCurrency.WithDetails(map[string]any{"currency": cur})
	}
	return nil
}
