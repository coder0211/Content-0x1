package usecase

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	fileuc "github.com/be/acs/api/internal/modules/file/usecase"
	jobuc "github.com/be/acs/api/internal/modules/job/usecase"
	"github.com/be/acs/api/internal/modules/video/domain"
	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
)

// voiceFingerprint is "" when the project has no voice-over to make.
func voiceFingerprint(p *domain.Project, scenes []domain.Scene) string {
	if !p.Settings.Voiceover.Enabled || !hasNarration(scenes) {
		return ""
	}
	return domain.VoiceFingerprint(scenes, p.Settings.Voiceover)
}

func audioExt(mime string) string {
	switch mime {
	case "audio/wav", "audio/x-wav", "audio/wave":
		return ".wav"
	case "audio/mpeg", "audio/mp3":
		return ".mp3"
	case "audio/ogg":
		return ".ogg"
	case "audio/aac", "audio/mp4":
		return ".m4a"
	}
	return ".audio"
}

// RunVoiceover is the voiceover_generation step: TTS of each scene's
// narration placed at the scene start (or one call for the whole script).
// An unchanged voice-over (same fingerprint) is reused without new calls.
func (s *Service) RunVoiceover(ctx context.Context, sc jobuc.StepContext) (jobuc.StepResult, error) {
	p, err := s.projectOf(ctx, sc)
	if err != nil {
		return jobuc.StepResult{}, err
	}
	scenes, err := s.Repo.ListScenes(ctx, p.AssetID)
	if err != nil {
		return jobuc.StepResult{}, err
	}
	fp := voiceFingerprint(p, scenes)
	if fp == "" {
		return jobuc.StepResult{Output: map[string]any{"skipped": true}}, nil
	}
	if p.Voice != nil && p.Voice.Fingerprint == fp {
		return jobuc.StepResult{Output: p.Voice}, nil
	}
	req := s.request(sc)
	if req.VoiceProvider == "" {
		m, err := s.voiceModel(ctx, p.Settings)
		if err != nil || m == nil {
			return jobuc.StepResult{}, jobuc.Permanent(errors.New("no voice model"))
		}
		req.VoiceProvider, req.VoiceModel = m.ProviderCode, m.ModelCode
	}
	synth, err := s.AI.Voice(req.VoiceProvider)
	if err != nil {
		return jobuc.StepResult{}, jobuc.Permanent(err)
	}
	dir, cleanup, err := s.workDir("voice-*")
	if err != nil {
		return jobuc.StepResult{}, err
	}
	defer cleanup()
	slots, total := domain.SceneTimeline(scenes)
	type part struct {
		sceneID *kernel.ID
		text    string
		start   int
		maxMs   int
	}
	var parts []part
	if p.Settings.Voiceover.Mode == "whole" {
		var texts []string
		for _, s := range scenes {
			if t := strings.TrimSpace(s.Narration); t != "" {
				texts = append(texts, t)
			}
		}
		parts = append(parts, part{text: strings.Join(texts, " "), start: 0, maxMs: total})
	} else {
		for i, scn := range scenes {
			if t := strings.TrimSpace(scn.Narration); t != "" {
				id := scn.ID
				parts = append(parts, part{sceneID: &id, text: t, start: slots[i].StartMs, maxMs: slots[i].EndMs - slots[i].StartMs})
			}
		}
	}
	track := &domain.VoiceTrack{Fingerprint: fp, Segments: []domain.VoiceSegment{}}
	for i, pt := range parts {
		if cancelled, err := sc.CancelRequested(ctx); err == nil && cancelled {
			return jobuc.StepResult{}, errCancelled
		}
		resp, err := synth.Synthesize(ctx, ports.VoiceRequest{
			Model: req.VoiceModel, Text: pt.text, Language: voiceLanguage(p.Settings.Voiceover.Language),
			VoiceID: p.Settings.Voiceover.VoiceID, SpeakingRate: 1.0, Format: "mp3",
		})
		if err != nil {
			return jobuc.StepResult{}, err
		}
		if err := s.recordUsage(ctx, sc, req.VoiceProvider, req.VoiceModel, resp.Usage, false); err != nil {
			return jobuc.StepResult{}, err
		}
		path := filepath.Join(dir, fmt.Sprintf("voice_%02d%s", i, audioExt(resp.MimeType)))
		if err := os.WriteFile(path, resp.Audio, 0o600); err != nil {
			return jobuc.StepResult{}, err
		}
		f, err := s.store(ctx, sc, path, fileuc.StoreInput{MimeType: resp.MimeType, Kind: "audio", Source: "generated", DurationMs: resp.DurationMs})
		if err != nil {
			return jobuc.StepResult{}, err
		}
		timings := make([]domain.WordTiming, len(resp.Timings))
		for k, t := range resp.Timings {
			timings[k] = domain.WordTiming{Text: t.Text, StartMs: t.StartMs, EndMs: t.EndMs}
		}
		track.Segments = append(track.Segments, domain.VoiceSegment{
			SceneID: pt.sceneID, FileID: f.ID, StartMs: pt.start, DurationMs: resp.DurationMs, MaxMs: pt.maxMs, Timings: timings,
		})
		_ = sc.Progress(ctx, (i+1)*100/len(parts), "")
	}
	cur, err := s.Repo.GetProject(ctx, p.AssetID)
	if err != nil {
		return jobuc.StepResult{}, err
	}
	cur.Voice = track
	if err := s.Repo.UpdateProject(ctx, cur); err != nil {
		return jobuc.StepResult{}, err
	}
	return jobuc.StepResult{Output: track}, nil
}

