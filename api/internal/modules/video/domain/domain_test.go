package domain_test

import (
	"encoding/json"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/be/acs/api/internal/modules/video/domain"
	"github.com/be/acs/api/internal/shared/kernel"
)

func drafts(durs ...int) []domain.SceneDraftInput {
	out := make([]domain.SceneDraftInput, len(durs))
	for i, d := range durs {
		out[i] = domain.SceneDraftInput{DurationMs: d, VisualPrompt: "cảnh"}
	}
	return out
}

func issues(ds []kernel.Detail) []string {
	var out []string
	for _, d := range ds {
		out = append(out, d["field"].(string)+":"+d["issue"].(string))
	}
	return out
}

func TestValidateStoryboard(t *testing.T) {
	for _, tc := range []struct {
		name   string
		scenes []domain.SceneDraftInput
		want   []string
	}{
		{"ok", drafts(5000, 5000, 5000, 5000), nil},
		{"empty", nil, []string{"scenes:required"}},
		{"too short", drafts(1500), []string{"scenes[0].duration_ms:out_of_range"}},
		{"too long", drafts(8500), []string{"scenes[0].duration_ms:out_of_range"}},
		{"too many", drafts(2000, 2000, 2000, 2000, 2000, 2000, 2000, 2000, 2000, 2000, 2000), []string{"scenes:max_scenes"}},
		{"total over 60s", drafts(8000, 8000, 8000, 8000, 8000, 8000, 8000, 8000), []string{"scenes:total_duration_exceeded"}},
		{"blank prompt", []domain.SceneDraftInput{{DurationMs: 3000, VisualPrompt: "  "}}, []string{"scenes[0].visual_prompt:required"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := issues(domain.ValidateStoryboard(tc.scenes))
			if !slices.Equal(got, tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestSceneCountAndNormalize20s(t *testing.T) {
	n := domain.SceneCountFor(20)
	if n < 4 || n > 6 {
		t.Fatalf("scene count for 20s = %d", n)
	}
	if got := domain.SceneCountFor(60); got > domain.MaxScenes || got*domain.MaxSceneMs < 60000 {
		t.Fatalf("scene count for 60s = %d", got)
	}
	raw := []domain.RawScene{
		{VisualPrompt: "a", Narration: "n1", SubtitleText: "s1", DurationSeconds: 3},
		{VisualPrompt: "b", Narration: "n2", DurationSeconds: 7},
		{VisualPrompt: " ", Narration: "dropped"},
		{VisualPrompt: "c", Narration: "n3", DurationSeconds: 12},
		{VisualPrompt: "d", Narration: "n4", DurationSeconds: 1},
	}
	src := []kernel.ID{kernel.NewID(), kernel.NewID()}
	out := domain.NormalizeStoryboard(raw, 20, src)
	if len(out) != 4 {
		t.Fatalf("want 4 scenes, got %d", len(out))
	}
	total := 0
	for i, s := range out {
		total += s.DurationMs
		if s.DurationMs < domain.MinSceneMs || s.DurationMs > domain.MaxSceneMs || s.DurationMs%500 != 0 {
			t.Errorf("scene %d duration %d", i, s.DurationMs)
		}
		if s.SubtitleText == "" || s.VisualPrompt == "" {
			t.Errorf("scene %d missing text: %+v", i, s)
		}
		if s.SourceFileID == nil || *s.SourceFileID != src[i%2] {
			t.Errorf("scene %d source not round-robin", i)
		}
	}
	if total != 20000 {
		t.Fatalf("total %d, want 20000", total)
	}
	if issues := domain.ValidateStoryboard(out); len(issues) > 0 {
		t.Fatalf("normalized storyboard invalid: %v", issues)
	}
	// More scenes than limits allow are cut to 10 and to ≤ 60s.
	many := make([]domain.RawScene, 14)
	for i := range many {
		many[i] = domain.RawScene{VisualPrompt: "x", DurationSeconds: 8}
	}
	out = domain.NormalizeStoryboard(many, 60, nil)
	if len(out) != domain.MaxScenes || len(domain.ValidateStoryboard(out)) > 0 {
		t.Fatalf("limits not enforced: %d scenes, %v", len(out), domain.ValidateStoryboard(out))
	}
}

func TestRenderStepsDAG(t *testing.T) {
	ids := []kernel.ID{kernel.NewID(), kernel.NewID(), kernel.NewID()}
	plan := domain.RenderPlan{SceneSeq: map[kernel.ID]int{}, Voice: true, Subtitles: true}
	for i, id := range ids {
		plan.SceneSeq[id] = i + 1
		plan.Scenes = append(plan.Scenes, domain.SceneStepInput{SceneID: id})
	}
	nodes := domain.RenderSteps(plan)
	if err := domain.ValidateDAG(nodes); err != nil {
		t.Fatal(err)
	}
	order, _ := domain.TopoOrder(nodes)
	pos := map[string]int{}
	for i, k := range order {
		pos[k] = i
	}
	for _, k := range []string{"scene_generation:1", "scene_generation:2", "scene_generation:3", domain.StepVoiceover, domain.StepSubtitle} {
		if pos[k] >= pos[domain.StepRender] {
			t.Errorf("%s must run before render", k)
		}
	}
	if pos[domain.StepVoiceover] >= pos[domain.StepSubtitle] || pos[domain.StepRender] >= pos[domain.StepQC] || pos[domain.StepQC] >= pos[domain.StepPublish] {
		t.Errorf("bad order %v", order)
	}
	var render domain.StepNode
	for _, n := range nodes {
		if n.Key == domain.StepRender {
			render = n
		}
		if domain.IsSceneStep(n.Key) && len(n.DependsOn) != 0 {
			t.Errorf("scene steps run in parallel, %s has deps %v", n.Key, n.DependsOn)
		}
	}
	if len(render.DependsOn) != 5 {
		t.Errorf("render deps %v", render.DependsOn)
	}
	// video_render: no scenes, no audio → render has no deps.
	nodes = domain.RenderSteps(domain.RenderPlan{})
	if len(nodes) != 3 || len(nodes[0].DependsOn) != 0 {
		t.Errorf("render-only DAG %+v", nodes)
	}
	cyc := []domain.StepNode{{Key: "a", DependsOn: []string{"b"}}, {Key: "b", DependsOn: []string{"a"}}}
	if domain.ValidateDAG(cyc) == nil {
		t.Error("cycle accepted")
	}
	if domain.ValidateDAG([]domain.StepNode{{Key: "a", DependsOn: []string{"x"}}}) == nil {
		t.Error("unknown dependency accepted")
	}
}

func TestProgress(t *testing.T) {
	steps := []domain.StepState{{Type: domain.StepScene, Status: "succeeded"}, {Type: domain.StepScene, Status: "pending"}, {Type: domain.StepRender, Status: "pending"}}
	p := domain.Progress(steps)
	if p <= 0 || p >= 100 {
		t.Fatalf("progress %d", p)
	}
	steps[1].Status = "waiting_provider"
	if q := domain.Progress(steps); q <= p {
		t.Fatalf("waiting step must add progress: %d <= %d", q, p)
	}
	for i := range steps {
		steps[i].Status = "succeeded"
	}
	if domain.Progress(steps) != 100 || domain.Progress(nil) != 0 {
		t.Fatal("bounds")
	}
}

func TestDeriveStage(t *testing.T) {
	for _, tc := range []struct {
		typ, status string
		out         bool
		want        domain.Stage
	}{
		{"video_storyboard", "running", false, domain.StageStoryboarding},
		{"video_storyboard", "succeeded", false, domain.StageEditing},
		{"video_storyboard", "failed", false, domain.StageEditing},
		{"video_production", "queued", false, domain.StageProducing},
		{"video_production", "failed", false, domain.StageFailed},
		{"video_render", "succeeded", true, domain.StageReady},
		{"", "", false, domain.StageEditing},
	} {
		if got := domain.DeriveStage(tc.typ, tc.status, tc.out); got != tc.want {
			t.Errorf("%s/%s: %s want %s", tc.typ, tc.status, got, tc.want)
		}
	}
}

func TestSceneTransitions(t *testing.T) {
	if !domain.SceneDraft.CanTransition(domain.SceneGenerating) || !domain.SceneGenerating.CanTransition(domain.SceneReady) ||
		!domain.SceneReady.CanTransition(domain.SceneGenerating) || domain.SceneDraft.CanTransition(domain.SceneReady) {
		t.Fatal("scene state machine")
	}
}

var srtRe = regexp.MustCompile(`^(\d\d):(\d\d):(\d\d),(\d\d\d) --> (\d\d):(\d\d):(\d\d),(\d\d\d)$`)

func TestCuesNoOverlapAndSRT(t *testing.T) {
	scenes := []domain.Scene{
		{ID: kernel.NewID(), DurationMs: 4000, SubtitleText: "Mùa thu này, học tiếng Anh cùng Sao Mai với lớp học nhỏ và giáo viên tận tâm"},
		{ID: kernel.NewID(), DurationMs: 3000, Narration: "Đăng ký tư vấn ngay hôm nay"},
		{ID: kernel.NewID(), DurationMs: 2000},
		{ID: kernel.NewID(), DurationMs: 5000, SubtitleText: "Ưu đãi có hạn"},
	}
	voice := &domain.VoiceTrack{Segments: []domain.VoiceSegment{{SceneID: &scenes[1].ID, StartMs: 3700, Timings: []domain.WordTiming{
		{Text: "Đăng", StartMs: 0, EndMs: 300}, {Text: "ký", StartMs: 300, EndMs: 600}, {Text: "tư", StartMs: 600, EndMs: 900},
		{Text: "vấn", StartMs: 900, EndMs: 1200}, {Text: "ngay", StartMs: 1200, EndMs: 1500}, {Text: "hôm", StartMs: 1500, EndMs: 1800}, {Text: "nay", StartMs: 1800, EndMs: 2100},
	}}}}
	for _, v := range []*domain.VoiceTrack{nil, voice} {
		cues := domain.BuildCues(scenes, v)
		if len(cues) < 3 {
			t.Fatalf("cues %+v", cues)
		}
		_, total := domain.SceneTimeline(scenes)
		for i, c := range cues {
			if c.EndMs <= c.StartMs || c.StartMs < 0 || c.EndMs > total {
				t.Errorf("cue %d bad range %+v", i, c)
			}
			if i > 0 && c.StartMs < cues[i-1].EndMs {
				t.Errorf("cue %d overlaps previous: %+v / %+v", i, cues[i-1], c)
			}
			if len([]rune(c.Text)) > 42 {
				t.Errorf("cue %d too long: %q", i, c.Text)
			}
		}
		srt := domain.SRT(cues)
		blocks := strings.Split(strings.TrimSpace(srt), "\n\n")
		if len(blocks) != len(cues) {
			t.Fatalf("srt blocks %d", len(blocks))
		}
		for _, b := range blocks {
			lines := strings.Split(b, "\n")
			if len(lines) < 3 || !srtRe.MatchString(lines[1]) {
				t.Fatalf("bad srt block %q", b)
			}
		}
		if !strings.Contains(srt, "Sao Mai") {
			t.Fatal("vietnamese text lost")
		}
	}
	// Overlapping input is trimmed.
	n := domain.NormalizeCues([]domain.Cue{{StartMs: 1000, EndMs: 3000, Text: "b"}, {StartMs: 0, EndMs: 2000, Text: "a"}, {StartMs: 2500, EndMs: 2550, Text: "short"}})
	if len(n) != 2 || n[0].EndMs > n[1].StartMs {
		t.Fatalf("normalize %+v", n)
	}
}

func TestASSEscapesUserText(t *testing.T) {
	ass := domain.ASS([]domain.Cue{{StartMs: 0, EndMs: 1500, Text: `Giảm {\b1}50%\N ngay`}}, domain.Presets["social_vertical"])
	if strings.Contains(ass, `{\b1}`) || strings.Contains(ass, `\N ngay`) {
		t.Fatalf("override tags not escaped: %s", ass)
	}
	if !strings.Contains(ass, domain.SubtitleFont) || !strings.Contains(ass, "PlayResX: 1080") || !strings.Contains(ass, "0:00:01.50") {
		t.Fatalf("ass header/time: %s", ass)
	}
}

func TestTimeline(t *testing.T) {
	slots, total := domain.Timeline([]int{4000, 3000, 5000}, 300)
	if total != 11400 || slots[1].StartMs != 3700 || slots[2].StartMs != 6400 {
		t.Fatalf("slots %+v total %d", slots, total)
	}
	if _, total := domain.Timeline(nil, 300); total != 0 {
		t.Fatal("empty")
	}
}

func TestCheckOutput(t *testing.T) {
	good := domain.ProbeResult{DurationMs: 19800, VideoCodec: "h264", Width: 1080, Height: 1920, PixFmt: "yuv420p", HasAudio: true, AudioCodec: "aac", FormatNames: "mov,mp4,m4a"}
	exp := domain.Expectation{Width: 1080, Height: 1920, DurationMs: 20000}
	if is := domain.CheckOutput(good, exp); len(is) != 0 {
		t.Fatalf("good output rejected: %v", is)
	}
	bad := good
	bad.DurationMs, bad.HasAudio, bad.Width = 21000, false, 720
	if is := domain.CheckOutput(bad, exp); len(is) != 3 {
		t.Fatalf("want 3 issues, got %v", is)
	}
	if is := domain.CheckOutput(domain.ProbeResult{FormatNames: "unreadable"}, exp); len(is) < 4 {
		t.Fatalf("unreadable: %v", is)
	}
}

func TestMusicLicensed(t *testing.T) {
	now := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	lic := func(s string) json.RawMessage { return json.RawMessage(s) }
	for _, tc := range []struct {
		typ  string
		lic  json.RawMessage
		want bool
	}{
		{"music", lic(`{"source":"Artlist","type":"commercial"}`), true},
		// The brand module stores license_type (brand/domain.License).
		{"music", lic(`{"source":"Artlist","license_type":"commercial","expires_at":"2027-12-31"}`), true},
		{"music", lic(`{"source":"Artlist","license_type":"commercial","expires_at":"2026-10-18"}`), false},
		{"music", lic(`{"source":"Artlist","license":"CC-BY","expires_at":"2027-01-01"}`), true},
		{"music", lic(`{"source":"Artlist","type":"commercial","expires_at":"2026-10-01T00:00:00Z"}`), false},
		{"music", lic(`{"type":"commercial"}`), false},
		{"music", nil, false},
		{"logo", lic(`{"source":"x","type":"y"}`), false},
		{"music", lic(`not json`), false},
	} {
		if got := domain.MusicLicensed(tc.typ, tc.lic, now); got != tc.want {
			t.Errorf("%s %s: %v", tc.typ, tc.lic, got)
		}
	}
}

func TestFingerprints(t *testing.T) {
	sc := []domain.Scene{{DurationMs: 3000, Narration: "a", SubtitleText: "x"}}
	v := domain.VoiceSettings{Enabled: true, Language: "vi", Mode: "per_scene"}
	a := domain.VoiceFingerprint(sc, v)
	sc2 := []domain.Scene{{DurationMs: 3000, Narration: "a", SubtitleText: "changed"}}
	if a != domain.VoiceFingerprint(sc2, v) {
		t.Error("subtitle edit must not change the voice fingerprint")
	}
	sc2[0].Narration = "b"
	if a == domain.VoiceFingerprint(sc2, v) {
		t.Error("narration edit must change the voice fingerprint")
	}
	st := domain.SubtitleSettings{Enabled: true, Language: "vi", Style: "burn_in"}
	if domain.SubtitleFingerprint(sc, st, a, "square") == domain.SubtitleFingerprint(sc, st, a, "landscape") {
		t.Error("preset must change the subtitle fingerprint")
	}
}

// ACS-101: special characters in subtitle text — markup, override blocks,
// controls, bidi overrides, CR/LF and "-->" — cannot alter the SRT/ASS
// structure; Vietnamese diacritics survive; long words are hard-split.
func TestSubtitleSpecialCharacters(t *testing.T) {
	text := "Giảm <font color=red>50%</font> {\\an8}ngay\r\n2\n00:00:00,000 --> 00:00:09,000\x07\u202Eabc Ưu đãi hè " + strings.Repeat("x", 100)
	cues := domain.BuildCues([]domain.Scene{{ID: kernel.NewID(), DurationMs: 8000, SubtitleText: text}}, nil)
	srt := domain.SRT(cues)
	for _, bad := range []string{"<font", "{\\an8}", "\x07", "\u202E", "\r"} {
		if strings.Contains(srt, bad) {
			t.Errorf("SRT contains %q:\n%s", bad, srt)
		}
	}
	// Exactly one timing line per cue: user "-->" never starts a line.
	timing := 0
	for _, l := range strings.Split(srt, "\n") {
		if strings.Contains(l, " --> ") && strings.HasPrefix(l, "00:") && !strings.Contains(l, "Giảm") {
			timing++
		}
		if len([]rune(l)) > 42 {
			t.Errorf("cue line longer than 42 runes: %q", l)
		}
	}
	if timing != len(cues) || !strings.Contains(srt, "Ưu") || !strings.Contains(srt, "đãi hè") {
		t.Fatalf("timing lines %d for %d cues:\n%s", timing, len(cues), srt)
	}
	ass := domain.ASS(cues, domain.Presets["social_vertical"])
	if strings.Contains(ass, "{\\an8}") || strings.Contains(ass, "\u202E") || !strings.Contains(ass, "đãi hè") {
		t.Fatalf("ASS not escaped:\n%s", ass)
	}
}
