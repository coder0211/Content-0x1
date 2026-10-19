// Package provider wires the provider module (ACS-014, ACS-090 BE,
// ACS-038 BE): ai_providers / ai_models / ai_model_pricing configuration,
// prompt_templates versions, model resolution for generation modules, and
// the bridge that pushes provider rows into the live adapter registry.
package provider

import (
	"context"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/be/acs/api/internal/app/registry"
	providerhttp "github.com/be/acs/api/internal/modules/provider/adapter/http"
	"github.com/be/acs/api/internal/modules/provider/adapter/postgres"
	"github.com/be/acs/api/internal/modules/provider/providertest"
	"github.com/be/acs/api/internal/modules/provider/usecase"
	"github.com/be/acs/api/internal/platform/config"
	"github.com/be/acs/api/internal/ports"
)

// SyncInterval is how often each process re-reads ai_providers so that an
// admin enable/disable made in another process (server → worker) takes
// effect without a restart.
const SyncInterval = 30 * time.Second

// Module is the provider module.
type Module struct {
	Svc     *usecase.Service
	handler *providerhttp.Handler
}

// Name implements registry.Module.
func (*Module) Name() string { return "provider" }

// Routes implements registry.HTTPModule.
func (m *Module) Routes(r chi.Router) {
	if m.handler != nil {
		m.handler.Routes(r)
	}
}

// Runners implements registry.RunnerModule: a periodic registry sync in the
// server and the worker.
func (m *Module) Runners(p registry.Process) []registry.Runner {
	if m.Svc == nil || m.Svc.Registry == nil || p == registry.ProcessCLI {
		return nil
	}
	return []registry.Runner{{Name: "provider-registry-sync", Run: func(ctx context.Context) error {
		t := time.NewTicker(SyncInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return nil
			case <-t.C:
				_ = m.Svc.SyncRegistry(ctx)
			}
		}
	}}}
}

// Register builds the module and publishes APIs.Providers. Without a
// database (unit tests composing the app) it publishes the in-memory fake.
func Register(c *registry.Container) (registry.Module, error) {
	if c.DB == nil {
		c.APIs.Providers = providertest.NewAPI()
		return &Module{}, nil
	}
	svc := &usecase.Service{
		Tx: c.Tx, Repo: postgres.NewRepository(c.DB), Clock: c.Clock, Audit: c.APIs.Audit,
		Secrets: func(ref string) bool { return config.Secret(ref) != "" },
	}
	if reg, ok := c.AI.(usecase.AdapterRegistry); ok {
		svc.Registry = reg
	}
	if su, ok := c.AI.(interface{ UseStorage(ports.ObjectStorage) }); ok && c.Storage != nil {
		su.UseStorage(c.Storage)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := svc.SyncRegistry(ctx); err != nil && c.Logger != nil {
		c.Logger.Warn("provider: initial registry sync failed", "error", err)
	}
	c.APIs.Providers = svc
	return &Module{Svc: svc, handler: &providerhttp.Handler{Svc: svc}}, nil
}
