// Package kenburns is the internal fallback video provider (OQ-01): it turns
// the scene's still image into a clip with a slow zoom (FFmpeg zoompan via
// ports.VideoRenderer, which execs FFmpeg with an argument vector — never a
// shell). It costs nothing (priced 0 in migration 0070) and needs no key.
//
// The clip is rendered synchronously inside Submit (a few seconds of FFmpeg
// on the video queue) and parked in object storage under
// providers/kenburns/<uuid>.mp4 so that GetStatus/Download work from any
// worker process. Without storage (unit tests, CLI) it falls back to a local
// directory.
package kenburns

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/providers/vendorhttp"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Code is ai_providers.code.
const Code = "kenburns"

// DefaultModel is the only model.
const DefaultModel = "kenburns-zoompan"

const (
	fps          = 30
	keyPrefix    = "providers/kenburns/"
	maxImageSize = 25 << 20
)

// Adapter implements ports.VideoGenerator.
type Adapter struct {
	Renderer ports.VideoRenderer
	// TempDir is the scratch directory for FFmpeg inputs/outputs and the
	// local fallback store.
	TempDir string

	mu    sync.RWMutex
	store ports.ObjectStorage
}

var _ ports.VideoGenerator = (*Adapter)(nil)

// New returns an adapter rendering with r in tempDir.
func New(r ports.VideoRenderer, tempDir string) *Adapter {
	if tempDir == "" {
		tempDir = os.TempDir()
	}
	return &Adapter{Renderer: r, TempDir: tempDir}
}

// UseStorage parks rendered clips in object storage (shared by every
// worker). Called once at start-up by the provider module.
func (a *Adapter) UseStorage(s ports.ObjectStorage) {
	a.mu.Lock()
	a.store = s
	a.mu.Unlock()
}

func (a *Adapter) storage() ports.ObjectStorage {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.store
}

// Code implements ports.VideoGenerator.
func (*Adapter) Code() string { return Code }

// Configure is a no-op (no key, no settings) kept for registry symmetry.
func (*Adapter) Configure(string, json.RawMessage) {}

// Capabilities implements ports.VideoGenerator. An input image is required
// (image-to-video only).
func (*Adapter) Capabilities(string) ports.Capabilities {
	return ports.Capabilities{AspectRatios: []string{"9:16", "1:1", "16:9"}, Resolutions: []string{"720p", "1080p"},
		MinDurationSec: 2, MaxDurationSec: 8, SupportsRefImage: true, SupportsCancel: true}
}

// Format returns the output size for an aspect ratio and resolution (OQ-07
// presets at 1080p).
func Format(aspect, res string) ports.OutputFormat {
	short := 1080
	if res == "720p" {
		short = 720
	}
	long := short * 16 / 9
	switch aspect {
	case "1:1":
		return ports.OutputFormat{Width: short, Height: short, FPS: fps}
	case "16:9":
		return ports.OutputFormat{Width: long, Height: short, FPS: fps}
	default:
		return ports.OutputFormat{Width: short, Height: long, FPS: fps}
	}
}

var extByMime = map[string]string{"image/png": ".png", "image/jpeg": ".jpg", "image/webp": ".webp"}

// jobID is "kb|<res>|<sec>|<uuid>".
func parseID(id string) (res string, sec int, u string, err error) {
	parts := strings.Split(id, "|")
	bad := vendorhttp.Errorf(Code, ports.ErrKindInvalidRequest, "bad_job_id", "malformed provider job id")
	if len(parts) != 4 || parts[0] != "kb" || (parts[1] != "720p" && parts[1] != "1080p") {
		return "", 0, "", bad
	}
	if _, perr := uuid.Parse(parts[3]); perr != nil {
		return "", 0, "", bad
	}
	sec, err = strconv.Atoi(parts[2])
	if err != nil {
		return "", 0, "", bad
	}
	return parts[1], sec, parts[3], nil
}

var safeName = regexp.MustCompile(`^[0-9a-f-]{36}$`)

