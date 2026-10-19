package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/be/acs/api/internal/modules/brand/adapter/postgres"
	"github.com/be/acs/api/internal/modules/brand/domain"
	"github.com/be/acs/api/internal/modules/brand/usecase"
	db "github.com/be/acs/api/internal/platform/postgres"
	"github.com/be/acs/api/internal/shared/kernel"
	"github.com/be/acs/api/internal/testutil/pgtest"
)

func TestBrandRepository(t *testing.T) {
	pool := pgtest.Postgres(t)
	ctx := context.Background()
	repo, tx := postgres.NewRepository(pool), db.NewTxManager(pool)
	now := time.Now().UTC().Truncate(time.Microsecond)
	var uid kernel.ID
	if err := pool.QueryRow(ctx, `INSERT INTO users (email, name) VALUES ('lead@acs.local','Lead') RETURNING id`).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	b := &domain.Brand{ID: kernel.NewID(), Name: "Sao Mai", CreatedBy: &uid, CreatedAt: now, UpdatedAt: now}
	b.Current = domain.Version{ID: kernel.NewID(), BrandID: b.ID, VersionNo: 1, Profile: domain.Profile{Colors: []domain.Color{{Name: "p", Hex: "#0055FF"}}}.Normalize(), CreatedBy: &uid, CreatedAt: now}
	if err := tx.WithinTx(ctx, func(ctx context.Context) error { return repo.CreateBrand(ctx, b) }); err != nil {
		t.Fatal(err)
	}
	dup := &domain.Brand{ID: kernel.NewID(), Name: "sao mai", CreatedAt: now}
	dup.Current = domain.Version{ID: kernel.NewID(), BrandID: dup.ID, VersionNo: 1, CreatedAt: now}
	if err := tx.WithinTx(ctx, func(ctx context.Context) error { return repo.CreateBrand(ctx, dup) }); !errors.Is(err, kernel.ErrConflict) {
		t.Fatalf("duplicate name: %v", err)
	}
	got, err := repo.GetBrandForUpdate(ctx, b.ID)
	if err != nil || got.Current.VersionNo != 1 || got.Current.Profile.Colors[0].Hex != "#0055FF" {
		t.Fatal(err, got)
	}
	v2 := got.NextVersion(domain.Profile{ToneOfVoice: []string{"formal"}}.Normalize(), uid, now)
	if err := repo.AddVersion(ctx, v2); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddVersion(ctx, domain.Version{ID: kernel.NewID(), BrandID: b.ID, VersionNo: 2, CreatedAt: now}); !errors.Is(err, kernel.ErrVersionConflict) {
		t.Fatalf("same version_no: %v", err)
	}
	vs, err := repo.ListVersions(ctx, b.ID, kernel.PageRequest{Limit: 1})
	if err != nil || len(vs.Items) != 1 || vs.Items[0].VersionNo != 2 || vs.NextCursor == nil {
		t.Fatal(err, vs)
	}
	vs, _ = repo.ListVersions(ctx, b.ID, kernel.PageRequest{Limit: 1, Cursor: *vs.NextCursor})
	if len(vs.Items) != 1 || vs.Items[0].VersionNo != 1 {
		t.Fatal(vs)
	}
	page, err := repo.ListBrands(ctx, usecase.BrandFilter{Q: "sao"}, kernel.PageRequest{Limit: 10})
	if err != nil || len(page.Items) != 1 || page.Items[0].Current.VersionNo != 2 {
		t.Fatal(err, page)
	}

	p := &domain.Product{ID: kernel.NewID(), BrandID: b.ID, Name: "Khóa A", Status: domain.ProductActive, CreatedAt: now, UpdatedAt: now,
		Facts: domain.Facts{Offers: []domain.Offer{{Text: "Giảm 20%", ValidTo: "2026-12-31"}}}.Normalize()}
	if err := repo.CreateProduct(ctx, p); err != nil {
		t.Fatal(err)
	}
	gp, err := repo.GetProduct(ctx, p.ID)
	if err != nil || gp.Facts.Offers[0].ValidTo != "2026-12-31" {
		t.Fatal(err, gp)
	}
	gp.Status = domain.ProductArchived
	if err := repo.UpdateProduct(ctx, gp); err != nil {
		t.Fatal(err)
	}
	if act, _ := repo.ListActiveProducts(ctx, b.ID); len(act) != 0 {
		t.Fatal("archived product listed as active")
	}

	var fid kernel.ID
	if err := pool.QueryRow(ctx, `INSERT INTO asset_files (storage_key, mime_type, size_bytes, kind, upload_status, source)
		VALUES ('k/1','audio/mpeg',10,'audio','ready','upload') RETURNING id`).Scan(&fid); err != nil {
		t.Fatal(err)
	}
	noLicense := &domain.Asset{ID: kernel.NewID(), BrandID: b.ID, FileID: fid, AssetType: domain.AssetMusic, CreatedAt: now}
	if err := repo.CreateAsset(ctx, noLicense); err == nil {
		t.Fatal("DB must reject music without license")
	}
	lic, _ := json.Marshal(domain.License{Source: "Artlist", LicenseType: "commercial"})
	a := &domain.Asset{ID: kernel.NewID(), BrandID: b.ID, FileID: fid, AssetType: domain.AssetMusic, License: lic, CreatedAt: now}
	if err := repo.CreateAsset(ctx, a); err != nil {
		t.Fatal(err)
	}
	as, err := repo.ListAssets(ctx, b.ID, []string{domain.AssetMusic})
	if err != nil || len(as) != 1 || !as[0].LicenseUsableAt(now) {
		t.Fatal(err, as)
	}
	if err := repo.DeleteAsset(ctx, b.ID, a.ID); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteAsset(ctx, b.ID, a.ID); !errors.Is(err, kernel.ErrNotFound) {
		t.Fatal(err)
	}
	if err := repo.SoftDeleteBrand(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetBrand(ctx, b.ID); !errors.Is(err, kernel.ErrNotFound) {
		t.Fatal(err)
	}
}
