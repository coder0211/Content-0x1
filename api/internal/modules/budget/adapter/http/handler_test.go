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

	"github.com/shopspring/decimal"

	nethttp "github.com/be/acs/api/internal/modules/budget/adapter/http"
	"github.com/be/acs/api/internal/modules/budget/budgettest"
	"github.com/be/acs/api/internal/modules/budget/usecase"
	"github.com/be/acs/api/internal/platform/httpx"
	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
	"github.com/be/acs/api/internal/testutil"
)

type auth struct{ actor kernel.Actor }

func (a auth) Authenticate(context.Context, string) (kernel.Actor, httpx.Session, error) {
	return a.actor, httpx.Session{CSRFToken: "t"}, nil
}

type fixture struct {
	store *budgettest.Store
	svc   *usecase.Service
	audit *testutil.Audit
}

func newFixture() *fixture {
	st := budgettest.NewStore()
	f := &fixture{store: st, audit: &testutil.Audit{}}
	f.svc = &usecase.Service{Tx: st, Repo: st, Clock: testutil.NewClock(), Audit: f.audit, Notifier: &testutil.Notifier{}}
	return f
}

func (f *fixture) do(t *testing.T, role, method, path, body string) (int, map[string]any) {
	t.Helper()
	root, api := httpx.NewRouter(httpx.Options{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Authenticator: auth{testutil.Actor(role)}, Cookies: httpx.CookieConfig{SessionName: "s"}})
	(&nethttp.Handler{Svc: f.svc}).Routes(api)
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, "/api/v1"+path, rd)
	req.AddCookie(&http.Cookie{Name: "s", Value: "x"})
	req.Header.Set(httpx.HeaderCSRF, "t")
	rec := httptest.NewRecorder()
	root.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestRBAC(t *testing.T) {
	f := newFixture()
	cid := kernel.NewID().String()
	for _, tc := range []struct {
		role, method, path, body string
		want                     int
	}{
		{kernel.RoleContentCreator, "POST", "/cost-estimates", `{"kind":"text","prompt_chars":300}`, 200},
		{kernel.RoleReviewer, "POST", "/cost-estimates", `{"kind":"text"}`, 403},
		{kernel.RoleMarketingLead, "GET", "/usage/summary?group_by=provider", "", 200},
		{kernel.RoleContentCreator, "GET", "/usage/summary", "", 403},
		{kernel.RoleAdmin, "GET", "/settings/budgets", "", 200},
		{kernel.RoleContentCreator, "GET", "/settings/budgets", "", 403},
		{kernel.RoleMarketingLead, "PATCH", "/settings/budgets", `{"per_job_limits":{"text":1}}`, 200},
		{kernel.RoleReviewer, "PATCH", "/settings/budgets", `{"per_job_limits":{"text":1}}`, 403},
		{kernel.RoleMarketingLead, "PUT", "/campaigns/" + cid + "/budget", `{"limit_amount":50}`, 200},
		{kernel.RoleContentCreator, "PUT", "/campaigns/" + cid + "/budget", `{"limit_amount":50}`, 403},
		{kernel.RoleAdmin, "GET", "/campaigns/" + cid + "/budget", "", 200},
	} {
		if got, body := f.do(t, tc.role, tc.method, tc.path, tc.body); got != tc.want {
			t.Errorf("%s %s %s: %d %v", tc.role, tc.method, tc.path, got, body)
		}
	}
}

func TestEstimateShapeAndPricingMissing(t *testing.T) {
	f := newFixture()
	code, body := f.do(t, kernel.RoleContentCreator, "POST", "/cost-estimates", `{"kind":"video","clip_seconds":20,"resolution":"1080p","voiceover_chars":420}`)
	if code != 200 {
		t.Fatalf("%d %v", code, body)
	}
	data := body["data"].(map[string]any)
	lines := data["breakdown"].([]any)
	if len(lines) != 3 { // scene_clips, voiceover, render
		t.Fatalf("breakdown %v", lines)
	}
	total := data["total"].(map[string]any)
	// 20 s × 0.15 + 420 chars × 0.000016 = 3.00672
	if total["amount"].(float64) != 3.00672 || total["is_estimate"] != true || data["requires_override"] != false {
		t.Fatalf("total %v", data)
	}
	code, body = f.do(t, kernel.RoleContentCreator, "POST", "/cost-estimates", `{"kind":"image","count":2,"quality":"ultra"}`)
	if code != 503 || body["error"].(map[string]any)["code"] != kernel.CodePricingMissing {
		t.Fatalf("pricing missing: %d %v", code, body)
	}
	if code, _ := f.do(t, kernel.RoleContentCreator, "POST", "/cost-estimates", `{"kind":"music"}`); code != 400 {
		t.Fatalf("invalid kind: %d", code)
	}
}

