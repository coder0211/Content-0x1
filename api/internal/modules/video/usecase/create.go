package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/shopspring/decimal"

	assetuc "github.com/be/acs/api/internal/modules/asset/usecase"
	branduc "github.com/be/acs/api/internal/modules/brand/usecase"
	budgetuc "github.com/be/acs/api/internal/modules/budget/usecase"
	jobuc "github.com/be/acs/api/internal/modules/job/usecase"
	"github.com/be/acs/api/internal/modules/video/domain"
	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
)

// errReplay aborts the creation transaction when the job module reports an
// idempotent replay, so nothing created in it (asset, project) survives.
var errReplay = errors.New("idempotent replay")

// storyboardRequest is ai_jobs.request_json of video_storyboard.
type storyboardRequest struct {
	TargetDurationSeconds int    `json:"target_duration_seconds"`
	SceneCount            int    `json:"scene_count"`
	Preset                string `json:"preset"`
}

// Create validates the request, creates the video asset + project and
// starts the video_storyboard job (script → storyboard → scenes).
func (s *Service) Create(ctx context.Context, actor kernel.Actor, in CreateInput) (Accepted, error) {
	if err := actor.Require(kernel.PermGenerateRun); err != nil {
		return Accepted{}, err
	}
	settings, err := normalizeCreate(&in)
	if err != nil {
		return Accepted{}, err
	}
	brandID, err := s.resolveBrand(ctx, actor, in.CampaignID, in.BrandID)
	if err != nil {
		return Accepted{}, err
	}
	script, title := "", strings.TrimSpace(in.Title)
	var scriptScenes []domain.RawScene
	if in.ScriptFromAssetID != nil {
		vs, err := s.Content.GetVideoScript(ctx, actor, *in.ScriptFromAssetID)
		if errors.Is(err, kernel.ErrNotFound) {
			return Accepted{}, kernel.Validation(kernel.FieldIssue("script_from_asset_id", "not_found"))
		}
		if err != nil {
			return Accepted{}, err
		}
		script = composeScript(vs.Hook, vs.Script, vs.CTA)
		if title == "" {
			title = vs.Title
		}
		for _, sc := range vs.Scenes {
			scriptScenes = append(scriptScenes, domain.RawScene{VisualPrompt: sc.VisualHint, Narration: sc.Narration, DurationSeconds: float64(sc.DurationHint)})
		}
	}
	if err := s.checkSourceFiles(ctx, actor, "source.file_ids", in.Source.FileIDs); err != nil {
		return Accepted{}, err
	}
	if err := s.checkMusic(ctx, brandID, settings.Music); err != nil {
		return Accepted{}, err
	}
	if _, err := s.videoModel(ctx, settings); err != nil {
		return Accepted{}, err
	}
	text, err := s.Providers.ResolveModel(ctx, ports.AIKindText, "")
	if err != nil {
		return Accepted{}, err
	}
	if title == "" {
		title = defaultTitle(in.Prompt)
	}
	sceneCount := domain.SceneCountFor(settings.TargetDurationSeconds)
	inputChars := utf8.RuneCountInString(in.Prompt) + utf8.RuneCountInString(script)
	est, err := s.Budget.Estimate(ctx, budgetuc.EstimateRequest{Category: budgetuc.CategoryVideo, Items: []budgetuc.EstimateItem{
		{Item: "script_storyboard", ProviderCode: text.ProviderCode, ModelCode: text.ModelCode, Unit: "input_token", Units: decimal.NewFromInt(int64(2 * (inputChars/3 + 1500)))},
		{Item: "script_storyboard", ProviderCode: text.ProviderCode, ModelCode: text.ModelCode, Unit: "output_token", Units: decimal.NewFromInt(int64(2 * 1500))},
	}})
	if err != nil {
		return Accepted{}, err
	}
	if _, err := s.gate(ctx, actor, in.CampaignID, est.Total, false); err != nil {
		return Accepted{}, err
	}
	content, _ := json.Marshal(map[string]any{
		"prompt": in.Prompt, "preset": settings.Preset, "aspect_ratio": settings.AspectRatio, "script": script,
		"scene_ids": []string{}, "final_file_id": nil, "subtitle_file_id": nil,
	})
	reqJSON, _ := json.Marshal(storyboardRequest{TargetDurationSeconds: settings.TargetDurationSeconds, SceneCount: sceneCount, Preset: settings.Preset})
	stepSpecs, err := toStepSpecs(domain.StoryboardSteps())
	if err != nil {
		return Accepted{}, err
	}
	// A storyboard proposed by Content Studio (scene breakdown with visual
	// hints) is kept as the script; the storyboard step reuses it.
	if len(scriptScenes) > 0 {
		b, _ := json.Marshal(map[string]any{"script_scenes": scriptScenes})
		stepSpecs[1].Input = b
	}
	var out Accepted
	var replay jobuc.CreateResult
	err = s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		ref, err := s.Assets.CreateAsset(ctx, assetuc.CreateAssetInput{
			CampaignID: in.CampaignID, BrandID: brandID, Type: assetuc.TypeVideo, Subtype: settings.Preset, Title: title,
			SourceAssetID: in.ScriptFromAssetID, CreatedBy: actor.UserID,
			Version: assetuc.NewVersion{ContentJSON: content, ChangeNote: "Tạo dự án video", CreatedBy: actor.UserID, SearchText: in.Prompt + " " + script},
		})
		if err != nil {
			return err
		}
		now := s.Clock.Now()
		p := &domain.Project{
			AssetID: ref.AssetID, BrandID: brandID, CampaignID: in.CampaignID, Title: title, Prompt: in.Prompt, Script: script,
			Settings: settings, CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now,
		}
		if err := s.Repo.CreateProject(ctx, p); err != nil {
			return err
		}
		res, err := s.Jobs.Create(ctx, jobuc.JobSpec{
			Type: jobuc.TypeVideoStoryboard, CreatedBy: actor.UserID, CampaignID: in.CampaignID, AssetID: &ref.AssetID,
			ProviderCode: text.ProviderCode, ModelCode: text.ModelCode, Request: reqJSON,
			IdempotencyKey: in.Idem.Key, RequestHash: in.Idem.RequestHash,
			CostEstimate: est.Total, PricingVersion: est.PricingVersion, Steps: stepSpecs,
		})
		if err != nil {
			return err
		}
		if res.Existing {
			replay = res
			return errReplay
		}
		if _, err := s.Budget.Reserve(ctx, budgetuc.ReserveRequest{JobID: res.JobID, CampaignID: in.CampaignID, Amount: est.Total, Category: budgetuc.CategoryVideo}); err != nil {
			return err
		}
		p.CurrentJobID = &res.JobID
		if err := s.Repo.UpdateProject(ctx, p); err != nil {
			return err
		}
		out = Accepted{AssetID: ref.AssetID, JobID: res.JobID, Status: res.Status, Estimate: est.Total}
		return nil
	})
	if errors.Is(err, errReplay) {
		j, err := s.Jobs.Get(ctx, replay.JobID)
		if err != nil {
			return Accepted{}, err
		}
		if j.AssetID == nil {
			return Accepted{}, kernel.ErrIdempotencyConflict
		}
		return Accepted{AssetID: *j.AssetID, JobID: j.ID, Status: j.Status, Estimate: replay.CostEstimate}, nil
	}
	return out, err
}

