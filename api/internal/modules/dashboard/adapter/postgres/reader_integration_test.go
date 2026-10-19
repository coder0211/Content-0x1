package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/be/acs/api/internal/modules/dashboard/adapter/postgres"
	"github.com/be/acs/api/internal/shared/kernel"
	"github.com/be/acs/api/internal/testutil/pgtest"
)

func TestReaderQueries(t *testing.T) {
	pool := pgtest.Postgres(t)
	ctx := context.Background()
	user := kernel.NewID()
	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	mustExec(`INSERT INTO users (id, email, name) VALUES ($1, 'd@acs.local', 'D')`, user)
	mustExec(`INSERT INTO ai_jobs (job_type, status, request_json, created_by) VALUES ('content_generation', 'running', '{}', $1)`, user)
	mustExec(`INSERT INTO ai_jobs (job_type, status, request_json, created_by, finished_at, error_code)
	          VALUES ('image_generation', 'failed', '{}', $1, now(), 'ContentPolicy')`, user)
	r := postgres.NewReader(pool)
	c, err := r.JobCounts(ctx, &user, time.Now().Add(-24*time.Hour))
	if err != nil || c.Running != 1 || c.Failed24h != 1 {
		t.Fatalf("%v %+v", err, c)
	}
	other := kernel.NewID()
	if c, _ := r.JobCounts(ctx, &other, time.Now().Add(-24*time.Hour)); c.Running != 0 {
		t.Fatalf("scoped %+v", c)
	}
	jobs, err := r.RecentJobs(ctx, nil, time.Now().Add(-24*time.Hour), 5)
	if err != nil || len(jobs) != 2 {
		t.Fatalf("%v %d", err, len(jobs))
	}
	now := time.Now().UTC()
	lim, err := r.MonthlyLimit(ctx, time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC))
	if err != nil || lim == nil || lim.IntPart() != 1000 {
		t.Fatalf("limit %v %v", lim, err)
	}
	if s, err := r.Spent(ctx, now.Add(-time.Hour)); err != nil || !s.IsZero() {
		t.Fatalf("spent %v %v", s, err)
	}
	if n, _, err := r.ActiveCampaigns(ctx, 5); err != nil || n != 0 {
		t.Fatalf("campaigns %d %v", n, err)
	}
}
