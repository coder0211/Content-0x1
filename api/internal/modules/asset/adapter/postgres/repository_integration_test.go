package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/be/acs/api/internal/modules/asset/adapter/postgres"
	"github.com/be/acs/api/internal/modules/asset/assettest"
	"github.com/be/acs/api/internal/modules/asset/domain"
	"github.com/be/acs/api/internal/modules/asset/usecase"
	db "github.com/be/acs/api/internal/platform/postgres"
	"github.com/be/acs/api/internal/shared/kernel"
	"github.com/be/acs/api/internal/testutil"
	"github.com/be/acs/api/internal/testutil/pgtest"
)

type world struct {
	svc     *usecase.Service
	pool    *pgxpool.Pool
	brandID kernel.ID
	creator kernel.Actor
	review  kernel.Actor
}

func setup(t *testing.T) *world {
	t.Helper()
	pool := pgtest.Postgres(t)
	ctx := context.Background()
	w := &world{pool: pool, brandID: kernel.NewID(),
		creator: testutil.Actor(kernel.RoleContentCreator), review: testutil.Actor(kernel.RoleReviewer)}
	for i, a := range []kernel.Actor{w.creator, w.review} {
		if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, name) VALUES ($1, $2, $3)`, a.UserID,
			[]string{"c@acs.local", "r@acs.local"}[i], "User"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO brands (id, name) VALUES ($1, 'Brand')`, w.brandID); err != nil {
		t.Fatal(err)
	}
	w.svc = &usecase.Service{Tx: db.NewTxManager(pool), Repo: postgres.NewRepository(pool), Clock: testutil.NewClock(),
		Audit: &testutil.Audit{}, Notifier: &testutil.Notifier{}, Files: assettest.NewFiles(), Identity: &assettest.Identity{}}
	return w
}

func (w *world) create(t *testing.T, title, body string) kernel.ID {
	t.Helper()
	b, _ := json.Marshal(map[string]any{"title": title, "body_text": body})
	ref, err := w.svc.CreateAsset(context.Background(), usecase.CreateAssetInput{
		BrandID: w.brandID, Type: usecase.TypeContent, Subtype: "facebook_post", Title: title, CreatedBy: w.creator.UserID,
		Tags: []string{"hè"}, Version: usecase.NewVersion{ContentJSON: b},
	})
	if err != nil {
		t.Fatal(err)
	}
	return ref.AssetID
}

func TestAssetRepositoryFlow(t *testing.T) {
	w := setup(t)
	ctx := kernel.WithActor(context.Background(), w.creator)
	id := w.create(t, "Ưu đãi học phí", "Giảm 20% đến 31/12")

	// Accent-insensitive prefix search over title, body and tags.
	for _, q := range []string{"uu dai", "ƯU ĐÃI", "giam", "he", "hoc ph"} {
		p, err := w.svc.Search(ctx, w.creator, usecase.SearchInput{Q: q})
		if err != nil || len(p.Items) != 1 {
			t.Fatalf("search %q: %v %d", q, err, len(p.Items))
		}
	}
	// Versions: conflict, append, restore.
	if _, err := w.svc.CreateVersion(ctx, w.creator, id, usecase.ManualVersionInput{ContentJSON: json.RawMessage(`{"body_text":"x"}`), BaseVersionNo: 5}); !errors.Is(err, kernel.ErrVersionConflict) {
		t.Fatalf("conflict: %v", err)
	}
	for i := 1; i <= 2; i++ {
		if _, err := w.svc.CreateVersion(ctx, w.creator, id, usecase.ManualVersionInput{ContentJSON: json.RawMessage(`{"body_text":"khuyến mãi"}`), BaseVersionNo: i}); err != nil {
			t.Fatal(err)
		}
	}
	v, err := w.svc.Restore(ctx, w.creator, id, 1)
	if err != nil || v.VersionNo != 4 {
		t.Fatalf("restore: %v %+v", err, v)
	}
	// Title edit keeps body lexemes.
	title := "Chương trình mới"
	if _, err := w.svc.Update(ctx, w.creator, id, usecase.UpdateInput{Title: &title}); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"chuong trinh", "giam"} {
		if p, _ := w.svc.Search(ctx, w.creator, usecase.SearchInput{Q: q}); len(p.Items) != 1 {
			t.Fatalf("search after title edit %q: %d", q, len(p.Items))
		}
	}
	// Review flow persists approvals.
	if _, err := w.svc.SubmitReview(ctx, w.creator, id, usecase.SubmitInput{Comment: "Nhờ duyệt"}); err != nil {
		t.Fatal(err)
	}
	q, err := w.svc.Queue(ctx, w.review, usecase.SearchFilter{}, kernel.PageRequest{})
	if err != nil || len(q.Items) != 1 {
		t.Fatalf("queue: %v %d", err, len(q.Items))
	}
	d, err := w.svc.Approve(ctx, w.review, id, "OK")
	if err != nil || d.Asset.Status != domain.StatusApproved || len(d.Approvals) != 2 {
		t.Fatalf("approve: %v %+v", err, d)
	}
	counts, _ := w.svc.CountByStatus(context.Background(), &w.creator.UserID)
	if counts[usecase.StatusApproved] != 1 {
		t.Fatalf("counts %v", counts)
	}
}

