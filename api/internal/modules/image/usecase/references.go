package usecase

import (
	"context"
	"slices"
	"time"

	branduc "github.com/be/acs/api/internal/modules/brand/usecase"

	"github.com/be/acs/api/internal/modules/image/domain"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Reference sources shown by the Image Studio reference picker.
const (
	RefSourceBrand   = "brand"
	RefSourceLibrary = "library"
)

// maxLibraryBatches bounds how many recent batches feed the "library" tab.
const maxLibraryBatches = 12

// ReferenceFilter is GET /images/references: exactly one of CampaignID or
// BrandID (campaign wins).
type ReferenceFilter struct {
	CampaignID *kernel.ID
	BrandID    *kernel.ID
}

// ReferenceCandidate is an image the actor may pick as a reference.
type ReferenceCandidate struct {
	FileID       kernel.ID
	Source       string
	Label        string
	MimeType     string
	Width        int
	Height       int
	URL          string
	ThumbnailURL string
	URLExpiresAt time.Time
}

// ListReferences returns the brand's logo/reference images plus recent images
// generated in the campaign (or by the actor when there is no campaign), with
// signed preview URLs. Only raster types providers accept are listed.
func (s *Service) ListReferences(ctx context.Context, actor kernel.Actor, f ReferenceFilter) ([]ReferenceCandidate, error) {
	if err := actor.Require(kernel.PermGenerateRun); err != nil {
		return nil, err
	}
	var brandID kernel.ID
	q := BatchQuery{Limit: maxLibraryBatches}
	switch {
	case f.CampaignID != nil:
		c, err := s.Campaigns.GetCampaignForGeneration(ctx, actor, *f.CampaignID)
		if err != nil {
			return nil, err
		}
		if !actor.IsLeadOrAdmin() {
			ok, err := s.Campaigns.IsMember(ctx, *f.CampaignID, actor.UserID)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, kernel.ErrNotFound
			}
		}
		brandID, q.CampaignID = c.BrandID, f.CampaignID
	case f.BrandID != nil:
		if _, err := s.Brands.GetBrandContext(ctx, branduc.BrandContextRequest{BrandID: *f.BrandID}); err != nil {
			return nil, err
		}
		brandID, q.CreatedBy = *f.BrandID, &actor.UserID
	default:
		return nil, kernel.Validation(kernel.FieldIssue("campaign_id", "required"))
	}

	out := []ReferenceCandidate{}
	seen := map[kernel.ID]bool{}
	assets, err := s.Brands.ListBrandAssets(ctx, brandID, "logo", "reference_image")
	if err != nil {
		return nil, err
	}
	ids := make([]kernel.ID, 0, len(assets))
	labels := map[kernel.ID]string{}
	for _, a := range assets {
		ids = append(ids, a.FileID)
		labels[a.FileID] = a.Label
	}
	files, err := s.Files.GetMany(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, fl := range files {
		if fl.UploadStatus != "ready" || !slices.Contains(domain.AllowedReferenceMIME, fl.MimeType) || seen[fl.ID] {
			continue
		}
		u, err := s.Files.SignedURL(ctx, fl.ID, "")
		if err != nil {
			return nil, err
		}
		seen[fl.ID] = true
		out = append(out, ReferenceCandidate{FileID: fl.ID, Source: RefSourceBrand, Label: labels[fl.ID], MimeType: fl.MimeType,
			Width: fl.Width, Height: fl.Height, URL: u.URL, ThumbnailURL: u.URL, URLExpiresAt: u.ExpiresAt})
	}

	batches, err := s.Repo.List(ctx, q)
	if err != nil {
		return nil, err
	}
	for _, b := range batches {
		if b.AssetID == nil || b.VersionNo == nil {
			continue
		}
		ver, err := s.Assets.GetVersion(ctx, *b.AssetID, *b.VersionNo)
		if err != nil {
			continue // asset deleted or not visible any more
		}
		imgs, err := s.versionImages(ctx, ver.Files)
		if err != nil {
			return nil, err
		}
		for _, im := range imgs {
			if seen[im.FileID] || !slices.Contains(domain.AllowedReferenceMIME, im.MimeType) {
				continue
			}
			seen[im.FileID] = true
			out = append(out, ReferenceCandidate{FileID: im.FileID, Source: RefSourceLibrary, Label: domain.Title(b.Options.Prompt, b.Options.Preset),
				MimeType: im.MimeType, Width: im.Width, Height: im.Height, URL: im.URL, ThumbnailURL: im.ThumbnailURL, URLExpiresAt: im.URLExpiresAt})
		}
	}
	return out, nil
}
