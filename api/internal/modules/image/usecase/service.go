package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"

	"github.com/shopspring/decimal"

	assetuc "github.com/be/acs/api/internal/modules/asset/usecase"
	branduc "github.com/be/acs/api/internal/modules/brand/usecase"
	budgetuc "github.com/be/acs/api/internal/modules/budget/usecase"
	campaignuc "github.com/be/acs/api/internal/modules/campaign/usecase"
	fileuc "github.com/be/acs/api/internal/modules/file/usecase"
	"github.com/be/acs/api/internal/modules/image/domain"
	jobuc "github.com/be/acs/api/internal/modules/job/usecase"
	provideruc "github.com/be/acs/api/internal/modules/provider/usecase"
	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Service implements API and the image_generate step.
type Service struct {
	Tx        ports.TxManager
	Clock     ports.Clock
	Repo      BatchRepository
	Jobs      jobuc.API
	Budget    budgetuc.API
	Assets    assetuc.API
	Campaigns campaignuc.API
	Brands    branduc.API
	Files     fileuc.API
	Providers provideruc.API
	AI        ports.AIProviders
	Logger    *slog.Logger
}

var _ API = (*Service)(nil)

const fileKindImage = "image"

func (s *Service) log() *slog.Logger {
	if s.Logger != nil {
		return s.Logger
	}
	return slog.Default()
}

func capsOf(m provideruc.ModelInfo) domain.Caps {
	c := m.Capabilities
	return domain.Caps{AspectRatios: c.AspectRatios, Qualities: c.Qualities, MaxVariants: c.MaxVariants,
		SupportsRefImage: c.SupportsRefImage, SupportsEdit: c.SupportsEdit}
}

// price estimates count images for model (ACS-054 lives in the budget
// module: this only computes the units). Models that price only the 1024px
// unit fall back to it.
func (s *Service) price(ctx context.Context, m provideruc.ModelInfo, o domain.Options) (budgetuc.Estimate, error) {
	item := budgetuc.EstimateItem{Item: "images", ProviderCode: m.ProviderCode, ModelCode: m.ModelCode,
		Unit: domain.PricingUnit(o.AspectRatio, o.Quality), Units: decimal.NewFromInt(int64(o.Count))}
	req := budgetuc.EstimateRequest{Category: budgetuc.CategoryImage, Items: []budgetuc.EstimateItem{item}}
	est, err := s.Budget.Estimate(ctx, req)
	if errors.Is(err, kernel.ErrPricingMissing) && item.Unit != domain.FallbackPricingUnit(o.Quality) {
		req.Items[0].Unit = domain.FallbackPricingUnit(o.Quality)
		est, err = s.Budget.Estimate(ctx, req)
	}
	return est, err
}

// fileMeta is the metadata Image Studio writes on generated files and reads
// back for access checks.
type fileMeta struct {
	CampaignID *kernel.ID `json:"campaign_id,omitempty"`
	JobID      *kernel.ID `json:"job_id,omitempty"`
	BatchID    *kernel.ID `json:"batch_id,omitempty"`
	Operation  string     `json:"operation,omitempty"`
	Purpose    string     `json:"purpose,omitempty"`
	// ThumbnailOf is set on thumbnails.
	ThumbnailOf *kernel.ID `json:"thumbnail_of,omitempty"`
}

func parseMeta(raw json.RawMessage) fileMeta {
	var m fileMeta
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &m)
	}
	return m
}

// refAccess decides whether actor may use files owned by others as
// references for an asset of brandID. Rules (FR-IMG-02): Lead/Admin all; own
// uploads/outputs; brand assets of the target brand; generated files of a
// campaign the actor belongs to. Anything else is NOT_FOUND (IDOR).
type refAccess struct {
	s          *Service
	actor      kernel.Actor
	brandID    kernel.ID
	brandFiles map[kernel.ID]bool
	members    map[kernel.ID]bool
}

func (s *Service) newRefAccess(actor kernel.Actor, brandID kernel.ID) *refAccess {
	return &refAccess{s: s, actor: actor, brandID: brandID, members: map[kernel.ID]bool{}}
}

func (r *refAccess) allowed(ctx context.Context, f fileuc.File) (bool, error) {
	if r.actor.IsLeadOrAdmin() || (f.CreatedBy != nil && *f.CreatedBy == r.actor.UserID) {
		return true, nil
	}
	if r.brandFiles == nil {
		r.brandFiles = map[kernel.ID]bool{}
		assets, err := r.s.Brands.ListBrandAssets(ctx, r.brandID)
		if err != nil && !errors.Is(err, kernel.ErrNotFound) {
			return false, err
		}
		for _, a := range assets {
			r.brandFiles[a.FileID] = true
		}
	}
	if r.brandFiles[f.ID] {
		return true, nil
	}
	m := parseMeta(f.Metadata)
	if m.CampaignID == nil {
		return false, nil
	}
	ok, cached := r.members[*m.CampaignID]
	if !cached {
		var err error
		ok, err = r.s.Campaigns.IsMember(ctx, *m.CampaignID, r.actor.UserID)
		if err != nil && !errors.Is(err, kernel.ErrNotFound) {
			return false, err
		}
		r.members[*m.CampaignID] = ok
	}
	return ok, nil
}