func composeScript(parts ...string) string {
	var out []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, "\n\n")
}

func defaultTitle(prompt string) string {
	t := strings.Join(strings.Fields(prompt), " ")
	if t == "" {
		return "Video mới"
	}
	if r := []rune(t); len(r) > 60 {
		return strings.TrimSpace(string(r[:60])) + "…"
	}
	return t
}

// normalizeCreate validates POST /videos and fills defaults.
func normalizeCreate(in *CreateInput) (domain.Settings, error) {
	var v kernel.ValidationErrors
	in.Prompt = strings.TrimSpace(in.Prompt)
	preset, ok := domain.PresetFor(in.Preset)
	if !ok {
		v.AddDetail(kernel.Detail{"field": "preset", "issue": "invalid", "allowed": []string{"social_vertical", "square", "landscape"}})
	} else if in.AspectRatio != "" && in.AspectRatio != preset.AspectRatio {
		v.AddDetail(kernel.Detail{"field": "aspect_ratio", "issue": "mismatch", "expected": preset.AspectRatio})
	}
	if in.TargetDurationSeconds == 0 {
		in.TargetDurationSeconds = 20
	}
	if in.TargetDurationSeconds < domain.MinTargetSeconds || in.TargetDurationSeconds > domain.MaxTargetSeconds {
		v.AddDetail(kernel.Detail{"field": "target_duration_seconds", "issue": "out_of_range", "min": domain.MinTargetSeconds, "max": domain.MaxTargetSeconds})
	}
	if in.Resolution == "" {
		in.Resolution = "1080p"
	}
	if in.Resolution != "1080p" {
		v.AddDetail(kernel.Detail{"field": "resolution", "issue": "unsupported", "supported": []string{"1080p"}})
	}
	if in.Source.Type == "" {
		switch {
		case len(in.Source.FileIDs) > 0:
			in.Source.Type = domain.SourceImage
		case in.ScriptFromAssetID != nil:
			in.Source.Type = domain.SourceScript
		default:
			in.Source.Type = domain.SourcePrompt
		}
	}
	switch in.Source.Type {
	case domain.SourcePrompt:
		if in.Prompt == "" && in.ScriptFromAssetID == nil {
			v.Add("prompt", "required")
		}
	case domain.SourceImage:
		if len(in.Source.FileIDs) == 0 {
			v.Add("source.file_ids", "required")
		}
	case domain.SourceScript:
		if in.ScriptFromAssetID == nil {
			v.Add("script_from_asset_id", "required")
		}
	default:
		v.Add("source.type", "invalid")
	}
	if len(in.Source.FileIDs) > domain.MaxScenes {
		v.AddDetail(kernel.Detail{"field": "source.file_ids", "issue": "too_many", "max": domain.MaxScenes})
	}
	if utf8.RuneCountInString(in.Prompt) > domain.MaxPromptLen {
		v.AddDetail(kernel.Detail{"field": "prompt", "issue": "too_long", "max": domain.MaxPromptLen})
	}
	if utf8.RuneCountInString(in.Title) > 200 {
		v.AddDetail(kernel.Detail{"field": "title", "issue": "too_long", "max": 200})
	}
	if err := normalizeAudio(&in.Voiceover, &in.Subtitles, &v); err != nil {
		return domain.Settings{}, err
	}
	if err := v.Err(); err != nil {
		return domain.Settings{}, err
	}
	if in.Source.FileIDs == nil {
		in.Source.FileIDs = []kernel.ID{}
	}
	return domain.Settings{
		Preset: preset.Code, AspectRatio: preset.AspectRatio, Width: preset.Width, Height: preset.Height, FPS: domain.FPS,
		TargetDurationSeconds: in.TargetDurationSeconds, Resolution: in.Resolution, Source: in.Source,
		Voiceover: in.Voiceover, Subtitles: in.Subtitles, Music: in.Music,
	}, nil
}

