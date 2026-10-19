package usecase

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/be/acs/api/internal/modules/provider/domain"
	"github.com/be/acs/api/internal/shared/kernel"
)

// MemoryRepo is an in-memory Repository for use-case and handler tests.
type MemoryRepo struct {
	mu        sync.Mutex
	Providers []domain.Provider
	Models    []domain.Model
	Prices    []domain.Price
	Templates []domain.PromptTemplate
}

var _ Repository = (*MemoryRepo)(nil)

func (m *MemoryRepo) ListProviders(context.Context) ([]domain.Provider, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.Providers), nil
}

func (m *MemoryRepo) GetProvider(_ context.Context, id kernel.ID) (*domain.Provider, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range m.Providers {
		if p.ID == id {
			return &p, nil
		}
	}
	return nil, kernel.ErrNotFound
}

func (m *MemoryRepo) GetProviderForUpdate(ctx context.Context, id kernel.ID) (*domain.Provider, error) {
	return m.GetProvider(ctx, id)
}

func (m *MemoryRepo) UpdateProvider(_ context.Context, p *domain.Provider) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.Providers {
		if m.Providers[i].ID == p.ID {
			m.Providers[i] = *p
			for j := range m.Models {
				if m.Models[j].ProviderID == p.ID {
					m.Models[j].ProviderStatus = p.Status
				}
			}
			return nil
		}
	}
	return kernel.ErrNotFound
}

func (m *MemoryRepo) ListModels(context.Context) ([]domain.Model, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.Models), nil
}

func (m *MemoryRepo) GetModel(_ context.Context, id kernel.ID) (*domain.Model, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, x := range m.Models {
		if x.ID == id {
			return &x, nil
		}
	}
	return nil, kernel.ErrNotFound
}

func (m *MemoryRepo) GetModelForUpdate(ctx context.Context, id kernel.ID) (*domain.Model, error) {
	return m.GetModel(ctx, id)
}

func (m *MemoryRepo) CreateModel(_ context.Context, x *domain.Model) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, y := range m.Models {
		if y.ProviderID == x.ProviderID && y.ModelCode == x.ModelCode {
			return kernel.ErrConflict
		}
	}
	m.Models = append(m.Models, *x)
	return nil
}

func (m *MemoryRepo) UpdateModel(_ context.Context, x *domain.Model) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.Models {
		if m.Models[i].ID == x.ID {
			m.Models[i] = *x
			return nil
		}
	}
	return kernel.ErrNotFound
}

func (m *MemoryRepo) ClearDefault(_ context.Context, kind string, except kernel.ID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.Models {
		if m.Models[i].Kind == kind && m.Models[i].ID != except {
			m.Models[i].IsDefault = false
		}
	}
	return nil
}

func (m *MemoryRepo) ListPrices(_ context.Context, modelID kernel.ID) ([]domain.Price, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []domain.Price
	for _, p := range m.Prices {
		if p.ModelID == modelID {
			out = append(out, p)
		}
	}
	return out, nil
}

func (m *MemoryRepo) PricingVersionExists(_ context.Context, modelID kernel.ID, v string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range m.Prices {
		if p.ModelID == modelID && p.PricingVersion == v {
			return true, nil
		}
	}
	return false, nil
}

func (m *MemoryRepo) ClosePrices(_ context.Context, modelID kernel.ID, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.Prices {
		p := &m.Prices[i]
		if p.ModelID == modelID && p.EffectiveTo == nil && p.EffectiveFrom.Before(at) {
			t := at
			p.EffectiveTo = &t
		}
	}
	return nil
}

func (m *MemoryRepo) InsertPrices(_ context.Context, ps []domain.Price) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Prices = append(m.Prices, ps...)
	return nil
}

func (m *MemoryRepo) ListTemplates(_ context.Context, code string) ([]domain.PromptTemplate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []domain.PromptTemplate{}
	for _, t := range m.Templates {
		if code == "" || t.Code == code {
			out = append(out, t)
		}
	}
	slices.SortFunc(out, func(a, b domain.PromptTemplate) int {
		if c := strings.Compare(a.Code, b.Code); c != 0 {
			return c
		}
		return b.Version - a.Version
	})
	return out, nil
}

func (m *MemoryRepo) GetTemplate(_ context.Context, id kernel.ID) (*domain.PromptTemplate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.Templates {
		if t.ID == id {
			return &t, nil
		}
	}
	return nil, kernel.ErrNotFound
}

func (m *MemoryRepo) GetTemplateForUpdate(ctx context.Context, id kernel.ID) (*domain.PromptTemplate, error) {
	return m.GetTemplate(ctx, id)
}

func (m *MemoryRepo) ActiveTemplate(_ context.Context, code string) (*domain.PromptTemplate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.Templates {
		if t.Code == code && t.Status == domain.TemplateActive {
			return &t, nil
		}
	}
	return nil, kernel.ErrNotFound
}

func (m *MemoryRepo) LockTemplateCode(context.Context, string) error { return nil }

func (m *MemoryRepo) MaxTemplateVersion(_ context.Context, code string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, t := range m.Templates {
		if t.Code == code && t.Version > n {
			n = t.Version
		}
	}
	return n, nil
}

func (m *MemoryRepo) CreateTemplate(_ context.Context, t *domain.PromptTemplate) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Templates = append(m.Templates, *t)
	return nil
}

func (m *MemoryRepo) SetTemplateStatus(_ context.Context, id kernel.ID, st domain.TemplateStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.Templates {
		if m.Templates[i].ID == id {
			if st == domain.TemplateActive {
				for _, o := range m.Templates {
					if o.Code == m.Templates[i].Code && o.Status == domain.TemplateActive && o.ID != id {
						return kernel.ErrConflict // mirrors uq_prompt_templates_active
					}
				}
			}
			m.Templates[i].Status = st
			return nil
		}
	}
	return kernel.ErrNotFound
}

// Seed adds a provider with models (tests).
func (m *MemoryRepo) Seed(code string, kinds []string, models ...domain.Model) domain.Provider {
	p := domain.Provider{ID: kernel.NewID(), Code: code, Name: code, Kinds: kinds, Status: domain.StatusActive,
		SecretRef: strings.ToUpper(code) + "_API_KEY", Config: []byte(`{}`)}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Providers = append(m.Providers, p)
	for _, x := range models {
		if x.ID == kernel.NilID {
			x.ID = kernel.NewID()
		}
		x.ProviderID, x.ProviderCode, x.ProviderStatus = p.ID, p.Code, p.Status
		if x.Status == "" {
			x.Status = domain.StatusActive
		}
		if x.Capabilities == nil {
			x.Capabilities = []byte(`{}`)
		}
		if x.ConcurrencyLimit == 0 {
			x.ConcurrencyLimit = 2
		}
		m.Models = append(m.Models, x)
	}
	return p
}
