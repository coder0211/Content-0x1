package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/be/acs/api/internal/modules/file/domain"
	"github.com/be/acs/api/internal/modules/file/usecase"
	db "github.com/be/acs/api/internal/platform/postgres"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Repository implements usecase.Repository.
type Repository struct{ pool *pgxpool.Pool }

// NewRepository returns a repository on pool.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

var _ usecase.Repository = (*Repository)(nil)

func (r *Repository) q(ctx context.Context) *Queries { return New(db.Conn(ctx, r.pool)) }

func i32(n int) *int32 {
	if n == 0 {
		return nil
	}
	v := int32(n)
	return &v
}

func str(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func deref32(p *int32) int {
	if p == nil {
		return 0
	}
	return int(*p)
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func toDomain(a AssetFile) domain.File {
	return domain.File{
		ID: a.ID, StorageKey: a.StorageKey, OriginalName: derefStr(a.OriginalName), MimeType: a.MimeType,
		SizeBytes: a.SizeBytes, ChecksumSHA256: derefStr(a.ChecksumSha256), Width: deref32(a.Width),
		Height: deref32(a.Height), DurationMs: deref32(a.DurationMs), Kind: domain.Kind(a.Kind),
		UploadStatus: domain.UploadStatus(a.UploadStatus), Source: domain.Source(a.Source),
		ParentFileID: a.ParentFileID, License: a.LicenseJson, Metadata: a.Metadata, CreatedBy: a.CreatedBy, CreatedAt: a.CreatedAt,
	}
}

// Create inserts a file row.
func (r *Repository) Create(ctx context.Context, f *domain.File) error {
	return r.q(ctx).CreateFile(ctx, CreateFileParams{
		ID: f.ID, StorageKey: f.StorageKey, OriginalName: str(f.OriginalName), MimeType: f.MimeType,
		SizeBytes: f.SizeBytes, ChecksumSha256: str(f.ChecksumSHA256), Width: i32(f.Width), Height: i32(f.Height),
		DurationMs: i32(f.DurationMs), Kind: string(f.Kind), UploadStatus: string(f.UploadStatus), Source: string(f.Source),
		ParentFileID: f.ParentFileID, LicenseJson: f.License, Metadata: f.Metadata, CreatedBy: f.CreatedBy, CreatedAt: f.CreatedAt,
	})
}

// Get loads a file.
func (r *Repository) Get(ctx context.Context, id kernel.ID) (*domain.File, error) {
	a, err := r.q(ctx).GetFile(ctx, id)
	if err != nil {
		return nil, db.MapNotFound(err)
	}
	f := toDomain(a)
	return &f, nil
}

// GetMany loads several files.
func (r *Repository) GetMany(ctx context.Context, ids []kernel.ID) ([]domain.File, error) {
	rows, err := r.q(ctx).GetFiles(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]domain.File, 0, len(rows))
	for _, a := range rows {
		out = append(out, toDomain(a))
	}
	return out, nil
}

// MarkReady stores verified metadata.
func (r *Repository) MarkReady(ctx context.Context, id kernel.ID, v usecase.Verified) error {
	return r.q(ctx).MarkFileReady(ctx, MarkFileReadyParams{
		ID: id, MimeType: v.MimeType, Kind: string(v.Kind), SizeBytes: v.SizeBytes, ChecksumSha256: str(v.Checksum),
		Width: i32(v.Width), Height: i32(v.Height), DurationMs: i32(v.DurationMs), StorageKey: str(v.StorageKey),
	})
}

// MarkRejected flags a failed upload with its reason.
func (r *Repository) MarkRejected(ctx context.Context, id kernel.ID, reason string) error {
	meta, _ := json.Marshal(map[string]string{"rejected_reason": reason})
	return r.q(ctx).MarkFileRejected(ctx, MarkFileRejectedParams{ID: id, Metadata: meta})
}

// ListStalePending lists pending uploads created before `before`.
func (r *Repository) ListStalePending(ctx context.Context, before time.Time, limit int) ([]usecase.StaleFile, error) {
	rows, err := r.q(ctx).ListStalePendingFiles(ctx, ListStalePendingFilesParams{Before: before, Lim: int32(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]usecase.StaleFile, 0, len(rows))
	for _, row := range rows {
		out = append(out, usecase.StaleFile{ID: row.ID, StorageKey: row.StorageKey})
	}
	return out, nil
}

// Delete removes a non-ready file row.
func (r *Repository) Delete(ctx context.Context, id kernel.ID) error {
	return r.q(ctx).DeleteFile(ctx, id)
}
