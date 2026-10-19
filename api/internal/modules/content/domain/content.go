// Package domain holds the content module's value objects and pure rules
// (formats, request validation, post-generation checks, HTML rendering).
package domain

import (
	"html"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/be/acs/api/internal/shared/kernel"
)

// Format is the output format of a content asset (content_assets.subtype)
// and selects the prompt template.
type Format string

const (
	FormatFacebookPost Format = "facebook_post"
	FormatTikTokScript Format = "tiktok_script"
	FormatCaption      Format = "caption"
	FormatEmail        Format = "email"
	FormatVideoScript  Format = "video_script"
)

// Formats lists every content format.
var Formats = []Format{FormatFacebookPost, FormatTikTokScript, FormatCaption, FormatEmail, FormatVideoScript}

// TransformTargets are the formats an existing post can be converted to
// (FR-CNT-03, FR-CNT-05).
var TransformTargets = []Format{FormatTikTokScript, FormatCaption, FormatEmail, FormatVideoScript, FormatFacebookPost}

// Platform is the publishing channel.
type Platform string

const (
	PlatformFacebook  Platform = "facebook"
	PlatformInstagram Platform = "instagram"
	PlatformTikTok    Platform = "tiktok"
	PlatformZalo      Platform = "zalo"
	PlatformEmail     Platform = "email"
)

// Platforms lists every platform.
var Platforms = []Platform{PlatformFacebook, PlatformInstagram, PlatformTikTok, PlatformZalo, PlatformEmail}

// DefaultFormat is the format used when the request names only a platform.
func DefaultFormat(p Platform) Format {
	switch p {
	case PlatformTikTok:
		return FormatTikTokScript
	case PlatformInstagram, PlatformZalo:
		return FormatCaption
	case PlatformEmail:
		return FormatEmail
	}
	return FormatFacebookPost
}

// ContentTypes are the purposes of a post (SPEC-04 example "promotional_post").
var ContentTypes = []string{"promotional_post", "educational_post", "engagement_post", "announcement", "testimonial"}

// Objectives mirror campaign goals.
var Objectives = []string{"awareness", "engagement", "lead", "conversion"}

// Frameworks are copywriting frameworks (FR-CNT-01); "" means free form.
var Frameworks = []string{"AIDA", "PAS", "Storytelling", "SEO"}

// Lengths are relative output lengths.
var Lengths = []string{"short", "medium", "long"}

// Languages supported for output (NFR-12).
var Languages = []string{"vi", "en"}

// RewriteInstructions are the quick rewrite actions (FR-CNT-02).
var RewriteInstructions = []string{"shorten", "expand", "change_tone", "custom"}

// Limits.
const (
	MinVariants      = 1
	MaxVariants      = 5
	MaxTones         = 5
	MaxToneLen       = 40
	MaxBriefFieldLen = 2000
	MaxReferenceLen  = 8000
	MaxCustomPrompt  = 1000
)

// ValidFormat reports whether f is known.
func ValidFormat(f string) bool { return slices.Contains(Formats, Format(f)) }

// ValidPlatform reports whether p is known.
func ValidPlatform(p string) bool { return slices.Contains(Platforms, Platform(p)) }

// CheckText adds a validation issue when s is longer than max runes.
func CheckText(v *kernel.ValidationErrors, field, s string, max int) {
	if utf8.RuneCountInString(s) > max {
		v.Add(field, "too_long")
	}
}

// CheckTones validates a tone list.
func CheckTones(v *kernel.ValidationErrors, field string, tones []string) {
	if len(tones) > MaxTones {
		v.Add(field, "too_many")
		return
	}
	for _, t := range tones {
		if strings.TrimSpace(t) == "" || utf8.RuneCountInString(t) > MaxToneLen {
			v.Add(field, "invalid")
			return
		}
	}
}

// Scene is a video-script scene.
type Scene struct {
	Narration   string
	VisualHint  string
	DurationSec int
}

// Variant is one generated piece of content.
type Variant struct {
	Title    string
	Hook     string
	Body     string
	CTA      string
	Hashtags []string
	Scenes   []Scene
}

// Normalize trims fields, drops empty hashtags and adds a leading '#'.
func (v Variant) Normalize() Variant {
	v.Title = strings.TrimSpace(v.Title)
	v.Hook = strings.TrimSpace(v.Hook)
	v.Body = strings.TrimSpace(v.Body)
	v.CTA = strings.TrimSpace(v.CTA)
	tags := make([]string, 0, len(v.Hashtags))
	for _, h := range v.Hashtags {
		h = strings.Join(strings.Fields(h), "")
		if h == "" || h == "#" {
			continue
		}
		if !strings.HasPrefix(h, "#") {
			h = "#" + h
		}
		if !slices.Contains(tags, h) {
			tags = append(tags, h)
		}
	}
	v.Hashtags = tags
	scenes := make([]Scene, 0, len(v.Scenes))
	for _, s := range v.Scenes {
		s.Narration, s.VisualHint = strings.TrimSpace(s.Narration), strings.TrimSpace(s.VisualHint)
		if s.Narration == "" && s.VisualHint == "" {
			continue
		}
		if s.DurationSec < 0 {
			s.DurationSec = 0
		}
		scenes = append(scenes, s)
	}
	v.Scenes = scenes
	return v
}

// IsEmpty reports whether the variant has no usable text.
func (v Variant) IsEmpty() bool { return v.Body == "" && v.Hook == "" && v.Title == "" }

// Texts returns every user-visible text of the variant (for checks/search).
func (v Variant) Texts() []string {
	out := []string{v.Title, v.Hook, v.Body, v.CTA, strings.Join(v.Hashtags, " ")}
	for _, s := range v.Scenes {
		out = append(out, s.Narration, s.VisualHint)
	}
	return out
}

// RenderHTML converts plain text into safe HTML paragraphs. Model output is
// never trusted as HTML: everything is escaped, blank lines split paragraphs
// and single newlines become <br>.
func RenderHTML(text string) string {
	text = strings.ReplaceAll(strings.TrimSpace(text), "\r\n", "\n")
	if text == "" {
		return ""
	}
	var b strings.Builder
	for _, para := range strings.Split(text, "\n\n") {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		lines := strings.Split(para, "\n")
		for i := range lines {
			lines[i] = html.EscapeString(strings.TrimSpace(lines[i]))
		}
		b.WriteString("<p>")
		b.WriteString(strings.Join(lines, "<br>"))
		b.WriteString("</p>")
	}
	return b.String()
}

// FallbackTitle names an asset whose variant has no title.
func FallbackTitle(f Format, n int) string {
	label := map[Format]string{
		FormatFacebookPost: "Bài Facebook", FormatTikTokScript: "Kịch bản TikTok", FormatCaption: "Caption",
		FormatEmail: "Email", FormatVideoScript: "Kịch bản video",
	}[f]
	if label == "" {
		label = "Nội dung"
	}
	return label + " " + string(rune('A'+(n%26)))
}

// Truncate cuts s to at most max runes.
func Truncate(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max])
}
