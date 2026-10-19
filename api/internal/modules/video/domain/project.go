// Package domain holds the Video Studio rules: presets and limits (OQ-07),
// storyboard validation and normalisation, the production DAGs (SPEC-02
// §5.5), timeline/subtitle building and output quality checks. Pure Go.
package domain

import (
	"time"

	"github.com/be/acs/api/internal/shared/kernel"
)

// Limits from DECISIONS OQ-07.
const (
	MaxScenes        = 10
	MinSceneMs       = 2000
	MaxSceneMs       = 8000
	MaxTotalMs       = 60000
	MinTargetSeconds = 4
	MaxTargetSeconds = 60
	FPS              = 30
	// TransitionMs is the cross-fade between scenes (xfade). Every
	// transition shortens the final video by this amount; Timeline accounts
	// for it so voice and subtitles stay aligned.
	TransitionMs = 300
	// DurationToleranceMs is the ffprobe duration tolerance (FR-VID-06).
	DurationToleranceMs = 500
	MaxPromptLen        = 4000
	MaxSceneTextLen     = 1000
)

// Preset is an output format (OQ-07).
type Preset struct {
	Code        string
	AspectRatio string
	Width       int
	Height      int
}

// Presets keyed by code.
var Presets = map[string]Preset{
	"social_vertical": {Code: "social_vertical", AspectRatio: "9:16", Width: 1080, Height: 1920},
	"square":          {Code: "square", AspectRatio: "1:1", Width: 1080, Height: 1080},
	"landscape":       {Code: "landscape", AspectRatio: "16:9", Width: 1920, Height: 1080},
}

// PresetFor returns the preset with code.
func PresetFor(code string) (Preset, bool) {
	p, ok := Presets[code]
	return p, ok
}

// SourceType is where the storyboard comes from.
type SourceType string

const (
	SourcePrompt SourceType = "prompt"
	SourceImage  SourceType = "image"
	SourceScript SourceType = "script"
)

// Source describes the user's input.
type Source struct {
	Type    SourceType  `json:"type"`
	FileIDs []kernel.ID `json:"file_ids"`
}

// VoiceSettings controls the voice-over.
type VoiceSettings struct {
	Enabled  bool   `json:"enabled"`
	Language string `json:"language"`
	VoiceID  string `json:"voice_id,omitempty"`
	// Mode is "per_scene" (default: one TTS call per scene, aligned to the
	// scene start) or "whole" (one call for the whole narration).
	Mode string `json:"mode,omitempty"`
}

// SubtitleSettings controls subtitles.
type SubtitleSettings struct {
	Enabled  bool   `json:"enabled"`
	Language string `json:"language"`
	Style    string `json:"style"` // burn_in | soft
}

// MusicSettings selects a licensed brand music track.
type MusicSettings struct {
	Enabled           bool       `json:"enabled"`
	TrackBrandAssetID *kernel.ID `json:"track_brand_asset_id"`
}

// Settings are the project's production settings.
type Settings struct {
	Preset                string           `json:"preset"`
	AspectRatio           string           `json:"aspect_ratio"`
	Width                 int              `json:"width"`
	Height                int              `json:"height"`
	FPS                   int              `json:"fps"`
	TargetDurationSeconds int              `json:"target_duration_seconds"`
	Resolution            string           `json:"resolution"`
	Source                Source           `json:"source"`
	Voiceover             VoiceSettings    `json:"voiceover"`
	Subtitles             SubtitleSettings `json:"subtitles"`
	Music                 MusicSettings    `json:"music"`
}

// VoiceSegment is one synthesized audio file placed on the timeline.
type VoiceSegment struct {
	SceneID    *kernel.ID `json:"scene_id"`
	FileID     kernel.ID  `json:"file_id"`
	StartMs    int        `json:"start_ms"`
	DurationMs int        `json:"duration_ms"`
	// MaxMs trims the segment to its scene slot (0 = untrimmed).
	MaxMs   int          `json:"max_ms"`
	Timings []WordTiming `json:"timings"`
}

// WordTiming is a TTS word timing relative to the segment start.
type WordTiming struct {
	Text    string `json:"text"`
	StartMs int    `json:"start_ms"`
	EndMs   int    `json:"end_ms"`
}

// VoiceTrack is the stored output of voiceover_generation.
type VoiceTrack struct {
	Fingerprint string         `json:"fingerprint"`
	Segments    []VoiceSegment `json:"segments"`
}

// SubtitleTrack is the stored output of subtitle_generation.
type SubtitleTrack struct {
	Fingerprint string    `json:"fingerprint"`
	SRTFileID   kernel.ID `json:"srt_file_id"`
	ASSFileID   kernel.ID `json:"ass_file_id"`
	CueCount    int       `json:"cue_count"`
}

// Output is the latest rendered video that passed quality checks.
type Output struct {
	FileID          kernel.ID  `json:"file_id"`
	ThumbnailFileID *kernel.ID `json:"thumbnail_file_id"`
	SubtitleFileID  *kernel.ID `json:"subtitle_file_id"`
	DurationMs      int        `json:"duration_ms"`
	VersionNo       int        `json:"version_no"`
}

// Project is a video project (video_projects row); its id is the
// content_assets id of type video.
type Project struct {
	AssetID      kernel.ID
	BrandID      kernel.ID
	CampaignID   *kernel.ID
	Title        string
	Prompt       string
	Script       string
	Settings     Settings
	CurrentJobID *kernel.ID
	Voice        *VoiceTrack
	Subtitle     *SubtitleTrack
	Output       *Output
	CreatedBy    kernel.ID
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Stage is the UI-level phase derived from the current job and output.
type Stage string

const (
	StageStoryboarding Stage = "storyboarding"
	StageEditing       Stage = "editing"
	StageProducing     Stage = "producing"
	StageReady         Stage = "ready"
	StageFailed        Stage = "failed"
)

// DeriveStage computes the stage from the current job (type/status may be
// empty when there is none) and whether an output exists.
func DeriveStage(jobType, jobStatus string, hasOutput bool) Stage {
	running := jobStatus == "queued" || jobStatus == "running"
	switch {
	case jobType == "video_storyboard" && running:
		return StageStoryboarding
	case jobType != "" && running:
		return StageProducing
	case jobType != "" && jobType != "video_storyboard" && jobStatus == "failed":
		return StageFailed
	case hasOutput:
		return StageReady
	}
	return StageEditing
}
