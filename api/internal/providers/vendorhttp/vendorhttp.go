// Package vendorhttp holds the plumbing shared by the real AI vendor
// adapters: live-reconfigurable endpoint settings (secret_ref, base_url,
// timeout from ai_providers), JSON requests over net/http, and the mapping of
// HTTP failures to ports.ProviderError (SPEC-02 §4.2). Keys are resolved per
// call via config.Secret and never logged or included in errors.
package vendorhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/be/acs/api/internal/platform/config"
	"github.com/be/acs/api/internal/ports"
)

// MaxErrorBody caps how much of an error body is read (and logged).
const MaxErrorBody = 16 << 10

// Endpoint is the live configuration of one vendor. It starts from the
// adapter's defaults and is updated by the provider module from the
// ai_providers row (Configure). Safe for concurrent use.
type Endpoint struct {
	Provider string

	mu        sync.RWMutex
	secretRef string
	baseURL   string
	timeout   time.Duration
	extra     map[string]json.RawMessage
	keyFunc   func(ref string) string
	client    *http.Client
}

// NewEndpoint returns an endpoint with defaults.
func NewEndpoint(provider, secretRef, baseURL string, timeout time.Duration) *Endpoint {
	return &Endpoint{Provider: provider, secretRef: secretRef, baseURL: strings.TrimRight(baseURL, "/"), timeout: timeout,
		extra: map[string]json.RawMessage{}, keyFunc: config.Secret, client: &http.Client{}}
}

// WithKeyFunc overrides how secret refs are resolved (tests).
func (e *Endpoint) WithKeyFunc(f func(string) string) *Endpoint {
	e.mu.Lock()
	e.keyFunc = f
	e.mu.Unlock()
	return e
}

// WithHTTPClient overrides the HTTP client (tests).
func (e *Endpoint) WithHTTPClient(c *http.Client) *Endpoint {
	e.mu.Lock()
	e.client = c
	e.mu.Unlock()
	return e
}

// SetBaseURL overrides the base URL without validation (tests, httptest).
func (e *Endpoint) SetBaseURL(u string) *Endpoint {
	e.mu.Lock()
	e.baseURL = strings.TrimRight(u, "/")
	e.mu.Unlock()
	return e
}

// Configure applies an ai_providers row. secretRef "" keeps the current one.
// config_json keys: base_url (https only), timeout_sec, plus vendor-specific
// keys read with Extra. Invalid values are ignored (defaults stay).
func (e *Endpoint) Configure(secretRef string, cfg json.RawMessage) {
	var m map[string]json.RawMessage
	if len(cfg) > 0 {
		_ = json.Unmarshal(cfg, &m)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if secretRef != "" {
		e.secretRef = secretRef
	}
	if m == nil {
		m = map[string]json.RawMessage{}
	}
	e.extra = m
	var s string
	if raw, ok := m["base_url"]; ok && json.Unmarshal(raw, &s) == nil {
		if u, err := url.Parse(s); err == nil && u.Scheme == "https" && u.Host != "" {
			e.baseURL = strings.TrimRight(s, "/")
		}
	}
	var sec float64
	if raw, ok := m["timeout_sec"]; ok && json.Unmarshal(raw, &sec) == nil && sec > 0 && sec <= 900 {
		e.timeout = time.Duration(sec * float64(time.Second))
	}
}

// Extra decodes a vendor-specific config_json key into v; false when absent
// or invalid.
func (e *Endpoint) Extra(key string, v any) bool {
	e.mu.RLock()
	raw, ok := e.extra[key]
	e.mu.RUnlock()
	return ok && json.Unmarshal(raw, v) == nil
}

// BaseURL returns the current base URL.
func (e *Endpoint) BaseURL() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.baseURL
}

// SecretRef returns the configured secret name (never the value).
func (e *Endpoint) SecretRef() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.secretRef
}

// Key resolves the API key or returns an Auth ProviderError when missing.
func (e *Endpoint) Key() (string, error) {
	e.mu.RLock()
	ref, f := e.secretRef, e.keyFunc
	e.mu.RUnlock()
	k := strings.TrimSpace(f(ref))
	if k == "" {
		return "", &ports.ProviderError{Kind: ports.ErrKindAuth, Provider: e.Provider, Code: "missing_key",
			Message: "secret " + ref + " is not set"}
	}
	return k, nil
}

// Client returns the HTTP client.
func (e *Endpoint) Client() *http.Client {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.client
}

// Timeout returns the per-call timeout.
func (e *Endpoint) Timeout() time.Duration {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.timeout
}

// Request describes one HTTP call.
type Request struct {
	Method      string
	URL         string // absolute
	Header      http.Header
	Body        io.Reader
	ContentType string
}

// Response is a fully read successful response.
type Response struct {
	Status int
	Header http.Header
	Body   []byte
}

// Classifier maps a non-2xx response to a ProviderError; nil means use the
// default status mapping.
type Classifier func(status int, header http.Header, body []byte) *ports.ProviderError

