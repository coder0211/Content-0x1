// Package http serves the provider-configuration admin API (SPEC-04 §2.9):
// /settings/providers, /settings/models (+ pricing) and
// /settings/prompt-templates. Responses never contain secret values — only
// the secret_ref name and whether it resolves (ACS-090 AC).
package http

import (
	"encoding/json"
	"net/http"
	"slices"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/shopspring/decimal"

	"github.com/be/acs/api/internal/modules/provider/domain"
	"github.com/be/acs/api/internal/modules/provider/usecase"
	"github.com/be/acs/api/internal/platform/httpx"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Handler serves the provider module routes.
type Handler struct{ Svc *usecase.Service }

// Routes registers routes. Coarse RBAC here; fine-grained checks (manage
// vs view, Lead on templates) are in the use cases.
func (h *Handler) Routes(r chi.Router) {
	r.Route("/settings", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(httpx.RequirePermission(kernel.PermProviderView))
			r.Get("/providers", h.listProviders)
			r.Get("/models", h.listModels)
			r.Get("/models/{id}", h.getModel)
			r.Get("/models/{id}/pricing", h.listPricing)
			r.Get("/prompt-templates", h.listTemplates)
			r.Get("/prompt-templates/{id}", h.getTemplate)
			r.Post("/prompt-templates", h.createTemplate)
			r.Post("/prompt-templates/{id}/activate", h.activateTemplate)
			r.Post("/prompt-templates/{id}/retire", h.retireTemplate)
		})
		r.Group(func(r chi.Router) {
			r.Use(httpx.RequirePermission(kernel.PermProviderManage))
			r.Patch("/providers/{id}", h.updateProvider)
			r.Post("/models", h.createModel)
			r.Patch("/models/{id}", h.updateModel)
			r.Post("/models/{id}/pricing", h.addPricing)
		})
	})
}

