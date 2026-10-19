package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sort"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/be/acs/api/internal/modules/budget/domain"
	identityuc "github.com/be/acs/api/internal/modules/identity/usecase"
	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
)

// UserDirectory is the part of identity.API the budget module uses
// (alert recipients, user labels in the usage summary).
type UserDirectory interface {
	UsersWithPermission(ctx context.Context, perm string) ([]identityuc.UserSummary, error)
	GetUsers(ctx context.Context, ids []kernel.ID) ([]identityuc.UserSummary, error)
}

// Service implements API plus the HTTP-facing use cases of the module.
type Service struct {
	Tx       ports.TxManager
	Repo     Repository
	Clock    ports.Clock
	Audit    ports.AuditRecorder
	Notifier ports.Notifier
	Users    UserDirectory // may be nil (no alert recipients)
	Mailer   ports.Mailer  // may be nil (alert e-mails stay queued)
	Logger   *slog.Logger
}

var _ API = (*Service)(nil)

func (s *Service) log() *slog.Logger {
	if s.Logger != nil {
		return s.Logger
	}
	return slog.Default()
}

func usd(d decimal.Decimal) kernel.Money { return kernel.NewMoney(d, kernel.CurrencyUSD) }

func f64(d decimal.Decimal) float64 {
	f, _ := d.Round(6).Float64()
	return f
}

// ---------------------------------------------------------------- estimate

// Estimate prices items with the pricing effective now (ACS-040).
func (s *Service) Estimate(ctx context.Context, req EstimateRequest) (Estimate, error) {
	out := Estimate{Total: kernel.ZeroUSD(), Lines: []EstimateLine{}}
	var versions []string
	for _, it := range req.Items {
		if it.Units.IsNegative() {
			return Estimate{}, kernel.Validation(kernel.FieldIssue("units", "negative"))
		}
		line := EstimateLine{Item: it.Item, ProviderCode: it.ProviderCode, ModelCode: it.ModelCode, Unit: it.Unit, Units: it.Units,
			UnitPrice: decimal.Zero, Amount: kernel.ZeroUSD()}
		if !domain.IsInternalUnit(it.Unit) {
			p, err := s.price(ctx, it.ProviderCode, it.ModelCode, it.Unit)
			if err != nil {
				return Estimate{}, err
			}
			line.UnitPrice = p.UnitPrice
			line.Amount = usd(p.UnitPrice.Mul(it.Units)).Round()
			if !slices.Contains(versions, p.Version) {
				versions = append(versions, p.Version)
			}
		}
		out.Lines = append(out.Lines, line)
		out.Total = out.Total.Add(line.Amount)
	}
	out.PricingVersion = strings.Join(versions, ",")
	return out, nil
}

func (s *Service) price(ctx context.Context, provider, model, unit string) (Price, error) {
	missing := kernel.ErrPricingMissing.WithDetails(kernel.Detail{"provider": provider, "model": model, "unit": unit})
	if provider == "" || model == "" || unit == "" {
		return Price{}, missing
	}
	p, err := s.Repo.EffectivePrice(ctx, provider, model, unit, s.Clock.Now())
	if errors.Is(err, kernel.ErrNotFound) {
		return Price{}, missing
	}
	if err != nil {
		return Price{}, err
	}
	if p.Currency != kernel.CurrencyUSD {
		return Price{}, missing
	}
	return p, nil
}

// ------------------------------------------------------------------ limits

func (s *Service) currentGlobal(ctx context.Context, settings domain.Settings, forUpdate bool) (domain.Budget, error) {
	now := s.Clock.Now()
	start, _ := domain.MonthPeriod(now)
	b, err := s.Repo.GetGlobalBudget(ctx, start, forUpdate)
	if err == nil {
		return b, nil
	}
	if !errors.Is(err, kernel.ErrNotFound) {
		return domain.Budget{}, err
	}
	if !forUpdate {
		return settings.NewGlobalBudget(now), nil // not created yet: the template applies
	}
	if err := s.Repo.EnsureGlobalBudget(ctx, settings.NewGlobalBudget(now)); err != nil {
		return domain.Budget{}, err
	}
	return s.Repo.GetGlobalBudget(ctx, start, true)
}

