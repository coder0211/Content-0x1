// Package budget wires the budget module.
//
// PLACEHOLDER (Agent 1): until the budget module is implemented, Register
// publishes the in-memory fake from budgettest so dependent modules compile and run.
// The owning agent replaces the body of Register (keeping its signature).
package budget

import (
	"github.com/be/acs/api/internal/app/registry"
	"github.com/be/acs/api/internal/modules/budget/budgettest"
)

// Module is the budget module.
type Module struct{}

// Name implements registry.Module.
func (*Module) Name() string { return "budget" }

// Register publishes APIs.Budget.
func Register(c *registry.Container) (registry.Module, error) {
	c.Logger.Warn("budget module is a placeholder (in-memory fake)")
	c.APIs.Budget = budgettest.NewAPI()
	return &Module{}, nil
}
