package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/be/acs/api/internal/modules/budget/domain"
	"github.com/be/acs/api/internal/modules/budget/usecase"
	db "github.com/be/acs/api/internal/platform/postgres"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Repository implements usecase.Repository on PostgreSQL.
type Repository struct{ pool *pgxpool.Pool }

// NewRepository returns a repository on pool.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

var _ usecase.Repository = (*Repository)(nil)

func (r *Repository) q(ctx context.Context) *Queries { return New(db.Conn(ctx, r.pool)) }

func date(t time.Time) pgtype.Date {
	t = t.UTC()
	return pgtype.Date{Time: time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), Valid: true}
}

func fromDate(d pgtype.Date) time.Time {
	if !d.Valid {
		return time.Time{}
	}
	return time.Date(d.Time.Year(), d.Time.Month(), d.Time.Day(), 0, 0, 0, 0, time.UTC)
}

func ints(in []int32) []int {
	out := make([]int, len(in))
	for i, v := range in {
		out[i] = int(v)
	}
	return out
}

func int32s(in []int) []int32 {
	out := make([]int32, len(in))
	for i, v := range in {
		out[i] = int32(v) //nolint:gosec // thresholds are validated to 1..100
	}
	return out
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ---- pricing

// EffectivePrice returns the pricing row effective at `at` (NOT_FOUND if none).
func (r *Repository) EffectivePrice(ctx context.Context, provider, model, unit string, at time.Time) (usecase.Price, error) {
	row, err := r.q(ctx).EffectivePrice(ctx, EffectivePriceParams{ProviderCode: provider, ModelCode: model, Unit: unit, At: at})
	if err != nil {
		return usecase.Price{}, db.MapNotFound(err)
	}
	return usecase.Price{UnitPrice: row.UnitPrice, Currency: row.Currency, Version: row.PricingVersion}, nil
}

// DefaultModel returns the active default model of kind.
func (r *Repository) DefaultModel(ctx context.Context, kind string) (string, string, error) {
	row, err := r.q(ctx).DefaultModel(ctx, kind)
	if err != nil {
		return "", "", db.MapNotFound(err)
	}
	return row.ProviderCode, row.ModelCode, nil
}

// ---- settings

// GetSettings loads the singleton budget_settings row.
func (r *Repository) GetSettings(ctx context.Context) (domain.Settings, error) {
	row, err := r.q(ctx).GetSettings(ctx)
	if err != nil {
		return domain.Settings{}, db.MapNotFound(err)
	}
	return domain.Settings{
		PerJobText: row.PerJobLimitText, PerJobImage: row.PerJobLimitImage, PerJobVideo: row.PerJobLimitVideo,
		GlobalMonthlyLimit: row.GlobalMonthlyLimit, GlobalThresholds: ints(row.GlobalAlertThresholds),
		GlobalHardStop: row.GlobalHardStop, MaxConcurrentVideoJobs: int(row.MaxConcurrentVideoJobsPerUser),
		Currency: row.Currency, UpdatedBy: row.UpdatedBy, UpdatedAt: row.UpdatedAt,
	}, nil
}

// SaveSettings updates the singleton row.
func (r *Repository) SaveSettings(ctx context.Context, s domain.Settings) error {
	return r.q(ctx).SaveSettings(ctx, SaveSettingsParams{
		PerJobLimitText: s.PerJobText, PerJobLimitImage: s.PerJobImage, PerJobLimitVideo: s.PerJobVideo,
		GlobalMonthlyLimit: s.GlobalMonthlyLimit, GlobalAlertThresholds: int32s(s.GlobalThresholds),
		GlobalHardStop: s.GlobalHardStop, MaxConcurrentVideoJobsPerUser: int32(s.MaxConcurrentVideoJobs), //nolint:gosec // validated 1..50
		UpdatedBy: s.UpdatedBy, UpdatedAt: s.UpdatedAt,
	})
}

// ---- budgets

func toBudget(row GetBudgetRow) domain.Budget {
	return domain.Budget{
		ID: row.ID, Scope: domain.Scope(row.Scope), CampaignID: row.CampaignID,
		PeriodStart: fromDate(row.PeriodStart), PeriodEnd: fromDate(row.PeriodEnd),
		Limit: row.LimitAmount, Reserved: row.ReservedAmount, Spent: row.SpentAmount, Currency: row.Currency,
		Thresholds: ints(row.AlertThresholds), AlertsSent: ints(row.AlertsSent), HardStop: row.HardStop, UpdatedAt: row.UpdatedAt,
	}
}

// EnsureGlobalBudget inserts b unless the month's global budget exists.
func (r *Repository) EnsureGlobalBudget(ctx context.Context, b domain.Budget) error {
	return r.q(ctx).EnsureGlobalBudget(ctx, EnsureGlobalBudgetParams{
		ID: b.ID, PeriodStart: date(b.PeriodStart), PeriodEnd: date(b.PeriodEnd), LimitAmount: b.Limit,
		Currency: b.Currency, AlertThresholds: int32s(b.Thresholds), HardStop: b.HardStop, UpdatedAt: b.UpdatedAt,
	})
}

// GetGlobalBudget loads the global budget of the period starting at periodStart.
func (r *Repository) GetGlobalBudget(ctx context.Context, periodStart time.Time, forUpdate bool) (domain.Budget, error) {
	if forUpdate {
		row, err := r.q(ctx).GetGlobalBudgetForUpdate(ctx, date(periodStart))
		if err != nil {
			return domain.Budget{}, db.MapNotFound(err)
		}
		return toBudget(GetBudgetRow(row)), nil
	}
	row, err := r.q(ctx).GetGlobalBudget(ctx, date(periodStart))
	if err != nil {
		return domain.Budget{}, db.MapNotFound(err)
	}
	return toBudget(GetBudgetRow(row)), nil
}

// GetCampaignBudget loads a campaign's budget.
func (r *Repository) GetCampaignBudget(ctx context.Context, campaignID kernel.ID, forUpdate bool) (domain.Budget, error) {
	if forUpdate {
		row, err := r.q(ctx).GetCampaignBudgetForUpdate(ctx, &campaignID)
		if err != nil {
			return domain.Budget{}, db.MapNotFound(err)
		}
		return toBudget(GetBudgetRow(row)), nil
	}
	row, err := r.q(ctx).GetCampaignBudget(ctx, &campaignID)
	if err != nil {
		return domain.Budget{}, db.MapNotFound(err)
	}
	return toBudget(GetBudgetRow(row)), nil
}

// GetBudget loads a budget by id.
func (r *Repository) GetBudget(ctx context.Context, id kernel.ID, forUpdate bool) (domain.Budget, error) {
	if forUpdate {
		row, err := r.q(ctx).GetBudgetForUpdate(ctx, id)
		if err != nil {
			return domain.Budget{}, db.MapNotFound(err)
		}
		return toBudget(GetBudgetRow(row)), nil
	}
	row, err := r.q(ctx).GetBudget(ctx, id)
	if err != nil {
		return domain.Budget{}, db.MapNotFound(err)
	}
	return toBudget(row), nil
}

// ListCampaignBudgets lists all campaign budgets.
func (r *Repository) ListCampaignBudgets(ctx context.Context) ([]domain.Budget, error) {
	rows, err := r.q(ctx).ListCampaignBudgets(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Budget, 0, len(rows))
	for _, row := range rows {
		out = append(out, toBudget(GetBudgetRow(row)))
	}
	return out, nil
}

// InsertBudget creates a campaign budget (NOT_FOUND for an unknown campaign).
func (r *Repository) InsertBudget(ctx context.Context, b domain.Budget) error {
	err := r.q(ctx).InsertBudget(ctx, InsertBudgetParams{
		ID: b.ID, Scope: string(b.Scope), CampaignID: b.CampaignID, PeriodStart: date(b.PeriodStart), PeriodEnd: date(b.PeriodEnd),
		LimitAmount: b.Limit, ReservedAmount: b.Reserved, SpentAmount: b.Spent, Currency: b.Currency,
		AlertThresholds: int32s(b.Thresholds), AlertsSent: int32s(b.AlertsSent), HardStop: b.HardStop, UpdatedAt: b.UpdatedAt,
	})
	if db.IsForeignKeyViolation(err, "") {
		return kernel.ErrNotFound.WithMessage("Không tìm thấy chiến dịch")
	}
	if db.IsUniqueViolation(err, "") {
		return kernel.ErrConflict.WithMessage("Chiến dịch đã có ngân sách")
	}
	return err
}

// SaveBudget updates a budget row.
func (r *Repository) SaveBudget(ctx context.Context, b domain.Budget) error {
	return r.q(ctx).SaveBudget(ctx, SaveBudgetParams{
		PeriodStart: date(b.PeriodStart), PeriodEnd: date(b.PeriodEnd), LimitAmount: b.Limit,
		ReservedAmount: b.Reserved, SpentAmount: b.Spent, AlertThresholds: int32s(b.Thresholds),
		AlertsSent: int32s(b.AlertsSent), HardStop: b.HardStop, UpdatedAt: b.UpdatedAt, ID: b.ID,
	})
}

// CampaignNames maps campaign ids to names (usage summary labels).
func (r *Repository) CampaignNames(ctx context.Context, ids []kernel.ID) (map[kernel.ID]string, error) {
	out := map[kernel.ID]string{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.q(ctx).CampaignNames(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.ID] = row.Name
	}
	return out, nil
}

// ---- reservations

// HeldReservations lists a job's held reservations, global first.
func (r *Repository) HeldReservations(ctx context.Context, jobID kernel.ID) ([]usecase.ReservationRow, error) {
	rows, err := r.q(ctx).HeldReservations(ctx, jobID)
	if err != nil {
		return nil, err
	}
	out := make([]usecase.ReservationRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, usecase.ReservationRow{ID: row.ID, BudgetID: row.BudgetID, Scope: domain.Scope(row.Scope),
			JobID: row.JobID, Amount: row.Amount, Status: row.Status})
	}
	return out, nil
}

