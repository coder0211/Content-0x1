package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"maps"
	"strings"
	"time"

	"github.com/be/acs/api/internal/modules/job/domain"
	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
)

// claim is the state of one leased step attempt.
type claim struct {
	job     JobRecord
	step    StepRecord
	steps   []StepRecord
	owner   string
	attempt int
	handler StepHandler
	deps    map[string]json.RawMessage
}

// Execute runs one step task (ACS-013). It never returns an error for
// stale or duplicate tasks; an error means infrastructure trouble (the
// reconciler recovers the step through its lease).
func (s *Service) Execute(ctx context.Context, p TaskPayload) error {
	st, err := s.Repo.GetStep(ctx, p.StepID)
	if errors.Is(err, kernel.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if stale(st, p) {
		return nil
	}
	h, ok := s.Steps.Lookup(st.Type)
	if !ok {
		return s.failUnclaimed(ctx, p, CodeStepTypeUnknown, "unknown step type "+st.Type)
	}
	j, err := s.Repo.GetJob(ctx, p.JobID)
	if err != nil {
		return err
	}
	if j.Status.IsTerminal() {
		return nil
	}
	if j.CancelRequested {
		return s.cancelAtClaim(ctx, p, h)
	}
	release, acquired, err := s.acquire(ctx, j, st, h, p)
	if err != nil {
		return err
	}
	if !acquired {
		return s.deferStep(ctx, st, p)
	}
	defer release()

	c, ok, err := s.claim(ctx, p, h)
	if err != nil || !ok {
		return err
	}
	return s.run(ctx, c)
}

// stale reports whether a task no longer matches the step's state.
func stale(st StepRecord, p TaskPayload) bool {
	if p.Gen != st.Generation {
		return true
	}
	switch p.Kind {
	case TaskPoll:
		return st.Status != StepWaitingProvider || p.Attempt != st.AttemptCount
	default:
		return st.Status != StepQueued || p.Attempt != st.AttemptCount+1
	}
}

// acquire takes the concurrency slots of ACS-043: provider:model
// (ai_models.concurrency_limit, held for the attempt) and, for the video
// queue, the per-user cap of concurrent video jobs (held until the job
// finishes). Polls only refresh the user slot.
func (s *Service) acquire(ctx context.Context, j JobRecord, st StepRecord, h StepHandler, p TaskPayload) (func(), bool, error) {
	noop := func() {}
	if s.Sem == nil {
		return noop, true, nil
	}
	if h.Queue() == QueueVideo {
		limit := s.UserVideoLimit
		if limit <= 0 {
			limit = DefaultUserVideoLimit
		}
		ok, err := s.Sem.TryAcquire(ctx, userVideoKey(j.CreatedBy), j.ID.String(), limit, durOr(s.MaxProviderWait, DefaultMaxProviderWait)+5*time.Minute)
		if err != nil {
			s.log().WarnContext(ctx, "user semaphore unavailable", slog.String("error", err.Error()))
		} else if !ok && p.Kind != TaskPoll {
			return noop, false, nil
		}
	}
	if p.Kind == TaskPoll || s.ConcurrencyLimit == nil || j.ProviderCode == "" || j.ModelCode == "" {
		return noop, true, nil
	}
	if q := h.Queue(); q != QueueText && q != QueueImage && q != QueueVideo {
		return noop, true, nil
	}
	limit, err := s.ConcurrencyLimit(ctx, h.Queue(), j.ProviderCode, j.ModelCode)
	if err != nil || limit <= 0 {
		return noop, true, nil //nolint:nilerr // unknown limit: do not block
	}
	key := "model:" + j.ProviderCode + ":" + j.ModelCode
	holder := st.ID.String()
	ok, err := s.Sem.TryAcquire(ctx, key, holder, limit, h.Timeout()+time.Minute)
	if err != nil {
		s.log().WarnContext(ctx, "model semaphore unavailable", slog.String("error", err.Error()))
		return noop, true, nil
	}
	if !ok {
		return noop, false, nil
	}
	return func() { _ = s.Sem.Release(context.WithoutCancel(ctx), key, holder) }, true, nil
}

// deferStep re-dispatches the same attempt later: over cap → wait, not fail.
func (s *Service) deferStep(ctx context.Context, st StepRecord, p TaskPayload) error {
	return s.inTx(ctx, func(ctx context.Context, _ *effects) error {
		wait := durOr(s.ConcurrencyWait, DefaultConcurrencyWait)
		at := s.now().Add(wait + time.Duration(s.rnd()*float64(time.Second)))
		if p.Kind != TaskPoll {
			// Keep next_attempt_at current so the reconciler does not
			// treat a long concurrency wait as a lost task.
			cur, err := s.Repo.GetStep(ctx, st.ID)
			if err != nil {
				return err
			}
			if stale(cur, p) {
				return nil
			}
			cur.NextAttemptAt = &at
			if err := s.Repo.UpdateStep(ctx, &cur); err != nil {
				return err
			}
		}
		if p.Kind == TaskRun {
			id := st.ID
			if err := s.appendEvent(ctx, st.JobID, &id, EventConcurrencyWait, map[string]any{"step_key": st.Key}); err != nil {
				return err
			}
		}
		return s.enqueue(ctx, &st, TaskDefer, p.Attempt, at)
	})
}

// claim leases the step in a transaction (job row locked first).
func (s *Service) claim(ctx context.Context, p TaskPayload, h StepHandler) (*claim, bool, error) {
	var c *claim
	err := s.inTx(ctx, func(ctx context.Context, fx *effects) error {
		j, err := s.Repo.LockJob(ctx, p.JobID)
		if err != nil {
			return err
		}
		st, err := s.Repo.GetStep(ctx, p.StepID)
		if err != nil {
			return err
		}
		if j.Status.IsTerminal() || j.CancelRequested || stale(st, p) {
			return nil
		}
		now := s.now()
		lease := now.Add(durOr(s.LeaseTTL, DefaultLeaseTTL))
		if p.Kind != TaskPoll {
			st.AttemptCount++
		}
		st.NextAttemptAt = nil
		if err := s.setStepStatus(ctx, fx, &st, StepRunning); err != nil {
			return err
		}
		st.LeaseOwner, st.LeaseExpiresAt = s.workerID()+"/"+kernel.NewID().String()[:8], &lease
		if err := s.Repo.UpdateStep(ctx, &st); err != nil {
			return err
		}
		if j.Status == StatusQueued {
			if err := s.setJobStatus(ctx, fx, &j, StatusRunning); err != nil {
				return err
			}
		}
		steps, err := s.Repo.ListSteps(ctx, j.ID)
		if err != nil {
			return err
		}
		deps := map[string]json.RawMessage{}
		for _, o := range steps {
			for _, d := range st.DependsOn {
				if o.Key == d {
					deps[d] = o.Output
				}
			}
		}
		c = &claim{job: j, step: st, steps: steps, owner: st.LeaseOwner, attempt: st.AttemptCount, handler: h, deps: deps}
		return nil
	})
	return c, c != nil, err
}

// run executes the handler with timeout + heartbeat and records the outcome.
func (s *Service) run(ctx context.Context, c *claim) error {
	runCtx, cancel := context.WithTimeout(ctx, c.handler.Timeout())
	defer cancel()
	lost := make(chan struct{})
	hbDone := make(chan struct{})
	go func() {
		defer close(hbDone)
		t := time.NewTicker(durOr(s.Heartbeat, DefaultHeartbeat))
		defer t.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-t.C:
				ok, err := s.Repo.ExtendLease(context.WithoutCancel(ctx), c.step.ID, c.owner, s.now().Add(durOr(s.LeaseTTL, DefaultLeaseTTL)))
				if err == nil && !ok {
					close(lost)
					cancel()
					return
				}
			}
		}
	}()
	start := time.Now()
	sc := &stepContext{s: s, c: c}
	res, herr := safeHandle(runCtx, c.handler, sc)
	timedOut := errors.Is(runCtx.Err(), context.DeadlineExceeded)
	cancel()
	<-hbDone
	if s.Metrics != nil {
		s.Metrics.StepDuration(c.step.Type, c.job.ProviderCode, time.Since(start))
	}
	select {
	case <-lost:
		s.log().WarnContext(ctx, "step lease lost; result dropped", slog.String("step_id", c.step.ID.String()))
		return nil
	default:
	}
	return s.complete(context.WithoutCancel(ctx), c, res, herr, timedOut)
}

