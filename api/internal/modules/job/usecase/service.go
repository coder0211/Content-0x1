package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"github.com/be/acs/api/internal/modules/job/domain"
	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
)

var knownJobTypes = map[JobType]bool{
	TypeContentGeneration: true, TypeContentRewrite: true, TypeContentTransform: true, TypeContentPlan: true,
	TypeImageGeneration: true, TypeImageEdit: true, TypeImagePromptSuggest: true,
	TypeVideoStoryboard: true, TypeVideoProduction: true, TypeVideoSceneRegenerate: true, TypeVideoRender: true,
}

// IsKnownJobType reports whether t is a valid ai_jobs.job_type.
func IsKnownJobType(t JobType) bool { return knownJobTypes[t] }

func validateSpec(spec JobSpec) error {
	var v kernel.ValidationErrors
	if !knownJobTypes[spec.Type] {
		v.Add("type", "invalid")
	}
	if spec.CreatedBy == kernel.NilID {
		v.Add("created_by", "required")
	}
	if len(spec.Request) > 0 && !json.Valid(spec.Request) {
		v.Add("request", "invalid_json")
	}
	nodes := make([]domain.Node, len(spec.Steps))
	for i, st := range spec.Steps {
		nodes[i] = domain.Node{Key: st.Key, Type: st.Type, DependsOn: st.DependsOn}
		if len(st.Input) > 0 && !json.Valid(st.Input) {
			v.Add("steps["+strconv.Itoa(i)+"].input", "invalid_json")
		}
	}
	if err := v.Err(); err != nil {
		return err
	}
	return domain.ValidateDAG(nodes)
}

// CheckCreateRate implements CreateRateLimiter (ACS-043).
func (s *Service) CheckCreateRate(ctx context.Context, userID kernel.ID) (context.Context, error) {
	if RateChecked(ctx) {
		return ctx, nil
	}
	if err := s.consumeCreateRate(ctx, userID); err != nil {
		return ctx, err
	}
	return WithRateChecked(ctx), nil
}

func (s *Service) consumeCreateRate(ctx context.Context, userID kernel.ID) error {
	if s.Limiter == nil {
		return nil
	}
	ok, retryAfter, err := s.Limiter.Allow(ctx, "jobcreate:"+userID.String(), CreateRateLimit, CreateRateLimitWindow)
	if err != nil {
		// Fail open: Redis trouble must not block every generation.
		s.log().WarnContext(ctx, "job create rate limiter unavailable", slog.String("error", err.Error()))
		return nil
	}
	if !ok {
		secs := int(retryAfter.Round(time.Second) / time.Second)
		return kernel.ErrRateLimited.WithDetails(kernel.Detail{"limit": CreateRateLimit, "window_seconds": 60, "retry_after_seconds": max(1, secs)})
	}
	return nil
}

func moneyPtr(m kernel.Money) *kernel.Money {
	if m.Currency == "" {
		return nil
	}
	return &m
}

