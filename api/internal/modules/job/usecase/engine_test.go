package usecase_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/be/acs/api/internal/modules/job/jobtest"
	"github.com/be/acs/api/internal/modules/job/usecase"
	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
	"github.com/be/acs/api/internal/testutil"
)

// --- fakes -----------------------------------------------------------------

type fakeQueue struct {
	mu    sync.Mutex
	tasks []usecase.Task
	ids   map[string]bool
	fail  atomic.Bool
}

func (q *fakeQueue) Enqueue(_ context.Context, t usecase.Task) error {
	if q.fail.Load() {
		return errors.New("redis down")
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.ids == nil {
		q.ids = map[string]bool{}
	}
	if q.ids[t.ID] {
		return nil // duplicate task id: already queued
	}
	q.ids[t.ID] = true
	q.tasks = append(q.tasks, t)
	return nil
}

func (q *fakeQueue) take() []usecase.Task {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := q.tasks
	q.tasks = nil
	return out
}

type handler struct {
	typ     string
	queue   usecase.Queue
	timeout time.Duration
	calls   atomic.Int32
	fn      func(ctx context.Context, sc usecase.StepContext, call int) (usecase.StepResult, error)
	cancels atomic.Int32
}

func (h *handler) StepType() string { return h.typ }
func (h *handler) Queue() usecase.Queue {
	if h.queue == "" {
		return usecase.QueueText
	}
	return h.queue
}
func (h *handler) Timeout() time.Duration {
	if h.timeout == 0 {
		return 5 * time.Second
	}
	return h.timeout
}
func (h *handler) Handle(ctx context.Context, sc usecase.StepContext) (usecase.StepResult, error) {
	n := int(h.calls.Add(1))
	if h.fn == nil {
		return usecase.StepResult{Output: map[string]any{"ok": true}}, nil
	}
	return h.fn(ctx, sc, n)
}

type cancelHandler struct{ *handler }

func (c cancelHandler) OnCancel(context.Context, usecase.StepContext) error {
	c.cancels.Add(1)
	return nil
}

type fakeSem struct {
	mu      sync.Mutex
	deny    int // deny the next n acquisitions of model keys
	holders map[string]map[string]bool
}

func (s *fakeSem) TryAcquire(_ context.Context, key, holder string, limit int, _ time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.holders == nil {
		s.holders = map[string]map[string]bool{}
	}
	if s.deny > 0 && key[:6] == "model:" {
		s.deny--
		return false, nil
	}
	h := s.holders[key]
	if h == nil {
		h = map[string]bool{}
		s.holders[key] = h
	}
	if h[holder] || len(h) < limit {
		h[holder] = true
		return true, nil
	}
	return false, nil
}

func (s *fakeSem) Release(_ context.Context, key, holder string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.holders[key], holder)
	return nil
}

type fakeLimiter struct {
	mu     sync.Mutex
	counts map[string]int
}

func (l *fakeLimiter) Allow(_ context.Context, key string, limit int, _ time.Duration) (bool, time.Duration, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.counts == nil {
		l.counts = map[string]int{}
	}
	l.counts[key]++
	if l.counts[key] > limit {
		return false, 30 * time.Second, nil
	}
	return true, 0, nil
}

type metricsRec struct {
	mu   sync.Mutex
	lags []float64
}

func (m *metricsRec) OutboxLag(s float64)                        { m.mu.Lock(); m.lags = append(m.lags, s); m.mu.Unlock() }
func (m *metricsRec) JobFinished(string, string)                 {}
func (m *metricsRec) StepDuration(string, string, time.Duration) {}
func (m *metricsRec) ProviderError(string, string)               {}

// --- harness ---------------------------------------------------------------

type env struct {
	t        *testing.T
	svc      *usecase.Service
	mem      *jobtest.Memory
	q        *fakeQueue
	clock    *testutil.Clock
	events   *testutil.Events
	notifier *testutil.Notifier
	metrics  *metricsRec
}