func safeHandle(ctx context.Context, h StepHandler, sc StepContext) (res StepResult, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = Permanent(errors.New("step handler panic"))
		}
	}()
	return h.Handle(ctx, sc)
}

// complete records the outcome of an attempt in one transaction.
func (s *Service) complete(ctx context.Context, c *claim, res StepResult, herr error, timedOut bool) error {
	return s.inTx(ctx, func(ctx context.Context, fx *effects) error {
		j, err := s.Repo.LockJob(ctx, c.job.ID)
		if err != nil {
			return err
		}
		st, err := s.Repo.GetStep(ctx, c.step.ID)
		if err != nil {
			return err
		}
		if st.Status != StepRunning || st.LeaseOwner != c.owner || st.AttemptCount != c.attempt {
			s.log().WarnContext(ctx, "step no longer leased by this worker; result dropped", slog.String("step_id", st.ID.String()))
			return nil
		}
		steps, err := s.Repo.ListSteps(ctx, j.ID)
		if err != nil {
			return err
		}
		idx := indexOf(steps, st.ID)
		now := s.now()
		if herr == nil {
			if res.Output != nil {
				b, err := json.Marshal(res.Output)
				if err != nil {
					return err
				}
				st.Output = b
			}
			if len(res.JobResult) > 0 {
				merged := map[string]any{}
				if len(j.Result) > 0 {
					_ = json.Unmarshal(j.Result, &merged)
				}
				maps.Copy(merged, res.JobResult)
				b, err := json.Marshal(merged)
				if err != nil {
					return err
				}
				j.Result = b
				if err := s.Repo.UpdateJob(ctx, &j); err != nil {
					return err
				}
			}
			if res.Wait != nil {
				return s.park(ctx, fx, &j, &st, res.Wait)
			}
			st.ErrorKind, st.ErrorMessage = "", ""
			if err := s.setStepStatus(ctx, fx, &st, StepSucceeded); err != nil {
				return err
			}
			steps[idx] = st
			switch {
			case j.CancelRequested:
				if hasInFlight(steps) {
					return nil
				}
				return s.finalize(ctx, fx, &j, steps, StatusCancelled, CodeCancelled, "Tác vụ đã bị hủy")
			case domain.AllDone(states(steps)):
				return s.finalize(ctx, fx, &j, steps, StatusSucceeded, "", "")
			}
			if err := s.scheduleReady(ctx, fx, steps); err != nil {
				return err
			}
			return s.updateProgress(ctx, fx, &j, steps, 0)
		}

		f := classify(herr, timedOut)
		st.ErrorKind, st.ErrorMessage = f.kind, f.detail
		id := st.ID
		if f.provider {
			if s.Metrics != nil {
				s.Metrics.ProviderError(j.ProviderCode, f.kind)
			}
			if err := s.appendEvent(ctx, j.ID, &id, EventProviderError, map[string]any{"step_key": st.Key, "kind": f.kind, "attempt": st.AttemptCount}); err != nil {
				return err
			}
		}
		if f.retryable && st.AttemptCount < maxAttempts(st, c.handler) && !j.CancelRequested {
			delay := domain.Backoff(st.AttemptCount, s.rnd())
			if f.retryAfter > delay {
				delay = f.retryAfter
			}
			at := now.Add(delay)
			st.NextAttemptAt = &at
			if err := s.setStepStatus(ctx, fx, &st, StepQueued); err != nil {
				return err
			}
			if err := s.appendEvent(ctx, j.ID, &id, EventRetryScheduled, map[string]any{
				"step_key": st.Key, "attempt": st.AttemptCount, "next_attempt_at": at, "error_kind": f.kind,
			}); err != nil {
				return err
			}
			return s.enqueue(ctx, &st, TaskRun, st.AttemptCount+1, at)
		}
		// Dead letter: the step and the job fail.
		if err := s.setStepStatus(ctx, fx, &st, StepFailed); err != nil {
			return err
		}
		steps[idx] = st
		if j.CancelRequested {
			return s.finalize(ctx, fx, &j, steps, StatusCancelled, CodeCancelled, "Tác vụ đã bị hủy")
		}
		if f.kind == "Timeout" && st.ProviderJobID != "" {
			j.NeedsReconciliation = true
		}
		s.log().WarnContext(ctx, "job step failed", slog.String("job_id", j.ID.String()), slog.String("step_id", st.ID.String()),
			slog.String("error_kind", f.kind), slog.String("error", f.detail))
		return s.finalize(ctx, fx, &j, steps, StatusFailed, f.code, f.message)
	})
}

