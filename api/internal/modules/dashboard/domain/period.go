// Package domain holds the dashboard's pure rules: reporting periods and
// budget usage percentages.
package domain

import (
	"time"

	"github.com/shopspring/decimal"
)

// Periods are the reporting windows of the dashboard, in UTC (the budget
// module's monthly periods are UTC calendar months).
type Periods struct {
	DayStart   time.Time
	MonthStart time.Time
	FailedFrom time.Time // window of the "failed in the last 24h" counter
}

// PeriodsAt returns the windows containing now.
func PeriodsAt(now time.Time) Periods {
	u := now.UTC()
	return Periods{
		DayStart:   time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC),
		MonthStart: time.Date(u.Year(), u.Month(), 1, 0, 0, 0, 0, time.UTC),
		FailedFrom: u.Add(-24 * time.Hour),
	}
}

// UsedPct is spent/limit in percent rounded to one decimal; nil when there
// is no positive limit.
func UsedPct(spent decimal.Decimal, limit *decimal.Decimal) *float64 {
	if limit == nil || !limit.IsPositive() {
		return nil
	}
	f, _ := spent.Div(*limit).Mul(decimal.NewFromInt(100)).Round(1).Float64()
	return &f
}
