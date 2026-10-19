package http_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	identityhttp "github.com/be/acs/api/internal/modules/identity/adapter/http"
	"github.com/be/acs/api/internal/modules/identity/domain"
	"github.com/be/acs/api/internal/modules/identity/identitytest"
	"github.com/be/acs/api/internal/modules/identity/usecase"
	"github.com/be/acs/api/internal/platform/httpx"
	"github.com/be/acs/api/internal/shared/kernel"
	"github.com/be/acs/api/internal/testutil"
)

var cookies = httpx.CookieConfig{SessionName: "acs_session", CSRFName: "acs_csrf"}

func setup(t *testing.T) (http.Handler, *usecase.Users) {
	t.Helper()
	ur, sr, clk, tx, au := identitytest.NewUsers(), identitytest.NewSessions(), testutil.NewClock(), &testutil.Tx{}, &testutil.Audit{}
	auth := &usecase.Auth{Tx: tx, Users: ur, Sessions: sr, Hasher: identitytest.PlainHasher{}, Clock: clk, Audit: au,
		Lockout: domain.LockoutPolicy{MaxFailed: 5, Window: 15 * time.Minute},
		Session: domain.SessionPolicy{Idle: 12 * time.Hour, Absolute: 7 * 24 * time.Hour}}
	users := &usecase.Users{Tx: tx, Users: ur, Sessions: sr, Hasher: identitytest.PlainHasher{}, Clock: clk, Audit: au}
	root, api := httpx.NewRouter(httpx.Options{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Authenticator: identityhttp.Authenticator{Auth: auth}, Cookies: cookies})
	(&identityhttp.Handler{Auth: auth, Users: users, Cookies: cookies}).Routes(api)
	return root, users
}

type client struct {
	t       *testing.T
	h       http.Handler
	cookies []*http.Cookie
	csrf    string
}

func (c *client) do(method, path, body string) (*httptest.ResponseRecorder, map[string]any) {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for _, ck := range c.cookies {
		req.AddCookie(ck)
	}
	if c.csrf != "" {
		req.Header.Set(httpx.HeaderCSRF, c.csrf)
	}
	rec := httptest.NewRecorder()
	c.h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}

func errCode(m map[string]any) string {
	if e, ok := m["error"].(map[string]any); ok {
		return e["code"].(string)
	}
	return ""
}

func TestLoginMeLogoutFlow(t *testing.T) {
	h, users := setup(t)
	if _, err := users.Bootstrap(context.Background(), usecase.CreateUserInput{Email: "lead@acs.local", Name: "Lead", RoleCodes: []string{kernel.RoleMarketingLead}, TempPassword: "correct-horse"}); err != nil {
		t.Fatal(err)
	}
	c := &client{t: t, h: h}
	rec, body := c.do("POST", "/api/v1/auth/login", `{"email":"lead@acs.local","password":"correct-horse"}`)
	if rec.Code != 200 {
		t.Fatalf("login %d %v", rec.Code, body)
	}
	c.cookies = rec.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, ck := range c.cookies {
		if ck.Name == "acs_session" {
			sessionCookie = ck
		}
	}
	if sessionCookie == nil || !sessionCookie.HttpOnly || sessionCookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("session cookie flags: %+v", sessionCookie)
	}
	c.csrf = body["data"].(map[string]any)["csrf_token"].(string)

	rec, body = c.do("GET", "/api/v1/me", "")
	perms := body["data"].(map[string]any)["permissions"].([]any)
	if rec.Code != 200 || len(perms) != len(kernel.RolePermissions[kernel.RoleMarketingLead]) {
		t.Fatalf("me %d %v", rec.Code, body)
	}
	// Lead lacks user.manage → 403 even though the UI would hide the menu.
	if rec, body = c.do("GET", "/api/v1/users", ""); rec.Code != 403 || errCode(body) != kernel.CodeForbidden {
		t.Fatalf("users %d %v", rec.Code, body)
	}
	csrf := c.csrf
	c.csrf = ""
	if rec, body = c.do("POST", "/api/v1/auth/logout", ""); rec.Code != 403 || errCode(body) != kernel.CodeCSRFInvalid {
		t.Fatalf("logout without csrf %d %v", rec.Code, body)
	}
	c.csrf = csrf
	if rec, _ = c.do("POST", "/api/v1/auth/logout", ""); rec.Code != 204 {
		t.Fatalf("logout %d", rec.Code)
	}
	if rec, body = c.do("GET", "/api/v1/me", ""); rec.Code != 401 || errCode(body) != kernel.CodeSessionExpired {
		t.Fatalf("after logout %d %v", rec.Code, body)
	}
}

func TestLoginLockoutReturns423(t *testing.T) {
	h, users := setup(t)
	_, _ = users.Bootstrap(context.Background(), usecase.CreateUserInput{Email: "a@acs.local", Name: "A", RoleCodes: []string{kernel.RoleReviewer}, TempPassword: "correct-horse"})
	c := &client{t: t, h: h}
	var rec *httptest.ResponseRecorder
	for i := 0; i < 5; i++ {
		rec, _ = c.do("POST", "/api/v1/auth/login", `{"email":"a@acs.local","password":"nope"}`)
	}
	if rec.Code != 423 {
		t.Fatalf("5th wrong password: %d", rec.Code)
	}
}

func TestAdminCreatesUser(t *testing.T) {
	h, users := setup(t)
	_, _ = users.Bootstrap(context.Background(), usecase.CreateUserInput{Email: "admin@acs.local", Name: "Admin", RoleCodes: []string{kernel.RoleAdmin}, TempPassword: "correct-horse"})
	c := &client{t: t, h: h}
	rec, body := c.do("POST", "/api/v1/auth/login", `{"email":"admin@acs.local","password":"correct-horse"}`)
	c.cookies = rec.Result().Cookies()
	c.csrf = body["data"].(map[string]any)["csrf_token"].(string)
	rec, body = c.do("POST", "/api/v1/users", `{"email":"new@acs.local","name":"Mới","role_codes":["content_creator"],"temp_password":"temporary-pw"}`)
	if rec.Code != 201 {
		t.Fatalf("%d %v", rec.Code, body)
	}
	rec, body = c.do("POST", "/api/v1/users", `{"email":"new@acs.local","name":"Mới","role_codes":["content_creator"],"temp_password":"temporary-pw"}`)
	if rec.Code != 409 {
		t.Fatalf("duplicate: %d %v", rec.Code, body)
	}
	rec, body = c.do("GET", "/api/v1/users?limit=1", "")
	if rec.Code != 200 || body["meta"].(map[string]any)["next_cursor"] == nil {
		t.Fatalf("list %d %v", rec.Code, body)
	}
}
