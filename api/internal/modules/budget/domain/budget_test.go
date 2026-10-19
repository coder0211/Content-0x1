package domain

import (
	"slices"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func TestDueAlertsAndReset(t *testing.T) {
	b := Budget{Limit: d("100"), Spent: d("50"), Reserved: d("30"), Thresholds: []int{80, 100}}
	if got := b.DueAlerts(); !slices.Equal(got, []int{80}) {
		t.Fatalf("80%%: %v", got)
	}
	b.AlertsSent = []int{80}
	if got := b.DueAlerts(); len(got) != 0 {
		t.Fatalf("sent once: %v", got)
	}
	b.Spent = d("70")
	if got := b.DueAlerts(); !slices.Equal(got, []int{100}) {
		t.Fatalf("100%%: %v", got)
	}
	b.AlertsSent = []int{80, 100}
	b.Limit = d("1000")
	b.ResetAlertsBelow()
	if len(b.AlertsSent) != 0 {
		t.Fatalf("raised limit resets: %v", b.AlertsSent)
	}
}

func TestBudgetMath(t *testing.T) {
	b := Budget{Limit: d("10"), Spent: d("4"), Reserved: d("5"), HardStop: true}
	if !b.Remaining().Equal(d("1")) || b.WouldExceed(d("1")) || !b.WouldExceed(d("1.01")) {
		t.Fatal("remaining/would exceed")
	}
	if !b.PercentUsed().Equal(d("90")) {
		t.Fatal(b.PercentUsed())
	}
	z := Budget{Limit: decimal.Zero}
	if !z.PercentUsed().IsZero() {
		t.Fatal("empty zero-limit budget")
	}
}

func TestMonthPeriod(t *testing.T) {
	s, e := MonthPeriod(time.Date(2026, 2, 14, 23, 0, 0, 0, time.UTC))
	if s.Format(time.DateOnly) != "2026-02-01" || e.Format(time.DateOnly) != "2026-02-28" {
		t.Fatal(s, e)
	}
}

func TestThresholdsAndUnits(t *testing.T) {
	for _, tc := range []struct {
		ts []int
		ok bool
	}{{[]int{80, 100}, true}, {nil, false}, {[]int{0}, false}, {[]int{101}, false}, {[]int{50, 50}, false}} {
		if ValidThresholds(tc.ts) != tc.ok {
			t.Errorf("%v", tc.ts)
		}
	}
	if ImageUnit("", "") != "image_1024_standard" || ImageUnit("1024", "HIGH") != "image_1024_high" || VideoSecondUnit("720p") != "video_second_720p" {
		t.Fatal("units")
	}
	if EstimateInputTokens(10) != 4+PromptOverheadTokens || !IsInternalUnit(UnitRenderSecond) {
		t.Fatal("tokens")
	}
	s := Settings{PerJobText: d("0.5"), PerJobImage: d("2"), PerJobVideo: d("8")}
	if !s.PerJobLimit("video").Equal(d("8")) || !s.PerJobLimit("x").Equal(d("0.5")) {
		t.Fatal("per job")
	}
}
