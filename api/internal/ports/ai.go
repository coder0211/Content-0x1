package ports

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/be/acs/api/internal/shared/kernel"
)

// AIKind is the capability family of a provider model.
type AIKind string

const (
	AIKindText  AIKind = "text"
	AIKindImage AIKind = "image"
	AIKindVideo AIKind = "video"
	AIKindVoice AIKind = "voice"
)

// Capabilities describes what a model accepts (ai_models.capabilities).
// Use cases validate requests against it and answer 422
// CAPABILITY_NOT_SUPPORTED instead of silently changing parameters.
type Capabilities struct {
	AspectRatios     []string `json:"aspect_ratios,omitempty"`
	Resolutions      []string `json:"resolutions,omitempty"`
	Qualities        []string `json:"qualities,omitempty"`
	MinDurationSec   int      `json:"min_duration_sec,omitempty"`
	MaxDurationSec   int      `json:"max_duration_sec,omitempty"`
	MaxVariants      int      `json:"max_variants,omitempty"`
	SupportsRefImage bool     `json:"supports_ref_image,omitempty"`
	SupportsEdit     bool     `json:"supports_edit,omitempty"`
	SupportsCancel   bool     `json:"supports_cancel,omitempty"`
	Languages        []string `json:"languages,omitempty"`
	Voices           []string `json:"voices,omitempty"`
	MaxOutputTokens  int      `json:"max_output_tokens,omitempty"`
}

// Usage is what one provider call consumed. Units is keyed by pricing unit
// (model_pricing.unit, e.g. "input_token", "output_token",
// "image_1024_standard", "video_second_1080p", "char") so the budget module
// can price it without knowing the provider. ReportedCost is set only when
// the provider returns an authoritative cost (cost_source=provider_reported).
type Usage struct {
	InputTokens  int64              `json:"input_tokens,omitempty"`
	OutputTokens int64              `json:"output_tokens,omitempty"`
	CachedTokens int64              `json:"cached_tokens,omitempty"`
	Images       int                `json:"images,omitempty"`
	VideoSeconds float64            `json:"video_seconds,omitempty"`
	Characters   int                `json:"characters,omitempty"`
	Units        map[string]float64 `json:"units"`
	ReportedCost *kernel.Money      `json:"-"`
}

// TextUsage builds a Usage for a token-billed text call.
func TextUsage(input, output, cached int64) Usage {
	return Usage{
		InputTokens: input, OutputTokens: output, CachedTokens: cached,
		Units: map[string]float64{"input_token": float64(input), "output_token": float64(output)},
	}
}

// Message is one chat turn. Untrusted user data (brief, documents) goes in
// role=user messages wrapped in <user_data> tags by the prompt builder;
// instructions go in TextRequest.System only.
type Message struct {
	Role    string `json:"role"` // user | assistant
	Content string `json:"content"`
}

// TextRequest is a provider-neutral text generation request.
type TextRequest struct {
	Model       string
	System      string
	Messages    []Message
	JSONSchema  json.RawMessage // optional structured output schema
	SchemaName  string
	MaxTokens   int
	Temperature float32
}

// TextResponse is the provider-neutral result. JSON is set when JSONSchema
// was requested. Raw is the provider payload with secrets stripped, stored
// in ai_job_steps.output_json for debugging.
type TextResponse struct {
	Content    string
	JSON       json.RawMessage
	StopReason string
	Usage      Usage
	Raw        json.RawMessage
}

// TextGenerator is implemented by text provider adapters.
type TextGenerator interface {
	Code() string
	Generate(ctx context.Context, req TextRequest) (TextResponse, error)
}

// MediaInput is binary media handed to a provider. Workers only load it from
// internal object storage (no user-supplied URLs: SSRF rule, SPEC-02 §7).
type MediaInput struct {
	MimeType string
	Data     []byte
}

// ImageRequest is a provider-neutral image generation request.
type ImageRequest struct {
	Model           string
	Prompt          string
	NegativePrompt  string
	AspectRatio     string // "1:1", "4:5", "9:16", "16:9"
	Quality         string // "standard" | "high"
	Count           int
	ReferenceImages []MediaInput
	Seed            *int64
}

// ImageEditRequest edits an existing image with a prompt and optional mask.
type ImageEditRequest struct {
	Model       string
	Prompt      string
	Image       MediaInput
	Mask        *MediaInput
	AspectRatio string
	Quality     string
	Count       int
}

// GeneratedImage is one output image, already downloaded by the adapter.
type GeneratedImage struct {
	MimeType string
	Data     []byte
	Width    int
	Height   int
}

// ImageResponse is the result of Generate or Edit.
type ImageResponse struct {
	Images []GeneratedImage
	Usage  Usage
	Raw    json.RawMessage
}

// ImageGenerator is implemented by image provider adapters.
type ImageGenerator interface {
	Code() string
	Capabilities(model string) Capabilities
	Generate(ctx context.Context, req ImageRequest) (ImageResponse, error)
	Edit(ctx context.Context, req ImageEditRequest) (ImageResponse, error)
}

