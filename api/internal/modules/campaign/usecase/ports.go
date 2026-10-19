package usecase

import (
	"context"

	brand "github.com/be/acs/api/internal/modules/brand/usecase"
	"github.com/be/acs/api/internal/modules/campaign/domain"
	identity "github.com/be/acs/api/internal/modules/identity/usecase"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Filter filters List.
type Filter struct {
	BrandID *kernel.ID
	Status  string
	Q       string
	// MemberID keeps campaigns created by or assigned to this user.
	MemberID *kernel.ID
}

// Repository persists campaigns and members; implementations join the
// caller's transaction (ports.TxManager).
type Repository interface {
	Create(ctx context.Context, c *domain.Campaign) error
	// Get returns a non-deleted campaign with its members.
	Get(ctx context.Context, id kernel.ID) (*domain.Campaign, error)
	// GetForUpdate is Get with a row lock.
	GetForUpdate(ctx context.Context, id kernel.ID) (*domain.Campaign, error)
	// Update writes every mutable column (not members).
	Update(ctx context.Context, c *domain.Campaign) error
	// SetMembers replaces the member list.
	SetMembers(ctx context.Context, id kernel.ID, members []kernel.ID) error
	List(ctx context.Context, f Filter, p kernel.PageRequest) (kernel.Page[domain.Campaign], error)
}

// Brands is the subset of brand.API the campaign module uses.
type Brands interface {
	GetBrandContext(ctx context.Context, req brand.BrandContextRequest) (brand.BrandContext, error)
}

// Users is the subset of identity.API the campaign module uses.
type Users interface {
	UsersWithPermission(ctx context.Context, perm string) ([]identity.UserSummary, error)
	GetUsers(ctx context.Context, ids []kernel.ID) ([]identity.UserSummary, error)
}
