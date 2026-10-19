package usecase

import (
	"context"
	"fmt"
	"strings"

	"github.com/be/acs/api/internal/modules/video/domain"
	"github.com/be/acs/api/internal/shared/kernel"
)

// SaveScenes replaces the storyboard (add / remove / reorder / edit),
// enforcing OQ-07 limits. Scenes whose picture changes (visual prompt,
// duration, source image) lose their clip and go back to draft; narration
// and subtitle edits keep the clip (only voice/subtitles are redone).
func (s *Service) SaveScenes(ctx context.Context, actor kernel.Actor, assetID kernel.ID, in []domain.SceneDraftInput) (ProjectView, error) {
	p, err := s.loadEditable(ctx, actor, assetID)
	if err != nil {
		return ProjectView{}, err
	}
	for i := range in {
		in[i].VisualPrompt = strings.TrimSpace(in[i].VisualPrompt)
		in[i].Narration = strings.TrimSpace(in[i].Narration)
		in[i].SubtitleText = strings.TrimSpace(in[i].SubtitleText)
	}
	if issues := domain.ValidateStoryboard(in); len(issues) > 0 {
		return ProjectView{}, kernel.Validation(issues...)
	}
	var sources []kernel.ID
	for _, d := range in {
		if d.SourceFileID != nil && !containsID(p.Settings.Source.FileIDs, *d.SourceFileID) {
			sources = append(sources, *d.SourceFileID)
		}
	}
	if err := s.checkSourceFiles(ctx, actor, "scenes.source_file_id", sources); err != nil {
		return ProjectView{}, err
	}
	err = s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		p, err = s.Repo.LockProject(ctx, assetID)
		if err != nil {
			return err
		}
		if err := s.requireIdle(ctx, p); err != nil {
			return err
		}
		existing, err := s.Repo.ListScenes(ctx, assetID)
		if err != nil {
			return err
		}
		byID := make(map[kernel.ID]domain.Scene, len(existing))
		for _, sc := range existing {
			byID[sc.ID] = sc
		}
		now := s.Clock.Now()
		next := make([]domain.Scene, 0, len(in))
		seen := map[kernel.ID]bool{}
		for i, d := range in {
			var sc domain.Scene
			if d.ID != nil {
				old, ok := byID[*d.ID]
				if !ok || seen[*d.ID] {
					return kernel.Validation(kernel.FieldIssue(fmt.Sprintf("scenes[%d].id", i), "not_found"))
				}
				seen[*d.ID] = true
				sc = old
				if old.DurationMs != d.DurationMs || old.VisualPrompt != d.VisualPrompt || !sameID(old.SourceFileID, d.SourceFileID) {
					sc.Status, sc.ClipFileID, sc.ThumbnailFileID, sc.LastError, sc.ProviderJobID = domain.SceneDraft, nil, nil, "", ""
				}
			} else {
				sc = domain.Scene{ID: kernel.NewID(), AssetID: assetID, Status: domain.SceneDraft, CreatedAt: now}
			}
			sc.DurationMs, sc.VisualPrompt, sc.Narration, sc.SubtitleText, sc.SourceFileID = d.DurationMs, d.VisualPrompt, d.Narration, d.SubtitleText, d.SourceFileID
			sc.SequenceNo, sc.UpdatedAt = i+1, now
			next = append(next, sc)
		}
		return s.Repo.SaveStoryboard(ctx, assetID, next)
	})
	if err != nil {
		return ProjectView{}, err
	}
	return s.view(ctx, p, true)
}

func containsID(ids []kernel.ID, id kernel.ID) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

func sameID(a, b *kernel.ID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
