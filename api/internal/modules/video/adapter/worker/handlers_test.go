package worker_test

import (
	"testing"

	jobuc "github.com/be/acs/api/internal/modules/job/usecase"
	"github.com/be/acs/api/internal/modules/video/adapter/worker"
	"github.com/be/acs/api/internal/modules/video/domain"
	"github.com/be/acs/api/internal/modules/video/usecase"
)

func TestHandlersCoverEveryDAGStep(t *testing.T) {
	hs := worker.Handlers(&usecase.Service{})
	byType := map[string]jobuc.StepHandler{}
	for _, h := range hs {
		if _, dup := byType[h.StepType()]; dup {
			t.Fatalf("duplicate handler %s", h.StepType())
		}
		if h.Timeout() <= 0 {
			t.Errorf("%s: no timeout", h.StepType())
		}
		byType[h.StepType()] = h
	}
	nodes := append(domain.StoryboardSteps(), domain.RenderSteps(domain.RenderPlan{Voice: true, Subtitles: true})...)
	nodes = append(nodes, domain.StepNode{Type: domain.StepScene})
	for _, n := range nodes {
		if byType[n.Type] == nil {
			t.Errorf("no handler for %s", n.Type)
		}
	}
	want := map[string]jobuc.Queue{domain.StepScript: jobuc.QueueText, domain.StepScene: jobuc.QueueVideo, domain.StepRender: jobuc.QueueRender, domain.StepQC: jobuc.QueueRender}
	for typ, q := range want {
		if byType[typ].Queue() != q {
			t.Errorf("%s queue %s, want %s", typ, byType[typ].Queue(), q)
		}
	}
}
