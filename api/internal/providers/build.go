package providers

import (
	"github.com/be/acs/api/internal/platform/config"
	"github.com/be/acs/api/internal/providers/fake"
)

// Build creates the process registry. Real vendor adapters (anthropic,
// openai, veo, kenburns, gcp_tts — OQ-01) are added here by the providers
// agent, each reading its key via config.Secret(<secret_ref>).
func Build(cfg config.Config) *Registry {
	r := NewRegistry()
	if cfg.EnableFakeProviders {
		for _, a := range fake.New().Adapters() {
			r.Register(a)
		}
	}
	return r
}
