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

	"github.com/be/acs/api/internal/modules/brand/brandtest"
	camphttp "github.com/be/acs/api/internal/modules/campaign/adapter/http"
	"github.com/be/acs/api/internal/modules/campaign/campaigntest"
	"github.com/be/acs/api/internal/modules/campaign/usecase"
	"github.com/be/acs/api/internal/platform/httpx"
	"github.com/be/acs/api/internal/shared/kernel"
	"github.com/be/acs/api/internal/testutil"
)

type auth struct{ actor kernel.Actor }

func (a auth) Authenticate(context.Context, string) (kernel.Actor, httpx.Session, error) {
	return a.actor, httpx.Session{CSRFToken: "t"}, nil
}

type fixture struct {
	svc     *usecase.Service
	brandID kernel.ID
	users   *campaigntest.Users
}

func newFixture() *fixture {
	brands := brandtest.NewAPI()
	bc := brands.AddSample()
	users := campaigntest.NewUsers()
	return &fixture{brandID: bc.BrandID, users: users,
		svc: &usecase.Service{Tx: &testutil.Tx{}, Repo: campaigntest.NewRepo(), Clock: testutil.NewClock(), Audit: &testutil.Audit{}, Brands: brands, Users: users}}
}

func (f *fixture) do(t *testing.T, a kernel.Actor, method, path, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	root, api := httpx.NewRouter(httpx.Options{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Authenticator: auth{a}, Cookies: httpx.CookieConfig{SessionName: "s"}})
	(&camphttp.Handler{Svc: f.svc}).Routes(api)
	req := httptest.NewRequest(method, "/api/v1"+path, strings.NewReader(body))
	req.AddCookie(&http.Cookie{Name: "s", Value: "x"})
	req.Header.Set(httpx.HeaderCSRF, "t")
	rec := httptest.NewRecorder()
	root.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}

func (f *fixture) body() string {
	return `{"brand_id":"` + f.brandID.String() + `","name":"Khóa giao tiếp T11","goal":"lead",
	"brief":{"product_ids":[],"target_audience":"Sinh viên","key_message":"Tự tin","verified_offer":"","channels":["facebook"],
	"formats":[{"type":"facebook_post","count":5}],"deadline":"2026-11-15","notes":""},"start_date":"2026-11-01","end_date":null}`
}

func TestCampaignHTTP(t *testing.T) {
	f := newFixture()
	creator := testutil.Actor(kernel.RoleContentCreator)
	reviewer := testutil.Actor(kernel.RoleReviewer)

	if rec, _ := f.do(t, reviewer, "POST", "/campaigns", f.body()); rec.Code != http.StatusForbidden {
		t.Fatalf("reviewer create: %d", rec.Code)
	}
	rec, out := f.do(t, creator, "POST", "/campaigns", `{"brand_id":"`+f.brandID.String()+`","name":"x","goal":"lead","brief":{}}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing brief: %d", rec.Code)
	}
	details := out["error"].(map[string]any)["details"].([]any)
	if len(details) < 5 {
		t.Fatalf("details per field: %v", details)
	}
	rec, out = f.do(t, creator, "POST", "/campaigns", f.body())
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	data := out["data"].(map[string]any)
	id := data["id"].(string)
	if data["status"] != "draft" || data["end_date"] != nil || data["content_plan"] != nil || len(data["members"].([]any)) != 0 {
		t.Fatalf("%v", data)
	}
	if rec, _ := f.do(t, reviewer, "GET", "/campaigns?status=draft", ""); rec.Code != http.StatusOK {
		t.Fatalf("list: %d", rec.Code)
	}
	if rec, _ := f.do(t, reviewer, "GET", "/campaigns/"+id, ""); rec.Code != http.StatusOK {
		t.Fatalf("get: %d", rec.Code)
	}
	if rec, _ := f.do(t, reviewer, "GET", "/campaigns/"+kernel.NewID().String(), ""); rec.Code != http.StatusNotFound {
		t.Fatalf("get missing: %d", rec.Code)
	}
	// ACS-025 AC
	rec, out = f.do(t, creator, "PATCH", "/campaigns/"+id, `{"status":"completed"}`)
	if rec.Code != http.StatusConflict || out["error"].(map[string]any)["code"] != "INVALID_STATE_TRANSITION" {
		t.Fatalf("draft→completed: %d %s", rec.Code, rec.Body)
	}
	if rec, _ := f.do(t, creator, "PATCH", "/campaigns/"+id, `{"status":"in_progress"}`); rec.Code != http.StatusOK {
		t.Fatalf("draft→in_progress: %d", rec.Code)
	}
	if rec, _ := f.do(t, testutil.Actor(kernel.RoleContentCreator), "PATCH", "/campaigns/"+id, `{"name":"y"}`); rec.Code != http.StatusForbidden {
		t.Fatalf("non-member: %d", rec.Code)
	}
	rec, out = f.do(t, creator, "PUT", "/campaigns/"+id+"/plan", `{"plan":{"summary":"","themes":["A"],"items":[{"topic":"Mở đầu","format":"facebook_post","publish_date":"2026-11-02"}]}}`)
	if rec.Code != http.StatusOK || out["data"].(map[string]any)["content_plan"] == nil {
		t.Fatalf("plan: %d %s", rec.Code, rec.Body)
	}
	if rec, _ := f.do(t, creator, "DELETE", "/campaigns/"+id+"/plan-proposal", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("discard: %d", rec.Code)
	}
	f.users.Add("Ai Đó", kernel.RoleContentCreator)
	rec, out = f.do(t, creator, "GET", "/campaigns/member-candidates", "")
	if rec.Code != http.StatusOK || len(out["data"].([]any)) != 1 {
		t.Fatalf("candidates: %d %s", rec.Code, rec.Body)
	}
	if rec, _ := f.do(t, creator, "POST", "/campaigns", `{"brand_id":"`+f.brandID.String()+`","budget":{"limit_amount":5}}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown field: %d", rec.Code)
	}
}