// UpsertReservation holds amount on a budget for a job.
func (r *Repository) UpsertReservation(ctx context.Context, budgetID, jobID kernel.ID, amount decimal.Decimal) error {
	err := r.q(ctx).UpsertReservation(ctx, UpsertReservationParams{BudgetID: budgetID, JobID: jobID, Amount: amount})
	if db.IsForeignKeyViolation(err, "") {
		return kernel.ErrNotFound.WithMessage("Không tìm thấy tác vụ")
	}
	return err
}

// SetReservationStatus marks a reservation settled/released.
func (r *Repository) SetReservationStatus(ctx context.Context, id kernel.ID, status string, at time.Time) error {
	return r.q(ctx).SetReservationStatus(ctx, SetReservationStatusParams{Status: status, SettledAt: &at, ID: id})
}

// ---- usage

// InsertUsage stores one usage record.
func (r *Repository) InsertUsage(ctx context.Context, u usecase.UsageRow) error {
	return r.q(ctx).InsertUsage(ctx, InsertUsageParams{
		ID: u.ID, JobID: u.JobID, StepID: u.StepID, CampaignID: u.CampaignID, UserID: u.UserID,
		ProviderCode: u.ProviderCode, ModelCode: u.ModelCode, JobType: strPtr(u.JobType), UsageJson: u.UsageJSON,
		CostAmount: u.Cost, CostSource: string(u.Source), Currency: u.Currency, PricingVersion: strPtr(u.PricingVersion),
		OccurredAt: u.OccurredAt,
	})
}