// park moves a step to waiting_provider and schedules the next poll.
func (s *Service) park(ctx context.Context, fx *effects, j *JobRecord, st *StepRecord, w *WaitSpec) error {
	if w.ProviderJobID != "" {
		st.ProviderJobID = w.ProviderJobID
	}
	poll := w.PollAfter
	if poll <= 0 {
		poll = DefaultPollAfter
	}
	if j.CancelRequested {
		poll = 0
	}
	at := s.now().Add(poll)
	st.NextAttemptAt = &at
	if err := s.setStepStatus(ctx, fx, st, StepWaitingProvider); err != nil {
		return err
	}
	return s.enqueue(ctx, st, TaskPoll, st.AttemptCount, at)
}

func (s *Service) updateProgress(ctx context.Context, fx *effects, j *JobRecord, steps []StepRecord, runningPct int) error {
	p := domain.Progress(states(steps), runningPct)
	if p <= j.Progress {
		return nil
	}
	if err := s.Repo.RaiseJobProgress(ctx, j.ID, p); err != nil {
		return err
	}
	j.Progress = p
	fx.publish(j.ID, ports.JobEventProgress, map[string]any{"job_id": j.ID, "status": j.Status, "progress": p})
	return nil
}

func indexOf(steps []StepRecord, id kernel.ID) int {
	for i := range steps {
		if steps[i].ID == id {
			return i
		}
	}
	return 0
}