func (s *Service) campaignBudget(ctx context.Context, id *kernel.ID, forUpdate bool) (*domain.Budget, error) {
	if id == nil {
		return nil, nil
	}
	b, err := s.Repo.GetCampaignBudget(ctx, *id, forUpdate)
	if errors.Is(err, kernel.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// blocks reports whether a hard-stop budget refuses amount: the amount does
// not fit, or the budget is already at 100% (ACS-044).
func blocks(b domain.Budget, amount decimal.Decimal) bool {
	return b.HardStop && (b.WouldExceed(amount) || b.Committed().GreaterThanOrEqual(b.Limit))
}

// Limits reports remaining budget for amount (no locking).
func (s *Service) Limits(ctx context.Context, campaignID *kernel.ID, category Category, amount kernel.Money) (Limits, error) {
	settings, err := s.Repo.GetSettings(ctx)
	if err != nil {
		return Limits{}, err
	}
	g, err := s.currentGlobal(ctx, settings, false)
	if err != nil {
		return Limits{}, err
	}
	c, err := s.campaignBudget(ctx, campaignID, false)
	if err != nil {
		return Limits{}, err
	}
	perJob := settings.PerJobLimit(string(category))
	out := Limits{
		PerJobLimit:      usd(perJob),
		GlobalRemaining:  usd(g.Remaining()),
		RequiresOverride: amount.Amount.GreaterThan(perJob),
		Blocked:          blocks(g, amount.Amount),
	}
	if c != nil {
		r := usd(c.Remaining())
		out.CampaignRemaining = &r
		out.Blocked = out.Blocked || blocks(*c, amount.Amount)
	}
	return out, nil
}

// ----------------------------------------------------------------- reserve

// Reserve holds amount for a job (ACS-041). Lock order: global → campaign.
func (s *Service) Reserve(ctx context.Context, req ReserveRequest) (Reservation, error) {
	if req.Amount.Currency != kernel.CurrencyUSD || req.Amount.IsNegative() {
		return Reservation{}, kernel.Validation(kernel.FieldIssue("amount", "invalid"))
	}
	var out Reservation
	err := s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		settings, err := s.Repo.GetSettings(ctx)
		if err != nil {
			return err
		}
		g, err := s.currentGlobal(ctx, settings, true)
		if err != nil {
			return err
		}
		c, err := s.campaignBudget(ctx, req.CampaignID, true)
		if err != nil {
			return err
		}
		// Idempotent: a job that already holds budget is not reserved twice.
		held, err := s.Repo.HeldReservations(ctx, req.JobID)
		if err != nil {
			return err
		}
		if len(held) > 0 {
			out = Reservation{JobID: req.JobID, Amount: usd(held[0].Amount)}
			return nil
		}
		amount := req.Amount.Amount
		limit := settings.PerJobLimit(string(req.Category))
		if req.PerJobLimit != nil {
			limit = req.PerJobLimit.Amount
		}
		details := kernel.Detail{"estimate": f64(amount), "per_job_limit": f64(limit), "currency": kernel.CurrencyUSD, "category": string(req.Category)}
		if amount.GreaterThan(limit) {
			if req.Override == nil || !req.Override.IsLeadOrAdmin() {
				return kernel.ErrJobBudgetExceeded.WithDetails(details)
			}
			if err := s.auditOverride(ctx, req, "per_job", details); err != nil {
				return err
			}
		}
		if blocks(g, amount) {
			d := kernel.Detail{"scope": "global", "estimate": f64(amount), "limit": f64(g.Limit), "remaining": f64(g.Remaining()), "currency": kernel.CurrencyUSD}
			if req.Override == nil || !req.Override.IsAdmin() {
				return kernel.ErrBudgetExceeded.WithMessage("Ngân sách toàn hệ thống đã hết, vui lòng liên hệ Admin").WithDetails(d)
			}
			if err := s.auditOverride(ctx, req, "global_hard_stop", d); err != nil {
				return err
			}
		}
		if c != nil && blocks(*c, amount) {
			d := kernel.Detail{"scope": "campaign", "campaign_id": c.CampaignID.String(), "estimate": f64(amount), "limit": f64(c.Limit), "remaining": f64(c.Remaining()), "currency": kernel.CurrencyUSD}
			if req.Override == nil || !req.Override.IsLeadOrAdmin() {
				return kernel.ErrBudgetExceeded.WithMessage("Ngân sách chiến dịch đã hết").WithDetails(d)
			}
			if err := s.auditOverride(ctx, req, "campaign_hard_stop", d); err != nil {
				return err
			}
		}
		budgets := []*domain.Budget{&g}
		if c != nil {
			budgets = append(budgets, c)
		}
		for _, b := range budgets {
			b.Reserved = b.Reserved.Add(amount)
			if err := s.fireAlerts(ctx, b); err != nil {
				return err
			}
			b.UpdatedAt = s.Clock.Now()
			if err := s.Repo.SaveBudget(ctx, *b); err != nil {
				return err
			}
			if err := s.Repo.UpsertReservation(ctx, b.ID, req.JobID, amount); err != nil {
				return err
			}
		}
		out = Reservation{JobID: req.JobID, Amount: usd(amount)}
		return nil
	})
	return out, err
}

