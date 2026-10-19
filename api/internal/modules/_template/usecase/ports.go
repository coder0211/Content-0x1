package usecase

import (
	"context"

	"github.com/be/acs/api/internal/modules/_template/domain"
	"github.com/be/acs/api/internal/shared/kernel"
)

// NoteRepository is declared where it is used (R2); adapters implement it.
type NoteRepository interface {
	Create(ctx context.Context, n *domain.Note) error
	GetForUpdate(ctx context.Context, id kernel.ID) (*domain.Note, error)
	Get(ctx context.Context, id kernel.ID) (*domain.Note, error)
	Save(ctx context.Context, n *domain.Note) error
}
