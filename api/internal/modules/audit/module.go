// Package audit wires the audit module: append-only audit log and in-app
// notifications. It must be the first module registered.
package audit

import (
	"github.com/go-chi/chi/v5"

	"github.com/be/acs/api/internal/app/registry"
	audithttp "github.com/be/acs/api/internal/modules/audit/adapter/http"
	"github.com/be/acs/api/internal/modules/audit/adapter/postgres"
	"github.com/be/acs/api/internal/modules/audit/usecase"
)

// Module is the audit module.
type Module struct{ handler *audithttp.Handler }

// Name implements registry.Module.
func (*Module) Name() string { return "audit" }

// Routes implements registry.HTTPModule.
func (m *Module) Routes(r chi.Router) { m.handler.Routes(r) }

// Register builds the module and publishes APIs.Audit.
func Register(c *registry.Container) (registry.Module, error) {
	svc := &usecase.Service{Repo: postgres.NewRepository(c.DB)}
	c.APIs.Audit = svc
	return &Module{handler: &audithttp.Handler{Svc: svc}}, nil
}