func newEnv(t *testing.T, hs ...usecase.StepHandler) *env {
	t.Helper()
	mem := jobtest.NewMemory()
	reg := usecase.NewStepRegistry()
	for _, h := range hs {
		if err := reg.Register(h); err != nil {
			t.Fatal(err)
		}
	}
	e := &env{t: t, mem: mem, q: &fakeQueue{}, clock: testutil.NewClock(), events: &testutil.Events{}, notifier: &testutil.Notifier{}, metrics: &metricsRec{}}
	e.svc = &usecase.Service{
		Tx: mem, Repo: mem, Clock: e.clock, Steps: reg, Events: e.events, Notifier: e.notifier, Queue: e.q,
		Metrics: e.metrics, WorkerID: "test", Rand: func() float64 { return 0.5 },
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	return e
}

var userA = kernel.NewID()

func spec(steps ...usecase.StepSpec) usecase.JobSpec {
	return usecase.JobSpec{
		Type: usecase.TypeContentGeneration, CreatedBy: userA, ProviderCode: "fake", ModelCode: "fake-text",
		Request: json.RawMessage(`{"brief":"x"}`), CostEstimate: kernel.USD("0.10"), Steps: steps,
	}
}

func (e *env) create(s usecase.JobSpec) kernel.ID {
	e.t.Helper()
	res, err := e.svc.Create(context.Background(), s)
	if err != nil {
		e.t.Fatal(err)
	}
	return res.JobID
}

// round dispatches due outbox rows and executes the resulting tasks
// (concurrently when parallel). It returns the number of tasks run.
func (e *env) round(parallel bool) int {
	e.t.Helper()
	ctx := context.Background()
	if _, err := e.svc.DispatchOnce(ctx); err != nil {
		e.t.Fatal(err)
	}
	tasks := e.q.take()
	var wg sync.WaitGroup
	for _, task := range tasks {
		var p usecase.TaskPayload
		if err := json.Unmarshal(task.Payload, &p); err != nil {
			e.t.Fatal(err)
		}
		run := func() {
			if err := e.svc.Execute(ctx, p); err != nil {
				e.t.Errorf("execute %s: %v", task.ID, err)
			}
		}
		if parallel {
			wg.Add(1)
			go func() { defer wg.Done(); run() }()
		} else {
			run()
		}
	}
	wg.Wait()
	return len(tasks)
}

// drain runs rounds until nothing is left, advancing the fake clock to the
// next delayed outbox row (backoff, polls, concurrency waits).
func (e *env) drain(parallel bool) {
	e.t.Helper()
	for i := 0; i < 200; i++ {
		if e.round(parallel) > 0 {
			continue
		}
		var next *time.Time
		for _, o := range e.mem.Outbox() {
			if o.PublishedAt == nil && (next == nil || o.AvailableAt.Before(*next)) {
				at := o.AvailableAt
				next = &at
			}
		}
		if next == nil {
			return
		}
		e.clock.Advance(next.Sub(e.clock.Now()) + time.Millisecond)
	}
	e.t.Fatal("drain did not converge")
}

func (e *env) job(id kernel.ID) usecase.JobView {
	e.t.Helper()
	v, err := e.svc.Get(context.Background(), id)
	if err != nil {
		e.t.Fatal(err)
	}
	return v
}

func step(v usecase.JobView, key string) usecase.StepView {
	for _, s := range v.Steps {
		if s.Key == key {
			return s
		}
	}
	return usecase.StepView{}
}

func transient() error {
	return &ports.ProviderError{Kind: ports.ErrKindTransient, Provider: "fake", Message: "503"}
}

// --- tests -----------------------------------------------------------------

func TestTransientTwiceThenSucceeds(t *testing.T) {
	h := &handler{typ: "gen", fn: func(_ context.Context, _ usecase.StepContext, n int) (usecase.StepResult, error) {
		if n <= 2 {
			return usecase.StepResult{}, transient()
		}
		return usecase.StepResult{Output: map[string]any{"text": "ok"}, JobResult: map[string]any{"asset_ids": []string{"a1"}}}, nil
	}}
	e := newEnv(t, h)
	id := e.create(spec(usecase.StepSpec{Key: "gen", Type: "gen"}))
	start := e.clock.Now()
	e.drain(false)
	v := e.job(id)
	if v.Status != usecase.StatusSucceeded || v.Progress != 100 {
		t.Fatalf("status=%s progress=%d", v.Status, v.Progress)
	}
	if s := step(v, "gen"); s.Status != usecase.StepSucceeded || s.AttemptCount != 3 {
		t.Fatalf("step %+v", s)
	}
	if string(v.Result) != `{"asset_ids":["a1"]}` {
		t.Fatalf("result %s", v.Result)
	}
	// Backoff: 2^1*2s+0.5s then 2^2*2s+0.5s.
	if el := e.clock.Now().Sub(start); el < 12*time.Second {
		t.Fatalf("backoff too short: %s", el)
	}
	retries := 0
	for _, ev := range e.mem.Events() {
		if ev.Type == usecase.EventRetryScheduled {
			retries++
		}
	}
	if retries != 2 {
		t.Fatalf("retry events = %d", retries)
	}
	if len(e.notifier.Sent) != 1 || e.notifier.Sent[0].Type != ports.NotifyJobSucceeded {
		t.Fatalf("notifications %+v", e.notifier.Sent)
	}
}

func TestInvalidRequestFailsImmediately(t *testing.T) {
	h := &handler{typ: "gen", fn: func(context.Context, usecase.StepContext, int) (usecase.StepResult, error) {
		return usecase.StepResult{}, &ports.ProviderError{Kind: ports.ErrKindInvalidRequest, Provider: "fake"}
	}}
	e := newEnv(t, h)
	var finished []usecase.JobFinished
	e.svc.OnFinished(func(_ context.Context, ev usecase.JobFinished) error { finished = append(finished, ev); return nil })
	id := e.create(spec(usecase.StepSpec{Key: "gen", Type: "gen"}, usecase.StepSpec{Key: "after", Type: "gen", DependsOn: []string{"gen"}}))
	e.drain(false)
	v := e.job(id)
	if v.Status != usecase.StatusFailed || v.ErrorCode != "PROVIDER_INVALID_REQUEST" || v.ErrorMessage == "" {
		t.Fatalf("job %s %s %q", v.Status, v.ErrorCode, v.ErrorMessage)
	}
	if h.calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1 (no retry)", h.calls.Load())
	}
	if s := step(v, "gen"); s.Status != usecase.StepFailed || s.AttemptCount != 1 || s.ErrorKind != "InvalidRequest" {
		t.Fatalf("step %+v", s)
	}
	if s := step(v, "after"); s.Status != usecase.StepCancelled {
		t.Fatalf("dependent %+v", s)
	}
	if len(finished) != 1 || finished[0].Status != usecase.StatusFailed {
		t.Fatalf("OnFinished %+v", finished)
	}
}

