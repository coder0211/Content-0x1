package usecase

import (
	"context"

	"github.com/be/acs/api/internal/modules/brand/domain"
	fileuc "github.com/be/acs/api/internal/modules/file/usecase"
	"github.com/be/acs/api/internal/shared/kernel"
)

// BrandFilter filters ListBrands.
type BrandFilter struct {
	Q string
}

// ProductFilter filters ListProducts; Status "" means all.
type ProductFilter struct {
	Status string
}

// Repository persists brands, versions, products and brand assets.
// Implementations join the caller's transaction (ports.TxManager).
type Repository interface {
	// CreateBrand inserts the brand and its first version and points
	// current_version_id at it. Duplicate name → kernel.ErrConflict.
	CreateBrand(ctx context.Context, b *domain.Brand) error
	// GetBrand returns a non-deleted brand with its current version.
	GetBrand(ctx context.Context, id kernel.ID) (*domain.Brand, error)
	// GetBrandForUpdate is GetBrand with a row lock (optimistic-lock check).
	GetBrandForUpdate(ctx context.Context, id kernel.ID) (*domain.Brand, error)
	ListBrands(ctx context.Context, f BrandFilter, p kernel.PageRequest) (kernel.Page[domain.Brand], error)
	// RenameBrand updates the name. Duplicate name → kernel.ErrConflict.
	RenameBrand(ctx context.Context, id kernel.ID, name string) error
	// AddVersion inserts v and makes it current. A concurrent insert of the
	// same version_no → kernel.ErrVersionConflict.
	AddVersion(ctx context.Context, v domain.Version) error
	SoftDeleteBrand(ctx context.Context, id kernel.ID) error
	ListVersions(ctx context.Context, brandID kernel.ID, p kernel.PageRequest) (kernel.Page[domain.Version], error)
	GetVersion(ctx context.Context, id kernel.ID) (*domain.Version, error)

	CreateProduct(ctx context.Context, p *domain.Product) error
	// GetProduct returns a product of a non-deleted brand.
	GetProduct(ctx context.Context, id kernel.ID) (*domain.Product, error)
	UpdateProduct(ctx context.Context, p *domain.Product) error
	ListProducts(ctx context.Context, brandID kernel.ID, f ProductFilter, p kernel.PageRequest) (kernel.Page[domain.Product], error)
	ListActiveProducts(ctx context.Context, brandID kernel.ID) ([]domain.Product, error)

	CreateAsset(ctx context.Context, a *domain.Asset) error
	// GetAsset returns an asset of a non-deleted brand.
	GetAsset(ctx context.Context, id kernel.ID) (*domain.Asset, error)
	ListAssets(ctx context.Context, brandID kernel.ID, types []string) ([]domain.Asset, error)
	DeleteAsset(ctx context.Context, brandID, id kernel.ID) error
}

// FileReader is the subset of file.API the brand module uses (file.API
// satisfies it). Readiness and kind are checked here so tests need no
// file-module internals.
type FileReader interface {
	Get(ctx context.Context, id kernel.ID) (fileuc.File, error)
	GetMany(ctx context.Context, ids []kernel.ID) ([]fileuc.File, error)
	SignedURL(ctx context.Context, id kernel.ID, downloadName string) (fileuc.SignedURL, error)
}
