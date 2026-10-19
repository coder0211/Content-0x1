// Package usecase is the public contract of the content module, used by
// the video module to turn a content asset into a video script (FR-CNT-05).
package usecase

import (
	"context"

	"github.com/be/acs/api/internal/shared/kernel"
)

// ScriptScene is an optional scene breakdown carried by a video_script asset.
type ScriptScene struct {
	Narration    string
	VisualHint   string
	DurationHint int // seconds, 0 = unspecified
}

// VideoScript is the script extracted from a content asset.
type VideoScript struct {
	AssetID   kernel.ID
	VersionNo int
	Title     string
	Hook      string
	Script    string // plain text narration
	CTA       string
	Language  string
	Scenes    []ScriptScene
}

// API is the content module's public interface.
type API interface {
	// GetVideoScript returns the current version of a content asset as a
	// script. NOT_FOUND when the asset is not content or not visible.
	GetVideoScript(ctx context.Context, actor kernel.Actor, assetID kernel.ID) (VideoScript, error)
}
