package usecase_test

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	assetuc "github.com/be/acs/api/internal/modules/asset/usecase"
	branduc "github.com/be/acs/api/internal/modules/brand/usecase"
	budgetuc "github.com/be/acs/api/internal/modules/budget/usecase"
	contentuc "github.com/be/acs/api/internal/modules/content/usecase"
	"github.com/be/acs/api/internal/modules/job/jobtest"
	jobuc "github.com/be/acs/api/internal/modules/job/usecase"
	"github.com/be/acs/api/internal/modules/video/domain"
	"github.com/be/acs/api/internal/modules/video/usecase"
	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
	"github.com/be/acs/api/internal/testutil"
)

func code(err error) string {
	if de, ok := kernel.AsDomainError(err); ok {
		return de.Code
	}
	if err == nil {
		return ""
	}
	return "other:" + err.Error()
}

// ACS-060 AC: storyboard 20s 9:16 → 4–6 scenes with duration, visual
// prompt, narration and subtitle.
func TestCreateStoryboard20s(t *testing.T) {
	e := newEnv(t)
	in := e.createInput(20)
	acc, err := e.svc.Create(e.ctx, e.creator, in)
	if err != nil {
		t.Fatal(err)
	}
	spec := e.jobs.Specs[acc.JobID]
	if spec.Type != jobuc.TypeVideoStoryboard || len(spec.Steps) != 2 || spec.Steps[1].DependsOn[0] != domain.StepScript {
		t.Fatalf("job spec %+v", spec)
	}
	if _, ok := e.budget.Reserved[acc.JobID]; !ok {
		t.Fatal("storyboard job not reserved")
	}
	a, _ := e.assets.Get(e.ctx, acc.AssetID)
	if a.Type != assetuc.TypeVideo || a.Subtype != "social_vertical" {
		t.Fatalf("asset %+v", a)
	}
	// Idempotent replay returns the same job and asset, and no new job or
	// reservation (the real Tx rolls back the replay's asset/project).
	again, err := e.svc.Create(e.ctx, e.creator, in)
	if err != nil || again.JobID != acc.JobID || again.AssetID != acc.AssetID || len(e.jobs.Jobs) != 1 || len(e.budget.Reserved) != 1 {
		t.Fatalf("replay: %+v %v", again, err)
	}

	r := e.run(acc.JobID, nil)
	if r.job != jobuc.StatusSucceeded || r.index(domain.StepScript) > r.index(domain.StepStoryboard) {
		t.Fatalf("run %+v", r)
	}
	scenes := e.scenes(acc.AssetID)
	if len(scenes) < 4 || len(scenes) > 6 {
		t.Fatalf("want 4–6 scenes, got %d", len(scenes))
	}
	for i, sc := range scenes {
		if sc.DurationMs < 2000 || sc.DurationMs > 8000 || sc.VisualPrompt == "" || sc.Narration == "" || sc.SubtitleText == "" ||
			sc.SequenceNo != i+1 || sc.Status != domain.SceneDraft {
			t.Errorf("scene %d incomplete: %+v", i, sc)
		}
	}
	if total := domain.TotalMs(scenes); total != 20000 {
		t.Errorf("total %d", total)
	}
	if len(e.ai.TextCalls) != 2 || e.budget.Usage[0].JobType != "video_storyboard" {
		t.Errorf("text calls %d usage %+v", len(e.ai.TextCalls), e.budget.Usage)
	}
	if !strings.Contains(e.ai.TextCalls[0].Messages[0].Content, "<user_data>") {
		t.Error("user data must be fenced")
	}
	v, err := e.svc.Get(e.ctx, e.creator, acc.AssetID)
	if err != nil || v.Stage != domain.StageEditing || len(v.Scenes) != len(scenes) || v.Progress != 100 {
		t.Fatalf("view %+v %v", v, err)
	}
}

