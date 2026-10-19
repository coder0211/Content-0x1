// Package content wires the content module.
//
// PLACEHOLDER (Agent 1): until the content module is implemented, Register
// publishes the in-memory fake from contenttest so dependent modules compile and run.
// The owning agent replaces the body of Register (keeping its signature).
package content

import (
	"github.com/be/acs/api/internal/app/registry"
	"github.com/be/acs/api/internal/modules/content/contenttest"
)

// Module is the content module.
type Module struct{}

// Name implements registry.Module.
func (*Module) Name() string { return "content" }

// Register publishes APIs.Content.
func Register(c *registry.Container) (registry.Module, error) {
	c.Logger.Warn("content module is a placeholder (in-memory fake)")
	c.APIs.Content = contenttest.NewAPI()
	return &Module{}, nil
}
