package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"time"

	"github.com/be/acs/api/internal/modules/identity/domain"
	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Auth implements login, logout, session authentication and /me.
type Auth struct {
	Tx       ports.TxManager
	Users    UserRepository
	Sessions SessionRepository
	Hasher   PasswordHasher
	Clock    ports.Clock
	Audit    ports.AuditRecorder
	Lockout  domain.LockoutPolicy
	Session  domain.SessionPolicy
	// dummyHash equalizes timing for unknown e-mails.
	dummyHash string
}

// LoginInput is the login request.
type LoginInput struct {
	Email     string
	Password  string
	IP        string
	UserAgent string
}

// LoginOutput carries the raw token for the cookie; it is never stored.
type LoginOutput struct {
	User      Me
	Token     string
	CSRFToken string
	ExpiresAt time.Time
}

// Me is the current user with effective permissions (GET /me).
type Me struct {
	ID          kernel.ID
	Email       string
	Name        string
	Roles       []string
	Permissions []string
}

// AuthSession is what HTTP middleware needs from Authenticate.
type AuthSession struct {
	ID        kernel.ID
	CSRFToken string
}

func randomToken() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b[:])
}

// HashToken is the stored form of a session token.
func HashToken(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

// Login verifies credentials, applies lockout and opens a session. Wrong
// e-mail and wrong password are indistinguishable (INVALID_CREDENTIALS).
func (a *Auth) Login(ctx context.Context, in LoginInput) (LoginOutput, error) {
	email := domain.NormalizeEmail(in.Email)
	if email == "" || in.Password == "" {
		return LoginOutput{}, kernel.ErrInvalidCredentials
	}
	var out LoginOutput
	var loginErr error
	err := a.Tx.WithinTx(ctx, func(ctx context.Context) error {
		u, err := a.Users.GetByEmail(ctx, email)
		if errors.Is(err, kernel.ErrNotFound) {
			a.burnHash(in.Password)
			loginErr = kernel.ErrInvalidCredentials
			return nil //nolint:nilerr // unknown e-mail is reported via loginErr, indistinguishable from a wrong password
		}
		if err != nil {
			return err
		}
		if u, err = a.Users.GetForUpdate(ctx, u.ID); err != nil {
			return err
		}
		now := a.Clock.Now()
		if err := u.CheckCanLogin(now); err != nil {
			loginErr = err
			return nil //nolint:nilerr // business rejection; nothing to roll back
		}
		ok, err := a.Hasher.Verify(in.Password, u.PasswordHash)
		if err != nil {
			return err
		}
		if !ok {
			// Commit the failure counter, then report the error.
			locked := u.RegisterFailedLogin(now, a.Lockout)
			loginErr = kernel.ErrInvalidCredentials
			if locked {
				loginErr = kernel.ErrAccountLocked
				ctx = kernel.WithActor(ctx, kernel.Actor{UserID: u.ID})
				if err := a.Audit.Record(ctx, "user.lockout", "user", u.ID.String(), nil, map[string]any{"locked_until": u.LockedUntil}); err != nil {
					return err
				}
			}
			return a.Users.UpdateLoginState(ctx, u)
		}
		u.RegisterSuccessfulLogin(now)
		if err := a.Users.UpdateLoginState(ctx, u); err != nil {
			return err
		}
		token, csrf := randomToken(), randomToken()
		s := domain.NewSession(u.ID, HashToken(token), csrf, in.IP, in.UserAgent, now, a.Session)
		if err := a.Sessions.Create(ctx, s); err != nil {
			return err
		}
		perms, err := a.Users.Permissions(ctx, u.ID)
		if err != nil {
			return err
		}
		out = LoginOutput{
			User:  Me{ID: u.ID, Email: u.Email, Name: u.Name, Roles: u.Roles, Permissions: perms},
			Token: token, CSRFToken: csrf, ExpiresAt: s.ExpiresAt,
		}
		return nil
	})
	if err != nil {
		return LoginOutput{}, err
	}
	return out, loginErr
}

func (a *Auth) burnHash(pw string) {
	if a.dummyHash == "" {
		a.dummyHash, _ = a.Hasher.Hash("not-a-real-password")
	}
	_, _ = a.Hasher.Verify(pw, a.dummyHash)
}

// Logout revokes the session.
func (a *Auth) Logout(ctx context.Context, sessionID kernel.ID) error {
	return a.Sessions.Revoke(ctx, sessionID, a.Clock.Now())
}

// Authenticate resolves a cookie token to an actor. Invalid, expired,
// revoked tokens and inactive users yield SESSION_EXPIRED / UNAUTHENTICATED.
func (a *Auth) Authenticate(ctx context.Context, token string) (kernel.Actor, AuthSession, error) {
	if token == "" {
		return kernel.Actor{}, AuthSession{}, kernel.ErrUnauthenticated
	}
	s, err := a.Sessions.GetByTokenHash(ctx, HashToken(token))
	if errors.Is(err, kernel.ErrNotFound) {
		return kernel.Actor{}, AuthSession{}, kernel.ErrUnauthenticated
	}
	if err != nil {
		return kernel.Actor{}, AuthSession{}, err
	}
	now := a.Clock.Now()
	if err := s.Validate(now, a.Session); err != nil {
		return kernel.Actor{}, AuthSession{}, err
	}
	u, err := a.Users.GetByID(ctx, s.UserID)
	if err != nil {
		return kernel.Actor{}, AuthSession{}, err
	}
	if !u.IsActive() {
		return kernel.Actor{}, AuthSession{}, kernel.ErrSessionExpired
	}
	perms, err := a.Users.Permissions(ctx, u.ID)
	if err != nil {
		return kernel.Actor{}, AuthSession{}, err
	}
	if s.NeedsTouch(now) {
		if err := a.Sessions.Touch(ctx, s.ID, now); err != nil {
			return kernel.Actor{}, AuthSession{}, err
		}
	}
	actor := kernel.Actor{UserID: u.ID, Email: u.Email, Name: u.Name, Roles: u.Roles, Permissions: perms, SessionID: s.ID}
	return actor, AuthSession{ID: s.ID, CSRFToken: s.CSRFToken}, nil
}

// GetMe returns the actor's profile and permissions.
func (a *Auth) GetMe(_ context.Context, actor kernel.Actor) (Me, error) {
	if !actor.IsAuthenticated() {
		return Me{}, kernel.ErrUnauthenticated
	}
	return Me{ID: actor.UserID, Email: actor.Email, Name: actor.Name, Roles: actor.Roles, Permissions: actor.Permissions}, nil
}
