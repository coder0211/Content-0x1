package usecase

import (
	"context"
	"time"

	"github.com/be/acs/api/internal/modules/identity/domain"
	"github.com/be/acs/api/internal/shared/kernel"
)

// UserFilter filters ListUsers.
type UserFilter struct {
	Query  string // matches e-mail or name
	Status string
}

// UserRepository persists users and their roles. Get* return
// kernel.ErrNotFound when missing.
type UserRepository interface {
	Create(ctx context.Context, u *domain.User) error // CONFLICT on duplicate e-mail
	GetByID(ctx context.Context, id kernel.ID) (*domain.User, error)
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
	// GetForUpdate locks the row for the rest of the transaction.
	GetForUpdate(ctx context.Context, id kernel.ID) (*domain.User, error)
	UpdateLoginState(ctx context.Context, u *domain.User) error
	UpdateProfile(ctx context.Context, u *domain.User) error
	SetRoles(ctx context.Context, userID kernel.ID, roles []string) error
	List(ctx context.Context, f UserFilter, page kernel.PageRequest) (kernel.Page[domain.User], error)
	ListByIDs(ctx context.Context, ids []kernel.ID) ([]domain.User, error)
	// Permissions returns the union of permissions of the user's roles.
	Permissions(ctx context.Context, userID kernel.ID) ([]string, error)
	WithPermission(ctx context.Context, perm string) ([]domain.User, error)
}

// SessionRepository persists sessions.
type SessionRepository interface {
	Create(ctx context.Context, s *domain.Session) error
	GetByTokenHash(ctx context.Context, hash []byte) (*domain.Session, error)
	Touch(ctx context.Context, id kernel.ID, at time.Time) error
	Revoke(ctx context.Context, id kernel.ID, at time.Time) error
	RevokeAllForUser(ctx context.Context, userID kernel.ID, at time.Time) error
}

// PasswordHasher hashes and verifies passwords (argon2id).
type PasswordHasher interface {
	Hash(password string) (string, error)
	Verify(password, encoded string) (bool, error)
}