func (a *Adapter) localPath(u string) string {
	return filepath.Join(a.TempDir, "acs-kenburns", u+".mp4")
}

// Submit implements ports.VideoGenerator: renders the clip now.
func (a *Adapter) Submit(ctx context.Context, req ports.VideoRequest) (ports.VideoSubmission, error) {
	caps := a.Capabilities(req.Model)
	if err := vendorhttp.CheckOneOf("aspect_ratio", req.AspectRatio, caps.AspectRatios); err != nil {
		return ports.VideoSubmission{}, err
	}
	if err := vendorhttp.CheckOneOf("resolution", req.Resolution, caps.Resolutions); err != nil {
		return ports.VideoSubmission{}, err
	}
	sec := req.DurationSec
	if sec == 0 {
		sec = 4
	}
	if sec < caps.MinDurationSec || sec > caps.MaxDurationSec {
		return ports.VideoSubmission{}, vendorhttp.Unsupported("duration_sec", req.DurationSec,
			map[string]int{"min": caps.MinDurationSec, "max": caps.MaxDurationSec})
	}
	if req.Image == nil || len(req.Image.Data) == 0 {
		return ports.VideoSubmission{}, kernel.ErrCapability.WithMessage("Nhà cung cấp kenburns cần ảnh đầu vào cho mỗi cảnh").
			WithDetails(kernel.Detail{"field": "image", "issue": "required"})
	}
	ext, ok := extByMime[req.Image.MimeType]
	if !ok {
		return ports.VideoSubmission{}, vendorhttp.Unsupported("image.mime_type", req.Image.MimeType, []string{"image/png", "image/jpeg", "image/webp"})
	}
	if len(req.Image.Data) > maxImageSize {
		return ports.VideoSubmission{}, vendorhttp.Errorf(Code, ports.ErrKindInvalidRequest, "image_too_large", "image exceeds %d bytes", maxImageSize)
	}
	if a.Renderer == nil {
		return ports.VideoSubmission{}, vendorhttp.Errorf(Code, ports.ErrKindUnknown, "no_renderer", "ffmpeg renderer is not configured")
	}
	res := req.Resolution
	if res == "" {
		res = "1080p"
	}
	aspect := req.AspectRatio
	if aspect == "" {
		aspect = "9:16"
	}
	work, err := os.MkdirTemp(a.TempDir, "kenburns-*")
	if err != nil {
		return ports.VideoSubmission{}, vendorhttp.Errorf(Code, ports.ErrKindTransient, "tempdir", "%v", err)
	}
	defer os.RemoveAll(work)
	in := filepath.Join(work, "in"+ext)
	out := filepath.Join(work, "out.mp4")
	if err := os.WriteFile(in, req.Image.Data, 0o600); err != nil {
		return ports.VideoSubmission{}, vendorhttp.Errorf(Code, ports.ErrKindTransient, "write_input", "%v", err)
	}
	spec := ports.StillClipSpec{Image: in, Output: out, Format: Format(aspect, res), DurationMs: sec * 1000}
	if err := a.Renderer.StillToClip(ctx, spec); err != nil {
		if ctx.Err() != nil {
			return ports.VideoSubmission{}, vendorhttp.Errorf(Code, ports.ErrKindTransient, "timeout", "render interrupted")
		}
		// A corrupt or unsupported image will not render on retry either.
		return ports.VideoSubmission{}, &ports.ProviderError{Kind: ports.ErrKindInvalidRequest, Provider: Code, Code: "render_failed", Message: "ffmpeg could not render the image", Err: err}
	}
	id := uuid.NewString()
	if err := a.park(ctx, id, out); err != nil {
		return ports.VideoSubmission{}, err
	}
	return ports.VideoSubmission{ProviderJobID: "kb|" + res + "|" + strconv.Itoa(sec) + "|" + id, PollAfter: time.Second}, nil
}

