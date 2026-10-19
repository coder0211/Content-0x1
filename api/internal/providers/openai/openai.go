// Package openai is the image adapter for OpenAI Images (gpt-image-1,
// generate + edit, OQ-01). Outputs come back base64-encoded and are decoded
// in memory (no URL is ever fetched); the caller stores them through the
// Files contract. Aspect ratios the API cannot produce natively (4:5, 9:16,
// 16:9) are generated at the nearest native size and centre-cropped.
package openai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg" // decode jpeg outputs
	"image/png"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"
	"time"

	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/providers/vendorhttp"
)

// Code is ai_providers.code.
const Code = "openai"

// Defaults.
const (
	DefaultSecretRef = "OPENAI_API_KEY" //nolint:gosec // G101: env var name
	DefaultBaseURL   = "https://api.openai.com"
	DefaultModel     = "gpt-image-1"
	maxVariants      = 4
)

// Adapter implements ports.ImageGenerator.
type Adapter struct{ EP *vendorhttp.Endpoint }

var _ ports.ImageGenerator = (*Adapter)(nil)

// New returns an adapter with production defaults (180 s timeout: high
// quality images can take over a minute).
func New() *Adapter {
	return &Adapter{EP: vendorhttp.NewEndpoint(Code, DefaultSecretRef, DefaultBaseURL, 180*time.Second)}
}

// Code implements ports.ImageGenerator.
func (*Adapter) Code() string { return Code }

// Configure applies the ai_providers row.
func (a *Adapter) Configure(secretRef string, cfg json.RawMessage) { a.EP.Configure(secretRef, cfg) }

// AspectRatios supported (native or via crop).
var AspectRatios = []string{"1:1", "4:5", "9:16", "16:9", "2:3", "3:2"}

// Qualities maps the product's quality tiers to the API's.
var Qualities = []string{"standard", "high"}

// Capabilities implements ports.ImageGenerator.
func (*Adapter) Capabilities(string) ports.Capabilities {
	return ports.Capabilities{AspectRatios: AspectRatios, Qualities: Qualities, MaxVariants: maxVariants,
		SupportsRefImage: true, SupportsEdit: true}
}

// target describes the requested output: native API size plus crop ratio.
type target struct {
	size       string
	cropW      int // ratio numerator (0 = no crop)
	cropH      int
	nativeW    int
	nativeH    int
	qualityAPI string
	unit       string
}

func plan(aspect, quality string, count int) (target, error) {
	if aspect == "" {
		aspect = "1:1"
	}
	if quality == "" {
		quality = "standard"
	}
	if err := vendorhttp.CheckOneOf("aspect_ratio", aspect, AspectRatios); err != nil {
		return target{}, err
	}
	if err := vendorhttp.CheckOneOf("quality", quality, Qualities); err != nil {
		return target{}, err
	}
	if count < 0 || count > maxVariants {
		return target{}, vendorhttp.Unsupported("count", count, map[string]int{"min": 1, "max": maxVariants})
	}
	t := target{qualityAPI: "medium", unit: "image_1024_standard"}
	if quality == "high" {
		t.qualityAPI, t.unit = "high", "image_1024_high"
	}
	switch aspect {
	case "1:1":
		t.size, t.nativeW, t.nativeH = "1024x1024", 1024, 1024
	case "2:3", "4:5", "9:16":
		t.size, t.nativeW, t.nativeH = "1024x1536", 1024, 1536
	default: // 3:2, 16:9
		t.size, t.nativeW, t.nativeH = "1536x1024", 1536, 1024
	}
	switch aspect {
	case "4:5":
		t.cropW, t.cropH = 4, 5
	case "9:16":
		t.cropW, t.cropH = 9, 16
	case "16:9":
		t.cropW, t.cropH = 16, 9
	}
	return t, nil
}

type genRequest struct {
	Model        string `json:"model"`
	Prompt       string `json:"prompt"`
	N            int    `json:"n"`
	Size         string `json:"size"`
	Quality      string `json:"quality"`
	OutputFormat string `json:"output_format"`
	Moderation   string `json:"moderation,omitempty"`
}

