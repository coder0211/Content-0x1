package http

import (
	"time"

	jobuc "github.com/be/acs/api/internal/modules/job/usecase"
	"github.com/be/acs/api/internal/modules/video/domain"
	"github.com/be/acs/api/internal/modules/video/usecase"
	"github.com/be/acs/api/internal/shared/kernel"
)

type voiceDTO struct {
	Enabled  bool    `json:"enabled"`
	Language string  `json:"language"`
	VoiceID  *string `json:"voice_id"`
	Mode     *string `json:"mode"`
}

type subtitlesDTO struct {
	Enabled  bool   `json:"enabled"`
	Language string `json:"language"`
	Style    string `json:"style"`
}

type musicDTO struct {
	Enabled           bool       `json:"enabled"`
	TrackBrandAssetID *kernel.ID `json:"track_brand_asset_id"`
}

type sourceDTO struct {
	Type    string      `json:"type"`
	FileIDs []kernel.ID `json:"file_ids"`
}

type createRequest struct {
	CampaignID            *kernel.ID    `json:"campaign_id"`
	BrandID               *kernel.ID    `json:"brand_id"`
	Title                 *string       `json:"title"`
	Source                *sourceDTO    `json:"source"`
	Prompt                *string       `json:"prompt"`
	ScriptFromAssetID     *kernel.ID    `json:"script_from_asset_id"`
	Preset                string        `json:"preset"`
	AspectRatio           *string       `json:"aspect_ratio"`
	TargetDurationSeconds int           `json:"target_duration_seconds"`
	Resolution            *string       `json:"resolution"`
	Voiceover             *voiceDTO     `json:"voiceover"`
	Subtitles             *subtitlesDTO `json:"subtitles"`
	Music                 *musicDTO     `json:"music"`
}

