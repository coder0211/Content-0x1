package kernel

import (
	"fmt"

	"github.com/shopspring/decimal"
)

// CurrencyUSD is the only currency used for AI spend in the MVP (OQ-03).
const CurrencyUSD = "USD"

// Money is an exact decimal amount in a currency. Amounts are stored as
// NUMERIC(14,6) in Postgres.
type Money struct {
	Amount   decimal.Decimal
	Currency string
}

// NewMoney builds Money from a decimal amount.
func NewMoney(amount decimal.Decimal, currency string) Money {
	return Money{Amount: amount, Currency: currency}
}

// USD builds a USD amount from a decimal string such as "0.50". It panics on
// malformed input and is meant for constants and tests.
func USD(amount string) Money {
	return Money{Amount: decimal.RequireFromString(amount), Currency: CurrencyUSD}
}

// ZeroUSD is a zero USD amount.
func ZeroUSD() Money { return Money{Amount: decimal.Zero, Currency: CurrencyUSD} }

func (m Money) sameCurrency(o Money) {
	if m.Currency != o.Currency {
		panic(fmt.Sprintf("kernel.Money: currency mismatch %s vs %s", m.Currency, o.Currency))
	}
}

// Add returns m + o. Mixing currencies is a programming error and panics.
func (m Money) Add(o Money) Money {
	m.sameCurrency(o)
	return Money{Amount: m.Amount.Add(o.Amount), Currency: m.Currency}
}

// Sub returns m - o.
func (m Money) Sub(o Money) Money {
	m.sameCurrency(o)
	return Money{Amount: m.Amount.Sub(o.Amount), Currency: m.Currency}
}

// Mul returns m multiplied by a quantity (e.g. units × unit price).
func (m Money) Mul(q decimal.Decimal) Money {
	return Money{Amount: m.Amount.Mul(q), Currency: m.Currency}
}

// GreaterThan reports m > o.
func (m Money) GreaterThan(o Money) bool {
	m.sameCurrency(o)
	return m.Amount.GreaterThan(o.Amount)
}

// LessThanOrEqual reports m <= o.
func (m Money) LessThanOrEqual(o Money) bool {
	m.sameCurrency(o)
	return m.Amount.LessThanOrEqual(o.Amount)
}

// IsZero reports whether the amount is zero.
func (m Money) IsZero() bool { return m.Amount.IsZero() }

// IsNegative reports whether the amount is below zero.
func (m Money) IsNegative() bool { return m.Amount.IsNegative() }

// Round rounds to the storage precision (6 decimal places).
func (m Money) Round() Money {
	return Money{Amount: m.Amount.Round(6), Currency: m.Currency}
}

// Float64 returns an approximate float, for JSON presentation only.
func (m Money) Float64() float64 {
	f, _ := m.Amount.Float64()
	return f
}

func (m Money) String() string {
	return m.Amount.StringFixed(6) + " " + m.Currency
}
