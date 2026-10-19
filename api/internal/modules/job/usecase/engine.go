package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math/rand/v2"
	"strings"
	"sync"
	"time"

	"github.com/be/acs/api/internal/modules/job/domain"
	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Engine defaults (SPEC-02 §5.3).
const (
	DefaultLeaseTTL          = 2 * time.Minute
	DefaultHeartbeat         = 30 * time.Second
	DefaultPollAfter         = 10 * time.Second
	DefaultConcurrencyWait   = 5 * time.Second
	DefaultUserVideoLimit    = 3
	DefaultMaxProviderWait   = 30 * time.Minute
	DefaultOverdueGrace      = 2 * time.Minute
	DefaultStaleQueuedAfter  = 10 * time.Minute
	DefaultOutboxRetention   = 7 * 24 * time.Hour
	DefaultPendingUploadTTL  = 24 * time.Hour
	reconcilerAdvisoryLockID = 0x6a6f625f72656331 // "job_rec1"
)

// Job error codes stored in ai_jobs.error_code.
const (
	CodeStepFailed          = "STEP_FAILED"
	CodeStepTimeout         = "STEP_TIMEOUT"
	CodeLeaseExpired        = "LEASE_EXPIRED"
	CodeProviderWaitTimeout = "PROVIDER_WAIT_TIMEOUT"
	CodeStepTypeUnknown     = "STEP_TYPE_UNKNOWN"
	CodeCancelled           = "CANCELLED"
)

// retryableJobCodes may be retried by the owner via POST /jobs/{id}/retry;
// any other failure requires an admin with force=true.
var retryableJobCodes = map[string]bool{
	CodeStepFailed: true, CodeStepTimeout: true, CodeLeaseExpired: true, CodeProviderWaitTimeout: true,
	"PROVIDER_TRANSIENT": true, "PROVIDER_RATE_LIMITED": true, "PROVIDER_UNKNOWN": true,
}

// IsRetryableCode reports whether a failed job with this error code may be
// retried without force.
func IsRetryableCode(code string) bool { return retryableJobCodes[code] }

// Service implements API plus the engine use cases (job API, dispatcher,
// worker runtime, reconciler). Optional collaborators may be nil.
type Service struct {
	Tx       ports.TxManager
	Repo     Repository
	Clock    ports.Clock
	Steps    *StepRegistry
	Events   ports.EventPublisher
	Notifier ports.Notifier
	Queue    TaskQueue
	Sem      Semaphore
	Limiter  RateLimiter
	// ConcurrencyLimit resolves ai_models.concurrency_limit.
	ConcurrencyLimit ConcurrencyLimitFunc
	// PurgeStaleUploads deletes pending uploads (file module) during maintenance.
	PurgeStaleUploads func(ctx context.Context, olderThan time.Duration) (int, error)
	Metrics           Metrics
	Logger            *slog.Logger
	WorkerID          string
	// Rand returns a float in [0,1) for jitter (defaults to math/rand).
	Rand func() float64

	LeaseTTL         time.Duration
	Heartbeat        time.Duration
	ConcurrencyWait  time.Duration
	UserVideoLimit   int
	MaxProviderWait  time.Duration
	OverdueGrace     time.Duration
	StaleQueuedAfter time.Duration

	mu       sync.RWMutex
	finished []func(context.Context, JobFinished) error
	retried  []func(context.Context, JobRetried) error
}

var (
	_ API               = (*Service)(nil)
	_ CreateRateLimiter = (*Service)(nil)
)

func (s *Service) log() *slog.Logger {
	if s.Logger == nil {
		return slog.Default()
	}
	return s.Logger
}

func (s *Service) now() time.Time { return s.Clock.Now().UTC() }

func (s *Service) rnd() float64 {
	if s.Rand != nil {
		return s.Rand()
	}
	return rand.Float64() //nolint:gosec // jitter, not security
}

func durOr(d, def time.Duration) time.Duration {
	if d > 0 {
		return d
	}
	return def
}

func (s *Service) workerID() string {
	if s.WorkerID == "" {
		return "worker"
	}
	return s.WorkerID
}

