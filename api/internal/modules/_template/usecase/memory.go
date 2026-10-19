package usecase

import (
	"context"
	"sync"

	"github.com/be/acs/api/internal/modules/_template/domain"
	"github.com/be/acs/api/internal/shared/kernel"
)

// MemoryRepo is an in-memory NoteRepository (tests; real modules put the
// postgres implementation in adapter/postgres).
type MemoryRepo struct {
	mu    sync.Mutex
	notes map[kernel.ID]domain.Note
}

// NewMemoryRepo returns an empty repository.
func NewMemoryRepo() *MemoryRepo { return &MemoryRepo{notes: map[kernel.ID]domain.Note{}} }

func (m *MemoryRepo) Create(_ context.Context, n *domain.Note) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notes[n.ID] = *n
	return nil
}

func (m *MemoryRepo) Get(_ context.Context, id kernel.ID) (*domain.Note, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n, ok := m.notes[id]
	if !ok {
		return nil, kernel.ErrNotFound
	}
	return &n, nil
}

func (m *MemoryRepo) GetForUpdate(ctx context.Context, id kernel.ID) (*domain.Note, error) {
	return m.Get(ctx, id)
}

func (m *MemoryRepo) Save(ctx context.Context, n *domain.Note) error { return m.Create(ctx, n) }
