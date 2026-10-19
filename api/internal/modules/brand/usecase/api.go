// Package usecase is the public contract of the brand module: brand context
// (a specific brand version + valid product facts) for prompt building.
package usecase

import (
	"context"
	"encoding/json"
	"time"

	"github.com/shopspring/decimal"

	"github.com/be/acs/api/internal/shared/kernel"
)

// Color is a named brand color.
type Color struct {
	Name string
	Hex  string
}

// Typography holds font families.
type Typography struct {
	Heading string
	Body    string
}

// Persona is an audience persona.
type Persona struct {
	Name        string
	PainPoints  []string
	Motivations []string
}

// Price is a verified product price.
type Price struct {
	Amount   decimal.Decimal
	Currency string
	Note     string
}

// Offer is a time-boxed offer; only offers valid at the request time are
// returned in ProductFacts.
type Offer struct {
	Text      string
	ValidFrom *time.Time
	ValidTo   *time.Time
}

// Proof is social proof; only verified proof is returned.
type Proof struct {
	Type   string
	Text   string
	Source string
}

// ProductFacts are the ONLY commercial facts prompts may use (FR-CNT-06).
type ProductFacts struct {
	ProductID   kernel.ID
	Name        string
	Description string
	USP         []string
	Price       *Price
	Offers      []Offer
	Policies    []string
	Proof       []Proof
}

// BrandContext is a brand version flattened for generation.
type BrandContext struct {
	BrandID          kernel.ID
	BrandVersionID   kernel.ID
	VersionNo        int
	Name             string
	Description      string
	Colors           []Color
	Typography       Typography
	ToneOfVoice      []string
	PreferredWords   []string
	ForbiddenWords   []string
	DefaultCTAs      []string
	AudiencePersonas []Persona
	Notes            string
	Products         []ProductFacts
	// Profile is the raw brand_versions.profile for forward compatibility.
	Profile json.RawMessage
}

// BrandContextRequest selects the brand version and products.
type BrandContextRequest struct {
	BrandID kernel.ID
	// BrandVersionID pins a version (re-generation); nil = current version.
	BrandVersionID *kernel.ID
	ProductIDs     []kernel.ID
	// At filters offers by validity; zero = now.
	At time.Time
}

// BrandAsset is a file attached to a brand (logo, reference image, music).
type BrandAsset struct {
	ID        kernel.ID
	BrandID   kernel.ID
	FileID    kernel.ID
	AssetType string // logo | reference_image | font | document | sample_content | music
	Label     string
	// License is asset_files.license_json; music without a license must not
	// be used in videos (ACS-023).
	License json.RawMessage
}

// API is the brand module's public interface.
type API interface {
	// GetBrandContext returns NOT_FOUND for deleted brands or products of
	// another brand.
	GetBrandContext(ctx context.Context, req BrandContextRequest) (BrandContext, error)
	// GetBrandAsset returns one brand asset (e.g. a music track).
	GetBrandAsset(ctx context.Context, brandAssetID kernel.ID) (BrandAsset, error)
	// ListBrandAssets lists assets of a brand, optionally filtered by type.
	ListBrandAssets(ctx context.Context, brandID kernel.ID, assetTypes ...string) ([]BrandAsset, error)
}
