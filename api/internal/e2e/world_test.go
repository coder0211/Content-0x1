package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	assethttp "github.com/be/acs/api/internal/modules/asset/adapter/http"
	"github.com/be/acs/api/internal/modules/asset/assettest"
	assetuc "github.com/be/acs/api/internal/modules/asset/usecase"
	brandhttp "github.com/be/acs/api/internal/modules/brand/adapter/http"
	"github.com/be/acs/api/internal/modules/brand/brandtest"
	branduc "github.com/be/acs/api/internal/modules/brand/usecase"
	budgethttp "github.com/be/acs/api/internal/modules/budget/adapter/http"
	"github.com/be/acs/api/internal/modules/budget/budgettest"
	budgetuc "github.com/be/acs/api/internal/modules/budget/usecase"
	camphttp "github.com/be/acs/api/internal/modules/campaign/adapter/http"
	"github.com/be/acs/api/internal/modules/campaign/campaigntest"
	campaignuc "github.com/be/acs/api/internal/modules/campaign/usecase"
	contenthttp "github.com/be/acs/api/internal/modules/content/adapter/http"
	contentworker "github.com/be/acs/api/internal/modules/content/adapter/worker"
	contentuc "github.com/be/acs/api/internal/modules/content/usecase"
	filehttp "github.com/be/acs/api/internal/modules/file/adapter/http"
	filedomain "github.com/be/acs/api/internal/modules/file/domain"
	fileuc "github.com/be/acs/api/internal/modules/file/usecase"
	identitydomain "github.com/be/acs/api/internal/modules/identity/domain"
	"github.com/be/acs/api/internal/modules/identity/identitytest"
	identityuc "github.com/be/acs/api/internal/modules/identity/usecase"
	imagehttp "github.com/be/acs/api/internal/modules/image/adapter/http"
	imageworker "github.com/be/acs/api/internal/modules/image/adapter/worker"
	"github.com/be/acs/api/internal/modules/image/imagetest"
	imageuc "github.com/be/acs/api/internal/modules/image/usecase"
	jobhttp "github.com/be/acs/api/internal/modules/job/adapter/http"
	"github.com/be/acs/api/internal/modules/job/jobtest"
	jobuc "github.com/be/acs/api/internal/modules/job/usecase"
	providerhttp "github.com/be/acs/api/internal/modules/provider/adapter/http"
	providerdomain "github.com/be/acs/api/internal/modules/provider/domain"
	provideruc "github.com/be/acs/api/internal/modules/provider/usecase"
	videohttp "github.com/be/acs/api/internal/modules/video/adapter/http"
	videoworker "github.com/be/acs/api/internal/modules/video/adapter/worker"
	videouc "github.com/be/acs/api/internal/modules/video/usecase"
	"github.com/be/acs/api/internal/platform/httpx"
	"github.com/be/acs/api/internal/providers"
	"github.com/be/acs/api/internal/providers/fake"
	"github.com/be/acs/api/internal/shared/kernel"
	"github.com/be/acs/api/internal/testutil"
)

// world is every business module built from its REAL use-case service and
// HTTP handler over in-memory repositories (the <m>test packages), wired
// the way app.Modules wires them, plus the real job engine driven in
// process (DispatchOnce + Execute) with the fake AI provider.
type world struct {
	t       *testing.T
	clock   *testutil.Clock
	storage *testutil.Storage
	audit   *auditAPI
	queue   *memQueue
	budgetS *budgettest.Store

	identity *identityuc.Users
	files    *fileuc.Service
	provider *provideruc.Service
	jobs     *jobuc.Service
	budget   *budgetuc.Service
	brands   *branduc.Service
	camps    *campaignuc.Service
	assets   *assetuc.Service
	content  *contentuc.Service
	images   *imageuc.Service
	videos   *videouc.Service

	actors map[string]kernel.Actor // session token → actor
	router chi.Router
}

// auditAPI satisfies audit.API (AuditRecorder + Notifier) in memory.
type auditAPI struct {
	*testutil.Audit
	*testutil.Notifier
}

// memQueue is a jobuc.TaskQueue collecting tasks for in-process execution.
type memQueue struct {
	mu    sync.Mutex
	tasks []jobuc.Task
	ids   map[string]bool
}

func (q *memQueue) Enqueue(_ context.Context, t jobuc.Task) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.ids == nil {
		q.ids = map[string]bool{}
	}
	if !q.ids[t.ID] {
		q.ids[t.ID] = true
		q.tasks = append(q.tasks, t)
	}
	return nil
}

func (q *memQueue) take() []jobuc.Task {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := q.tasks
	q.tasks = nil
	return out
}

