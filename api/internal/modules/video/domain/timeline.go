package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// Slot is a scene's position on the final timeline.
type Slot struct {
	StartMs int
	EndMs   int
}

// Timeline places scenes on the rendered timeline: each cross-fade of
// transitionMs overlaps consecutive clips, so scene i starts at
// sum(d[0..i)) - i*transitionMs. Total = sum(d) - (n-1)*transitionMs.
func Timeline(durationsMs []int, transitionMs int) ([]Slot, int) {
	slots := make([]Slot, len(durationsMs))
	start := 0
	for i, d := range durationsMs {
		slots[i] = Slot{StartMs: start, EndMs: start + d}
		start += d - transitionMs
	}
	if len(durationsMs) == 0 {
		return slots, 0
	}
	return slots, slots[len(slots)-1].EndMs
}

// SceneTimeline is Timeline over scenes with the default transition.
func SceneTimeline(scenes []Scene) ([]Slot, int) {
	d := make([]int, len(scenes))
	for i, s := range scenes {
		d[i] = s.DurationMs
	}
	return Timeline(d, TransitionFor(len(scenes)))
}

// TransitionFor returns the transition used for n scenes (none for one).
func TransitionFor(n int) int {
	if n < 2 {
		return 0
	}
	return TransitionMs
}

// VoiceFingerprint identifies the inputs of a voice-over so a re-render
// can reuse it when narration, timing and voice settings are unchanged.
func VoiceFingerprint(scenes []Scene, v VoiceSettings) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s|%s|%s|%s\n", v.Language, v.VoiceID, v.Mode, "v1")
	for _, s := range scenes {
		fmt.Fprintf(&b, "%d|%s\n", s.DurationMs, s.Narration)
	}
	return hash(b.String())
}

// SubtitleFingerprint identifies the inputs of the subtitle files.
func SubtitleFingerprint(scenes []Scene, st SubtitleSettings, voiceFingerprint string, preset string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s|%s|%s|%s|v1\n", st.Language, preset, voiceFingerprint, st.Style)
	for _, s := range scenes {
		fmt.Fprintf(&b, "%d|%s|%s\n", s.DurationMs, s.SubtitleText, s.Narration)
	}
	return hash(b.String())
}

func hash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:8])
}

// Cue is one subtitle line.
type Cue struct {
	StartMs int
	EndMs   int
	Text    string
}

// maxCueChars keeps a cue on at most two short lines.
const maxCueChars = 42

// BuildCues builds subtitle cues: per scene, the subtitle text (or the
// narration) is split into chunks of ≤ 42 characters. When word timings of a
// per-scene voice segment are available, chunk times follow the speech;
// otherwise chunks share the scene slot proportionally to their length. Cues
// never leave their scene slot and never overlap (NormalizeCues).
func BuildCues(scenes []Scene, voice *VoiceTrack) []Cue {
	slots, _ := SceneTimeline(scenes)
	timings := map[string]VoiceSegment{}
	if voice != nil {
		for _, seg := range voice.Segments {
			if seg.SceneID != nil && len(seg.Timings) > 0 {
				timings[seg.SceneID.String()] = seg
			}
		}
	}
	var cues []Cue
	for i, s := range scenes {
		text := strings.TrimSpace(s.SubtitleText)
		if text == "" {
			text = strings.TrimSpace(s.Narration)
		}
		if text == "" {
			continue
		}
		slot := slots[i]
		// Keep clear of the cross-fade into the next scene.
		end := slot.EndMs
		if i < len(scenes)-1 {
			end -= TransitionFor(len(scenes))
		}
		chunks := splitChunks(text, maxCueChars)
		if seg, ok := timings[s.ID.String()]; ok && strings.TrimSpace(s.SubtitleText) == "" {
			cues = append(cues, timedCues(chunks, seg, slot.StartMs, end)...)
			continue
		}
		cues = append(cues, proportionalCues(chunks, slot.StartMs, end)...)
	}
	return NormalizeCues(cues)
}

func proportionalCues(chunks []string, start, end int) []Cue {
	total := 0
	for _, c := range chunks {
		total += len([]rune(c))
	}
	out := make([]Cue, 0, len(chunks))
	t := start
	for i, c := range chunks {
		d := (end - start) * len([]rune(c)) / max(total, 1)
		e := t + d
		if i == len(chunks)-1 {
			e = end
		}
		out = append(out, Cue{StartMs: t, EndMs: e, Text: c})
		t = e
	}
	return out
}

