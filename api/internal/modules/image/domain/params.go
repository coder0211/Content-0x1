// Package domain holds the Image Studio rules (FR-IMG-01..06): presets,
// option validation, capability checks against the chosen model, pricing
// units and the content_json of an image asset version. Pure: stdlib +
// kernel only.
package domain

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/be/acs/api/internal/shared/kernel"
)

// Preset is the purpose of an image (FR-IMG-01); stored as the asset subtype.
type Preset string

const (
	PresetSocialPost   Preset = "social_post"
	PresetPoster       Preset = "poster"
	PresetProductPhoto Preset = "product_photo"
	PresetThumbnail    Preset = "thumbnail"
)

// Presets lists every preset in UI order.
var Presets = []Preset{PresetSocialPost, PresetPoster, PresetProductPhoto, PresetThumbnail}

// AspectRatios are the ratios the product offers (the model may support fewer).
var AspectRatios = []string{"1:1", "4:5", "9:16", "16:9"}

// Qualities are the quality levels the product offers.
var Qualities = []string{"standard", "high"}

// Limits of a single request.
const (
	MinCount        = 1
	MaxCount        = 4
	MaxPromptRunes  = 4000
	MaxReferences   = 4
	MaxTitleRunes   = 80
	DefaultAspect   = "1:1"
	DefaultQuality  = "standard"
	DefaultCount    = 1
	ThumbnailMaxDim = 384
)

// Operation distinguishes the three image jobs.
type Operation string

const (
	OpGenerate  Operation = "generate"
	OpEdit      Operation = "edit"
	OpVariation Operation = "variation"
)

// Options are the user-chosen generation options.
type Options struct {
	Prompt      string
	Preset      Preset
	AspectRatio string
	Quality     string
	Count       int
}

// Normalize trims the prompt and fills defaults for omitted options.
func (o Options) Normalize() Options {
	o.Prompt = strings.TrimSpace(o.Prompt)
	if o.AspectRatio == "" {
		o.AspectRatio = DefaultAspect
	}
	if o.Quality == "" {
		o.Quality = DefaultQuality
	}
	if o.Count == 0 {
		o.Count = DefaultCount
	}
	return o
}

// Validate reports schema problems (400 VALIDATION_FAILED). Values the
// product offers but the model does not are checked by CheckCapabilities
// (422) so the FE can show the supported values.
func (o Options) Validate(requirePrompt bool) error {
	var v kernel.ValidationErrors
	switch n := utf8.RuneCountInString(o.Prompt); {
	case requirePrompt && n == 0:
		v.Add("prompt", "required")
	case n > MaxPromptRunes:
		v.Add("prompt", "too_long")
	}
	if !slices.Contains(Presets, o.Preset) {
		v.AddDetail(kernel.Detail{"field": "preset", "issue": "invalid", "supported": presetStrings()})
	}
	if !slices.Contains(AspectRatios, o.AspectRatio) {
		v.AddDetail(kernel.Detail{"field": "aspect_ratio", "issue": "invalid", "supported": AspectRatios})
	}
	if !slices.Contains(Qualities, o.Quality) {
		v.AddDetail(kernel.Detail{"field": "quality", "issue": "invalid", "supported": Qualities})
	}
	if o.Count < MinCount || o.Count > MaxCount {
		v.Add("count", "out_of_range")
	}
	return v.Err()
}

func presetStrings() []string {
	out := make([]string, len(Presets))
	for i, p := range Presets {
		out[i] = string(p)
	}
	return out
}

// Caps is the subset of model capabilities Image Studio relies on (mapped
// from ports.Capabilities by the use case).
type Caps struct {
	AspectRatios     []string
	Qualities        []string
	MaxVariants      int
	SupportsRefImage bool
	SupportsEdit     bool
}

