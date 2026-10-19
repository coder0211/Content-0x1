package postgres

import (
	"context"
	"net/netip"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/be/acs/api/internal/modules/identity/domain"
	"github.com/be/acs/api/internal/modules/identity/usecase"
	db "github.com/be/acs/api/internal/platform/postgres"
	"github.com/be/acs/api/internal/shared/kernel"
)

// UserRepository implements usecase.UserRepository.
type UserRepository struct{ pool *pgxpool.Pool }

// SessionRepository implements usecase.SessionRepository.
type SessionRepository struct{ pool *pgxpool.Pool }

// NewUserRepository returns a repository on pool.
func NewUserRepository(pool *pgxpool.Pool) *UserRepository { return &UserRepository{pool: pool} }

// NewSessionRepository returns a repository on pool.
func NewSessionRepository(pool *pgxpool.Pool) *SessionRepository {
	return &SessionRepository{pool: pool}
}

var (
	_ usecase.UserRepository    = (*UserRepository)(nil)
	_ usecase.SessionRepository = (*SessionRepository)(nil)
)

func (r *UserRepository) q(ctx context.Context) *Queries    { return New(db.Conn(ctx, r.pool)) }
func (r *SessionRepository) q(ctx context.Context) *Queries { return New(db.Conn(ctx, r.pool)) }

func toUser(row GetUserByIDRow) *domain.User {
	u := &domain.User{
		ID: row.ID, Email: row.Email, Name: row.Name, Status: domain.UserStatus(row.Status),
		FailedLoginCount: int(row.FailedLoginCount), LastFailedLoginAt: row.LastFailedLoginAt,
		LockedUntil: row.LockedUntil, LastLoginAt: row.LastLoginAt, Roles: row.Roles,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
	if row.PasswordHash != nil {
		u.PasswordHash = *row.PasswordHash
	}
	return u
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// Create inserts a user (CONFLICT on duplicate e-mail).
func (r *UserRepository) Create(ctx context.Context, u *domain.User) error {
	err := r.q(ctx).CreateUser(ctx, CreateUserParams{
		ID: u.ID, Email: u.Email, Name: u.Name, PasswordHash: strPtr(u.PasswordHash), Status: string(u.Status), CreatedAt: u.CreatedAt,
	})
	if db.IsUniqueViolation(err, "") {
		return kernel.ErrConflict.WithMessage("Email đã được sử dụng").WithDetails(kernel.FieldIssue("email", "taken"))
	}
	return err
}

// GetByID loads a user with roles.
func (r *UserRepository) GetByID(ctx context.Context, id kernel.ID) (*domain.User, error) {
	row, err := r.q(ctx).GetUserByID(ctx, id)
	if err != nil {
		return nil, db.MapNotFound(err)
	}
	return toUser(row), nil
}

// GetByEmail loads a user by (case-insensitive) e-mail.
func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	row, err := r.q(ctx).GetUserByEmail(ctx, email)
	if err != nil {
		return nil, db.MapNotFound(err)
	}
	return toUser(GetUserByIDRow(row)), nil
}

// GetForUpdate locks the user row then loads it.
func (r *UserRepository) GetForUpdate(ctx context.Context, id kernel.ID) (*domain.User, error) {
	if err := r.q(ctx).LockUser(ctx, id); err != nil {
		return nil, db.MapNotFound(err)
	}
	return r.GetByID(ctx, id)
}

// UpdateLoginState persists lockout counters and last login.
func (r *UserRepository) UpdateLoginState(ctx context.Context, u *domain.User) error {
	return r.q(ctx).UpdateUserLoginState(ctx, UpdateUserLoginStateParams{
		ID: u.ID, FailedLoginCount: int32(u.FailedLoginCount), LastFailedLoginAt: u.LastFailedLoginAt,
		LockedUntil: u.LockedUntil, LastLoginAt: u.LastLoginAt,
	})
}

// UpdateProfile persists name and status.
func (r *UserRepository) UpdateProfile(ctx context.Context, u *domain.User) error {
	return r.q(ctx).UpdateUserProfile(ctx, UpdateUserProfileParams{ID: u.ID, Name: u.Name, Status: string(u.Status)})
}

// SetRoles replaces the user's roles.
func (r *UserRepository) SetRoles(ctx context.Context, userID kernel.ID, roles []string) error {
	q := r.q(ctx)
	if err := q.DeleteUserRoles(ctx, userID); err != nil {
		return err
	}
	n, err := q.InsertUserRoles(ctx, InsertUserRolesParams{UserID: userID, Codes: roles})
	if err != nil {
		return err
	}
	if int(n) != len(roles) {
		return kernel.Validation(kernel.FieldIssue("role_codes", "invalid"))
	}
	return nil
}

// List pages users ordered by (created_at, id) DESC.
func (r *UserRepository) List(ctx context.Context, f usecase.UserFilter, page kernel.PageRequest) (kernel.Page[domain.User], error) {
	var cur kernel.TimeIDCursor
	has, err := kernel.DecodeCursor(page.Cursor, &cur)
	if err != nil {
		return kernel.Page[domain.User]{}, err
	}
	p := ListUsersParams{Query: strPtr(f.Query), Status: strPtr(f.Status), Lim: int32(page.Limit + 1)}
	if has {
		p.CursorAt, p.CursorID = &cur.At, &cur.ID
	}
	rows, err := r.q(ctx).ListUsers(ctx, p)
	if err != nil {
		return kernel.Page[domain.User]{}, err
	}
	users := make([]domain.User, 0, len(rows))
	for _, row := range rows {
		users = append(users, *toUser(GetUserByIDRow(row)))
	}
	return kernel.PageFromRows(users, page.Limit, func(u domain.User) any {
		return kernel.TimeIDCursor{At: u.CreatedAt, ID: u.ID}
	}), nil
}

// ListByIDs loads several users.
func (r *UserRepository) ListByIDs(ctx context.Context, ids []kernel.ID) ([]domain.User, error) {
	rows, err := r.q(ctx).ListUsersByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]domain.User, 0, len(rows))
	for _, row := range rows {
		out = append(out, *toUser(GetUserByIDRow(row)))
	}
	return out, nil
}

