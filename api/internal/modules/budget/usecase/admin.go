package usecase

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/shopspring/decimal"

	"github.com/be/acs/api/internal/modules/budget/domain"
	"github.com/be/acs/api/internal/shared/kernel"
)

// SettingsView is GET /settings/budgets: the per-job/global template plus
// the current month's global budget.
type SettingsView struct {
	Settings domain.Settings
	Global   domain.Budget
	// Campaigns is filled by GetSettings only (nil after UpdateSettings).
	Campaigns []CampaignBudgetView
}

// CampaignBudgetView is a campaign budget with the campaign's name.
type CampaignBudgetView struct {
	Budget       domain.Budget
	CampaignName string
}

// GetSettings returns budget settings (usage.view).
func (s *Service) GetSettings(ctx context.Context, actor kernel.Actor) (SettingsView, error) {
	if err := actor.Require(kernel.PermUsageView); err != nil {
		return SettingsView{}, err
	}
	var out SettingsView
	err := s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		st, err := s.Repo.GetSettings(ctx)
		if err != nil {
			return err
		}
		g, err := s.currentGlobal(ctx, st, false)
		if err != nil {
			return err
		}
		list, err := s.Repo.ListCampaignBudgets(ctx)
		if err != nil {
			return err
		}
		ids := make([]kernel.ID, 0, len(list))
		for _, b := range list {
			ids = append(ids, *b.CampaignID)
		}
		names, err := s.Repo.CampaignNames(ctx, ids)
		if err != nil {
			return err
		}
		views := make([]CampaignBudgetView, 0, len(list))
		for _, b := range list {
			views = append(views, CampaignBudgetView{Budget: b, CampaignName: names[*b.CampaignID]})
		}
		out = SettingsView{Settings: st, Global: g, Campaigns: views}
		return nil
	})
	return out, err
}

// SettingsPatch is PATCH /settings/budgets; nil fields are unchanged.
// Global changes apply to the template and to the current month's budget.
type SettingsPatch struct {
	PerJobText             *decimal.Decimal
	PerJobImage            *decimal.Decimal
	PerJobVideo            *decimal.Decimal
	GlobalMonthlyLimit     *decimal.Decimal
	GlobalThresholds       []int
	GlobalHardStop         *bool
	MaxConcurrentVideoJobs *int
}

func checkAmount(v *kernel.ValidationErrors, field string, d *decimal.Decimal) {
	if d != nil && (d.IsNegative() || d.GreaterThan(decimal.NewFromInt(100000000))) {
		v.Add(field, "out_of_range")
	}
}

func settingsAudit(st domain.Settings, g domain.Budget) map[string]any {
	return map[string]any{
		"per_job_limit_text": st.PerJobText.String(), "per_job_limit_image": st.PerJobImage.String(), "per_job_limit_video": st.PerJobVideo.String(),
		"global_monthly_limit": st.GlobalMonthlyLimit.String(), "global_alert_thresholds": st.GlobalThresholds, "global_hard_stop": st.GlobalHardStop,
		"max_concurrent_video_jobs_per_user": st.MaxConcurrentVideoJobs, "current_global_limit": g.Limit.String(),
	}
}

