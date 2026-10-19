// Package http serves the asset library, versions, review and download
// endpoints (api/openapi/asset.yaml).
package http

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/be/acs/api/internal/modules/asset/domain"
	"github.com/be/acs/api/internal/modules/asset/usecase"
	"github.com/be/acs/api/internal/platform/httpx"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Handler serves /assets and /reviews.
type Handler struct{ Svc *usecase.Service }

// Routes registers the routes. brand.view gates everything; ownership,
// review and download rules are enforced in the use cases.
func (h *Handler) Routes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(httpx.RequirePermission(kernel.PermBrandView))
		r.Get("/assets", h.list)
		r.Get("/assets/{id}", h.get)
		r.Patch("/assets/{id}", h.update)
		r.Delete("/assets/{id}", h.delete)
		r.Get("/assets/{id}/versions", h.versions)
		r.Post("/assets/{id}/versions", h.createVersion)
		r.Post("/assets/{id}/versions/{versionNo}/restore", h.restore)
		r.Post("/assets/{id}/submit-review", h.submit)
		r.With(httpx.RequirePermission(kernel.PermAssetDownload)).Post("/assets/{id}/download-url", h.download)
		r.Group(func(r chi.Router) {
			r.Use(httpx.RequirePermission(kernel.PermAssetReview))
			r.Post("/assets/{id}/approve", h.approve)
			r.Post("/assets/{id}/request-changes", h.requestChanges)
			r.Get("/reviews/queue", h.queue)
		})
	})
}

// decodeOptional decodes a JSON body when one is present.
func decodeOptional(r *http.Request, v any) error {
	b, err := io.ReadAll(io.LimitReader(r.Body, httpx.MaxJSONBody+1))
	if err != nil {
		return kernel.Validation(kernel.FieldIssue("body", "invalid"))
	}
	if len(bytes.TrimSpace(b)) == 0 {
		return nil
	}
	r.Body = io.NopCloser(bytes.NewReader(b))
	return httpx.Decode(r, v)
}

// parseTime accepts RFC3339 or YYYY-MM-DD (UTC). endOfDay moves a date to
// the next day so `to=2026-10-31` includes the whole day.
func parseTime(name, s string, endOfDay bool) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return &t, nil
	}
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return nil, kernel.Validation(kernel.FieldIssue(name, "invalid_date"))
	}
	if endOfDay {
		t = t.AddDate(0, 0, 1)
	}
	return &t, nil
}

