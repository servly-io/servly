// Package money holds amounts as integer minor units with an explicit
// currency. There is no floating point anywhere in the money path.
//
// Rounding policy: prices are entered and stored in minor units and are
// tax-inclusive; the MVP computes totals by integer addition and
// multiplication only, so no rounding occurs. A future feature that divides
// or applies percentages must round half to even at line level and document
// it here.
package money

import (
	"fmt"
	"math"

	"github.com/servly-io/servly/internal/apperr"
)

// Amount is a quantity of minor currency units (cents for EUR).
type Amount int64

// MaxAmount bounds any single stored amount. It keeps every order total far
// from int64 overflow (lines × quantity × price).
const MaxAmount Amount = 1_000_000_000

// Currency is an ISO 4217 alphabetic code, such as "EUR".
type Currency string

var (
	ErrInvalidCurrency = apperr.Invalid("invalid_currency", "currency must be a three-letter ISO 4217 code")
	ErrAmountRange     = apperr.Invalid("amount_out_of_range", "amount is out of range")
)

// ParseCurrency validates the shape of an ISO 4217 code.
func ParseCurrency(s string) (Currency, error) {
	if len(s) != 3 {
		return "", ErrInvalidCurrency
	}
	for _, c := range s {
		if c < 'A' || c > 'Z' {
			return "", ErrInvalidCurrency
		}
	}
	return Currency(s), nil
}

// Add returns a+b, or ErrAmountRange if the result leaves ±MaxAmount×1000.
func Add(a, b Amount) (Amount, error) {
	s := a + b
	if (b > 0 && s < a) || (b < 0 && s > a) || abs(s) > MaxAmount*1000 {
		return 0, ErrAmountRange
	}
	return s, nil
}

// Mul returns a×n for a non-negative count n, or ErrAmountRange.
func Mul(a Amount, n int) (Amount, error) {
	if n < 0 {
		return 0, ErrAmountRange
	}
	if n == 0 || a == 0 {
		return 0, nil
	}
	if abs(a) > math.MaxInt64/Amount(n) {
		return 0, ErrAmountRange
	}
	p := a * Amount(n)
	if abs(p) > MaxAmount*1000 {
		return 0, ErrAmountRange
	}
	return p, nil
}

func abs(a Amount) Amount {
	if a < 0 {
		return -a
	}
	return a
}

// Minor-unit digits per ISO 4217. The web client uses the browser's Intl
// tables, which agree for every currency listed here.
var (
	zeroDecimal  = map[Currency]bool{"BIF": true, "CLP": true, "DJF": true, "GNF": true, "ISK": true, "JPY": true, "KMF": true, "KRW": true, "PYG": true, "RWF": true, "UGX": true, "VND": true, "VUV": true, "XAF": true, "XOF": true, "XPF": true}
	threeDecimal = map[Currency]bool{"BHD": true, "IQD": true, "JOD": true, "KWD": true, "LYD": true, "OMR": true, "TND": true}
)

// Digits is the number of minor-unit digits of a currency: 2 for EUR, 0 for JPY.
func Digits(c Currency) int {
	switch {
	case zeroDecimal[c]:
		return 0
	case threeDecimal[c]:
		return 3
	}
	return 2
}

// Format renders an amount for people, such as "24.50 EUR". Display only.
func Format(a Amount, c Currency) string {
	d := Digits(c)
	sign := ""
	if a < 0 {
		sign, a = "-", -a
	}
	if d == 0 {
		return fmt.Sprintf("%s%d %s", sign, a, c)
	}
	p := Amount(math.Pow10(d))
	return fmt.Sprintf("%s%d.%0*d %s", sign, a/p, d, a%p, c)
}
