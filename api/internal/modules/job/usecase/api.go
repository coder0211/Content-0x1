// Package usecase is the public contract of the job module (SPEC-06 §2.4).
// Other modules import ONLY this package (never job/adapter): they create
// jobs through API and execute their steps by registering StepHandlers.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/be/acs/api/internal/shared/kernel"
)

// JobType mirrors ai_jobs.job_type.
type JobType string

const (
	TypeContentGeneration    JobType = "content_generation"
	TypeContentRewrite       JobType = "content_rewrite"
	TypeContentTransform     JobType = "content_transform"
	TypeContentPlan          JobType = "content_plan"
	TypeImageGeneration      JobType = "image_generation"
	TypeImageEdit            JobType = "image_edit"
	TypeImagePromptSuggest   JobType = "image_prompt_suggest"
	TypeVideoStoryboard      JobType = "video_storyboard"
	TypeVideoProduction      JobType = "video_production"
	TypeVideoSceneRegenerate JobType = "video_scene_regenerate"
	TypeVideoRender          JobType = "video_render"
)

// Queue is an asynq queue name (SPEC-02 §5.4). Values equal
// platform/queue constants (a test enforces it).
type Queue string

const (
	QueueText        Queue = "text"
	QueueImage       Queue = "image"
	QueueVideo       Queue = "video"
	QueueRender      Queue = "render"
	QueueMaintenance Queue = "maintenance"
)

// AllQueues lists every queue.
var AllQueues = []Queue{QueueText, QueueImage, QueueVideo, QueueRender, QueueMaintenance}

// Status mirrors ai_jobs.status.
type Status string

