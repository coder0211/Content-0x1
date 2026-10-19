package usecase

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"time"

	"github.com/be/acs/api/internal/modules/provider/domain"
	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
)

// ProviderView is what the admin API shows. It never carries a secret
// value: only the secret_ref name and whether it resolves.
type ProviderView struct {
	domain.Provider
	SecretConfigured bool
	AdapterAvailable bool
}

// ModelView is a model with its decoded capabilities.
type ModelView struct {
	domain.Model
	AdapterAvailable bool
}

func (s *Service) providerView(p domain.Provider) ProviderView {
	v := ProviderView{Provider: p, AdapterAvailable: true}
	if s.Secrets != nil {
		v.SecretConfigured = p.SecretRef == "none" || s.Secrets(p.SecretRef)
	}
	if s.Registry != nil {
		v.AdapterAvailable = false
		for _, k := range p.Kinds {
			v.AdapterAvailable = v.AdapterAvailable || s.adapterAvailable(k, p.Code)
		}
	}
	return v
}

// AdminListProviders lists providers (provider.view).
func (s *Service) AdminListProviders(ctx context.Context, actor kernel.Actor) ([]ProviderView, error) {
	if err := actor.Require(kernel.PermProviderView); err != nil {
		return nil, err
	}
	ps, err := s.Repo.ListProviders(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ProviderView, 0, len(ps))
	for _, p := range ps {
		out = append(out, s.providerView(p))
	}
	return out, nil
}

// UpdateProviderInput is a partial update (nil = unchanged).
type UpdateProviderInput struct {
	Status    *string
	SecretRef *string
	Config    json.RawMessage
}

// auditProvider is the audited shape (secret_ref is a name, safe to log).
func auditProvider(p domain.Provider) map[string]any {
	return map[string]any{"code": p.Code, "status": p.Status, "secret_ref": p.SecretRef, "config_json": p.Config}
}

// UpdateProvider enables/disables a provider, changes secret_ref or
// config_json (provider.manage), audits, and applies it to the live
// registry of this process (workers refresh periodically).
func (s *Service) UpdateProvider(ctx context.Context, actor kernel.Actor, id kernel.ID, in UpdateProviderInput) (ProviderView, error) {
	if err := actor.Require(kernel.PermProviderManage); err != nil {
		return ProviderView{}, err
	}
	var v kernel.ValidationErrors
	if in.Status != nil && !domain.Status(*in.Status).Valid() {
		v.AddDetail(kernel.Detail{"field": "status", "issue": "invalid", "supported": []string{"active", "disabled"}})
	}
	if in.SecretRef != nil {
		domain.ValidateSecretRef(&v, *in.SecretRef)
	}
	if in.Config != nil {
		domain.ValidateConfig(&v, in.Config)
	}
	if err := v.Err(); err != nil {
		return ProviderView{}, err
	}
	var out domain.Provider
	err := s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		p, err := s.Repo.GetProviderForUpdate(ctx, id)
		if err != nil {
			return err
		}
		before := auditProvider(*p)
		if in.Status != nil {
			p.Status = domain.Status(*in.Status)
		}
		if in.SecretRef != nil {
			p.SecretRef = *in.SecretRef
		}
		if in.Config != nil {
			p.Config = in.Config
		}
		p.UpdatedAt = s.Clock.Now()
		if err := s.Repo.UpdateProvider(ctx, p); err != nil {
			return err
		}
		out = *p
		return s.Audit.Record(ctx, "provider.update", "ai_provider", p.ID.String(), before, auditProvider(*p))
	})
	if err != nil {
		return ProviderView{}, err
	}
	if s.Registry != nil {
		s.Registry.ApplyProviderConfig(out.Code, out.Status == domain.StatusActive, out.SecretRef, out.Config)
	}
	return s.providerView(out), nil
}

// AdminListModels lists models, optionally of one kind (provider.view).
func (s *Service) AdminListModels(ctx context.Context, actor kernel.Actor, kind string) ([]ModelView, error) {
	if err := actor.Require(kernel.PermProviderView); err != nil {
		return nil, err
	}
	ms, err := s.Repo.ListModels(ctx)
	if err != nil {
		return nil, err
	}
	out := []ModelView{}
	for _, m := range ms {
		if kind == "" || m.Kind == kind {
			out = append(out, ModelView{Model: m, AdapterAvailable: s.adapterAvailable(m.Kind, m.ProviderCode)})
		}
	}
	return out, nil
}

