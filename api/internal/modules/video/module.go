// Package video wires Video Studio (M06): projects, storyboard editing,
// estimate + budget gate, the production / regenerate / render DAGs and
// their step handlers (SPEC-02 §5.5–5.6).
package video

import (
	"github.com/go-chi/chi/v5"

	"github.com/be/acs/api/internal/app/registry"
	jobuc "github.com/be/acs/api/internal/modules/job/usecase"
	videohttp "github.com/be/acs/api/internal/modules/video/adapter/http"
	"github.com/be/acs/api/internal/modules/video/adapter/postgres"
	"github.com/be/acs/api/internal/modules/video/adapter/worker"
	"github.com/be/acs/api/internal/modules/video/usecase"
)

// Module is the video module.
type Module struct {
	Svc     *usecase.Service
	handler *videohttp.Handler
}

// Name implements registry.Module.
func (*Module) Name() string { return "video" }

// Routes implements registry.HTTPModule.
func (m *Module) Routes(r chi.Router) { m.handler.Routes(r) }

// StepHandlers implements registry.StepModule.
func (m *Module) StepHandlers() []jobuc.StepHandler { return worker.Handlers(m.Svc) }

// Register builds the module. Video publishes no API (no module depends on
// it). Drivers are only stored, never called, here.
func Register(c *registry.Container) (registry.Module, error) {
	svc := &usecase.Service{
		Tx: c.Tx, Repo: postgres.NewRepository(c.DB), Clock: c.Clock, Audit: c.APIs.Audit,
		Jobs: c.APIs.Jobs, Budget: c.APIs.Budget, Assets: c.APIs.Assets, Brands: c.APIs.Brands, Campaigns: c.APIs.Campaigns,
		Providers: c.APIs.Providers, Content: c.APIs.Content, Files: c.APIs.Files, AI: c.AI, Renderer: c.Renderer, Prober: c.Prober,
		Logger: c.Logger, TempDir: c.Config.FFmpeg.TempDir, FontsDir: c.Config.FFmpeg.FontsDir,
	}
	if c.Config.EnableFakeProviders {
		svc.StubVideoProviders = []string{"fake"}
	}
	return &Module{Svc: svc, handler: &videohttp.Handler{Svc: svc}}, nil
}
