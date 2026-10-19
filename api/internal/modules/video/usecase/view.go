package usecase

import (
	"context"
	"errors"
	"log/slog"

	jobuc "github.com/be/acs/api/internal/modules/job/usecase"
	"github.com/be/acs/api/internal/modules/video/domain"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Get returns the project with scenes, output and the current job.
func (s *Service) Get(ctx context.Context, actor kernel.Actor, id kernel.ID) (ProjectView, error) {
	p, err := s.loadViewable(ctx, actor, id)
	if err != nil {
		return ProjectView{}, err
	}
	return s.view(ctx, p, true)
}

// List returns the actor's projects (all projects for Lead/Admin/Reviewer),
// newest first, optionally filtered by campaign.
func (s *Service) List(ctx context.Context, actor kernel.Actor, campaignID *kernel.ID, page kernel.PageRequest) ([]ProjectView, *kernel.Cursor, error) {
	if err := actor.Require(kernel.PermBrandView); err != nil {
		return nil, nil, err
	}
	page = page.Normalized()
	f := ProjectFilter{CampaignID: campaignID, Limit: page.Limit + 1}
	if !actor.IsLeadOrAdmin() && !actor.Can(kernel.PermAssetReview) {
		uid := actor.UserID
		f.CreatedBy = &uid
	}
	var cur kernel.TimeIDCursor
	ok, err := kernel.DecodeCursor(page.Cursor, &cur)
	if err != nil {
		return nil, nil, err
	}
	if ok {
		f.AfterCreatedAt, f.AfterID = &cur.At, &cur.ID
	}
	rows, err := s.Repo.ListProjects(ctx, f)
	if err != nil {
		return nil, nil, err
	}
	pg := kernel.PageFromRows(rows, page.Limit, func(p domain.Project) any {
		return kernel.TimeIDCursor{At: p.CreatedAt, ID: p.AssetID}
	})
	rows, next := pg.Items, pg.NextCursor
	out := make([]ProjectView, 0, len(rows))
	for i := range rows {
		v, err := s.view(ctx, &rows[i], false)
		if err != nil {
			return nil, nil, err
		}
		out = append(out, v)
	}
	return out, next, nil
}

func (s *Service) signed(ctx context.Context, id *kernel.ID) string {
	if id == nil || s.Files == nil {
		return ""
	}
	u, err := s.Files.SignedURL(ctx, *id, "")
	if err != nil {
		s.log().WarnContext(ctx, "video: sign url failed", slog.String("file_id", id.String()), slog.Any("err", err))
		return ""
	}
	return u.URL
}

func (s *Service) view(ctx context.Context, p *domain.Project, withURLs bool) (ProjectView, error) {
	scenes, err := s.Repo.ListScenes(ctx, p.AssetID)
	if err != nil {
		return ProjectView{}, err
	}
	v := ProjectView{Project: *p, Scenes: make([]SceneView, 0, len(scenes)), TotalMs: domain.TotalMs(scenes)}
	if a, err := s.Assets.Get(ctx, p.AssetID); err == nil {
		v.AssetStatus = string(a.Status)
		if a.CurrentVersion != nil {
			v.VersionNo = a.CurrentVersion.VersionNo
		}
	} else if !errors.Is(err, kernel.ErrNotFound) {
		return ProjectView{}, err
	}
	for _, sc := range scenes {
		sv := SceneView{Scene: sc}
		if withURLs {
			sv.PreviewURL = s.signed(ctx, sc.ClipFileID)
			sv.ThumbnailURL = s.signed(ctx, sc.ThumbnailFileID)
		}
		v.Scenes = append(v.Scenes, sv)
	}
	if p.Output != nil {
		o := &OutputView{Output: *p.Output}
		if withURLs {
			o.URL = s.signed(ctx, &p.Output.FileID)
			o.ThumbnailURL = s.signed(ctx, p.Output.ThumbnailFileID)
			o.SubtitleURL = s.signed(ctx, p.Output.SubtitleFileID)
		} else {
			o.ThumbnailURL = s.signed(ctx, p.Output.ThumbnailFileID)
		}
		v.Output = o
	}
	jobType, jobStatus := "", ""
	if p.CurrentJobID != nil {
		j, err := s.Jobs.Get(ctx, *p.CurrentJobID)
		switch {
		case err == nil:
			v.Job = &j
			jobType, jobStatus = string(j.Type), string(j.Status)
			v.Progress = max(j.Progress, domain.Progress(stepStates(j.Steps)))
			if j.Status == jobuc.StatusSucceeded {
				v.Progress = 100
			}
		case !errors.Is(err, kernel.ErrNotFound):
			return ProjectView{}, err
		}
	}
	v.Stage = domain.DeriveStage(jobType, jobStatus, p.Output != nil)
	return v, nil
}

func stepStates(steps []jobuc.StepView) []domain.StepState {
	out := make([]domain.StepState, len(steps))
	for i, st := range steps {
		out[i] = domain.StepState{Type: st.Type, Status: string(st.Status)}
	}
	return out
}
