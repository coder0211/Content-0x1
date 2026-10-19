// Package usecase builds the dashboard summary (ACS-095, FR-DSH-01).
//
// The dashboard publishes no API. Asset counters come from the asset API;
// campaign, job and cost widgets come from a read-only query port (Reader)
// because the campaign/job/budget APIs expose no aggregate reads. The
// Reader never writes and is implemented by adapter/postgres.
package usecase

import (
	"context"
	"time"

	"github.com/shopspring/decimal"

	"github.com/be/acs/api/internal/shared/kernel"
)

// Campaign is an active campaign row.
type Campaign struct {
	ID        kernel.ID
	Name      string
	Status    string
	UpdatedAt time.Time
}

// Job is a recent job row.
type Job struct {
	ID        kernel.ID
	JobType   string
	Status    string
	Progress  int
	ErrorCode *string
	CreatedAt time.Time
}

// JobCounts are the job widget counters.
type JobCounts struct {
	Queued    int
	Running   int
	Failed24h int
}

// Reader is the dashboard's read-only query port. createdBy nil = all users.
type Reader interface {
	ActiveCampaigns(ctx context.Context, limit int) (count int, recent []Campaign, err error)
	JobCounts(ctx context.Context, createdBy *kernel.ID, failedFrom time.Time) (JobCounts, error)
	// RecentJobs returns running/queued jobs and jobs failed since failedFrom, newest first.
	RecentJobs(ctx context.Context, createdBy *kernel.ID, failedFrom time.Time, limit int) ([]Job, error)
	// Spent sums ai_usage_records.cost_amount since `from`.
	Spent(ctx context.Context, from time.Time) (decimal.Decimal, error)
	// MonthlyLimit is the global budget limit of the month starting at
	// monthStart (or the settings template when the month has no row yet).
	MonthlyLimit(ctx context.Context, monthStart time.Time) (*decimal.Decimal, error)
}