const (
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

// IsTerminal reports whether no further transition is possible.
func (s Status) IsTerminal() bool {
	return s == StatusSucceeded || s == StatusFailed || s == StatusCancelled
}

// StepStatus mirrors ai_job_steps.status.
type StepStatus string

const (
	StepPending         StepStatus = "pending"
	StepQueued          StepStatus = "queued"
	StepRunning         StepStatus = "running"
	StepWaitingProvider StepStatus = "waiting_provider"
	StepSucceeded       StepStatus = "succeeded"
	StepFailed          StepStatus = "failed"
	StepCancelled       StepStatus = "cancelled"
	StepSkipped         StepStatus = "skipped"
)

// StepSpec declares one node of the job DAG.
type StepSpec struct {
	// Key is unique within the job, e.g. "scene_generation:3".
	Key string
	// Type selects the StepHandler, e.g. "scene_generation".
	Type string
	// DependsOn lists step Keys that must succeed first.
	DependsOn []string
	Input     json.RawMessage
	// MaxAttempts overrides the default (3; 5 for video polling) when > 0.
	MaxAttempts int
	SceneID     *kernel.ID
}

// JobSpec is everything needed to create a job (one user request).
type JobSpec struct {
	Type             JobType
	CreatedBy        kernel.ID
	CampaignID       *kernel.ID
	AssetID          *kernel.ID
	BrandVersionID   *kernel.ID
	PromptTemplateID *kernel.ID
	ProviderCode     string
	ModelCode        string
	// Request is the validated request parameters (ai_jobs.request_json).
	Request        json.RawMessage
	IdempotencyKey string // from httpx.IdempotencyFrom; required for user-initiated jobs
	RequestHash    string
	CostEstimate   kernel.Money
	PricingVersion string
	// BudgetOverrideBy is set when a Lead/Admin overrode the per-job limit.
	BudgetOverrideBy *kernel.ID
	Steps            []StepSpec
}

// CreateResult is returned by API.Create.
type CreateResult struct {
	JobID  kernel.ID
	Status Status
	// Existing is true when (CreatedBy, IdempotencyKey) matched an earlier job
	// with the same RequestHash: nothing was created and the caller MUST NOT
	// reserve budget again — just return the existing job.
	Existing     bool
	CostEstimate kernel.Money
}

// StepView is a read model of one step.
type StepView struct {
	ID           kernel.ID
	Key          string
	Type         string
	Status       StepStatus
	AttemptCount int
	ErrorKind    string
	ErrorMessage string
	Output       json.RawMessage
	StartedAt    *time.Time
	FinishedAt   *time.Time
}

// JobView is a read model of a job (GET /jobs/{id}).
type JobView struct {
	ID               kernel.ID
	Type             JobType
	Status           Status
	Progress         int
	CreatedBy        kernel.ID
	CampaignID       *kernel.ID
	AssetID          *kernel.ID
	BrandVersionID   *kernel.ID
	PromptTemplateID *kernel.ID
	ProviderCode     string
	ModelCode        string
	Request          json.RawMessage
	Result           json.RawMessage
	CostEstimate     *kernel.Money
	CostActual       *kernel.Money
	CostSource       string
	ErrorCode        string
	ErrorMessage     string
	CancelRequested  bool
	Steps            []StepView
	CreatedAt        time.Time
	StartedAt        *time.Time
	FinishedAt       *time.Time
}

// JobFinished is emitted when a job reaches a terminal status. Hooks run in
// the transaction that finalizes the job.
type JobFinished struct {
	JobID      kernel.ID
	Type       JobType
	Status     Status
	CreatedBy  kernel.ID
	CampaignID *kernel.ID
	ErrorCode  string
	FinishedAt time.Time
}

// JobRetried is emitted when POST /jobs/{id}/retry re-opens a failed job.
// Hooks run inside the retry transaction; returning an error (e.g.
// BUDGET_EXCEEDED from a new reservation) aborts the retry.
type JobRetried struct {
	JobID        kernel.ID
	Type         JobType
	CreatedBy    kernel.ID
	CampaignID   *kernel.ID
	CostEstimate kernel.Money
	Actor        kernel.Actor
}

// API is the job module's public interface.
type API interface {
	// Create inserts job + steps + outbox rows. It MUST be called inside the
	// caller's ports.TxManager transaction so it commits atomically with the
	// budget reservation. Idempotency: same (CreatedBy, IdempotencyKey) and
	// same RequestHash → Existing=true; different hash → IDEMPOTENCY_CONFLICT.
	Create(ctx context.Context, spec JobSpec) (CreateResult, error)
	// Get returns a job without authorization checks (for other modules).
	Get(ctx context.Context, id kernel.ID) (JobView, error)
	// RequestCancel marks cancel_requested (owner or admin); 409 when terminal.
	RequestCancel(ctx context.Context, actor kernel.Actor, id kernel.ID) error
	// OnFinished registers a hook (composition root wires budget settlement).
	OnFinished(func(ctx context.Context, ev JobFinished) error)
	// OnRetried registers a hook (composition root wires a new reservation).
	OnRetried(func(ctx context.Context, ev JobRetried) error)
}

// JobInfo is the job-level context available to a step handler.
type JobInfo struct {
	ID               kernel.ID
	Type             JobType
	CreatedBy        kernel.ID
	CampaignID       *kernel.ID
	AssetID          *kernel.ID
	BrandVersionID   *kernel.ID
	PromptTemplateID *kernel.ID
	ProviderCode     string
	ModelCode        string
	Request          json.RawMessage
}

// StepContext is what the engine hands to a StepHandler for one attempt.
type StepContext interface {
	Job() JobInfo
	StepID() kernel.ID
	StepKey() string
	StepType() string
	SceneID() *kernel.ID
	// Attempt is 1-based.
	Attempt() int
	Input() json.RawMessage
	DecodeInput(v any) error
	// DependencyOutputs returns the outputs of DependsOn steps by step key.
	DependencyOutputs() map[string]json.RawMessage
	// ProviderJobID is set when resuming a step that returned Wait.
	ProviderJobID() string
	// SaveOutput checkpoints partial output (ai_job_steps.output_json) so a
	// retry does not redo completed provider calls.
	SaveOutput(ctx context.Context, v any) error
	// Progress publishes step progress (0–100) and an optional message.
	Progress(ctx context.Context, pct int, message string) error
	// CancelRequested reports whether the user asked to cancel the job.
	CancelRequested(ctx context.Context) (bool, error)
}

// WaitSpec asks the engine to park the step in waiting_provider and run it
// again after PollAfter (asynchronous provider polling; never block a worker).
type WaitSpec struct {
	ProviderJobID string
	PollAfter     time.Duration
}

// StepResult is the outcome of a successful Handle call.
type StepResult struct {
	// Output is persisted to ai_job_steps.output_json (JSON-marshalled).
	Output any
	// Wait, when set, re-enqueues the step instead of completing it.
	Wait *WaitSpec
	// JobResult, when set, is merged into ai_jobs.result_json (e.g.
	// {"asset_ids": [...]}) — typically by the last step.
	JobResult map[string]any
}

// StepHandler executes one step type. Error semantics:
//   - *ports.ProviderError: retried only when Retryable() (Transient,
//     RateLimited) with backoff, up to max attempts; otherwise the step fails.
//   - Permanent(err) or *kernel.DomainError: the step fails without retry.
//   - any other error (DB down, timeout): treated as transient.
type StepHandler interface {
	StepType() string
	Queue() Queue
	// Timeout bounds one attempt (text 120s, image 300s, render 15m...).
	Timeout() time.Duration
	Handle(ctx context.Context, sc StepContext) (StepResult, error)
}

// StepRegistrar receives the StepHandlers of every module at start-up.
type StepRegistrar interface {
	Register(h StepHandler) error
}

type permanentError struct{ err error }

func (p permanentError) Error() string { return p.err.Error() }
func (p permanentError) Unwrap() error { return p.err }

// Permanent marks err as non-retryable.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return permanentError{err: err}
}

// IsPermanent reports whether err was marked with Permanent or is a domain
// error.
func IsPermanent(err error) bool {
	var p permanentError
	if errors.As(err, &p) {
		return true
	}
	_, ok := kernel.AsDomainError(err)
	return ok
}
