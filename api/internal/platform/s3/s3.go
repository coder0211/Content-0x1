// Package s3 implements ports.ObjectStorage on any S3-compatible service
// (MinIO locally, AWS S3 or MinIO in production — OQ-05).
package s3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/be/acs/api/internal/platform/config"
	"github.com/be/acs/api/internal/ports"
)

// Storage implements ports.ObjectStorage.
type Storage struct {
	client *minio.Client // internal endpoint: object I/O
	signer *minio.Client // public endpoint: presigning only (no network I/O)
	bucket string
	maxTTL time.Duration
}

var _ ports.ObjectStorage = (*Storage)(nil)

// New builds a Storage. Region is set explicitly so presigning never needs
// a network round trip.
func New(cfg config.S3Config, maxTTL time.Duration) (*Storage, error) {
	mk := func(endpoint string) (*minio.Client, error) {
		return minio.New(endpoint, &minio.Options{
			Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
			Secure: cfg.UseSSL,
			Region: cfg.Region,
		})
	}
	c, err := mk(cfg.Endpoint)
	if err != nil {
		return nil, err
	}
	signer := c
	if cfg.PublicEndpoint != "" && cfg.PublicEndpoint != cfg.Endpoint {
		if signer, err = mk(cfg.PublicEndpoint); err != nil {
			return nil, err
		}
	}
	return &Storage{client: c, signer: signer, bucket: cfg.Bucket, maxTTL: maxTTL}, nil
}

// EnsureBucket creates the bucket if missing (dev convenience; production
// buckets are provisioned with versioning enabled).
func (s *Storage) EnsureBucket(ctx context.Context, region string) error {
	ok, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	return s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{Region: region})
}

// Ping checks that the bucket is reachable (readiness probe).
func (s *Storage) Ping(ctx context.Context) error {
	ok, err := s.client.BucketExists(ctx, s.bucket)
	if err == nil && !ok {
		err = fmt.Errorf("bucket %q does not exist", s.bucket)
	}
	return err
}

func (s *Storage) ttl(d time.Duration) time.Duration {
	if d <= 0 || d > s.maxTTL {
		return s.maxTTL
	}
	return d
}

// PresignPut signs a PUT bound to content type and length.
func (s *Storage) PresignPut(ctx context.Context, key string, opts ports.PresignPutOptions) (ports.PresignedRequest, error) {
	ttl := s.ttl(opts.TTL)
	headers := http.Header{}
	headers.Set("Content-Type", opts.ContentType)
	if opts.ContentLength > 0 {
		headers.Set("Content-Length", strconv.FormatInt(opts.ContentLength, 10))
	}
	u, err := s.signer.PresignHeader(ctx, http.MethodPut, s.bucket, key, ttl, nil, headers)
	if err != nil {
		return ports.PresignedRequest{}, err
	}
	return ports.PresignedRequest{
		URL:       u.String(),
		Method:    http.MethodPut,
		Headers:   map[string]string{"Content-Type": opts.ContentType},
		ExpiresAt: time.Now().Add(ttl).UTC(),
	}, nil
}

// PresignGet signs a GET, optionally forcing a download file name.
func (s *Storage) PresignGet(ctx context.Context, key string, opts ports.PresignGetOptions) (ports.PresignedRequest, error) {
	ttl := s.ttl(opts.TTL)
	params := url.Values{}
	if opts.DownloadName != "" {
		params.Set("response-content-disposition", "attachment; filename*=UTF-8''"+url.PathEscape(opts.DownloadName))
	}
	u, err := s.signer.PresignedGetObject(ctx, s.bucket, key, ttl, params)
	if err != nil {
		return ports.PresignedRequest{}, err
	}
	return ports.PresignedRequest{URL: u.String(), Method: http.MethodGet, ExpiresAt: time.Now().Add(ttl).UTC()}, nil
}

// Put uploads r (size -1 when unknown).
//
// Over plain HTTP (local S3/SeaweedFS) minio-go would otherwise use the
// aws-chunked streaming signature, and some S3-compatible stores persist
// "Content-Encoding: aws-chunked" on the object and serve it back on GET,
// which browsers cannot decode (<video>/<img> and downloads). Over TLS no
// streaming signature is used, so this only changes the local transport.
func (s *Storage) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	_, err := s.client.PutObject(ctx, s.bucket, key, r, size, minio.PutObjectOptions{ContentType: contentType, DisableContentSha256: true})
	return err
}

// Get opens a byte range of the object (length <= 0: to the end).
func (s *Storage) Get(ctx context.Context, key string, offset, length int64) (io.ReadCloser, error) {
	opts := minio.GetObjectOptions{}
	if offset > 0 || length > 0 {
		end := int64(0)
		if length > 0 {
			end = offset + length - 1
		}
		if err := opts.SetRange(offset, end); err != nil {
			return nil, err
		}
	}
	obj, err := s.client.GetObject(ctx, s.bucket, key, opts)
	if err != nil {
		return nil, mapErr(err)
	}
	// GetObject is lazy; Stat surfaces a missing key now rather than on Read.
	if _, err := obj.Stat(); err != nil {
		_ = obj.Close()
		return nil, mapErr(err)
	}
	return obj, nil
}

// Stat returns object metadata.
func (s *Storage) Stat(ctx context.Context, key string) (ports.ObjectInfo, error) {
	info, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return ports.ObjectInfo{}, mapErr(err)
	}
	return ports.ObjectInfo{Key: key, Size: info.Size, ContentType: info.ContentType, ETag: info.ETag, LastModified: info.LastModified}, nil
}

// Delete removes the object; deleting a missing key is not an error.
func (s *Storage) Delete(ctx context.Context, key string) error {
	return s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
}

func mapErr(err error) error {
	var resp minio.ErrorResponse
	if errors.As(err, &resp) && (resp.Code == "NoSuchKey" || resp.StatusCode == http.StatusNotFound) {
		return ports.ErrObjectNotFound
	}
	return err
}
