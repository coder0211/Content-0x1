// Package http serves GET /dashboard/summary (api/openapi/dashboard.yaml).
package http

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/be/acs/api/internal/modules/dashboard/usecase"
	"github.com/be/acs/api/internal/platform/httpx"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Handler serves the dashboard.
type Handler struct{ Svc *usecase.Service }

// Routes registers the route (any authenticated user; widgets are filtered
// by permission in the use case).
func (h *Handler) Routes(r chi.Router) {
	r.With(httpx.RequireAuth).Get("/dashboard/summary", h.summary)
}

type campaignDTO struct {
	ID        kernel.ID `json:"id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	UpdatedAt time.Time `json:"updated_at"`
}

type campaignsDTO struct {
	ActiveCount int           `json:"active_count"`
	Recent      []campaignDTO `json:"recent"`
}

type jobDTO struct {
	ID        kernel.ID `json:"id"`
	JobType   string    `json:"job_type"`
	Status    string    `json:"status"`
	Progress  int       `json:"progress"`
	ErrorCode *string   `json:"error_code"`
	CreatedAt time.Time `json:"created_at"`
}

type jobsDTO struct {
	Scope     string   `json:"scope"`
	Queued    int      `json:"queued"`
	Running   int      `json:"running"`
	Failed24h int      `json:"failed_24h"`
	Recent    []jobDTO `json:"recent"`
}

type reviewsDTO struct {
	PendingReview      *int `json:"pending_review"`
	MyPendingReview    int  `json:"my_pending_review"`
	MyChangesRequested int  `json:"my_changes_requested"`
	MyDrafts           int  `json:"my_drafts"`
}

type costDTO struct {
	Currency     string   `json:"currency"`
	Today        float64  `json:"today"`
	Month        float64  `json:"month"`
	MonthlyLimit *float64 `json:"monthly_limit"`
	MonthUsedPct *float64 `json:"month_used_pct"`
}

type summaryDTO struct {
	Campaigns   *campaignsDTO `json:"campaigns"`
	Jobs        jobsDTO       `json:"jobs"`
	Reviews     reviewsDTO    `json:"reviews"`
	Cost        *costDTO      `json:"cost"`
	GeneratedAt time.Time     `json:"generated_at"`
}

func present(s usecase.Summary) summaryDTO {
	out := summaryDTO{
		Jobs: jobsDTO{Scope: s.Jobs.Scope, Queued: s.Jobs.Queued, Running: s.Jobs.Running, Failed24h: s.Jobs.Failed24h,
			Recent: make([]jobDTO, 0, len(s.Jobs.Recent))},
		Reviews: reviewsDTO{PendingReview: s.Reviews.PendingReview, MyPendingReview: s.Reviews.MyPendingReview,
			MyChangesRequested: s.Reviews.MyChangesRequested, MyDrafts: s.Reviews.MyDrafts},
		GeneratedAt: s.GeneratedAt,
	}
	for _, j := range s.Jobs.Recent {
		out.Jobs.Recent = append(out.Jobs.Recent, jobDTO{ID: j.ID, JobType: j.JobType, Status: j.Status, Progress: j.Progress,
			ErrorCode: j.ErrorCode, CreatedAt: j.CreatedAt})
	}
	if c := s.Campaigns; c != nil {
		out.Campaigns = &campaignsDTO{ActiveCount: c.ActiveCount, Recent: make([]campaignDTO, 0, len(c.Recent))}
		for _, x := range c.Recent {
			out.Campaigns.Recent = append(out.Campaigns.Recent, campaignDTO{ID: x.ID, Name: x.Name, Status: x.Status, UpdatedAt: x.UpdatedAt})
		}
	}
	if c := s.Cost; c != nil {
		today, _ := c.Today.Round(6).Float64()
		month, _ := c.Month.Round(6).Float64()
		out.Cost = &costDTO{Currency: c.Currency, Today: today, Month: month, MonthUsedPct: c.MonthUsedPct}
		if c.MonthlyLimit != nil {
			l, _ := c.MonthlyLimit.Float64()
			out.Cost.MonthlyLimit = &l
		}
	}
	return out
}

func (h *Handler) summary(w http.ResponseWriter, r *http.Request) {
	s, err := h.Svc.Summary(r.Context(), httpx.ActorFrom(r))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.OK(w, r, present(s))
}
