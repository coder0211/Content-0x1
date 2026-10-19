package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/be/acs/api/internal/modules/brand/domain"
	fileuc "github.com/be/acs/api/internal/modules/file/usecase"
	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Service implements the brand use cases and the public API.
type Service struct {
	Tx    ports.TxManager
	Repo  Repository
	Clock ports.Clock
	Audit ports.AuditRecorder
	Files FileReader
}

var _ API = (*Service)(nil)

// Audit actions and resource types.
const (
	resBrand   = "brand"
	resProduct = "product"
	resAsset   = "brand_asset"
)

// ---------- brands ----------

// ListBrands lists non-deleted brands (brand.view).
func (s *Service) ListBrands(ctx context.Context, actor kernel.Actor, f BrandFilter, p kernel.PageRequest) (kernel.Page[domain.Brand], error) {
	if err := actor.Require(kernel.PermBrandView); err != nil {
		return kernel.Page[domain.Brand]{}, err
	}
	f.Q = strings.TrimSpace(f.Q)
	return s.Repo.ListBrands(ctx, f, p.Normalized())
}

// GetBrand returns a brand with its current version (brand.view).
func (s *Service) GetBrand(ctx context.Context, actor kernel.Actor, id kernel.ID) (*domain.Brand, error) {
	if err := actor.Require(kernel.PermBrandView); err != nil {
		return nil, err
	}
	return s.Repo.GetBrand(ctx, id)
}

// CreateBrandInput is POST /brands.
type CreateBrandInput struct {
	Name    string
	Profile domain.Profile
}

// CreateBrand creates a brand and its version 1 (brand.manage).
func (s *Service) CreateBrand(ctx context.Context, actor kernel.Actor, in CreateBrandInput) (*domain.Brand, error) {
	if err := actor.Require(kernel.PermBrandManage); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(in.Name)
	profile := in.Profile.Normalize()
	var v kernel.ValidationErrors
	domain.ValidateName(&v, "name", name)
	profile.Validate(&v, "profile")
	if err := v.Err(); err != nil {
		return nil, err
	}
	now := s.Clock.Now()
	by := actor.UserID
	b := &domain.Brand{ID: kernel.NewID(), Name: name, CreatedBy: &by, CreatedAt: now, UpdatedAt: now}
	b.Current = domain.Version{ID: kernel.NewID(), BrandID: b.ID, VersionNo: 1, Profile: profile, CreatedBy: &by, CreatedAt: now}
	err := s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.Repo.CreateBrand(ctx, b); err != nil {
			return dupName(err)
		}
		return s.Audit.Record(ctx, "brand.created", resBrand, b.ID.String(), nil, auditBrand(b))
	})
	if err != nil {
		return nil, err
	}
	return b, nil
}

// UpdateBrandInput is PATCH /brands/{id}.
type UpdateBrandInput struct {
	Name              *string
	Profile           *domain.Profile
	ExpectedVersionNo int
}

// UpdateBrand renames a brand and/or saves a new profile version, guarded
// by an optimistic lock on the current version number (ACS-020).
func (s *Service) UpdateBrand(ctx context.Context, actor kernel.Actor, id kernel.ID, in UpdateBrandInput) (*domain.Brand, error) {
	if err := actor.Require(kernel.PermBrandManage); err != nil {
		return nil, err
	}
	var v kernel.ValidationErrors
	if in.ExpectedVersionNo <= 0 {
		v.Add("expected_version_no", "required")
	}
	var name string
	if in.Name != nil {
		name = strings.TrimSpace(*in.Name)
		domain.ValidateName(&v, "name", name)
	}
	var profile domain.Profile
	if in.Profile != nil {
		profile = in.Profile.Normalize()
		profile.Validate(&v, "profile")
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	var out *domain.Brand
	err := s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		b, err := s.Repo.GetBrandForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if err := b.CheckVersion(in.ExpectedVersionNo); err != nil {
			return err
		}
		before := auditBrand(b)
		changed := false
		if in.Name != nil && name != b.Name {
			if err := s.Repo.RenameBrand(ctx, b.ID, name); err != nil {
				return dupName(err)
			}
			b.Name = name
			changed = true
		}
		if in.Profile != nil && !profile.Equal(b.Current.Profile) {
			nv := b.NextVersion(profile, actor.UserID, s.Clock.Now())
			if err := s.Repo.AddVersion(ctx, nv); err != nil {
				return err
			}
			b.Current = nv
			changed = true
		}
		out = b
		if !changed {
			return nil
		}
		b.UpdatedAt = s.Clock.Now()
		return s.Audit.Record(ctx, "brand.updated", resBrand, b.ID.String(), before, auditBrand(b))
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// DeleteBrand soft-deletes a brand (brand.manage).
func (s *Service) DeleteBrand(ctx context.Context, actor kernel.Actor, id kernel.ID) error {
	if err := actor.Require(kernel.PermBrandManage); err != nil {
		return err
	}
	return s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		b, err := s.Repo.GetBrandForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if err := s.Repo.SoftDeleteBrand(ctx, id); err != nil {
			return err
		}
		return s.Audit.Record(ctx, "brand.deleted", resBrand, id.String(), auditBrand(b), nil)
	})
}

