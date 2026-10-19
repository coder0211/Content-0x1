package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/be/acs/api/internal/modules/asset/domain"
	"github.com/be/acs/api/internal/shared/kernel"
)

// SearchInput is the library query (ACS-082).
type SearchInput struct {
	Q          string
	Filter     SearchFilter // Query is derived from Q
	PageCursor kernel.PageRequest
}

// MaxSearchQueryLen bounds q.
const MaxSearchQueryLen = 200

// Search lists live assets (brand.view), newest first.
func (s *Service) Search(ctx context.Context, actor kernel.Actor, in SearchInput) (kernel.Page[Summary], error) {
	if err := actor.Require(kernel.PermBrandView); err != nil {
		return kernel.Page[Summary]{}, err
	}
	if utf8.RuneCountInString(in.Q) > MaxSearchQueryLen {
		return kernel.Page[Summary]{}, kernel.Validation(kernel.FieldIssue("q", "too_long"))
	}
	f := in.Filter
	f.Query = domain.PrefixQuery(in.Q)
	return s.page(ctx, in.PageCursor, func(after *kernel.TimeIDCursor, limit int) ([]domain.Asset, error) {
		return s.Repo.Search(ctx, f, after, limit)
	}, func(a domain.Asset) kernel.TimeIDCursor { return kernel.TimeIDCursor{At: a.CreatedAt, ID: a.ID} })
}

// Queue lists pending_review assets, oldest submission first (asset.review).
func (s *Service) Queue(ctx context.Context, actor kernel.Actor, f SearchFilter, p kernel.PageRequest) (kernel.Page[Summary], error) {
	if err := actor.Require(kernel.PermAssetReview); err != nil {
		return kernel.Page[Summary]{}, err
	}
	f.Query = ""
	f.Statuses = []domain.Status{domain.StatusPendingReview}
	return s.page(ctx, p, func(after *kernel.TimeIDCursor, limit int) ([]domain.Asset, error) {
		return s.Repo.Queue(ctx, f, after, limit)
	}, func(a domain.Asset) kernel.TimeIDCursor {
		at := a.UpdatedAt
		if a.SubmittedAt != nil {
			at = *a.SubmittedAt
		}
		return kernel.TimeIDCursor{At: at, ID: a.ID}
	})
}

func (s *Service) page(ctx context.Context, p kernel.PageRequest,
	fetch func(*kernel.TimeIDCursor, int) ([]domain.Asset, error), key func(domain.Asset) kernel.TimeIDCursor,
) (kernel.Page[Summary], error) {
	p = p.Normalized()
	var cur kernel.TimeIDCursor
	ok, err := kernel.DecodeCursor(p.Cursor, &cur)
	if err != nil {
		return kernel.Page[Summary]{}, err
	}
	var after *kernel.TimeIDCursor
	if ok {
		after = &cur
	}
	rows, err := fetch(after, p.Limit+1)
	if err != nil {
		return kernel.Page[Summary]{}, err
	}
	pg := kernel.PageFromRows(rows, p.Limit, func(a domain.Asset) any { return key(a) })
	items, err := s.summaries(ctx, pg.Items)
	if err != nil {
		return kernel.Page[Summary]{}, err
	}
	return kernel.Page[Summary]{Items: items, NextCursor: pg.NextCursor}, nil
}

// visible loads an asset the actor may see (brand.view), else NOT_FOUND.
func (s *Service) visible(ctx context.Context, actor kernel.Actor, id kernel.ID) (*domain.Asset, error) {
	if err := actor.Require(kernel.PermBrandView); err != nil {
		if kernel.ErrorCode(err) == kernel.CodeForbidden {
			return nil, kernel.ErrNotFound
		}
		return nil, err
	}
	return s.Repo.Get(ctx, id)
}

// Detail returns the asset with current version, provenance and history.
func (s *Service) Detail(ctx context.Context, actor kernel.Actor, id kernel.ID) (Detail, error) {
	a, err := s.visible(ctx, actor, id)
	if err != nil {
		return Detail{}, err
	}
	return s.detail(ctx, actor, a)
}

// ListVersions returns every version, newest first.
func (s *Service) ListVersions(ctx context.Context, actor kernel.Actor, id kernel.ID) ([]VersionDetail, error) {
	if _, err := s.visible(ctx, actor, id); err != nil {
		return nil, err
	}
	vs, err := s.Repo.ListVersions(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.versionDetails(ctx, vs)
}

// UpdateInput is PATCH /assets/{id}; nil fields are unchanged.
// UsageRightsSet distinguishes "clear" (set, nil) from "unchanged".
type UpdateInput struct {
	Title          *string
	Tags           *[]string
	UsageRights    *string
	UsageRightsSet bool
}

type auditView struct {
	Title       string   `json:"title"`
	Tags        []string `json:"tags"`
	UsageRights *string  `json:"usage_rights"`
	Status      string   `json:"status"`
	VersionNo   int      `json:"version_no"`
}

func snapshot(a *domain.Asset) auditView {
	return auditView{Title: a.Title, Tags: a.Tags, UsageRights: a.UsageRights, Status: string(a.Status), VersionNo: a.CurrentVersionNo}
}

// Update edits title, tags and usage rights (ACS-085), audited.
func (s *Service) Update(ctx context.Context, actor kernel.Actor, id kernel.ID, in UpdateInput) (Detail, error) {
	var v kernel.ValidationErrors
	var title string
	var tags []string
	var rights *string
	var err error
	if in.Title != nil {
		if title, err = domain.NormalizeTitle(*in.Title); err != nil {
			v.Add("title", "invalid")
		}
	}
	if in.Tags != nil {
		if tags, err = domain.NormalizeTags(*in.Tags); err != nil {
			v.Add("tags", "invalid")
		}
	}
	if in.UsageRightsSet {
		if rights, err = domain.NormalizeUsageRights(in.UsageRights); err != nil {
			v.Add("usage_rights", "too_long")
		}
	}
	if err := v.Err(); err != nil {
		return Detail{}, err
	}
	var out *domain.Asset
	err = s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		a, err := s.lockEditable(ctx, actor, id)
		if err != nil {
			return err
		}
		before := snapshot(a)
		if in.Title != nil {
			a.Title = title
		}
		if in.Tags != nil {
			a.Tags = tags
		}
		if in.UsageRightsSet {
			a.UsageRights = rights
		}
		a.UpdatedAt = s.Clock.Now()
		if err := s.Repo.Save(ctx, a); err != nil {
			return err
		}
		if err := s.Repo.RefreshSearchTitle(ctx, a.ID); err != nil {
			return err
		}
		out = a
		return s.Audit.Record(ctx, "asset.update", "asset", a.ID.String(), before, snapshot(a))
	})
	if err != nil {
		return Detail{}, err
	}
	return s.detail(ctx, actor, out)
}

