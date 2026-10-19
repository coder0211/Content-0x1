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

	nethttp "github.com/be/acs/api/internal/modules/provider/adapter/http"
	"github.com/be/acs/api/internal/modules/provider/domain"
	"github.com/be/acs/api/internal/modules/provider/usecase"
	"github.com/be/acs/api/internal/platform/httpx"
	"github.com/be/acs/api/internal/shared/kernel"
	"github.com/be/acs/api/internal/testutil"
)

type auth struct{ actor kernel.Actor }

func (a auth) Authenticate(context.Context, string) (kernel.Actor, httpx.Session, error) {
	return a.actor, httpx.Session{CSRFToken: "t"}, nil
}

const secretValue = "sk-ant-THIS-MUST-NEVER-LEAK"

type env struct {
	svc  *usecase.Service
	repo *usecase.MemoryRepo
	ids  map[string]kernel.ID
}

func newEnv() *env {
	repo := &usecase.MemoryRepo{}
	ids := map[string]kernel.ID{"claude": kernel.NewID()}
	ids["anthropic"] = repo.Seed("anthropic", []string{"text"},
		domain.Model{ID: ids["claude"], ModelCode: "claude-sonnet-5-5", Kind: "text", Capabilities: []byte(`{"max_output_tokens":64000}`)}).ID
	secrets := map[string]string{"ANTHROPIC_API_KEY": secretValue}
	svc := &usecase.Service{Tx: &testutil.Tx{}, Repo: repo, Clock: testutil.NewClock(), Audit: &testutil.Audit{},
		Secrets: func(ref string) bool { return secrets[ref] != "" }}
	return &env{svc: svc, repo: repo, ids: ids}
}

func (e *env) do(t *testing.T, role, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	root, api := httpx.NewRouter(httpx.Options{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Authenticator: auth{testutil.Actor(role)}, Cookies: httpx.CookieConfig{SessionName: "s"}})
	(&nethttp.Handler{Svc: e.svc}).Routes(api)
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, "/api/v1"+path, rd)
	req.AddCookie(&http.Cookie{Name: "s", Value: "x"})
	req.Header.Set(httpx.HeaderCSRF, "t")
	rec := httptest.NewRecorder()
	root.ServeHTTP(rec, req)
	return rec
}

func TestRBACMatrix(t *testing.T) {
	e := newEnv()
	m := e.ids["claude"].String()
	p := e.ids["anthropic"].String()
	type call struct{ method, path, body string }
	view := []call{{"GET", "/settings/providers", ""}, {"GET", "/settings/models", ""}, {"GET", "/settings/models/" + m, ""},
		{"GET", "/settings/models/" + m + "/pricing", ""}, {"GET", "/settings/prompt-templates", ""}}
	manage := []call{{"PATCH", "/settings/providers/" + p, `{"status":"active"}`}, {"PATCH", "/settings/models/" + m, `{"concurrency_limit":3}`}}
	for _, tc := range []struct {
		role         string
		view, manage bool
	}{
		{kernel.RoleAdmin, true, true}, {kernel.RoleMarketingLead, true, false},
		{kernel.RoleContentCreator, false, false}, {kernel.RoleReviewer, false, false},
	} {
		for _, c := range view {
			rec := e.do(t, tc.role, c.method, c.path, c.body)
			if (rec.Code == http.StatusOK) != tc.view || (!tc.view && rec.Code != http.StatusForbidden) {
				t.Errorf("%s %s %s: %d", tc.role, c.method, c.path, rec.Code)
			}
		}
		for _, c := range manage {
			rec := e.do(t, tc.role, c.method, c.path, c.body)
			if (rec.Code == http.StatusOK) != tc.manage || (!tc.manage && rec.Code != http.StatusForbidden) {
				t.Errorf("%s %s %s: %d %s", tc.role, c.method, c.path, rec.Code, rec.Body)
			}
		}
	}
}

