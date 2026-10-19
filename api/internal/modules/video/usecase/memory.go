package usecase

import (
	"context"
	"slices"
	"sort"
	"sync"

	"github.com/be/acs/api/internal/modules/video/domain"
	"github.com/be/acs/api/internal/shared/kernel"
)

// MemoryRepo is an in-memory Repository for unit tests (no Docker).
type MemoryRepo struct {
	mu       sync.Mutex
	projects map[kernel.ID]domain.Project
	scenes   map[kernel.ID]domain.Scene
	deleted  map[kernel.ID]bool
}

// NewMemoryRepo returns an empty repository.
func NewMemoryRepo() *MemoryRepo {
	return &MemoryRepo{projects: map[kernel.ID]domain.Project{}, scenes: map[kernel.ID]domain.Scene{}, deleted: map[kernel.ID]bool{}}
}

var _ Repository = (*MemoryRepo)(nil)

func (m *MemoryRepo) CreateProject(_ context.Context, p *domain.Project) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, dup := m.projects[p.AssetID]; dup {
		return kernel.ErrConflict
	}
	m.projects[p.AssetID] = *p
	return nil
}

func (m *MemoryRepo) GetProject(_ context.Context, id kernel.ID) (*domain.Project, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.projects[id]
	if !ok {
		return nil, kernel.ErrNotFound
	}
	return &p, nil
}

func (m *MemoryRepo) LockProject(ctx context.Context, id kernel.ID) (*domain.Project, error) {
	return m.GetProject(ctx, id)
}

func (m *MemoryRepo) UpdateProject(_ context.Context, p *domain.Project) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.projects[p.AssetID]; !ok {
		return kernel.ErrNotFound
	}
	m.projects[p.AssetID] = *p
	return nil
}

func (m *MemoryRepo) ListProjects(_ context.Context, f ProjectFilter) ([]domain.Project, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []domain.Project
	for _, p := range m.projects {
		if f.CreatedBy != nil && p.CreatedBy != *f.CreatedBy {
			continue
		}
		if f.CampaignID != nil && (p.CampaignID == nil || *p.CampaignID != *f.CampaignID) {
			continue
		}
		if f.AfterCreatedAt != nil && !p.CreatedAt.Before(*f.AfterCreatedAt) &&
			(!p.CreatedAt.Equal(*f.AfterCreatedAt) || f.AfterID == nil || p.AssetID.String() >= f.AfterID.String()) {
			continue
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].AssetID.String() > out[j].AssetID.String()
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	if f.Limit > 0 && len(out) > f.Limit {
		out = out[:f.Limit]
	}
	return out, nil
}

func (m *MemoryRepo) ListScenes(_ context.Context, assetID kernel.ID) ([]domain.Scene, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []domain.Scene
	for id, s := range m.scenes {
		if s.AssetID == assetID && !m.deleted[id] {
			out = append(out, s)
		}
	}
	slices.SortFunc(out, func(a, b domain.Scene) int { return a.SequenceNo - b.SequenceNo })
	return out, nil
}

func (m *MemoryRepo) GetScene(_ context.Context, id kernel.ID) (*domain.Scene, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.scenes[id]
	if !ok || m.deleted[id] {
		return nil, kernel.ErrNotFound
	}
	return &s, nil
}

func (m *MemoryRepo) SaveStoryboard(_ context.Context, assetID kernel.ID, scenes []domain.Scene) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	keep := map[kernel.ID]bool{}
	for i, s := range scenes {
		s.AssetID = assetID
		s.SequenceNo = i + 1
		keep[s.ID] = true
		m.scenes[s.ID] = s
	}
	for id, s := range m.scenes {
		if s.AssetID == assetID && !keep[id] {
			m.deleted[id] = true
		}
	}
	return nil
}

func (m *MemoryRepo) UpdateScene(_ context.Context, s *domain.Scene) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.scenes[s.ID]; !ok || m.deleted[s.ID] {
		return kernel.ErrNotFound
	}
	m.scenes[s.ID] = *s
	return nil
}
