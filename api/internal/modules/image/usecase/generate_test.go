package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"

	assetuc "github.com/be/acs/api/internal/modules/asset/usecase"
	branduc "github.com/be/acs/api/internal/modules/brand/usecase"
	campaignuc "github.com/be/acs/api/internal/modules/campaign/usecase"
	"github.com/be/acs/api/internal/modules/image/domain"
	"github.com/be/acs/api/internal/modules/image/usecase"
	"github.com/be/acs/api/internal/modules/job/jobtest"
	jobuc "github.com/be/acs/api/internal/modules/job/usecase"
	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
	"github.com/be/acs/api/internal/testutil"
)

func TestGenerateCreatesJobReservationAndBatch(t *testing.T) {
	e := newEnv(t)
	res := e.generate(t, usecase.GenerateInput{AspectRatio: "1:1", Count: 2, IdempotencyKey: "k1", RequestHash: "h1"})
	if res.Status != "queued" || !res.EstimatedCost.Amount.Equal(decimal.RequireFromString("0.08")) {
		t.Fatalf("accepted %+v", res)
	}
	spec := e.jobs.Specs[res.JobID]
	if spec.Type != jobuc.TypeImageGeneration || spec.Steps[0].Type != usecase.StepTypeImage || spec.CampaignID == nil {
		t.Fatalf("spec %+v", spec)
	}
	if _, ok := e.budget.Reserved[res.JobID]; !ok {
		t.Fatal("budget not reserved")
	}
	if len(e.repo.Batches) != 1 {
		t.Fatalf("batches %d", len(e.repo.Batches))
	}
	// Idempotent replay: same job, no second reservation or batch.
	again := e.generate(t, usecase.GenerateInput{AspectRatio: "1:1", Count: 2, IdempotencyKey: "k1", RequestHash: "h1"})
	if again.JobID != res.JobID || len(e.budget.Reserved) != 1 || len(e.repo.Batches) != 1 {
		t.Fatalf("replay created duplicates: %+v", again)
	}
}

func TestGenerateNonSquareFallsBackToBasePricing(t *testing.T) {
	e := newEnv(t)
	res := e.generate(t, usecase.GenerateInput{AspectRatio: "4:5", Quality: "high", Count: 1})
	// image_1536_high is not priced → falls back to image_1024_high ($0.17).
	if !res.EstimatedCost.Amount.Equal(decimal.RequireFromString("0.17")) {
		t.Fatalf("estimate %s", res.EstimatedCost)
	}
}

