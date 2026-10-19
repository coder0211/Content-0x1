package usecase

import (
	"context"

	"github.com/be/acs/api/internal/modules/image/domain"
	"github.com/be/acs/api/internal/shared/kernel"
)

// BatchQuery filters ListBatches at the repository level.
type BatchQuery struct {
	CampaignID *kernel.ID
	AssetID    *kernel.ID
	// CreatedBy restricts to one user's batches (nil = everyone's).
	CreatedBy *kernel.ID
	Limit     int
	// After is the keyset position (created_at DESC, id DESC).
	After *kernel.TimeIDCursor
}

// BatchRepository persists image_batches (adapter/postgres).
type BatchRepository interface {
	Insert(ctx context.Context, b *domain.Batch) error
	Get(ctx context.Context, id kernel.ID) (*domain.Batch, error)
	// SetResult links the produced asset/version (idempotent).
	SetResult(ctx context.Context, id, assetID kernel.ID, versionNo int) error
	List(ctx context.Context, q BatchQuery) ([]domain.Batch, error)
}