// OnFinished implements API.
func (s *Service) OnFinished(f func(context.Context, JobFinished) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.finished = append(s.finished, f)
}

// OnRetried implements API.
func (s *Service) OnRetried(f func(context.Context, JobRetried) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.retried = append(s.retried, f)
}

// effects collects work that must happen only after the transaction
// commits: Redis pub/sub events (SSE) and semaphore releases.
type effects struct {
	events []ports.JobEvent
	after  []func(context.Context)
}

func (fx *effects) publish(jobID kernel.ID, typ string, data any) {
	b, _ := json.Marshal(data)
	fx.events = append(fx.events, ports.JobEvent{JobID: jobID, Type: typ, Data: b})
}

// inTx runs fn in a transaction and applies effects after commit.
func (s *Service) inTx(ctx context.Context, fn func(ctx context.Context, fx *effects) error) error {
	fx := &effects{}
	if err := s.Tx.WithinTx(ctx, func(ctx context.Context) error { return fn(ctx, fx) }); err != nil {
		return err
	}
	s.applyEffects(ctx, fx)
	return nil
}

func (s *Service) applyEffects(ctx context.Context, fx *effects) {
	ctx = context.WithoutCancel(ctx)
	for _, f := range fx.after {
		f(ctx)
	}
	if s.Events == nil {
		return
	}
	for _, ev := range fx.events {
		if err := s.Events.Publish(ctx, ev); err != nil {
			s.log().WarnContext(ctx, "publish job event", slog.String("job_id", ev.JobID.String()), slog.String("error", err.Error()))
		}
	}
}

func (s *Service) appendEvent(ctx context.Context, jobID kernel.ID, stepID *kernel.ID, typ string, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if payload == nil {
		b = []byte("{}")
	}
	return s.Repo.AppendEvent(ctx, &EventRecord{JobID: jobID, StepID: stepID, Type: typ, Payload: b, CreatedAt: s.now()})
}

// setJobStatus validates and applies a job transition, records it in
// ai_job_events and queues an SSE "status" event.
func (s *Service) setJobStatus(ctx context.Context, fx *effects, j *JobRecord, to Status) error {
	from := j.Status
	if err := domain.CheckJobTransition(domain.JobStatus(from), domain.JobStatus(to)); err != nil {
		return err
	}
	now := s.now()
	j.Status = to
	switch {
	case to == StatusRunning && j.StartedAt == nil:
		j.StartedAt = &now
	case to.IsTerminal():
		j.FinishedAt = &now
	case to == StatusQueued:
		j.FinishedAt = nil
	}
	if err := s.Repo.UpdateJob(ctx, j); err != nil {
		return err
	}
	if err := s.appendEvent(ctx, j.ID, nil, EventStatusChanged, map[string]any{"from": from, "to": to}); err != nil {
		return err
	}
	fx.publish(j.ID, ports.JobEventStatus, map[string]any{"job_id": j.ID, "status": to, "progress": j.Progress})
	return nil
}

// setStepStatus validates and applies a step transition (caller persists
// other step fields in the same UpdateStep).
func (s *Service) setStepStatus(ctx context.Context, fx *effects, st *StepRecord, to StepStatus) error {
	from := st.Status
	if err := domain.CheckStepTransition(domain.StepStatus(from), domain.StepStatus(to)); err != nil {
		return err
	}
	now := s.now()
	st.Status = to
	switch to {
	case StepRunning:
		if st.StartedAt == nil {
			st.StartedAt = &now
		}
	case StepSucceeded, StepFailed, StepCancelled, StepSkipped:
		st.FinishedAt = &now
		st.LeaseOwner, st.LeaseExpiresAt = "", nil
	case StepQueued, StepWaitingProvider, StepPending:
		st.LeaseOwner, st.LeaseExpiresAt = "", nil
		if to == StepPending {
			st.FinishedAt, st.StartedAt = nil, nil
		}
	}
	if err := s.Repo.UpdateStep(ctx, st); err != nil {
		return err
	}
	payload := map[string]any{"step_key": st.Key, "from": from, "to": to, "attempt_count": st.AttemptCount}
	if st.ErrorKind != "" && to != StepSucceeded {
		payload["error_kind"] = st.ErrorKind
	}
	id := st.ID
	if err := s.appendEvent(ctx, st.JobID, &id, EventStepStatus, payload); err != nil {
		return err
	}
	ev := map[string]any{"job_id": st.JobID, "step_key": st.Key, "status": to, "attempt_count": st.AttemptCount}
	if st.ErrorKind != "" && to != StepSucceeded {
		ev["error_kind"] = st.ErrorKind
	}
	fx.publish(st.JobID, ports.JobEventStep, ev)
	return nil
}

