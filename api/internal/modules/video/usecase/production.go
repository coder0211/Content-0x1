package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/shopspring/decimal"

	budgetuc "github.com/be/acs/api/internal/modules/budget/usecase"
	jobuc "github.com/be/acs/api/internal/modules/job/usecase"
	provideruc "github.com/be/acs/api/internal/modules/provider/usecase"
	"github.com/be/acs/api/internal/modules/video/domain"
	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
)

// productionRequest is ai_jobs.request_json of video_production,
// video_scene_regenerate and video_render: the models chosen at creation
// so every step (and a retry) uses the same ones.
type productionRequest struct {
	VideoProvider  string      `json:"video_provider"`
	VideoModel     string      `json:"video_model"`
	VoiceProvider  string      `json:"voice_provider,omitempty"`
	VoiceModel     string      `json:"voice_model,omitempty"`
	SceneIDs       []kernel.ID `json:"scene_ids"`
	Voice          bool        `json:"voice"`
	Subtitles      bool        `json:"subtitles"`
	BudgetOverride bool        `json:"budget_override"`
	Preset         string      `json:"preset"`
}

// videoModel resolves the default video model and validates the preset
// against its capabilities (422 CAPABILITY_NOT_SUPPORTED, never silently
// changed).
func (s *Service) videoModel(ctx context.Context, st domain.Settings) (provideruc.ModelInfo, error) {
	m, err := s.Providers.ResolveModel(ctx, ports.AIKindVideo, "")
	if err != nil {
		return m, err
	}
	c := m.Capabilities
	if len(c.AspectRatios) > 0 && !slices.Contains(c.AspectRatios, st.AspectRatio) {
		return m, kernel.ErrCapability.WithDetails(kernel.Detail{"field": "aspect_ratio", "supported": c.AspectRatios})
	}
	if len(c.Resolutions) > 0 && !slices.Contains(c.Resolutions, st.Resolution) {
		return m, kernel.ErrCapability.WithDetails(kernel.Detail{"field": "resolution", "supported": c.Resolutions})
	}
	return m, nil
}

// checkSceneDurations validates clip lengths against the model.
func checkSceneDurations(m provideruc.ModelInfo, scenes []domain.Scene) error {
	c := m.Capabilities
	var details []kernel.Detail
	for i, sc := range scenes {
		sec := clipSeconds(sc.DurationMs)
		if (c.MinDurationSec > 0 && sec < c.MinDurationSec) || (c.MaxDurationSec > 0 && sec > c.MaxDurationSec) {
			details = append(details, kernel.Detail{"field": fmt.Sprintf("scenes[%d].duration_ms", i), "min_seconds": c.MinDurationSec, "max_seconds": c.MaxDurationSec})
		}
	}
	if len(details) > 0 {
		return kernel.ErrCapability.WithDetails(details...)
	}
	return nil
}

// clipSeconds is the provider clip length for a scene (whole seconds,
// rounded up; normalize trims to the exact scene length).
func clipSeconds(ms int) int { return (ms + 999) / 1000 }

// voiceLanguage maps a settings language to a BCP-47 TTS locale.
func voiceLanguage(l string) string {
	switch strings.ToLower(l) {
	case "", "vi", "vi-vn":
		return "vi-VN"
	case "en", "en-us":
		return "en-US"
	}
	return l
}

func (s *Service) voiceModel(ctx context.Context, st domain.Settings) (*provideruc.ModelInfo, error) {
	if !st.Voiceover.Enabled {
		return nil, nil
	}
	m, err := s.Providers.ResolveModel(ctx, ports.AIKindVoice, "")
	if err != nil {
		return nil, err
	}
	c := m.Capabilities
	if len(c.Languages) > 0 && !slices.Contains(c.Languages, voiceLanguage(st.Voiceover.Language)) {
		return nil, kernel.ErrCapability.WithDetails(kernel.Detail{"field": "voiceover.language", "supported": c.Languages})
	}
	if st.Voiceover.VoiceID != "" && len(c.Voices) > 0 && !slices.Contains(c.Voices, st.Voiceover.VoiceID) {
		return nil, kernel.ErrCapability.WithDetails(kernel.Detail{"field": "voiceover.voice_id", "supported": c.Voices})
	}
	return &m, nil
}

func hasNarration(scenes []domain.Scene) bool {
	for _, sc := range scenes {
		if strings.TrimSpace(sc.Narration) != "" {
			return true
		}
	}
	return false
}

