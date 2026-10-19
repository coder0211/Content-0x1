package domain

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/be/acs/api/internal/shared/kernel"
)

// Step types of the video workflows (SPEC-02 §5.5).
const (
	StepScript     = "script_generation"
	StepStoryboard = "storyboard_generation"
	StepScene      = "scene_generation"
	StepVoiceover  = "voiceover_generation"
	StepSubtitle   = "subtitle_generation"
	StepRender     = "video_render"
	StepQC         = "quality_check"
	StepPublish    = "asset_publish_ready"
)

// SceneStepKey is "scene_generation:<n>" (n = 1-based sequence).
func SceneStepKey(seq int) string { return StepScene + ":" + strconv.Itoa(seq) }

// IsSceneStep reports whether key is a scene_generation step.
func IsSceneStep(key string) bool { return strings.HasPrefix(key, StepScene+":") }

// SceneStepInput is the input of scene_generation.
type SceneStepInput struct {
	SceneID kernel.ID `json:"scene_id"`
	// Force regenerates a scene that is already ready (regenerate job);
	// BaseAttempt is the scene attempt_count when the job was created, so a
	// retry after this job already regenerated it reuses the new clip.
	Force       bool `json:"force,omitempty"`
	BaseAttempt int  `json:"base_attempt,omitempty"`
}

// StepNode is one node of a workflow DAG.
type StepNode struct {
	Key       string
	Type      string
	DependsOn []string
	Input     any
	// MaxAttempts 0 = engine default.
	MaxAttempts int
}

// StoryboardSteps is job 1 (video_storyboard): script → storyboard.
func StoryboardSteps() []StepNode {
	return []StepNode{
		{Key: StepScript, Type: StepScript, Input: map[string]any{}},
		{Key: StepStoryboard, Type: StepStoryboard, DependsOn: []string{StepScript}, Input: map[string]any{}},
	}
}

// RenderPlan says which optional nodes a production-like job needs.
type RenderPlan struct {
	// Scenes to (re)generate; others reuse their clips.
	Scenes []SceneStepInput
	// SceneSeq maps scene id → 1-based sequence for step keys.
	SceneSeq map[kernel.ID]int
	// Voice / Subtitles: generate a new voice-over / subtitle files. When
	// false the render reuses the stored track (or renders without).
	Voice     bool
	Subtitles bool
}

// pollAttempts bounds scene polling: max 5 transient retries (SPEC-02 §5.3).
const pollAttempts = 5

// RenderSteps builds video_production / video_scene_regenerate /
// video_render DAGs: scene steps run in parallel; subtitles wait for the
// voice-over (timings); the render waits for every scene, the voice-over
// and the subtitles; then quality_check and asset_publish_ready.
func RenderSteps(p RenderPlan) []StepNode {
	var nodes []StepNode
	var renderDeps []string
	for _, s := range p.Scenes {
		key := SceneStepKey(p.SceneSeq[s.SceneID])
		nodes = append(nodes, StepNode{Key: key, Type: StepScene, Input: s, MaxAttempts: pollAttempts})
		renderDeps = append(renderDeps, key)
	}
	if p.Voice {
		nodes = append(nodes, StepNode{Key: StepVoiceover, Type: StepVoiceover, Input: map[string]any{}})
		renderDeps = append(renderDeps, StepVoiceover)
	}
	if p.Subtitles {
		var deps []string
		if p.Voice {
			deps = []string{StepVoiceover}
		}
		nodes = append(nodes, StepNode{Key: StepSubtitle, Type: StepSubtitle, DependsOn: deps, Input: map[string]any{}})
		renderDeps = append(renderDeps, StepSubtitle)
	}
	nodes = append(nodes,
		StepNode{Key: StepRender, Type: StepRender, DependsOn: renderDeps, Input: map[string]any{}},
		StepNode{Key: StepQC, Type: StepQC, DependsOn: []string{StepRender}, Input: map[string]any{}},
		StepNode{Key: StepPublish, Type: StepPublish, DependsOn: []string{StepQC}, Input: map[string]any{}},
	)
	return nodes
}

// ValidateDAG checks unique keys, known dependencies and the absence of
// cycles (topological order exists).
func ValidateDAG(nodes []StepNode) error {
	idx := map[string]int{}
	for i, n := range nodes {
		if _, dup := idx[n.Key]; dup {
			return fmt.Errorf("duplicate step key %q", n.Key)
		}
		idx[n.Key] = i
	}
	for _, n := range nodes {
		for _, d := range n.DependsOn {
			if _, ok := idx[d]; !ok {
				return fmt.Errorf("step %q depends on unknown %q", n.Key, d)
			}
		}
	}
	if _, err := TopoOrder(nodes); err != nil {
		return err
	}
	return nil
}

// TopoOrder returns keys in a valid execution order (Kahn).
func TopoOrder(nodes []StepNode) ([]string, error) {
	indeg := map[string]int{}
	children := map[string][]string{}
	for _, n := range nodes {
		indeg[n.Key] += 0
		for _, d := range n.DependsOn {
			indeg[n.Key]++
			children[d] = append(children[d], n.Key)
		}
	}
	var queue, out []string
	for _, n := range nodes {
		if indeg[n.Key] == 0 {
			queue = append(queue, n.Key)
		}
	}
	for len(queue) > 0 {
		k := queue[0]
		queue = queue[1:]
		out = append(out, k)
		for _, c := range children[k] {
			indeg[c]--
			if indeg[c] == 0 {
				queue = append(queue, c)
			}
		}
	}
	if len(out) != len(nodes) {
		return nil, fmt.Errorf("workflow has a cycle")
	}
	return out, nil
}

// stepWeight weights progress: provider-bound scene steps dominate.
func stepWeight(stepType string) int {
	switch stepType {
	case StepScene:
		return 10
	case StepRender:
		return 8
	case StepVoiceover, StepStoryboard, StepScript:
		return 4
	}
	return 1
}

// StepState is the minimal view of a step for progress.
type StepState struct {
	Type   string
	Status string
}

// Progress computes job progress 0–100 from step states (succeeded and
// skipped count fully; running/waiting count half).
func Progress(steps []StepState) int {
	total, done := 0, 0
	for _, s := range steps {
		w := stepWeight(s.Type)
		total += 2 * w
		switch s.Status {
		case "succeeded", "skipped":
			done += 2 * w
		case "running", "waiting_provider":
			done += w
		}
	}
	if total == 0 {
		return 0
	}
	return done * 100 / total
}
