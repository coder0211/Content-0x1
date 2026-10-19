// Package contenttest provides an in-memory content usecase.API.
package contenttest

import (
	"context"

	"github.com/be/acs/api/internal/modules/content/usecase"
	"github.com/be/acs/api/internal/shared/kernel"
)

// API is the fake; populate Scripts directly.
type API struct {
	Scripts map[kernel.ID]usecase.VideoScript
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