// ACS-082 AC: p95 < 500 ms on a 10k-asset dataset (skips without Docker).
func TestSearchPerformance10k(t *testing.T) {
	w := setup(t)
	ctx := context.Background()
	_, err := w.pool.Exec(ctx, `
		INSERT INTO content_assets (brand_id, asset_type, subtype, title, status, tags, created_by, created_at, search_tsv)
		SELECT $1, (ARRAY['content','image','video'])[1 + g % 3], 'facebook_post', t.title,
		       (ARRAY['draft','pending_review','approved','changes_requested'])[1 + g % 4],
		       ARRAY['tag' || (g % 20)], $2, now() - (g || ' minutes')::interval,
		       setweight(to_tsvector('simple', immutable_unaccent(t.title)), 'A')
		FROM generate_series(1, 10000) g
		CROSS JOIN LATERAL (SELECT 'Bài đăng ' || g || CASE WHEN g % 50 = 0 THEN ' ưu đãi hè' ELSE ' khóa học' END AS title) t`, w.brandID, w.creator.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.pool.Exec(ctx, `ANALYZE content_assets`); err != nil {
		t.Fatal(err)
	}
	img, approved := domain.TypeImage, []domain.Status{domain.StatusApproved}
	inputs := []usecase.SearchInput{
		{Q: "uu dai"}, {Q: "khoa hoc 99"}, {Filter: usecase.SearchFilter{Type: &img, Statuses: approved}},
		{Filter: usecase.SearchFilter{Tag: "tag7"}}, {Q: "bai dang", Filter: usecase.SearchFilter{Type: &img}},
	}
	var durs []time.Duration
	for range 8 {
		for _, in := range inputs {
			in.PageCursor = kernel.PageRequest{Limit: 50}
			start := time.Now()
			if _, err := w.svc.Search(ctx, w.creator, in); err != nil {
				t.Fatal(err)
			}
			durs = append(durs, time.Since(start))
		}
	}
	sort.Slice(durs, func(i, j int) bool { return durs[i] < durs[j] })
	p95 := durs[len(durs)*95/100]
	t.Logf("library search p95 = %s over %d queries", p95, len(durs))
	if p95 > 500*time.Millisecond {
		t.Fatalf("p95 %s > 500ms", p95)
	}
	p, err := w.svc.Search(ctx, w.creator, usecase.SearchInput{Q: "uu dai", PageCursor: kernel.PageRequest{Limit: 100}})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Items) != 100 || !slices.ContainsFunc(p.Items, func(s usecase.Summary) bool { return s.Asset.Title != "" }) {
		t.Fatalf("uu dai hits %d", len(p.Items))
	}
}
