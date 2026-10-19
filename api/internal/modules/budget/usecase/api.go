// Package usecase is the public contract of the budget module: pricing,
// estimation, reservations and usage records (SPEC-02 §6).
package usecase

import (
	"context"

	"github.com/shopspring/decimal"

	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Category selects the per-job limit (budget_settings.per_job_limit_*).
type Category string

const (
	CategoryText  Category = "text"
	CategoryImage Category = "image"
	CategoryVideo Category = "video"
)

// CostSource mirrors ai_usage_records.cost_source.
type CostSource string

const (
	SourceProviderReported CostSource = "provider_reported"
	SourceCalculated       CostSource = "calculated"
	SourceEstimated        CostSource = "estimated"
)

// EstimateItem is one priced line requested by the caller, which computes
// the quantities (tokens from prompt length, clip seconds, characters...).
type EstimateItem struct {
	Item         string // "generation", "scene_clips", "voiceover", ...
	ProviderCode string
	ModelCode    string
	Unit         string // model_pricing.unit
	Units        decimal.Decimal
}

// EstimateRequest asks for a cost estimate.
type EstimateRequest struct {
	Category Category
	Items    []EstimateItem
}

// EstimateLine is one priced line (SPEC-04 §2.6 breakdown).
type EstimateLine struct {
	Item         string
	ProviderCode string
	ModelCode    string
	Unit         string
	Units        decimal.Decimal
	UnitPrice    decimal.Decimal
	Amount       kernel.Money
}

// Estimate is the priced result. PricingVersion is stored on the job.
type Estimate struct {
	Lines          []EstimateLine
	Total          kernel.Money
	PricingVersion string
}

// Limits is what remains before a job would be refused.
type Limits struct {
	PerJobLimit       kernel.Money
	CampaignRemaining *kernel.Money // nil when the campaign has no budget
	GlobalRemaining   kernel.Money
	// RequiresOverride is true when the amount exceeds the per-job limit.
	RequiresOverride bool
	// Blocked is true when global/campaign hard-stop would refuse the amount.
	Blocked bool
}

// ReserveRequest holds budget for a job. Call inside the job-creation
// transaction, after job.API.Create returned a new (non-Existing) job.
type ReserveRequest struct {
	JobID      kernel.ID
	CampaignID *kernel.ID
	Amount     kernel.Money
	Category   Category
	// PerJobLimit overrides the configured per-job limit when non-nil.
	PerJobLimit *kernel.Money
	// Override, when non-nil, is the Lead/Admin who explicitly accepted
	// exceeding the per-job limit (audited). Admin may also bypass a global
	// hard stop.
	Override *kernel.Actor
}

// Reservation is a held amount.
type Reservation struct {
	JobID  kernel.ID
	Amount kernel.Money
}

// UsageInput records what one provider call consumed.
type UsageInput struct {
	JobID        kernel.ID
	StepID       *kernel.ID
	CampaignID   *kernel.ID
	UserID       kernel.ID
	ProviderCode string
	ModelCode    string
	JobType      string
	Usage        ports.Usage
	// Estimated marks usage recorded after a timeout where the provider may
	// have charged (cost_source=estimated, job needs_reconciliation).
	Estimated bool
}

// UsageRecord is the priced usage row.
type UsageRecord struct {
	ID             kernel.ID
	Cost           kernel.Money
	Source         CostSource
	PricingVersion string
}

// API is the budget module's public interface.
type API interface {
	// Estimate prices items with the pricing effective now; PRICING_MISSING
	// (503) when a model has no effective price for a unit.
	Estimate(ctx context.Context, req EstimateRequest) (Estimate, error)
	// Limits reports remaining budget for amount (no locking).
	Limits(ctx context.Context, campaignID *kernel.ID, category Category, amount kernel.Money) (Limits, error)
	// Reserve locks budgets (global, then campaign), checks per-job /
	// campaign / global limits and holds amount. Errors:
	// JOB_BUDGET_EXCEEDED, BUDGET_EXCEEDED. Must run in a transaction.
	Reserve(ctx context.Context, req ReserveRequest) (Reservation, error)
	// Settle converts the job's reservation into spend equal to the sum of
	// its usage records and writes ai_jobs.cost_actual/cost_source.
	Settle(ctx context.Context, jobID kernel.ID) error
	// Release frees the reservation without spend (cancel/fail before any
	// provider call). Settle with no usage is equivalent.
	Release(ctx context.Context, jobID kernel.ID) error
	// RecordUsage prices and stores one usage record.
	RecordUsage(ctx context.Context, in UsageInput) (UsageRecord, error)
}
