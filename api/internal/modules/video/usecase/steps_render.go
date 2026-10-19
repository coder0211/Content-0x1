package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	assetuc "github.com/be/acs/api/internal/modules/asset/usecase"
	fileuc "github.com/be/acs/api/internal/modules/file/usecase"
	jobuc "github.com/be/acs/api/internal/modules/job/usecase"
	"github.com/be/acs/api/internal/modules/video/domain"
	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Mix levels of the final soundtrack.
const (
	voiceVolume = 1.0
	musicVolume = 0.25
)

// renderOutput is the output of video_render and quality_check.
type renderOutput struct {
	FileID          kernel.ID  `json:"file_id"`
	ThumbnailFileID *kernel.ID `json:"thumbnail_file_id"`
	SubtitleFileID  *kernel.ID `json:"subtitle_file_id"`
	DurationMs      int        `json:"duration_ms"`
	Width           int        `json:"width"`
	Height          int        `json:"height"`
	Rerendered      bool       `json:"rerendered,omitempty"`
}

// RunRender is the video_render step. The DAG only schedules it after every
// scene, the voice-over and the subtitles succeeded; it re-checks that the
// inputs are complete and fails permanently otherwise.
func (s *Service) RunRender(ctx context.Context, sc jobuc.StepContext) (jobuc.StepResult, error) {
	p, err := s.projectOf(ctx, sc)
	if err != nil {
		return jobuc.StepResult{}, err
	}
	out, err := s.renderOnce(ctx, sc, p)
	if err != nil {
		return jobuc.StepResult{}, err
	}
	return jobuc.StepResult{Output: out}, nil
}

func permanent(msg string) error {
	return jobuc.Permanent(kernel.ErrInvalidTransition.WithMessage(msg))
}

func (s *Service) renderOnce(ctx context.Context, sc jobuc.StepContext, p *domain.Project) (renderOutput, error) {
	scenes, err := s.Repo.ListScenes(ctx, p.AssetID)
	if err != nil {
		return renderOutput{}, err
	}
	if len(scenes) == 0 {
		return renderOutput{}, permanent("Video chưa có cảnh nào")
	}
	for _, scn := range scenes {
		if !scn.IsReady() {
			return renderOutput{}, permanent(fmt.Sprintf("Cảnh %d chưa sẵn sàng", scn.SequenceNo))
		}
	}
	dir, cleanup, err := s.workDir("render-*")
	if err != nil {
		return renderOutput{}, err
	}
	defer cleanup()
	format := s.format(p)
	spec := ports.RenderSpec{WorkDir: dir, Format: format, Output: filepath.Join(dir, "final.mp4"), FontsDir: s.FontsDir}
	for i, scn := range scenes {
		path := filepath.Join(dir, fmt.Sprintf("clip_%02d.mp4", i+1))
		if _, err := s.download(ctx, *scn.ClipFileID, path); err != nil {
			return renderOutput{}, err
		}
		spec.Clips = append(spec.Clips, path)
		spec.ClipDurationsMs = append(spec.ClipDurationsMs, scn.DurationMs)
	}
	if t := domain.TransitionFor(len(scenes)); t > 0 {
		spec.Transition, spec.TransitionMs = "fade", t
	}
	_, total := domain.SceneTimeline(scenes)

	voiceFP := voiceFingerprint(p, scenes)
	if voiceFP != "" {
		if p.Voice == nil || p.Voice.Fingerprint != voiceFP {
			return renderOutput{}, permanent("Giọng đọc chưa được tạo cho storyboard hiện tại")
		}
		for i, seg := range p.Voice.Segments {
			f, err := s.Files.Get(ctx, seg.FileID)
			if err != nil {
				return renderOutput{}, err
			}
			path := filepath.Join(dir, fmt.Sprintf("voice_%02d%s", i, audioExt(f.MimeType)))
			if _, err := s.download(ctx, seg.FileID, path); err != nil {
				return renderOutput{}, err
			}
			spec.VoiceSegments = append(spec.VoiceSegments, ports.VoiceSegment{Path: path, StartMs: seg.StartMs, MaxMs: seg.MaxMs})
		}
		spec.Voice = &ports.AudioTrack{Volume: voiceVolume}
	}
	if m := p.Settings.Music; m.Enabled && m.TrackBrandAssetID != nil {
		ba, err := s.Brands.GetBrandAsset(ctx, *m.TrackBrandAssetID)
		if err != nil {
			return renderOutput{}, jobuc.Permanent(err)
		}
		if ba.BrandID != p.BrandID || !domain.MusicLicensed(ba.AssetType, ba.License, s.Clock.Now()) {
			return renderOutput{}, jobuc.Permanent(ErrMusicNotLicensed)
		}
		f, err := s.Files.Get(ctx, ba.FileID)
		if err != nil {
			return renderOutput{}, err
		}
		path := filepath.Join(dir, "music"+audioExt(f.MimeType))
		if _, err := s.download(ctx, ba.FileID, path); err != nil {
			return renderOutput{}, err
		}
		spec.Music = &ports.AudioTrack{Path: path, Volume: musicVolume, Duck: len(spec.VoiceSegments) > 0}
	}
	var subtitleFile *kernel.ID
	if st := p.Settings.Subtitles; st.Enabled {
		fp := domain.SubtitleFingerprint(scenes, st, voiceFP, p.Settings.Preset)
		if p.Subtitle == nil || p.Subtitle.Fingerprint != fp {
			return renderOutput{}, permanent("Phụ đề chưa được tạo cho storyboard hiện tại")
		}
		if p.Subtitle.CueCount > 0 {
			src, name := p.Subtitle.SRTFileID, "subtitles.srt"
			if st.Style == "burn_in" {
				src, name = p.Subtitle.ASSFileID, "subtitles.ass"
			}
			path := filepath.Join(dir, name)
			if _, err := s.download(ctx, src, path); err != nil {
				return renderOutput{}, err
			}
			spec.Subtitles, spec.BurnIn = path, st.Style == "burn_in"
			id := p.Subtitle.SRTFileID
			subtitleFile = &id
		}
	}
	if _, err := s.Renderer.Render(ctx, spec); err != nil {
		return renderOutput{}, err
	}
	final, err := s.store(ctx, sc, spec.Output, fileuc.StoreInput{MimeType: "video/mp4", Kind: "video", Source: "rendered",
		Width: format.Width, Height: format.Height, DurationMs: total})
	if err != nil {
		return renderOutput{}, err
	}
	out := renderOutput{FileID: final.ID, SubtitleFileID: subtitleFile, DurationMs: total, Width: format.Width, Height: format.Height}
	thumb := filepath.Join(dir, "thumbnail.jpg")
	at := time.Duration(min(1000, total/2)) * time.Millisecond
	if err := s.Renderer.Thumbnail(ctx, spec.Output, thumb, at); err == nil {
		if t, err := s.store(ctx, sc, thumb, fileuc.StoreInput{MimeType: "image/jpeg", Kind: "image", Source: "rendered", ParentFileID: &final.ID,
			Width: format.Width, Height: format.Height}); err == nil {
			out.ThumbnailFileID = &t.ID
		}
	} else {
		s.log().WarnContext(ctx, "video: thumbnail failed", slog.Any("err", err))
	}
	return out, nil
}

