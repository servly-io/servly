// Package midtrans is the Midtrans adapter: Snap's hosted payment page for
// guests, the Core API for status and refunds, and HTTP notifications
// verified by their signature_key. It talks to the REST API directly.
//
// The page offers what the restaurant enabled in its Midtrans dashboard:
// QRIS, GoPay, ShopeePay, virtual accounts and cards.
package midtrans

import (
	"bytes"
	"context"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/servly-io/servly/internal/money"
	"github.com/servly-io/servly/internal/payments/provider"
)

// Client is the Midtrans adapter. The zero value is not usable; call New.
// A sandbox server key (SB-Mid-server-...) selects the sandbox hosts.
type Client struct {
	// Base URLs by environment; tests point all four at one fake server.
	SnapLive, SnapSandbox, APILive, APISandbox string
	HTTP                                       *http.Client
}

var _ provider.Provider = (*Client)(nil)

func New() *Client {
	return &Client{
		SnapLive: "https://app.midtrans.com", SnapSandbox: "https://app.sandbox.midtrans.com",
		APILive: "https://api.midtrans.com", APISandbox: "https://api.sandbox.midtrans.com",
		HTTP: &http.Client{Timeout: 20 * time.Second},
	}
}

func (c *Client) ID() string { return "midtrans" }

func sandbox(key string) bool { return strings.HasPrefix(key, "SB-") }

func (c *Client) snap(key string) string {
	if sandbox(key) {
		return c.SnapSandbox
	}
	return c.SnapLive
}

func (c *Client) api(key string) string {
	if sandbox(key) {
		return c.APISandbox
	}
	return c.APILive
}

// coreReply is the envelope of every Core API answer: errors come back as a
// status_code in the body, often with HTTP 200.
type coreReply struct {
	StatusCode    string   `json:"status_code"`
	StatusMessage string   `json:"status_message"`
	ErrorMessages []string `json:"error_messages"`
}

// do calls the API with the server key as the Basic user and returns the
// body. Network failures and 5xx are ErrUnavailable; 401 ErrBadCredentials.
func (c *Client) do(ctx context.Context, key, method, u string, in any) ([]byte, error) {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(key, "")
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, provider.ErrUnavailable
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return nil, provider.ErrUnavailable
	}
	var r coreReply
	_ = json.Unmarshal(raw, &r)
	switch {
	case res.StatusCode == http.StatusUnauthorized || r.StatusCode == "401":
		return nil, provider.ErrBadCredentials
	case res.StatusCode >= 500 || res.StatusCode == http.StatusTooManyRequests || strings.HasPrefix(r.StatusCode, "5"):
		return nil, provider.ErrUnavailable
	case res.StatusCode >= 400 && res.StatusCode != http.StatusNotFound:
		msg := r.StatusMessage
		if len(r.ErrorMessages) > 0 {
			msg = strings.Join(r.ErrorMessages, "; ")
		}
		return nil, provider.ErrRejected.WithMessage("Midtrans: %s", msg)
	}
	return raw, nil
}

// Verify checks the key by asking for a transaction that cannot exist: a
// valid key gets status_code 404, a wrong one 401.
func (c *Client) Verify(ctx context.Context, cr provider.Credentials) (*provider.Account, error) {
	k := cr.SecretKey
	if !strings.HasPrefix(k, "SB-Mid-server-") && !strings.HasPrefix(k, "Mid-server-") {
		return nil, provider.ErrBadCredentials.WithMessage("a Midtrans server key starts with SB-Mid-server- (sandbox) or Mid-server- (production)")
	}
	if _, err := c.do(ctx, k, http.MethodGet, c.api(k)+"/v2/servly-key-check/status", nil); err != nil {
		return nil, err
	}
	return &provider.Account{Name: "Midtrans account", Country: "ID", Live: !sandbox(k)}, nil
}

func (c *Client) CreateCheckout(ctx context.Context, cr provider.Credentials, in provider.CheckoutInput, _ string) (*provider.Checkout, error) {
	if in.Currency != "IDR" {
		// shortcut: rupiah only; Midtrans' other currencies are card-only.
		return nil, provider.ErrCurrency.WithDetails(map[string]any{"currency": in.Currency})
	}
	minutes := max(1, int(time.Until(in.ExpiresAt).Minutes()))
	req := map[string]any{
		// order_id is servly's checkout id: Midtrans refuses a second
		// transaction with the same id, which makes retries safe.
		"transaction_details": map[string]any{"order_id": in.Reference, "gross_amount": int64(in.Amount)},
		"item_details":        []map[string]any{{"id": "order", "name": truncate(in.Description, 50), "price": int64(in.Amount), "quantity": 1}},
		"callbacks":           map[string]any{"finish": in.ReturnURL},
		"expiry":              map[string]any{"unit": "minutes", "duration": minutes},
	}
	raw, err := c.do(ctx, cr.SecretKey, http.MethodPost, c.snap(cr.SecretKey)+"/snap/v1/transactions", req)
	if err != nil {
		return nil, err
	}
	var s struct {
		Token       string `json:"token"`
		RedirectURL string `json:"redirect_url"`
	}
	if err := json.Unmarshal(raw, &s); err != nil || s.RedirectURL == "" {
		return nil, provider.ErrRejected.WithMessage("Midtrans did not return a payment page")
	}
	return &provider.Checkout{Ref: in.Reference, URL: s.RedirectURL}, nil
}

