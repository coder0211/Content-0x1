// Package campaign wires the campaign module (ACS-025, ACS-037 storage):
// campaigns with a shared brief, the status state machine, members and the
// approved content plan.
package campaign

import (
	"github.com/go-chi/chi/v5"

	"github.com/be/acs/api/internal/app/registry"
	camphttp "github.com/be/acs/api/internal/modules/campaign/adapter/http"
	"github.com/be/acs/api/internal/modules/campaign/adapter/postgres"
	"github.com/be/acs/api/internal/modules/campaign/usecase"
)

// Module is the campaign module.
type Module struct {
	Svc     *usecase.Service
	handler *camphttp.Handler
}

// Name implements registry.Module.
func (*Module) Name() string { return "campaign" }

// Routes implements registry.HTTPModule.
func (m *Module) Routes(r chi.Router) { m.handler.Routes(r) }

// Register builds the module and publishes APIs.Campaigns.
func Register(c *registry.Container) (registry.Module, error) {
	svc := &usecase.Service{Tx: c.Tx, Repo: postgres.NewRepository(c.DB), Clock: c.Clock, Audit: c.APIs.Audit,
		Brands: c.APIs.Brands, Users: c.APIs.Identity}
	c.APIs.Campaigns = svc
	return &Module{Svc: svc, handler: &camphttp.Handler{Svc: svc}}, nil
}
