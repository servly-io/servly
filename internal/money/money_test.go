package money

import (
	"errors"
	"math"
	"testing"
)

func TestParseCurrency(t *testing.T) {
	if c, err := ParseCurrency("EUR"); err != nil || c != "EUR" {
		t.Fatalf("EUR: %q %v", c, err)
	}
	for _, s := range []string{"", "eur", "EURO", "E1R", "€"} {
		if _, err := ParseCurrency(s); !errors.Is(err, ErrInvalidCurrency) {
			t.Errorf("ParseCurrency(%q) err = %v", s, err)
		}
	}
}

func TestArithmetic(t *testing.T) {
	if s, err := Add(1050, -50); err != nil || s != 1000 {
		t.Fatalf("Add = %d %v", s, err)
	}
	if p, err := Mul(1250, 3); err != nil || p != 3750 {
		t.Fatalf("Mul = %d %v", p, err)
	}
	if _, err := Add(math.MaxInt64, 1); !errors.Is(err, ErrAmountRange) {
		t.Fatalf("Add overflow err = %v", err)
	}
	if _, err := Mul(math.MaxInt64/2, 3); !errors.Is(err, ErrAmountRange) {
		t.Fatalf("Mul overflow err = %v", err)
	}
	if _, err := Mul(10, -1); !errors.Is(err, ErrAmountRange) {
		t.Fatalf("Mul negative count err = %v", err)
	}
}