type providerDTO struct {
	ID               kernel.ID       `json:"id"`
	Code             string          `json:"code"`
	Name             string          `json:"name"`
	Kinds            []string        `json:"kinds"`
	Status           string          `json:"status"`
	SecretRef        string          `json:"secret_ref"`
	SecretConfigured bool            `json:"secret_configured"`
	AdapterAvailable bool            `json:"adapter_available"`
	Config           json.RawMessage `json:"config_json"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

func presentProvider(v usecase.ProviderView) providerDTO {
	kinds := v.Kinds
	if kinds == nil {
		kinds = []string{}
	}
	cfg := v.Config
	if len(cfg) == 0 {
		cfg = json.RawMessage(`{}`)
	}
	return providerDTO{ID: v.ID, Code: v.Code, Name: v.Name, Kinds: kinds, Status: string(v.Status), SecretRef: v.SecretRef,
		SecretConfigured: v.SecretConfigured, AdapterAvailable: v.AdapterAvailable, Config: cfg, UpdatedAt: v.UpdatedAt.UTC()}
}

type modelDTO struct {
	ID               kernel.ID       `json:"id"`
	ProviderID       kernel.ID       `json:"provider_id"`
	ProviderCode     string          `json:"provider_code"`
	ProviderStatus   string          `json:"provider_status"`
	ModelCode        string          `json:"model_code"`
	Kind             string          `json:"kind"`
	Capabilities     json.RawMessage `json:"capabilities"`
	ConcurrencyLimit int             `json:"concurrency_limit"`
	IsDefault        bool            `json:"is_default"`
	Status           string          `json:"status"`
	AdapterAvailable bool            `json:"adapter_available"`
	Usable           bool            `json:"usable"`
}

func presentModel(m usecase.ModelView) modelDTO {
	caps := m.Capabilities
	if len(caps) == 0 {
		caps = json.RawMessage(`{}`)
	}
	return modelDTO{ID: m.ID, ProviderID: m.ProviderID, ProviderCode: m.ProviderCode, ProviderStatus: string(m.ProviderStatus),
		ModelCode: m.ModelCode, Kind: m.Kind, Capabilities: caps, ConcurrencyLimit: m.ConcurrencyLimit, IsDefault: m.IsDefault,
		Status: string(m.Status), AdapterAvailable: m.AdapterAvailable, Usable: m.Usable() && m.AdapterAvailable}
}

type priceItemDTO struct {
	Unit      string          `json:"unit"`
	UnitPrice decimal.Decimal `json:"unit_price"`
}

type pricingDTO struct {
	PricingVersion string         `json:"pricing_version"`
	Currency       string         `json:"currency"`
	EffectiveFrom  time.Time      `json:"effective_from"`
	EffectiveTo    *time.Time     `json:"effective_to"`
	IsCurrent      bool           `json:"is_current"`
	Items          []priceItemDTO `json:"items"`
}

func presentPricing(p usecase.PricingVersion) pricingDTO {
	out := pricingDTO{PricingVersion: p.Version, Currency: p.Currency, EffectiveFrom: p.EffectiveFrom.UTC(), IsCurrent: p.Current, Items: []priceItemDTO{}}
	if p.EffectiveTo != nil {
		t := p.EffectiveTo.UTC()
		out.EffectiveTo = &t
	}
	for _, it := range p.Items {
		out.Items = append(out.Items, priceItemDTO{Unit: it.Unit, UnitPrice: it.UnitPrice})
	}
	return out
}

type templateDTO struct {
	ID           kernel.ID       `json:"id"`
	Code         string          `json:"code"`
	Version      int             `json:"version"`
	Status       string          `json:"status"`
	SystemPrompt string          `json:"system_prompt"`
	UserTemplate string          `json:"user_template"`
	OutputSchema json.RawMessage `json:"output_schema"`
	CreatedBy    *kernel.ID      `json:"created_by"`
	CreatedAt    time.Time       `json:"created_at"`
}

func presentTemplate(t *domain.PromptTemplate) templateDTO {
	schema := t.OutputSchema
	if len(schema) == 0 {
		schema = json.RawMessage(`null`)
	}
	return templateDTO{ID: t.ID, Code: t.Code, Version: t.Version, Status: string(t.Status), SystemPrompt: t.SystemPrompt,
		UserTemplate: t.UserTemplate, OutputSchema: schema, CreatedBy: t.CreatedBy, CreatedAt: t.CreatedAt.UTC()}
}

func pathID(w http.ResponseWriter, r *http.Request) (kernel.ID, bool) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return kernel.NilID, false
	}
	return id, true
}

func (h *Handler) listProviders(w http.ResponseWriter, r *http.Request) {
	ps, err := h.Svc.AdminListProviders(r.Context(), httpx.ActorFrom(r))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := make([]providerDTO, 0, len(ps))
	for _, p := range ps {
		out = append(out, presentProvider(p))
	}
	httpx.List(w, r, out, nil)
}

type updateProviderRequest struct {
	Status    *string         `json:"status"`
	SecretRef *string         `json:"secret_ref"`
	Config    json.RawMessage `json:"config_json"`
}

func (h *Handler) updateProvider(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req updateProviderRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	v, err := h.Svc.UpdateProvider(r.Context(), httpx.ActorFrom(r), id, usecase.UpdateProviderInput{Status: req.Status, SecretRef: req.SecretRef, Config: req.Config})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.OK(w, r, presentProvider(v))
}

func (h *Handler) listModels(w http.ResponseWriter, r *http.Request) {
	kind := r.URL.Query().Get("kind")
	if kind != "" && !slices.Contains(domain.Kinds, kind) {
		httpx.Error(w, r, kernel.Validation(kernel.Detail{"field": "kind", "issue": "invalid", "supported": domain.Kinds}))
		return
	}
	ms, err := h.Svc.AdminListModels(r.Context(), httpx.ActorFrom(r), kind)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := make([]modelDTO, 0, len(ms))
	for _, m := range ms {
		out = append(out, presentModel(m))
	}
	httpx.List(w, r, out, nil)
}

func (h *Handler) getModel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	m, err := h.Svc.GetModel(r.Context(), httpx.ActorFrom(r), id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.OK(w, r, presentModel(m))
}

type createModelRequest struct {
	ProviderID       kernel.ID       `json:"provider_id"`
	ModelCode        string          `json:"model_code"`
	Kind             string          `json:"kind"`
	Capabilities     json.RawMessage `json:"capabilities"`
	ConcurrencyLimit int             `json:"concurrency_limit"`
	IsDefault        bool            `json:"is_default"`
	Status           string          `json:"status"`
}

func (h *Handler) createModel(w http.ResponseWriter, r *http.Request) {
	var req createModelRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	m, err := h.Svc.CreateModel(r.Context(), httpx.ActorFrom(r), usecase.CreateModelInput{ProviderID: req.ProviderID, ModelCode: req.ModelCode,
		Kind: req.Kind, Capabilities: req.Capabilities, ConcurrencyLimit: req.ConcurrencyLimit, IsDefault: req.IsDefault, Status: req.Status})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.Created(w, r, presentModel(m))
}

type updateModelRequest struct {
	Capabilities     json.RawMessage `json:"capabilities"`
	ConcurrencyLimit *int            `json:"concurrency_limit"`
	IsDefault        *bool           `json:"is_default"`
	Status           *string         `json:"status"`
}

func (h *Handler) updateModel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req updateModelRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	m, err := h.Svc.UpdateModel(r.Context(), httpx.ActorFrom(r), id, usecase.UpdateModelInput{Capabilities: req.Capabilities,
		ConcurrencyLimit: req.ConcurrencyLimit, IsDefault: req.IsDefault, Status: req.Status})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.OK(w, r, presentModel(m))
}

func (h *Handler) listPricing(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	ps, err := h.Svc.ListPricing(r.Context(), httpx.ActorFrom(r), id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := make([]pricingDTO, 0, len(ps))
	for _, p := range ps {
		out = append(out, presentPricing(p))
	}
	httpx.List(w, r, out, nil)
}

type addPricingRequest struct {
	PricingVersion string         `json:"pricing_version"`
	Currency       string         `json:"currency"`
	EffectiveFrom  *time.Time     `json:"effective_from"`
	Items          []priceItemDTO `json:"items"`
}

func (h *Handler) addPricing(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req addPricingRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	items := make([]domain.PriceItem, 0, len(req.Items))
	for _, it := range req.Items {
		items = append(items, domain.PriceItem{Unit: it.Unit, UnitPrice: it.UnitPrice})
	}
	p, err := h.Svc.AddPricingVersion(r.Context(), httpx.ActorFrom(r), id, usecase.AddPricingInput{Version: req.PricingVersion,
		Currency: req.Currency, EffectiveFrom: req.EffectiveFrom, Items: items})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.Created(w, r, presentPricing(p))
}

func (h *Handler) listTemplates(w http.ResponseWriter, r *http.Request) {
	ts, err := h.Svc.ListTemplates(r.Context(), httpx.ActorFrom(r), r.URL.Query().Get("code"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := make([]templateDTO, 0, len(ts))
	for i := range ts {
		out = append(out, presentTemplate(&ts[i]))
	}
	httpx.List(w, r, out, nil)
}

func (h *Handler) getTemplate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	t, err := h.Svc.GetTemplate(r.Context(), httpx.ActorFrom(r), id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.OK(w, r, presentTemplate(t))
}

type createTemplateRequest struct {
	Code         string          `json:"code"`
	SystemPrompt string          `json:"system_prompt"`
	UserTemplate string          `json:"user_template"`
	OutputSchema json.RawMessage `json:"output_schema"`
	Activate     bool            `json:"activate"`
}

func (h *Handler) createTemplate(w http.ResponseWriter, r *http.Request) {
	var req createTemplateRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	t, err := h.Svc.CreateTemplateVersion(r.Context(), httpx.ActorFrom(r), usecase.CreateTemplateInput{Code: req.Code,
		SystemPrompt: req.SystemPrompt, UserTemplate: req.UserTemplate, OutputSchema: req.OutputSchema, Activate: req.Activate})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.Created(w, r, presentTemplate(t))
}

func (h *Handler) activateTemplate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	t, err := h.Svc.ActivateTemplate(r.Context(), httpx.ActorFrom(r), id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.OK(w, r, presentTemplate(t))
}

func (h *Handler) retireTemplate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	t, err := h.Svc.RetireTemplate(r.Context(), httpx.ActorFrom(r), id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.OK(w, r, presentTemplate(t))
}
