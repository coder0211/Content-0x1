// Package dashboard wires the dashboard module (ACS-095): GET
// /dashboard/summary with widgets filtered by permission.
package dashboard

import (
	"github.com/go-chi/chi/v5"

	"github.com/be/acs/api/internal/app/registry"
	dashhttp "github.com/be/acs/api/internal/modules/dashboard/adapter/http"
	"github.com/be/acs/api/internal/modules/dashboard/adapter/postgres"
	"github.com/be/acs/api/internal/modules/dashboard/usecase"
)

// Module is the dashboard module.
type Module struct{ handler *dashhttp.Handler }

// Name implements registry.Module.
func (*Module) Name() string { return "dashboard" }

// Routes implements registry.HTTPModule.
func (m *Module) Routes(r chi.Router) { m.handler.Routes(r) }

// Register builds the module (it publishes no API).
func Register(c *registry.Container) (registry.Module, error) {
	svc := &usecase.Service{Reader: postgres.NewReader(c.DB), Assets: c.APIs.Assets, Clock: c.Clock}
	return &Module{handler: &dashhttp.Handler{Svc: svc}}, nil
}
