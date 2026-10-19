package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/be/acs/api/internal/modules/brand/domain"
	"github.com/be/acs/api/internal/modules/brand/usecase"
	db "github.com/be/acs/api/internal/platform/postgres"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Repository implements usecase.Repository on PostgreSQL.
type Repository struct{ pool *pgxpool.Pool }

// NewRepository returns a repository on pool.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

var _ usecase.Repository = (*Repository)(nil)

func (r *Repository) q(ctx context.Context) *Queries { return New(db.Conn(ctx, r.pool)) }

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

type brandRow struct {
	ID               uuid.UUID
	Name             string
	CreatedBy        *uuid.UUID
	CreatedAt        time.Time
	UpdatedAt        time.Time
	VersionID        uuid.UUID
	VersionNo        int32
	Profile          json.RawMessage
	VersionCreatedBy *uuid.UUID
	VersionCreatedAt time.Time
}

func (b brandRow) toDomain() domain.Brand {
	return domain.Brand{ID: b.ID, Name: b.Name, CreatedBy: b.CreatedBy, CreatedAt: b.CreatedAt, UpdatedAt: b.UpdatedAt,
		Current: domain.Version{ID: b.VersionID, BrandID: b.ID, VersionNo: int(b.VersionNo), Profile: domain.ParseProfile(b.Profile),
			CreatedBy: b.VersionCreatedBy, CreatedAt: b.VersionCreatedAt}}
}

func versionToDomain(v BrandVersion) domain.Version {
	return domain.Version{ID: v.ID, BrandID: v.BrandID, VersionNo: int(v.VersionNo), Profile: domain.ParseProfile(v.Profile), CreatedBy: v.CreatedBy, CreatedAt: v.CreatedAt}
}

func productToDomain(p Product) domain.Product {
	return domain.Product{ID: p.ID, BrandID: p.BrandID, Name: p.Name, Description: deref(p.Description), Facts: domain.ParseFacts(p.Facts),
		Status: p.Status, CreatedBy: p.CreatedBy, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt}
}

