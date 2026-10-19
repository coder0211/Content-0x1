package runtime_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/be/acs/api/internal/modules/job/adapter/runtime"
	"github.com/be/acs/api/internal/modules/job/usecase"
	"github.com/be/acs/api/internal/platform/queue"
	"github.com/be/acs/api/internal/shared/kernel"
)

type enq struct {
	got []queue.Task
	err error
}

func (e *enq) Enqueue(_ context.Context, t queue.Task) error {
	e.got = append(e.got, t)
	return e.err
}

func TestQueueMapsTaskAndIgnoresDuplicates(t *testing.T) {
	e := &enq{}
	q := runtime.Queue{C: e}
	if err := q.Enqueue(context.Background(), usecase.Task{Queue: "text", ID: "s:1", Payload: []byte(`{}`)}); err != nil {
		t.Fatal(err)
	}
	if e.got[0].Type != usecase.TaskTypeStep || e.got[0].ID != "s:1" || e.got[0].Queue != "text" {
		t.Fatalf("%+v", e.got[0])
	}
	e.err = queue.ErrDuplicate
	if err := q.Enqueue(context.Background(), usecase.Task{ID: "s:1"}); err != nil {
		t.Fatalf("duplicate must be nil: %v", err)
	}
	e.err = errors.New("down")
	if err := q.Enqueue(context.Background(), usecase.Task{ID: "s:2"}); err == nil {
		t.Fatal("real errors must surface")
	}
}

type exec struct{ got []usecase.TaskPayload }

func (x *exec) Execute(_ context.Context, p usecase.TaskPayload) error {
	x.got = append(x.got, p)
	return nil
}

func TestHandleTask(t *testing.T) {
	x := &exec{}
	h := runtime.HandleTask(x, slog.New(slog.NewTextHandler(io.Discard, nil)))
	p := usecase.TaskPayload{JobID: kernel.NewID(), StepID: kernel.NewID(), Attempt: 1, Kind: usecase.TaskRun}
	b, _ := json.Marshal(p)
	if err := h(context.Background(), queue.Task{Payload: b}); err != nil || len(x.got) != 1 || x.got[0] != p {
		t.Fatalf("%v %+v", err, x.got)
	}
	if err := h(context.Background(), queue.Task{Payload: []byte("nope")}); err != nil || len(x.got) != 1 {
		t.Fatal("malformed payload must be dropped")
	}
}
