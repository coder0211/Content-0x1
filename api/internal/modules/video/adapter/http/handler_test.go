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

	"github.com/be/acs/api/internal/modules/asset/assettest"
	"github.com/be/acs/api/internal/modules/brand/brandtest"
	branduc "github.com/be/acs/api/internal/modules/brand/usecase"
	"github.com/be/acs/api/internal/modules/budget/budgettest"
	budgetuc "github.com/be/acs/api/internal/modules/budget/usecase"
	"github.com/be/acs/api/internal/modules/campaign/campaigntest"
	"github.com/be/acs/api/internal/modules/content/contenttest"
	"github.com/be/acs/api/internal/modules/job/jobtest"
	"github.com/be/acs/api/internal/modules/provider/providertest"
	videohttp "github.com/be/acs/api/internal/modules/video/adapter/http"
	"github.com/be/acs/api/internal/modules/video/domain"
	"github.com/be/acs/api/internal/modules/video/usecase"
	"github.com/be/acs/api/internal/modules/video/videotest"
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
	repo    *usecase.MemoryRepo
	budget  *budgettest.API
	brandID kernel.ID
}

func newFixture(t *testing.T) *fixture {
	clock := testutil.NewClock()
	brands := brandtest.NewAPI()
	f := &fixture{repo: usecase.NewMemoryRepo(), budget: budgettest.NewAPI(), brandID: brands.AddSample().BrandID}
	f.svc = &usecase.Service{
		Tx: &testutil.Tx{}, Repo: f.repo, Clock: clock, Audit: &testutil.Audit{}, Jobs: jobtest.NewAPI(), Budget: f.budget,
		Assets: assettest.NewAPI(), Brands: brands, Campaigns: campaigntest.NewAPI(), Providers: providertest.NewAPI(),
		Content: contenttest.NewAPI(), Files: videotest.NewFiles(clock), AI: videotest.NewAI(), Renderer: &videotest.Renderer{},
		Prober: videotest.Prober{}, TempDir: t.TempDir(),
	}
	return f
}

// project creates a project with a 3-scene storyboard owned by owner.
func (f *fixture) project(t *testing.T, owner kernel.Actor) kernel.ID {
	acc, err := f.svc.Create(context.Background(), owner, usecase.CreateInput{BrandID: &f.brandID, Prompt: "Video thử", Preset: "square",
		TargetDurationSeconds: 12, Idem: usecase.Idempotency{Key: kernel.NewID().String()}})
	if err != nil {
		t.Fatal(err)
	}
	scenes := make([]domain.Scene, 3)
	for i := range scenes {
		scenes[i] = domain.Scene{ID: kernel.NewID(), AssetID: acc.AssetID, DurationMs: 4000, VisualPrompt: "Cảnh", Narration: "Lời", Status: domain.SceneDraft}
	}
	if err := f.repo.SaveStoryboard(context.Background(), acc.AssetID, scenes); err != nil {
		t.Fatal(err)
	}
	// Mark the storyboard job finished so the project is editable.
	jobs := f.svc.Jobs.(*jobtest.API)
	_ = jobs.Finish(context.Background(), acc.JobID, "succeeded")
	return acc.AssetID
}

