// Package fake is a deterministic AI provider (code "fake") for local runs
// and tests: text (schema-shaped JSON), images (solid PNG), video (async
// submit/poll) and voice (silent WAV with word timings). Failures and delays
// are injectable via Hooks.
//
// Minimal implementation by Agent 1; the providers agent extends it (sample
// media, configurable scenarios) without changing the exported API.
package fake

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/be/acs/api/internal/ports"
)

// Code is the provider code seeded in migration 0011.
const Code = "fake"

// Hooks customise behaviour; Fail returns an error to inject for an
// operation ("text", "image", "image_edit", "video_submit", "video_status",
// "voice") on its n-th call (1-based), or nil.
type Hooks struct {
	Fail  func(op string, call int) error
	Delay time.Duration
}

// Provider implements every AI port.
type Provider struct {
	Hooks Hooks

	mu     sync.Mutex
	calls  map[string]int
	videos map[string]ports.VideoRequest
}

// New returns a fake provider.
func New() *Provider {
	return &Provider{calls: map[string]int{}, videos: map[string]ports.VideoRequest{}}
}

// Text, Image, Video and Voice are the per-capability adapters sharing one
// Provider (TextGenerator and ImageGenerator both declare Generate, so a
// vendor offering both exposes two adapter types).
type (
	Text  struct{ *Provider }
	Image struct{ *Provider }
	Video struct{ *Provider }
	Voice struct{ *Provider }
)

var (
	_ ports.TextGenerator    = Text{}
	_ ports.ImageGenerator   = Image{}
	_ ports.VideoGenerator   = Video{}
	_ ports.VoiceSynthesizer = Voice{}
)

// Adapters returns every capability adapter for registry registration.
func (p *Provider) Adapters() []any { return []any{Text{p}, Image{p}, Video{p}, Voice{p}} }

// Code implements the ports.
func (*Provider) Code() string { return Code }

// Calls returns how many times op was invoked.
func (p *Provider) Calls(op string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls[op]
}

func (p *Provider) enter(ctx context.Context, op string) error {
	p.mu.Lock()
	p.calls[op]++
	n := p.calls[op]
	p.mu.Unlock()
	if p.Hooks.Delay > 0 {
		select {
		case <-time.After(p.Hooks.Delay):
		case <-ctx.Done():
			return &ports.ProviderError{Kind: ports.ErrKindTransient, Provider: Code, Code: "timeout", Message: ctx.Err().Error(), Err: ctx.Err()}
		}
	}
	if p.Hooks.Fail != nil {
		return p.Hooks.Fail(op, n)
	}
	return nil
}

// Capabilities mirrors the seeded ai_models.capabilities.
func (*Provider) Capabilities(model string) ports.Capabilities {
	switch {
	case strings.Contains(model, "image"):
		return ports.Capabilities{AspectRatios: []string{"1:1", "4:5", "9:16", "16:9"}, Qualities: []string{"standard", "high"}, MaxVariants: 4, SupportsRefImage: true, SupportsEdit: true}
	case strings.Contains(model, "video"):
		return ports.Capabilities{AspectRatios: []string{"9:16", "1:1", "16:9"}, Resolutions: []string{"720p", "1080p"}, MinDurationSec: 2, MaxDurationSec: 8, SupportsRefImage: true, SupportsCancel: true}
	case strings.Contains(model, "voice"):
		return ports.Capabilities{Languages: []string{"vi-VN", "en-US"}, Voices: []string{"vi-VN-female-1", "vi-VN-male-1"}}
	}
	return ports.Capabilities{Languages: []string{"vi", "en"}, MaxOutputTokens: 4096}
}

// Generate returns deterministic text; with a JSON schema it returns an
// object populated from the schema's properties.
func (p Text) Generate(ctx context.Context, req ports.TextRequest) (ports.TextResponse, error) {
	if err := p.enter(ctx, "text"); err != nil {
		return ports.TextResponse{}, err
	}
	in := len(req.System)
	for _, m := range req.Messages {
		in += len(m.Content)
	}
	resp := ports.TextResponse{Content: "[fake] Nội dung mẫu do fake provider tạo.", StopReason: "end_turn"}
	if len(req.JSONSchema) > 0 {
		var schema map[string]any
		if err := json.Unmarshal(req.JSONSchema, &schema); err != nil {
			return ports.TextResponse{}, &ports.ProviderError{Kind: ports.ErrKindInvalidRequest, Provider: Code, Code: "bad_schema", Message: err.Error()}
		}
		b, _ := json.Marshal(sample(schema, 0))
		resp.JSON, resp.Content = b, string(b)
	}
	resp.Usage = ports.TextUsage(int64(in/4+1), int64(len(resp.Content)/4+1), 0)
	resp.Raw = json.RawMessage(`{"provider":"fake"}`)
	return resp, nil
}

// sample builds a value conforming to a (simple) JSON schema.
func sample(s map[string]any, depth int) any {
	if depth > 6 {
		return nil
	}
	if enum, ok := s["enum"].([]any); ok && len(enum) > 0 {
		return enum[0]
	}
	switch s["type"] {
	case "object":
		out := map[string]any{}
		props, _ := s["properties"].(map[string]any)
		for k, v := range props {
			if ps, ok := v.(map[string]any); ok {
				out[k] = sample(ps, depth+1)
			}
		}
		return out
	case "array":
		items, _ := s["items"].(map[string]any)
		n := 1
		if min, ok := s["minItems"].(float64); ok && int(min) > n {
			n = int(min)
		}
		arr := make([]any, 0, n)
		for i := 0; i < n; i++ {
			arr = append(arr, sample(items, depth+1))
		}
		return arr
	case "integer":
		return 1
	case "number":
		return 1.0
	case "boolean":
		return false
	case "string":
		return "Nội dung mẫu"
	}
	return nil
}

