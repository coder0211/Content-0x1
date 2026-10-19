package usecase

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"time"

	assetuc "github.com/be/acs/api/internal/modules/asset/usecase"
	branduc "github.com/be/acs/api/internal/modules/brand/usecase"
	budgetuc "github.com/be/acs/api/internal/modules/budget/usecase"
	campaignuc "github.com/be/acs/api/internal/modules/campaign/usecase"
	contentuc "github.com/be/acs/api/internal/modules/content/usecase"
	fileuc "github.com/be/acs/api/internal/modules/file/usecase"
	jobuc "github.com/be/acs/api/internal/modules/job/usecase"
	provideruc "github.com/be/acs/api/internal/modules/provider/usecase"
	"github.com/be/acs/api/internal/modules/video/domain"
	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Error codes specific to Video Studio (registered with httpx by the HTTP
// adapter).
const (
	CodeMusicNotLicensed = "MUSIC_NOT_LICENSED"
	CodeJobInProgress    = "VIDEO_JOB_IN_PROGRESS"
	CodeQualityCheck     = "VIDEO_QUALITY_CHECK_FAILED"
)

var (
	// ErrMusicNotLicensed refuses music without a valid license (OQ-04).
	ErrMusicNotLicensed = kernel.NewError(CodeMusicNotLicensed, "Nhạc nền chưa có giấy phép hợp lệ, không thể dùng trong video")
	// ErrJobInProgress refuses edits/new jobs while a job runs for the project.
	ErrJobInProgress = kernel.NewError(CodeJobInProgress, "Video đang có tác vụ chạy, vui lòng chờ hoàn tất")
	// ErrQualityCheck is the failure of quality_check after one re-render.
	ErrQualityCheck = kernel.NewError(CodeQualityCheck, "Video render không đạt kiểm tra chất lượng")
)

// DefaultPollInterval is how long a scene step waits between provider
// status polls (SPEC-02 §5.3: poll every 10s).
const DefaultPollInterval = 10 * time.Second

// Service implements the Video Studio use cases and step logic.
type Service struct {
	Tx        ports.TxManager
	Repo      Repository
	Clock     ports.Clock
	Audit     ports.AuditRecorder
	Jobs      jobuc.API
	Budget    budgetuc.API
	Assets    assetuc.API
	Brands    branduc.API
	Campaigns campaignuc.API
	Providers provideruc.API
	Content   contentuc.API
	Files     fileuc.API
	AI        ports.AIProviders
	Renderer  ports.VideoRenderer
	Prober    ports.MediaProber
	Logger    *slog.Logger

	// TempDir hosts per-step work directories (removed after each step).
	TempDir string
	// FontsDir holds the subtitle fonts (Vietnamese glyphs); optional.
	FontsDir string
	// PollInterval between provider status polls (default 10s).
	PollInterval time.Duration
	// StubVideoProviders produce placeholder bytes instead of real clips
	// (the "fake" provider): their clips are replaced by a still clip when
	// normalization fails, so local/CI runs produce a playable video.
	StubVideoProviders []string
}

func (s *Service) log() *slog.Logger {
	if s.Logger == nil {
		return slog.Default()
	}
	return s.Logger
}

func (s *Service) pollInterval() time.Duration {
	if s.PollInterval <= 0 {
		return DefaultPollInterval
	}
	return s.PollInterval
}

// loadViewable returns the project when actor may see it (owner, campaign
// member, Lead/Admin, reviewer); NOT_FOUND otherwise (IDOR guard).
func (s *Service) loadViewable(ctx context.Context, actor kernel.Actor, id kernel.ID) (*domain.Project, error) {
	if err := actor.Require(kernel.PermBrandView); err != nil {
		return nil, err
	}
	p, err := s.Repo.GetProject(ctx, id)
	if err != nil {
		return nil, err
	}
	if actor.Can(kernel.PermAssetReview) || actor.IsLeadOrAdmin() {
		return p, nil
	}
	if err := s.Assets.CanEdit(ctx, actor, id); err != nil {
		return nil, kernel.ErrNotFound
	}
	return p, nil
}

// loadEditable returns the project when actor may change it (generate.run
// + owner / campaign member / Lead / Admin). Projects the actor cannot even
// see are NOT_FOUND; visible but read-only ones are FORBIDDEN.
func (s *Service) loadEditable(ctx context.Context, actor kernel.Actor, id kernel.ID) (*domain.Project, error) {
	if err := actor.Require(kernel.PermGenerateRun); err != nil {
		return nil, err
	}
	p, err := s.Repo.GetProject(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.Assets.CanEdit(ctx, actor, id); err != nil {
		if errors.Is(err, kernel.ErrForbidden) && (actor.Can(kernel.PermAssetReview) || actor.IsLeadOrAdmin()) {
			return nil, kernel.ErrForbidden
		}
		if errors.Is(err, kernel.ErrForbidden) {
			return nil, kernel.ErrNotFound
		}
		return nil, err
	}
	return p, nil
}

// activeJob returns the project's current job when it is still running.
func (s *Service) activeJob(ctx context.Context, p *domain.Project) (*jobuc.JobView, error) {
	if p.CurrentJobID == nil {
		return nil, nil
	}
	j, err := s.Jobs.Get(ctx, *p.CurrentJobID)
	if errors.Is(err, kernel.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if j.Status.IsTerminal() {
		return nil, nil
	}
	return &j, nil
}

func (s *Service) requireIdle(ctx context.Context, p *domain.Project) error {
	j, err := s.activeJob(ctx, p)
	if err != nil {
		return err
	}
	if j != nil {
		return ErrJobInProgress.WithDetails(kernel.Detail{"job_id": j.ID.String(), "job_type": string(j.Type)})
	}
	return nil
}

// checkMusic validates a music selection: the brand asset must be music of
// the project's brand with a valid license (OQ-04). Unlicensed → 422.
func (s *Service) checkMusic(ctx context.Context, brandID kernel.ID, m domain.MusicSettings) error {
	if !m.Enabled {
		return nil
	}
	if m.TrackBrandAssetID == nil {
		return kernel.Validation(kernel.FieldIssue("music.track_brand_asset_id", "required"))
	}
	ba, err := s.Brands.GetBrandAsset(ctx, *m.TrackBrandAssetID)
	if errors.Is(err, kernel.ErrNotFound) || (err == nil && ba.BrandID != brandID) {
		return kernel.Validation(kernel.FieldIssue("music.track_brand_asset_id", "not_found"))
	}
	if err != nil {
		return err
	}
	if !domain.MusicLicensed(ba.AssetType, ba.License, s.Clock.Now()) {
		return ErrMusicNotLicensed.WithDetails(kernel.FieldIssue("music.track_brand_asset_id", "unlicensed"))
	}
	return nil
}

// checkSourceFiles verifies that each file is a ready image the actor may
// use (own upload, or Lead/Admin).
func (s *Service) checkSourceFiles(ctx context.Context, actor kernel.Actor, field string, ids []kernel.ID) error {
	for i, id := range ids {
		f, err := s.Files.RequireReady(ctx, id, "image")
		if err != nil {
			if de, ok := kernel.AsDomainError(err); ok && de.Code == kernel.CodeUnsupportedMediaType {
				return err
			}
			return kernel.Validation(kernel.FieldIssue(fieldIndex(field, i, len(ids)), "not_found"))
		}
		if f.CreatedBy != nil && *f.CreatedBy != actor.UserID && !actor.IsLeadOrAdmin() {
			return kernel.Validation(kernel.FieldIssue(fieldIndex(field, i, len(ids)), "not_found"))
		}
	}
	return nil
}

func fieldIndex(field string, i, n int) string {
	if n <= 1 && field != "source.file_ids" {
		return field
	}
	return field + "[" + strconv.Itoa(i) + "]"
}
