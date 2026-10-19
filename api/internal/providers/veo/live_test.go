//go:build live

package veo

import (
	"context"
	"io"
	"os"
	"testing"
	"time"

	"github.com/be/acs/api/internal/ports"
)

// Run manually (bills ~4 s of video): GEMINI_API_KEY=… go test -tags=live -timeout 15m ./internal/providers/veo/
func TestLiveTextToVideo(t *testing.T) {
	if os.Getenv(DefaultSecretRef) == "" {
		t.Skip(DefaultSecretRef + " not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	a := New()
	start := time.Now()
	sub, err := a.Submit(ctx, ports.VideoRequest{Prompt: "A cup of coffee on a wooden table, morning light, slow dolly in", AspectRatio: "9:16", Resolution: "720p", DurationSec: 4})
	if err != nil {
		t.Fatal(err)
	}
	for {
		st, err := a.GetStatus(ctx, sub.ProviderJobID)
		if err != nil {
			t.Fatal(err)
		}
		if st.State == ports.VideoSucceeded {
			break
		}
		if st.State == ports.VideoFailed {
			t.Fatal(st.Error)
		}
		time.Sleep(10 * time.Second)
	}
	ms, err := a.Download(ctx, sub.ProviderJobID)
	if err != nil {
		t.Fatal(err)
	}
	n, _ := io.Copy(io.Discard, ms.Body)
	_ = ms.Body.Close()
	t.Logf("latency=%s bytes=%d", time.Since(start), n)
}
