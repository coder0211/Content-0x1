// Package usecase implements Video Studio (M06): project + storyboard job,
// storyboard editing, cost estimate + budget gate, production / scene
// regeneration / re-render jobs and the step logic of their DAGs
// (SPEC-02 §5.5). No other module depends on video, so registry.APIs has no
// Video field; the types below are the inputs/outputs of the HTTP adapter.
package usecase

import (
	"time"

	budgetuc "github.com/be/acs/api/internal/modules/budget/usecase"
	jobuc "github.com/be/acs/api/internal/modules/job/usecase"
	"github.com/be/acs/api/internal/modules/video/domain"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Idempotency carries the Idempotency-Key header data into job creation.
type Idempotency struct {
	Key         string
	RequestHash string
}

// CreateInput is POST /videos.
type CreateInput struct {
	CampaignID            *kernel.ID
	BrandID               *kernel.ID
	Title                 string
	Source                domain.Source
	Prompt                string
	ScriptFromAssetID     *kernel.ID
	Preset                string
	AspectRatio           string
	TargetDurationSeconds int
	Resolution            string
	Voiceover             domain.VoiceSettings
	Subtitles             domain.SubtitleSettings
	Music                 domain.MusicSettings
	Idem                  Idempotency
}

// Accepted is the 202 answer of every job-creating endpoint.
type Accepted struct {
	AssetID  kernel.ID
	JobID    kernel.ID
	Status   jobuc.Status
	Estimate kernel.Money
}

// ProduceInput is POST /videos/{id}/productions.
type ProduceInput struct {
	BudgetOverride bool
	Idem           Idempotency
}

// RegenerateInput is POST /videos/{id}/scenes/{sceneId}/regenerations.
type RegenerateInput struct {
	SceneID        kernel.ID
	VisualPrompt   *string
	BudgetOverride bool
	Idem           Idempotency
}

// RenderInput is POST /videos/{id}/renders: optional new audio/subtitle
// settings, then a render that reuses every clip.
type RenderInput struct {
	Voiceover      *domain.VoiceSettings
	Subtitles      *domain.SubtitleSettings
	Music          *domain.MusicSettings
	BudgetOverride bool
	Idem           Idempotency
}

// SceneView is a scene with short-lived signed URLs.
type SceneView struct {
	domain.Scene
	PreviewURL   string
	ThumbnailURL string
}

// OutputView is the latest rendered video with signed URLs.
type OutputView struct {
	domain.Output
	URL          string
	ThumbnailURL string
	SubtitleURL  string
}

// ProjectView is GET /videos/{id}.
type ProjectView struct {
	Project     domain.Project
	AssetStatus string
	VersionNo   int
	Stage       domain.Stage
	Scenes      []SceneView
	TotalMs     int
	Output      *OutputView
	Job         *jobuc.JobView
	Progress    int
	URLExpires  *time.Time
}

// EstimateView is POST /videos/{id}/estimate (SPEC-04 §2.6).
type EstimateView struct {
	SceneCount       int
	TotalClipSeconds int
	Lines            []budgetuc.EstimateLine
	Total            kernel.Money
	Limits           budgetuc.Limits
	// Violations are storyboard limit issues (OQ-07); production is refused
	// while any exists.
	Violations []kernel.Detail
}
