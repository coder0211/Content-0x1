package worker_test

import (
	"testing"
	"time"

	"github.com/be/acs/api/internal/modules/image/adapter/worker"
	"github.com/be/acs/api/internal/modules/image/usecase"
	jobuc "github.com/be/acs/api/internal/modules/job/usecase"
)

func TestImageStepRegistration(t *testing.T) {
	s := &worker.ImageStep{Svc: &usecase.Service{}}
	if s.StepType() != usecase.StepTypeImage || s.Queue() != jobuc.QueueImage || s.Timeout() != 300*time.Second {
		t.Fatalf("%s %s %s", s.StepType(), s.Queue(), s.Timeout())
	}
}
