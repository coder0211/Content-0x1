// Package domain holds brand entities and pure rules: profile validation
// (hex colors, word lists), versioning, product facts and offer validity,
// brand asset types and music licenses.
package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/be/acs/api/internal/shared/kernel"
)

// Limits keep profiles prompt-sized.
const (
	MaxNameLen      = 200
	MaxTextLen      = 4000
	MaxListItems    = 50
	MaxItemLen      = 300
	MaxColors       = 20
	MaxPersonas     = 20
	MaxFactsEntries = 50
)

var hexColor = regexp.MustCompile(`^#(?:[0-9A-Fa-f]{3}|[0-9A-Fa-f]{6})$`)

// IsHexColor reports whether s is #RGB or #RRGGBB (FR-BRD-01).
func IsHexColor(s string) bool { return hexColor.MatchString(s) }

// Color is a named brand color.
type Color struct {
	Name string `json:"name"`
	Hex  string `json:"hex"`
}

// Typography holds font families.
type Typography struct {
	Heading string `json:"heading"`
	Body    string `json:"body"`
}

// Persona is an audience persona.
type Persona struct {
	Name        string   `json:"name"`
	PainPoints  []string `json:"pain_points"`
	Motivations []string `json:"motivations"`
}

// Profile is brand_versions.profile (SPEC-04 §2.2).
type Profile struct {
	Description      string     `json:"description"`
	Colors           []Color    `json:"colors"`
	Typography       Typography `json:"typography"`
	ToneOfVoice      []string   `json:"tone_of_voice"`
	PreferredWords   []string   `json:"preferred_words"`
	ForbiddenWords   []string   `json:"forbidden_words"`
	DefaultCTAs      []string   `json:"default_ctas"`
	AudiencePersonas []Persona  `json:"audience_personas"`
	Notes            string     `json:"notes"`
}

// cleanList trims, drops empties and case-insensitive duplicates; never nil.
func cleanList(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, s := range in {
		s = strings.TrimSpace(s)
		k := strings.ToLower(s)
		if s == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, s)
	}
	return out
}

// Normalize trims text, de-duplicates lists, upper-cases hex colors and
// replaces nil slices with empty ones (stable JSON for version comparison).
func (p Profile) Normalize() Profile {
	p.Description = strings.TrimSpace(p.Description)
	p.Notes = strings.TrimSpace(p.Notes)
	p.Typography.Heading = strings.TrimSpace(p.Typography.Heading)
	p.Typography.Body = strings.TrimSpace(p.Typography.Body)
	p.ToneOfVoice = cleanList(p.ToneOfVoice)
	p.PreferredWords = cleanList(p.PreferredWords)
	p.ForbiddenWords = cleanList(p.ForbiddenWords)
	p.DefaultCTAs = cleanList(p.DefaultCTAs)
	colors := make([]Color, 0, len(p.Colors))
	for _, c := range p.Colors {
		colors = append(colors, Color{Name: strings.TrimSpace(c.Name), Hex: strings.ToUpper(strings.TrimSpace(c.Hex))})
	}
	p.Colors = colors
	personas := make([]Persona, 0, len(p.AudiencePersonas))
	for _, ps := range p.AudiencePersonas {
		personas = append(personas, Persona{Name: strings.TrimSpace(ps.Name), PainPoints: cleanList(ps.PainPoints), Motivations: cleanList(ps.Motivations)})
	}
	p.AudiencePersonas = personas
	return p
}

func checkList(v *kernel.ValidationErrors, field string, list []string) {
	if len(list) > MaxListItems {
		v.Add(field, "too_many")
	}
	for i, s := range list {
		if utf8.RuneCountInString(s) > MaxItemLen {
			v.Add(fmt.Sprintf("%s[%d]", field, i), "too_long")
		}
	}
}

func checkText(v *kernel.ValidationErrors, field, s string, max int) {
	if utf8.RuneCountInString(s) > max {
		v.Add(field, "too_long")
	}
}

