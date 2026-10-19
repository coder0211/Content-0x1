package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // register decoders for DecodeConfig
	_ "image/png"
	"io"
	"log/slog"
	"os"
	"time"

	_ "golang.org/x/image/webp"

	"github.com/be/acs/api/internal/modules/file/domain"
	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Service implements the file use cases and API.
type Service struct {
	Repo    Repository
	Storage ports.ObjectStorage
	Prober  ports.MediaProber
	Clock   ports.Clock
	Limits  domain.Limits
	// URLTTL is the signed URL lifetime (≤ 15m).
	URLTTL time.Duration
	// TempDir holds downloads for ffprobe.
	TempDir string
	Logger  *slog.Logger
}

var _ API = (*Service)(nil)

// RequestUploadInput is POST /files/upload-url.
type RequestUploadInput struct {
	Filename  string
	MimeType  string
	SizeBytes int64
	Purpose   domain.Purpose
}

// UploadTicket is returned to the browser.
type UploadTicket struct {
	FileID    kernel.ID
	URL       string
	Method    string
	Headers   map[string]string
	ExpiresAt time.Time
}

func (s *Service) key(prefix string, id kernel.ID) string {
	now := s.Clock.Now()
	return fmt.Sprintf("%s/%04d/%02d/%s", prefix, now.Year(), int(now.Month()), id)
}

// RequestUpload validates the request, creates a pending file and presigns
// a PUT bound to the declared content type and length.
func (s *Service) RequestUpload(ctx context.Context, actor kernel.Actor, in RequestUploadInput) (UploadTicket, error) {
	if !actor.IsAuthenticated() {
		return UploadTicket{}, kernel.ErrUnauthenticated
	}
	if err := domain.ValidateUploadRequest(in.Purpose, in.Filename, in.MimeType, in.SizeBytes, s.Limits); err != nil {
		return UploadTicket{}, err
	}
	id := kernel.NewID()
	meta, _ := json.Marshal(map[string]string{"purpose": string(in.Purpose)})
	f := &domain.File{
		ID: id, StorageKey: s.key("uploads", id), OriginalName: in.Filename, MimeType: in.MimeType,
		SizeBytes: in.SizeBytes, Kind: domain.KindOf(in.MimeType), UploadStatus: domain.StatusPending,
		Source: domain.SourceUpload, License: json.RawMessage(`{}`), Metadata: meta,
		CreatedBy: &actor.UserID, CreatedAt: s.Clock.Now(),
	}
	if err := s.Repo.Create(ctx, f); err != nil {
		return UploadTicket{}, err
	}
	p, err := s.Storage.PresignPut(ctx, f.StorageKey, ports.PresignPutOptions{ContentType: in.MimeType, ContentLength: in.SizeBytes, TTL: s.URLTTL})
	if err != nil {
		return UploadTicket{}, err
	}
	return UploadTicket{FileID: id, URL: p.URL, Method: p.Method, Headers: p.Headers, ExpiresAt: p.ExpiresAt}, nil
}

// ownedPending loads a file the actor uploaded (NOT_FOUND otherwise, to
// avoid leaking existence).
func (s *Service) owned(ctx context.Context, actor kernel.Actor, id kernel.ID) (*domain.File, error) {
	f, err := s.Repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if f.CreatedBy == nil || (*f.CreatedBy != actor.UserID && !actor.IsAdmin()) {
		return nil, kernel.ErrNotFound
	}
	return f, nil
}

// CompleteUpload verifies the uploaded object: presence, size, magic bytes,
// image dimensions or ffprobe streams/duration. A failing file is deleted
// from storage and marked rejected (413/415).
func (s *Service) CompleteUpload(ctx context.Context, actor kernel.Actor, id kernel.ID) (File, error) {
	if !actor.IsAuthenticated() {
		return File{}, kernel.ErrUnauthenticated
	}
	f, err := s.owned(ctx, actor, id)
	if err != nil {
		return File{}, err
	}
	switch f.UploadStatus {
	case domain.StatusReady:
		return *f, nil // idempotent
	case domain.StatusRejected:
		return File{}, kernel.ErrUnsupportedMedia
	}
	info, err := s.Storage.Stat(ctx, f.StorageKey)
	if errors.Is(err, ports.ErrObjectNotFound) {
		return File{}, kernel.Validation(kernel.FieldIssue("file", "not_uploaded"))
	}
	if err != nil {
		return File{}, err
	}
	purpose := domain.Purpose(metaString(f.Metadata, "purpose"))
	if max := domain.MaxBytesFor(purpose, f.MimeType, s.Limits); info.Size > max || info.Size != f.SizeBytes {
		return File{}, s.reject(ctx, f, "size_mismatch_or_too_large", kernel.ErrFileTooLarge.WithDetails(kernel.Detail{"max": max}))
	}
	v, verr := s.verify(ctx, f, info.Size)
	if verr != nil {
		if de, ok := kernel.AsDomainError(verr); ok {
			return File{}, s.reject(ctx, f, de.Code, verr)
		}
		return File{}, verr
	}
	if err := s.Repo.MarkReady(ctx, f.ID, v); err != nil {
		return File{}, err
	}
	return s.Get(ctx, f.ID)
}

