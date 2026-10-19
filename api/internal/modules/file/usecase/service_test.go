package usecase_test

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"io"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/be/acs/api/internal/modules/file/domain"
	"github.com/be/acs/api/internal/modules/file/usecase"
	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
	"github.com/be/acs/api/internal/testutil"
)

type memRepo struct{ files map[kernel.ID]*domain.File }

func (m *memRepo) Create(_ context.Context, f *domain.File) error {
	c := *f
	m.files[f.ID] = &c
	return nil
}
func (m *memRepo) Get(_ context.Context, id kernel.ID) (*domain.File, error) {
	f, ok := m.files[id]
	if !ok {
		return nil, kernel.ErrNotFound
	}
	c := *f
	return &c, nil
}
func (m *memRepo) GetMany(_ context.Context, ids []kernel.ID) ([]domain.File, error) {
	var out []domain.File
	for _, id := range ids {
		if f, ok := m.files[id]; ok {
			out = append(out, *f)
		}
	}
	return out, nil
}
func (m *memRepo) MarkReady(_ context.Context, id kernel.ID, v usecase.Verified) error {
	f := m.files[id]
	f.UploadStatus, f.MimeType, f.Kind, f.SizeBytes, f.ChecksumSHA256 = domain.StatusReady, v.MimeType, v.Kind, v.SizeBytes, v.Checksum
	f.Width, f.Height, f.DurationMs = v.Width, v.Height, v.DurationMs
	if v.StorageKey != "" {
		f.StorageKey = v.StorageKey
	}
	return nil
}
func (m *memRepo) MarkRejected(_ context.Context, id kernel.ID, _ string) error {
	m.files[id].UploadStatus = domain.StatusRejected
	return nil
}
func (m *memRepo) ListStalePending(_ context.Context, before time.Time, _ int) ([]usecase.StaleFile, error) {
	var out []usecase.StaleFile
	for _, f := range m.files {
		if f.UploadStatus == domain.StatusPending && f.CreatedAt.Before(before) {
			out = append(out, usecase.StaleFile{ID: f.ID, StorageKey: f.StorageKey})
		}
	}
	return out, nil
}
func (m *memRepo) Delete(_ context.Context, id kernel.ID) error { delete(m.files, id); return nil }

type fakeProber struct{ info ports.MediaInfo }

func (p fakeProber) Probe(context.Context, string) (ports.MediaInfo, error) { return p.info, nil }

func newService(prober ports.MediaProber) (*usecase.Service, *memRepo, *testutil.Storage, *testutil.Clock) {
	repo, store, clk := &memRepo{files: map[kernel.ID]*domain.File{}}, testutil.NewStorage(), testutil.NewClock()
	return &usecase.Service{
		Repo: repo, Storage: store, Prober: prober, Clock: clk, URLTTL: 15 * time.Minute,
		Limits: domain.Limits{Image: 20 << 20, Video: 500 << 20, Audio: 50 << 20, Document: 20 << 20},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}, repo, store, clk
}