func (a *Adapter) park(ctx context.Context, id, path string) error {
	f, err := os.Open(path) //nolint:gosec // G304: path built inside our own temp dir
	if err != nil {
		return vendorhttp.Errorf(Code, ports.ErrKindUnknown, "output_missing", "render produced no output")
	}
	defer f.Close()
	if s := a.storage(); s != nil {
		st, _ := f.Stat()
		if err := s.Put(ctx, keyPrefix+id+".mp4", f, st.Size(), "video/mp4"); err != nil {
			return vendorhttp.Errorf(Code, ports.ErrKindTransient, "store", "cannot store clip")
		}
		return nil
	}
	dst := a.localPath(id)
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return vendorhttp.Errorf(Code, ports.ErrKindTransient, "store", "%v", err)
	}
	w, err := os.Create(dst) //nolint:gosec // G304: uuid file name inside our own dir
	if err != nil {
		return vendorhttp.Errorf(Code, ports.ErrKindTransient, "store", "%v", err)
	}
	if _, err := io.Copy(w, f); err != nil {
		_ = w.Close()
		return vendorhttp.Errorf(Code, ports.ErrKindTransient, "store", "%v", err)
	}
	return w.Close()
}

func (a *Adapter) open(ctx context.Context, u string) (io.ReadCloser, int64, error) {
	if !safeName.MatchString(u) {
		return nil, 0, errors.New("bad id")
	}
	if s := a.storage(); s != nil {
		info, err := s.Stat(ctx, keyPrefix+u+".mp4")
		if err != nil {
			return nil, 0, err
		}
		rc, err := s.Get(ctx, keyPrefix+u+".mp4", 0, 0)
		return rc, info.Size, err
	}
	f, err := os.Open(a.localPath(u))
	if err != nil {
		return nil, 0, err
	}
	st, _ := f.Stat()
	return f, st.Size(), nil
}

// GetStatus implements ports.VideoGenerator. Usage is priced at 0 but still
// reported so the clip shows up in usage statistics.
func (a *Adapter) GetStatus(ctx context.Context, id string) (ports.VideoStatus, error) {
	res, sec, u, err := parseID(id)
	if err != nil {
		return ports.VideoStatus{}, err
	}
	rc, _, err := a.open(ctx, u)
	if err != nil {
		return ports.VideoStatus{State: ports.VideoFailed, Raw: json.RawMessage(`{"provider":"kenburns"}`), //nolint:nilerr // a missing clip is a failed job state, not a call error
			Error: vendorhttp.Errorf(Code, ports.ErrKindUnknown, "output_missing", "rendered clip is no longer available")}, nil
	}
	_ = rc.Close()
	usage := ports.Usage{VideoSeconds: float64(sec), Units: map[string]float64{"video_second_" + res: float64(sec)}}
	return ports.VideoStatus{State: ports.VideoSucceeded, Progress: 100, DurationSec: float64(sec), Usage: &usage,
		Raw: json.RawMessage(`{"provider":"kenburns"}`)}, nil
}

// Download implements ports.VideoGenerator.
func (a *Adapter) Download(ctx context.Context, id string) (ports.MediaStream, error) {
	_, _, u, err := parseID(id)
	if err != nil {
		return ports.MediaStream{}, err
	}
	rc, size, err := a.open(ctx, u)
	if err != nil {
		return ports.MediaStream{}, vendorhttp.Errorf(Code, ports.ErrKindInvalidRequest, "not_found", "rendered clip is not available")
	}
	return ports.MediaStream{Body: rc, MimeType: "video/mp4", Size: size}, nil
}

// Cancel implements ports.VideoGenerator: rendering is already done, so
// cancel just discards the parked clip.
func (a *Adapter) Cancel(ctx context.Context, id string) error {
	_, _, u, err := parseID(id)
	if err != nil {
		return err
	}
	if s := a.storage(); s != nil {
		_ = s.Delete(ctx, keyPrefix+u+".mp4")
		return nil
	}
	_ = os.Remove(a.localPath(u))
	return nil
}
