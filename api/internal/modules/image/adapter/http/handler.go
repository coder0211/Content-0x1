// Package http serves the Image Studio endpoints (SPEC-04 §2.5).
package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/be/acs/api/internal/modules/image/domain"
	"github.com/be/acs/api/internal/modules/image/usecase"
	"github.com/be/acs/api/internal/platform/httpx"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Handler serves /images/*.
type Handler struct{ Svc usecase.API }

// Routes registers the routes; campaign/asset access is checked in the use
// cases (IDOR → 404).
func (h *Handler) Routes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(httpx.RequirePermission(kernel.PermGenerateRun))
		r.Get("/images/generations", h.list)
		r.Get("/images/references", h.references)
		r.Post("/images/estimate", h.estimate)
		r.With(httpx.RequireIdempotencyKey).Post("/images/generations", h.generate)
		r.With(httpx.RequireIdempotencyKey).Post("/images/{assetId}/edits", h.edit)
		r.With(httpx.RequireIdempotencyKey).Post("/images/{assetId}/variations", h.vary)
	})
}

func idem(r *http.Request) httpx.Idempotency {
	i, _ := httpx.IdempotencyFrom(r)
	return i
}

func (h *Handler) generate(w http.ResponseWriter, r *http.Request) {
	var req generateRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	i := idem(r)
	res, err := h.Svc.Generate(r.Context(), httpx.ActorFrom(r), usecase.GenerateInput{
		CampaignID: req.CampaignID, BrandID: req.BrandID, Prompt: req.Prompt, Preset: preset(req.Preset),
		ReferenceFileIDs: req.ReferenceFileIDs, AspectRatio: req.AspectRatio, Quality: req.Quality, Count: req.Count,
		ModelCode: str(req.ModelCode), BudgetOverride: req.BudgetOverride, IdempotencyKey: i.Key, RequestHash: i.RequestHash,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.Accepted(w, r, presentAccepted(res))
}

func (h *Handler) edit(w http.ResponseWriter, r *http.Request) {
	assetID, err := httpx.PathID(r, "assetId")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var req editRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	i := idem(r)
	res, err := h.Svc.Edit(r.Context(), httpx.ActorFrom(r), assetID, usecase.EditInput{
		FileID: req.FileID, Prompt: req.Prompt, MaskFileID: req.MaskFileID, Quality: req.Quality,
		BudgetOverride: req.BudgetOverride, IdempotencyKey: i.Key, RequestHash: i.RequestHash,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.Accepted(w, r, presentAccepted(res))
}

func (h *Handler) vary(w http.ResponseWriter, r *http.Request) {
	assetID, err := httpx.PathID(r, "assetId")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var req variationRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	i := idem(r)
	res, err := h.Svc.Vary(r.Context(), httpx.ActorFrom(r), assetID, usecase.VariationInput{
		FileID: req.FileID, Count: req.Count, Prompt: req.Prompt, BudgetOverride: req.BudgetOverride,
		IdempotencyKey: i.Key, RequestHash: i.RequestHash,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.Accepted(w, r, presentAccepted(res))
}

func (h *Handler) estimate(w http.ResponseWriter, r *http.Request) {
	var req estimateRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	res, err := h.Svc.Estimate(r.Context(), httpx.ActorFrom(r), usecase.EstimateInput{
		CampaignID: req.CampaignID, Operation: domain.Operation(req.Operation), AspectRatio: req.AspectRatio,
		Quality: req.Quality, Count: req.Count, ModelCode: str(req.ModelCode),
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.OK(w, r, presentEstimate(res))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	p, err := httpx.PageRequest(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	campaignID, err := httpx.QueryID(r, "campaign_id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	assetID, err := httpx.QueryID(r, "asset_id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	page, err := h.Svc.ListBatches(r.Context(), httpx.ActorFrom(r), usecase.BatchFilter{CampaignID: campaignID, AssetID: assetID}, p)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	items := make([]batchDTO, 0, len(page.Items))
	for _, b := range page.Items {
		items = append(items, presentBatch(b))
	}
	httpx.List(w, r, items, page.NextCursor)
}

func (h *Handler) references(w http.ResponseWriter, r *http.Request) {
	campaignID, err := httpx.QueryID(r, "campaign_id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	brandID, err := httpx.QueryID(r, "brand_id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	refs, err := h.Svc.ListReferences(r.Context(), httpx.ActorFrom(r), usecase.ReferenceFilter{CampaignID: campaignID, BrandID: brandID})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	items := make([]referenceDTO, 0, len(refs))
	for _, ref := range refs {
		items = append(items, presentReference(ref))
	}
	httpx.List(w, r, items, nil)
}
