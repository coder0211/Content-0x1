package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	assetuc "github.com/be/acs/api/internal/modules/asset/usecase"
	branduc "github.com/be/acs/api/internal/modules/brand/usecase"
	"github.com/be/acs/api/internal/modules/image/domain"
	jobuc "github.com/be/acs/api/internal/modules/job/usecase"
	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Estimate prices a request without creating a job (CostChip + over-budget
// banner in Image Studio).
func (s *Service) Estimate(ctx context.Context, actor kernel.Actor, in EstimateInput) (EstimateView, error) {
	if err := actor.Require(kernel.PermGenerateRun); err != nil {
		return EstimateView{}, err
	}
	op := in.Operation
	if op == "" {
		op = domain.OpGenerate
	}
	if !slices.Contains([]domain.Operation{domain.OpGenerate, domain.OpEdit, domain.OpVariation}, op) {
		return EstimateView{}, kernel.Validation(kernel.FieldIssue("operation", "invalid"))
	}
	opts := domain.Options{Prompt: "-", Preset: domain.PresetSocialPost, AspectRatio: in.AspectRatio, Quality: in.Quality, Count: in.Count}.Normalize()
	if op == domain.OpEdit {
		opts.Count = 1
	}
	if err := opts.Validate(false); err != nil {
		return EstimateView{}, err
	}
	if in.CampaignID != nil {
		if _, err := s.Campaigns.GetCampaignForGeneration(ctx, actor, *in.CampaignID); err != nil {
			return EstimateView{}, err
		}
	}
	model, err := s.Providers.ResolveModel(ctx, ports.AIKindImage, in.ModelCode)
	if err != nil {
		return EstimateView{}, err
	}
	if err := domain.CheckCapabilities(capsOf(model), opts, op, 0); err != nil {
		return EstimateView{}, err
	}
	est, err := s.price(ctx, model, opts)
	if err != nil {
		return EstimateView{}, err
	}
	lim, err := s.Budget.Limits(ctx, in.CampaignID, "image", est.Total)
	if err != nil {
		return EstimateView{}, err
	}
	out := EstimateView{Total: est.Total, PricingVersion: est.PricingVersion, PerJobLimit: lim.PerJobLimit,
		CampaignRemaining: lim.CampaignRemaining, GlobalRemaining: lim.GlobalRemaining,
		RequiresOverride: lim.RequiresOverride, Blocked: lim.Blocked, Lines: []EstimateLine{}}
	for _, l := range est.Lines {
		u, _ := l.Units.Float64()
		out.Lines = append(out.Lines, EstimateLine{Item: l.Item, ProviderCode: l.ProviderCode, ModelCode: l.ModelCode, Unit: l.Unit, Units: u, Amount: l.Amount})
	}
	return out, nil
}

// Generate is POST /images/generations (FR-IMG-01, 02).
func (s *Service) Generate(ctx context.Context, actor kernel.Actor, in GenerateInput) (JobAccepted, error) {
	if err := actor.Require(kernel.PermGenerateRun); err != nil {
		return JobAccepted{}, err
	}
	opts := domain.Options{Prompt: in.Prompt, Preset: in.Preset, AspectRatio: in.AspectRatio, Quality: in.Quality, Count: in.Count}.Normalize()
	var v kernel.ValidationErrors
	if err := opts.Validate(true); err != nil {
		de, _ := kernel.AsDomainError(err)
		for _, d := range de.Details {
			v.AddDetail(d)
		}
	}
	if in.CampaignID == nil && in.BrandID == nil {
		v.Add("campaign_id", "required")
	}
	if len(in.ReferenceFileIDs) > domain.MaxReferences {
		v.Add("reference_file_ids", "too_many")
	}
	if err := v.Err(); err != nil {
		return JobAccepted{}, err
	}
	refs := uniqueIDs(in.ReferenceFileIDs)

	var brandID kernel.ID
	if in.CampaignID != nil {
		c, err := s.Campaigns.GetCampaignForGeneration(ctx, actor, *in.CampaignID)
		if err != nil {
			return JobAccepted{}, err
		}
		brandID = c.BrandID
	} else {
		brandID = *in.BrandID
	}
	bc, err := s.Brands.GetBrandContext(ctx, branduc.BrandContextRequest{BrandID: brandID})
	if err != nil {
		return JobAccepted{}, err
	}
	model, err := s.Providers.ResolveModel(ctx, ports.AIKindImage, in.ModelCode)
	if err != nil {
		return JobAccepted{}, err
	}
	if err := domain.CheckCapabilities(capsOf(model), opts, domain.OpGenerate, len(refs)); err != nil {
		return JobAccepted{}, err
	}
	acc := s.newRefAccess(actor, brandID)
	for _, id := range refs {
		if _, err := acc.requireImage(ctx, "reference_file_ids", id); err != nil {
			return JobAccepted{}, err
		}
	}
	bv := bc.BrandVersionID
	return s.submit(ctx, actor, submission{
		op: domain.OpGenerate, jobType: jobuc.TypeImageGeneration, model: model, opts: opts,
		campaignID: in.CampaignID, brandID: brandID, brandVersionID: &bv, refs: refs,
		override: in.BudgetOverride, idemKey: in.IdempotencyKey, requestHash: in.RequestHash,
	})
}

// sourceContext loads an image asset the actor may edit and checks that
// fileID is one of its current images.
func (s *Service) sourceContext(ctx context.Context, actor kernel.Actor, assetID, fileID kernel.ID) (assetuc.AssetView, domain.Content, error) {
	if err := actor.Require(kernel.PermGenerateRun); err != nil {
		return assetuc.AssetView{}, domain.Content{}, err
	}
	if err := s.Assets.CanEdit(ctx, actor, assetID); err != nil {
		if errors.Is(err, kernel.ErrForbidden) {
			err = kernel.ErrNotFound // IDOR: never confirm another user's asset
		}
		return assetuc.AssetView{}, domain.Content{}, err
	}
	a, err := s.Assets.Get(ctx, assetID)
	if err != nil {
		return assetuc.AssetView{}, domain.Content{}, err
	}
	if a.Type != assetuc.TypeImage || a.CurrentVersion == nil {
		return assetuc.AssetView{}, domain.Content{}, kernel.ErrNotFound
	}
	var c domain.Content
	if len(a.CurrentVersion.ContentJSON) > 0 {
		_ = json.Unmarshal(a.CurrentVersion.ContentJSON, &c)
	}
	found := false
	for _, f := range a.CurrentVersion.Files {
		if f.FileID == fileID && isImageRole(f.Role) {
			found = true
		}
	}
	if !found {
		return assetuc.AssetView{}, domain.Content{}, kernel.Validation(kernel.FieldIssue("file_id", "not_in_asset"))
	}
	if a.CampaignID != nil {
		if _, err := s.Campaigns.GetCampaignForGeneration(ctx, actor, *a.CampaignID); err != nil {
			return assetuc.AssetView{}, domain.Content{}, err
		}
	}
	if _, err := s.Files.RequireReady(ctx, fileID, fileKindImage); err != nil {
		return assetuc.AssetView{}, domain.Content{}, err
	}
	return a, c, nil
}

func presetOf(c domain.Content, a assetuc.AssetView) domain.Preset {
	if c.Preset != "" {
		return c.Preset
	}
	return domain.Preset(a.Subtype)
}

// Edit is POST /images/{assetId}/edits: edit-by-prompt → new version
// (FR-IMG-04).
func (s *Service) Edit(ctx context.Context, actor kernel.Actor, assetID kernel.ID, in EditInput) (JobAccepted, error) {
	a, c, err := s.sourceContext(ctx, actor, assetID, in.FileID)
	if err != nil {
		return JobAccepted{}, err
	}
	quality := in.Quality
	if quality == "" {
		quality = c.Quality
	}
	opts := domain.Options{Prompt: in.Prompt, Preset: presetOf(c, a), AspectRatio: c.AspectRatio, Quality: quality, Count: 1}.Normalize()
	if err := opts.Validate(true); err != nil {
		return JobAccepted{}, err
	}
	model, err := s.Providers.ResolveModel(ctx, ports.AIKindImage, c.ModelCode)
	if err != nil {
		return JobAccepted{}, err
	}
	if err := domain.CheckCapabilities(capsOf(model), opts, domain.OpEdit, 0); err != nil {
		return JobAccepted{}, err
	}
	acc := s.newRefAccess(actor, a.BrandID)
	src, err := acc.requireImage(ctx, "file_id", in.FileID)
	if err != nil {
		return JobAccepted{}, err
	}
	if in.MaskFileID != nil {
		mask, err := acc.requireImage(ctx, "mask_file_id", *in.MaskFileID)
		if err != nil {
			return JobAccepted{}, err
		}
		if mask.MimeType != "image/png" {
			return JobAccepted{}, kernel.ErrUnsupportedMedia.WithDetails(kernel.Detail{"field": "mask_file_id", "supported": []string{"image/png"}})
		}
	}
	srcID := src.ID
	return s.submit(ctx, actor, submission{
		op: domain.OpEdit, jobType: jobuc.TypeImageEdit, model: model, opts: opts, campaignID: a.CampaignID,
		brandID: a.BrandID, assetID: &a.ID, sourceFileID: &srcID, maskFileID: in.MaskFileID,
		override: in.BudgetOverride, idemKey: in.IdempotencyKey, requestHash: in.RequestHash,
	})
}

// Vary is POST /images/{assetId}/variations: N variants of one image, stored
// with parent_file_id (FR-IMG-04).
func (s *Service) Vary(ctx context.Context, actor kernel.Actor, assetID kernel.ID, in VariationInput) (JobAccepted, error) {
	a, c, err := s.sourceContext(ctx, actor, assetID, in.FileID)
	if err != nil {
		return JobAccepted{}, err
	}
	prompt := in.Prompt
	if prompt == "" {
		prompt = c.Prompt
	}
	opts := domain.Options{Prompt: prompt, Preset: presetOf(c, a), AspectRatio: c.AspectRatio, Quality: c.Quality, Count: in.Count}.Normalize()
	if err := opts.Validate(false); err != nil {
		return JobAccepted{}, err
	}
	model, err := s.Providers.ResolveModel(ctx, ports.AIKindImage, c.ModelCode)
	if err != nil {
		return JobAccepted{}, err
	}
	if err := domain.CheckCapabilities(capsOf(model), opts, domain.OpVariation, 1); err != nil {
		return JobAccepted{}, err
	}
	src, err := s.newRefAccess(actor, a.BrandID).requireImage(ctx, "file_id", in.FileID)
	if err != nil {
		return JobAccepted{}, err
	}
	srcID := src.ID
	return s.submit(ctx, actor, submission{
		op: domain.OpVariation, jobType: jobuc.TypeImageGeneration, model: model, opts: opts, campaignID: a.CampaignID,
		brandID: a.BrandID, assetID: &a.ID, sourceFileID: &srcID,
		override: in.BudgetOverride, idemKey: in.IdempotencyKey, requestHash: in.RequestHash,
	})
}

func uniqueIDs(ids []kernel.ID) []kernel.ID {
	out := make([]kernel.ID, 0, len(ids))
	for _, id := range ids {
		if !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}