// topicFor returns the queue for a step type, or "auto" when this process
// has no handler (the API server); the dispatcher resolves it then.
func (s *Service) topicFor(stepType string) string {
	if s.Steps != nil {
		if h, ok := s.Steps.Lookup(stepType); ok {
			return string(h.Queue())
		}
	}
	return "auto"
}

// enqueue writes an outbox row (same transaction as the state change).
func (s *Service) enqueue(ctx context.Context, st *StepRecord, kind string, attempt int, at time.Time) error {
	p := TaskPayload{JobID: st.JobID, StepID: st.ID, Attempt: attempt, Kind: kind, StepType: st.Type, Gen: st.Generation}
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return s.Repo.InsertOutbox(ctx, &OutboxRecord{Topic: s.topicFor(st.Type), Payload: b, AvailableAt: at, CreatedAt: s.now()})
}

func states(steps []StepRecord) []domain.StepState {
	out := make([]domain.StepState, len(steps))
	for i, st := range steps {
		out[i] = domain.StepState{Key: st.Key, Status: domain.StepStatus(st.Status), DependsOn: st.DependsOn}
	}
	return out
}

// scheduleReady queues every pending step whose dependencies are done.
func (s *Service) scheduleReady(ctx context.Context, fx *effects, steps []StepRecord) error {
	ready := domain.ReadyKeys(states(steps))
	now := s.now()
	for _, key := range ready {
		for i := range steps {
			st := &steps[i]
			if st.Key != key {
				continue
			}
			st.NextAttemptAt = &now
			if err := s.setStepStatus(ctx, fx, st, StepQueued); err != nil {
				return err
			}
			if err := s.enqueue(ctx, st, TaskRun, st.AttemptCount+1, now); err != nil {
				return err
			}
		}
	}
	return nil
}

// finalize moves the job to a terminal status: every non-terminal step is
// cancelled, hooks run inside the transaction, SSE "done" after commit.
func (s *Service) finalize(ctx context.Context, fx *effects, j *JobRecord, steps []StepRecord, to Status, code, msg string) error {
	for i := range steps {
		st := &steps[i]
		if !StepStatusTerminal(st.Status) {
			if err := s.setStepStatus(ctx, fx, st, StepCancelled); err != nil {
				return err
			}
		}
	}
	if to == StatusSucceeded {
		j.Progress = 100
	}
	if to != StatusSucceeded {
		j.ErrorCode, j.ErrorMessage = code, msg
	}
	if err := s.setJobStatus(ctx, fx, j, to); err != nil {
		return err
	}
	ev := JobFinished{JobID: j.ID, Type: j.Type, Status: to, CreatedBy: j.CreatedBy, CampaignID: j.CampaignID, ErrorCode: j.ErrorCode, FinishedAt: *j.FinishedAt}
	s.mu.RLock()
	hooks := append([]func(context.Context, JobFinished) error(nil), s.finished...)
	s.mu.RUnlock()
	for _, h := range hooks {
		if err := h(ctx, ev); err != nil {
			return err
		}
	}
	if s.Notifier != nil && to != StatusCancelled {
		typ := ports.NotifyJobSucceeded
		if to == StatusFailed {
			typ = ports.NotifyJobFailed
		}
		payload := map[string]any{"job_id": j.ID, "job_type": j.Type, "status": to, "error_code": j.ErrorCode}
		if err := s.Notifier.Notify(ctx, []kernel.ID{j.CreatedBy}, typ, payload); err != nil {
			return err
		}
	}
	done := map[string]any{"job_id": j.ID, "status": to, "result": rawOrNull(j.Result)}
	if to != StatusSucceeded {
		done["error"] = map[string]any{"code": j.ErrorCode, "message": j.ErrorMessage}
	}
	fx.publish(j.ID, ports.JobEventDone, done)
	jobID, jobType, creator := j.ID, j.Type, j.CreatedBy
	fx.after = append(fx.after, func(ctx context.Context) {
		if s.Metrics != nil {
			s.Metrics.JobFinished(string(jobType), string(to))
		}
		if s.Sem != nil && strings.HasPrefix(string(jobType), "video_") {
			_ = s.Sem.Release(ctx, userVideoKey(creator), jobID.String())
		}
	})
	return nil
}