func (s *Service) auditOverride(ctx context.Context, req ReserveRequest, kind string, details kernel.Detail) error {
	if s.Audit == nil {
		return nil
	}
	after := map[string]any{"kind": kind, "override_by": req.Override.UserID.String(), "details": details}
	if req.CampaignID != nil {
		after["campaign_id"] = req.CampaignID.String()
	}
	ctx = kernel.WithActor(ctx, *req.Override)
	return s.Audit.Record(ctx, "budget.override", "ai_job", req.JobID.String(), nil, after)
}

// ------------------------------------------------------------ settle/release

// Settle converts the reservation into spend = unsettled usage of the job,
// and writes ai_jobs.cost_actual = sum of all usage records (ACS-042).
func (s *Service) Settle(ctx context.Context, jobID kernel.ID) error { return s.finish(ctx, jobID) }

// Release frees the reservation. Usage already recorded for the job is still
// counted as spend (money a provider may have charged is never dropped), so
// Release and Settle share one implementation.
func (s *Service) Release(ctx context.Context, jobID kernel.ID) error { return s.finish(ctx, jobID) }

func (s *Service) finish(ctx context.Context, jobID kernel.ID) error {
	return s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		now := s.Clock.Now()
		held, err := s.Repo.HeldReservations(ctx, jobID)
		if err != nil {
			return err
		}
		usage, err := s.Repo.UnsettledUsage(ctx, jobID)
		if err != nil {
			return err
		}
		// Lock order: global before campaign.
		sort.SliceStable(held, func(i, j int) bool { return held[i].Scope == domain.ScopeGlobal && held[j].Scope != domain.ScopeGlobal })
		type touch struct {
			b        domain.Budget
			released decimal.Decimal
		}
		var touched []touch
		if len(held) > 0 {
			for _, r := range held {
				b, err := s.Repo.GetBudget(ctx, r.BudgetID, true)
				if err != nil {
					return err
				}
				touched = append(touched, touch{b: b, released: r.Amount})
			}
		} else if usage.Total.IsPositive() {
			// Late usage without a held reservation: book it on the current
			// global budget (and the campaign's, if any).
			settings, err := s.Repo.GetSettings(ctx)
			if err != nil {
				return err
			}
			g, err := s.currentGlobal(ctx, settings, true)
			if err != nil {
				return err
			}
			touched = append(touched, touch{b: g, released: decimal.Zero})
			c, err := s.campaignBudget(ctx, usage.CampaignID, true)
			if err != nil {
				return err
			}
			if c != nil {
				touched = append(touched, touch{b: *c, released: decimal.Zero})
			}
		}
		for _, t := range touched {
			b := t.b
			b.Reserved = b.Reserved.Sub(t.released)
			if b.Reserved.IsNegative() {
				b.Reserved = decimal.Zero
			}
			b.Spent = b.Spent.Add(usage.Total)
			if err := s.fireAlerts(ctx, &b); err != nil {
				return err
			}
			b.UpdatedAt = now
			if err := s.Repo.SaveBudget(ctx, b); err != nil {
				return err
			}
		}
		status := "released"
		if usage.Count > 0 {
			status = "settled"
		}
		for _, r := range held {
			if err := s.Repo.SetReservationStatus(ctx, r.ID, status, now); err != nil {
				return err
			}
		}
		if usage.Count > 0 {
			if err := s.Repo.MarkUsageSettled(ctx, jobID, now); err != nil {
				return err
			}
		}
		tot, err := s.Repo.JobUsageTotals(ctx, jobID)
		if err != nil {
			return err
		}
		if tot.Count == 0 {
			return nil
		}
		return s.Repo.UpdateJobCost(ctx, jobID, tot.Total.Round(6), jobCostSource(tot), tot.Estimated > 0)
	})
}

