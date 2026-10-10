package orders

import (
	"github.com/servly-io/servly/internal/apperr"
	"github.com/servly-io/servly/internal/tenant"
)

// Status is the preparation state of an order. It is independent of the
// payment status.
type Status string

const (
	StatusNew       Status = "new"
	StatusAccepted  Status = "accepted"
	StatusPreparing Status = "preparing"
	StatusReady     Status = "ready"
	StatusCompleted Status = "completed"
	StatusCancelled Status = "cancelled"
)

// The one documented workflow:
//
//	new -> accepted -> preparing -> ready -> completed
//
// and cancelled from any state before completed. completed and cancelled are
// final. There is no way back; a wrong order is cancelled and placed again.
var next = map[Status]Status{
	StatusNew:       StatusAccepted,
	StatusAccepted:  StatusPreparing,
	StatusPreparing: StatusReady,
	StatusReady:     StatusCompleted,
}

var (
	ErrInvalidTransition = apperr.Invalid("invalid_transition", "order status change not allowed")
	ErrReasonRequired    = apperr.Invalid("reason_required", "a cancellation needs a reason")
	ErrForbidden         = apperr.Forbidden("forbidden", "your role does not permit this order change")
)

// Valid reports whether s is a known status.
func (s Status) Valid() bool {
	_, ok := next[s]
	return ok || s == StatusCompleted || s == StatusCancelled
}

// Final reports whether no transition leaves s.
func (s Status) Final() bool { return s == StatusCompleted || s == StatusCancelled }

// Next returns the status that follows s in the normal workflow, if any.
func (s Status) Next() (Status, bool) {
	n, ok := next[s]
	return n, ok
}

// ValidateTransition checks a status change requested by a member with the
// given role. reason is required for a cancellation. It returns
// ErrInvalidTransition, ErrReasonRequired or ErrForbidden.
func ValidateTransition(from, to Status, role tenant.Role, reason string) error {
	details := map[string]any{"from": from, "to": to}
	switch {
	case from.Final() || !from.Valid():
		return ErrInvalidTransition.WithDetails(details)
	case to == StatusCancelled:
		if reason == "" {
			return ErrReasonRequired
		}
	case next[from] != to:
		return ErrInvalidTransition.WithDetails(details)
	}
	if !role.Can(TransitionPermission(from, to)) {
		return ErrForbidden.WithDetails(details)
	}
	return nil
}

// TransitionPermission is the permission a status change needs. Cancelling an
// order whose preparation has begun needs more than moving it forward.
func TransitionPermission(from, to Status) tenant.Permission {
	if to == StatusCancelled && (from == StatusPreparing || from == StatusReady) {
		return tenant.PermCancelStartedOrder
	}
	return tenant.PermUpdateOrderStatus
}