// StepStatusTerminal reports whether a step status is terminal.
func StepStatusTerminal(s StepStatus) bool { return domain.StepStatus(s).IsTerminal() }

func rawOrNull(b json.RawMessage) json.RawMessage {
	if len(b) == 0 {
		return json.RawMessage("null")
	}
	return b
}

func userVideoKey(userID kernel.ID) string { return "user:" + userID.String() + ":video" }

// maxAttempts resolves the effective attempt cap of a step.
func maxAttempts(st StepRecord, h StepHandler) int {
	if st.MaxAttempts > 0 {
		return st.MaxAttempts
	}
	if h != nil && h.Queue() == QueueVideo {
		return domain.DefaultVideoMaxAttempts
	}
	return domain.DefaultMaxAttempts
}

// failure is a classified step error.
type failure struct {
	kind       string // ProviderError kind, "Timeout", "Permanent", "Domain", "Internal"
	retryable  bool
	code       string // job error_code
	message    string // Vietnamese, user-facing
	detail     string // internal (step error_message)
	provider   bool
	retryAfter time.Duration
}

var providerMessages = map[ports.ErrKind]string{
	ports.ErrKindContentPolicy:  "Nội dung vi phạm chính sách của nhà cung cấp AI",
	ports.ErrKindInvalidRequest: "Yêu cầu không hợp lệ với nhà cung cấp AI",
	ports.ErrKindAuth:           "Lỗi xác thực với nhà cung cấp AI, vui lòng liên hệ quản trị viên",
}

func providerCode(k ports.ErrKind) string {
	var b strings.Builder
	for i, r := range string(k) {
		if i > 0 && r >= 'A' && r <= 'Z' {
			b.WriteByte('_')
		}
		b.WriteRune(r)
	}
	return "PROVIDER_" + strings.ToUpper(b.String())
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// classify maps a handler error onto the retry policy.
func classify(err error, timedOut bool) failure {
	f := failure{detail: truncate(err.Error(), 2000)}
	var perm permanentError
	isPerm := errors.As(err, &perm)
	if pe, ok := ports.AsProviderError(err); ok {
		f.kind, f.provider, f.retryAfter = string(pe.Kind), true, pe.RetryAfter
		f.retryable = pe.Retryable() && !isPerm
		f.code = providerCode(pe.Kind)
		f.message = providerMessages[pe.Kind]
		if f.message == "" {
			f.message = kernel.ErrProvider.Message
		}
		return f
	}
	if de, ok := kernel.AsDomainError(err); ok {
		f.kind, f.code, f.message = "Domain", de.Code, de.Message
		return f
	}
	if isPerm {
		f.kind, f.code, f.message = "Permanent", CodeStepFailed, "Tác vụ xử lý thất bại"
		return f
	}
	if timedOut || errors.Is(err, context.DeadlineExceeded) {
		f.kind, f.retryable, f.code, f.message = "Timeout", true, CodeStepTimeout, "Tác vụ quá thời gian xử lý"
		return f
	}
	f.kind, f.retryable, f.code, f.message = "Internal", true, CodeStepFailed, "Tác vụ xử lý thất bại"
	return f
}