// CheckCapabilities returns 422 CAPABILITY_NOT_SUPPORTED listing every
// unsupported option with the valid values. Parameters are never changed
// silently (FR-IMG-01). Empty capability lists mean "not restricted".
func CheckCapabilities(c Caps, o Options, op Operation, refCount int) error {
	var details []kernel.Detail
	if len(c.AspectRatios) > 0 && !slices.Contains(c.AspectRatios, o.AspectRatio) {
		details = append(details, kernel.Detail{"field": "aspect_ratio", "supported": c.AspectRatios})
	}
	if len(c.Qualities) > 0 && !slices.Contains(c.Qualities, o.Quality) {
		details = append(details, kernel.Detail{"field": "quality", "supported": c.Qualities})
	}
	if c.MaxVariants > 0 && o.Count > c.MaxVariants {
		details = append(details, kernel.Detail{"field": "count", "supported": rangeInts(1, c.MaxVariants)})
	}
	if (refCount > 0 || op == OpVariation) && !c.SupportsRefImage {
		field := "reference_file_ids"
		if op == OpVariation {
			field = "file_id"
		}
		details = append(details, kernel.Detail{"field": field, "issue": "reference_image_not_supported", "supported": []string{}})
	}
	if op == OpEdit && !c.SupportsEdit {
		details = append(details, kernel.Detail{"field": "prompt", "issue": "edit_not_supported", "supported": []string{}})
	}
	if len(details) == 0 {
		return nil
	}
	return kernel.ErrCapability.WithDetails(details...)
}

func rangeInts(from, to int) []int {
	out := make([]int, 0, to-from+1)
	for i := from; i <= to; i++ {
		out = append(out, i)
	}
	return out
}

// AllowedReferenceMIME are the image types providers accept as references
// or edit sources (SVG is rejected: it is not a raster image).
var AllowedReferenceMIME = []string{"image/png", "image/jpeg", "image/webp"}

// CheckReferenceMIME returns 415 UNSUPPORTED_MEDIA_TYPE for other types.
func CheckReferenceMIME(field, mime string) error {
	if slices.Contains(AllowedReferenceMIME, mime) {
		return nil
	}
	return kernel.ErrUnsupportedMedia.WithDetails(kernel.Detail{"field": field, "mime_type": mime, "supported": AllowedReferenceMIME})
}

// PricingUnit is the model_pricing.unit of one image: square images are
// priced as 1024px, other ratios as the larger 1536px canvas
// (`image_<long side>_<quality>`). FallbackPricingUnit is used when a model
// only prices the 1024 unit.
func PricingUnit(aspectRatio, quality string) string {
	if aspectRatio == "" || aspectRatio == "1:1" {
		return FallbackPricingUnit(quality)
	}
	return fmt.Sprintf("image_1536_%s", quality)
}

// FallbackPricingUnit is the base unit every image model prices.
func FallbackPricingUnit(quality string) string {
	if quality == "" {
		quality = DefaultQuality
	}
	return "image_1024_" + quality
}

// RequiresTypographyCheck reports whether a reviewer checklist item
// "typography_checked" is mandatory before review (FR-IMG-06).
func RequiresTypographyCheck(p Preset) bool { return p == PresetPoster }

// Title derives an asset title from the prompt (first line, ≤ 80 runes).
func Title(prompt string, p Preset) string {
	line, _, _ := strings.Cut(strings.TrimSpace(prompt), "\n")
	line = strings.TrimSpace(line)
	if line == "" {
		return "Ảnh " + string(p)
	}
	if utf8.RuneCountInString(line) <= MaxTitleRunes {
		return line
	}
	r := []rune(line)
	return strings.TrimSpace(string(r[:MaxTitleRunes-1])) + "…"
}

// ThumbSize scales (w, h) to fit in max×max keeping the ratio.
func ThumbSize(w, h, maxDim int) (int, int) {
	if w <= 0 || h <= 0 {
		return 0, 0
	}
	if w <= maxDim && h <= maxDim {
		return w, h
	}
	if w >= h {
		return maxDim, max(1, h*maxDim/w)
	}
	return max(1, w*maxDim/h), maxDim
}