// UpdateSettings applies p (budget.manage, audited as budget.update).
func (s *Service) UpdateSettings(ctx context.Context, actor kernel.Actor, p SettingsPatch) (SettingsView, error) {
	if err := actor.Require(kernel.PermBudgetManage); err != nil {
		return SettingsView{}, err
	}
	var v kernel.ValidationErrors
	checkAmount(&v, "per_job_limits.text", p.PerJobText)
	checkAmount(&v, "per_job_limits.image", p.PerJobImage)
	checkAmount(&v, "per_job_limits.video", p.PerJobVideo)
	checkAmount(&v, "global.limit_amount", p.GlobalMonthlyLimit)
	if p.GlobalThresholds != nil && !domain.ValidThresholds(p.GlobalThresholds) {
		v.Add("global.alert_thresholds", "invalid")
	}
	if p.MaxConcurrentVideoJobs != nil && (*p.MaxConcurrentVideoJobs < 1 || *p.MaxConcurrentVideoJobs > 50) {
		v.Add("max_concurrent_video_jobs_per_user", "out_of_range")
	}
	if err := v.Err(); err != nil {
		return SettingsView{}, err
	}
	var out SettingsView
	err := s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		st, err := s.Repo.GetSettings(ctx)
		if err != nil {
			return err
		}
		g, err := s.currentGlobal(ctx, st, true)
		if err != nil {
			return err
		}
		before := settingsAudit(st, g)
		if p.PerJobText != nil {
			st.PerJobText = *p.PerJobText
		}
		if p.PerJobImage != nil {
			st.PerJobImage = *p.PerJobImage
		}
		if p.PerJobVideo != nil {
			st.PerJobVideo = *p.PerJobVideo
		}
		if p.GlobalMonthlyLimit != nil {
			st.GlobalMonthlyLimit, g.Limit = *p.GlobalMonthlyLimit, *p.GlobalMonthlyLimit
		}
		if p.GlobalThresholds != nil {
			st.GlobalThresholds = domain.NormalizeThresholds(p.GlobalThresholds)
			g.Thresholds = st.GlobalThresholds
		}
		if p.GlobalHardStop != nil {
			st.GlobalHardStop, g.HardStop = *p.GlobalHardStop, *p.GlobalHardStop
		}
		if p.MaxConcurrentVideoJobs != nil {
			st.MaxConcurrentVideoJobs = *p.MaxConcurrentVideoJobs
		}
		now := s.Clock.Now()
		uid := actor.UserID
		st.UpdatedBy, st.UpdatedAt = &uid, now
		if err := s.Repo.SaveSettings(ctx, st); err != nil {
			return err
		}
		g.ResetAlertsBelow()
		if err := s.fireAlerts(ctx, &g); err != nil {
			return err
		}
		g.UpdatedAt = now
		if err := s.Repo.SaveBudget(ctx, g); err != nil {
			return err
		}
		if s.Audit != nil {
			if err := s.Audit.Record(ctx, "budget.update", "budget_settings", "global", before, settingsAudit(st, g)); err != nil {
				return err
			}
		}
		out = SettingsView{Settings: st, Global: g}
		return nil
	})
	return out, err
}

// ListCampaignBudgets lists every campaign budget (usage.view).
func (s *Service) ListCampaignBudgets(ctx context.Context, actor kernel.Actor) ([]domain.Budget, error) {
	if err := actor.Require(kernel.PermUsageView); err != nil {
		return nil, err
	}
	return s.Repo.ListCampaignBudgets(ctx)
}

// GetCampaignBudget returns the campaign's budget, or nil when it has none.
func (s *Service) GetCampaignBudget(ctx context.Context, actor kernel.Actor, campaignID kernel.ID) (*domain.Budget, error) {
	if err := actor.Require(kernel.PermUsageView); err != nil {
		return nil, err
	}
	return s.campaignBudget(ctx, &campaignID, false)
}

// CampaignBudgetInput is PUT /campaigns/{id}/budget.
type CampaignBudgetInput struct {
	Limit       decimal.Decimal
	Currency    string
	PeriodStart *time.Time
	PeriodEnd   *time.Time
	Thresholds  []int
	HardStop    *bool
}

