// Package http serves /brands and /products (SPEC-04 §2.2).
package http

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/be/acs/api/internal/modules/brand/domain"
	"github.com/be/acs/api/internal/modules/brand/usecase"
	"github.com/be/acs/api/internal/platform/httpx"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Handler serves brand, product and brand-asset routes.
type Handler struct{ Svc *usecase.Service }

// Routes registers routes. Fine-grained checks happen in the use cases.
func (h *Handler) Routes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(httpx.RequirePermission(kernel.PermBrandView))
		r.Get("/brands", h.listBrands)
		r.Get("/brands/{id}", h.getBrand)
		r.Get("/brands/{id}/versions", h.listVersions)
		r.Get("/brands/{id}/assets", h.listAssets)
		r.Get("/brands/{id}/products", h.listProducts)
		r.Get("/products/{id}", h.getProduct)
	})
	r.Group(func(r chi.Router) {
		r.Use(httpx.RequirePermission(kernel.PermBrandManage))
		r.Post("/brands", h.createBrand)
		r.Patch("/brands/{id}", h.updateBrand)
		r.Delete("/brands/{id}", h.deleteBrand)
		r.Post("/brands/{id}/assets", h.addAsset)
		r.Delete("/brands/{id}/assets/{brandAssetId}", h.deleteAsset)
		r.Post("/brands/{id}/products", h.createProduct)
		r.Patch("/products/{id}", h.updateProduct)
	})
}

// ---------- DTOs ----------

type versionResponse struct {
	ID        kernel.ID      `json:"id"`
	VersionNo int            `json:"version_no"`
	Profile   domain.Profile `json:"profile"`
	CreatedBy *kernel.ID     `json:"created_by"`
	CreatedAt time.Time      `json:"created_at"`
}

