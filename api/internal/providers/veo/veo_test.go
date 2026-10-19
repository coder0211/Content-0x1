package veo

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/providers/recorded"
	"github.com/be/acs/api/internal/shared/kernel"
)

const opPath = "/v1beta/models/veo-3.1-generate-preview/operations/op123abc"

func adapter(srv *recorded.Server) *Adapter {
	a := New()
	a.EP.WithKeyFunc(func(ref string) string {
		if ref == DefaultSecretRef {
			return "gem-key"
		}
		return ""
	})
	a.EP.SetBaseURL(srv.URL)
	return a
}

func withBase(t *testing.T, srvURL *string, file string) recorded.Reply {
	raw, err := os.ReadFile("testdata/" + file)
	if err != nil {
		t.Fatal(err)
	}
	return recorded.Reply{Handler: func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.ReplaceAll(string(raw), "__BASE__", *srvURL)))
	}}
}

func TestSubmitPollDownload(t *testing.T) {
	var base string
	polls := 0
	done := withBase(t, &base, "operation_done.json")
	srv := recorded.New(t, recorded.Route{
		"POST /v1beta/models/veo-3.1-generate-preview:predictLongRunning": {Body: []byte(`{"name":"models/veo-3.1-generate-preview/operations/op123abc"}`)},
		"GET " + opPath: {Handler: func(w http.ResponseWriter, r *http.Request) {
			polls++
			if polls == 1 {
				_, _ = w.Write([]byte(`{"name":"models/veo-3.1-generate-preview/operations/op123abc","done":false}`))
				return
			}
			done.Handler(w, r)
		}},
		"GET /v1beta/files/vid42:download": {Handler: func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("x-goog-api-key") != "gem-key" {
				w.WriteHeader(403)
				return
			}
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte("\x00\x00\x00\x18ftypmp42clip"))
		}},
	})
	base = srv.URL
	a := adapter(srv)
	sub, err := a.Submit(context.Background(), ports.VideoRequest{
		Model: DefaultModel, Prompt: "Cảnh quán cà phê buổi sáng", AspectRatio: "9:16", Resolution: "1080p", DurationSec: 5,
		Image: &ports.MediaInput{MimeType: "image/png", Data: []byte("png")},
	})
	if err != nil {
		t.Fatal(err)
	}
	var sent submitRequest
	_ = json.Unmarshal(srv.Last().Body, &sent)
	if sent.Parameters.DurationSeconds != 6 || sent.Parameters.AspectRatio != "9:16" || sent.Instances[0].Image == nil || sent.Parameters.SampleCount != 1 {
		t.Fatalf("%+v", sent)
	}
	if srv.Last().Header.Get("x-goog-api-key") != "gem-key" || strings.Contains(srv.Last().Query, "key=") {
		t.Fatal("key must go in header")
	}
	st, err := a.GetStatus(context.Background(), sub.ProviderJobID)
	if err != nil || st.State != ports.VideoRunning {
		t.Fatal(err, st.State)
	}
	st, err = a.GetStatus(context.Background(), sub.ProviderJobID)
	if err != nil || st.State != ports.VideoSucceeded || st.Usage.Units["video_second_1080p"] != 6 {
		t.Fatalf("%v %+v", err, st)
	}
	ms, err := a.Download(context.Background(), sub.ProviderJobID)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(ms.Body)
	_ = ms.Body.Close()
	if !strings.Contains(string(b), "ftyp") || ms.MimeType != "video/mp4" {
		t.Fatal(string(b))
	}
	if !errors.Is(a.Cancel(context.Background(), sub.ProviderJobID), ports.ErrCancelUnsupported) {
		t.Fatal("cancel")
	}
}

// ACS-061 AC: provider fails for content policy → ContentPolicy, not retried.
func TestContentPolicyFailure(t *testing.T) {
	srv := recorded.New(t, recorded.Route{"GET " + opPath: {File: "operation_rai_filtered.json"}})
	st, err := adapter(srv).GetStatus(context.Background(), jobID("720p", 8, "models/veo-3.1-generate-preview/operations/op123abc"))
	if err != nil {
		t.Fatal(err)
	}
	if st.State != ports.VideoFailed || st.Error == nil || st.Error.Kind != ports.ErrKindContentPolicy || st.Error.Retryable() {
		t.Fatalf("%+v", st)
	}
	srv2 := recorded.New(t, recorded.Route{"GET " + opPath: {Body: []byte(`{"name":"x","done":true,"error":{"code":3,"message":"Video generation failed due to safety policy","status":"INVALID_ARGUMENT"}}`)}})
	st, _ = adapter(srv2).GetStatus(context.Background(), jobID("720p", 8, "models/veo-3.1-generate-preview/operations/op123abc"))
	if st.Error == nil || st.Error.Kind != ports.ErrKindContentPolicy {
		t.Fatalf("%+v", st)
	}
	srv3 := recorded.New(t, recorded.Route{"POST /v1beta/models/veo-3.1-generate-preview:predictLongRunning": {Status: 400,
		Body: []byte(`{"error":{"code":400,"message":"The prompt contains sensitive words that violate Responsible AI practices","status":"INVALID_ARGUMENT"}}`)}})
	_, err = adapter(srv3).Submit(context.Background(), ports.VideoRequest{Prompt: "x", AspectRatio: "16:9", DurationSec: 4})
	if pe, ok := ports.AsProviderError(err); !ok || pe.Kind != ports.ErrKindContentPolicy {
		t.Fatal(err)
	}
}

func TestErrorMapping(t *testing.T) {
	srv := recorded.New(t, recorded.Route{"POST /v1beta/models/veo-3.1-generate-preview:predictLongRunning": {Status: 429,
		Body: []byte(`{"error":{"code":429,"message":"Quota exceeded","status":"RESOURCE_EXHAUSTED"}}`)}})
	_, err := adapter(srv).Submit(context.Background(), ports.VideoRequest{Prompt: "x", AspectRatio: "16:9", DurationSec: 8})
	pe, ok := ports.AsProviderError(err)
	if !ok || pe.Kind != ports.ErrKindRateLimited || pe.RetryAfter != time.Minute {
		t.Fatal(err)
	}
	for code, kind := range map[int]ports.ErrKind{14: ports.ErrKindTransient, 8: ports.ErrKindRateLimited, 3: ports.ErrKindInvalidRequest, 7: ports.ErrKindAuth} {
		if got := opError(code, "", "boom").Kind; got != kind {
			t.Errorf("%d: %s", code, got)
		}
	}
}

func TestValidationAndSSRF(t *testing.T) {
	srv := recorded.New(t, recorded.Route{"GET " + opPath: {Body: []byte(`{"name":"x","done":true,"response":{"generateVideoResponse":{"generatedSamples":[{"video":{"uri":"https://evil.example/x.mp4"}}]}}}`)}})
	a := adapter(srv)
	for _, req := range []ports.VideoRequest{
		{Prompt: "x", AspectRatio: "1:1"}, {Prompt: "x", Resolution: "4k"}, {Prompt: "x", DurationSec: 12},
	} {
		if _, err := a.Submit(context.Background(), req); !errors.Is(err, kernel.ErrCapability) {
			t.Errorf("%+v: %v", req, err)
		}
	}
	if _, err := a.GetStatus(context.Background(), "veo|720p|8|../../etc"); err == nil {
		t.Fatal("malformed id")
	}
	_, err := a.Download(context.Background(), jobID("720p", 8, "models/veo-3.1-generate-preview/operations/op123abc"))
	if pe, ok := ports.AsProviderError(err); !ok || pe.Code != "unexpected_uri" {
		t.Fatal(err)
	}
}
