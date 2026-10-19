package usecase_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/be/acs/api/internal/modules/budget/budgettest"
	"github.com/be/acs/api/internal/modules/budget/domain"
	"github.com/be/acs/api/internal/modules/budget/usecase"
	identityuc "github.com/be/acs/api/internal/modules/identity/usecase"
	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
	"github.com/be/acs/api/internal/testutil"
)

type directory struct{ users []identityuc.UserSummary }

func (d directory) UsersWithPermission(context.Context, string) ([]identityuc.UserSummary, error) {
	return d.users, nil
}
func (d directory) GetUsers(_ context.Context, ids []kernel.ID) ([]identityuc.UserSummary, error) {
	var out []identityuc.UserSummary
	for _, u := range d.users {
		for _, id := range ids {
			if u.ID == id {
				out = append(out, u)
			}
		}
	}
	return out, nil
}

type mailer struct {
	mu   sync.Mutex
	sent [][]string
	fail bool
}

func (m *mailer) Send(_ context.Context, to []string, _, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail {
		return errors.New("smtp down")
	}
	m.sent = append(m.sent, to)
	return nil
}

type env struct {
	svc    *usecase.Service
	store  *budgettest.Store
	audit  *testutil.Audit
	notify *testutil.Notifier
	clock  *testutil.Clock
	mail   *mailer
	lead   identityuc.UserSummary
}

func setup(t *testing.T) *env {
	t.Helper()
	st := budgettest.NewStore()
	lead := identityuc.UserSummary{ID: kernel.NewID(), Email: "lead@acs.local", Name: "Lead"}
	e := &env{store: st, audit: &testutil.Audit{}, notify: &testutil.Notifier{}, clock: testutil.NewClock(), mail: &mailer{}, lead: lead}
	e.svc = &usecase.Service{Tx: st, Repo: st, Clock: e.clock, Audit: e.audit, Notifier: e.notify,
		Users: directory{users: []identityuc.UserSummary{lead}}, Mailer: e.mail}
	return e
}

func money(s string) kernel.Money { return kernel.USD(s) }

func TestEstimateTextImageVideo(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	// text: 300 chars → 100 + 600 overhead input tokens, 1000 output tokens.
	est, err := e.svc.Estimate(ctx, usecase.EstimateRequest{Category: usecase.CategoryText, Items: usecase.TextItems("", "fake", "fake-text", 300, 1000)})
	if err != nil {
		t.Fatal(err)
	}
	want := decimal.RequireFromString("0.0021").Add(decimal.RequireFromString("0.015")) // 700×0.000003 + 1000×0.000015
	if !est.Total.Amount.Equal(want) || est.PricingVersion != "fake-2026-10" {
		t.Fatalf("text %s %s", est.Total, est.PricingVersion)
	}
	// image (ACS-054): 4 × high = 0.68.
	est, err = e.svc.Estimate(ctx, usecase.EstimateRequest{Category: usecase.CategoryImage, Items: usecase.ImageItems("fake", "fake-image", 4, "1024", "high")})
	if err != nil || !est.Total.Amount.Equal(decimal.RequireFromString("0.68")) {
		t.Fatalf("image %v %s", err, est.Total)
	}
	// video: 20s 1080p (3.00) + 420 chars (0.00672) + render 0.
	est, err = e.svc.Estimate(ctx, usecase.EstimateRequest{Category: usecase.CategoryVideo, Items: usecase.VideoItems(usecase.VideoEstimateInput{
		ClipProvider: "fake", ClipModel: "fake-video", ClipSeconds: decimal.NewFromInt(20), Resolution: "1080p",
		VoiceProvider: "fake", VoiceModel: "fake-voice", VoiceoverChars: 420, RenderSeconds: decimal.NewFromInt(30)})})
	if err != nil || !est.Total.Amount.Equal(decimal.RequireFromString("3.00672")) || len(est.Lines) != 3 || est.Lines[2].Item != "render" || !est.Lines[2].Amount.IsZero() {
		t.Fatalf("video %v %+v", err, est)
	}
}

func TestEstimatePricingMissing(t *testing.T) {
	e := setup(t)
	_, err := e.svc.Estimate(context.Background(), usecase.EstimateRequest{Items: usecase.ImageItems("fake", "fake-image", 1, "2048", "standard")})
	if !errors.Is(err, kernel.ErrPricingMissing) {
		t.Fatalf("want PRICING_MISSING, got %v", err)
	}
	// Pricing not yet effective.
	e.store.Prices = append(e.store.Prices, budgettest.PriceRow{Provider: "x", Model: "m", Unit: "char", Price: decimal.NewFromInt(1), Version: "v", From: e.clock.Now().Add(time.Hour)})
	_, err = e.svc.Estimate(context.Background(), usecase.EstimateRequest{Items: []usecase.EstimateItem{{ProviderCode: "x", ModelCode: "m", Unit: "char", Units: decimal.NewFromInt(1)}}})
	if !errors.Is(err, kernel.ErrPricingMissing) {
		t.Fatalf("future pricing must not apply: %v", err)
	}
}

