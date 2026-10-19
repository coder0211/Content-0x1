package domain

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/be/acs/api/internal/shared/kernel"
)

// SceneStatus mirrors video_scenes.status.
type SceneStatus string

const (
	SceneDraft      SceneStatus = "draft"
	SceneGenerating SceneStatus = "generating"
	SceneReady      SceneStatus = "ready"
	SceneFailed     SceneStatus = "failed"
)

// CanTransition implements draft → generating → ready|failed and
// ready|failed → generating (regenerate); editing a scene's content sends
// it back to draft.
func (s SceneStatus) CanTransition(to SceneStatus) bool {
	switch to {
	case SceneGenerating:
		return s == SceneDraft || s == SceneReady || s == SceneFailed || s == SceneGenerating
	case SceneReady, SceneFailed:
		return s == SceneGenerating
	case SceneDraft:
		return true
	}
	return false
}

// Scene is one storyboard scene (video_scenes row).
type Scene struct {
	ID              kernel.ID
	AssetID         kernel.ID
	SequenceNo      int
	DurationMs      int
	VisualPrompt    string
	Narration       string
	SubtitleText    string
	SourceFileID    *kernel.ID
	ClipFileID      *kernel.ID
	ThumbnailFileID *kernel.ID
	Status          SceneStatus
	AttemptCount    int
	LastError       string
	// ProviderJobID is the provider submission of the current attempt;
	// UsageRecordedFor is the submission whose usage was already recorded
	// (a retried step must not bill twice).
	ProviderJobID    string
	UsageRecordedFor string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// IsReady reports whether the scene has a normalized clip.
func (s Scene) IsReady() bool { return s.Status == SceneReady && s.ClipFileID != nil }

// SceneDraftInput is one scene of a PUT /videos/{id}/scenes request.
type SceneDraftInput struct {
	ID           *kernel.ID
	DurationMs   int
	VisualPrompt string
	Narration    string
	SubtitleText string
	SourceFileID *kernel.ID
}

// ValidateStoryboard checks OQ-07 limits and returns every violation
// (field/issue details, empty when the storyboard can be produced).
func ValidateStoryboard(scenes []SceneDraftInput) []kernel.Detail {
	var out []kernel.Detail
	if len(scenes) == 0 {
		out = append(out, kernel.FieldIssue("scenes", "required"))
	}
	if len(scenes) > MaxScenes {
		out = append(out, kernel.Detail{"field": "scenes", "issue": "max_scenes", "max": MaxScenes})
	}
	total := 0
	for i, s := range scenes {
		total += s.DurationMs
		if s.DurationMs < MinSceneMs || s.DurationMs > MaxSceneMs {
			out = append(out, kernel.Detail{"field": fmt.Sprintf("scenes[%d].duration_ms", i), "issue": "out_of_range", "min": MinSceneMs, "max": MaxSceneMs})
		}
		if strings.TrimSpace(s.VisualPrompt) == "" {
			out = append(out, kernel.FieldIssue(fmt.Sprintf("scenes[%d].visual_prompt", i), "required"))
		}
		if len([]rune(s.VisualPrompt)) > MaxSceneTextLen || len([]rune(s.Narration)) > MaxSceneTextLen || len([]rune(s.SubtitleText)) > MaxSceneTextLen {
			out = append(out, kernel.Detail{"field": fmt.Sprintf("scenes[%d]", i), "issue": "too_long", "max": MaxSceneTextLen})
		}
	}
	if total > MaxTotalMs {
		out = append(out, kernel.Detail{"field": "scenes", "issue": "total_duration_exceeded", "max_ms": MaxTotalMs, "total_ms": total})
	}
	return out
}

// ScenesAsDrafts converts stored scenes for validation.
func ScenesAsDrafts(scenes []Scene) []SceneDraftInput {
	out := make([]SceneDraftInput, len(scenes))
	for i, s := range scenes {
		id := s.ID
		out[i] = SceneDraftInput{ID: &id, DurationMs: s.DurationMs, VisualPrompt: s.VisualPrompt, Narration: s.Narration, SubtitleText: s.SubtitleText, SourceFileID: s.SourceFileID}
	}
	return out
}

// TotalMs sums scene durations.
func TotalMs(scenes []Scene) int {
	t := 0
	for _, s := range scenes {
		t += s.DurationMs
	}
	return t
}

// SceneCountFor suggests how many scenes a storyboard of target seconds
// should have (~5 s per scene, within limits): 20 s → 4.
func SceneCountFor(targetSeconds int) int {
	n := int(math.Ceil(float64(targetSeconds) / 5.0))
	minN := int(math.Ceil(float64(targetSeconds*1000) / MaxSceneMs))
	n = max(n, minN, 1)
	return min(n, MaxScenes)
}

// RawScene is one scene as proposed by the text model.
type RawScene struct {
	VisualPrompt    string  `json:"visual_prompt"`
	Narration       string  `json:"narration"`
	SubtitleText    string  `json:"subtitle_text"`
	DurationSeconds float64 `json:"duration_seconds"`
}

// NormalizeStoryboard turns a model proposal into valid scenes: drops empty
// scenes, keeps at most MaxScenes, and rescales durations so they sum to the
// target (clamped to 2–8 s per scene, rounded to 500 ms, total ≤ 60 s).
// Source images are assigned round-robin.
func NormalizeStoryboard(raw []RawScene, targetSeconds int, sourceFiles []kernel.ID) []SceneDraftInput {
	var kept []RawScene
	for _, r := range raw {
		r.VisualPrompt = strings.TrimSpace(r.VisualPrompt)
		if r.VisualPrompt == "" {
			continue
		}
		kept = append(kept, r)
		if len(kept) == MaxScenes {
			break
		}
	}
	if len(kept) == 0 {
		return nil
	}
	target := min(max(targetSeconds, MinTargetSeconds), MaxTargetSeconds) * 1000
	// Never ask for more than the scenes can hold, nor less than their minimum.
	target = min(max(target, len(kept)*MinSceneMs), len(kept)*MaxSceneMs, MaxTotalMs)
	sum := 0.0
	for _, r := range kept {
		if r.DurationSeconds > 0 {
			sum += r.DurationSeconds
		}
	}
	durs := make([]int, len(kept))
	for i, r := range kept {
		share := 1.0 / float64(len(kept))
		if sum > 0 && r.DurationSeconds > 0 {
			share = r.DurationSeconds / sum
		}
		durs[i] = clampScene(roundHalfSecond(share * float64(target)))
	}
	fitTotal(durs, target)
	out := make([]SceneDraftInput, len(kept))
	for i, r := range kept {
		sub := strings.TrimSpace(r.SubtitleText)
		if sub == "" {
			sub = strings.TrimSpace(r.Narration)
		}
		out[i] = SceneDraftInput{DurationMs: durs[i], VisualPrompt: truncate(r.VisualPrompt), Narration: truncate(strings.TrimSpace(r.Narration)), SubtitleText: truncate(sub)}
		if len(sourceFiles) > 0 {
			f := sourceFiles[i%len(sourceFiles)]
			out[i].SourceFileID = &f
		}
	}
	return out
}

func truncate(s string) string {
	r := []rune(s)
	if len(r) > MaxSceneTextLen {
		return string(r[:MaxSceneTextLen])
	}
	return s
}

func roundHalfSecond(ms float64) int { return int(math.Round(ms/500.0)) * 500 }

func clampScene(ms int) int { return min(max(ms, MinSceneMs), MaxSceneMs) }

// fitTotal nudges durations by 500 ms steps until they sum to target (or
// no scene can move further).
func fitTotal(durs []int, target int) {
	for range 200 {
		total := 0
		for _, d := range durs {
			total += d
		}
		diff := target - total
		if diff > -500 && diff < 500 {
			return
		}
		moved := false
		for i := range durs {
			if diff >= 500 && durs[i] < MaxSceneMs {
				durs[i] += 500
				diff -= 500
				moved = true
			} else if diff <= -500 && durs[i] > MinSceneMs {
				durs[i] -= 500
				diff += 500
				moved = true
			}
			if diff > -500 && diff < 500 {
				return
			}
		}
		if !moved {
			return
		}
	}
}
