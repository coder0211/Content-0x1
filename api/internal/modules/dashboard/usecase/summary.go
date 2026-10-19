package usecase

import (
	"context"
	"time"

	"github.com/shopspring/decimal"

	assetuc "github.com/be/acs/api/internal/modules/asset/usecase"
	"github.com/be/acs/api/internal/modules/dashboard/domain"
	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Widget sizes.
const (
	RecentCampaigns = 5
	RecentJobs      = 5
)

// CampaignWidget lists active campaigns.
type CampaignWidget struct {
	ActiveCount int
	Recent      []Campaign
}

// JobWidget shows the actor's jobs (or all jobs with provider.view).
type JobWidget struct {
	Scope string // own | all
	JobCounts
	Recent []Job
}

// ReviewWidget holds asset counters; PendingReview is nil without asset.review.
type ReviewWidget struct {
	PendingReview      *int
	MyPendingReview    int
	MyChangesRequested int
	MyDrafts           int
}

// CostWidget is today's and this month's actual cost (usage.view only).
type CostWidget struct {
	Currency     string
	Today        decimal.Decimal
	Month        decimal.Decimal
	MonthlyLimit *decimal.Decimal
	MonthUsedPct *float64
}

// Summary is GET /dashboard/summary. Nil widgets are hidden by permission.
type Summary struct {
	Campaigns   *CampaignWidget
	Jobs        JobWidget
	Reviews     ReviewWidget
	Cost        *CostWidget
	GeneratedAt time.Time
}

// Service builds the summary.
type Service struct {
	Reader Reader
	Assets assetuc.API
	Clock  ports.Clock
}

// Summary returns the widgets the actor may see (auth only).
func (s *Service) Summary(ctx context.Context, actor kernel.Actor) (Summary, error) {
	if !actor.IsAuthenticated() {
		return Summary{}, kernel.ErrUnauthenticated
	}
	now := s.Clock.Now().UTC()
	p := domain.PeriodsAt(now)
	out := Summary{GeneratedAt: now}

	if actor.Can(kernel.PermBrandView) {
		n, recent, err := s.Reader.ActiveCampaigns(ctx, RecentCampaigns)
		if err != nil {
			return Summary{}, err
		}
		out.Campaigns = &CampaignWidget{ActiveCount: n, Recent: nonNil(recent)}
	}

	var owner *kernel.ID
	out.Jobs.Scope = "all"
	if !actor.Can(kernel.PermProviderView) {
		id := actor.UserID
		owner, out.Jobs.Scope = &id, "own"
	}
	counts, err := s.Reader.JobCounts(ctx, owner, p.FailedFrom)
	if err != nil {
		return Summary{}, err
	}
	jobs, err := s.Reader.RecentJobs(ctx, owner, p.FailedFrom, RecentJobs)
	if err != nil {
		return Summary{}, err
	}
	out.Jobs.JobCounts, out.Jobs.Recent = counts, nonNil(jobs)

	mine, err := s.Assets.CountByStatus(ctx, &actor.UserID)
	if err != nil {
		return Summary{}, err
	}
	out.Reviews = ReviewWidget{
		MyPendingReview: mine[assetuc.StatusPendingReview], MyChangesRequested: mine[assetuc.StatusChangesRequested],
		MyDrafts: mine[assetuc.StatusDraft],
	}
	if actor.Can(kernel.PermAssetReview) {
		all, err := s.Assets.CountByStatus(ctx, nil)
		if err != nil {
			return Summary{}, err
		}
		n := all[assetuc.StatusPendingReview]
		out.Reviews.PendingReview = &n
	}

	if actor.Can(kernel.PermUsageView) {
		today, err := s.Reader.Spent(ctx, p.DayStart)
		if err != nil {
			return Summary{}, err
		}
		month, err := s.Reader.Spent(ctx, p.MonthStart)
		if err != nil {
			return Summary{}, err
		}
		limit, err := s.Reader.MonthlyLimit(ctx, p.MonthStart)
		if err != nil {
			return Summary{}, err
		}
		out.Cost = &CostWidget{Currency: "USD", Today: today, Month: month, MonthlyLimit: limit, MonthUsedPct: domain.UsedPct(month, limit)}
	}
	return out, nil
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
