package anthropic

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/providers/recorded"
)

func adapter(srv *recorded.Server, cfg string) *Adapter {
	a := New()
	a.EP.WithKeyFunc(func(ref string) string {
		if ref == DefaultSecretRef {
			return "sk-test-123"
		}
		return ""
	})
	if cfg != "" {
		a.Configure("", json.RawMessage(cfg))
	}
	a.EP.SetBaseURL(srv.URL)
	return a
}

const schema = `{"type":"object","properties":{"title":{"type":"string"},"body":{"type":"string"},"hashtags":{"type":"array","items":{"type":"string"}},"meta":{"type":"object","properties":{"x":{"type":"string"}}}},"required":["title","body","hashtags"]}`

func TestStructuredOutputContract(t *testing.T) {
	srv := recorded.New(t, recorded.Route{"POST /v1/messages": {File: "messages_json_ok.json"}})
	a := adapter(srv, `{"effort":"medium","refusal_fallback":true}`)
	resp, err := a.Generate(context.Background(), ports.TextRequest{
		Model: "claude-sonnet-5-5", System: "Bạn là copywriter.", Temperature: 0.7,
		Messages:   []ports.Message{{Role: "user", Content: "<user_data>brief</user_data>"}},
		JSONSchema: json.RawMessage(schema), SchemaName: "facebook_post",
	})
	if err != nil {
		t.Fatal(err)
	}
	var post struct {
		Title    string   `json:"title"`
		Hashtags []string `json:"hashtags"`
	}
	if err := json.Unmarshal(resp.JSON, &post); err != nil || post.Title == "" || len(post.Hashtags) != 2 {
		t.Fatalf("%v %s", err, resp.JSON)
	}
	if resp.Usage.Units["input_token"] != 812 || resp.Usage.Units["output_token"] != 164 || resp.Usage.Units["cache_read_token"] != 256 {
		t.Fatalf("usage %+v", resp.Usage)
	}
	if resp.Usage.CachedTokens != 256 {
		t.Fatal(resp.Usage.CachedTokens)
	}
	req := srv.Last()
	if req.Header.Get("x-api-key") != "sk-test-123" || req.Header.Get("anthropic-version") != APIVersion || req.Header.Get("anthropic-beta") != FallbackBeta {
		t.Fatalf("headers %v", req.Header)
	}
	var sent map[string]any
	_ = json.Unmarshal(req.Body, &sent)
	if _, ok := sent["temperature"]; ok {
		t.Fatal("temperature must not be sent")
	}
	if _, ok := sent["tool_choice"]; ok {
		t.Fatal("no forced tool use")
	}
	oc := sent["output_config"].(map[string]any)
	f := oc["format"].(map[string]any)
	if f["type"] != "json_schema" || oc["effort"] != "medium" || sent["fallbacks"] != "default" {
		t.Fatalf("%v", sent)
	}
	sch := f["schema"].(map[string]any)
	if sch["additionalProperties"] != false {
		t.Fatal("root additionalProperties")
	}
	meta := sch["properties"].(map[string]any)["meta"].(map[string]any)
	if meta["additionalProperties"] != false {
		t.Fatal("nested additionalProperties")
	}
	if string(resp.Raw) == "" || json.Valid(resp.Raw) == false {
		t.Fatal("raw")
	}
}

// ACS-030 AC: provider rate limit → RateLimited with RetryAfter.
func TestRateLimitMapsToRetryAfter(t *testing.T) {
	srv := recorded.New(t, recorded.Route{"POST /v1/messages": {Status: 429, Header: map[string]string{"retry-after": "12"}, File: "error_rate_limit.json"}})
	_, err := adapter(srv, "").Generate(context.Background(), ports.TextRequest{Messages: []ports.Message{{Role: "user", Content: "x"}}})
	pe, ok := ports.AsProviderError(err)
	if !ok || pe.Kind != ports.ErrKindRateLimited || pe.RetryAfter != 12*time.Second || !pe.Retryable() || pe.Code != "rate_limit_error" {
		t.Fatalf("%v", err)
	}
}

func TestErrorKinds(t *testing.T) {
	for _, c := range []struct {
		status int
		typ    string
		kind   ports.ErrKind
	}{
		{529, "overloaded_error", ports.ErrKindTransient},
		{500, "api_error", ports.ErrKindTransient},
		{401, "authentication_error", ports.ErrKindAuth},
		{400, "invalid_request_error", ports.ErrKindInvalidRequest},
		{413, "request_too_large", ports.ErrKindInvalidRequest},
		{502, "", ports.ErrKindTransient},
	} {
		body := []byte(`{"type":"error","error":{"type":"` + c.typ + `","message":"m"}}`)
		if c.typ == "" {
			body = []byte("<html>bad gateway</html>")
		}
		srv := recorded.New(t, recorded.Route{"POST /v1/messages": {Status: c.status, Body: body}})
		_, err := adapter(srv, "").Generate(context.Background(), ports.TextRequest{Messages: []ports.Message{{Role: "user", Content: "x"}}})
		if pe, ok := ports.AsProviderError(err); !ok || pe.Kind != c.kind {
			t.Errorf("%d %s: %v", c.status, c.typ, err)
		}
	}
}

func TestRefusalIsContentPolicy(t *testing.T) {
	srv := recorded.New(t, recorded.Route{"POST /v1/messages": {File: "messages_refusal.json"}})
	resp, err := adapter(srv, "").Generate(context.Background(), ports.TextRequest{Messages: []ports.Message{{Role: "user", Content: "x"}}})
	pe, ok := ports.AsProviderError(err)
	if !ok || pe.Kind != ports.ErrKindContentPolicy || pe.Retryable() {
		t.Fatal(err)
	}
	if resp.Usage.InputTokens != 120 {
		t.Fatal("usage must still be reported on refusal")
	}
}

func TestMissingKeyIsAuth(t *testing.T) {
	a := New()
	a.EP.WithKeyFunc(func(string) string { return "" })
	_, err := a.Generate(context.Background(), ports.TextRequest{Messages: []ports.Message{{Role: "user", Content: "x"}}})
	if pe, ok := ports.AsProviderError(err); !ok || pe.Kind != ports.ErrKindAuth {
		t.Fatal(err)
	}
}

func TestInvalidJSONOutput(t *testing.T) {
	srv := recorded.New(t, recorded.Route{"POST /v1/messages": {Body: []byte(`{"content":[{"type":"text","text":"not json"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)}})
	_, err := adapter(srv, "").Generate(context.Background(), ports.TextRequest{Messages: []ports.Message{{Role: "user", Content: "x"}}, JSONSchema: json.RawMessage(schema)})
	if pe, ok := ports.AsProviderError(err); !ok || pe.Code != "invalid_json" {
		t.Fatal(err)
	}
}
