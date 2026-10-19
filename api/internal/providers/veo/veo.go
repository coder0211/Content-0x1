// Package veo is the video adapter for Google Veo through the Gemini API
// (OQ-01): Submit = models/{model}:predictLongRunning, GetStatus = GET the
// long-running operation, Download = GET the generated file URI (only on the
// configured Gemini host — never an arbitrary URL). The Gemini API has no
// documented cancel for Veo operations, so Cancel returns
// ports.ErrCancelUnsupported (usage is still recorded by the caller).
package veo

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/providers/vendorhttp"
)

// Code is ai_providers.code.
const Code = "veo"

// Defaults.
const (
	DefaultSecretRef = "GEMINI_API_KEY" //nolint:gosec // G101: env var name
	DefaultBaseURL   = "https://generativelanguage.googleapis.com"
	DefaultModel     = "veo-3.1-generate-preview"
	maxClipBytes     = 512 << 20
)

// SupportedDurations are the clip lengths Veo 3.x accepts. Shorter scenes
// are rounded up and trimmed by the normalize step (NormalizeSpec.DurationMs).
var SupportedDurations = []int{4, 6, 8}

// Adapter implements ports.VideoGenerator.
type Adapter struct{ EP *vendorhttp.Endpoint }

var _ ports.VideoGenerator = (*Adapter)(nil)

// New returns an adapter with production defaults.
func New() *Adapter {
	return &Adapter{EP: vendorhttp.NewEndpoint(Code, DefaultSecretRef, DefaultBaseURL, 120*time.Second)}
}

// Code implements ports.VideoGenerator.
func (*Adapter) Code() string { return Code }

// Configure applies the ai_providers row.
func (a *Adapter) Configure(secretRef string, cfg json.RawMessage) { a.EP.Configure(secretRef, cfg) }

// Capabilities implements ports.VideoGenerator (Veo 3.1: 16:9 and 9:16 only;
// 1:1 is produced by the kenburns fallback or by pad in normalize).
func (*Adapter) Capabilities(string) ports.Capabilities {
	return ports.Capabilities{AspectRatios: []string{"16:9", "9:16"}, Resolutions: []string{"720p", "1080p"},
		MinDurationSec: 2, MaxDurationSec: 8, SupportsRefImage: true, SupportsCancel: false}
}

type imageInput struct {
	BytesBase64Encoded string `json:"bytesBase64Encoded"`
	MimeType           string `json:"mimeType"`
}

type instance struct {
	Prompt string      `json:"prompt"`
	Image  *imageInput `json:"image,omitempty"`
}

type parameters struct {
	AspectRatio      string `json:"aspectRatio,omitempty"`
	NegativePrompt   string `json:"negativePrompt,omitempty"`
	DurationSeconds  int    `json:"durationSeconds,omitempty"`
	Resolution       string `json:"resolution,omitempty"`
	SampleCount      int    `json:"sampleCount"`
	Seed             *int64 `json:"seed,omitempty"`
	PersonGeneration string `json:"personGeneration,omitempty"`
}

type submitRequest struct {
	Instances  []instance `json:"instances"`
	Parameters parameters `json:"parameters"`
}

type operation struct {
	Name     string `json:"name"`
	Done     bool   `json:"done"`
	Metadata *struct {
		ProgressPercent int `json:"progressPercent"`
	} `json:"metadata,omitempty"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error,omitempty"`
	Response *struct {
		GenerateVideoResponse struct {
			GeneratedSamples []struct {
				Video struct {
					URI      string `json:"uri"`
					MimeType string `json:"mimeType,omitempty"`
				} `json:"video"`
			} `json:"generatedSamples"`
			RaiMediaFilteredCount   int      `json:"raiMediaFilteredCount"`
			RaiMediaFilteredReasons []string `json:"raiMediaFilteredReasons"`
		} `json:"generateVideoResponse"`
	} `json:"response,omitempty"`
}

type googleError struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}

var policyWords = []string{"safety", "policy", "blocked", "sensitive", "responsible ai", "prohibited", "rai"}