type brandResponse struct {
	ID             kernel.ID       `json:"id"`
	Name           string          `json:"name"`
	CurrentVersion versionResponse `json:"current_version"`
	CreatedBy      *kernel.ID      `json:"created_by"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

func presentVersion(v domain.Version) versionResponse {
	return versionResponse{ID: v.ID, VersionNo: v.VersionNo, Profile: v.Profile.Normalize(), CreatedBy: v.CreatedBy, CreatedAt: v.CreatedAt}
}

func presentBrand(b *domain.Brand) brandResponse {
	return brandResponse{ID: b.ID, Name: b.Name, CurrentVersion: presentVersion(b.Current), CreatedBy: b.CreatedBy, CreatedAt: b.CreatedAt, UpdatedAt: b.UpdatedAt}
}

type createBrandRequest struct {
	Name    string          `json:"name"`
	Profile *domain.Profile `json:"profile"`
}

type updateBrandRequest struct {
	Name              *string         `json:"name"`
	Profile           *domain.Profile `json:"profile"`
	ExpectedVersionNo int             `json:"expected_version_no"`
}

type offerResponse struct {
	Text      string  `json:"text"`
	ValidFrom *string `json:"valid_from"`
	ValidTo   *string `json:"valid_to"`
	Status    string  `json:"status"` // active | expired | upcoming
}

type factsResponse struct {
	USP      []string        `json:"usp"`
	Price    *domain.Price   `json:"price"`
	Offers   []offerResponse `json:"offers"`
	Policies []string        `json:"policies"`
	Proof    []domain.Proof  `json:"proof"`
}

type productResponse struct {
	ID          kernel.ID     `json:"id"`
	BrandID     kernel.ID     `json:"brand_id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Facts       factsResponse `json:"facts"`
	Status      string        `json:"status"`
	CreatedBy   *kernel.ID    `json:"created_by"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
}

func optStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func presentProduct(p *domain.Product, now time.Time) productResponse {
	f := p.Facts.Normalize()
	offers := make([]offerResponse, 0, len(f.Offers))
	for _, o := range f.Offers {
		offers = append(offers, offerResponse{Text: o.Text, ValidFrom: optStr(o.ValidFrom), ValidTo: optStr(o.ValidTo), Status: o.StatusAt(now)})
	}
	return productResponse{ID: p.ID, BrandID: p.BrandID, Name: p.Name, Description: p.Description, Status: p.Status,
		Facts:     factsResponse{USP: f.USP, Price: f.Price, Offers: offers, Policies: f.Policies, Proof: f.Proof},
		CreatedBy: p.CreatedBy, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt}
}

type productRequest struct {
	Name        *string       `json:"name"`
	Description *string       `json:"description"`
	Facts       *domain.Facts `json:"facts"`
	Status      *string       `json:"status"`
}

func (r productRequest) input() usecase.ProductInput {
	return usecase.ProductInput{Name: r.Name, Description: r.Description, Facts: r.Facts, Status: r.Status}
}

type fileResponse struct {
	MimeType     string     `json:"mime_type"`
	OriginalName string     `json:"original_name"`
	SizeBytes    int64      `json:"size_bytes"`
	Width        int        `json:"width"`
	Height       int        `json:"height"`
	DurationMs   int        `json:"duration_ms"`
	URL          *string    `json:"url"`
	URLExpiresAt *time.Time `json:"url_expires_at"`
}

type assetResponse struct {
	ID           kernel.ID       `json:"id"`
	BrandID      kernel.ID       `json:"brand_id"`
	FileID       kernel.ID       `json:"file_id"`
	AssetType    string          `json:"asset_type"`
	Label        string          `json:"label"`
	License      json.RawMessage `json:"license"`
	LicenseValid bool            `json:"license_valid"`
	File         *fileResponse   `json:"file"`
	CreatedBy    *kernel.ID      `json:"created_by"`
	CreatedAt    time.Time       `json:"created_at"`
}

func presentAsset(a *domain.Asset, v *usecase.AssetView, now time.Time) assetResponse {
	lic := a.License
	if len(lic) == 0 {
		lic = json.RawMessage("null")
	}
	out := assetResponse{ID: a.ID, BrandID: a.BrandID, FileID: a.FileID, AssetType: a.AssetType, Label: a.Label, License: lic,
		LicenseValid: a.LicenseUsableAt(now), CreatedBy: a.CreatedBy, CreatedAt: a.CreatedAt}
	if v != nil && v.File != nil {
		f := v.File
		out.File = &fileResponse{MimeType: f.MimeType, OriginalName: f.OriginalName, SizeBytes: f.SizeBytes, Width: f.Width, Height: f.Height, DurationMs: f.DurationMs}
		if v.URL != nil {
			out.File.URL, out.File.URLExpiresAt = &v.URL.URL, &v.URL.ExpiresAt
		}
	}
	return out
}

type addAssetRequest struct {
	FileID    kernel.ID       `json:"file_id"`
	AssetType string          `json:"asset_type"`
	Label     string          `json:"label"`
	License   *domain.License `json:"license"`
}

// ---------- brands ----------

func (h *Handler) listBrands(w http.ResponseWriter, r *http.Request) {
	p, err := httpx.PageRequest(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	page, err := h.Svc.ListBrands(r.Context(), httpx.ActorFrom(r), usecase.BrandFilter{Q: r.URL.Query().Get("q")}, p)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := make([]brandResponse, len(page.Items))
	for i := range page.Items {
		out[i] = presentBrand(&page.Items[i])
	}
	httpx.List(w, r, out, page.NextCursor)
}

func (h *Handler) getBrand(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	b, err := h.Svc.GetBrand(r.Context(), httpx.ActorFrom(r), id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.OK(w, r, presentBrand(b))
}

func (h *Handler) createBrand(w http.ResponseWriter, r *http.Request) {
	var req createBrandRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	in := usecase.CreateBrandInput{Name: req.Name}
	if req.Profile != nil {
		in.Profile = *req.Profile
	}
	b, err := h.Svc.CreateBrand(r.Context(), httpx.ActorFrom(r), in)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.Created(w, r, presentBrand(b))
}

func (h *Handler) updateBrand(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var req updateBrandRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	b, err := h.Svc.UpdateBrand(r.Context(), httpx.ActorFrom(r), id, usecase.UpdateBrandInput{Name: req.Name, Profile: req.Profile, ExpectedVersionNo: req.ExpectedVersionNo})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.OK(w, r, presentBrand(b))
}

func (h *Handler) deleteBrand(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := h.Svc.DeleteBrand(r.Context(), httpx.ActorFrom(r), id); err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (h *Handler) listVersions(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	p, err := httpx.PageRequest(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	page, err := h.Svc.ListVersions(r.Context(), httpx.ActorFrom(r), id, p)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := make([]versionResponse, len(page.Items))
	for i, v := range page.Items {
		out[i] = presentVersion(v)
	}
	httpx.List(w, r, out, page.NextCursor)
}

// ---------- products ----------

func (h *Handler) listProducts(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	p, err := httpx.PageRequest(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	page, err := h.Svc.ListProducts(r.Context(), httpx.ActorFrom(r), id, usecase.ProductFilter{Status: r.URL.Query().Get("status")}, p)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	now := h.Svc.Clock.Now()
	out := make([]productResponse, len(page.Items))
	for i := range page.Items {
		out[i] = presentProduct(&page.Items[i], now)
	}
	httpx.List(w, r, out, page.NextCursor)
}

func (h *Handler) getProduct(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	p, err := h.Svc.GetProduct(r.Context(), httpx.ActorFrom(r), id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.OK(w, r, presentProduct(p, h.Svc.Clock.Now()))
}

func (h *Handler) createProduct(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var req productRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	p, err := h.Svc.CreateProduct(r.Context(), httpx.ActorFrom(r), id, req.input())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.Created(w, r, presentProduct(p, h.Svc.Clock.Now()))
}

func (h *Handler) updateProduct(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var req productRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	p, err := h.Svc.UpdateProduct(r.Context(), httpx.ActorFrom(r), id, req.input())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.OK(w, r, presentProduct(p, h.Svc.Clock.Now()))
}

// ---------- assets ----------

func (h *Handler) listAssets(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var types []string
	if t := r.URL.Query().Get("asset_type"); t != "" {
		types = strings.Split(t, ",")
	}
	views, err := h.Svc.ListAssets(r.Context(), httpx.ActorFrom(r), id, types)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	now := h.Svc.Clock.Now()
	out := make([]assetResponse, len(views))
	for i := range views {
		out[i] = presentAsset(&views[i].Asset, &views[i], now)
	}
	httpx.List(w, r, out, nil)
}

func (h *Handler) addAsset(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var req addAssetRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	a, err := h.Svc.AddAsset(r.Context(), httpx.ActorFrom(r), id, usecase.AddAssetInput{FileID: req.FileID, AssetType: req.AssetType, Label: req.Label, License: req.License})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.Created(w, r, presentAsset(a, nil, h.Svc.Clock.Now()))
}

func (h *Handler) deleteAsset(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	aid, err := httpx.PathID(r, "brandAssetId")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := h.Svc.DeleteAsset(r.Context(), httpx.ActorFrom(r), id, aid); err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.NoContent(w)
}
