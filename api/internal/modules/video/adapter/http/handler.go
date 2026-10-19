// Package http is the Video Studio HTTP adapter (SPEC-04 §2.6).
package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/be/acs/api/internal/modules/video/domain"
	"github.com/be/acs/api/internal/modules/video/usecase"
	"github.com/be/acs/api/internal/platform/httpx"
	"github.com/be/acs/api/internal/shared/kernel"
)

func init() {
	httpx.RegisterErrorCode(usecase.CodeMusicNotLicensed, http.StatusUnprocessableEntity)
	httpx.RegisterErrorCode(usecase.CodeJobInProgress, http.StatusConflict)
	httpx.RegisterErrorCode(usecase.CodeQualityCheck, http.StatusUnprocessableEntity)
}

// Handler serves /videos.
type Handler struct{ Svc *usecase.Service }

// Routes registers the video routes on the /api/v1 router.
func (h *Handler) Routes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(httpx.RequirePermission(kernel.PermBrandView))
		r.Get("/videos", h.list)
		r.Get("/videos/{id}", h.get)
	})
	r.Group(func(r chi.Router) {
		r.Use(httpx.RequirePermission(kernel.PermGenerateRun))
		r.With(httpx.RequireIdempotencyKey).Post("/videos", h.create)
		r.Put("/videos/{id}/scenes", h.saveScenes)
		r.Post("/videos/{id}/estimate", h.estimate)
		r.With(httpx.RequireIdempotencyKey).Post("/videos/{id}/productions", h.produce)
		r.With(httpx.RequireIdempotencyKey).Post("/videos/{id}/scenes/{sceneId}/regenerations", h.regenerate)
		r.With(httpx.RequireIdempotencyKey).Post("/videos/{id}/renders", h.render)
	})
}

func idem(r *http.Request) usecase.Idempotency {
	i, _ := httpx.IdempotencyFrom(r)
	return usecase.Idempotency{Key: i.Key, RequestHash: i.RequestHash}
}

// decodeOptional accepts an empty body as the zero value.
func decodeOptional(r *http.Request, v any) error {
	if r.ContentLength == 0 {
		return nil
	}
	err := httpx.Decode(r, v)
	if de, ok := kernel.AsDomainError(err); ok && len(de.Details) == 1 && de.Details[0]["field"] == "body" && de.Details[0]["issue"] == "required" {
		return nil
	}
	return err
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	page, err := httpx.PageRequest(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	campaignID, err := httpx.QueryID(r, "campaign_id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	items, next, err := h.Svc.List(r.Context(), httpx.ActorFrom(r), campaignID, page)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := make([]projectDTO, len(items))
	for i, v := range items {
		out[i] = projectFrom(v)
	}
	httpx.List(w, r, out, next)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	v, err := h.Svc.Get(r.Context(), httpx.ActorFrom(r), id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.OK(w, r, projectFrom(v))
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	in := usecase.CreateInput{
		CampaignID: req.CampaignID, BrandID: req.BrandID, Title: s(req.Title), Prompt: s(req.Prompt),
		ScriptFromAssetID: req.ScriptFromAssetID, Preset: req.Preset, AspectRatio: s(req.AspectRatio),
		TargetDurationSeconds: req.TargetDurationSeconds, Resolution: s(req.Resolution),
		Voiceover: req.Voiceover.toDomain(), Subtitles: req.Subtitles.toDomain(), Music: req.Music.toDomain(), Idem: idem(r),
	}
	if req.Source != nil {
		in.Source = domain.Source{Type: domain.SourceType(req.Source.Type), FileIDs: req.Source.FileIDs}
	}
	a, err := h.Svc.Create(r.Context(), httpx.ActorFrom(r), in)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.Accepted(w, r, accepted(a))
}

func (h *Handler) saveScenes(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var req saveScenesRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	in := make([]domain.SceneDraftInput, len(req.Scenes))
	for i, sc := range req.Scenes {
		in[i] = domain.SceneDraftInput{ID: sc.ID, DurationMs: sc.DurationMs, VisualPrompt: sc.VisualPrompt,
			Narration: s(sc.Narration), SubtitleText: s(sc.SubtitleText), SourceFileID: sc.SourceFileID}
	}
	v, err := h.Svc.SaveScenes(r.Context(), httpx.ActorFrom(r), id, in)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.OK(w, r, projectFrom(v))
}

func (h *Handler) estimate(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var req struct{}
	if err := decodeOptional(r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	e, err := h.Svc.Estimate(r.Context(), httpx.ActorFrom(r), id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.OK(w, r, estimateFrom(e))
}

func (h *Handler) produce(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var req produceRequest
	if err := decodeOptional(r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	a, err := h.Svc.Produce(r.Context(), httpx.ActorFrom(r), id, usecase.ProduceInput{BudgetOverride: req.BudgetOverride, Idem: idem(r)})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.Accepted(w, r, accepted(a))
}

func (h *Handler) regenerate(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	sceneID, err := httpx.PathID(r, "sceneId")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var req regenerateRequest
	if err := decodeOptional(r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	a, err := h.Svc.RegenerateScene(r.Context(), httpx.ActorFrom(r), id, usecase.RegenerateInput{
		SceneID: sceneID, VisualPrompt: req.VisualPrompt, BudgetOverride: req.BudgetOverride, Idem: idem(r)})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.Accepted(w, r, accepted(a))
}

func (h *Handler) render(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var req renderRequest
	if err := decodeOptional(r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	in := usecase.RenderInput{BudgetOverride: req.BudgetOverride, Idem: idem(r)}
	if req.Voiceover != nil {
		v := req.Voiceover.toDomain()
		in.Voiceover = &v
	}
	if req.Subtitles != nil {
		v := req.Subtitles.toDomain()
		in.Subtitles = &v
	}
	if req.Music != nil {
		v := req.Music.toDomain()
		in.Music = &v
	}
	a, err := h.Svc.Rerender(r.Context(), httpx.ActorFrom(r), id, in)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.Accepted(w, r, accepted(a))
}
