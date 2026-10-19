package usecase

import (
	"fmt"
	"slices"
	"sync"
)

// StepRegistry is the in-process map step_type → handler used by the
// worker runtime.
type StepRegistry struct {
	mu       sync.RWMutex
	handlers map[string]StepHandler
}

// NewStepRegistry returns an empty registry.
func NewStepRegistry() *StepRegistry {
	return &StepRegistry{handlers: map[string]StepHandler{}}
}

var _ StepRegistrar = (*StepRegistry)(nil)

// Register adds h; duplicate step types and unknown queues are errors.
func (r *StepRegistry) Register(h StepHandler) error {
	if !slices.Contains(AllQueues, h.Queue()) {
		return fmt.Errorf("step %q: unknown queue %q", h.StepType(), h.Queue())
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.handlers[h.StepType()]; dup {
		return fmt.Errorf("step %q registered twice", h.StepType())
	}
	r.handlers[h.StepType()] = h
	return nil
}

// Lookup returns the handler for stepType.
func (r *StepRegistry) Lookup(stepType string) (StepHandler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	h, ok := r.handlers[stepType]
	return h, ok
}

// Types lists registered step types (sorted).
func (r *StepRegistry) Types() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.handlers))
	for t := range r.handlers {
		out = append(out, t)
	}
	slices.Sort(out)
	return out
}