func TestCreateFromContentScript(t *testing.T) {
	e := newEnv(t)
	scriptID := kernel.NewID()
	e.content.Scripts[scriptID] = contentuc.VideoScript{AssetID: scriptID, Title: "Kịch bản thu", Hook: "Bạn muốn nói tiếng Anh?", Script: "Lời thoại", CTA: "Đăng ký ngay",
		Scenes: []contentuc.ScriptScene{{Narration: "Mở đầu", VisualHint: "Học viên bước vào lớp", DurationHint: 4}, {Narration: "Kết", VisualHint: "Logo", DurationHint: 4}}}
	in := e.createInput(8)
	in.Prompt, in.ScriptFromAssetID = "", &scriptID
	acc, err := e.svc.Create(e.ctx, e.creator, in)
	if err != nil {
		t.Fatal(err)
	}
	if r := e.run(acc.JobID, nil); r.job != jobuc.StatusSucceeded {
		t.Fatal(r.errs)
	}
	if len(e.ai.TextCalls) != 0 {
		t.Fatalf("imported script must not call the text model, got %d calls", len(e.ai.TextCalls))
	}
	p, _ := e.repo.GetProject(e.ctx, acc.AssetID)
	if p.Title != "Kịch bản thu" || !strings.Contains(p.Script, "Đăng ký ngay") || p.Settings.Source.Type != domain.SourceScript {
		t.Fatalf("project %+v", p)
	}
	scenes := e.scenes(acc.AssetID)
	if len(scenes) != 2 || scenes[0].VisualPrompt != "Học viên bước vào lớp" {
		t.Fatalf("scenes %+v", scenes)
	}
	a, _ := e.assets.Get(e.ctx, acc.AssetID)
	if a.SourceAssetID == nil || *a.SourceAssetID != scriptID {
		t.Fatal("source asset not linked")
	}

	missing := kernel.NewID()
	in.ScriptFromAssetID, in.Idem = &missing, newIdem()
	if _, err := e.svc.Create(e.ctx, e.creator, in); code(err) != kernel.CodeValidation {
		t.Fatalf("missing script: %v", err)
	}
}

