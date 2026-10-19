// Package brandtest provides an in-memory brand usecase.API.
package brandtest

import (
	"context"
	"slices"

	"github.com/be/acs/api/internal/modules/brand/usecase"
	"github.com/be/acs/api/internal/shared/kernel"
)

// API is the fake; populate Brands/Assets directly.
type API struct {
	Brands map[kernel.ID]usecase.BrandContext
	Assets map[kernel.ID]usecase.BrandAsset
}

// NewAPI returns an empty fake.
func NewAPI() *API {
	return &API{Brands: map[kernel.ID]usecase.BrandContext{}, Assets: map[kernel.ID]usecase.BrandAsset{}}
}

// AddSample registers a Vietnamese sample brand and returns its context.
func (a *API) AddSample() usecase.BrandContext {
	bc := usecase.BrandContext{
		BrandID: kernel.NewID(), BrandVersionID: kernel.NewID(), VersionNo: 1, Name: "Anh ngữ Sao Mai",
		ToneOfVoice: []string{"friendly", "trustworthy"}, ForbiddenWords: []string{"cam kết 100%", "rẻ nhất"},
		DefaultCTAs: []string{"Đăng ký tư vấn"}, Colors: []usecase.Color{{Name: "primary", Hex: "#0055FF"}},
	}
	a.Brands[bc.BrandID] = bc
	return bc
}

var _ usecase.API = (*API)(nil)

func (a *API) GetBrandContext(_ context.Context, req usecase.BrandContextRequest) (usecase.BrandContext, error) {
	bc, ok := a.Brands[req.BrandID]
	if !ok || (req.BrandVersionID != nil && *req.BrandVersionID != bc.BrandVersionID) {
		return usecase.BrandContext{}, kernel.ErrNotFound
	}
	if len(req.ProductIDs) > 0 {
		var ps []usecase.ProductFacts
		for _, p := range bc.Products {
			if slices.Contains(req.ProductIDs, p.ProductID) {
				ps = append(ps, p)
			}
		}
		if len(ps) != len(req.ProductIDs) {
			return usecase.BrandContext{}, kernel.ErrNotFound
		}
		bc.Products = ps
	}
	return bc, nil
}

func (a *API) GetBrandAsset(_ context.Context, id kernel.ID) (usecase.BrandAsset, error) {
	ba, ok := a.Assets[id]
	if !ok {
		return usecase.BrandAsset{}, kernel.ErrNotFound
	}
	return ba, nil
}

func (a *API) ListBrandAssets(_ context.Context, brandID kernel.ID, types ...string) ([]usecase.BrandAsset, error) {
	var out []usecase.BrandAsset
	for _, ba := range a.Assets {
		if ba.BrandID == brandID && (len(types) == 0 || slices.Contains(types, ba.AssetType)) {
			out = append(out, ba)
		}
	}
	return out, nil
}
