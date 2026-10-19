// Package usecase implements Image Studio (ACS-051, FR-IMG-01..06):
// generation, edit-by-prompt and variations as AI jobs, their step handler,
// estimates and the gallery read model. api.go is the module's public
// contract (no other module depends on it yet; it is not in registry.APIs).
package usecase

import (
	"context"
	"encoding/json"
	"time"

	"github.com/be/acs/api/internal/modules/image/domain"
	"github.com/be/acs/api/internal/shared/kernel"
)

// StepTypeImage is the single step type of every image job.
const StepTypeImage = "image_generate"

// GenerateInput is POST /images/generations.
type GenerateInput struct {
	CampaignID *kernel.ID
	// BrandID is required when CampaignID is nil (independent task).
	BrandID          *kernel.ID
	Prompt           string
	Preset           domain.Preset
	ReferenceFileIDs []kernel.ID
	AspectRatio      string
	Quality          string
	Count            int
	ModelCode        string
	// BudgetOverride: a Lead/Admin accepts exceeding the per-job limit.
	BudgetOverride bool
	IdempotencyKey string
	RequestHash    string
}

// EditInput is POST /images/{assetId}/edits.
type EditInput struct {
	FileID         kernel.ID
	Prompt         string
	MaskFileID     *kernel.ID
	Quality        string
	BudgetOverride bool
	IdempotencyKey string
	RequestHash    string
}

// VariationInput is POST /images/{assetId}/variations.
type VariationInput struct {
	FileID         kernel.ID
	Count          int
	Prompt         string // optional; defaults to the version prompt
	BudgetOverride bool
	IdempotencyKey string
	RequestHash    string
}

// EstimateInput is POST /images/estimate (no job is created).
type EstimateInput struct {
	CampaignID  *kernel.ID
	Operation   domain.Operation
	AspectRatio string
	Quality     string
	Count       int
	ModelCode   string
}

// EstimateLine is one priced line.
type EstimateLine struct {
	Item         string
	ProviderCode string
	ModelCode    string
	Unit         string
	Units        float64
	Amount       kernel.Money
}

// EstimateView is the estimate with the remaining limits (SPEC-05 over
// budget banner).
type EstimateView struct {
	Total             kernel.Money
	Lines             []EstimateLine
	PricingVersion    string
	PerJobLimit       kernel.Money
	CampaignRemaining *kernel.Money
	GlobalRemaining   kernel.Money
	RequiresOverride  bool
	Blocked           bool
}

// JobAccepted is the 202 body.
type JobAccepted struct {
	JobID         kernel.ID
	Status        string
	EstimatedCost kernel.Money
}

// BatchFilter selects gallery batches.
type BatchFilter struct {
	CampaignID *kernel.ID
	// AssetID lists the batches of one asset (edits/variations history).
	AssetID *kernel.ID
}

// ImageFile is one image of a batch with signed preview URLs (≤ 15 min).
type ImageFile struct {
	FileID          kernel.ID
	Role            string
	SortOrder       int
	MimeType        string
	Width           int
	Height          int
	ParentFileID    *kernel.ID
	URL             string
	ThumbnailFileID *kernel.ID
	ThumbnailURL    string
	URLExpiresAt    time.Time
}

// BatchAsset is the asset produced by a batch.
type BatchAsset struct {
	ID                      kernel.ID
	Title                   string
	Status                  string
	VersionNo               int
	CurrentVersionNo        int
	RequiresTypographyCheck bool
	Images                  []ImageFile
}

// BatchJob is the job state of a batch.
type BatchJob struct {
	ID           kernel.ID
	Status       string
	Progress     int
	ErrorCode    string
	ErrorKind    string
	ErrorMessage string
	CostEstimate *kernel.Money
	CostActual   *kernel.Money
}

// BatchView is one gallery entry ("đợt tạo").
type BatchView struct {
	ID           kernel.ID
	Operation    domain.Operation
	CampaignID   *kernel.ID
	BrandID      kernel.ID
	Options      domain.Options
	ModelCode    string
	SourceFileID *kernel.ID
	CreatedBy    kernel.ID
	CreatedAt    time.Time
	Job          BatchJob
	Asset        *BatchAsset
}

// StepInput is the input of the image_generate step (ai_job_steps.input_json).
type StepInput struct {
	BatchID          kernel.ID        `json:"batch_id"`
	Operation        domain.Operation `json:"operation"`
	Prompt           string           `json:"prompt"`
	Preset           domain.Preset    `json:"preset"`
	AspectRatio      string           `json:"aspect_ratio"`
	Quality          string           `json:"quality"`
	Count            int              `json:"count"`
	ProviderCode     string           `json:"provider_code"`
	ModelCode        string           `json:"model_code"`
	CampaignID       *kernel.ID       `json:"campaign_id"`
	BrandID          kernel.ID        `json:"brand_id"`
	AssetID          *kernel.ID       `json:"asset_id"`
	ReferenceFileIDs []kernel.ID      `json:"reference_file_ids"`
	SourceFileID     *kernel.ID       `json:"source_file_id"`
	MaskFileID       *kernel.ID       `json:"mask_file_id"`
}

// StepOutput is persisted on the step and merged into the job result.
type StepOutput struct {
	AssetID   kernel.ID   `json:"asset_id"`
	VersionNo int         `json:"version_no"`
	FileIDs   []kernel.ID `json:"file_ids"`
}

// API is Image Studio's public interface (used by the HTTP adapter).
type API interface {
	Estimate(ctx context.Context, actor kernel.Actor, in EstimateInput) (EstimateView, error)
	Generate(ctx context.Context, actor kernel.Actor, in GenerateInput) (JobAccepted, error)
	Edit(ctx context.Context, actor kernel.Actor, assetID kernel.ID, in EditInput) (JobAccepted, error)
	Vary(ctx context.Context, actor kernel.Actor, assetID kernel.ID, in VariationInput) (JobAccepted, error)
	ListBatches(ctx context.Context, actor kernel.Actor, f BatchFilter, p kernel.PageRequest) (kernel.Page[BatchView], error)
	ListReferences(ctx context.Context, actor kernel.Actor, f ReferenceFilter) ([]ReferenceCandidate, error)
}

// requestJSON is ai_jobs.request_json (validated parameters, no secrets).
func requestJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return b
}
