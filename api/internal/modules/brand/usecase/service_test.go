package usecase_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/be/acs/api/internal/modules/brand/brandtest"
	"github.com/be/acs/api/internal/modules/brand/domain"
	"github.com/be/acs/api/internal/modules/brand/usecase"
	"github.com/be/acs/api/internal/shared/kernel"
	"github.com/be/acs/api/internal/testutil"
)

type env struct {
	svc   *usecase.Service
	files *brandtest.Files
	audit *testutil.Audit
	clock *testutil.Clock
	lead  kernel.Actor
}

func newEnv() *env {
	e := &env{files: brandtest.NewFiles(), audit: &testutil.Audit{}, clock: testutil.NewClock(), lead: testutil.Actor(kernel.RoleMarketingLead)}
	e.svc = &usecase.Service{Tx: &testutil.Tx{}, Repo: brandtest.NewRepo(), Clock: e.clock, Audit: e.audit, Files: e.files}
	return e
}

var ctx = context.Background()

func (e *env) brand(t *testing.T) *domain.Brand {
	t.Helper()
	b, err := e.svc.CreateBrand(ctx, e.lead, usecase.CreateBrandInput{Name: "Anh ngữ Sao Mai",
		Profile: domain.Profile{Colors: []domain.Color{{Name: "primary", Hex: "#0055FF"}}, ForbiddenWords: []string{"rẻ nhất"}}})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestCreateBrandRBACAndValidation(t *testing.T) {
	e := newEnv()
	for _, role := range []string{kernel.RoleContentCreator, kernel.RoleReviewer} {
		if _, err := e.svc.CreateBrand(ctx, testutil.Actor(role), usecase.CreateBrandInput{Name: "x"}); !errors.Is(err, kernel.ErrForbidden) {
			t.Errorf("%s: %v", role, err)
		}
	}
	_, err := e.svc.CreateBrand(ctx, e.lead, usecase.CreateBrandInput{Name: "", Profile: domain.Profile{Colors: []domain.Color{{Name: "p", Hex: "#GGG"}}}})
	if !errors.Is(err, kernel.ErrValidation) {
		t.Fatal(err)
	}
	b := e.brand(t)
	if b.Current.VersionNo != 1 {
		t.Fatal(b.Current.VersionNo)
	}
	if _, err := e.svc.CreateBrand(ctx, e.lead, usecase.CreateBrandInput{Name: "anh ngữ sao mai"}); !errors.Is(err, kernel.ErrConflict) {
		t.Fatalf("duplicate: %v", err)
	}
	if got := e.audit.Actions(); len(got) != 1 || got[0] != "brand.created" {
		t.Fatal(got)
	}
}

// ACS-020 AC: two people edit concurrently → the later one gets 409 VERSION_CONFLICT.
func TestUpdateBrandVersioningAndOptimisticLock(t *testing.T) {
	e := newEnv()
	b := e.brand(t)
	alice, bob := e.lead, testutil.Actor(kernel.RoleAdmin)
	p1 := b.Current.Profile
	p1.ToneOfVoice = []string{"friendly"}
	b2, err := e.svc.UpdateBrand(ctx, alice, b.ID, usecase.UpdateBrandInput{Profile: &p1, ExpectedVersionNo: 1})
	if err != nil || b2.Current.VersionNo != 2 {
		t.Fatal(err, b2)
	}
	p2 := b.Current.Profile
	p2.ToneOfVoice = []string{"formal"}
	_, err = e.svc.UpdateBrand(ctx, bob, b.ID, usecase.UpdateBrandInput{Profile: &p2, ExpectedVersionNo: 1})
	if !errors.Is(err, kernel.ErrVersionConflict) {
		t.Fatalf("want VERSION_CONFLICT, got %v", err)
	}
	// Same profile → no new version; rename only → no new version.
	name := "Sao Mai English"
	b3, err := e.svc.UpdateBrand(ctx, bob, b.ID, usecase.UpdateBrandInput{Name: &name, Profile: &p1, ExpectedVersionNo: 2})
	if err != nil || b3.Current.VersionNo != 2 || b3.Name != name {
		t.Fatal(err, b3)
	}
	vs, _ := e.svc.ListVersions(ctx, testutil.Actor(kernel.RoleReviewer), b.ID, kernel.PageRequest{})
	if len(vs.Items) != 2 || vs.Items[0].VersionNo != 2 {
		t.Fatal(vs.Items)
	}
	if _, err := e.svc.UpdateBrand(ctx, alice, b.ID, usecase.UpdateBrandInput{Name: &name}); !errors.Is(err, kernel.ErrValidation) {
		t.Fatal("expected_version_no must be required")
	}
	bad := domain.Profile{Colors: []domain.Color{{Name: "x", Hex: "123456"}}}
	if _, err := e.svc.UpdateBrand(ctx, alice, b.ID, usecase.UpdateBrandInput{Profile: &bad, ExpectedVersionNo: 2}); !errors.Is(err, kernel.ErrValidation) {
		t.Fatal("hex must be validated")
	}
}

// ACS-022 AC: expired offers are flagged and never reach the prompt.
func TestBrandContextExcludesExpiredOffersAndUnverifiedProof(t *testing.T) {
	e := newEnv()
	b := e.brand(t)
	name := "Khóa giao tiếp"
	facts := domain.Facts{
		USP:   []string{"12 tuần"},
		Price: &domain.Price{Amount: decimal.NewFromInt(2990000), Currency: "VND"},
		Offers: []domain.Offer{
			{Text: "Giảm 10% (hết hạn)", ValidTo: "2026-10-01"},
			{Text: "Giảm 20% đến 31/12", ValidFrom: "2026-10-01", ValidTo: "2026-12-31"},
		},
		Proof: []domain.Proof{{Type: "testimonial", Text: "Tốt", Verified: true}, {Type: "statistic", Text: "99%"}},
	}
	p, err := e.svc.CreateProduct(ctx, e.lead, b.ID, usecase.ProductInput{Name: &name, Facts: &facts})
	if err != nil {
		t.Fatal(err)
	}
	if p.Facts.Offers[0].StatusAt(e.clock.Now()) != domain.OfferExpired {
		t.Fatal("expired offer must be flagged")
	}
	bc, err := e.svc.GetBrandContext(ctx, usecase.BrandContextRequest{BrandID: b.ID, ProductIDs: []kernel.ID{p.ID}})
	if err != nil {
		t.Fatal(err)
	}
	pf := bc.Products[0]
	if len(pf.Offers) != 1 || pf.Offers[0].Text != "Giảm 20% đến 31/12" || len(pf.Proof) != 1 || !pf.Price.Amount.Equal(decimal.NewFromInt(2990000)) {
		t.Fatalf("%+v", pf)
	}
	if bc.ForbiddenWords[0] != "rẻ nhất" || bc.Colors[0].Hex != "#0055FF" || bc.VersionNo != 1 {
		t.Fatalf("%+v", bc)
	}
	// Archived products and products of other brands → NOT_FOUND.
	other, _ := e.svc.CreateBrand(ctx, e.lead, usecase.CreateBrandInput{Name: "Other"})
	if _, err := e.svc.GetBrandContext(ctx, usecase.BrandContextRequest{BrandID: other.ID, ProductIDs: []kernel.ID{p.ID}}); !errors.Is(err, kernel.ErrNotFound) {
		t.Fatal(err)
	}
	archived := domain.ProductArchived
	if _, err := e.svc.UpdateProduct(ctx, e.lead, p.ID, usecase.ProductInput{Status: &archived}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.GetBrandContext(ctx, usecase.BrandContextRequest{BrandID: b.ID, ProductIDs: []kernel.ID{p.ID}}); !errors.Is(err, kernel.ErrNotFound) {
		t.Fatal(err)
	}
}

func TestBrandContextPinnedVersion(t *testing.T) {
	e := newEnv()
	b := e.brand(t)
	v1 := b.Current.ID
	p := b.Current.Profile
	p.ForbiddenWords = []string{"cam kết 100%"}
	if _, err := e.svc.UpdateBrand(ctx, e.lead, b.ID, usecase.UpdateBrandInput{Profile: &p, ExpectedVersionNo: 1}); err != nil {
		t.Fatal(err)
	}
	bc, err := e.svc.GetBrandContext(ctx, usecase.BrandContextRequest{BrandID: b.ID, BrandVersionID: &v1})
	if err != nil || bc.VersionNo != 1 || bc.ForbiddenWords[0] != "rẻ nhất" {
		t.Fatal(err, bc)
	}
	cur, _ := e.svc.GetBrandContext(ctx, usecase.BrandContextRequest{BrandID: b.ID})
	if cur.VersionNo != 2 {
		t.Fatal(cur.VersionNo)
	}
	if err := e.svc.DeleteBrand(ctx, e.lead, b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.GetBrandContext(ctx, usecase.BrandContextRequest{BrandID: b.ID}); !errors.Is(err, kernel.ErrNotFound) {
		t.Fatal("deleted brand must be NOT_FOUND")
	}
}

// ACS-023 AC: music without a license cannot be used in a video.
func TestMusicAssetsRequireLicense(t *testing.T) {
	e := newEnv()
	b := e.brand(t)
	audio := e.files.Add("audio", "audio/mpeg", e.lead.UserID)
	if _, err := e.svc.AddAsset(ctx, e.lead, b.ID, usecase.AddAssetInput{FileID: audio, AssetType: domain.AssetMusic}); !errors.Is(err, kernel.ErrValidation) {
		t.Fatalf("music without license: %v", err)
	}
	lic := &domain.License{Source: "Artlist", LicenseType: "commercial", ExpiresAt: "2026-12-31"}
	a, err := e.svc.AddAsset(ctx, e.lead, b.ID, usecase.AddAssetInput{FileID: audio, AssetType: domain.AssetMusic, Label: "Upbeat", License: lic})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.RequireLicensedMusic(ctx, b.ID, a.ID); err != nil {
		t.Fatal(err)
	}
	e.clock.T = e.clock.T.AddDate(0, 3, 0) // license expired
	if _, err := e.svc.RequireLicensedMusic(ctx, b.ID, a.ID); !errors.Is(err, kernel.ErrValidation) {
		t.Fatalf("expired license: %v", err)
	}
	if _, err := e.svc.RequireLicensedMusic(ctx, kernel.NewID(), a.ID); !errors.Is(err, kernel.ErrNotFound) {
		t.Fatal("other brand must be NOT_FOUND")
	}
	// Logo must be an image; an audio file is rejected.
	if _, err := e.svc.AddAsset(ctx, e.lead, b.ID, usecase.AddAssetInput{FileID: audio, AssetType: domain.AssetLogo}); !errors.Is(err, kernel.ErrUnsupportedMedia) {
		t.Fatal(err)
	}
	img := e.files.Add("image", "image/png", e.lead.UserID)
	logo, err := e.svc.AddAsset(ctx, e.lead, b.ID, usecase.AddAssetInput{FileID: img, AssetType: domain.AssetLogo})
	if err != nil {
		t.Fatal(err)
	}
	views, err := e.svc.ListAssets(ctx, testutil.Actor(kernel.RoleReviewer), b.ID, []string{domain.AssetLogo})
	if err != nil || len(views) != 1 || views[0].URL == nil {
		t.Fatal(err, views)
	}
	list, _ := e.svc.ListBrandAssets(ctx, b.ID, domain.AssetMusic)
	if len(list) != 1 || list[0].LicenseValid {
		t.Fatal(list)
	}
	var l domain.License
	_ = json.Unmarshal(list[0].License, &l)
	if l.Source != "Artlist" {
		t.Fatal(l)
	}
	if err := e.svc.DeleteAsset(ctx, e.lead, kernel.NewID(), logo.ID); !errors.Is(err, kernel.ErrNotFound) {
		t.Fatal(err)
	}
	if err := e.svc.DeleteAsset(ctx, e.lead, b.ID, logo.ID); err != nil {
		t.Fatal(err)
	}
}

func TestAddAssetFileOfAnotherUserIsNotFound(t *testing.T) {
	e := newEnv()
	b := e.brand(t)
	// A file uploaded by a creator may be attached by a Lead (Lead may use all files).
	creatorFile := e.files.Add("image", "image/png", kernel.NewID())
	if _, err := e.svc.AddAsset(ctx, e.lead, b.ID, usecase.AddAssetInput{FileID: creatorFile, AssetType: domain.AssetReferenceImage}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.AddAsset(ctx, e.lead, b.ID, usecase.AddAssetInput{FileID: kernel.NewID(), AssetType: domain.AssetReferenceImage}); !errors.Is(err, kernel.ErrNotFound) {
		t.Fatal(err)
	}
}