// audioPlan decides whether the voice-over / subtitles must be (re)made:
// only when enabled and their inputs changed since the stored track.
func audioPlan(p *domain.Project, scenes []domain.Scene) (voice, subs bool) {
	st := p.Settings
	voiceFP := ""
	if st.Voiceover.Enabled && hasNarration(scenes) {
		voiceFP = domain.VoiceFingerprint(scenes, st.Voiceover)
		voice = p.Voice == nil || p.Voice.Fingerprint != voiceFP
	}
	if st.Subtitles.Enabled {
		fp := domain.SubtitleFingerprint(scenes, st.Subtitles, voiceFP, st.Preset)
		subs = p.Subtitle == nil || p.Subtitle.Fingerprint != fp
	}
	return voice, subs
}

// buildPlan selects the scenes to generate: every scene that is not ready,
// plus target (forced) for a regeneration.
func buildPlan(p *domain.Project, scenes []domain.Scene, target *domain.Scene) domain.RenderPlan {
	plan := domain.RenderPlan{SceneSeq: map[kernel.ID]int{}}
	for _, sc := range scenes {
		plan.SceneSeq[sc.ID] = sc.SequenceNo
		switch {
		case target != nil && sc.ID == target.ID:
			plan.Scenes = append(plan.Scenes, domain.SceneStepInput{SceneID: sc.ID, Force: true, BaseAttempt: sc.AttemptCount})
		case !sc.IsReady():
			plan.Scenes = append(plan.Scenes, domain.SceneStepInput{SceneID: sc.ID})
		}
	}
	plan.Voice, plan.Subtitles = audioPlan(p, scenes)
	return plan
}

// estimate prices a plan: clip seconds of the scenes to generate and TTS
// characters when the voice-over is (re)made.
func (s *Service) estimate(ctx context.Context, p *domain.Project, scenes []domain.Scene, plan domain.RenderPlan, video provideruc.ModelInfo, voice *provideruc.ModelInfo) (budgetuc.Estimate, int, error) {
	gen := map[kernel.ID]bool{}
	for _, in := range plan.Scenes {
		gen[in.SceneID] = true
	}
	secs, chars := 0, 0
	for _, sc := range scenes {
		if gen[sc.ID] {
			secs += clipSeconds(sc.DurationMs)
		}
		chars += utf8.RuneCountInString(strings.TrimSpace(sc.Narration))
	}
	var items []budgetuc.EstimateItem
	if secs > 0 && video.ProviderCode != "kenburns" {
		items = append(items, budgetuc.EstimateItem{Item: "scene_clips", ProviderCode: video.ProviderCode, ModelCode: video.ModelCode,
			Unit: "video_second_" + p.Settings.Resolution, Units: decimal.NewFromInt(int64(secs))})
	}
	if plan.Voice && voice != nil && chars > 0 {
		items = append(items, budgetuc.EstimateItem{Item: "voiceover", ProviderCode: voice.ProviderCode, ModelCode: voice.ModelCode,
			Unit: "char", Units: decimal.NewFromInt(int64(chars))})
	}
	if len(items) == 0 {
		return budgetuc.Estimate{Total: kernel.ZeroUSD()}, secs, nil
	}
	est, err := s.Budget.Estimate(ctx, budgetuc.EstimateRequest{Category: budgetuc.CategoryVideo, Items: items})
	return est, secs, err
}

// Estimate is POST /videos/{id}/estimate: the cost of producing the current
// storyboard (scenes already ready are reused and not counted).
func (s *Service) Estimate(ctx context.Context, actor kernel.Actor, assetID kernel.ID) (EstimateView, error) {
	p, err := s.loadEditable(ctx, actor, assetID)
	if err != nil {
		return EstimateView{}, err
	}
	scenes, err := s.Repo.ListScenes(ctx, assetID)
	if err != nil {
		return EstimateView{}, err
	}
	video, err := s.videoModel(ctx, p.Settings)
	if err != nil {
		return EstimateView{}, err
	}
	voice, err := s.voiceModel(ctx, p.Settings)
	if err != nil {
		return EstimateView{}, err
	}
	plan := buildPlan(p, scenes, nil)
	est, secs, err := s.estimate(ctx, p, scenes, plan, video, voice)
	if err != nil {
		return EstimateView{}, err
	}
	lim, err := s.Budget.Limits(ctx, p.CampaignID, budgetuc.CategoryVideo, est.Total)
	if err != nil {
		return EstimateView{}, err
	}
	violations := domain.ValidateStoryboard(domain.ScenesAsDrafts(scenes))
	if violations == nil {
		violations = []kernel.Detail{}
	}
	if est.Lines == nil {
		est.Lines = []budgetuc.EstimateLine{}
	}
	return EstimateView{SceneCount: len(scenes), TotalClipSeconds: secs, Lines: est.Lines, Total: est.Total, Limits: lim, Violations: violations}, nil
}

