package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/be/acs/api/internal/platform/log"
	"github.com/be/acs/api/internal/platform/metrics"
	"github.com/be/acs/api/internal/shared/kernel"
)

type fakeAuth struct{}

func (fakeAuth) Authenticate(_ context.Context, token string) (kernel.Actor, Session, error) {
	switch token {
	case "admin":
		return kernel.Actor{UserID: kernel.NewID(), Roles: []string{kernel.RoleAdmin}, Permissions: kernel.AllPermissions}, Session{ID: kernel.NewID(), CSRFToken: "csrf-1"}, nil
	case "creator":
		return kernel.Actor{UserID: kernel.NewID(), Permissions: kernel.RolePermissions[kernel.RoleContentCreator]}, Session{ID: kernel.NewID(), CSRFToken: "csrf-2"}, nil
	case "expired":
		return kernel.Actor{}, Session{}, kernel.ErrSessionExpired
	}
	return kernel.Actor{}, Session{}, kernel.ErrUnauthenticated
}

var cookies = CookieConfig{SessionName: "acs_session", CSRFName: "acs_csrf"}

func newTestRouter(t *testing.T, buf *bytes.Buffer) chi.Router {
	t.Helper()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	if buf != nil {
		logger = log.NewWithWriter(buf, "debug")
	}
	root, api := NewRouter(Options{Logger: logger, Metrics: metrics.New(), Authenticator: fakeAuth{}, Cookies: cookies, CORSOrigins: []string{"http://localhost:5173"}})
	api.Get("/panic", func(http.ResponseWriter, *http.Request) { panic("boom") })
	api.Group(func(r chi.Router) {
		r.Use(RequireAuth)
		r.Post("/things", func(w http.ResponseWriter, r *http.Request) { Created(w, r, map[string]string{"ok": "1"}) })
		r.With(RequirePermission(kernel.PermUserManage)).Get("/admin-only", func(w http.ResponseWriter, r *http.Request) { OK(w, r, "ok") })
		r.With(RequireIdempotencyKey).Post("/jobs", func(w http.ResponseWriter, r *http.Request) {
			idem, _ := IdempotencyFrom(r)
			var body map[string]any
			if err := Decode(r, &body); err != nil {
				Error(w, r, err)
				return
			}
			Accepted(w, r, idem)
		})
	})
	api.Post("/strict", func(w http.ResponseWriter, r *http.Request) {
		var v struct {
			Name string `json:"name"`
		}
		if err := Decode(r, &v); err != nil {
			Error(w, r, err)
			return
		}
		OK(w, r, v)
	})
	return root
}

type errEnv struct {
	Error struct {
		Code    string          `json:"code"`
		Message string          `json:"message"`
		Details []kernel.Detail `json:"details"`
	} `json:"error"`
	Meta struct {
		RequestID string `json:"request_id"`
	} `json:"meta"`
}

func do(t *testing.T, h http.Handler, method, path, body string, hdr map[string]string) (*httptest.ResponseRecorder, errEnv) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range hdr {
		if k == "Cookie" {
			req.AddCookie(&http.Cookie{Name: "acs_session", Value: v})
			continue
		}
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var env errEnv
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	return rec, env
}

func TestPanicReturns500EnvelopeWithRequestID(t *testing.T) {
	var buf bytes.Buffer
	h := newTestRouter(t, &buf)
	rec, env := do(t, h, "GET", "/api/v1/panic", "", map[string]string{"Cookie": "admin", "Authorization": "Bearer top-secret"})
	if rec.Code != 500 || env.Error.Code != kernel.CodeInternal {
		t.Fatalf("got %d %+v", rec.Code, env)
	}
	if env.Meta.RequestID == "" || env.Meta.RequestID != rec.Header().Get(HeaderRequestID) {
		t.Fatalf("request id mismatch: %q vs %q", env.Meta.RequestID, rec.Header().Get(HeaderRequestID))
	}
	if strings.Contains(buf.String(), "top-secret") || strings.Contains(buf.String(), "acs_session") {
		t.Fatalf("log leaked headers: %s", buf.String())
	}
}

func TestAccessLogRedactsSignedURL(t *testing.T) {
	var buf bytes.Buffer
	h := newTestRouter(t, &buf)
	do(t, h, "GET", "/api/v1/nope?X-Amz-Signature=abcdef123", "", nil)
	if strings.Contains(buf.String(), "abcdef123") {
		t.Fatalf("signature leaked: %s", buf.String())
	}
}

