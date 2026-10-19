package usecase

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/be/acs/api/internal/shared/kernel"
)

// JobRecord is the persisted ai_jobs row.
type JobRecord struct {
	ID                  kernel.ID
	Type                JobType
	Status              Status
	CampaignID          *kernel.ID
	AssetID             *kernel.ID
	BrandVersionID      *kernel.ID
	PromptTemplateID    *kernel.ID
	ProviderCode        string
	ModelCode           string
	Request             json.RawMessage
	Result              json.RawMessage
	IdempotencyKey      string
	RequestHash         string
	CostEstimate        *kernel.Money
	CostActual          *kernel.Money
	CostSource          string
	PricingVersion      string
	BudgetOverrideBy    *kernel.ID
	CancelRequested     bool
	NeedsReconciliation bool
	Progress            int
	ErrorCode           string
	ErrorMessage        string
	CreatedBy           kernel.ID
	StartedAt           *time.Time
	FinishedAt          *time.Time
	CreatedAt           time.Time
}

// StepRecord is the persisted ai_job_steps row. MaxAttempts 0 means "engine
// default for the handler's queue" (3, or 5 on the video queue).
type StepRecord struct {
	ID             kernel.ID
	JobID          kernel.ID
	Type           string
	Key            string
	DependsOn      []string
	SceneID        *kernel.ID
	Status         StepStatus
	Input          json.RawMessage
	Output         json.RawMessage
	ProviderJobID  string
	AttemptCount   int
	MaxAttempts    int
	NextAttemptAt  *time.Time
	LeaseOwner     string
	LeaseExpiresAt *time.Time
	ErrorKind      string
	ErrorMessage   string
	StartedAt      *time.Time
	FinishedAt     *time.Time
	CreatedAt      time.Time
	// Generation counts job retries that re-opened this step.
	Generation int
}

// EventRecord is one append-only ai_job_events row.
type EventRecord struct {
	ID        int64
	JobID     kernel.ID
	StepID    *kernel.ID
	Type      string
	Payload   json.RawMessage
	CreatedAt time.Time
}

// Job event types persisted in ai_job_events.
const (
	EventCreated         = "created"
	EventStatusChanged   = "status_changed"
	EventStepStatus      = "step_status_changed"
	EventRetryScheduled  = "retry_scheduled"
	EventProviderError   = "provider_error"
	EventCancelRequested = "cancel_requested"
	EventRetried         = "retried"
	EventLeaseExpired    = "lease_expired"
	EventConcurrencyWait = "concurrency_wait"
	EventReconciled      = "reconciled"
)

// OutboxRecord is one outbox_events row (topic = queue name).
type OutboxRecord struct {
	ID          int64
	Topic       string
	Payload     json.RawMessage
	AvailableAt time.Time
	PublishedAt *time.Time
	CreatedAt   time.Time
}

// Task kinds carried in TaskPayload.Kind.
const (
	TaskRun   = "run"   // first execution of an attempt
	TaskPoll  = "poll"  // resume a waiting_provider step
	TaskDefer = "defer" // re-dispatch after a concurrency wait (same attempt)
)

// TaskPayload is the outbox/asynq payload of a step task.
type TaskPayload struct {
	JobID  kernel.ID `json:"job_id"`
	StepID kernel.ID `json:"step_id"`
	// Attempt is the attempt number this task executes (attempt_count+1 for
	// run/defer, attempt_count for poll).
	Attempt  int    `json:"attempt"`
	Kind     string `json:"kind"`
	StepType string `json:"step_type"`
	// Gen is the step generation (job retries); tasks of an older
	// generation are stale.
	Gen int `json:"gen,omitempty"`
}

// TaskTypeStep is the asynq task type of every job step.
const TaskTypeStep = "job:step"

// TaskID is the asynq unique task id: step_id + attempt (+ generation after
// a job retry, which restarts attempts at 1); re-dispatches of
// the same attempt (poll, concurrency wait, reconciler requeue) add the
// outbox row id, so replaying one outbox row is still de-duplicated while a
// new dispatch is never swallowed by an earlier task id.
func TaskID(p TaskPayload, outboxID int64) string {
	id := p.StepID.String() + ":" + strconv.Itoa(p.Attempt)
	if p.Gen > 0 {
		id += ":g" + strconv.Itoa(p.Gen)
	}
	if p.Kind != TaskRun {
		id += ":" + p.Kind + ":" + strconv.FormatInt(outboxID, 10)
	}
	return id
}

// JobFilter narrows GET /jobs.
type JobFilter struct {
	Status              []Status
	Type                []JobType
	CampaignID          *kernel.ID
	CreatedBy           *kernel.ID
	ProviderCode        string
	NeedsReconciliation *bool
}

