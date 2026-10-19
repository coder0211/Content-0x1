package providers

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/be/acs/api/internal/platform/config"
	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
	"github.com/be/acs/api/internal/testutil"
)

func TestBuildRegistersEveryProvider(t *testing.T) {
	r := Build(config.Config{EnableFakeProviders: true})
	if _, err := r.Text("fake"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Voice("fake"); err != nil {
		t.Fatal(err)
	}
	for kind, code := range map[ports.AIKind]string{ports.AIKindText: "anthropic", ports.AIKindImage: "openai",
		ports.AIKindVideo: "veo", ports.AIKindVoice: "gcp_tts"} {
		found := false
		for _, c := range r.Codes()[kind] {
			found = found || c == code
		}
		if !found {
			t.Errorf("%s adapter %s missing", kind, code)
		}
	}
	if _, err := r.Video("kenburns"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Video("sora"); !errors.Is(err, kernel.ErrCapability) {
		t.Fatalf("unknown provider: %v", err)
	}
	if _, err := r.Image("anthropic"); !errors.Is(err, kernel.ErrCapability) {
		t.Fatal("anthropic has no image capability")
	}
	if _, err := Build(config.Config{}).Text("fake"); err == nil {
		t.Fatal("fake must be disabled when not enabled")
	}
}

func TestApplyProviderConfigDisablesAndConfigures(t *testing.T) {
	r := Build(config.Config{EnableFakeProviders: true})
	r.ApplyProviderConfig("fake", false, "none", nil)
	_, err := r.Text("fake")
	de, ok := kernel.AsDomainError(err)
	if !ok || !errors.Is(err, kernel.ErrCapability) || de.Details[0]["issue"] != "disabled" {
		t.Fatal(err)
	}
	r.ApplyProviderConfig("fake", true, "none", json.RawMessage(`{"fail":{"text":{"kind":"Auth"}}}`))
	tg, err := r.Text("fake")
	if err != nil {
		t.Fatal(err)
	}
	_, err = tg.Generate(t.Context(), ports.TextRequest{Messages: []ports.Message{{Role: "user", Content: "x"}}})
	if pe, ok := ports.AsProviderError(err); !ok || pe.Kind != ports.ErrKindAuth {
		t.Fatalf("scenario from config_json must apply: %v", err)
	}
	r.ApplyProviderConfig("unknown", true, "X", nil) // ignored
	r.UseStorage(testutil.NewStorage())
}
