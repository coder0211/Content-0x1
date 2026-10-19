// Package postgres implements the dashboard's read-only Reader.
package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/be/acs/api/internal/modules/dashboard/usecase"
	db "github.com/be/acs/api/internal/platform/postgres"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Reader implements usecase.Reader.
type Reader struct{ pool *pgxpool.Pool }

// NewReader returns a reader on pool.
func NewReader(pool *pgxpool.Pool) *Reader { return &Reader{pool: pool} }

var _ usecase.Reader = (*Reader)(nil)

func (r *Reader) q(ctx context.Context) *Queries { return New(db.Conn(ctx, r.pool)) }

// ActiveCampaigns counts in_progress/review campaigns and lists the latest.
func (r *Reader) ActiveCampaigns(ctx context.Context, limit int) (int, []usecase.Campaign, error) {
	n, err := r.q(ctx).ActiveCampaignCount(ctx)
	if err != nil {
		return 0, nil, err
	}
	rows, err := r.q(ctx).RecentActiveCampaigns(ctx, int32(limit))
	if err != nil {
		return 0, nil, err
	}
	out := make([]usecase.Campaign, 0, len(rows))
	for _, c := range rows {
		out = append(out, usecase.Campaign{ID: c.ID, Name: c.Name, Status: c.Status, UpdatedAt: c.UpdatedAt})
	}
	return int(n), out, nil
}

// JobCounts counts queued/running jobs and failures since failedFrom.
func (r *Reader) JobCounts(ctx context.Context, createdBy *kernel.ID, failedFrom time.Time) (usecase.JobCounts, error) {
	c, err := r.q(ctx).JobCounts(ctx, JobCountsParams{CreatedBy: createdBy, FailedFrom: failedFrom})
	if err != nil {
		return usecase.JobCounts{}, err
	}
	return usecase.JobCounts{Queued: int(c.Queued), Running: int(c.Running), Failed24h: int(c.Failed24h)}, nil
}

// RecentJobs lists active and recently failed jobs.
func (r *Reader) RecentJobs(ctx context.Context, createdBy *kernel.ID, failedFrom time.Time, limit int) ([]usecase.Job, error) {
	rows, err := r.q(ctx).RecentJobs(ctx, RecentJobsParams{CreatedBy: createdBy, FailedFrom: failedFrom, Lim: int32(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]usecase.Job, 0, len(rows))
	for _, j := range rows {
		out = append(out, usecase.Job{ID: j.ID, JobType: j.JobType, Status: j.Status, Progress: int(j.Progress),
			ErrorCode: j.ErrorCode, CreatedAt: j.CreatedAt})
	}
	return out, nil
}

// Spent sums recorded AI cost since from.
func (r *Reader) Spent(ctx context.Context, from time.Time) (decimal.Decimal, error) {
	return r.q(ctx).SpentSince(ctx, from)
}

// MonthlyLimit returns the month's global limit (nil when none).
func (r *Reader) MonthlyLimit(ctx context.Context, monthStart time.Time) (*decimal.Decimal, error) {
	l, err := r.q(ctx).GlobalMonthlyLimit(ctx, pgtype.Date{Time: monthStart, Valid: true})
	if err != nil {
		return nil, err
	}
	if l.IsNegative() {
		return nil, nil
	}
	return &l, nil
}
