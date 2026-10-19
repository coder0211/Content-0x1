package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/be/acs/api/internal/modules/provider/adapter/postgres"
	"github.com/be/acs/api/internal/modules/provider/domain"
	"github.com/be/acs/api/internal/modules/provider/usecase"
	db "github.com/be/acs/api/internal/platform/postgres"
	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
	"github.com/be/acs/api/internal/testutil"
	"github.com/be/acs/api/internal/testutil/pgtest"
)

func TestProviderRepositoryAgainstSeed(t *testing.T) {
	pool := pgtest.Postgres(t)
	ctx := context.Background()
	svc := &usecase.Service{Tx: db.NewTxManager(pool), Repo: postgres.NewRepository(pool), Clock: ports.SystemClock{}, Audit: &testutil.Audit{}}
	admin := testutil.Actor(kernel.RoleAdmin)
	// Template versions reference users(id) via created_by.
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, name, password_hash) VALUES ($1, 'provider-admin@acs.local', 'Admin', 'x')`, admin.UserID); err != nil {
		t.Fatal(err)
	}

	ps, err := svc.AdminListProviders(ctx, admin)
	if err != nil || len(ps) != 6 {
		t.Fatalf("seed 0011+0070: %v %d", err, len(ps))
	}
	m, err := svc.ResolveModel(ctx, ports.AIKindText, "claude-sonnet-5-5")
	if err != nil || m.ProviderCode != "anthropic" || m.Capabilities.MaxOutputTokens != 64000 {
		t.Fatalf("%v %+v", err, m)
	}
	if d, err := svc.ResolveModel(ctx, ports.AIKindVideo, ""); err != nil || d.ProviderCode != "fake" {
		t.Fatalf("fake stays the local default: %v %+v", err, d)
	}

	// Pricing: append a version, old rows closed but unchanged; DB refuses edits.
	list, _ := svc.ListPricing(ctx, admin, m.ModelID)
	if len(list) != 1 || list[0].Version != "2026-10" || len(list[0].Items) != 4 {
		t.Fatalf("%+v", list)
	}
	if _, err := svc.AddPricingVersion(ctx, admin, m.ModelID, usecase.AddPricingInput{Version: "2026-11",
		Items: []domain.PriceItem{{Unit: "input_token", UnitPrice: decimal.RequireFromString("0.0000019")}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE model_pricing SET unit_price = 0 WHERE pricing_version = '2026-10'`); err == nil {
		t.Fatal("pricing rows must be immutable")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM model_pricing WHERE pricing_version = '2026-11'`); err == nil {
		t.Fatal("pricing rows must not be deleted")
	}

	// Default switch respects the partial unique index.
	yes := true
	mv, err := svc.UpdateModel(ctx, admin, m.ModelID, usecase.UpdateModelInput{IsDefault: &yes})
	if err != nil || !mv.IsDefault {
		t.Fatal(err)
	}

	// Templates: versions, single active, immutable text.
	v1, err := svc.CreateTemplateVersion(ctx, admin, usecase.CreateTemplateInput{Code: "test.sample", SystemPrompt: "s", UserTemplate: "u",
		OutputSchema: json.RawMessage(`{"type":"object"}`), Activate: true})
	if err != nil {
		t.Fatal(err)
	}
	v2, err := svc.CreateTemplateVersion(ctx, admin, usecase.CreateTemplateInput{Code: "test.sample", SystemPrompt: "s2", UserTemplate: "u2"})
	if err != nil || v2.Version != 2 {
		t.Fatal(err)
	}
	if _, err := svc.ActivateTemplate(ctx, admin, v2.ID); err != nil {
		t.Fatal(err)
	}
	if a, _ := svc.ActivePromptTemplate(ctx, "test.sample"); a.ID != v2.ID {
		t.Fatal("active")
	}
	if _, err := pool.Exec(ctx, `UPDATE prompt_templates SET system_prompt = 'x' WHERE id = $1`, v1.ID); err == nil {
		t.Fatal("template text must be immutable")
	}
	if _, err := svc.UpdateProvider(ctx, admin, kernel.NewID(), usecase.UpdateProviderInput{}); !errors.Is(err, kernel.ErrNotFound) {
		t.Fatal(err)
	}
}
