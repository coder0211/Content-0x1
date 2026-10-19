package httpx

import (
	"context"
	"crypto/subtle"
	"net/http"
	"time"

	"github.com/be/acs/api/internal/shared/kernel"
)

// HeaderCSRF carries the CSRF token on every unsafe request.
const HeaderCSRF = "X-CSRF-Token"

// Session is what the auth middleware needs to know about a session.
type Session struct {
	ID        kernel.ID
	CSRFToken string
}

// Authenticator resolves a session cookie token. Implemented by the identity
// module. It returns kernel.ErrSessionExpired / kernel.ErrUnauthenticated
// for invalid tokens.
type Authenticator interface {
	Authenticate(ctx context.Context, token string) (kernel.Actor, Session, error)
}

// CookieConfig names and flags the session cookies.
type CookieConfig struct {
	SessionName string
	CSRFName    string
	Secure      bool
	Domain      string
}

type sessionKey struct{}
type authErrKey struct{}

// Authn resolves the session cookie, if any, and stores the actor and
// session in ctx. It never rejects: RequireAuth does, so public routes keep
// working with a stale cookie.
func Authn(auth Authenticator, cookies CookieConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := r.Cookie(cookies.SessionName)
			if err != nil || c.Value == "" {
				next.ServeHTTP(w, r)
				return
			}
			actor, sess, err := auth.Authenticate(r.Context(), c.Value)
			ctx := r.Context()
			if err != nil {
				ctx = context.WithValue(ctx, authErrKey{}, err)
			} else {
				ctx = kernel.WithActor(ctx, actor)
				ctx = context.WithValue(ctx, sessionKey{}, sess)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ActorFrom returns the authenticated actor (zero Actor when anonymous).
func ActorFrom(r *http.Request) kernel.Actor { return kernel.ActorFrom(r.Context()) }

// SessionFrom returns the current session, if authenticated.
func SessionFrom(r *http.Request) (Session, bool) {
	s, ok := r.Context().Value(sessionKey{}).(Session)
	return s, ok
}

// RequireAuth rejects anonymous requests with 401 (SESSION_EXPIRED when a
// cookie was sent but is no longer valid).
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !ActorFrom(r).IsAuthenticated() {
			if err, ok := r.Context().Value(authErrKey{}).(error); ok && kernel.ErrorCode(err) == kernel.CodeSessionExpired {
				Error(w, r, kernel.ErrSessionExpired)
				return
			}
			Error(w, r, kernel.ErrUnauthenticated)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequirePermission rejects requests whose actor lacks perm (403). It
// implies RequireAuth. Ownership (IDOR) checks still belong in use cases.
func RequirePermission(perm string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !ActorFrom(r).Can(perm) {
				Error(w, r, kernel.ErrForbidden)
				return
			}
			next.ServeHTTP(w, r)
		}))
	}
}

// CSRF enforces the double-submit token on unsafe methods for
// authenticated requests: X-CSRF-Token must equal the session's token (also
// exposed to the SPA in the non-HttpOnly CSRF cookie). Paths in exempt
// (e.g. login) are skipped.
func CSRF(exempt ...string) func(http.Handler) http.Handler {
	skip := map[string]bool{}
	for _, p := range exempt {
		skip[p] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				next.ServeHTTP(w, r)
				return
			}
			sess, ok := SessionFrom(r)
			if !ok || skip[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}
			got := r.Header.Get(HeaderCSRF)
			if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(sess.CSRFToken)) != 1 {
				Error(w, r, kernel.ErrCSRFInvalid)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// SetSessionCookies writes the HttpOnly session cookie and the readable CSRF
// cookie. Both are SameSite=Lax; Secure follows configuration.
func SetSessionCookies(w http.ResponseWriter, c CookieConfig, token, csrf string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // G124: Secure is configuration-driven (forced on in production by config.validate)
		Name: c.SessionName, Value: token, Path: "/", Domain: c.Domain, Expires: expires,
		HttpOnly: true, Secure: c.Secure, SameSite: http.SameSiteLaxMode,
	})
	// The CSRF cookie is deliberately readable by the SPA (double-submit).
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // G124: HttpOnly=false by design; Secure is configuration-driven
		Name: c.CSRFName, Value: csrf, Path: "/", Domain: c.Domain, Expires: expires,
		HttpOnly: false, Secure: c.Secure, SameSite: http.SameSiteLaxMode,
	})
}

// ClearSessionCookies expires both cookies.
func ClearSessionCookies(w http.ResponseWriter, c CookieConfig) {
	for _, name := range []string{c.SessionName, c.CSRFName} {
		http.SetCookie(w, &http.Cookie{ //nolint:gosec // G124: expiring cookies; flags mirror SetSessionCookies
			Name: name, Value: "", Path: "/", Domain: c.Domain, MaxAge: -1,
			HttpOnly: name == c.SessionName, Secure: c.Secure, SameSite: http.SameSiteLaxMode,
		})
	}
}