func depOutput(sc jobuc.StepContext, key string, v any) error {
	raw, ok := sc.DependencyOutputs()[key]
	if !ok || len(raw) == 0 {
		return jobuc.Permanent(fmt.Errorf("missing output of %s", key))
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return jobuc.Permanent(err)
	}
	return nil
}

// probe downloads a stored video and inspects it with ffprobe.
func (s *Service) probe(ctx context.Context, id kernel.ID) (domain.ProbeResult, error) {
	dir, cleanup, err := s.workDir("qc-*")
	if err != nil {
		return domain.ProbeResult{}, err
	}
	defer cleanup()
	path := filepath.Join(dir, "final.mp4")
	if _, err := s.download(ctx, id, path); err != nil {
		return domain.ProbeResult{}, err
	}
	info, err := s.Prober.Probe(ctx, path)
	if err != nil {
		// Unreadable media is a quality failure, not an infrastructure one.
		return domain.ProbeResult{FormatNames: "unreadable"}, nil
	}
	r := domain.ProbeResult{DurationMs: int(info.Duration / time.Millisecond), FormatNames: info.FormatName}
	if v, ok := info.Video(); ok {
		r.VideoCodec, r.Width, r.Height, r.PixFmt = v.Codec, v.Width, v.Height, v.PixFmt
	}
	if a, ok := info.Audio(); ok {
		r.HasAudio, r.AudioCodec = true, a.Codec
	}
	return r, nil
}

