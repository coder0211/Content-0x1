// Package asset wires the asset module: content assets and their versions
// (ACS-034), review flow (ACS-080), library search (ACS-082), audited
// downloads (ACS-084), tags and usage rights (ACS-085).
package asset

import (
	"github.com/go-chi/chi/v5"

	"github.com/be/acs/api/internal/app/registry"
	assethttp "github.com/be/acs/api/internal/modules/asset/adapter/http"
	"github.com/be/acs/api/internal/modules/asset/adapter/postgres"
	"github.com/be/acs/api/internal/modules/asset/usecase"
)

// Module is the asset module.
type Module struct {
	Svc     *usecase.Service
	handler *assethttp.Handler
}

// Name implements registry.Module.
func (*Module) Name() string { return "asset" }

// Routes implements registry.HTTPModule.
func (m *Module) Routes(r chi.Router) { m.handler.Routes(r) }

// Register builds the module and publishes APIs.Assets.
func Register(c *registry.Container) (registry.Module, error) {
	svc := &usecase.Service{
		Tx: c.Tx, Repo: postgres.NewRepository(c.DB), Clock: c.Clock, Audit: c.APIs.Audit, Notifier: c.APIs.Audit,
		Identity: c.APIs.Identity, Campaigns: c.APIs.Campaigns, Logger: c.Logger,
	}
	if c.APIs.Files != nil {
		svc.Files = c.APIs.Files
	}
	c.APIs.Assets = svc
	return &Module{Svc: svc, handler: &assethttp.Handler{Svc: svc}}, nil
}