// tx joins the budget store's row-lock transaction and the job store's
// snapshot transaction, so every use case runs in "one" transaction like
// Postgres (nested calls join the outer one).
type tx struct {
	budget *budgettest.Store
	jobs   *jobtest.Memory
}

func (t tx) WithinTx(ctx context.Context, fn func(context.Context) error) error {
	return t.budget.WithinTx(ctx, func(ctx context.Context) error { return t.jobs.WithinTx(ctx, fn) })
}

// memFiles is an in-memory file.Repository.
type memFiles struct {
	mu sync.Mutex
	m  map[kernel.ID]filedomain.File
}

func (r *memFiles) Create(_ context.Context, f *filedomain.File) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[f.ID] = *f
	return nil
}

func (r *memFiles) Get(_ context.Context, id kernel.ID) (*filedomain.File, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	f, ok := r.m[id]
	if !ok {
		return nil, kernel.ErrNotFound
	}
	return &f, nil
}

func (r *memFiles) GetMany(_ context.Context, ids []kernel.ID) ([]filedomain.File, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []filedomain.File{}
	for _, id := range ids {
		if f, ok := r.m[id]; ok {
			out = append(out, f)
		}
	}
	return out, nil
}

func (r *memFiles) MarkReady(_ context.Context, id kernel.ID, v fileuc.Verified) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	f := r.m[id]
	f.UploadStatus, f.MimeType, f.Kind, f.SizeBytes, f.ChecksumSHA256 = filedomain.StatusReady, v.MimeType, v.Kind, v.SizeBytes, v.Checksum
	f.Width, f.Height, f.DurationMs = v.Width, v.Height, v.DurationMs
	if v.StorageKey != "" {
		f.StorageKey = v.StorageKey
	}
	r.m[id] = f
	return nil
}

func (r *memFiles) MarkRejected(_ context.Context, id kernel.ID, _ string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	f := r.m[id]
	f.UploadStatus = filedomain.StatusRejected
	r.m[id] = f
	return nil
}

func (r *memFiles) ListStalePending(context.Context, time.Time, int) ([]fileuc.StaleFile, error) {
	return nil, nil
}

func (r *memFiles) Delete(_ context.Context, id kernel.ID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.m, id)
	return nil
}

