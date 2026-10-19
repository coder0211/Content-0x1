// Package http maps HTTP ⇄ use cases: decode DTO, call use case, present.
package http

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/be/acs/api/internal/modules/_template/domain"
	"github.com/be/acs/api/internal/modules/_template/usecase"
	"github.com/be/acs/api/internal/platform/httpx"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Handler serves /notes.
type Handler struct{ Notes *usecase.Notes }

// Routes registers routes with permission middleware; ownership is checked
// in the use case.
func (h *Handler) Routes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(httpx.RequirePermission(kernel.PermCampaignManage))
		r.Post("/notes", h.create)
		r.Post("/notes/{id}/publish", h.publish)
	})
}

type createNoteRequest struct {
	Title string `json:"title"`
}

type noteResponse struct {
	ID        kernel.ID `json:"id"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

func present(n *domain.Note) noteResponse {
	return noteResponse{ID: n.ID, Title: n.Title, Status: string(n.Status), CreatedAt: n.CreatedAt}
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req createNoteRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	n, err := h.Notes.Create(r.Context(), httpx.ActorFrom(r), req.Title)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.Created(w, r, present(n))
}

func (h *Handler) publish(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	n, err := h.Notes.Publish(r.Context(), httpx.ActorFrom(r), id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.OK(w, r, present(n))
}