// Produce is POST /videos/{id}/productions: budget gate, then the
// video_production DAG for every scene that is not ready yet.
func (s *Service) Produce(ctx context.Context, actor kernel.Actor, assetID kernel.ID, in ProduceInput) (Accepted, error) {
	p, err := s.loadEditable(ctx, actor, assetID)
	if err != nil {
		return Accepted{}, err
	}
	scenes, err := s.Repo.ListScenes(ctx, assetID)
	if err != nil {
		return Accepted{}, err
	}
	if issues := domain.ValidateStoryboard(domain.ScenesAsDrafts(scenes)); len(issues) > 0 {
		return Accepted{}, kernel.Validation(issues...)
	}
	return s.startRender(ctx, actor, p, scenes, jobuc.TypeVideoProduction, nil, in.BudgetOverride, in.Idem, nil)
}

// RegenerateScene is POST /videos/{id}/scenes/{sceneId}/regenerations:
// regenerate one scene (optionally with a new visual prompt) and re-render,
// reusing every other ready clip.
func (s *Service) RegenerateScene(ctx context.Context, actor kernel.Actor, assetID kernel.ID, in RegenerateInput) (Accepted, error) {
	p, err := s.loadEditable(ctx, actor, assetID)
	if err != nil {
		return Accepted{}, err
	}
	scenes, err := s.Repo.ListScenes(ctx, assetID)
	if err != nil {
		return Accepted{}, err
	}
	idx := slices.IndexFunc(scenes, func(sc domain.Scene) bool { return sc.ID == in.SceneID })
	if idx < 0 {
		return Accepted{}, kernel.ErrNotFound
	}
	var newPrompt string
	if in.VisualPrompt != nil {
		newPrompt = strings.TrimSpace(*in.VisualPrompt)
		if newPrompt == "" {
			return Accepted{}, kernel.Validation(kernel.FieldIssue("visual_prompt", "required"))
		}
		if utf8.RuneCountInString(newPrompt) > domain.MaxSceneTextLen {
			return Accepted{}, kernel.Validation(kernel.Detail{"field": "visual_prompt", "issue": "too_long", "max": domain.MaxSceneTextLen})
		}
		scenes[idx].VisualPrompt = newPrompt
	}
	target := scenes[idx]
	mutate := func(ctx context.Context) error {
		if newPrompt == "" {
			return nil
		}
		sc, err := s.Repo.GetScene(ctx, target.ID)
		if err != nil {
			return err
		}
		sc.VisualPrompt = newPrompt
		return s.Repo.UpdateScene(ctx, sc)
	}
	return s.startRender(ctx, actor, p, scenes, jobuc.TypeVideoSceneRegenerate, &target, in.BudgetOverride, in.Idem, mutate)
}

// Rerender is POST /videos/{id}/renders: optionally change voice-over,
// subtitles or music, then render again without generating any scene.
func (s *Service) Rerender(ctx context.Context, actor kernel.Actor, assetID kernel.ID, in RenderInput) (Accepted, error) {
	p, err := s.loadEditable(ctx, actor, assetID)
	if err != nil {
		return Accepted{}, err
	}
	scenes, err := s.Repo.ListScenes(ctx, assetID)
	if err != nil {
		return Accepted{}, err
	}
	if len(scenes) == 0 || slices.ContainsFunc(scenes, func(sc domain.Scene) bool { return !sc.IsReady() }) {
		return Accepted{}, kernel.ErrInvalidTransition.WithMessage("Cần sinh xong mọi cảnh trước khi render lại").
			WithDetails(kernel.Detail{"reason": "scenes_not_ready"})
	}
	var v kernel.ValidationErrors
	_ = normalizeAudio(in.Voiceover, in.Subtitles, &v)
	if err := v.Err(); err != nil {
		return Accepted{}, err
	}
	if in.Voiceover != nil {
		p.Settings.Voiceover = *in.Voiceover
	}
	if in.Subtitles != nil {
		p.Settings.Subtitles = *in.Subtitles
	}
	if in.Music != nil {
		if err := s.checkMusic(ctx, p.BrandID, *in.Music); err != nil {
			return Accepted{}, err
		}
		p.Settings.Music = *in.Music
	}
	settings := p.Settings
	mutate := func(ctx context.Context) error {
		cur, err := s.Repo.GetProject(ctx, assetID)
		if err != nil {
			return err
		}
		cur.Settings = settings
		return s.Repo.UpdateProject(ctx, cur)
	}
	return s.startRender(ctx, actor, p, scenes, jobuc.TypeVideoRender, nil, in.BudgetOverride, in.Idem, mutate)
}