// PutCampaignBudget creates or updates a campaign budget (budget.manage,
// audited). Default period: today .. +1 year; thresholds {80,100}; hard stop on.
func (s *Service) PutCampaignBudget(ctx context.Context, actor kernel.Actor, campaignID kernel.ID, in CampaignBudgetInput) (domain.Budget, error) {
	if err := actor.Require(kernel.PermBudgetManage); err != nil {
		return domain.Budget{}, err
	}
	var v kernel.ValidationErrors
	checkAmount(&v, "limit_amount", &in.Limit)
	if in.Currency != "" && in.Currency != kernel.CurrencyUSD {
		v.Add("currency", "unsupported")
	}
	if in.Thresholds != nil && !domain.ValidThresholds(in.Thresholds) {
		v.Add("alert_thresholds", "invalid")
	}
	if in.PeriodStart != nil && in.PeriodEnd != nil && in.PeriodEnd.Before(*in.PeriodStart) {
		v.Add("period_end", "before_start")
	}
	if err := v.Err(); err != nil {
		return domain.Budget{}, err
	}
	var out domain.Budget
	err := s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		now := s.Clock.Now()
		existing, err := s.campaignBudget(ctx, &campaignID, true)
		if err != nil {
			return err
		}
		var b domain.Budget
		var before any
		if existing != nil {
			b, before = *existing, campaignAudit(*existing)
		} else {
			today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
			cid := campaignID
			b = domain.Budget{ID: kernel.NewID(), Scope: domain.ScopeCampaign, CampaignID: &cid, PeriodStart: today,
				PeriodEnd: today.AddDate(1, 0, 0), Reserved: decimal.Zero, Spent: decimal.Zero, Currency: kernel.CurrencyUSD,
				Thresholds: []int{80, 100}, AlertsSent: []int{}, HardStop: true}
		}
		b.Limit = in.Limit
		if in.PeriodStart != nil {
			b.PeriodStart = in.PeriodStart.UTC()
		}
		if in.PeriodEnd != nil {
			b.PeriodEnd = in.PeriodEnd.UTC()
		}
		if b.PeriodEnd.Before(b.PeriodStart) {
			return kernel.Validation(kernel.FieldIssue("period_end", "before_start"))
		}
		if in.Thresholds != nil {
			b.Thresholds = domain.NormalizeThresholds(in.Thresholds)
		}
		if in.HardStop != nil {
			b.HardStop = *in.HardStop
		}
		b.ResetAlertsBelow()
		if err := s.fireAlerts(ctx, &b); err != nil {
			return err
		}
		b.UpdatedAt = now
		if existing == nil {
			err = s.Repo.InsertBudget(ctx, b)
		} else {
			err = s.Repo.SaveBudget(ctx, b)
		}
		if err != nil {
			return err
		}
		if s.Audit != nil {
			if err := s.Audit.Record(ctx, "budget.update", "campaign_budget", campaignID.String(), before, campaignAudit(b)); err != nil {
				return err
			}
		}
		out = b
		return nil
	})
	return out, err
}

func campaignAudit(b domain.Budget) map[string]any {
	return map[string]any{"limit_amount": b.Limit.String(), "alert_thresholds": b.Thresholds, "hard_stop": b.HardStop,
		"period_start": b.PeriodStart.Format(time.DateOnly), "period_end": b.PeriodEnd.Format(time.DateOnly)}
}

// --------------------------------------------------------------- summary

// Group-by dimensions of GET /usage/summary.
var GroupBys = []string{"day", "campaign", "provider", "model", "user", "job_type"}

// SummaryRequest is GET /usage/summary. From/To are inclusive dates; zero
// values default to the current month up to today.
type SummaryRequest struct {
	From, To *time.Time
	GroupBy  string
}

// SourceTotals splits an amount by cost_source.
type SourceTotals struct {
	ProviderReported decimal.Decimal
	Calculated       decimal.Decimal
	Estimated        decimal.Decimal
	Total            decimal.Decimal
	Count            int64
}

func (t *SourceTotals) add(c SummaryCell) {
	switch c.Source {
	case SourceProviderReported:
		t.ProviderReported = t.ProviderReported.Add(c.Amount)
	case SourceCalculated:
		t.Calculated = t.Calculated.Add(c.Amount)
	case SourceEstimated:
		t.Estimated = t.Estimated.Add(c.Amount)
	}
	t.Total = t.Total.Add(c.Amount)
	t.Count += c.Count
}

// SummaryRow is one group.
type SummaryRow struct {
	Key   string // "" = none (e.g. usage without campaign)
	Label string
	SourceTotals
}

// Summary is the usage report. Totals is the exact sum of Rows.
type Summary struct {
	From, To time.Time // inclusive dates
	GroupBy  string
	Currency string
	Rows     []SummaryRow
	Totals   SourceTotals
}

