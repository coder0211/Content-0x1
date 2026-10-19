package usecase

import (
	"context"
	"slices"

	"github.com/be/acs/api/internal/modules/identity/domain"
	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Users implements user management (FR-AUTH-02, ACS-008) and the public API.
type Users struct {
	Tx       ports.TxManager
	Users    UserRepository
	Sessions SessionRepository
	Hasher   PasswordHasher
	Clock    ports.Clock
	Audit    ports.AuditRecorder
}

var _ API = (*Users)(nil)

// CreateUserInput is POST /users.
type CreateUserInput struct {
	Email        string
	Name         string
	RoleCodes    []string
	TempPassword string
}

// UpdateUserInput is PATCH /users/{id}; nil fields are unchanged.
type UpdateUserInput struct {
	Name      *string
	Status    *string
	RoleCodes *[]string
}

func auditView(u *domain.User) map[string]any {
	return map[string]any{"email": u.Email, "name": u.Name, "status": u.Status, "roles": u.Roles}
}

// Create adds a user with a temporary password (audited).
func (uc *Users) Create(ctx context.Context, actor kernel.Actor, in CreateUserInput) (*domain.User, error) {
	if err := actor.Require(kernel.PermUserManage); err != nil {
		return nil, err
	}
	return uc.create(ctx, in)
}

// Bootstrap creates a user without an actor (CLI `acs admin create`).
func (uc *Users) Bootstrap(ctx context.Context, in CreateUserInput) (*domain.User, error) {
	return uc.create(ctx, in)
}

func (uc *Users) create(ctx context.Context, in CreateUserInput) (*domain.User, error) {
	if err := domain.ValidatePassword("temp_password", in.TempPassword); err != nil {
		return nil, err
	}
	hash, err := uc.Hasher.Hash(in.TempPassword)
	if err != nil {
		return nil, err
	}
	u, err := domain.NewUser(in.Email, in.Name, hash, in.RoleCodes, uc.Clock.Now())
	if err != nil {
		return nil, err
	}
	err = uc.Tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := uc.Users.Create(ctx, u); err != nil {
			return err
		}
		if err := uc.Users.SetRoles(ctx, u.ID, u.Roles); err != nil {
			return err
		}
		return uc.Audit.Record(ctx, "user.create", "user", u.ID.String(), nil, auditView(u))
	})
	if err != nil {
		return nil, err
	}
	return u, nil
}

// Get returns one user.
func (uc *Users) Get(ctx context.Context, actor kernel.Actor, id kernel.ID) (*domain.User, error) {
	if err := actor.Require(kernel.PermUserManage); err != nil {
		return nil, err
	}
	return uc.Users.GetByID(ctx, id)
}

// List pages through users.
func (uc *Users) List(ctx context.Context, actor kernel.Actor, f UserFilter, page kernel.PageRequest) (kernel.Page[domain.User], error) {
	if err := actor.Require(kernel.PermUserManage); err != nil {
		return kernel.Page[domain.User]{}, err
	}
	if f.Status != "" && !domain.UserStatus(f.Status).Valid() {
		return kernel.Page[domain.User]{}, kernel.Validation(kernel.FieldIssue("status", "invalid"))
	}
	return uc.Users.List(ctx, f, page.Normalized())
}

// Update changes name/status/roles. Locking or disabling a user revokes all
// their sessions immediately; role changes are audited as role.assign.
func (uc *Users) Update(ctx context.Context, actor kernel.Actor, id kernel.ID, in UpdateUserInput) (*domain.User, error) {
	if err := actor.Require(kernel.PermUserManage); err != nil {
		return nil, err
	}
	var out *domain.User
	err := uc.Tx.WithinTx(ctx, func(ctx context.Context) error {
		u, err := uc.Users.GetForUpdate(ctx, id)
		if err != nil {
			return err
		}
		before := auditView(u)
		var v kernel.ValidationErrors
		if in.Name != nil {
			if *in.Name == "" {
				v.Add("name", "required")
			}
			u.Name = *in.Name
		}
		statusChanged := false
		if in.Status != nil {
			st := domain.UserStatus(*in.Status)
			if !st.Valid() {
				v.Add("status", "invalid")
			}
			statusChanged = st != u.Status
			u.Status = st
		}
		rolesChanged := false
		if in.RoleCodes != nil {
			if err := domain.ValidateRoles(*in.RoleCodes); err != nil {
				v.Add("role_codes", "invalid")
			}
			rolesChanged = !sameSet(u.Roles, *in.RoleCodes)
			u.Roles = slices.Clone(*in.RoleCodes)
		}
		if err := v.Err(); err != nil {
			return err
		}
		if u.ID == actor.UserID && (u.Status != domain.StatusActive || !slices.Contains(u.Roles, kernel.RoleAdmin)) {
			return kernel.ErrForbidden.WithMessage("Không thể tự khóa hoặc gỡ quyền Admin của chính mình")
		}
		if err := uc.Users.UpdateProfile(ctx, u); err != nil {
			return err
		}
		if rolesChanged {
			if err := uc.Users.SetRoles(ctx, u.ID, u.Roles); err != nil {
				return err
			}
			if err := uc.Audit.Record(ctx, "role.assign", "user", u.ID.String(), map[string]any{"roles": before["roles"]}, map[string]any{"roles": u.Roles}); err != nil {
				return err
			}
		}
		if statusChanged && u.Status != domain.StatusActive {
			if err := uc.Sessions.RevokeAllForUser(ctx, u.ID, uc.Clock.Now()); err != nil {
				return err
			}
		}
		out = u
		return uc.Audit.Record(ctx, "user.update", "user", u.ID.String(), before, auditView(u))
	})
	return out, err
}

func sameSet(a, b []string) bool {
	x, y := slices.Clone(a), slices.Clone(b)
	slices.Sort(x)
	slices.Sort(y)
	return slices.Equal(x, y)
}

func summaries(us []domain.User) []UserSummary {
	out := make([]UserSummary, 0, len(us))
	for _, u := range us {
		out = append(out, UserSummary{ID: u.ID, Email: u.Email, Name: u.Name, Status: string(u.Status), Roles: u.Roles})
	}
	return out
}

// UsersWithPermission implements API.
func (uc *Users) UsersWithPermission(ctx context.Context, perm string) ([]UserSummary, error) {
	us, err := uc.Users.WithPermission(ctx, perm)
	if err != nil {
		return nil, err
	}
	return summaries(us), nil
}

// GetUsers implements API.
func (uc *Users) GetUsers(ctx context.Context, ids []kernel.ID) ([]UserSummary, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	us, err := uc.Users.ListByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	return summaries(us), nil
}
