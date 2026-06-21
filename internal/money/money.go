// Package money represents crypto amounts as exact integer base units
// (satoshi, wei, lamport, drop, …) using math/big. Floating-point math is
// never used for balances or transfers: it is the classic source of
// "sent 1000x too much" bugs. All arithmetic is exact.
package money

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
)

// Amount is a quantity of a single asset, stored as an integer number of the
// asset's smallest unit together with how many decimal places that asset uses.
// For example 0.12 BTC is Amount{raw: 12000000, decimals: 8}.
type Amount struct {
	raw      *big.Int
	decimals int
}

var (
	// ErrNegative is returned when a negative value is supplied where a
	// non-negative amount is required.
	ErrNegative = errors.New("money: amount is negative")
	// ErrDecimalsMismatch is returned when two amounts of different precision
	// are combined.
	ErrDecimalsMismatch = errors.New("money: amounts have different decimals")
	// ErrTooPrecise is returned when a decimal string has more fractional
	// digits than the asset supports (silently dropping them could lose funds).
	ErrTooPrecise = errors.New("money: value has more decimal places than the asset supports")
	// ErrBadFormat is returned when a decimal string cannot be parsed.
	ErrBadFormat = errors.New("money: invalid decimal string")
)

// New returns an Amount from a raw base-unit value. The big.Int is copied so
// the caller cannot mutate the Amount afterwards.
func New(raw *big.Int, decimals int) Amount {
	if decimals < 0 {
		panic("money: negative decimals")
	}
	c := new(big.Int)
	if raw != nil {
		c.Set(raw)
	}
	return Amount{raw: c, decimals: decimals}
}

// Zero returns a zero amount with the given precision.
func Zero(decimals int) Amount { return New(big.NewInt(0), decimals) }

// Decimals reports how many fractional digits the asset uses.
func (a Amount) Decimals() int { return a.decimals }

// Raw returns a copy of the underlying base-unit integer.
func (a Amount) Raw() *big.Int {
	c := new(big.Int)
	if a.raw != nil {
		c.Set(a.raw)
	}
	return c
}

// IsZero reports whether the amount is exactly zero.
func (a Amount) IsZero() bool { return a.raw == nil || a.raw.Sign() == 0 }

// Sign returns -1, 0, or +1.
func (a Amount) Sign() int {
	if a.raw == nil {
		return 0
	}
	return a.raw.Sign()
}

// Cmp compares two amounts of equal precision: -1 if a<b, 0 if equal, +1 if a>b.
func (a Amount) Cmp(b Amount) (int, error) {
	if a.decimals != b.decimals {
		return 0, ErrDecimalsMismatch
	}
	return a.Raw().Cmp(b.Raw()), nil
}

// Add returns a+b. Both amounts must share the same precision.
func (a Amount) Add(b Amount) (Amount, error) {
	if a.decimals != b.decimals {
		return Amount{}, ErrDecimalsMismatch
	}
	return New(new(big.Int).Add(a.Raw(), b.Raw()), a.decimals), nil
}

// Sub returns a-b. Both amounts must share the same precision.
func (a Amount) Sub(b Amount) (Amount, error) {
	if a.decimals != b.decimals {
		return Amount{}, ErrDecimalsMismatch
	}
	return New(new(big.Int).Sub(a.Raw(), b.Raw()), a.decimals), nil
}

// pow10 returns 10^n as a big.Int.
func pow10(n int) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil)
}

// ParseDecimal converts a human-readable decimal string (e.g. "0.12",
// "1,234.5", "-3") into an Amount with the given precision. It rejects values
// that carry more fractional digits than the asset supports rather than
// silently truncating them.
func ParseDecimal(s string, decimals int) (Amount, error) {
	if decimals < 0 {
		return Amount{}, fmt.Errorf("money: negative decimals")
	}

	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", "") // tolerate grouping separators
	if s == "" {
		return Amount{}, ErrBadFormat
	}

	neg := false
	switch s[0] {
	case '+':
		s = s[1:]
	case '-':
		neg = true
		s = s[1:]
	}
	if s == "" {
		return Amount{}, ErrBadFormat
	}

	intPart, fracPart := s, ""
	if dot := strings.IndexByte(s, '.'); dot >= 0 {
		intPart = s[:dot]
		fracPart = s[dot+1:]
		if strings.IndexByte(fracPart, '.') >= 0 {
			return Amount{}, ErrBadFormat // more than one decimal point
		}
	}
	if intPart == "" {
		intPart = "0"
	}
	if !isDigits(intPart) || (fracPart != "" && !isDigits(fracPart)) {
		return Amount{}, ErrBadFormat
	}
	if len(fracPart) > decimals {
		return Amount{}, ErrTooPrecise
	}

	// Right-pad the fractional part to exactly `decimals` digits, then the
	// whole string of digits is the base-unit integer.
	fracPart += strings.Repeat("0", decimals-len(fracPart))
	digits := intPart + fracPart

	raw, ok := new(big.Int).SetString(digits, 10)
	if !ok {
		return Amount{}, ErrBadFormat
	}
	if neg {
		raw.Neg(raw)
	}
	return New(raw, decimals), nil
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// String renders the amount as a plain decimal string with trailing zeros
// trimmed (e.g. "0.12", "-3"). It never uses scientific notation.
func (a Amount) String() string {
	if a.raw == nil {
		return "0"
	}
	neg := a.raw.Sign() < 0
	abs := new(big.Int).Abs(a.raw)

	out := abs.String()
	if a.decimals > 0 {
		if len(out) <= a.decimals {
			out = strings.Repeat("0", a.decimals-len(out)+1) + out
		}
		split := len(out) - a.decimals
		intPart := out[:split]
		fracPart := strings.TrimRight(out[split:], "0")
		if fracPart == "" {
			out = intPart
		} else {
			out = intPart + "." + fracPart
		}
	}
	if neg && out != "0" {
		out = "-" + out
	}
	return out
}

// Float64 returns the amount as a float64 in whole units. For DISPLAY and
// USD valuation only — never use it for crypto arithmetic, which must stay in
// exact base-unit integers.
func (a Amount) Float64() float64 {
	if a.raw == nil {
		return 0
	}
	f := new(big.Float).SetInt(a.raw)
	res, _ := new(big.Float).Quo(f, new(big.Float).SetInt(pow10(a.decimals))).Float64()
	return res
}

// MarshalText implements encoding.TextMarshaler so amounts serialize as their
// raw base-unit integer (lossless, precision-independent).
func (a Amount) MarshalText() ([]byte, error) {
	return []byte(a.Raw().String()), nil
}