// ACS-090 AC: no response ever contains a secret value.
func TestResponsesNeverContainSecrets(t *testing.T) {
	e := newEnv()
	p := e.ids["anthropic"].String()
	for _, rec := range []*httptest.ResponseRecorder{
		e.do(t, kernel.RoleAdmin, "GET", "/settings/providers", ""),
		e.do(t, kernel.RoleAdmin, "PATCH", "/settings/providers/"+p, `{"secret_ref":"ANTHROPIC_API_KEY","config_json":{"effort":"low"}}`),
	} {
		if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), secretValue) {
			t.Fatalf("%d %s", rec.Code, rec.Body)
		}
		if !strings.Contains(rec.Body.String(), `"secret_configured":true`) {
			t.Fatalf("secret status must be shown: %s", rec.Body)
		}
	}
	rec := e.do(t, kernel.RoleAdmin, "PATCH", "/settings/providers/"+p, `{"secret_ref":"`+secretValue+`"}`)
	if rec.Code != http.StatusBadRequest || strings.Contains(rec.Body.String(), secretValue) {
		t.Fatalf("pasted key must be rejected without echo: %d %s", rec.Code, rec.Body)
	}
	rec = e.do(t, kernel.RoleAdmin, "PATCH", "/settings/providers/"+p, `{"config_json":{"api_key":"x"}}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}

func TestModelPricingAndTemplatesFlow(t *testing.T) {
	e := newEnv()
	m := e.ids["claude"].String()
	rec := e.do(t, kernel.RoleAdmin, "POST", "/settings/models/"+m+"/pricing",
		`{"pricing_version":"2026-11","items":[{"unit":"input_token","unit_price":"0.000002"},{"unit":"output_token","unit_price":0.00001}]}`)
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"unit_price":"0.000002"`) {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if rec := e.do(t, kernel.RoleAdmin, "POST", "/settings/models/"+m+"/pricing", `{"pricing_version":"2026-11","items":[{"unit":"input_token","unit_price":"1"}]}`); rec.Code != http.StatusConflict {
		t.Fatalf("duplicate version: %d", rec.Code)
	}
	if rec := e.do(t, kernel.RoleAdmin, "POST", "/settings/models/"+m+"/pricing", `{"pricing_version":"x","items":[{"unit":"char","unit_price":"1"}]}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("unit of another kind: %d", rec.Code)
	}
	rec = e.do(t, kernel.RoleAdmin, "GET", "/settings/models/"+m+"/pricing", "")
	var list struct {
		Data []struct {
			PricingVersion string `json:"pricing_version"`
			IsCurrent      bool   `json:"is_current"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list.Data) != 1 || !list.Data[0].IsCurrent {
		t.Fatalf("%s", rec.Body)
	}
	if rec := e.do(t, kernel.RoleAdmin, "GET", "/settings/models?kind=sound", ""); rec.Code != http.StatusBadRequest {
		t.Fatal(rec.Code)
	}
	if rec := e.do(t, kernel.RoleAdmin, "GET", "/settings/models/"+kernel.NewID().String(), ""); rec.Code != http.StatusNotFound {
		t.Fatal(rec.Code)
	}
	rec = e.do(t, kernel.RoleAdmin, "POST", "/settings/models", `{"provider_id":"`+e.ids["anthropic"].String()+
		`","model_code":"claude-haiku-4-5","kind":"text","capabilities":{"languages":["vi"]},"concurrency_limit":4}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}

	// Templates: Lead may create; Creator may not.
	body := `{"code":"content.facebook_post","system_prompt":"Bạn là copywriter","user_template":"{{.Brief}}","output_schema":{"type":"object"},"activate":true}`
	if rec := e.do(t, kernel.RoleContentCreator, "POST", "/settings/prompt-templates", body); rec.Code != http.StatusForbidden {
		t.Fatal(rec.Code)
	}
	rec = e.do(t, kernel.RoleMarketingLead, "POST", "/settings/prompt-templates", body)
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"status":"active"`) {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	var tpl struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &tpl)
	if rec := e.do(t, kernel.RoleMarketingLead, "POST", "/settings/prompt-templates/"+tpl.Data.ID+"/retire", ""); rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if rec := e.do(t, kernel.RoleMarketingLead, "POST", "/settings/prompt-templates/"+tpl.Data.ID+"/retire", ""); rec.Code != http.StatusConflict {
		t.Fatalf("%d", rec.Code)
	}
	if rec := e.do(t, kernel.RoleAdmin, "POST", "/settings/prompt-templates/"+tpl.Data.ID+"/activate", ""); rec.Code != http.StatusOK {
		t.Fatalf("%d", rec.Code)
	}
	rec = e.do(t, kernel.RoleMarketingLead, "GET", "/settings/prompt-templates?code=content.facebook_post", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"version":1`) {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if rec := e.do(t, kernel.RoleAdmin, "POST", "/settings/prompt-templates", `{"code":"x","system_prompt":"","user_template":""}`); rec.Code != http.StatusBadRequest {
		t.Fatal(rec.Code)
	}
}
