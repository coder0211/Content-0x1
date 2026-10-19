// Package template is the copyable module skeleton (see README.md).
package template

import (
	"github.com/go-chi/chi/v5"

	"github.com/be/acs/api/internal/app/registry"
	nethttp "github.com/be/acs/api/internal/modules/_template/adapter/http"
	"github.com/be/acs/api/internal/modules/_template/usecase"
)

// Module bundles the module's adapters.
type Module struct{ handler *nethttp.Handler }

// Name implements registry.Module.
func (*Module) Name() string { return "template" }

// Routes implements registry.HTTPModule.
func (m *Module) Routes(r chi.Router) { m.handler.Routes(r) }

// Register wires dependencies from the container. A real module would use
// adapter/postgres.NewRepository(c.DB) and publish its API, e.g.
// c.APIs.<Name> = notes.
func Register(c *registry.Container) (registry.Module, error) {
	notes := &usecase.Notes{Tx: c.Tx, Repo: usecase.NewMemoryRepo(), Clock: c.Clock, Audit: c.APIs.Audit}
	return &Module{handler: &nethttp.Handler{Notes: notes}}, nil
}
