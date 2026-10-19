// Package providertest provides an in-memory provider usecase.API seeded
// with the "fake" provider models of migration 0011.
package providertest

import (
	"context"

	"github.com/be/acs/api/internal/modules/provider/usecase"
	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
)

// API is the fake.
type API struct {
	Models    []usecase.ModelInfo
	Templates map[string]usecase.PromptTemplate
}

// NewAPI returns the fake with one default fake model per kind.
func NewAPI() *API {
	return &API{
		Models: []usecase.ModelInfo{
			{ModelID: kernel.NewID(), ProviderCode: "fake", ModelCode: "fake-text", Kind: ports.AIKindText, ConcurrencyLimit: 10,
				Capabilities: ports.Capabilities{Languages: []string{"vi", "en"}, MaxOutputTokens: 4096}},
			{ModelID: kernel.NewID(), ProviderCode: "fake", ModelCode: "fake-image", Kind: ports.AIKindImage, ConcurrencyLimit: 4,
				Capabilities: ports.Capabilities{AspectRatios: []string{"1:1", "4:5", "9:16", "16:9"}, Qualities: []string{"standard", "high"}, MaxVariants: 4, SupportsRefImage: true, SupportsEdit: true}},
			{ModelID: kernel.NewID(), ProviderCode: "fake", ModelCode: "fake-video", Kind: ports.AIKindVideo, ConcurrencyLimit: 4,
				Capabilities: ports.Capabilities{AspectRatios: []string{"9:16", "1:1", "16:9"}, Resolutions: []string{"720p", "1080p"}, MinDurationSec: 2, MaxDurationSec: 8, SupportsRefImage: true, SupportsCancel: true}},
			{ModelID: kernel.NewID(), ProviderCode: "fake", ModelCode: "fake-voice", Kind: ports.AIKindVoice, ConcurrencyLimit: 4,
				Capabilities: ports.Capabilities{Languages: []string{"vi-VN", "en-US"}, Voices: []string{"vi-VN-female-1", "vi-VN-male-1"}}},
		},
		Templates: map[string]usecase.PromptTemplate{},
	}
}

var _ usecase.API = (*API)(nil)

func (a *API) ResolveModel(_ context.Context, kind ports.AIKind, code string) (usecase.ModelInfo, error) {
	for _, m := range a.Models {
		if m.Kind == kind && (code == "" || m.ModelCode == code) {
			return m, nil
		}
	}
	return usecase.ModelInfo{}, kernel.ErrCapability.WithDetails(kernel.FieldIssue("model_code", "unsupported"))
}

func (a *API) ActivePromptTemplate(_ context.Context, code string) (usecase.PromptTemplate, error) {
	t, ok := a.Templates[code]
	if !ok {
		return usecase.PromptTemplate{}, kernel.ErrNotFound
	}
	return t, nil
}

func (a *API) GetPromptTemplate(_ context.Context, id kernel.ID) (usecase.PromptTemplate, error) {
	for _, t := range a.Templates {
		if t.ID == id {
			return t, nil
		}
	}
	return usecase.PromptTemplate{}, kernel.ErrNotFound
}

func (a *API) ListProviders(context.Context) ([]usecase.ProviderConfig, error) {
	return []usecase.ProviderConfig{{Code: "fake", Active: true, SecretRef: "none",
		Kinds: []ports.AIKind{ports.AIKindText, ports.AIKindImage, ports.AIKindVideo, ports.AIKindVoice}}}, nil
}