func assetToDomain(a BrandAsset) domain.Asset {
	return domain.Asset{ID: a.ID, BrandID: a.BrandID, FileID: a.FileID, AssetType: a.AssetType, Label: deref(a.Label),
		License: a.LicenseJson, CreatedBy: a.CreatedBy, CreatedAt: a.CreatedAt}
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

// CreateBrand implements usecase.Repository.
func (r *Repository) CreateBrand(ctx context.Context, b *domain.Brand) error {
	q := r.q(ctx)
	if err := q.InsertBrand(ctx, InsertBrandParams{ID: b.ID, Name: b.Name, CreatedBy: b.CreatedBy, CreatedAt: b.CreatedAt}); err != nil {
		if db.IsUniqueViolation(err, "") {
			return kernel.ErrConflict.Wrap(err)
		}
		return err
	}
	return r.AddVersion(ctx, b.Current)
}

// GetBrand implements usecase.Repository.
func (r *Repository) GetBrand(ctx context.Context, id kernel.ID) (*domain.Brand, error) {
	row, err := r.q(ctx).GetBrand(ctx, id)
	if err != nil {
		return nil, db.MapNotFound(err)
	}
	b := brandRow(row).toDomain()
	return &b, nil
}

// GetBrandForUpdate implements usecase.Repository.
func (r *Repository) GetBrandForUpdate(ctx context.Context, id kernel.ID) (*domain.Brand, error) {
	row, err := r.q(ctx).GetBrandForUpdate(ctx, id)
	if err != nil {
		return nil, db.MapNotFound(err)
	}
	b := brandRow(row).toDomain()
	return &b, nil
}

// ListBrands implements usecase.Repository.
func (r *Repository) ListBrands(ctx context.Context, f usecase.BrandFilter, p kernel.PageRequest) (kernel.Page[domain.Brand], error) {
	params := ListBrandsParams{Q: strPtr(f.Q), Lim: int32(p.Limit + 1)}
	var c kernel.TimeIDCursor
	if ok, err := kernel.DecodeCursor(p.Cursor, &c); err != nil {
		return kernel.Page[domain.Brand]{}, err
	} else if ok {
		params.CursorAt, params.CursorID = &c.At, &c.ID
	}
	rows, err := r.q(ctx).ListBrands(ctx, params)
	if err != nil {
		return kernel.Page[domain.Brand]{}, err
	}
	out := make([]domain.Brand, len(rows))
	for i, row := range rows {
		out[i] = brandRow(row).toDomain()
	}
	return kernel.PageFromRows(out, p.Limit, func(b domain.Brand) any { return kernel.TimeIDCursor{At: b.CreatedAt, ID: b.ID} }), nil
}

// RenameBrand implements usecase.Repository.
func (r *Repository) RenameBrand(ctx context.Context, id kernel.ID, name string) error {
	if err := r.q(ctx).RenameBrand(ctx, RenameBrandParams{ID: id, Name: name}); err != nil {
		if db.IsUniqueViolation(err, "") {
			return kernel.ErrConflict.Wrap(err)
		}
		return err
	}
	return nil
}

// AddVersion implements usecase.Repository.
func (r *Repository) AddVersion(ctx context.Context, v domain.Version) error {
	q := r.q(ctx)
	err := q.InsertBrandVersion(ctx, InsertBrandVersionParams{ID: v.ID, BrandID: v.BrandID, VersionNo: int32(v.VersionNo),
		Profile: mustJSON(v.Profile), CreatedBy: v.CreatedBy, CreatedAt: v.CreatedAt})
	if err != nil {
		if db.IsUniqueViolation(err, "") {
			return kernel.ErrVersionConflict.Wrap(err)
		}
		return err
	}
	return q.SetBrandCurrentVersion(ctx, SetBrandCurrentVersionParams{ID: v.BrandID, VersionID: &v.ID})
}

// SoftDeleteBrand implements usecase.Repository.
func (r *Repository) SoftDeleteBrand(ctx context.Context, id kernel.ID) error {
	return r.q(ctx).SoftDeleteBrand(ctx, id)
}

// ListVersions implements usecase.Repository (cursor = version_no).
func (r *Repository) ListVersions(ctx context.Context, brandID kernel.ID, p kernel.PageRequest) (kernel.Page[domain.Version], error) {
	params := ListBrandVersionsParams{BrandID: brandID, Lim: int32(p.Limit + 1)}
	var before int32
	if ok, err := kernel.DecodeCursor(p.Cursor, &before); err != nil {
		return kernel.Page[domain.Version]{}, err
	} else if ok {
		params.BeforeNo = &before
	}
	rows, err := r.q(ctx).ListBrandVersions(ctx, params)
	if err != nil {
		return kernel.Page[domain.Version]{}, err
	}
	out := make([]domain.Version, len(rows))
	for i, v := range rows {
		out[i] = versionToDomain(v)
	}
	return kernel.PageFromRows(out, p.Limit, func(v domain.Version) any { return v.VersionNo }), nil
}

// GetVersion implements usecase.Repository.
func (r *Repository) GetVersion(ctx context.Context, id kernel.ID) (*domain.Version, error) {
	v, err := r.q(ctx).GetBrandVersion(ctx, id)
	if err != nil {
		return nil, db.MapNotFound(err)
	}
	d := versionToDomain(v)
	return &d, nil
}

// CreateProduct implements usecase.Repository.
func (r *Repository) CreateProduct(ctx context.Context, p *domain.Product) error {
	return r.q(ctx).InsertProduct(ctx, InsertProductParams{ID: p.ID, BrandID: p.BrandID, Name: p.Name, Description: strPtr(p.Description),
		Facts: mustJSON(p.Facts), Status: p.Status, CreatedBy: p.CreatedBy, CreatedAt: p.CreatedAt})
}

// GetProduct implements usecase.Repository.
func (r *Repository) GetProduct(ctx context.Context, id kernel.ID) (*domain.Product, error) {
	p, err := r.q(ctx).GetProduct(ctx, id)
	if err != nil {
		return nil, db.MapNotFound(err)
	}
	d := productToDomain(p)
	return &d, nil
}

// UpdateProduct implements usecase.Repository.
func (r *Repository) UpdateProduct(ctx context.Context, p *domain.Product) error {
	return r.q(ctx).UpdateProduct(ctx, UpdateProductParams{ID: p.ID, Name: p.Name, Description: strPtr(p.Description), Facts: mustJSON(p.Facts), Status: p.Status})
}

// ListProducts implements usecase.Repository.
func (r *Repository) ListProducts(ctx context.Context, brandID kernel.ID, f usecase.ProductFilter, p kernel.PageRequest) (kernel.Page[domain.Product], error) {
	params := ListProductsParams{BrandID: brandID, Status: strPtr(f.Status), Lim: int32(p.Limit + 1)}
	var c kernel.TimeIDCursor
	if ok, err := kernel.DecodeCursor(p.Cursor, &c); err != nil {
		return kernel.Page[domain.Product]{}, err
	} else if ok {
		params.CursorAt, params.CursorID = &c.At, &c.ID
	}
	rows, err := r.q(ctx).ListProducts(ctx, params)
	if err != nil {
		return kernel.Page[domain.Product]{}, err
	}
	out := make([]domain.Product, len(rows))
	for i, row := range rows {
		out[i] = productToDomain(row)
	}
	return kernel.PageFromRows(out, p.Limit, func(p domain.Product) any { return kernel.TimeIDCursor{At: p.CreatedAt, ID: p.ID} }), nil
}

// ListActiveProducts implements usecase.Repository.
func (r *Repository) ListActiveProducts(ctx context.Context, brandID kernel.ID) ([]domain.Product, error) {
	rows, err := r.q(ctx).ListActiveProducts(ctx, brandID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Product, len(rows))
	for i, row := range rows {
		out[i] = productToDomain(row)
	}
	return out, nil
}

// CreateAsset implements usecase.Repository.
func (r *Repository) CreateAsset(ctx context.Context, a *domain.Asset) error {
	return r.q(ctx).InsertBrandAsset(ctx, InsertBrandAssetParams{ID: a.ID, BrandID: a.BrandID, FileID: a.FileID, AssetType: a.AssetType,
		Label: strPtr(a.Label), LicenseJson: a.License, CreatedBy: a.CreatedBy, CreatedAt: a.CreatedAt})
}

// GetAsset implements usecase.Repository.
func (r *Repository) GetAsset(ctx context.Context, id kernel.ID) (*domain.Asset, error) {
	a, err := r.q(ctx).GetBrandAsset(ctx, id)
	if err != nil {
		return nil, db.MapNotFound(err)
	}
	d := assetToDomain(a)
	return &d, nil
}

// ListAssets implements usecase.Repository.
func (r *Repository) ListAssets(ctx context.Context, brandID kernel.ID, types []string) ([]domain.Asset, error) {
	if types == nil {
		types = []string{}
	}
	rows, err := r.q(ctx).ListBrandAssets(ctx, ListBrandAssetsParams{BrandID: brandID, Types: types})
	if err != nil {
		return nil, err
	}
	out := make([]domain.Asset, len(rows))
	for i, row := range rows {
		out[i] = assetToDomain(row)
	}
	return out, nil
}

// DeleteAsset implements usecase.Repository.
func (r *Repository) DeleteAsset(ctx context.Context, brandID, id kernel.ID) error {
	n, err := r.q(ctx).DeleteBrandAsset(ctx, DeleteBrandAssetParams{ID: id, BrandID: brandID})
	if err != nil {
		return err
	}
	if n == 0 {
		return kernel.ErrNotFound
	}
	return nil
}