func zeroTotals() SourceTotals {
	return SourceTotals{ProviderReported: decimal.Zero, Calculated: decimal.Zero, Estimated: decimal.Zero, Total: decimal.Zero}
}

// UsageSummary aggregates ai_usage_records (usage.view, ACS-092).
func (s *Service) UsageSummary(ctx context.Context, actor kernel.Actor, req SummaryRequest) (Summary, error) {
	if err := actor.Require(kernel.PermUsageView); err != nil {
		return Summary{}, err
	}
	if req.GroupBy == "" {
		req.GroupBy = "day"
	}
	var v kernel.ValidationErrors
	valid := false
	for _, g := range GroupBys {
		valid = valid || g == req.GroupBy
	}
	if !valid {
		v.AddDetail(kernel.Detail{"field": "group_by", "issue": "invalid", "allowed": GroupBys})
	}
	now := s.Clock.Now()
	from, _ := domain.MonthPeriod(now)
	to := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	if req.From != nil {
		from = req.From.UTC().Truncate(24 * time.Hour)
	}
	if req.To != nil {
		to = req.To.UTC().Truncate(24 * time.Hour)
	}
	if to.Before(from) {
		v.Add("to", "before_from")
	} else if to.Sub(from) > 366*24*time.Hour {
		v.Add("to", "range_too_long")
	}
	if err := v.Err(); err != nil {
		return Summary{}, err
	}
	cells, err := s.Repo.UsageSummary(ctx, SummaryFilter{From: from, To: to.AddDate(0, 0, 1), GroupBy: req.GroupBy})
	if err != nil {
		return Summary{}, err
	}
	out := Summary{From: from, To: to, GroupBy: req.GroupBy, Currency: kernel.CurrencyUSD, Rows: []SummaryRow{}, Totals: zeroTotals()}
	idx := map[string]int{}
	for _, c := range cells {
		i, ok := idx[c.Key]
		if !ok {
			i = len(out.Rows)
			idx[c.Key] = i
			out.Rows = append(out.Rows, SummaryRow{Key: c.Key, Label: c.Key, SourceTotals: zeroTotals()})
		}
		out.Rows[i].add(c)
		out.Totals.add(c)
	}
	if req.GroupBy == "day" {
		sort.Slice(out.Rows, func(i, j int) bool { return out.Rows[i].Key < out.Rows[j].Key })
	} else {
		sort.SliceStable(out.Rows, func(i, j int) bool { return out.Rows[i].Total.GreaterThan(out.Rows[j].Total) })
	}
	if req.GroupBy == "campaign" {
		var ids []kernel.ID
		for _, r := range out.Rows {
			if id, err := kernel.ParseID("key", r.Key); err == nil {
				ids = append(ids, id)
			}
		}
		names, err := s.Repo.CampaignNames(ctx, ids)
		if err != nil {
			return Summary{}, err
		}
		for i := range out.Rows {
			if out.Rows[i].Key == "" {
				out.Rows[i].Label = "Không thuộc chiến dịch"
			} else if id, err := kernel.ParseID("key", out.Rows[i].Key); err == nil && names[id] != "" {
				out.Rows[i].Label = names[id]
			}
		}
	}
	if req.GroupBy == "user" && s.Users != nil {
		var ids []kernel.ID
		for _, r := range out.Rows {
			if id, err := kernel.ParseID("key", r.Key); err == nil {
				ids = append(ids, id)
			}
		}
		if len(ids) > 0 {
			users, err := s.Users.GetUsers(ctx, ids)
			if err != nil && !errors.Is(err, kernel.ErrNotFound) {
				return Summary{}, err
			}
			names := map[string]string{}
			for _, u := range users {
				names[u.ID.String()] = u.Name
				if u.Name == "" {
					names[u.ID.String()] = u.Email
				}
			}
			for i := range out.Rows {
				if n, ok := names[out.Rows[i].Key]; ok {
					out.Rows[i].Label = n
				}
			}
		}
	}
	return out, nil
}
