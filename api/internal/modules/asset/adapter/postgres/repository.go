// Package postgres implements the asset module's Repository with sqlc.
package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/be/acs/api/internal/modules/asset/domain"
	"github.com/be/acs/api/internal/modules/asset/usecase"
	db "github.com/be/acs/api/internal/platform/postgres"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Repository implements usecase.Repository.
type Repository struct{ pool *pgxpool.Pool }

// NewRepository returns a repository on pool.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

var _ usecase.Repository = (*Repository)(nil)

func (r *Repository) q(ctx context.Context) *Queries { return New(db.Conn(ctx, r.pool)) }

func str(s string) *string {
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

func tags(t []string) []string {
	if t == nil {
		return []string{}
	}
	return t
}

func toAsset(a GetAssetRow) domain.Asset {
	out := domain.Asset{
		ID: a.ID, CampaignID: a.CampaignID, BrandID: a.BrandID, Type: domain.Type(a.AssetType), Subtype: deref(a.Subtype),
		Title: a.Title, Status: domain.Status(a.Status), CurrentVersionID: a.CurrentVersionID,
		CurrentVersionNo: int(a.CurrentVersionNo), SourceAssetID: a.SourceAssetID, Tags: tags(a.Tags),
		UsageRights: a.UsageRights, CreatedBy: a.CreatedBy, SubmittedAt: a.SubmittedAt, CreatedAt: a.CreatedAt,
		UpdatedAt: a.UpdatedAt, DeletedAt: a.DeletedAt,
	}
	if a.CurrentVersionBy != nil {
		out.CurrentVersionBy = *a.CurrentVersionBy
	}
	return out
}

// CreateAsset inserts the asset row (without version).
func (r *Repository) CreateAsset(ctx context.Context, a *domain.Asset) error {
	return r.q(ctx).CreateAsset(ctx, CreateAssetParams{
		ID: a.ID, CampaignID: a.CampaignID, BrandID: a.BrandID, AssetType: string(a.Type), Subtype: str(a.Subtype),
		Title: a.Title, Status: string(a.Status), SourceAssetID: a.SourceAssetID, Tags: tags(a.Tags),
		UsageRights: a.UsageRights, CreatedBy: a.CreatedBy, CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt,
	})
}

// NextVersionNo returns max(version_no)+1.
func (r *Repository) NextVersionNo(ctx context.Context, assetID kernel.ID) (int, error) {
	n, err := r.q(ctx).NextVersionNo(ctx, assetID)
	return int(n), err
}

// InsertVersion inserts the version and its files.
func (r *Repository) InsertVersion(ctx context.Context, v *usecase.VersionRecord) error {
	if err := r.q(ctx).InsertVersion(ctx, InsertVersionParams{
		ID: v.ID, AssetID: v.AssetID, VersionNo: int32(v.VersionNo), ContentJson: v.ContentJSON, ChangeNote: v.ChangeNote,
		GeneratedByJobID: v.GeneratedByJobID, CreatedBy: v.CreatedBy, CreatedAt: v.CreatedAt,
	}); err != nil {
		return err
	}
	return r.AttachFiles(ctx, v.ID, v.Files)
}

// Get loads a live asset.
func (r *Repository) Get(ctx context.Context, id kernel.ID) (*domain.Asset, error) {
	row, err := r.q(ctx).GetAsset(ctx, id)
	if err != nil {
		return nil, db.MapNotFound(err)
	}
	a := toAsset(row)
	return &a, nil
}

// GetForUpdate loads and row-locks a live asset.
func (r *Repository) GetForUpdate(ctx context.Context, id kernel.ID) (*domain.Asset, error) {
	row, err := r.q(ctx).LockAsset(ctx, id)
	if err != nil {
		return nil, db.MapNotFound(err)
	}
	a := toAsset(GetAssetRow(row))
	return &a, nil
}

// Save writes the mutable columns.
func (r *Repository) Save(ctx context.Context, a *domain.Asset) error {
	return r.q(ctx).SaveAsset(ctx, SaveAssetParams{
		ID: a.ID, Title: a.Title, Tags: tags(a.Tags), UsageRights: a.UsageRights, Status: string(a.Status),
		SubmittedAt: a.SubmittedAt, CurrentVersionID: a.CurrentVersionID, DeletedAt: a.DeletedAt,
	})
}

// SetSearch recomputes search_tsv.
func (r *Repository) SetSearch(ctx context.Context, assetID kernel.ID, body string) error {
	return r.q(ctx).SetSearch(ctx, SetSearchParams{ID: assetID, Body: body})
}

// RefreshSearchTitle recomputes title/tag weights.
func (r *Repository) RefreshSearchTitle(ctx context.Context, assetID kernel.ID) error {
	return r.q(ctx).RefreshSearchTitle(ctx, assetID)
}

func toVersion(v AssetVersion) usecase.VersionRecord {
	return usecase.VersionRecord{
		ID: v.ID, AssetID: v.AssetID, VersionNo: int(v.VersionNo), ContentJSON: v.ContentJson, ChangeNote: v.ChangeNote,
		GeneratedByJobID: v.GeneratedByJobID, CreatedBy: v.CreatedBy, CreatedAt: v.CreatedAt, Files: []usecase.VersionFile{},
	}
}

func (r *Repository) withFiles(ctx context.Context, vs []usecase.VersionRecord) error {
	ids := make([]kernel.ID, 0, len(vs))
	for _, v := range vs {
		ids = append(ids, v.ID)
	}
	files, err := r.VersionFiles(ctx, ids)
	if err != nil {
		return err
	}
	for i := range vs {
		if fs, ok := files[vs[i].ID]; ok {
			vs[i].Files = fs
		}
	}
	return nil
}

func (r *Repository) one(ctx context.Context, v AssetVersion, err error) (*usecase.VersionRecord, error) {
	if err != nil {
		return nil, db.MapNotFound(err)
	}
	vs := []usecase.VersionRecord{toVersion(v)}
	if err := r.withFiles(ctx, vs); err != nil {
		return nil, err
	}
	return &vs[0], nil
}

// GetVersion loads a version by number.
func (r *Repository) GetVersion(ctx context.Context, assetID kernel.ID, versionNo int) (*usecase.VersionRecord, error) {
	v, err := r.q(ctx).GetVersion(ctx, GetVersionParams{AssetID: assetID, VersionNo: int32(versionNo)})
	return r.one(ctx, v, err)
}

// GetVersionByID loads a version by ID.
func (r *Repository) GetVersionByID(ctx context.Context, versionID kernel.ID) (*usecase.VersionRecord, error) {
	v, err := r.q(ctx).GetVersionByID(ctx, versionID)
	return r.one(ctx, v, err)
}

// ListVersions returns all versions, newest first.
func (r *Repository) ListVersions(ctx context.Context, assetID kernel.ID) ([]usecase.VersionRecord, error) {
	rows, err := r.q(ctx).ListVersions(ctx, assetID)
	if err != nil {
		return nil, err
	}
	out := make([]usecase.VersionRecord, 0, len(rows))
	for _, v := range rows {
		out = append(out, toVersion(v))
	}
	return out, r.withFiles(ctx, out)
}

// AttachFiles upserts version files.
func (r *Repository) AttachFiles(ctx context.Context, versionID kernel.ID, files []usecase.VersionFile) error {
	q := r.q(ctx)
	for _, f := range files {
		if err := q.InsertVersionFile(ctx, InsertVersionFileParams{
			VersionID: versionID, FileID: f.FileID, Role: string(f.Role), SortOrder: int32(f.SortOrder),
		}); err != nil {
			return err
		}
	}
	return nil
}

// VersionFiles returns the files of several versions.
func (r *Repository) VersionFiles(ctx context.Context, versionIDs []kernel.ID) (map[kernel.ID][]usecase.VersionFile, error) {
	out := map[kernel.ID][]usecase.VersionFile{}
	if len(versionIDs) == 0 {
		return out, nil
	}
	rows, err := r.q(ctx).VersionFiles(ctx, versionIDs)
	if err != nil {
		return nil, err
	}
	for _, f := range rows {
		out[f.VersionID] = append(out[f.VersionID], usecase.VersionFile{FileID: f.FileID, Role: usecase.FileRole(f.Role), SortOrder: int(f.SortOrder)})
	}
	return out, nil
}

// AddApproval appends a review-history row.
func (r *Repository) AddApproval(ctx context.Context, a usecase.Approval) error {
	return r.q(ctx).AddApproval(ctx, AddApprovalParams{
		ID: a.ID, AssetID: a.AssetID, VersionID: a.VersionID, ReviewerID: a.ReviewerID, Decision: string(a.Decision),
		Comment: a.Comment, Checklist: a.Checklist, CreatedAt: a.CreatedAt,
	})
}

// ListApprovals returns the review history, newest first.
func (r *Repository) ListApprovals(ctx context.Context, assetID kernel.ID) ([]usecase.Approval, error) {
	rows, err := r.q(ctx).ListApprovals(ctx, assetID)
	if err != nil {
		return nil, err
	}
	out := make([]usecase.Approval, 0, len(rows))
	for _, a := range rows {
		out = append(out, usecase.Approval{
			ID: a.ID, AssetID: a.AssetID, VersionID: a.VersionID, VersionNo: int(a.VersionNo), ReviewerID: a.ReviewerID,
			Decision: usecase.Decision(a.Decision), Comment: a.Comment, Checklist: a.Checklist, CreatedAt: a.CreatedAt,
		})
	}
	return out, nil
}

func cursorArgs(c *kernel.TimeIDCursor) (*time.Time, *kernel.ID) {
	if c == nil {
		return nil, nil
	}
	at, id := c.At, c.ID
	return &at, &id
}

func typeArg(t *domain.Type) *string {
	if t == nil {
		return nil
	}
	s := string(*t)
	return &s
}

// Search lists live assets newest first.
func (r *Repository) Search(ctx context.Context, f usecase.SearchFilter, after *kernel.TimeIDCursor, limit int) ([]domain.Asset, error) {
	statuses := make([]string, 0, len(f.Statuses))
	for _, s := range f.Statuses {
		statuses = append(statuses, string(s))
	}
	afterAt, afterID := cursorArgs(after)
	rows, err := r.q(ctx).SearchAssets(ctx, SearchAssetsParams{
		Query: f.Query, AssetType: typeArg(f.Type), Statuses: statuses, CampaignID: f.CampaignID, BrandID: f.BrandID,
		CreatedBy: f.CreatedBy, Tag: f.Tag, FromAt: f.From, ToAt: f.To, AfterAt: afterAt, AfterID: afterID, Lim: int32(limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.Asset, 0, len(rows))
	for _, row := range rows {
		out = append(out, toAsset(GetAssetRow(row)))
	}
	return out, nil
}

// Queue lists pending_review assets, oldest submission first.
func (r *Repository) Queue(ctx context.Context, f usecase.SearchFilter, after *kernel.TimeIDCursor, limit int) ([]domain.Asset, error) {
	afterAt, afterID := cursorArgs(after)
	rows, err := r.q(ctx).ReviewQueue(ctx, ReviewQueueParams{
		AssetType: typeArg(f.Type), CampaignID: f.CampaignID, AfterAt: afterAt, AfterID: afterID, Lim: int32(limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.Asset, 0, len(rows))
	for _, row := range rows {
		out = append(out, toAsset(GetAssetRow(row)))
	}
	return out, nil
}

// CountByStatus counts live assets per status (optionally one creator's).
func (r *Repository) CountByStatus(ctx context.Context, createdBy *kernel.ID) (map[domain.Status]int, error) {
	rows, err := r.q(ctx).CountAssets(ctx, createdBy)
	if err != nil {
		return nil, err
	}
	out := map[domain.Status]int{}
	for _, row := range rows {
		out[domain.Status(row.Status)] = int(row.N)
	}
	return out, nil
}

// Provenance reads the generating jobs.
func (r *Repository) Provenance(ctx context.Context, jobIDs []kernel.ID) (map[kernel.ID]usecase.Provenance, error) {
	rows, err := r.q(ctx).Provenance(ctx, jobIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[kernel.ID]usecase.Provenance, len(rows))
	for _, p := range rows {
		pv := usecase.Provenance{
			JobID: p.JobID, JobType: p.JobType, ProviderCode: p.ProviderCode, ModelCode: p.ModelCode,
			PromptTemplateCode: p.PromptTemplateCode, BrandVersionID: p.BrandVersionID,
		}
		if p.PromptTemplateVersion != nil {
			n := int(*p.PromptTemplateVersion)
			pv.PromptTemplateVersion = &n
		}
		if p.BrandVersionNo != nil {
			n := int(*p.BrandVersionNo)
			pv.BrandVersionNo = &n
		}
		out[p.JobID] = pv
	}
	return out, nil
}