// timedCues aligns chunks with word timings (words are consumed in order).
func timedCues(chunks []string, seg VoiceSegment, start, end int) []Cue {
	out := make([]Cue, 0, len(chunks))
	w := 0
	for i, c := range chunks {
		n := len(strings.Fields(c))
		if w >= len(seg.Timings) {
			return append(out, proportionalCues(chunks[i:], lastEnd(out, start), end)...)
		}
		first := seg.Timings[w]
		lastIdx := min(w+n-1, len(seg.Timings)-1)
		last := seg.Timings[lastIdx]
		w += n
		s := min(max(start+first.StartMs, start), end)
		e := min(start+last.EndMs, end)
		if i == len(chunks)-1 {
			e = max(e, min(s+1000, end))
		}
		out = append(out, Cue{StartMs: s, EndMs: e, Text: c})
	}
	return out
}

func lastEnd(cs []Cue, def int) int {
	if len(cs) == 0 {
		return def
	}
	return cs[len(cs)-1].EndMs
}

func splitChunks(text string, maxChars int) []string {
	words := strings.Fields(text)
	var out []string
	var cur []string
	n := 0
	for _, w := range words {
		l := len([]rune(w))
		if n > 0 && n+1+l > maxChars {
			out = append(out, strings.Join(cur, " "))
			cur, n = nil, 0
		}
		if n > 0 {
			n++
		}
		cur = append(cur, w)
		n += l
	}
	if len(cur) > 0 {
		out = append(out, strings.Join(cur, " "))
	}
	return out
}

// minCueMs drops cues too short to read.
const minCueMs = 200

// NormalizeCues sorts cues and trims each one so it ends before the next
// starts (valid SRT, no overlap); empty or too-short cues are dropped.
func NormalizeCues(cues []Cue) []Cue {
	sorted := append([]Cue(nil), cues...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].StartMs < sorted[j].StartMs })
	out := make([]Cue, 0, len(sorted))
	for i, c := range sorted {
		c.Text = strings.TrimSpace(c.Text)
		c.StartMs = max(c.StartMs, 0)
		if len(out) > 0 && c.StartMs < out[len(out)-1].EndMs {
			c.StartMs = out[len(out)-1].EndMs
		}
		if i+1 < len(sorted) && c.EndMs > sorted[i+1].StartMs && sorted[i+1].StartMs >= c.StartMs {
			c.EndMs = sorted[i+1].StartMs
		}
		if c.Text == "" || c.EndMs-c.StartMs < minCueMs {
			continue
		}
		out = append(out, c)
	}
	return out
}

func srtTime(ms int) string {
	return fmt.Sprintf("%02d:%02d:%02d,%03d", ms/3600000, ms/60000%60, ms/1000%60, ms%1000)
}

// SRT renders cues as a SubRip file.
func SRT(cues []Cue) string {
	var b strings.Builder
	for i, c := range cues {
		fmt.Fprintf(&b, "%d\n%s --> %s\n%s\n\n", i+1, srtTime(c.StartMs), srtTime(c.EndMs), c.Text)
	}
	return b.String()
}

func assTime(ms int) string {
	cs := ms / 10
	return fmt.Sprintf("%d:%02d:%02d.%02d", cs/360000, cs/6000%60, cs/100%60, cs%100)
}

// SubtitleFont is installed in the render image (deploy/Dockerfile.render)
// and covers Vietnamese diacritics.
const SubtitleFont = "Noto Sans"

// assEscape neutralises ASS override blocks and line-break escapes in user
// text: braces start override tags and a backslash starts \N, \h...
func assEscape(s string) string {
	r := strings.NewReplacer("\\", "⧵", "{", "(", "}", ")", "\r", "", "\n", " ")
	return r.Replace(s)
}

// ASS renders cues as an Advanced SubStation file styled for preset p
// (bottom-centred, outlined white text sized to the frame).
func ASS(cues []Cue, p Preset) string {
	fontSize := p.Height * 5 / 100
	if p.Width < p.Height {
		fontSize = p.Width * 6 / 100
	}
	margin := p.Height * 8 / 100
	var b strings.Builder
	fmt.Fprintf(&b, "[Script Info]\nScriptType: v4.00+\nPlayResX: %d\nPlayResY: %d\nWrapStyle: 0\nScaledBorderAndShadow: yes\n\n", p.Width, p.Height)
	b.WriteString("[V4+ Styles]\nFormat: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding\n")
	fmt.Fprintf(&b, "Style: Default,%s,%d,&H00FFFFFF,&H00FFFFFF,&H00000000,&H64000000,0,0,0,0,100,100,0,0,1,%d,0,2,%d,%d,%d,1\n\n",
		SubtitleFont, fontSize, max(fontSize/12, 2), p.Width/12, p.Width/12, margin)
	b.WriteString("[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n")
	for _, c := range cues {
		fmt.Fprintf(&b, "Dialogue: 0,%s,%s,Default,,0,0,0,,%s\n", assTime(c.StartMs), assTime(c.EndMs), assEscape(c.Text))
	}
	return b.String()
}
