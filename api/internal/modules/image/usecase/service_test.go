package usecase_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"sync"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/be/acs/api/internal/modules/asset/assettest"
	assetuc "github.com/be/acs/api/internal/modules/asset/usecase"
	"github.com/be/acs/api/internal/modules/brand/brandtest"
	branduc "github.com/be/acs/api/internal/modules/brand/usecase"
	"github.com/be/acs/api/internal/modules/budget/budgettest"
	"github.com/be/acs/api/internal/modules/campaign/campaigntest"
	campaignuc "github.com/be/acs/api/internal/modules/campaign/usecase"
	"github.com/be/acs/api/internal/modules/image/domain"
	"github.com/be/acs/api/internal/modules/image/imagetest"
	"github.com/be/acs/api/internal/modules/image/usecase"
	"github.com/be/acs/api/internal/modules/job/jobtest"
	"github.com/be/acs/api/internal/modules/provider/providertest"
	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
	"github.com/be/acs/api/internal/testutil"
)

// fakeImages is an in-test ports.ImageGenerator/AIProviders returning
// solid PNGs; it records the last request.
type fakeImages struct {
	mu      sync.Mutex
	calls   int
	lastGen ports.ImageRequest
	lastEd  ports.ImageEditRequest
	err     error
}

func solidPNG(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := range w {
		for y := range h {
			img.Set(x, y, color.RGBA{R: 10, G: 80, B: 200, A: 255})
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

func (f *fakeImages) out(n int, quality string) ports.ImageResponse {
	var imgs []ports.GeneratedImage
	for range n {
		imgs = append(imgs, ports.GeneratedImage{MimeType: "image/png", Data: solidPNG(64, 48), Width: 64, Height: 48})
	}
	return ports.ImageResponse{Images: imgs, Usage: ports.Usage{Images: n, Units: map[string]float64{"image_1024_" + quality: float64(n)}}}
}

func (f *fakeImages) Code() string                             { return "fake" }
func (f *fakeImages) Capabilities(string) ports.Capabilities   { return ports.Capabilities{} }
func (f *fakeImages) Text(string) (ports.TextGenerator, error) { return nil, errors.New("no text") }
func (f *fakeImages) Image(string) (ports.ImageGenerator, error) {
	return f, nil
}
func (f *fakeImages) Video(string) (ports.VideoGenerator, error) { return nil, errors.New("no video") }
func (f *fakeImages) Voice(string) (ports.VoiceSynthesizer, error) {
	return nil, errors.New("no voice")
}

func (f *fakeImages) Generate(_ context.Context, req ports.ImageRequest) (ports.ImageResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.lastGen = req
	if f.err != nil {
		return ports.ImageResponse{}, f.err
	}
	return f.out(req.Count, req.Quality), nil
}

func (f *fakeImages) Edit(_ context.Context, req ports.ImageEditRequest) (ports.ImageResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.lastEd = req
	if f.err != nil {
		return ports.ImageResponse{}, f.err
	}
	return f.out(1, req.Quality), nil
}

type env struct {
	svc       *usecase.Service
	jobs      *jobtest.API
	budget    *budgettest.API
	assets    *assettest.API
	brands    *brandtest.API
	campaigns *campaigntest.API
	providers *providertest.API
	files     *imagetest.Files
	repo      *imagetest.Repo
	ai        *fakeImages
	brand     branduc.BrandContext
	campaign  kernel.ID
	creator   kernel.Actor
}

func newEnv(t *testing.T) *env {
	t.Helper()
	e := &env{jobs: jobtest.NewAPI(), budget: budgettest.NewAPI(), assets: assettest.NewAPI(), brands: brandtest.NewAPI(),
		campaigns: campaigntest.NewAPI(), providers: providertest.NewAPI(), files: imagetest.NewFiles(), repo: imagetest.NewRepo(),
		ai: &fakeImages{}, creator: testutil.Actor(kernel.RoleContentCreator)}
	e.budget.Prices["image_1536_standard"] = decimal.RequireFromString("0.06")
	e.brand = e.brands.AddSample()
	e.campaign = kernel.NewID()
	e.campaigns.Campaigns[e.campaign] = campaignuc.CampaignForGeneration{ID: e.campaign, BrandID: e.brand.BrandID,
		Status: campaignuc.StatusInProgress, CreatedBy: e.creator.UserID}
	e.svc = &usecase.Service{Tx: &testutil.Tx{}, Clock: testutil.NewClock(), Repo: e.repo, Jobs: e.jobs, Budget: e.budget,
		Assets: e.assets, Campaigns: e.campaigns, Brands: e.brands, Files: e.files, Providers: e.providers, AI: e.ai}
	return e
}

// run executes the job's single step like the engine would.
func (e *env) run(t *testing.T, jobID kernel.ID) usecase.StepOutput {
	t.Helper()
	sc := stepCtx(e.jobs.Jobs[jobID], e.jobs.Specs[jobID])
	res, err := e.svc.Process(context.Background(), sc)
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	out, ok := res.Output.(usecase.StepOutput)
	if !ok {
		t.Fatalf("output %T", res.Output)
	}
	if ids, _ := res.JobResult["asset_ids"].([]kernel.ID); len(ids) != 1 || ids[0] != out.AssetID {
		t.Fatalf("job result %v", res.JobResult)
	}
	return out
}

func (e *env) generate(t *testing.T, in usecase.GenerateInput) usecase.JobAccepted {
	t.Helper()
	if in.CampaignID == nil && in.BrandID == nil {
		in.CampaignID = &e.campaign
	}
	if in.Prompt == "" {
		in.Prompt = "Poster khóa học tiếng Anh"
	}
	if in.Preset == "" {
		in.Preset = domain.PresetSocialPost
	}
	res, err := e.svc.Generate(context.Background(), e.creator, in)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	return res
}

func content(t *testing.T, v assetuc.VersionView) domain.Content {
	t.Helper()
	var c domain.Content
	if err := json.Unmarshal(v.ContentJSON, &c); err != nil {
		t.Fatal(err)
	}
	return c
}
