// Package image wires Image Studio (SPEC-01 M05, ACS-051).
package image

import (
	"github.com/go-chi/chi/v5"

	"github.com/be/acs/api/internal/app/registry"
	nethttp "github.com/be/acs/api/internal/modules/image/adapter/http"
	"github.com/be/acs/api/internal/modules/image/adapter/postgres"
	"github.com/be/acs/api/internal/modules/image/adapter/worker"
	"github.com/be/acs/api/internal/modules/image/usecase"
	jobuc "github.com/be/acs/api/internal/modules/job/usecase"
)

// Module is the image module.
type Module struct {
	handler *nethttp.Handler
	step    *worker.ImageStep
}

// Name implements registry.Module.
func (*Module) Name() string { return "image" }

// Routes implements registry.HTTPModule.
func (m *Module) Routes(r chi.Router) { m.handler.Routes(r) }

// StepHandlers implements registry.StepModule.
func (m *Module) StepHandlers() []jobuc.StepHandler { return []jobuc.StepHandler{m.step} }

// Register builds the service from published APIs. Image Studio publishes no
// API to other modules (registry.APIs has no image field).
func Register(c *registry.Container) (registry.Module, error) {
	svc := &usecase.Service{
		Tx: c.Tx, Clock: c.Clock, Repo: postgres.NewRepository(c.DB), Jobs: c.APIs.Jobs, Budget: c.APIs.Budget,
		Assets: c.APIs.Assets, Campaigns: c.APIs.Campaigns, Brands: c.APIs.Brands, Files: c.APIs.Files,
		Providers: c.APIs.Providers, AI: c.AI, Logger: c.Logger,
	}
	return &Module{handler: &nethttp.Handler{Svc: svc}, step: &worker.ImageStep{Svc: svc}}, nil
}
