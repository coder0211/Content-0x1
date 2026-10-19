package campaigntest

import (
	"context"
	"slices"
	"strings"
	"sync"

	"github.com/be/acs/api/internal/modules/campaign/domain"
	"github.com/be/acs/api/internal/modules/campaign/usecase"
	identity "github.com/be/acs/api/internal/modules/identity/usecase"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Repo is an in-memory usecase.Repository for campaign tests.
type Repo struct {
	mu sync.Mutex
	m  map[kernel.ID]domain.Campaign
}

// NewRepo returns an empty repository.
func NewRepo() *Repo { return &Repo{m: map[kernel.ID]domain.Campaign{}} }

var _ usecase.Repository = (*Repo)(nil)

func clone(c domain.Campaign) *domain.Campaign {
	c.MemberIDs = slices.Clone(c.MemberIDs)
	if c.MemberIDs == nil {
		c.MemberIDs = []kernel.ID{}
	}
	return &c
}

// Create implements usecase.Repository.
func (r *Repo) Create(_ context.Context, c *domain.Campaign) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[c.ID] = *clone(*c)
	return nil
}

// Get implements usecase.Repository.
func (r *Repo) Get(_ context.Context, id kernel.ID) (*domain.Campaign, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.m[id]
	if !ok {
		return nil, kernel.ErrNotFound
	}
	return clone(c), nil
}

// GetForUpdate implements usecase.Repository.
func (r *Repo) GetForUpdate(ctx context.Context, id kernel.ID) (*domain.Campaign, error) {
	return r.Get(ctx, id)
}

// Update implements usecase.Repository (members are kept).
func (r *Repo) Update(_ context.Context, c *domain.Campaign) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	old, ok := r.m[c.ID]
	if !ok {
		return kernel.ErrNotFound
	}
	n := *clone(*c)
	n.MemberIDs = old.MemberIDs
	r.m[c.ID] = n
	return nil
}

// SetMembers implements usecase.Repository.
func (r *Repo) SetMembers(_ context.Context, id kernel.ID, members []kernel.ID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.m[id]
	if !ok {
		return kernel.ErrNotFound
	}
	c.MemberIDs = slices.Clone(members)
	r.m[id] = c
	return nil
}

// List implements usecase.Repository (no pagination beyond limit).
func (r *Repo) List(_ context.Context, f usecase.Filter, p kernel.PageRequest) (kernel.Page[domain.Campaign], error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []domain.Campaign
	for _, c := range r.m {
		if f.BrandID != nil && c.BrandID != *f.BrandID || f.Status != "" && string(c.Status) != f.Status ||
			f.Q != "" && !strings.Contains(strings.ToLower(c.Name), strings.ToLower(f.Q)) ||
			f.MemberID != nil && !c.IsMember(*f.MemberID) {
			continue
		}
		out = append(out, *clone(c))
	}
	slices.SortFunc(out, func(a, b domain.Campaign) int { return b.CreatedAt.Compare(a.CreatedAt) })
	if len(out) > p.Limit {
		out = out[:p.Limit]
	}
	if out == nil {
		out = []domain.Campaign{}
	}
	return kernel.Page[domain.Campaign]{Items: out}, nil
}

// Users is an in-memory usecase.Users.
type Users struct {
	M map[kernel.ID]identity.UserSummary
}

// NewUsers returns an empty fake.
func NewUsers() *Users { return &Users{M: map[kernel.ID]identity.UserSummary{}} }

// Add registers an active user with roles.
func (u *Users) Add(name string, roles ...string) kernel.ID {
	id := kernel.NewID()
	u.M[id] = identity.UserSummary{ID: id, Name: name, Email: strings.ToLower(strings.ReplaceAll(name, " ", ".")) + "@acs.local", Status: "active", Roles: roles}
	return id
}

// UsersWithPermission implements usecase.Users.
func (u *Users) UsersWithPermission(_ context.Context, perm string) ([]identity.UserSummary, error) {
	var out []identity.UserSummary
	for _, x := range u.M {
		for _, r := range x.Roles {
			if slices.Contains(kernel.RolePermissions[r], perm) && x.Status == "active" {
				out = append(out, x)
				break
			}
		}
	}
	return out, nil
}

// GetUsers implements usecase.Users.
func (u *Users) GetUsers(_ context.Context, ids []kernel.ID) ([]identity.UserSummary, error) {
	var out []identity.UserSummary
	for _, id := range ids {
		if x, ok := u.M[id]; ok {
			out = append(out, x)
		}
	}
	return out, nil
}
