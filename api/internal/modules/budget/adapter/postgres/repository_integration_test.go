package postgres_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/be/acs/api/internal/modules/budget/adapter/postgres"
	"github.com/be/acs/api/internal/modules/budget/usecase"
	db "github.com/be/acs/api/internal/platform/postgres"
	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
	"github.com/be/acs/api/internal/testutil"
	"github.com/be/acs/api/internal/testutil/pgtest"
)

func newUser(t *testing.T, pool *pgxpool.Pool) kernel.ID {
	t.Helper()
	id := kernel.NewID()
	if _, err := pool.Exec(context.Background(), `INSERT INTO users (id, email, name, status) VALUES ($1, $2, 'U', 'active')`,
		id, id.String()+"@acs.local"); err != nil {
		t.Fatal(err)
	}
	return id
}

func insertJob(ctx context.Context, pool *pgxpool.Pool, id, user kernel.ID) error {
	_, err := db.Conn(ctx, pool).Exec(ctx, `INSERT INTO ai_jobs (id, job_type, request_json, created_by) VALUES ($1, 'video_production', '{}', $2)`, id, user)
	return err
}

// TestConcurrentReserve is ACS-041's AC on a real database: 20 parallel
// job creations with budget for 10 → at most 10 succeed, and the global
// budget's reserved amount equals the sum of held reservations.
func TestConcurrentReserve(t *testing.T) {
	pool := pgtest.Postgres(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE budget_settings SET global_monthly_limit = 10`); err != nil {
		t.Fatal(err)
	}
	user := newUser(t, pool)
	tx := db.NewTxManager(pool)
	svc := &usecase.Service{Tx: tx, Repo: postgres.NewRepository(pool), Clock: testutil.NewClock(), Audit: &testutil.Audit{}, Notifier: &testutil.Notifier{}}

	var ok, refused atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			job := kernel.NewID()
			err := tx.WithinTx(ctx, func(ctx context.Context) error {
				if err := insertJob(ctx, pool, job, user); err != nil {
					return err
				}
				_, err := svc.Reserve(ctx, usecase.ReserveRequest{JobID: job, Amount: kernel.USD("1"), Category: usecase.CategoryVideo})
				return err
			})
			switch {
			case err == nil:
				ok.Add(1)
			case errors.Is(err, kernel.ErrBudgetExceeded):
				refused.Add(1)
			default:
				t.Error(err)
			}
		}()
	}
	close(start)
	wg.Wait()
	if ok.Load() > 10 || ok.Load()+refused.Load() != 20 {
		t.Fatalf("ok=%d refused=%d", ok.Load(), refused.Load())
	}
	var reserved, held decimal.Decimal
	var jobs int
	if err := pool.QueryRow(ctx, `SELECT reserved_amount FROM budgets WHERE scope = 'global'`).Scan(&reserved); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT COALESCE(SUM(amount),0) FROM budget_reservations WHERE status = 'held'`).Scan(&held); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ai_jobs`).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if !reserved.Equal(held) || int32(jobs) != ok.Load() || reserved.GreaterThan(decimal.NewFromInt(10)) {
		t.Fatalf("reserved=%s held=%s jobs=%d ok=%d", reserved, held, jobs, ok.Load())
	}
}

// TestUsageSettleSummary checks pricing lookup, usage → settle → job cost
// and the SQL summary on a real database (ACS-040/042/092).
func TestUsageSettleSummary(t *testing.T) {
	pool := pgtest.Postgres(t)
	ctx := context.Background()
	user := newUser(t, pool)
	tx := db.NewTxManager(pool)
	repo := postgres.NewRepository(pool)
	svc := &usecase.Service{Tx: tx, Repo: repo, Clock: testutil.NewClock(), Audit: &testutil.Audit{}, Notifier: &testutil.Notifier{}}

	est, err := svc.Estimate(ctx, usecase.EstimateRequest{Category: usecase.CategoryText,
		Items: usecase.TextItems("generation", "fake", "fake-text", 300, 1000)})
	if err != nil || !est.Total.Amount.IsPositive() || est.PricingVersion != "fake-2026-10" {
		t.Fatalf("%v %+v", err, est)
	}
	if _, err := svc.Estimate(ctx, usecase.EstimateRequest{Items: []usecase.EstimateItem{{ProviderCode: "fake", ModelCode: "fake-text", Unit: "nope", Units: decimal.NewFromInt(1)}}}); !errors.Is(err, kernel.ErrPricingMissing) {
		t.Fatalf("pricing missing: %v", err)
	}

	job := kernel.NewID()
	err = tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := insertJob(ctx, pool, job, user); err != nil {
			return err
		}
		_, err := svc.Reserve(ctx, usecase.ReserveRequest{JobID: job, Amount: kernel.USD("1"), Category: usecase.CategoryVideo})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	reported := kernel.USD("0.25")
	for _, in := range []usecase.UsageInput{
		{ProviderCode: "fake", ModelCode: "fake-video", Usage: ports.Usage{Units: map[string]float64{"video_second_1080p": 4}}},
		{ProviderCode: "fake", ModelCode: "fake-voice", Usage: ports.Usage{ReportedCost: &reported}},
		{ProviderCode: "fake", ModelCode: "fake-video", Usage: ports.Usage{Units: map[string]float64{"video_second_1080p": 2}}, Estimated: true},
	} {
		in.JobID, in.UserID, in.JobType = job, user, "video_production"
		if _, err := svc.RecordUsage(ctx, in); err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.Settle(ctx, job); err != nil {
		t.Fatal(err)
	}
	if err := svc.Settle(ctx, job); err != nil { // idempotent
		t.Fatal(err)
	}
	var actual decimal.Decimal
	var source string
	var recon bool
	if err := pool.QueryRow(ctx, `SELECT cost_actual, cost_source, needs_reconciliation FROM ai_jobs WHERE id = $1`, job).Scan(&actual, &source, &recon); err != nil {
		t.Fatal(err)
	}
	// 4×0.15 + 0.25 + 2×0.15 = 1.15
	if !actual.Equal(decimal.RequireFromString("1.15")) || source != "estimated" || !recon {
		t.Fatalf("job cost %s %s %v", actual, source, recon)
	}
	var spent, reserved decimal.Decimal
	if err := pool.QueryRow(ctx, `SELECT spent_amount, reserved_amount FROM budgets WHERE scope = 'global'`).Scan(&spent, &reserved); err != nil {
		t.Fatal(err)
	}
	if !spent.Equal(actual) || !reserved.IsZero() {
		t.Fatalf("spent %s reserved %s", spent, reserved)
	}
	for _, g := range usecase.GroupBys {
		cells, err := repo.UsageSummary(ctx, usecase.SummaryFilter{From: testutil.NewClock().Now().AddDate(0, 0, -1), To: testutil.NewClock().Now().AddDate(0, 0, 1), GroupBy: g})
		if err != nil {
			t.Fatal(g, err)
		}
		sum := decimal.Zero
		for _, c := range cells {
			sum = sum.Add(c.Amount)
		}
		if !sum.Equal(actual) {
			t.Fatalf("%s: summary %s != %s", g, sum, actual)
		}
	}
}
