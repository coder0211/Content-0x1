// Package budgettest provides an in-memory budget usecase.API: flat unit
// prices, a single global limit and per-category per-job limits (OQ-03).
package budgettest

import (
	"context"
	"sync"

	"github.com/shopspring/decimal"

	"github.com/be/acs/api/internal/modules/budget/usecase"
	"github.com/be/acs/api/internal/shared/kernel"
)

// API is the fake.
type API struct {
	mu          sync.Mutex
	Prices      map[string]decimal.Decimal // unit → price; missing → PRICING_MISSING
	GlobalLimit kernel.Money
	PerJob      map[usecase.Category]kernel.Money
	Reserved    map[kernel.ID]kernel.Money
	Spent       kernel.Money
	Usage       []usecase.UsageInput
	// Overrides records jobs whose reservation needed an override (the real
	// service audits these as budget.override): job ID → kind.
	Overrides map[kernel.ID]string
	usageCost map[kernel.ID]kernel.Money
}

// NewAPI returns a fake with the fake-provider seed prices.
func NewAPI() *API {
	return &API{
		Prices: map[string]decimal.Decimal{
			"input_token": decimal.RequireFromString("0.000003"), "output_token": decimal.RequireFromString("0.000015"),
			"image_1024_standard": decimal.RequireFromString("0.04"), "image_1024_high": decimal.RequireFromString("0.17"),
			"video_second_720p": decimal.RequireFromString("0.10"), "video_second_1080p": decimal.RequireFromString("0.15"),
			"char": decimal.RequireFromString("0.000016"),
		},
		GlobalLimit: kernel.USD("1000"),
		PerJob: map[usecase.Category]kernel.Money{
			usecase.CategoryText: kernel.USD("0.50"), usecase.CategoryImage: kernel.USD("2"), usecase.CategoryVideo: kernel.USD("8"),
		},
		Reserved:  map[kernel.ID]kernel.Money{},
		Spent:     kernel.ZeroUSD(),
		usageCost: map[kernel.ID]kernel.Money{},
	}
}

var _ usecase.API = (*API)(nil)

func (a *API) Estimate(_ context.Context, req usecase.EstimateRequest) (usecase.Estimate, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := usecase.Estimate{Total: kernel.ZeroUSD(), PricingVersion: "fake-2026-10"}
	for _, it := range req.Items {
		p, ok := a.Prices[it.Unit]
		if !ok {
			return usecase.Estimate{}, kernel.ErrPricingMissing.WithDetails(kernel.Detail{"unit": it.Unit, "model": it.ModelCode})
		}
		amt := kernel.NewMoney(p.Mul(it.Units), kernel.CurrencyUSD).Round()
		out.Lines = append(out.Lines, usecase.EstimateLine{Item: it.Item, ProviderCode: it.ProviderCode, ModelCode: it.ModelCode, Unit: it.Unit, Units: it.Units, UnitPrice: p, Amount: amt})
		out.Total = out.Total.Add(amt)
	}
	return out, nil
}

func (a *API) reservedTotal() kernel.Money {
	t := kernel.ZeroUSD()
	for _, m := range a.Reserved {
		t = t.Add(m)
	}
	return t
}

func (a *API) Limits(_ context.Context, _ *kernel.ID, c usecase.Category, amount kernel.Money) (usecase.Limits, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	remaining := a.GlobalLimit.Sub(a.Spent).Sub(a.reservedTotal())
	return usecase.Limits{
		PerJobLimit: a.PerJob[c], GlobalRemaining: remaining,
		RequiresOverride: amount.GreaterThan(a.PerJob[c]), Blocked: amount.GreaterThan(remaining),
	}, nil
}

func (a *API) Reserve(_ context.Context, req usecase.ReserveRequest) (usecase.Reservation, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	limit := a.PerJob[req.Category]
	if req.PerJobLimit != nil {
		limit = *req.PerJobLimit
	}
	if req.Amount.GreaterThan(limit) && (req.Override == nil || !req.Override.IsLeadOrAdmin()) {
		return usecase.Reservation{}, kernel.ErrJobBudgetExceeded
	}
	if req.Amount.GreaterThan(limit) {
		if a.Overrides == nil {
			a.Overrides = map[kernel.ID]string{}
		}
		a.Overrides[req.JobID] = "per_job"
	}
	if a.Spent.Add(a.reservedTotal()).Add(req.Amount).GreaterThan(a.GlobalLimit) && (req.Override == nil || !req.Override.IsAdmin()) {
		return usecase.Reservation{}, kernel.ErrBudgetExceeded
	}
	a.Reserved[req.JobID] = req.Amount
	return usecase.Reservation{JobID: req.JobID, Amount: req.Amount}, nil
}

func (a *API) Settle(_ context.Context, jobID kernel.ID) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.Reserved, jobID)
	if c, ok := a.usageCost[jobID]; ok {
		a.Spent = a.Spent.Add(c)
	}
	return nil
}

func (a *API) Release(_ context.Context, jobID kernel.ID) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.Reserved, jobID)
	return nil
}

func (a *API) RecordUsage(_ context.Context, in usecase.UsageInput) (usecase.UsageRecord, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	cost := kernel.ZeroUSD()
	source := usecase.SourceCalculated
	if in.Usage.ReportedCost != nil {
		cost, source = *in.Usage.ReportedCost, usecase.SourceProviderReported
	} else {
		for unit, q := range in.Usage.Units {
			p, ok := a.Prices[unit]
			if !ok {
				return usecase.UsageRecord{}, kernel.ErrPricingMissing
			}
			cost = cost.Add(kernel.NewMoney(p.Mul(decimal.NewFromFloat(q)), kernel.CurrencyUSD))
		}
	}
	if in.Estimated {
		source = usecase.SourceEstimated
	}
	cost = cost.Round()
	prev, ok := a.usageCost[in.JobID]
	if !ok {
		prev = kernel.ZeroUSD()
	}
	a.usageCost[in.JobID] = prev.Add(cost)
	a.Usage = append(a.Usage, in)
	return usecase.UsageRecord{ID: kernel.NewID(), Cost: cost, Source: source, PricingVersion: "fake-2026-10"}, nil
}
