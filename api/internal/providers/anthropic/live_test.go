//go:build live

package anthropic

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/be/acs/api/internal/ports"
)

// Run manually: ANTHROPIC_API_KEY=… go test -tags=live ./internal/providers/anthropic/
func TestLiveStructuredOutput(t *testing.T) {
	if os.Getenv(DefaultSecretRef) == "" {
		t.Skip(DefaultSecretRef + " not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	resp, err := New().Generate(ctx, ports.TextRequest{
		Model: DefaultModel, System: "Bạn là copywriter tiếng Việt. Trả lời đúng schema.", MaxTokens: 2000,
		Messages:   []ports.Message{{Role: "user", Content: "<user_data>Viết tiêu đề ngắn cho khóa học IELTS.</user_data>"}},
		JSONSchema: json.RawMessage(`{"type":"object","properties":{"title":{"type":"string"}},"required":["title"]}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("json=%s usage=%+v", resp.JSON, resp.Usage)
	if resp.Usage.InputTokens == 0 || len(resp.JSON) == 0 {
		t.Fatal("expected usage and JSON")
	}
}
