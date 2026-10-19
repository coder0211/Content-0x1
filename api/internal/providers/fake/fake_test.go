package fake

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image/png"
	"io"
	"testing"
	"time"

	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
)

func TestTextFollowsSchema(t *testing.T) {
	p := New()
	schema := `{"type":"object","properties":{"title":{"type":"string"},"hashtags":{"type":"array","items":{"type":"string"},"minItems":2},"platform":{"enum":["facebook"]}}}`
	r, err := Text{p}.Generate(context.Background(), ports.TextRequest{System: "s", Messages: []ports.Message{{Role: "user", Content: "brief"}}, JSONSchema: json.RawMessage(schema)})
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Title    string   `json:"title"`
		Hashtags []string `json:"hashtags"`
		Platform string   `json:"platform"`
	}
	if err := json.Unmarshal(r.JSON, &out); err != nil || out.Title == "" || len(out.Hashtags) != 2 || out.Platform != "facebook" {
		t.Fatalf("%v %s", err, r.JSON)
	}
	if r.Usage.Units["input_token"] == 0 || r.Usage.Units["output_token"] == 0 {
		t.Fatal("usage units")
	}
}

func TestImageAspectAndFailureHook(t *testing.T) {
	p := New()
	p.Hooks.Fail = func(op string, call int) error {
		if op == "image" && call <= 2 {
			return &ports.ProviderError{Kind: ports.ErrKindTransient, Provider: Code, Code: "503"}
		}
		return nil
	}
	img := Image{p}
	for i := 0; i < 2; i++ {
		_, err := img.Generate(context.Background(), ports.ImageRequest{Count: 1})
		pe, ok := ports.AsProviderError(err)
		if !ok || !pe.Retryable() {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	r, err := img.Generate(context.Background(), ports.ImageRequest{Count: 2, AspectRatio: "9:16"})
	if err != nil || len(r.Images) != 2 {
		t.Fatal(err)
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(r.Images[0].Data))
	if err != nil || cfg.Width >= cfg.Height {
		t.Fatalf("9:16 should be portrait: %+v %v", cfg, err)
	}
}

func TestVideoAndVoice(t *testing.T) {
	p := New()
	v := Video{p}
	sub, err := v.Submit(context.Background(), ports.VideoRequest{DurationSec: 4, Resolution: "1080p", AspectRatio: "16:9"})
	if err != nil {
		t.Fatal(err)
	}
	st, err := v.GetStatus(context.Background(), sub.ProviderJobID)
	if err != nil || st.State != ports.VideoSucceeded || st.Usage.Units["video_second_1080p"] != 4 {
		t.Fatalf("%v %+v", err, st)
	}
	if _, err := v.GetStatus(context.Background(), "nope"); err == nil {
		t.Fatal("unknown id")
	}
	ms, err := v.Download(context.Background(), sub.ProviderJobID)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(ms.Body)
	if len(b) < 1000 || string(b[4:8]) != "ftyp" {
		t.Fatalf("not an mp4: %d bytes", len(b))
	}
	vr, err := Voice{p}.Synthesize(context.Background(), ports.VoiceRequest{Text: "Xin chào các bạn"})
	if err != nil || len(vr.Timings) != 4 || !bytes.HasPrefix(vr.Audio, []byte("RIFF")) {
		t.Fatalf("%v %+v", err, vr.Timings)
	}
	if _, err := (Voice{p}).Synthesize(context.Background(), ports.VoiceRequest{Text: "x", Format: "ogg"}); !errors.Is(err, kernel.ErrCapability) {
		t.Fatal(err)
	}
}

func TestVideoAsyncScenarioPolicyAndCancel(t *testing.T) {
	p := New()
	now := time.Unix(1_700_000_000, 0)
	p.Now = func() time.Time { return now }
	p.Configure("", json.RawMessage(`{"video_ready_after_ms":5000}`))
	v := Video{p}
	sub, err := v.Submit(context.Background(), ports.VideoRequest{Prompt: "cảnh 1", DurationSec: 6})
	if err != nil {
		t.Fatal(err)
	}
	if st, _ := v.GetStatus(context.Background(), sub.ProviderJobID); st.State != ports.VideoRunning {
		t.Fatalf("%+v", st)
	}
	now = now.Add(6 * time.Second)
	if st, _ := v.GetStatus(context.Background(), sub.ProviderJobID); st.State != ports.VideoSucceeded {
		t.Fatalf("%+v", st)
	}
	// A fresh provider (process restart) can still poll the same id.
	q := New()
	q.Now = p.Now
	if st, err := (Video{q}).GetStatus(context.Background(), sub.ProviderJobID); err != nil || st.State != ports.VideoSucceeded {
		t.Fatalf("%v %+v", err, st)
	}
	p.Configure("", nil)
	pol, _ := v.Submit(context.Background(), ports.VideoRequest{Prompt: "x [fake:policy]"})
	st, _ := v.GetStatus(context.Background(), pol.ProviderJobID)
	if st.State != ports.VideoFailed || st.Error.Kind != ports.ErrKindContentPolicy || st.Error.Retryable() {
		t.Fatalf("%+v", st)
	}
	c, _ := v.Submit(context.Background(), ports.VideoRequest{Prompt: "x"})
	if err := v.Cancel(context.Background(), c.ProviderJobID); err != nil {
		t.Fatal(err)
	}
	if st, _ := v.GetStatus(context.Background(), c.ProviderJobID); st.State != ports.VideoCancelled {
		t.Fatalf("%+v", st)
	}
	if _, err := v.Submit(context.Background(), ports.VideoRequest{AspectRatio: "4:3"}); !errors.Is(err, kernel.ErrCapability) {
		t.Fatal(err)
	}
}

func TestScenarioFailuresAndMarkers(t *testing.T) {
	p := New()
	p.Configure("none", json.RawMessage(`{"fail":{"text":{"kind":"RateLimited","times":1,"retry_after_sec":3}},"text":"Mẫu"}`))
	_, err := Text{p}.Generate(context.Background(), ports.TextRequest{Messages: []ports.Message{{Role: "user", Content: "a"}}})
	pe, ok := ports.AsProviderError(err)
	if !ok || pe.Kind != ports.ErrKindRateLimited || pe.RetryAfter != 3*time.Second {
		t.Fatal(err)
	}
	r, err := Text{p}.Generate(context.Background(), ports.TextRequest{Messages: []ports.Message{{Role: "user", Content: "a"}}})
	if err != nil || r.Content != "Mẫu" {
		t.Fatal(err, r.Content)
	}
	_, err = Image{p}.Generate(context.Background(), ports.ImageRequest{Prompt: "poster [fake:policy]"})
	if pe, ok := ports.AsProviderError(err); !ok || pe.Kind != ports.ErrKindContentPolicy {
		t.Fatal(err)
	}
	if _, err := (Image{p}).Generate(context.Background(), ports.ImageRequest{Prompt: "x", Count: 9}); !errors.Is(err, kernel.ErrCapability) {
		t.Fatal(err)
	}
	p.Configure("", json.RawMessage(`{"delay_ms":200}`))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err = Voice{p}.Synthesize(ctx, ports.VoiceRequest{Text: "x"})
	if pe, ok := ports.AsProviderError(err); !ok || pe.Kind != ports.ErrKindTransient {
		t.Fatal(err)
	}
}
