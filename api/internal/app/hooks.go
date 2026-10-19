package app

import (
	"context"
	"strings"

	"github.com/be/acs/api/internal/app/registry"
	budgetuc "github.com/be/acs/api/internal/modules/budget/usecase"
	jobuc "github.com/be/acs/api/internal/modules/job/usecase"
	"github.com/be/acs/api/internal/shared/kernel"
)

// wireHooks connects cross-module reactions that module boundaries forbid
// as direct calls (job must not import budget; SPEC-06 §2.5).
func wireHooks(c *registry.Container) {
	if c.APIs.Jobs == nil || c.APIs.Budget == nil {
		return
	}
	budget := c.APIs.Budget
	// Terminal job → settle (spend = recorded usage; no usage = release).
	c.APIs.Jobs.OnFinished(func(ctx context.Context, ev jobuc.JobFinished) error {
		return budget.Settle(ctx, ev.JobID)
	})
	// Retry → new reservation (SPEC-04 §2.7).
	c.APIs.Jobs.OnRetried(func(ctx context.Context, ev jobuc.JobRetried) error {
		var override *kernel.Actor
		if ev.Actor.IsLeadOrAdmin() {
			a := ev.Actor
			override = &a
		}
		_, err := budget.Reserve(ctx, budgetuc.ReserveRequest{
			JobID: ev.JobID, CampaignID: ev.CampaignID, Amount: ev.CostEstimate,
			Category: CategoryFor(ev.Type), Override: override,
		})
		return err
	})
}

// CategoryFor maps a job type to its per-job budget category.
func CategoryFor(t jobuc.JobType) budgetuc.Category {
	switch {
	case strings.HasPrefix(string(t), "video_"):
		return budgetuc.CategoryVideo
	case t == jobuc.TypeImageGeneration || t == jobuc.TypeImageEdit:
		return budgetuc.CategoryImage
	}
	return budgetuc.CategoryText
}
