// Package anthropic is the text adapter for Anthropic Claude (Messages API,
// OQ-01). Structured output uses `output_config.format` with a JSON schema
// (forced tool_choice is rejected by claude-sonnet-5-5). Plain net/http, no
// SDK; the key comes from ai_providers.secret_ref (default ANTHROPIC_API_KEY).
package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/providers/vendorhttp"
)

// Code is ai_providers.code.
const Code = "anthropic"

// Defaults.
const (
	DefaultSecretRef = "ANTHROPIC_API_KEY" //nolint:gosec // G101: name of the env var, not a credential
	DefaultBaseURL   = "https://api.anthropic.com"
	DefaultModel     = "claude-sonnet-5-5"
	APIVersion       = "2023-06-01"
	FallbackBeta     = "server-side-fallback-2026-07-01"
	defaultMaxTokens = 16000
)

// Adapter implements ports.TextGenerator.
type Adapter struct{ EP *vendorhttp.Endpoint }

var _ ports.TextGenerator = (*Adapter)(nil)

// New returns an adapter with production defaults (120 s timeout).
func New() *Adapter {
	return &Adapter{EP: vendorhttp.NewEndpoint(Code, DefaultSecretRef, DefaultBaseURL, 120*time.Second)}
}

// Code implements ports.TextGenerator.
func (*Adapter) Code() string { return Code }

// Configure applies the ai_providers row (registry hook).
func (a *Adapter) Configure(secretRef string, cfg json.RawMessage) { a.EP.Configure(secretRef, cfg) }

