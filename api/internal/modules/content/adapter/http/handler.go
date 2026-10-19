// Package http serves the content endpoints (SPEC-04 §2.4) plus the two AI
// helpers owned by the content module: campaign plan generation (§2.3) and
// image prompt suggestions (§2.5).
package http

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/be/acs/api/internal/modules/content/usecase"
	"github.com/be/acs/api/internal/platform/httpx"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Service is the subset of usecase.Service used by the handler.
type Service interface {
	Generate(ctx context.Context, actor kernel.Actor, in usecase.GenerateInput) (usecase.JobAccepted, error)
	Rewrite(ctx context.Context, actor kernel.Actor, in usecase.RewriteInput) (usecase.JobAccepted, error)
	Transform(ctx context.Context, actor kernel.Actor, in usecase.TransformInput) (usecase.JobAccepted, error)
	StartContentPlan(ctx context.Context, actor kernel.Actor, in usecase.PlanInput) (usecase.JobAccepted, error)
	SuggestImagePrompts(ctx context.Context, actor kernel.Actor, in usecase.ImagePromptInput) (usecase.JobAccepted, error)
}

// Handler serves the content routes.
type Handler struct{ Svc Service }

// Routes registers every content route; each creates an AI job so it needs
// generate.run and an Idempotency-Key. Ownership checks live in the use cases.
func (h *Handler) Routes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(httpx.RequirePermission(kernel.PermGenerateRun))
		r.Use(httpx.RequireIdempotencyKey)
		r.Post("/content/generations", h.generate)
		r.Post("/content/{assetId}/rewrites", h.rewrite)
		r.Post("/content/{assetId}/transforms", h.transform)
		r.Post("/campaigns/{id}/plan-generations", h.plan)
		r.Post("/images/prompt-suggestions", h.imagePrompts)
	})
}

type moneyDTO struct {
	Amount     json.Number `json:"amount"`
	Currency   string      `json:"currency"`
	IsEstimate bool        `json:"is_estimate"`
}

type jobAcceptedDTO struct {
	JobID         kernel.ID `json:"job_id"`
	Status        string    `json:"status"`
	EstimatedCost moneyDTO  `json:"estimated_cost"`
}

func accepted(w http.ResponseWriter, r *http.Request, a usecase.JobAccepted) {
	cur := a.EstimatedCost.Currency
	if cur == "" {
		cur = kernel.CurrencyUSD
	}
	httpx.Accepted(w, r, jobAcceptedDTO{
		JobID: a.JobID, Status: a.Status,
		EstimatedCost: moneyDTO{Amount: json.Number(a.EstimatedCost.Amount.String()), Currency: cur, IsEstimate: true},
	})
}

type briefDTO struct {
	ProductID      *kernel.ID `json:"product_id"`
	TargetAudience string     `json:"target_audience"`
	KeyMessage     string     `json:"key_message"`
	CTA            string     `json:"cta"`
	Notes          string     `json:"notes"`
	ReferenceText  string     `json:"reference_text"`
}

type generateRequest struct {
	CampaignID     *kernel.ID `json:"campaign_id"`
	BrandID        *kernel.ID `json:"brand_id"`
	Platform       string     `json:"platform"`
	Format         string     `json:"format"`
	ContentType    string     `json:"content_type"`
	Objective      string     `json:"objective"`
	Framework      string     `json:"framework"`
	Length         string     `json:"length"`
	Language       string     `json:"language"`
	Tone           []string   `json:"tone"`
	Brief          briefDTO   `json:"brief"`
	Variants       int        `json:"variants"`
	ModelCode      string     `json:"model_code"`
	BudgetOverride bool       `json:"budget_override"`
}

