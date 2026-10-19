// Package http serves /campaigns (SPEC-04 §2.3). POST
// /campaigns/{id}/plan-generations creates a content_plan AI job and is
// served by the content module (campaign may not depend on job/budget).
package http

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/be/acs/api/internal/modules/campaign/domain"
	"github.com/be/acs/api/internal/modules/campaign/usecase"
	"github.com/be/acs/api/internal/platform/httpx"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Handler serves campaign routes.
type Handler struct{ Svc *usecase.Service }

// Routes registers routes; ownership checks happen in the use cases.
func (h *Handler) Routes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(httpx.RequirePermission(kernel.PermBrandView))
		r.Get("/campaigns", h.list)
		r.Get("/campaigns/{id}", h.get)
	})
	r.Group(func(r chi.Router) {
		r.Use(httpx.RequirePermission(kernel.PermCampaignManage))
		r.Get("/campaigns/member-candidates", h.memberCandidates)
		r.Post("/campaigns", h.create)
		r.Patch("/campaigns/{id}", h.update)
		r.Put("/campaigns/{id}/plan", h.savePlan)
		r.Delete("/campaigns/{id}/plan-proposal", h.discardProposal)
	})
}

// ---------- DTOs ----------

type memberResponse struct {
	ID    kernel.ID `json:"id"`
	Name  string    `json:"name"`
	Email string    `json:"email"`
}

type campaignResponse struct {
	ID                    kernel.ID       `json:"id"`
	BrandID               kernel.ID       `json:"brand_id"`
	Name                  string          `json:"name"`
	Goal                  string          `json:"goal"`
	Status                domain.Status   `json:"status"`
	NextStatuses          []domain.Status `json:"next_statuses"`
	Brief                 domain.Brief    `json:"brief"`
	StartDate             *string         `json:"start_date"`
	EndDate               *string         `json:"end_date"`
	ContentPlan           *domain.Plan    `json:"content_plan"`
	ContentPlanApprovedBy *kernel.ID      `json:"content_plan_approved_by"`
	ContentPlanApprovedAt *time.Time      `json:"content_plan_approved_at"`
	PlanProposal          *domain.Plan    `json:"content_plan_proposal"`
	PlanProposedAt        *time.Time      `json:"content_plan_proposed_at"`
	MemberIDs             []kernel.ID     `json:"member_ids"`
	CreatedBy             kernel.ID       `json:"created_by"`
	CreatedAt             time.Time       `json:"created_at"`
	UpdatedAt             time.Time       `json:"updated_at"`
}

// detailResponse adds member names (GET/POST/PATCH/PUT of one campaign).
type detailResponse struct {
	campaignResponse
	Members []memberResponse `json:"members"`
}

func optStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func present(c *domain.Campaign) campaignResponse {
	out := campaignResponse{ID: c.ID, BrandID: c.BrandID, Name: c.Name, Goal: c.Goal, Status: c.Status, NextStatuses: domain.NextStatuses(c.Status),
		Brief: c.Brief.Normalize(), StartDate: optStr(c.StartDate), EndDate: optStr(c.EndDate),
		ContentPlan: c.ContentPlan, ContentPlanApprovedBy: c.ContentPlanApprovedBy, ContentPlanApprovedAt: c.ContentPlanApprovedAt,
		PlanProposal: c.PlanProposal, PlanProposedAt: c.PlanProposedAt, MemberIDs: c.MemberIDs,
		CreatedBy: c.CreatedBy, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt}
	if out.MemberIDs == nil {
		out.MemberIDs = []kernel.ID{}
	}
	if out.NextStatuses == nil {
		out.NextStatuses = []domain.Status{}
	}
	return out
}

func presentView(v *usecase.View) detailResponse {
	out := detailResponse{campaignResponse: present(&v.Campaign), Members: make([]memberResponse, 0, len(v.Members))}
	for _, m := range v.Members {
		out.Members = append(out.Members, memberResponse(m))
	}
	return out
}

type createRequest struct {
	BrandID   kernel.ID    `json:"brand_id"`
	Name      string       `json:"name"`
	Goal      string       `json:"goal"`
	Brief     domain.Brief `json:"brief"`
	StartDate *string      `json:"start_date"`
	EndDate   *string      `json:"end_date"`
	MemberIDs []kernel.ID  `json:"member_ids"`
}

type updateRequest struct {
	Name      *string       `json:"name"`
	Goal      *string       `json:"goal"`
	Brief     *domain.Brief `json:"brief"`
	StartDate *string       `json:"start_date"`
	EndDate   *string       `json:"end_date"`
	Status    *string       `json:"status"`
	MemberIDs *[]kernel.ID  `json:"member_ids"`
}

type planRequest struct {
	Plan domain.Plan `json:"plan"`
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// ---------- handlers ----------

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	p, err := httpx.PageRequest(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	q := r.URL.Query()
	f := usecase.Filter{Status: q.Get("status"), Q: q.Get("q")}
	if s := q.Get("brand_id"); s != "" {
		id, err := kernel.ParseID("brand_id", s)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		f.BrandID = &id
	}
	page, err := h.Svc.List(r.Context(), httpx.ActorFrom(r), f, q.Get("mine") == "true", p)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := make([]campaignResponse, len(page.Items))
	for i := range page.Items {
		out[i] = present(&page.Items[i])
	}
	httpx.List(w, r, out, page.NextCursor)
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
	httpx.OK(w, r, presentView(v))
}

func (h *Handler) memberCandidates(w http.ResponseWriter, r *http.Request) {
	ms, err := h.Svc.MemberCandidates(r.Context(), httpx.ActorFrom(r))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := make([]memberResponse, 0, len(ms))
	for _, m := range ms {
		out = append(out, memberResponse(m))
	}
	httpx.List(w, r, out, nil)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	v, err := h.Svc.Create(r.Context(), httpx.ActorFrom(r), usecase.CreateInput{BrandID: req.BrandID, Name: req.Name, Goal: req.Goal,
		Brief: req.Brief, StartDate: deref(req.StartDate), EndDate: deref(req.EndDate), MemberIDs: req.MemberIDs})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.Created(w, r, presentView(v))
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var req updateRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	v, err := h.Svc.Update(r.Context(), httpx.ActorFrom(r), id, usecase.UpdateInput{Name: req.Name, Goal: req.Goal, Brief: req.Brief,
		StartDate: req.StartDate, EndDate: req.EndDate, Status: req.Status, MemberIDs: req.MemberIDs})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.OK(w, r, presentView(v))
}

func (h *Handler) savePlan(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var req planRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	v, err := h.Svc.SavePlan(r.Context(), httpx.ActorFrom(r), id, req.Plan)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.OK(w, r, presentView(v))
}

func (h *Handler) discardProposal(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := h.Svc.DiscardProposal(r.Context(), httpx.ActorFrom(r), id); err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.NoContent(w)
}
