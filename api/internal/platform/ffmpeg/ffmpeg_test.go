package ffmpeg

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/be/acs/api/internal/ports"
)

const sampleProbe = `{"streams":[{"index":0,"codec_type":"video","codec_name":"h264","width":1080,"height":1920,"pix_fmt":"yuv420p","avg_frame_rate":"30/1"},{"index":1,"codec_type":"audio","codec_name":"aac","sample_rate":"48000","channels":2}],"format":{"format_name":"mov,mp4,m4a,3gp,3g2,mj2","duration":"20.033000","size":"5242880","bit_rate":"2093000"}}`

func TestProbeParsesAndUsesArgVector(t *testing.T) {
	var gotBin string
	var gotArgs []string
	p := &Prober{Bin: "ffprobe", Run: func(_ context.Context, bin string, args ...string) ([]byte, error) {
		gotBin, gotArgs = bin, args
		return []byte(sampleProbe), nil
	}}
	info, err := p.Probe(context.Background(), "/tmp/a b; rm -rf.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if gotBin != "ffprobe" || gotArgs[len(gotArgs)-1] != "/tmp/a b; rm -rf.mp4" || gotArgs[len(gotArgs)-2] != "--" {
		t.Fatalf("unexpected invocation %s %v", gotBin, gotArgs)
	}
	v, ok := info.Video()
	if !ok || v.Width != 1080 || v.Height != 1920 || v.FPS != 30 || v.Codec != "h264" {
		t.Fatalf("video stream = %+v", v)
	}
	if a, ok := info.Audio(); !ok || a.SampleRate != 48000 {
		t.Fatalf("audio stream = %+v", a)
	}
	if info.Duration.Round(time.Millisecond) != 20033*time.Millisecond {
		t.Fatalf("duration = %v", info.Duration)
	}
}

func TestProbeRejectsOptionLikeSource(t *testing.T) {
	p := &Prober{Bin: "ffprobe", Run: func(context.Context, string, ...string) ([]byte, error) { return nil, nil }}
	if _, err := p.Probe(context.Background(), "-i"); err == nil {
		t.Fatal("expected error")
	}
}

func TestNormalizeArgs(t *testing.T) {
	args := NormalizeArgs(ports.NormalizeSpec{Input: "in.mp4", Output: "out.mp4", Format: ports.OutputFormat{Width: 1080, Height: 1920, FPS: 30}, DurationMs: 4000})
	joined := strings.Join(args, " ")
	for _, want := range []string{"libx264", "yuv420p", "-ar 48000", "anullsrc", "-t 4.000", "fps=30", "+faststart"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in %v", want, args)
		}
	}
	if args[len(args)-1] != "out.mp4" {
		t.Fatal("output must be last")
	}
}

func TestRenderArgsRejectsFilterInjection(t *testing.T) {
	_, err := RenderArgs(ports.RenderSpec{Clips: []string{"a.mp4"}, Subtitles: "x.srt:force_style='x'", BurnIn: true, Output: "o.mp4"})
	if err == nil {
		t.Fatal("expected rejection of metacharacters in subtitle path")
	}
	args, err := RenderArgs(ports.RenderSpec{
		Clips: []string{"a.mp4", "b.mp4"}, Output: "o.mp4", Subtitles: "/work/sub.srt", BurnIn: true,
		Voice: &ports.AudioTrack{Path: "v.mp3", Volume: 1}, Music: &ports.AudioTrack{Path: "m.mp3", Volume: 0.3, Duck: true},
		Format: ports.OutputFormat{Width: 1080, Height: 1080, FPS: 30},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(args, "-filter_complex") {
		t.Fatal("missing filter_complex")
	}
}
