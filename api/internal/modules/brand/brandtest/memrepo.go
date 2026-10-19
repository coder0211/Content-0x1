package brandtest

import (
	"context"
	"slices"
	"strings"
	"sync"

	"github.com/be/acs/api/internal/modules/brand/domain"
	"github.com/be/acs/api/internal/modules/brand/usecase"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Repo is an in-memory usecase.Repository for use-case and handler tests.
type Repo struct {
	mu       sync.Mutex
	brands   map[kernel.ID]*domain.Brand
	deleted  map[kernel.ID]bool
	versions map[kernel.ID]domain.Version
	products map[kernel.ID]*domain.Product
	assets   map[kernel.ID]*domain.Asset
}

// NewRepo returns an empty repository.
func NewRepo() *Repo {
	return &Repo{brands: map[kernel.ID]*domain.Brand{}, deleted: map[kernel.ID]bool{}, versions: map[kernel.ID]domain.Version{},
		products: map[kernel.ID]*domain.Product{}, assets: map[kernel.ID]*domain.Asset{}}
}

var _ usecase.Repository = (*Repo)(nil)

func (r *Repo) CreateBrand(_ context.Context, b *domain.Brand) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, x := range r.brands {
		if !r.deleted[id] && strings.EqualFold(x.Name, b.Name) {
			return kernel.ErrConflict
		}
	}
	c := *b
	r.brands[b.ID] = &c
	r.versions[b.Current.ID] = b.Current
	return nil
}

func (r *Repo) GetBrand(_ context.Context, id kernel.ID) (*domain.Brand, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.brands[id]
	if !ok || r.deleted[id] {
		return nil, kernel.ErrNotFound
	}
	c := *b
	return &c, nil
}

func (r *Repo) GetBrandForUpdate(ctx context.Context, id kernel.ID) (*domain.Brand, error) {
	return r.GetBrand(ctx, id)
}

func (r *Repo) ListBrands(_ context.Context, f usecase.BrandFilter, p kernel.PageRequest) (kernel.Page[domain.Brand], error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []domain.Brand
	for id, b := range r.brands {
		if !r.deleted[id] && (f.Q == "" || strings.Contains(strings.ToLower(b.Name), strings.ToLower(f.Q))) {
			out = append(out, *b)
		}
	}
	slices.SortFunc(out, func(a, b domain.Brand) int { return b.CreatedAt.Compare(a.CreatedAt) })
	return kernel.Page[domain.Brand]{Items: out}, nil
}

func (r *Repo) RenameBrand(_ context.Context, id kernel.ID, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for oid, x := range r.brands {
		if oid != id && !r.deleted[oid] && strings.EqualFold(x.Name, name) {
			return kernel.ErrConflict
		}
	}
	r.brands[id].Name = name
	return nil
}

func (r *Repo) AddVersion(_ context.Context, v domain.Version) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, x := range r.versions {
		if x.BrandID == v.BrandID && x.VersionNo == v.VersionNo {
			return kernel.ErrVersionConflict
		}
	}
	r.versions[v.ID] = v
	r.brands[v.BrandID].Current = v
	return nil
}

func (r *Repo) SoftDeleteBrand(_ context.Context, id kernel.ID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deleted[id] = true
	return nil
}

func (r *Repo) ListVersions(_ context.Context, brandID kernel.ID, _ kernel.PageRequest) (kernel.Page[domain.Version], error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []domain.Version
	for _, v := range r.versions {
		if v.BrandID == brandID {
			out = append(out, v)
		}
	}
	slices.SortFunc(out, func(a, b domain.Version) int { return b.VersionNo - a.VersionNo })
	return kernel.Page[domain.Version]{Items: out}, nil
}

func (r *Repo) GetVersion(_ context.Context, id kernel.ID) (*domain.Version, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.versions[id]
	if !ok {
		return nil, kernel.ErrNotFound
	}
	return &v, nil
}

func (r *Repo) CreateProduct(_ context.Context, p *domain.Product) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := *p
	r.products[p.ID] = &c
	return nil
}

func (r *Repo) GetProduct(_ context.Context, id kernel.ID) (*domain.Product, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.products[id]
	if !ok || r.deleted[p.BrandID] {
		return nil, kernel.ErrNotFound
	}
	c := *p
	return &c, nil
}

func (r *Repo) UpdateProduct(_ context.Context, p *domain.Product) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := *p
	r.products[p.ID] = &c
	return nil
}

func (r *Repo) ListProducts(_ context.Context, brandID kernel.ID, f usecase.ProductFilter, _ kernel.PageRequest) (kernel.Page[domain.Product], error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []domain.Product
	for _, p := range r.products {
		if p.BrandID == brandID && (f.Status == "" || f.Status == p.Status) {
			out = append(out, *p)
		}
	}
	slices.SortFunc(out, func(a, b domain.Product) int { return b.CreatedAt.Compare(a.CreatedAt) })
	return kernel.Page[domain.Product]{Items: out}, nil
}

func (r *Repo) ListActiveProducts(ctx context.Context, brandID kernel.ID) ([]domain.Product, error) {
	p, err := r.ListProducts(ctx, brandID, usecase.ProductFilter{Status: domain.ProductActive}, kernel.PageRequest{})
	return p.Items, err
}

func (r *Repo) CreateAsset(_ context.Context, a *domain.Asset) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := *a
	r.assets[a.ID] = &c
	return nil
}

func (r *Repo) GetAsset(_ context.Context, id kernel.ID) (*domain.Asset, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.assets[id]
	if !ok || r.deleted[a.BrandID] {
		return nil, kernel.ErrNotFound
	}
	c := *a
	return &c, nil
}

func (r *Repo) ListAssets(_ context.Context, brandID kernel.ID, types []string) ([]domain.Asset, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []domain.Asset{}
	for _, a := range r.assets {
		if a.BrandID == brandID && (len(types) == 0 || slices.Contains(types, a.AssetType)) {
			out = append(out, *a)
		}
	}
	return out, nil
}

func (r *Repo) DeleteAsset(_ context.Context, brandID, id kernel.ID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if a, ok := r.assets[id]; !ok || a.BrandID != brandID {
		return kernel.ErrNotFound
	}
	delete(r.assets, id)
	return nil
}