// Create implements API (ACS-011/012/017).
func (s *Service) Create(ctx context.Context, spec JobSpec) (CreateResult, error) {
	if err := validateSpec(spec); err != nil {
		return CreateResult{}, err
	}
	var res CreateResult
	err := s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		if spec.IdempotencyKey != "" {
			if r, found, err := s.existing(ctx, spec); err != nil || found {
				res = r
				return err
			}
			if !RateChecked(ctx) {
				if err := s.consumeCreateRate(ctx, spec.CreatedBy); err != nil {
					return err
				}
			}
		}
		now := s.now()
		req := spec.Request
		if len(req) == 0 {
			req = json.RawMessage("{}")
		}
		j := &JobRecord{
			ID: kernel.NewID(), Type: spec.Type, Status: StatusQueued, CampaignID: spec.CampaignID, AssetID: spec.AssetID,
			BrandVersionID: spec.BrandVersionID, PromptTemplateID: spec.PromptTemplateID, ProviderCode: spec.ProviderCode,
			ModelCode: spec.ModelCode, Request: req, IdempotencyKey: spec.IdempotencyKey, RequestHash: spec.RequestHash,
			CostEstimate: moneyPtr(spec.CostEstimate), PricingVersion: spec.PricingVersion, BudgetOverrideBy: spec.BudgetOverrideBy,
			CreatedBy: spec.CreatedBy, CreatedAt: now,
		}
		inserted, err := s.Repo.InsertJob(ctx, j)
		if err != nil {
			return err
		}
		if !inserted { // concurrent request with the same key won the race
			r, found, err := s.existing(ctx, spec)
			if err == nil && !found {
				err = kernel.ErrIdempotencyConflict
			}
			res = r
			return err
		}
		steps := make([]StepRecord, len(spec.Steps))
		for i, sp := range spec.Steps {
			in := sp.Input
			if len(in) == 0 {
				in = json.RawMessage("{}")
			}
			deps := sp.DependsOn
			if deps == nil {
				deps = []string{}
			}
			steps[i] = StepRecord{
				ID: kernel.NewID(), JobID: j.ID, Type: sp.Type, Key: sp.Key, DependsOn: deps, SceneID: sp.SceneID,
				Status: StepPending, Input: in, MaxAttempts: sp.MaxAttempts,
				// Distinct timestamps keep creation order stable.
				CreatedAt: now.Add(time.Duration(i) * time.Microsecond),
			}
			if err := s.Repo.InsertStep(ctx, &steps[i]); err != nil {
				return err
			}
		}
		if err := s.appendEvent(ctx, j.ID, nil, EventCreated, map[string]any{"job_type": j.Type, "steps": len(steps)}); err != nil {
			return err
		}
		// SSE events of Create are dropped: nobody can subscribe to a job
		// that does not exist yet, and the caller owns the commit.
		if err := s.scheduleReady(ctx, &effects{}, steps); err != nil {
			return err
		}
		res = CreateResult{JobID: j.ID, Status: StatusQueued, CostEstimate: spec.CostEstimate}
		return nil
	})
	if err != nil {
		return CreateResult{}, err
	}
	return res, nil
}

// existing looks up (CreatedBy, IdempotencyKey): same hash → Existing.
func (s *Service) existing(ctx context.Context, spec JobSpec) (CreateResult, bool, error) {
	j, err := s.Repo.FindJobByIdempotency(ctx, spec.CreatedBy, spec.IdempotencyKey)
	if errors.Is(err, kernel.ErrNotFound) {
		return CreateResult{}, false, nil
	}
	if err != nil {
		return CreateResult{}, false, err
	}
	if j.RequestHash != spec.RequestHash {
		return CreateResult{}, true, kernel.ErrIdempotencyConflict
	}
	est := spec.CostEstimate
	if j.CostEstimate != nil {
		est = *j.CostEstimate
	}
	return CreateResult{JobID: j.ID, Status: j.Status, Existing: true, CostEstimate: est}, true, nil
}

func stepView(st StepRecord) StepView {
	return StepView{
		ID: st.ID, Key: st.Key, Type: st.Type, Status: st.Status, AttemptCount: st.AttemptCount,
		ErrorKind: st.ErrorKind, ErrorMessage: st.ErrorMessage, Output: st.Output,
		StartedAt: st.StartedAt, FinishedAt: st.FinishedAt,
	}
}

func jobView(j JobRecord, steps []StepRecord) JobView {
	v := JobView{
		ID: j.ID, Type: j.Type, Status: j.Status, Progress: j.Progress, CreatedBy: j.CreatedBy, CampaignID: j.CampaignID,
		AssetID: j.AssetID, BrandVersionID: j.BrandVersionID, PromptTemplateID: j.PromptTemplateID,
		ProviderCode: j.ProviderCode, ModelCode: j.ModelCode, Request: j.Request, Result: j.Result,
		CostEstimate: j.CostEstimate, CostActual: j.CostActual, CostSource: j.CostSource,
		ErrorCode: j.ErrorCode, ErrorMessage: j.ErrorMessage, CancelRequested: j.CancelRequested,
		Steps: make([]StepView, 0, len(steps)), CreatedAt: j.CreatedAt, StartedAt: j.StartedAt, FinishedAt: j.FinishedAt,
	}
	for _, st := range steps {
		v.Steps = append(v.Steps, stepView(st))
	}
	return v
}

// Get implements API (no authorization; for other modules).
func (s *Service) Get(ctx context.Context, id kernel.ID) (JobView, error) {
	j, err := s.Repo.GetJob(ctx, id)
	if err != nil {
		return JobView{}, err
	}
	steps, err := s.Repo.ListSteps(ctx, id)
	if err != nil {
		return JobView{}, err
	}
	return jobView(j, steps), nil
}

