package testutil

import (
	"bytes"
	"context"
	"io"
	"sync"
	"time"

	"github.com/be/acs/api/internal/ports"
)

// Storage is an in-memory ports.ObjectStorage.
type Storage struct {
	mu      sync.Mutex
	Objects map[string][]byte
	Types   map[string]string
	Deleted []string
	// LastGet is the options of the last PresignGet call.
	LastGet ports.PresignGetOptions
}

// NewStorage returns an empty store.
func NewStorage() *Storage {
	return &Storage{Objects: map[string][]byte{}, Types: map[string]string{}}
}

var _ ports.ObjectStorage = (*Storage)(nil)

func (s *Storage) PresignPut(_ context.Context, key string, o ports.PresignPutOptions) (ports.PresignedRequest, error) {
	return ports.PresignedRequest{URL: "https://s3.test/" + key + "?X-Amz-Signature=x", Method: "PUT",
		Headers: map[string]string{"Content-Type": o.ContentType}, ExpiresAt: time.Now().Add(15 * time.Minute)}, nil
}

func (s *Storage) PresignGet(_ context.Context, key string, o ports.PresignGetOptions) (ports.PresignedRequest, error) {
	s.mu.Lock()
	s.LastGet = o
	s.mu.Unlock()
	return ports.PresignedRequest{URL: "https://s3.test/" + key + "?X-Amz-Signature=x", Method: "GET", ExpiresAt: time.Now().Add(15 * time.Minute)}, nil
}

func (s *Storage) Put(_ context.Context, key string, r io.Reader, _ int64, ct string) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Objects[key], s.Types[key] = b, ct
	return nil
}

func (s *Storage) Get(_ context.Context, key string, off, n int64) (io.ReadCloser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.Objects[key]
	if !ok {
		return nil, ports.ErrObjectNotFound
	}
	if off > int64(len(b)) {
		off = int64(len(b))
	}
	b = b[off:]
	if n > 0 && n < int64(len(b)) {
		b = b[:n]
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

func (s *Storage) Stat(_ context.Context, key string) (ports.ObjectInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.Objects[key]
	if !ok {
		return ports.ObjectInfo{}, ports.ErrObjectNotFound
	}
	return ports.ObjectInfo{Key: key, Size: int64(len(b)), ContentType: s.Types[key]}, nil
}

func (s *Storage) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.Objects, key)
	s.Deleted = append(s.Deleted, key)
	return nil
}
