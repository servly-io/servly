package payments

import (
	"errors"
	"testing"

	"github.com/servly-io/servly/internal/money"
)

func TestTransitions(t *testing.T) {
	all := []Status{StatusUnpaid, StatusPending, StatusPaid, StatusFailed, StatusPartiallyRefunded, StatusRefunded}
	allowed := map[[2]Status]bool{
		{StatusPending, StatusPaid}:                        true,
		{StatusPending, StatusFailed}:                      true,
		{StatusPaid, StatusPartiallyRefunded}:              true,
		{StatusPaid, StatusRefunded}:                       true,
		{StatusPartiallyRefunded, StatusPartiallyRefunded}: true,
		{StatusPartiallyRefunded, StatusRefunded}:          true,
	}
	for _, from := range all {
		for _, to := range all {
			err := ValidateTransition(from, to)
			if want := allowed[[2]Status{from, to}]; (err == nil) != want {
				t.Errorf("%s -> %s: err = %v, want allowed=%v", from, to, err, want)
			}
			if err != nil && !errors.Is(err, ErrInvalidTransition) {
				t.Errorf("%s -> %s: wrong error %v", from, to, err)
			}
		}
	}
}

func TestValidateNew(t *testing.T) {
	ok := Payment{Method: MethodCash, Status: StatusPaid, Amount: 1500}
	if err := ok.ValidateNew(1500); err != nil {
		t.Fatal(err)
	}
	cases := map[string]struct {
		p           Payment
		outstanding int64
		want        error
	}{
		"bad method":   {Payment{Method: "card", Status: StatusPaid, Amount: 1}, 1, ErrInvalidMethod},
		"failed":       {Payment{Method: MethodCash, Status: StatusFailed, Amount: 1}, 1, ErrInvalidInitial},
		"refunded":     {Payment{Method: MethodCash, Status: StatusRefunded, Amount: 1}, 1, ErrInvalidInitial},
		"partial":      {Payment{Method: MethodCash, Status: StatusPaid, Amount: 500}, 1500, ErrInvalidAmount},
		"overpay":      {Payment{Method: MethodCash, Status: StatusPaid, Amount: 2000}, 1500, ErrInvalidAmount},
		"nothing owed": {Payment{Method: MethodCash, Status: StatusPaid, Amount: 0}, 0, ErrInvalidAmount},
	}
	for name, c := range cases {
		if err := c.p.ValidateNew(moneyAmount(c.outstanding)); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", name, err, c.want)
		}
	}
}

func TestRefund(t *testing.T) {
	p := Payment{Status: StatusPaid, Amount: 1000}
	if err := p.Refund(300); err != nil || p.Status != StatusPartiallyRefunded || p.Refunded != 300 {
		t.Fatalf("partial refund: %v %+v", err, p)
	}
	if err := p.Refund(800); !errors.Is(err, ErrInvalidRefund) {
		t.Fatalf("over-refund err = %v", err)
	}
	if err := p.Refund(700); err != nil || p.Status != StatusRefunded || p.Refunded != 1000 {
		t.Fatalf("final refund: %v %+v", err, p)
	}
	if err := p.Refund(1); !errors.Is(err, ErrInvalidRefund) {
		t.Fatalf("refund after full refund err = %v", err)
	}
	pending := Payment{Status: StatusPending, Amount: 1000}
	if err := pending.Refund(100); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("refund of pending payment err = %v", err)
	}
}

func TestSummarize(t *testing.T) {
	cases := map[string]struct {
		total int64
		ps    []Payment
		want  Status
	}{
		"none":        {1000, nil, StatusUnpaid},
		"failed":      {1000, []Payment{{Status: StatusFailed, Amount: 1000}}, StatusUnpaid},
		"pending":     {1000, []Payment{{Status: StatusFailed, Amount: 1000}, {Status: StatusPending, Amount: 1000}}, StatusPending},
		"paid":        {1000, []Payment{{Status: StatusPaid, Amount: 1000}}, StatusPaid},
		"zero total":  {0, nil, StatusPaid},
		"part refund": {1000, []Payment{{Status: StatusPartiallyRefunded, Amount: 1000, Refunded: 200}}, StatusPartiallyRefunded},
		"full refund": {1000, []Payment{{Status: StatusRefunded, Amount: 1000, Refunded: 1000}}, StatusRefunded},
		"underpaid":   {1000, []Payment{{Status: StatusPaid, Amount: 400}}, StatusUnpaid},
	}
	for name, c := range cases {
		if got := Summarize(moneyAmount(c.total), c.ps); got != c.want {
			t.Errorf("%s: Summarize = %s, want %s", name, got, c.want)
		}
	}
	if got := Outstanding(1000, []Payment{{Status: StatusPaid, Amount: 400}, {Status: StatusFailed, Amount: 600}}); got != 600 {
		t.Errorf("Outstanding = %d, want 600", got)
	}
}

func moneyAmount(v int64) money.Amount { return money.Amount(v) }