func isPolicy(msg string) bool {
	m := strings.ToLower(msg)
	for _, w := range policyWords {
		if strings.Contains(m, w) {
			return true
		}
	}
	return false
}

func classify(status int, h http.Header, body []byte) *ports.ProviderError {
	var ge googleError
	if json.Unmarshal(body, &ge) != nil || ge.Error.Message == "" {
		return nil
	}
	code := ge.Error.Status
	if code == "" {
		code = strconv.Itoa(status)
	}
	if status == http.StatusBadRequest && isPolicy(ge.Error.Message) {
		return &ports.ProviderError{Kind: ports.ErrKindContentPolicy, Provider: Code, Code: code, Message: ge.Error.Message}
	}
	pe := vendorhttp.ByStatus(Code, status, h, code, ge.Error.Message)
	if pe.Kind == ports.ErrKindRateLimited && pe.RetryAfter == 30*time.Second {
		pe.RetryAfter = 60 * time.Second // Veo quotas are per minute
	}
	return pe
}

// jobID packs what GetStatus needs to bill the clip: "veo|<res>|<sec>|<operation>".
func jobID(res string, sec int, op string) string {
	return "veo|" + res + "|" + strconv.Itoa(sec) + "|" + op
}

var modelCode = regexp.MustCompile(`^[A-Za-z0-9.\-]+$`)

var opName = regexp.MustCompile(`^models/[A-Za-z0-9.\-]+/operations/[A-Za-z0-9_\-]+$`)

func parseJobID(id string) (res string, sec int, op string, err error) {
	parts := strings.SplitN(id, "|", 4)
	if len(parts) != 4 || parts[0] != "veo" || !opName.MatchString(parts[3]) {
		return "", 0, "", vendorhttp.Errorf(Code, ports.ErrKindInvalidRequest, "bad_job_id", "malformed provider job id")
	}
	sec, err = strconv.Atoi(parts[2])
	if err != nil {
		return "", 0, "", vendorhttp.Errorf(Code, ports.ErrKindInvalidRequest, "bad_job_id", "malformed provider job id")
	}
	return parts[1], sec, parts[3], nil
}

func (a *Adapter) header() (http.Header, error) {
	key, err := a.EP.Key()
	if err != nil {
		return nil, err
	}
	h := http.Header{}
	h.Set("x-goog-api-key", key)
	return h, nil
}

// clipSeconds rounds a requested scene length up to a supported Veo length.
func clipSeconds(d int) (int, error) {
	if d <= 0 {
		d = 8
	}
	for _, s := range SupportedDurations {
		if d <= s {
			return s, nil
		}
	}
	return 0, vendorhttp.Unsupported("duration_sec", d, map[string]int{"min": 2, "max": 8})
}

