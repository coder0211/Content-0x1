package providers

import (
	"github.com/be/acs/api/internal/platform/config"
	"github.com/be/acs/api/internal/platform/ffmpeg"
	"github.com/be/acs/api/internal/providers/anthropic"
	"github.com/be/acs/api/internal/providers/fake"
	"github.com/be/acs/api/internal/providers/gcptts"
	"github.com/be/acs/api/internal/providers/kenburns"
	"github.com/be/acs/api/internal/providers/openai"
	"github.com/be/acs/api/internal/providers/veo"
)

// Build creates the process registry with every adapter of OQ-01:
// anthropic (text), openai (image), veo + kenburns (video), gcp_tts (voice),
// plus the fake provider when enabled. Construction never touches the
// network or reads keys: each call resolves its key from the configured
// secret_ref via config.Secret, so a missing key surfaces as an Auth
// ProviderError on use, not as a start-up failure. The provider module then
// applies the ai_providers rows (ApplyProviderConfig) and object storage
// (UseStorage).
func Build(cfg config.Config) *Registry {
	r := NewRegistry()
	if cfg.EnableFakeProviders {
		for _, a := range fake.New().Adapters() {
			r.Register(a)
		}
	}
	r.Register(anthropic.New())
	r.Register(openai.New())
	r.Register(veo.New())
	r.Register(gcptts.New())
	bin := cfg.FFmpeg.FFmpegPath
	if bin == "" {
		bin = "ffmpeg"
	}
	r.Register(kenburns.New(ffmpeg.NewRenderer(bin), cfg.FFmpeg.TempDir))
	return r
}
