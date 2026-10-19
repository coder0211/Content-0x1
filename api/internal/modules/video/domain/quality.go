package domain

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ProbeResult is the subset of ffprobe output quality_check needs (the
// worker maps ports.MediaInfo into it).
type ProbeResult struct {
	DurationMs  int
	VideoCodec  string
	Width       int
	Height      int
	PixFmt      string
	HasAudio    bool
	AudioCodec  string
	FormatNames string
}

// Expectation is what the render must produce.
type Expectation struct {
	Width      int
	Height     int
	DurationMs int
}

// CheckOutput returns every quality issue (empty = pass): MP4 container,
// H.264 video at the preset resolution, an AAC audio stream and a duration
// within ±0.5 s of the timeline (FR-VID-06, ACS-067).
func CheckOutput(p ProbeResult, e Expectation) []string {
	var issues []string
	if !strings.Contains(p.FormatNames, "mp4") {
		issues = append(issues, "container: not mp4 ("+p.FormatNames+")")
	}
	if p.VideoCodec != "h264" {
		issues = append(issues, "video codec: "+orNone(p.VideoCodec))
	}
	if p.Width != e.Width || p.Height != e.Height {
		issues = append(issues, fmt.Sprintf("resolution: %dx%d, expected %dx%d", p.Width, p.Height, e.Width, e.Height))
	}
	if p.PixFmt != "" && p.PixFmt != "yuv420p" {
		issues = append(issues, "pixel format: "+p.PixFmt)
	}
	if !p.HasAudio {
		issues = append(issues, "audio stream: missing")
	} else if p.AudioCodec != "aac" {
		issues = append(issues, "audio codec: "+orNone(p.AudioCodec))
	}
	if d := p.DurationMs - e.DurationMs; d > DurationToleranceMs || d < -DurationToleranceMs {
		issues = append(issues, fmt.Sprintf("duration: %d ms, expected %d ±%d ms", p.DurationMs, e.DurationMs, DurationToleranceMs))
	}
	return issues
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}

// musicLicense is the subset of asset_files.license_json the video module
// checks (ACS-023 records source, license type and validity).
type musicLicense struct {
	Source      string `json:"source"`
	LicenseType string `json:"license_type"` // brand module's shape (brand/domain.License)
	Type        string `json:"type"`
	License     string `json:"license"`
	ExpiresAt   string `json:"expires_at"`
	ValidTo     string `json:"valid_to"`
}

// MusicLicensed reports whether a brand music asset may be used at now: it
// must carry a license with a source and a type, and not be expired
// (OQ-04: only licensed tracks; unlicensed music is refused).
func MusicLicensed(assetType string, license json.RawMessage, now time.Time) bool {
	if assetType != "music" || len(license) == 0 {
		return false
	}
	var l musicLicense
	if err := json.Unmarshal(license, &l); err != nil {
		return false
	}
	if strings.TrimSpace(l.Source) == "" || strings.TrimSpace(l.LicenseType+l.Type+l.License) == "" {
		return false
	}
	for _, exp := range []string{l.ExpiresAt, l.ValidTo} {
		if exp == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339, exp)
		if err != nil {
			t, err = time.Parse("2006-01-02", exp)
			if err == nil {
				t = t.Add(24 * time.Hour)
			}
		}
		if err != nil || !now.Before(t) {
			return false
		}
	}
	return true
}