func (h *Handler) generate(w http.ResponseWriter, r *http.Request) {
	var req generateRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	idem, _ := httpx.IdempotencyFrom(r)
	brandID := kernel.NilID
	if req.BrandID != nil {
		brandID = *req.BrandID
	}
	res, err := h.Svc.Generate(r.Context(), httpx.ActorFrom(r), usecase.GenerateInput{
		CampaignID: req.CampaignID, BrandID: brandID, Platform: req.Platform, Format: req.Format,
		ContentType: req.ContentType, Objective: req.Objective, Framework: req.Framework, Length: req.Length,
		Language: req.Language, Tone: req.Tone, Variants: req.Variants, ModelCode: req.ModelCode,
		BudgetOverride: req.BudgetOverride, IdempotencyKey: idem.Key, RequestHash: idem.RequestHash,
		Brief: usecase.Brief{
			ProductID: req.Brief.ProductID, TargetAudience: req.Brief.TargetAudience, KeyMessage: req.Brief.KeyMessage,
			CTA: req.Brief.CTA, Notes: req.Brief.Notes, ReferenceText: req.Brief.ReferenceText,
		},
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	accepted(w, r, res)
}

type rewriteRequest struct {
	Instruction    string   `json:"instruction"`
	Tone           []string `json:"tone"`
	CustomPrompt   string   `json:"custom_prompt"`
	ModelCode      string   `json:"model_code"`
	BudgetOverride bool     `json:"budget_override"`
}

func (h *Handler) rewrite(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "assetId")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var req rewriteRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	idem, _ := httpx.IdempotencyFrom(r)
	res, err := h.Svc.Rewrite(r.Context(), httpx.ActorFrom(r), usecase.RewriteInput{
		AssetID: id, Instruction: req.Instruction, Tone: req.Tone, CustomPrompt: req.CustomPrompt,
		ModelCode: req.ModelCode, BudgetOverride: req.BudgetOverride, IdempotencyKey: idem.Key, RequestHash: idem.RequestHash,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	accepted(w, r, res)
}

type transformRequest struct {
	TargetType     string `json:"target_type"`
	ModelCode      string `json:"model_code"`
	BudgetOverride bool   `json:"budget_override"`
}

func (h *Handler) transform(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "assetId")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var req transformRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	idem, _ := httpx.IdempotencyFrom(r)
	res, err := h.Svc.Transform(r.Context(), httpx.ActorFrom(r), usecase.TransformInput{
		AssetID: id, TargetType: req.TargetType, ModelCode: req.ModelCode, BudgetOverride: req.BudgetOverride,
		IdempotencyKey: idem.Key, RequestHash: idem.RequestHash,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	accepted(w, r, res)
}

type planRequest struct {
	Language       string `json:"language"`
	ModelCode      string `json:"model_code"`
	BudgetOverride bool   `json:"budget_override"`
}

func (h *Handler) plan(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var req planRequest
	if r.ContentLength != 0 {
		if err := httpx.Decode(r, &req); err != nil {
			httpx.Error(w, r, err)
			return
		}
	}
	idem, _ := httpx.IdempotencyFrom(r)
	res, err := h.Svc.StartContentPlan(r.Context(), httpx.ActorFrom(r), usecase.PlanInput{
		CampaignID: id, Language: req.Language, ModelCode: req.ModelCode, BudgetOverride: req.BudgetOverride,
		IdempotencyKey: idem.Key, RequestHash: idem.RequestHash,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	accepted(w, r, res)
}

type imagePromptsRequest struct {
	CampaignID     *kernel.ID `json:"campaign_id"`
	SourceAssetID  *kernel.ID `json:"source_asset_id"`
	BrandID        *kernel.ID `json:"brand_id"`
	Preset         string     `json:"preset"`
	Description    string     `json:"description"`
	ModelCode      string     `json:"model_code"`
	BudgetOverride bool       `json:"budget_override"`
}

func (h *Handler) imagePrompts(w http.ResponseWriter, r *http.Request) {
	var req imagePromptsRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	idem, _ := httpx.IdempotencyFrom(r)
	res, err := h.Svc.SuggestImagePrompts(r.Context(), httpx.ActorFrom(r), usecase.ImagePromptInput{
		CampaignID: req.CampaignID, SourceAssetID: req.SourceAssetID, BrandID: req.BrandID, Preset: req.Preset,
		Description: req.Description, ModelCode: req.ModelCode, BudgetOverride: req.BudgetOverride,
		IdempotencyKey: idem.Key, RequestHash: idem.RequestHash,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	accepted(w, r, res)
}
