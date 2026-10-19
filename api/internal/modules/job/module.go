// Package job wires the job module: job/step persistence, outbox
// dispatcher, worker runtime, reconciler, job API + SSE.
//
// PLACEHOLDER (Agent 1): until the job engine is implemented, Register
// publishes the in-memory fake API and a real StepRegistry. The owning
// agent replaces the body of Register (keeping its signature) and adds
// Routes/Runners (queue consumer, dispatcher, reconciler).
package job

import (
	"github.com/be/acs/api/internal/app/registry"
	"github.com/be/acs/api/internal/modules/job/jobtest"
	"github.com/be/acs/api/internal/modules/job/usecase"
)

// Module is the job module.
type Module struct {
	// Steps holds every StepHandler registered by other modules.
	Steps *usecase.StepRegistry
}

// Name implements registry.Module.
func (*Module) Name() string { return "job" }

// Register publishes APIs.Jobs and APIs.Steps.
func Register(c *registry.Container) (registry.Module, error) {
	c.Logger.Warn("job module is a placeholder (in-memory fake, no engine)")
	steps := usecase.NewStepRegistry()
	c.APIs.Jobs = jobtest.NewAPI()
	c.APIs.Steps = steps
	return &Module{Steps: steps}, nil
}
