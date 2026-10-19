// Package postgres implements the provider module's Repository with sqlc.
package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/be/acs/api/internal/modules/provider/domain"
	"github.com/be/acs/api/internal/modules/provider/usecase"
	db "github.com/be/acs/api/internal/platform/postgres"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Repository implements usecase.Repository.
type Repository struct{ pool *pgxpool.Pool }

// NewRepository returns a repository on pool.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

var _ usecase.Repository = (*Repository)(nil)

func (r *Repository) q(ctx context.Context) *Queries { return New(db.Conn(ctx, r.pool)) }

func toProvider(p AiProvider) domain.Provider {
	return domain.Provider{ID: p.ID, Code: p.Code, Name: p.Name, Kinds: p.Kind, Status: domain.Status(p.Status),
		SecretRef: p.SecretRef, Config: p.ConfigJson, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt}
}

func modelOf(id, providerID kernel.ID, pcode, pstatus, code, kind string, caps []byte, conc int32, def bool, status string) domain.Model {
	return domain.Model{ID: id, ProviderID: providerID, ProviderCode: pcode, ProviderStatus: domain.Status(pstatus), ModelCode: code,
		Kind: kind, Capabilities: caps, ConcurrencyLimit: int(conc), IsDefault: def, Status: domain.Status(status)}
}

func toTemplate(t PromptTemplate) domain.PromptTemplate {
	return domain.PromptTemplate{ID: t.ID, Code: t.Code, Version: int(t.Version), SystemPrompt: t.SystemPrompt,
		UserTemplate: t.UserTemplate, OutputSchema: t.OutputSchema, Status: domain.TemplateStatus(t.Status),
		CreatedBy: t.CreatedBy, CreatedAt: t.CreatedAt}
}

func notFoundIfZero(n int64, err error) error {
	if err != nil {
		return err
	}
	if n == 0 {
		return kernel.ErrNotFound
	}
	return nil
}

// ListProviders implements usecase.Repository.
func (r *Repository) ListProviders(ctx context.Context) ([]domain.Provider, error) {
	rows, err := r.q(ctx).ListProviders(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Provider, 0, len(rows))
	for _, p := range rows {
		out = append(out, toProvider(p))
	}
	return out, nil
}

// GetProvider implements usecase.Repository.
func (r *Repository) GetProvider(ctx context.Context, id kernel.ID) (*domain.Provider, error) {
	p, err := r.q(ctx).GetProvider(ctx, id)
	if err != nil {
		return nil, db.MapNotFound(err)
	}
	d := toProvider(p)
	return &d, nil
}

// GetProviderForUpdate implements usecase.Repository.
func (r *Repository) GetProviderForUpdate(ctx context.Context, id kernel.ID) (*domain.Provider, error) {
	p, err := r.q(ctx).GetProviderForUpdate(ctx, id)
	if err != nil {
		return nil, db.MapNotFound(err)
	}
	d := toProvider(p)
	return &d, nil
}

// UpdateProvider implements usecase.Repository (updated_at is set by trigger).
func (r *Repository) UpdateProvider(ctx context.Context, p *domain.Provider) error {
	return notFoundIfZero(r.q(ctx).UpdateProvider(ctx, UpdateProviderParams{ID: p.ID, Status: string(p.Status), SecretRef: p.SecretRef, ConfigJson: p.Config}))
}

// ListModels implements usecase.Repository.
func (r *Repository) ListModels(ctx context.Context) ([]domain.Model, error) {
	rows, err := r.q(ctx).ListModels(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Model, 0, len(rows))
	for _, m := range rows {
		out = append(out, modelOf(m.ID, m.ProviderID, m.ProviderCode, m.ProviderStatus, m.ModelCode, m.Kind, m.Capabilities, m.ConcurrencyLimit, m.IsDefault, m.Status))
	}
	return out, nil
}

// GetModel implements usecase.Repository.
func (r *Repository) GetModel(ctx context.Context, id kernel.ID) (*domain.Model, error) {
	m, err := r.q(ctx).GetModel(ctx, id)
	if err != nil {
		return nil, db.MapNotFound(err)
	}
	d := modelOf(m.ID, m.ProviderID, m.ProviderCode, m.ProviderStatus, m.ModelCode, m.Kind, m.Capabilities, m.ConcurrencyLimit, m.IsDefault, m.Status)
	return &d, nil
}

// GetModelForUpdate implements usecase.Repository.
func (r *Repository) GetModelForUpdate(ctx context.Context, id kernel.ID) (*domain.Model, error) {
	m, err := r.q(ctx).GetModelForUpdate(ctx, id)
	if err != nil {
		return nil, db.MapNotFound(err)
	}
	d := modelOf(m.ID, m.ProviderID, m.ProviderCode, m.ProviderStatus, m.ModelCode, m.Kind, m.Capabilities, m.ConcurrencyLimit, m.IsDefault, m.Status)
	return &d, nil
}

// CreateModel implements usecase.Repository.
func (r *Repository) CreateModel(ctx context.Context, m *domain.Model) error {
	err := r.q(ctx).CreateModel(ctx, CreateModelParams{ID: m.ID, ProviderID: m.ProviderID, ModelCode: m.ModelCode, Kind: m.Kind,
		Capabilities: m.Capabilities, ConcurrencyLimit: int32(m.ConcurrencyLimit), IsDefault: m.IsDefault, Status: string(m.Status)})
	if db.IsUniqueViolation(err, "") {
		return kernel.ErrConflict.WithMessage("Model đã tồn tại cho nhà cung cấp này").WithDetails(kernel.FieldIssue("model_code", "already_exists"))
	}
	return err
}

