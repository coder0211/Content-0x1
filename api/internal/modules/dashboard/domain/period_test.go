package domain

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestPeriodsAt(t *testing.T) {
	now := time.Date(2026, 10, 19, 2, 30, 0, 0, time.FixedZone("ICT", 7*3600)) // 2026-10-18 19:30 UTC
	p := PeriodsAt(now)
	if !p.DayStart.Equal(time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)) || !p.MonthStart.Equal(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("%+v", p)
	}
	if !p.FailedFrom.Equal(now.Add(-24 * time.Hour)) {
		t.Fatalf("failed from %v", p.FailedFrom)
	}
}

func TestUsedPct(t *testing.T) {
	lim := decimal.NewFromInt(1000)
	zero := decimal.Zero
	for _, tc := range []struct {
		spent string
		limit *decimal.Decimal
		want  *float64
	}{
		{"812.345", &lim, ptr(81.2)},
		{"0", &lim, ptr(0)},
		{"1200", &lim, ptr(120)},
		{"5", nil, nil},
		{"5", &zero, nil},
	} {
		got := UsedPct(decimal.RequireFromString(tc.spent), tc.limit)
		if (got == nil) != (tc.want == nil) || (got != nil && *got != *tc.want) {
			t.Errorf("%s/%v: got %v", tc.spent, tc.limit, got)
		}
	}
}

func ptr(f float64) *float64 { return &f }
