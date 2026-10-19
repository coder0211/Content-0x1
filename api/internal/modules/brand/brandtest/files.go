package brandtest

import (
	"context"
	"time"

	"github.com/be/acs/api/internal/modules/brand/usecase"
	fileuc "github.com/be/acs/api/internal/modules/file/usecase"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Files is an in-memory usecase.FileReader for brand tests.
type Files struct{ M map[kernel.ID]fileuc.File }

// NewFiles returns an empty fake.
func NewFiles() *Files { return &Files{M: map[kernel.ID]fileuc.File{}} }

var _ usecase.FileReader = (*Files)(nil)

// Add registers a ready file of kind ("image", "audio", …) created by owner.
func (f *Files) Add(kind, mime string, owner kernel.ID) kernel.ID {
	id := kernel.NewID()
	x := fileuc.File{ID: id, MimeType: mime, CreatedBy: &owner, OriginalName: "file", SizeBytes: 10}
	x.Kind = "image"
	switch kind {
	case "audio":
		x.Kind = "audio"
	case "document":
		x.Kind = "document"
	case "video":
		x.Kind = "video"
	}
	x.UploadStatus = "ready"
	f.M[id] = x
	return id
}

// Get implements usecase.FileReader.
func (f *Files) Get(_ context.Context, id kernel.ID) (fileuc.File, error) {
	x, ok := f.M[id]
	if !ok {
		return fileuc.File{}, kernel.ErrNotFound
	}
	return x, nil
}

// GetMany implements usecase.FileReader.
func (f *Files) GetMany(_ context.Context, ids []kernel.ID) ([]fileuc.File, error) {
	var out []fileuc.File
	for _, id := range ids {
		if x, ok := f.M[id]; ok {
			out = append(out, x)
		}
	}
	return out, nil
}

// SignedURL implements usecase.FileReader.
func (f *Files) SignedURL(_ context.Context, id kernel.ID, _ string) (fileuc.SignedURL, error) {
	return fileuc.SignedURL{URL: "https://s3.local/" + id.String(), ExpiresAt: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)}, nil
}