// UnsettledUsage sums the job's usage not yet moved into spend.
func (r *Repository) UnsettledUsage(ctx context.Context, jobID kernel.ID) (usecase.UnsettledUsage, error) {
	row, err := r.q(ctx).UnsettledUsage(ctx, jobID)
	if err != nil {
		return usecase.UnsettledUsage{}, err
	}
	out := usecase.UnsettledUsage{Total: row.Total, Count: row.Count}
	if row.Count == 0 {
		return out, nil
	}
	cs, err := r.q(ctx).UsageCampaigns(ctx, jobID)
	if err != nil {
		return usecase.UnsettledUsage{}, err
	}
	if len(cs) > 0 {
		out.CampaignID = cs[0]
	}
	return out, nil
}

// MarkUsageSettled stamps the job's unsettled usage.
func (r *Repository) MarkUsageSettled(ctx context.Context, jobID kernel.ID, at time.Time) error {
	return r.q(ctx).MarkUsageSettled(ctx, MarkUsageSettledParams{SettledAt: &at, JobID: jobID})
}

// JobUsageTotals aggregates all usage of a job.
func (r *Repository) JobUsageTotals(ctx context.Context, jobID kernel.ID) (usecase.JobUsageTotals, error) {
	row, err := r.q(ctx).JobUsageTotals(ctx, jobID)
	if err != nil {
		return usecase.JobUsageTotals{}, err
	}
	return usecase.JobUsageTotals{Total: row.Total, Count: row.Count, ProviderReported: row.ProviderReported,
		Calculated: row.Calculated, Estimated: row.Estimated}, nil
}

