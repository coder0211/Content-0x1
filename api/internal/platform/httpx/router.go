// Package httpx is the HTTP framework layer: chi router, middleware
// (request ID, recover, access log, CORS, auth, CSRF, RBAC, idempotency),
// the response envelope and domain-error → HTTP mapping (SPEC-04 §1).
package httpx

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/be/acs/api/internal/platform/metrics"
	"github.com/be/acs/api/internal/shared/kernel"
)

// APIPrefix is the versioned base path.
const APIPrefix = "/api/v1"

// LoginPath is exempt from the CSRF check (no session yet).
const LoginPath = APIPrefix + "/auth/login"

// ReadinessCheck is one dependency probe for /readyz.
type ReadinessCheck struct {
	Name  string
	Check func(ctx context.Context) error
}

// Options configures the router.
type Options struct {
	Logger        *slog.Logger
	Metrics       *metrics.Metrics
	CORSOrigins   []string
	Authenticator Authenticator
	Cookies       CookieConfig
	Readiness     []ReadinessCheck
}

// NewRouter builds the root router with operational endpoints and returns it
// together with the /api/v1 sub-router on which modules register routes.
// The API router already runs Authn and CSRF; modules add RequireAuth /
// RequirePermission per route group.
func NewRouter(opts Options) (root chi.Router, api chi.Router) {
	r := chi.NewRouter()
	r.Use(RequestMeta(opts.Logger), AccessLog(opts.Metrics), Recover, SecurityHeaders, CORS(opts.CORSOrigins))
	r.NotFound(func(w http.ResponseWriter, r *http.Request) { Error(w, r, kernel.ErrNotFound) })
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, r, http.StatusMethodNotAllowed, kernel.NewError("METHOD_NOT_ALLOWED", "Phương thức không được hỗ trợ"))
	})

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) { OK(w, r, map[string]string{"status": "ok"}) })
	r.Get("/readyz", readyz(opts.Readiness))
	if opts.Metrics != nil {
		r.Method(http.MethodGet, "/metrics", opts.Metrics.Handler())
	}

	r.Route(APIPrefix, func(sub chi.Router) {
		if opts.Authenticator != nil {
			sub.Use(Authn(opts.Authenticator, opts.Cookies))
		}
		sub.Use(CSRF(LoginPath))
		api = sub
	})
	return r, api
}

func readyz(checks []ReadinessCheck) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		status := map[string]string{}
		ok := true
		for _, c := range checks {
			if err := c.Check(ctx); err != nil {
				status[c.Name] = "down"
				ok = false
				loggerFrom(r.Context()).WarnContext(r.Context(), "readiness check failed", "check", c.Name, "error", err.Error())
				continue
			}
			status[c.Name] = "up"
		}
		if !ok {
			writeError(w, r, http.StatusServiceUnavailable, kernel.ErrUnavailable.WithDetails(kernel.Detail{"checks": status}))
			return
		}
		OK(w, r, map[string]any{"status": "ready", "checks": status})
	}
}