// GetModel returns one model (provider.view).
func (s *Service) GetModel(ctx context.Context, actor kernel.Actor, id kernel.ID) (ModelView, error) {
	if err := actor.Require(kernel.PermProviderView); err != nil {
		return ModelView{}, err
	}
	m, err := s.Repo.GetModel(ctx, id)
	if err != nil {
		return ModelView{}, err
	}
	return ModelView{Model: *m, AdapterAvailable: s.adapterAvailable(m.Kind, m.ProviderCode)}, nil
}

// validateCapabilities strictly decodes capabilities: unknown keys and
// wrong types are rejected (a typo would silently disable a check).
func validateCapabilities(v *kernel.ValidationErrors, raw json.RawMessage) {
	if len(raw) > 16<<10 {
		v.Add("capabilities", "too_large")
		return
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var c ports.Capabilities
	if err := dec.Decode(&c); err != nil || bytes.TrimSpace(raw)[0] != '{' {
		reason := "must_be_object"
		if err != nil {
			reason = err.Error()
		}
		v.AddDetail(kernel.Detail{"field": "capabilities", "issue": "invalid", "reason": reason})
		return
	}
	if c.MinDurationSec > 0 && c.MaxDurationSec > 0 && c.MinDurationSec > c.MaxDurationSec {
		v.Add("capabilities.min_duration_sec", "greater_than_max")
	}
	if c.MaxVariants < 0 || c.MaxOutputTokens < 0 || c.MinDurationSec < 0 {
		v.Add("capabilities", "negative_value")
	}
}

// CreateModelInput registers a model of a provider.
type CreateModelInput struct {
	ProviderID       kernel.ID
	ModelCode        string
	Kind             string
	Capabilities     json.RawMessage
	ConcurrencyLimit int
	IsDefault        bool
	Status           string
}

func auditModel(m domain.Model) map[string]any {
	return map[string]any{"provider_code": m.ProviderCode, "model_code": m.ModelCode, "kind": m.Kind, "capabilities": m.Capabilities,
		"concurrency_limit": m.ConcurrencyLimit, "is_default": m.IsDefault, "status": m.Status}
}

// CreateModel adds a model (provider.manage).
func (s *Service) CreateModel(ctx context.Context, actor kernel.Actor, in CreateModelInput) (ModelView, error) {
	if err := actor.Require(kernel.PermProviderManage); err != nil {
		return ModelView{}, err
	}
	if in.Status == "" {
		in.Status = string(domain.StatusActive)
	}
	if in.Capabilities == nil {
		in.Capabilities = json.RawMessage(`{}`)
	}
	var v kernel.ValidationErrors
	domain.ValidateModelCode(&v, in.ModelCode)
	if !slices.Contains(domain.Kinds, in.Kind) {
		v.AddDetail(kernel.Detail{"field": "kind", "issue": "invalid", "supported": domain.Kinds})
	}
	validateCapabilities(&v, in.Capabilities)
	domain.ValidateConcurrency(&v, in.ConcurrencyLimit)
	if !domain.Status(in.Status).Valid() {
		v.AddDetail(kernel.Detail{"field": "status", "issue": "invalid", "supported": []string{"active", "disabled"}})
	}
	if err := v.Err(); err != nil {
		return ModelView{}, err
	}
	var out domain.Model
	err := s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		p, err := s.Repo.GetProvider(ctx, in.ProviderID)
		if err != nil {
			if kernel.ErrorCode(err) == kernel.CodeNotFound {
				return kernel.Validation(kernel.FieldIssue("provider_id", "not_found"))
			}
			return err
		}
		if !slices.Contains(p.Kinds, in.Kind) {
			return kernel.Validation(kernel.Detail{"field": "kind", "issue": "not_offered_by_provider", "supported": p.Kinds})
		}
		m := domain.Model{ID: kernel.NewID(), ProviderID: p.ID, ProviderCode: p.Code, ProviderStatus: p.Status, ModelCode: in.ModelCode,
			Kind: in.Kind, Capabilities: in.Capabilities, ConcurrencyLimit: in.ConcurrencyLimit,
			IsDefault: in.IsDefault && in.Status == string(domain.StatusActive), Status: domain.Status(in.Status)}
		if m.IsDefault {
			if err := s.Repo.ClearDefault(ctx, m.Kind, m.ID); err != nil {
				return err
			}
		}
		if err := s.Repo.CreateModel(ctx, &m); err != nil {
			return err
		}
		out = m
		return s.Audit.Record(ctx, "model.create", "ai_model", m.ID.String(), nil, auditModel(m))
	})
	if err != nil {
		return ModelView{}, err
	}
	return ModelView{Model: out, AdapterAvailable: s.adapterAvailable(out.Kind, out.ProviderCode)}, nil
}

