package worker_test

import (
	"context"
	"testing"

	"github.com/be/acs/api/internal/modules/content/adapter/worker"
	jobuc "github.com/be/acs/api/internal/modules/job/usecase"
)

type exec struct{ calls []string }

func (e *exec) ExecuteStep(_ context.Context, sc jobuc.StepContext) (jobuc.StepResult, error) {
	e.calls = append(e.calls, sc.StepType())
	return jobuc.StepResult{}, nil
}

func TestHandlersRegisterEveryStep(t *testing.T) {
	reg := jobuc.NewStepRegistry()
	e := &exec{}
	for _, h := range worker.Handlers(e) {
		if h.Queue() != jobuc.QueueText || h.Timeout() <= 0 {
			t.Fatal(h.StepType())
		}
		if err := reg.Register(h); err != nil {
			t.Fatal(err)
		}
	}
	if len(worker.Handlers(e)) != 5 {
		t.Fatal("expected 5 step types")
	}
}
