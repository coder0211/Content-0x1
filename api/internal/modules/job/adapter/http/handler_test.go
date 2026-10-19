package http_test

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	jobhttp "github.com/be/acs/api/internal/modules/job/adapter/http"
	"github.com/be/acs/api/internal/modules/job/jobtest"
	"github.com/be/acs/api/internal/modules/job/usecase"
	"github.com/be/acs/api/internal/platform/httpx"
	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
	"github.com/be/acs/api/internal/testutil"
)

type auth struct{ actor kernel.Actor }

func (a auth) Authenticate(context.Context, string) (kernel.Actor, httpx.Session, error) {
	return a.actor, httpx.Session{CSRFToken: "t"}, nil
}

type subscriber struct{ ch chan ports.JobEvent }

func (s subscriber) Subscribe(context.Context, kernel.ID) (<-chan ports.JobEvent, func(), error) {
	return s.ch, func() {}, nil
}

type fixture struct {
	svc   *usecase.Service
	sub   subscriber
	owner kernel.Actor
	jobID kernel.ID
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	mem := jobtest.NewMemory()
	f := &fixture{
		svc: &usecase.Service{Tx: mem, Repo: mem, Clock: testutil.NewClock(), Steps: usecase.NewStepRegistry(),
			Logger: slog.New(slog.NewTextHandler(io.Discard, nil))},
		sub:   subscriber{ch: make(chan ports.JobEvent, 4)},
		owner: testutil.Actor(kernel.RoleContentCreator),
	}
	res, err := f.svc.Create(context.Background(), usecase.JobSpec{
		Type: usecase.TypeContentGeneration, CreatedBy: f.owner.UserID, ProviderCode: "fake", ModelCode: "m",
		CostEstimate: kernel.USD("0.12"), Steps: []usecase.StepSpec{{Key: "gen", Type: "gen"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	f.jobID = res.JobID
	return f
}

func (f *fixture) do(t *testing.T, actor kernel.Actor, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	root, api := httpx.NewRouter(httpx.Options{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Authenticator: auth{actor}, Cookies: httpx.CookieConfig{SessionName: "s"}})
	(&jobhttp.Handler{Svc: f.svc, Subscriber: f.sub, UserNames: func(_ context.Context, ids []kernel.ID) map[kernel.ID]string {
		out := map[kernel.ID]string{}
		for _, id := range ids {
			if id == f.owner.UserID {
				out[id] = "Người sáng tạo"
			}
		}
		return out
	}}).Routes(api)
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.AddCookie(&http.Cookie{Name: "s", Value: "x"})
	req.Header.Set(httpx.HeaderCSRF, "t")
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	root.ServeHTTP(rec, req)
	return rec
}

func code(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var env struct {
		Error struct{ Code string } `json:"error"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	return env.Error.Code
}

func TestGetJobAccess(t *testing.T) {
	f := newFixture(t)
	path := "/api/v1/jobs/" + f.jobID.String()
	for _, tc := range []struct {
		name   string
		actor  kernel.Actor
		status int
	}{
		{"owner", f.owner, 200},
		{"admin", testutil.Actor(kernel.RoleAdmin), 200},
		{"other creator", testutil.Actor(kernel.RoleContentCreator), 404},
		{"reviewer", testutil.Actor(kernel.RoleReviewer), 404},
		{"anonymous", kernel.Actor{}, 401},
	} {
		if rec := f.do(t, tc.actor, "GET", path, ""); rec.Code != tc.status {
			t.Errorf("%s: %d %s", tc.name, rec.Code, rec.Body)
		}
	}
	rec := f.do(t, f.owner, "GET", path, "")
	var env struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	d := env.Data
	if d["job_type"] != "content_generation" || d["status"] != "queued" || d["error"] != nil || d["result"] != nil || d["created_by_name"] != "Người sáng tạo" {
		t.Fatalf("body %v", d)
	}
	cost := d["cost"].(map[string]any)
	if cost["estimate"] != 0.12 || cost["actual"] != nil || cost["currency"] != "USD" {
		t.Fatalf("cost %v", cost)
	}
	steps := d["steps"].([]any)
	if len(steps) != 1 || steps[0].(map[string]any)["step_key"] != "gen" || steps[0].(map[string]any)["status"] != "queued" {
		t.Fatalf("steps %v", steps)
	}
}

func TestListJobs(t *testing.T) {
	f := newFixture(t)
	rec := f.do(t, f.owner, "GET", "/api/v1/jobs?status=queued,running&type=content_generation&limit=10", "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), f.jobID.String()) || !strings.Contains(rec.Body.String(), `"created_by_name":"Người sáng tạo"`) {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	rec = f.do(t, testutil.Actor(kernel.RoleContentCreator), "GET", "/api/v1/jobs", "")
	if rec.Code != 200 || strings.Contains(rec.Body.String(), f.jobID.String()) {
		t.Fatalf("other user sees job: %s", rec.Body)
	}
	if rec := f.do(t, f.owner, "GET", "/api/v1/jobs?status=bogus", ""); rec.Code != 400 {
		t.Fatalf("bad filter: %d", rec.Code)
	}
}

func TestCancelAndRetry(t *testing.T) {
	f := newFixture(t)
	path := "/api/v1/jobs/" + f.jobID.String()
	if rec := f.do(t, testutil.Actor(kernel.RoleContentCreator), "POST", path+"/cancel", ""); rec.Code != 404 {
		t.Fatalf("stranger cancel: %d", rec.Code)
	}
	rec := f.do(t, f.owner, "POST", path+"/cancel", "")
	if rec.Code != 202 || !strings.Contains(rec.Body.String(), `"status":"cancelled"`) {
		t.Fatalf("cancel: %d %s", rec.Code, rec.Body)
	}
	rec = f.do(t, f.owner, "POST", path+"/cancel", "")
	if rec.Code != 409 || code(t, rec) != kernel.CodeInvalidStateTransition {
		t.Fatalf("cancel finished: %d %s", rec.Code, rec.Body)
	}
	rec = f.do(t, f.owner, "POST", path+"/retry", `{"force":false}`)
	if rec.Code != 409 {
		t.Fatalf("retry cancelled: %d %s", rec.Code, rec.Body)
	}
	if rec := f.do(t, f.owner, "POST", path+"/retry", `{"bogus":1}`); rec.Code != 400 {
		t.Fatalf("unknown field: %d", rec.Code)
	}
	rec = f.do(t, f.owner, "GET", path+"/history", "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), usecase.EventCancelRequested) {
		t.Fatalf("history: %d %s", rec.Code, rec.Body)
	}
}

func readEvents(t *testing.T, body string) []string {
	t.Helper()
	var names []string
	sc := bufio.NewScanner(strings.NewReader(body))
	for sc.Scan() {
		if name, ok := strings.CutPrefix(sc.Text(), "event: "); ok {
			names = append(names, name)
		}
	}
	return names
}

func TestSSEStreamsUntilDone(t *testing.T) {
	f := newFixture(t)
	f.sub.ch <- ports.JobEvent{JobID: f.jobID, Type: ports.JobEventProgress, Data: json.RawMessage(`{"progress":50}`)}
	f.sub.ch <- ports.JobEvent{JobID: f.jobID, Type: ports.JobEventDone, Data: json.RawMessage(`{"status":"succeeded"}`)}
	done := make(chan *httptest.ResponseRecorder)
	go func() { done <- f.do(t, f.owner, "GET", "/api/v1/jobs/"+f.jobID.String()+"/events", "") }()
	select {
	case rec := <-done:
		if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
			t.Fatalf("content-type %q", ct)
		}
		got := readEvents(t, rec.Body.String())
		if strings.Join(got, ",") != "status,progress,done" {
			t.Fatalf("events %v\n%s", got, rec.Body)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("stream did not end after done")
	}
	if rec := f.do(t, testutil.Actor(kernel.RoleContentCreator), "GET", "/api/v1/jobs/"+f.jobID.String()+"/events", ""); rec.Code != 404 {
		t.Fatalf("stranger SSE: %d", rec.Code)
	}
}

func TestSSETerminalJobSendsDoneImmediately(t *testing.T) {
	f := newFixture(t)
	if err := f.svc.RequestCancel(context.Background(), f.owner, f.jobID); err != nil {
		t.Fatal(err)
	}
	rec := f.do(t, f.owner, "GET", "/api/v1/jobs/"+f.jobID.String()+"/events", "")
	if got := readEvents(t, rec.Body.String()); strings.Join(got, ",") != "status,done" {
		t.Fatalf("events %v", got)
	}
}
