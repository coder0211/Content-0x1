package http_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	nethttp "github.com/be/acs/api/internal/modules/_template/adapter/http"
	"github.com/be/acs/api/internal/modules/_template/usecase"
	"github.com/be/acs/api/internal/platform/httpx"
	"github.com/be/acs/api/internal/shared/kernel"
	"github.com/be/acs/api/internal/testutil"
)

type auth struct{ actor kernel.Actor }

func (a auth) Authenticate(context.Context, string) (kernel.Actor, httpx.Session, error) {
	return a.actor, httpx.Session{CSRFToken: "t"}, nil
}

func TestCreateNote(t *testing.T) {
	notes := &usecase.Notes{Tx: &testutil.Tx{}, Repo: usecase.NewMemoryRepo(), Clock: testutil.NewClock(), Audit: &testutil.Audit{}}
	for _, tc := range []struct {
		role   string
		body   string
		status int
	}{
		{kernel.RoleContentCreator, `{"title":"Ý tưởng"}`, http.StatusCreated},
		{kernel.RoleContentCreator, `{"title":""}`, http.StatusBadRequest},
		{kernel.RoleReviewer, `{"title":"x"}`, http.StatusForbidden},
	} {
		root, api := httpx.NewRouter(httpx.Options{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
			Authenticator: auth{testutil.Actor(tc.role)}, Cookies: httpx.CookieConfig{SessionName: "s"}})
		(&nethttp.Handler{Notes: notes}).Routes(api)
		req := httptest.NewRequest("POST", "/api/v1/notes", strings.NewReader(tc.body))
		req.AddCookie(&http.Cookie{Name: "s", Value: "x"})
		req.Header.Set(httpx.HeaderCSRF, "t")
		rec := httptest.NewRecorder()
		root.ServeHTTP(rec, req)
		if rec.Code != tc.status {
			t.Errorf("%s %s: %d %s", tc.role, tc.body, rec.Code, rec.Body)
		}
	}
}
