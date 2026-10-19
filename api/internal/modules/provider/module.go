// Package provider wires the provider module.
//
// PLACEHOLDER (Agent 1): until the provider module is implemented, Register
// publishes the in-memory fake from providertest so dependent modules compile and run.
// The owning agent replaces the body of Register (keeping its signature).
package provider

import (
	"github.com/be/acs/api/internal/app/registry"
	"github.com/be/acs/api/internal/modules/provider/providertest"
)

// Module is the provider module.
type Module struct{}

// Name implements registry.Module.
func (*Module) Name() string { return "provider" }

// Register publishes APIs.Providers.
func Register(c *registry.Container) (registry.Module, error) {
	c.Logger.Warn("provider module is a placeholder (in-memory fake)")
	c.APIs.Providers = providertest.NewAPI()
	return &Module{}, nil
}