func TestPermanentAndDomainErrorsAreNotRetried(t *testing.T) {
	for name, err := range map[string]error{
		"permanent": usecase.Permanent(errors.New("bad input")),
		"domain":    kernel.ErrBudgetExceeded,
		"policy":    &ports.ProviderError{Kind: ports.ErrKindContentPolicy},
	} {
		t.Run(name, func(t *testing.T) {
			h := &handler{typ: "gen", fn: func(context.Context, usecase.StepContext, int) (usecase.StepResult, error) {
				return usecase.StepResult{}, err
			}}
			e := newEnv(t, h)
			id := e.create(spec(usecase.StepSpec{Key: "gen", Type: "gen"}))
			e.drain(false)
			if v := e.job(id); v.Status != usecase.StatusFailed || h.calls.Load() != 1 {
				t.Fatalf("status %s calls %d", v.Status, h.calls.Load())
			}
		})
	}
}

func TestTransientExhaustsAttempts(t *testing.T) {
	h := &handler{typ: "gen", fn: func(context.Context, usecase.StepContext, int) (usecase.StepResult, error) {
		return usecase.StepResult{}, transient()
	}}
	e := newEnv(t, h)
	id := e.create(spec(usecase.StepSpec{Key: "gen", Type: "gen"}))
	e.drain(false)
	v := e.job(id)
	if v.Status != usecase.StatusFailed || h.calls.Load() != 3 || v.ErrorCode != "PROVIDER_TRANSIENT" {
		t.Fatalf("status %s calls %d code %s", v.Status, h.calls.Load(), v.ErrorCode)
	}
}

func TestDAGParallelBranchesAndJoin(t *testing.T) {
	var mu sync.Mutex
	var order []string
	running, maxRunning := 0, 0
	gate := make(chan struct{})
	var branchStarted sync.WaitGroup
	branchStarted.Add(3)
	mk := func(typ string) *handler {
		return &handler{typ: typ, fn: func(_ context.Context, sc usecase.StepContext, _ int) (usecase.StepResult, error) {
			mu.Lock()
			running++
			maxRunning = max(maxRunning, running)
			order = append(order, "start:"+sc.StepKey())
			mu.Unlock()
			if typ == "branch" {
				// All three branches must be in flight at the same time.
				branchStarted.Done()
				<-gate
			}
			if typ == "join" {
				deps := sc.DependencyOutputs()
				if len(deps) != 3 {
					return usecase.StepResult{}, usecase.Permanent(errors.New("join saw missing deps"))
				}
			}
			mu.Lock()
			running--
			order = append(order, "end:"+sc.StepKey())
			mu.Unlock()
			return usecase.StepResult{Output: map[string]string{"key": sc.StepKey()}}, nil
		}}
	}
	e := newEnv(t, mk("root"), mk("branch"), mk("join"))
	id := e.create(spec(
		usecase.StepSpec{Key: "root", Type: "root"},
		usecase.StepSpec{Key: "b1", Type: "branch", DependsOn: []string{"root"}},
		usecase.StepSpec{Key: "b2", Type: "branch", DependsOn: []string{"root"}},
		usecase.StepSpec{Key: "b3", Type: "branch", DependsOn: []string{"root"}},
		usecase.StepSpec{Key: "join", Type: "join", DependsOn: []string{"b1", "b2", "b3"}},
	))
	go func() { branchStarted.Wait(); close(gate) }()
	e.drain(true)
	v := e.job(id)
	if v.Status != usecase.StatusSucceeded {
		t.Fatalf("status %s (%s)", v.Status, v.ErrorCode)
	}
	if maxRunning < 3 {
		t.Fatalf("branches did not run in parallel (max %d)", maxRunning)
	}
	pos := func(s string) int { return slices.Index(order, s) }
	for _, b := range []string{"b1", "b2", "b3"} {
		if pos("end:root") > pos("start:"+b) || pos("end:"+b) > pos("start:join") {
			t.Fatalf("bad order %v", order)
		}
	}
}

func TestDAGValidation(t *testing.T) {
	e := newEnv(t)
	_, err := e.svc.Create(context.Background(), spec(
		usecase.StepSpec{Key: "a", Type: "x", DependsOn: []string{"b"}},
		usecase.StepSpec{Key: "b", Type: "x", DependsOn: []string{"a"}},
	))
	if !errors.Is(err, kernel.ErrValidation) {
		t.Fatalf("cycle: %v", err)
	}
	if _, err := e.svc.Create(context.Background(), spec()); !errors.Is(err, kernel.ErrValidation) {
		t.Fatalf("no steps: %v", err)
	}
}

