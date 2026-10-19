// Package asset wires the asset module.
//
// PLACEHOLDER (Agent 1): until the asset module is implemented, Register
// publishes the in-memory fake from assettest so dependent modules compile and run.
// The owning agent replaces the body of Register (keeping its signature).
package asset

import (
	"github.com/be/acs/api/internal/app/registry"
	"github.com/be/acs/api/internal/modules/asset/assettest"
)

// Module is the asset module.
type Module struct{}

// Name implements registry.Module.
func (*Module) Name() string { return "asset" }

// Register publishes APIs.Assets.
func Register(c *registry.Container) (registry.Module, error) {
	c.Logger.Warn("asset module is a placeholder (in-memory fake)")
	c.APIs.Assets = assettest.NewAPI()
	return &Module{}, nil
}
