package openai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/providers/recorded"
	"github.com/be/acs/api/internal/shared/kernel"
)

func pngOf(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = 200
	}
	img.Set(0, 0, color.Black)
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

// fixture serves testdata/name with __PNG__ replaced by a w×h PNG.
func fixture(t *testing.T, name string, w, h int) recorded.Reply {
	raw, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return recorded.Reply{Body: []byte(strings.ReplaceAll(string(raw), "__PNG__", base64.StdEncoding.EncodeToString(pngOf(w, h))))}
}

func adapter(srv *recorded.Server) *Adapter {
	a := New()
	a.EP.WithKeyFunc(func(ref string) string {
		if ref == DefaultSecretRef {
			return "sk-openai"
		}
		return ""
	})
	a.EP.SetBaseURL(srv.URL)
	return a
}

func TestGenerateCropsTo9x16(t *testing.T) {
	srv := recorded.New(t, recorded.Route{"POST /v1/images/generations": fixture(t, "generations_ok.json", 64, 96)})
	resp, err := adapter(srv).Generate(context.Background(), ports.ImageRequest{
		Prompt: "Poster khóa học", NegativePrompt: "chữ méo", AspectRatio: "9:16", Quality: "high", Count: 2, Seed: new(int64),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Images) != 2 {
		t.Fatal(len(resp.Images))
	}
	im := resp.Images[0]
	cfg, err := png.DecodeConfig(bytes.NewReader(im.Data))
	if err != nil || cfg.Width != im.Width || cfg.Height != im.Height {
		t.Fatal(err, cfg, im.Width, im.Height)
	}
	if im.Width*16 != im.Height*9 && (im.Width != 54 || im.Height != 96) {
		t.Fatalf("not 9:16: %dx%d", im.Width, im.Height)
	}
	if resp.Usage.Units["image_1024_high"] != 2 || resp.Usage.Images != 2 || resp.Usage.OutputTokens != 3120 {
		t.Fatalf("%+v", resp.Usage)
	}
	var sent genRequest
	_ = json.Unmarshal(srv.Last().Body, &sent)
	if sent.Size != "1024x1536" || sent.Quality != "high" || sent.N != 2 || sent.Model != DefaultModel || !strings.Contains(sent.Prompt, "Avoid: chữ méo") {
		t.Fatalf("%+v", sent)
	}
	if srv.Last().Header.Get("Authorization") != "Bearer sk-openai" {
		t.Fatal("auth header")
	}
	if strings.Contains(string(resp.Raw), "b64") {
		t.Fatal("raw must not contain image data")
	}
}

// ACS-050 AC: unsupported parameter → 422 with the list of valid values.
func TestUnsupportedParamsListValidValues(t *testing.T) {
	srv := recorded.New(t, recorded.Route{})
	a := adapter(srv)
	for _, req := range []ports.ImageRequest{
		{Prompt: "x", AspectRatio: "21:9"},
		{Prompt: "x", Quality: "ultra"},
		{Prompt: "x", Count: 9},
	} {
		_, err := a.Generate(context.Background(), req)
		if !errors.Is(err, kernel.ErrCapability) {
			t.Fatalf("%+v: %v", req, err)
		}
		de, _ := kernel.AsDomainError(err)
		if de.Details[0]["supported"] == nil || de.Details[0]["field"] == nil {
			t.Fatal(de.Details)
		}
	}
	if srv.Count() != 0 {
		t.Fatal("invalid requests must not reach the provider")
	}
}

func TestEditMultipartWithMask(t *testing.T) {
	srv := recorded.New(t, recorded.Route{"POST /v1/images/edits": fixture(t, "generations_ok.json", 32, 32)})
	resp, err := adapter(srv).Edit(context.Background(), ports.ImageEditRequest{
		Prompt: "Đổi nền xanh", Image: ports.MediaInput{MimeType: "image/png", Data: pngOf(8, 8)},
		Mask: &ports.MediaInput{MimeType: "image/png", Data: pngOf(8, 8)}, Count: 1,
	})
	if err != nil || len(resp.Images) != 2 {
		t.Fatal(err)
	}
	last := srv.Last()
	_, params, _ := mime.ParseMediaType(last.Header.Get("Content-Type"))
	mr := multipart.NewReader(bytes.NewReader(last.Body), params["boundary"])
	got := map[string]bool{}
	for {
		p, err := mr.NextPart()
		if err != nil {
			break
		}
		got[p.FormName()] = true
	}
	for _, f := range []string{"model", "prompt", "image", "mask", "size", "quality"} {
		if !got[f] {
			t.Errorf("missing part %s (%v)", f, got)
		}
	}
	_, err = adapter(srv).Edit(context.Background(), ports.ImageEditRequest{Prompt: "x",
		Image: ports.MediaInput{MimeType: "image/png", Data: pngOf(8, 8)}, Mask: &ports.MediaInput{MimeType: "image/jpeg", Data: []byte{1}}})
	if !errors.Is(err, kernel.ErrCapability) {
		t.Fatal("jpeg mask must be rejected")
	}
}

func TestReferenceImagesUseEdits(t *testing.T) {
	srv := recorded.New(t, recorded.Route{"POST /v1/images/edits": fixture(t, "generations_ok.json", 32, 32)})
	_, err := adapter(srv).Generate(context.Background(), ports.ImageRequest{Prompt: "x", ReferenceImages: []ports.MediaInput{
		{MimeType: "image/png", Data: pngOf(4, 4)}, {MimeType: "image/jpeg", Data: []byte("jpg")}}})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(srv.Last().Body, []byte(`name="image[]"`)) {
		t.Fatal("multiple refs must use image[]")
	}
}

func TestErrorMapping(t *testing.T) {
	cases := []struct {
		status int
		body   string
		kind   ports.ErrKind
	}{
		{400, "", ports.ErrKindContentPolicy},
		{429, `{"error":{"message":"Rate limit reached","type":"requests","code":"rate_limit_exceeded"}}`, ports.ErrKindRateLimited},
		{429, `{"error":{"message":"quota","type":"insufficient_quota","code":"insufficient_quota"}}`, ports.ErrKindAuth},
		{401, `{"error":{"message":"bad key","type":"invalid_request_error","code":"invalid_api_key"}}`, ports.ErrKindAuth},
		{500, `{"error":{"message":"oops","type":"server_error"}}`, ports.ErrKindTransient},
		{400, `{"error":{"message":"bad size","type":"invalid_request_error","param":"size"}}`, ports.ErrKindInvalidRequest},
	}
	for _, c := range cases {
		rep := recorded.Reply{Status: c.status, Body: []byte(c.body), Header: map[string]string{"Retry-After": "4"}}
		if c.body == "" {
			rep.File = "error_moderation.json"
		}
		srv := recorded.New(t, recorded.Route{"POST /v1/images/generations": rep})
		_, err := adapter(srv).Generate(context.Background(), ports.ImageRequest{Prompt: "x"})
		pe, ok := ports.AsProviderError(err)
		if !ok || pe.Kind != c.kind {
			t.Errorf("%d %s: %v", c.status, c.body, err)
			continue
		}
		if pe.Kind == ports.ErrKindRateLimited && pe.RetryAfter != 4*time.Second {
			t.Error(pe.RetryAfter)
		}
	}
}

func TestCapabilities(t *testing.T) {
	c := New().Capabilities(DefaultModel)
	if c.MaxVariants != 4 || !c.SupportsEdit || len(c.AspectRatios) != len(AspectRatios) {
		t.Fatal(c)
	}
	_ = http.MethodPost
}
