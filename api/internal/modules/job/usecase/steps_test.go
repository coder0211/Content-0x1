package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/be/acs/api/internal/shared/kernel"
)

type h struct {
	t string
	q Queue
}

func (x h) StepType() string       { return x.t }
func (x h) Queue() Queue           { return x.q }
func (x h) Timeout() time.Duration { return time.Second }
func (x h) Handle(context.Context, StepContext) (StepResult, error) {
	return StepResult{}, nil
}

func TestStepRegistry(t *testing.T) {
	r := NewStepRegistry()
	if err := r.Register(h{"a", QueueText}); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(h{"a", QueueText}); err == nil {
		t.Fatal("duplicate accepted")
	}
	if err := r.Register(h{"b", "gpu"}); err == nil {
		t.Fatal("unknown queue accepted")
	}
	if _, ok := r.Lookup("a"); !ok {
		t.Fatal("lookup")
	}
}

func TestPermanent(t *testing.T) {
	if !IsPermanent(Permanent(errors.New("x"))) || IsPermanent(errors.New("x")) {
		t.Fatal("Permanent")
	}
	if !IsPermanent(kernel.ErrValidation) {
		t.Fatal("domain errors are permanent")
	}
}