func pngBytes(t *testing.T, w, h int) []byte {
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func upload(t *testing.T, s *usecase.Service, store *testutil.Storage, actor kernel.Actor, mime string, purpose domain.Purpose, body []byte) usecase.UploadTicket {
	t.Helper()
	tk, err := s.RequestUpload(context.Background(), actor, usecase.RequestUploadInput{Filename: "x", MimeType: mime, SizeBytes: int64(len(body)), Purpose: purpose})
	if err != nil {
		t.Fatal(err)
	}
	f, _ := s.Get(context.Background(), tk.FileID)
	_ = store.Put(context.Background(), f.StorageKey, bytes.NewReader(body), int64(len(body)), mime)
	return tk
}

func TestCompletePNG(t *testing.T) {
	s, _, store, _ := newService(nil)
	actor := testutil.Actor(kernel.RoleContentCreator)
	tk := upload(t, s, store, actor, "image/png", domain.PurposeReferenceImage, pngBytes(t, 64, 32))
	if !strings.Contains(tk.URL, "uploads/2026/10/") || tk.Headers["Content-Type"] != "image/png" {
		t.Fatalf("%+v", tk)
	}
	f, err := s.CompleteUpload(context.Background(), actor, tk.FileID)
	if err != nil {
		t.Fatal(err)
	}
	if f.UploadStatus != domain.StatusReady || f.Width != 64 || f.Height != 32 || f.ChecksumSHA256 == "" {
		t.Fatalf("%+v", f)
	}
	u, err := s.SignedURL(context.Background(), f.ID, "")
	if err != nil || u.URL == "" {
		t.Fatal(err)
	}
}

// ACS-024 AC: an .exe renamed to .png → 415 and the object is deleted.
// ACS-101: bytes PUT again to the still-valid presigned URL after
// /complete never reach readers: the verified copy lives under a new key
// and the upload key is deleted.
func TestCompletePinsVerifiedBytes(t *testing.T) {
	s, _, store, _ := newService(nil)
	actor := testutil.Actor(kernel.RoleContentCreator)
	good := pngBytes(t, 8, 8)
	tk := upload(t, s, store, actor, "image/png", domain.PurposeReferenceImage, good)
	pending, _ := s.Get(context.Background(), tk.FileID)
	f, err := s.CompleteUpload(context.Background(), actor, tk.FileID)
	if err != nil {
		t.Fatal(err)
	}
	if f.StorageKey == pending.StorageKey || !strings.HasPrefix(f.StorageKey, "files/") {
		t.Fatalf("verified bytes must move to a final key: %s", f.StorageKey)
	}
	if _, ok := store.Objects[pending.StorageKey]; ok {
		t.Fatal("upload key must be deleted after verification")
	}
	// Attacker re-PUTs different bytes of the same length to the upload URL.
	evil := bytes.Repeat([]byte{'A'}, len(good))
	_ = store.Put(context.Background(), pending.StorageKey, bytes.NewReader(evil), int64(len(evil)), "image/png")
	rc, _, err := s.Open(context.Background(), f.ID)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(rc)
	_ = rc.Close()
	if !bytes.Equal(got, good) {
		t.Fatal("reader got bytes that were never verified")
	}
}

// ACS-101: decompression bomb (huge declared canvas) → 413, object deleted.
func TestCompleteRejectsOversizedImageDimensions(t *testing.T) {
	s, repo, store, _ := newService(nil)
	actor := testutil.Actor(kernel.RoleContentCreator)
	tk := upload(t, s, store, actor, "image/png", domain.PurposeReferenceImage, pngBytes(t, usecase.MaxImageSide+1, 2))
	if _, err := s.CompleteUpload(context.Background(), actor, tk.FileID); !errors.Is(err, kernel.ErrFileTooLarge) {
		t.Fatalf("got %v", err)
	}
	if repo.files[tk.FileID].UploadStatus != domain.StatusRejected {
		t.Fatal("must be rejected")
	}
}

// ACS-101: SVG is always signed as an attachment (no inline script on the
// storage origin), other images stay inline.
func TestSignedURLForcesAttachmentForSVG(t *testing.T) {
	s, _, store, _ := newService(nil)
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)
	f, err := s.Store(context.Background(), usecase.StoreInput{Body: bytes.NewReader(svg), Size: int64(len(svg)), MimeType: "image/svg+xml",
		Kind: domain.KindImage, Source: domain.SourceGenerated})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SignedURL(context.Background(), f.ID, ""); err != nil {
		t.Fatal(err)
	}
	if got := store.LastGet.DownloadName; got != f.ID.String()+".svg" {
		t.Fatalf("svg download name %q", got)
	}
}

