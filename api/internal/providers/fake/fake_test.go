package fake

import (
	"bytes"
	"context"
	"encoding/json"
	"image/png"
	"testing"

	"github.com/be/acs/api/internal/ports"
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
	sub, err := v.Submit(context.Background(), ports.VideoRequest{DurationSec: 4, Resolution: "1080p"})
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
	vr, err := Voice{p}.Synthesize(context.Background(), ports.VoiceRequest{Text: "Xin chào các bạn"})
	if err != nil || len(vr.Timings) != 4 || !bytes.HasPrefix(vr.Audio, []byte("RIFF")) {
		t.Fatalf("%v %+v", err, vr.Timings)
	}
}
