package domain

import (
	"time"

	"github.com/be/acs/api/internal/shared/kernel"
)

// Batch is one image request ("đợt tạo"): a generation, an edit or a
// variation run, linked to its job and — once the job produced images — to
// the asset/version it created. It is Image Studio's read model for the
// gallery (image_batches table); assets themselves live in the asset module.
type Batch struct {
	ID           kernel.ID
	JobID        kernel.ID
	Operation    Operation
	CampaignID   *kernel.ID
	BrandID      kernel.ID
	AssetID      *kernel.ID
	VersionNo    *int
	SourceFileID *kernel.ID
	Options      Options
	ModelCode    string
	CreatedBy    kernel.ID
	CreatedAt    time.Time
}

// Content is the content_json of an image asset version (migration 0006:
// image → {prompt, preset, aspect_ratio, file_ids[]}) plus the provenance
// Image Studio needs to reproduce or review it.
type Content struct {
	Prompt                  string      `json:"prompt"`
	Preset                  Preset      `json:"preset"`
	AspectRatio             string      `json:"aspect_ratio"`
	Quality                 string      `json:"quality"`
	Count                   int         `json:"count"`
	Operation               Operation   `json:"operation"`
	ProviderCode            string      `json:"provider_code"`
	ModelCode               string      `json:"model_code"`
	FileIDs                 []kernel.ID `json:"file_ids"`
	ThumbnailFileIDs        []kernel.ID `json:"thumbnail_file_ids"`
	ReferenceFileIDs        []kernel.ID `json:"reference_file_ids"`
	SourceFileID            *kernel.ID  `json:"source_file_id"`
	RequiresTypographyCheck bool        `json:"requires_typography_check"`
}