func hasInFlight(steps []StepRecord) bool {
	for _, st := range steps {
		if st.Status == StepRunning || st.Status == StepWaitingProvider {
			return true
		}
	}
	return false
}

// cancelAtClaim handles a task of a job whose cancellation was requested:
// the step is cancelled (OnCancel first for waiting provider jobs) and the
// job is finalized once nothing is in flight.
func (s *Service) cancelAtClaim(ctx context.Context, p TaskPayload, h StepHandler) error {
	var toCancel *claim
	err := s.inTx(ctx, func(ctx context.Context, fx *effects) error {
		j, err := s.Repo.LockJob(ctx, p.JobID)
		if err != nil {
			return err
		}
		st, err := s.Repo.GetStep(ctx, p.StepID)
		if err != nil {
			return err
		}
		if j.Status.IsTerminal() || stale(st, p) {
			return nil
		}
		steps, err := s.Repo.ListSteps(ctx, j.ID)
		if err != nil {
			return err
		}
		if st.Status == StepWaitingProvider && st.ProviderJobID != "" {
			if _, ok := h.(CancelHandler); ok {
				toCancel = &claim{job: j, step: st, steps: steps, handler: h, attempt: st.AttemptCount}
			}
		}
		if err := s.setStepStatus(ctx, fx, &st, StepCancelled); err != nil {
			return err
		}
		steps[indexOf(steps, st.ID)] = st
		if hasInFlight(steps) {
			return nil
		}
		return s.finalize(ctx, fx, &j, steps, StatusCancelled, CodeCancelled, "Tác vụ đã bị hủy")
	})
	if err == nil && toCancel != nil {
		cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
		defer cancel()
		if cerr := h.(CancelHandler).OnCancel(cctx, &stepContext{s: s, c: toCancel}); cerr != nil {
			s.log().WarnContext(ctx, "provider cancel failed", slog.String("step_id", p.StepID.String()), slog.String("error", cerr.Error()))
		}
	}
	return err
}

