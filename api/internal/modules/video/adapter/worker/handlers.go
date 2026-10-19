// Package worker exposes the Video Studio step handlers to the job engine
// (SPEC-02 §5.4 queues, §5.3 timeouts). The logic lives in usecase.
package worker

import (
	"context"
	"time"

	jobuc "github.com/be/acs/api/internal/modules/job/usecase"
	"github.com/be/acs/api/internal/modules/video/domain"
	"github.com/be/acs/api/internal/modules/video/usecase"
)

type handler struct {
	typ     string
	queue   jobuc.Queue
	timeout time.Duration
	run     func(context.Context, jobuc.StepContext) (jobuc.StepResult, error)
}

func (h handler) StepType() string       { return h.typ }
func (h handler) Queue() jobuc.Queue     { return h.queue }
func (h handler) Timeout() time.Duration { return h.timeout }
func (h handler) Handle(ctx context.Context, sc jobuc.StepContext) (jobuc.StepResult, error) {
	return h.run(ctx, sc)
}

// Handlers returns every video step handler.
func Handlers(svc *usecase.Service) []jobuc.StepHandler {
	return []jobuc.StepHandler{
		handler{domain.StepScript, jobuc.QueueText, 120 * time.Second, svc.RunScript},
		handler{domain.StepStoryboard, jobuc.QueueText, 120 * time.Second, svc.RunStoryboard},
		// One attempt = submit or one poll + download + normalize; polling
		// itself is asynchronous (WaitSpec), bounded by the engine (30 min).
		handler{domain.StepScene, jobuc.QueueVideo, 5 * time.Minute, svc.RunScene},
		handler{domain.StepVoiceover, jobuc.QueueRender, 5 * time.Minute, svc.RunVoiceover},
		handler{domain.StepSubtitle, jobuc.QueueRender, time.Minute, svc.RunSubtitles},
		handler{domain.StepRender, jobuc.QueueRender, 15 * time.Minute, svc.RunRender},
		// Includes at most one re-render.
		handler{domain.StepQC, jobuc.QueueRender, 15 * time.Minute, svc.RunQualityCheck},
		handler{domain.StepPublish, jobuc.QueueRender, time.Minute, svc.RunPublish},
	}
}
