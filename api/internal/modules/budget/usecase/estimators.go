package usecase

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/shopspring/decimal"

	"github.com/be/acs/api/internal/modules/budget/domain"
	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Item builders shared by the studios (content/image/video) so every module
// prices the same work the same way. They only compute quantities; Estimate
// prices them.

// TextItems estimates a text generation: input tokens from the prompt length
// (runes) plus overhead, output tokens = maxOutputTokens (default 1024).
func TextItems(item, provider, model string, promptChars, maxOutputTokens int) []EstimateItem {
	if item == "" {
		item = "generation"
	}
	if maxOutputTokens <= 0 {
		maxOutputTokens = domain.DefaultMaxOutputTokens
	}
	return []EstimateItem{
		{Item: item, ProviderCode: provider, ModelCode: model, Unit: domain.UnitInputToken, Units: decimal.NewFromInt(domain.EstimateInputTokens(promptChars))},
		{Item: item, ProviderCode: provider, ModelCode: model, Unit: domain.UnitOutputToken, Units: decimal.NewFromInt(int64(maxOutputTokens))},
	}
}

// ImageItems estimates count images of size × quality on a model (ACS-054).
func ImageItems(provider, model string, count int, size, quality string) []EstimateItem {
	if count < 1 {
		count = 1
	}
	return []EstimateItem{{Item: "images", ProviderCode: provider, ModelCode: model, Unit: domain.ImageUnit(size, quality), Units: decimal.NewFromInt(int64(count))}}
}

// VideoEstimateInput describes a video production for VideoItems (ACS-064).
// Zero quantities produce no line; render is always a 0-cost line.
type VideoEstimateInput struct {
	ClipProvider, ClipModel   string
	ClipSeconds               decimal.Decimal
	Resolution                string // 720p | 1080p
	VoiceProvider, VoiceModel string
	VoiceoverChars            int
	// Script/subtitle text generation (optional).
	TextProvider, TextModel string
	ScriptPromptChars       int
	ScriptMaxOutputTokens   int
	RenderSeconds           decimal.Decimal
}

// VideoItems returns the breakdown lines: scene_clips (per second), voiceover
// (TTS characters), optional subtitles_script (tokens) and render (0).
func VideoItems(in VideoEstimateInput) []EstimateItem {
	var out []EstimateItem
	if in.ClipSeconds.IsPositive() {
		out = append(out, EstimateItem{Item: "scene_clips", ProviderCode: in.ClipProvider, ModelCode: in.ClipModel,
			Unit: domain.VideoSecondUnit(in.Resolution), Units: in.ClipSeconds})
	}
	if in.VoiceoverChars > 0 {
		out = append(out, EstimateItem{Item: "voiceover", ProviderCode: in.VoiceProvider, ModelCode: in.VoiceModel,
			Unit: domain.UnitChar, Units: decimal.NewFromInt(int64(in.VoiceoverChars))})
	}
	if in.ScriptPromptChars > 0 {
		out = append(out, TextItems("subtitles_script", in.TextProvider, in.TextModel, in.ScriptPromptChars, in.ScriptMaxOutputTokens)...)
	}
	out = append(out, EstimateItem{Item: "render", ProviderCode: "internal", ModelCode: "ffmpeg", Unit: domain.UnitRenderSecond, Units: in.RenderSeconds})
	return out
}

func marshalUsage(u ports.Usage) (json.RawMessage, error) {
	if u.Units == nil {
		u.Units = map[string]float64{}
	}
	return json.Marshal(u)
}

// QuoteRequest is the generic estimate used by the studios' cost chip
// (POST /cost-estimates). Empty provider/model use the default model of
// the kind.
type QuoteRequest struct {
	Kind         Category
	ProviderCode string
	ModelCode    string
	CampaignID   *kernel.ID
	// text
	PromptChars     int
	MaxOutputTokens int
	// image
	Count   int
	Size    string
	Quality string
	// video
	ClipSeconds    decimal.Decimal
	Resolution     string
	VoiceoverChars int
}

// Quote is an estimate with the limits it would be checked against.
type Quote struct {
	Estimate Estimate
	Limits   Limits
}

func (s *Service) resolveModel(ctx context.Context, kind, provider, model string) (string, string, error) {
	if provider != "" && model != "" {
		return provider, model, nil
	}
	p, m, err := s.Repo.DefaultModel(ctx, kind)
	if errors.Is(err, kernel.ErrNotFound) {
		return "", "", kernel.ErrPricingMissing.WithMessage("Chưa cấu hình model mặc định").WithDetails(kernel.Detail{"kind": kind})
	}
	return p, m, err
}

// Quote estimates a request and reports limits (generate.run).
func (s *Service) Quote(ctx context.Context, actor kernel.Actor, req QuoteRequest) (Quote, error) {
	if err := actor.Require(kernel.PermGenerateRun); err != nil {
		return Quote{}, err
	}
	var v kernel.ValidationErrors
	var items []EstimateItem
	switch req.Kind {
	case CategoryText:
		if req.PromptChars < 0 || req.PromptChars > 200000 {
			v.Add("prompt_chars", "out_of_range")
		}
		if req.MaxOutputTokens < 0 || req.MaxOutputTokens > 32000 {
			v.Add("max_output_tokens", "out_of_range")
		}
	case CategoryImage:
		if req.Count < 1 || req.Count > 8 {
			v.Add("count", "out_of_range")
		}
	case CategoryVideo:
		if req.ClipSeconds.IsNegative() || req.ClipSeconds.GreaterThan(decimal.NewFromInt(600)) {
			v.Add("clip_seconds", "out_of_range")
		}
		if req.VoiceoverChars < 0 || req.VoiceoverChars > 100000 {
			v.Add("voiceover_chars", "out_of_range")
		}
	default:
		v.Add("kind", "invalid")
	}
	if err := v.Err(); err != nil {
		return Quote{}, err
	}
	p, m, err := s.resolveModel(ctx, string(req.Kind), req.ProviderCode, req.ModelCode)
	if err != nil {
		return Quote{}, err
	}
	switch req.Kind {
	case CategoryText:
		items = TextItems("generation", p, m, req.PromptChars, req.MaxOutputTokens)
	case CategoryImage:
		items = ImageItems(p, m, req.Count, req.Size, req.Quality)
	case CategoryVideo:
		in := VideoEstimateInput{ClipProvider: p, ClipModel: m, ClipSeconds: req.ClipSeconds, Resolution: req.Resolution,
			VoiceoverChars: req.VoiceoverChars, RenderSeconds: decimal.Zero}
		if req.VoiceoverChars > 0 {
			if in.VoiceProvider, in.VoiceModel, err = s.resolveModel(ctx, "voice", "", ""); err != nil {
				return Quote{}, err
			}
		}
		items = VideoItems(in)
	}
	est, err := s.Estimate(ctx, EstimateRequest{Category: req.Kind, Items: items})
	if err != nil {
		return Quote{}, err
	}
	lim, err := s.Limits(ctx, req.CampaignID, req.Kind, est.Total)
	if err != nil {
		return Quote{}, err
	}
	return Quote{Estimate: est, Limits: lim}, nil
}