func TestCreateValidationAndPermissions(t *testing.T) {
	e := newEnv(t)
	for name, mut := range map[string]func(*usecase.CreateInput){
		"preset":     func(in *usecase.CreateInput) { in.Preset = "4:5" },
		"aspect":     func(in *usecase.CreateInput) { in.AspectRatio = "16:9" },
		"duration":   func(in *usecase.CreateInput) { in.TargetDurationSeconds = 61 },
		"resolution": func(in *usecase.CreateInput) { in.Resolution = "4k" },
		"prompt":     func(in *usecase.CreateInput) { in.Prompt = "" },
		"brand":      func(in *usecase.CreateInput) { in.BrandID = nil },
		"images":     func(in *usecase.CreateInput) { in.Source = domain.Source{Type: domain.SourceImage} },
		"voice mode": func(in *usecase.CreateInput) { in.Voiceover.Mode = "duet" },
	} {
		in := e.createInput(20)
		mut(&in)
		if _, err := e.svc.Create(e.ctx, e.creator, in); code(err) != kernel.CodeValidation {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := e.svc.Create(e.ctx, testutil.Actor(kernel.RoleReviewer), e.createInput(20)); !errors.Is(err, kernel.ErrForbidden) {
		t.Errorf("reviewer create: %v", err)
	}
	// Another user's source image is not usable.
	in := e.createInput(20)
	in.Source = domain.Source{Type: domain.SourceImage, FileIDs: []kernel.ID{e.files.AddImage(e.ctx, kernel.NewID())}}
	if _, err := e.svc.Create(e.ctx, e.creator, in); code(err) != kernel.CodeValidation {
		t.Errorf("foreign image: %v", err)
	}
	if len(e.jobs.Jobs) != 0 {
		t.Fatal("no job may be created by invalid requests")
	}
}

func licensedTrack(e *env, license string) kernel.ID {
	id := kernel.NewID()
	e.brands.Assets[id] = branduc.BrandAsset{ID: id, BrandID: e.brandID, FileID: e.files.AddAudio(e.ctx), AssetType: "music", Label: "Nhạc nền", License: json.RawMessage(license)}
	return id
}

func TestMusicLicenseGate(t *testing.T) {
	e := newEnv(t)
	in := e.createInput(10)
	bad := licensedTrack(e, `{"source":"youtube"}`)
	in.Music = domain.MusicSettings{Enabled: true, TrackBrandAssetID: &bad}
	if _, err := e.svc.Create(e.ctx, e.creator, in); code(err) != usecase.CodeMusicNotLicensed {
		t.Fatalf("unlicensed music: %v", err)
	}
	other := kernel.NewID()
	e.brands.Assets[other] = branduc.BrandAsset{ID: other, BrandID: kernel.NewID(), AssetType: "music", License: json.RawMessage(`{"source":"a","type":"b"}`)}
	in.Music.TrackBrandAssetID = &other
	if _, err := e.svc.Create(e.ctx, e.creator, in); code(err) != kernel.CodeValidation {
		t.Fatalf("other brand's music: %v", err)
	}
	good := licensedTrack(e, `{"source":"Artlist","type":"commercial"}`)
	in.Music.TrackBrandAssetID, in.Idem = &good, newIdem()
	id := e.storyboard(e.creator, in)
	acc, err := e.svc.Produce(e.ctx, e.creator, id, usecase.ProduceInput{Idem: newIdem()})
	if err != nil {
		t.Fatal(err)
	}
	if r := e.run(acc.JobID, nil); r.job != jobuc.StatusSucceeded {
		t.Fatal(r.errs)
	}
	last := e.renderer.Renders[len(e.renderer.Renders)-1]
	if last.Music == nil || !last.Music.Duck || last.Music.Volume >= 1 {
		t.Fatalf("music not mixed with ducking: %+v", last.Music)
	}
	// Re-render with an unlicensed track is refused before any job exists.
	jobs := len(e.jobs.Jobs)
	if _, err := e.svc.Rerender(e.ctx, e.creator, id, usecase.RenderInput{Music: &domain.MusicSettings{Enabled: true, TrackBrandAssetID: &bad}, Idem: newIdem()}); code(err) != usecase.CodeMusicNotLicensed {
		t.Fatalf("rerender unlicensed: %v", err)
	}
	if len(e.jobs.Jobs) != jobs {
		t.Fatal("job created for unlicensed music")
	}
}

func TestSaveScenes(t *testing.T) {
	e := newEnv(t)
	id := e.storyboard(e.creator, e.createInput(20))
	scenes := e.scenes(id)
	in := make([]domain.SceneDraftInput, 0, len(scenes)+1)
	for i := len(scenes) - 1; i >= 0; i-- { // reverse order
		sid := scenes[i].ID
		in = append(in, domain.SceneDraftInput{ID: &sid, DurationMs: scenes[i].DurationMs, VisualPrompt: scenes[i].VisualPrompt, Narration: scenes[i].Narration, SubtitleText: scenes[i].SubtitleText})
	}
	in = in[1:] // remove one
	in = append(in, domain.SceneDraftInput{DurationMs: 3000, VisualPrompt: " Cảnh mới ", Narration: "Mới"})
	v, err := e.svc.SaveScenes(e.ctx, e.creator, id, in)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Scenes) != len(scenes) || v.Scenes[0].ID != scenes[len(scenes)-2].ID || v.Scenes[len(v.Scenes)-1].VisualPrompt != "Cảnh mới" {
		t.Fatalf("saved %+v", v.Scenes)
	}
	for i, sc := range v.Scenes {
		if sc.SequenceNo != i+1 {
			t.Fatalf("sequence %d = %d", i, sc.SequenceNo)
		}
	}
	// Over 60 s is refused with the limit in details.
	long := make([]domain.SceneDraftInput, 8)
	for i := range long {
		long[i] = domain.SceneDraftInput{DurationMs: 8000, VisualPrompt: "x"}
	}
	_, err = e.svc.SaveScenes(e.ctx, e.creator, id, long)
	de, _ := kernel.AsDomainError(err)
	if de == nil || de.Code != kernel.CodeValidation || de.Details[0]["issue"] != "total_duration_exceeded" {
		t.Fatalf("over limit: %v", err)
	}
	// Unknown scene id.
	ghost := kernel.NewID()
	if _, err := e.svc.SaveScenes(e.ctx, e.creator, id, []domain.SceneDraftInput{{ID: &ghost, DurationMs: 3000, VisualPrompt: "x"}}); code(err) != kernel.CodeValidation {
		t.Fatalf("ghost: %v", err)
	}
	// Other creator: not found; reviewer: forbidden.
	if _, err := e.svc.SaveScenes(e.ctx, testutil.Actor(kernel.RoleContentCreator), id, in); !errors.Is(err, kernel.ErrNotFound) {
		t.Fatalf("idor: %v", err)
	}
	if _, err := e.svc.SaveScenes(e.ctx, testutil.Actor(kernel.RoleReviewer), id, in); !errors.Is(err, kernel.ErrForbidden) {
		t.Fatalf("reviewer: %v", err)
	}
	// While a job runs the storyboard is locked.
	if _, err := e.svc.Produce(e.ctx, e.creator, id, usecase.ProduceInput{Idem: newIdem()}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.SaveScenes(e.ctx, e.creator, id, in); code(err) != usecase.CodeJobInProgress {
		t.Fatalf("locked: %v", err)
	}
}

func TestGetAndListAccess(t *testing.T) {
	e := newEnv(t)
	id := e.storyboard(e.creator, e.createInput(10))
	other := testutil.Actor(kernel.RoleContentCreator)
	if _, err := e.svc.Get(e.ctx, other, id); !errors.Is(err, kernel.ErrNotFound) {
		t.Fatalf("idor get: %v", err)
	}
	for _, a := range []kernel.Actor{e.creator, testutil.Actor(kernel.RoleReviewer), testutil.Actor(kernel.RoleMarketingLead)} {
		if _, err := e.svc.Get(e.ctx, a, id); err != nil {
			t.Fatalf("%v: %v", a.Roles, err)
		}
	}
	items, _, err := e.svc.List(e.ctx, other, nil, kernel.PageRequest{Limit: 10})
	if err != nil || len(items) != 0 {
		t.Fatalf("other's list: %d %v", len(items), err)
	}
	items, _, err = e.svc.List(e.ctx, e.creator, nil, kernel.PageRequest{Limit: 10})
	if err != nil || len(items) != 1 {
		t.Fatalf("own list: %d %v", len(items), err)
	}
}

// ACS-064: estimate + per-job gate; only Lead/Admin may override (audited).
func TestEstimateAndBudgetGate(t *testing.T) {
	e := newEnv(t)
	id := e.storyboard(e.creator, e.createInput(20))
	est, err := e.svc.Estimate(e.ctx, e.creator, id)
	if err != nil {
		t.Fatal(err)
	}
	if est.SceneCount < 4 || est.TotalClipSeconds != 20 || len(est.Lines) != 2 || !est.Total.GreaterThan(kernel.ZeroUSD()) || len(est.Violations) != 0 {
		t.Fatalf("estimate %+v", est)
	}
	e.budget.PerJob[budgetuc.CategoryVideo] = kernel.USD("1")
	est, _ = e.svc.Estimate(e.ctx, e.creator, id)
	if !est.Limits.RequiresOverride {
		t.Fatal("estimate must flag the per-job limit")
	}
	_, err = e.svc.Produce(e.ctx, e.creator, id, usecase.ProduceInput{Idem: newIdem()})
	if !errors.Is(err, kernel.ErrJobBudgetExceeded) {
		t.Fatalf("creator over limit: %v", err)
	}
	de, _ := kernel.AsDomainError(err)
	if de.Details[0]["per_job_limit"] != 1.0 || !strings.Contains(de.Message, "Giảm số cảnh") {
		t.Fatalf("details %+v %s", de.Details, de.Message)
	}
	if _, err := e.svc.Produce(e.ctx, e.creator, id, usecase.ProduceInput{BudgetOverride: true, Idem: newIdem()}); !errors.Is(err, kernel.ErrForbidden) {
		t.Fatalf("creator override: %v", err)
	}
	if len(e.jobs.Jobs) != 1 {
		t.Fatal("refused productions must not create jobs")
	}
	lead := testutil.Actor(kernel.RoleMarketingLead)
	acc, err := e.svc.Produce(e.ctx, lead, id, usecase.ProduceInput{BudgetOverride: true, Idem: newIdem()})
	if err != nil {
		t.Fatal(err)
	}
	if spec := e.jobs.Specs[acc.JobID]; spec.BudgetOverrideBy == nil || *spec.BudgetOverrideBy != lead.UserID {
		t.Fatal("override not recorded on the job")
	}
	// budget.Reserve audits the override (budget.override); video must not add a duplicate row.
	if e.budget.Overrides[acc.JobID] != "per_job" || slices.Contains(e.audit.Actions(), "budget.override") {
		t.Fatalf("override %v audit %v", e.budget.Overrides, e.audit.Actions())
	}
}

// ACS-069 / ACS-063 / ACS-065 / ACS-066 / ACS-067: the production DAG runs
// scenes in parallel, voice before subtitles and render only after all of
// them, then QC and publish (new asset version).
func TestProductionDAGOrder(t *testing.T) {
	e := newEnv(t)
	e.ai.PollsBeforeDone = 2
	id := e.storyboard(e.creator, e.createInput(20))
	scenes := e.scenes(id)
	acc, err := e.svc.Produce(e.ctx, e.creator, id, usecase.ProduceInput{Idem: newIdem()})
	if err != nil {
		t.Fatal(err)
	}
	spec := e.jobs.Specs[acc.JobID]
	if spec.Type != jobuc.TypeVideoProduction || len(spec.Steps) != len(scenes)+5 {
		t.Fatalf("spec steps %d", len(spec.Steps))
	}
	v, _ := e.svc.Get(e.ctx, e.creator, id)
	if v.Stage != domain.StageProducing {
		t.Fatalf("stage %s", v.Stage)
	}
	r := e.run(acc.JobID, nil)
	if r.job != jobuc.StatusSucceeded {
		t.Fatalf("job %s %v", r.job, r.errs)
	}
	render := r.index(domain.StepRender)
	for _, sc := range scenes {
		if i := r.index(domain.SceneStepKey(sc.SequenceNo)); i < 0 || i > render {
			t.Errorf("scene %d after render", sc.SequenceNo)
		}
	}
	if r.index(domain.StepVoiceover) >= r.index(domain.StepSubtitle) || r.index(domain.StepSubtitle) >= render ||
		render >= r.index(domain.StepQC) || r.index(domain.StepQC) >= r.index(domain.StepPublish) {
		t.Fatalf("order %v", r.order)
	}
	if r.polls < len(scenes)*3 {
		t.Errorf("scenes must poll asynchronously via WaitSpec, polls=%d", r.polls)
	}
	for _, sc := range e.scenes(id) {
		if !sc.IsReady() || sc.ThumbnailFileID == nil || sc.AttemptCount != 1 {
			t.Errorf("scene not ready: %+v", sc)
		}
	}
	if e.ai.SubmitCount() != len(scenes) || e.videoUsage() != len(scenes) {
		t.Errorf("submits %d usage %d", e.ai.SubmitCount(), e.videoUsage())
	}
	if len(e.ai.VoiceCalls) != len(scenes) || e.ai.VoiceCalls[0].Language != "vi-VN" {
		t.Errorf("voice calls %+v", e.ai.VoiceCalls)
	}
	if e.renderer.RenderCount() != 1 {
		t.Fatalf("renders %d", e.renderer.RenderCount())
	}
	rs := e.renderer.Renders[0]
	if len(rs.Clips) != len(scenes) || rs.Format.Width != 1080 || rs.Format.Height != 1920 || rs.Transition != "fade" ||
		!rs.BurnIn || !strings.HasSuffix(rs.Subtitles, ".ass") || len(rs.VoiceSegments) != len(scenes) || rs.Music != nil {
		t.Fatalf("render spec %+v", rs)
	}
	p, _ := e.repo.GetProject(e.ctx, id)
	if p.Output == nil || p.Output.VersionNo != 2 || p.Output.SubtitleFileID == nil || p.Output.ThumbnailFileID == nil || p.Subtitle.CueCount == 0 {
		t.Fatalf("output %+v", p.Output)
	}
	a, _ := e.assets.Get(e.ctx, id)
	if a.CurrentVersion.VersionNo != 2 || len(a.CurrentVersion.Files) != 3 || a.CurrentVersion.GeneratedByJobID == nil {
		t.Fatalf("version %+v", a.CurrentVersion)
	}
	v, _ = e.svc.Get(e.ctx, e.creator, id)
	if v.Stage != domain.StageReady || v.Output == nil || v.Output.URL == "" || v.Progress != 100 {
		t.Fatalf("view stage %s output %+v", v.Stage, v.Output)
	}
}

// ACS-063 AC: 5 scenes, 1 fails → 4 ready, job failed, render never runs;
// the retry does not regenerate the 4 ready scenes.
func TestPartialSceneFailure(t *testing.T) {
	e := newEnv(t)
	id := e.storyboard(e.creator, e.createInput(25))
	scenes := e.scenes(id)
	if len(scenes) != 5 {
		t.Fatalf("want 5 scenes, got %d", len(scenes))
	}
	bad := scenes[2].VisualPrompt
	e.ai.FailVideo = func(prompt string) *ports.ProviderError {
		if prompt == bad {
			return &ports.ProviderError{Kind: ports.ErrKindContentPolicy, Provider: "fake", Message: "blocked by policy"}
		}
		return nil
	}
	acc, err := e.svc.Produce(e.ctx, e.creator, id, usecase.ProduceInput{Idem: newIdem()})
	if err != nil {
		t.Fatal(err)
	}
	r := e.run(acc.JobID, nil)
	if r.job != jobuc.StatusFailed {
		t.Fatalf("job %s", r.job)
	}
	failedKey := domain.SceneStepKey(3)
	if r.status[failedKey] != jobuc.StepFailed || !jobuc.IsPermanent(r.errs[failedKey]) {
		t.Fatalf("scene 3: %s %v", r.status[failedKey], r.errs[failedKey])
	}
	for _, k := range []string{domain.StepRender, domain.StepQC, domain.StepPublish} {
		if r.status[k] != jobuc.StepSkipped {
			t.Errorf("%s must not run, got %s", k, r.status[k])
		}
	}
	if e.renderer.RenderCount() != 0 {
		t.Fatal("render ran with a failed scene")
	}
	ready := 0
	for _, sc := range e.scenes(id) {
		switch {
		case sc.IsReady():
			ready++
		case sc.SequenceNo == 3 && sc.Status == domain.SceneFailed && sc.LastError != "":
		default:
			t.Errorf("unexpected scene %+v", sc)
		}
	}
	if ready != 4 || e.ai.SubmitCount() != 5 {
		t.Fatalf("ready %d submits %d", ready, e.ai.SubmitCount())
	}
	v, _ := e.svc.Get(e.ctx, e.creator, id)
	if v.Stage != domain.StageFailed {
		t.Fatalf("stage %s", v.Stage)
	}

	// Job retry: failed/skipped steps rerun; ready scenes are reused.
	e.ai.FailVideo = nil
	r2 := e.run(acc.JobID, r)
	if r2.job != jobuc.StatusSucceeded {
		t.Fatalf("retry %v", r2.errs)
	}
	if e.ai.SubmitCount() != 6 || e.videoUsage() != 5 {
		t.Fatalf("retry regenerated ready scenes: submits %d usage %d", e.ai.SubmitCount(), e.videoUsage())
	}
	// Re-running a succeeded scene step (e.g. redelivery) reuses the clip.
	before := e.ai.SubmitCount()
	sc := e.scenes(id)[0]
	in, _ := json.Marshal(domain.SceneStepInput{SceneID: sc.ID})
	res, err := e.svc.RunScene(e.ctx, stepCtx(e, acc.JobID, domain.SceneStepKey(1), domain.StepScene, in))
	if err != nil || e.ai.SubmitCount() != before || res.Output.(map[string]any)["reused"] != true {
		t.Fatalf("redelivery: %v %v", res, err)
	}
}

// A new production after a failure only generates the scenes not ready.
func TestProduceAgainOnlyMissingScenes(t *testing.T) {
	e := newEnv(t)
	id := e.storyboard(e.creator, e.createInput(15))
	scenes := e.scenes(id)
	bad := scenes[0].VisualPrompt
	e.ai.FailVideo = func(p string) *ports.ProviderError {
		if p == bad {
			return &ports.ProviderError{Kind: ports.ErrKindInvalidRequest, Provider: "fake", Message: "bad"}
		}
		return nil
	}
	acc, _ := e.svc.Produce(e.ctx, e.creator, id, usecase.ProduceInput{Idem: newIdem()})
	e.run(acc.JobID, nil)
	e.ai.FailVideo = nil
	acc2, err := e.svc.Produce(e.ctx, e.creator, id, usecase.ProduceInput{Idem: newIdem()})
	if err != nil {
		t.Fatal(err)
	}
	sceneSteps := 0
	for _, st := range e.jobs.Specs[acc2.JobID].Steps {
		if domain.IsSceneStep(st.Key) {
			sceneSteps++
		}
	}
	if sceneSteps != 1 {
		t.Fatalf("second production generates %d scenes, want 1", sceneSteps)
	}
	if r := e.run(acc2.JobID, nil); r.job != jobuc.StatusSucceeded {
		t.Fatal(r.errs)
	}
}

// Transient provider errors are retried; the poll resumes the submission.
func TestTransientSubmitRetried(t *testing.T) {
	e := newEnv(t)
	id := e.storyboard(e.creator, e.createInput(8))
	n := 0
	e.ai.FailVideo = func(string) *ports.ProviderError {
		n++
		if n == 1 {
			return &ports.ProviderError{Kind: ports.ErrKindTransient, Provider: "fake", Message: "503"}
		}
		return nil
	}
	acc, _ := e.svc.Produce(e.ctx, e.creator, id, usecase.ProduceInput{Idem: newIdem()})
	if r := e.run(acc.JobID, nil); r.job != jobuc.StatusSucceeded {
		t.Fatal(r.errs)
	}
}

func stepCtx(e *env, jobID kernel.ID, key, typ string, in json.RawMessage) jobuc.StepContext {
	spec := e.jobs.Specs[jobID]
	return &jobtest.StepContext{JobInfo: jobuc.JobInfo{ID: jobID, Type: spec.Type, CreatedBy: spec.CreatedBy, AssetID: spec.AssetID,
		ProviderCode: spec.ProviderCode, ModelCode: spec.ModelCode, Request: spec.Request}, ID: kernel.NewID(), Key: key, Type: typ, InputJSON: in}
}

// ACS-068 AC: regenerating scene 3 → exactly one new video usage record,
// the other clips are reused, and the asset gets a new version.
func TestRegenerateSceneAndRerender(t *testing.T) {
	e := newEnv(t)
	id := e.storyboard(e.creator, e.createInput(20))
	acc, _ := e.svc.Produce(e.ctx, e.creator, id, usecase.ProduceInput{Idem: newIdem()})
	if r := e.run(acc.JobID, nil); r.job != jobuc.StatusSucceeded {
		t.Fatal(r.errs)
	}
	before := e.scenes(id)
	usage, submits, voice := e.videoUsage(), e.ai.SubmitCount(), len(e.ai.VoiceCalls)
	newPrompt := "Cận cảnh giáo viên mỉm cười"
	acc, err := e.svc.RegenerateScene(e.ctx, e.creator, id, usecase.RegenerateInput{SceneID: before[2].ID, VisualPrompt: &newPrompt, Idem: newIdem()})
	if err != nil {
		t.Fatal(err)
	}
	spec := e.jobs.Specs[acc.JobID]
	if spec.Type != jobuc.TypeVideoSceneRegenerate || spec.Steps[0].Key != domain.SceneStepKey(3) || len(spec.Steps) != 4 {
		t.Fatalf("regenerate steps %+v", spec.Steps)
	}
	r := e.run(acc.JobID, nil)
	if r.job != jobuc.StatusSucceeded {
		t.Fatal(r.errs)
	}
	if e.videoUsage() != usage+1 || e.ai.SubmitCount() != submits+1 || len(e.ai.VoiceCalls) != voice {
		t.Fatalf("usage %d→%d submits %d→%d voice %d→%d", usage, e.videoUsage(), submits, e.ai.SubmitCount(), voice, len(e.ai.VoiceCalls))
	}
	if last := e.ai.Submits[len(e.ai.Submits)-1]; last.Prompt != newPrompt {
		t.Fatalf("regenerated with %q", last.Prompt)
	}
	after := e.scenes(id)
	for i := range after {
		same := *after[i].ClipFileID == *before[i].ClipFileID
		if (i == 2) == same {
			t.Errorf("scene %d clip reuse wrong (same=%v)", i+1, same)
		}
	}
	if after[2].AttemptCount != 2 {
		t.Errorf("attempts %d", after[2].AttemptCount)
	}
	p, _ := e.repo.GetProject(e.ctx, id)
	if p.Output.VersionNo != 3 {
		t.Fatalf("version %d", p.Output.VersionNo)
	}
	// Retrying the finished regenerate step does not regenerate again.
	in := spec.Steps[0].Input
	if _, err := e.svc.RunScene(e.ctx, stepCtx(e, acc.JobID, spec.Steps[0].Key, domain.StepScene, in)); err != nil || e.ai.SubmitCount() != submits+1 {
		t.Fatalf("forced step replay resubmitted: %v", err)
	}

	// video_render: subtitles turned to soft → no scene/voice calls, new version.
	acc, err = e.svc.Rerender(e.ctx, e.creator, id, usecase.RenderInput{Subtitles: &domain.SubtitleSettings{Enabled: true, Style: "soft"}, Idem: newIdem()})
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range e.jobs.Specs[acc.JobID].Steps {
		if domain.IsSceneStep(st.Key) || st.Key == domain.StepVoiceover {
			t.Fatalf("render job contains %s", st.Key)
		}
	}
	if r := e.run(acc.JobID, nil); r.job != jobuc.StatusSucceeded {
		t.Fatal(r.errs)
	}
	last := e.renderer.Renders[len(e.renderer.Renders)-1]
	if last.BurnIn || !strings.HasSuffix(last.Subtitles, ".srt") || e.ai.SubmitCount() != submits+1 || len(e.ai.VoiceCalls) != voice {
		t.Fatalf("soft subtitles render %+v", last)
	}
	p, _ = e.repo.GetProject(e.ctx, id)
	if p.Output.VersionNo != 4 || p.Settings.Subtitles.Style != "soft" {
		t.Fatalf("after rerender %+v", p.Output)
	}
	// Unknown scene → not found.
	if _, err := e.svc.RegenerateScene(e.ctx, e.creator, id, usecase.RegenerateInput{SceneID: kernel.NewID(), Idem: newIdem()}); !errors.Is(err, kernel.ErrNotFound) {
		t.Fatalf("unknown scene: %v", err)
	}
}

func TestRerenderNeedsReadyScenes(t *testing.T) {
	e := newEnv(t)
	id := e.storyboard(e.creator, e.createInput(10))
	if _, err := e.svc.Rerender(e.ctx, e.creator, id, usecase.RenderInput{Idem: newIdem()}); !errors.Is(err, kernel.ErrInvalidTransition) {
		t.Fatalf("rerender drafts: %v", err)
	}
}

// ACS-067: a broken render is rendered once more; a second failure fails
// quality_check and the asset gets no new version.
func TestQualityCheck(t *testing.T) {
	for _, tc := range []struct {
		corrupt int
		ok      bool
	}{{1, true}, {2, false}} {
		e := newEnv(t)
		id := e.storyboard(e.creator, e.createInput(10))
		e.renderer.CorruptRenders = tc.corrupt
		acc, _ := e.svc.Produce(e.ctx, e.creator, id, usecase.ProduceInput{Idem: newIdem()})
		r := e.run(acc.JobID, nil)
		if e.renderer.RenderCount() != 2 {
			t.Fatalf("corrupt=%d: renders %d", tc.corrupt, e.renderer.RenderCount())
		}
		a, _ := e.assets.Get(e.ctx, id)
		if tc.ok {
			if r.job != jobuc.StatusSucceeded || a.CurrentVersion.VersionNo != 2 {
				t.Fatalf("rerender should pass: %v", r.errs)
			}
			continue
		}
		if r.status[domain.StepQC] != jobuc.StepFailed || code(r.errs[domain.StepQC]) != usecase.CodeQualityCheck || r.status[domain.StepPublish] != jobuc.StepSkipped {
			t.Fatalf("qc: %v %v", r.status, r.errs[domain.StepQC])
		}
		if a.CurrentVersion.VersionNo != 1 {
			t.Fatal("broken video published")
		}
	}
}

func TestStillClipFallbacks(t *testing.T) {
	// Stub provider bytes that cannot be normalised → Ken Burns clip.
	e := newEnv(t)
	e.svc.StubVideoProviders = []string{"fake"}
	e.renderer.FailNormalize = true
	id := e.storyboard(e.creator, e.createInput(8))
	acc, _ := e.svc.Produce(e.ctx, e.creator, id, usecase.ProduceInput{Idem: newIdem()})
	if r := e.run(acc.JobID, nil); r.job != jobuc.StatusSucceeded || e.renderer.Stills == 0 {
		t.Fatalf("stub fallback: %v stills=%d", r.errs, e.renderer.Stills)
	}
	// Without the stub flag a normalize failure fails the scene.
	e = newEnv(t)
	e.renderer.FailNormalize = true
	id = e.storyboard(e.creator, e.createInput(8))
	acc, _ = e.svc.Produce(e.ctx, e.creator, id, usecase.ProduceInput{Idem: newIdem()})
	if r := e.run(acc.JobID, nil); r.job != jobuc.StatusFailed || e.renderer.RenderCount() != 0 {
		t.Fatal("normalize failure must fail the job")
	}
}

func TestKenBurnsProviderCostsNothing(t *testing.T) {
	e := newEnv(t)
	for i, m := range e.providers.Models {
		if m.Kind == ports.AIKindVideo {
			e.providers.Models[i].ProviderCode = usecase.KenBurnsProvider
		}
	}
	in := e.createInput(10)
	in.Voiceover.Enabled = false
	id := e.storyboard(e.creator, in)
	est, err := e.svc.Estimate(e.ctx, e.creator, id)
	if err != nil || !est.Total.IsZero() {
		t.Fatalf("kenburns estimate %+v %v", est, err)
	}
	acc, _ := e.svc.Produce(e.ctx, e.creator, id, usecase.ProduceInput{Idem: newIdem()})
	if r := e.run(acc.JobID, nil); r.job != jobuc.StatusSucceeded || e.ai.SubmitCount() != 0 || e.renderer.Stills == 0 {
		t.Fatalf("kenburns run %v submits %d", r.errs, e.ai.SubmitCount())
	}
}
