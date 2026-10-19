package gcptts

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/providers/recorded"
	"github.com/be/acs/api/internal/shared/kernel"
)

func newAdapter(srvURL, secret string) *Adapter {
	a := New()
	a.EP.SetBaseURL(srvURL).WithKeyFunc(func(string) string { return secret })
	a.TokenEndpoint = srvURL + "/token"
	return a
}

func TestSynthesizeRecorded(t *testing.T) {
	srv := recorded.New(t, recorded.Route{"POST /v1beta1/text:synthesize": {File: "synthesize_ok.json"}})
	a := newAdapter(srv.URL, "api-key-123")
	r, err := a.Synthesize(context.Background(), ports.VoiceRequest{Text: "Xin chào <bạn>", Language: "vi-VN", SpeakingRate: 1.1})
	if err != nil {
		t.Fatal(err)
	}
	req := srv.Last()
	if req.Header.Get("x-goog-api-key") != "api-key-123" || req.Query != "" {
		t.Fatal("api key must go in the header, never the URL")
	}
	var body synthRequest
	if err := json.Unmarshal(req.Body, &body); err != nil {
		t.Fatal(err)
	}
	if body.Voice.Name != "vi-VN-Neural2-A" || body.AudioConfig.AudioEncoding != "MP3" || body.EnableTimePointing[0] != "SSML_MARK" {
		t.Fatalf("%+v", body)
	}
	if !strings.Contains(body.Input.SSML, `<mark name="w2"/>&lt;bạn&gt;`) {
		t.Fatalf("ssml must be escaped and marked: %s", body.Input.SSML)
	}
	if r.MimeType != "audio/mpeg" || r.Usage.Units["char"] != 14 || r.Usage.Characters != 14 {
		t.Fatalf("%+v", r.Usage)
	}
	if r.DurationMs < 1100 || r.DurationMs > 1300 {
		t.Fatalf("mp3 duration %d", r.DurationMs)
	}
	// 3 words + end mark: w0=50ms, w1=420ms, w2=710ms, end=1150ms.
	if len(r.Timings) != 3 || r.Timings[0].StartMs != 50 || r.Timings[0].EndMs != 420 || r.Timings[2].StartMs != 710 || r.Timings[2].EndMs != 1150 {
		t.Fatalf("%+v", r.Timings)
	}
}

func TestSynthesizeErrors(t *testing.T) {
	srv := recorded.New(t, recorded.Route{"POST /v1beta1/text:synthesize": {Status: 429, File: "error_quota.json"}})
	a := newAdapter(srv.URL, "k")
	_, err := a.Synthesize(context.Background(), ports.VoiceRequest{Text: "xin chào"})
	pe, ok := ports.AsProviderError(err)
	if !ok || pe.Kind != ports.ErrKindRateLimited || pe.RetryAfter <= 0 || !pe.Retryable() {
		t.Fatal(err)
	}
	for _, req := range []ports.VoiceRequest{
		{Text: "x", VoiceID: "en-US-Neural2-A"},
		{Text: "x", Language: "en-US"},
		{Text: "x", Format: "ogg"},
		{Text: "x", Model: "chirp"},
		{Text: "x", SpeakingRate: 9},
		{Text: strings.Repeat("chữ ", 2000)},
	} {
		if _, err := a.Synthesize(context.Background(), req); !errors.Is(err, kernel.ErrCapability) {
			t.Errorf("%+v: %v", req.VoiceID+req.Language+req.Format+req.Model, err)
		}
	}
	missing := newAdapter(srv.URL, "")
	_, err = missing.Synthesize(context.Background(), ports.VoiceRequest{Text: "x"})
	if pe, ok := ports.AsProviderError(err); !ok || pe.Kind != ports.ErrKindAuth {
		t.Fatal(err)
	}
}

func TestServiceAccountTokenIsCached(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	der, _ := x509.MarshalPKCS8PrivateKey(key)
	pemKey := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	sa, _ := json.Marshal(map[string]string{"type": "service_account", "client_email": "tts@p.iam.gserviceaccount.com", "private_key": pemKey})
	tokens := 0
	srv := recorded.New(t, recorded.Route{
		"POST /token": {Handler: func(w http.ResponseWriter, r *http.Request) {
			_ = r.ParseForm()
			parts := strings.Split(r.Form.Get("assertion"), ".")
			if len(parts) != 3 || r.Form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
				w.WriteHeader(400)
				return
			}
			tokens++
			_, _ = w.Write([]byte(`{"access_token":"ya29.tok","expires_in":3600}`))
		}},
		"POST /v1beta1/text:synthesize": {File: "synthesize_ok.json"},
	})
	a := newAdapter(srv.URL, string(sa))
	for i := 0; i < 2; i++ {
		if _, err := a.Synthesize(context.Background(), ports.VoiceRequest{Text: "xin chào"}); err != nil {
			t.Fatal(err)
		}
	}
	if tokens != 1 || srv.Last().Header.Get("Authorization") != "Bearer ya29.tok" {
		t.Fatalf("tokens=%d", tokens)
	}
	a.Now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	if _, err := a.Synthesize(context.Background(), ports.VoiceRequest{Text: "xin chào"}); err != nil || tokens != 2 {
		t.Fatalf("expired token must be refreshed: %v %d", err, tokens)
	}
}

func TestWAVDuration(t *testing.T) {
	// 24 kHz mono 16-bit: 48000 bytes/s; 24000 bytes → 500 ms.
	h := []byte("RIFF\x00\x00\x00\x00WAVEfmt \x10\x00\x00\x00\x01\x00\x01\x00\xc0\x5d\x00\x00\x80\xbb\x00\x00\x02\x00\x10\x00data\xc0\x5d\x00\x00")
	b := append(h, make([]byte, 24000)...)
	if d := WAVDurationMs(b); d != 500 {
		t.Fatal(d)
	}
	if WAVDurationMs([]byte("nope")) != 0 || MP3DurationMs([]byte("nope")) != 0 {
		t.Fatal("garbage")
	}
}