// Validate reports every problem of a normalized profile under prefix
// (e.g. "profile").
func (p Profile) Validate(v *kernel.ValidationErrors, prefix string) {
	f := func(s string) string { return prefix + "." + s }
	checkText(v, f("description"), p.Description, MaxTextLen)
	checkText(v, f("notes"), p.Notes, MaxTextLen)
	checkText(v, f("typography.heading"), p.Typography.Heading, MaxItemLen)
	checkText(v, f("typography.body"), p.Typography.Body, MaxItemLen)
	if len(p.Colors) > MaxColors {
		v.Add(f("colors"), "too_many")
	}
	for i, c := range p.Colors {
		if c.Name == "" {
			v.Add(fmt.Sprintf("%s.colors[%d].name", prefix, i), "required")
		}
		if !IsHexColor(c.Hex) {
			v.Add(fmt.Sprintf("%s.colors[%d].hex", prefix, i), "invalid_hex")
		}
	}
	checkList(v, f("tone_of_voice"), p.ToneOfVoice)
	checkList(v, f("preferred_words"), p.PreferredWords)
	checkList(v, f("forbidden_words"), p.ForbiddenWords)
	checkList(v, f("default_ctas"), p.DefaultCTAs)
	if len(p.AudiencePersonas) > MaxPersonas {
		v.Add(f("audience_personas"), "too_many")
	}
	for i, ps := range p.AudiencePersonas {
		pf := fmt.Sprintf("%s.audience_personas[%d]", prefix, i)
		if ps.Name == "" {
			v.Add(pf+".name", "required")
		}
		checkText(v, pf+".name", ps.Name, MaxItemLen)
		checkList(v, pf+".pain_points", ps.PainPoints)
		checkList(v, pf+".motivations", ps.Motivations)
	}
	for _, w := range p.PreferredWords {
		if slices.ContainsFunc(p.ForbiddenWords, func(x string) bool { return strings.EqualFold(x, w) }) {
			v.AddDetail(kernel.Detail{"field": f("preferred_words"), "issue": "conflicts_with_forbidden_words", "word": w})
		}
	}
}

// Equal reports whether two normalized profiles are the same (no new version).
func (p Profile) Equal(o Profile) bool {
	a, _ := json.Marshal(p)
	b, _ := json.Marshal(o)
	return bytes.Equal(a, b)
}

// ParseProfile decodes a stored profile tolerantly (unknown keys ignored).
func ParseProfile(raw json.RawMessage) Profile {
	var p Profile
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &p)
	}
	return p.Normalize()
}

// ValidateName checks a brand or product name.
func ValidateName(v *kernel.ValidationErrors, field, name string) {
	switch {
	case name == "":
		v.Add(field, "required")
	case utf8.RuneCountInString(name) > MaxNameLen:
		v.Add(field, "too_long")
	}
}

// Brand is a brands row joined with its current version.
type Brand struct {
	ID        kernel.ID
	Name      string
	CreatedBy *kernel.ID
	CreatedAt time.Time
	UpdatedAt time.Time
	Current   Version
}

// Version is an immutable brand_versions row.
type Version struct {
	ID        kernel.ID
	BrandID   kernel.ID
	VersionNo int
	Profile   Profile
	CreatedBy *kernel.ID
	CreatedAt time.Time
}

// NextVersion builds version n+1 of a brand.
func (b *Brand) NextVersion(p Profile, by kernel.ID, at time.Time) Version {
	return Version{ID: kernel.NewID(), BrandID: b.ID, VersionNo: b.Current.VersionNo + 1, Profile: p, CreatedBy: &by, CreatedAt: at}
}

// CheckVersion implements the optimistic lock of PATCH /brands/{id}.
func (b *Brand) CheckVersion(expected int) error {
	if expected != b.Current.VersionNo {
		return kernel.ErrVersionConflict.WithDetails(kernel.Detail{"current_version_no": b.Current.VersionNo, "expected_version_no": expected})
	}
	return nil
}