// RunSubtitles is the subtitle_generation step: cues from subtitle text (or
// narration) timed by scene slots or TTS word timings, never overlapping,
// written as SRT (soft track / download) and ASS (burn-in, Vietnamese font).
func (s *Service) RunSubtitles(ctx context.Context, sc jobuc.StepContext) (jobuc.StepResult, error) {
	p, err := s.projectOf(ctx, sc)
	if err != nil {
		return jobuc.StepResult{}, err
	}
	scenes, err := s.Repo.ListScenes(ctx, p.AssetID)
	if err != nil {
		return jobuc.StepResult{}, err
	}
	if !p.Settings.Subtitles.Enabled {
		return jobuc.StepResult{Output: map[string]any{"skipped": true}}, nil
	}
	voiceFP := voiceFingerprint(p, scenes)
	fp := domain.SubtitleFingerprint(scenes, p.Settings.Subtitles, voiceFP, p.Settings.Preset)
	if p.Subtitle != nil && p.Subtitle.Fingerprint == fp {
		return jobuc.StepResult{Output: p.Subtitle}, nil
	}
	var voice *domain.VoiceTrack
	if voiceFP != "" && p.Voice != nil && p.Voice.Fingerprint == voiceFP {
		voice = p.Voice
	}
	cues := domain.BuildCues(scenes, voice)
	track := &domain.SubtitleTrack{Fingerprint: fp, CueCount: len(cues)}
	if len(cues) > 0 {
		dir, cleanup, err := s.workDir("subs-*")
		if err != nil {
			return jobuc.StepResult{}, err
		}
		defer cleanup()
		preset, _ := domain.PresetFor(p.Settings.Preset)
		srt, ass := filepath.Join(dir, "subtitles.srt"), filepath.Join(dir, "subtitles.ass")
		if err := os.WriteFile(srt, []byte(domain.SRT(cues)), 0o600); err != nil {
			return jobuc.StepResult{}, err
		}
		if err := os.WriteFile(ass, []byte(domain.ASS(cues, preset)), 0o600); err != nil {
			return jobuc.StepResult{}, err
		}
		fs, err := s.store(ctx, sc, srt, fileuc.StoreInput{MimeType: "application/x-subrip", Kind: "subtitle", Source: "generated"})
		if err != nil {
			return jobuc.StepResult{}, err
		}
		fa, err := s.store(ctx, sc, ass, fileuc.StoreInput{MimeType: "text/x-ssa", Kind: "subtitle", Source: "generated"})
		if err != nil {
			return jobuc.StepResult{}, err
		}
		track.SRTFileID, track.ASSFileID = fs.ID, fa.ID
	}
	cur, err := s.Repo.GetProject(ctx, p.AssetID)
	if err != nil {
		return jobuc.StepResult{}, err
	}
	cur.Subtitle = track
	if err := s.Repo.UpdateProject(ctx, cur); err != nil {
		return jobuc.StepResult{}, err
	}
	return jobuc.StepResult{Output: track}, nil
}
