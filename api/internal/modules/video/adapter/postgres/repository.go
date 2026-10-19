package postgres

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/be/acs/api/internal/modules/video/domain"
	"github.com/be/acs/api/internal/modules/video/usecase"
	db "github.com/be/acs/api/internal/platform/postgres"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Repository implements usecase.Repository on video_projects/video_scenes.
type Repository struct{ pool *pgxpool.Pool }

// NewRepository returns a repository on pool.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

var _ usecase.Repository = (*Repository)(nil)

func (r *Repository) q(ctx context.Context) *Queries { return New(db.Conn(ctx, r.pool)) }

func str(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// jsonOrNull marshals v, mapping nil pointers to SQL NULL.
func jsonOrNull[T any](v *T) (json.RawMessage, error) {
	if v == nil {
		return nil, nil
	}
	return json.Marshal(v)
}

func decodeOpt[T any](raw json.RawMessage) (*T, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

func toProject(m VideoProject) (domain.Project, error) {
	p := domain.Project{
		AssetID: m.AssetID, BrandID: m.BrandID, CampaignID: m.CampaignID, Title: m.Title, Prompt: deref(m.Prompt),
		Script: deref(m.Script), CurrentJobID: m.CurrentJobID, CreatedBy: m.CreatedBy, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
	if err := json.Unmarshal(m.Settings, &p.Settings); err != nil {
		return p, err
	}
	var err error
	if p.Voice, err = decodeOpt[domain.VoiceTrack](m.VoiceJson); err != nil {
		return p, err
	}
	if p.Subtitle, err = decodeOpt[domain.SubtitleTrack](m.SubtitleJson); err != nil {
		return p, err
	}
	if p.Output, err = decodeOpt[domain.Output](m.OutputJson); err != nil {
		return p, err
	}
	return p, nil
}

type projectJSON struct{ settings, voice, subtitle, output json.RawMessage }

func encodeProject(p *domain.Project) (projectJSON, error) {
	var out projectJSON
	var err error
	if out.settings, err = json.Marshal(p.Settings); err != nil {
		return out, err
	}
	if out.voice, err = jsonOrNull(p.Voice); err != nil {
		return out, err
	}
	if out.subtitle, err = jsonOrNull(p.Subtitle); err != nil {
		return out, err
	}
	out.output, err = jsonOrNull(p.Output)
	return out, err
}

// CreateProject inserts a project.
func (r *Repository) CreateProject(ctx context.Context, p *domain.Project) error {
	j, err := encodeProject(p)
	if err != nil {
		return err
	}
	return r.q(ctx).CreateProject(ctx, CreateProjectParams{
		AssetID: p.AssetID, BrandID: p.BrandID, CampaignID: p.CampaignID, Title: p.Title, Prompt: str(p.Prompt), Script: str(p.Script),
		Settings: j.settings, CurrentJobID: p.CurrentJobID, VoiceJson: j.voice, SubtitleJson: j.subtitle, OutputJson: j.output,
		CreatedBy: p.CreatedBy, CreatedAt: p.CreatedAt,
	})
}

// GetProject loads a project.
func (r *Repository) GetProject(ctx context.Context, id kernel.ID) (*domain.Project, error) {
	m, err := r.q(ctx).GetProject(ctx, id)
	if err != nil {
		return nil, db.MapNotFound(err)
	}
	p, err := toProject(m)
	return &p, err
}

// LockProject loads a project FOR UPDATE (inside a transaction).
func (r *Repository) LockProject(ctx context.Context, id kernel.ID) (*domain.Project, error) {
	m, err := r.q(ctx).GetProjectForUpdate(ctx, id)
	if err != nil {
		return nil, db.MapNotFound(err)
	}
	p, err := toProject(m)
	return &p, err
}

// UpdateProject writes the mutable fields.
func (r *Repository) UpdateProject(ctx context.Context, p *domain.Project) error {
	j, err := encodeProject(p)
	if err != nil {
		return err
	}
	return r.q(ctx).UpdateProject(ctx, UpdateProjectParams{
		AssetID: p.AssetID, Title: p.Title, Prompt: str(p.Prompt), Script: str(p.Script), Settings: j.settings,
		CurrentJobID: p.CurrentJobID, VoiceJson: j.voice, SubtitleJson: j.subtitle, OutputJson: j.output,
	})
}

// ListProjects lists projects newest first.
func (r *Repository) ListProjects(ctx context.Context, f usecase.ProjectFilter) ([]domain.Project, error) {
	rows, err := r.q(ctx).ListProjects(ctx, ListProjectsParams{
		CreatedBy: f.CreatedBy, CampaignID: f.CampaignID, CursorCreatedAt: f.AfterCreatedAt, CursorID: f.AfterID, Lim: int32(min(max(f.Limit, 1), 101)), //nolint:gosec // bounded
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.Project, 0, len(rows))
	for _, m := range rows {
		p, err := toProject(m)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func toScene(m VideoScene) domain.Scene {
	return domain.Scene{
		ID: m.ID, AssetID: m.AssetID, SequenceNo: int(m.SequenceNo), DurationMs: int(m.DurationMs), VisualPrompt: m.VisualPrompt,
		Narration: deref(m.Narration), SubtitleText: deref(m.SubtitleText), SourceFileID: m.SourceFileID, ClipFileID: m.ClipFileID,
		ThumbnailFileID: m.ThumbnailFileID, Status: domain.SceneStatus(m.Status), AttemptCount: int(m.AttemptCount),
		LastError: deref(m.LastError), ProviderJobID: deref(m.ProviderJobID), UsageRecordedFor: deref(m.UsageRecordedFor),
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

// ListScenes returns live scenes in order.
func (r *Repository) ListScenes(ctx context.Context, assetID kernel.ID) ([]domain.Scene, error) {
	rows, err := r.q(ctx).ListScenes(ctx, assetID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Scene, len(rows))
	for i, m := range rows {
		out[i] = toScene(m)
	}
	return out, nil
}

// GetScene loads a live scene.
func (r *Repository) GetScene(ctx context.Context, id kernel.ID) (*domain.Scene, error) {
	m, err := r.q(ctx).GetScene(ctx, id)
	if err != nil {
		return nil, db.MapNotFound(err)
	}
	s := toScene(m)
	return &s, nil
}

func updateParams(s *domain.Scene) UpdateSceneParams {
	return UpdateSceneParams{
		ID: s.ID, SequenceNo: int32(s.SequenceNo), DurationMs: int32(s.DurationMs), VisualPrompt: s.VisualPrompt, //nolint:gosec // bounded by domain limits
		Narration: str(s.Narration), SubtitleText: str(s.SubtitleText), SourceFileID: s.SourceFileID, ClipFileID: s.ClipFileID,
		Status: string(s.Status), AttemptCount: int32(s.AttemptCount), LastError: str(s.LastError), //nolint:gosec // small counter
		ProviderJobID: str(s.ProviderJobID), UsageRecordedFor: str(s.UsageRecordedFor), ThumbnailFileID: s.ThumbnailFileID,
	}
}

// SaveStoryboard upserts scenes in order and soft-deletes the others. Live
// sequence numbers are first offset so the unique index never collides
// while reordering.
func (r *Repository) SaveStoryboard(ctx context.Context, assetID kernel.ID, scenes []domain.Scene) error {
	q := r.q(ctx)
	existing, err := q.ListScenes(ctx, assetID)
	if err != nil {
		return err
	}
	keep := map[kernel.ID]bool{}
	for _, s := range scenes {
		keep[s.ID] = true
	}
	for _, m := range existing {
		if !keep[m.ID] {
			if err := q.SoftDeleteScene(ctx, m.ID); err != nil {
				return err
			}
		}
	}
	if err := q.OffsetSceneSequences(ctx, assetID); err != nil {
		return err
	}
	known := map[kernel.ID]bool{}
	for _, m := range existing {
		known[m.ID] = true
	}
	for i := range scenes {
		s := scenes[i]
		s.AssetID, s.SequenceNo = assetID, i+1
		if known[s.ID] {
			if err := q.UpdateScene(ctx, updateParams(&s)); err != nil {
				return err
			}
			continue
		}
		p := updateParams(&s)
		if err := q.InsertScene(ctx, InsertSceneParams{
			ID: s.ID, AssetID: assetID, SequenceNo: p.SequenceNo, DurationMs: p.DurationMs, VisualPrompt: s.VisualPrompt,
			Narration: p.Narration, SubtitleText: p.SubtitleText, SourceFileID: s.SourceFileID, ClipFileID: s.ClipFileID,
			Status: p.Status, AttemptCount: p.AttemptCount, LastError: p.LastError, ProviderJobID: p.ProviderJobID,
			UsageRecordedFor: p.UsageRecordedFor, ThumbnailFileID: s.ThumbnailFileID, CreatedAt: s.CreatedAt,
		}); err != nil {
			return err
		}
	}
	return nil
}

// UpdateScene writes one scene.
func (r *Repository) UpdateScene(ctx context.Context, s *domain.Scene) error {
	return r.q(ctx).UpdateScene(ctx, updateParams(s))
}