func parseFilter(r *http.Request) (usecase.SearchFilter, error) {
	q := r.URL.Query()
	var f usecase.SearchFilter
	var v kernel.ValidationErrors
	if s := q.Get("type"); s != "" {
		t := domain.Type(s)
		if !t.Valid() {
			v.Add("type", "invalid")
		}
		f.Type = &t
	}
	if s := q.Get("status"); s != "" {
		for _, p := range strings.Split(s, ",") {
			st := domain.Status(strings.TrimSpace(p))
			if !st.Valid() {
				v.Add("status", "invalid")
				break
			}
			f.Statuses = append(f.Statuses, st)
		}
	}
	var err error
	for name, dst := range map[string]**kernel.ID{"campaign_id": &f.CampaignID, "brand_id": &f.BrandID, "created_by": &f.CreatedBy} {
		if *dst, err = httpx.QueryID(r, name); err != nil {
			v.Add(name, "invalid")
		}
	}
	f.Tag = strings.ToLower(strings.TrimSpace(q.Get("tag")))
	if f.From, err = parseTime("from", q.Get("from"), false); err != nil {
		v.Add("from", "invalid_date")
	}
	if f.To, err = parseTime("to", q.Get("to"), true); err != nil {
		v.Add("to", "invalid_date")
	}
	return f, v.Err()
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	page, err := httpx.PageRequest(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	f, err := parseFilter(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	res, err := h.Svc.Search(r.Context(), httpx.ActorFrom(r), usecase.SearchInput{Q: r.URL.Query().Get("q"), Filter: f, PageCursor: page})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.List(w, r, presentSummaries(res.Items), res.NextCursor)
}

func (h *Handler) queue(w http.ResponseWriter, r *http.Request) {
	page, err := httpx.PageRequest(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	f, err := parseFilter(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	res, err := h.Svc.Queue(r.Context(), httpx.ActorFrom(r), f, page)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.List(w, r, presentSummaries(res.Items), res.NextCursor)
}

// withID runs fn with the parsed {id}.
func withID(w http.ResponseWriter, r *http.Request, fn func(id kernel.ID) (any, int, error)) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	data, status, err := fn(id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if status == http.StatusNoContent {
		httpx.NoContent(w)
		return
	}
	httpx.JSON(w, r, status, data)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	withID(w, r, func(id kernel.ID) (any, int, error) {
		d, err := h.Svc.Detail(r.Context(), httpx.ActorFrom(r), id)
		return presentDetail(d), http.StatusOK, err
	})
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	withID(w, r, func(id kernel.ID) (any, int, error) {
		var req updateRequest
		if err := httpx.Decode(r, &req); err != nil {
			return nil, 0, err
		}
		in := usecase.UpdateInput{Title: req.Title, Tags: req.Tags}
		if req.UsageRights != nil {
			in.UsageRightsSet = true
			if string(req.UsageRights) != "null" {
				var s string
				if json.Unmarshal(req.UsageRights, &s) != nil {
					return nil, 0, kernel.Validation(kernel.FieldIssue("usage_rights", "invalid_type"))
				}
				in.UsageRights = &s
			}
		}
		d, err := h.Svc.Update(r.Context(), httpx.ActorFrom(r), id, in)
		return presentDetail(d), http.StatusOK, err
	})
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	withID(w, r, func(id kernel.ID) (any, int, error) {
		return nil, http.StatusNoContent, h.Svc.Delete(r.Context(), httpx.ActorFrom(r), id)
	})
}

func presentVersions(vs []usecase.VersionDetail) []versionDTO {
	out := make([]versionDTO, 0, len(vs))
	for _, v := range vs {
		out = append(out, presentVersion(v))
	}
	return out
}

func (h *Handler) versions(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	vs, err := h.Svc.ListVersions(r.Context(), httpx.ActorFrom(r), id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.List(w, r, presentVersions(vs), nil)
}

func (h *Handler) createVersion(w http.ResponseWriter, r *http.Request) {
	withID(w, r, func(id kernel.ID) (any, int, error) {
		var req versionCreateRequest
		if err := httpx.Decode(r, &req); err != nil {
			return nil, 0, err
		}
		v, err := h.Svc.CreateVersion(r.Context(), httpx.ActorFrom(r), id, usecase.ManualVersionInput{
			ContentJSON: req.ContentJSON, ChangeNote: req.ChangeNote, BaseVersionNo: req.BaseVersionNo,
		})
		return presentVersion(v), http.StatusCreated, err
	})
}

func (h *Handler) restore(w http.ResponseWriter, r *http.Request) {
	withID(w, r, func(id kernel.ID) (any, int, error) {
		no, err := strconv.Atoi(chi.URLParam(r, "versionNo"))
		if err != nil || no < 1 {
			return nil, 0, kernel.ErrNotFound
		}
		v, err := h.Svc.Restore(r.Context(), httpx.ActorFrom(r), id, no)
		return presentVersion(v), http.StatusCreated, err
	})
}

func (h *Handler) submit(w http.ResponseWriter, r *http.Request) {
	withID(w, r, func(id kernel.ID) (any, int, error) {
		var req submitRequest
		if err := decodeOptional(r, &req); err != nil {
			return nil, 0, err
		}
		d, err := h.Svc.SubmitReview(r.Context(), httpx.ActorFrom(r), id, usecase.SubmitInput{
			Comment: req.Comment, Checklist: domain.Checklist{TypographyChecked: req.Checklist.TypographyChecked},
		})
		return presentDetail(d), http.StatusOK, err
	})
}

func (h *Handler) approve(w http.ResponseWriter, r *http.Request) {
	withID(w, r, func(id kernel.ID) (any, int, error) {
		var req decisionRequest
		if err := decodeOptional(r, &req); err != nil {
			return nil, 0, err
		}
		d, err := h.Svc.Approve(r.Context(), httpx.ActorFrom(r), id, req.Comment)
		return presentDetail(d), http.StatusOK, err
	})
}

func (h *Handler) requestChanges(w http.ResponseWriter, r *http.Request) {
	withID(w, r, func(id kernel.ID) (any, int, error) {
		var req decisionRequest
		if err := decodeOptional(r, &req); err != nil {
			return nil, 0, err
		}
		d, err := h.Svc.RequestChanges(r.Context(), httpx.ActorFrom(r), id, req.Comment)
		return presentDetail(d), http.StatusOK, err
	})
}

func (h *Handler) download(w http.ResponseWriter, r *http.Request) {
	withID(w, r, func(id kernel.ID) (any, int, error) {
		var req downloadRequest
		if err := decodeOptional(r, &req); err != nil {
			return nil, 0, err
		}
		u, err := h.Svc.Download(r.Context(), httpx.ActorFrom(r), id, req.FileID)
		return downloadDTO{URL: u.URL, ExpiresAt: u.ExpiresAt, FileID: u.FileID}, http.StatusOK, err
	})
}
