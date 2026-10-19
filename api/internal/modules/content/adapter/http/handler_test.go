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

	nethttp "github.com/be/acs/api/internal/modules/content/adapter/http"
	"github.com/be/acs/api/internal/modules/content/usecase"
	"github.com/be/acs/api/internal/platform/httpx"
	"github.com/be/acs/api/internal/shared/kernel"
	"github.com/be/acs/api/internal/testutil"
)

type auth struct{ actor kernel.Actor }

func (a auth) Authenticate(context.Context, string) (kernel.Actor, httpx.Session, error) {
	return a.actor, httpx.Session{CSRFToken: "t"}, nil
}

type stub struct {
	gen     usecase.GenerateInput
	rewrite usecase.RewriteInput
	plan    usecase.PlanInput
	prompts usecase.ImagePromptInput
	tr      usecase.TransformInput
	err     error
}

var ok = usecase.JobAccepted{JobID: kernel.NewID(), Status: "queued", EstimatedCost: kernel.USD("0.0435")}

func (s *stub) Generate(_ context.Context, _ kernel.Actor, in usecase.GenerateInput) (usecase.JobAccepted, error) {
	s.gen = in
	return ok, s.err
}
func (s *stub) Rewrite(_ context.Context, _ kernel.Actor, in usecase.RewriteInput) (usecase.JobAccepted, error) {
	s.rewrite = in
	return ok, s.err
}
func (s *stub) Transform(_ context.Context, _ kernel.Actor, in usecase.TransformInput) (usecase.JobAccepted, error) {
	s.tr = in
	return ok, s.err
}
func (s *stub) StartContentPlan(_ context.Context, _ kernel.Actor, in usecase.PlanInput) (usecase.JobAccepted, error) {
	s.plan = in
	return ok, s.err
}
func (s *stub) SuggestImagePrompts(_ context.Context, _ kernel.Actor, in usecase.ImagePromptInput) (usecase.JobAccepted, error) {
	s.prompts = in
	return ok, s.err
}

func do(t *testing.T, svc *stub, role, method, path, body string, idem bool) *httptest.ResponseRecorder {
	t.Helper()
	root, api := httpx.NewRouter(httpx.Options{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Authenticator: auth{testutil.Actor(role)}, Cookies: httpx.CookieConfig{SessionName: "s"}})
	(&nethttp.Handler{Svc: svc}).Routes(api)
	req := httptest.NewRequest(method, "/api/v1"+path, strings.NewReader(body))
	req.AddCookie(&http.Cookie{Name: "s", Value: "x"})
	req.Header.Set(httpx.HeaderCSRF, "t")
	if idem {
		req.Header.Set(httpx.HeaderIdempotencyKey, kernel.NewID().String())
	}
	rec := httptest.NewRecorder()
	root.ServeHTTP(rec, req)
	return rec
}

func TestGenerationRBACAndShape(t *testing.T) {
	asset := kernel.NewID().String()
	camp := kernel.NewID().String()
	routes := []struct{ path, body string }{
		{"/content/generations", `{"brand_id":"` + kernel.NewID().String() + `","platform":"facebook","variants":3,"tone":["friendly"],"brief":{"target_audience":"SV","reference_text":"x"}}`},
		{"/content/" + asset + "/rewrites", `{"instruction":"shorten"}`},
		{"/content/" + asset + "/transforms", `{"target_type":"video_script"}`},
		{"/campaigns/" + camp + "/plan-generations", ``},
		{"/images/prompt-suggestions", `{"campaign_id":"` + camp + `","preset":"poster"}`},
	}
	roles := map[string]int{
		kernel.RoleAdmin: http.StatusAccepted, kernel.RoleMarketingLead: http.StatusAccepted,
		kernel.RoleContentCreator: http.StatusAccepted, kernel.RoleReviewer: http.StatusForbidden,
	}
	for _, rt := range routes {
		for role, want := range roles {
			rec := do(t, &stub{}, role, "POST", rt.path, rt.body, true)
			if rec.Code != want {
				t.Errorf("%s %s: %d %s", role, rt.path, rec.Code, rec.Body)
			}
		}
		if rec := do(t, &stub{}, kernel.RoleContentCreator, "POST", rt.path, rt.body, false); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "IDEMPOTENCY_KEY_REQUIRED") {
			t.Errorf("%s without key: %d %s", rt.path, rec.Code, rec.Body)
		}
	}
	svc := &stub{}
	rec := do(t, svc, kernel.RoleContentCreator, "POST", routes[0].path, routes[0].body, true)
	var out struct {
		Data struct {
			JobID         string `json:"job_id"`
			Status        string `json:"status"`
			EstimatedCost struct {
				Amount     float64 `json:"amount"`
				Currency   string  `json:"currency"`
				IsEstimate bool    `json:"is_estimate"`
			} `json:"estimated_cost"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Data.JobID != ok.JobID.String() || out.Data.EstimatedCost.Amount != 0.0435 || !out.Data.EstimatedCost.IsEstimate || out.Data.EstimatedCost.Currency != "USD" {
		t.Fatalf("%s", rec.Body)
	}
	if svc.gen.Variants != 3 || svc.gen.Brief.ReferenceText != "x" || svc.gen.IdempotencyKey == "" || svc.gen.RequestHash == "" {
		t.Fatalf("%+v", svc.gen)
	}
}

func TestValidationErrors(t *testing.T) {
	if rec := do(t, &stub{}, kernel.RoleContentCreator, "POST", "/content/generations", `{"unknown":1}`, true); rec.Code != http.StatusBadRequest {
		t.Fatalf("%d", rec.Code)
	}
	if rec := do(t, &stub{}, kernel.RoleContentCreator, "POST", "/content/not-a-uuid/rewrites", `{"instruction":"shorten"}`, true); rec.Code != http.StatusNotFound {
		t.Fatalf("%d", rec.Code)
	}
	svc := &stub{err: kernel.ErrNotFound}
	if rec := do(t, svc, kernel.RoleContentCreator, "POST", "/content/"+kernel.NewID().String()+"/rewrites", `{"instruction":"shorten"}`, true); rec.Code != http.StatusNotFound {
		t.Fatalf("%d", rec.Code)
	}
}