// Submit implements ports.VideoGenerator (text-to-video, or image-to-video
// when req.Image is set).
func (a *Adapter) Submit(ctx context.Context, req ports.VideoRequest) (ports.VideoSubmission, error) {
	caps := a.Capabilities(req.Model)
	if err := vendorhttp.CheckOneOf("aspect_ratio", req.AspectRatio, caps.AspectRatios); err != nil {
		return ports.VideoSubmission{}, err
	}
	if err := vendorhttp.CheckOneOf("resolution", req.Resolution, caps.Resolutions); err != nil {
		return ports.VideoSubmission{}, err
	}
	sec, err := clipSeconds(req.DurationSec)
	if err != nil {
		return ports.VideoSubmission{}, err
	}
	if strings.TrimSpace(req.Prompt) == "" && req.Image == nil {
		return ports.VideoSubmission{}, vendorhttp.Errorf(Code, ports.ErrKindInvalidRequest, "empty_prompt", "prompt or image is required")
	}
	h, err := a.header()
	if err != nil {
		return ports.VideoSubmission{}, err
	}
	res := req.Resolution
	if res == "" {
		res = "720p"
	}
	model := req.Model
	if model == "" {
		model = DefaultModel
	}
	if !modelCode.MatchString(model) {
		return ports.VideoSubmission{}, vendorhttp.Unsupported("model_code", model, []string{DefaultModel})
	}
	in := instance{Prompt: req.Prompt}
	if req.Image != nil {
		in.Image = &imageInput{BytesBase64Encoded: base64.StdEncoding.EncodeToString(req.Image.Data), MimeType: req.Image.MimeType}
	}
	person := "allow_adult"
	_ = a.EP.Extra("person_generation", &person)
	body := submitRequest{Instances: []instance{in}, Parameters: parameters{
		AspectRatio: req.AspectRatio, NegativePrompt: req.NegativePrompt, DurationSeconds: sec,
		Resolution: res, SampleCount: 1, Seed: req.Seed, PersonGeneration: person,
	}}
	var op operation
	u := a.EP.BaseURL() + "/v1beta/models/" + model + ":predictLongRunning"
	if _, err := a.EP.DoJSON(ctx, http.MethodPost, u, h, body, &op, classify); err != nil {
		return ports.VideoSubmission{}, err
	}
	if !opName.MatchString(op.Name) {
		return ports.VideoSubmission{}, vendorhttp.Errorf(Code, ports.ErrKindUnknown, "bad_operation", "unexpected operation name")
	}
	return ports.VideoSubmission{ProviderJobID: jobID(res, sec, op.Name), PollAfter: 10 * time.Second}, nil
}

// GetStatus implements ports.VideoGenerator.
func (a *Adapter) GetStatus(ctx context.Context, id string) (ports.VideoStatus, error) {
	res, sec, name, err := parseJobID(id)
	if err != nil {
		return ports.VideoStatus{}, err
	}
	h, err := a.header()
	if err != nil {
		return ports.VideoStatus{}, err
	}
	var op operation
	resp, err := a.EP.DoJSON(ctx, http.MethodGet, a.EP.BaseURL()+"/v1beta/"+name, h, nil, &op, classify)
	if err != nil {
		return ports.VideoStatus{}, err
	}
	st := ports.VideoStatus{Raw: stripOp(resp.Body)}
	if !op.Done {
		st.State = ports.VideoRunning
		if op.Metadata != nil {
			st.Progress = op.Metadata.ProgressPercent
		}
		return st, nil
	}
	if op.Error != nil {
		st.State = ports.VideoFailed
		st.Error = opError(op.Error.Code, op.Error.Status, op.Error.Message)
		return st, nil
	}
	if op.Response == nil || len(op.Response.GenerateVideoResponse.GeneratedSamples) == 0 {
		st.State = ports.VideoFailed
		msg := "no video returned"
		if r := op.Response; r != nil && r.GenerateVideoResponse.RaiMediaFilteredCount > 0 {
			msg = "filtered by safety policy: " + strings.Join(r.GenerateVideoResponse.RaiMediaFilteredReasons, "; ")
			st.Error = &ports.ProviderError{Kind: ports.ErrKindContentPolicy, Provider: Code, Code: "rai_filtered", Message: msg}
		} else {
			st.Error = &ports.ProviderError{Kind: ports.ErrKindUnknown, Provider: Code, Code: "empty_response", Message: msg}
		}
		return st, nil
	}
	u := ports.Usage{VideoSeconds: float64(sec), Units: map[string]float64{"video_second_" + res: float64(sec)}}
	st.State, st.Progress, st.DurationSec, st.Usage = ports.VideoSucceeded, 100, float64(sec), &u
	return st, nil
}

