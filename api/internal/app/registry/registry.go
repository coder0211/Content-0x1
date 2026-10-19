// Package registry defines the composition contract between cmd/* and the
// business modules: the Platform (drivers built once per process), the APIs
// container (each module publishes its public API here), and the optional
// interfaces a module implements to contribute routes, step handlers or
// background runners.
package registry

import (
	"context"
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	assetuc "github.com/be/acs/api/internal/modules/asset/usecase"
	audituc "github.com/be/acs/api/internal/modules/audit/usecase"
	brand "github.com/be/acs/api/internal/modules/brand/usecase"
	budgetuc "github.com/be/acs/api/internal/modules/budget/usecase"
	campaignuc "github.com/be/acs/api/internal/modules/campaign/usecase"
	contentuc "github.com/be/acs/api/internal/modules/content/usecase"
	fileuc "github.com/be/acs/api/internal/modules/file/usecase"
	identityuc "github.com/be/acs/api/internal/modules/identity/usecase"
	jobuc "github.com/be/acs/api/internal/modules/job/usecase"
	provideruc "github.com/be/acs/api/internal/modules/provider/usecase"
	"github.com/be/acs/api/internal/platform/config"
	"github.com/be/acs/api/internal/platform/httpx"
	"github.com/be/acs/api/internal/platform/metrics"
	"github.com/be/acs/api/internal/platform/queue"
	"github.com/be/acs/api/internal/ports"
)

// Process identifies which binary is composing modules.
type Process string

const (
	ProcessServer Process = "server"
	ProcessWorker Process = "worker"
	ProcessCLI    Process = "cli"
)

// Platform holds the drivers. Fields may be nil in unit tests and in the
// CLI (e.g. Queue, Redis); modules must tolerate what their process needs.
type Platform struct {
	Process    Process
	Config     config.Config
	Logger     *slog.Logger
	DB         *pgxpool.Pool
	Tx         ports.TxManager
	Clock      ports.Clock
	Storage    ports.ObjectStorage
	Prober     ports.MediaProber
	Renderer   ports.VideoRenderer
	Events     ports.EventPublisher
	Subscriber ports.EventSubscriber
	Queue      *queue.Client
	Redis      *goredis.Client
	AI         ports.AIProviders
	Metrics    *metrics.Metrics
	Mailer     ports.Mailer
}

// APIs is the cross-module API container. A module constructor reads the
// APIs it depends on and sets its own field. Modules are constructed in the
// order of app.Modules, so dependencies must come first.
type APIs struct {
	Audit         audituc.API
	Identity      identityuc.API
	Authenticator httpx.Authenticator
	Files         fileuc.API
	Jobs          jobuc.API
	Steps         jobuc.StepRegistrar
	Budget        budgetuc.API
	Assets        assetuc.API
	Brands        brand.API
	Campaigns     campaignuc.API
	Providers     provideruc.API
	Content       contentuc.API
}

// Container is passed to every module constructor.
type Container struct {
	Platform
	APIs APIs
}

// Module is implemented by every business module.
type Module interface {
	Name() string
}

// Constructor builds a module and publishes its API into c.APIs.
type Constructor func(c *Container) (Module, error)

// HTTPModule contributes routes to the /api/v1 router (server only).
type HTTPModule interface {
	Routes(r chi.Router)
}

// StepModule contributes job step handlers (worker only).
type StepModule interface {
	StepHandlers() []jobuc.StepHandler
}

// Runner is a long-running background loop (dispatcher, reconciler, queue
// consumer). Run returns when ctx is cancelled.
type Runner struct {
	Name string
	Run  func(ctx context.Context) error
}

// RunnerModule contributes background runners for the given process.
type RunnerModule interface {
	Runners(p Process) []Runner
}
