// Package domain holds the budget rules: periods, limits, thresholds and
// the unit vocabulary used by the estimator (SPEC-02 §6). Pure Go.
package domain

import (
	"slices"
	"time"

	"github.com/shopspring/decimal"

	"github.com/be/acs/api/internal/shared/kernel"
)

// Scope of a budget row.
type Scope string

const (
	ScopeGlobal   Scope = "global"
	ScopeCampaign Scope = "campaign"
)

// Budget is one row of budgets: a limit over a period with running
// reserved/spent totals.
type Budget struct {
	ID          kernel.ID
	Scope       Scope
	CampaignID  *kernel.ID
	PeriodStart time.Time // date (UTC midnight)
	PeriodEnd   time.Time // inclusive date
	Limit       decimal.Decimal
	Reserved    decimal.Decimal
	Spent       decimal.Decimal
	Currency    string
	Thresholds  []int
	AlertsSent  []int
	HardStop    bool
	UpdatedAt   time.Time
}

// Committed is spent + reserved (what the limit is checked against).
func (b Budget) Committed() decimal.Decimal { return b.Spent.Add(b.Reserved) }

// Remaining is limit − spent − reserved (may be negative after overrides).
func (b Budget) Remaining() decimal.Decimal { return b.Limit.Sub(b.Committed()) }

// WouldExceed reports whether holding amount pushes committed over the limit.
func (b Budget) WouldExceed(amount decimal.Decimal) bool {
	return b.Committed().Add(amount).GreaterThan(b.Limit)
}

// Blocks reports whether a hard-stop budget refuses amount.
func (b Budget) Blocks(amount decimal.Decimal) bool { return b.HardStop && b.WouldExceed(amount) }

// PercentUsed is committed / limit × 100 (100 when the limit is 0 and
// something is committed, 0 when nothing is).
func (b Budget) PercentUsed() decimal.Decimal {
	if b.Limit.IsZero() {
		if b.Committed().IsPositive() {
			return decimal.NewFromInt(100)
		}
		return decimal.Zero
	}
	return b.Committed().Mul(decimal.NewFromInt(100)).Div(b.Limit)
}

// DueAlerts returns thresholds reached by the current committed amount that
// have not been sent in this period, ascending. Each threshold fires at most
// once per budget row (= per period, ACS-044).
func (b Budget) DueAlerts() []int {
	pct := b.PercentUsed()
	var out []int
	for _, t := range b.Thresholds {
		if pct.GreaterThanOrEqual(decimal.NewFromInt(int64(t))) && !slices.Contains(b.AlertsSent, t) && !slices.Contains(out, t) {
			out = append(out, t)
		}
	}
	slices.Sort(out)
	return out
}

// ResetAlertsBelow drops sent thresholds that are no longer reached (after
// the limit was raised), so they can fire again if spend climbs back.
func (b *Budget) ResetAlertsBelow() {
	pct := b.PercentUsed()
	kept := make([]int, 0, len(b.AlertsSent))
	for _, t := range b.AlertsSent {
		if pct.GreaterThanOrEqual(decimal.NewFromInt(int64(t))) {
			kept = append(kept, t)
		}
	}
	b.AlertsSent = kept
}

// MonthPeriod returns the first and last day (UTC dates) of now's month: the
// period of the lazily created global budget (OQ-03).
func MonthPeriod(now time.Time) (start, end time.Time) {
	now = now.UTC()
	start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	end = start.AddDate(0, 1, -1)
	return start, end
}

// ValidThresholds checks alert thresholds: 1..100, unique, at most 5.
func ValidThresholds(ts []int) bool {
	if len(ts) == 0 || len(ts) > 5 {
		return false
	}
	seen := map[int]bool{}
	for _, t := range ts {
		if t < 1 || t > 100 || seen[t] {
			return false
		}
		seen[t] = true
	}
	return true
}

// NormalizeThresholds returns a sorted copy.
func NormalizeThresholds(ts []int) []int {
	out := slices.Clone(ts)
	slices.Sort(out)
	return out
}

// Settings is the singleton budget_settings row.
type Settings struct {
	PerJobText             decimal.Decimal
	PerJobImage            decimal.Decimal
	PerJobVideo            decimal.Decimal
	GlobalMonthlyLimit     decimal.Decimal
	GlobalThresholds       []int
	GlobalHardStop         bool
	MaxConcurrentVideoJobs int
	Currency               string
	UpdatedBy              *kernel.ID
	UpdatedAt              time.Time
}

// PerJobLimit returns the configured limit for a category ("text", "image",
// "video"); unknown categories use the text limit (the strictest default).
func (s Settings) PerJobLimit(category string) decimal.Decimal {
	switch category {
	case "image":
		return s.PerJobImage
	case "video":
		return s.PerJobVideo
	}
	return s.PerJobText
}

// NewGlobalBudget builds this month's global budget from the settings template.
func (s Settings) NewGlobalBudget(now time.Time) Budget {
	start, end := MonthPeriod(now)
	return Budget{
		ID: kernel.NewID(), Scope: ScopeGlobal, PeriodStart: start, PeriodEnd: end,
		Limit: s.GlobalMonthlyLimit, Reserved: decimal.Zero, Spent: decimal.Zero, Currency: s.Currency,
		Thresholds: slices.Clone(s.GlobalThresholds), AlertsSent: []int{}, HardStop: s.GlobalHardStop, UpdatedAt: now,
	}
}
