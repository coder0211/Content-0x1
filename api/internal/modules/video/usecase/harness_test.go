package usecase_test

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/be/acs/api/internal/modules/asset/assettest"
	"github.com/be/acs/api/internal/modules/brand/brandtest"
	"github.com/be/acs/api/internal/modules/budget/budgettest"
	"github.com/be/acs/api/internal/modules/campaign/campaigntest"
	"github.com/be/acs/api/internal/modules/content/contenttest"
	"github.com/be/acs/api/internal/modules/job/jobtest"
	jobuc "github.com/be/acs/api/internal/modules/job/usecase"
	"github.com/be/acs/api/internal/modules/provider/providertest"
	"github.com/be/acs/api/internal/modules/video/domain"
	"github.com/be/acs/api/internal/modules/video/usecase"
	"github.com/be/acs/api/internal/modules/video/videotest"
	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
	"github.com/be/acs/api/internal/testutil"
)

// env wires the video service to in-memory fakes of every dependency.
type env struct {
	t         *testing.T
	ctx       context.Context
	svc       *usecase.Service
	repo      *usecase.MemoryRepo
	jobs      *jobtest.API
	budget    *budgettest.API
	assets    *assettest.API
	brands    *brandtest.API
	campaigns *campaigntest.API
	content   *contenttest.API
	providers *providertest.API
	files     *videotest.Files
	ai        *videotest.AI
	renderer  *videotest.Renderer
	audit     *testutil.Audit
	clock     *testutil.Clock
	brandID   kernel.ID
	creator   kernel.Actor
}

func newEnv(t *testing.T) *env {
	t.Helper()
	clock := testutil.NewClock()
	e := &env{
		t: t, ctx: context.Background(), repo: usecase.NewMemoryRepo(), jobs: jobtest.NewAPI(), budget: budgettest.NewAPI(),
		assets: assettest.NewAPI(), brands: brandtest.NewAPI(), campaigns: campaigntest.NewAPI(), content: contenttest.NewAPI(),
		providers: providertest.NewAPI(), files: videotest.NewFiles(clock), ai: videotest.NewAI(), renderer: &videotest.Renderer{},
		audit: &testutil.Audit{}, clock: clock, creator: testutil.Actor(kernel.RoleContentCreator),
	}
	e.brandID = e.brands.AddSample().BrandID
	e.svc = &usecase.Service{
		Tx: &testutil.Tx{}, Repo: e.repo, Clock: clock, Audit: e.audit, Jobs: e.jobs, Budget: e.budget, Assets: e.assets,
		Brands: e.brands, Campaigns: e.campaigns, Providers: e.providers, Content: e.content, Files: e.files, AI: e.ai,
		Renderer: e.renderer, Prober: videotest.Prober{}, TempDir: t.TempDir(),
	}
	return e
}

func (e *env) createInput(target int) usecase.CreateInput {
	return usecase.CreateInput{
		BrandID: &e.brandID, Prompt: "Video quảng cáo khóa học tiếng Anh mùa thu", Preset: "social_vertical", TargetDurationSeconds: target,
		Voiceover: domain.VoiceSettings{Enabled: true}, Subtitles: domain.SubtitleSettings{Enabled: true},
		Idem: usecase.Idempotency{Key: kernel.NewID().String(), RequestHash: "h"},
	}
}

// runResult is what the fake engine did with one job.
type runResult struct {
	order  []string // keys in completion order (succeeded or failed)
	status map[string]jobuc.StepStatus
	errs   map[string]error
	polls  int
	job    jobuc.Status
	saved  map[string]json.RawMessage
}

func isRetryable(err error) bool {
	if jobuc.IsPermanent(err) {
		return false
	}
	var pe *ports.ProviderError
	if errors.As(err, &pe) {
		return pe.Retryable()
	}
	return true
}

