package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/be/acs/api/internal/modules/image/domain"
	"github.com/be/acs/api/internal/modules/image/usecase"
	db "github.com/be/acs/api/internal/platform/postgres"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Repository implements usecase.BatchRepository on image_batches.
type Repository struct{ pool *pgxpool.Pool }

// NewRepository returns a repository on pool.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

var _ usecase.BatchRepository = (*Repository)(nil)

func (r *Repository) q(ctx context.Context) *Queries { return New(db.Conn(ctx, r.pool)) }

func i32p(p *int) *int32 {
	if p == nil {
		return nil
	}
	v := int32(*p)
	return &v
}

func toDomain(b ImageBatch) domain.Batch {
	var vn *int
	if b.VersionNo != nil {
		v := int(*b.VersionNo)
		vn = &v
	}
	return domain.Batch{
		ID: b.ID, JobID: b.JobID, Operation: domain.Operation(b.Operation), CampaignID: b.CampaignID, BrandID: b.BrandID,
		AssetID: b.AssetID, VersionNo: vn, SourceFileID: b.SourceFileID, ModelCode: b.ModelCode, CreatedBy: b.CreatedBy,
		CreatedAt: b.CreatedAt.UTC(),
		Options: domain.Options{Prompt: b.Prompt, Preset: domain.Preset(b.Preset), AspectRatio: b.AspectRatio,
			Quality: b.Quality, Count: int(b.ImageCount)},
	}
}

func (r *Repository) Insert(ctx context.Context, b *domain.Batch) error {
	return r.q(ctx).InsertBatch(ctx, InsertBatchParams{
		ID: b.ID, JobID: b.JobID, Operation: string(b.Operation), CampaignID: b.CampaignID, BrandID: b.BrandID,
		AssetID: b.AssetID, VersionNo: i32p(b.VersionNo), SourceFileID: b.SourceFileID, Prompt: b.Options.Prompt,
		Preset: string(b.Options.Preset), AspectRatio: b.Options.AspectRatio, Quality: b.Options.Quality,
		ImageCount: int32(b.Options.Count), ModelCode: b.ModelCode, CreatedBy: b.CreatedBy, CreatedAt: b.CreatedAt,
	})
}

func (r *Repository) Get(ctx context.Context, id kernel.ID) (*domain.Batch, error) {
	row, err := r.q(ctx).GetBatch(ctx, id)
	if err != nil {
		return nil, db.MapNotFound(err)
	}
	b := toDomain(row)
	return &b, nil
}

func (r *Repository) SetResult(ctx context.Context, id, assetID kernel.ID, versionNo int) error {
	n, err := r.q(ctx).SetBatchResult(ctx, SetBatchResultParams{ID: id, AssetID: &assetID, VersionNo: i32p(&versionNo)})
	if err != nil {
		return err
	}
	if n == 0 {
		return kernel.ErrNotFound
	}
	return nil
}

func (r *Repository) List(ctx context.Context, q usecase.BatchQuery) ([]domain.Batch, error) {
	p := ListBatchesParams{CampaignID: q.CampaignID, AssetID: q.AssetID, CreatedBy: q.CreatedBy, Lim: int32(q.Limit)}
	if q.After != nil {
		at, id := q.After.At, q.After.ID
		p.CursorAt, p.CursorID = &at, &id
	}
	rows, err := r.q(ctx).ListBatches(ctx, p)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Batch, 0, len(rows))
	for _, row := range rows {
		out = append(out, toDomain(row))
	}
	return out, nil
}