// lockEditable locks a live asset the actor may edit. Archived assets are
// read-only (INVALID_STATE_TRANSITION).
func (s *Service) lockEditable(ctx context.Context, actor kernel.Actor, id kernel.ID) (*domain.Asset, error) {
	if !actor.Can(kernel.PermBrandView) {
		return nil, kernel.ErrNotFound
	}
	a, err := s.Repo.GetForUpdate(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.canEdit(ctx, actor, a); err != nil {
		return nil, err
	}
	return a, nil
}

// Delete soft-deletes the asset (audited).
func (s *Service) Delete(ctx context.Context, actor kernel.Actor, id kernel.ID) error {
	return s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		a, err := s.lockEditable(ctx, actor, id)
		if err != nil {
			return err
		}
		now := s.Clock.Now()
		a.DeletedAt = &now
		if err := s.Repo.Save(ctx, a); err != nil {
			return err
		}
		return s.Audit.Record(ctx, "asset.delete", "asset", a.ID.String(), snapshot(a), nil)
	})
}

// ManualVersionInput is POST /assets/{id}/versions.
type ManualVersionInput struct {
	ContentJSON   json.RawMessage
	ChangeNote    string
	BaseVersionNo int
}

// CreateVersion appends a manual revision with optimistic concurrency
// (base_version_no ≠ current → 409 VERSION_CONFLICT). Files of the current
// version are carried over (a text edit does not drop the image/video).
func (s *Service) CreateVersion(ctx context.Context, actor kernel.Actor, id kernel.ID, in ManualVersionInput) (VersionDetail, error) {
	var v kernel.ValidationErrors
	if !validObject(in.ContentJSON) {
		v.Add("content_json", "invalid")
	}
	if in.BaseVersionNo < 1 {
		v.Add("base_version_no", "required")
	}
	note := strings.TrimSpace(in.ChangeNote)
	if utf8.RuneCountInString(note) > domain.MaxChangeNoteLen {
		v.Add("change_note", "too_long")
	}
	if err := v.Err(); err != nil {
		return VersionDetail{}, err
	}
	return s.newVersionTx(ctx, actor, id, "asset.version_create", func(a *domain.Asset, cur *VersionRecord) (NewVersion, error) {
		if a.CurrentVersionNo != in.BaseVersionNo {
			return NewVersion{}, kernel.ErrVersionConflict.WithDetails(kernel.Detail{
				"field": "base_version_no", "current_version_no": a.CurrentVersionNo,
			})
		}
		nv := NewVersion{ContentJSON: in.ContentJSON, ChangeNote: note, CreatedBy: actor.UserID}
		if cur != nil {
			nv.Files = cur.Files
		}
		return nv, nil
	})
}

// Restore creates version N+1 with the content and files of versionNo.
func (s *Service) Restore(ctx context.Context, actor kernel.Actor, id kernel.ID, versionNo int) (VersionDetail, error) {
	return s.newVersionTx(ctx, actor, id, "asset.version_restore", func(a *domain.Asset, _ *VersionRecord) (NewVersion, error) {
		src, err := s.Repo.GetVersion(ctx, a.ID, versionNo)
		if err != nil {
			return NewVersion{}, err
		}
		return NewVersion{
			ContentJSON: src.ContentJSON, ChangeNote: fmt.Sprintf("Khôi phục từ phiên bản %d", versionNo),
			CreatedBy: actor.UserID, Files: src.Files,
		}, nil
	})
}

func (s *Service) newVersionTx(ctx context.Context, actor kernel.Actor, id kernel.ID, action string,
	build func(a *domain.Asset, cur *VersionRecord) (NewVersion, error),
) (VersionDetail, error) {
	var vr *VersionRecord
	err := s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		a, err := s.lockEditable(ctx, actor, id)
		if err != nil {
			return err
		}
		var cur *VersionRecord
		if a.CurrentVersionID != nil {
			if cur, err = s.Repo.GetVersionByID(ctx, *a.CurrentVersionID); err != nil {
				return err
			}
		}
		nv, err := build(a, cur)
		if err != nil {
			return err
		}
		before := snapshot(a)
		if vr, err = s.appendVersion(ctx, a, nv); err != nil {
			return err
		}
		return s.Audit.Record(ctx, action, "asset", a.ID.String(), before, snapshot(a))
	})
	if err != nil {
		return VersionDetail{}, err
	}
	ds, err := s.versionDetails(ctx, []VersionRecord{*vr})
	if err != nil {
		return VersionDetail{}, err
	}
	return ds[0], nil
}
