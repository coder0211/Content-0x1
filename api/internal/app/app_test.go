package app

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/be/acs/api/internal/app/registry"
	"github.com/be/acs/api/internal/modules/budget/budgettest"
	budgetuc "github.com/be/acs/api/internal/modules/budget/usecase"
	"github.com/be/acs/api/internal/modules/job/jobtest"
	jobuc "github.com/be/acs/api/internal/modules/job/usecase"
	"github.com/be/acs/api/internal/shared/kernel"
)

func TestComposeAllModulesWithoutDrivers(t *testing.T) {
	c := &registry.Container{Platform: registry.Platform{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}}
	mods, err := Compose(c, Modules)
	if err != nil {
		t.Fatal(err)
	}
	if len(mods) != len(Modules) {
		t.Fatal("module count")
	}
	a := c.APIs
	if a.Audit == nil || a.Identity == nil || a.Authenticator == nil || a.Files == nil || a.Jobs == nil || a.Steps == nil ||
		a.Budget == nil || a.Assets == nil || a.Brands == nil || a.Campaigns == nil || a.Providers == nil || a.Content == nil {
		t.Fatalf("some API not published: %+v", a)
	}
	if _, err := Compose(c, []registry.Constructor{Modules[0], Modules[0]}); err == nil {
		t.Fatal("duplicate module accepted")
	}
}

func TestFinishedJobSettlesBudget(t *testing.T) {
	jobs, budget := jobtest.NewAPI(), budgettest.NewAPI()
	c := &registry.Container{APIs: registry.APIs{Jobs: jobs, Budget: budget}}
	wireHooks(c)
	ctx := context.Background()
	res, err := jobs.Create(ctx, jobuc.JobSpec{Type: jobuc.TypeContentGeneration, CreatedBy: kernel.NewID(),
		Steps: []jobuc.StepSpec{{Key: "generate", Type: "content_generation"}}, CostEstimate: kernel.USD("0.02")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := budget.Reserve(ctx, budgetuc.ReserveRequest{JobID: res.JobID, Amount: kernel.USD("0.02"), Category: budgetuc.CategoryText}); err != nil {
		t.Fatal(err)
	}
	if err := jobs.Finish(ctx, res.JobID, jobuc.StatusFailed); err != nil {
		t.Fatal(err)
	}
	if len(budget.Reserved) != 0 {
		t.Fatal("reservation not released on finish")
	}
}

func TestCategoryFor(t *testing.T) {
	cases := map[jobuc.JobType]budgetuc.Category{
		jobuc.TypeVideoProduction: budgetuc.CategoryVideo, jobuc.TypeImageEdit: budgetuc.CategoryImage,
		jobuc.TypeImagePromptSuggest: budgetuc.CategoryText, jobuc.TypeContentPlan: budgetuc.CategoryText,
	}
	for jt, want := range cases {
		if got := CategoryFor(jt); got != want {
			t.Errorf("%s: %s", jt, got)
		}
	}
}
