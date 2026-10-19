// Package domain holds identity entities and rules: users, login lockout
// and session expiry (FR-AUTH-01).
package domain

import (
	"net/mail"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/be/acs/api/internal/shared/kernel"
)

// UserStatus mirrors users.status.
type UserStatus string

const (
	StatusActive   UserStatus = "active"
	StatusLocked   UserStatus = "locked"   // locked by an admin
	StatusDisabled UserStatus = "disabled" // offboarded
)

// Valid reports whether s is a known status.
func (s UserStatus) Valid() bool {
	return s == StatusActive || s == StatusLocked || s == StatusDisabled
}

// MinPasswordLength is the minimum password length.
const MinPasswordLength = 10

// LockoutPolicy: MaxFailed wrong passwords within Window lock the account
// for Window (5 / 15 min by default).
type LockoutPolicy struct {
	MaxFailed int
	Window    time.Duration
}

// User is an internal user account.
type User struct {
	ID                kernel.ID
	Email             string
	Name              string
	PasswordHash      string
	Status            UserStatus
	FailedLoginCount  int
	LastFailedLoginAt *time.Time
	LockedUntil       *time.Time
	LastLoginAt       *time.Time
	Roles             []string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

var (
	// ErrAccountDisabled is returned when a disabled user logs in.
	ErrAccountDisabled = kernel.NewError(kernel.CodeAccountDisabled, "Tài khoản đã bị vô hiệu hóa")
	// ErrAccountLockedByAdmin is returned when an admin-locked user logs in.
	ErrAccountLockedByAdmin = kernel.ErrAccountLocked.WithMessage("Tài khoản đã bị khóa, vui lòng liên hệ quản trị viên")
)

// NewUser validates and builds a user. passwordHash may be empty for SSO.
func NewUser(email, name, passwordHash string, roles []string, now time.Time) (*User, error) {
	var v kernel.ValidationErrors
	email = NormalizeEmail(email)
	if _, err := mail.ParseAddress(email); err != nil || !strings.Contains(email, "@") {
		v.Add("email", "invalid")
	}
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 120 {
		v.Add("name", "required")
	}
	if err := ValidateRoles(roles); err != nil {
		v.Add("role_codes", "invalid")
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	return &User{
		ID: kernel.NewID(), Email: email, Name: name, PasswordHash: passwordHash,
		Status: StatusActive, Roles: slices.Clone(roles), CreatedAt: now, UpdatedAt: now,
	}, nil
}

// NormalizeEmail trims and lower-cases an e-mail address.
func NormalizeEmail(e string) string { return strings.ToLower(strings.TrimSpace(e)) }

// ValidatePassword enforces the password policy.
func ValidatePassword(field, pw string) error {
	if utf8.RuneCountInString(pw) < MinPasswordLength {
		return kernel.Validation(kernel.Detail{"field": field, "issue": "too_short", "min": MinPasswordLength})
	}
	if len(pw) > 256 {
		return kernel.Validation(kernel.FieldIssue(field, "too_long"))
	}
	return nil
}

// ValidateRoles checks role codes (at least one, all known, no duplicates).
func ValidateRoles(roles []string) error {
	if len(roles) == 0 {
		return kernel.Validation(kernel.FieldIssue("role_codes", "required"))
	}
	seen := map[string]bool{}
	for _, r := range roles {
		if !kernel.IsKnownRole(r) || seen[r] {
			return kernel.Validation(kernel.Detail{"field": "role_codes", "issue": "invalid", "value": r})
		}
		seen[r] = true
	}
	return nil
}

// CheckCanLogin returns an error when the account may not log in now.
func (u *User) CheckCanLogin(now time.Time) error {
	switch u.Status {
	case StatusDisabled:
		return ErrAccountDisabled
	case StatusLocked:
		return ErrAccountLockedByAdmin
	}
	if u.LockedUntil != nil && now.Before(*u.LockedUntil) {
		return kernel.ErrAccountLocked.WithDetails(kernel.Detail{"locked_until": u.LockedUntil.UTC().Format(time.RFC3339)})
	}
	if u.PasswordHash == "" {
		return kernel.ErrInvalidCredentials
	}
	return nil
}

// RegisterFailedLogin counts a wrong password. Failures older than the
// window restart the count; reaching MaxFailed locks for Window. It
// reports whether the account just became locked.
func (u *User) RegisterFailedLogin(now time.Time, p LockoutPolicy) bool {
	if u.LastFailedLoginAt == nil || now.Sub(*u.LastFailedLoginAt) > p.Window || u.lockExpired(now) {
		u.FailedLoginCount = 0
	}
	u.FailedLoginCount++
	t := now
	u.LastFailedLoginAt = &t
	if u.FailedLoginCount >= p.MaxFailed {
		until := now.Add(p.Window)
		u.LockedUntil = &until
		u.FailedLoginCount = 0
		return true
	}
	return false
}

func (u *User) lockExpired(now time.Time) bool {
	return u.LockedUntil != nil && !now.Before(*u.LockedUntil)
}

// RegisterSuccessfulLogin resets lockout state.
func (u *User) RegisterSuccessfulLogin(now time.Time) {
	u.FailedLoginCount = 0
	u.LastFailedLoginAt = nil
	u.LockedUntil = nil
	t := now
	u.LastLoginAt = &t
}

// IsActive reports whether requests on behalf of the user are allowed.
func (u *User) IsActive() bool { return u.Status == StatusActive }
