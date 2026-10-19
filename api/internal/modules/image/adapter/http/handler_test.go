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

	nethttp "github.com/be/acs/api/internal/modules/image/adapter/http"
	"github.com/be/acs/api/internal/modules/image/domain"
	"github.com/be/acs/api/internal/modules/image/usecase"
	"github.com/be/acs/api/internal/platform/httpx"
	"github.com/be/acs/api/internal/shared/kernel"
	"github.com/be/acs/api/internal/testutil"
)

type auth struct{ actor kernel.Actor }

func (a auth) Authenticate(context.Context, string) (kernel.Actor, httpx.Session, error) {
	return a.actor, httpx.Session{CSRFToken: "t"}, nil
}

// stub records inputs and returns canned results.
type stub struct {
	gen     usecase.GenerateInput
	vary    usecase.VariationInput
	edit    usecase.EditInput
	err     error
	batches []usecase.BatchView
}

func (s *stub) Estimate(context.Context, kernel.Actor, usecase.EstimateInput) (usecase.EstimateView, error) {
	return usecase.EstimateView{Total: kernel.USD("0.16"), PerJobLimit: kernel.USD("2"), GlobalRemaining: kernel.USD("900"), Lines: []usecase.EstimateLine{}}, s.err
}

func (s *stub) Generate(_ context.Context, _ kernel.Actor, in usecase.GenerateInput) (usecase.JobAccepted, error) {
	s.gen = in
	return usecase.JobAccepted{JobID: kernel.NewID(), Status: "queued", EstimatedCost: kernel.USD("0.08")}, s.err
}

func (s *stub) Edit(_ context.Context, _ kernel.Actor, _ kernel.ID, in usecase.EditInput) (usecase.JobAccepted, error) {
	s.edit = in
	return usecase.JobAccepted{JobID: kernel.NewID(), Status: "queued", EstimatedCost: kernel.USD("0.04")}, s.err
}

func (s *stub) Vary(_ context.Context, _ kernel.Actor, _ kernel.ID, in usecase.VariationInput) (usecase.JobAccepted, error) {
	s.vary = in
	return usecase.JobAccepted{JobID: kernel.NewID(), Status: "queued", EstimatedCost: kernel.USD("0.04")}, s.err
}

func (s *stub) ListBatches(context.Context, kernel.Actor, usecase.BatchFilter, kernel.PageRequest) (kernel.Page[usecase.BatchView], error) {
	return kernel.Page[usecase.BatchView]{Items: s.batches}, s.err
}

func (s *stub) ListReferences(context.Context, kernel.Actor, usecase.ReferenceFilter) ([]usecase.ReferenceCandidate, error) {
	return nil, s.err
}

