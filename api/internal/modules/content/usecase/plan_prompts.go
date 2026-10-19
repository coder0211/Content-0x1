package usecase

import (
	"context"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/be/acs/api/internal/modules/content/domain"
	"github.com/be/acs/api/internal/modules/content/usecase/prompt"
	jobuc "github.com/be/acs/api/internal/modules/job/usecase"
	"github.com/be/acs/api/internal/shared/kernel"
)

// ImagePresets are the Image Studio presets (FR-IMG-01).
var ImagePresets = []string{"social_post", "poster", "product_photo", "thumbnail"}

// MaxImageDescription bounds the free-text description of an image request.
const MaxImageDescription = 2000

// StartContentPlan starts a content_plan job for a campaign (ACS-037). The
// worker stores the result as a proposal via campaign.API; approving it is
// the campaign module's job (PUT /campaigns/{id}/plan).
func (s *Service) StartContentPlan(ctx context.Context, actor kernel.Actor, in PlanInput) (JobAccepted, error) {
	if err := actor.Require(kernel.PermGenerateRun); err != nil {
		return JobAccepted{}, err
	}
	if in.Language == "" {
		in.Language = "vi"
	}
	if !slices.Contains(domain.Languages, in.Language) {
		return JobAccepted{}, validationFor("language", "invalid")
	}
	cid := in.CampaignID
	gc, err := s.loadContext(ctx, actor, &cid, kernel.NilID, nil, BriefJSON{})
	if err != nil {
		return JobAccepted{}, err
	}
	model, err := s.resolveTextModel(ctx, in.ModelCode, in.Language)
	if err != nil {
		return JobAccepted{}, err
	}
	tpl, err := s.template(ctx, prompt.CodeContentPlan)
	if err != nil {
		return JobAccepted{}, err
	}
	c := gc.Campaign
	formats := make([]map[string]any, 0, len(c.Brief.Formats))
	formatSlugs := make([]string, 0, len(c.Brief.Formats))
	for _, f := range c.Brief.Formats {
		formats = append(formats, map[string]any{"type": f.Type, "count": f.Count})
		formatSlugs = append(formatSlugs, f.Type)
	}
	channels := c.Brief.Channels
	if channels == nil {
		channels = []string{}
	}
	end := c.Brief.Deadline
	p, err := prompt.Build(prompt.Input{
		Template: tpl, Brand: brandForPrompt(gc.Brand),
		Task: map[string]string{
			"objective": c.Goal, "language": in.Language,
			"start_date": s.Clock.Now().UTC().Format("2006-01-02"), "end_date": end,
		},
		UserData: map[string]any{
			"brief": gc.briefData(map[string]any{"channels": channels, "formats": formats, "deadline": end}),
			"facts": gc.Facts.Products,
		},
	})
	if err != nil {
		return JobAccepted{}, err
	}
	return s.start(ctx, jobPlan{
		Type: jobuc.TypeContentPlan, StepType: StepPlan, Actor: actor, CampaignID: &cid,
		Model: model, Template: tpl, Prompt: p, Idem: in.IdempotencyKey, Hash: in.RequestHash, Override: in.BudgetOverride,
		Input: stepInput{
			MaxTokens: 2500, Language: in.Language, ForbiddenWords: gc.Brand.ForbiddenWords,
			BrandID: gc.Brand.BrandID, BrandVersionID: gc.Brand.BrandVersionID, BrandVersionNo: gc.Brand.VersionNo,
			ProductIDs: gc.ProductIDs, Brief: gc.Brief,
			PlanChannels: channels, PlanFormats: formatSlugs,
		},
		Request: map[string]any{"campaign_id": cid, "language": in.Language},
	})
}

// SuggestImagePrompts starts an image_prompt_suggest job (ACS-052): three
// English image prompts built from the brief or a content asset, using the
// brand colors and tone. The worker guarantees every prompt names the brand
// palette.
func (s *Service) SuggestImagePrompts(ctx context.Context, actor kernel.Actor, in ImagePromptInput) (JobAccepted, error) {
	if err := actor.Require(kernel.PermGenerateRun); err != nil {
		return JobAccepted{}, err
	}
	var v kernel.ValidationErrors
	if in.Preset == "" {
		in.Preset = "social_post"
	}
	if !slices.Contains(ImagePresets, in.Preset) {
		v.Add("preset", "invalid")
	}
	if utf8.RuneCountInString(in.Description) > MaxImageDescription {
		v.Add("description", "too_long")
	}
	if in.CampaignID == nil && in.SourceAssetID == nil && in.BrandID == nil {
		v.Add("campaign_id", "required")
	}
	if err := v.Err(); err != nil {
		return JobAccepted{}, err
	}
	brandID := kernel.NilID
	if in.BrandID != nil {
		brandID = *in.BrandID
	}
	campaignID := in.CampaignID
	var source *ContentJSON
	var products []kernel.ID
	brief := BriefJSON{}
	if in.SourceAssetID != nil {
		a, c, err := s.sourceContent(ctx, *in.SourceAssetID)
		if err != nil {
			return JobAccepted{}, err
		}
		if brandID != kernel.NilID && brandID != a.BrandID {
			return JobAccepted{}, validationFor("brand_id", "asset_mismatch")
		}
		brandID = a.BrandID
		if campaignID == nil {
			campaignID = a.CampaignID
		}
		if c.Provenance != nil {
			products = c.Provenance.ProductIDs
			if c.Provenance.Brief != nil {
				brief = *c.Provenance.Brief
			}
		}
		source = &c
	}
	gc, err := s.loadContext(ctx, actor, campaignID, brandID, products, brief)
	if err != nil {
		return JobAccepted{}, err
	}
	model, err := s.resolveTextModel(ctx, in.ModelCode, "")
	if err != nil {
		return JobAccepted{}, err
	}
	tpl, err := s.template(ctx, prompt.CodeImagePromptSuggest)
	if err != nil {
		return JobAccepted{}, err
	}
	data := map[string]any{"brief": gc.briefData(nil), "facts": gc.Facts.Products, "description": strings.TrimSpace(in.Description)}
	if source != nil {
		data["source"] = variantFromDomain(source.variant())
	}
	p, err := prompt.Build(prompt.Input{
		Template: tpl, Brand: brandForPrompt(gc.Brand), UserData: data,
		Task: map[string]string{"preset": in.Preset},
	})
	if err != nil {
		return JobAccepted{}, err
	}
	colors := brandForPrompt(gc.Brand).Colors
	return s.start(ctx, jobPlan{
		Type: jobuc.TypeImagePromptSuggest, StepType: StepImagePrompts, Actor: actor, CampaignID: campaignID, AssetID: in.SourceAssetID,
		Model: model, Template: tpl, Prompt: p, Idem: in.IdempotencyKey, Hash: in.RequestHash, Override: in.BudgetOverride,
		Input: stepInput{
			MaxTokens: 1200, BrandID: gc.Brand.BrandID, BrandVersionID: gc.Brand.BrandVersionID, BrandVersionNo: gc.Brand.VersionNo,
			Colors: colors, Tone: gc.Brand.ToneOfVoice, SourceAssetID: in.SourceAssetID,
		},
		Request: map[string]any{"campaign_id": campaignID, "source_asset_id": in.SourceAssetID, "brand_id": gc.Brand.BrandID, "preset": in.Preset},
	})
}