// run executes a job's DAG like the job engine: a step starts only when
// every dependency succeeded; steps behind a failed dependency are skipped;
// WaitSpec re-runs the step with the provider job id; retryable errors are
// retried up to MaxAttempts (default 3). retryFailed reruns only failed and
// skipped steps of a previous run (POST /jobs/{id}/retry).
func (e *env) run(jobID kernel.ID, prev *runResult) *runResult {
	e.t.Helper()
	spec := e.jobs.Specs[jobID]
	view := e.jobs.Jobs[jobID]
	info := jobuc.JobInfo{ID: jobID, Type: spec.Type, CreatedBy: spec.CreatedBy, CampaignID: spec.CampaignID, AssetID: spec.AssetID,
		ProviderCode: spec.ProviderCode, ModelCode: spec.ModelCode, Request: spec.Request}
	handlers := map[string]func(context.Context, jobuc.StepContext) (jobuc.StepResult, error){
		domain.StepScript: e.svc.RunScript, domain.StepStoryboard: e.svc.RunStoryboard, domain.StepScene: e.svc.RunScene,
		domain.StepVoiceover: e.svc.RunVoiceover, domain.StepSubtitle: e.svc.RunSubtitles, domain.StepRender: e.svc.RunRender,
		domain.StepQC: e.svc.RunQualityCheck, domain.StepPublish: e.svc.RunPublish,
	}
	res := &runResult{status: map[string]jobuc.StepStatus{}, errs: map[string]error{}}
	outputs := map[string]json.RawMessage{}
	if prev != nil {
		for k, st := range prev.status {
			if st == jobuc.StepSucceeded {
				res.status[k] = st
			}
		}
		for k, v := range prev.saved {
			outputs[k] = v
		}
	}
	view.Status = jobuc.StatusRunning
	for {
		progressed := false
		for i, st := range spec.Steps {
			if _, done := res.status[st.Key]; done {
				continue
			}
			ready, blocked := true, false
			for _, d := range st.DependsOn {
				switch res.status[d] {
				case jobuc.StepSucceeded:
				case jobuc.StepFailed, jobuc.StepSkipped:
					blocked = true
				default:
					ready = false
				}
			}
			if blocked {
				res.status[st.Key] = jobuc.StepSkipped
				progressed = true
				continue
			}
			if !ready {
				continue
			}
			deps := map[string]json.RawMessage{}
			for _, d := range st.DependsOn {
				deps[d] = outputs[d]
			}
			sc := &jobtest.StepContext{JobInfo: info, ID: view.Steps[i].ID, Key: st.Key, Type: st.Type, Scene: st.SceneID, InputJSON: st.Input, Deps: deps}
			maxAttempts := st.MaxAttempts
			if maxAttempts == 0 {
				maxAttempts = 3
			}
			var out jobuc.StepResult
			var err error
			for attempt := 1; ; {
				sc.AttemptNo = attempt
				out, err = handlers[st.Type](e.ctx, sc)
				if err == nil && out.Wait != nil {
					sc.ProviderJob = out.Wait.ProviderJobID
					res.polls++
					if res.polls > 1000 {
						e.t.Fatal("endless polling")
					}
					continue
				}
				if err != nil && isRetryable(err) && attempt < maxAttempts {
					attempt++
					continue
				}
				break
			}
			if err != nil {
				res.status[st.Key], res.errs[st.Key] = jobuc.StepFailed, err
			} else {
				b, _ := json.Marshal(out.Output)
				outputs[st.Key] = b
				res.status[st.Key] = jobuc.StepSucceeded
			}
			view.Steps[i].Status = res.status[st.Key]
			res.order = append(res.order, st.Key)
			progressed = true
		}
		if !progressed {
			break
		}
	}
	res.job = jobuc.StatusSucceeded
	for _, st := range res.status {
		if st != jobuc.StepSucceeded {
			res.job = jobuc.StatusFailed
		}
	}
	res.saved = outputs
	if err := e.jobs.Finish(e.ctx, jobID, res.job); err != nil {
		e.t.Fatal(err)
	}
	return res
}

func (r *runResult) index(key string) int { return slices.Index(r.order, key) }

// storyboard creates a project and runs its storyboard job.
func (e *env) storyboard(actor kernel.Actor, in usecase.CreateInput) kernel.ID {
	e.t.Helper()
	acc, err := e.svc.Create(e.ctx, actor, in)
	if err != nil {
		e.t.Fatalf("create: %v", err)
	}
	if r := e.run(acc.JobID, nil); r.job != jobuc.StatusSucceeded {
		e.t.Fatalf("storyboard job %s: %v", r.job, r.errs)
	}
	return acc.AssetID
}

func (e *env) scenes(assetID kernel.ID) []domain.Scene {
	e.t.Helper()
	sc, err := e.repo.ListScenes(e.ctx, assetID)
	if err != nil {
		e.t.Fatal(err)
	}
	return sc
}

func (e *env) videoUsage() int {
	n := 0
	for _, u := range e.budget.Usage {
		if u.Usage.VideoSeconds > 0 {
			n++
		}
	}
	return n
}

func newIdem() usecase.Idempotency {
	return usecase.Idempotency{Key: kernel.NewID().String(), RequestHash: "h"}
}