// Repository persists jobs, steps, events and the outbox. Every method joins
// the transaction carried by ctx (ports.TxManager).
type Repository interface {
	// InsertJob inserts j unless (created_by, idempotency_key) exists, in
	// which case it returns inserted=false (ON CONFLICT DO NOTHING).
	InsertJob(ctx context.Context, j *JobRecord) (inserted bool, err error)
	FindJobByIdempotency(ctx context.Context, createdBy kernel.ID, key string) (JobRecord, error)
	GetJob(ctx context.Context, id kernel.ID) (JobRecord, error)
	// LockJob loads the job FOR UPDATE (always lock the job before its steps).
	LockJob(ctx context.Context, id kernel.ID) (JobRecord, error)
	UpdateJob(ctx context.Context, j *JobRecord) error
	// RaiseJobProgress sets progress = max(progress, pct) on a running job.
	RaiseJobProgress(ctx context.Context, id kernel.ID, pct int) error
	// ListJobs returns up to limit jobs ordered by (created_at, id) DESC,
	// strictly after the cursor position when given.
	ListJobs(ctx context.Context, f JobFilter, limit int, after *kernel.TimeIDCursor) ([]JobRecord, error)

	InsertStep(ctx context.Context, s *StepRecord) error
	// ListSteps returns the steps of a job in creation order.
	ListSteps(ctx context.Context, jobID kernel.ID) ([]StepRecord, error)
	GetStep(ctx context.Context, id kernel.ID) (StepRecord, error)
	UpdateStep(ctx context.Context, s *StepRecord) error
	// ExtendLease moves lease_expires_at when owner still holds a running
	// step; false when the lease was lost.
	ExtendLease(ctx context.Context, stepID kernel.ID, owner string, until time.Time) (bool, error)
	// SaveStepOutput checkpoints output_json while owner holds the lease.
	SaveStepOutput(ctx context.Context, stepID kernel.ID, owner string, output json.RawMessage) (bool, error)

	AppendEvent(ctx context.Context, e *EventRecord) error
	// ListEvents returns the newest events of a job (oldest first), at most limit.
	ListEvents(ctx context.Context, jobID kernel.ID, limit int) ([]EventRecord, error)

	InsertOutbox(ctx context.Context, o *OutboxRecord) error
	// ClaimOutbox locks due, unpublished rows (FOR UPDATE SKIP LOCKED).
	ClaimOutbox(ctx context.Context, now time.Time, limit int) ([]OutboxRecord, error)
	MarkOutboxPublished(ctx context.Context, ids []int64, at time.Time) error
	// OldestUnpublishedOutbox returns available_at of the oldest due row.
	OldestUnpublishedOutbox(ctx context.Context, now time.Time) (*time.Time, error)
	DeletePublishedOutbox(ctx context.Context, before time.Time) (int64, error)

	// ListExpiredLeases: running steps whose lease expired before now.
	ListExpiredLeases(ctx context.Context, now time.Time, limit int) ([]StepRecord, error)
	// ListOverdueWaiting: waiting_provider steps whose next poll is before `before`.
	ListOverdueWaiting(ctx context.Context, before time.Time, limit int) ([]StepRecord, error)
	// ListStaleQueued: queued steps due before `before` with no unpublished outbox row.
	ListStaleQueued(ctx context.Context, before time.Time, limit int) ([]StepRecord, error)
	CountNeedsReconciliation(ctx context.Context) (int, error)
	// TryLock takes a transaction-scoped advisory lock (single reconciler).
	TryLock(ctx context.Context, key int64) (bool, error)
}

// TaskQueue enqueues a step task (asynq). ErrDuplicateTask-style conflicts
// must be reported as nil: the task is already queued.
type TaskQueue interface {
	Enqueue(ctx context.Context, t Task) error
}

// Task is a queue message produced by the dispatcher.
type Task struct {
	Queue     string
	ID        string
	Payload   []byte
	ProcessIn time.Duration
	Timeout   time.Duration
}

// Semaphore is a lease-based counting semaphore (Redis).
type Semaphore interface {
	TryAcquire(ctx context.Context, key, holder string, limit int, ttl time.Duration) (bool, error)
	Release(ctx context.Context, key, holder string) error
}

// RateLimiter is a fixed-window counter (Redis).
type RateLimiter interface {
	// Allow consumes one unit of key; ok=false with retryAfter when over limit.
	Allow(ctx context.Context, key string, limit int, window time.Duration) (ok bool, retryAfter time.Duration, err error)
}

// ConcurrencyLimitFunc returns ai_models.concurrency_limit for a provider
// model used on queue q (0 = unlimited / unknown).
type ConcurrencyLimitFunc func(ctx context.Context, q Queue, providerCode, modelCode string) (int, error)

// Metrics receives engine measurements (nil-safe: Service checks).
type Metrics interface {
	OutboxLag(seconds float64)
	JobFinished(jobType string, status string)
	StepDuration(stepType, provider string, d time.Duration)
	ProviderError(provider, kind string)
}
