// Package jobtest provides in-memory implementations of the job contract:
// a fake API for use-case tests of other modules (and as the placeholder
// until the job module agent ships the real engine) and a fake StepContext
// for StepHandler tests.
package jobtest

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/be/acs/api/internal/modules/job/usecase"
	"github.com/be/acs/api/internal/shared/kernel"
)

// API is an in-memory usecase.API.
type API struct {
	mu       sync.Mutex
	Jobs     map[kernel.ID]*usecase.JobView
	Specs    map[kernel.ID]usecase.JobSpec
	idem     map[string]kernel.ID
	finished []func(context.Context, usecase.JobFinished) error
	retried  []func(context.Context, usecase.JobRetried) error
}

// NewAPI returns an empty fake.
func NewAPI() *API {
	return &API{Jobs: map[kernel.ID]*usecase.JobView{}, Specs: map[kernel.ID]usecase.JobSpec{}, idem: map[string]kernel.ID{}}
}

var _ usecase.API = (*API)(nil)

// Create stores the job, honouring idempotency semantics.
func (a *API) Create(_ context.Context, s usecase.JobSpec) (usecase.CreateResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(s.Steps) == 0 {
		return usecase.CreateResult{}, kernel.Validation(kernel.FieldIssue("steps", "required"))
	}
	if s.IdempotencyKey != "" {
		k := s.CreatedBy.String() + "/" + s.IdempotencyKey
		if id, ok := a.idem[k]; ok {
			if a.Specs[id].RequestHash != s.RequestHash {
				return usecase.CreateResult{}, kernel.ErrIdempotencyConflict
			}
			return usecase.CreateResult{JobID: id, Status: a.Jobs[id].Status, Existing: true, CostEstimate: s.CostEstimate}, nil
		}
	}
	id := kernel.NewID()
	steps := make([]usecase.StepView, 0, len(s.Steps))
	for _, st := range s.Steps {
		steps = append(steps, usecase.StepView{ID: kernel.NewID(), Key: st.Key, Type: st.Type, Status: usecase.StepPending})
	}
	est := s.CostEstimate
	a.Jobs[id] = &usecase.JobView{
		ID: id, Type: s.Type, Status: usecase.StatusQueued, CreatedBy: s.CreatedBy, CampaignID: s.CampaignID,
		AssetID: s.AssetID, BrandVersionID: s.BrandVersionID, PromptTemplateID: s.PromptTemplateID,
		ProviderCode: s.ProviderCode, ModelCode: s.ModelCode, Request: s.Request, CostEstimate: &est,
		Steps: steps, CreatedAt: time.Now().UTC(),
	}
	a.Specs[id] = s
	if s.IdempotencyKey != "" {
		a.idem[s.CreatedBy.String()+"/"+s.IdempotencyKey] = id
	}
	return usecase.CreateResult{JobID: id, Status: usecase.StatusQueued, CostEstimate: s.CostEstimate}, nil
}

// Get returns a copy of the job.
func (a *API) Get(_ context.Context, id kernel.ID) (usecase.JobView, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	j, ok := a.Jobs[id]
	if !ok {
		return usecase.JobView{}, kernel.ErrNotFound
	}
	return *j, nil
}

// RequestCancel flags the job.
func (a *API) RequestCancel(_ context.Context, actor kernel.Actor, id kernel.ID) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	j, ok := a.Jobs[id]
	if !ok {
		return kernel.ErrNotFound
	}
	if j.CreatedBy != actor.UserID && !actor.IsAdmin() {
		return kernel.ErrNotFound
	}
	if j.Status.IsTerminal() {
		return kernel.InvalidTransition(string(j.Status), string(usecase.StatusCancelled))
	}
	j.CancelRequested = true
	return nil
}

// OnFinished registers a hook.
func (a *API) OnFinished(f func(context.Context, usecase.JobFinished) error) {
	a.finished = append(a.finished, f)
}

// OnRetried registers a hook.
func (a *API) OnRetried(f func(context.Context, usecase.JobRetried) error) {
	a.retried = append(a.retried, f)
}

// Finish moves a job to a terminal status and runs the hooks (test helper).
func (a *API) Finish(ctx context.Context, id kernel.ID, status usecase.Status) error {
	a.mu.Lock()
	j := a.Jobs[id]
	j.Status = status
	ev := usecase.JobFinished{JobID: id, Type: j.Type, Status: status, CreatedBy: j.CreatedBy, CampaignID: j.CampaignID, FinishedAt: time.Now().UTC()}
	hooks := append([]func(context.Context, usecase.JobFinished) error(nil), a.finished...)
	a.mu.Unlock()
	for _, h := range hooks {
		if err := h(ctx, ev); err != nil {
			return err
		}
	}
	return nil
}

// StepContext is a configurable usecase.StepContext for handler tests.
type StepContext struct {
	JobInfo     usecase.JobInfo
	ID          kernel.ID
	Key         string
	Type        string
	Scene       *kernel.ID
	AttemptNo   int
	InputJSON   json.RawMessage
	Deps        map[string]json.RawMessage
	ProviderJob string
	Cancel      bool
	Saved       []json.RawMessage
	ProgressLog []int
}

var _ usecase.StepContext = (*StepContext)(nil)

func (s *StepContext) Job() usecase.JobInfo                          { return s.JobInfo }
func (s *StepContext) StepID() kernel.ID                             { return s.ID }
func (s *StepContext) StepKey() string                               { return s.Key }
func (s *StepContext) StepType() string                              { return s.Type }
func (s *StepContext) SceneID() *kernel.ID                           { return s.Scene }
func (s *StepContext) Input() json.RawMessage                        { return s.InputJSON }
func (s *StepContext) ProviderJobID() string                         { return s.ProviderJob }
func (s *StepContext) DependencyOutputs() map[string]json.RawMessage { return s.Deps }

// Attempt defaults to 1.
func (s *StepContext) Attempt() int {
	if s.AttemptNo == 0 {
		return 1
	}
	return s.AttemptNo
}

func (s *StepContext) DecodeInput(v any) error {
	if len(s.InputJSON) == 0 {
		return nil
	}
	return json.Unmarshal(s.InputJSON, v)
}

func (s *StepContext) SaveOutput(_ context.Context, v any) error {
	b, err := json.Marshal(v)
	if err == nil {
		s.Saved = append(s.Saved, b)
	}
	return err
}

func (s *StepContext) Progress(_ context.Context, pct int, _ string) error {
	s.ProgressLog = append(s.ProgressLog, pct)
	return nil
}

func (s *StepContext) CancelRequested(context.Context) (bool, error) { return s.Cancel, nil }
