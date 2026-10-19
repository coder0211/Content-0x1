package usecase

import (
	"context"
	"strings"

	"github.com/be/acs/api/internal/shared/kernel"
)

// GetVideoScript returns the current version of a content asset as a video
// script (FR-CNT-05, "Gửi sang Video Studio"). Any content format works: a
// video_script asset carries scenes, other formats only text.
func (s *Service) GetVideoScript(ctx context.Context, actor kernel.Actor, assetID kernel.ID) (VideoScript, error) {
	if err := actor.Require(kernel.PermBrandView); err != nil {
		return VideoScript{}, err
	}
	a, c, err := s.sourceContent(ctx, assetID)
	if err != nil {
		return VideoScript{}, err
	}
	v := c.variant()
	script := strings.TrimSpace(v.Body)
	if script == "" {
		parts := make([]string, 0, len(v.Scenes))
		for _, sc := range v.Scenes {
			parts = append(parts, sc.Narration)
		}
		script = strings.Join(parts, "\n")
	}
	scenes := make([]ScriptScene, 0, len(v.Scenes))
	for _, sc := range v.Scenes {
		scenes = append(scenes, ScriptScene{Narration: sc.Narration, VisualHint: sc.VisualHint, DurationHint: sc.DurationSec})
	}
	title := v.Title
	if title == "" {
		title = a.Title
	}
	return VideoScript{
		AssetID: a.ID, VersionNo: a.CurrentVersion.VersionNo, Title: title, Hook: v.Hook, Script: script,
		CTA: v.CTA, Language: languageOf(c), Scenes: scenes,
	}, nil
}