func TestIdempotency(t *testing.T) {
	e := newEnv(t)
	s := spec(usecase.StepSpec{Key: "gen", Type: "gen"})
	s.IdempotencyKey, s.RequestHash = "k1", "h1"
	ctx := context.Background()
	r1, err := e.svc.Create(ctx, s)
	if err != nil || r1.Existing {
		t.Fatal(r1, err)
	}
	r2, err := e.svc.Create(ctx, s)
	if err != nil || !r2.Existing || r2.JobID != r1.JobID {
		t.Fatalf("replay: %+v %v", r2, err)
	}
	if !r2.CostEstimate.Amount.Equal(s.CostEstimate.Amount) {
		t.Fatalf("estimate %v", r2.CostEstimate)
	}
	s.RequestHash = "h2"
	if _, err := e.svc.Create(ctx, s); !errors.Is(err, kernel.ErrIdempotencyConflict) {
		t.Fatalf("different body: %v", err)
	}
	// Another user may reuse the key.
	s.CreatedBy = kernel.NewID()
	if r, err := e.svc.Create(ctx, s); err != nil || r.Existing {
		t.Fatalf("other user: %+v %v", r, err)
	}
	page, err := e.svc.List(ctx, testutil.Actor(kernel.RoleAdmin), usecase.JobFilter{}, kernel.PageRequest{Limit: 50})
	if err != nil || len(page.Items) != 2 {
		t.Fatalf("jobs = %d %v", len(page.Items), err)
	}
	// Only one outbox row for the replayed job.
	n := 0
	for _, o := range e.mem.Outbox() {
		var p usecase.TaskPayload
		_ = json.Unmarshal(o.Payload, &p)
		if p.JobID == r1.JobID {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("outbox rows = %d", n)
	}
}

func TestCreateRateLimit(t *testing.T) {
	e := newEnv(t)
	e.svc.Limiter = &fakeLimiter{}
	ctx := context.Background()
	for i := 0; i < usecase.CreateRateLimit; i++ {
		s := spec(usecase.StepSpec{Key: "gen", Type: "gen"})
		s.IdempotencyKey, s.RequestHash = kernel.NewID().String(), "h"
		if _, err := e.svc.Create(ctx, s); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}
	s := spec(usecase.StepSpec{Key: "gen", Type: "gen"})
	s.IdempotencyKey, s.RequestHash = kernel.NewID().String(), "h"
	if _, err := e.svc.Create(ctx, s); !errors.Is(err, kernel.ErrRateLimited) {
		t.Fatalf("31st create: %v", err)
	}
	// The middleware helper marks ctx so Create does not count twice.
	other := kernel.NewID()
	rctx, err := usecase.RateLimitCreate(ctx, e.svc, other)
	if err != nil || !usecase.RateChecked(rctx) {
		t.Fatal(err)
	}
	// Fakes without a limiter are a no-op.
	if _, err := usecase.RateLimitCreate(ctx, jobtest.NewAPI(), other); err != nil {
		t.Fatal(err)
	}
}

func TestCancel(t *testing.T) {
	e := newEnv(t, &handler{typ: "gen"})
	ctx := context.Background()
	owner := testutil.Actor(kernel.RoleContentCreator)
	s := spec(usecase.StepSpec{Key: "gen", Type: "gen"})
	s.CreatedBy = owner.UserID
	id := e.create(s)
	stranger := testutil.Actor(kernel.RoleContentCreator)
	if err := e.svc.RequestCancel(ctx, stranger, id); !errors.Is(err, kernel.ErrNotFound) {
		t.Fatalf("stranger: %v", err)
	}
	if err := e.svc.RequestCancel(ctx, owner, id); err != nil {
		t.Fatal(err)
	}
	v := e.job(id)
	if v.Status != usecase.StatusCancelled || step(v, "gen").Status != usecase.StepCancelled {
		t.Fatalf("job %s step %s", v.Status, step(v, "gen").Status)
	}
	// The queued task is now stale and does nothing.
	e.drain(false)
	if err := e.svc.RequestCancel(ctx, owner, id); !errors.Is(err, kernel.ErrInvalidTransition) {
		t.Fatalf("cancel finished job: %v", err)
	}
	// Succeeded job → 409 too.
	id2 := e.create(s)
	e.drain(false)
	if err := e.svc.RequestCancel(ctx, testutil.Actor(kernel.RoleAdmin), id2); !errors.Is(err, kernel.ErrInvalidTransition) {
		t.Fatalf("cancel succeeded job: %v", err)
	}
}

func TestCancelWhileWaitingProviderCallsOnCancel(t *testing.T) {
	h := &handler{typ: "scene", queue: usecase.QueueVideo, fn: func(context.Context, usecase.StepContext, int) (usecase.StepResult, error) {
		return usecase.StepResult{Wait: &usecase.WaitSpec{ProviderJobID: "op-1", PollAfter: 10 * time.Second}}, nil
	}}
	e := newEnv(t, cancelHandler{h})
	ctx := context.Background()
	s := spec(usecase.StepSpec{Key: "scene:1", Type: "scene"})
	s.Type = usecase.TypeVideoProduction
	id := e.create(s)
	e.round(false) // submit → waiting_provider
	if st := step(e.job(id), "scene:1"); st.Status != usecase.StepWaitingProvider {
		t.Fatalf("step %s", st.Status)
	}
	if err := e.svc.RequestCancel(ctx, kernel.Actor{UserID: userA}, id); err != nil {
		t.Fatal(err)
	}
	if v := e.job(id); v.Status != usecase.StatusRunning || !v.CancelRequested {
		t.Fatalf("job %s cancel=%v", v.Status, v.CancelRequested)
	}
	e.drain(false)
	v := e.job(id)
	if v.Status != usecase.StatusCancelled || h.cancels.Load() != 1 {
		t.Fatalf("job %s cancels %d", v.Status, h.cancels.Load())
	}
}

func TestWaitPollsAsynchronously(t *testing.T) {
	h := &handler{typ: "scene", queue: usecase.QueueVideo, fn: func(_ context.Context, sc usecase.StepContext, n int) (usecase.StepResult, error) {
		switch {
		case n == 1:
			if sc.ProviderJobID() != "" {
				return usecase.StepResult{}, usecase.Permanent(errors.New("unexpected provider job"))
			}
			return usecase.StepResult{Wait: &usecase.WaitSpec{ProviderJobID: "op-9", PollAfter: 10 * time.Second}}, nil
		case n < 4:
			if sc.ProviderJobID() != "op-9" {
				return usecase.StepResult{}, usecase.Permanent(errors.New("lost provider job id"))
			}
			return usecase.StepResult{Wait: &usecase.WaitSpec{PollAfter: 10 * time.Second}}, nil
		}
		return usecase.StepResult{Output: map[string]string{"clip": "f1"}}, nil
	}}
	e := newEnv(t, h)
	s := spec(usecase.StepSpec{Key: "scene:1", Type: "scene"})
	s.Type = usecase.TypeVideoProduction
	id := e.create(s)
	e.drain(false)
	v := e.job(id)
	if v.Status != usecase.StatusSucceeded {
		t.Fatalf("status %s %s", v.Status, v.ErrorCode)
	}
	if st := step(v, "scene:1"); st.AttemptCount != 1 || h.calls.Load() != 4 {
		t.Fatalf("attempts %d calls %d", st.AttemptCount, h.calls.Load())
	}
}

func TestConcurrencyCapWaitsInsteadOfFailing(t *testing.T) {
	e := newEnv(t, &handler{typ: "gen"})
	sem := &fakeSem{deny: 2}
	e.svc.Sem = sem
	e.svc.ConcurrencyLimit = func(context.Context, usecase.Queue, string, string) (int, error) { return 1, nil }
	id := e.create(spec(usecase.StepSpec{Key: "gen", Type: "gen"}))
	e.round(false)
	v := e.job(id)
	if st := step(v, "gen"); st.Status != usecase.StepQueued || st.AttemptCount != 0 {
		t.Fatalf("over cap: step %+v", st)
	}
	e.drain(false)
	v = e.job(id)
	if v.Status != usecase.StatusSucceeded || step(v, "gen").AttemptCount != 1 {
		t.Fatalf("status %s attempts %d", v.Status, step(v, "gen").AttemptCount)
	}
	waits := 0
	for _, ev := range e.mem.Events() {
		if ev.Type == usecase.EventConcurrencyWait {
			waits++
		}
	}
	if waits != 1 {
		t.Fatalf("concurrency_wait events = %d", waits)
	}
	if len(sem.holders["model:fake:fake-text"]) != 0 {
		t.Fatal("model slot not released")
	}
}

func TestUserVideoCap(t *testing.T) {
	block := make(chan struct{})
	h := &handler{typ: "scene", queue: usecase.QueueVideo, fn: func(context.Context, usecase.StepContext, int) (usecase.StepResult, error) {
		return usecase.StepResult{Wait: &usecase.WaitSpec{ProviderJobID: "x", PollAfter: time.Hour}}, nil
	}}
	_ = block
	e := newEnv(t, h)
	sem := &fakeSem{}
	e.svc.Sem, e.svc.UserVideoLimit = sem, 1
	s := spec(usecase.StepSpec{Key: "scene:1", Type: "scene"})
	s.Type = usecase.TypeVideoProduction
	first, second := e.create(s), e.create(s)
	e.round(false)
	if st := step(e.job(first), "scene:1"); st.Status != usecase.StepWaitingProvider {
		t.Fatalf("first %s", st.Status)
	}
	if st := step(e.job(second), "scene:1"); st.Status != usecase.StepQueued {
		t.Fatalf("second should wait for the user slot, got %s", st.Status)
	}
	// Cancelling the first job frees the slot for the second.
	if err := e.svc.RequestCancel(context.Background(), kernel.Actor{UserID: userA}, first); err != nil {
		t.Fatal(err)
	}
	e.clock.Advance(10 * time.Second)
	e.round(false)
	e.round(false)
	if v := e.job(first); v.Status != usecase.StatusCancelled {
		t.Fatalf("first %s", v.Status)
	}
	if st := step(e.job(second), "scene:1"); st.Status != usecase.StepWaitingProvider {
		t.Fatalf("second %s", st.Status)
	}
}

func TestRetryRerunsOnlyFailedSteps(t *testing.T) {
	failB := atomic.Bool{}
	failB.Store(true)
	a := &handler{typ: "a"}
	b := &handler{typ: "b", fn: func(context.Context, usecase.StepContext, int) (usecase.StepResult, error) {
		if failB.Load() {
			return usecase.StepResult{}, &ports.ProviderError{Kind: ports.ErrKindTransient}
		}
		return usecase.StepResult{Output: map[string]bool{"ok": true}}, nil
	}}
	e := newEnv(t, a, b)
	ctx := context.Background()
	owner := kernel.Actor{UserID: userA}
	id := e.create(spec(usecase.StepSpec{Key: "a", Type: "a"}, usecase.StepSpec{Key: "b", Type: "b", DependsOn: []string{"a"}}))
	e.drain(false)
	if v := e.job(id); v.Status != usecase.StatusFailed {
		t.Fatalf("status %s", v.Status)
	}
	// A failing hook (e.g. budget) aborts the retry atomically.
	hookErr := true
	var retried []usecase.JobRetried
	e.svc.OnRetried(func(_ context.Context, ev usecase.JobRetried) error {
		if hookErr {
			return kernel.ErrBudgetExceeded
		}
		retried = append(retried, ev)
		return nil
	})
	if _, err := e.svc.Retry(ctx, owner, id, false); !errors.Is(err, kernel.ErrBudgetExceeded) {
		t.Fatalf("retry with failing hook: %v", err)
	}
	if v := e.job(id); v.Status != usecase.StatusFailed {
		t.Fatalf("rollback: %s", v.Status)
	}
	hookErr = false
	failB.Store(false)
	if _, err := e.svc.Retry(ctx, kernel.Actor{UserID: kernel.NewID()}, id, false); !errors.Is(err, kernel.ErrNotFound) {
		t.Fatalf("stranger retry: %v", err)
	}
	if _, err := e.svc.Retry(ctx, owner, id, true); !errors.Is(err, kernel.ErrForbidden) {
		t.Fatalf("force by non-admin: %v", err)
	}
	res, err := e.svc.Retry(ctx, owner, id, false)
	if err != nil || res.Status != usecase.StatusQueued {
		t.Fatal(res, err)
	}
	if len(retried) != 1 || retried[0].JobID != id {
		t.Fatalf("OnRetried %+v", retried)
	}
	e.drain(false)
	v := e.job(id)
	if v.Status != usecase.StatusSucceeded {
		t.Fatalf("after retry %s", v.Status)
	}
	if a.calls.Load() != 1 {
		t.Fatalf("succeeded step re-ran: %d", a.calls.Load())
	}
	if _, err := e.svc.Retry(ctx, owner, id, false); !errors.Is(err, kernel.ErrInvalidTransition) {
		t.Fatalf("retry succeeded job: %v", err)
	}
}

func TestRetryNonRetryableNeedsAdminForce(t *testing.T) {
	h := &handler{typ: "gen", fn: func(context.Context, usecase.StepContext, int) (usecase.StepResult, error) {
		return usecase.StepResult{}, &ports.ProviderError{Kind: ports.ErrKindContentPolicy}
	}}
	e := newEnv(t, h)
	id := e.create(spec(usecase.StepSpec{Key: "gen", Type: "gen"}))
	e.drain(false)
	ctx := context.Background()
	if _, err := e.svc.Retry(ctx, kernel.Actor{UserID: userA}, id, false); !errors.Is(err, kernel.ErrInvalidTransition) {
		t.Fatalf("owner retry of policy error: %v", err)
	}
	if _, err := e.svc.Retry(ctx, testutil.Actor(kernel.RoleAdmin), id, true); err != nil {
		t.Fatal(err)
	}
}

func TestDispatcherSurvivesQueueOutage(t *testing.T) {
	e := newEnv(t, &handler{typ: "gen"})
	id := e.create(spec(usecase.StepSpec{Key: "gen", Type: "gen"}))
	e.q.fail.Store(true)
	n, err := e.svc.DispatchOnce(context.Background())
	if err != nil || n != 0 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	e.clock.Advance(5 * time.Second)
	if _, err := e.svc.DispatchOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(e.metrics.lags) == 0 || e.metrics.lags[len(e.metrics.lags)-1] < 5 {
		t.Fatalf("outbox lag not reported: %v", e.metrics.lags)
	}
	e.q.fail.Store(false)
	tasksBefore := e.q.ids
	_ = tasksBefore
	e.drain(false)
	if v := e.job(id); v.Status != usecase.StatusSucceeded {
		t.Fatalf("status %s", v.Status)
	}
	st, _ := e.mem.Step(id, "gen")
	if !e.q.ids[st.ID.String()+":1"] {
		t.Fatalf("task id must be step_id:attempt, got %v", e.q.ids)
	}
}

func TestTaskIDIsStepAndAttempt(t *testing.T) {
	sid := kernel.NewID()
	if got := usecase.TaskID(usecase.TaskPayload{StepID: sid, Attempt: 2, Kind: usecase.TaskRun}, 9); got != sid.String()+":2" {
		t.Fatal(got)
	}
	if got := usecase.TaskID(usecase.TaskPayload{StepID: sid, Attempt: 2, Kind: usecase.TaskPoll}, 9); got != sid.String()+":2:poll:9" {
		t.Fatal(got)
	}
}

func TestDuplicateTaskIsIgnored(t *testing.T) {
	h := &handler{typ: "gen"}
	e := newEnv(t, h)
	id := e.create(spec(usecase.StepSpec{Key: "gen", Type: "gen"}))
	ctx := context.Background()
	if _, err := e.svc.DispatchOnce(ctx); err != nil {
		t.Fatal(err)
	}
	tasks := e.q.take()
	var p usecase.TaskPayload
	_ = json.Unmarshal(tasks[0].Payload, &p)
	for i := 0; i < 3; i++ { // at-least-once delivery
		if err := e.svc.Execute(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	if h.calls.Load() != 1 || e.job(id).Status != usecase.StatusSucceeded {
		t.Fatalf("calls %d", h.calls.Load())
	}
}

func TestStepTimeoutIsRetried(t *testing.T) {
	h := &handler{typ: "gen", timeout: 20 * time.Millisecond, fn: func(ctx context.Context, _ usecase.StepContext, n int) (usecase.StepResult, error) {
		if n == 1 {
			<-ctx.Done()
			return usecase.StepResult{}, ctx.Err()
		}
		return usecase.StepResult{}, nil
	}}
	e := newEnv(t, h)
	id := e.create(spec(usecase.StepSpec{Key: "gen", Type: "gen"}))
	e.drain(false)
	v := e.job(id)
	if v.Status != usecase.StatusSucceeded || step(v, "gen").AttemptCount != 2 {
		t.Fatalf("status %s attempts %d", v.Status, step(v, "gen").AttemptCount)
	}
}

func TestUnknownStepTypeFailsJob(t *testing.T) {
	e := newEnv(t)
	id := e.create(spec(usecase.StepSpec{Key: "x", Type: "nobody"}))
	e.drain(false)
	if v := e.job(id); v.Status != usecase.StatusFailed || v.ErrorCode != usecase.CodeStepTypeUnknown {
		t.Fatalf("status %s code %s", v.Status, v.ErrorCode)
	}
}

func TestEventsPublishedAfterCommit(t *testing.T) {
	e := newEnv(t, &handler{typ: "gen", fn: func(ctx context.Context, sc usecase.StepContext, _ int) (usecase.StepResult, error) {
		_ = sc.Progress(ctx, 50, "half")
		return usecase.StepResult{}, nil
	}})
	id := e.create(spec(usecase.StepSpec{Key: "gen", Type: "gen"}))
	e.drain(false)
	var types []string
	for _, ev := range e.events.Published {
		if ev.JobID == id {
			types = append(types, ev.Type)
		}
	}
	for _, want := range []string{ports.JobEventStatus, ports.JobEventStep, ports.JobEventProgress, ports.JobEventDone} {
		if !slices.Contains(types, want) {
			t.Fatalf("missing %s in %v", want, types)
		}
	}
	if types[len(types)-1] != ports.JobEventDone {
		t.Fatalf("done must be last: %v", types)
	}
}

func TestSaveOutputCheckpoint(t *testing.T) {
	h := &handler{typ: "gen", fn: func(ctx context.Context, sc usecase.StepContext, n int) (usecase.StepResult, error) {
		if n == 1 {
			if err := sc.SaveOutput(ctx, map[string]int{"done": 2}); err != nil {
				return usecase.StepResult{}, err
			}
			return usecase.StepResult{}, transient()
		}
		saved := sc.(interface{ SavedOutput() json.RawMessage }).SavedOutput()
		if string(saved) != `{"done":2}` {
			return usecase.StepResult{}, usecase.Permanent(errors.New("checkpoint lost: " + string(saved)))
		}
		return usecase.StepResult{Output: map[string]int{"done": 3}}, nil
	}}
	e := newEnv(t, h)
	id := e.create(spec(usecase.StepSpec{Key: "gen", Type: "gen"}))
	e.drain(false)
	if v := e.job(id); v.Status != usecase.StatusSucceeded {
		t.Fatalf("status %s %s", v.Status, step(v, "gen").ErrorMessage)
	}
}

func TestReconcilerRequeuesExpiredLease(t *testing.T) {
	h := &handler{typ: "scene", queue: usecase.QueueVideo}
	e := newEnv(t, h)
	s := spec(usecase.StepSpec{Key: "scene:1", Type: "scene"})
	s.Type = usecase.TypeVideoProduction
	id := e.create(s)
	// Simulate a worker that claimed the step and died.
	st, _ := e.mem.Step(id, "scene:1")
	exp := e.clock.Now().Add(-time.Minute)
	st.Status, st.AttemptCount, st.LeaseOwner, st.LeaseExpiresAt = usecase.StepRunning, 1, "dead", &exp
	e.mem.SetStep(st)
	_ = e.q.take()
	for _, o := range e.mem.Outbox() { // the original task was consumed
		_ = o
	}
	if _, err := e.svc.DispatchOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	_ = e.q.take()
	rep, err := e.svc.ReconcileOnce(context.Background())
	if err != nil || rep.LeasesRequeued != 1 {
		t.Fatalf("rep %+v err %v", rep, err)
	}
	e.drain(false)
	v := e.job(id)
	if v.Status != usecase.StatusSucceeded || step(v, "scene:1").AttemptCount != 2 {
		t.Fatalf("status %s attempts %d", v.Status, step(v, "scene:1").AttemptCount)
	}
}

func TestReconcilerPollsOverdueWaitingAndTimesOut(t *testing.T) {
	h := &handler{typ: "scene", queue: usecase.QueueVideo, fn: func(context.Context, usecase.StepContext, int) (usecase.StepResult, error) {
		return usecase.StepResult{Wait: &usecase.WaitSpec{ProviderJobID: "op", PollAfter: 10 * time.Second}}, nil
	}}
	e := newEnv(t, h)
	s := spec(usecase.StepSpec{Key: "scene:1", Type: "scene"})
	s.Type = usecase.TypeVideoProduction
	id := e.create(s)
	e.round(false)
	// The poll task got lost (Redis flushed): mark it published without running.
	if _, err := e.svc.DispatchOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	e.clock.Advance(20 * time.Second)
	_, _ = e.svc.DispatchOnce(context.Background())
	_ = e.q.take()
	e.clock.Advance(5 * time.Minute)
	rep, err := e.svc.ReconcileOnce(context.Background())
	if err != nil || rep.WaitingPolled != 1 {
		t.Fatalf("rep %+v err %v", rep, err)
	}
	e.round(false)
	if h.calls.Load() != 2 {
		t.Fatalf("poll did not run: calls %d", h.calls.Load())
	}
	// Past the provider deadline the step fails and the job needs reconciliation.
	_ = e.q.take()
	e.clock.Advance(time.Hour)
	_, _ = e.svc.DispatchOnce(context.Background())
	_ = e.q.take()
	rep, err = e.svc.ReconcileOnce(context.Background())
	if err != nil || rep.WaitingTimedOut != 1 || rep.NeedsReconciliation != 1 {
		t.Fatalf("rep %+v err %v", rep, err)
	}
	d, err := e.svc.GetForActor(context.Background(), kernel.Actor{UserID: userA}, id)
	if err != nil || d.Status != usecase.StatusFailed || !d.NeedsReconciliation || d.ErrorCode != usecase.CodeProviderWaitTimeout {
		t.Fatalf("job %+v err %v", d.Status, err)
	}
}

func TestReconcilerPurgesUploads(t *testing.T) {
	e := newEnv(t)
	var got time.Duration
	e.svc.PurgeStaleUploads = func(_ context.Context, d time.Duration) (int, error) { got = d; return 4, nil }
	rep, err := e.svc.ReconcileOnce(context.Background())
	if err != nil || rep.UploadsPurged != 4 || got != 24*time.Hour {
		t.Fatalf("rep %+v got %s err %v", rep, got, err)
	}
}

func TestListAndViewAuthorization(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	owner := testutil.Actor(kernel.RoleContentCreator)
	s := spec(usecase.StepSpec{Key: "gen", Type: "gen"})
	s.CreatedBy = owner.UserID
	id := e.create(s)
	e.create(spec(usecase.StepSpec{Key: "gen", Type: "gen"})) // userA's job
	page, err := e.svc.List(ctx, owner, usecase.JobFilter{}, kernel.PageRequest{Limit: 10})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != id {
		t.Fatalf("owner list %d %v", len(page.Items), err)
	}
	admin := testutil.Actor(kernel.RoleAdmin)
	page, err = e.svc.List(ctx, admin, usecase.JobFilter{}, kernel.PageRequest{Limit: 1})
	if err != nil || len(page.Items) != 1 || page.NextCursor == nil {
		t.Fatalf("admin page 1: %d %v", len(page.Items), err)
	}
	page2, err := e.svc.List(ctx, admin, usecase.JobFilter{}, kernel.PageRequest{Limit: 1, Cursor: *page.NextCursor})
	if err != nil || len(page2.Items) != 1 || page2.Items[0].ID == page.Items[0].ID {
		t.Fatalf("admin page 2: %v", err)
	}
	if _, err := e.svc.GetForActor(ctx, testutil.Actor(kernel.RoleContentCreator), id); !errors.Is(err, kernel.ErrNotFound) {
		t.Fatalf("IDOR: %v", err)
	}
	if _, err := e.svc.GetForActor(ctx, admin, id); err != nil {
		t.Fatal(err)
	}
	hist, err := e.svc.History(ctx, owner, id, 0)
	if err != nil || len(hist) == 0 || hist[0].Type != usecase.EventCreated {
		t.Fatalf("history %v %v", hist, err)
	}
}