func newWorld(t *testing.T) *world {
	t.Helper()
	w := &world{t: t, clock: testutil.NewClock(), storage: testutil.NewStorage(), audit: &auditAPI{&testutil.Audit{}, &testutil.Notifier{}},
		queue: &memQueue{}, budgetS: budgettest.NewStore(), actors: map[string]kernel.Actor{}}
	log := discardLogger()
	jobMem := jobtest.NewMemory()
	txm := tx{budget: w.budgetS, jobs: jobMem}

	// AI providers: the real registry with the fake provider (OQ-01).
	reg := providers.NewRegistry()
	for _, a := range fake.New().Adapters() {
		reg.Register(a)
	}

	users, sessions := identitytest.NewUsers(), identitytest.NewSessions()
	w.identity = &identityuc.Users{Tx: txm, Users: users, Sessions: sessions, Hasher: identitytest.PlainHasher{}, Clock: w.clock, Audit: w.audit}
	for _, role := range kernel.AllRoles {
		for _, suffix := range []string{"", "2"} {
			u := &identitydomain.User{ID: kernel.NewID(), Email: role + suffix + "@acs.local", Name: role + suffix, PasswordHash: "x",
				Status: identitydomain.StatusActive, Roles: []string{role}, CreatedAt: w.clock.Now(), UpdatedAt: w.clock.Now()}
			if err := users.Create(context.Background(), u); err != nil {
				t.Fatal(err)
			}
			w.actors[role+suffix] = kernel.Actor{UserID: u.ID, Roles: u.Roles, Permissions: kernel.RolePermissions[role], Email: u.Email, Name: u.Name}
		}
	}

	w.files = &fileuc.Service{Repo: &memFiles{m: map[kernel.ID]filedomain.File{}}, Storage: w.storage, Clock: w.clock,
		Limits: filedomain.Limits{Image: 20 << 20, Video: 500 << 20, Audio: 50 << 20, Document: 20 << 20}, URLTTL: 15 * time.Minute,
		TempDir: t.TempDir(), Logger: log}

	provRepo := &provideruc.MemoryRepo{}
	provRepo.Seed("fake", []string{"text", "image", "video", "voice"},
		providerdomain.Model{ModelCode: "fake-text", Kind: "text", IsDefault: true},
		providerdomain.Model{ModelCode: "fake-image", Kind: "image", IsDefault: true,
			Capabilities: []byte(`{"aspect_ratios":["1:1","4:5","9:16","16:9"],"qualities":["standard","high"],"max_variants":4,"supports_ref_image":true,"supports_edit":true}`)},
		providerdomain.Model{ModelCode: "fake-video", Kind: "video", IsDefault: true},
		providerdomain.Model{ModelCode: "fake-voice", Kind: "voice", IsDefault: true})
	w.provider = &provideruc.Service{Tx: txm, Repo: provRepo, Clock: w.clock, Audit: w.audit, Registry: reg}
	if err := w.provider.SyncRegistry(context.Background()); err != nil {
		t.Fatal(err)
	}

	steps := jobuc.NewStepRegistry()
	w.jobs = &jobuc.Service{Tx: txm, Repo: jobMem, Clock: w.clock, Steps: steps, Events: &testutil.Events{}, Notifier: w.audit,
		Queue: w.queue, Logger: log, WorkerID: "e2e", Rand: func() float64 { return 0.5 }}

	w.budget = &budgetuc.Service{Tx: txm, Repo: w.budgetS, Clock: w.clock, Audit: w.audit, Notifier: w.audit, Users: w.identity, Logger: log}
	w.jobs.OnFinished(func(ctx context.Context, ev jobuc.JobFinished) error { return w.budget.Settle(ctx, ev.JobID) })

	w.brands = &branduc.Service{Tx: txm, Repo: brandtest.NewRepo(), Clock: w.clock, Audit: w.audit, Files: w.files}
	w.camps = &campaignuc.Service{Tx: txm, Repo: campaigntest.NewRepo(), Clock: w.clock, Audit: w.audit, Brands: w.brands, Users: w.identity}
	w.assets = &assetuc.Service{Tx: txm, Repo: assettest.NewRepo(), Clock: w.clock, Audit: w.audit, Notifier: w.audit,
		Files: w.files, Identity: w.identity, Campaigns: w.camps, Logger: log}
	w.content = &contentuc.Service{Tx: txm, Clock: w.clock, Jobs: w.jobs, Budget: w.budget, Assets: w.assets, Brands: w.brands,
		Campaigns: w.camps, Providers: w.provider, AI: reg}
	w.images = &imageuc.Service{Tx: txm, Clock: w.clock, Repo: imagetest.NewRepo(), Jobs: w.jobs, Budget: w.budget, Assets: w.assets,
		Campaigns: w.camps, Brands: w.brands, Files: w.files, Providers: w.provider, AI: reg, Logger: log}
	w.videos = &videouc.Service{Tx: txm, Repo: videouc.NewMemoryRepo(), Clock: w.clock, Audit: w.audit, Jobs: w.jobs, Budget: w.budget,
		Assets: w.assets, Brands: w.brands, Campaigns: w.camps, Providers: w.provider, Content: w.content, Files: w.files, AI: reg,
		Logger: log, TempDir: t.TempDir()}

	handlers := append(contentworker.Handlers(w.content), &imageworker.ImageStep{Svc: w.images})
	handlers = append(handlers, videoworker.Handlers(w.videos)...)
	for _, h := range handlers {
		if err := steps.Register(h); err != nil {
			t.Fatal(err)
		}
	}

	root, api := httpx.NewRouter(httpx.Options{Logger: log, Authenticator: tokenAuth(w.actors), Cookies: testCookies})
	for _, r := range []interface{ Routes(chi.Router) }{
		&filehttp.Handler{Svc: w.files}, &providerhttp.Handler{Svc: w.provider}, &jobhttp.Handler{Svc: w.jobs},
		&budgethttp.Handler{Svc: w.budget}, &brandhttp.Handler{Svc: w.brands}, &camphttp.Handler{Svc: w.camps},
		&assethttp.Handler{Svc: w.assets}, &contenthttp.Handler{Svc: w.content}, &imagehttp.Handler{Svc: w.images},
		&videohttp.Handler{Svc: w.videos},
	} {
		r.Routes(api)
	}
	w.router = root
	return w
}

// call sends a JSON request as the actor behind token ("" = anonymous) and
// decodes the envelope's data into out (when non-nil and 2xx).
func (w *world) call(token, method, path string, body any, out any) *httptest.ResponseRecorder {
	w.t.Helper()
	rec, _ := doRequest(w.t, w.router, method, httpx.APIPrefix+path, token, body)
	if out != nil && rec.Code < 300 {
		var env struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			w.t.Fatalf("%s %s: %v: %s", method, path, err, rec.Body.String())
		}
		if err := json.Unmarshal(env.Data, out); err != nil {
			w.t.Fatalf("%s %s: %v: %s", method, path, err, rec.Body.String())
		}
	}
	return rec
}