// ListVersions lists a brand's versions, newest first (brand.view).
func (s *Service) ListVersions(ctx context.Context, actor kernel.Actor, brandID kernel.ID, p kernel.PageRequest) (kernel.Page[domain.Version], error) {
	if err := actor.Require(kernel.PermBrandView); err != nil {
		return kernel.Page[domain.Version]{}, err
	}
	if _, err := s.Repo.GetBrand(ctx, brandID); err != nil {
		return kernel.Page[domain.Version]{}, err
	}
	return s.Repo.ListVersions(ctx, brandID, p.Normalized())
}

func dupName(err error) error {
	if errors.Is(err, kernel.ErrConflict) {
		return kernel.ErrConflict.WithMessage("Tên brand đã tồn tại").WithDetails(kernel.FieldIssue("name", "duplicate"))
	}
	return err
}

func auditBrand(b *domain.Brand) map[string]any {
	return map[string]any{"name": b.Name, "version_no": b.Current.VersionNo, "version_id": b.Current.ID}
}

// ---------- products ----------

// ListProducts lists products of a brand (brand.view).
func (s *Service) ListProducts(ctx context.Context, actor kernel.Actor, brandID kernel.ID, f ProductFilter, p kernel.PageRequest) (kernel.Page[domain.Product], error) {
	if err := actor.Require(kernel.PermBrandView); err != nil {
		return kernel.Page[domain.Product]{}, err
	}
	if f.Status != "" && f.Status != domain.ProductActive && f.Status != domain.ProductArchived {
		return kernel.Page[domain.Product]{}, kernel.Validation(kernel.FieldIssue("status", "invalid"))
	}
	if _, err := s.Repo.GetBrand(ctx, brandID); err != nil {
		return kernel.Page[domain.Product]{}, err
	}
	return s.Repo.ListProducts(ctx, brandID, f, p.Normalized())
}

// GetProduct returns one product (brand.view).
func (s *Service) GetProduct(ctx context.Context, actor kernel.Actor, id kernel.ID) (*domain.Product, error) {
	if err := actor.Require(kernel.PermBrandView); err != nil {
		return nil, err
	}
	return s.Repo.GetProduct(ctx, id)
}

// ProductInput is POST /brands/{id}/products and PATCH /products/{id}
// (nil = unchanged on PATCH).
type ProductInput struct {
	Name        *string
	Description *string
	Facts       *domain.Facts
	Status      *string
}

func (in ProductInput) apply(p *domain.Product, v *kernel.ValidationErrors) {
	if in.Name != nil {
		p.Name = strings.TrimSpace(*in.Name)
	}
	if in.Description != nil {
		p.Description = strings.TrimSpace(*in.Description)
	}
	if in.Facts != nil {
		p.Facts = in.Facts.Normalize()
	}
	if in.Status != nil {
		p.Status = *in.Status
	}
	domain.ValidateName(v, "name", p.Name)
	if len([]rune(p.Description)) > domain.MaxTextLen {
		v.Add("description", "too_long")
	}
	if p.Status != domain.ProductActive && p.Status != domain.ProductArchived {
		v.Add("status", "invalid")
	}
	p.Facts.Validate(v, "facts")
}

