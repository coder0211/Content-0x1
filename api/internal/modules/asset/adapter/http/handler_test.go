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

	assethttp "github.com/be/acs/api/internal/modules/asset/adapter/http"
	"github.com/be/acs/api/internal/modules/asset/assettest"
	"github.com/be/acs/api/internal/modules/asset/usecase"
	identityuc "github.com/be/acs/api/internal/modules/identity/usecase"
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
	files   *assettest.Files
	actors  map[string]kernel.Actor
	assetID kernel.ID
	fileID  kernel.ID
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{files: assettest.NewFiles(), actors: map[string]kernel.Actor{}}
	idn := &assettest.Identity{}
	for _, r := range []string{kernel.RoleAdmin, kernel.RoleMarketingLead, kernel.RoleContentCreator, kernel.RoleReviewer} {
		a := testutil.Actor(r)
		f.actors[r] = a
		idn.Users = append(idn.Users, identityuc.UserSummary{ID: a.UserID, Name: r, Roles: []string{r}})
	}
	f.svc = &usecase.Service{Tx: &testutil.Tx{}, Repo: assettest.NewRepo(), Clock: testutil.NewClock(), Audit: &testutil.Audit{},
		Notifier: &testutil.Notifier{}, Files: f.files, Identity: idn}
	f.fileID = f.files.Put("image/png", "image")
	ref, err := f.svc.CreateAsset(context.Background(), usecase.CreateAssetInput{
		BrandID: kernel.NewID(), Type: usecase.TypeImage, Subtype: "poster", Title: "Poster hè",
		CreatedBy: f.actors[kernel.RoleContentCreator].UserID,
		Version: usecase.NewVersion{ContentJSON: json.RawMessage(`{"prompt":"poster"}`),
			Files: []usecase.VersionFile{{FileID: f.fileID, Role: usecase.RolePrimary}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	f.assetID = ref.AssetID
	return f
}

func (f *fixture) do(t *testing.T, role, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	root, api := httpx.NewRouter(httpx.Options{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Authenticator: auth{f.actors[role]}, Cookies: httpx.CookieConfig{SessionName: "s"}})
	(&assethttp.Handler{Svc: f.svc}).Routes(api)
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, "/api/v1"+path, rd)
	req.AddCookie(&http.Cookie{Name: "s", Value: "x"})
	req.Header.Set(httpx.HeaderCSRF, "t")
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	root.ServeHTTP(rec, req)
	return rec
}

func errorCode(rec *httptest.ResponseRecorder) string {
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	return env.Error.Code
}

func TestReviewFlowOverHTTP(t *testing.T) {
	f := newFixture(t)
	p := "/assets/" + f.assetID.String()
	creator, reviewer := kernel.RoleContentCreator, kernel.RoleReviewer

	// Creator downloading a draft → 403 (ACS-084 AC).
	if rec := f.do(t, creator, "POST", p+"/download-url", ""); rec.Code != http.StatusForbidden {
		t.Fatalf("creator draft download: %d %s", rec.Code, rec.Body)
	}
	// Poster without the typography checklist → 400.
	if rec := f.do(t, creator, "POST", p+"/submit-review", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("submit poster: %d %s", rec.Code, rec.Body)
	}
	rec := f.do(t, creator, "POST", p+"/submit-review", `{"checklist":{"typography_checked":true}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("submit: %d %s", rec.Code, rec.Body)
	}
	// Reviewer queue lists it.
	rec = f.do(t, reviewer, "GET", "/reviews/queue", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), f.assetID.String()) {
		t.Fatalf("queue: %d %s", rec.Code, rec.Body)
	}
	if rec := f.do(t, creator, "GET", "/reviews/queue", ""); rec.Code != http.StatusForbidden {
		t.Fatalf("creator queue: %d", rec.Code)
	}
	// Request changes requires a comment.
	if rec := f.do(t, reviewer, "POST", p+"/request-changes", `{}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("request-changes w/o comment: %d", rec.Code)
	}
	if rec := f.do(t, reviewer, "POST", p+"/approve", `{"comment":"Đẹp"}`); rec.Code != http.StatusOK {
		t.Fatalf("approve: %d %s", rec.Code, rec.Body)
	}
	// Approving twice → 409.
	if rec := f.do(t, reviewer, "POST", p+"/approve", ""); rec.Code != http.StatusConflict {
		t.Fatalf("approve twice: %d", rec.Code)
	}
	rec = f.do(t, creator, "POST", p+"/download-url", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("download approved: %d %s", rec.Code, rec.Body)
	}
	var dl struct {
		Data struct {
			URL    string `json:"url"`
			FileID string `json:"file_id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &dl)
	if dl.Data.FileID != f.fileID.String() || dl.Data.URL == "" {
		t.Fatalf("download body %s", rec.Body)
	}
	// Detail exposes history and permissions.
	rec = f.do(t, reviewer, "GET", p, "")
	body := rec.Body.String()
	for _, want := range []string{`"status":"approved"`, `"decision":"approved"`, `"can_download":true`, `"current_version"`, `"provenance":null`} {
		if !strings.Contains(body, want) {
			t.Errorf("detail missing %s: %s", want, body)
		}
	}
}

func TestSelfApprovalOverHTTP(t *testing.T) {
	f := newFixture(t)
	lead := kernel.RoleMarketingLead
	ref, _ := f.svc.CreateAsset(context.Background(), usecase.CreateAssetInput{
		BrandID: kernel.NewID(), Type: usecase.TypeContent, Title: "Lead post", CreatedBy: f.actors[lead].UserID,
		Version: usecase.NewVersion{ContentJSON: json.RawMessage(`{"body_text":"x"}`)},
	})
	p := "/assets/" + ref.AssetID.String()
	if rec := f.do(t, lead, "POST", p+"/submit-review", "{}"); rec.Code != http.StatusOK {
		t.Fatalf("submit %d %s", rec.Code, rec.Body)
	}
	rec := f.do(t, lead, "POST", p+"/approve", "")
	if rec.Code != http.StatusForbidden || errorCode(rec) != kernel.CodeSelfApproval {
		t.Fatalf("self approve: %d %s", rec.Code, rec.Body)
	}
}

func TestVersionsOverHTTP(t *testing.T) {
	f := newFixture(t)
	p := "/assets/" + f.assetID.String()
	creator := kernel.RoleContentCreator
	rec := f.do(t, creator, "POST", p+"/versions", `{"content_json":{"prompt":"v2"},"change_note":"sửa","base_version_no":2}`)
	if rec.Code != http.StatusConflict || errorCode(rec) != kernel.CodeVersionConflict {
		t.Fatalf("stale base: %d %s", rec.Code, rec.Body)
	}
	if rec := f.do(t, creator, "POST", p+"/versions", `{"content_json":{"prompt":"v2"},"base_version_no":1}`); rec.Code != http.StatusCreated {
		t.Fatalf("create v2: %d %s", rec.Code, rec.Body)
	}
	rec = f.do(t, creator, "POST", p+"/versions/1/restore", "")
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"version_no":3`) {
		t.Fatalf("restore: %d %s", rec.Code, rec.Body)
	}
	rec = f.do(t, kernel.RoleReviewer, "GET", p+"/versions", "")
	if rec.Code != http.StatusOK || strings.Count(rec.Body.String(), `"version_no"`) != 3 {
		t.Fatalf("list: %d %s", rec.Code, rec.Body)
	}
	if rec := f.do(t, creator, "POST", p+"/versions", `{"content_json":{},"base_version_no":3,"extra":1}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown field: %d", rec.Code)
	}
	if rec := f.do(t, creator, "POST", "/assets/not-a-uuid/versions/1/restore", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("bad id: %d", rec.Code)
	}
}

func TestLibraryAndPatchOverHTTP(t *testing.T) {
	f := newFixture(t)
	p := "/assets/" + f.assetID.String()
	creator := kernel.RoleContentCreator
	rec := f.do(t, creator, "PATCH", p, `{"tags":["Hè","Sale"],"usage_rights":"Ảnh tham chiếu: tự chụp"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"tags":["hè","sale"]`) {
		t.Fatalf("patch: %d %s", rec.Code, rec.Body)
	}
	rec = f.do(t, creator, "PATCH", p, `{"usage_rights":null}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"usage_rights":null`) {
		t.Fatalf("clear rights: %d %s", rec.Code, rec.Body)
	}
	if rec := f.do(t, kernel.RoleReviewer, "PATCH", p, `{"title":"x"}`); rec.Code != http.StatusForbidden {
		t.Fatalf("reviewer patch: %d", rec.Code)
	}
	for _, tc := range []struct {
		q    string
		code int
		hit  bool
	}{
		{"?type=image&status=draft,pending_review&tag=hè", http.StatusOK, true},
		{"?q=poster", http.StatusOK, true},
		{"?type=content", http.StatusOK, false},
		{"?status=bogus", http.StatusBadRequest, false},
		{"?from=2026-13-01", http.StatusBadRequest, false},
		{"?campaign_id=x", http.StatusBadRequest, false},
		{"?limit=500", http.StatusBadRequest, false},
	} {
		rec := f.do(t, kernel.RoleReviewer, "GET", "/assets"+tc.q, "")
		if rec.Code != tc.code || strings.Contains(rec.Body.String(), f.assetID.String()) != tc.hit {
			t.Errorf("%s: %d %s", tc.q, rec.Code, rec.Body)
		}
	}
	if rec := f.do(t, creator, "DELETE", p, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body)
	}
	if rec := f.do(t, creator, "GET", p, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("get deleted: %d", rec.Code)
	}
}

// RBAC table: which roles may reach each endpoint (coarse permissions).
func TestRBACMatrix(t *testing.T) {
	f := newFixture(t)
	p := "/assets/" + f.assetID.String()
	for _, tc := range []struct {
		method, path string
		allowed      map[string]bool
	}{
		{"GET", "/assets", map[string]bool{kernel.RoleAdmin: true, kernel.RoleMarketingLead: true, kernel.RoleContentCreator: true, kernel.RoleReviewer: true}},
		{"GET", "/reviews/queue", map[string]bool{kernel.RoleAdmin: true, kernel.RoleMarketingLead: true, kernel.RoleReviewer: true}},
		{"GET", p + "/versions", map[string]bool{kernel.RoleAdmin: true, kernel.RoleMarketingLead: true, kernel.RoleContentCreator: true, kernel.RoleReviewer: true}},
	} {
		for role := range f.actors {
			rec := f.do(t, role, tc.method, tc.path, "")
			if (rec.Code == http.StatusOK) != tc.allowed[role] {
				t.Errorf("%s %s as %s: %d", tc.method, tc.path, role, rec.Code)
			}
		}
	}
}
