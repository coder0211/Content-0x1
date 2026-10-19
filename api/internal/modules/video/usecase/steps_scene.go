package usecase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"time"

	fileuc "github.com/be/acs/api/internal/modules/file/usecase"
	jobuc "github.com/be/acs/api/internal/modules/job/usecase"
	"github.com/be/acs/api/internal/modules/video/domain"
	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
)

// KenBurnsProvider is the internal fallback "provider" (OQ-01): a still
// image (the scene's source image, or a neutral frame) turned into a clip
// with a slow zoom by FFmpeg, at no cost.
const KenBurnsProvider = "kenburns"

// errCancelled stops a step whose job the user cancelled.
var errCancelled = jobuc.Permanent(errors.New("job cancelled"))

func (s *Service) request(sc jobuc.StepContext) productionRequest {
	var r productionRequest
	_ = json.Unmarshal(sc.Job().Request, &r)
	if r.VideoProvider == "" {
		r.VideoProvider, r.VideoModel = sc.Job().ProviderCode, sc.Job().ModelCode
	}
	return r
}

func (s *Service) format(p *domain.Project) ports.OutputFormat {
	return ports.OutputFormat{Width: p.Settings.Width, Height: p.Settings.Height, FPS: domain.FPS}
}

// workDir creates a private per-step directory; the returned func removes it.
func (s *Service) workDir(prefix string) (string, func(), error) {
	base := s.TempDir
	if base != "" {
		if err := os.MkdirAll(base, 0o700); err != nil {
			return "", nil, err
		}
	}
	dir, err := os.MkdirTemp(base, prefix)
	if err != nil {
		return "", nil, err
	}
	return dir, func() { _ = os.RemoveAll(dir) }, nil
}

// download copies a stored file into dir/name (workers only read inputs
// from internal storage: SSRF rule).
func (s *Service) download(ctx context.Context, id kernel.ID, path string) (fileuc.File, error) {
	rc, f, err := s.Files.Open(ctx, id)
	if err != nil {
		return fileuc.File{}, err
	}
	defer rc.Close()
	return f, writeFile(path, rc)
}

func writeFile(path string, r io.Reader) error {
	out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, r); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// store uploads a local file through the file module; in carries the
// metadata (MimeType, Kind, Source, sizes).
func (s *Service) store(ctx context.Context, sc jobuc.StepContext, path string, in fileuc.StoreInput) (fileuc.File, error) {
	f, err := os.Open(path) //nolint:gosec // path is inside our own work dir
	if err != nil {
		return fileuc.File{}, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return fileuc.File{}, err
	}
	by := sc.Job().CreatedBy
	in.Body, in.Size, in.CreatedBy, in.OriginalName = f, st.Size(), &by, filepath.Base(path)
	return s.Files.Store(ctx, in)
}

