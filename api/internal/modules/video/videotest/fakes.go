// Package videotest holds in-memory fakes used by the video module's own
// tests (files backed by the real file use case, FFmpeg renderer/prober and
// AI providers). Video publishes no API, so there is no API fake.
package videotest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	fileuc "github.com/be/acs/api/internal/modules/file/usecase"
	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
	"github.com/be/acs/api/internal/testutil"
)

// FileRepo is an in-memory file usecase.Repository.
type FileRepo struct {
	mu    sync.Mutex
	files map[kernel.ID]fileuc.File
}

var _ fileuc.Repository = (*FileRepo)(nil)

func (r *FileRepo) Create(_ context.Context, f *fileuc.File) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.files[f.ID] = *f
	return nil
}

func (r *FileRepo) Get(_ context.Context, id kernel.ID) (*fileuc.File, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	f, ok := r.files[id]
	if !ok {
		return nil, kernel.ErrNotFound
	}
	return &f, nil
}

func (r *FileRepo) GetMany(ctx context.Context, ids []kernel.ID) ([]fileuc.File, error) {
	var out []fileuc.File
	for _, id := range ids {
		if f, err := r.Get(ctx, id); err == nil {
			out = append(out, *f)
		}
	}
	return out, nil
}

func (r *FileRepo) MarkReady(context.Context, kernel.ID, fileuc.Verified) error { return nil }
func (r *FileRepo) MarkRejected(context.Context, kernel.ID, string) error       { return nil }
func (r *FileRepo) ListStalePending(context.Context, time.Time, int) ([]fileuc.StaleFile, error) {
	return nil, nil
}
func (r *FileRepo) Delete(_ context.Context, id kernel.ID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.files, id)
	return nil
}

// Count returns the number of stored files whose MIME type starts with prefix.
func (r *FileRepo) Count(prefix string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, f := range r.files {
		if strings.HasPrefix(f.MimeType, prefix) {
			n++
		}
	}
	return n
}

// Files is the real file use case on in-memory storage.
type Files struct {
	*fileuc.Service
	Repo    *FileRepo
	Storage *testutil.Storage
}

// NewFiles returns a file service backed by memory.
func NewFiles(clock ports.Clock) *Files {
	repo := &FileRepo{files: map[kernel.ID]fileuc.File{}}
	st := testutil.NewStorage()
	return &Files{Service: &fileuc.Service{Repo: repo, Storage: st, Clock: clock, URLTTL: 15 * time.Minute}, Repo: repo, Storage: st}
}

// AddImage stores a ready PNG owned by owner (a scene source image).
func (f *Files) AddImage(ctx context.Context, owner kernel.ID) kernel.ID {
	file, err := f.Store(ctx, fileuc.StoreInput{Body: bytes.NewReader([]byte("\x89PNG fake")), Size: 9, MimeType: "image/png",
		Kind: "image", Source: "generated", CreatedBy: &owner})
	if err != nil {
		panic(err)
	}
	return file.ID
}

// AddAudio stores a ready audio file (a brand music track).
func (f *Files) AddAudio(ctx context.Context) kernel.ID {
	file, err := f.Store(ctx, fileuc.StoreInput{Body: bytes.NewReader([]byte("RIFFfake")), Size: 8, MimeType: "audio/mpeg",
		Kind: "audio", Source: "generated"})
	if err != nil {
		panic(err)
	}
	return file.ID
}

// fakeMedia is what the fake renderer writes and the fake prober reads.
type fakeMedia struct {
	Codec  string `json:"codec"`
	Width  int    `json:"w"`
	Height int    `json:"h"`
	Ms     int    `json:"ms"`
}

// Renderer is a ports.VideoRenderer that writes small JSON "media" files.
// CorruptRenders makes the next N renders unreadable (quality check tests).
type Renderer struct {
	mu             sync.Mutex
	CorruptRenders int
	FailNormalize  bool
	Renders        []ports.RenderSpec
	Normalized     int
	Stills         int
}

var _ ports.VideoRenderer = (*Renderer)(nil)

func writeMedia(path string, m fakeMedia) error {
	b, _ := json.Marshal(m)
	return os.WriteFile(path, b, 0o600)
}

func (r *Renderer) Normalize(_ context.Context, s ports.NormalizeSpec) error {
	r.mu.Lock()
	r.Normalized++
	fail := r.FailNormalize
	r.mu.Unlock()
	if fail {
		return errors.New("ffmpeg: invalid data found when processing input")
	}
	return writeMedia(s.Output, fakeMedia{Codec: "h264", Width: s.Format.Width, Height: s.Format.Height, Ms: s.DurationMs})
}