// UpdateJobCost writes ai_jobs.cost_actual/cost_source.
func (r *Repository) UpdateJobCost(ctx context.Context, jobID kernel.ID, actual decimal.Decimal, source usecase.CostSource, recon bool) error {
	src := string(source)
	return r.q(ctx).UpdateJobCost(ctx, UpdateJobCostParams{CostActual: decimal.NullDecimal{Decimal: actual, Valid: true},
		CostSource: &src, NeedsReconciliation: recon, ID: jobID})
}

// MarkJobNeedsReconciliation flags the job.
func (r *Repository) MarkJobNeedsReconciliation(ctx context.Context, jobID kernel.ID) error {
	return r.q(ctx).MarkJobNeedsReconciliation(ctx, jobID)
}

// UsageSummary aggregates usage by (group key, cost_source).
func (r *Repository) UsageSummary(ctx context.Context, f usecase.SummaryFilter) ([]usecase.SummaryCell, error) {
	rows, err := r.q(ctx).UsageSummary(ctx, UsageSummaryParams{GroupBy: f.GroupBy, FromAt: f.From, ToAt: f.To})
	if err != nil {
		return nil, err
	}
	out := make([]usecase.SummaryCell, 0, len(rows))
	for _, row := range rows {
		out = append(out, usecase.SummaryCell{Key: row.Key, Source: usecase.CostSource(row.CostSource), Amount: row.Amount, Count: row.Count})
	}
	return out, nil
}

// ---- alert e-mail outbox

// QueueAlertEmail stores an e-mail to send.
func (r *Repository) QueueAlertEmail(ctx context.Context, e usecase.AlertEmail) error {
	return r.q(ctx).QueueAlertEmail(ctx, QueueAlertEmailParams{ID: e.ID, BudgetID: e.BudgetID, Threshold: int32(e.Threshold), //nolint:gosec // 1..100
		Recipients: e.Recipients, Subject: e.Subject, Body: e.Body})
}

// PendingAlertEmails lists unsent e-mails below maxAttempts.
func (r *Repository) PendingAlertEmails(ctx context.Context, maxAttempts, limit int) ([]usecase.AlertEmail, error) {
	rows, err := r.q(ctx).PendingAlertEmails(ctx, PendingAlertEmailsParams{MaxAttempts: int32(maxAttempts), Lim: int32(limit)}) //nolint:gosec // small constants
	if err != nil {
		return nil, err
	}
	out := make([]usecase.AlertEmail, 0, len(rows))
	for _, row := range rows {
		out = append(out, usecase.AlertEmail{ID: row.ID, BudgetID: row.BudgetID, Threshold: int(row.Threshold),
			Recipients: row.Recipients, Subject: row.Subject, Body: row.Body, Attempts: int(row.Attempts)})
	}
	return out, nil
}

// MarkAlertEmailSent stamps sent_at.
func (r *Repository) MarkAlertEmailSent(ctx context.Context, id kernel.ID, at time.Time) error {
	return r.q(ctx).MarkAlertEmailSent(ctx, MarkAlertEmailSentParams{SentAt: &at, ID: id})
}

// MarkAlertEmailFailed records a delivery failure.
func (r *Repository) MarkAlertEmailFailed(ctx context.Context, id kernel.ID, msg string) error {
	return r.q(ctx).MarkAlertEmailFailed(ctx, MarkAlertEmailFailedParams{LastError: &msg, ID: id})
}
