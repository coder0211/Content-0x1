// Package identity wires the identity module: authentication (sessions,
// argon2id, CSRF, lockout), RBAC (/me) and user management.
package identity

import (
	"errors"

	"github.com/go-chi/chi/v5"

	"github.com/be/acs/api/internal/app/registry"
	"github.com/be/acs/api/internal/modules/identity/adapter/http"
	"github.com/be/acs/api/internal/modules/identity/adapter/password"
	"github.com/be/acs/api/internal/modules/identity/adapter/postgres"
	"github.com/be/acs/api/internal/modules/identity/domain"
	"github.com/be/acs/api/internal/modules/identity/usecase"
	"github.com/be/acs/api/internal/platform/httpx"
)

// Module is the identity module.
type Module struct {
	Auth    *usecase.Auth
	Users   *usecase.Users
	handler *http.Handler
}

// Name implements registry.Module.
func (*Module) Name() string { return "identity" }

// Routes implements registry.HTTPModule.
func (m *Module) Routes(r chi.Router) { m.handler.Routes(r) }

// Register builds the module and publishes Identity + Authenticator.
func Register(c *registry.Container) (registry.Module, error) {
	if c.APIs.Audit == nil {
		return nil, errors.New("identity: audit module must be registered first")
	}
	users := postgres.NewUserRepository(c.DB)
	sessions := postgres.NewSessionRepository(c.DB)
	hasher := password.New()
	cfg := c.Config.Session
	auth := &usecase.Auth{
		Tx: c.Tx, Users: users, Sessions: sessions, Hasher: hasher, Clock: c.Clock, Audit: c.APIs.Audit,
		Lockout: domain.LockoutPolicy{MaxFailed: cfg.MaxFailed, Window: cfg.LockoutFor},
		Session: domain.SessionPolicy{Idle: cfg.IdleTimeout, Absolute: cfg.Absolute},
	}
	um := &usecase.Users{Tx: c.Tx, Users: users, Sessions: sessions, Hasher: hasher, Clock: c.Clock, Audit: c.APIs.Audit}
	m := &Module{Auth: auth, Users: um, handler: &http.Handler{Auth: auth, Users: um, Cookies: Cookies(c)}}
	c.APIs.Identity = um
	c.APIs.Authenticator = http.Authenticator{Auth: auth}
	return m, nil
}

// Cookies derives the cookie configuration from config.
func Cookies(c *registry.Container) httpx.CookieConfig {
	s := c.Config.Session
	return httpx.CookieConfig{SessionName: s.CookieName, CSRFName: s.CSRFCookie, Secure: s.CookieSecure, Domain: s.CookieDomain}
}