func serve(t *testing.T, svc usecase.API, role, method, path, body string, idem bool) *httptest.ResponseRecorder {
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

func TestGenerateEndpoint(t *testing.T) {
	cid := kernel.NewID()
	body := `{"campaign_id":"` + cid.String() + `","prompt":"Poster","preset":"poster","reference_file_ids":[],"aspect_ratio":"4:5","quality":"standard","count":2,"model_code":null}`
	for _, tc := range []struct {
		name   string
		role   string
		body   string
		idem   bool
		status int
	}{
		{"creator", kernel.RoleContentCreator, body, true, http.StatusAccepted},
		{"lead", kernel.RoleMarketingLead, body, true, http.StatusAccepted},
		{"reviewer", kernel.RoleReviewer, body, true, http.StatusForbidden},
		{"no idempotency key", kernel.RoleContentCreator, body, false, http.StatusBadRequest},
		{"unknown field", kernel.RoleContentCreator, `{"prompt":"x","nope":1}`, true, http.StatusBadRequest},
	} {
		s := &stub{}
		rec := serve(t, s, tc.role, "POST", "/images/generations", tc.body, tc.idem)
		if rec.Code != tc.status {
			t.Errorf("%s: %d %s", tc.name, rec.Code, rec.Body)
			continue
		}
		if rec.Code != http.StatusAccepted {
			continue
		}
		var env struct {
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
		_ = json.Unmarshal(rec.Body.Bytes(), &env)
		if env.Data.Status != "queued" || env.Data.EstimatedCost.Amount != 0.08 || !env.Data.EstimatedCost.IsEstimate {
			t.Errorf("%s: body %s", tc.name, rec.Body)
		}
		if s.gen.IdempotencyKey == "" || s.gen.RequestHash == "" || s.gen.Preset != domain.PresetPoster || s.gen.Count != 2 || *s.gen.CampaignID != cid {
			t.Errorf("%s: input %+v", tc.name, s.gen)
		}
	}
}

func TestCapabilityErrorIs422(t *testing.T) {
	s := &stub{err: kernel.ErrCapability.WithDetails(kernel.Detail{"field": "aspect_ratio", "supported": []string{"1:1"}})}
	rec := serve(t, s, kernel.RoleContentCreator, "POST", "/images/generations", `{"prompt":"x","preset":"poster"}`, true)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), `"supported":["1:1"]`) {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}

func TestEditAndVariationEndpoints(t *testing.T) {
	s := &stub{}
	aid, fid := kernel.NewID(), kernel.NewID()
	rec := serve(t, s, kernel.RoleContentCreator, "POST", "/images/"+aid.String()+"/variations", `{"file_id":"`+fid.String()+`","count":3}`, true)
	if rec.Code != http.StatusAccepted || s.vary.FileID != fid || s.vary.Count != 3 {
		t.Fatalf("variations %d %s", rec.Code, rec.Body)
	}
	rec = serve(t, s, kernel.RoleContentCreator, "POST", "/images/"+aid.String()+"/edits", `{"file_id":"`+fid.String()+`","prompt":"Nền xanh","mask_file_id":null}`, true)
	if rec.Code != http.StatusAccepted || s.edit.Prompt != "Nền xanh" {
		t.Fatalf("edits %d %s", rec.Code, rec.Body)
	}
	rec = serve(t, s, kernel.RoleContentCreator, "POST", "/images/not-a-uuid/edits", `{"file_id":"`+fid.String()+`","prompt":"x"}`, true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("bad id %d", rec.Code)
	}
}

func TestListAndEstimate(t *testing.T) {
	now := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	s := &stub{batches: []usecase.BatchView{{ID: kernel.NewID(), Operation: domain.OpGenerate, BrandID: kernel.NewID(), CreatedAt: now,
		Options: domain.Options{Prompt: "x", Preset: domain.PresetPoster, AspectRatio: "1:1", Quality: "standard", Count: 1},
		Job:     usecase.BatchJob{ID: kernel.NewID(), Status: "running", Progress: 25},
		Asset:   &usecase.BatchAsset{ID: kernel.NewID(), Title: "x", Status: "draft", VersionNo: 1, RequiresTypographyCheck: true}}}}
	rec := serve(t, s, kernel.RoleContentCreator, "GET", "/images/generations?campaign_id="+kernel.NewID().String(), "", false)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"images":[]`) || !strings.Contains(rec.Body.String(), `"error_code":null`) ||
		!strings.Contains(rec.Body.String(), `"requires_typography_check":true`) {
		t.Fatalf("list %d %s", rec.Code, rec.Body)
	}
	rec = serve(t, s, kernel.RoleContentCreator, "GET", "/images/references?campaign_id="+kernel.NewID().String(), "", false)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"data":[]`) {
		t.Fatalf("references %d %s", rec.Code, rec.Body)
	}
	rec = serve(t, s, kernel.RoleContentCreator, "POST", "/images/estimate", `{"count":4,"quality":"standard","aspect_ratio":"1:1"}`, false)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"total":{"amount":0.16`) {
		t.Fatalf("estimate %d %s", rec.Code, rec.Body)
	}
	rec = serve(t, s, kernel.RoleReviewer, "GET", "/images/generations", "", false)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("reviewer list %d", rec.Code)
	}
}