// canView: owner or provider.view (SPEC-04 §2.7).
func canView(actor kernel.Actor, j JobRecord) bool {
	return actor.IsAuthenticated() && (j.CreatedBy == actor.UserID || actor.Can(kernel.PermProviderView))
}

// canModify: owner or admin.
func canModify(actor kernel.Actor, j JobRecord) bool {
	return actor.IsAuthenticated() && (j.CreatedBy == actor.UserID || actor.IsAdmin())
}

// JobDetail is GET /jobs/{id}: the view plus the reconciliation flag.
type JobDetail struct {
	JobView
	NeedsReconciliation bool
}

// GetForActor returns a job the actor may see (else NOT_FOUND, anti-IDOR).
func (s *Service) GetForActor(ctx context.Context, actor kernel.Actor, id kernel.ID) (JobDetail, error) {
	if !actor.IsAuthenticated() {
		return JobDetail{}, kernel.ErrUnauthenticated
	}
	j, err := s.Repo.GetJob(ctx, id)
	if err != nil {
		return JobDetail{}, err
	}
	if !canView(actor, j) {
		return JobDetail{}, kernel.ErrNotFound
	}
	steps, err := s.Repo.ListSteps(ctx, id)
	if err != nil {
		return JobDetail{}, err
	}
	return JobDetail{JobView: jobView(j, steps), NeedsReconciliation: j.NeedsReconciliation}, nil
}

// JobSummary is a list row of GET /jobs.
type JobSummary struct {
	JobView
	NeedsReconciliation bool
}

// List implements GET /jobs. Without provider.view the actor only sees
// their own jobs (created_by is forced).
func (s *Service) List(ctx context.Context, actor kernel.Actor, f JobFilter, page kernel.PageRequest) (kernel.Page[JobSummary], error) {
	if !actor.IsAuthenticated() {
		return kernel.Page[JobSummary]{}, kernel.ErrUnauthenticated
	}
	if !actor.Can(kernel.PermProviderView) {
		uid := actor.UserID
		f.CreatedBy = &uid
	}
	page = page.Normalized()
	var cur kernel.TimeIDCursor
	has, err := kernel.DecodeCursor(page.Cursor, &cur)
	if err != nil {
		return kernel.Page[JobSummary]{}, err
	}
	var after *kernel.TimeIDCursor
	if has {
		after = &cur
	}
	rows, err := s.Repo.ListJobs(ctx, f, page.Limit+1, after)
	if err != nil {
		return kernel.Page[JobSummary]{}, err
	}
	items := make([]JobSummary, 0, len(rows))
	for _, j := range rows {
		items = append(items, JobSummary{JobView: jobView(j, nil), NeedsReconciliation: j.NeedsReconciliation})
	}
	return kernel.PageFromRows(items, page.Limit, func(v JobSummary) any {
		return kernel.TimeIDCursor{At: v.CreatedAt, ID: v.ID}
	}), nil
}

// History returns the ai_job_events log of a job the actor may see.
func (s *Service) History(ctx context.Context, actor kernel.Actor, id kernel.ID, limit int) ([]EventRecord, error) {
	if _, err := s.GetForActor(ctx, actor, id); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	return s.Repo.ListEvents(ctx, id, limit)
}

