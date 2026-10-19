// Package fake is a deterministic AI provider (code "fake") so the whole
// product runs locally and in CI without vendor keys (ACS-014):
//
//   - text: schema-shaped JSON (or a configured sample text),
//   - image: real PNGs with the requested aspect ratio,
//   - video: asynchronous submit/poll returning a real playable H.264/AAC MP4,
//   - voice: a real 16 kHz mono WAV with word timings.
//
// Behaviour is configurable at three levels:
//
//  1. Scenario — from ai_providers.config_json of the "fake" row (Configure),
//     e.g. {"delay_ms":800,"video_ready_after_ms":5000,
//     "fail":{"image":{"kind":"RateLimited","times":2,"retry_after_sec":3}}}.
//  2. Prompt markers — "[fake:policy]", "[fake:ratelimit]", "[fake:transient]",
//     "[fake:invalid]", "[fake:auth]" anywhere in the prompt/brief force that
//     failure (video "[fake:policy]" fails asynchronously, like Veo).
//  3. Hooks — programmatic injection for unit tests.
//
// Video provider job IDs are self-describing, so polling survives process
// restarts and works across worker processes.
package fake

import (
	"bytes"
	"context"
	"embed"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/providers/vendorhttp"
)

// Code is the provider code seeded in migration 0011.
const Code = "fake"

//go:embed samples/*.mp4
var samples embed.FS

// Hooks customise behaviour in tests; Fail returns an error to inject for
// an operation ("text", "image", "image_edit", "video_submit",
// "video_status", "voice") on its n-th call (1-based), or nil.
type Hooks struct {
	Fail  func(op string, call int) error
	Delay time.Duration
}

// FailSpec injects a ProviderError for one operation.
type FailSpec struct {
	Kind          ports.ErrKind `json:"kind"`
	Code          string        `json:"code,omitempty"`
	Message       string        `json:"message,omitempty"`
	Times         int           `json:"times,omitempty"` // fail the first N calls; 0 = always
	RetryAfterSec float64       `json:"retry_after_sec,omitempty"`
}

// Scenario is the config_json of the fake provider row.
type Scenario struct {
	DelayMs           int                 `json:"delay_ms,omitempty"`
	VideoReadyAfterMs int                 `json:"video_ready_after_ms,omitempty"`
	Text              string              `json:"text,omitempty"`
	Fail              map[string]FailSpec `json:"fail,omitempty"`
}

// Provider implements every AI port.
type Provider struct {
	Hooks Hooks
	// Now is the clock (tests).
	Now func() time.Time

	mu        sync.Mutex
	scenario  Scenario
	calls     map[string]int
	seq       int
	cancelled map[string]bool
}

// New returns a fake provider.
func New() *Provider {
	return &Provider{Now: time.Now, calls: map[string]int{}, cancelled: map[string]bool{}}
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

// Configure applies ai_providers.config_json (invalid JSON is ignored and
// resets to defaults). secretRef is unused: the fake needs no key.
func (p *Provider) Configure(_ string, cfg json.RawMessage) {
	var s Scenario
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &s); err != nil {
			s = Scenario{}
		}
	}
	p.mu.Lock()
	p.scenario = s
	p.calls = map[string]int{}
	p.mu.Unlock()
}

// Calls returns how many times op was invoked.
func (p *Provider) Calls(op string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls[op]
}

func (p *Provider) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

// markers maps prompt markers to injected failures.
var markers = map[string]ports.ErrKind{
	"[fake:policy]":    ports.ErrKindContentPolicy,
	"[fake:ratelimit]": ports.ErrKindRateLimited,
	"[fake:transient]": ports.ErrKindTransient,
	"[fake:invalid]":   ports.ErrKindInvalidRequest,
	"[fake:auth]":      ports.ErrKindAuth,
}