func (f *fixture) do(t *testing.T, actor kernel.Actor, method, path, body string, idem bool) *httptest.ResponseRecorder {
	t.Helper()
	root, api := httpx.NewRouter(httpx.Options{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Authenticator: auth{actor}, Cookies: httpx.CookieConfig{SessionName: "s"}})
	(&videohttp.Handler{Svc: f.svc}).Routes(api)
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, "/api/v1"+path, rd)
	req.AddCookie(&http.Cookie{Name: "s", Value: "x"})
	req.Header.Set(httpx.HeaderCSRF, "t")
	if idem {
		req.Header.Set(httpx.HeaderIdempotencyKey, kernel.NewID().String())
	}
	rec := httptest.NewRecorder()
	root.ServeHTTP(rec, req)
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var env struct {
		Data  map[string]any `json:"data"`
		Error map[string]any `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("body %s: %v", rec.Body, err)
	}
	if env.Error != nil {
		return env.Error
	}
	return env.Data
}

func TestCreateRBACAndValidation(t *testing.T) {
	f := newFixture(t)
	body := `{"brand_id":"` + f.brandID.String() + `","prompt":"Video thu","preset":"social_vertical","target_duration_seconds":20,` +
		`"voiceover":{"enabled":true,"language":"vi","voice_id":null,"mode":null},"subtitles":{"enabled":true,"language":"vi","style":"burn_in"},"music":{"enabled":false,"track_brand_asset_id":null}}`
	for _, tc := range []struct {
		role   string
		body   string
		idem   bool
		status int
		code   string
	}{
		{kernel.RoleContentCreator, body, true, http.StatusAccepted, ""},
		{kernel.RoleMarketingLead, body, true, http.StatusAccepted, ""},
		{kernel.RoleAdmin, body, true, http.StatusAccepted, ""},
		{kernel.RoleReviewer, body, true, http.StatusForbidden, "FORBIDDEN"},
		{kernel.RoleContentCreator, body, false, http.StatusBadRequest, kernel.CodeIdempotencyKeyRequired},
		{kernel.RoleContentCreator, `{"preset":"bogus","prompt":"x","brand_id":"` + f.brandID.String() + `"}`, true, http.StatusBadRequest, kernel.CodeValidation},
		{kernel.RoleContentCreator, `{"preset":"square","unknown":1}`, true, http.StatusBadRequest, kernel.CodeValidation},
	} {
		rec := f.do(t, testutil.Actor(tc.role), "POST", "/videos", tc.body, tc.idem)
		if rec.Code != tc.status {
			t.Errorf("%s: %d %s", tc.role, rec.Code, rec.Body)
			continue
		}
		data := decode(t, rec)
		if tc.code != "" && data["code"] != tc.code {
			t.Errorf("%s: code %v", tc.role, data["code"])
		}
		if tc.status == http.StatusAccepted {
			cost := data["estimated_cost"].(map[string]any)
			if data["asset_id"] == nil || data["job_id"] == nil || data["status"] != "queued" || cost["is_estimate"] != true || cost["currency"] != "USD" {
				t.Errorf("accepted body %v", data)
			}
		}
	}
}

func TestGetProjectShapeAndIDOR(t *testing.T) {
	f := newFixture(t)
	owner := testutil.Actor(kernel.RoleContentCreator)
	id := f.project(t, owner)
	rec := f.do(t, owner, "GET", "/videos/"+id.String(), "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	data := decode(t, rec)
	scenes := data["scenes"].([]any)
	limits := data["limits"].(map[string]any)
	settings := data["settings"].(map[string]any)
	if len(scenes) != 3 || data["stage"] != "editing" || data["total_duration_ms"] != 12000.0 || limits["max_total_seconds"] != 60.0 ||
		settings["aspect_ratio"] != "1:1" || settings["width"] != 1080.0 || data["output"] != nil {
		t.Fatalf("project %v", data)
	}
	sc := scenes[0].(map[string]any)
	for _, k := range []string{"id", "sequence_no", "duration_ms", "visual_prompt", "narration", "subtitle_text", "clip_file_id", "status", "preview_url", "thumbnail_url"} {
		if _, ok := sc[k]; !ok {
			t.Errorf("scene missing %s", k)
		}
	}
	if rec := f.do(t, testutil.Actor(kernel.RoleContentCreator), "GET", "/videos/"+id.String(), "", false); rec.Code != http.StatusNotFound {
		t.Fatalf("idor: %d", rec.Code)
	}
	if rec := f.do(t, testutil.Actor(kernel.RoleReviewer), "GET", "/videos/"+id.String(), "", false); rec.Code != http.StatusOK {
		t.Fatalf("reviewer view: %d", rec.Code)
	}
	if rec := f.do(t, owner, "GET", "/videos/not-a-uuid", "", false); rec.Code != http.StatusNotFound {
		t.Fatalf("bad id: %d", rec.Code)
	}
	rec = f.do(t, owner, "GET", "/videos?limit=10", "", false)
	var list struct {
		Data []any `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if rec.Code != http.StatusOK || len(list.Data) != 1 {
		t.Fatalf("list %d %s", rec.Code, rec.Body)
	}
}