// must is call + a status check.
func (w *world) must(status int, token, method, path string, body any, out any) {
	w.t.Helper()
	if rec := w.call(token, method, path, body, out); rec.Code != status {
		w.t.Fatalf("%s %s as %s: got %d, want %d: %s", method, path, token, rec.Code, status, rec.Body.String())
	}
}

// runJobs drives the job engine like the worker: dispatch the outbox, run
// the queued tasks, until nothing is left (or rounds run out).
func (w *world) runJobs() {
	w.t.Helper()
	ctx := context.Background()
	for range 20 {
		if _, err := w.jobs.DispatchOnce(ctx); err != nil {
			w.t.Fatal(err)
		}
		tasks := w.queue.take()
		if len(tasks) == 0 {
			return
		}
		for _, task := range tasks {
			var p jobuc.TaskPayload
			if err := json.Unmarshal(task.Payload, &p); err != nil {
				w.t.Fatal(err)
			}
			if err := w.jobs.Execute(ctx, p); err != nil {
				w.t.Logf("execute %s: %v", p.StepType, err)
			}
		}
		w.clock.Advance(time.Minute)
	}
}

// seed creates, through the real HTTP API, a brand with one product (as
// lead) and returns their ids.
func (w *world) seedBrand() (brandID, productID kernel.ID) {
	w.t.Helper()
	var b struct {
		ID kernel.ID `json:"id"`
	}
	w.must(http.StatusCreated, kernel.RoleMarketingLead, "POST", "/brands", map[string]any{
		"name": "Anh ngữ " + kernel.NewID().String()[:6],
		"profile": map[string]any{"description": "Trung tâm tiếng Anh", "tone_of_voice": []string{"friendly"},
			"forbidden_words": []string{"rẻ nhất"}, "default_ctas": []string{"Đăng ký tư vấn"}},
	}, &b)
	var p struct {
		ID kernel.ID `json:"id"`
	}
	w.must(http.StatusCreated, kernel.RoleMarketingLead, "POST", "/brands/"+b.ID.String()+"/products", map[string]any{
		"name": "Khóa giao tiếp 12 tuần", "description": "Giao tiếp",
		"facts": map[string]any{"usp": []string{"Giáo viên bản ngữ"}, "price": map[string]any{"amount": 2990000, "currency": "VND", "note": "trọn khóa"},
			"policies": []string{"Hoàn phí trong 7 ngày"}},
	}, &p)
	return b.ID, p.ID
}

// seedCampaign creates a campaign as token (a creator) for brand/product.
func (w *world) seedCampaign(token string, brandID, productID kernel.ID) kernel.ID {
	w.t.Helper()
	var c struct {
		ID kernel.ID `json:"id"`
	}
	w.must(http.StatusCreated, token, "POST", "/campaigns", map[string]any{
		"brand_id": brandID, "name": "Khóa giao tiếp T11", "goal": "lead",
		"brief": map[string]any{"product_ids": []kernel.ID{productID}, "target_audience": "Sinh viên năm 2-4", "key_message": "Tự tin giao tiếp sau 12 tuần",
			"channels": []string{"facebook"}, "formats": []map[string]any{{"type": "facebook_post", "count": 3}}, "deadline": "2026-11-15", "notes": ""},
		"start_date": "2026-11-01", "end_date": "2026-12-31",
	}, &c)
	return c.ID
}

// generateContent starts a content generation as token and runs the job to
// completion; it returns the job id and the created asset ids.
func (w *world) generateContent(token string, campaignID, brandID, productID kernel.ID, variants int) (kernel.ID, []kernel.ID) {
	w.t.Helper()
	var acc struct {
		JobID kernel.ID `json:"job_id"`
	}
	w.must(http.StatusAccepted, token, "POST", "/content/generations", map[string]any{
		"campaign_id": campaignID, "brand_id": brandID, "platform": "facebook", "content_type": "promotional_post",
		"objective": "lead", "framework": "PAS", "length": "medium", "language": "vi", "tone": []string{"friendly"},
		"brief":    map[string]any{"product_id": productID, "target_audience": "Sinh viên", "key_message": "Luyện giao tiếp", "cta": "Đăng ký tư vấn"},
		"variants": variants,
	}, &acc)
	w.runJobs()
	var job struct {
		Status string `json:"status"`
		Result struct {
			AssetIDs []kernel.ID `json:"asset_ids"`
		} `json:"result"`
		Error any `json:"error"`
	}
	w.must(http.StatusOK, token, "GET", "/jobs/"+acc.JobID.String(), nil, &job)
	if job.Status != "succeeded" || len(job.Result.AssetIDs) != variants {
		w.t.Fatalf("job %s: status %s, assets %v, error %v", acc.JobID, job.Status, job.Result.AssetIDs, job.Error)
	}
	return acc.JobID, job.Result.AssetIDs
}
