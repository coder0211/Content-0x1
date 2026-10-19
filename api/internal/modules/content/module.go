// Package content wires the content module: Content Studio generation,
// rewrite/transform, post-generation checks, campaign content plans and
// image prompt suggestions. It owns no tables: results are stored through
// asset.API (content assets/versions) and campaign.API (plan proposals).
package content

import (
	"github.com/go-chi/chi/v5"

	"github.com/be/acs/api/internal/app/registry"
	nethttp "github.com/be/acs/api/internal/modules/content/adapter/http"
	"github.com/be/acs/api/internal/modules/content/adapter/worker"
	"github.com/be/acs/api/internal/modules/content/usecase"
	jobuc "github.com/be/acs/api/internal/modules/job/usecase"
)

// Module is the content module.
type Module struct {
	svc     *usecase.Service
	handler *nethttp.Handler
}

// Name implements registry.Module.
func (*Module) Name() string { return "content" }

// Routes implements registry.HTTPModule.
func (m *Module) Routes(r chi.Router) { m.handler.Routes(r) }

// StepHandlers implements registry.StepModule.
func (m *Module) StepHandlers() []jobuc.StepHandler { return worker.Handlers(m.svc) }

// Register publishes APIs.Content. Drivers are not touched here (c.AI may
// be nil in tests; it is only used by step handlers in the worker).
func Register(c *registry.Container) (registry.Module, error) {
	svc := &usecase.Service{
		Tx: c.Tx, Clock: c.Clock, Jobs: c.APIs.Jobs, Budget: c.APIs.Budget, Assets: c.APIs.Assets,
		Brands: c.APIs.Brands, Campaigns: c.APIs.Campaigns, Providers: c.APIs.Providers, AI: c.AI,
	}
	c.APIs.Content = svc
	return &Module{svc: svc, handler: &nethttp.Handler{Svc: svc}}, nil
}