// opError maps google.rpc codes of a finished operation.
func opError(code int, status, msg string) *ports.ProviderError {
	pe := &ports.ProviderError{Provider: Code, Code: status, Message: msg}
	if pe.Code == "" {
		pe.Code = "rpc_" + strconv.Itoa(code)
	}
	switch {
	case isPolicy(msg):
		pe.Kind = ports.ErrKindContentPolicy
	case code == 8: // RESOURCE_EXHAUSTED
		pe.Kind, pe.RetryAfter = ports.ErrKindRateLimited, 60*time.Second
	case code == 4 || code == 13 || code == 14 || code == 10: // DEADLINE, INTERNAL, UNAVAILABLE, ABORTED
		pe.Kind = ports.ErrKindTransient
	case code == 7 || code == 16:
		pe.Kind = ports.ErrKindAuth
	case code == 3 || code == 9 || code == 11: // INVALID_ARGUMENT, FAILED_PRECONDITION, OUT_OF_RANGE
		pe.Kind = ports.ErrKindInvalidRequest
	default:
		pe.Kind = ports.ErrKindUnknown
	}
	return pe
}

func stripOp(b []byte) json.RawMessage {
	if json.Valid(b) && len(b) < 64<<10 {
		return json.RawMessage(b)
	}
	return json.RawMessage(`{}`)
}

// Download implements ports.VideoGenerator: re-reads the operation, then
// streams the first sample from the Gemini files endpoint.
func (a *Adapter) Download(ctx context.Context, id string) (ports.MediaStream, error) {
	_, _, name, err := parseJobID(id)
	if err != nil {
		return ports.MediaStream{}, err
	}
	h, err := a.header()
	if err != nil {
		return ports.MediaStream{}, err
	}
	var op operation
	if _, err := a.EP.DoJSON(ctx, http.MethodGet, a.EP.BaseURL()+"/v1beta/"+name, h, nil, &op, classify); err != nil {
		return ports.MediaStream{}, err
	}
	if !op.Done || op.Response == nil || len(op.Response.GenerateVideoResponse.GeneratedSamples) == 0 {
		return ports.MediaStream{}, vendorhttp.Errorf(Code, ports.ErrKindInvalidRequest, "not_ready", "video is not available")
	}
	uri := op.Response.GenerateVideoResponse.GeneratedSamples[0].Video.URI
	if err := a.sameHost(uri); err != nil {
		return ports.MediaStream{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
	if err != nil {
		return ports.MediaStream{}, vendorhttp.Errorf(Code, ports.ErrKindUnknown, "bad_uri", "invalid download uri")
	}
	req.Header = h
	resp, err := a.EP.Client().Do(req) //nolint:gosec // G107: host checked against the configured Gemini base URL
	if err != nil {
		return ports.MediaStream{}, a.EP.TransportError(err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, vendorhttp.MaxErrorBody))
		_ = resp.Body.Close()
		if pe := classify(resp.StatusCode, resp.Header, body); pe != nil {
			return ports.MediaStream{}, pe
		}
		return ports.MediaStream{}, vendorhttp.ByStatus(Code, resp.StatusCode, resp.Header, "download_"+strconv.Itoa(resp.StatusCode), vendorhttp.Snippet(body))
	}
	mt := resp.Header.Get("Content-Type")
	if mt == "" || strings.HasPrefix(mt, "application/octet-stream") {
		mt = "video/mp4"
	}
	return ports.MediaStream{Body: limitedBody{io.LimitReader(resp.Body, maxClipBytes), resp.Body}, MimeType: mt, Size: resp.ContentLength}, nil
}

type limitedBody struct {
	io.Reader
	c io.Closer
}

func (l limitedBody) Close() error { return l.c.Close() }

func (a *Adapter) sameHost(uri string) error {
	u, err := url.Parse(uri)
	base, berr := url.Parse(a.EP.BaseURL())
	if err != nil || berr != nil || u.Host != base.Host || u.Scheme != base.Scheme {
		return &ports.ProviderError{Kind: ports.ErrKindUnknown, Provider: Code, Code: "unexpected_uri",
			Message: fmt.Sprintf("download host %q is not the provider host", hostOf(u))}
	}
	return nil
}

func hostOf(u *url.URL) string {
	if u == nil {
		return ""
	}
	return u.Host
}

// Cancel implements ports.VideoGenerator: unsupported by the Gemini API.
func (a *Adapter) Cancel(_ context.Context, id string) error {
	if _, _, _, err := parseJobID(id); err != nil {
		return err
	}
	return ports.ErrCancelUnsupported
}