func TestSettingsPatchAndSummaryTotals(t *testing.T) {
	f := newFixture()
	code, body := f.do(t, kernel.RoleAdmin, "PATCH", "/settings/budgets",
		`{"per_job_limits":{"video":10},"global":{"limit_amount":500,"alert_thresholds":[50,90,100],"hard_stop":false}}`)
	if code != 200 {
		t.Fatalf("%d %v", code, body)
	}
	data := body["data"].(map[string]any)
	if data["per_job_limits"].(map[string]any)["video"].(float64) != 10 || data["global"].(map[string]any)["limit_amount"].(float64) != 500 {
		t.Fatalf("%v", data)
	}
	if acts := f.audit.Actions(); len(acts) != 1 || acts[0] != "budget.update" {
		t.Fatalf("audit %v", acts)
	}
	if code, _ := f.do(t, kernel.RoleAdmin, "PATCH", "/settings/budgets", `{"global":{"alert_thresholds":[0]}}`); code != 400 {
		t.Fatalf("invalid thresholds: %d", code)
	}

	job := kernel.NewID()
	for _, u := range []ports.Usage{
		{Units: map[string]float64{"input_token": 1000, "output_token": 500}},
		{Units: map[string]float64{"image_1024_high": 1}},
	} {
		model := "fake-text"
		if _, ok := u.Units["image_1024_high"]; ok {
			model = "fake-image"
		}
		if _, err := f.svc.RecordUsage(context.Background(), usecase.UsageInput{JobID: job, UserID: kernel.NewID(),
			ProviderCode: "fake", ModelCode: model, JobType: "content_generation", Usage: u}); err != nil {
			t.Fatal(err)
		}
	}
	code, body = f.do(t, kernel.RoleAdmin, "GET", "/usage/summary?group_by=model", "")
	if code != 200 {
		t.Fatalf("%d %v", code, body)
	}
	data = body["data"].(map[string]any)
	sum := decimal.Zero
	for _, r := range data["rows"].([]any) {
		sum = sum.Add(decimal.NewFromFloat(r.(map[string]any)["total"].(float64)))
	}
	total := decimal.NewFromFloat(data["totals"].(map[string]any)["total"].(float64))
	// 1000×0.000003 + 500×0.000015 + 0.17 = 0.1805
	if !sum.Equal(total) || !total.Equal(decimal.RequireFromString("0.1805")) {
		t.Fatalf("rows %s totals %s", sum, total)
	}
	if code, _ := f.do(t, kernel.RoleAdmin, "GET", "/usage/summary?group_by=planet", ""); code != 400 {
		t.Fatalf("bad group_by: %d", code)
	}
	if code, _ := f.do(t, kernel.RoleAdmin, "GET", "/usage/summary?from=2026-13-01", ""); code != 400 {
		t.Fatalf("bad date: %d", code)
	}
}

func TestCampaignBudgetPutGet(t *testing.T) {
	f := newFixture()
	cid := kernel.NewID()
	f.store.CampaignLabels = map[kernel.ID]string{cid: "Tết 2027"}
	code, body := f.do(t, kernel.RoleMarketingLead, "PUT", "/campaigns/"+cid.String()+"/budget",
		`{"limit_amount":50,"period_start":"2026-10-01","period_end":"2026-12-31","alert_thresholds":[80,100]}`)
	if code != 200 || body["data"].(map[string]any)["limit_amount"].(float64) != 50 {
		t.Fatalf("%d %v", code, body)
	}
	if code, _ := f.do(t, kernel.RoleMarketingLead, "PUT", "/campaigns/"+cid.String()+"/budget", `{"period_end":"2026-12-31"}`); code != 400 {
		t.Fatalf("missing limit: %d", code)
	}
	if code, _ := f.do(t, kernel.RoleMarketingLead, "PUT", "/campaigns/"+cid.String()+"/budget",
		`{"limit_amount":5,"period_start":"2026-12-01","period_end":"2026-11-01"}`); code != 400 {
		t.Fatalf("end before start: %d", code)
	}
	_, body = f.do(t, kernel.RoleAdmin, "GET", "/settings/budgets", "")
	camps := body["data"].(map[string]any)["campaigns"].([]any)
	if len(camps) != 1 || camps[0].(map[string]any)["campaign_name"] != "Tết 2027" {
		t.Fatalf("campaigns %v", camps)
	}
	_, body = f.do(t, kernel.RoleAdmin, "GET", "/campaigns/"+kernel.NewID().String()+"/budget", "")
	if body["data"] != nil {
		t.Fatalf("no budget → null, got %v", body["data"])
	}
}
