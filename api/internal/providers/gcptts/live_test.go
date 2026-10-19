//go:build live

package gcptts

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/be/acs/api/internal/ports"
)

// Run manually: GOOGLE_TTS_CREDENTIALS=<api key or service-account JSON> go test -tags=live ./internal/providers/gcptts/
func TestLiveSynthesizeVietnamese(t *testing.T) {
	if os.Getenv(DefaultSecretRef) == "" {
		t.Skip(DefaultSecretRef + " not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	start := time.Now()
	r, err := New().Synthesize(ctx, ports.VoiceRequest{Text: "Khóa học tiếng Anh giao tiếp dành cho người đi làm, khai giảng tháng này.", Language: "vi-VN"})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Audio) == 0 || len(r.Timings) == 0 {
		t.Fatalf("audio=%d timings=%d", len(r.Audio), len(r.Timings))
	}
	t.Logf("latency=%s duration=%dms chars=%d timings=%d", time.Since(start), r.DurationMs, r.Usage.Characters, len(r.Timings))
}
