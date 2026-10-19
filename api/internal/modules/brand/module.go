// Package brand wires the brand module.
//
// PLACEHOLDER (Agent 1): until the brand module is implemented, Register
// publishes the in-memory fake from brandtest so dependent modules compile and run.
// The owning agent replaces the body of Register (keeping its signature).
package brand

import (
	"github.com/be/acs/api/internal/app/registry"
	"github.com/be/acs/api/internal/modules/brand/brandtest"
)

// Module is the brand module.
type Module struct{}

// Name implements registry.Module.
func (*Module) Name() string { return "brand" }

// Register publishes APIs.Brands.
func Register(c *registry.Container) (registry.Module, error) {
	c.Logger.Warn("brand module is a placeholder (in-memory fake)")
	c.APIs.Brands = brandtest.NewAPI()
	return &Module{}, nil
}
