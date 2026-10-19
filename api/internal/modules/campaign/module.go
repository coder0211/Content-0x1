// Package campaign wires the campaign module.
//
// PLACEHOLDER (Agent 1): until the campaign module is implemented, Register
// publishes the in-memory fake from campaigntest so dependent modules compile and run.
// The owning agent replaces the body of Register (keeping its signature).
package campaign

import (
	"github.com/be/acs/api/internal/app/registry"
	"github.com/be/acs/api/internal/modules/campaign/campaigntest"
)

// Module is the campaign module.
type Module struct{}

// Name implements registry.Module.
func (*Module) Name() string { return "campaign" }

// Register publishes APIs.Campaigns.
func Register(c *registry.Container) (registry.Module, error) {
	c.Logger.Warn("campaign module is a placeholder (in-memory fake)")
	c.APIs.Campaigns = campaigntest.NewAPI()
	return &Module{}, nil
}