type imageData struct {
	B64JSON       string `json:"b64_json"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
}

type usage struct {
	InputTokens        int64 `json:"input_tokens"`
	OutputTokens       int64 `json:"output_tokens"`
	TotalTokens        int64 `json:"total_tokens"`
	InputTokensDetails struct {
		TextTokens  int64 `json:"text_tokens"`
		ImageTokens int64 `json:"image_tokens"`
	} `json:"input_tokens_details"`
}

type imagesResponse struct {
	Created int64       `json:"created"`
	Data    []imageData `json:"data"`
	Usage   *usage      `json:"usage"`
}

type errorBody struct {
	Error struct {
		Message string  `json:"message"`
		Type    string  `json:"type"`
		Param   *string `json:"param"`
		Code    *string `json:"code"`
	} `json:"error"`
}

// classify maps OpenAI errors: moderation → ContentPolicy, quota → Auth
// (needs an admin, retrying does not help), 429 → RateLimited.
func classify(status int, h http.Header, body []byte) *ports.ProviderError {
	var eb errorBody
	if json.Unmarshal(body, &eb) != nil || eb.Error.Message == "" {
		return nil //nolint:nilerr // not a vendor error envelope: caller falls back to ByStatus
	}
	code := eb.Error.Type
	if eb.Error.Code != nil && *eb.Error.Code != "" {
		code = *eb.Error.Code
	}
	pe := &ports.ProviderError{Provider: Code, Code: code, Message: eb.Error.Message}
	switch {
	case code == "moderation_blocked" || code == "content_policy_violation" || strings.Contains(eb.Error.Message, "safety system"):
		pe.Kind = ports.ErrKindContentPolicy
	case code == "insufficient_quota" || code == "billing_hard_limit_reached":
		pe.Kind = ports.ErrKindAuth
	case status == http.StatusTooManyRequests:
		pe.Kind, pe.RetryAfter = ports.ErrKindRateLimited, vendorhttp.RetryAfter(h, 20*time.Second)
	default:
		return vendorhttp.ByStatus(Code, status, h, code, eb.Error.Message)
	}
	return pe
}

func (a *Adapter) moderation() string {
	var m string
	if a.EP.Extra("moderation", &m) && (m == "low" || m == "auto") {
		return m
	}
	return "auto"
}

func buildPrompt(prompt, negative string) string {
	prompt = strings.TrimSpace(prompt)
	if n := strings.TrimSpace(negative); n != "" {
		// gpt-image-1 has no negative prompt parameter.
		prompt += "\n\nAvoid: " + n
	}
	return prompt
}

// Generate implements ports.ImageGenerator. Reference images switch the
// call to /v1/images/edits (gpt-image-1 accepts several input images).
// Seed is not supported by the API and is ignored.
func (a *Adapter) Generate(ctx context.Context, req ports.ImageRequest) (ports.ImageResponse, error) {
	t, err := plan(req.AspectRatio, req.Quality, req.Count)
	if err != nil {
		return ports.ImageResponse{}, err
	}
	n := max(req.Count, 1)
	prompt := buildPrompt(req.Prompt, req.NegativePrompt)
	if prompt == "" {
		return ports.ImageResponse{}, vendorhttp.Errorf(Code, ports.ErrKindInvalidRequest, "empty_prompt", "prompt is required")
	}
	if len(req.ReferenceImages) > 0 {
		return a.edit(ctx, modelOr(req.Model), prompt, req.ReferenceImages, nil, n, t)
	}
	key, err := a.EP.Key()
	if err != nil {
		return ports.ImageResponse{}, err
	}
	h := http.Header{}
	h.Set("Authorization", "Bearer "+key)
	body := genRequest{Model: modelOr(req.Model), Prompt: prompt, N: n, Size: t.size, Quality: t.qualityAPI, OutputFormat: "png", Moderation: a.moderation()}
	var out imagesResponse
	if _, err := a.EP.DoJSON(ctx, http.MethodPost, a.EP.BaseURL()+"/v1/images/generations", h, body, &out, classify); err != nil {
		return ports.ImageResponse{}, err
	}
	return finish(out, t, n)
}

// Edit implements ports.ImageGenerator (image + optional PNG mask).
func (a *Adapter) Edit(ctx context.Context, req ports.ImageEditRequest) (ports.ImageResponse, error) {
	t, err := plan(req.AspectRatio, req.Quality, req.Count)
	if err != nil {
		return ports.ImageResponse{}, err
	}
	if len(req.Image.Data) == 0 {
		return ports.ImageResponse{}, vendorhttp.Errorf(Code, ports.ErrKindInvalidRequest, "image_required", "image is required")
	}
	if req.Mask != nil && req.Mask.MimeType != "" && req.Mask.MimeType != "image/png" {
		return ports.ImageResponse{}, vendorhttp.Unsupported("mask.mime_type", req.Mask.MimeType, []string{"image/png"})
	}
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		return ports.ImageResponse{}, vendorhttp.Errorf(Code, ports.ErrKindInvalidRequest, "empty_prompt", "prompt is required")
	}
	return a.edit(ctx, modelOr(req.Model), prompt, []ports.MediaInput{req.Image}, req.Mask, max(req.Count, 1), t)
}

func modelOr(m string) string {
	if m == "" {
		return DefaultModel
	}
	return m
}

var extByMime = map[string]string{"image/png": "png", "image/jpeg": "jpg", "image/webp": "webp"}

func (a *Adapter) edit(ctx context.Context, model, prompt string, imgs []ports.MediaInput, mask *ports.MediaInput, n int, t target) (ports.ImageResponse, error) {
	key, err := a.EP.Key()
	if err != nil {
		return ports.ImageResponse{}, err
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fields := [][2]string{{"model", model}, {"prompt", prompt}, {"n", strconv.Itoa(n)}, {"size", t.size},
		{"quality", t.qualityAPI}, {"output_format", "png"}, {"moderation", a.moderation()}}
	for _, f := range fields {
		_ = mw.WriteField(f[0], f[1])
	}
	addFile := func(field string, i int, m ports.MediaInput) error {
		ext, ok := extByMime[m.MimeType]
		if !ok {
			return vendorhttp.Unsupported(field+".mime_type", m.MimeType, []string{"image/png", "image/jpeg", "image/webp"})
		}
		hdr := make(textproto.MIMEHeader)
		hdr.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="input-%d.%s"`, field, i, ext))
		hdr.Set("Content-Type", m.MimeType)
		w, err := mw.CreatePart(hdr)
		if err != nil {
			return err
		}
		_, err = w.Write(m.Data)
		return err
	}
	field := "image"
	if len(imgs) > 1 {
		field = "image[]"
	}
	for i, img := range imgs {
		if err := addFile(field, i, img); err != nil {
			return ports.ImageResponse{}, err
		}
	}
	if mask != nil {
		m := *mask
		if m.MimeType == "" {
			m.MimeType = "image/png"
		}
		if err := addFile("mask", 0, m); err != nil {
			return ports.ImageResponse{}, err
		}
	}
	_ = mw.Close()
	h := http.Header{}
	h.Set("Authorization", "Bearer "+key)
	resp, err := a.EP.Do(ctx, vendorhttp.Request{Method: http.MethodPost, URL: a.EP.BaseURL() + "/v1/images/edits", Header: h,
		Body: &buf, ContentType: mw.FormDataContentType()}, 128<<20, classify)
	if err != nil {
		return ports.ImageResponse{}, err
	}
	var out imagesResponse
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		return ports.ImageResponse{}, vendorhttp.Errorf(Code, ports.ErrKindUnknown, "decode", "unexpected response: %v", err)
	}
	return finish(out, t, n)
}