// Capabilities of the seeded models.
func (*Adapter) Capabilities(string) ports.Capabilities {
	return ports.Capabilities{Languages: []string{"vi", "en"}, MaxOutputTokens: 64000}
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type format struct {
	Type   string          `json:"type"`
	Schema json.RawMessage `json:"schema"`
}

type outputConfig struct {
	Format *format `json:"format,omitempty"`
	Effort string  `json:"effort,omitempty"`
}

type request struct {
	Model        string        `json:"model"`
	MaxTokens    int           `json:"max_tokens"`
	System       string        `json:"system,omitempty"`
	Messages     []message     `json:"messages"`
	OutputConfig *outputConfig `json:"output_config,omitempty"`
	Fallbacks    string        `json:"fallbacks,omitempty"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type usage struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
}

type stopDetails struct {
	Type        string  `json:"type"`
	Category    *string `json:"category"`
	Explanation string  `json:"explanation,omitempty"`
}

type response struct {
	ID          string         `json:"id"`
	Model       string         `json:"model"`
	Content     []contentBlock `json:"content"`
	StopReason  string         `json:"stop_reason"`
	StopDetails *stopDetails   `json:"stop_details"`
	Usage       usage          `json:"usage"`
}

type errorBody struct {
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
	RequestID string `json:"request_id"`
}

// classify maps Anthropic error types (docs: errors) to ErrKind.
func classify(status int, h http.Header, body []byte) *ports.ProviderError {
	var eb errorBody
	_ = json.Unmarshal(body, &eb)
	typ, msg := eb.Error.Type, eb.Error.Message
	if typ == "" {
		return nil
	}
	pe := &ports.ProviderError{Provider: Code, Code: typ, Message: msg}
	switch typ {
	case "rate_limit_error":
		pe.Kind, pe.RetryAfter = ports.ErrKindRateLimited, vendorhttp.RetryAfter(h, 30*time.Second)
	case "overloaded_error", "api_error", "timeout_error":
		pe.Kind, pe.RetryAfter = ports.ErrKindTransient, vendorhttp.RetryAfter(h, 0)
	case "authentication_error", "permission_error", "billing_error":
		pe.Kind = ports.ErrKindAuth
	case "invalid_request_error", "not_found_error", "request_too_large":
		pe.Kind = ports.ErrKindInvalidRequest
	default:
		return vendorhttp.ByStatus(Code, status, h, typ, msg)
	}
	return pe
}

// Generate implements ports.TextGenerator.
func (a *Adapter) Generate(ctx context.Context, req ports.TextRequest) (ports.TextResponse, error) {
	key, err := a.EP.Key()
	if err != nil {
		return ports.TextResponse{}, err
	}
	model := req.Model
	if model == "" {
		model = DefaultModel
	}
	body := request{Model: model, MaxTokens: req.MaxTokens, System: req.System}
	if body.MaxTokens <= 0 {
		body.MaxTokens = defaultMaxTokens
	}
	for _, m := range req.Messages {
		role := m.Role
		if role != "assistant" {
			role = "user"
		}
		body.Messages = append(body.Messages, message{Role: role, Content: m.Content})
	}
	if len(body.Messages) == 0 {
		return ports.TextResponse{}, vendorhttp.Errorf(Code, ports.ErrKindInvalidRequest, "no_messages", "at least one message is required")
	}
	oc := &outputConfig{}
	if len(req.JSONSchema) > 0 {
		schema, err := NormalizeSchema(req.JSONSchema)
		if err != nil {
			return ports.TextResponse{}, vendorhttp.Errorf(Code, ports.ErrKindInvalidRequest, "bad_schema", "%v", err)
		}
		oc.Format = &format{Type: "json_schema", Schema: schema}
	}
	var effort string
	if a.EP.Extra("effort", &effort) && effort != "" {
		oc.Effort = effort
	}
	if oc.Format != nil || oc.Effort != "" {
		body.OutputConfig = oc
	}
	// Temperature is intentionally not sent: Sonnet 5.5 rejects non-default
	// sampling parameters (400).
	h := http.Header{}
	h.Set("x-api-key", key)
	h.Set("anthropic-version", APIVersion)
	var fallback bool
	if a.EP.Extra("refusal_fallback", &fallback) && fallback {
		body.Fallbacks = "default"
		h.Set("anthropic-beta", FallbackBeta)
	}
	var out response
	if _, err := a.EP.DoJSON(ctx, http.MethodPost, a.EP.BaseURL()+"/v1/messages", h, body, &out, classify); err != nil {
		return ports.TextResponse{}, err
	}
	return toResponse(out, len(req.JSONSchema) > 0)
}

func toResponse(out response, wantJSON bool) (ports.TextResponse, error) {
	raw := vendorhttp.StripRaw(map[string]any{"id": out.ID, "model": out.Model, "stop_reason": out.StopReason, "usage": out.Usage})
	u := ports.TextUsage(out.Usage.InputTokens, out.Usage.OutputTokens, out.Usage.CacheReadInputTokens)
	if out.Usage.CacheReadInputTokens > 0 {
		u.Units["cache_read_token"] = float64(out.Usage.CacheReadInputTokens)
	}
	if out.Usage.CacheCreationInputTokens > 0 {
		u.Units["cache_write_token"] = float64(out.Usage.CacheCreationInputTokens)
	}
	if out.StopReason == "refusal" {
		msg := "request declined by safety classifier"
		if out.StopDetails != nil && out.StopDetails.Category != nil {
			msg += " (" + *out.StopDetails.Category + ")"
		}
		// Usage is still billed; callers record it from the error path
		// with Estimated=true (CONVENTIONS §6).
		return ports.TextResponse{Usage: u, Raw: raw, StopReason: out.StopReason},
			&ports.ProviderError{Kind: ports.ErrKindContentPolicy, Provider: Code, Code: "refusal", Message: msg}
	}
	var sb strings.Builder
	for _, c := range out.Content {
		if c.Type == "text" {
			sb.WriteString(c.Text)
		}
	}
	resp := ports.TextResponse{Content: sb.String(), StopReason: out.StopReason, Usage: u, Raw: raw}
	if wantJSON {
		if out.StopReason == "max_tokens" {
			return resp, &ports.ProviderError{Kind: ports.ErrKindInvalidRequest, Provider: Code, Code: "max_tokens",
				Message: "structured output truncated by max_tokens"}
		}
		js := bytes.TrimSpace([]byte(resp.Content))
		if !json.Valid(js) {
			return resp, &ports.ProviderError{Kind: ports.ErrKindUnknown, Provider: Code, Code: "invalid_json", Message: "model returned invalid JSON"}
		}
		resp.JSON = json.RawMessage(js)
	}
	return resp, nil
}

// NormalizeSchema prepares a JSON schema for structured outputs: every
// object without an explicit additionalProperties gets false (required by
// the API). The input is not modified.
func NormalizeSchema(in json.RawMessage) (json.RawMessage, error) {
	var v any
	if err := json.Unmarshal(in, &v); err != nil {
		return nil, err
	}
	walk(v)
	return json.Marshal(v)
}

func walk(v any) {
	switch t := v.(type) {
	case map[string]any:
		if t["type"] == "object" {
			if _, ok := t["additionalProperties"]; !ok {
				t["additionalProperties"] = false
			}
		}
		for _, c := range t {
			walk(c)
		}
	case []any:
		for _, c := range t {
			walk(c)
		}
	}
}