// Permissions returns effective permissions.
func (r *UserRepository) Permissions(ctx context.Context, userID kernel.ID) ([]string, error) {
	return r.q(ctx).GetUserPermissions(ctx, userID)
}

// WithPermission lists active users holding perm.
func (r *UserRepository) WithPermission(ctx context.Context, perm string) ([]domain.User, error) {
	rows, err := r.q(ctx).ListUsersWithPermission(ctx, perm)
	if err != nil {
		return nil, err
	}
	out := make([]domain.User, 0, len(rows))
	for _, row := range rows {
		out = append(out, *toUser(GetUserByIDRow(row)))
	}
	return out, nil
}

// Create inserts a session.
func (r *SessionRepository) Create(ctx context.Context, s *domain.Session) error {
	var ip *netip.Addr
	if a, err := netip.ParseAddr(s.IP); err == nil {
		ip = &a
	}
	return r.q(ctx).CreateSession(ctx, CreateSessionParams{
		ID: s.ID, UserID: s.UserID, TokenHash: s.TokenHash, CsrfToken: s.CSRFToken, Ip: ip,
		UserAgent: strPtr(s.UserAgent), LastSeenAt: s.LastSeenAt, ExpiresAt: s.ExpiresAt, CreatedAt: s.CreatedAt,
	})
}

// GetByTokenHash loads a session.
func (r *SessionRepository) GetByTokenHash(ctx context.Context, hash []byte) (*domain.Session, error) {
	row, err := r.q(ctx).GetSessionByTokenHash(ctx, hash)
	if err != nil {
		return nil, db.MapNotFound(err)
	}
	s := &domain.Session{
		ID: row.ID, UserID: row.UserID, TokenHash: row.TokenHash, CSRFToken: row.CsrfToken, IP: row.Ip,
		LastSeenAt: row.LastSeenAt, ExpiresAt: row.ExpiresAt, RevokedAt: row.RevokedAt, CreatedAt: row.CreatedAt,
	}
	if row.UserAgent != nil {
		s.UserAgent = *row.UserAgent
	}
	return s, nil
}

// Touch refreshes last_seen_at.
func (r *SessionRepository) Touch(ctx context.Context, id kernel.ID, at time.Time) error {
	return r.q(ctx).TouchSession(ctx, TouchSessionParams{ID: id, At: at})
}

// Revoke revokes one session.
func (r *SessionRepository) Revoke(ctx context.Context, id kernel.ID, at time.Time) error {
	return r.q(ctx).RevokeSession(ctx, RevokeSessionParams{ID: id, At: &at})
}

// RevokeAllForUser revokes every active session of a user.
func (r *SessionRepository) RevokeAllForUser(ctx context.Context, userID kernel.ID, at time.Time) error {
	return r.q(ctx).RevokeUserSessions(ctx, RevokeUserSessionsParams{UserID: userID, At: &at})
}
