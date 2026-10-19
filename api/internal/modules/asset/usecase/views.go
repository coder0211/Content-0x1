package usecase

import (
	"context"
	"log/slog"
	"slices"

	"github.com/be/acs/api/internal/modules/asset/domain"
	"github.com/be/acs/api/internal/shared/kernel"
)

// FileView is a version file with metadata and an inline preview URL.
type FileView struct {
	VersionFile
	MimeType   string
	Kind       string
	Width      int
	Height     int
	DurationMs int
	URL        *string
}

// VersionDetail is a version for the HTTP layer.
type VersionDetail struct {
	VersionRecord
	CreatedByName string
	FileViews     []FileView
	Provenance    *Provenance
}

// Summary is a library / queue row.
type Summary struct {
	Asset         domain.Asset
	CreatedByName string
	ThumbnailURL  *string
}

// ApprovalView is a review-history row with the actor's name.
type ApprovalView struct {
	Approval
	ReviewerName string
}

// Permissions tells the FE which actions the actor may take.
type Permissions struct {
	CanEdit     bool
	CanSubmit   bool
	CanReview   bool
	CanDownload bool
}

// Detail is the asset detail (FR-AST-02).
type Detail struct {
	Summary
	Current     VersionDetail
	Approvals   []ApprovalView
	Permissions Permissions
}

func (s *Service) names(ctx context.Context, ids []kernel.ID) map[kernel.ID]string {
	out := map[kernel.ID]string{}
	if s.Identity == nil || len(ids) == 0 {
		return out
	}
	slices.SortFunc(ids, func(a, b kernel.ID) int { return slices.Compare(a[:], b[:]) })
	users, err := s.Identity.GetUsers(ctx, slices.Compact(ids))
	if err != nil {
		s.log().WarnContext(ctx, "asset: load user names", slog.Any("err", err))
		return out
	}
	for _, u := range users {
		out[u.ID] = u.Name
	}
	return out
}

func (s *Service) log() *slog.Logger {
	if s.Logger != nil {
		return s.Logger
	}
	return slog.Default()
}

// previewURL presigns an inline URL; failures degrade to nil (logged).
func (s *Service) previewURL(ctx context.Context, fileID kernel.ID) *string {
	if s.Files == nil {
		return nil
	}
	u, err := s.Files.SignedURL(ctx, fileID, "")
	if err != nil || u.URL == "" {
		if err != nil {
			s.log().WarnContext(ctx, "asset: preview url", slog.String("file_id", fileID.String()), slog.Any("err", err))
		}
		return nil
	}
	return &u.URL
}

// thumbnailFile picks the file shown in grids: thumbnail, else preview, else
// an image's primary file.
func thumbnailFile(t domain.Type, files []VersionFile) *kernel.ID {
	for _, role := range []FileRole{RoleThumbnail, RolePreview} {
		for _, f := range files {
			if f.Role == role {
				return &f.FileID
			}
		}
	}
	if t == domain.TypeImage {
		for _, f := range files {
			if f.Role == RolePrimary || f.Role == RoleVariant {
				return &f.FileID
			}
		}
	}
	return nil
}

func (s *Service) summaries(ctx context.Context, assets []domain.Asset) ([]Summary, error) {
	ids := make([]kernel.ID, 0, len(assets))
	var vids []kernel.ID
	for _, a := range assets {
		ids = append(ids, a.CreatedBy)
		if a.CurrentVersionID != nil && a.Type != domain.TypeContent {
			vids = append(vids, *a.CurrentVersionID)
		}
	}
	files := map[kernel.ID][]VersionFile{}
	if len(vids) > 0 {
		var err error
		if files, err = s.Repo.VersionFiles(ctx, vids); err != nil {
			return nil, err
		}
	}
	names := s.names(ctx, ids)
	out := make([]Summary, 0, len(assets))
	for _, a := range assets {
		sm := Summary{Asset: a, CreatedByName: names[a.CreatedBy]}
		if a.CurrentVersionID != nil {
			if fid := thumbnailFile(a.Type, files[*a.CurrentVersionID]); fid != nil {
				sm.ThumbnailURL = s.previewURL(ctx, *fid)
			}
		}
		out = append(out, sm)
	}
	return out, nil
}

