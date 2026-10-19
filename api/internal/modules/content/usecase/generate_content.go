package usecase

import (
	"context"
	"slices"
	"strconv"
	"strings"

	branduc "github.com/be/acs/api/internal/modules/brand/usecase"
	campaignuc "github.com/be/acs/api/internal/modules/campaign/usecase"
	"github.com/be/acs/api/internal/modules/content/domain"
	"github.com/be/acs/api/internal/modules/content/usecase/prompt"
	jobuc "github.com/be/acs/api/internal/modules/job/usecase"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Brief is the per-request marketing brief (SPEC-04 §2.4).
type Brief struct {
	ProductID      *kernel.ID
	TargetAudience string
	KeyMessage     string
	CTA            string
	Notes          string
	// ReferenceText is pasted reference material; it is passed to the model
	// strictly as data inside <user_data>.
	ReferenceText string
}

// GenerateInput is POST /content/generations.
type GenerateInput struct {
	CampaignID     *kernel.ID
	BrandID        kernel.ID
	Platform       string
	Format         string // optional; default from platform
	ContentType    string
	Objective      string
	Framework      string
	Length         string
	Language       string
	Tone           []string
	Brief          Brief
	Variants       int
	ModelCode      string
	BudgetOverride bool
	IdempotencyKey string
	RequestHash    string
}

// normalize fills defaults and validates enums (400 with every issue).
func (in *GenerateInput) normalize() error {
	var v kernel.ValidationErrors
	if !domain.ValidPlatform(in.Platform) {
		v.Add("platform", "invalid")
	}
	if in.Format == "" && domain.ValidPlatform(in.Platform) {
		in.Format = string(domain.DefaultFormat(domain.Platform(in.Platform)))
	} else if in.Format != "" && !domain.ValidFormat(in.Format) {
		v.Add("format", "invalid")
	}
	if in.ContentType == "" {
		in.ContentType = "promotional_post"
	} else if !slices.Contains(domain.ContentTypes, in.ContentType) {
		v.Add("content_type", "invalid")
	}
	if in.Objective != "" && !slices.Contains(domain.Objectives, in.Objective) {
		v.Add("objective", "invalid")
	}
	if in.Framework != "" && !slices.Contains(domain.Frameworks, in.Framework) {
		v.Add("framework", "invalid")
	}
	if in.Length == "" {
		in.Length = "medium"
	} else if !slices.Contains(domain.Lengths, in.Length) {
		v.Add("length", "invalid")
	}
	if in.Language == "" {
		in.Language = "vi"
	} else if !slices.Contains(domain.Languages, in.Language) {
		v.Add("language", "invalid")
	}
	if in.Variants == 0 {
		in.Variants = 3
	}
	if in.Variants < domain.MinVariants || in.Variants > domain.MaxVariants {
		v.Add("variants", "out_of_range")
	}
	if in.BrandID == kernel.NilID && in.CampaignID == nil {
		v.Add("brand_id", "required")
	}
	domain.CheckTones(&v, "tone", in.Tone)
	domain.CheckText(&v, "brief.target_audience", in.Brief.TargetAudience, domain.MaxBriefFieldLen)
	domain.CheckText(&v, "brief.key_message", in.Brief.KeyMessage, domain.MaxBriefFieldLen)
	domain.CheckText(&v, "brief.cta", in.Brief.CTA, 200)
	domain.CheckText(&v, "brief.notes", in.Brief.Notes, domain.MaxBriefFieldLen)
	domain.CheckText(&v, "brief.reference_text", in.Brief.ReferenceText, domain.MaxReferenceLen)
	return v.Err()
}

// generationContext is the resolved brand/campaign/facts of a request.
type generationContext struct {
	Campaign   *campaignuc.CampaignForGeneration
	Brand      branduc.BrandContext
	ProductIDs []kernel.ID
	Brief      *BriefJSON
	Facts      factsContext
}

// loadContext resolves campaign (permission + state), brand version and
// valid product facts.
func (s *Service) loadContext(ctx context.Context, actor kernel.Actor, campaignID *kernel.ID, brandID kernel.ID, productIDs []kernel.ID, brief BriefJSON) (generationContext, error) {
	var gc generationContext
	if campaignID != nil {
		c, err := s.Campaigns.GetCampaignForGeneration(ctx, actor, *campaignID)
		if err != nil {
			return gc, err
		}
		if brandID == kernel.NilID {
			brandID = c.BrandID
		} else if brandID != c.BrandID {
			return gc, validationFor("brand_id", "campaign_mismatch")
		}
		if len(productIDs) == 0 {
			productIDs = c.Brief.ProductIDs
		}
		if brief.TargetAudience == "" {
			brief.TargetAudience = c.Brief.TargetAudience
		}
		if brief.KeyMessage == "" {
			brief.KeyMessage = c.Brief.KeyMessage
		}
		if brief.VerifiedOffer == "" {
			brief.VerifiedOffer = c.Brief.VerifiedOffer
		}
		if brief.Notes == "" {
			brief.Notes = c.Brief.Notes
		}
		gc.Campaign = &c
	}
	bc, err := s.Brands.GetBrandContext(ctx, branduc.BrandContextRequest{BrandID: brandID, ProductIDs: productIDs, At: s.Clock.Now()})
	if err != nil {
		return gc, err
	}
	if brief.CTA == "" && len(bc.DefaultCTAs) > 0 {
		brief.CTA = bc.DefaultCTAs[0]
	}
	gc.Brand, gc.ProductIDs, gc.Brief = bc, productIDs, &brief
	gc.Facts = buildFacts(bc.Products, &brief)
	return gc, nil
}

func (gc generationContext) briefData(extra map[string]any) map[string]any {
	b := map[string]any{
		"target_audience": gc.Brief.TargetAudience, "key_message": gc.Brief.KeyMessage, "cta": gc.Brief.CTA,
		"verified_offer": gc.Brief.VerifiedOffer, "notes": gc.Brief.Notes,
	}
	if gc.Campaign != nil {
		b["campaign_name"] = gc.Campaign.Name
		b["campaign_goal"] = gc.Campaign.Goal
	}
	for k, v := range extra {
		b[k] = v
	}
	return b
}

// Generate starts a content_generation job producing Variants assets.
func (s *Service) Generate(ctx context.Context, actor kernel.Actor, in GenerateInput) (JobAccepted, error) {
	if err := actor.Require(kernel.PermGenerateRun); err != nil {
		return JobAccepted{}, err
	}
	if err := in.normalize(); err != nil {
		return JobAccepted{}, err
	}
	var products []kernel.ID
	if in.Brief.ProductID != nil {
		products = []kernel.ID{*in.Brief.ProductID}
	}
	gc, err := s.loadContext(ctx, actor, in.CampaignID, in.BrandID, products,
		BriefJSON{TargetAudience: in.Brief.TargetAudience, KeyMessage: in.Brief.KeyMessage, CTA: in.Brief.CTA, Notes: in.Brief.Notes})
	if err != nil {
		return JobAccepted{}, err
	}
	if in.Objective == "" {
		in.Objective = "engagement"
		if gc.Campaign != nil && slices.Contains(domain.Objectives, gc.Campaign.Goal) {
			in.Objective = gc.Campaign.Goal
		}
	}
	model, err := s.resolveTextModel(ctx, in.ModelCode, in.Language)
	if err != nil {
		return JobAccepted{}, err
	}
	tpl, err := s.template(ctx, prompt.CodeForFormat(in.Format))
	if err != nil {
		return JobAccepted{}, err
	}
	tone := in.Tone
	if len(tone) == 0 {
		tone = gc.Brand.ToneOfVoice
	}
	docs := []string{}
	if strings.TrimSpace(in.Brief.ReferenceText) != "" {
		docs = append(docs, in.Brief.ReferenceText)
	}
	p, err := prompt.Build(prompt.Input{
		Template: tpl, Brand: brandForPrompt(gc.Brand), Variants: in.Variants,
		Task: map[string]string{
			"variants": strconv.Itoa(in.Variants), "platform": in.Platform, "content_type": in.ContentType, "objective": in.Objective,
			"framework": in.Framework, "length": in.Length, "language": in.Language,
		},
		UserData: map[string]any{"brief": gc.briefData(map[string]any{"tone": tone}), "facts": gc.Facts.Products, "documents": docs},
	})
	if err != nil {
		return JobAccepted{}, err
	}
	return s.start(ctx, jobPlan{
		Type: jobuc.TypeContentGeneration, StepType: StepGeneration, Actor: actor, CampaignID: in.CampaignID,
		Model: model, Template: tpl, Prompt: p, Idem: in.IdempotencyKey, Hash: in.RequestHash, Override: in.BudgetOverride,
		Input: stepInput{
			MaxTokens: tokensPerVariant(in.Length, in.Format)*in.Variants + 200, Format: in.Format, Platform: in.Platform,
			Language: in.Language, Variants: in.Variants, ForbiddenWords: gc.Brand.ForbiddenWords,
			FactTexts: gc.Facts.Texts, FactAmounts: gc.Facts.Amounts, BrandID: gc.Brand.BrandID,
			BrandVersionID: gc.Brand.BrandVersionID, BrandVersionNo: gc.Brand.VersionNo, ProductIDs: gc.ProductIDs, Brief: gc.Brief,
		},
		Request: map[string]any{
			"campaign_id": in.CampaignID, "brand_id": gc.Brand.BrandID, "platform": in.Platform, "format": in.Format,
			"content_type": in.ContentType, "objective": in.Objective, "framework": in.Framework, "length": in.Length,
			"language": in.Language, "tone": tone, "variants": in.Variants, "product_ids": gc.ProductIDs,
		},
	})
}
