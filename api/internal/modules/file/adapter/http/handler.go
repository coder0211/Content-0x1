// Package http is the file HTTP adapter: upload URL, complete, metadata.
package http

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/be/acs/api/internal/modules/file/domain"
	"github.com/be/acs/api/internal/modules/file/usecase"
	"github.com/be/acs/api/internal/platform/httpx"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Handler serves /files.
type Handler struct{ Svc *usecase.Service }

// Routes registers routes (all require authentication).
func (h *Handler) Routes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(httpx.RequireAuth)
		r.Post("/files/upload-url", h.uploadURL)
		r.Post("/files/{id}/complete", h.complete)
		r.Get("/files/{id}", h.get)
	})
}

type uploadURLRequest struct {
	Filename  string `json:"filename"`
	MimeType  string `json:"mime_type"`
	SizeBytes int64  `json:"size_bytes"`
	Purpose   string `json:"purpose"`
}

type uploadURLResponse struct {
	FileID    kernel.ID         `json:"file_id"`
	URL       string            `json:"url"`
	Method    string            `json:"method"`
	Headers   map[string]string `json:"headers"`
	ExpiresAt time.Time         `json:"expires_at"`
}

// FileResponse is the JSON shape of a file (reusable by other modules'
// presenters via their own DTOs; kept here for /files endpoints).
type FileResponse struct {
	ID           kernel.ID  `json:"id"`
	OriginalName string     `json:"original_name"`
	MimeType     string     `json:"mime_type"`
	SizeBytes    int64      `json:"size_bytes"`
	Kind         string     `json:"kind"`
	UploadStatus string     `json:"upload_status"`
	Width        *int       `json:"width"`
	Height       *int       `json:"height"`
	DurationMs   *int       `json:"duration_ms"`
	CreatedAt    time.Time  `json:"created_at"`
	URL          *string    `json:"url"`
	URLExpiresAt *time.Time `json:"url_expires_at"`
}

func nz(n int) *int {
	if n == 0 {
		return nil
	}
	return &n
}

func toResponse(f domain.File, u *usecase.SignedURL) FileResponse {
	r := FileResponse{ID: f.ID, OriginalName: f.OriginalName, MimeType: f.MimeType, SizeBytes: f.SizeBytes,
		Kind: string(f.Kind), UploadStatus: string(f.UploadStatus), Width: nz(f.Width), Height: nz(f.Height),
		DurationMs: nz(f.DurationMs), CreatedAt: f.CreatedAt}
	if u != nil && u.URL != "" {
		r.URL, r.URLExpiresAt = &u.URL, &u.ExpiresAt
	}
	return r
}

func (h *Handler) uploadURL(w http.ResponseWriter, r *http.Request) {
	var req uploadURLRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	tk, err := h.Svc.RequestUpload(r.Context(), httpx.ActorFrom(r), usecase.RequestUploadInput{
		Filename: req.Filename, MimeType: req.MimeType, SizeBytes: req.SizeBytes, Purpose: domain.Purpose(req.Purpose),
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.Created(w, r, uploadURLResponse{FileID: tk.FileID, URL: tk.URL, Method: tk.Method, Headers: tk.Headers, ExpiresAt: tk.ExpiresAt})
}

func (h *Handler) complete(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	f, err := h.Svc.CompleteUpload(r.Context(), httpx.ActorFrom(r), id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.OK(w, r, toResponse(f, nil))
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	f, u, err := h.Svc.GetForActor(r.Context(), httpx.ActorFrom(r), id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.OK(w, r, toResponse(f, &u))
}
