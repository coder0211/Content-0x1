package usecase

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/be/acs/api/internal/modules/asset/domain"
	campaignuc "github.com/be/acs/api/internal/modules/campaign/usecase"
	fileuc "github.com/be/acs/api/internal/modules/file/usecase"
	identityuc "github.com/be/acs/api/internal/modules/identity/usecase"
	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Service implements API (for other modules) and the actor-facing use cases
// of the asset module (ACS-034, 080, 082, 084, 085).
type Service struct {
	Tx        ports.TxManager
	Repo      Repository
	Clock     ports.Clock
	Audit     ports.AuditRecorder
	Notifier  ports.Notifier
	Files     FileReader
	Identity  identityuc.API
	Campaigns campaignuc.API
	Logger    *slog.Logger
}

var _ API = (*Service)(nil)

// FileReader is the subset of the file module's API the asset module uses
// (fileuc.API satisfies it).
type FileReader interface {
	Get(ctx context.Context, id kernel.ID) (fileuc.File, error)
	GetMany(ctx context.Context, ids []kernel.ID) ([]fileuc.File, error)
	SignedURL(ctx context.Context, id kernel.ID, downloadName string) (fileuc.SignedURL, error)
}

var _ FileReader = fileuc.API(nil)

func validObject(raw json.RawMessage) bool {
	var m map[string]any
	return len(raw) > 0 && json.Unmarshal(raw, &m) == nil && m != nil
}

func (s *Service) searchBody(title string, v NewVersion) string {
	if v.SearchText != "" {
		return v.SearchText
	}
	return domain.SearchText(v.ContentJSON)
}

// CreateAsset implements API.
func (s *Service) CreateAsset(ctx context.Context, in CreateAssetInput) (AssetRef, error) {
	var v kernel.ValidationErrors
	title, err := domain.NormalizeTitle(in.Title)
	if err != nil {
		v.Add("title", "required")
	}
	if !domain.Type(in.Type).Valid() {
		v.Add("type", "invalid")
	}
	if in.BrandID == kernel.NilID {
		v.Add("brand_id", "required")
	}
	if in.CreatedBy == kernel.NilID {
		v.Add("created_by", "required")
	}
	if !validObject(in.Version.ContentJSON) {
		v.Add("content_json", "invalid")
	}
	tags, terr := domain.NormalizeTags(in.Tags)
	if terr != nil {
		v.Add("tags", "invalid")
	}
	if err := v.Err(); err != nil {
		return AssetRef{}, err
	}
	now := s.Clock.Now()
	a := &domain.Asset{
		ID: kernel.NewID(), CampaignID: in.CampaignID, BrandID: in.BrandID, Type: domain.Type(in.Type), Subtype: in.Subtype,
		Title: title, Status: domain.StatusDraft, SourceAssetID: in.SourceAssetID, Tags: tags, CreatedBy: in.CreatedBy,
		CreatedAt: now, UpdatedAt: now,
	}
	var ref AssetRef
	err = s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.Repo.CreateAsset(ctx, a); err != nil {
			return err
		}
		vr, err := s.insertVersion(ctx, a, 1, in.Version)
		if err != nil {
			return err
		}
		ref = AssetRef{AssetID: a.ID, VersionID: vr.ID, VersionNo: 1}
		return s.Repo.SetSearch(ctx, a.ID, s.searchBody(title, in.Version))
	})
	return ref, err
}

// insertVersion stores version `no` and makes it current (saving a).
func (s *Service) insertVersion(ctx context.Context, a *domain.Asset, no int, nv NewVersion) (*VersionRecord, error) {
	createdBy := nv.CreatedBy
	if createdBy == kernel.NilID {
		createdBy = a.CreatedBy
	}
	vr := &VersionRecord{
		ID: kernel.NewID(), AssetID: a.ID, VersionNo: no, ContentJSON: nv.ContentJSON,
		GeneratedByJobID: nv.GeneratedByJobID, CreatedBy: createdBy, CreatedAt: s.Clock.Now(), Files: nv.Files,
	}
	if nv.ChangeNote != "" {
		n := nv.ChangeNote
		vr.ChangeNote = &n
	}
	if err := s.Repo.InsertVersion(ctx, vr); err != nil {
		return nil, err
	}
	a.CurrentVersionID, a.CurrentVersionNo, a.CurrentVersionBy = &vr.ID, no, createdBy
	if err := s.Repo.Save(ctx, a); err != nil {
		return nil, err
	}
	return vr, nil
}

// appendVersion locks the asset, applies new_version (→ draft) and stores
// version N+1. Caller holds a transaction.
func (s *Service) appendVersion(ctx context.Context, a *domain.Asset, nv NewVersion) (*VersionRecord, error) {
	if !validObject(nv.ContentJSON) {
		return nil, kernel.Validation(kernel.FieldIssue("content_json", "invalid"))
	}
	if err := a.OnNewVersion(); err != nil {
		return nil, err
	}
	no, err := s.Repo.NextVersionNo(ctx, a.ID)
	if err != nil {
		return nil, err
	}
	vr, err := s.insertVersion(ctx, a, no, nv)
	if err != nil {
		return nil, err
	}
	return vr, s.Repo.SetSearch(ctx, a.ID, s.searchBody(a.Title, nv))
}

