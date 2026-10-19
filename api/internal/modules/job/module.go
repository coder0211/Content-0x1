// Package job wires the job module: job/step persistence, outbox
// dispatcher, worker runtime, reconciler, job API + SSE (SPEC-02 §5).
package job

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/be/acs/api/internal/app/registry"
	jobhttp "github.com/be/acs/api/internal/modules/job/adapter/http"
	"github.com/be/acs/api/internal/modules/job/adapter/postgres"
	"github.com/be/acs/api/internal/modules/job/adapter/runtime"
	"github.com/be/acs/api/internal/modules/job/usecase"
	"github.com/be/acs/api/internal/platform/queue"
	"github.com/be/acs/api/internal/platform/redis"
	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Background loop periods (SPEC-02 §5.3).
const (
	DispatchInterval  = time.Second
	ReconcileInterval = 5 * time.Minute
)

// Module is the job module.
type Module struct {
	// Steps holds every StepHandler registered by other modules.
	Steps   *usecase.StepRegistry
	Svc     *usecase.Service
	handler *jobhttp.Handler
	c       *registry.Container
}

var (
	_ registry.HTTPModule   = (*Module)(nil)
	_ registry.RunnerModule = (*Module)(nil)
)

// Name implements registry.Module.
func (*Module) Name() string { return "job" }

var queueKinds = map[usecase.Queue]ports.AIKind{
	usecase.QueueText: ports.AIKindText, usecase.QueueImage: ports.AIKindImage, usecase.QueueVideo: ports.AIKindVideo,
}

// Register publishes APIs.Jobs and APIs.Steps. Drivers are not touched
// here (they may be nil in tests); runners connect lazily.
func Register(c *registry.Container) (registry.Module, error) {
	steps := usecase.NewStepRegistry()
	host, _ := os.Hostname()
	svc := &usecase.Service{
		Tx: c.Tx, Repo: postgres.NewRepository(c.DB), Clock: c.Clock, Steps: steps, Events: c.Events,
		Logger: c.Logger, WorkerID: fmt.Sprintf("%s:%d", host, os.Getpid()),
	}
	if c.APIs.Audit != nil {
		svc.Notifier = c.APIs.Audit
	}
	if c.Queue != nil {
		svc.Queue = runtime.Queue{C: c.Queue}
	}
	if c.Redis != nil {
		svc.Sem = redis.NewSemaphore(c.Redis)
		svc.Limiter = runtime.RateLimiter{C: c.Redis}
	}
	if c.Metrics != nil {
		svc.Metrics = runtime.Metrics{M: c.Metrics}
	}
	apis := &c.APIs
	// ai_models.concurrency_limit through the provider module (ACS-043).
	svc.ConcurrencyLimit = func(ctx context.Context, q usecase.Queue, providerCode, modelCode string) (int, error) {
		kind, ok := queueKinds[q]
		if !ok || apis.Providers == nil {
			return 0, nil
		}
		m, err := apis.Providers.ResolveModel(ctx, kind, modelCode)
		if err != nil || m.ProviderCode != providerCode {
			return 0, err
		}
		return m.ConcurrencyLimit, nil
	}
	// Pending uploads older than 24h (OQ-06) are purged by the file module.
	svc.PurgeStaleUploads = func(ctx context.Context, olderThan time.Duration) (int, error) {
		if apis.Files == nil {
			return 0, nil
		}
		return apis.Files.PurgeStalePending(ctx, olderThan)
	}
	c.APIs.Jobs = svc
	c.APIs.Steps = steps
	return &Module{Steps: steps, Svc: svc, handler: &jobhttp.Handler{Svc: svc, Subscriber: c.Subscriber, UserNames: userNames(apis)}, c: c}, nil
}

// Routes implements registry.HTTPModule.
func (m *Module) Routes(r chi.Router) { m.handler.Routes(r) }

// Runners implements registry.RunnerModule. Workers run the outbox
// dispatcher (every worker; FOR UPDATE SKIP LOCKED makes that safe), the
// reconciler (advisory lock: one instance per round) and the queue
// consumer for the configured queues. The API server runs none: it only
// writes outbox rows.
func (m *Module) Runners(p registry.Process) []registry.Runner {
	if p != registry.ProcessWorker {
		return nil
	}
	return []registry.Runner{
		{Name: "job-dispatcher", Run: func(ctx context.Context) error { return m.Svc.RunDispatcher(ctx, DispatchInterval) }},
		{Name: "job-reconciler", Run: func(ctx context.Context) error { return m.Svc.RunReconciler(ctx, ReconcileInterval) }},
		{Name: "job-consumer", Run: m.consume},
	}
}

func (m *Module) consume(ctx context.Context) error {
	cfg := m.c.Config
	srv, err := queue.NewServer(queue.ServerConfig{
		RedisURL: cfg.RedisURL, Queues: cfg.Worker.Queues, Concurrency: cfg.Worker.Concurrency, Logger: m.c.Logger,
	})
	if err != nil {
		return err
	}
	logger := m.c.Logger
	if logger == nil {
		logger = slog.Default()
	}
	srv.Handle(usecase.TaskTypeStep, runtime.HandleTask(m.Svc, logger))
	return srv.Run(ctx)
}

// userNames resolves creator names through the identity API (resolved
// lazily: identity may register after job). Lookup errors leave names empty.
func userNames(apis *registry.APIs) func(ctx context.Context, ids []kernel.ID) map[kernel.ID]string {
	return func(ctx context.Context, ids []kernel.ID) map[kernel.ID]string {
		out := map[kernel.ID]string{}
		if apis.Identity == nil {
			return out
		}
		uniq := slices.Clone(ids)
		slices.SortFunc(uniq, func(a, b kernel.ID) int { return slices.Compare(a[:], b[:]) })
		users, err := apis.Identity.GetUsers(ctx, slices.Compact(uniq))
		if err != nil {
			return out
		}
		for _, u := range users {
			out[u.ID] = u.Name
		}
		return out
	}
}
