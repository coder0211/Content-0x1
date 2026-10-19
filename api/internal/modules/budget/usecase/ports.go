package usecase

import (
	"context"
	"encoding/json"
	"time"

	"github.com/shopspring/decimal"

	"github.com/be/acs/api/internal/modules/budget/domain"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Price is one effective model_pricing row.
type Price struct {
	UnitPrice decimal.Decimal
	Currency  string
	Version   string
}

// ReservationRow is one budget_reservations row joined with its budget scope.
type ReservationRow struct {
	ID       kernel.ID
	BudgetID kernel.ID
	Scope    domain.Scope
	JobID    kernel.ID
	Amount   decimal.Decimal
	Status   string // held | settled | released
}

// UsageRow is a new ai_usage_records row.
type UsageRow struct {
	ID             kernel.ID
	JobID          kernel.ID
	StepID         *kernel.ID
	CampaignID     *kernel.ID
	UserID         kernel.ID
	ProviderCode   string
	ModelCode      string
	JobType        string
	UsageJSON      json.RawMessage
	Cost           decimal.Decimal
	Source         CostSource
	Currency       string
	PricingVersion string
	OccurredAt     time.Time
}

// UnsettledUsage is the usage of a job not yet moved into budgets.spent.
type UnsettledUsage struct {
	Total      decimal.Decimal
	Count      int64
	CampaignID *kernel.ID
}

// JobUsageTotals aggregates every usage record of a job (ACS-042: job
// cost_actual = sum of its usage records).
type JobUsageTotals struct {
	Total            decimal.Decimal
	Count            int64
	ProviderReported int64
	Calculated       int64
	Estimated        int64
}

// SummaryFilter selects usage for GET /usage/summary.
type SummaryFilter struct {
	From    time.Time // inclusive, UTC midnight
	To      time.Time // exclusive, UTC midnight
	GroupBy string
}

// SummaryCell is one (group key, cost_source) aggregate from the database.
type SummaryCell struct {
	Key    string
	Source CostSource
	Amount decimal.Decimal
	Count  int64
}

// AlertEmail is one queued budget alert e-mail.
type AlertEmail struct {
	ID         kernel.ID
	BudgetID   kernel.ID
	Threshold  int
	Recipients []string
	Subject    string
	Body       string
	Attempts   int
}

// Repository is the budget module's persistence port. Methods taking
// forUpdate lock the row (SELECT … FOR UPDATE) until the transaction ends;
// callers lock global before campaign budgets (deadlock-free order).
type Repository interface {
	// Pricing.
	EffectivePrice(ctx context.Context, providerCode, modelCode, unit string, at time.Time) (Price, error)
	DefaultModel(ctx context.Context, kind string) (providerCode, modelCode string, err error)

	// Settings.
	GetSettings(ctx context.Context) (domain.Settings, error)
	SaveSettings(ctx context.Context, s domain.Settings) error

	// Budgets.
	EnsureGlobalBudget(ctx context.Context, b domain.Budget) error
	GetGlobalBudget(ctx context.Context, periodStart time.Time, forUpdate bool) (domain.Budget, error)
	GetCampaignBudget(ctx context.Context, campaignID kernel.ID, forUpdate bool) (domain.Budget, error)
	GetBudget(ctx context.Context, id kernel.ID, forUpdate bool) (domain.Budget, error)
	ListCampaignBudgets(ctx context.Context) ([]domain.Budget, error)
	// InsertBudget creates a campaign budget; NOT_FOUND if the campaign does not exist.
	InsertBudget(ctx context.Context, b domain.Budget) error
	SaveBudget(ctx context.Context, b domain.Budget) error
	// CampaignNames maps campaign ids to display names (usage summary labels).
	CampaignNames(ctx context.Context, ids []kernel.ID) (map[kernel.ID]string, error)

	// Reservations.
	HeldReservations(ctx context.Context, jobID kernel.ID) ([]ReservationRow, error)
	// UpsertReservation holds amount on a budget for a job (a retried job
	// re-uses its settled/released row).
	UpsertReservation(ctx context.Context, budgetID, jobID kernel.ID, amount decimal.Decimal) error
	SetReservationStatus(ctx context.Context, id kernel.ID, status string, at time.Time) error

	// Usage.
	InsertUsage(ctx context.Context, u UsageRow) error
	UnsettledUsage(ctx context.Context, jobID kernel.ID) (UnsettledUsage, error)
	MarkUsageSettled(ctx context.Context, jobID kernel.ID, at time.Time) error
	JobUsageTotals(ctx context.Context, jobID kernel.ID) (JobUsageTotals, error)
	// UpdateJobCost writes ai_jobs.cost_actual / cost_source (SPEC-02 §6.3 settle).
	UpdateJobCost(ctx context.Context, jobID kernel.ID, actual decimal.Decimal, source CostSource, needsReconciliation bool) error
	MarkJobNeedsReconciliation(ctx context.Context, jobID kernel.ID) error
	UsageSummary(ctx context.Context, f SummaryFilter) ([]SummaryCell, error)

	// Alert e-mail outbox.
	QueueAlertEmail(ctx context.Context, e AlertEmail) error
	PendingAlertEmails(ctx context.Context, maxAttempts, limit int) ([]AlertEmail, error)
	MarkAlertEmailSent(ctx context.Context, id kernel.ID, at time.Time) error
	MarkAlertEmailFailed(ctx context.Context, id kernel.ID, msg string) error
}