// requireImage loads a ready raster image the actor may use; field names the
// request field for error details.
func (r *refAccess) requireImage(ctx context.Context, field string, id kernel.ID) (fileuc.File, error) {
	f, err := r.s.Files.RequireReady(ctx, id, fileKindImage)
	if err != nil {
		if de, ok := kernel.AsDomainError(err); ok && de.Code == kernel.CodeUnsupportedMediaType {
			return fileuc.File{}, de.WithDetails(kernel.Detail{"field": field, "supported": domain.AllowedReferenceMIME})
		}
		return fileuc.File{}, err
	}
	ok, err := r.allowed(ctx, f)
	if err != nil {
		return fileuc.File{}, err
	}
	if !ok {
		return fileuc.File{}, kernel.ErrNotFound.WithDetails(kernel.Detail{"field": field})
	}
	if err := domain.CheckReferenceMIME(field, f.MimeType); err != nil {
		return fileuc.File{}, err
	}
	return f, nil
}

// submission is everything needed to create an image job.
type submission struct {
	op             domain.Operation
	jobType        jobuc.JobType
	model          provideruc.ModelInfo
	opts           domain.Options
	campaignID     *kernel.ID
	brandID        kernel.ID
	brandVersionID *kernel.ID
	assetID        *kernel.ID
	refs           []kernel.ID
	sourceFileID   *kernel.ID
	maskFileID     *kernel.ID
	override       bool
	idemKey        string
	requestHash    string
}

// submit estimates, then creates the job + reservation + batch atomically
// (SPEC-06 §2.4) and returns the 202 body.
func (s *Service) submit(ctx context.Context, actor kernel.Actor, sub submission) (JobAccepted, error) {
	if sub.override && !actor.IsLeadOrAdmin() {
		return JobAccepted{}, kernel.ErrForbidden.WithMessage("Chỉ Marketing Lead hoặc Admin được duyệt vượt hạn mức")
	}
	est, err := s.price(ctx, sub.model, sub.opts)
	if err != nil {
		return JobAccepted{}, err
	}
	batch := &domain.Batch{
		ID: kernel.NewID(), Operation: sub.op, CampaignID: sub.campaignID, BrandID: sub.brandID, AssetID: sub.assetID,
		SourceFileID: sub.sourceFileID, Options: sub.opts, ModelCode: sub.model.ModelCode, CreatedBy: actor.UserID,
		CreatedAt: s.Clock.Now(),
	}
	refs := sub.refs
	if refs == nil {
		refs = []kernel.ID{}
	}
	in := StepInput{
		BatchID: batch.ID, Operation: sub.op, Prompt: sub.opts.Prompt, Preset: sub.opts.Preset,
		AspectRatio: sub.opts.AspectRatio, Quality: sub.opts.Quality, Count: sub.opts.Count,
		ProviderCode: sub.model.ProviderCode, ModelCode: sub.model.ModelCode, CampaignID: sub.campaignID,
		BrandID: sub.brandID, AssetID: sub.assetID, ReferenceFileIDs: refs, SourceFileID: sub.sourceFileID,
		MaskFileID: sub.maskFileID,
	}
	var override *kernel.Actor
	var overrideBy *kernel.ID
	if sub.override {
		a := actor
		override, overrideBy = &a, &a.UserID
	}
	spec := jobuc.JobSpec{
		Type: sub.jobType, CreatedBy: actor.UserID, CampaignID: sub.campaignID, AssetID: sub.assetID,
		BrandVersionID: sub.brandVersionID, ProviderCode: sub.model.ProviderCode, ModelCode: sub.model.ModelCode,
		Request: requestJSON(map[string]any{
			"operation": sub.op, "prompt": sub.opts.Prompt, "preset": sub.opts.Preset, "aspect_ratio": sub.opts.AspectRatio,
			"quality": sub.opts.Quality, "count": sub.opts.Count, "reference_file_ids": refs, "source_file_id": sub.sourceFileID,
			"mask_file_id": sub.maskFileID, "brand_id": sub.brandID,
		}),
		IdempotencyKey: sub.idemKey, RequestHash: sub.requestHash, CostEstimate: est.Total,
		PricingVersion: est.PricingVersion, BudgetOverrideBy: overrideBy,
		Steps: []jobuc.StepSpec{{Key: StepTypeImage, Type: StepTypeImage, Input: requestJSON(in)}},
	}
	var out JobAccepted
	err = s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		res, err := s.Jobs.Create(ctx, spec)
		if err != nil {
			return err
		}
		out = JobAccepted{JobID: res.JobID, Status: string(res.Status), EstimatedCost: res.CostEstimate}
		if res.Existing {
			return nil // idempotent replay: never reserve twice
		}
		if _, err := s.Budget.Reserve(ctx, budgetuc.ReserveRequest{
			JobID: res.JobID, CampaignID: sub.campaignID, Amount: est.Total, Category: budgetuc.CategoryImage, Override: override,
		}); err != nil {
			return err
		}
		batch.JobID = res.JobID
		return s.Repo.Insert(ctx, batch)
	})
	if err != nil {
		return JobAccepted{}, err
	}
	return out, nil
}

// imageRoles are the version-file roles that hold full images.
var imageRoles = []assetuc.FileRole{assetuc.RolePrimary, assetuc.RoleVariant}

func isImageRole(r assetuc.FileRole) bool { return slices.Contains(imageRoles, r) }
