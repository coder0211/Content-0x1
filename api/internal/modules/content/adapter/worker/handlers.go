// Package worker exposes the content step handlers to the job engine.
package worker

import (
	"context"
	"time"

	"github.com/be/acs/api/internal/modules/content/usecase"
	jobuc "github.com/be/acs/api/internal/modules/job/usecase"
)

// Executor runs a content step (usecase.Service).
type Executor interface {
	ExecuteStep(ctx context.Context, sc jobuc.StepContext) (jobuc.StepResult, error)
}

// Handler is one content step type on the text queue.
type Handler struct {
	Type string
	Exec Executor
}

// StepType implements jobuc.StepHandler.
func (h Handler) StepType() string { return h.Type }

// Queue implements jobuc.StepHandler.
func (Handler) Queue() jobuc.Queue { return jobuc.QueueText }

// Timeout implements jobuc.StepHandler (text calls: 120s).
func (Handler) Timeout() time.Duration { return 120 * time.Second }

// Handle implements jobuc.StepHandler.
func (h Handler) Handle(ctx context.Context, sc jobuc.StepContext) (jobuc.StepResult, error) {
	return h.Exec.ExecuteStep(ctx, sc)
}

// Handlers returns a handler for every content step type.
func Handlers(exec Executor) []jobuc.StepHandler {
	types := []string{usecase.StepGeneration, usecase.StepRewrite, usecase.StepTransform, usecase.StepPlan, usecase.StepImagePrompts}
	out := make([]jobuc.StepHandler, 0, len(types))
	for _, t := range types {
		out = append(out, Handler{Type: t, Exec: exec})
	}
	return out
}
