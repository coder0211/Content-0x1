package usecase

import (
	"context"
	"time"

	"github.com/be/acs/api/internal/modules/video/domain"
	"github.com/be/acs/api/internal/shared/kernel"
)

// ProjectFilter selects projects for ListProjects.
type ProjectFilter struct {
	CreatedBy  *kernel.ID
	CampaignID *kernel.ID
	// Cursor: strictly older than (CreatedAt, ID).
	AfterCreatedAt *time.Time
	AfterID        *kernel.ID
	Limit          int
}

// Repository persists video projects and their scenes (video_projects,
// video_scenes). Methods join the caller's transaction via ctx.
type Repository interface {
	CreateProject(ctx context.Context, p *domain.Project) error
	GetProject(ctx context.Context, assetID kernel.ID) (*domain.Project, error)
	// LockProject loads the project with a row lock (SELECT … FOR UPDATE)
	// to serialise job creation per project.
	LockProject(ctx context.Context, assetID kernel.ID) (*domain.Project, error)
	UpdateProject(ctx context.Context, p *domain.Project) error
	ListProjects(ctx context.Context, f ProjectFilter) ([]domain.Project, error)

	// ListScenes returns live scenes ordered by sequence.
	ListScenes(ctx context.Context, assetID kernel.ID) ([]domain.Scene, error)
	GetScene(ctx context.Context, id kernel.ID) (*domain.Scene, error)
	// SaveStoryboard makes scenes the live storyboard of assetID: rows with
	// a known id are updated, new ids inserted, missing ones soft-deleted;
	// sequence numbers follow slice order (1-based).
	SaveStoryboard(ctx context.Context, assetID kernel.ID, scenes []domain.Scene) error
	UpdateScene(ctx context.Context, s *domain.Scene) error
}
