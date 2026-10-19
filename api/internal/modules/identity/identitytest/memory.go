// Package identitytest provides in-memory identity repositories for tests.
package identitytest

import (
	"bytes"
	"context"
	"slices"
	"sync"
	"time"

	"github.com/be/acs/api/internal/modules/identity/domain"
	"github.com/be/acs/api/internal/modules/identity/usecase"
	"github.com/be/acs/api/internal/shared/kernel"
)

type Users struct {
	mu    sync.Mutex
	byID  map[kernel.ID]*domain.User
	email map[string]kernel.ID
}

func NewUsers() *Users {
	return &Users{byID: map[kernel.ID]*domain.User{}, email: map[string]kernel.ID{}}
}

func clone(u *domain.User) *domain.User { c := *u; c.Roles = slices.Clone(u.Roles); return &c }

func (m *Users) Create(_ context.Context, u *domain.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.email[u.Email]; ok {
		return kernel.ErrConflict
	}
	m.byID[u.ID], m.email[u.Email] = clone(u), u.ID
	return nil
}
func (m *Users) get(id kernel.ID) (*domain.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.byID[id]
	if !ok {
		return nil, kernel.ErrNotFound
	}
	return clone(u), nil
}
func (m *Users) GetByID(_ context.Context, id kernel.ID) (*domain.User, error) { return m.get(id) }
func (m *Users) GetForUpdate(_ context.Context, id kernel.ID) (*domain.User, error) {
	return m.get(id)
}
func (m *Users) GetByEmail(_ context.Context, e string) (*domain.User, error) {
	m.mu.Lock()
	id, ok := m.email[e]
	m.mu.Unlock()
	if !ok {
		return nil, kernel.ErrNotFound
	}
	return m.get(id)
}
func (m *Users) put(u *domain.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.byID[u.ID] = clone(u)
	return nil
}
func (m *Users) UpdateLoginState(_ context.Context, u *domain.User) error { return m.put(u) }
func (m *Users) UpdateProfile(_ context.Context, u *domain.User) error    { return m.put(u) }
func (m *Users) SetRoles(_ context.Context, id kernel.ID, roles []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.byID[id].Roles = slices.Clone(roles)
	return nil
}
func (m *Users) List(_ context.Context, _ usecase.UserFilter, p kernel.PageRequest) (kernel.Page[domain.User], error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []domain.User
	for _, u := range m.byID {
		out = append(out, *u)
	}
	return kernel.PageFromRows(out, p.Limit, func(u domain.User) any { return u.ID }), nil
}
func (m *Users) ListByIDs(_ context.Context, ids []kernel.ID) ([]domain.User, error) {
	var out []domain.User
	for _, id := range ids {
		if u, err := m.get(id); err == nil {
			out = append(out, *u)
		}
	}
	return out, nil
}
func (m *Users) Permissions(_ context.Context, id kernel.ID) ([]string, error) {
	u, err := m.get(id)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, r := range u.Roles {
		for _, p := range kernel.RolePermissions[r] {
			if !slices.Contains(out, p) {
				out = append(out, p)
			}
		}
	}
	return out, nil
}
func (m *Users) WithPermission(ctx context.Context, perm string) ([]domain.User, error) {
	var out []domain.User
	for id := range m.byID {
		ps, _ := m.Permissions(ctx, id)
		if slices.Contains(ps, perm) {
			u, _ := m.get(id)
			out = append(out, *u)
		}
	}
	return out, nil
}

type Sessions struct {
	mu sync.Mutex
	s  map[kernel.ID]*domain.Session
}

func NewSessions() *Sessions { return &Sessions{s: map[kernel.ID]*domain.Session{}} }

func (m *Sessions) Create(_ context.Context, s *domain.Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := *s
	m.s[s.ID] = &c
	return nil
}
func (m *Sessions) GetByTokenHash(_ context.Context, h []byte) (*domain.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.s {
		if bytes.Equal(s.TokenHash, h) {
			c := *s
			return &c, nil
		}
	}
	return nil, kernel.ErrNotFound
}
func (m *Sessions) Touch(_ context.Context, id kernel.ID, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.s[id].LastSeenAt = at
	return nil
}
func (m *Sessions) Revoke(_ context.Context, id kernel.ID, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.s[id]; ok {
		s.RevokedAt = &at
	}
	return nil
}
func (m *Sessions) RevokeAllForUser(_ context.Context, uid kernel.ID, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.s {
		if s.UserID == uid && s.RevokedAt == nil {
			s.RevokedAt = &at
		}
	}
	return nil
}

// PlainHasher stores "h:"+password; argon2 itself is tested in adapter/password.
type PlainHasher struct{}

func (PlainHasher) Hash(p string) (string, error)      { return "h:" + p, nil }
func (PlainHasher) Verify(p, enc string) (bool, error) { return enc == "h:"+p, nil }
