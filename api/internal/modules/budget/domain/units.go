package domain

import (
	"fmt"
	"strings"
)

// Pricing units (model_pricing.unit). Providers report usage keyed by these.
const (
	UnitInputToken  = "input_token"
	UnitOutputToken = "output_token"
	UnitChar        = "char"
	// UnitRenderSecond is internal FFmpeg worker time: tracked for
	// statistics only and always priced at 0 (SPEC-02 §6.1).
	UnitRenderSecond = "render_second"
)

// IsInternalUnit reports units that are never looked up in model_pricing
// and cost nothing.
func IsInternalUnit(unit string) bool { return unit == UnitRenderSecond }

// Text token estimation (SPEC-02 §6.3: "ước token từ độ dài prompt + max_tokens").
const (
	// CharsPerToken is a conservative ratio for Vietnamese/English mixed text
	// (Vietnamese diacritics tokenize worse than English's ~4 chars/token).
	CharsPerToken = 3
	// PromptOverheadTokens covers the system prompt, brand context and
	// output schema that wrap the user's prompt.
	PromptOverheadTokens = 600
	// DefaultMaxOutputTokens is used when the caller does not cap output.
	DefaultMaxOutputTokens = 1024
)

// EstimateInputTokens estimates prompt tokens from its length in characters
// (runes), rounding up, plus the fixed overhead.
func EstimateInputTokens(promptChars int) int64 {
	if promptChars < 0 {
		promptChars = 0
	}
	return int64((promptChars+CharsPerToken-1)/CharsPerToken) + PromptOverheadTokens
}

// ImageUnit is the pricing unit for one image: image_<size>_<quality>, e.g.
// image_1024_standard (ACS-054: count × size × quality × model). size is the
// long edge bucket ("1024", "1536"...); quality is standard|high.
func ImageUnit(size, quality string) string {
	if size == "" {
		size = "1024"
	}
	if quality == "" {
		quality = "standard"
	}
	return fmt.Sprintf("image_%s_%s", strings.ToLower(size), strings.ToLower(quality))
}

// VideoSecondUnit is the pricing unit for one second of generated clip at a
// resolution, e.g. video_second_1080p.
func VideoSecondUnit(resolution string) string {
	if resolution == "" {
		resolution = "1080p"
	}
	return "video_second_" + strings.ToLower(resolution)
}