func TestReservePerJobLimitAndOverride(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	creator := testutil.Actor(kernel.RoleContentCreator)
	_, err := e.svc.Reserve(ctx, usecase.ReserveRequest{JobID: kernel.NewID(), Amount: money("0.60"), Category: usecase.CategoryText, Override: &creator})
	de, _ := kernel.AsDomainError(err)
	if !errors.Is(err, kernel.ErrJobBudgetExceeded) || len(de.Details) != 1 || de.Details[0]["per_job_limit"] != 0.5 {
		t.Fatalf("creator must get JOB_BUDGET_EXCEEDED with details: %v", err)
	}
	lead := testutil.Actor(kernel.RoleMarketingLead)
	job := kernel.NewID()
	if _, err := e.svc.Reserve(ctx, usecase.ReserveRequest{JobID: job, Amount: money("0.60"), Category: usecase.CategoryText, Override: &lead}); err != nil {
		t.Fatal(err)
	}
	if acts := e.audit.Actions(); len(acts) != 1 || acts[0] != "budget.override" || e.audit.Entries[0].ActorID != lead.UserID {
		t.Fatalf("override must be audited as the lead: %v", acts)
	}
	// Idempotent: reserving the same job again holds nothing more.
	if _, err := e.svc.Reserve(ctx, usecase.ReserveRequest{JobID: job, Amount: money("0.60"), Category: usecase.CategoryText, Override: &lead}); err != nil {
		t.Fatal(err)
	}
	g := globalBudget(t, e)
	if !g.Reserved.Equal(decimal.RequireFromString("0.6")) {
		t.Fatalf("reserved %s", g.Reserved)
	}
}

func globalBudget(t *testing.T, e *env) domain.Budget {
	t.Helper()
	start, _ := domain.MonthPeriod(e.clock.Now())
	var b domain.Budget
	_ = e.store.WithinTx(context.Background(), func(ctx context.Context) error {
		var err error
		b, err = e.store.GetGlobalBudget(ctx, start, false)
		if err != nil {
			t.Fatal(err)
		}
		return nil
	})
	return b
}

