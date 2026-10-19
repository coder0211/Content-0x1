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

	brandhttp "github.com/be/acs/api/internal/modules/brand/adapter/http"
	"github.com/be/acs/api/internal/modules/brand/brandtest"
	"github.com/be/acs/api/internal/modules/brand/usecase"
	"github.com/be/acs/api/internal/platform/httpx"
	"github.com/be/acs/api/internal/shared/kernel"
	"github.com/be/acs/api/internal/testutil"
)

type auth struct{ actor kernel.Actor }

func (a auth) Authenticate(context.Context, string) (kernel.Actor, httpx.Session, error) {
	return a.actor, httpx.Session{CSRFToken: "t"}, nil
}

type fixture struct {
	svc   *usecase.Service
	files *brandtest.Files
}

func (f *fixture) do(t *testing.T, a kernel.Actor, method, path, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	root, api := httpx.NewRouter(httpx.Options{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Authenticator: auth{a}, Cookies: httpx.CookieConfig{SessionName: "s"}})
	(&brandhttp.Handler{Svc: f.svc}).Routes(api)
	req := httptest.NewRequest(method, "/api/v1"+path, strings.NewReader(body))
	req.AddCookie(&http.Cookie{Name: "s", Value: "x"})
	req.Header.Set(httpx.HeaderCSRF, "t")
	rec := httptest.NewRecorder()
	root.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}

func newFixture() *fixture {
	files := brandtest.NewFiles()
	return &fixture{files: files, svc: &usecase.Service{Tx: &testutil.Tx{}, Repo: brandtest.NewRepo(), Clock: testutil.NewClock(), Audit: &testutil.Audit{}, Files: files}}
}

func code(out map[string]any) any { return out["error"].(map[string]any)["code"] }

