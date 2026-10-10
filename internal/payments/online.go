package payments

import (
	"github.com/google/uuid"

	"github.com/servly-io/servly/internal/apperr"
	"github.com/servly-io/servly/internal/money"
	"github.com/servly-io/servly/internal/platform/database"
)

// MethodOnline is a payment taken through a provider's hosted checkout. Staff
// never record it by hand.
const MethodOnline Method = "online"

// When guests pay online, per kind of dining location (docs/docs/payments.md).
type Flow string

const (
	FlowAfter   Flow = "after"   // tables: order, then pay from the phone when finished
	FlowBefore  Flow = "before"  // pay when ordering; the order exists only once paid
	FlowEither  Flow = "either"  // pickup: the guest chooses to pay now or at the counter
	FlowStaff   Flow = "staff"   // tables: no online payment, staff take it
	FlowCounter Flow = "counter" // pickup: no online payment, paid at the counter
)

// ValidTableFlow and ValidPickupFlow report the flows each kind allows.
func ValidTableFlow(f Flow) bool  { return f == FlowAfter || f == FlowBefore || f == FlowStaff }
func ValidPickupFlow(f Flow) bool { return f == FlowBefore || f == FlowEither || f == FlowCounter }

// PayFirst reports whether an order must be paid before it is sent.
func (f Flow) PayFirst() bool { return f == FlowBefore }

// PayLater reports whether a placed, unpaid order may be paid online.
func (f Flow) PayLater() bool { return f == FlowAfter || f == FlowEither }

// Account is a restaurant location's connected provider account.
type Account struct {
	ID          uuid.UUID
	TenantID    uuid.UUID
	LocationID  uuid.UUID
	Provider    string
	Mode        string // test or live
	Status      string // active or disabled
	AccountRef  string
	AccountName string
	Country     string
	TableFlow   Flow
	PickupFlow  Flow
	// Credentials are sealed (secretbox); empty once disconnected.
	Credentials string
	CreatedAt   database.Timestamp
	UpdatedAt   database.Timestamp
}

const (
	AccountActive   = "active"
	AccountDisabled = "disabled"
)

// CheckoutKind tells what a checkout pays for.
type CheckoutKind string

const (
	CheckoutBill   CheckoutKind = "bill"   // an existing order's outstanding amount
	CheckoutPrepay CheckoutKind = "prepay" // a cart; the order is created once paid (ADR 0008)
)

// CheckoutStatus is servly's record of a hosted payment.
type CheckoutStatus string

const (
	CheckoutOpen    CheckoutStatus = "open"
	CheckoutPaid    CheckoutStatus = "paid"
	CheckoutFailed  CheckoutStatus = "failed"
	CheckoutExpired CheckoutStatus = "expired"
)

// Checkout is one hosted payment attempt.
type Checkout struct {
	ID               uuid.UUID
	TenantID         uuid.UUID
	LocationID       uuid.UUID
	AccountID        uuid.UUID
	PublicToken      string
	Kind             CheckoutKind
	OrderID          uuid.NullUUID
	DiningLocationID uuid.NullUUID
	IdempotencyKey   *string
	Draft            string // prepay: the priced order (JSON)
	Provider         string
	ProviderRef      string
	RedirectURL      string
	Amount           money.Amount
	Currency         money.Currency
	Status           CheckoutStatus
	Attempts         int
	NextCheckAt      database.Timestamp
	ExpiresAt        database.Timestamp
	CreatedAt        database.Timestamp
	UpdatedAt        database.Timestamp
}

var (
	ErrNoAccount        = apperr.NotFound("payments_not_connected", "online payments are not set up for this restaurant")
	ErrOnlineNotAllowed = apperr.Invalid("online_payment_not_offered", "online payment is not offered here; pay the staff")
	ErrNothingToPay     = apperr.Invalid("nothing_to_pay", "this order has nothing left to pay")
	ErrPayFirst         = apperr.Invalid("payment_required", "orders here are paid when ordering; use Pay and order")
	ErrOpenCheckouts    = apperr.Conflict("payments_in_progress", "guests are paying right now; disconnect again once their payments finish (at most 30 minutes)")
	ErrSecretKeyMissing = apperr.Invalid("secret_key_missing", "online payments need the server's secret key (secret_key); ask the person who runs servly")
	ErrUnknownProvider  = apperr.Invalid("unknown_provider", "this payment provider is not available")
	ErrInvalidFlow      = apperr.Invalid("invalid_payment_flow", "unknown choice for when guests pay")
)