func s(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func ptr(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func (v *voiceDTO) toDomain() domain.VoiceSettings {
	if v == nil {
		return domain.VoiceSettings{}
	}
	return domain.VoiceSettings{Enabled: v.Enabled, Language: v.Language, VoiceID: s(v.VoiceID), Mode: s(v.Mode)}
}

func (v *subtitlesDTO) toDomain() domain.SubtitleSettings {
	if v == nil {
		return domain.SubtitleSettings{}
	}
	return domain.SubtitleSettings{Enabled: v.Enabled, Language: v.Language, Style: v.Style}
}

func (v *musicDTO) toDomain() domain.MusicSettings {
	if v == nil {
		return domain.MusicSettings{}
	}
	return domain.MusicSettings{Enabled: v.Enabled, TrackBrandAssetID: v.TrackBrandAssetID}
}

func voiceFrom(v domain.VoiceSettings) voiceDTO {
	return voiceDTO{Enabled: v.Enabled, Language: v.Language, VoiceID: ptr(v.VoiceID), Mode: ptr(v.Mode)}
}

type moneyDTO struct {
	Amount     float64 `json:"amount"`
	Currency   string  `json:"currency"`
	IsEstimate bool    `json:"is_estimate"`
}

func money(m kernel.Money, estimate bool) moneyDTO {
	c := m.Currency
	if c == "" {
		c = kernel.CurrencyUSD
	}
	return moneyDTO{Amount: m.Float64(), Currency: c, IsEstimate: estimate}
}

type acceptedDTO struct {
	AssetID       kernel.ID `json:"asset_id"`
	JobID         kernel.ID `json:"job_id"`
	Status        string    `json:"status"`
	EstimatedCost moneyDTO  `json:"estimated_cost"`
}

func accepted(a usecase.Accepted) acceptedDTO {
	return acceptedDTO{AssetID: a.AssetID, JobID: a.JobID, Status: string(a.Status), EstimatedCost: money(a.Estimate, true)}
}

type sceneDTO struct {
	ID           kernel.ID  `json:"id"`
	SequenceNo   int        `json:"sequence_no"`
	DurationMs   int        `json:"duration_ms"`
	VisualPrompt string     `json:"visual_prompt"`
	Narration    *string    `json:"narration"`
	SubtitleText *string    `json:"subtitle_text"`
	SourceFileID *kernel.ID `json:"source_file_id"`
	ClipFileID   *kernel.ID `json:"clip_file_id"`
	Status       string     `json:"status"`
	AttemptCount int        `json:"attempt_count"`
	LastError    *string    `json:"last_error"`
	PreviewURL   *string    `json:"preview_url"`
	ThumbnailURL *string    `json:"thumbnail_url"`
}

type stepDTO struct {
	StepKey      string  `json:"step_key"`
	StepType     string  `json:"step_type"`
	Status       string  `json:"status"`
	AttemptCount int     `json:"attempt_count"`
	ErrorKind    *string `json:"error_kind"`
	ErrorMessage *string `json:"error_message"`
}

type jobDTO struct {
	ID           kernel.ID `json:"id"`
	JobType      string    `json:"job_type"`
	Status       string    `json:"status"`
	Progress     int       `json:"progress"`
	Steps        []stepDTO `json:"steps"`
	ErrorCode    *string   `json:"error_code"`
	ErrorMessage *string   `json:"error_message"`
}

type outputDTO struct {
	FileID         kernel.ID  `json:"file_id"`
	URL            string     `json:"url"`
	ThumbnailURL   *string    `json:"thumbnail_url"`
	DurationMs     int        `json:"duration_ms"`
	VersionNo      int        `json:"version_no"`
	SubtitleFileID *kernel.ID `json:"subtitle_file_id"`
	SubtitleURL    *string    `json:"subtitle_url"`
}

type settingsDTO struct {
	Preset                string       `json:"preset"`
	AspectRatio           string       `json:"aspect_ratio"`
	Width                 int          `json:"width"`
	Height                int          `json:"height"`
	FPS                   int          `json:"fps"`
	TargetDurationSeconds int          `json:"target_duration_seconds"`
	Resolution            string       `json:"resolution"`
	Voiceover             voiceDTO     `json:"voiceover"`
	Subtitles             subtitlesDTO `json:"subtitles"`
	Music                 musicDTO     `json:"music"`
	Source                sourceDTO    `json:"source"`
}

type limitsDTO struct {
	MaxTotalSeconds int `json:"max_total_seconds"`
	MaxScenes       int `json:"max_scenes"`
	MinSceneMs      int `json:"min_scene_ms"`
	MaxSceneMs      int `json:"max_scene_ms"`
}

type projectDTO struct {
	ID              kernel.ID   `json:"id"`
	Title           string      `json:"title"`
	Status          string      `json:"status"`
	Stage           string      `json:"stage"`
	CampaignID      *kernel.ID  `json:"campaign_id"`
	BrandID         kernel.ID   `json:"brand_id"`
	CreatedBy       kernel.ID   `json:"created_by"`
	Prompt          *string     `json:"prompt"`
	Script          *string     `json:"script"`
	Settings        settingsDTO `json:"settings"`
	Scenes          []sceneDTO  `json:"scenes"`
	TotalDurationMs int         `json:"total_duration_ms"`
	Limits          limitsDTO   `json:"limits"`
	Output          *outputDTO  `json:"output"`
	CurrentJob      *jobDTO     `json:"current_job"`
	VersionNo       int         `json:"version_no"`
	CreatedAt       time.Time   `json:"created_at"`
	UpdatedAt       time.Time   `json:"updated_at"`
}

func jobFrom(j *jobuc.JobView, progress int) *jobDTO {
	if j == nil {
		return nil
	}
	out := &jobDTO{ID: j.ID, JobType: string(j.Type), Status: string(j.Status), Progress: progress, Steps: make([]stepDTO, 0, len(j.Steps)),
		ErrorCode: ptr(j.ErrorCode), ErrorMessage: ptr(j.ErrorMessage)}
	for _, st := range j.Steps {
		out.Steps = append(out.Steps, stepDTO{StepKey: st.Key, StepType: st.Type, Status: string(st.Status), AttemptCount: st.AttemptCount,
			ErrorKind: ptr(st.ErrorKind), ErrorMessage: ptr(st.ErrorMessage)})
	}
	return out
}

func projectFrom(v usecase.ProjectView) projectDTO {
	p := v.Project
	st := p.Settings
	fileIDs := st.Source.FileIDs
	if fileIDs == nil {
		fileIDs = []kernel.ID{}
	}
	out := projectDTO{
		ID: p.AssetID, Title: p.Title, Status: v.AssetStatus, Stage: string(v.Stage), CampaignID: p.CampaignID, BrandID: p.BrandID,
		CreatedBy: p.CreatedBy, Prompt: ptr(p.Prompt), Script: ptr(p.Script),
		Settings: settingsDTO{
			Preset: st.Preset, AspectRatio: st.AspectRatio, Width: st.Width, Height: st.Height, FPS: st.FPS,
			TargetDurationSeconds: st.TargetDurationSeconds, Resolution: st.Resolution, Voiceover: voiceFrom(st.Voiceover),
			Subtitles: subtitlesDTO(st.Subtitles), Music: musicDTO(st.Music), Source: sourceDTO{Type: string(st.Source.Type), FileIDs: fileIDs},
		},
		Scenes: make([]sceneDTO, 0, len(v.Scenes)), TotalDurationMs: v.TotalMs,
		Limits:     limitsDTO{MaxTotalSeconds: domain.MaxTotalMs / 1000, MaxScenes: domain.MaxScenes, MinSceneMs: domain.MinSceneMs, MaxSceneMs: domain.MaxSceneMs},
		CurrentJob: jobFrom(v.Job, v.Progress), VersionNo: v.VersionNo, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
	if out.Status == "" {
		out.Status = "draft"
	}
	for _, sc := range v.Scenes {
		out.Scenes = append(out.Scenes, sceneDTO{
			ID: sc.ID, SequenceNo: sc.SequenceNo, DurationMs: sc.DurationMs, VisualPrompt: sc.VisualPrompt, Narration: ptr(sc.Narration),
			SubtitleText: ptr(sc.SubtitleText), SourceFileID: sc.SourceFileID, ClipFileID: sc.ClipFileID, Status: string(sc.Status),
			AttemptCount: sc.AttemptCount, LastError: ptr(sc.LastError), PreviewURL: ptr(sc.PreviewURL), ThumbnailURL: ptr(sc.ThumbnailURL),
		})
	}
	if o := v.Output; o != nil {
		out.Output = &outputDTO{FileID: o.FileID, URL: o.URL, ThumbnailURL: ptr(o.ThumbnailURL), DurationMs: o.DurationMs,
			VersionNo: o.VersionNo, SubtitleFileID: o.SubtitleFileID, SubtitleURL: ptr(o.SubtitleURL)}
	}
	return out
}

type sceneInputDTO struct {
	ID           *kernel.ID `json:"id"`
	DurationMs   int        `json:"duration_ms"`
	VisualPrompt string     `json:"visual_prompt"`
	Narration    *string    `json:"narration"`
	SubtitleText *string    `json:"subtitle_text"`
	SourceFileID *kernel.ID `json:"source_file_id"`
}

type saveScenesRequest struct {
	Scenes []sceneInputDTO `json:"scenes"`
}

type produceRequest struct {
	BudgetOverride bool `json:"budget_override"`
}

type regenerateRequest struct {
	VisualPrompt   *string `json:"visual_prompt"`
	BudgetOverride bool    `json:"budget_override"`
}

type renderRequest struct {
	Voiceover      *voiceDTO     `json:"voiceover"`
	Subtitles      *subtitlesDTO `json:"subtitles"`
	Music          *musicDTO     `json:"music"`
	BudgetOverride bool          `json:"budget_override"`
}

type estimateLineDTO struct {
	Item      string  `json:"item"`
	Provider  *string `json:"provider"`
	Model     *string `json:"model"`
	Units     float64 `json:"units"`
	Unit      string  `json:"unit"`
	UnitPrice float64 `json:"unit_price"`
	Amount    float64 `json:"amount"`
}

type estimateLimitsDTO struct {
	PerJobLimit       float64  `json:"per_job_limit"`
	CampaignRemaining *float64 `json:"campaign_remaining"`
	GlobalRemaining   float64  `json:"global_remaining"`
}

type estimateDTO struct {
	SceneCount       int               `json:"scene_count"`
	TotalClipSeconds int               `json:"total_clip_seconds"`
	Breakdown        []estimateLineDTO `json:"breakdown"`
	Total            moneyDTO          `json:"total"`
	Limits           estimateLimitsDTO `json:"limits"`
	RequiresOverride bool              `json:"requires_override"`
	Blocked          bool              `json:"blocked"`
	Violations       []kernel.Detail   `json:"violations"`
}

func estimateFrom(e usecase.EstimateView) estimateDTO {
	out := estimateDTO{
		SceneCount: e.SceneCount, TotalClipSeconds: e.TotalClipSeconds, Breakdown: make([]estimateLineDTO, 0, len(e.Lines)),
		Total: money(e.Total, true), RequiresOverride: e.Limits.RequiresOverride, Blocked: e.Limits.Blocked, Violations: e.Violations,
		Limits: estimateLimitsDTO{PerJobLimit: e.Limits.PerJobLimit.Float64(), GlobalRemaining: e.Limits.GlobalRemaining.Float64()},
	}
	if e.Limits.CampaignRemaining != nil {
		f := e.Limits.CampaignRemaining.Float64()
		out.Limits.CampaignRemaining = &f
	}
	for _, l := range e.Lines {
		units, _ := l.Units.Float64()
		price, _ := l.UnitPrice.Float64()
		out.Breakdown = append(out.Breakdown, estimateLineDTO{Item: l.Item, Provider: ptr(l.ProviderCode), Model: ptr(l.ModelCode),
			Units: units, Unit: l.Unit, UnitPrice: price, Amount: l.Amount.Float64()})
	}
	if out.Violations == nil {
		out.Violations = []kernel.Detail{}
	}
	return out
}