func TestAuthAndCSRF(t *testing.T) {
	h := newTestRouter(t, nil)
	cases := []struct {
		name   string
		method string
		path   string
		hdr    map[string]string
		status int
		code   string
	}{
		{"anonymous", "POST", "/api/v1/things", nil, 401, kernel.CodeUnauthenticated},
		{"expired session", "POST", "/api/v1/things", map[string]string{"Cookie": "expired"}, 401, kernel.CodeSessionExpired},
		{"missing csrf", "POST", "/api/v1/things", map[string]string{"Cookie": "admin"}, 403, kernel.CodeCSRFInvalid},
		{"wrong csrf", "POST", "/api/v1/things", map[string]string{"Cookie": "admin", HeaderCSRF: "csrf-2"}, 403, kernel.CodeCSRFInvalid},
		{"valid csrf", "POST", "/api/v1/things", map[string]string{"Cookie": "admin", HeaderCSRF: "csrf-1"}, 201, ""},
		{"permission denied", "GET", "/api/v1/admin-only", map[string]string{"Cookie": "creator"}, 403, kernel.CodeForbidden},
		{"permission granted", "GET", "/api/v1/admin-only", map[string]string{"Cookie": "admin"}, 200, ""},
		{"not found envelope", "GET", "/api/v1/missing", nil, 404, kernel.CodeNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec, env := do(t, h, tc.method, tc.path, "", tc.hdr)
			if rec.Code != tc.status || env.Error.Code != tc.code {
				t.Fatalf("got %d %q, want %d %q", rec.Code, env.Error.Code, tc.status, tc.code)
			}
		})
	}
}

func TestIdempotencyKey(t *testing.T) {
	h := newTestRouter(t, nil)
	auth := map[string]string{"Cookie": "admin", HeaderCSRF: "csrf-1"}
	rec, env := do(t, h, "POST", "/api/v1/jobs", `{"a":1}`, auth)
	if rec.Code != 400 || env.Error.Code != kernel.CodeIdempotencyKeyRequired {
		t.Fatalf("got %d %+v", rec.Code, env)
	}
	auth[HeaderIdempotencyKey] = "6f1c1b0e-1f7e-4a53-9d0c-6f6d0e9b6b11"
	rec1, _ := do(t, h, "POST", "/api/v1/jobs", `{"a":1}`, auth)
	rec2, _ := do(t, h, "POST", "/api/v1/jobs", `{"a":2}`, auth)
	if rec1.Code != 202 {
		t.Fatalf("status %d: %s", rec1.Code, rec1.Body)
	}
	var a, b struct{ Data Idempotency }
	_ = json.Unmarshal(rec1.Body.Bytes(), &a)
	_ = json.Unmarshal(rec2.Body.Bytes(), &b)
	if a.Data.RequestHash == "" || a.Data.RequestHash == b.Data.RequestHash {
		t.Fatal("different bodies must hash differently")
	}
}

func TestDecodeRejectsUnknownFields(t *testing.T) {
	h := newTestRouter(t, nil)
	rec, env := do(t, h, "POST", "/api/v1/strict", `{"name":"x","evil":1}`, nil)
	if rec.Code != 400 || env.Error.Code != kernel.CodeValidation || env.Error.Details[0]["field"] != "evil" {
		t.Fatalf("got %d %+v", rec.Code, env)
	}
}

func TestErrorMapping(t *testing.T) {
	want := map[string]int{
		kernel.CodeValidation: 400, kernel.CodeSessionExpired: 401, kernel.CodeSelfApproval: 403,
		kernel.CodeVersionConflict: 409, kernel.CodeFileTooLarge: 413, kernel.CodeUnsupportedMediaType: 415,
		kernel.CodeCapabilityNotSupported: 422, kernel.CodeAccountLocked: 423, kernel.CodeJobBudgetExceeded: 429,
		kernel.CodeProviderError: 502, kernel.CodePricingMissing: 503, "SOMETHING_NEW": 400,
	}
	for code, status := range want {
		if got := StatusFor(code); got != status {
			t.Errorf("%s: got %d want %d", code, got, status)
		}
	}
}

func TestCORSPreflight(t *testing.T) {
	h := newTestRouter(t, nil)
	rec, _ := do(t, h, "OPTIONS", "/api/v1/things", "", map[string]string{"Origin": "http://localhost:5173", "Access-Control-Request-Method": "POST"})
	if rec.Code != 204 || rec.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatalf("preflight: %d %v", rec.Code, rec.Header())
	}
	rec, _ = do(t, h, "OPTIONS", "/api/v1/things", "", map[string]string{"Origin": "http://evil", "Access-Control-Request-Method": "POST"})
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("foreign origin allowed")
	}
}