// CreateProduct adds a product to a brand (brand.manage).
func (s *Service) CreateProduct(ctx context.Context, actor kernel.Actor, brandID kernel.ID, in ProductInput) (*domain.Product, error) {
	if err := actor.Require(kernel.PermBrandManage); err != nil {
		return nil, err
	}
	now := s.Clock.Now()
	by := actor.UserID
	p := &domain.Product{ID: kernel.NewID(), BrandID: brandID, Status: domain.ProductActive, Facts: domain.Facts{}.Normalize(), CreatedBy: &by, CreatedAt: now, UpdatedAt: now}
	if in.Name == nil {
		in.Name = new(string)
	}
	var v kernel.ValidationErrors
	in.apply(p, &v)
	if err := v.Err(); err != nil {
		return nil, err
	}
	err := s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		if _, err := s.Repo.GetBrand(ctx, brandID); err != nil {
			return err
		}
		if err := s.Repo.CreateProduct(ctx, p); err != nil {
			return err
		}
		return s.Audit.Record(ctx, "product.created", resProduct, p.ID.String(), nil, auditProduct(p))
	})
	if err != nil {
		return nil, err
	}
	return p, nil
}

// UpdateProduct edits a product (brand.manage).
func (s *Service) UpdateProduct(ctx context.Context, actor kernel.Actor, id kernel.ID, in ProductInput) (*domain.Product, error) {
	if err := actor.Require(kernel.PermBrandManage); err != nil {
		return nil, err
	}
	var out *domain.Product
	err := s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		p, err := s.Repo.GetProduct(ctx, id)
		if err != nil {
			return err
		}
		before := auditProduct(p)
		var v kernel.ValidationErrors
		in.apply(p, &v)
		if err := v.Err(); err != nil {
			return err
		}
		p.UpdatedAt = s.Clock.Now()
		if err := s.Repo.UpdateProduct(ctx, p); err != nil {
			return err
		}
		out = p
		return s.Audit.Record(ctx, "product.updated", resProduct, p.ID.String(), before, auditProduct(p))
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func auditProduct(p *domain.Product) map[string]any {
	return map[string]any{"brand_id": p.BrandID, "name": p.Name, "status": p.Status, "facts": p.Facts}
}

// ---------- assets ----------

// AssetView is a brand asset with its file metadata and a short-lived URL.
type AssetView struct {
	domain.Asset
	File *fileuc.File
	URL  *fileuc.SignedURL
}

// ListAssets lists brand assets with signed URLs (brand.view).
func (s *Service) ListAssets(ctx context.Context, actor kernel.Actor, brandID kernel.ID, types []string) ([]AssetView, error) {
	if err := actor.Require(kernel.PermBrandView); err != nil {
		return nil, err
	}
	for _, t := range types {
		if !domain.IsAssetType(t) {
			return nil, kernel.Validation(kernel.FieldIssue("asset_type", "invalid"))
		}
	}
	if _, err := s.Repo.GetBrand(ctx, brandID); err != nil {
		return nil, err
	}
	as, err := s.Repo.ListAssets(ctx, brandID, types)
	if err != nil {
		return nil, err
	}
	ids := make([]kernel.ID, len(as))
	for i, a := range as {
		ids[i] = a.FileID
	}
	files := map[kernel.ID]fileuc.File{}
	if len(ids) > 0 {
		fs, err := s.Files.GetMany(ctx, ids)
		if err != nil {
			return nil, err
		}
		for _, f := range fs {
			files[f.ID] = f
		}
	}
	out := make([]AssetView, 0, len(as))
	for _, a := range as {
		av := AssetView{Asset: a}
		if f, ok := files[a.FileID]; ok {
			av.File = &f
			if u, err := s.Files.SignedURL(ctx, a.FileID, ""); err == nil {
				av.URL = &u
			}
		}
		out = append(out, av)
	}
	return out, nil
}

// AddAssetInput is POST /brands/{id}/assets.
type AddAssetInput struct {
	FileID    kernel.ID
	AssetType string
	Label     string
	License   *domain.License
}

// AddAsset attaches an uploaded file to a brand (brand.manage). Music
// requires a license (OQ-04, ACS-023).
func (s *Service) AddAsset(ctx context.Context, actor kernel.Actor, brandID kernel.ID, in AddAssetInput) (*domain.Asset, error) {
	if err := actor.Require(kernel.PermBrandManage); err != nil {
		return nil, err
	}
	var v kernel.ValidationErrors
	if in.FileID == kernel.NilID {
		v.Add("file_id", "required")
	}
	if !domain.IsAssetType(in.AssetType) {
		v.Add("asset_type", "invalid")
	}
	label := strings.TrimSpace(in.Label)
	if len([]rune(label)) > domain.MaxItemLen {
		v.Add("label", "too_long")
	}
	var license json.RawMessage
	if in.License != nil {
		l := in.License.Normalize()
		l.Validate(&v, "license")
		license, _ = json.Marshal(l)
	} else if in.AssetType == domain.AssetMusic {
		v.Add("license", "required")
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	f, err := s.requireFile(ctx, in.FileID, in.AssetType)
	if err != nil {
		return nil, err
	}
	if f.CreatedBy != nil && !actor.CanModifyOwned(*f.CreatedBy) {
		return nil, kernel.ErrNotFound
	}
	by := actor.UserID
	a := &domain.Asset{ID: kernel.NewID(), BrandID: brandID, FileID: in.FileID, AssetType: in.AssetType, Label: label, License: license, CreatedBy: &by, CreatedAt: s.Clock.Now()}
	err = s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		if _, err := s.Repo.GetBrand(ctx, brandID); err != nil {
			return err
		}
		if err := s.Repo.CreateAsset(ctx, a); err != nil {
			return err
		}
		return s.Audit.Record(ctx, "brand_asset.created", resAsset, a.ID.String(), nil,
			map[string]any{"brand_id": brandID, "file_id": a.FileID, "asset_type": a.AssetType, "license": a.License})
	})
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (s *Service) requireFile(ctx context.Context, id kernel.ID, assetType string) (fileuc.File, error) {
	f, err := s.Files.Get(ctx, id)
	if err != nil {
		return fileuc.File{}, err
	}
	if string(f.UploadStatus) != "ready" {
		return fileuc.File{}, kernel.ErrNotFound
	}
	var kinds []string
	switch assetType {
	case domain.AssetLogo, domain.AssetReferenceImage:
		kinds = []string{"image"}
	case domain.AssetMusic:
		kinds = []string{"audio"}
	case domain.AssetDocument, domain.AssetSampleContent:
		kinds = []string{"document", "image"}
	}
	if len(kinds) > 0 && !slices.Contains(kinds, string(f.Kind)) {
		return fileuc.File{}, kernel.ErrUnsupportedMedia.WithDetails(kernel.Detail{"field": "file_id", "expected_kinds": kinds})
	}
	return f, nil
}

// DeleteAsset detaches an asset from a brand (brand.manage).
func (s *Service) DeleteAsset(ctx context.Context, actor kernel.Actor, brandID, assetID kernel.ID) error {
	if err := actor.Require(kernel.PermBrandManage); err != nil {
		return err
	}
	return s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		a, err := s.Repo.GetAsset(ctx, assetID)
		if err != nil {
			return err
		}
		if a.BrandID != brandID {
			return kernel.ErrNotFound
		}
		if err := s.Repo.DeleteAsset(ctx, brandID, assetID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, "brand_asset.deleted", resAsset, assetID.String(),
			map[string]any{"brand_id": brandID, "file_id": a.FileID, "asset_type": a.AssetType}, nil)
	})
}