func (r *Renderer) StillToClip(_ context.Context, s ports.StillClipSpec) error {
	r.mu.Lock()
	r.Stills++
	r.mu.Unlock()
	return writeMedia(s.Output, fakeMedia{Codec: "h264", Width: s.Format.Width, Height: s.Format.Height, Ms: s.DurationMs})
}

func (r *Renderer) Render(_ context.Context, s ports.RenderSpec) (ports.RenderResult, error) {
	r.mu.Lock()
	r.Renders = append(r.Renders, s)
	corrupt := r.CorruptRenders > 0
	if corrupt {
		r.CorruptRenders--
	}
	r.mu.Unlock()
	for _, c := range s.Clips {
		if _, err := os.Stat(c); err != nil {
			return ports.RenderResult{}, err
		}
	}
	if corrupt {
		return ports.RenderResult{Output: s.Output}, os.WriteFile(s.Output, []byte("garbage"), 0o600)
	}
	total := 0
	for _, d := range s.ClipDurationsMs {
		total += d
	}
	if len(s.ClipDurationsMs) > 1 {
		total -= (len(s.ClipDurationsMs) - 1) * s.TransitionMs
	}
	return ports.RenderResult{Output: s.Output, Duration: time.Duration(total) * time.Millisecond},
		writeMedia(s.Output, fakeMedia{Codec: "h264", Width: s.Format.Width, Height: s.Format.Height, Ms: total})
}

func (r *Renderer) Thumbnail(_ context.Context, _, output string, _ time.Duration) error {
	return os.WriteFile(output, []byte("\xff\xd8\xffjpeg"), 0o600)
}

// RenderCount returns how many renders ran.
func (r *Renderer) RenderCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.Renders)
}

// Prober reads the fake media files written by Renderer.
type Prober struct{}

func (Prober) Probe(_ context.Context, src string) (ports.MediaInfo, error) {
	b, err := os.ReadFile(src) //nolint:gosec // test fake
	if err != nil {
		return ports.MediaInfo{}, err
	}
	var m fakeMedia
	if err := json.Unmarshal(b, &m); err != nil {
		return ports.MediaInfo{}, fmt.Errorf("ffprobe: invalid data: %w", err)
	}
	return ports.MediaInfo{FormatName: "mov,mp4,m4a,3gp,3g2,mj2", Duration: time.Duration(m.Ms) * time.Millisecond, Streams: []ports.StreamInfo{
		{Index: 0, Type: "video", Codec: m.Codec, Width: m.Width, Height: m.Height, FPS: 30, PixFmt: "yuv420p"},
		{Index: 1, Type: "audio", Codec: "aac", SampleRate: 48000, Channels: 2},
	}}, nil
}

// AI is a ports.AIProviders with scriptable text, video and voice fakes.
type AI struct {
	mu sync.Mutex
	// FailVideo returns a terminal provider error for a clip prompt.
	FailVideo func(prompt string) *ports.ProviderError
	// PollsBeforeDone is how many GetStatus calls report "running" first.
	PollsBeforeDone int
	TextCalls       []ports.TextRequest
	Submits         []ports.VideoRequest
	VoiceCalls      []ports.VoiceRequest
	polls           map[string]int
	jobs            map[string]ports.VideoRequest
}

// NewAI returns the fake providers.
func NewAI() *AI { return &AI{polls: map[string]int{}, jobs: map[string]ports.VideoRequest{}} }

var _ ports.AIProviders = (*AI)(nil)

func (a *AI) Text(string) (ports.TextGenerator, error)     { return text{a}, nil }
func (a *AI) Image(string) (ports.ImageGenerator, error)   { return nil, errors.New("no image") }
func (a *AI) Video(string) (ports.VideoGenerator, error)   { return video{a}, nil }
func (a *AI) Voice(string) (ports.VoiceSynthesizer, error) { return voice{a}, nil }

// SubmitCount returns how many clips were submitted.
func (a *AI) SubmitCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.Submits)
}

type text struct{ a *AI }

func (text) Code() string { return "fake" }

