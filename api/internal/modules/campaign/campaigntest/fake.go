// Package campaigntest provides an in-memory campaign usecase.API.
package campaigntest

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/be/acs/api/internal/modules/campaign/usecase"
	"github.com/be/acs/api/internal/shared/kernel"
)

// API is the fake; populate Campaigns directly.
type API struct {
	Campaigns map[kernel.ID]usecase.CampaignForGeneration
	Proposals map[kernel.ID]json.RawMessage
}

// NewAPI returns an empty fake.
func NewAPI() *API {
	return &API{Campaigns: map[kernel.ID]usecase.CampaignForGeneration{}, Proposals: map[kernel.ID]json.RawMessage{}}
}

var _ usecase.API = (*API)(nil)

func (a *API) GetCampaignForGeneration(_ context.Context, actor kernel.Actor, id kernel.ID) (usecase.CampaignForGeneration, error) {
	if err := actor.Require(kernel.PermGenerateRun); err != nil {
		return usecase.CampaignForGeneration{}, err
	}
	c, ok := a.Campaigns[id]
	if !ok {
		return usecase.CampaignForGeneration{}, kernel.ErrNotFound
	}
	if c.Status == usecase.StatusArchived || c.Status == usecase.StatusCompleted {
		return usecase.CampaignForGeneration{}, kernel.InvalidTransition(string(c.Status), "generate")
	}
	return c, nil
}

func (a *API) IsMember(_ context.Context, id, userID kernel.ID) (bool, error) {
	c, ok := a.Campaigns[id]
	if !ok {
		return false, kernel.ErrNotFound
	}
	return c.CreatedBy == userID || slices.Contains(c.MemberIDs, userID), nil
}

func (a *API) SaveContentPlanProposal(_ context.Context, id kernel.ID, plan json.RawMessage) error {
	if _, ok := a.Campaigns[id]; !ok {
		return kernel.ErrNotFound
	}
	a.Proposals[id] = plan
	return nil
}