func metaString(m json.RawMessage, k string) string {
	var v map[string]any
	_ = json.Unmarshal(m, &v)
	s, _ := v[k].(string)
	return s
}

func (s *Service) reject(ctx context.Context, f *domain.File, reason string, cause error) error {
	if err := s.Storage.Delete(ctx, f.StorageKey); err != nil {
		s.Logger.WarnContext(ctx, "delete rejected upload", "file_id", f.ID, "error", err)
	}
	if err := s.Repo.MarkRejected(ctx, f.ID, reason); err != nil {
		return err
	}
	return cause
}

func (s *Service) verify(ctx context.Context, f *domain.File, size int64) (Verified, error) {
	head, err := s.Storage.Get(ctx, f.StorageKey, 0, domain.SniffLen)
	if err != nil {
		return Verified{}, err
	}
	buf, err := io.ReadAll(head)
	_ = head.Close()
	if err != nil {
		return Verified{}, err
	}
	sniffed := domain.Sniff(buf)
	if sniffed == "" || !domain.SniffMatches(f.MimeType, sniffed) {
		return Verified{}, kernel.ErrUnsupportedMedia.WithDetails(kernel.Detail{"declared": f.MimeType, "detected": sniffed})
	}
	v := Verified{MimeType: f.MimeType, Kind: domain.KindOf(f.MimeType), SizeBytes: size}
	switch v.Kind {
	case domain.KindVideo, domain.KindAudio:
		err = s.probe(ctx, f, &v)
	default:
		err = s.hashAndMeasure(ctx, f, &v)
	}
	return v, err
}

func (s *Service) hashAndMeasure(ctx context.Context, f *domain.File, v *Verified) error {
	r, err := s.Storage.Get(ctx, f.StorageKey, 0, 0)
	if err != nil {
		return err
	}
	defer r.Close()
	h := sha256.New()
	tee := io.TeeReader(r, h)
	if v.Kind == domain.KindImage && v.MimeType != "image/svg+xml" {
		cfg, _, err := image.DecodeConfig(tee)
		if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
			return kernel.ErrUnsupportedMedia.WithDetails(kernel.FieldIssue("file", "corrupt_image"))
		}
		v.Width, v.Height = cfg.Width, cfg.Height
	}
	if _, err := io.Copy(io.Discard, tee); err != nil {
		return err
	}
	v.Checksum = hex.EncodeToString(h.Sum(nil))
	return nil
}

// probe downloads the object to a private temp file (no external URLs:
// SSRF rule) and runs ffprobe on it.
func (s *Service) probe(ctx context.Context, f *domain.File, v *Verified) error {
	r, err := s.Storage.Get(ctx, f.StorageKey, 0, 0)
	if err != nil {
		return err
	}
	defer r.Close()
	tmp, err := os.CreateTemp(s.TempDir, "acs-probe-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, h), r); err != nil {
		return err
	}
	v.Checksum = hex.EncodeToString(h.Sum(nil))
	info, err := s.Prober.Probe(ctx, tmp.Name())
	if err != nil {
		return kernel.ErrUnsupportedMedia.WithDetails(kernel.FieldIssue("file", "unreadable_media")).Wrap(err)
	}
	if v.Kind == domain.KindVideo {
		vs, ok := info.Video()
		if !ok {
			return kernel.ErrUnsupportedMedia.WithDetails(kernel.FieldIssue("file", "no_video_stream"))
		}
		v.Width, v.Height = vs.Width, vs.Height
	} else if _, ok := info.Audio(); !ok {
		return kernel.ErrUnsupportedMedia.WithDetails(kernel.FieldIssue("file", "no_audio_stream"))
	}
	v.DurationMs = int(info.Duration.Milliseconds())
	return nil
}

// Get implements API.
func (s *Service) Get(ctx context.Context, id kernel.ID) (File, error) {
	f, err := s.Repo.Get(ctx, id)
	if err != nil {
		return File{}, err
	}
	return *f, nil
}