type status struct {
	coreReply
	TransactionID     string `json:"transaction_id"`
	TransactionStatus string `json:"transaction_status"`
	FraudStatus       string `json:"fraud_status"`
	GrossAmount       string `json:"gross_amount"`
	Currency          string `json:"currency"`
	PaymentType       string `json:"payment_type"`
	Issuer            string `json:"issuer"`
	MaskedCard        string `json:"masked_card"`
	Bank              string `json:"bank"`
}

func (c *Client) Lookup(ctx context.Context, cr provider.Credentials, ref string) (*provider.Result, error) {
	raw, err := c.do(ctx, cr.SecretKey, http.MethodGet, c.api(cr.SecretKey)+"/v2/"+url.PathEscape(ref)+"/status", nil)
	if err != nil {
		return nil, err
	}
	var s status
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, provider.ErrUnavailable
	}
	// Until the guest picks a method on the Snap page there is no
	// transaction yet: still open.
	if s.StatusCode == "404" {
		return &provider.Result{State: provider.StateOpen}, nil
	}
	amount, err := provider.WholeAmount(s.GrossAmount)
	if err != nil {
		return nil, err
	}
	cur := money.Currency(strings.ToUpper(s.Currency))
	if cur == "" {
		cur = "IDR"
	}
	r := &provider.Result{State: provider.StateOpen, Amount: amount, Currency: cur, PaymentRef: ref}
	switch s.TransactionStatus {
	case "settlement", "refund", "partial_refund":
		r.State = provider.StatePaid
	case "capture":
		// A card capture counts only once fraud screening accepted it.
		if s.FraudStatus == "" || s.FraudStatus == "accept" {
			r.State = provider.StatePaid
		}
	case "deny", "cancel", "failure":
		r.State = provider.StateFailed
	case "expire":
		r.State = provider.StateExpired
	}
	if r.State == provider.StatePaid {
		r.Detail = detail(s)
	}
	return r, nil
}

var methodNames = map[string]string{
	"qris": "QRIS", "gopay": "GoPay", "shopeepay": "ShopeePay", "credit_card": "Card", "bank_transfer": "Bank transfer",
	"echannel": "Mandiri Bill", "cstore": "Convenience store", "akulaku": "Akulaku", "kredivo": "Kredivo", "dana": "DANA", "ovo": "OVO",
}

// detail names how the guest paid: "QRIS · DANA", "Card •••• 1112", "Bank transfer · BCA".
func detail(s status) string {
	d := methodNames[s.PaymentType]
	if d == "" {
		d = s.PaymentType
	}
	switch {
	case s.MaskedCard != "":
		return d + " •••• " + s.MaskedCard[max(0, len(s.MaskedCard)-4):]
	case s.Issuer != "":
		return d + " · " + strings.ToUpper(s.Issuer[:1]) + s.Issuer[1:]
	case s.Bank != "":
		return d + " · " + strings.ToUpper(s.Bank)
	}
	return d
}

// ParseWebhook verifies a payment notification by its signature_key:
// SHA-512 of order_id + status_code + gross_amount + the server key.
func (c *Client) ParseWebhook(cr provider.Credentials, _ http.Header, body []byte, _ time.Time) (*provider.Event, error) {
	var n struct {
		OrderID           string `json:"order_id"`
		StatusCode        string `json:"status_code"`
		GrossAmount       string `json:"gross_amount"`
		SignatureKey      string `json:"signature_key"`
		TransactionID     string `json:"transaction_id"`
		TransactionStatus string `json:"transaction_status"`
	}
	if err := json.Unmarshal(body, &n); err != nil || n.OrderID == "" || n.SignatureKey == "" {
		return nil, provider.ErrBadSignature
	}
	sum := sha512.Sum512([]byte(n.OrderID + n.StatusCode + n.GrossAmount + cr.SecretKey))
	if subtle.ConstantTimeCompare([]byte(strings.ToLower(n.SignatureKey)), []byte(hex.EncodeToString(sum[:]))) != 1 {
		return nil, provider.ErrBadSignature
	}
	return &provider.Event{ID: fmt.Sprintf("%s:%s:%s", n.TransactionID, n.TransactionStatus, n.StatusCode),
		Type: n.TransactionStatus, CheckoutRef: n.OrderID}, nil
}

// Refund refunds through the Core API, keyed by the order id. Midtrans
// supports it for cards, GoPay, ShopeePay and QRIS; bank transfers are
// refunded by the restaurant outside Midtrans.
func (c *Client) Refund(ctx context.Context, cr provider.Credentials, paymentRef string, amount money.Amount, cur money.Currency, idem string) error {
	if cur != "IDR" {
		return provider.ErrCurrency.WithDetails(map[string]any{"currency": cur})
	}
	// refund_key is at most 50 characters; a digest keeps servly's key unique.
	key := sha512.Sum512([]byte(idem))
	req := map[string]any{"refund_key": hex.EncodeToString(key[:])[:40], "amount": int64(amount), "reason": "Refunded by the restaurant"}
	raw, err := c.do(ctx, cr.SecretKey, http.MethodPost, c.api(cr.SecretKey)+"/v2/"+url.PathEscape(paymentRef)+"/refund", req)
	if err != nil {
		return err
	}
	var r coreReply
	_ = json.Unmarshal(raw, &r)
	if r.StatusCode != "200" && r.StatusCode != "201" {
		return provider.ErrRejected.WithMessage("Midtrans could not refund this payment: %s", r.StatusMessage)
	}
	return nil
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}
