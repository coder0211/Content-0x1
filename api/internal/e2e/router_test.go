// Package e2e holds cross-module tests: the composition of every module,
// the RBAC matrix over the real chi router (ACS-100), IDOR checks and an
// in-process run of Flow A. All tests run without Docker.
package e2e

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/be/acs/api/internal/app"
	"github.com/be/acs/api/internal/app/registry"
	"github.com/be/acs/api/internal/platform/config"
	"github.com/be/acs/api/internal/platform/httpx"
	"github.com/be/acs/api/internal/platform/postgres"
	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
	"github.com/be/acs/api/internal/testutil"
)

// csrfToken is the session CSRF token every fake session carries.
const csrfToken = "csrf-test-token"

// tokenAuth resolves the session cookie value to a fixed actor.
type tokenAuth map[string]kernel.Actor

func (a tokenAuth) Authenticate(_ context.Context, token string) (kernel.Actor, httpx.Session, error) {
	actor, ok := a[token]
	if !ok {
		return kernel.Actor{}, httpx.Session{}, kernel.ErrSessionExpired
	}
	return actor, httpx.Session{ID: kernel.NewID(), CSRFToken: csrfToken}, nil
}

var testCookies = httpx.CookieConfig{SessionName: "acs_session", CSRFName: "acs_csrf"}

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// unreachablePool is a lazily-connecting pool whose every query fails fast
// (connection refused). With it every module registers its production
// wiring (e.g. provider routes need c.DB != nil) without Docker; requests
// that pass the RBAC gate end in a 5xx/4xx, never in 401/403.
func unreachablePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	cfg, err := pgxpool.ParseConfig("postgres://acs:acs@127.0.0.1:1/acs?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// realRouter composes app.Modules exactly like cmd/server (same Routes
// calls, same middleware) over an unreachable database.
func realRouter(t *testing.T, auth httpx.Authenticator) (chi.Router, *registry.Container) {
	t.Helper()
	pool := unreachablePool(t)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	c := &registry.Container{Platform: registry.Platform{
		Process: registry.ProcessServer, Config: cfg, Logger: discardLogger(), DB: pool,
		Tx: postgres.NewTxManager(pool), Clock: ports.SystemClock{}, Storage: testutil.NewStorage(),
	}}
	mods, err := app.Compose(c, app.Modules)
	if err != nil {
		t.Fatal(err)
	}
	root, api := httpx.NewRouter(httpx.Options{Logger: c.Logger, Authenticator: auth, Cookies: testCookies})
	for _, m := range mods {
		if hm, ok := m.(registry.HTTPModule); ok {
			hm.Routes(api)
		}
	}
	return root, c
}

// route is one registered METHOD + pattern (pattern relative to /api/v1).
type route struct{ Method, Pattern string }

func (r route) String() string { return r.Method + " " + r.Pattern }

// apiRoutes lists every /api/v1 route registered on root, via chi.Walk, so
// routes added later are covered automatically.
func apiRoutes(t *testing.T, root chi.Router) []route {
	t.Helper()
	var out []route
	err := chi.Walk(root, func(method, pattern string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if p, ok := strings.CutPrefix(pattern, httpx.APIPrefix); ok {
			out = append(out, route{method, strings.TrimSuffix(p, "/")})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}

// concretePath fills chi URL parameters with values of the right shape.
func concretePath(pattern string, id kernel.ID) string {
	parts := strings.Split(pattern, "/")
	for i, p := range parts {
		if strings.HasPrefix(p, "{") && strings.HasSuffix(p, "}") {
			switch strings.Trim(p, "{}") {
			case "versionNo":
				parts[i] = "1"
			default:
				parts[i] = id.String()
			}
		}
	}
	return httpx.APIPrefix + strings.Join(parts, "/")
}