// RunScene is the scene_generation:<n> step: submit the clip, then poll the
// provider every PollInterval through WaitSpec (never blocking a worker),
// download, normalise to the preset (30 fps, yuv420p, H.264, AAC/silent)
// and store clip_file_id + a thumbnail. A ready scene is never regenerated
// (retry reuses it) unless the step is a forced regeneration that has not
// run yet. One failing scene only fails its own step.
func (s *Service) RunScene(ctx context.Context, sc jobuc.StepContext) (jobuc.StepResult, error) {
	var in domain.SceneStepInput
	if err := sc.DecodeInput(&in); err != nil {
		return jobuc.StepResult{}, jobuc.Permanent(err)
	}
	if in.SceneID == kernel.NilID && sc.SceneID() != nil {
		in.SceneID = *sc.SceneID()
	}
	p, err := s.projectOf(ctx, sc)
	if err != nil {
		return jobuc.StepResult{}, err
	}
	scene, err := s.Repo.GetScene(ctx, in.SceneID)
	if errors.Is(err, kernel.ErrNotFound) || (err == nil && scene.AssetID != p.AssetID) {
		return jobuc.StepResult{}, jobuc.Permanent(fmt.Errorf("scene %s not found", in.SceneID))
	}
	if err != nil {
		return jobuc.StepResult{}, err
	}
	if scene.IsReady() && (!in.Force || scene.AttemptCount > in.BaseAttempt) {
		return jobuc.StepResult{Output: sceneOutput(scene, true)}, nil
	}
	if cancelled, err := sc.CancelRequested(ctx); err == nil && cancelled {
		s.cancelProvider(ctx, sc, scene)
		s.failScene(ctx, scene, "Đã hủy")
		return jobuc.StepResult{}, errCancelled
	}
	req := s.request(sc)
	if req.VideoProvider == KenBurnsProvider {
		return s.stillScene(ctx, sc, p, scene)
	}
	gen, err := s.AI.Video(req.VideoProvider)
	if err != nil {
		return jobuc.StepResult{}, jobuc.Permanent(err)
	}
	pjid := sc.ProviderJobID()
	if pjid == "" || pjid != scene.ProviderJobID {
		return s.submitScene(ctx, sc, gen, req, p, scene)
	}
	st, err := gen.GetStatus(ctx, pjid)
	if err != nil {
		return jobuc.StepResult{}, err
	}
	switch st.State {
	case ports.VideoQueued, ports.VideoRunning:
		_ = sc.Progress(ctx, st.Progress, "")
		return jobuc.StepResult{Wait: &jobuc.WaitSpec{ProviderJobID: pjid, PollAfter: s.pollInterval()}}, nil
	case ports.VideoFailed, ports.VideoCancelled:
		if st.Usage != nil {
			if err := s.billScene(ctx, sc, req, scene, pjid, *st.Usage); err != nil {
				return jobuc.StepResult{}, err
			}
		}
		perr := st.Error
		if perr == nil {
			perr = &ports.ProviderError{Kind: ports.ErrKindUnknown, Provider: req.VideoProvider, Code: string(st.State), Message: "clip generation " + string(st.State)}
		}
		// The submission is dead: a retry (transient errors only) resubmits.
		scene.ProviderJobID = ""
		s.failScene(ctx, scene, perr.Message)
		if perr.Retryable() {
			return jobuc.StepResult{}, perr
		}
		return jobuc.StepResult{}, jobuc.Permanent(perr)
	}
	usage := st.Usage
	if usage == nil {
		usage = &ports.Usage{VideoSeconds: float64(clipSeconds(scene.DurationMs)),
			Units: map[string]float64{"video_second_" + p.Settings.Resolution: float64(clipSeconds(scene.DurationMs))}}
	}
	if err := s.billScene(ctx, sc, req, scene, pjid, *usage); err != nil {
		return jobuc.StepResult{}, err
	}
	stream, err := gen.Download(ctx, pjid)
	if err != nil {
		return jobuc.StepResult{}, err
	}
	defer stream.Body.Close()
	return s.finishScene(ctx, sc, p, scene, req, stream.Body)
}

func sceneOutput(sc *domain.Scene, reused bool) map[string]any {
	out := map[string]any{"scene_id": sc.ID.String(), "reused": reused}
	if sc.ClipFileID != nil {
		out["clip_file_id"] = sc.ClipFileID.String()
	}
	if sc.ThumbnailFileID != nil {
		out["thumbnail_file_id"] = sc.ThumbnailFileID.String()
	}
	return out
}

