package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/be/acs/api/internal/modules/campaign/adapter/postgres"
	"github.com/be/acs/api/internal/modules/campaign/domain"
	"github.com/be/acs/api/internal/modules/campaign/usecase"
	db "github.com/be/acs/api/internal/platform/postgres"
	"github.com/be/acs/api/internal/shared/kernel"
	"github.com/be/acs/api/internal/testutil/pgtest"
)

func TestCampaignRepository(t *testing.T) {
	pool := pgtest.Postgres(t)
	ctx := context.Background()
	repo, tx := postgres.NewRepository(pool), db.NewTxManager(pool)
	now := time.Now().UTC().Truncate(time.Microsecond)
	var uid, member, brandID kernel.ID
	if err := pool.QueryRow(ctx, `INSERT INTO users (email, name) VALUES ('c@acs.local','C') RETURNING id`).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email, name) VALUES ('m@acs.local','M') RETURNING id`).Scan(&member); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO brands (name) VALUES ('B') RETURNING id`).Scan(&brandID); err != nil {
		t.Fatal(err)
	}
	c := &domain.Campaign{ID: kernel.NewID(), BrandID: brandID, Name: "T11", Goal: "lead", Status: domain.StatusDraft, StartDate: "2026-11-01",
		Brief:     domain.Brief{TargetAudience: "SV", KeyMessage: "x", Channels: []string{"facebook"}, Formats: []domain.Format{{Type: "poster", Count: 2}}, Deadline: "2026-11-15"}.Normalize(),
		CreatedBy: uid, CreatedAt: now, UpdatedAt: now}
	err := tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := repo.Create(ctx, c); err != nil {
			return err
		}
		return repo.SetMembers(ctx, c.ID, []kernel.ID{member})
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetForUpdate(ctx, c.ID)
	if err != nil || got.StartDate != "2026-11-01" || got.EndDate != "" || len(got.MemberIDs) != 1 || got.Brief.Formats[0].Count != 2 || got.ContentPlan != nil {
		t.Fatal(err, got)
	}
	got.Status = domain.StatusInProgress
	got.ContentPlan = &domain.Plan{Themes: []string{}, Items: []domain.PlanItem{{ID: "item-1", Topic: "A", Format: "poster"}}}
	got.ContentPlanApprovedBy, got.ContentPlanApprovedAt = &uid, &now
	if err := repo.Update(ctx, got); err != nil {
		t.Fatal(err)
	}
	got, _ = repo.Get(ctx, c.ID)
	if got.Status != domain.StatusInProgress || got.ContentPlan == nil || got.ContentPlan.Items[0].Topic != "A" || got.PlanProposal != nil {
		t.Fatal(got)
	}
	page, err := repo.List(ctx, usecase.Filter{MemberID: &member, Status: "in_progress", Q: "t1"}, kernel.PageRequest{Limit: 10})
	if err != nil || len(page.Items) != 1 || len(page.Items[0].MemberIDs) != 1 {
		t.Fatal(err, page)
	}
	other := kernel.NewID()
	if page, _ := repo.List(ctx, usecase.Filter{MemberID: &other}, kernel.PageRequest{Limit: 10}); len(page.Items) != 0 {
		t.Fatal(page)
	}
	if _, err := repo.Get(ctx, kernel.NewID()); !errors.Is(err, kernel.ErrNotFound) {
		t.Fatal(err)
	}
}
