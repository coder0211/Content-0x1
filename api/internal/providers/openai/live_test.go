//go:build live

package openai

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/be/acs/api/internal/ports"
)

// Run manually: OPENAI_API_KEY=… go test -tags=live ./internal/providers/openai/
func TestLiveGenerate(t *testing.T) {
	if os.Getenv(DefaultSecretRef) == "" {
		t.Skip(DefaultSecretRef + " not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	start := time.Now()
	resp, err := New().Generate(ctx, ports.ImageRequest{Prompt: "Minimal poster of a coffee cup, flat colors", AspectRatio: "9:16", Quality: "standard", Count: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("latency=%s size=%dx%d usage=%+v", time.Since(start), resp.Images[0].Width, resp.Images[0].Height, resp.Usage)
}