func markerError(text string) error {
	for m, k := range markers {
		if strings.Contains(text, m) {
			pe := &ports.ProviderError{Kind: k, Provider: Code, Code: "fake_" + strings.ToLower(string(k)), Message: "injected by prompt marker " + m}
			if k == ports.ErrKindRateLimited {
				pe.RetryAfter = 2 * time.Second
			}
			return pe
		}
	}
	return nil
}

func (p *Provider) enter(ctx context.Context, op, text string) error {
	p.mu.Lock()
	p.calls[op]++
	n := p.calls[op]
	sc := p.scenario
	p.mu.Unlock()
	delay := p.Hooks.Delay
	if sc.DelayMs > 0 {
		delay += time.Duration(sc.DelayMs) * time.Millisecond
	}
	if delay > 0 {
		t := time.NewTimer(delay)
		defer t.Stop()
		select {
		case <-t.C:
		case <-ctx.Done():
			return &ports.ProviderError{Kind: ports.ErrKindTransient, Provider: Code, Code: "timeout", Message: ctx.Err().Error(), Err: ctx.Err()}
		}
	}
	if p.Hooks.Fail != nil {
		if err := p.Hooks.Fail(op, n); err != nil {
			return err
		}
	}
	if f, ok := sc.Fail[op]; ok && (f.Times == 0 || n <= f.Times) {
		pe := &ports.ProviderError{Kind: f.Kind, Provider: Code, Code: f.Code, Message: f.Message,
			RetryAfter: time.Duration(f.RetryAfterSec * float64(time.Second))}
		if pe.Kind == "" {
			pe.Kind = ports.ErrKindTransient
		}
		if pe.Code == "" {
			pe.Code = "fake_injected"
		}
		if pe.Message == "" {
			pe.Message = "injected by scenario"
		}
		return pe
	}
	return markerError(text)
}

// Capabilities mirrors the seeded ai_models.capabilities (migration 0011).
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
	all := req.System
	for _, m := range req.Messages {
		all += "\n" + m.Content
	}
	if err := p.enter(ctx, "text", all); err != nil {
		return ports.TextResponse{}, err
	}
	p.mu.Lock()
	sampleText := p.scenario.Text
	p.mu.Unlock()
	if sampleText == "" {
		sampleText = "[fake] Nội dung mẫu do fake provider tạo."
	}
	resp := ports.TextResponse{Content: sampleText, StopReason: "end_turn"}
	if len(req.JSONSchema) > 0 {
		var schema map[string]any
		if err := json.Unmarshal(req.JSONSchema, &schema); err != nil {
			return ports.TextResponse{}, &ports.ProviderError{Kind: ports.ErrKindInvalidRequest, Provider: Code, Code: "bad_schema", Message: err.Error()}
		}
		b, _ := json.Marshal(sample(schema, 0))
		resp.JSON, resp.Content = b, string(b)
	}
	resp.Usage = ports.TextUsage(int64(len(all)/4+1), int64(len(resp.Content)/4+1), 0)
	resp.Raw = json.RawMessage(`{"provider":"fake"}`)
	return resp, nil
}