// UpdateModelInput is a partial update (nil = unchanged).
type UpdateModelInput struct {
	Capabilities     json.RawMessage
	ConcurrencyLimit *int
	IsDefault        *bool
	Status           *string
}

// UpdateModel changes capabilities, concurrency, default flag or status
// (provider.manage). Making a model default clears the previous default of
// its kind; disabling a model clears its default flag.
func (s *Service) UpdateModel(ctx context.Context, actor kernel.Actor, id kernel.ID, in UpdateModelInput) (ModelView, error) {
	if err := actor.Require(kernel.PermProviderManage); err != nil {
		return ModelView{}, err
	}
	var v kernel.ValidationErrors
	if in.Capabilities != nil {
		validateCapabilities(&v, in.Capabilities)
	}
	if in.ConcurrencyLimit != nil {
		domain.ValidateConcurrency(&v, *in.ConcurrencyLimit)
	}
	if in.Status != nil && !domain.Status(*in.Status).Valid() {
		v.AddDetail(kernel.Detail{"field": "status", "issue": "invalid", "supported": []string{"active", "disabled"}})
	}
	if err := v.Err(); err != nil {
		return ModelView{}, err
	}
	var out domain.Model
	err := s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		m, err := s.Repo.GetModelForUpdate(ctx, id)
		if err != nil {
			return err
		}
		before := auditModel(*m)
		if in.Capabilities != nil {
			m.Capabilities = in.Capabilities
		}
		if in.ConcurrencyLimit != nil {
			m.ConcurrencyLimit = *in.ConcurrencyLimit
		}
		if in.Status != nil {
			m.Status = domain.Status(*in.Status)
		}
		if in.IsDefault != nil {
			if *in.IsDefault && m.Status != domain.StatusActive {
				return kernel.Validation(kernel.FieldIssue("is_default", "model_disabled"))
			}
			m.IsDefault = *in.IsDefault
		}
		if m.Status != domain.StatusActive {
			m.IsDefault = false
		}
		if m.IsDefault {
			if err := s.Repo.ClearDefault(ctx, m.Kind, m.ID); err != nil {
				return err
			}
		}
		if err := s.Repo.UpdateModel(ctx, m); err != nil {
			return err
		}
		out = *m
		return s.Audit.Record(ctx, "model.update", "ai_model", m.ID.String(), before, auditModel(*m))
	})
	if err != nil {
		return ModelView{}, err
	}
	return ModelView{Model: out, AdapterAvailable: s.adapterAvailable(out.Kind, out.ProviderCode)}, nil
}

// PricingVersion groups the rows of one version.
type PricingVersion struct {
	Version       string
	Currency      string
	EffectiveFrom time.Time
	EffectiveTo   *time.Time
	Current       bool
	Items         []domain.Price
}

// ListPricing returns a model's pricing versions, newest first (provider.view).
func (s *Service) ListPricing(ctx context.Context, actor kernel.Actor, modelID kernel.ID) ([]PricingVersion, error) {
	if err := actor.Require(kernel.PermProviderView); err != nil {
		return nil, err
	}
	if _, err := s.Repo.GetModel(ctx, modelID); err != nil {
		return nil, err
	}
	rows, err := s.Repo.ListPrices(ctx, modelID)
	if err != nil {
		return nil, err
	}
	return groupPricing(rows, s.Clock.Now()), nil
}