func (t text) Generate(_ context.Context, req ports.TextRequest) (ports.TextResponse, error) {
	t.a.mu.Lock()
	t.a.TextCalls = append(t.a.TextCalls, req)
	t.a.mu.Unlock()
	var out any
	switch req.SchemaName {
	case "video_script":
		out = map[string]any{"title": "Ưu đãi mùa thu", "script": "Mùa thu này, học tiếng Anh cùng Sao Mai. Lớp nhỏ, giáo viên tận tâm. Đăng ký tư vấn ngay hôm nay."}
	default:
		var schema struct {
			Properties struct {
				Scenes struct {
					MinItems int `json:"minItems"`
				} `json:"scenes"`
			} `json:"properties"`
		}
		_ = json.Unmarshal(req.JSONSchema, &schema)
		n := max(schema.Properties.Scenes.MinItems, 1)
		scenes := make([]map[string]any, n)
		for i := range scenes {
			scenes[i] = map[string]any{
				"visual_prompt": fmt.Sprintf("Cảnh %d: lớp học sáng sủa, học viên cười", i+1), "narration": fmt.Sprintf("Lời thoại cảnh %d rất ngắn gọn.", i+1),
				"subtitle_text": fmt.Sprintf("Phụ đề cảnh %d", i+1), "duration_seconds": 5,
			}
		}
		out = map[string]any{"scenes": scenes}
	}
	b, _ := json.Marshal(out)
	return ports.TextResponse{JSON: b, Content: string(b), Usage: ports.TextUsage(500, 300, 0)}, nil
}

type video struct{ a *AI }

func (video) Code() string                           { return "fake" }
func (video) Capabilities(string) ports.Capabilities { return ports.Capabilities{} }

func (v video) Submit(_ context.Context, req ports.VideoRequest) (ports.VideoSubmission, error) {
	v.a.mu.Lock()
	defer v.a.mu.Unlock()
	v.a.Submits = append(v.a.Submits, req)
	id := fmt.Sprintf("vid-%d", len(v.a.Submits))
	v.a.jobs[id] = req
	return ports.VideoSubmission{ProviderJobID: id, PollAfter: time.Millisecond}, nil
}

func (v video) GetStatus(_ context.Context, id string) (ports.VideoStatus, error) {
	v.a.mu.Lock()
	defer v.a.mu.Unlock()
	req, ok := v.a.jobs[id]
	if !ok {
		return ports.VideoStatus{}, &ports.ProviderError{Kind: ports.ErrKindInvalidRequest, Provider: "fake", Code: "not_found"}
	}
	v.a.polls[id]++
	if v.a.polls[id] <= v.a.PollsBeforeDone {
		return ports.VideoStatus{State: ports.VideoRunning, Progress: 50}, nil
	}
	if v.a.FailVideo != nil {
		if pe := v.a.FailVideo(req.Prompt); pe != nil {
			return ports.VideoStatus{State: ports.VideoFailed, Error: pe}, nil
		}
	}
	u := ports.Usage{VideoSeconds: float64(req.DurationSec), Units: map[string]float64{"video_second_" + req.Resolution: float64(req.DurationSec)}}
	return ports.VideoStatus{State: ports.VideoSucceeded, Progress: 100, DurationSec: float64(req.DurationSec), Usage: &u}, nil
}

func (video) Download(_ context.Context, id string) (ports.MediaStream, error) {
	return ports.MediaStream{Body: io.NopCloser(strings.NewReader("raw clip " + id)), MimeType: "video/mp4", Size: -1}, nil
}

func (video) Cancel(context.Context, string) error { return nil }

type voice struct{ a *AI }

func (voice) Code() string                           { return "fake" }
func (voice) Capabilities(string) ports.Capabilities { return ports.Capabilities{} }

func (v voice) Synthesize(_ context.Context, req ports.VoiceRequest) (ports.VoiceResponse, error) {
	v.a.mu.Lock()
	v.a.VoiceCalls = append(v.a.VoiceCalls, req)
	v.a.mu.Unlock()
	words := strings.Fields(req.Text)
	timings := make([]ports.WordTiming, len(words))
	for i, w := range words {
		timings[i] = ports.WordTiming{Text: w, StartMs: i * 300, EndMs: (i + 1) * 300}
	}
	chars := len([]rune(req.Text))
	return ports.VoiceResponse{MimeType: "audio/wav", Audio: []byte("RIFFvoice"), DurationMs: len(words) * 300, Timings: timings,
		Usage: ports.Usage{Characters: chars, Units: map[string]float64{"char": float64(chars)}}}, nil
}
