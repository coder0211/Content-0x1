// Package brand wires the brand module (ACS-020..023): brand profiles with
// versions, products with verified facts, brand assets (logo, references,
// licensed music) and the BrandAPI used for prompt building.
package brand

import (
	"github.com/go-chi/chi/v5"

	"github.com/be/acs/api/internal/app/registry"
	brandhttp "github.com/be/acs/api/internal/modules/brand/adapter/http"
	"github.com/be/acs/api/internal/modules/brand/adapter/postgres"
	"github.com/be/acs/api/internal/modules/brand/usecase"
)

// Module is the brand module.
type Module struct {
	Svc     *usecase.Service
	handler *brandhttp.Handler
}

// Name implements registry.Module.
func (*Module) Name() string { return "brand" }

// Routes implements registry.HTTPModule.
func (m *Module) Routes(r chi.Router) { m.handler.Routes(r) }

// Register builds the module and publishes APIs.Brands.
func Register(c *registry.Container) (registry.Module, error) {
	svc := &usecase.Service{Tx: c.Tx, Repo: postgres.NewRepository(c.DB), Clock: c.Clock, Audit: c.APIs.Audit, Files: c.APIs.Files}
	c.APIs.Brands = svc
	return &Module{Svc: svc, handler: &brandhttp.Handler{Svc: svc}}, nil
}
