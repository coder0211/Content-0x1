package usecase

import (
	"encoding/json"
	"strings"

	"github.com/be/acs/api/internal/modules/content/domain"
	"github.com/be/acs/api/internal/shared/kernel"
)

// ContentJSON is asset_versions.content_json for asset_type=content
// (migration 0006 comment + SPEC-04 §2.4). Extra keys (format, language,
// scenes, provenance) are additive.
type ContentJSON struct {
	Title      string        `json:"title"`
	Hook       string        `json:"hook"`
	BodyHTML   string        `json:"body_html"`
	BodyText   string        `json:"body_text"`
	CTA        string        `json:"cta"`
	Hashtags   []string      `json:"hashtags"`
	Platform   string        `json:"platform"`
	Format     string        `json:"format,omitempty"`
	Language   string        `json:"language,omitempty"`
	Scenes     []SceneJSON   `json:"scenes,omitempty"`
	Warnings   []WarningJSON `json:"warnings"`
	Provenance *Provenance   `json:"provenance,omitempty"`
}

// SceneJSON is a video-script scene.
type SceneJSON struct {
	Narration   string `json:"narration"`
	VisualHint  string `json:"visual_hint"`
	DurationSec int    `json:"duration_sec"`
}

// WarningJSON is a post-generation check result.
type WarningJSON struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Provenance records how a version was generated (FR-CNT-08: provider,
// model, prompt template + version, brand version) and the facts context
// reused by rewrites and checks.
type Provenance struct {
	JobID                 kernel.ID   `json:"job_id"`
	ProviderCode          string      `json:"provider_code"`
	ModelCode             string      `json:"model_code"`
	PromptTemplateCode    string      `json:"prompt_template_code"`
	PromptTemplateVersion int         `json:"prompt_template_version"`
	PromptTemplateID      *kernel.ID  `json:"prompt_template_id"`
	BrandVersionID        kernel.ID   `json:"brand_version_id"`
	BrandVersionNo        int         `json:"brand_version_no"`
	ProductIDs            []kernel.ID `json:"product_ids"`
	Brief                 *BriefJSON  `json:"brief,omitempty"`
}

// BriefJSON is the brief stored with the generated content.
type BriefJSON struct {
	TargetAudience string `json:"target_audience,omitempty"`
	KeyMessage     string `json:"key_message,omitempty"`
	CTA            string `json:"cta,omitempty"`
	VerifiedOffer  string `json:"verified_offer,omitempty"`
	Notes          string `json:"notes,omitempty"`
}

func (b *BriefJSON) texts() []string {
	if b == nil {
		return nil
	}
	return []string{b.TargetAudience, b.KeyMessage, b.CTA, b.VerifiedOffer, b.Notes}
}

func warningsJSON(ws []domain.Warning) []WarningJSON {
	out := make([]WarningJSON, 0, len(ws))
	for _, w := range ws {
		out = append(out, WarningJSON{Type: string(w.Type), Text: w.Text})
	}
	return out
}

// toContentJSON renders a variant into a version payload.
func toContentJSON(v domain.Variant, format, platform, language string, ws []domain.Warning, prov *Provenance) ContentJSON {
	scenes := make([]SceneJSON, 0, len(v.Scenes))
	for _, s := range v.Scenes {
		scenes = append(scenes, SceneJSON{Narration: s.Narration, VisualHint: s.VisualHint, DurationSec: s.DurationSec})
	}
	tags := v.Hashtags
	if tags == nil {
		tags = []string{}
	}
	return ContentJSON{
		Title: v.Title, Hook: v.Hook, BodyHTML: domain.RenderHTML(v.Body), BodyText: v.Body, CTA: v.CTA,
		Hashtags: tags, Platform: platform, Format: format, Language: language, Scenes: scenes,
		Warnings: warningsJSON(ws), Provenance: prov,
	}
}

// variant converts stored content back to a domain variant.
func (c ContentJSON) variant() domain.Variant {
	v := domain.Variant{Title: c.Title, Hook: c.Hook, Body: c.BodyText, CTA: c.CTA, Hashtags: c.Hashtags}
	if v.Body == "" && c.BodyHTML != "" {
		v.Body = stripTags(c.BodyHTML)
	}
	for _, s := range c.Scenes {
		v.Scenes = append(v.Scenes, domain.Scene{Narration: s.Narration, VisualHint: s.VisualHint, DurationSec: s.DurationSec})
	}
	return v
}

func parseContentJSON(raw json.RawMessage) (ContentJSON, error) {
	var c ContentJSON
	if len(raw) == 0 {
		return c, nil
	}
	err := json.Unmarshal(raw, &c)
	return c, err
}

// stripTags is a minimal HTML → text conversion for editor output
// (<p>, <br>) when body_text is missing.
func stripTags(s string) string {
	s = strings.NewReplacer("</p>", "\n\n", "<br>", "\n", "<br/>", "\n", "<br />", "\n").Replace(s)
	var b strings.Builder
	in := false
	for _, r := range s {
		switch {
		case r == '<':
			in = true
		case r == '>':
			in = false
		case !in:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(strings.NewReplacer("&lt;", "<", "&gt;", ">", "&amp;", "&", "&#34;", `"`, "&#39;", "'", "&quot;", `"`).Replace(b.String()))
}

// variantJSON is the provider output shape of one variant.
type variantJSON struct {
	Title    string      `json:"title"`
	Hook     string      `json:"hook"`
	Body     string      `json:"body"`
	CTA      string      `json:"cta"`
	Hashtags []string    `json:"hashtags"`
	Scenes   []SceneJSON `json:"scenes"`
}

func (v variantJSON) domain() domain.Variant {
	d := domain.Variant{Title: v.Title, Hook: v.Hook, Body: v.Body, CTA: v.CTA, Hashtags: v.Hashtags}
	for _, s := range v.Scenes {
		d.Scenes = append(d.Scenes, domain.Scene{Narration: s.Narration, VisualHint: s.VisualHint, DurationSec: s.DurationSec})
	}
	return d.Normalize()
}

func variantFromDomain(v domain.Variant) variantJSON {
	out := variantJSON{Title: v.Title, Hook: v.Hook, Body: v.Body, CTA: v.CTA, Hashtags: v.Hashtags}
	for _, s := range v.Scenes {
		out.Scenes = append(out.Scenes, SceneJSON{Narration: s.Narration, VisualHint: s.VisualHint, DurationSec: s.DurationSec})
	}
	return out
}