// sample builds a value conforming to a (simple) JSON schema.
func sample(s map[string]any, depth int) any {
	if depth > 6 || s == nil {
		return nil
	}
	if c, ok := s["const"]; ok {
		return c
	}
	if enum, ok := s["enum"].([]any); ok && len(enum) > 0 {
		return enum[0]
	}
	typ := s["type"]
	if arr, ok := typ.([]any); ok && len(arr) > 0 { // ["string","null"]
		typ = arr[0]
	}
	switch typ {
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
		if mi, ok := s["minItems"].(float64); ok && int(mi) > n {
			n = int(mi)
		}
		arr := make([]any, 0, n)
		for i := 0; i < n; i++ {
			arr = append(arr, sample(items, depth+1))
		}
		return arr
	case "integer":
		if mi, ok := s["minimum"].(float64); ok {
			return int(mi)
		}
		return 1
	case "number":
		if mi, ok := s["minimum"].(float64); ok {
			return mi
		}
		return 1.0
	case "boolean":
		return false
	case "string":
		v := "Nội dung mẫu"
		if ml, ok := s["minLength"].(float64); ok && len([]rune(v)) < int(ml) {
			v += strings.Repeat(".", int(ml)-len([]rune(v)))
		}
		return v
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

// samplePNG draws a solid background with a darker frame and diagonal band
// so variants are visually distinct; compresses to a few KB.
func samplePNG(w, h int, c color.RGBA) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	dark := color.RGBA{R: c.R / 2, G: c.G / 2, B: c.B / 2, A: 255}
	border := max(w, h) / 40
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			px := c
			if x < border || y < border || x >= w-border || y >= h-border || int(math.Abs(float64(x*h/w-y))) < border {
				px = dark
			}
			i := img.PixOffset(x, y)
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = px.R, px.G, px.B, px.A
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

var palette = []color.RGBA{{R: 37, G: 99, B: 235, A: 255}, {R: 22, G: 163, B: 74, A: 255}, {R: 217, G: 119, B: 6, A: 255}, {R: 147, G: 51, B: 234, A: 255}}

func images(count int, ar, quality string) ([]ports.GeneratedImage, ports.Usage) {
	if count <= 0 {
		count = 1
	}
	w, h := ratioSize(ar)
	out := make([]ports.GeneratedImage, 0, count)
	for i := 0; i < count; i++ {
		out = append(out, ports.GeneratedImage{MimeType: "image/png", Data: samplePNG(w, h, palette[i%len(palette)]), Width: w, Height: h})
	}
	unit := "image_1024_standard"
	if quality == "high" {
		unit = "image_1024_high"
	}
	return out, ports.Usage{Images: count, Units: map[string]float64{unit: float64(count)}}
}

func checkCaps(caps ports.Capabilities, ar, quality string, count int) error {
	if err := vendorhttp.CheckOneOf("aspect_ratio", ar, caps.AspectRatios); err != nil {
		return err
	}
	if err := vendorhttp.CheckOneOf("quality", quality, caps.Qualities); err != nil {
		return err
	}
	if count < 0 || count > caps.MaxVariants {
		return vendorhttp.Unsupported("count", count, map[string]int{"min": 1, "max": caps.MaxVariants})
	}
	return nil
}

// Generate implements ports.ImageGenerator.
func (p Image) Generate(ctx context.Context, req ports.ImageRequest) (ports.ImageResponse, error) {
	if err := p.enter(ctx, "image", req.Prompt); err != nil {
		return ports.ImageResponse{}, err
	}
	if err := checkCaps(p.Capabilities("fake-image"), req.AspectRatio, req.Quality, req.Count); err != nil {
		return ports.ImageResponse{}, err
	}
	imgs, u := images(req.Count, req.AspectRatio, req.Quality)
	return ports.ImageResponse{Images: imgs, Usage: u, Raw: json.RawMessage(`{"provider":"fake"}`)}, nil
}

// Edit implements ports.ImageGenerator.
func (p Image) Edit(ctx context.Context, req ports.ImageEditRequest) (ports.ImageResponse, error) {
	if err := p.enter(ctx, "image_edit", req.Prompt); err != nil {
		return ports.ImageResponse{}, err
	}
	if err := checkCaps(p.Capabilities("fake-image"), req.AspectRatio, req.Quality, req.Count); err != nil {
		return ports.ImageResponse{}, err
	}
	imgs, u := images(req.Count, req.AspectRatio, req.Quality)
	return ports.ImageResponse{Images: imgs, Usage: u, Raw: json.RawMessage(`{"provider":"fake"}`)}, nil
}

// videoID is "fakevid|<ar>|<res>|<sec>|<submitted unix ms>|<flags>|<seq>".
type videoID struct {
	ar, res     string
	sec         int
	submittedMs int64
	policy      bool
	seq         int
}

func (v videoID) String() string {
	flags := "-"
	if v.policy {
		flags = "p"
	}
	return fmt.Sprintf("fakevid|%s|%s|%d|%d|%s|%d", v.ar, v.res, v.sec, v.submittedMs, flags, v.seq)
}

func parseVideoID(id string) (videoID, error) {
	parts := strings.Split(id, "|")
	bad := &ports.ProviderError{Kind: ports.ErrKindInvalidRequest, Provider: Code, Code: "not_found", Message: "unknown provider job id"}
	if len(parts) != 7 || parts[0] != "fakevid" {
		return videoID{}, bad
	}
	sec, err1 := strconv.Atoi(parts[3])
	ms, err2 := strconv.ParseInt(parts[4], 10, 64)
	seq, err3 := strconv.Atoi(parts[6])
	if err1 != nil || err2 != nil || err3 != nil {
		return videoID{}, bad
	}
	return videoID{ar: parts[1], res: parts[2], sec: sec, submittedMs: ms, policy: parts[5] == "p", seq: seq}, nil
}

// Submit implements ports.VideoGenerator.
func (p Video) Submit(ctx context.Context, req ports.VideoRequest) (ports.VideoSubmission, error) {
	// The policy marker fails asynchronously (like Veo's RAI filter), so it
	// is not passed to enter.
	if err := p.enter(ctx, "video_submit", strings.ReplaceAll(req.Prompt, "[fake:policy]", "")); err != nil {
		return ports.VideoSubmission{}, err
	}
	caps := p.Capabilities("fake-video")
	if err := vendorhttp.CheckOneOf("aspect_ratio", req.AspectRatio, caps.AspectRatios); err != nil {
		return ports.VideoSubmission{}, err
	}
	if err := vendorhttp.CheckOneOf("resolution", req.Resolution, caps.Resolutions); err != nil {
		return ports.VideoSubmission{}, err
	}
	if req.DurationSec != 0 && (req.DurationSec < caps.MinDurationSec || req.DurationSec > caps.MaxDurationSec) {
		return ports.VideoSubmission{}, vendorhttp.Unsupported("duration_sec", req.DurationSec, map[string]int{"min": caps.MinDurationSec, "max": caps.MaxDurationSec})
	}
	v := videoID{ar: req.AspectRatio, res: req.Resolution, sec: req.DurationSec, submittedMs: p.now().UnixMilli(),
		policy: strings.Contains(req.Prompt, "[fake:policy]")}
	if v.ar == "" {
		v.ar = "9:16"
	}
	if v.res == "" {
		v.res = "720p"
	}
	if v.sec == 0 {
		v.sec = 4
	}
	p.mu.Lock()
	p.seq++
	v.seq = p.seq
	ready := p.scenario.VideoReadyAfterMs
	p.mu.Unlock()
	poll := 10 * time.Millisecond
	if ready > 0 {
		poll = time.Duration(ready) * time.Millisecond / 2
	}
	return ports.VideoSubmission{ProviderJobID: v.String(), PollAfter: poll}, nil
}

// GetStatus implements ports.VideoGenerator: the clip is "running" until
// video_ready_after_ms elapsed since submit, then succeeds (or fails with
// ContentPolicy when the prompt carried [fake:policy]).
func (p Video) GetStatus(ctx context.Context, id string) (ports.VideoStatus, error) {
	if err := p.enter(ctx, "video_status", ""); err != nil {
		return ports.VideoStatus{}, err
	}
	v, err := parseVideoID(id)
	if err != nil {
		return ports.VideoStatus{}, err
	}
	p.mu.Lock()
	cancelled := p.cancelled[id]
	ready := p.scenario.VideoReadyAfterMs
	p.mu.Unlock()
	raw := json.RawMessage(`{"provider":"fake"}`)
	if cancelled {
		return ports.VideoStatus{State: ports.VideoCancelled, Raw: raw}, nil
	}
	elapsed := p.now().UnixMilli() - v.submittedMs
	if ready > 0 && elapsed < int64(ready) {
		return ports.VideoStatus{State: ports.VideoRunning, Progress: int(elapsed * 100 / int64(ready)), Raw: raw}, nil
	}
	if v.policy {
		return ports.VideoStatus{State: ports.VideoFailed, Raw: raw, Error: &ports.ProviderError{Kind: ports.ErrKindContentPolicy,
			Provider: Code, Code: "fake_content_policy", Message: "clip blocked by (fake) safety filter"}}, nil
	}
	u := ports.Usage{VideoSeconds: float64(v.sec), Units: map[string]float64{"video_second_" + v.res: float64(v.sec)}}
	return ports.VideoStatus{State: ports.VideoSucceeded, Progress: 100, DurationSec: float64(v.sec), Usage: &u, Raw: raw}, nil
}

// Download implements ports.VideoGenerator: a real 2 s H.264/AAC MP4 in the
// requested aspect ratio (the render pipeline pads/trims it to the scene
// length).
func (p Video) Download(_ context.Context, id string) (ports.MediaStream, error) {
	v, err := parseVideoID(id)
	if err != nil {
		return ports.MediaStream{}, err
	}
	b, err := SampleClip(v.ar)
	if err != nil {
		return ports.MediaStream{}, &ports.ProviderError{Kind: ports.ErrKindUnknown, Provider: Code, Code: "sample_missing", Message: err.Error()}
	}
	return ports.MediaStream{Body: io.NopCloser(bytes.NewReader(b)), MimeType: "video/mp4", Size: int64(len(b))}, nil
}

// SampleClip returns the embedded sample MP4 for an aspect ratio.
func SampleClip(aspect string) ([]byte, error) {
	name := map[string]string{"9:16": "clip_9x16.mp4", "1:1": "clip_1x1.mp4", "16:9": "clip_16x9.mp4"}[aspect]
	if name == "" {
		name = "clip_9x16.mp4"
	}
	return samples.ReadFile("samples/" + name)
}

// Cancel implements ports.VideoGenerator.
func (p Video) Cancel(_ context.Context, id string) error {
	if _, err := parseVideoID(id); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cancelled[id] = true
	return nil
}

// Synthesize implements ports.VoiceSynthesizer: a 16 kHz mono WAV (quiet
// 220 Hz tone so players show a waveform), ~70 ms per character, with
// evenly spaced word timings.
func (p Voice) Synthesize(ctx context.Context, req ports.VoiceRequest) (ports.VoiceResponse, error) {
	if err := p.enter(ctx, "voice", req.Text); err != nil {
		return ports.VoiceResponse{}, err
	}
	if req.Format != "" && req.Format != "wav" && req.Format != "mp3" {
		return ports.VoiceResponse{}, vendorhttp.Unsupported("format", req.Format, []string{"mp3", "wav"})
	}
	chars := len([]rune(req.Text))
	rate := req.SpeakingRate
	if rate <= 0 {
		rate = 1
	}
	durMs := int(float64(chars*70)/rate) + 200
	words := strings.Fields(req.Text)
	timings := make([]ports.WordTiming, 0, len(words))
	if len(words) > 0 {
		step := durMs / len(words)
		for i, w := range words {
			timings = append(timings, ports.WordTiming{Text: w, StartMs: i * step, EndMs: (i + 1) * step})
		}
	}
	// The fake always returns WAV (mp3 encoding is not in the stdlib);
	// MimeType tells the caller what it got.
	return ports.VoiceResponse{
		MimeType: "audio/wav", Audio: toneWAV(durMs), DurationMs: durMs, Timings: timings,
		Usage: ports.Usage{Characters: chars, Units: map[string]float64{"char": float64(chars)}},
		Raw:   json.RawMessage(`{"provider":"fake"}`),
	}, nil
}

func toneWAV(ms int) []byte {
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
	pcm := make([]byte, n*2)
	for i := 0; i < n; i++ {
		s := int16(600 * math.Sin(2*math.Pi*220*float64(i)/rate))
		binary.LittleEndian.PutUint16(pcm[i*2:], uint16(s))
	}
	b.Write(pcm)
	return b.Bytes()
}