// RequestCancel implements API (ACS-016): owner or admin; 409 when the job
// already finished. Jobs with no running/waiting step are cancelled at once;
// otherwise the worker finishes the in-flight step and cancels the rest.
func (s *Service) RequestCancel(ctx context.Context, actor kernel.Actor, id kernel.ID) error {
	if !actor.IsAuthenticated() {
		return kernel.ErrUnauthenticated
	}
	return s.inTx(ctx, func(ctx context.Context, fx *effects) error {
		j, err := s.Repo.LockJob(ctx, id)
		if err != nil {
			return err
		}
		if !canModify(actor, j) {
			return kernel.ErrNotFound
		}
		if j.Status.IsTerminal() {
			return kernel.InvalidTransition(string(j.Status), string(StatusCancelled)).
				WithMessage("Tác vụ đã kết thúc, không thể hủy")
		}
		if j.CancelRequested {
			return nil
		}
		j.CancelRequested = true
		if err := s.Repo.UpdateJob(ctx, &j); err != nil {
			return err
		}
		if err := s.appendEvent(ctx, j.ID, nil, EventCancelRequested, map[string]any{"by": actor.UserID}); err != nil {
			return err
		}
		steps, err := s.Repo.ListSteps(ctx, id)
		if err != nil {
			return err
		}
		inFlight := false
		now := s.now()
		for i := range steps {
			switch steps[i].Status {
			case StepRunning:
				inFlight = true
			case StepWaitingProvider:
				inFlight = true
				// Poll now so the handler can cancel at the provider.
				if err := s.enqueue(ctx, &steps[i], TaskPoll, steps[i].AttemptCount, now); err != nil {
					return err
				}
			}
		}
		if inFlight {
			fx.publish(j.ID, ports.JobEventStatus, map[string]any{"job_id": j.ID, "status": j.Status, "progress": j.Progress, "cancel_requested": true})
			return nil
		}
		return s.finalize(ctx, fx, &j, steps, StatusCancelled, CodeCancelled, "Tác vụ đã bị hủy")
	})
}

// Retry re-opens a failed job (ACS-016): only failed/cancelled/skipped
// steps run again; succeeded steps keep their output. Owner or admin; the
// error must be retryable unless an admin passes force. OnRetried hooks run
// in the same transaction (new budget reservation) and may abort it.
func (s *Service) Retry(ctx context.Context, actor kernel.Actor, id kernel.ID, force bool) (CreateResult, error) {
	if !actor.IsAuthenticated() {
		return CreateResult{}, kernel.ErrUnauthenticated
	}
	var res CreateResult
	err := s.inTx(ctx, func(ctx context.Context, fx *effects) error {
		j, err := s.Repo.LockJob(ctx, id)
		if err != nil {
			return err
		}
		if !canModify(actor, j) {
			return kernel.ErrNotFound
		}
		if j.Status != StatusFailed {
			return kernel.InvalidTransition(string(j.Status), string(StatusQueued)).
				WithMessage("Chỉ có thể chạy lại tác vụ đã thất bại")
		}
		if force && !actor.IsAdmin() {
			return kernel.ErrForbidden
		}
		if !force && !IsRetryableCode(j.ErrorCode) {
			return kernel.InvalidTransition(string(j.Status), string(StatusQueued)).
				WithMessage("Lỗi này không thể chạy lại tự động").WithDetails(kernel.Detail{"error_code": j.ErrorCode})
		}
		steps, err := s.Repo.ListSteps(ctx, id)
		if err != nil {
			return err
		}
		prevCode := j.ErrorCode
		j.ErrorCode, j.ErrorMessage, j.CancelRequested = "", "", false
		j.Progress = domain.Progress(states(steps), 0)
		if err := s.setJobStatus(ctx, fx, &j, StatusQueued); err != nil {
			return err
		}
		reset := 0
		for i := range steps {
			st := &steps[i]
			if st.Status == StepFailed || st.Status == StepCancelled || st.Status == StepSkipped {
				st.AttemptCount, st.ErrorKind, st.ErrorMessage, st.NextAttemptAt = 0, "", "", nil
				st.ProviderJobID = ""
				st.Generation++
				if err := s.setStepStatus(ctx, fx, st, StepPending); err != nil {
					return err
				}
				reset++
			}
		}
		if err := s.appendEvent(ctx, j.ID, nil, EventRetried, map[string]any{"by": actor.UserID, "force": force, "previous_error": prevCode, "steps": reset}); err != nil {
			return err
		}
		est := kernel.ZeroUSD()
		if j.CostEstimate != nil {
			est = *j.CostEstimate
		}
		ev := JobRetried{JobID: j.ID, Type: j.Type, CreatedBy: j.CreatedBy, CampaignID: j.CampaignID, CostEstimate: est, Actor: actor}
		s.mu.RLock()
		hooks := append([]func(context.Context, JobRetried) error(nil), s.retried...)
		s.mu.RUnlock()
		for _, h := range hooks {
			if err := h(ctx, ev); err != nil {
				return err
			}
		}
		if err := s.scheduleReady(ctx, fx, steps); err != nil {
			return err
		}
		res = CreateResult{JobID: j.ID, Status: StatusQueued, CostEstimate: est}
		return nil
	})
	return res, err
}