func groupPricing(rows []domain.Price, now time.Time) []PricingVersion {
	idx := map[string]int{}
	var out []PricingVersion
	for _, r := range rows {
		i, ok := idx[r.PricingVersion]
		if !ok {
			i = len(out)
			idx[r.PricingVersion] = i
			out = append(out, PricingVersion{Version: r.PricingVersion, Currency: r.Currency, EffectiveFrom: r.EffectiveFrom, EffectiveTo: r.EffectiveTo})
		}
		out[i].Items = append(out[i].Items, r)
	}
	slices.SortStableFunc(out, func(a, b PricingVersion) int { return b.EffectiveFrom.Compare(a.EffectiveFrom) })
	for i := range out {
		p := &out[i]
		p.Current = !p.EffectiveFrom.After(now) && (p.EffectiveTo == nil || p.EffectiveTo.After(now))
		slices.SortFunc(p.Items, func(a, b domain.Price) int {
			if a.Unit < b.Unit {
				return -1
			}
			if a.Unit > b.Unit {
				return 1
			}
			return 0
		})
	}
	if out == nil {
		out = []PricingVersion{}
	}
	return out
}

// AddPricingInput is a new pricing version. EffectiveFrom nil = now.
type AddPricingInput struct {
	Version       string
	Currency      string
	EffectiveFrom *time.Time
	Items         []domain.PriceItem
}

// AddPricingVersion appends a version (provider.manage). Existing rows are
// never modified except for closing the open version's effective_to at the
// new effective_from (also enforced by a DB trigger, migration 0071).
func (s *Service) AddPricingVersion(ctx context.Context, actor kernel.Actor, modelID kernel.ID, in AddPricingInput) (PricingVersion, error) {
	if err := actor.Require(kernel.PermProviderManage); err != nil {
		return PricingVersion{}, err
	}
	if in.Currency == "" {
		in.Currency = kernel.CurrencyUSD
	}
	now := s.Clock.Now().UTC()
	from := now
	if in.EffectiveFrom != nil {
		from = in.EffectiveFrom.UTC()
		if from.Before(now.Add(-5 * time.Minute)) {
			return PricingVersion{}, kernel.Validation(kernel.FieldIssue("effective_from", "in_past"))
		}
	}
	var out PricingVersion
	err := s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		m, err := s.Repo.GetModelForUpdate(ctx, modelID)
		if err != nil {
			return err
		}
		if err := domain.ValidatePricing(m.Kind, in.Version, in.Currency, in.Items); err != nil {
			return err
		}
		exists, err := s.Repo.PricingVersionExists(ctx, modelID, in.Version)
		if err != nil {
			return err
		}
		if exists {
			return kernel.ErrConflict.WithMessage("Phiên bản bảng giá đã tồn tại; hãy tạo phiên bản mới").
				WithDetails(kernel.FieldIssue("pricing_version", "already_exists"))
		}
		prev, err := s.Repo.ListPrices(ctx, modelID)
		if err != nil {
			return err
		}
		for _, p := range prev {
			if !from.After(p.EffectiveFrom) {
				return kernel.Validation(kernel.Detail{"field": "effective_from", "issue": "not_after_latest_version",
					"latest": p.EffectiveFrom.Format(time.RFC3339)})
			}
		}
		if err := s.Repo.ClosePrices(ctx, modelID, from); err != nil {
			return err
		}
		rows := make([]domain.Price, 0, len(in.Items))
		for _, it := range in.Items {
			rows = append(rows, domain.Price{ID: kernel.NewID(), ModelID: modelID, PricingVersion: in.Version, Unit: it.Unit,
				UnitPrice: it.UnitPrice, Currency: in.Currency, EffectiveFrom: from})
		}
		if err := s.Repo.InsertPrices(ctx, rows); err != nil {
			return err
		}
		out = groupPricing(rows, now)[0]
		items := map[string]string{}
		for _, r := range rows {
			items[r.Unit] = r.UnitPrice.String()
		}
		return s.Audit.Record(ctx, "pricing.add", "ai_model", modelID.String(), nil, map[string]any{
			"model_code": m.ModelCode, "pricing_version": in.Version, "currency": in.Currency,
			"effective_from": from.Format(time.RFC3339), "unit_prices": items})
	})
	return out, err
}
