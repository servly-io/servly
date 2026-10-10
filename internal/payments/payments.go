// Package payments records payments taken outside servly against orders.
// servly never processes or verifies a card payment and never stores card data.
//
// Payment status is independent of order status.
// Contract: docs/docs/domain-model.md#payment-lifecycle.
package payments

import (
	"context"

	"github.com/google/uuid"

	"github.com/servly-io/servly/internal/apperr"
	"github.com/servly-io/servly/internal/money"
	"github.com/servly-io/servly/internal/platform/database"
)

type Method string

const (
	MethodCash             Method = "cash"
	MethodExternalTerminal Method = "external_terminal"
	MethodOther            Method = "other"
)

func (m Method) Valid() bool {
	return m == MethodCash || m == MethodExternalTerminal || m == MethodOther
}

// Status is the state of one payment record, or, through Summarize, of an
// order as a whole. StatusUnpaid only ever describes an order.
type Status string

const (
	StatusUnpaid            Status = "unpaid"
	StatusPending           Status = "pending"
	StatusPaid              Status = "paid"
	StatusFailed            Status = "failed"
	StatusPartiallyRefunded Status = "partially_refunded"
	StatusRefunded          Status = "refunded"
)

var transitions = map[Status][]Status{
	StatusPending:           {StatusPaid, StatusFailed},
	StatusPaid:              {StatusPartiallyRefunded, StatusRefunded},
	StatusPartiallyRefunded: {StatusPartiallyRefunded, StatusRefunded},
}

var (
	ErrNotFound          = apperr.NotFound("payment_not_found", "payment not found")
	ErrInvalidTransition = apperr.Invalid("invalid_payment_transition", "payment status change not allowed")
	ErrInvalidInitial    = apperr.Invalid("invalid_payment_status", "a payment is recorded as pending or paid")
	ErrInvalidMethod     = apperr.Invalid("invalid_payment_method", "unknown payment method")
	ErrInvalidAmount     = apperr.Invalid("invalid_payment_amount", "payment amount is not valid for this order")
	ErrInvalidRefund     = apperr.Invalid("invalid_refund_amount", "refund must be positive and not exceed the unrefunded amount")
)

// ValidateTransition reports whether a payment may move from one status to
// another. Changing to the status a payment already has is not a transition,
// except a further partial refund.
func ValidateTransition(from, to Status) error {
	for _, s := range transitions[from] {
		if s == to {
			return nil
		}
	}
	return ErrInvalidTransition.WithDetails(map[string]any{"from": from, "to": to})
}

// Payment is one recorded payment against an order.
type Payment struct {
	ID       uuid.UUID
	TenantID uuid.UUID
	OrderID  uuid.UUID
	Method   Method
	Status   Status
	Amount   money.Amount
	Refunded money.Amount
	Currency money.Currency
	// Reference is free text such as a terminal receipt number.
	Reference        string
	RecordedByUserID uuid.NullUUID
	CreatedAt        database.Timestamp
	UpdatedAt        database.Timestamp
}

// ValidateNew checks a payment about to be recorded against an order with the
// given outstanding amount. MVP rule: one payment settles the order in full;
// split and partial payments come later.
func (p *Payment) ValidateNew(outstanding money.Amount) error {
	if !p.Method.Valid() {
		return ErrInvalidMethod
	}
	if p.Status != StatusPending && p.Status != StatusPaid {
		return ErrInvalidInitial
	}
	if p.Amount <= 0 || p.Amount != outstanding {
		return ErrInvalidAmount.WithDetails(map[string]any{"outstanding_minor": outstanding})
	}
	return nil
}

// Refund records a refund of amount and moves the payment to
// partially_refunded or refunded.
func (p *Payment) Refund(amount money.Amount) error {
	if amount <= 0 || amount > p.Amount-p.Refunded {
		return ErrInvalidRefund
	}
	to := StatusPartiallyRefunded
	if p.Refunded+amount == p.Amount {
		to = StatusRefunded
	}
	if err := ValidateTransition(p.Status, to); err != nil {
		return err
	}
	p.Refunded += amount
	p.Status = to
	return nil
}

// Summarize derives the payment status of an order from its payments:
//
//   - any refund recorded: refunded if nothing net remains, else partially_refunded
//   - paid payments cover the total: paid (an order with total 0 is paid)
//   - a payment is pending: pending
//   - otherwise (none, or all failed): unpaid
func Summarize(total money.Amount, ps []Payment) Status {
	var paid, refunded money.Amount
	var pending bool
	for _, p := range ps {
		switch p.Status {
		case StatusPaid, StatusPartiallyRefunded, StatusRefunded:
			paid += p.Amount
			refunded += p.Refunded
		case StatusPending:
			pending = true
		}
	}
	switch {
	case refunded > 0 && paid-refunded <= 0:
		return StatusRefunded
	case refunded > 0:
		return StatusPartiallyRefunded
	case paid >= total:
		return StatusPaid
	case pending:
		return StatusPending
	default:
		return StatusUnpaid
	}
}

// Outstanding is what remains to be paid on an order.
func Outstanding(total money.Amount, ps []Payment) money.Amount {
	var paid money.Amount
	for _, p := range ps {
		switch p.Status {
		case StatusPaid, StatusPartiallyRefunded, StatusRefunded:
			paid += p.Amount
		}
	}
	return max(total-paid, 0)
}

// Repository persists payments. Every method takes the tenant id.
type Repository interface {
	Create(ctx context.Context, p *Payment) error
	ListForOrder(ctx context.Context, tenantID, orderID uuid.UUID) ([]Payment, error)
	Get(ctx context.Context, tenantID, id uuid.UUID) (*Payment, error)
	// Update writes Status and Refunded if the stored status still equals
	// expected, else returns an apperr conflict.
	Update(ctx context.Context, p *Payment, expected Status) error
}
