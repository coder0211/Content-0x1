package http_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/be/acs/api/internal/modules/asset/assettest"
	dashhttp "github.com/be/acs/api/internal/modules/dashboard/adapter/http"
	"github.com/be/acs/api/internal/modules/dashboard/usecase"
	"github.com/be/acs/api/internal/platform/httpx"
	"github.com/be/acs/api/internal/shared/kernel"
	"github.com/be/acs/api/internal/testutil"
)

type auth struct{ actor kernel.Actor }

func (a auth) Authenticate(context.Context, string) (kernel.Actor, httpx.Session, error) {
	return a.actor, httpx.Session{CSRFToken: "t"}, nil
}

type reader struct{}

func (reader) ActiveCampaigns(context.Context, int) (int, []usecase.Campaign, error) {
	return 0, nil, nil
}
func (reader) JobCounts(context.Context, *kernel.ID, time.Time) (usecase.JobCounts, error) {
	return usecase.JobCounts{Running: 1}, nil
}
func (reader) RecentJobs(context.Context, *kernel.ID, time.Time, int) ([]usecase.Job, error) {
	return []usecase.Job{{ID: kernel.NewID(), JobType: "content_generation", Status: "running", Progress: 40}}, nil
}
func (reader) Spent(context.Context, time.Time) (decimal.Decimal, error) {
	return decimal.RequireFromString("0.42"), nil
}
func (reader) MonthlyLimit(context.Context, time.Time) (*decimal.Decimal, error) { return nil, nil }

func TestSummaryEndpoint(t *testing.T) {
	svc := &usecase.Service{Reader: reader{}, Assets: assettest.NewAPI(), Clock: testutil.NewClock()}
	for _, tc := range []struct {
		role string
		want []string
	}{
		{kernel.RoleContentCreator, []string{`"cost":null`, `"pending_review":null`, `"scope":"own"`, `"recent":[]`, `"error_code":null`}},
		{kernel.RoleMarketingLead, []string{`"today":0.42`, `"monthly_limit":null`, `"pending_review":0`, `"scope":"all"`}},
	} {
		root, api := httpx.NewRouter(httpx.Options{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
			Authenticator: auth{testutil.Actor(tc.role)}, Cookies: httpx.CookieConfig{SessionName: "s"}})
		(&dashhttp.Handler{Svc: svc}).Routes(api)
		req := httptest.NewRequest("GET", "/api/v1/dashboard/summary", nil)
		req.AddCookie(&http.Cookie{Name: "s", Value: "x"})
		rec := httptest.NewRecorder()
		root.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", tc.role, rec.Code, rec.Body)
		}
		for _, w := range tc.want {
			if !strings.Contains(rec.Body.String(), w) {
				t.Errorf("%s: missing %s in %s", tc.role, w, rec.Body)
			}
		}
	}
	// Anonymous → 401.
	root, api := httpx.NewRouter(httpx.Options{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Authenticator: auth{}, Cookies: httpx.CookieConfig{SessionName: "s"}})
	(&dashhttp.Handler{Svc: svc}).Routes(api)
	rec := httptest.NewRecorder()
	root.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/dashboard/summary", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", rec.Code)
	}
}
