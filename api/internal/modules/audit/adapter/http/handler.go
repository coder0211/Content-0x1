// Package http is the audit HTTP adapter: /audit-logs and /notifications.
package http

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/be/acs/api/internal/modules/audit/usecase"
	"github.com/be/acs/api/internal/platform/httpx"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Handler serves audit endpoints.
type Handler struct{ Svc *usecase.Service }

// Routes registers routes.
func (h *Handler) Routes(r chi.Router) {
	r.With(httpx.RequirePermission(kernel.PermAuditView)).Get("/audit-logs", h.listLogs)
	r.Group(func(r chi.Router) {
		r.Use(httpx.RequireAuth)
		r.Get("/notifications", h.listNotifications)
		r.Post("/notifications/{id}/read", h.markRead)
	})
}

type logResponse struct {
	ID           int64           `json:"id"`
	ActorID      *kernel.ID      `json:"actor_id"`
	ActorEmail   string          `json:"actor_email"`
	Action       string          `json:"action"`
	ResourceType string          `json:"resource_type"`
	ResourceID   string          `json:"resource_id"`
	Before       json.RawMessage `json:"before"`
	After        json.RawMessage `json:"after"`
	IP           string          `json:"ip"`
	RequestID    string          `json:"request_id"`
	CreatedAt    time.Time       `json:"created_at"`
}

type notificationResponse struct {
	ID        kernel.ID       `json:"id"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
	ReadAt    *time.Time      `json:"read_at"`
	CreatedAt time.Time       `json:"created_at"`
}

func parseTime(r *http.Request, name string) (*time.Time, error) {
	s := r.URL.Query().Get(name)
	if s == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		if t, err = time.Parse(time.DateOnly, s); err != nil {
			return nil, kernel.Validation(kernel.FieldIssue(name, "invalid_datetime"))
		}
	}
	return &t, nil
}

func (h *Handler) listLogs(w http.ResponseWriter, r *http.Request) {
	page, err := httpx.PageRequest(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	q := r.URL.Query()
	f := usecase.LogFilter{ResourceType: q.Get("resource_type"), ResourceID: q.Get("resource_id"), Action: q.Get("action")}
	if f.ActorID, err = httpx.QueryID(r, "actor_id"); err != nil {
		httpx.Error(w, r, err)
		return
	}
	if f.From, err = parseTime(r, "from"); err != nil {
		httpx.Error(w, r, err)
		return
	}
	if f.To, err = parseTime(r, "to"); err != nil {
		httpx.Error(w, r, err)
		return
	}
	res, err := h.Svc.ListLogs(r.Context(), httpx.ActorFrom(r), f, page)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	items := make([]logResponse, 0, len(res.Items))
	for _, l := range res.Items {
		items = append(items, logResponse(l))
	}
	httpx.List(w, r, items, res.NextCursor)
}

func (h *Handler) listNotifications(w http.ResponseWriter, r *http.Request) {
	page, err := httpx.PageRequest(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	res, _, err := h.Svc.ListNotifications(r.Context(), httpx.ActorFrom(r), r.URL.Query().Get("unread") == "true", page)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	items := make([]notificationResponse, 0, len(res.Items))
	for _, n := range res.Items {
		items = append(items, notificationResponse(n))
	}
	httpx.List(w, r, items, res.NextCursor)
}

func (h *Handler) markRead(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := h.Svc.MarkRead(r.Context(), httpx.ActorFrom(r), id); err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.NoContent(w)
}
