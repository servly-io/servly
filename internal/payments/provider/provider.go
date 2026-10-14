// Package provider is the contract between servly and a payment provider
// (ADR 0007, docs/docs/payments.md). An adapter only translates between
// servly and the provider's API: business rules, idempotency and state
// changes stay in internal/app. Guests pay on the provider's hosted pages, so
// card data never reaches servly.
package provider

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/servly-io/servly/internal/apperr"
	"github.com/servly-io/servly/internal/money"
)

var (
	ErrBadCredentials = apperr.Invalid("payment_credentials_invalid", "the payment provider did not accept these credentials")
	ErrBadSignature   = apperr.Invalid("payment_webhook_invalid", "webhook signature is missing, wrong or too old")
	ErrUnavailable    = apperr.New(apperr.KindConflict, "payment_provider_unavailable", "the payment provider cannot be reached; try again or pay at the counter")
	ErrRejected       = apperr.Invalid("payment_provider_rejected", "the payment provider rejected the request")
	ErrCurrency       = apperr.Invalid("payment_currency_unsupported", "this currency is not supported for online payments yet")
)

// Credentials are what a restaurant gives servly to act on its provider
// account. They are stored sealed and never shown again in full.
type Credentials struct {
	SecretKey     string `json:"secret_key"`
	WebhookSecret string `json:"webhook_secret,omitempty"`
}

// Account identifies a connected provider account.
type Account struct {
	Ref     string // the provider's account id, if the key may read it
	Name    string
	Country string // ISO 3166-1 alpha-2, if known
	Live    bool   // live money, not a test or sandbox account
}

// CheckoutInput is one hosted payment for one amount.
type CheckoutInput struct {
	Reference   string // servly's checkout id, echoed back by the provider
	Amount      money.Amount
	Currency    money.Currency
	Description string // what the guest sees on the payment page
	ReturnURL   string // where the guest lands after paying or giving up
	ExpiresAt   time.Time
	Locale      string // the guest's language, if the provider supports it; "" = auto
}

// Checkout is a created hosted payment.
type Checkout struct {
	Ref string // the provider's id of the checkout
	URL string // the hosted page the guest is sent to
}

// State is the provider's view of a checkout.
type State string

const (
	StateOpen    State = "open"    // not paid yet, still payable
	StatePaid    State = "paid"    // money taken
	StateFailed  State = "failed"  // will not be paid
	StateExpired State = "expired" // timed out unpaid
)

// Result is the authoritative state of a checkout, read from the provider.
type Result struct {
	State      State
	Amount     money.Amount
	Currency   money.Currency
	PaymentRef string // the provider's payment id, for refunds
	Detail     string // how the guest paid, e.g. "Visa •••• 4242"; never a full card number
}

// Event is a verified webhook. CheckoutRef is empty for events about
// anything other than a checkout.
type Event struct {
	ID          string
	Type        string
	CheckoutRef string
}

// Provider is one payment provider.
type Provider interface {
	ID() string
	// Verify checks credentials and describes the account.
	Verify(ctx context.Context, c Credentials) (*Account, error)
	// CreateCheckout starts a hosted payment. The idempotency key makes a
	// retry return the same checkout.
	CreateCheckout(ctx context.Context, c Credentials, in CheckoutInput, idempotencyKey string) (*Checkout, error)
	// Lookup reads the authoritative state of a checkout.
	Lookup(ctx context.Context, c Credentials, checkoutRef string) (*Result, error)
	// ParseWebhook verifies a webhook and returns its event, or ErrBadSignature.
	ParseWebhook(c Credentials, header http.Header, body []byte, now time.Time) (*Event, error)
	// Refund gives back amount of a payment.
	Refund(ctx context.Context, c Credentials, paymentRef string, amount money.Amount, currency money.Currency, idempotencyKey string) error
}

// WholeAmount parses a provider amount such as "17000" or "17000.00" into whole
// units without floating point; a fraction is refused.
func WholeAmount(s string) (money.Amount, error) {
	n, frac, _ := strings.Cut(strings.TrimSpace(s), ".")
	if strings.Trim(frac, "0") != "" || n == "" {
		return 0, ErrCurrency.WithMessage("amount %q is not a whole rupiah amount", s)
	}
	var a int64
	for _, d := range n {
		if d < '0' || d > '9' || a > int64(money.MaxAmount) {
			return 0, ErrCurrency.WithMessage("amount %q is not a whole rupiah amount", s)
		}
		a = a*10 + int64(d-'0')
	}
	return money.Amount(a), nil
}