func finish(out imagesResponse, t target, n int) (ports.ImageResponse, error) {
	if len(out.Data) == 0 {
		return ports.ImageResponse{}, vendorhttp.Errorf(Code, ports.ErrKindUnknown, "no_images", "provider returned no images")
	}
	res := ports.ImageResponse{}
	for i, d := range out.Data {
		raw, err := base64.StdEncoding.DecodeString(d.B64JSON)
		if err != nil {
			return ports.ImageResponse{}, vendorhttp.Errorf(Code, ports.ErrKindUnknown, "decode_image", "image %d: %v", i, err)
		}
		img, err := cropToRatio(raw, t.cropW, t.cropH)
		if err != nil {
			return ports.ImageResponse{}, vendorhttp.Errorf(Code, ports.ErrKindUnknown, "decode_image", "image %d: %v", i, err)
		}
		res.Images = append(res.Images, img)
	}
	u := ports.Usage{Images: len(res.Images), Units: map[string]float64{t.unit: float64(len(res.Images))}}
	if out.Usage != nil {
		u.InputTokens, u.OutputTokens = out.Usage.InputTokens, out.Usage.OutputTokens
	}
	res.Usage = u
	res.Raw = vendorhttp.StripRaw(map[string]any{"created": out.Created, "requested": n, "returned": len(out.Data),
		"size": t.size, "quality": t.qualityAPI, "usage": out.Usage})
	return res, nil
}

// cropToRatio centre-crops a PNG/JPEG to w:h (no-op when w==0) and returns a PNG.
func cropToRatio(data []byte, rw, rh int) (ports.GeneratedImage, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return ports.GeneratedImage{}, err
	}
	b := img.Bounds()
	if rw == 0 {
		return ports.GeneratedImage{MimeType: "image/png", Data: data, Width: b.Dx(), Height: b.Dy()}, nil
	}
	w, h := b.Dx(), b.Dy()
	cw, ch := w, w*rh/rw
	if ch > h {
		ch, cw = h, h*rw/rh
	}
	x0, y0 := b.Min.X+(w-cw)/2, b.Min.Y+(h-ch)/2
	dst := image.NewRGBA(image.Rect(0, 0, cw, ch))
	draw.Draw(dst, dst.Bounds(), img, image.Pt(x0, y0), draw.Src)
	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return ports.GeneratedImage{}, err
	}
	return ports.GeneratedImage{MimeType: "image/png", Data: buf.Bytes(), Width: cw, Height: ch}, nil
}
