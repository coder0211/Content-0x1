package usecase

import (
	"context"
	"encoding/json"
	"time"

	"github.com/be/acs/api/internal/modules/provider/domain"
	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Repository persists provider configuration (adapter/postgres).
type Repository interface {
	ListProviders(ctx context.Context) ([]domain.Provider, error)
	GetProvider(ctx context.Context, id kernel.ID) (*domain.Provider, error)
	GetProviderForUpdate(ctx context.Context, id kernel.ID) (*domain.Provider, error)
	UpdateProvider(ctx context.Context, p *domain.Provider) error

	ListModels(ctx context.Context) ([]domain.Model, error)
	GetModel(ctx context.Context, id kernel.ID) (*domain.Model, error)
	GetModelForUpdate(ctx context.Context, id kernel.ID) (*domain.Model, error)
	// CreateModel returns kernel.ErrConflict when (provider, model_code) exists.
	CreateModel(ctx context.Context, m *domain.Model) error
	UpdateModel(ctx context.Context, m *domain.Model) error
	// ClearDefault unsets is_default on every other model of kind.
	ClearDefault(ctx context.Context, kind string, except kernel.ID) error

	ListPrices(ctx context.Context, modelID kernel.ID) ([]domain.Price, error)
	PricingVersionExists(ctx context.Context, modelID kernel.ID, version string) (bool, error)
	// ClosePrices sets effective_to=at on the model's open rows that started before at.
	ClosePrices(ctx context.Context, modelID kernel.ID, at time.Time) error
	InsertPrices(ctx context.Context, prices []domain.Price) error

	ListTemplates(ctx context.Context, code string) ([]domain.PromptTemplate, error)
	GetTemplate(ctx context.Context, id kernel.ID) (*domain.PromptTemplate, error)
	GetTemplateForUpdate(ctx context.Context, id kernel.ID) (*domain.PromptTemplate, error)
	ActiveTemplate(ctx context.Context, code string) (*domain.PromptTemplate, error)
	// LockTemplateCode serialises version allocation/activation per code (tx-scoped).
	LockTemplateCode(ctx context.Context, code string) error
	MaxTemplateVersion(ctx context.Context, code string) (int, error)
	CreateTemplate(ctx context.Context, t *domain.PromptTemplate) error
	SetTemplateStatus(ctx context.Context, id kernel.ID, status domain.TemplateStatus) error
}

// AdapterRegistry is the live adapter registry (providers.Registry),
// declared with stdlib/ports types only so the use case stays
// framework-free. Nil in tests that do not care.
type AdapterRegistry interface {
	ApplyProviderConfig(code string, active bool, secretRef string, cfg json.RawMessage)
	Codes() map[ports.AIKind][]string
}

// SecretProbe reports whether a secret_ref resolves to a non-empty value
// (the value itself never leaves the probe).
type SecretProbe func(ref string) bool