// failUnclaimed fails a step that cannot run at all (unknown step type).
func (s *Service) failUnclaimed(ctx context.Context, p TaskPayload, code, detail string) error {
	return s.inTx(ctx, func(ctx context.Context, fx *effects) error {
		j, err := s.Repo.LockJob(ctx, p.JobID)
		if err != nil {
			return err
		}
		st, err := s.Repo.GetStep(ctx, p.StepID)
		if err != nil {
			return err
		}
		if j.Status.IsTerminal() || StepStatusTerminal(st.Status) {
			return nil
		}
		st.ErrorKind, st.ErrorMessage = "Permanent", detail
		if err := s.setStepStatus(ctx, fx, &st, StepFailed); err != nil {
			return err
		}
		steps, err := s.Repo.ListSteps(ctx, j.ID)
		if err != nil {
			return err
		}
		steps[indexOf(steps, st.ID)] = st
		return s.finalize(ctx, fx, &j, steps, StatusFailed, code, "Tác vụ xử lý thất bại")
	})
}

// stepContext implements StepContext for one leased attempt.
type stepContext struct {
	s *Service
	c *claim
}

var _ StepContext = (*stepContext)(nil)

func (x *stepContext) Job() JobInfo {
	j := x.c.job
	return JobInfo{
		ID: j.ID, Type: j.Type, CreatedBy: j.CreatedBy, CampaignID: j.CampaignID, AssetID: j.AssetID,
		BrandVersionID: j.BrandVersionID, PromptTemplateID: j.PromptTemplateID, ProviderCode: j.ProviderCode,
		ModelCode: j.ModelCode, Request: j.Request,
	}
}
func (x *stepContext) StepID() kernel.ID      { return x.c.step.ID }
func (x *stepContext) StepKey() string        { return x.c.step.Key }
func (x *stepContext) StepType() string       { return x.c.step.Type }
func (x *stepContext) SceneID() *kernel.ID    { return x.c.step.SceneID }
func (x *stepContext) Attempt() int           { return max(1, x.c.attempt) }
func (x *stepContext) Input() json.RawMessage { return x.c.step.Input }
func (x *stepContext) ProviderJobID() string  { return x.c.step.ProviderJobID }

func (x *stepContext) DecodeInput(v any) error {
	in := x.c.step.Input
	if len(in) == 0 {
		return nil
	}
	return json.Unmarshal(in, v)
}

func (x *stepContext) DependencyOutputs() map[string]json.RawMessage { return maps.Clone(x.c.deps) }

// SavedOutput returns the checkpointed output of a previous attempt.
func (x *stepContext) SavedOutput() json.RawMessage { return x.c.step.Output }

func (x *stepContext) SaveOutput(ctx context.Context, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	ok, err := x.s.Repo.SaveStepOutput(ctx, x.c.step.ID, x.c.owner, b)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("step lease lost")
	}
	x.c.step.Output = b
	return nil
}

func (x *stepContext) Progress(ctx context.Context, pct int, message string) error {
	p := domain.Progress(states(x.c.steps), pct)
	if err := x.s.Repo.RaiseJobProgress(ctx, x.c.job.ID, p); err != nil {
		return err
	}
	if x.s.Events != nil {
		data, _ := json.Marshal(map[string]any{
			"job_id": x.c.job.ID, "status": StatusRunning, "progress": p, "step_key": x.c.step.Key,
			"step_progress": pct, "message": strings.TrimSpace(message),
		})
		_ = x.s.Events.Publish(ctx, ports.JobEvent{JobID: x.c.job.ID, Type: ports.JobEventProgress, Data: data})
	}
	return nil
}

func (x *stepContext) CancelRequested(ctx context.Context) (bool, error) {
	j, err := x.s.Repo.GetJob(ctx, x.c.job.ID)
	if err != nil {
		return false, err
	}
	return j.CancelRequested, nil
}