// UpdateModel implements usecase.Repository.
func (r *Repository) UpdateModel(ctx context.Context, m *domain.Model) error {
	n, err := r.q(ctx).UpdateModel(ctx, UpdateModelParams{ID: m.ID, Capabilities: m.Capabilities, ConcurrencyLimit: int32(m.ConcurrencyLimit),
		IsDefault: m.IsDefault, Status: string(m.Status)})
	if db.IsUniqueViolation(err, "uq_models_default_kind") {
		return kernel.ErrVersionConflict
	}
	return notFoundIfZero(n, err)
}

// ClearDefault implements usecase.Repository.
func (r *Repository) ClearDefault(ctx context.Context, kind string, except kernel.ID) error {
	return r.q(ctx).ClearDefault(ctx, ClearDefaultParams{Kind: kind, ID: except})
}

// ListPrices implements usecase.Repository.
func (r *Repository) ListPrices(ctx context.Context, modelID kernel.ID) ([]domain.Price, error) {
	rows, err := r.q(ctx).ListPrices(ctx, modelID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Price, 0, len(rows))
	for _, p := range rows {
		out = append(out, domain.Price{ID: p.ID, ModelID: p.ModelID, PricingVersion: p.PricingVersion, Unit: p.Unit,
			UnitPrice: p.UnitPrice, Currency: p.Currency, EffectiveFrom: p.EffectiveFrom, EffectiveTo: p.EffectiveTo})
	}
	return out, nil
}

// PricingVersionExists implements usecase.Repository.
func (r *Repository) PricingVersionExists(ctx context.Context, modelID kernel.ID, version string) (bool, error) {
	return r.q(ctx).PricingVersionExists(ctx, PricingVersionExistsParams{ModelID: modelID, PricingVersion: version})
}

// ClosePrices implements usecase.Repository.
func (r *Repository) ClosePrices(ctx context.Context, modelID kernel.ID, at time.Time) error {
	return r.q(ctx).ClosePrices(ctx, ClosePricesParams{At: &at, ModelID: modelID})
}

// InsertPrices implements usecase.Repository.
func (r *Repository) InsertPrices(ctx context.Context, prices []domain.Price) error {
	q := r.q(ctx)
	for _, p := range prices {
		err := q.InsertPrice(ctx, InsertPriceParams{ID: p.ID, ModelID: p.ModelID, PricingVersion: p.PricingVersion, Unit: p.Unit,
			UnitPrice: p.UnitPrice, Currency: p.Currency, EffectiveFrom: p.EffectiveFrom})
		if db.IsUniqueViolation(err, "") {
			return kernel.ErrConflict.WithDetails(kernel.FieldIssue("pricing_version", "already_exists"))
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// ListTemplates implements usecase.Repository.
func (r *Repository) ListTemplates(ctx context.Context, code string) ([]domain.PromptTemplate, error) {
	rows, err := r.q(ctx).ListTemplates(ctx, code)
	if err != nil {
		return nil, err
	}
	out := make([]domain.PromptTemplate, 0, len(rows))
	for _, t := range rows {
		out = append(out, toTemplate(t))
	}
	return out, nil
}

// GetTemplate implements usecase.Repository.
func (r *Repository) GetTemplate(ctx context.Context, id kernel.ID) (*domain.PromptTemplate, error) {
	t, err := r.q(ctx).GetTemplate(ctx, id)
	if err != nil {
		return nil, db.MapNotFound(err)
	}
	d := toTemplate(t)
	return &d, nil
}

// GetTemplateForUpdate implements usecase.Repository.
func (r *Repository) GetTemplateForUpdate(ctx context.Context, id kernel.ID) (*domain.PromptTemplate, error) {
	t, err := r.q(ctx).GetTemplateForUpdate(ctx, id)
	if err != nil {
		return nil, db.MapNotFound(err)
	}
	d := toTemplate(t)
	return &d, nil
}

// ActiveTemplate implements usecase.Repository.
func (r *Repository) ActiveTemplate(ctx context.Context, code string) (*domain.PromptTemplate, error) {
	t, err := r.q(ctx).ActiveTemplate(ctx, code)
	if err != nil {
		return nil, db.MapNotFound(err)
	}
	d := toTemplate(t)
	return &d, nil
}

// LockTemplateCode implements usecase.Repository.
func (r *Repository) LockTemplateCode(ctx context.Context, code string) error {
	return r.q(ctx).LockTemplateCode(ctx, code)
}

// MaxTemplateVersion implements usecase.Repository.
func (r *Repository) MaxTemplateVersion(ctx context.Context, code string) (int, error) {
	n, err := r.q(ctx).MaxTemplateVersion(ctx, code)
	return int(n), err
}

// CreateTemplate implements usecase.Repository.
func (r *Repository) CreateTemplate(ctx context.Context, t *domain.PromptTemplate) error {
	err := r.q(ctx).CreateTemplate(ctx, CreateTemplateParams{ID: t.ID, Code: t.Code, Version: int32(t.Version), SystemPrompt: t.SystemPrompt,
		UserTemplate: t.UserTemplate, OutputSchema: t.OutputSchema, Status: string(t.Status), CreatedBy: t.CreatedBy, CreatedAt: t.CreatedAt})
	if db.IsUniqueViolation(err, "") {
		return kernel.ErrVersionConflict
	}
	return err
}

// SetTemplateStatus implements usecase.Repository.
func (r *Repository) SetTemplateStatus(ctx context.Context, id kernel.ID, status domain.TemplateStatus) error {
	n, err := r.q(ctx).SetTemplateStatus(ctx, SetTemplateStatusParams{ID: id, Status: string(status)})
	if db.IsUniqueViolation(err, "uq_prompt_templates_active") {
		return kernel.ErrVersionConflict
	}
	return notFoundIfZero(n, err)
}
