package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/be/acs/api/internal/modules/asset/assettest"
	assetuc "github.com/be/acs/api/internal/modules/asset/usecase"
	"github.com/be/acs/api/internal/modules/dashboard/usecase"
	"github.com/be/acs/api/internal/shared/kernel"
	"github.com/be/acs/api/internal/testutil"
)

type reader struct {
	owners     []*kernel.ID
	spentFrom  []time.Time
	monthLimit *decimal.Decimal
}

func (r *reader) ActiveCampaigns(context.Context, int) (int, []usecase.Campaign, error) {
	return 2, []usecase.Campaign{{ID: kernel.NewID(), Name: "T11", Status: "in_progress"}}, nil
}

func (r *reader) JobCounts(_ context.Context, owner *kernel.ID, _ time.Time) (usecase.JobCounts, error) {
	r.owners = append(r.owners, owner)
	return usecase.JobCounts{Queued: 1, Running: 2, Failed24h: 3}, nil
}

func (r *reader) RecentJobs(context.Context, *kernel.ID, time.Time, int) ([]usecase.Job, error) {
	return nil, nil
}

func (r *reader) Spent(_ context.Context, from time.Time) (decimal.Decimal, error) {
	r.spentFrom = append(r.spentFrom, from)
	if from.Day() == 1 {
		return decimal.RequireFromString("250"), nil
	}
	return decimal.RequireFromString("1.5"), nil
}

func (r *reader) MonthlyLimit(context.Context, time.Time) (*decimal.Decimal, error) {
	return r.monthLimit, nil
}

func TestSummaryWidgetsByRole(t *testing.T) {
	lim := decimal.NewFromInt(1000)
	creator, reviewer, lead, admin := testutil.Actor(kernel.RoleContentCreator), testutil.Actor(kernel.RoleReviewer),
		testutil.Actor(kernel.RoleMarketingLead), testutil.Actor(kernel.RoleAdmin)
	assets := assettest.NewAPI()
	for _, st := range []assetuc.Status{assetuc.StatusDraft, assetuc.StatusDraft, assetuc.StatusPendingReview} {
		id := kernel.NewID()
		assets.Assets[id] = &assetuc.AssetView{ID: id, Status: st, CreatedBy: creator.UserID}
	}
	other := kernel.NewID()
	assets.Assets[other] = &assetuc.AssetView{ID: other, Status: assetuc.StatusPendingReview, CreatedBy: lead.UserID}

	for _, tc := range []struct {
		name     string
		actor    kernel.Actor
		scope    string
		pending  *int
		cost     bool
		myDrafts int
	}{
		{"creator", creator, "own", nil, false, 2},
		{"reviewer", reviewer, "own", intp(2), false, 0},
		{"lead", lead, "all", intp(2), true, 0},
		{"admin", admin, "all", intp(2), true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &reader{monthLimit: &lim}
			svc := &usecase.Service{Reader: r, Assets: assets, Clock: testutil.NewClock()}
			s, err := svc.Summary(context.Background(), tc.actor)
			if err != nil {
				t.Fatal(err)
			}
			if s.Jobs.Scope != tc.scope || (tc.scope == "own") != (r.owners[0] != nil) || s.Jobs.Recent == nil {
				t.Fatalf("jobs %+v owners %v", s.Jobs, r.owners)
			}
			if (s.Reviews.PendingReview == nil) != (tc.pending == nil) || (tc.pending != nil && *s.Reviews.PendingReview != *tc.pending) {
				t.Fatalf("pending %v", s.Reviews.PendingReview)
			}
			if s.Reviews.MyDrafts != tc.myDrafts {
				t.Fatalf("my drafts %d", s.Reviews.MyDrafts)
			}
			if (s.Cost != nil) != tc.cost {
				t.Fatalf("cost visible = %v", s.Cost != nil)
			}
			if s.Cost != nil && (s.Cost.MonthUsedPct == nil || *s.Cost.MonthUsedPct != 25 || !s.Cost.Today.Equal(decimal.RequireFromString("1.5"))) {
				t.Fatalf("cost %+v", s.Cost)
			}
			if s.Campaigns == nil || s.Campaigns.ActiveCount != 2 {
				t.Fatalf("campaigns %+v", s.Campaigns)
			}
		})
	}
	if _, err := (&usecase.Service{Clock: testutil.NewClock()}).Summary(context.Background(), kernel.Actor{}); !errors.Is(err, kernel.ErrUnauthenticated) {
		t.Fatalf("anonymous: %v", err)
	}
}

func intp(n int) *int { return &n }