func (s *Service) versionDetails(ctx context.Context, vs []VersionRecord) ([]VersionDetail, error) {
	var userIDs, fileIDs, jobIDs []kernel.ID
	for _, v := range vs {
		userIDs = append(userIDs, v.CreatedBy)
		for _, f := range v.Files {
			fileIDs = append(fileIDs, f.FileID)
		}
		if v.GeneratedByJobID != nil {
			jobIDs = append(jobIDs, *v.GeneratedByJobID)
		}
	}
	prov := map[kernel.ID]Provenance{}
	if len(jobIDs) > 0 {
		var err error
		if prov, err = s.Repo.Provenance(ctx, jobIDs); err != nil {
			return nil, err
		}
	}
	meta := map[kernel.ID]FileView{}
	if len(fileIDs) > 0 && s.Files != nil {
		fs, err := s.Files.GetMany(ctx, fileIDs)
		if err != nil {
			return nil, err
		}
		for _, f := range fs {
			meta[f.ID] = FileView{MimeType: f.MimeType, Kind: string(f.Kind), Width: f.Width, Height: f.Height, DurationMs: f.DurationMs}
		}
	}
	names := s.names(ctx, userIDs)
	out := make([]VersionDetail, 0, len(vs))
	for _, v := range vs {
		d := VersionDetail{VersionRecord: v, CreatedByName: names[v.CreatedBy], FileViews: []FileView{}}
		for _, f := range v.Files {
			fv := meta[f.FileID]
			fv.VersionFile = f
			fv.URL = s.previewURL(ctx, f.FileID)
			d.FileViews = append(d.FileViews, fv)
		}
		if v.GeneratedByJobID != nil {
			if p, ok := prov[*v.GeneratedByJobID]; ok {
				d.Provenance = &p
			}
		}
		out = append(out, d)
	}
	return out, nil
}

func (s *Service) permissions(ctx context.Context, actor kernel.Actor, a *domain.Asset) Permissions {
	p := Permissions{}
	p.CanEdit = a.Status != domain.StatusArchived && s.canEdit(ctx, actor, a) == nil
	p.CanSubmit = p.CanEdit && a.Status == domain.StatusDraft
	p.CanReview = actor.Can(kernel.PermAssetReview) && a.Status == domain.StatusPendingReview &&
		(actor.IsAdmin() || (actor.UserID != a.CreatedBy && actor.UserID != a.CurrentVersionBy))
	p.CanDownload = actor.Can(kernel.PermAssetDownload) && (actor.IsLeadOrAdmin() || a.Status == domain.StatusApproved)
	return p
}

func (s *Service) detail(ctx context.Context, actor kernel.Actor, a *domain.Asset) (Detail, error) {
	sums, err := s.summaries(ctx, []domain.Asset{*a})
	if err != nil {
		return Detail{}, err
	}
	d := Detail{Summary: sums[0], Approvals: []ApprovalView{}, Permissions: s.permissions(ctx, actor, a)}
	if a.CurrentVersionID != nil {
		v, err := s.Repo.GetVersionByID(ctx, *a.CurrentVersionID)
		if err != nil {
			return Detail{}, err
		}
		vds, err := s.versionDetails(ctx, []VersionRecord{*v})
		if err != nil {
			return Detail{}, err
		}
		d.Current = vds[0]
	}
	aps, err := s.Repo.ListApprovals(ctx, a.ID)
	if err != nil {
		return Detail{}, err
	}
	ids := make([]kernel.ID, 0, len(aps))
	for _, ap := range aps {
		ids = append(ids, ap.ReviewerID)
	}
	names := s.names(ctx, ids)
	for _, ap := range aps {
		d.Approvals = append(d.Approvals, ApprovalView{Approval: ap, ReviewerName: names[ap.ReviewerID]})
	}
	return d, nil
}
