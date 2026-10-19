package domain

import (
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/be/acs/api/internal/shared/kernel"
)

// Brand asset types (brand_assets.asset_type).
const (
	AssetLogo           = "logo"
	AssetReferenceImage = "reference_image"
	AssetFont           = "font"
	AssetDocument       = "document"
	AssetSampleContent  = "sample_content"
	AssetMusic          = "music"
)

// AssetTypes lists every accepted asset type.
var AssetTypes = []string{AssetLogo, AssetReferenceImage, AssetFont, AssetDocument, AssetSampleContent, AssetMusic}

// IsAssetType reports whether t is a known asset type.
func IsAssetType(t string) bool { return slices.Contains(AssetTypes, t) }

// License is the license of a music track (OQ-04): source and license type
// are mandatory; ExpiresAt (YYYY-MM-DD, inclusive) is optional.
type License struct {
	Source      string `json:"source"`
	LicenseType string `json:"license_type"`
	ExpiresAt   string `json:"expires_at,omitempty"`
	Attribution string `json:"attribution,omitempty"`
	Notes       string `json:"notes,omitempty"`
}

// ParseLicense decodes a stored license; ok is false when absent/invalid.
func ParseLicense(raw json.RawMessage) (License, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return License{}, false
	}
	var l License
	if err := json.Unmarshal(raw, &l); err != nil {
		return License{}, false
	}
	return l, true
}

// Normalize trims every field.
func (l License) Normalize() License {
	l.Source = strings.TrimSpace(l.Source)
	l.LicenseType = strings.TrimSpace(l.LicenseType)
	l.ExpiresAt = strings.TrimSpace(l.ExpiresAt)
	l.Attribution = strings.TrimSpace(l.Attribution)
	l.Notes = strings.TrimSpace(l.Notes)
	return l
}

// Validate reports problems under prefix (e.g. "license").
func (l License) Validate(v *kernel.ValidationErrors, prefix string) {
	if l.Source == "" {
		v.Add(prefix+".source", "required")
	}
	if l.LicenseType == "" {
		v.Add(prefix+".license_type", "required")
	}
	if l.ExpiresAt != "" && !ValidDate(l.ExpiresAt) {
		v.Add(prefix+".expires_at", "invalid_date")
	}
	checkText(v, prefix+".notes", l.Notes, MaxTextLen)
}

// UsableAt reports whether the license is complete and not expired at t.
func (l License) UsableAt(t time.Time) bool {
	return l.Source != "" && l.LicenseType != "" && (l.ExpiresAt == "" || l.ExpiresAt >= Today(t))
}

// Asset is a brand_assets row.
type Asset struct {
	ID        kernel.ID
	BrandID   kernel.ID
	FileID    kernel.ID
	AssetType string
	Label     string
	License   json.RawMessage // nil unless provided (always set for music)
	CreatedBy *kernel.ID
	CreatedAt time.Time
}

// LicenseUsableAt reports whether a music asset may be used in a video.
func (a Asset) LicenseUsableAt(t time.Time) bool {
	l, ok := ParseLicense(a.License)
	return ok && l.Normalize().UsableAt(t)
}
