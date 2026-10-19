package usecase

import (
	"context"
	"time"

	"github.com/be/acs/api/internal/modules/file/domain"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Verified is what CompleteUpload measured on the stored object.
type Verified struct {
	MimeType   string
	Kind       domain.Kind
	SizeBytes  int64
	Checksum   string
	Width      int
	Height     int
	DurationMs int
	// StorageKey is the final object key holding the verified bytes ("" =
	// keep the upload key).
	StorageKey string
}

// StaleFile identifies a pending upload to purge.
type StaleFile struct {
	ID         kernel.ID
	StorageKey string
}

// Repository persists asset_files rows.
type Repository interface {
	Create(ctx context.Context, f *domain.File) error
	Get(ctx context.Context, id kernel.ID) (*domain.File, error)
	GetMany(ctx context.Context, ids []kernel.ID) ([]domain.File, error)
	MarkReady(ctx context.Context, id kernel.ID, v Verified) error
	MarkRejected(ctx context.Context, id kernel.ID, reason string) error
	ListStalePending(ctx context.Context, before time.Time, limit int) ([]StaleFile, error)
	Delete(ctx context.Context, id kernel.ID) error
}
