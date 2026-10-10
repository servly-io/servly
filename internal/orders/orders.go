// Package orders holds orders, their lines and their lifecycle.
//
// Contract: docs/docs/domain-model.md#order-lifecycle and docs/docs/api.md.
package orders

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/servly-io/servly/internal/apperr"
	"github.com/servly-io/servly/internal/money"
	"github.com/servly-io/servly/internal/payments"
	"github.com/servly-io/servly/internal/platform/database"
)

type Mode string

const (
	ModeDineIn Mode = "dine_in"
	ModePickup Mode = "pickup"
)

// Limits on a public order. The HTTP layer also caps the request body size.
const (
	MaxLines       = 50
	MaxQuantity    = 99
	MaxNotesLength = 500 // characters, for order and line notes
)

var (
	ErrNotFound = apperr.NotFound("order_not_found", "order not found")
	// ErrConflict is returned when the order changed since the client read
	// it; the client refetches and retries.
	ErrConflict = apperr.Conflict("order_conflict", "the order was changed by someone else")
	// ErrIdempotencyMismatch is returned when an idempotency key is reused
	// with a different request body.
	ErrIdempotencyMismatch = apperr.Conflict("idempotency_key_reused", "idempotency key already used for a different order")
)

// Order is a submitted order. Lines and totals are snapshots taken at
// submission; later menu edits never change them.
type Order struct {
	ID               uuid.UUID
	TenantID         uuid.UUID
	LocationID       uuid.UUID
	DiningLocationID uuid.NullUUID
	// Number is the short number shown to staff and customers, unique per
	// location and business date, starting at 1 each day.
	Number        int
	BusinessDate  string // YYYY-MM-DD in the location's timezone
	PublicToken   string
	Mode          Mode
	Status        Status
	PaymentStatus payments.Status
	Currency      money.Currency
	Subtotal      money.Amount
	Total         money.Amount
	Notes         string
	// Version increases with every change; status updates compare it to
	// detect concurrent edits.
	Version   int64
	Lines     []Line
	CreatedAt database.Timestamp
	UpdatedAt database.Timestamp
}

// Line is one ordered product with its chosen options.
type Line struct {
	ID         uuid.UUID
	TenantID   uuid.UUID
	OrderID    uuid.UUID
	MenuItemID uuid.NullUUID // historical reference; may point at an archived item
	Position   int
	Name       string
	Quantity   int
	UnitPrice  money.Amount // item base price at submission
	// OptionsTotal is the sum of the chosen options' deltas, per unit.
	OptionsTotal money.Amount
	// LineTotal = (UnitPrice + OptionsTotal) × Quantity.
	LineTotal money.Amount
	Notes     string
	Options   []LineOption
}

// LineOption is a chosen option, snapshotted.
type LineOption struct {
	ID         uuid.UUID
	TenantID   uuid.UUID
	LineID     uuid.UUID
	OptionID   uuid.NullUUID
	GroupName  string
	OptionName string
	PriceDelta money.Amount
}

// StatusEvent records one status change: who, when, from and to. The creation
// of an order is recorded with an empty From.
type StatusEvent struct {
	ID          uuid.UUID
	TenantID    uuid.UUID
	OrderID     uuid.UUID
	From        Status
	To          Status
	ActorUserID uuid.NullUUID // empty for the customer or the system
	Reason      string
	CreatedAt   database.Timestamp
}

// BusinessDate is the calendar date of t in the location's timezone.
func BusinessDate(t time.Time, loc *time.Location) string {
	return t.In(loc).Format(time.DateOnly)
}

// ListFilter selects orders for the staff board and history.
type ListFilter struct {
	LocationID uuid.UUID
	Statuses   []Status // empty means all
	Mode       Mode     // empty means all
	Limit      int
	// Cursor continues a previous listing; empty starts at the newest order.
	Cursor string
}

// Repository persists orders. Every method on staff data takes the tenant id;
// an order of another tenant is reported as ErrNotFound.
type Repository interface {
	// NextNumber allocates the next order number for a location and date.
	// Call it in the transaction that creates the order.
	NextNumber(ctx context.Context, tenantID, locationID uuid.UUID, businessDate string) (int, error)
	// Create writes the order, its lines and options and its creation event.
	Create(ctx context.Context, o *Order) error
	Get(ctx context.Context, tenantID, id uuid.UUID) (*Order, error)
	// GetByPublicToken serves the public status page. It is the one lookup
	// without a tenant id: the unguessable token is the access check.
	GetByPublicToken(ctx context.Context, token string) (*Order, error)
	List(ctx context.Context, tenantID uuid.UUID, f ListFilter) (orders []Order, nextCursor string, err error)
	// UpdateStatus applies ev if the order's version still equals
	// expectedVersion, increments the version and records ev. Otherwise it
	// returns ErrConflict.
	UpdateStatus(ctx context.Context, tenantID, id uuid.UUID, expectedVersion int64, ev StatusEvent) (*Order, error)
	// SetPaymentStatus stores the summarized payment status and increments
	// the version.
	SetPaymentStatus(ctx context.Context, tenantID, id uuid.UUID, s payments.Status) error
	Events(ctx context.Context, tenantID, id uuid.UUID) ([]StatusEvent, error)
}

// IdempotencyRecord remembers which order a public submission created, so a
// retried submission returns the same order instead of a duplicate.
type IdempotencyRecord struct {
	TenantID    uuid.UUID
	Scope       string // "public_order_create"
	Key         string // client-generated, sent as the Idempotency-Key header
	RequestHash string // SHA-256 of the canonical request
	ResourceID  uuid.UUID
	CreatedAt   database.Timestamp
	ExpiresAt   database.Timestamp
}

// IdempotencyStore persists IdempotencyRecords. Get and Put run in the same
// transaction as the order creation.
type IdempotencyStore interface {
	// Get returns the record for key, or nil if there is none or it expired.
	Get(ctx context.Context, tenantID uuid.UUID, scope, key string) (*IdempotencyRecord, error)
	Put(ctx context.Context, r *IdempotencyRecord) error
	DeleteExpired(ctx context.Context, before database.Timestamp) (int64, error)
}
