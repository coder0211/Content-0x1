package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/be/acs/api/internal/modules/video/adapter/postgres"
	"github.com/be/acs/api/internal/modules/video/domain"
	"github.com/be/acs/api/internal/modules/video/usecase"
	db "github.com/be/acs/api/internal/platform/postgres"
	"github.com/be/acs/api/internal/shared/kernel"
	"github.com/be/acs/api/internal/testutil/pgtest"
)

// Integration test: skips without Docker.
func TestRepository(t *testing.T) {
	pool := pgtest.Postgres(t)
	ctx := context.Background()
	user, brand, asset := kernel.NewID(), kernel.NewID(), kernel.NewID()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO users (id, email, name, password_hash) VALUES ($1, 'video@acs.local', 'Video', 'x')`, []any{user}},
		{`INSERT INTO brands (id, name, created_by) VALUES ($1, 'Brand', $2)`, []any{brand, user}},
		{`INSERT INTO content_assets (id, brand_id, asset_type, title, created_by) VALUES ($1, $2, 'video', 'Video', $3)`, []any{asset, brand, user}},
	} {
		if _, err := pool.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatalf("seed %q: %v", q.sql, err)
		}
	}
	repo := postgres.NewRepository(pool)
	tx := db.NewTxManager(pool)
	now := time.Now().UTC().Truncate(time.Microsecond)
	p := &domain.Project{AssetID: asset, BrandID: brand, Title: "Video", Prompt: "p", CreatedBy: user, CreatedAt: now, UpdatedAt: now,
		Settings: domain.Settings{Preset: "square", AspectRatio: "1:1", Width: 1080, Height: 1080, FPS: 30, TargetDurationSeconds: 10}}
	if err := repo.CreateProject(ctx, p); err != nil {
		t.Fatal(err)
	}
	p.Script = "kịch bản"
	p.Voice = &domain.VoiceTrack{Fingerprint: "abc", Segments: []domain.VoiceSegment{}}
	if err := repo.UpdateProject(ctx, p); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetProject(ctx, asset)
	if err != nil || got.Script != "kịch bản" || got.Voice == nil || got.Voice.Fingerprint != "abc" || got.Output != nil || got.Settings.Preset != "square" {
		t.Fatalf("get %+v %v", got, err)
	}
	if _, err := repo.GetProject(ctx, kernel.NewID()); !errors.Is(err, kernel.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
	list, err := repo.ListProjects(ctx, usecase.ProjectFilter{CreatedBy: &user, Limit: 10})
	if err != nil || len(list) != 1 {
		t.Fatalf("list %d %v", len(list), err)
	}

	mk := func(prompt string) domain.Scene {
		return domain.Scene{ID: kernel.NewID(), AssetID: asset, DurationMs: 3000, VisualPrompt: prompt, Status: domain.SceneDraft, CreatedAt: now, UpdatedAt: now}
	}
	a, b, c := mk("a"), mk("b"), mk("c")
	save := func(sc ...domain.Scene) {
		t.Helper()
		if err := tx.WithinTx(ctx, func(ctx context.Context) error { return repo.SaveStoryboard(ctx, asset, sc) }); err != nil {
			t.Fatal(err)
		}
	}
	save(a, b, c)
	// Reorder (c, a) and drop b: unique live sequence numbers must hold.
	save(c, a)
	scenes, err := repo.ListScenes(ctx, asset)
	if err != nil || len(scenes) != 2 || scenes[0].ID != c.ID || scenes[0].SequenceNo != 1 || scenes[1].ID != a.ID {
		t.Fatalf("scenes %+v %v", scenes, err)
	}
	if _, err := repo.GetScene(ctx, b.ID); !errors.Is(err, kernel.ErrNotFound) {
		t.Fatalf("deleted scene visible: %v", err)
	}
	s, _ := repo.GetScene(ctx, a.ID)
	s.Status, s.ProviderJobID, s.AttemptCount = domain.SceneGenerating, "op-1", 1
	if err := repo.UpdateScene(ctx, s); err != nil {
		t.Fatal(err)
	}
	s, _ = repo.GetScene(ctx, a.ID)
	if s.Status != domain.SceneGenerating || s.ProviderJobID != "op-1" || s.AttemptCount != 1 {
		t.Fatalf("scene %+v", s)
	}
	if err := tx.WithinTx(ctx, func(ctx context.Context) error {
		_, err := repo.LockProject(ctx, asset)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