func (s *Service) submitScene(ctx context.Context, sc jobuc.StepContext, gen ports.VideoGenerator, req productionRequest,
	p *domain.Project, scene *domain.Scene) (jobuc.StepResult, error) {
	vr := ports.VideoRequest{
		Model: req.VideoModel, Prompt: scene.VisualPrompt, AspectRatio: p.Settings.AspectRatio,
		Resolution: p.Settings.Resolution, DurationSec: clipSeconds(scene.DurationMs),
	}
	if scene.SourceFileID != nil {
		rc, f, err := s.Files.Open(ctx, *scene.SourceFileID)
		if err != nil {
			return jobuc.StepResult{}, err
		}
		data, err := io.ReadAll(io.LimitReader(rc, 25<<20))
		_ = rc.Close()
		if err != nil {
			return jobuc.StepResult{}, err
		}
		vr.Image = &ports.MediaInput{MimeType: f.MimeType, Data: data}
	}
	sub, err := gen.Submit(ctx, vr)
	if err != nil {
		if pe, ok := ports.AsProviderError(err); ok && !pe.Retryable() {
			s.failScene(ctx, scene, pe.Message)
		}
		return jobuc.StepResult{}, err
	}
	scene.Status = domain.SceneGenerating
	scene.AttemptCount++
	scene.LastError = ""
	scene.ProviderJobID = sub.ProviderJobID
	scene.UpdatedAt = s.Clock.Now()
	if err := s.Repo.UpdateScene(ctx, scene); err != nil {
		return jobuc.StepResult{}, err
	}
	_ = sc.SaveOutput(ctx, map[string]any{"scene_id": scene.ID.String(), "provider_job_id": sub.ProviderJobID})
	return jobuc.StepResult{Wait: &jobuc.WaitSpec{ProviderJobID: sub.ProviderJobID, PollAfter: max(sub.PollAfter, s.pollInterval())}}, nil
}

// billScene records the clip usage once per submission (a retried poll or
// download never bills twice).
func (s *Service) billScene(ctx context.Context, sc jobuc.StepContext, req productionRequest, scene *domain.Scene, pjid string, u ports.Usage) error {
	if scene.UsageRecordedFor == pjid {
		return nil
	}
	if err := s.recordUsage(ctx, sc, req.VideoProvider, req.VideoModel, u, false); err != nil {
		return err
	}
	scene.UsageRecordedFor = pjid
	return s.Repo.UpdateScene(ctx, scene)
}

func (s *Service) cancelProvider(ctx context.Context, sc jobuc.StepContext, scene *domain.Scene) {
	if scene.ProviderJobID == "" {
		return
	}
	if gen, err := s.AI.Video(s.request(sc).VideoProvider); err == nil {
		if err := gen.Cancel(ctx, scene.ProviderJobID); err != nil && !errors.Is(err, ports.ErrCancelUnsupported) {
			s.log().WarnContext(ctx, "video: provider cancel failed", slog.Any("err", err))
		}
	}
}

func (s *Service) failScene(ctx context.Context, scene *domain.Scene, msg string) {
	scene.Status = domain.SceneFailed
	if len([]rune(msg)) > 500 {
		msg = string([]rune(msg)[:500])
	}
	scene.LastError = msg
	scene.UpdatedAt = s.Clock.Now()
	if err := s.Repo.UpdateScene(ctx, scene); err != nil {
		s.log().ErrorContext(ctx, "video: mark scene failed", slog.Any("err", err))
	}
}

// finishScene normalises the downloaded clip and stores it.
func (s *Service) finishScene(ctx context.Context, sc jobuc.StepContext, p *domain.Project, scene *domain.Scene, req productionRequest, body io.Reader) (jobuc.StepResult, error) {
	dir, cleanup, err := s.workDir("scene-*")
	if err != nil {
		return jobuc.StepResult{}, err
	}
	defer cleanup()
	raw := filepath.Join(dir, "raw.mp4")
	if err := writeFile(raw, body); err != nil {
		return jobuc.StepResult{}, err
	}
	clip := filepath.Join(dir, "clip.mp4")
	err = s.Renderer.Normalize(ctx, ports.NormalizeSpec{Input: raw, Output: clip, Format: s.format(p), DurationMs: scene.DurationMs})
	if err != nil {
		if !slices.Contains(s.StubVideoProviders, req.VideoProvider) {
			return jobuc.StepResult{}, err
		}
		// Stub provider bytes are not real media: use the Ken Burns clip.
		return s.stillScene(ctx, sc, p, scene)
	}
	return s.storeScene(ctx, sc, p, scene, dir, clip)
}