// ---------- public API ----------

// GetBrandContext implements API.
func (s *Service) GetBrandContext(ctx context.Context, req BrandContextRequest) (BrandContext, error) {
	b, err := s.Repo.GetBrand(ctx, req.BrandID)
	if err != nil {
		return BrandContext{}, err
	}
	ver := b.Current
	if req.BrandVersionID != nil && *req.BrandVersionID != ver.ID {
		v, err := s.Repo.GetVersion(ctx, *req.BrandVersionID)
		if err != nil {
			return BrandContext{}, err
		}
		if v.BrandID != b.ID {
			return BrandContext{}, kernel.ErrNotFound
		}
		ver = *v
	}
	at := req.At
	if at.IsZero() {
		at = s.Clock.Now()
	}
	products, err := s.Repo.ListActiveProducts(ctx, b.ID)
	if err != nil {
		return BrandContext{}, err
	}
	var selected []domain.Product
	if len(req.ProductIDs) == 0 {
		selected = products
	} else {
		byID := map[kernel.ID]domain.Product{}
		for _, p := range products {
			byID[p.ID] = p
		}
		for _, id := range req.ProductIDs {
			p, ok := byID[id]
			if !ok {
				return BrandContext{}, kernel.ErrNotFound.WithDetails(kernel.Detail{"product_id": id})
			}
			selected = append(selected, p)
		}
	}
	return toContext(b, ver, selected, at), nil
}

