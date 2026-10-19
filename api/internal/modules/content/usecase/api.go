// Package usecase is the public contract of the content module, used by
// the video module to turn a content asset into a video script (FR-CNT-05),
// and the content use cases (generation, rewrite, transform, plan, image
// prompt suggestions).
package usecase

import (
	"context"

	"github.com/be/acs/api/internal/shared/kernel"
)

// ScriptScene is an optional scene breakdown carried by a video_script asset.
type ScriptScene struct {
	Narration    string
	VisualHint   string
	DurationHint int // seconds, 0 = unspecified
}

// VideoScript is the script extracted from a content asset.
type VideoScript struct {
	AssetID   kernel.ID
	VersionNo int
	Title     string
	Hook      string
	Script    string // plain text narration
	CTA       string
	Language  string
	Scenes    []ScriptScene
}

// JobAccepted is returned by every use case that starts an AI job (202).
type JobAccepted struct {
	JobID         kernel.ID
	Status        string
	EstimatedCost kernel.Money
}

// PlanInput starts a content_plan job for a campaign (FR-CMP-03, ACS-037).
// The job result is {"plan": {...}}; the plan is also stored as a proposal
// via campaign.API.SaveContentPlanProposal (approval stays with campaign).
type PlanInput struct {
	CampaignID     kernel.ID
	ModelCode      string
	Language       string // vi (default) | en
	BudgetOverride bool
	IdempotencyKey string
	RequestHash    string
}

// ImagePromptInput starts an image_prompt_suggest job (FR-IMG-03, ACS-052).
// One of CampaignID, SourceAssetID or BrandID must identify the brand. The
// job result is {"prompts": [{"prompt","negative_prompt","rationale"}]}
// (exactly 3, each using the brand colors and tone).
type ImagePromptInput struct {
	CampaignID     *kernel.ID
	SourceAssetID  *kernel.ID
	BrandID        *kernel.ID
	Preset         string
	Description    string
	ModelCode      string
	BudgetOverride bool
	IdempotencyKey string
	RequestHash    string
}

// API is the content module's public interface.
type API interface {
	// GetVideoScript returns the current version of a content asset as a
	// script. NOT_FOUND when the asset is not content or not visible.
	GetVideoScript(ctx context.Context, actor kernel.Actor, assetID kernel.ID) (VideoScript, error)
	// StartContentPlan validates, estimates, reserves budget and queues a
	// content_plan job (additive, wave 2).
	StartContentPlan(ctx context.Context, actor kernel.Actor, in PlanInput) (JobAccepted, error)
	// SuggestImagePrompts queues an image_prompt_suggest job (additive, wave 2).
	SuggestImagePrompts(ctx context.Context, actor kernel.Actor, in ImagePromptInput) (JobAccepted, error)
}