// stillScene renders the Ken Burns fallback clip for a scene.
func (s *Service) stillScene(ctx context.Context, sc jobuc.StepContext, p *domain.Project, scene *domain.Scene) (jobuc.StepResult, error) {
	dir, cleanup, err := s.workDir("still-*")
	if err != nil {
		return jobuc.StepResult{}, err
	}
	defer cleanup()
	img := filepath.Join(dir, "still.png")
	if scene.SourceFileID != nil {
		if _, err := s.download(ctx, *scene.SourceFileID, img); err != nil {
			return jobuc.StepResult{}, err
		}
	} else if err := os.WriteFile(img, placeholderFrame(scene.SequenceNo), 0o600); err != nil {
		return jobuc.StepResult{}, err
	}
	if scene.Status != domain.SceneGenerating {
		scene.Status = domain.SceneGenerating
		scene.AttemptCount++
	}
	clip := filepath.Join(dir, "clip.mp4")
	if err := s.Renderer.StillToClip(ctx, ports.StillClipSpec{Image: img, Output: clip, Format: s.format(p), DurationMs: scene.DurationMs}); err != nil {
		return jobuc.StepResult{}, err
	}
	return s.storeScene(ctx, sc, p, scene, dir, clip)
}

func (s *Service) storeScene(ctx context.Context, sc jobuc.StepContext, p *domain.Project, scene *domain.Scene, dir, clip string) (jobuc.StepResult, error) {
	f := s.format(p)
	stored, err := s.store(ctx, sc, clip, fileuc.StoreInput{MimeType: "video/mp4", Kind: "video", Source: "generated",
		Width: f.Width, Height: f.Height, DurationMs: scene.DurationMs})
	if err != nil {
		return jobuc.StepResult{}, err
	}
	scene.ClipFileID = &stored.ID
	thumb := filepath.Join(dir, "thumb.jpg")
	if err := s.Renderer.Thumbnail(ctx, clip, thumb, time.Duration(scene.DurationMs/2)*time.Millisecond); err == nil {
		if t, err := s.store(ctx, sc, thumb, fileuc.StoreInput{MimeType: "image/jpeg", Kind: "image", Source: "generated", ParentFileID: &stored.ID}); err == nil {
			scene.ThumbnailFileID = &t.ID
		}
	} else {
		s.log().WarnContext(ctx, "video: scene thumbnail failed", slog.Any("err", err))
	}
	scene.Status = domain.SceneReady
	scene.LastError = ""
	scene.UpdatedAt = s.Clock.Now()
	if err := s.Repo.UpdateScene(ctx, scene); err != nil {
		return jobuc.StepResult{}, err
	}
	return jobuc.StepResult{Output: sceneOutput(scene, false)}, nil
}

// placeholderFrame is a neutral 16:9 frame with a per-scene tint, used by
// the Ken Burns fallback when the scene has no source image.
func placeholderFrame(seq int) []byte {
	palette := []color.RGBA{{38, 70, 83, 255}, {42, 157, 143, 255}, {233, 196, 106, 255}, {244, 162, 97, 255}, {231, 111, 81, 255}}
	c := palette[(max(seq, 1)-1)%len(palette)]
	img := image.NewRGBA(image.Rect(0, 0, 640, 640))
	for y := 0; y < 640; y++ {
		for x := 0; x < 640; x++ {
			shade := uint8((x + y) / 20)
			img.Set(x, y, color.RGBA{R: c.R - min(c.R, shade), G: c.G - min(c.G, shade), B: c.B - min(c.B, shade), A: 255})
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}