// normalizeAudio fills defaults and validates voice/subtitle settings.
func normalizeAudio(vo *domain.VoiceSettings, st *domain.SubtitleSettings, v *kernel.ValidationErrors) error {
	if vo != nil {
		if vo.Language == "" {
			vo.Language = "vi"
		}
		if vo.Mode == "" {
			vo.Mode = "per_scene"
		}
		if vo.Mode != "per_scene" && vo.Mode != "whole" {
			v.Add("voiceover.mode", "invalid")
		}
	}
	if st != nil {
		if st.Language == "" {
			st.Language = "vi"
		}
		if st.Style == "" {
			st.Style = "burn_in"
		}
		if st.Style != "burn_in" && st.Style != "soft" {
			v.Add("subtitles.style", "invalid")
		}
	}
	return nil
}

// resolveBrand derives the brand from the campaign (which also checks the
// actor may generate in it) or validates an explicit brand_id.
func (s *Service) resolveBrand(ctx context.Context, actor kernel.Actor, campaignID, brandID *kernel.ID) (kernel.ID, error) {
	if campaignID != nil {
		c, err := s.Campaigns.GetCampaignForGeneration(ctx, actor, *campaignID)
		if errors.Is(err, kernel.ErrNotFound) {
			return kernel.NilID, kernel.Validation(kernel.FieldIssue("campaign_id", "not_found"))
		}
		if err != nil {
			return kernel.NilID, err
		}
		if brandID != nil && *brandID != c.BrandID {
			return kernel.NilID, kernel.Validation(kernel.FieldIssue("brand_id", "mismatch"))
		}
		return c.BrandID, nil
	}
	if brandID == nil {
		return kernel.NilID, kernel.Validation(kernel.FieldIssue("brand_id", "required"))
	}
	if _, err := s.Brands.GetBrandContext(ctx, branduc.BrandContextRequest{BrandID: *brandID}); err != nil {
		if errors.Is(err, kernel.ErrNotFound) {
			return kernel.NilID, kernel.Validation(kernel.FieldIssue("brand_id", "not_found"))
		}
		return kernel.NilID, err
	}
	return *brandID, nil
}

// gate checks the per-job limit before any job is created. A Lead/Admin
// may explicitly override (returned as the Reserve override actor); others
// get 429 JOB_BUDGET_EXCEEDED with the numbers to reduce the storyboard.
func (s *Service) gate(ctx context.Context, actor kernel.Actor, campaignID *kernel.ID, total kernel.Money, override bool) (*kernel.Actor, error) {
	if override && !actor.IsLeadOrAdmin() {
		return nil, kernel.ErrForbidden.WithMessage("Chỉ Marketing Lead hoặc Admin được duyệt vượt hạn mức")
	}
	lim, err := s.Budget.Limits(ctx, campaignID, budgetuc.CategoryVideo, total)
	if err != nil {
		return nil, err
	}
	if lim.RequiresOverride && !override {
		return nil, kernel.ErrJobBudgetExceeded.WithMessage("Dự toán vượt hạn mức mỗi job. Giảm số cảnh hoặc thời lượng, hoặc xin Marketing Lead duyệt.").
			WithDetails(kernel.Detail{"estimate": total.Float64(), "per_job_limit": lim.PerJobLimit.Float64(), "currency": total.Currency})
	}
	if !override {
		return nil, nil
	}
	a := actor
	return &a, nil
}

func toStepSpecs(nodes []domain.StepNode) ([]jobuc.StepSpec, error) {
	if err := domain.ValidateDAG(nodes); err != nil {
		return nil, err
	}
	out := make([]jobuc.StepSpec, 0, len(nodes))
	for _, n := range nodes {
		in, err := json.Marshal(n.Input)
		if err != nil {
			return nil, err
		}
		spec := jobuc.StepSpec{Key: n.Key, Type: n.Type, DependsOn: n.DependsOn, Input: in, MaxAttempts: n.MaxAttempts}
		if si, ok := n.Input.(domain.SceneStepInput); ok {
			id := si.SceneID
			spec.SceneID = &id
		}
		out = append(out, spec)
	}
	return out, nil
}