// VideoRequest asks for one scene clip (text-to-video, or image-to-video
// when Image is set).
type VideoRequest struct {
	Model          string
	Prompt         string
	NegativePrompt string
	AspectRatio    string
	Resolution     string // "720p" | "1080p"
	DurationSec    int
	Image          *MediaInput
	Seed           *int64
}

// VideoSubmission is returned once the provider accepted the request.
type VideoSubmission struct {
	ProviderJobID string
	// PollAfter is the suggested delay before the first GetStatus call.
	PollAfter time.Duration
}

// VideoState is the provider-side state of a submitted clip.
type VideoState string

const (
	VideoQueued    VideoState = "queued"
	VideoRunning   VideoState = "running"
	VideoSucceeded VideoState = "succeeded"
	VideoFailed    VideoState = "failed"
	VideoCancelled VideoState = "cancelled"
)

// VideoStatus is the result of polling a submission. Error is set when
// State is VideoFailed; Usage is set once the provider bills the clip.
type VideoStatus struct {
	State       VideoState
	Progress    int
	DurationSec float64
	Error       *ProviderError
	Usage       *Usage
	Raw         json.RawMessage
}

// MediaStream is a downloadable provider output.
type MediaStream struct {
	Body     io.ReadCloser
	MimeType string
	Size     int64 // -1 when unknown
}

// VideoGenerator is implemented by video provider adapters. Generation is
// asynchronous: Submit, then poll GetStatus (re-enqueue with delay; never
// block a worker), then Download.
type VideoGenerator interface {
	Code() string
	Capabilities(model string) Capabilities
	Submit(ctx context.Context, req VideoRequest) (VideoSubmission, error)
	GetStatus(ctx context.Context, providerJobID string) (VideoStatus, error)
	Download(ctx context.Context, providerJobID string) (MediaStream, error)
	// Cancel is best effort; ErrCancelUnsupported when the vendor cannot.
	Cancel(ctx context.Context, providerJobID string) error
}

// ErrCancelUnsupported is returned by VideoGenerator.Cancel when the vendor
// has no cancel API. Usage must still be recorded.
var ErrCancelUnsupported = errors.New("provider does not support cancel")

// VoiceRequest is a text-to-speech request.
type VoiceRequest struct {
	Model        string
	Text         string
	Language     string // "vi-VN"
	VoiceID      string
	SpeakingRate float64 // 1.0 = normal
	Format       string  // "mp3" | "wav"
}

// WordTiming aligns a word (or SSML mark) with the audio timeline; used to
// build subtitles.
type WordTiming struct {
	Text    string `json:"text"`
	StartMs int    `json:"start_ms"`
	EndMs   int    `json:"end_ms"`
}

// VoiceResponse is synthesized audio plus optional timings.
type VoiceResponse struct {
	MimeType   string
	Audio      []byte
	DurationMs int
	Timings    []WordTiming
	Usage      Usage
	Raw        json.RawMessage
}

// VoiceSynthesizer is implemented by TTS adapters.
type VoiceSynthesizer interface {
	Code() string
	Capabilities(model string) Capabilities
	Synthesize(ctx context.Context, req VoiceRequest) (VoiceResponse, error)
}

// AIProviders resolves adapters by provider code (ai_providers.code). The
// model itself is chosen by the provider-config module; this only maps the
// code to a live adapter.
type AIProviders interface {
	Text(providerCode string) (TextGenerator, error)
	Image(providerCode string) (ImageGenerator, error)
	Video(providerCode string) (VideoGenerator, error)
	Voice(providerCode string) (VoiceSynthesizer, error)
}

// ErrKind classifies provider failures (SPEC-02 §4.2). Adapters MUST map
// every vendor error to one of these.
type ErrKind string

const (
	ErrKindTransient      ErrKind = "Transient"
	ErrKindRateLimited    ErrKind = "RateLimited"
	ErrKindInvalidRequest ErrKind = "InvalidRequest"
	ErrKindContentPolicy  ErrKind = "ContentPolicy"
	ErrKindAuth           ErrKind = "Auth"
	ErrKindUnknown        ErrKind = "Unknown"
)

// ProviderError is the only error type adapters return for vendor failures.
type ProviderError struct {
	Kind       ErrKind
	Provider   string
	Code       string // vendor error code
	Message    string // vendor message, safe to log (no secrets)
	RetryAfter time.Duration
	Err        error
}

func (e *ProviderError) Error() string {
	return fmt.Sprintf("provider %s: %s (%s): %s", e.Provider, e.Kind, e.Code, e.Message)
}

func (e *ProviderError) Unwrap() error { return e.Err }

// Retryable reports whether the job engine may retry the step: only
// Transient and RateLimited are retried.
func (e *ProviderError) Retryable() bool {
	return e.Kind == ErrKindTransient || e.Kind == ErrKindRateLimited
}

// AsProviderError extracts a *ProviderError from err's chain.
func AsProviderError(err error) (*ProviderError, bool) {
	var pe *ProviderError
	if errors.As(err, &pe) {
		return pe, true
	}
	return nil, false
}
