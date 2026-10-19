package kenburns

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/be/acs/api/internal/platform/ffmpeg"
	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
	"github.com/be/acs/api/internal/testutil"
)

// fakeRenderer records the spec and writes a dummy output.
type fakeRenderer struct {
	spec ports.StillClipSpec
	err  error
}

func (f *fakeRenderer) Normalize(context.Context, ports.NormalizeSpec) error { return nil }
func (f *fakeRenderer) Render(context.Context, ports.RenderSpec) (ports.RenderResult, error) {
	return ports.RenderResult{}, nil
}
func (f *fakeRenderer) Thumbnail(context.Context, string, string, time.Duration) error { return nil }
func (f *fakeRenderer) StillToClip(_ context.Context, s ports.StillClipSpec) error {
	f.spec = s
	if f.err != nil {
		return f.err
	}
	return os.WriteFile(s.Output, []byte("\x00\x00\x00\x18ftypisom-clip"), 0o600)
}

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = 200
	}
	img.Set(1, 1, color.Black)
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestSubmitStatusDownloadWithStorage(t *testing.T) {
	r := &fakeRenderer{}
	a := New(r, t.TempDir())
	st := testutil.NewStorage()
	a.UseStorage(st)
	sub, err := a.Submit(context.Background(), ports.VideoRequest{AspectRatio: "16:9", Resolution: "1080p", DurationSec: 5,
		Image: &ports.MediaInput{MimeType: "image/png", Data: pngBytes(t, 8, 8)}})
	if err != nil {
		t.Fatal(err)
	}
	if r.spec.Format != (ports.OutputFormat{Width: 1920, Height: 1080, FPS: 30}) || r.spec.DurationMs != 5000 {
		t.Fatalf("%+v", r.spec)
	}
	if len(st.Objects) != 1 {
		t.Fatal("clip must be parked in storage")
	}
	s, err := a.GetStatus(context.Background(), sub.ProviderJobID)
	if err != nil || s.State != ports.VideoSucceeded || s.Usage.Units["video_second_1080p"] != 5 {
		t.Fatalf("%v %+v", err, s)
	}
	ms, err := a.Download(context.Background(), sub.ProviderJobID)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(ms.Body)
	if !bytes.Contains(b, []byte("ftyp")) {
		t.Fatal("download content")
	}
	if err := a.Cancel(context.Background(), sub.ProviderJobID); err != nil || len(st.Objects) != 0 {
		t.Fatal("cancel discards the clip")
	}
	if s, _ := a.GetStatus(context.Background(), sub.ProviderJobID); s.State != ports.VideoFailed {
		t.Fatalf("%+v", s)
	}
}

func TestSubmitValidation(t *testing.T) {
	a := New(&fakeRenderer{}, t.TempDir())
	img := &ports.MediaInput{MimeType: "image/png", Data: pngBytes(t, 4, 4)}
	cases := []ports.VideoRequest{
		{AspectRatio: "4:5", Image: img},
		{Resolution: "4k", Image: img},
		{DurationSec: 12, Image: img},
		{Prompt: "text only"},
		{Image: &ports.MediaInput{MimeType: "image/gif", Data: []byte("GIF89a")}},
	}
	for i, c := range cases {
		if _, err := a.Submit(context.Background(), c); !errors.Is(err, kernel.ErrCapability) {
			t.Errorf("case %d: %v", i, err)
		}
	}
	if _, err := a.GetStatus(context.Background(), "kb|1080p|4|../../etc/passwd"); err == nil {
		t.Fatal("malformed id must fail")
	}
	bad := New(&fakeRenderer{err: errors.New("exit 1")}, t.TempDir())
	_, err := bad.Submit(context.Background(), ports.VideoRequest{Image: img})
	if pe, ok := ports.AsProviderError(err); !ok || pe.Retryable() {
		t.Fatalf("render failure must not be retried: %v", err)
	}
}

// TestRealFFmpeg renders an actual clip when ffmpeg is installed (local
// fallback store, no storage).
func TestRealFFmpeg(t *testing.T) {
	bin, err := exec.LookPath("ffmpeg")
	if err != nil || testing.Short() {
		t.Skip("ffmpeg not installed")
	}
	a := New(ffmpeg.NewRenderer(bin), t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	sub, err := a.Submit(ctx, ports.VideoRequest{AspectRatio: "1:1", Resolution: "720p", DurationSec: 2,
		Image: &ports.MediaInput{MimeType: "image/png", Data: pngBytes(t, 64, 64)}})
	if err != nil {
		t.Fatal(err)
	}
	ms, err := a.Download(ctx, sub.ProviderJobID)
	if err != nil {
		t.Fatal(err)
	}
	defer ms.Body.Close()
	b, _ := io.ReadAll(ms.Body)
	if len(b) < 1000 || !bytes.Contains(b[:64], []byte("ftyp")) {
		t.Fatalf("not an mp4 (%d bytes)", len(b))
	}
}
