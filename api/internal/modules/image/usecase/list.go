package usecase

import (
	"context"
	"encoding/json"
	"errors"

	assetuc "github.com/be/acs/api/internal/modules/asset/usecase"
	fileuc "github.com/be/acs/api/internal/modules/file/usecase"
	"github.com/be/acs/api/internal/modules/image/domain"
	jobuc "github.com/be/acs/api/internal/modules/job/usecase"
	"github.com/be/acs/api/internal/shared/kernel"
)

// ListBatches is GET /images/generations: the gallery grouped by request,
// newest first. Without a filter it lists the actor's own batches; with a
// campaign it lists everyone's batches in that campaign (members and
// Lead/Admin only, else NOT_FOUND); with an asset, that asset's history.
func (s *Service) ListBatches(ctx context.Context, actor kernel.Actor, f BatchFilter, p kernel.PageRequest) (kernel.Page[BatchView], error) {
	if err := actor.Require(kernel.PermGenerateRun); err != nil {
		return kernel.Page[BatchView]{}, err
	}
	p = p.Normalized()
	q := BatchQuery{Limit: p.Limit + 1}
	var after kernel.TimeIDCursor
	if ok, err := kernel.DecodeCursor(p.Cursor, &after); err != nil {
		return kernel.Page[BatchView]{}, err
	} else if ok {
		q.After = &after
	}
	switch {
	case f.AssetID != nil:
		if err := s.Assets.CanEdit(ctx, actor, *f.AssetID); err != nil {
			if errors.Is(err, kernel.ErrForbidden) {
				return kernel.Page[BatchView]{}, kernel.ErrNotFound
			}
			return kernel.Page[BatchView]{}, err
		}
		q.AssetID = f.AssetID
	case f.CampaignID != nil:
		if !actor.IsLeadOrAdmin() {
			ok, err := s.Campaigns.IsMember(ctx, *f.CampaignID, actor.UserID)
			if err != nil {
				return kernel.Page[BatchView]{}, err
			}
			if !ok {
				return kernel.Page[BatchView]{}, kernel.ErrNotFound
			}
		}
		q.CampaignID = f.CampaignID
	default:
		q.CreatedBy = &actor.UserID
	}
	rows, err := s.Repo.List(ctx, q)
	if err != nil {
		return kernel.Page[BatchView]{}, err
	}
	page := kernel.PageFromRows(rows, p.Limit, func(b domain.Batch) any { return kernel.TimeIDCursor{At: b.CreatedAt, ID: b.ID} })
	out := kernel.Page[BatchView]{Items: make([]BatchView, 0, len(page.Items)), NextCursor: page.NextCursor}
	for _, b := range page.Items {
		v, err := s.present(ctx, b)
		if err != nil {
			return kernel.Page[BatchView]{}, err
		}
		out.Items = append(out.Items, v)
	}
	return out, nil
}

func (s *Service) present(ctx context.Context, b domain.Batch) (BatchView, error) {
	v := BatchView{
		ID: b.ID, Operation: b.Operation, CampaignID: b.CampaignID, BrandID: b.BrandID, Options: b.Options,
		ModelCode: b.ModelCode, SourceFileID: b.SourceFileID, CreatedBy: b.CreatedBy, CreatedAt: b.CreatedAt,
		Job: BatchJob{ID: b.JobID, Status: string(jobuc.StatusQueued)},
	}
	if j, err := s.Jobs.Get(ctx, b.JobID); err == nil {
		v.Job = BatchJob{ID: j.ID, Status: string(j.Status), Progress: j.Progress, ErrorCode: j.ErrorCode,
			ErrorMessage: j.ErrorMessage, CostEstimate: j.CostEstimate, CostActual: j.CostActual}
		for _, st := range j.Steps {
			if st.ErrorKind != "" {
				v.Job.ErrorKind = st.ErrorKind
			}
		}
	} else if !errors.Is(err, kernel.ErrNotFound) {
		return BatchView{}, err
	}
	if b.AssetID == nil || b.VersionNo == nil {
		return v, nil
	}
	a, err := s.Assets.Get(ctx, *b.AssetID)
	if errors.Is(err, kernel.ErrNotFound) {
		return v, nil // asset deleted: keep the batch, without images
	}
	if err != nil {
		return BatchView{}, err
	}
	ver, err := s.Assets.GetVersion(ctx, a.ID, *b.VersionNo)
	if err != nil {
		return BatchView{}, err
	}
	var c domain.Content
	_ = json.Unmarshal(ver.ContentJSON, &c)
	ba := &BatchAsset{ID: a.ID, Title: a.Title, Status: string(a.Status), VersionNo: ver.VersionNo,
		RequiresTypographyCheck: c.RequiresTypographyCheck || domain.RequiresTypographyCheck(domain.Preset(a.Subtype)),
		Images:                  []ImageFile{}}
	if a.CurrentVersion != nil {
		ba.CurrentVersionNo = a.CurrentVersion.VersionNo
	}
	images, err := s.versionImages(ctx, ver.Files)
	if err != nil {
		return BatchView{}, err
	}
	ba.Images = images
	v.Asset = ba
	return v, nil
}

// versionImages pairs every image with its thumbnail (same sort order) and
// signs short-lived preview URLs.
func (s *Service) versionImages(ctx context.Context, vf []assetuc.VersionFile) ([]ImageFile, error) {
	ids := make([]kernel.ID, 0, len(vf))
	for _, f := range vf {
		ids = append(ids, f.FileID)
	}
	files, err := s.Files.GetMany(ctx, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[kernel.ID]fileuc.File, len(files))
	for _, f := range files {
		byID[f.ID] = f
	}
	thumbs := map[int]kernel.ID{}
	for _, f := range vf {
		if f.Role == assetuc.RoleThumbnail {
			thumbs[f.SortOrder] = f.FileID
		}
	}
	out := []ImageFile{}
	for _, f := range vf {
		if !isImageRole(f.Role) {
			continue
		}
		meta, ok := byID[f.FileID]
		if !ok {
			continue
		}
		u, err := s.Files.SignedURL(ctx, f.FileID, "")
		if err != nil {
			return nil, err
		}
		img := ImageFile{FileID: f.FileID, Role: string(f.Role), SortOrder: f.SortOrder, MimeType: meta.MimeType,
			Width: meta.Width, Height: meta.Height, ParentFileID: meta.ParentFileID, URL: u.URL, URLExpiresAt: u.ExpiresAt,
			ThumbnailURL: u.URL}
		if tid, ok := thumbs[f.SortOrder]; ok {
			if tu, err := s.Files.SignedURL(ctx, tid, ""); err == nil {
				id := tid
				img.ThumbnailFileID, img.ThumbnailURL = &id, tu.URL
			}
		}
		out = append(out, img)
	}
	return out, nil
}