// jobCostSource is the weakest source among the job's records:
// estimated < calculated < provider_reported.
func jobCostSource(t JobUsageTotals) CostSource {
	switch {
	case t.Estimated > 0:
		return SourceEstimated
	case t.Calculated > 0:
		return SourceCalculated
	}
	return SourceProviderReported
}

// ------------------------------------------------------------------- usage

// RecordUsage prices and stores one usage record (ACS-042).
func (s *Service) RecordUsage(ctx context.Context, in UsageInput) (UsageRecord, error) {
	now := s.Clock.Now()
	cost, source, version, missing := s.priceUsage(ctx, in)
	if missing != "" {
		// Never drop usage a provider charged for: keep it as an estimate
		// and flag the job for reconciliation.
		s.log().WarnContext(ctx, "usage priced without effective pricing", slog.String("job_id", in.JobID.String()),
			slog.String("provider", in.ProviderCode), slog.String("model", in.ModelCode), slog.String("unit", missing))
		source = SourceEstimated
	}
	if in.Estimated {
		source = SourceEstimated
	}
	raw, err := marshalUsage(in.Usage)
	if err != nil {
		return UsageRecord{}, err
	}
	row := UsageRow{ID: kernel.NewID(), JobID: in.JobID, StepID: in.StepID, CampaignID: in.CampaignID, UserID: in.UserID,
		ProviderCode: in.ProviderCode, ModelCode: in.ModelCode, JobType: in.JobType, UsageJSON: raw,
		Cost: cost.Round(6), Source: source, Currency: kernel.CurrencyUSD, PricingVersion: version, OccurredAt: now}
	err = s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.Repo.InsertUsage(ctx, row); err != nil {
			return err
		}
		if source == SourceEstimated {
			return s.Repo.MarkJobNeedsReconciliation(ctx, in.JobID)
		}
		return nil
	})
	if err != nil {
		return UsageRecord{}, err
	}
	return UsageRecord{ID: row.ID, Cost: usd(row.Cost), Source: source, PricingVersion: version}, nil
}

func (s *Service) priceUsage(ctx context.Context, in UsageInput) (cost decimal.Decimal, source CostSource, version, missing string) {
	if in.Usage.ReportedCost != nil && in.Usage.ReportedCost.Currency == kernel.CurrencyUSD {
		return in.Usage.ReportedCost.Amount, SourceProviderReported, "", ""
	}
	units := make([]string, 0, len(in.Usage.Units))
	for u := range in.Usage.Units {
		units = append(units, u)
	}
	sort.Strings(units)
	cost = decimal.Zero
	var versions []string
	for _, u := range units {
		if domain.IsInternalUnit(u) {
			continue
		}
		p, err := s.price(ctx, in.ProviderCode, in.ModelCode, u)
		if err != nil {
			missing = u
			continue
		}
		cost = cost.Add(p.UnitPrice.Mul(decimal.NewFromFloat(in.Usage.Units[u])))
		if !slices.Contains(versions, p.Version) {
			versions = append(versions, p.Version)
		}
	}
	return cost, SourceCalculated, strings.Join(versions, ","), missing
}

// ------------------------------------------------------------------ alerts

