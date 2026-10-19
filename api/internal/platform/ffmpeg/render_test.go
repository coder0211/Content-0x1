package ffmpeg

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/be/acs/api/internal/ports"
)

func TestRenderArgsCrossfadeOffsetsAndInputsOrder(t *testing.T) {
	args, err := RenderArgs(ports.RenderSpec{
		Clips: []string{"a.mp4", "b.mp4", "c.mp4"}, ClipDurationsMs: []int{4000, 3000, 5000},
		Transition: "fade", TransitionMs: 300, Output: "o.mp4", Subtitles: "/w/s.srt",
		VoiceSegments: []ports.VoiceSegment{{Path: "v1.wav", StartMs: 0, MaxMs: 4000}, {Path: "v2.wav", StartMs: 3700, MaxMs: 3000}},
		Music:         &ports.AudioTrack{Path: "m.mp3", Volume: 0.3, Duck: true},
		Format:        ports.OutputFormat{Width: 1080, Height: 1920, FPS: 30},
	})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"xfade=transition=fade:duration=0.300:offset=3.700", "xfade=transition=fade:duration=0.300:offset=6.400",
		"acrossfade=d=0.300", "adelay=3700|3700", "atrim=0:3.000", "sidechaincompress", "-c:s mov_text", "-r 30", "+faststart",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in\n%s", want, joined)
		}
	}
	// Every input precedes the output options; the subtitle input is last.
	lastInput := 0
	for i, a := range args {
		if a == "-i" {
			lastInput = i
		}
	}
	if args[lastInput+1] != "/w/s.srt" || slices.Index(args, "-filter_complex") < lastInput {
		t.Fatalf("inputs must precede output options: %v", args)
	}
	if args[len(args)-1] != "o.mp4" {
		t.Fatal("output must be last")
	}
}

func TestRenderArgsRejectsMetacharactersInFontsDir(t *testing.T) {
	if _, err := RenderArgs(ports.RenderSpec{Clips: []string{"a"}, Subtitles: "/w/s.ass", BurnIn: true, FontsDir: "/f;x", Output: "o"}); err == nil {
		t.Fatal("expected error")
	}
}