// ACS-041 AC: 20 parallel requests with budget for 10 → at most (exactly) 10 succeed.
func TestReserveConcurrentRespectsGlobalLimit(t *testing.T) {
	e := setup(t)
	e.store.Settings.GlobalMonthlyLimit = decimal.NewFromInt(10)
	var ok, refused atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			err := e.store.WithinTx(context.Background(), func(ctx context.Context) error {
				_, err := e.svc.Reserve(ctx, usecase.ReserveRequest{JobID: kernel.NewID(), Amount: money("1"), Category: usecase.CategoryVideo})
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
	if ok.Load() != 10 || refused.Load() != 10 {
		t.Fatalf("ok=%d refused=%d", ok.Load(), refused.Load())
	}
	if g := globalBudget(t, e); !g.Reserved.Equal(decimal.NewFromInt(10)) {
		t.Fatalf("reserved %s", g.Reserved)
	}
}

func TestHardStopAndAdminOverride(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	e.store.Settings.GlobalMonthlyLimit = decimal.NewFromInt(5)
	if _, err := e.svc.Reserve(ctx, usecase.ReserveRequest{JobID: kernel.NewID(), Amount: money("5"), Category: usecase.CategoryVideo}); err != nil {
		t.Fatal(err)
	}
	lead := testutil.Actor(kernel.RoleMarketingLead)
	_, err := e.svc.Reserve(ctx, usecase.ReserveRequest{JobID: kernel.NewID(), Amount: money("0"), Category: usecase.CategoryText, Override: &lead})
	if !errors.Is(err, kernel.ErrBudgetExceeded) {
		t.Fatalf("100%% + hard stop must block even a 0 job for a lead: %v", err)
	}
	admin := testutil.Actor(kernel.RoleAdmin)
	if _, err := e.svc.Reserve(ctx, usecase.ReserveRequest{JobID: kernel.NewID(), Amount: money("1"), Category: usecase.CategoryVideo, Override: &admin}); err != nil {
		t.Fatalf("admin override: %v", err)
	}
	// Without hard stop: allowed.
	off := false
	if _, err := e.svc.UpdateSettings(ctx, admin, usecase.SettingsPatch{GlobalHardStop: &off}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Reserve(ctx, usecase.ReserveRequest{JobID: kernel.NewID(), Amount: money("1"), Category: usecase.CategoryVideo}); err != nil {
		t.Fatalf("no hard stop: %v", err)
	}
}

func TestCampaignBudgetBlocks(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	lead := testutil.Actor(kernel.RoleMarketingLead)
	camp := kernel.NewID()
	if _, err := e.svc.PutCampaignBudget(ctx, lead, camp, usecase.CampaignBudgetInput{Limit: decimal.NewFromInt(2)}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Reserve(ctx, usecase.ReserveRequest{JobID: kernel.NewID(), CampaignID: &camp, Amount: money("1.5"), Category: usecase.CategoryImage}); err != nil {
		t.Fatal(err)
	}
	_, err := e.svc.Reserve(ctx, usecase.ReserveRequest{JobID: kernel.NewID(), CampaignID: &camp, Amount: money("1"), Category: usecase.CategoryImage})
	de, _ := kernel.AsDomainError(err)
	if !errors.Is(err, kernel.ErrBudgetExceeded) || de.Details[0]["scope"] != "campaign" {
		t.Fatalf("campaign must block: %v", err)
	}
	lim, err := e.svc.Limits(ctx, &camp, usecase.CategoryImage, money("1"))
	if err != nil || !lim.Blocked || lim.CampaignRemaining == nil || !lim.CampaignRemaining.Amount.Equal(decimal.RequireFromString("0.5")) {
		t.Fatalf("limits %+v %v", lim, err)
	}
}

// ACS-042 AC: job cost_actual = sum of usage records; settle moves reserved → spent.
func TestUsageSettleRelease(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	user := kernel.NewID()
	job := kernel.NewID()
	if _, err := e.svc.Reserve(ctx, usecase.ReserveRequest{JobID: job, Amount: money("0.40"), Category: usecase.CategoryImage}); err != nil {
		t.Fatal(err)
	}
	r1, err := e.svc.RecordUsage(ctx, usecase.UsageInput{JobID: job, UserID: user, ProviderCode: "fake", ModelCode: "fake-image", JobType: "image_generation",
		Usage: ports.Usage{Images: 2, Units: map[string]float64{"image_1024_standard": 2}}})
	if err != nil || r1.Source != usecase.SourceCalculated || !r1.Cost.Amount.Equal(decimal.RequireFromString("0.08")) {
		t.Fatalf("%v %+v", err, r1)
	}
	reported := money("0.05")
	r2, err := e.svc.RecordUsage(ctx, usecase.UsageInput{JobID: job, UserID: user, ProviderCode: "fake", ModelCode: "fake-image",
		Usage: ports.Usage{ReportedCost: &reported, Units: map[string]float64{}}})
	if err != nil || r2.Source != usecase.SourceProviderReported {
		t.Fatalf("%v %+v", err, r2)
	}
	if err := e.svc.Settle(ctx, job); err != nil {
		t.Fatal(err)
	}
	g := globalBudget(t, e)
	if !g.Reserved.IsZero() || !g.Spent.Equal(decimal.RequireFromString("0.13")) {
		t.Fatalf("reserved %s spent %s", g.Reserved, g.Spent)
	}
	jc := e.store.Jobs[job]
	if !jc.Actual.Equal(decimal.RequireFromString("0.13")) || jc.Source != usecase.SourceCalculated || jc.NeedsReconciliation {
		t.Fatalf("job cost %+v", jc)
	}
	// Settle is idempotent.
	if err := e.svc.Settle(ctx, job); err != nil {
		t.Fatal(err)
	}
	if g := globalBudget(t, e); !g.Spent.Equal(decimal.RequireFromString("0.13")) {
		t.Fatalf("double settle spent %s", g.Spent)
	}
	// Retry: new reservation, timeout usage (estimated) → needs_reconciliation.
	if _, err := e.svc.Reserve(ctx, usecase.ReserveRequest{JobID: job, Amount: money("0.40"), Category: usecase.CategoryImage}); err != nil {
		t.Fatal(err)
	}
	r3, err := e.svc.RecordUsage(ctx, usecase.UsageInput{JobID: job, UserID: user, ProviderCode: "fake", ModelCode: "fake-image", Estimated: true,
		Usage: ports.Usage{Units: map[string]float64{"image_1024_standard": 1}}})
	if err != nil || r3.Source != usecase.SourceEstimated {
		t.Fatal(err, r3)
	}
	if err := e.svc.Settle(ctx, job); err != nil {
		t.Fatal(err)
	}
	jc = e.store.Jobs[job]
	if !jc.Actual.Equal(decimal.RequireFromString("0.17")) || jc.Source != usecase.SourceEstimated || !jc.NeedsReconciliation {
		t.Fatalf("job cost after retry %+v", jc)
	}
	if g := globalBudget(t, e); !g.Spent.Equal(decimal.RequireFromString("0.17")) || !g.Reserved.IsZero() {
		t.Fatalf("spent %s reserved %s", g.Spent, g.Reserved)
	}
	// Release without usage frees the hold.
	j2 := kernel.NewID()
	if _, err := e.svc.Reserve(ctx, usecase.ReserveRequest{JobID: j2, Amount: money("1"), Category: usecase.CategoryImage}); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Release(ctx, j2); err != nil {
		t.Fatal(err)
	}
	if g := globalBudget(t, e); !g.Reserved.IsZero() {
		t.Fatalf("release reserved %s", g.Reserved)
	}
	for _, r := range e.store.Reservations() {
		if r.JobID == j2 && r.Status != "released" {
			t.Fatalf("status %s", r.Status)
		}
	}
}

func TestRecordUsageWithoutPricingKeepsUsage(t *testing.T) {
	e := setup(t)
	job := kernel.NewID()
	r, err := e.svc.RecordUsage(context.Background(), usecase.UsageInput{JobID: job, UserID: kernel.NewID(), ProviderCode: "fake", ModelCode: "unknown",
		Usage: ports.Usage{Units: map[string]float64{"char": 10}}})
	if err != nil || r.Source != usecase.SourceEstimated || !e.store.Jobs[job].NeedsReconciliation {
		t.Fatalf("%v %+v", err, r)
	}
}

// ACS-044 AC: each threshold fires once per period; notification + e-mail to budget managers.
func TestAlertsOncePerPeriod(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	e.store.Settings.GlobalMonthlyLimit = decimal.NewFromInt(10)
	reserve := func(amount string) {
		t.Helper()
		if _, err := e.svc.Reserve(ctx, usecase.ReserveRequest{JobID: kernel.NewID(), Amount: money(amount), Category: usecase.CategoryVideo}); err != nil {
			t.Fatal(err)
		}
	}
	reserve("7")
	if len(e.notify.Sent) != 0 {
		t.Fatal("no alert below 80%")
	}
	reserve("1") // 80%
	reserve("1") // 90%: no new alert
	if len(e.notify.Sent) != 1 || e.notify.Sent[0].Type != ports.NotifyBudgetAlert || e.notify.Sent[0].UserIDs[0] != e.lead.ID {
		t.Fatalf("80%% alert: %+v", e.notify.Sent)
	}
	reserve("1") // 100%
	if _, err := e.svc.Reserve(ctx, usecase.ReserveRequest{JobID: kernel.NewID(), Amount: money("0"), Category: usecase.CategoryText}); !errors.Is(err, kernel.ErrBudgetExceeded) {
		t.Fatalf("100%% + hard stop blocks: %v", err)
	}
	if len(e.notify.Sent) != 2 {
		t.Fatalf("100%% alert once: %d", len(e.notify.Sent))
	}
	if n, err := e.svc.DeliverAlertEmails(ctx); err != nil || n != 2 || len(e.mail.sent) != 2 || e.mail.sent[0][0] != "lead@acs.local" {
		t.Fatalf("emails %d %v %v", n, err, e.mail.sent)
	}
	if n, _ := e.svc.DeliverAlertEmails(ctx); n != 0 {
		t.Fatal("emails sent twice")
	}
	// Next month: new period, alerts may fire again.
	e.clock.Advance(31 * 24 * time.Hour)
	reserve("8")
	if len(e.notify.Sent) != 3 {
		t.Fatalf("new period alert: %d", len(e.notify.Sent))
	}
}

func TestSettingsRBACAndAudit(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	v := decimal.RequireFromString("0.75")
	if _, err := e.svc.UpdateSettings(ctx, testutil.Actor(kernel.RoleContentCreator), usecase.SettingsPatch{PerJobText: &v}); !errors.Is(err, kernel.ErrForbidden) {
		t.Fatalf("creator: %v", err)
	}
	if _, err := e.svc.GetSettings(ctx, testutil.Actor(kernel.RoleReviewer)); !errors.Is(err, kernel.ErrForbidden) {
		t.Fatalf("reviewer: %v", err)
	}
	if _, err := e.svc.UpdateSettings(ctx, testutil.Actor(kernel.RoleMarketingLead), usecase.SettingsPatch{GlobalThresholds: []int{0}}); !errors.Is(err, kernel.ErrValidation) {
		t.Fatalf("thresholds: %v", err)
	}
	lim := decimal.NewFromInt(500)
	view, err := e.svc.UpdateSettings(ctx, testutil.Actor(kernel.RoleMarketingLead), usecase.SettingsPatch{PerJobText: &v, GlobalMonthlyLimit: &lim, GlobalThresholds: []int{100, 50}})
	if err != nil || !view.Settings.PerJobText.Equal(v) || !view.Global.Limit.Equal(lim) || view.Global.Thresholds[0] != 50 {
		t.Fatalf("%v %+v", err, view)
	}
	if a := e.audit.Actions(); len(a) != 1 || a[0] != "budget.update" {
		t.Fatalf("audit %v", a)
	}
}

func TestUsageSummaryTotalsMatch(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	camp := kernel.NewID()
	rec := func(units float64, est bool, c *kernel.ID) {
		if _, err := e.svc.RecordUsage(ctx, usecase.UsageInput{JobID: kernel.NewID(), UserID: e.lead.ID, CampaignID: c, ProviderCode: "fake", ModelCode: "fake-voice", JobType: "video_production",
			Estimated: est, Usage: ports.Usage{Units: map[string]float64{"char": units}}}); err != nil {
			t.Fatal(err)
		}
	}
	rec(1000, false, &camp)
	e.clock.Advance(24 * time.Hour)
	rec(500, true, nil)
	reported := money("0.5")
	if _, err := e.svc.RecordUsage(ctx, usecase.UsageInput{JobID: kernel.NewID(), UserID: e.lead.ID, ProviderCode: "fake", ModelCode: "fake-text", Usage: ports.Usage{ReportedCost: &reported}}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.UsageSummary(ctx, testutil.Actor(kernel.RoleContentCreator), usecase.SummaryRequest{}); !errors.Is(err, kernel.ErrForbidden) {
		t.Fatal("creator must not see usage")
	}
	if _, err := e.svc.UsageSummary(ctx, testutil.Actor(kernel.RoleAdmin), usecase.SummaryRequest{GroupBy: "bogus"}); !errors.Is(err, kernel.ErrValidation) {
		t.Fatal("bad group_by")
	}
	for _, g := range usecase.GroupBys {
		s, err := e.svc.UsageSummary(ctx, testutil.Actor(kernel.RoleMarketingLead), usecase.SummaryRequest{GroupBy: g})
		if err != nil {
			t.Fatal(err)
		}
		sum := decimal.Zero
		for _, r := range s.Rows {
			sum = sum.Add(r.Total)
			if !r.ProviderReported.Add(r.Calculated).Add(r.Estimated).Equal(r.Total) {
				t.Fatalf("%s: row split mismatch %+v", g, r)
			}
		}
		want := decimal.RequireFromString("0.524") // 0.016 + 0.008 + 0.5
		if !sum.Equal(want) || !s.Totals.Total.Equal(want) || s.Totals.Count != 3 {
			t.Fatalf("%s: rows %s totals %s", g, sum, s.Totals.Total)
		}
		if g == "day" && len(s.Rows) != 2 {
			t.Fatalf("days %+v", s.Rows)
		}
		if g == "user" && s.Rows[0].Label != "Lead" {
			t.Fatalf("user label %+v", s.Rows)
		}
		if g == "day" && !s.Totals.Estimated.Equal(decimal.RequireFromString("0.008")) {
			t.Fatalf("estimated %s", s.Totals.Estimated)
		}
	}
}

func TestQuote(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	q, err := e.svc.Quote(ctx, testutil.Actor(kernel.RoleContentCreator), usecase.QuoteRequest{Kind: usecase.CategoryVideo, ClipSeconds: decimal.NewFromInt(60), Resolution: "1080p", VoiceoverChars: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if !q.Estimate.Total.Amount.Equal(decimal.RequireFromString("9.016")) || !q.Limits.RequiresOverride {
		t.Fatalf("%+v", q)
	}
	if _, err := e.svc.Quote(ctx, testutil.Actor(kernel.RoleReviewer), usecase.QuoteRequest{Kind: usecase.CategoryText}); !errors.Is(err, kernel.ErrForbidden) {
		t.Fatal("reviewer cannot generate")
	}
	delete(e.store.Defaults, "image")
	if _, err := e.svc.Quote(ctx, testutil.Actor(kernel.RoleAdmin), usecase.QuoteRequest{Kind: usecase.CategoryImage, Count: 1}); !errors.Is(err, kernel.ErrPricingMissing) {
		t.Fatalf("no default model: %v", err)
	}
}
