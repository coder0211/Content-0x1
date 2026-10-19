package ports

import (
	"context"
	"errors"
	"io"
	"time"
)

// ErrObjectNotFound is returned when a storage key does not exist.
var ErrObjectNotFound = errors.New("object not found")

// PresignPutOptions constrains a browser upload. ContentType and
// ContentLength are signed, so the client cannot upload something else.
type PresignPutOptions struct {
	ContentType   string
	ContentLength int64
	TTL           time.Duration
}

// PresignGetOptions customises a signed download.
type PresignGetOptions struct {
	TTL time.Duration
	// DownloadName, when set, forces Content-Disposition: attachment.
	DownloadName string
}

// PresignedRequest is what the client needs to perform the request.
type PresignedRequest struct {
	URL       string
	Method    string
	Headers   map[string]string
	ExpiresAt time.Time
}

// ObjectInfo is object metadata from storage.
type ObjectInfo struct {
	Key          string
	Size         int64
	ContentType  string
	ETag         string
	LastModified time.Time
}

// ObjectStorage is private S3-compatible storage. Keys never contain user
// supplied names or sensitive data.
type ObjectStorage interface {
	PresignPut(ctx context.Context, key string, opts PresignPutOptions) (PresignedRequest, error)
	PresignGet(ctx context.Context, key string, opts PresignGetOptions) (PresignedRequest, error)
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	// Get opens the object. length <= 0 reads to the end.
	Get(ctx context.Context, key string, offset, length int64) (io.ReadCloser, error)
	Stat(ctx context.Context, key string) (ObjectInfo, error)
	Delete(ctx context.Context, key string) error
}