func TestScenesEstimateProduce(t *testing.T) {
	f := newFixture(t)
	owner := testutil.Actor(kernel.RoleContentCreator)
	id := f.project(t, owner)
	base := "/videos/" + id.String()

	over := `{"scenes":[` + strings.Repeat(`{"id":null,"duration_ms":8000,"visual_prompt":"x","narration":null,"subtitle_text":null,"source_file_id":null},`, 7) +
		`{"id":null,"duration_ms":8000,"visual_prompt":"x","narration":null,"subtitle_text":null,"source_file_id":null}]}`
	rec := f.do(t, owner, "PUT", base+"/scenes", over, false)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "total_duration_exceeded") {
		t.Fatalf("over limit: %d %s", rec.Code, rec.Body)
	}
	ok := `{"scenes":[{"id":null,"duration_ms":5000,"visual_prompt":"Lớp học","narration":"Xin chào","subtitle_text":null,"source_file_id":null},` +
		`{"id":null,"duration_ms":5000,"visual_prompt":"Logo","narration":null,"subtitle_text":"Đăng ký","source_file_id":null}]}`
	rec = f.do(t, owner, "PUT", base+"/scenes", ok, false)
	if rec.Code != http.StatusOK || len(decode(t, rec)["scenes"].([]any)) != 2 {
		t.Fatalf("save: %d %s", rec.Code, rec.Body)
	}

	rec = f.do(t, owner, "POST", base+"/estimate", "", false)
	est := decode(t, rec)
	if rec.Code != http.StatusOK || est["scene_count"] != 2.0 || est["total_clip_seconds"] != 10.0 || est["requires_override"] != false ||
		len(est["breakdown"].([]any)) == 0 || est["limits"].(map[string]any)["per_job_limit"] != 8.0 {
		t.Fatalf("estimate %d %v", rec.Code, est)
	}

	f.budget.PerJob[budgetuc.CategoryVideo] = kernel.USD("0.5")
	rec = f.do(t, owner, "POST", base+"/productions", `{"budget_override":false}`, true)
	if rec.Code != http.StatusTooManyRequests || decode(t, rec)["code"] != kernel.CodeJobBudgetExceeded {
		t.Fatalf("budget gate: %d %s", rec.Code, rec.Body)
	}
	if rec := f.do(t, owner, "POST", base+"/productions", "", false); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing idempotency: %d", rec.Code)
	}
	rec = f.do(t, testutil.Actor(kernel.RoleMarketingLead), "POST", base+"/productions", `{"budget_override":true}`, true)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("lead override: %d %s", rec.Code, rec.Body)
	}
	// A job is running now: another production conflicts.
	f.budget.PerJob[budgetuc.CategoryVideo] = kernel.USD("8")
	rec = f.do(t, owner, "POST", base+"/productions", "", true)
	if rec.Code != http.StatusConflict || decode(t, rec)["code"] != usecase.CodeJobInProgress {
		t.Fatalf("in progress: %d %s", rec.Code, rec.Body)
	}
	// Re-render needs ready scenes; regenerate of an unknown scene is 404.
	if rec := f.do(t, owner, "POST", base+"/scenes/"+kernel.NewID().String()+"/regenerations", "", true); rec.Code != http.StatusNotFound {
		t.Fatalf("regenerate unknown: %d %s", rec.Code, rec.Body)
	}
	if rec := f.do(t, testutil.Actor(kernel.RoleReviewer), "POST", base+"/renders", "", true); rec.Code != http.StatusForbidden {
		t.Fatalf("reviewer render: %d", rec.Code)
	}
}

func TestUnlicensedMusicIs422(t *testing.T) {
	f := newFixture(t)
	brands := f.svc.Brands.(*brandtest.API)
	track := kernel.NewID()
	brands.Assets[track] = brandAsset(track, f.brandID)
	body := `{"brand_id":"` + f.brandID.String() + `","prompt":"x","preset":"landscape","music":{"enabled":true,"track_brand_asset_id":"` + track.String() + `"}}`
	rec := f.do(t, testutil.Actor(kernel.RoleContentCreator), "POST", "/videos", body, true)
	if rec.Code != http.StatusUnprocessableEntity || decode(t, rec)["code"] != usecase.CodeMusicNotLicensed {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}

func brandAsset(id, brandID kernel.ID) branduc.BrandAsset {
	return branduc.BrandAsset{ID: id, BrandID: brandID, FileID: kernel.NewID(), AssetType: "music", Label: "Không license"}
}
