// Package contenttest provides an in-memory content usecase.API.
package contenttest

import (
	"context"

	"github.com/be/acs/api/internal/modules/content/usecase"
	"github.com/be/acs/api/internal/shared/kernel"
)

// API is the fake; populate Scripts directly. Plans and Prompts record the
// inputs of StartContentPlan / SuggestImagePrompts.
type API struct {
	Scripts map[kernel.ID]usecase.VideoScript
	Plans   []usecase.PlanInput
	Prompts []usecase.ImagePromptInput
}

// NewAPI returns an empty fake.
func NewAPI() *API { return &API{Scripts: map[kernel.ID]usecase.VideoScript{}} }

var _ usecase.API = (*API)(nil)

func (a *API) GetVideoScript(_ context.Context, actor kernel.Actor, id kernel.ID) (usecase.VideoScript, error) {
	if err := actor.Require(kernel.PermBrandView); err != nil {
		return usecase.VideoScript{}, err
	}
	s, ok := a.Scripts[id]
	if !ok {
		return usecase.VideoScript{}, kernel.ErrNotFound
	}
	return s, nil
}

func (a *API) StartContentPlan(_ context.Context, actor kernel.Actor, in usecase.PlanInput) (usecase.JobAccepted, error) {
	if err := actor.Require(kernel.PermGenerateRun); err != nil {
		return usecase.JobAccepted{}, err
	}
	a.Plans = append(a.Plans, in)
	return usecase.JobAccepted{JobID: kernel.NewID(), Status: "queued", EstimatedCost: kernel.USD("0.01")}, nil
}

func (a *API) SuggestImagePrompts(_ context.Context, actor kernel.Actor, in usecase.ImagePromptInput) (usecase.JobAccepted, error) {
	if err := actor.Require(kernel.PermGenerateRun); err != nil {
		return usecase.JobAccepted{}, err
	}
	a.Prompts = append(a.Prompts, in)
	return usecase.JobAccepted{JobID: kernel.NewID(), Status: "queued", EstimatedCost: kernel.USD("0.01")}, nil
}
