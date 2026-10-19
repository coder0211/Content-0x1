package log

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/be/acs/api/internal/shared/kernel"
)

func TestRedaction(t *testing.T) {
	var buf bytes.Buffer
	l := NewWithWriter(&buf, "debug")
	ctx := kernel.WithRequestMeta(context.Background(), kernel.RequestMeta{RequestID: "req-1"})
	l.InfoContext(ctx, "x",
		slog.String("cookie", "acs_session=abc"),
		slog.String("Authorization", "Bearer s3cr3t"),
		slog.String("api_key", "sk-123"),
		slog.String("url", "https://minio/b/k?X-Amz-Signature=deadbeef&X-Amz-Expires=900"),
	)
	out := buf.String()
	for _, leak := range []string{"abc", "s3cr3t", "sk-123", "deadbeef"} {
		if strings.Contains(out, leak) {
			t.Fatalf("log leaked %q: %s", leak, out)
		}
	}
	if !strings.Contains(out, `"request_id":"req-1"`) {
		t.Fatalf("missing request_id: %s", out)
	}
	if !strings.Contains(out, "X-Amz-Expires=900") {
		t.Fatalf("non-sensitive query param should be kept: %s", out)
	}
}

func TestWithFields(t *testing.T) {
	var buf bytes.Buffer
	l := NewWithWriter(&buf, "info")
	ctx := With(context.Background(), slog.String("job_id", "j1"))
	l.InfoContext(ctx, "step")
	if !strings.Contains(buf.String(), `"job_id":"j1"`) {
		t.Fatal(buf.String())
	}
}