func solidPNG(t *testing.T, path string, c color.RGBA) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 320, 320))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, 255
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func sineWAV(t *testing.T, path string, ms int) {
	t.Helper()
	const rate = 16000
	n := rate * ms / 1000
	var b bytes.Buffer
	w := func(v any) { _ = binary.Write(&b, binary.LittleEndian, v) }
	b.WriteString("RIFF")
	w(uint32(36 + n*2))
	b.WriteString("WAVEfmt ")
	w(uint32(16))
	w(uint16(1))
	w(uint16(1))
	w(uint32(rate))
	w(uint32(rate * 2))
	w(uint16(2))
	w(uint16(16))
	b.WriteString("data")
	w(uint32(n * 2))
	for i := range n {
		w(int16(8000 * math.Sin(2*math.Pi*440*float64(i)/rate)))
	}
	if err := os.WriteFile(path, b.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func hasFilter(name string) bool {
	out, err := exec.Command("ffmpeg", "-hide_banner", "-filters").Output()
	return err == nil && strings.Contains(string(out), " "+name+" ")
}

// TestRenderWithRealFFmpeg renders a 3-scene video for every preset with
// cross-fades, a segmented voice-over, ducked music and Vietnamese subtitles,
// then verifies codec/resolution/duration with ffprobe (ACS-066/067).
func TestRenderWithRealFFmpeg(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not installed")
	}
	if testing.Short() {
		t.Skip("short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	r, p := NewRenderer("ffmpeg"), NewProber("ffprobe")
	dir := t.TempDir()
	img := filepath.Join(dir, "frame.png")
	solidPNG(t, img, color.RGBA{R: 20, G: 90, B: 200})
	voice1, voice2, music := filepath.Join(dir, "v1.wav"), filepath.Join(dir, "v2.wav"), filepath.Join(dir, "m.wav")
	sineWAV(t, voice1, 1500)
	sineWAV(t, voice2, 2500)
	sineWAV(t, music, 1000)
	ass := filepath.Join(dir, "subs.ass")
	srt := filepath.Join(dir, "subs.srt")
	if err := os.WriteFile(srt, []byte("1\n00:00:00,000 --> 00:00:01,500\nKhuyến mãi đặc biệt hôm nay\n\n2\n00:00:01,700 --> 00:00:03,000\nĐăng ký ngay để nhận ưu đãi\n\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ass, []byte("[Script Info]\nScriptType: v4.00+\nPlayResX: 360\nPlayResY: 640\n\n[V4+ Styles]\nFormat: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding\nStyle: Default,Noto Sans,24,&H00FFFFFF,&H00FFFFFF,&H00000000,&H64000000,0,0,0,0,100,100,0,0,1,2,0,2,20,20,40,1\n\n[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\nDialogue: 0,0:00:00.00,0:00:01.50,Default,,0,0,0,,Khuyến mãi đặc biệt hôm nay\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	burn := hasFilter("subtitles")
	// Small frames keep the test fast; presets only change the numbers.
	for _, f := range []ports.OutputFormat{{Width: 360, Height: 640, FPS: 30}, {Width: 360, Height: 360, FPS: 30}, {Width: 640, Height: 360, FPS: 30}} {
		durs := []int{2000, 2500, 2000}
		var clips []string
		for i, d := range durs {
			still := filepath.Join(dir, "still.mp4")
			if err := r.StillToClip(ctx, ports.StillClipSpec{Image: img, Output: still, Format: f, DurationMs: d + 500}); err != nil {
				t.Fatal(err)
			}
			out := filepath.Join(dir, "clip"+string(rune('a'+i))+".mp4")
			if err := r.Normalize(ctx, ports.NormalizeSpec{Input: still, Output: out, Format: f, DurationMs: d}); err != nil {
				t.Fatal(err)
			}
			clips = append(clips, out)
		}
		spec := ports.RenderSpec{
			WorkDir: dir, Clips: clips, ClipDurationsMs: durs, Transition: "fade", TransitionMs: 300, Format: f,
			VoiceSegments: []ports.VoiceSegment{{Path: voice1, StartMs: 0, MaxMs: 2000}, {Path: voice2, StartMs: 1700, MaxMs: 2500}},
			Voice:         &ports.AudioTrack{Volume: 1}, Music: &ports.AudioTrack{Path: music, Volume: 0.3, Duck: true},
			Subtitles: srt, Output: filepath.Join(dir, "final.mp4"),
		}
		if burn {
			spec.Subtitles, spec.BurnIn = ass, true
		}
		if _, err := r.Render(ctx, spec); err != nil {
			t.Fatal(err)
		}
		info, err := p.Probe(ctx, spec.Output)
		if err != nil {
			t.Fatal(err)
		}
		v, ok := info.Video()
		if !ok || v.Codec != "h264" || v.Width != f.Width || v.Height != f.Height || v.PixFmt != "yuv420p" {
			t.Fatalf("video stream %+v", v)
		}
		if a, ok := info.Audio(); !ok || a.Codec != "aac" || a.SampleRate != 48000 {
			t.Fatalf("audio stream %+v", a)
		}
		want := time.Duration(2000+2500+2000-2*300) * time.Millisecond
		if d := info.Duration - want; d > 500*time.Millisecond || d < -500*time.Millisecond {
			t.Fatalf("duration %v, want %v ±0.5s", info.Duration, want)
		}
		if !strings.Contains(info.FormatName, "mp4") {
			t.Fatalf("format %s", info.FormatName)
		}
		thumb := filepath.Join(dir, "thumb.jpg")
		if err := r.Thumbnail(ctx, spec.Output, thumb, time.Second); err != nil {
			t.Fatal(err)
		}
		if st, err := os.Stat(thumb); err != nil || st.Size() == 0 {
			t.Fatal("thumbnail missing")
		}
	}
}