func toContext(b *domain.Brand, v domain.Version, products []domain.Product, at time.Time) BrandContext {
	p := v.Profile
	raw, _ := json.Marshal(p)
	bc := BrandContext{
		BrandID: b.ID, BrandVersionID: v.ID, VersionNo: v.VersionNo, Name: b.Name, Description: p.Description,
		Typography:  Typography{Heading: p.Typography.Heading, Body: p.Typography.Body},
		ToneOfVoice: p.ToneOfVoice, PreferredWords: p.PreferredWords, ForbiddenWords: p.ForbiddenWords,
		DefaultCTAs: p.DefaultCTAs, Notes: p.Notes, Profile: raw,
		Colors: make([]Color, 0, len(p.Colors)), AudiencePersonas: make([]Persona, 0, len(p.AudiencePersonas)),
		Products: make([]ProductFacts, 0, len(products)),
	}
	for _, c := range p.Colors {
		bc.Colors = append(bc.Colors, Color{Name: c.Name, Hex: c.Hex})
	}
	for _, ps := range p.AudiencePersonas {
		bc.AudiencePersonas = append(bc.AudiencePersonas, Persona{Name: ps.Name, PainPoints: ps.PainPoints, Motivations: ps.Motivations})
	}
	for _, pr := range products {
		bc.Products = append(bc.Products, toFacts(pr, at))
	}
	return bc
}

func toFacts(p domain.Product, at time.Time) ProductFacts {
	f := p.Facts.PromptFacts(at)
	pf := ProductFacts{ProductID: p.ID, Name: p.Name, Description: p.Description, USP: f.USP, Policies: f.Policies,
		Offers: make([]Offer, 0, len(f.Offers)), Proof: make([]Proof, 0, len(f.Proof))}
	if f.Price != nil {
		pf.Price = &Price{Amount: f.Price.Amount, Currency: f.Price.Currency, Note: f.Price.Note}
	}
	for _, o := range f.Offers {
		pf.Offers = append(pf.Offers, Offer{Text: o.Text, ValidFrom: parseDate(o.ValidFrom), ValidTo: parseDate(o.ValidTo)})
	}
	for _, pr := range f.Proof {
		pf.Proof = append(pf.Proof, Proof{Type: pr.Type, Text: pr.Text, Source: pr.Source})
	}
	return pf
}

func parseDate(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse(domain.DateLayout, s)
	if err != nil {
		return nil
	}
	return &t
}

func (s *Service) toAPIAsset(a *domain.Asset) BrandAsset {
	return BrandAsset{ID: a.ID, BrandID: a.BrandID, FileID: a.FileID, AssetType: a.AssetType, Label: a.Label,
		License: a.License, LicenseValid: a.LicenseUsableAt(s.Clock.Now())}
}

// GetBrandAsset implements API.
func (s *Service) GetBrandAsset(ctx context.Context, id kernel.ID) (BrandAsset, error) {
	a, err := s.Repo.GetAsset(ctx, id)
	if err != nil {
		return BrandAsset{}, err
	}
	return s.toAPIAsset(a), nil
}

// ListBrandAssets implements API.
func (s *Service) ListBrandAssets(ctx context.Context, brandID kernel.ID, types ...string) ([]BrandAsset, error) {
	if _, err := s.Repo.GetBrand(ctx, brandID); err != nil {
		return nil, err
	}
	as, err := s.Repo.ListAssets(ctx, brandID, types)
	if err != nil {
		return nil, err
	}
	out := make([]BrandAsset, 0, len(as))
	for i := range as {
		out = append(out, s.toAPIAsset(&as[i]))
	}
	return out, nil
}

// RequireLicensedMusic implements API.
func (s *Service) RequireLicensedMusic(ctx context.Context, brandID, id kernel.ID) (BrandAsset, error) {
	a, err := s.Repo.GetAsset(ctx, id)
	if err != nil {
		return BrandAsset{}, err
	}
	if a.BrandID != brandID || a.AssetType != domain.AssetMusic {
		return BrandAsset{}, kernel.ErrNotFound
	}
	ba := s.toAPIAsset(a)
	if !ba.LicenseValid {
		return BrandAsset{}, ErrMusicLicenseRequired
	}
	return ba, nil
}