// GetForActor returns a file the actor uploaded (GET /files/{id}).
func (s *Service) GetForActor(ctx context.Context, actor kernel.Actor, id kernel.ID) (File, SignedURL, error) {
	f, err := s.owned(ctx, actor, id)
	if err != nil {
		return File{}, SignedURL{}, err
	}
	if f.UploadStatus != domain.StatusReady {
		return *f, SignedURL{}, nil
	}
	u, err := s.SignedURL(ctx, id, "")
	return *f, u, err
}

// GetMany implements API.
func (s *Service) GetMany(ctx context.Context, ids []kernel.ID) ([]File, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	return s.Repo.GetMany(ctx, ids)
}

// RequireReady implements API.
func (s *Service) RequireReady(ctx context.Context, id kernel.ID, kinds ...domain.Kind) (File, error) {
	f, err := s.Repo.Get(ctx, id)
	if err != nil {
		return File{}, err
	}
	if f.UploadStatus != domain.StatusReady {
		return File{}, kernel.ErrNotFound
	}
	if len(kinds) > 0 {
		for _, k := range kinds {
			if f.Kind == k {
				return *f, nil
			}
		}
		return File{}, kernel.ErrUnsupportedMedia.WithDetails(kernel.Detail{"file_id": id.String(), "kind": string(f.Kind)})
	}
	return *f, nil
}

// Store implements API.
func (s *Service) Store(ctx context.Context, in StoreInput) (File, error) {
	if in.Source != domain.SourceGenerated && in.Source != domain.SourceRendered {
		return File{}, kernel.Validation(kernel.FieldIssue("source", "invalid"))
	}
	id := kernel.NewID()
	h := sha256.New()
	cr := &countingReader{r: io.TeeReader(in.Body, h)}
	key := s.key(string(in.Source), id)
	if err := s.Storage.Put(ctx, key, cr, in.Size, in.MimeType); err != nil {
		return File{}, err
	}
	kind := in.Kind
	if kind == "" {
		kind = domain.KindOf(in.MimeType)
	}
	sum := hex.EncodeToString(h.Sum(nil))
	f := &domain.File{
		ID: id, StorageKey: key, OriginalName: in.OriginalName, MimeType: in.MimeType, SizeBytes: cr.n,
		ChecksumSHA256: sum, Width: in.Width, Height: in.Height, DurationMs: in.DurationMs, Kind: kind,
		UploadStatus: domain.StatusReady, Source: in.Source, ParentFileID: in.ParentFileID,
		License: orEmpty(in.License), Metadata: orEmpty(in.Metadata), CreatedBy: in.CreatedBy, CreatedAt: s.Clock.Now(),
	}
	if err := s.Repo.Create(ctx, f); err != nil {
		_ = s.Storage.Delete(context.WithoutCancel(ctx), key)
		return File{}, err
	}
	return *f, nil
}

func orEmpty(j json.RawMessage) json.RawMessage {
	if len(j) == 0 {
		return json.RawMessage(`{}`)
	}
	return j
}

type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

// Open implements API.
func (s *Service) Open(ctx context.Context, id kernel.ID) (io.ReadCloser, File, error) {
	f, err := s.RequireReady(ctx, id)
	if err != nil {
		return nil, File{}, err
	}
	r, err := s.Storage.Get(ctx, f.StorageKey, 0, 0)
	return r, f, err
}

// SignedURL implements API.
func (s *Service) SignedURL(ctx context.Context, id kernel.ID, downloadName string) (SignedURL, error) {
	f, err := s.RequireReady(ctx, id)
	if err != nil {
		return SignedURL{}, err
	}
	p, err := s.Storage.PresignGet(ctx, f.StorageKey, ports.PresignGetOptions{TTL: s.URLTTL, DownloadName: downloadName})
	if err != nil {
		return SignedURL{}, err
	}
	return SignedURL{URL: p.URL, ExpiresAt: p.ExpiresAt}, nil
}

// PurgeStalePending implements API.
func (s *Service) PurgeStalePending(ctx context.Context, olderThan time.Duration) (int, error) {
	stale, err := s.Repo.ListStalePending(ctx, s.Clock.Now().Add(-olderThan), 500)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, f := range stale {
		if err := s.Storage.Delete(ctx, f.StorageKey); err != nil {
			s.Logger.WarnContext(ctx, "purge pending upload", "file_id", f.ID, "error", err)
			continue
		}
		if err := s.Repo.Delete(ctx, f.ID); err != nil {
			// Still referenced (FK) or concurrently completed; keep the row.
			s.Logger.WarnContext(ctx, "purge pending row", "file_id", f.ID, "error", err)
			continue
		}
		n++
	}
	return n, nil
}
