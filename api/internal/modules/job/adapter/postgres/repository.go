// Package postgres implements the job module's Repository with sqlc.
package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/be/acs/api/internal/modules/job/usecase"
	db "github.com/be/acs/api/internal/platform/postgres"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Repository implements usecase.Repository.
type Repository struct{ pool *pgxpool.Pool }

// NewRepository returns a repository on pool.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

var _ usecase.Repository = (*Repository)(nil)

func (r *Repository) q(ctx context.Context) *Queries { return New(db.Conn(ctx, r.pool)) }

func str(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func nullMoney(m *kernel.Money) decimal.NullDecimal {
	if m == nil {
		return decimal.NullDecimal{}
	}
	return decimal.NullDecimal{Decimal: m.Amount, Valid: true}
}

func money(d decimal.NullDecimal, currency string) *kernel.Money {
	if !d.Valid {
		return nil
	}
	m := kernel.NewMoney(d.Decimal, currency)
	return &m
}

func toJob(a AiJob) usecase.JobRecord {
	return usecase.JobRecord{
		ID: a.ID, Type: usecase.JobType(a.JobType), Status: usecase.Status(a.Status), CampaignID: a.CampaignID,
		AssetID: a.AssetID, BrandVersionID: a.BrandVersionID, PromptTemplateID: a.PromptTemplateID,
		ProviderCode: deref(a.ProviderCode), ModelCode: deref(a.ModelCode), Request: a.RequestJson, Result: a.ResultJson,
		IdempotencyKey: deref(a.IdempotencyKey), RequestHash: deref(a.RequestHash),
		CostEstimate: money(a.CostEstimate, a.Currency), CostActual: money(a.CostActual, a.Currency),
		CostSource: deref(a.CostSource), PricingVersion: deref(a.PricingVersion), BudgetOverrideBy: a.BudgetOverrideBy,
		CancelRequested: a.CancelRequested, NeedsReconciliation: a.NeedsReconciliation, Progress: int(a.Progress),
		ErrorCode: deref(a.ErrorCode), ErrorMessage: deref(a.ErrorMessage), CreatedBy: a.CreatedBy,
		StartedAt: a.StartedAt, FinishedAt: a.FinishedAt, CreatedAt: a.CreatedAt,
	}
}

func toStep(s AiJobStep) usecase.StepRecord {
	return usecase.StepRecord{
		ID: s.ID, JobID: s.JobID, Type: s.StepType, Key: s.StepKey, DependsOn: s.DependsOn, SceneID: s.SceneID,
		Status: usecase.StepStatus(s.Status), Input: s.InputJson, Output: s.OutputJson, ProviderJobID: deref(s.ProviderJobID),
		AttemptCount: int(s.AttemptCount), MaxAttempts: int(s.MaxAttempts), NextAttemptAt: s.NextAttemptAt,
		LeaseOwner: deref(s.LeaseOwner), LeaseExpiresAt: s.LeaseExpiresAt, ErrorKind: deref(s.ErrorKind),
		ErrorMessage: deref(s.ErrorMessage), StartedAt: s.StartedAt, FinishedAt: s.FinishedAt, CreatedAt: s.CreatedAt,
		Generation: int(s.Generation),
	}
}

func toSteps(rows []AiJobStep) []usecase.StepRecord {
	out := make([]usecase.StepRecord, len(rows))
	for i, s := range rows {
		out[i] = toStep(s)
	}
	return out
}

// InsertJob inserts j; inserted=false on an idempotency-key conflict.
func (r *Repository) InsertJob(ctx context.Context, j *usecase.JobRecord) (bool, error) {
	currency := kernel.CurrencyUSD
	if j.CostEstimate != nil && j.CostEstimate.Currency != "" {
		currency = j.CostEstimate.Currency
	}
	n, err := r.q(ctx).InsertJob(ctx, InsertJobParams{
		ID: j.ID, JobType: string(j.Type), Status: string(j.Status), CampaignID: j.CampaignID, AssetID: j.AssetID,
		BrandVersionID: j.BrandVersionID, PromptTemplateID: j.PromptTemplateID, ProviderCode: str(j.ProviderCode),
		ModelCode: str(j.ModelCode), RequestJson: j.Request, IdempotencyKey: str(j.IdempotencyKey),
		RequestHash: str(j.RequestHash), CostEstimate: nullMoney(j.CostEstimate), Currency: currency,
		PricingVersion: str(j.PricingVersion), BudgetOverrideBy: j.BudgetOverrideBy, Progress: int16(j.Progress),
		CreatedBy: j.CreatedBy, CreatedAt: j.CreatedAt,
	})
	return n == 1, err
}

// FindJobByIdempotency loads the job of (createdBy, key).
func (r *Repository) FindJobByIdempotency(ctx context.Context, createdBy kernel.ID, key string) (usecase.JobRecord, error) {
	a, err := r.q(ctx).FindJobByIdempotency(ctx, FindJobByIdempotencyParams{CreatedBy: createdBy, IdempotencyKey: &key})
	if err != nil {
		return usecase.JobRecord{}, db.MapNotFound(err)
	}
	return toJob(a), nil
}

// GetJob loads a job.
func (r *Repository) GetJob(ctx context.Context, id kernel.ID) (usecase.JobRecord, error) {
	a, err := r.q(ctx).GetJob(ctx, id)
	if err != nil {
		return usecase.JobRecord{}, db.MapNotFound(err)
	}
	return toJob(a), nil
}

// LockJob loads a job FOR UPDATE.
func (r *Repository) LockJob(ctx context.Context, id kernel.ID) (usecase.JobRecord, error) {
	a, err := r.q(ctx).LockJob(ctx, id)
	if err != nil {
		return usecase.JobRecord{}, db.MapNotFound(err)
	}
	return toJob(a), nil
}

// UpdateJob persists the mutable job fields.
func (r *Repository) UpdateJob(ctx context.Context, j *usecase.JobRecord) error {
	return r.q(ctx).UpdateJob(ctx, UpdateJobParams{
		ID: j.ID, Status: string(j.Status), ResultJson: j.Result, CostActual: nullMoney(j.CostActual),
		CostSource: str(j.CostSource), CancelRequested: j.CancelRequested, NeedsReconciliation: j.NeedsReconciliation,
		Progress: int16(j.Progress), ErrorCode: str(j.ErrorCode), ErrorMessage: str(j.ErrorMessage),
		StartedAt: j.StartedAt, FinishedAt: j.FinishedAt,
	})
}

// RaiseJobProgress sets progress = max(progress, pct).
func (r *Repository) RaiseJobProgress(ctx context.Context, id kernel.ID, pct int) error {
	return r.q(ctx).RaiseJobProgress(ctx, RaiseJobProgressParams{ID: id, Progress: int16(max(0, min(100, pct)))})
}

// ListJobs lists jobs newest first.
func (r *Repository) ListJobs(ctx context.Context, f usecase.JobFilter, limit int, after *kernel.TimeIDCursor) ([]usecase.JobRecord, error) {
	p := ListJobsParams{
		CampaignID: f.CampaignID, CreatedBy: f.CreatedBy, ProviderCode: str(f.ProviderCode),
		NeedsReconciliation: f.NeedsReconciliation, Lim: int32(limit),
	}
	for _, s := range f.Status {
		p.Statuses = append(p.Statuses, string(s))
	}
	for _, t := range f.Type {
		p.Types = append(p.Types, string(t))
	}
	if after != nil {
		at, id := after.At, after.ID
		p.AfterAt, p.AfterID = &at, &id
	}
	rows, err := r.q(ctx).ListJobs(ctx, p)
	if err != nil {
		return nil, err
	}
	out := make([]usecase.JobRecord, len(rows))
	for i, a := range rows {
		out[i] = toJob(a)
	}
	return out, nil
}

// InsertStep inserts a step.
func (r *Repository) InsertStep(ctx context.Context, s *usecase.StepRecord) error {
	deps := s.DependsOn
	if deps == nil {
		deps = []string{}
	}
	return r.q(ctx).InsertStep(ctx, InsertStepParams{
		ID: s.ID, JobID: s.JobID, StepType: s.Type, StepKey: s.Key, DependsOn: deps, SceneID: s.SceneID,
		Status: string(s.Status), InputJson: s.Input, MaxAttempts: int32(s.MaxAttempts), NextAttemptAt: s.NextAttemptAt,
		CreatedAt: s.CreatedAt,
	})
}

// ListSteps returns the steps of a job in creation order.
func (r *Repository) ListSteps(ctx context.Context, jobID kernel.ID) ([]usecase.StepRecord, error) {
	rows, err := r.q(ctx).ListSteps(ctx, jobID)
	if err != nil {
		return nil, err
	}
	return toSteps(rows), nil
}

// GetStep loads a step.
func (r *Repository) GetStep(ctx context.Context, id kernel.ID) (usecase.StepRecord, error) {
	s, err := r.q(ctx).GetStep(ctx, id)
	if err != nil {
		return usecase.StepRecord{}, db.MapNotFound(err)
	}
	return toStep(s), nil
}

// UpdateStep persists the mutable step fields.
func (r *Repository) UpdateStep(ctx context.Context, s *usecase.StepRecord) error {
	return r.q(ctx).UpdateStep(ctx, UpdateStepParams{
		ID: s.ID, Status: string(s.Status), OutputJson: s.Output, ProviderJobID: str(s.ProviderJobID),
		AttemptCount: int32(s.AttemptCount), NextAttemptAt: s.NextAttemptAt, LeaseOwner: str(s.LeaseOwner),
		LeaseExpiresAt: s.LeaseExpiresAt, ErrorKind: str(s.ErrorKind), ErrorMessage: str(s.ErrorMessage),
		StartedAt: s.StartedAt, FinishedAt: s.FinishedAt, Generation: int32(s.Generation),
	})
}

// ExtendLease moves the lease while owner still holds it.
func (r *Repository) ExtendLease(ctx context.Context, stepID kernel.ID, owner string, until time.Time) (bool, error) {
	n, err := r.q(ctx).ExtendLease(ctx, ExtendLeaseParams{ID: stepID, LeaseOwner: &owner, LeaseExpiresAt: &until})
	return n == 1, err
}

// SaveStepOutput checkpoints output while owner holds the lease.
func (r *Repository) SaveStepOutput(ctx context.Context, stepID kernel.ID, owner string, output json.RawMessage) (bool, error) {
	n, err := r.q(ctx).SaveStepOutput(ctx, SaveStepOutputParams{ID: stepID, LeaseOwner: &owner, OutputJson: output})
	return n == 1, err
}

// AppendEvent inserts an ai_job_events row.
func (r *Repository) AppendEvent(ctx context.Context, e *usecase.EventRecord) error {
	id, err := r.q(ctx).AppendEvent(ctx, AppendEventParams{
		JobID: e.JobID, StepID: e.StepID, EventType: e.Type, Payload: e.Payload, CreatedAt: e.CreatedAt,
	})
	e.ID = id
	return err
}

// ListEvents returns the newest limit events, oldest first.
func (r *Repository) ListEvents(ctx context.Context, jobID kernel.ID, limit int) ([]usecase.EventRecord, error) {
	rows, err := r.q(ctx).ListEvents(ctx, ListEventsParams{JobID: jobID, Lim: int32(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]usecase.EventRecord, len(rows))
	for i, e := range rows {
		out[i] = usecase.EventRecord{ID: e.ID, JobID: e.JobID, StepID: e.StepID, Type: e.EventType, Payload: e.Payload, CreatedAt: e.CreatedAt}
	}
	return out, nil
}

// InsertOutbox inserts an outbox row.
func (r *Repository) InsertOutbox(ctx context.Context, o *usecase.OutboxRecord) error {
	id, err := r.q(ctx).InsertOutbox(ctx, InsertOutboxParams{Topic: o.Topic, Payload: o.Payload, AvailableAt: o.AvailableAt, CreatedAt: o.CreatedAt})
	o.ID = id
	return err
}

// ClaimOutbox locks due rows (FOR UPDATE SKIP LOCKED).
func (r *Repository) ClaimOutbox(ctx context.Context, now time.Time, limit int) ([]usecase.OutboxRecord, error) {
	rows, err := r.q(ctx).ClaimOutbox(ctx, ClaimOutboxParams{Now: now, Lim: int32(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]usecase.OutboxRecord, len(rows))
	for i, o := range rows {
		out[i] = usecase.OutboxRecord{ID: o.ID, Topic: o.Topic, Payload: o.Payload, AvailableAt: o.AvailableAt, PublishedAt: o.PublishedAt, CreatedAt: o.CreatedAt}
	}
	return out, nil
}

// MarkOutboxPublished stamps published_at.
func (r *Repository) MarkOutboxPublished(ctx context.Context, ids []int64, at time.Time) error {
	return r.q(ctx).MarkOutboxPublished(ctx, MarkOutboxPublishedParams{Ids: ids, PublishedAt: &at})
}

// OldestUnpublishedOutbox returns the oldest due row's available_at (now when none).
func (r *Repository) OldestUnpublishedOutbox(ctx context.Context, now time.Time) (*time.Time, error) {
	t, err := r.q(ctx).OldestUnpublishedOutbox(ctx, now)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// DeletePublishedOutbox removes rows published before `before`.
func (r *Repository) DeletePublishedOutbox(ctx context.Context, before time.Time) (int64, error) {
	return r.q(ctx).DeletePublishedOutbox(ctx, &before)
}

// ListExpiredLeases returns running steps with an expired lease.
func (r *Repository) ListExpiredLeases(ctx context.Context, now time.Time, limit int) ([]usecase.StepRecord, error) {
	rows, err := r.q(ctx).ListExpiredLeases(ctx, ListExpiredLeasesParams{Now: &now, Lim: int32(limit)})
	if err != nil {
		return nil, err
	}
	return toSteps(rows), nil
}

// ListOverdueWaiting returns waiting_provider steps whose poll is overdue.
func (r *Repository) ListOverdueWaiting(ctx context.Context, before time.Time, limit int) ([]usecase.StepRecord, error) {
	rows, err := r.q(ctx).ListOverdueWaiting(ctx, ListOverdueWaitingParams{Before: &before, Lim: int32(limit)})
	if err != nil {
		return nil, err
	}
	return toSteps(rows), nil
}

// ListStaleQueued returns queued steps whose task seems lost.
func (r *Repository) ListStaleQueued(ctx context.Context, before time.Time, limit int) ([]usecase.StepRecord, error) {
	rows, err := r.q(ctx).ListStaleQueued(ctx, ListStaleQueuedParams{Before: &before, Lim: int32(limit)})
	if err != nil {
		return nil, err
	}
	return toSteps(rows), nil
}

// CountNeedsReconciliation counts flagged jobs.
func (r *Repository) CountNeedsReconciliation(ctx context.Context) (int, error) {
	n, err := r.q(ctx).CountNeedsReconciliation(ctx)
	return int(n), err
}

// TryLock takes a transaction-scoped advisory lock.
func (r *Repository) TryLock(ctx context.Context, key int64) (bool, error) {
	return r.q(ctx).TryAdvisoryXactLock(ctx, key)
}
