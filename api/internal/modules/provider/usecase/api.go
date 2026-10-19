// Package usecase is the public contract of the provider-configuration
// module (ai_providers, ai_models, prompt_templates).
package usecase

import (
	"context"
	"encoding/json"

	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
)

// ModelInfo identifies a usable model and its capabilities.
type ModelInfo struct {
	ModelID          kernel.ID
	ProviderCode     string
	ModelCode        string
	Kind             ports.AIKind
	Capabilities     ports.Capabilities
	ConcurrencyLimit int
}

// PromptTemplate is a versioned prompt (FR-CNT-08).
type PromptTemplate struct {
	ID           kernel.ID
	Code         string
	Version      int
	SystemPrompt string
	UserTemplate string
	OutputSchema json.RawMessage
}

// ProviderConfig is a provider row for the adapter registry. SecretRef
// names an env var / secret; it is never returned to the FE.
type ProviderConfig struct {
	Code      string
	Kinds     []ports.AIKind
	Active    bool
	SecretRef string
	Config    json.RawMessage
}

// API is the provider module's public interface.
type API interface {
	// ResolveModel returns modelCode of kind, or the active default when
	// modelCode is "". Disabled/unknown → CAPABILITY_NOT_SUPPORTED (422)
	// with details {"field":"model_code"}.
	ResolveModel(ctx context.Context, kind ports.AIKind, modelCode string) (ModelInfo, error)
	// ActivePromptTemplate returns the active version of code.
	ActivePromptTemplate(ctx context.Context, code string) (PromptTemplate, error)
	// GetPromptTemplate returns a specific template (job reproduction).
	GetPromptTemplate(ctx context.Context, id kernel.ID) (PromptTemplate, error)
	// ListProviders feeds the adapter registry.
	ListProviders(ctx context.Context) ([]ProviderConfig, error)
}