func ratioSize(ar string) (int, int) {
	switch ar {
	case "4:5":
		return 819, 1024
	case "9:16":
		return 576, 1024
	case "16:9":
		return 1024, 576
	}
	return 1024, 1024
}

func solidPNG(w, h int, c color.RGBA) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

func images(count int, ar, quality string) ([]ports.GeneratedImage, ports.Usage) {
	if count <= 0 {
		count = 1
	}
	w, h := ratioSize(ar)
	out := make([]ports.GeneratedImage, 0, count)
	for i := 0; i < count; i++ {
		c := color.RGBA{R: uint8(40 + 50*i), G: 85, B: 255, A: 255}
		out = append(out, ports.GeneratedImage{MimeType: "image/png", Data: solidPNG(w, h, c), Width: w, Height: h})
	}
	unit := "image_1024_standard"
	if quality == "high" {
		unit = "image_1024_high"
	}
	return out, ports.Usage{Images: count, Units: map[string]float64{unit: float64(count)}}
}

// Generate implements ports.ImageGenerator.
func (p Image) Generate(ctx context.Context, req ports.ImageRequest) (ports.ImageResponse, error) {
	if err := p.enter(ctx, "image"); err != nil {
		return ports.ImageResponse{}, err
	}
	imgs, u := images(req.Count, req.AspectRatio, req.Quality)
	return ports.ImageResponse{Images: imgs, Usage: u, Raw: json.RawMessage(`{"provider":"fake"}`)}, nil
}

// Edit implements ports.ImageGenerator.
func (p Image) Edit(ctx context.Context, req ports.ImageEditRequest) (ports.ImageResponse, error) {
	if err := p.enter(ctx, "image_edit"); err != nil {
		return ports.ImageResponse{}, err
	}
	imgs, u := images(req.Count, req.AspectRatio, req.Quality)
	return ports.ImageResponse{Images: imgs, Usage: u, Raw: json.RawMessage(`{"provider":"fake"}`)}, nil
}

// Submit implements ports.VideoGenerator.
func (p Video) Submit(ctx context.Context, req ports.VideoRequest) (ports.VideoSubmission, error) {
	if err := p.enter(ctx, "video_submit"); err != nil {
		return ports.VideoSubmission{}, err
	}
	p.mu.Lock()
	id := fmt.Sprintf("fakevid-%d", len(p.videos)+1)
	p.videos[id] = req
	p.mu.Unlock()
	return ports.VideoSubmission{ProviderJobID: id, PollAfter: 10 * time.Millisecond}, nil
}

// GetStatus implements ports.VideoGenerator: clips succeed on first poll.
func (p Video) GetStatus(ctx context.Context, id string) (ports.VideoStatus, error) {
	if err := p.enter(ctx, "video_status"); err != nil {
		return ports.VideoStatus{}, err
	}
	p.mu.Lock()
	req, ok := p.videos[id]
	p.mu.Unlock()
	if !ok {
		return ports.VideoStatus{}, &ports.ProviderError{Kind: ports.ErrKindInvalidRequest, Provider: Code, Code: "not_found", Message: id}
	}
	res := req.Resolution
	if res == "" {
		res = "720p"
	}
	u := ports.Usage{VideoSeconds: float64(req.DurationSec), Units: map[string]float64{"video_second_" + res: float64(req.DurationSec)}}
	return ports.VideoStatus{State: ports.VideoSucceeded, Progress: 100, DurationSec: float64(req.DurationSec), Usage: &u}, nil
}

// Download implements ports.VideoGenerator. The fake returns a placeholder
// byte stream (not a playable MP4); the providers agent swaps in a real
// sample clip.
func (p Video) Download(_ context.Context, id string) (ports.MediaStream, error) {
	body := []byte("\x00\x00\x00\x18ftypisom" + id)
	return ports.MediaStream{Body: io.NopCloser(bytes.NewReader(body)), MimeType: "video/mp4", Size: int64(len(body))}, nil
}

// Cancel implements ports.VideoGenerator.
func (p Video) Cancel(_ context.Context, id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.videos, id)
	return nil
}

// Synthesize implements ports.VoiceSynthesizer: silent 16 kHz mono WAV,
// ~70 ms per character, with evenly spaced word timings.
func (p Voice) Synthesize(ctx context.Context, req ports.VoiceRequest) (ports.VoiceResponse, error) {
	if err := p.enter(ctx, "voice"); err != nil {
		return ports.VoiceResponse{}, err
	}
	chars := len([]rune(req.Text))
	durMs := chars*70 + 200
	words := strings.Fields(req.Text)
	timings := make([]ports.WordTiming, 0, len(words))
	if len(words) > 0 {
		step := durMs / len(words)
		for i, w := range words {
			timings = append(timings, ports.WordTiming{Text: w, StartMs: i * step, EndMs: (i + 1) * step})
		}
	}
	return ports.VoiceResponse{
		MimeType: "audio/wav", Audio: silentWAV(durMs), DurationMs: durMs, Timings: timings,
		Usage: ports.Usage{Characters: chars, Units: map[string]float64{"char": float64(chars)}},
		Raw:   json.RawMessage(`{"provider":"fake"}`),
	}, nil
}

func silentWAV(ms int) []byte {
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
	b.Write(make([]byte, n*2))
	return b.Bytes()
}
