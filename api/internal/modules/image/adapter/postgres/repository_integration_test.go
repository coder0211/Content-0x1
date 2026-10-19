package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/be/acs/api/internal/modules/image/adapter/postgres"
	"github.com/be/acs/api/internal/modules/image/domain"
	"github.com/be/acs/api/internal/modules/image/usecase"
	"github.com/be/acs/api/internal/shared/kernel"
	"github.com/be/acs/api/internal/testutil/pgtest"
)

func TestBatchRepository(t *testing.T) {
	pool := pgtest.Postgres(t)
	ctx := context.Background()
	user := kernel.NewID()
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, name, password_hash) VALUES ($1, 'img@acs.local', 'Img', 'x')`, user); err != nil {
		t.Fatal(err)
	}
	repo := postgres.NewRepository(pool)
	camp := kernel.NewID()
	base := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	for i := range 3 {
		b := &domain.Batch{ID: kernel.NewID(), JobID: kernel.NewID(), Operation: domain.OpGenerate, CampaignID: &camp,
			BrandID: kernel.NewID(), Options: domain.Options{Prompt: "p", Preset: domain.PresetPoster, AspectRatio: "1:1", Quality: "standard", Count: 4},
			ModelCode: "fake-image", CreatedBy: user, CreatedAt: base.Add(time.Duration(i) * time.Minute)}
		if err := repo.Insert(ctx, b); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			if err := repo.SetResult(ctx, b.ID, kernel.NewID(), 1); err != nil {
				t.Fatal(err)
			}
			got, err := repo.Get(ctx, b.ID)
			if err != nil || got.VersionNo == nil || *got.VersionNo != 1 || got.Options.Count != 4 {
				t.Fatalf("%v %+v", err, got)
			}
		}
	}
	rows, err := repo.List(ctx, usecase.BatchQuery{CampaignID: &camp, Limit: 2})
	if err != nil || len(rows) != 2 || !rows[0].CreatedAt.After(rows[1].CreatedAt) {
		t.Fatalf("%v %+v", err, rows)
	}
	after := kernel.TimeIDCursor{At: rows[1].CreatedAt, ID: rows[1].ID}
	rest, err := repo.List(ctx, usecase.BatchQuery{CreatedBy: &user, Limit: 10, After: &after})
	if err != nil || len(rest) != 1 {
		t.Fatalf("%v %+v", err, rest)
	}
}
