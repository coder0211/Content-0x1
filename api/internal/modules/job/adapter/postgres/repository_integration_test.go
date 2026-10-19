package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/be/acs/api/internal/modules/job/adapter/postgres"
	"github.com/be/acs/api/internal/modules/job/usecase"
	db "github.com/be/acs/api/internal/platform/postgres"
	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
	"github.com/be/acs/api/internal/testutil/pgtest"
)

type queue struct {
	mu    sync.Mutex
	tasks []usecase.Task
}

func (q *queue) Enqueue(_ context.Context, t usecase.Task) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.tasks = append(q.tasks, t)
	return nil
}

type gen struct{ calls int }

func (g *gen) StepType() string       { return "gen" }
func (g *gen) Queue() usecase.Queue   { return usecase.QueueText }
func (g *gen) Timeout() time.Duration { return 5 * time.Second }
func (g *gen) Handle(context.Context, usecase.StepContext) (usecase.StepResult, error) {
	g.calls++
	if g.calls == 1 {
		return usecase.StepResult{}, &ports.ProviderError{Kind: ports.ErrKindTransient}
	}
	return usecase.StepResult{Output: map[string]string{"text": "xin chào"}, JobResult: map[string]any{"asset_ids": []string{"a"}}}, nil
}

// Full engine round trip on Postgres: create (idempotent), outbox
// dispatch with SKIP LOCKED, lease, retry with backoff, DAG, events.
func TestEngineOnPostgres(t *testing.T) {
	pool := pgtest.Postgres(t)
	ctx := context.Background()
	var userID kernel.ID
	if err := pool.QueryRow(ctx, `INSERT INTO users (email, name) VALUES ('job@acs.local', 'Job') RETURNING id`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	reg := usecase.NewStepRegistry()
	g := &gen{}
	if err := reg.Register(g); err != nil {
		t.Fatal(err)
	}
	q := &queue{}
	clock := &ports.SystemClock{}
	svc := &usecase.Service{
		Tx: db.NewTxManager(pool), Repo: postgres.NewRepository(pool), Clock: clock, Steps: reg, Queue: q,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Rand: func() float64 { return 0 },
	}
	spec := usecase.JobSpec{
		Type: usecase.TypeContentGeneration, CreatedBy: userID, ProviderCode: "fake", ModelCode: "m",
		Request: json.RawMessage(`{"a":1}`), CostEstimate: kernel.USD("0.25"), IdempotencyKey: "k", RequestHash: "h",
		Steps: []usecase.StepSpec{{Key: "a", Type: "gen"}, {Key: "b", Type: "gen", DependsOn: []string{"a"}}},
	}
	res, err := svc.Create(ctx, spec)
	if err != nil {
		t.Fatal(err)
	}
	again, err := svc.Create(ctx, spec)
	if err != nil || !again.Existing || again.JobID != res.JobID {
		t.Fatalf("replay %+v %v", again, err)
	}
	spec.RequestHash = "other"
	if _, err := svc.Create(ctx, spec); !errors.Is(err, kernel.ErrIdempotencyConflict) {
		t.Fatalf("conflict: %v", err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := svc.DispatchOnce(ctx); err != nil {
			t.Fatal(err)
		}
		q.mu.Lock()
		tasks := q.tasks
		q.tasks = nil
		q.mu.Unlock()
		for _, task := range tasks {
			var p usecase.TaskPayload
			_ = json.Unmarshal(task.Payload, &p)
			if err := svc.Execute(ctx, p); err != nil {
				t.Fatal(err)
			}
		}
		v, err := svc.Get(ctx, res.JobID)
		if err != nil {
			t.Fatal(err)
		}
		if v.Status.IsTerminal() {
			if v.Status != usecase.StatusSucceeded || v.Steps[0].AttemptCount != 2 || v.Progress != 100 {
				t.Fatalf("job %+v", v)
			}
			if v.CostEstimate == nil || !v.CostEstimate.Amount.Equal(kernel.USD("0.25").Amount) {
				t.Fatalf("estimate %v", v.CostEstimate)
			}
			evs, err := svc.History(ctx, kernel.Actor{UserID: userID}, res.JobID, 0)
			if err != nil || len(evs) < 5 {
				t.Fatalf("events %d %v", len(evs), err)
			}
			rep, err := svc.ReconcileOnce(ctx)
			if err != nil || rep.Skipped {
				t.Fatalf("reconcile %+v %v", rep, err)
			}
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("job did not finish")
}