// Do executes req with the endpoint timeout. Non-2xx responses are mapped by
// classify (falling back to ByStatus); transport failures are Transient.
func (e *Endpoint) Do(ctx context.Context, req Request, maxBody int64, classify Classifier) (*Response, error) {
	if t := e.Timeout(); t > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, t)
		defer cancel()
	}
	hr, err := http.NewRequestWithContext(ctx, req.Method, req.URL, req.Body)
	if err != nil {
		return nil, &ports.ProviderError{Kind: ports.ErrKindInvalidRequest, Provider: e.Provider, Code: "bad_request", Message: err.Error(), Err: err}
	}
	for k, vs := range req.Header {
		for _, v := range vs {
			hr.Header.Add(k, v)
		}
	}
	if req.ContentType != "" {
		hr.Header.Set("Content-Type", req.ContentType)
	}
	resp, err := e.Client().Do(hr)
	if err != nil {
		return nil, e.TransportError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, MaxErrorBody))
		if classify != nil {
			if pe := classify(resp.StatusCode, resp.Header, body); pe != nil {
				return nil, pe
			}
		}
		return nil, ByStatus(e.Provider, resp.StatusCode, resp.Header, "http_"+strconv.Itoa(resp.StatusCode), Snippet(body))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return nil, e.TransportError(err)
	}
	if int64(len(body)) > maxBody {
		return nil, &ports.ProviderError{Kind: ports.ErrKindUnknown, Provider: e.Provider, Code: "response_too_large", Message: "response exceeds limit"}
	}
	return &Response{Status: resp.StatusCode, Header: resp.Header, Body: body}, nil
}

// DoJSON marshals in, calls Do and unmarshals the body into out.
func (e *Endpoint) DoJSON(ctx context.Context, method, url string, header http.Header, in, out any, classify Classifier) (*Response, error) {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return nil, &ports.ProviderError{Kind: ports.ErrKindInvalidRequest, Provider: e.Provider, Code: "encode", Message: err.Error(), Err: err}
		}
		body = bytes.NewReader(b)
	}
	resp, err := e.Do(ctx, Request{Method: method, URL: url, Header: header, Body: body, ContentType: "application/json"}, 64<<20, classify)
	if err != nil {
		return nil, err
	}
	if out != nil {
		if err := json.Unmarshal(resp.Body, out); err != nil {
			return resp, &ports.ProviderError{Kind: ports.ErrKindUnknown, Provider: e.Provider, Code: "decode", Message: "unexpected response: " + err.Error(), Err: err}
		}
	}
	return resp, nil
}

// TransportError classifies network failures as Transient (timeouts,
// resets) — the job engine retries them.
func (e *Endpoint) TransportError(err error) *ports.ProviderError {
	code := "network"
	var ne net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout()) {
		code = "timeout"
	}
	if errors.Is(err, context.Canceled) {
		code = "cancelled"
	}
	return &ports.ProviderError{Kind: ports.ErrKindTransient, Provider: e.Provider, Code: code, Message: redact(err.Error()), Err: err}
}

// redact strips query strings (which may carry keys) from URLs in messages.
func redact(s string) string {
	for _, marker := range []string{"key=", "access_token="} {
		if i := strings.Index(s, marker); i >= 0 {
			j := strings.IndexAny(s[i:], "\" &")
			if j < 0 {
				return s[:i] + marker + "REDACTED"
			}
			s = s[:i] + marker + "REDACTED" + s[i+j:]
		}
	}
	return s
}

// ByStatus is the default HTTP status → ErrKind mapping:
// 429 RateLimited (Retry-After honoured), 401/403 Auth, 408/409/5xx
// Transient, other 4xx InvalidRequest.
func ByStatus(provider string, status int, h http.Header, code, msg string) *ports.ProviderError {
	pe := &ports.ProviderError{Provider: provider, Code: code, Message: msg}
	switch {
	case status == http.StatusTooManyRequests:
		pe.Kind = ports.ErrKindRateLimited
		pe.RetryAfter = RetryAfter(h, 30*time.Second)
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		pe.Kind = ports.ErrKindAuth
	case status == http.StatusRequestTimeout || status == http.StatusConflict || status >= 500:
		pe.Kind = ports.ErrKindTransient
		if d := RetryAfter(h, 0); d > 0 {
			pe.RetryAfter = d
		}
	case status >= 400:
		pe.Kind = ports.ErrKindInvalidRequest
	default:
		pe.Kind = ports.ErrKindUnknown
	}
	return pe
}

// RetryAfter parses Retry-After (seconds or HTTP date) and the
// retry-after-ms extension; def when absent. Capped at 10 minutes.
func RetryAfter(h http.Header, def time.Duration) time.Duration {
	if h == nil {
		return def
	}
	if v := h.Get("Retry-After-Ms"); v != "" {
		if ms, err := strconv.ParseFloat(v, 64); err == nil && ms > 0 {
			return capDur(time.Duration(ms * float64(time.Millisecond)))
		}
	}
	v := strings.TrimSpace(h.Get("Retry-After"))
	if v == "" {
		return def
	}
	if s, err := strconv.ParseFloat(v, 64); err == nil && s >= 0 {
		return capDur(time.Duration(s * float64(time.Second)))
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := time.Until(t); d > 0 {
			return capDur(d)
		}
		return 0
	}
	return def
}

func capDur(d time.Duration) time.Duration {
	if d > 10*time.Minute {
		return 10 * time.Minute
	}
	return d
}

// Snippet returns a short, single-line excerpt of a vendor body for
// ProviderError.Message.
func Snippet(b []byte) string {
	s := strings.Join(strings.Fields(string(b)), " ")
	if len(s) > 300 {
		s = s[:300] + "…"
	}
	return redact(s)
}

// Errorf builds a ProviderError.
func Errorf(provider string, kind ports.ErrKind, code, format string, args ...any) *ports.ProviderError {
	return &ports.ProviderError{Kind: kind, Provider: provider, Code: code, Message: fmt.Sprintf(format, args...)}
}

// StripRaw re-marshals a vendor payload after dropping large/sensitive
// fields (base64 media) so it can be stored in ai_job_steps.output_json.
func StripRaw(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return b
}