// RunQualityCheck is the quality_check step (ACS-067): ffprobe codec,
// resolution, duration ±0.5 s and audio stream; on failure the video is
// rendered once more, then the step fails (the asset stays unchanged).
func (s *Service) RunQualityCheck(ctx context.Context, sc jobuc.StepContext) (jobuc.StepResult, error) {
	var out renderOutput
	if err := depOutput(sc, domain.StepRender, &out); err != nil {
		return jobuc.StepResult{}, err
	}
	expect := domain.Expectation{Width: out.Width, Height: out.Height, DurationMs: out.DurationMs}
	res, err := s.probe(ctx, out.FileID)
	if err != nil {
		return jobuc.StepResult{}, err
	}
	issues := domain.CheckOutput(res, expect)
	if len(issues) == 0 {
		return jobuc.StepResult{Output: out}, nil
	}
	s.log().WarnContext(ctx, "video: quality check failed, rendering again", slog.Any("issues", issues))
	p, err := s.projectOf(ctx, sc)
	if err != nil {
		return jobuc.StepResult{}, err
	}
	again, err := s.renderOnce(ctx, sc, p)
	if err != nil {
		return jobuc.StepResult{}, jobuc.Permanent(ErrQualityCheck.Wrap(err).WithDetails(kernel.Detail{"issues": issues}))
	}
	res, err = s.probe(ctx, again.FileID)
	if err != nil {
		return jobuc.StepResult{}, err
	}
	if issues := domain.CheckOutput(res, domain.Expectation{Width: again.Width, Height: again.Height, DurationMs: again.DurationMs}); len(issues) > 0 {
		return jobuc.StepResult{}, jobuc.Permanent(ErrQualityCheck.WithDetails(kernel.Detail{"issues": issues}))
	}
	again.Rerendered = true
	return jobuc.StepResult{Output: again}, nil
}

func changeNote(t jobuc.JobType) string {
	switch t {
	case jobuc.TypeVideoSceneRegenerate:
		return "Tạo lại cảnh và render lại"
	case jobuc.TypeVideoRender:
		return "Render lại video"
	}
	return "Sản xuất video"
}

// RunPublish is the asset_publish_ready step: a new asset version with the
// MP4 (primary), thumbnail and subtitles, so the video can be reviewed.
func (s *Service) RunPublish(ctx context.Context, sc jobuc.StepContext) (jobuc.StepResult, error) {
	var out renderOutput
	if err := depOutput(sc, domain.StepQC, &out); err != nil {
		return jobuc.StepResult{}, err
	}
	p, err := s.projectOf(ctx, sc)
	if err != nil {
		return jobuc.StepResult{}, err
	}
	result := func(versionNo int) jobuc.StepResult {
		return jobuc.StepResult{
			Output:    map[string]any{"file_id": out.FileID.String(), "version_no": versionNo},
			JobResult: map[string]any{"asset_ids": []string{p.AssetID.String()}, "file_id": out.FileID.String(), "version_no": versionNo},
		}
	}
	if p.Output != nil && p.Output.FileID == out.FileID {
		return result(p.Output.VersionNo), nil
	}
	scenes, err := s.Repo.ListScenes(ctx, p.AssetID)
	if err != nil {
		return jobuc.StepResult{}, err
	}
	ids := make([]string, len(scenes))
	for i, scn := range scenes {
		ids[i] = scn.ID.String()
	}
	content, _ := json.Marshal(map[string]any{
		"prompt": p.Prompt, "preset": p.Settings.Preset, "aspect_ratio": p.Settings.AspectRatio, "script": p.Script,
		"scene_ids": ids, "final_file_id": out.FileID, "subtitle_file_id": out.SubtitleFileID,
		"thumbnail_file_id": out.ThumbnailFileID, "duration_ms": out.DurationMs,
	})
	files := []assetuc.VersionFile{{FileID: out.FileID, Role: assetuc.RolePrimary}}
	if out.ThumbnailFileID != nil {
		files = append(files, assetuc.VersionFile{FileID: *out.ThumbnailFileID, Role: assetuc.RoleThumbnail})
	}
	if out.SubtitleFileID != nil {
		files = append(files, assetuc.VersionFile{FileID: *out.SubtitleFileID, Role: assetuc.RoleSubtitle})
	}
	j := sc.Job()
	var versionNo int
	err = s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		jobID := j.ID
		ref, err := s.Assets.AddVersion(ctx, p.AssetID, assetuc.NewVersion{
			ContentJSON: content, ChangeNote: changeNote(j.Type), GeneratedByJobID: &jobID, CreatedBy: j.CreatedBy,
			Files: files, SearchText: p.Prompt + " " + p.Script,
		})
		if err != nil {
			return err
		}
		versionNo = ref.VersionNo
		cur, err := s.Repo.GetProject(ctx, p.AssetID)
		if err != nil {
			return err
		}
		cur.Output = &domain.Output{FileID: out.FileID, ThumbnailFileID: out.ThumbnailFileID, SubtitleFileID: out.SubtitleFileID,
			DurationMs: out.DurationMs, VersionNo: ref.VersionNo}
		return s.Repo.UpdateProject(ctx, cur)
	})
	if err != nil {
		if errors.Is(err, kernel.ErrNotFound) {
			return jobuc.StepResult{}, jobuc.Permanent(err)
		}
		return jobuc.StepResult{}, err
	}
	return result(versionNo), nil
}