// startRender estimates, gates and creates a render-family job atomically
// with its reservation, the override audit and the project's current job.
func (s *Service) startRender(ctx context.Context, actor kernel.Actor, p *domain.Project, scenes []domain.Scene, jobType jobuc.JobType,
	target *domain.Scene, override bool, idem Idempotency, mutate func(context.Context) error) (Accepted, error) {
	if err := s.checkMusic(ctx, p.BrandID, p.Settings.Music); err != nil {
		return Accepted{}, err
	}
	video, err := s.videoModel(ctx, p.Settings)
	if err != nil {
		return Accepted{}, err
	}
	if err := checkSceneDurations(video, scenes); err != nil {
		return Accepted{}, err
	}
	voice, err := s.voiceModel(ctx, p.Settings)
	if err != nil {
		return Accepted{}, err
	}
	plan := buildPlan(p, scenes, target)
	if jobType == jobuc.TypeVideoRender {
		plan.Scenes = nil
	}
	est, _, err := s.estimate(ctx, p, scenes, plan, video, voice)
	if err != nil {
		return Accepted{}, err
	}
	overrideBy, err := s.gate(ctx, actor, p.CampaignID, est.Total, override)
	if err != nil {
		return Accepted{}, err
	}
	steps, err := toStepSpecs(domain.RenderSteps(plan))
	if err != nil {
		return Accepted{}, err
	}
	req := productionRequest{VideoProvider: video.ProviderCode, VideoModel: video.ModelCode, Voice: plan.Voice, Subtitles: plan.Subtitles,
		BudgetOverride: overrideBy != nil, Preset: p.Settings.Preset, SceneIDs: []kernel.ID{}}
	if voice != nil {
		req.VoiceProvider, req.VoiceModel = voice.ProviderCode, voice.ModelCode
	}
	for _, sc := range plan.Scenes {
		req.SceneIDs = append(req.SceneIDs, sc.SceneID)
	}
	reqJSON, _ := json.Marshal(req)
	spec := jobuc.JobSpec{
		Type: jobType, CreatedBy: actor.UserID, CampaignID: p.CampaignID, AssetID: &p.AssetID,
		ProviderCode: video.ProviderCode, ModelCode: video.ModelCode, Request: reqJSON,
		IdempotencyKey: idem.Key, RequestHash: idem.RequestHash, CostEstimate: est.Total, PricingVersion: est.PricingVersion, Steps: steps,
	}
	if overrideBy != nil {
		spec.BudgetOverrideBy = &overrideBy.UserID
	}
	var out Accepted
	err = s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		locked, err := s.Repo.LockProject(ctx, p.AssetID)
		if err != nil {
			return err
		}
		res, err := s.Jobs.Create(ctx, spec)
		if err != nil {
			return err
		}
		out = Accepted{AssetID: p.AssetID, JobID: res.JobID, Status: res.Status, Estimate: est.Total}
		if res.Existing {
			return nil
		}
		if err := s.requireIdle(ctx, locked); err != nil {
			return err
		}
		if _, err := s.Budget.Reserve(ctx, budgetuc.ReserveRequest{JobID: res.JobID, CampaignID: p.CampaignID, Amount: est.Total,
			Category: budgetuc.CategoryVideo, Override: overrideBy}); err != nil {
			return err
		}
		if overrideBy != nil {
			lim, _ := s.Budget.Limits(ctx, p.CampaignID, budgetuc.CategoryVideo, est.Total)
			if err := s.Audit.Record(ctx, "budget.override", "ai_job", res.JobID.String(), nil, map[string]any{
				"asset_id": p.AssetID, "job_type": jobType, "estimate": est.Total.Float64(), "per_job_limit": lim.PerJobLimit.Float64(),
			}); err != nil {
				return err
			}
		}
		if mutate != nil {
			if err := mutate(ctx); err != nil {
				return err
			}
		}
		cur, err := s.Repo.GetProject(ctx, p.AssetID)
		if err != nil {
			return err
		}
		cur.CurrentJobID = &res.JobID
		return s.Repo.UpdateProject(ctx, cur)
	})
	if err != nil {
		return Accepted{}, err
	}
	return out, nil
}