// AddVersion implements API.
func (s *Service) AddVersion(ctx context.Context, assetID kernel.ID, nv NewVersion) (AssetRef, error) {
	var ref AssetRef
	err := s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		a, err := s.Repo.GetForUpdate(ctx, assetID)
		if err != nil {
			return err
		}
		vr, err := s.appendVersion(ctx, a, nv)
		if err != nil {
			return err
		}
		ref = AssetRef{AssetID: a.ID, VersionID: vr.ID, VersionNo: vr.VersionNo}
		return nil
	})
	return ref, err
}

// AttachFiles implements API.
func (s *Service) AttachFiles(ctx context.Context, versionID kernel.ID, files []VersionFile) error {
	if len(files) == 0 {
		return nil
	}
	return s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		if _, err := s.Repo.GetVersionByID(ctx, versionID); err != nil {
			return err
		}
		return s.Repo.AttachFiles(ctx, versionID, files)
	})
}

func toVersionView(v *VersionRecord) VersionView {
	vv := VersionView{ID: v.ID, VersionNo: v.VersionNo, ContentJSON: v.ContentJSON, GeneratedByJobID: v.GeneratedByJobID,
		CreatedBy: v.CreatedBy, CreatedAt: v.CreatedAt, Files: v.Files}
	if v.ChangeNote != nil {
		vv.ChangeNote = *v.ChangeNote
	}
	return vv
}

// Get implements API.
func (s *Service) Get(ctx context.Context, assetID kernel.ID) (AssetView, error) {
	a, err := s.Repo.Get(ctx, assetID)
	if err != nil {
		return AssetView{}, err
	}
	out := AssetView{ID: a.ID, CampaignID: a.CampaignID, BrandID: a.BrandID, Type: AssetType(a.Type), Subtype: a.Subtype,
		Title: a.Title, Status: Status(a.Status), SourceAssetID: a.SourceAssetID, Tags: a.Tags, CreatedBy: a.CreatedBy,
		CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt, UsageRights: a.UsageRights, SubmittedAt: a.SubmittedAt}
	if a.CurrentVersionID != nil {
		v, err := s.Repo.GetVersionByID(ctx, *a.CurrentVersionID)
		if err != nil {
			return AssetView{}, err
		}
		vv := toVersionView(v)
		out.CurrentVersion = &vv
	}
	return out, nil
}

// GetVersion implements API.
func (s *Service) GetVersion(ctx context.Context, assetID kernel.ID, versionNo int) (VersionView, error) {
	if _, err := s.Repo.Get(ctx, assetID); err != nil {
		return VersionView{}, err
	}
	v, err := s.Repo.GetVersion(ctx, assetID, versionNo)
	if err != nil {
		return VersionView{}, err
	}
	return toVersionView(v), nil
}

// CanEdit implements API.
func (s *Service) CanEdit(ctx context.Context, actor kernel.Actor, assetID kernel.ID) error {
	a, err := s.Repo.Get(ctx, assetID)
	if err != nil {
		return err
	}
	return s.canEdit(ctx, actor, a)
}

// canEdit: owner, Lead/Admin, or a member of the asset's campaign who may
// generate content (SPEC-01 §2). Actors without brand.view do not see assets.
func (s *Service) canEdit(ctx context.Context, actor kernel.Actor, a *domain.Asset) error {
	if !actor.IsAuthenticated() {
		return kernel.ErrUnauthenticated
	}
	if !actor.Can(kernel.PermBrandView) {
		return kernel.ErrNotFound
	}
	if actor.CanModifyOwned(a.CreatedBy) {
		return nil
	}
	if a.CampaignID != nil && actor.Can(kernel.PermGenerateRun) && s.Campaigns != nil {
		ok, err := s.Campaigns.IsMember(ctx, *a.CampaignID, actor.UserID)
		if err != nil && !kernelNotFound(err) {
			return err
		}
		if ok {
			return nil
		}
	}
	return kernel.ErrForbidden.WithMessage("Bạn chỉ có thể sửa asset do mình tạo hoặc thuộc campaign được giao")
}

// CountByStatus implements API.
func (s *Service) CountByStatus(ctx context.Context, createdBy *kernel.ID) (map[Status]int, error) {
	m, err := s.Repo.CountByStatus(ctx, createdBy)
	if err != nil {
		return nil, err
	}
	out := make(map[Status]int, len(m))
	for k, v := range m {
		out[Status(k)] = v
	}
	return out, nil
}

func kernelNotFound(err error) bool { return kernel.ErrorCode(err) == kernel.CodeNotFound }
