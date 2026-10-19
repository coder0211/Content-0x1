// Package worker exposes the image_generate StepHandler to the job engine.
package worker

import (
	"context"
	"time"

	"github.com/be/acs/api/internal/modules/image/usecase"
	jobuc "github.com/be/acs/api/internal/modules/job/usecase"
)

// ImageStep runs every image job (generation, edit, variation).
type ImageStep struct{ Svc *usecase.Service }

var _ jobuc.StepHandler = (*ImageStep)(nil)

// StepType implements jobuc.StepHandler.
func (*ImageStep) StepType() string { return usecase.StepTypeImage }

// Queue implements jobuc.StepHandler.
func (*ImageStep) Queue() jobuc.Queue { return jobuc.QueueImage }

// Timeout implements jobuc.StepHandler (SPEC-02: image 300s).
func (*ImageStep) Timeout() time.Duration { return 300 * time.Second }

// Handle implements jobuc.StepHandler.
func (s *ImageStep) Handle(ctx context.Context, sc jobuc.StepContext) (jobuc.StepResult, error) {
	return s.Svc.Process(ctx, sc)
}