// fireAlerts marks newly reached thresholds of b as sent (caller saves b)
// and emits the in-app notification + queued e-mail, inside the caller's
// transaction (ACS-044: each threshold once per period).
func (s *Service) fireAlerts(ctx context.Context, b *domain.Budget) error {
	due := b.DueAlerts()
	if len(due) == 0 {
		return nil
	}
	b.AlertsSent = append(b.AlertsSent, due...)
	slices.Sort(b.AlertsSent)
	var recipients []identityuc.UserSummary
	if s.Users != nil {
		var err error
		if recipients, err = s.Users.UsersWithPermission(ctx, kernel.PermBudgetManage); err != nil {
			return err
		}
	}
	threshold := due[len(due)-1] // one message for the highest threshold reached
	ids := make([]kernel.ID, 0, len(recipients))
	emails := make([]string, 0, len(recipients))
	for _, u := range recipients {
		ids = append(ids, u.ID)
		if u.Email != "" {
			emails = append(emails, u.Email)
		}
	}
	payload := map[string]any{
		"scope": string(b.Scope), "budget_id": b.ID.String(), "threshold": threshold, "thresholds": due,
		"percent_used": f64(b.PercentUsed().Round(2)), "limit_amount": f64(b.Limit), "spent_amount": f64(b.Spent),
		"reserved_amount": f64(b.Reserved), "currency": b.Currency, "hard_stop": b.HardStop,
		"period_start": b.PeriodStart.Format("2006-01-02"), "period_end": b.PeriodEnd.Format("2006-01-02"),
	}
	if b.CampaignID != nil {
		payload["campaign_id"] = b.CampaignID.String()
	}
	if len(ids) > 0 && s.Notifier != nil {
		if err := s.Notifier.Notify(ctx, ids, ports.NotifyBudgetAlert, payload); err != nil {
			return err
		}
	}
	if len(emails) == 0 {
		return nil
	}
	subject, body := alertEmail(*b, threshold)
	return s.Repo.QueueAlertEmail(ctx, AlertEmail{ID: kernel.NewID(), BudgetID: b.ID, Threshold: threshold, Recipients: emails, Subject: subject, Body: body})
}

func alertEmail(b domain.Budget, threshold int) (subject, body string) {
	scope := "toàn hệ thống"
	if b.Scope == domain.ScopeCampaign && b.CampaignID != nil {
		scope = "chiến dịch " + b.CampaignID.String()
	}
	subject = fmt.Sprintf("[AI Content Studio] Ngân sách %s đã dùng %d%%", scope, threshold)
	body = fmt.Sprintf("Ngân sách %s (kỳ %s – %s) đã dùng %s%%.\n\nHạn mức: %s %s\nĐã chi: %s %s\nĐang giữ chỗ: %s %s\n",
		scope, b.PeriodStart.Format("02/01/2006"), b.PeriodEnd.Format("02/01/2006"), b.PercentUsed().StringFixed(1),
		b.Limit.StringFixed(2), b.Currency, b.Spent.StringFixed(2), b.Currency, b.Reserved.StringFixed(2), b.Currency)
	if threshold >= 100 && b.HardStop {
		body += "\nHard stop đang bật: các tác vụ AI mới sẽ bị chặn cho đến khi tăng hạn mức (Admin có thể override).\n"
	}
	return subject, body
}

// DeliverAlertEmails sends queued alert e-mails (worker runner). Returns the
// number sent. Failures are retried up to 5 attempts.
func (s *Service) DeliverAlertEmails(ctx context.Context) (int, error) {
	if s.Mailer == nil {
		return 0, nil
	}
	pending, err := s.Repo.PendingAlertEmails(ctx, 5, 20)
	if err != nil {
		return 0, err
	}
	sent := 0
	for _, e := range pending {
		if err := s.Mailer.Send(ctx, e.Recipients, e.Subject, e.Body); err != nil {
			s.log().WarnContext(ctx, "budget alert e-mail failed", slog.String("email_id", e.ID.String()), slog.String("error", err.Error()))
			if err := s.Repo.MarkAlertEmailFailed(ctx, e.ID, err.Error()); err != nil {
				return sent, err
			}
			continue
		}
		if err := s.Repo.MarkAlertEmailSent(ctx, e.ID, s.Clock.Now()); err != nil {
			return sent, err
		}
		sent++
	}
	return sent, nil
}
