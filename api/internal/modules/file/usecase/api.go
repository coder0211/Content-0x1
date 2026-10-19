// Package usecase implements the file module (ACS-024): browser uploads via
// presigned PUT, server-side verification, generated-file storage and
// signed downloads. api.go is the public contract for other modules.
package usecase

import (
	"context"
	"encoding/json"
	"io"
	"time"

	"github.com/be/acs/api/internal/modules/file/domain"
	"github.com/be/acs/api/internal/shared/kernel"
)

// File is the public view of an asset_files row.
type File = domain.File

// StoreInput stores bytes produced by a worker (provider output, render).
type StoreInput struct {
	Body         io.Reader
	Size         int64 // -1 when unknown
	MimeType     string
	Kind         domain.Kind
	Source       domain.Source // generated | rendered
	ParentFileID *kernel.ID
	CreatedBy    *kernel.ID
	Width        int
	Height       int
	DurationMs   int
	Metadata     json.RawMessage
	License      json.RawMessage
	// OriginalName is a display name only; never part of the storage key.
	OriginalName string
}

// SignedURL is a time-limited download URL (TTL ≤ 15 minutes).
type SignedURL struct {
	URL       string
	ExpiresAt time.Time
}

// API is the file module's public interface.
type API interface {
	// Get returns metadata (NOT_FOUND when missing).
	Get(ctx context.Context, id kernel.ID) (File, error)
	// GetMany returns files by id (missing ids skipped).
	GetMany(ctx context.Context, ids []kernel.ID) ([]File, error)
	// RequireReady returns the file when it is ready and, if kinds are
	// given, of one of those kinds; else NOT_FOUND / UNSUPPORTED_MEDIA_TYPE.
	// Callers still check that the actor may use the file (ownership/brand).
	RequireReady(ctx context.Context, id kernel.ID, kinds ...domain.Kind) (File, error)
	// Store uploads bytes and inserts a ready file row.
	Store(ctx context.Context, in StoreInput) (File, error)
	// Open streams the object (workers load inputs only from storage).
	Open(ctx context.Context, id kernel.ID) (io.ReadCloser, File, error)
	// SignedURL presigns a GET; downloadName forces attachment when set.
	SignedURL(ctx context.Context, id kernel.ID, downloadName string) (SignedURL, error)
	// PurgeStalePending deletes pending uploads older than olderThan
	// (maintenance, OQ-06: 24h). Returns the number of files removed.
	PurgeStalePending(ctx context.Context, olderThan time.Duration) (int, error)
}
