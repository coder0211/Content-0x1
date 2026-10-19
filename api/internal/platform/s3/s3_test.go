package s3

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/be/acs/api/internal/platform/config"
	"github.com/be/acs/api/internal/ports"
)

func TestPresignUsesPublicEndpointAndCapsTTL(t *testing.T) {
	s, err := New(config.S3Config{
		Endpoint: "minio:9000", PublicEndpoint: "localhost:9000", Region: "us-east-1",
		Bucket: "acs-assets", AccessKey: "k", SecretKey: "s",
	}, 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	req, err := s.PresignGet(context.Background(), "uploads/x", ports.PresignGetOptions{TTL: time.Hour, DownloadName: "bài viết.png"})
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(req.URL)
	if u.Host != "localhost:9000" {
		t.Fatalf("host = %s", u.Host)
	}
	if got := u.Query().Get("X-Amz-Expires"); got != "900" {
		t.Fatalf("X-Amz-Expires = %s, want 900 (15m cap)", got)
	}
	if !strings.Contains(u.Query().Get("response-content-disposition"), "attachment") {
		t.Fatal("missing content disposition")
	}

	put, err := s.PresignPut(context.Background(), "uploads/y", ports.PresignPutOptions{ContentType: "image/png", ContentLength: 10})
	if err != nil {
		t.Fatal(err)
	}
	pu, _ := url.Parse(put.URL)
	if !strings.Contains(pu.Query().Get("X-Amz-SignedHeaders"), "content-type") {
		t.Fatalf("content-type must be signed: %s", pu.RawQuery)
	}
}
