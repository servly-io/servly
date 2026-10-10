package orders

import (
	"errors"
	"testing"
	"time"

	"github.com/servly-io/servly/internal/tenant"
)

var allStatuses = []Status{StatusNew, StatusAccepted, StatusPreparing, StatusReady, StatusCompleted, StatusCancelled}

func TestForwardTransitions(t *testing.T) {
	forward := map[[2]Status]bool{
		{StatusNew, StatusAccepted}:       true,
		{StatusAccepted, StatusPreparing}: true,
		{StatusPreparing, StatusReady}:    true,
		{StatusReady, StatusCompleted}:    true,
	}
	for _, from := range allStatuses {
		for _, to := range allStatuses {
			if to == StatusCancelled {
				continue
			}
			err := ValidateTransition(from, to, tenant.RoleOwner, "")
			if want := forward[[2]Status{from, to}]; (err == nil) != want {
				t.Errorf("%s -> %s: err = %v, want allowed=%v", from, to, err, want)
			}
			if err != nil && !errors.Is(err, ErrInvalidTransition) {
				t.Errorf("%s -> %s: wrong error %v", from, to, err)
			}
		}
	}
}

func TestCancellation(t *testing.T) {
	cases := []struct {
		from         Status
		staffAllowed bool
		ownerAllowed bool
	}{
		{StatusNew, true, true},
		{StatusAccepted, true, true},
		{StatusPreparing, false, true},
		{StatusReady, false, true},
		{StatusCompleted, false, false},
		{StatusCancelled, false, false},
	}
	for _, c := range cases {
		if err := ValidateTransition(c.from, StatusCancelled, tenant.RoleStaff, "guest left"); (err == nil) != c.staffAllowed {
			t.Errorf("staff cancel from %s: err = %v", c.from, err)
		}
		if err := ValidateTransition(c.from, StatusCancelled, tenant.RoleManager, "guest left"); (err == nil) != c.ownerAllowed {
			t.Errorf("manager cancel from %s: err = %v", c.from, err)
		}
	}
	if err := ValidateTransition(StatusPreparing, StatusCancelled, tenant.RoleStaff, "x"); !errors.Is(err, ErrForbidden) {
		t.Errorf("staff late cancel err = %v, want ErrForbidden", err)
	}
	if err := ValidateTransition(StatusNew, StatusCancelled, tenant.RoleOwner, ""); !errors.Is(err, ErrReasonRequired) {
		t.Errorf("cancel without reason err = %v", err)
	}
}

func TestUnknownRoleCannotTransition(t *testing.T) {
	if err := ValidateTransition(StatusNew, StatusAccepted, tenant.Role(""), ""); !errors.Is(err, ErrForbidden) {
		t.Fatalf("err = %v", err)
	}
}

func TestBusinessDateUsesLocationTimezone(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skip("no tzdata:", err)
	}
	// 23:30 UTC on Oct 10 is already Oct 11 in Berlin.
	at := time.Date(2026, 10, 10, 23, 30, 0, 0, time.UTC)
	if got := BusinessDate(at, berlin); got != "2026-10-11" {
		t.Fatalf("BusinessDate = %s", got)
	}
}