func TestGenerateValidation(t *testing.T) {
	e := newEnv(t)
	for name, in := range map[string]usecase.GenerateInput{
		"empty prompt": {CampaignID: &e.campaign, Prompt: "  ", Preset: domain.PresetPoster},
		"bad preset":   {CampaignID: &e.campaign, Prompt: "x", Preset: "banner"},
		"count 5":      {CampaignID: &e.campaign, Prompt: "x", Preset: domain.PresetPoster, Count: 5},
		"no target":    {Prompt: "x", Preset: domain.PresetPoster},
		"ratio 3:2":    {CampaignID: &e.campaign, Prompt: "x", Preset: domain.PresetPoster, AspectRatio: "3:2"},
	} {
		_, err := e.svc.Generate(context.Background(), e.creator, in)
		if !errors.Is(err, kernel.ErrValidation) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestGenerateRBAC(t *testing.T) {
	e := newEnv(t)
	for _, role := range []string{kernel.RoleReviewer} {
		_, err := e.svc.Generate(context.Background(), testutil.Actor(role), usecase.GenerateInput{CampaignID: &e.campaign, Prompt: "x", Preset: domain.PresetPoster})
		if !errors.Is(err, kernel.ErrForbidden) {
			t.Errorf("%s: %v", role, err)
		}
	}
}

func TestGenerateUnsupportedOptionIs422WithSupportedValues(t *testing.T) {
	e := newEnv(t)
	e.providers.Models[1].Capabilities.AspectRatios = []string{"1:1", "16:9", "9:16"}
	e.providers.Models[1].Capabilities.MaxVariants = 2
	_, err := e.svc.Generate(context.Background(), e.creator, usecase.GenerateInput{CampaignID: &e.campaign, Prompt: "x",
		Preset: domain.PresetPoster, AspectRatio: "4:5", Count: 3})
	de, ok := kernel.AsDomainError(err)
	if !ok || de.Code != kernel.CodeCapabilityNotSupported || len(de.Details) != 2 {
		t.Fatalf("err %v", err)
	}
	if de.Details[0]["field"] != "aspect_ratio" {
		t.Fatalf("details %v", de.Details)
	}
	if len(e.jobs.Jobs) != 0 {
		t.Fatal("job created despite 422")
	}
}

func TestGenerateReferenceAccess(t *testing.T) {
	e := newEnv(t)
	png := solidPNG(8, 8)
	own := e.files.Upload(e.creator.UserID, "image/png", png, nil)
	if _, err := e.svc.Generate(context.Background(), e.creator, usecase.GenerateInput{CampaignID: &e.campaign, Prompt: "x",
		Preset: domain.PresetProductPhoto, ReferenceFileIDs: []kernel.ID{own.ID}}); err != nil {
		t.Fatalf("own reference: %v", err)
	}

	// A file generated in another campaign the creator is not a member of → 404.
	other := kernel.NewID()
	e.campaigns.Campaigns[other] = campaignuc.CampaignForGeneration{ID: other, BrandID: e.brand.BrandID, Status: campaignuc.StatusInProgress, CreatedBy: kernel.NewID()}
	foreign := e.files.Upload(kernel.NewID(), "image/png", png, []byte(`{"campaign_id":"`+other.String()+`"}`))
	_, err := e.svc.Generate(context.Background(), e.creator, usecase.GenerateInput{CampaignID: &e.campaign, Prompt: "x",
		Preset: domain.PresetProductPhoto, ReferenceFileIDs: []kernel.ID{foreign.ID}})
	if !errors.Is(err, kernel.ErrNotFound) {
		t.Fatalf("foreign reference: %v", err)
	}
	// Becoming a member grants access.
	c := e.campaigns.Campaigns[other]
	c.MemberIDs = []kernel.ID{e.creator.UserID}
	e.campaigns.Campaigns[other] = c
	if _, err := e.svc.Generate(context.Background(), e.creator, usecase.GenerateInput{CampaignID: &e.campaign, Prompt: "x",
		Preset: domain.PresetProductPhoto, ReferenceFileIDs: []kernel.ID{foreign.ID}}); err != nil {
		t.Fatalf("member reference: %v", err)
	}

	// Brand assets of the target brand are usable by everyone.
	logo := e.files.Upload(kernel.NewID(), "image/png", png, nil)
	e.brands.Assets[kernel.NewID()] = branduc.BrandAsset{ID: kernel.NewID(), BrandID: e.brand.BrandID, FileID: logo.ID, AssetType: "logo"}
	if _, err := e.svc.Generate(context.Background(), e.creator, usecase.GenerateInput{CampaignID: &e.campaign, Prompt: "x",
		Preset: domain.PresetProductPhoto, ReferenceFileIDs: []kernel.ID{logo.ID}}); err != nil {
		t.Fatalf("brand logo: %v", err)
	}

	// Non-image reference → 415.
	doc := e.files.Upload(e.creator.UserID, "application/pdf", []byte("%PDF-1.4 test"), nil)
	_, err = e.svc.Generate(context.Background(), e.creator, usecase.GenerateInput{CampaignID: &e.campaign, Prompt: "x",
		Preset: domain.PresetProductPhoto, ReferenceFileIDs: []kernel.ID{doc.ID}})
	if de, ok := kernel.AsDomainError(err); !ok || de.Code != kernel.CodeUnsupportedMediaType {
		t.Fatalf("pdf reference: %v", err)
	}

	// Unknown file → 404.
	_, err = e.svc.Generate(context.Background(), e.creator, usecase.GenerateInput{CampaignID: &e.campaign, Prompt: "x",
		Preset: domain.PresetProductPhoto, ReferenceFileIDs: []kernel.ID{kernel.NewID()}})
	if !errors.Is(err, kernel.ErrNotFound) {
		t.Fatalf("unknown reference: %v", err)
	}
}

func TestGenerateOverBudgetPerJob(t *testing.T) {
	e := newEnv(t)
	e.budget.PerJob["image"] = kernel.USD("0.10")
	_, err := e.svc.Generate(context.Background(), e.creator, usecase.GenerateInput{CampaignID: &e.campaign, Prompt: "x",
		Preset: domain.PresetPoster, Count: 4})
	if !errors.Is(err, kernel.ErrJobBudgetExceeded) {
		t.Fatalf("err %v", err)
	}
	// Creators may not override; leads may.
	_, err = e.svc.Generate(context.Background(), e.creator, usecase.GenerateInput{CampaignID: &e.campaign, Prompt: "x",
		Preset: domain.PresetPoster, Count: 4, BudgetOverride: true})
	if !errors.Is(err, kernel.ErrForbidden) {
		t.Fatalf("creator override: %v", err)
	}
	if _, err := e.svc.Generate(context.Background(), testutil.Actor(kernel.RoleMarketingLead), usecase.GenerateInput{CampaignID: &e.campaign,
		Prompt: "x", Preset: domain.PresetPoster, Count: 4, BudgetOverride: true}); err != nil {
		t.Fatalf("lead override: %v", err)
	}
}

func TestProcessCount4CreatesOneAssetWithFourFiles(t *testing.T) {
	e := newEnv(t)
	res := e.generate(t, usecase.GenerateInput{Preset: domain.PresetPoster, Count: 4})
	out := e.run(t, res.JobID)
	if len(out.FileIDs) != 4 || out.VersionNo != 1 {
		t.Fatalf("out %+v", out)
	}
	if len(e.assets.Assets) != 1 {
		t.Fatalf("assets %d", len(e.assets.Assets))
	}
	a := e.assets.Assets[out.AssetID]
	if a.Type != assetuc.TypeImage || a.Subtype != "poster" || a.Status != assetuc.StatusDraft || *a.CampaignID != e.campaign {
		t.Fatalf("asset %+v", a)
	}
	v := e.assets.Versions[out.AssetID][0]
	var variants, thumbs int
	for _, f := range v.Files {
		switch f.Role {
		case assetuc.RoleVariant:
			variants++
		case assetuc.RoleThumbnail:
			thumbs++
		}
	}
	if variants != 4 || thumbs != 4 {
		t.Fatalf("files %+v", v.Files)
	}
	c := content(t, v)
	if !c.RequiresTypographyCheck || len(c.FileIDs) != 4 || c.Preset != domain.PresetPoster {
		t.Fatalf("content %+v", c)
	}
	// Stored privately with job provenance; thumbnails ≤ 384px point to their image.
	f, _ := e.files.Get(context.Background(), out.FileIDs[0])
	if f.Source != "generated" || f.Kind != "image" || f.CreatedBy == nil || *f.CreatedBy != e.creator.UserID {
		t.Fatalf("file %+v", f)
	}
	if len(e.budget.Usage) != 1 || e.budget.Usage[0].Usage.Images != 4 || e.budget.Usage[0].JobID != res.JobID {
		t.Fatalf("usage %+v", e.budget.Usage)
	}
	// Replay after success does not call the provider again.
	calls := e.ai.calls
	e.run(t, res.JobID)
	if e.ai.calls != calls {
		t.Fatal("provider called twice")
	}
}

func TestProcessPassesReferencesFromStorage(t *testing.T) {
	e := newEnv(t)
	data := solidPNG(8, 8)
	ref := e.files.Upload(e.creator.UserID, "image/png", data, nil)
	res := e.generate(t, usecase.GenerateInput{Preset: domain.PresetProductPhoto, ReferenceFileIDs: []kernel.ID{ref.ID}})
	e.run(t, res.JobID)
	if len(e.ai.lastGen.ReferenceImages) != 1 || string(e.ai.lastGen.ReferenceImages[0].Data) != string(data) {
		t.Fatal("reference not loaded from storage")
	}
}

func TestProcessProviderErrorIsReturned(t *testing.T) {
	e := newEnv(t)
	res := e.generate(t, usecase.GenerateInput{})
	e.ai.err = &ports.ProviderError{Kind: ports.ErrKindTransient, Provider: "fake", Message: "busy"}
	j := e.jobs.Jobs[res.JobID]
	spec := e.jobs.Specs[res.JobID]
	_, err := e.svc.Process(context.Background(), stepCtx(j, spec))
	var pe *ports.ProviderError
	if !errors.As(err, &pe) || len(e.assets.Assets) != 0 {
		t.Fatalf("err %v", err)
	}
}

func TestVariationsAndEdits(t *testing.T) {
	e := newEnv(t)
	gen := e.run(t, e.generate(t, usecase.GenerateInput{Preset: domain.PresetSocialPost, Count: 1}).JobID)
	src := gen.FileIDs[0]

	vres, err := e.svc.Vary(context.Background(), e.creator, gen.AssetID, usecase.VariationInput{FileID: src, Count: 2})
	if err != nil {
		t.Fatal(err)
	}
	vout := e.run(t, vres.JobID)
	if vout.AssetID != gen.AssetID || vout.VersionNo != 2 || len(vout.FileIDs) != 2 {
		t.Fatalf("variation %+v", vout)
	}
	for _, id := range vout.FileIDs {
		f, _ := e.files.Get(context.Background(), id)
		if f.ParentFileID == nil || *f.ParentFileID != src {
			t.Fatalf("parent_file_id %+v", f.ParentFileID)
		}
	}
	v2 := content(t, e.assets.Versions[gen.AssetID][1])
	if len(v2.FileIDs) != 3 { // original + 2 variants
		t.Fatalf("v2 files %v", v2.FileIDs)
	}

	eres, err := e.svc.Edit(context.Background(), e.creator, gen.AssetID, usecase.EditInput{FileID: src, Prompt: "Đổi nền sang màu xanh"})
	if err != nil {
		t.Fatal(err)
	}
	eout := e.run(t, eres.JobID)
	if eout.VersionNo != 3 || e.ai.lastEd.Prompt != "Đổi nền sang màu xanh" {
		t.Fatalf("edit %+v", eout)
	}

	// Another creator's asset → 404 (IDOR); file not in the asset → 400.
	other := testutil.Actor(kernel.RoleContentCreator)
	if _, err := e.svc.Vary(context.Background(), other, gen.AssetID, usecase.VariationInput{FileID: src, Count: 1}); !errors.Is(err, kernel.ErrNotFound) {
		t.Fatalf("idor: %v", err)
	}
	if _, err := e.svc.Vary(context.Background(), e.creator, gen.AssetID, usecase.VariationInput{FileID: kernel.NewID(), Count: 1}); !errors.Is(err, kernel.ErrValidation) {
		t.Fatalf("foreign file: %v", err)
	}
	// Edit needs a prompt.
	if _, err := e.svc.Edit(context.Background(), e.creator, gen.AssetID, usecase.EditInput{FileID: src}); !errors.Is(err, kernel.ErrValidation) {
		t.Fatalf("edit without prompt: %v", err)
	}
}

func TestEditUnsupportedByModel(t *testing.T) {
	e := newEnv(t)
	gen := e.run(t, e.generate(t, usecase.GenerateInput{}).JobID)
	e.providers.Models[1].Capabilities.SupportsEdit = false
	_, err := e.svc.Edit(context.Background(), e.creator, gen.AssetID, usecase.EditInput{FileID: gen.FileIDs[0], Prompt: "x"})
	if de, ok := kernel.AsDomainError(err); !ok || de.Code != kernel.CodeCapabilityNotSupported {
		t.Fatalf("err %v", err)
	}
}

func TestListBatchesAndReferences(t *testing.T) {
	e := newEnv(t)
	res := e.generate(t, usecase.GenerateInput{Count: 2})
	page, err := e.svc.ListBatches(context.Background(), e.creator, usecase.BatchFilter{CampaignID: &e.campaign}, kernel.PageRequest{Limit: 10})
	if err != nil || len(page.Items) != 1 || page.Items[0].Asset != nil || page.Items[0].Job.Status != "queued" {
		t.Fatalf("before run: %+v %v", page.Items, err)
	}
	e.run(t, res.JobID)
	page, err = e.svc.ListBatches(context.Background(), e.creator, usecase.BatchFilter{CampaignID: &e.campaign}, kernel.PageRequest{Limit: 10})
	if err != nil || page.Items[0].Asset == nil || len(page.Items[0].Asset.Images) != 2 || page.Items[0].Asset.Images[0].URL == "" {
		t.Fatalf("after run: %+v %v", page.Items, err)
	}
	if page.Items[0].Asset.Images[0].ThumbnailFileID == nil {
		t.Fatal("thumbnail missing")
	}
	// Non-member → 404.
	if _, err := e.svc.ListBatches(context.Background(), testutil.Actor(kernel.RoleContentCreator), usecase.BatchFilter{CampaignID: &e.campaign}, kernel.PageRequest{Limit: 10}); !errors.Is(err, kernel.ErrNotFound) {
		t.Fatalf("non-member: %v", err)
	}

	logo := e.files.Upload(kernel.NewID(), "image/png", solidPNG(8, 8), nil)
	e.brands.Assets[kernel.NewID()] = branduc.BrandAsset{ID: kernel.NewID(), BrandID: e.brand.BrandID, FileID: logo.ID, AssetType: "logo", Label: "Logo"}
	refs, err := e.svc.ListReferences(context.Background(), e.creator, usecase.ReferenceFilter{CampaignID: &e.campaign})
	if err != nil {
		t.Fatal(err)
	}
	var brand, library int
	for _, r := range refs {
		switch r.Source {
		case usecase.RefSourceBrand:
			brand++
		case usecase.RefSourceLibrary:
			library++
		}
	}
	if brand != 1 || library != 2 {
		t.Fatalf("refs %+v", refs)
	}
}

func TestEstimate(t *testing.T) {
	e := newEnv(t)
	est, err := e.svc.Estimate(context.Background(), e.creator, usecase.EstimateInput{CampaignID: &e.campaign, Count: 4, Quality: "high"})
	if err != nil {
		t.Fatal(err)
	}
	if !est.Total.Amount.Equal(decimal.RequireFromString("0.68")) || est.RequiresOverride || len(est.Lines) != 1 {
		t.Fatalf("estimate %+v", est)
	}
	e.budget.PerJob["image"] = kernel.USD("0.50")
	est, _ = e.svc.Estimate(context.Background(), e.creator, usecase.EstimateInput{CampaignID: &e.campaign, Count: 4, Quality: "high"})
	if !est.RequiresOverride {
		t.Fatal("override not flagged")
	}
}

func stepCtx(j *jobuc.JobView, spec jobuc.JobSpec) *jobtest.StepContext {
	return &jobtest.StepContext{JobInfo: jobuc.JobInfo{ID: j.ID, Type: j.Type, CreatedBy: j.CreatedBy, CampaignID: j.CampaignID},
		ID: j.Steps[0].ID, Key: spec.Steps[0].Key, Type: spec.Steps[0].Type, InputJSON: spec.Steps[0].Input}
}