func TestBrandHTTP(t *testing.T) {
	f := newFixture()
	lead, creator, reviewer := testutil.Actor(kernel.RoleMarketingLead), testutil.Actor(kernel.RoleContentCreator), testutil.Actor(kernel.RoleReviewer)
	body := `{"name":"Anh ngữ Sao Mai","profile":{"description":"","colors":[{"name":"primary","hex":"#0055FF"}],"typography":{"heading":"Inter","body":"Inter"},
	"tone_of_voice":["friendly"],"preferred_words":[],"forbidden_words":["rẻ nhất"],"default_ctas":["Đăng ký tư vấn"],"audience_personas":[{"name":"Sinh viên","pain_points":[],"motivations":[]}]}}`
	for _, a := range []kernel.Actor{creator, reviewer} {
		if rec, _ := f.do(t, a, "POST", "/brands", body); rec.Code != http.StatusForbidden {
			t.Fatalf("%v create: %d", a.Roles, rec.Code)
		}
	}
	rec, out := f.do(t, lead, "POST", "/brands", `{"name":"X","profile":{"colors":[{"name":"p","hex":"blue"}]}}`)
	if rec.Code != http.StatusBadRequest || code(out) != "VALIDATION_FAILED" {
		t.Fatalf("hex: %d %s", rec.Code, rec.Body)
	}
	rec, out = f.do(t, lead, "POST", "/brands", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	id := out["data"].(map[string]any)["id"].(string)
	if v := out["data"].(map[string]any)["current_version"].(map[string]any)["version_no"]; v != float64(1) {
		t.Fatal(v)
	}
	// ACS-020 AC: two editors on version 1 → the second gets 409 VERSION_CONFLICT.
	patch := `{"profile":{"tone_of_voice":["formal"]},"expected_version_no":1}`
	if rec, _ := f.do(t, lead, "PATCH", "/brands/"+id, patch); rec.Code != http.StatusOK {
		t.Fatalf("first patch: %d %s", rec.Code, rec.Body)
	}
	rec, out = f.do(t, lead, "PATCH", "/brands/"+id, `{"profile":{"tone_of_voice":["casual"]},"expected_version_no":1}`)
	if rec.Code != http.StatusConflict || code(out) != "VERSION_CONFLICT" {
		t.Fatalf("second patch: %d %s", rec.Code, rec.Body)
	}
	rec, out = f.do(t, reviewer, "GET", "/brands/"+id+"/versions", "")
	if rec.Code != http.StatusOK || len(out["data"].([]any)) != 2 {
		t.Fatalf("versions: %d %s", rec.Code, rec.Body)
	}
	if rec, _ := f.do(t, creator, "PATCH", "/brands/"+id, patch); rec.Code != http.StatusForbidden {
		t.Fatalf("creator patch: %d", rec.Code)
	}

	// products: expired offers are flagged
	rec, out = f.do(t, lead, "POST", "/brands/"+id+"/products", `{"name":"Giao tiếp 12 tuần","description":"","facts":{"usp":["12 tuần"],"price":{"amount":2990000,"currency":"VND","note":""},
	"offers":[{"text":"Giảm 20%","valid_from":"2026-01-01","valid_to":"2026-01-31"},{"text":"Tặng giáo trình","valid_from":"","valid_to":""}],"policies":[],"proof":[{"type":"testimonial","text":"Hay","source":"HV","verified":false}]}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("product: %d %s", rec.Code, rec.Body)
	}
	p := out["data"].(map[string]any)
	offers := p["facts"].(map[string]any)["offers"].([]any)
	if offers[0].(map[string]any)["status"] != "expired" || offers[1].(map[string]any)["status"] != "active" || offers[1].(map[string]any)["valid_to"] != nil {
		t.Fatalf("offers %v", offers)
	}
	if p["facts"].(map[string]any)["price"].(map[string]any)["amount"] != float64(2990000) {
		t.Fatalf("price %v", p["facts"])
	}
	pid := p["id"].(string)
	if rec, _ := f.do(t, lead, "PATCH", "/products/"+pid, `{"status":"archived"}`); rec.Code != http.StatusOK {
		t.Fatalf("archive: %d %s", rec.Code, rec.Body)
	}
	if rec, out := f.do(t, reviewer, "GET", "/brands/"+id+"/products?status=archived", ""); rec.Code != http.StatusOK || len(out["data"].([]any)) != 1 {
		t.Fatalf("list products: %d %s", rec.Code, rec.Body)
	}

	// assets: music without license → 400
	fid := f.files.Add("audio", "audio/mpeg", lead.UserID)
	rec, out = f.do(t, lead, "POST", "/brands/"+id+"/assets", `{"file_id":"`+fid.String()+`","asset_type":"music","label":"Nhạc nền"}`)
	if rec.Code != http.StatusBadRequest || code(out) != "VALIDATION_FAILED" {
		t.Fatalf("music w/o license: %d %s", rec.Code, rec.Body)
	}
	rec, out = f.do(t, lead, "POST", "/brands/"+id+"/assets", `{"file_id":"`+fid.String()+`","asset_type":"music","label":"Nhạc nền","license":{"source":"Artlist","license_type":"commercial"}}`)
	if rec.Code != http.StatusCreated || out["data"].(map[string]any)["license_valid"] != true {
		t.Fatalf("music: %d %s", rec.Code, rec.Body)
	}
	aid := out["data"].(map[string]any)["id"].(string)
	rec, out = f.do(t, reviewer, "GET", "/brands/"+id+"/assets?asset_type=music", "")
	if rec.Code != http.StatusOK || len(out["data"].([]any)) != 1 {
		t.Fatalf("assets: %d %s", rec.Code, rec.Body)
	}
	if u := out["data"].([]any)[0].(map[string]any)["file"].(map[string]any)["url"]; u == nil {
		t.Fatal("signed url")
	}
	if rec, _ := f.do(t, lead, "DELETE", "/brands/"+id+"/assets/"+aid, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete asset: %d", rec.Code)
	}
	if rec, _ := f.do(t, reviewer, "GET", "/brands/"+kernel.NewID().String(), ""); rec.Code != http.StatusNotFound {
		t.Fatalf("missing brand: %d", rec.Code)
	}
}