func TestCompleteRejectsDisguisedExecutable(t *testing.T) {
	s, repo, store, _ := newService(nil)
	actor := testutil.Actor(kernel.RoleContentCreator)
	exe := append([]byte("MZ\x90\x00\x03\x00\x00\x00"), make([]byte, 100)...)
	tk := upload(t, s, store, actor, "image/png", domain.PurposeReferenceImage, exe)
	_, err := s.CompleteUpload(context.Background(), actor, tk.FileID)
	if !errors.Is(err, kernel.ErrUnsupportedMedia) {
		t.Fatalf("got %v", err)
	}
	key := repo.files[tk.FileID].StorageKey
	if _, ok := store.Objects[key]; ok || !slices.Contains(store.Deleted, key) {
		t.Fatal("object must be deleted")
	}
	if repo.files[tk.FileID].UploadStatus != domain.StatusRejected {
		t.Fatal("must be rejected")
	}
}

func TestCompleteOnlyByOwner(t *testing.T) {
	s, _, store, _ := newService(nil)
	owner := testutil.Actor(kernel.RoleContentCreator)
	tk := upload(t, s, store, owner, "image/png", domain.PurposeReferenceImage, pngBytes(t, 1, 1))
	if _, err := s.CompleteUpload(context.Background(), testutil.Actor(kernel.RoleContentCreator), tk.FileID); !errors.Is(err, kernel.ErrNotFound) {
		t.Fatalf("other user: %v", err)
	}
}

func TestCompleteVideoUsesProbe(t *testing.T) {
	s, _, store, _ := newService(fakeProber{info: ports.MediaInfo{Duration: 4 * time.Second,
		Streams: []ports.StreamInfo{{Type: "video", Width: 1080, Height: 1920}}}})
	actor := testutil.Actor(kernel.RoleContentCreator)
	mp4 := append([]byte("\x00\x00\x00\x18ftypisom"), make([]byte, 64)...)
	tk := upload(t, s, store, actor, "video/mp4", domain.PurposeVideoSource, mp4)
	f, err := s.CompleteUpload(context.Background(), actor, tk.FileID)
	if err != nil || f.DurationMs != 4000 || f.Height != 1920 {
		t.Fatalf("%v %+v", err, f)
	}
}

func TestCompleteAudioWithoutAudioStreamRejected(t *testing.T) {
	s, _, store, _ := newService(fakeProber{info: ports.MediaInfo{Streams: []ports.StreamInfo{{Type: "video"}}}})
	actor := testutil.Actor(kernel.RoleMarketingLead)
	tk := upload(t, s, store, actor, "audio/mpeg", domain.PurposeMusic, append([]byte("ID3\x03"), make([]byte, 64)...))
	if _, err := s.CompleteUpload(context.Background(), actor, tk.FileID); !errors.Is(err, kernel.ErrUnsupportedMedia) {
		t.Fatalf("got %v", err)
	}
}

func TestStoreAndPurge(t *testing.T) {
	s, repo, store, clk := newService(nil)
	f, err := s.Store(context.Background(), usecase.StoreInput{Body: bytes.NewReader([]byte("png-bytes")), Size: 9, MimeType: "image/png", Source: domain.SourceGenerated})
	if err != nil || f.SizeBytes != 9 || f.UploadStatus != domain.StatusReady || !strings.HasPrefix(f.StorageKey, "generated/") {
		t.Fatalf("%v %+v", err, f)
	}
	actor := testutil.Actor(kernel.RoleContentCreator)
	_, _ = s.RequestUpload(context.Background(), actor, usecase.RequestUploadInput{Filename: "a.png", MimeType: "image/png", SizeBytes: 10, Purpose: domain.PurposeReferenceImage})
	clk.Advance(25 * time.Hour)
	n, err := s.PurgeStalePending(context.Background(), 24*time.Hour)
	if err != nil || n != 1 || len(repo.files) != 1 {
		t.Fatalf("%v %d %d", err, n, len(repo.files))
	}
	_ = store
}
