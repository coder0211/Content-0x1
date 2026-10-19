package http

import (
	"time"

	"github.com/be/acs/api/internal/modules/identity/domain"
	"github.com/be/acs/api/internal/modules/identity/usecase"
	"github.com/be/acs/api/internal/shared/kernel"
)

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type meResponse struct {
	ID          kernel.ID `json:"id"`
	Email       string    `json:"email"`
	Name        string    `json:"name"`
	Roles       []string  `json:"roles"`
	Permissions []string  `json:"permissions"`
}

type loginResponse struct {
	User      meResponse `json:"user"`
	CSRFToken string     `json:"csrf_token"`
	ExpiresAt time.Time  `json:"expires_at"`
}

type createUserRequest struct {
	Email        string   `json:"email"`
	Name         string   `json:"name"`
	RoleCodes    []string `json:"role_codes"`
	TempPassword string   `json:"temp_password"`
}

type updateUserRequest struct {
	Name      *string   `json:"name"`
	Status    *string   `json:"status"`
	RoleCodes *[]string `json:"role_codes"`
}

type userResponse struct {
	ID          kernel.ID  `json:"id"`
	Email       string     `json:"email"`
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	Roles       []string   `json:"roles"`
	LockedUntil *time.Time `json:"locked_until"`
	LastLoginAt *time.Time `json:"last_login_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

func toMe(m usecase.Me) meResponse {
	roles, perms := m.Roles, m.Permissions
	if roles == nil {
		roles = []string{}
	}
	if perms == nil {
		perms = []string{}
	}
	return meResponse{ID: m.ID, Email: m.Email, Name: m.Name, Roles: roles, Permissions: perms}
}

func toUser(u *domain.User) userResponse {
	roles := u.Roles
	if roles == nil {
		roles = []string{}
	}
	return userResponse{ID: u.ID, Email: u.Email, Name: u.Name, Status: string(u.Status), Roles: roles,
		LockedUntil: u.LockedUntil, LastLoginAt: u.LastLoginAt, CreatedAt: u.CreatedAt}
}
