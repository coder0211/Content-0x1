// Package imagetest provides in-memory test doubles for the image module:
// a BatchRepository and a file usecase.API (the file module ships no fake).
package imagetest

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"sync"
	"time"

	fileuc "github.com/be/acs/api/internal/modules/file/usecase"
	"github.com/be/acs/api/internal/modules/image/domain"
	"github.com/be/acs/api/internal/modules/image/usecase"
	"github.com/be/acs/api/internal/shared/kernel"
	"github.com/be/acs/api/internal/testutil"
)

// Repo is an in-memory usecase.BatchRepository.
type Repo struct {
	mu      sync.Mutex
	Batches map[kernel.ID]domain.Batch
}

// NewRepo returns an empty repository.
func NewRepo() *Repo { return &Repo{Batches: map[kernel.ID]domain.Batch{}} }

var _ usecase.BatchRepository = (*Repo)(nil)

func (r *Repo) Insert(_ context.Context, b *domain.Batch) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Batches[b.ID] = *b
	return nil
}

func (r *Repo) Get(_ context.Context, id kernel.ID) (*domain.Batch, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.Batches[id]
	if !ok {
		return nil, kernel.ErrNotFound
	}
	return &b, nil
}

func (r *Repo) SetResult(_ context.Context, id, assetID kernel.ID, versionNo int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.Batches[id]
	if !ok {
		return kernel.ErrNotFound
	}
	b.AssetID, b.VersionNo = &assetID, &versionNo
	r.Batches[id] = b
	return nil
}

func (r *Repo) List(_ context.Context, q usecase.BatchQuery) ([]domain.Batch, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []domain.Batch
	for _, b := range r.Batches {
		if q.CampaignID != nil && (b.CampaignID == nil || *b.CampaignID != *q.CampaignID) {
			continue
		}
		if q.AssetID != nil && (b.AssetID == nil || *b.AssetID != *q.AssetID) {
			continue
		}
		if q.CreatedBy != nil && b.CreatedBy != *q.CreatedBy {
			continue
		}
		if q.After != nil && !b.CreatedAt.Before(q.After.At) && (!b.CreatedAt.Equal(q.After.At) || b.ID.String() >= q.After.ID.String()) {
			continue
		}
		out = append(out, b)
	}
	slices.SortFunc(out, func(a, b domain.Batch) int {
		if c := b.CreatedAt.Compare(a.CreatedAt); c != 0 {
			return c
		}
		return -bytes.Compare(a.ID[:], b.ID[:])
	})
	if q.Limit > 0 && len(out) > q.Limit {
		out = out[:q.Limit]
	}
	return out, nil
}

// FileRepo is an in-memory file usecase.Repository so tests can run the
// real file service (fileuc.Service) on testutil.Storage.
type FileRepo struct {
	mu    sync.Mutex
	Items map[kernel.ID]fileuc.File
}

var _ fileuc.Repository = (*FileRepo)(nil)

func (r *FileRepo) Create(_ context.Context, f *fileuc.File) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Items[f.ID] = *f
	return nil
}

func (r *FileRepo) Get(_ context.Context, id kernel.ID) (*fileuc.File, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	f, ok := r.Items[id]
	if !ok {
		return nil, kernel.ErrNotFound
	}
	return &f, nil
}

func (r *FileRepo) GetMany(_ context.Context, ids []kernel.ID) ([]fileuc.File, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []fileuc.File
	for _, id := range ids {
		if f, ok := r.Items[id]; ok {
			out = append(out, f)
		}
	}
	return out, nil
}

func (r *FileRepo) MarkReady(_ context.Context, id kernel.ID, v fileuc.Verified) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	f, ok := r.Items[id]
	if !ok {
		return kernel.ErrNotFound
	}
	f.UploadStatus, f.MimeType, f.Kind, f.SizeBytes, f.Width, f.Height = "ready", v.MimeType, v.Kind, v.SizeBytes, v.Width, v.Height
	r.Items[id] = f
	return nil
}

func (r *FileRepo) MarkRejected(_ context.Context, id kernel.ID, _ string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	f, ok := r.Items[id]
	if !ok {
		return kernel.ErrNotFound
	}
	f.UploadStatus = "rejected"
	r.Items[id] = f
	return nil
}

func (r *FileRepo) ListStalePending(context.Context, time.Time, int) ([]fileuc.StaleFile, error) {
	return nil, nil
}

func (r *FileRepo) Delete(_ context.Context, id kernel.ID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.Items, id)
	return nil
}

// Files bundles the real file service with in-memory storage.
type Files struct {
	*fileuc.Service
	Repo    *FileRepo
	Storage *testutil.Storage
}

// NewFiles returns a file service on in-memory repo + storage.
func NewFiles() *Files {
	repo := &FileRepo{Items: map[kernel.ID]fileuc.File{}}
	st := testutil.NewStorage()
	return &Files{Service: &fileuc.Service{Repo: repo, Storage: st, Clock: testutil.NewClock(), URLTTL: 15 * time.Minute}, Repo: repo, Storage: st}
}

// Upload stores a ready, user-uploaded file (test helper): kind is derived
// from mime by the file service; Source is overwritten to "upload".
func (f *Files) Upload(owner kernel.ID, mime string, data []byte, meta json.RawMessage) fileuc.File {
	file, err := f.Store(context.Background(), fileuc.StoreInput{Body: bytes.NewReader(data), Size: int64(len(data)),
		MimeType: mime, Source: "generated", CreatedBy: &owner, Metadata: meta})
	if err != nil {
		panic(err)
	}
	file.Source = "upload"
	f.Repo.Items[file.ID] = file
	return file
}

// Data returns the stored bytes of a file.
func (f *Files) Data(id kernel.ID) []byte {
	file := f.Repo.Items[id]
	return f.Storage.Objects[file.StorageKey]
}
